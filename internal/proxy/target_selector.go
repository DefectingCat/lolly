package proxy

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/valyala/fasthttp"
	glua "github.com/yuin/gopher-lua"
	"rua.plus/lolly/internal/loadbalance"
	"rua.plus/lolly/internal/lua"
	"rua.plus/lolly/internal/netutil"
)

// selectByLua 使用 Lua 脚本选择后端目标。
//
// 执行配置的 Lua 脚本，脚本可通过 ngx.balancer.set_current_peer() 选择目标。
// 如果 Lua 脚本执行失败或未调用 set_current_peer，返回 nil 表示需要使用 fallback 算法。
//
// 参数：
//   - ctx: FastHTTP 请求上下文
//   - targets: 候选目标列表
//
// 返回值：
//   - *loadbalance.Target: Lua 脚本选中的目标，nil 表示未选择
//   - error: Lua 执行失败时返回错误
func (p *Proxy) selectByLua(ctx *fasthttp.RequestCtx, targets []*loadbalance.Target) (*loadbalance.Target, error) {
	clientIP := netutil.ExtractClientIP(ctx)

	bctx := &lua.BalancerContext{
		Targets:  targets,
		ClientIP: clientIP,
		Retries:  p.config.NextUpstream.Tries,
	}

	// 创建 Lua 协程
	coro, err := p.luaEngine.NewCoroutine(ctx)
	if err != nil {
		return nil, fmt.Errorf("create lua coroutine: %w", err)
	}
	defer coro.Close()

	// 注册 balancer API
	L := coro.Co
	ngx, ok := L.GetGlobal("ngx").(*glua.LTable)
	if !ok {
		return nil, fmt.Errorf("global 'ngx' is not an LTable")
	}
	lua.RegisterBalancerAPI(L, bctx, ngx)

	// 设置超时
	timeout := p.config.BalancerByLua.Timeout
	if timeout <= 0 {
		timeout = 100 * time.Millisecond
	}

	// 执行脚本（带超时）
	execCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	coro.ExecutionContext = execCtx

	err = coro.ExecuteFile(p.config.BalancerByLua.Script)
	if err != nil {
		return nil, fmt.Errorf("execute lua script: %w", err)
	}

	// 检查是否调用了 set_current_peer
	if !bctx.IsSelected() {
		return nil, nil // 未选择，返回 nil 表示需使用 fallback
	}

	return bctx.Selected, nil
}

// selectByFallback 使用 fallback 负载均衡算法选择目标。
//
// 当 Lua balancer 执行失败或未选择目标时使用。
// 对于 IPHash 算法，会自动提取客户端 IP 进行哈希选择。
//
// 参数：
//   - ctx: FastHTTP 请求上下文
//   - targets: 候选目标列表
//
// 返回值：
//   - *loadbalance.Target: fallback 算法选中的目标
func (p *Proxy) selectByFallback(ctx *fasthttp.RequestCtx, targets []*loadbalance.Target) *loadbalance.Target {
	p.mu.RLock()
	balancer := p.fallbackBalancer
	p.mu.RUnlock()

	if ipHash, ok := balancer.(*loadbalance.IPHash); ok {
		clientIP := netutil.ExtractClientIP(ctx)
		return ipHash.SelectByIP(targets, clientIP)
	}

	return balancer.Select(targets)
}

// selectByBalancer 使用主负载均衡器选择目标。
//
// 对于特殊算法（IPHash、ConsistentHash），会从请求上下文中提取
// 相应的哈希键（客户端 IP、URI、自定义 Header 等）。
//
// 参数：
//   - ctx: FastHTTP 请求上下文
//   - targets: 候选目标列表
//
// 返回值：
//   - *loadbalance.Target: 主负载均衡器选中的目标
func (p *Proxy) selectByBalancer(ctx *fasthttp.RequestCtx, targets []*loadbalance.Target) *loadbalance.Target {
	p.mu.RLock()
	balancer := p.balancer
	p.mu.RUnlock()

	// 对于 IPHash 负载均衡器，提取客户端 IP
	if ipHash, ok := balancer.(*loadbalance.IPHash); ok {
		clientIP := netutil.ExtractClientIP(ctx)
		return ipHash.SelectByIP(targets, clientIP)
	}

	// 对于一致性哈希，根据 hash_key 配置选择
	if ch, ok := balancer.(*loadbalance.ConsistentHash); ok {
		hashKey := ch.GetHashKey()
		key := p.extractHashKey(ctx, hashKey)
		return ch.SelectByKey(targets, key)
	}

	return balancer.Select(targets)
}

// selectTargetExcluding 选择后端目标，排除已尝试失败的目标。
// 用于故障转移场景，避免重复选择已失败的目标。
// 如果没有可用的健康目标则返回 nil。
func (p *Proxy) selectTargetExcluding(ctx *fasthttp.RequestCtx, excluded []*loadbalance.Target) *loadbalance.Target {
	p.mu.RLock()
	balancer := p.balancer
	targets := p.targets
	p.mu.RUnlock()

	if len(targets) == 0 {
		return nil
	}

	// 对于 IPHash 负载均衡器，提取客户端 IP
	if ipHash, ok := balancer.(*loadbalance.IPHash); ok {
		clientIP := netutil.ExtractClientIP(ctx)
		return ipHash.SelectExcludingByIP(targets, excluded, clientIP)
	}

	// 对于一致性哈希，根据 hash_key 配置选择
	if ch, ok := balancer.(*loadbalance.ConsistentHash); ok {
		hashKey := ch.GetHashKey()
		key := p.extractHashKey(ctx, hashKey)
		return ch.SelectExcludingByKey(targets, excluded, key)
	}

	return balancer.SelectExcluding(targets, excluded)
}

// extractHashKey 根据一致性哈希配置提取哈希键值。
//
// 支持的 hash_key 配置：
//   - "ip" 或 "": 使用客户端 IP 地址
//   - "uri": 使用完整请求 URI
//   - "header:NAME": 使用指定请求头的值，缺失时回退到客户端 IP
//
// 参数：
//   - ctx: FastHTTP 请求上下文
//   - hashKey: 哈希键配置
//
// 返回值：
//   - string: 提取的哈希键值
func (p *Proxy) extractHashKey(ctx *fasthttp.RequestCtx, hashKey string) string {
	switch {
	case hashKey == "ip" || hashKey == "":
		return netutil.ExtractClientIP(ctx)
	case hashKey == "uri":
		return string(ctx.RequestURI())
	case strings.HasPrefix(hashKey, "header:"):
		headerName := strings.TrimPrefix(hashKey, "header:")
		value := ctx.Request.Header.Peek(headerName)
		if len(value) > 0 {
			return string(value)
		}
		return netutil.ExtractClientIP(ctx) // fallback to IP
	default:
		return netutil.ExtractClientIP(ctx)
	}
}

// getClient 返回指定目标 URL 对应的 HostClient 连接池实例。
// 如果目标 URL 不存在于连接池中，返回 nil。
func (p *Proxy) getClient(targetURL string) *fasthttp.HostClient {
	key := targetURL
	if p.config.ProxyBind != "" {
		key = targetURL + "|" + p.config.ProxyBind
	}
	p.mu.RLock()
	client := p.clients[key]
	p.mu.RUnlock()
	return client
}
