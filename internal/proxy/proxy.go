// Package proxy 反向代理包，为 Lolly HTTP 服务器提供反向代理功能。
//
// 该包使用 fasthttp.HostClient 实现高性能反向代理，支持连接池和自动 keep-alive 管理。
// 支持负载均衡、WebSocket 转发、自定义请求头/响应头和全面的超时配置。
//
// 使用示例：
//
//	targets := []*loadbalance.Target{
//	    {URL: "http://backend1:8080", Weight: 1},
//	    {URL: "http://backend2:8080", Weight: 2},
//	}
//	targets[0].Healthy.Store(true)
//	targets[1].Healthy.Store(true)
//
//	proxyConfig := &config.ProxyConfig{
//	    Path:        "/api",
//	    LoadBalance: "weighted_round_robin",
//	    Timeout: config.ProxyTimeout{
//	        Connect: 5 * time.Second,
//	        Read:    30 * time.Second,
//	        Write:   30 * time.Second,
//	    },
//	}
//
//	p, err := proxy.NewProxy(proxyConfig, targets)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// 使用 p.ServeHTTP 作为 fasthttp 请求处理器
//
//go:generate go test -v ./...
package proxy

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/valyala/fasthttp"
	glua "github.com/yuin/gopher-lua"
	"rua.plus/lolly/internal/cache"
	"rua.plus/lolly/internal/config"
	"rua.plus/lolly/internal/loadbalance"
	"rua.plus/lolly/internal/logging"
	"rua.plus/lolly/internal/lua"
	"rua.plus/lolly/internal/netutil"
	"rua.plus/lolly/internal/resolver"
	"rua.plus/lolly/internal/utils"
	"rua.plus/lolly/internal/variable"
)

const (
	// upstreamCache 上游缓存标识
	// 用于标记请求可直接使用缓存响应，无需转发到上游
	upstreamCache = "CACHE"

	// 负载均衡算法名称
	lbRoundRobin         = "round_robin"
	lbWeightedRoundRobin = "weighted_round_robin"
	lbLeastConn          = "least_conn"
	lbIPHash             = "ip_hash"
	lbConsistentHash     = "consistent_hash"
)

// Proxy 表示反向代理实例，负责将 HTTP 请求转发到后端目标。
//
// 它为每个后端目标管理连接池，并提供负载均衡功能。
//
// 注意事项：
//   - 所有公开方法均为并发安全
//   - 使用前需确保 targets 中至少有一个健康目标
type Proxy struct {
	balancer         loadbalance.Balancer
	fallbackBalancer loadbalance.Balancer // Lua 失败时的备用均衡器
	resolver         resolver.Resolver
	clients          map[string]*fasthttp.HostClient
	config           *config.ProxyConfig
	cache            *cache.ProxyCache
	healthChecker    *HealthChecker
	luaEngine        *lua.LuaEngine    // Lua 引擎引用
	redirectRewriter *RedirectRewriter // 重定向改写器
	stopCh           chan struct{}
	targets          []*loadbalance.Target
	mu               sync.RWMutex
	started          atomic.Bool
}

// NewProxy 使用给定的配置和后台目标创建一个新的反向代理实例。
// 它根据配置初始化负载均衡器，并为每个后端目标创建 HostClient。
//
// 参数：
//   - cfg: 代理配置，包括超时时间、请求头和负载均衡策略
//   - targets: 要代理请求的后端目标列表
//   - transportCfg: 可选的 Transport 连接池配置，nil 时使用默认值
//   - luaEngine: 可选的 Lua 引擎，用于 balancer_by_lua 功能
//
// 返回值：
//   - *Proxy: 配置完成并可处理请求的代理实例
//   - error: 初始化失败时非空（无效配置、没有健康目标等）
func NewProxy(cfg *config.ProxyConfig, targets []*loadbalance.Target, transportCfg *config.TransportConfig, luaEngine *lua.LuaEngine) (*Proxy, error) {
	if cfg == nil {
		return nil, errors.New("proxy config is nil")
	}

	if len(targets) == 0 {
		return nil, errors.New("no proxy targets provided")
	}

	// 根据配置创建负载均衡器
	balancer, err := createBalancer(cfg)
	if err != nil {
		return nil, err
	}

	// 创建 fallback 负载均衡器
	fallbackAlgo := cfg.BalancerByLua.Fallback
	if fallbackAlgo == "" {
		fallbackAlgo = lbRoundRobin
	}
	fallbackBalancer, err := createBalancerByName(fallbackAlgo, cfg)
	if err != nil {
		return nil, fmt.Errorf("create fallback balancer: %w", err)
	}

	p := &Proxy{
		targets:          targets,
		clients:          make(map[string]*fasthttp.HostClient),
		balancer:         balancer,
		fallbackBalancer: fallbackBalancer,
		config:           cfg,
		luaEngine:        luaEngine,
		stopCh:           make(chan struct{}),
	}

	// 为每个后端目标初始化 HostClient
	for _, target := range targets {
		if target.URL == "" {
			continue
		}

		client := createHostClient(target.URL, cfg.Timeout, transportCfg, cfg.ProxySSL)
		p.clients[target.URL] = client
	}

	// 初始化代理缓存（如果启用）
	if cfg.Cache.Enabled {
		rules := make([]cache.ProxyCacheRule, 0)
		if cfg.Cache.MaxAge > 0 {
			rules = append(rules, cache.ProxyCacheRule{
				Path:   cfg.Path,
				MaxAge: cfg.Cache.MaxAge,
			})
		}
		p.cache = cache.NewProxyCache(rules, cfg.Cache.CacheLock, cfg.Cache.StaleWhileRevalidate)
	}

	// 初始化重定向改写器
	rewriter, err := NewRedirectRewriter(cfg.RedirectRewrite, cfg.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to create redirect rewriter: %w", err)
	}
	p.redirectRewriter = rewriter

	return p, nil
}

// createBalancerByName 根据算法名称创建负载均衡器
func createBalancerByName(name string, cfg *config.ProxyConfig) (loadbalance.Balancer, error) {
	switch name {
	case lbRoundRobin, "":
		return loadbalance.NewRoundRobin(), nil
	case lbWeightedRoundRobin:
		return loadbalance.NewWeightedRoundRobin(), nil
	case lbLeastConn:
		return loadbalance.NewLeastConnections(), nil
	case lbIPHash:
		return loadbalance.NewIPHash(), nil
	case lbConsistentHash:
		virtualNodes := cfg.VirtualNodes
		if virtualNodes <= 0 {
			virtualNodes = 150
		}
		return loadbalance.NewConsistentHash(virtualNodes, cfg.HashKey), nil
	default:
		return nil, errors.New("unsupported load balance algorithm: " + name)
	}
}

// SetHealthChecker 设置健康检查器用于被动健康检查。
// 当代理请求失败时，将调用健康检查器的 MarkUnhealthy 方法。
func (p *Proxy) SetHealthChecker(hc *HealthChecker) {
	p.healthChecker = hc
}

// createBalancer 根据配置的算法创建负载均衡器。
func createBalancer(cfg *config.ProxyConfig) (loadbalance.Balancer, error) {
	return createBalancerByName(cfg.LoadBalance, cfg)
}

// createHostClient 为后台目标 URL 创建 fasthttp.HostClient。
func createHostClient(targetURL string, timeout config.ProxyTimeout, transportCfg *config.TransportConfig, sslCfg *config.ProxySSLConfig) *fasthttp.HostClient {
	// 从目标 URL 解析主机和协议
	// addDefaultPort=true 确保 HostClient.Addr 包含端口（host:port 格式）
	addr, isTLS := netutil.ParseTargetURL(targetURL, true)

	// 默认值
	maxIdleConnDuration := 90 * time.Second
	maxConns := 100

	// 应用 Transport 配置
	if transportCfg != nil {
		if transportCfg.IdleConnTimeout > 0 {
			maxIdleConnDuration = transportCfg.IdleConnTimeout
		}
		if transportCfg.MaxConnsPerHost > 0 {
			maxConns = transportCfg.MaxConnsPerHost
		}
	}

	client := &fasthttp.HostClient{
		Addr:                   addr,
		IsTLS:                  isTLS,
		ReadTimeout:            timeout.Read,
		WriteTimeout:           timeout.Write,
		MaxIdleConnDuration:    maxIdleConnDuration,
		MaxConns:               maxConns,
		MaxConnWaitTimeout:     timeout.Connect,
		RetryIf:                nil, // 禁用自动重试
		DisablePathNormalizing: false,
		SecureErrorLogMessage:  false,
	}

	// 上游 SSL 配置（使用原生 TLSConfig）
	if sslCfg != nil && sslCfg.Enabled && isTLS {
		tlsCfg, err := CreateTLSConfig(sslCfg, extractHostFromURL(targetURL))
		if err == nil {
			client.TLSConfig = tlsCfg
		}
	}

	return client
}

// UpstreamTiming 上游时间记录，用于捕获各种时间戳
type UpstreamTiming struct {
	start          time.Time
	connectStart   time.Time
	connectEnd     time.Time
	headerReceived time.Time
	responseEnd    time.Time
}

// NewUpstreamTiming 创建新的上游时间记录器
func NewUpstreamTiming() *UpstreamTiming {
	return &UpstreamTiming{
		start: time.Now(),
	}
}

// MarkConnectStart 标记连接开始
func (t *UpstreamTiming) MarkConnectStart() {
	t.connectStart = time.Now()
}

// MarkConnectEnd 标记连接完成
func (t *UpstreamTiming) MarkConnectEnd() {
	t.connectEnd = time.Now()
}

// MarkHeaderReceived 标记接收到响应头
func (t *UpstreamTiming) MarkHeaderReceived() {
	t.headerReceived = time.Now()
}

// MarkResponseEnd 标记响应完成
func (t *UpstreamTiming) MarkResponseEnd() {
	t.responseEnd = time.Now()
}

// GetConnectTime 获取连接时间（秒）
func (t *UpstreamTiming) GetConnectTime() float64 {
	if t.connectStart.IsZero() || t.connectEnd.IsZero() {
		return 0
	}
	return t.connectEnd.Sub(t.connectStart).Seconds()
}

// GetHeaderTime 获取首字节时间（秒）
func (t *UpstreamTiming) GetHeaderTime() float64 {
	if t.connectEnd.IsZero() || t.headerReceived.IsZero() {
		return 0
	}
	return t.headerReceived.Sub(t.connectEnd).Seconds()
}

// GetResponseTime 获取响应时间（秒）
func (t *UpstreamTiming) GetResponseTime() float64 {
	if t.connectEnd.IsZero() || t.responseEnd.IsZero() {
		return 0
	}
	return t.responseEnd.Sub(t.connectEnd).Seconds()
}

// FinalizeUpstreamVars 在请求处理结束时设置上游变量到 Context
// 这个函数应该在 ServeHTTP 的 defer 中调用
func FinalizeUpstreamVars(vc *variable.Context, upstreamAddr string, upstreamStatus int, timing *UpstreamTiming) {
	if vc == nil {
		return
	}

	connectTime := timing.GetConnectTime()
	headerTime := timing.GetHeaderTime()
	responseTime := timing.GetResponseTime()

	vc.SetUpstreamVars(upstreamAddr, upstreamStatus, responseTime, connectTime, headerTime)
}

// ServeHTTP 通过将传入的 HTTP 请求转发到选定的后端目标来处理请求。
// 实现了 fasthttp 请求处理器接口。
//
// 处理流程：
// 1. 使用负载均衡选择目标
// 2. 准备请求（修改请求头）
// 3. 将请求转发到后端
// 4. 将响应复制回客户端
//
// 如果没有可用的健康目标，返回 502 Bad Gateway。
// 如果后端请求失败，根据 next_upstream 配置尝试下一个目标。
func (p *Proxy) ServeHTTP(ctx *fasthttp.RequestCtx) {
	// DEBUG: 打印请求信息
	logging.Debug().Msgf("[PROXY] 收到请求: path=%s, host=%s, method=%s",
		string(ctx.Path()), string(ctx.Host()), string(ctx.Method()))

	// 上游变量捕获
	var upstreamAddr string
	var upstreamStatus int
	timing := NewUpstreamTiming()

	// 创建变量上下文用于设置上游变量
	vc := variable.NewContext(ctx)
	defer func() {
		// 确保记录了响应结束时间
		if timing.responseEnd.IsZero() {
			timing.MarkResponseEnd()
		}
		// 设置上游变量
		FinalizeUpstreamVars(vc, upstreamAddr, upstreamStatus, timing)
		// 释放变量上下文
		variable.ReleaseContext(vc)
	}()

	// 故障转移配置
	maxTries := p.config.NextUpstream.Tries
	if maxTries <= 0 {
		maxTries = 1 // 默认不重试
	}
	httpCodes := p.config.NextUpstream.HTTPCodes
	if len(httpCodes) == 0 {
		// 默认重试的状态码
		httpCodes = []int{502, 503, 504}
	}

	// 已尝试的目标列表（用于故障转移时排除）
	attemptedTargets := make([]*loadbalance.Target, 0, maxTries)

	var lastErr error

	for attempt := 0; attempt < maxTries; attempt++ {
		// 选择目标（第一次使用普通选择，后续排除已失败目标）
		var target *loadbalance.Target
		if attempt == 0 {
			target = p.selectTarget(ctx)
		} else {
			target = p.selectTargetExcluding(ctx, attemptedTargets)
		}

		if target == nil {
			if attempt == 0 {
				// 没有可用后端
				upstreamAddr = "FAILED"
				upstreamStatus = 502
				utils.SendErrorWithDetail(ctx, utils.ErrBadGateway, "no healthy upstream")
				return
			}
			// 没有更多可用目标，返回最后一次错误
			break
		}

		attemptedTargets = append(attemptedTargets, target)

		// DEBUG: 打印选中的目标
		logging.Debug().Msgf("[PROXY] 选中目标: url=%s, healthy=%v", target.URL, target.Healthy.Load())

		// 获取所选目标的客户端
		client := p.getClient(target.URL)
		if client == nil {
			logging.Warn().Msgf("[PROXY] client 为 nil, url=%s", target.URL)
			// 标记为不健康并继续尝试下一个
			if p.healthChecker != nil {
				p.healthChecker.MarkUnhealthy(target)
			}
			continue
		}

		// DEBUG: 打印客户端信息
		logging.Debug().Msgf("[PROXY] client 信息: Addr=%s, IsTLS=%v", client.Addr, client.IsTLS)

		// 增加连接计数（用于最少连接数负载均衡）
		loadbalance.IncrementConnections(target)

		// 保存客户端原始 host（在 modifyRequestHeaders 改写前）
		// 用于 redirect_rewrite 获取客户端实际访问地址
		originalClientHost := string(ctx.Host())

		// 设置上游地址
		upstreamAddr = target.URL

		// 检查是否为 WebSocket 升级请求
		if isWebSocketRequest(ctx) {
			// WebSocket 使用 defer 确保连接计数释放
			defer loadbalance.DecrementConnections(target)
			timing.MarkConnectStart()
			err := WebSocket(ctx, target, p.config.Timeout.Connect)
			timing.MarkConnectEnd()
			if err != nil {
				upstreamStatus = 502
				logging.Error().Msgf("WebSocket proxy error: %v", err)
				return
			}
			// WebSocket 成功
			upstreamStatus = 101
			return
		}

		// 准备请求
		req := &ctx.Request

		// 修改请求头
		p.modifyRequestHeaders(ctx, target)

		// 关键：修改请求 URI 为完整的目标 URL
		// HostClient 要求 URI 格式必须与 Addr/IsTLS 一致
		// 例如：IsTLS=true 时，URI 应为 https://host/path
		targetURI := target.URL + string(ctx.URI().Path())
		if len(ctx.URI().QueryString()) > 0 {
			targetURI += "?" + string(ctx.URI().QueryString())
		}
		req.SetRequestURI(targetURI)

		// DEBUG: 打印请求头
		logging.Debug().Msgf("[PROXY] 请求准备完成: Host=%s, URI=%s, targetURI=%s",
			string(req.Header.Host()), string(req.RequestURI()), targetURI)

		// 尝试从缓存获取（如果启用）
		if p.cache != nil && attempt == 0 {
			hashKey, origKey := p.buildCacheKeyHash(ctx)
			if entry, ok, stale := p.cache.Get(hashKey, origKey); ok {
				// 缓存命中
				loadbalance.DecrementConnections(target)
				if !stale {
					// 新鲜缓存，直接返回
					upstreamAddr = upstreamCache
					upstreamStatus = entry.Status
					p.writeCachedResponse(ctx, entry)
					if p.redirectRewriter != nil {
						p.redirectRewriter.RewriteRefreshOnly(&ctx.Response, ctx, upstreamCache, originalClientHost)
					}
					return
				}
				// 过期缓存，尝试后台刷新，同时返回旧数据

				go p.backgroundRefresh(ctx, target, hashKey, origKey)
				upstreamAddr = "CACHE"
				upstreamStatus = entry.Status

				p.writeCachedResponse(ctx, entry)
				if p.redirectRewriter != nil {
					p.redirectRewriter.RewriteRefreshOnly(&ctx.Response, ctx, upstreamCache, originalClientHost)
				}
				return
			}

			// 检查是否需要缓存锁（防止缓存击穿）
			if done := p.cache.AcquireLock(hashKey); done != nil {
				// 有其他请求正在生成缓存，等待
				loadbalance.DecrementConnections(target)
				<-done
				// 重新尝试获取缓存

				if entry, ok, _ := p.cache.Get(hashKey, origKey); ok {
					upstreamAddr = upstreamCache
					upstreamStatus = entry.Status

					p.writeCachedResponse(ctx, entry)
					if p.redirectRewriter != nil {
						p.redirectRewriter.RewriteRefreshOnly(&ctx.Response, ctx, upstreamCache, originalClientHost)
					}
					return
				}
				// 缓存未命中，需要重新选择目标
				loadbalance.IncrementConnections(target)
			}
		}

		// 执行代理请求
		timing.MarkConnectStart()
		err := client.Do(req, &ctx.Response)
		timing.MarkConnectEnd()

		// DEBUG: 打印执行结果
		if err != nil {
			logging.Error().Msgf("[PROXY] 请求失败: url=%s, err=%v, errType=%T", target.URL, err, err)
		} else {
			logging.Debug().Msgf("[PROXY] 请求成功: url=%s, status=%d", target.URL, ctx.Response.StatusCode())
		}

		if err != nil {
			loadbalance.DecrementConnections(target)

			// 被动健康检查：标记目标为不健康
			if p.healthChecker != nil {
				p.healthChecker.MarkUnhealthy(target)
			}

			// 释放缓存锁
			if p.cache != nil && attempt == 0 {
				hashKey, _ := p.buildCacheKeyHash(ctx)
				p.cache.ReleaseLock(hashKey, err)
			}

			// 设置失败状态
			if errors.Is(err, fasthttp.ErrTimeout) {
				upstreamStatus = 504
			} else {
				upstreamStatus = 502
			}

			lastErr = err
			// 继续尝试下一个目标
			continue
		}

		// 记录首字节时间
		timing.MarkHeaderReceived()

		// 请求成功，减少连接计数
		loadbalance.DecrementConnections(target)

		// 检查响应状态码是否需要重试
		statusCode := ctx.Response.StatusCode()
		upstreamStatus = statusCode

		shouldRetry := slices.Contains(httpCodes, statusCode)

		if shouldRetry {
			// 释放缓存锁
			if p.cache != nil && attempt == 0 {
				hashKey, _ := p.buildCacheKeyHash(ctx)
				p.cache.ReleaseLock(hashKey, fmt.Errorf("HTTP %d", statusCode))
			}

			// 如果不是最后一次尝试，继续下一个目标
			if attempt < maxTries-1 {
				// 标记目标为不健康
				if p.healthChecker != nil {
					p.healthChecker.MarkUnhealthy(target)
				}
				continue
			}
		}

		// 重试成功时恢复健康状态
		if attempt > 0 && p.healthChecker != nil {
			p.healthChecker.MarkHealthy(target)
		}

		// 存入缓存（如果启用且响应可缓存）
		if p.cache != nil {
			hashKey, origKey := p.buildCacheKeyHash(ctx)
			if statusCode >= 200 && statusCode < 300 {
				// 提取响应头
				headers := make(map[string]string)
				for key, value := range ctx.Response.Header.All() {
					headers[string(key)] = string(value)
				}
				p.cache.Set(hashKey, origKey, ctx.Response.Body(), headers, statusCode, p.getCacheDuration(statusCode))
			}
			p.cache.ReleaseLock(hashKey, nil)
		}

		// 改写重定向响应头（Location/Refresh）
		if p.redirectRewriter != nil && p.redirectRewriter.Mode() != "off" {
			p.redirectRewriter.RewriteResponse(&ctx.Response, ctx, upstreamAddr, originalClientHost)
		}

		// 修改响应头
		p.modifyResponseHeaders(ctx)
		return
	}

	// 所有尝试都失败
	if lastErr != nil {
		// 处理不同类型的错误
		if errors.Is(lastErr, fasthttp.ErrTimeout) {
			upstreamStatus = 504
			utils.SendError(ctx, utils.ErrGatewayTimeout)
		} else if errors.Is(lastErr, fasthttp.ErrConnectionClosed) {
			upstreamStatus = 502
			utils.SendErrorWithDetail(ctx, utils.ErrBadGateway, "upstream connection closed")
		} else {
			upstreamStatus = 502
			utils.SendError(ctx, utils.ErrBadGateway)
		}
	} else {
		upstreamAddr = "FAILED"
		upstreamStatus = 502
		utils.SendErrorWithDetail(ctx, utils.ErrBadGateway, "all upstreams failed")
	}
}

// selectTarget 使用配置的负载均衡器选择后端目标
// 如果启用 Lua balancer，先尝试 Lua 脚本选择
func (p *Proxy) selectTarget(ctx *fasthttp.RequestCtx) *loadbalance.Target {
	p.mu.RLock()
	targets := p.targets
	p.mu.RUnlock()

	if len(targets) == 0 {
		return nil
	}

	// 检查是否启用 Lua balancer
	if p.config.BalancerByLua.Enabled && p.config.BalancerByLua.Script != "" && p.luaEngine != nil {
		target, err := p.selectByLua(ctx, targets)
		if err != nil {
			logging.Warn().Err(err).Msg("lua balancer failed, using fallback")
			// Lua 失败，使用 fallback 算法
			return p.selectByFallback(ctx, targets)
		}
		if target != nil {
			return target
		}
		// Lua 未调用 set_current_peer，使用 fallback
		logging.Debug().Msg("lua balancer did not select target, using fallback")
		return p.selectByFallback(ctx, targets)
	}

	// 使用传统负载均衡算法
	return p.selectByBalancer(ctx, targets)
}

// selectByLua 使用 Lua 脚本选择目标
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

// selectByFallback 使用 fallback 算法选择目标
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

// selectByBalancer 使用主负载均衡器选择目标
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

// extractHashKey 根据配置提取哈希键值。
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

// getClient 返回给定目标 URL 对应的 HostClient。
func (p *Proxy) getClient(targetURL string) *fasthttp.HostClient {
	p.mu.RLock()
	client := p.clients[targetURL]
	p.mu.RUnlock()
	return client
}

// modifyRequestHeaders 在转发到后端之前修改请求头。
// 添加标准代理请求头并应用自定义请求头配置。
func (p *Proxy) modifyRequestHeaders(ctx *fasthttp.RequestCtx, target *loadbalance.Target) {
	headers := &ctx.Request.Header

	// 设置 Host header 为目标主机
	// 从 target.URL 提取 host:port（HostClient 连接需要此格式）
	targetHost := extractHostFromURL(target.URL)
	if targetHost != "" {
		headers.Set("Host", targetHost)
	}

	// 提取并设置 X-Forwarded 系列头
	fh := ExtractForwardedHeaders(ctx)
	SetForwardedHeaders(headers, fh, true)

	// 从配置设置自定义请求头（支持变量展开）
	if p.config.Headers.SetRequest != nil {
		vc := variable.NewContext(ctx)
		defer variable.ReleaseContext(vc)
		for key, value := range p.config.Headers.SetRequest {
			expanded := vc.Expand(value)
			headers.Set(key, expanded)
		}
	}

	// 移除配置的请求头
	if len(p.config.Headers.Remove) > 0 {
		for _, key := range p.config.Headers.Remove {
			headers.Del(key)
		}
	}
}

// modifyResponseHeaders 在发送给客户端之前修改响应头。
func (p *Proxy) modifyResponseHeaders(ctx *fasthttp.RequestCtx) {
	// 从配置设置自定义响应头（支持变量展开）
	if p.config.Headers.SetResponse != nil {
		vc := variable.NewContext(ctx)
		defer variable.ReleaseContext(vc)
		for key, value := range p.config.Headers.SetResponse {
			expanded := vc.Expand(value)
			ctx.Response.Header.Set(key, expanded)
		}
	}
}

// isWebSocketRequest 检查请求是否为 WebSocket 升级请求。
func isWebSocketRequest(ctx *fasthttp.RequestCtx) bool {
	// 检查 Connection 请求头
	connection := ctx.Request.Header.Peek("Connection")
	if !strings.EqualFold(string(connection), "upgrade") {
		// 也检查 "Upgrade" 子串（例如 "keep-alive, Upgrade"）
		if !strings.Contains(strings.ToLower(string(connection)), "upgrade") {
			return false
		}
	}

	// 检查 Upgrade 请求头
	upgrade := ctx.Request.Header.Peek("Upgrade")
	return strings.EqualFold(string(upgrade), "websocket")
}

// UpdateTargets 更新代理目标并重新初始化客户端。
// 适用于动态配置更新。
func (p *Proxy) UpdateTargets(targets []*loadbalance.Target) error {
	if len(targets) == 0 {
		return errors.New("no targets provided")
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	// 清除旧客户端
	p.clients = make(map[string]*fasthttp.HostClient)

	// 初始化新客户端（使用 nil TransportConfig 保持原有行为）
	for _, target := range targets {
		if target.URL == "" {
			continue
		}

		client := createHostClient(target.URL, p.config.Timeout, nil, p.config.ProxySSL)
		p.clients[target.URL] = client
	}

	p.targets = targets
	return nil
}

// GetTargets 返回当前的目标列表。
func (p *Proxy) GetTargets() []*loadbalance.Target {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.targets
}

// GetConfig 返回代理配置。
func (p *Proxy) GetConfig() *config.ProxyConfig {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.config
}

// buildCacheKey 构建缓存键。
func (p *Proxy) buildCacheKey(ctx *fasthttp.RequestCtx) string {
	// 使用请求方法和路径作为缓存键
	return string(ctx.Request.Header.Method()) + ":" + string(ctx.Request.URI().RequestURI())
}

// buildCacheKeyHash 使用 FNV-64a 计算缓存键的 uint64 哈希值。
// 这个函数分配 0 内存，比字符串键更高效。
func (p *Proxy) buildCacheKeyHash(ctx *fasthttp.RequestCtx) (uint64, string) {
	// 构建原始 key
	origKey := p.buildCacheKey(ctx)

	// 使用 FNV-64a 计算哈希
	h := fnv.New64a()
	h.Write([]byte(origKey))
	return h.Sum64(), origKey
}

// writeCachedResponse 写入缓存的响应。
func (p *Proxy) writeCachedResponse(ctx *fasthttp.RequestCtx, entry *cache.ProxyCacheEntry) {
	ctx.Response.SetBody(entry.Data)
	ctx.Response.SetStatusCode(entry.Status)
	for key, value := range entry.Headers {
		ctx.Response.Header.Set(key, value)
	}
	ctx.Response.Header.Set("X-Cache", "HIT")
}

// backgroundRefresh 后台刷新缓存。
func (p *Proxy) backgroundRefresh(ctx *fasthttp.RequestCtx, target *loadbalance.Target, hashKey uint64, origKey string) {
	// 创建新的请求上下文副本
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	// 复制原始请求
	ctx.Request.CopyTo(req)

	// 获取客户端
	client := p.getClient(target.URL)
	if client == nil {
		return
	}

	// 执行请求
	err := client.Do(req, resp)
	if err != nil {
		p.cache.ReleaseLock(hashKey, err)
		return
	}

	// 提取响应头
	headers := make(map[string]string)
	for key, value := range resp.Header.All() {
		headers[string(key)] = string(value)
	}

	// 更新缓存
	p.cache.Set(hashKey, origKey, resp.Body(), headers, resp.StatusCode(), p.getCacheDuration(resp.StatusCode()))
}

// GetCacheStats 返回代理缓存的统计信息。
// 如果缓存未启用，返回 nil。
func (p *Proxy) GetCacheStats() *cache.ProxyCacheStats {
	if p.cache == nil {
		return nil
	}
	stats := p.cache.Stats()
	return &stats
}

// extractHostFromURL 从 URL 中提取 host:port。
// 用于设置代理请求的 Host header。
func extractHostFromURL(urlStr string) string {
	// 移除协议前缀
	host := urlStr
	if strings.HasPrefix(host, "http://") {
		host = host[7:]
	} else if strings.HasPrefix(host, "https://") {
		host = host[8:]
	}

	// 移除路径部分
	if idx := strings.Index(host, "/"); idx != -1 {
		host = host[:idx]
	}

	return host
}

// getCacheDuration 根据状态码获取缓存时间。
// 优先级：CacheValid 配置 > MaxAge
//
// 映射规则：
//   - 200-299: CacheValid.OK（0 时继承 MaxAge）
//   - 301/302: CacheValid.Redirect
//   - 404: CacheValid.NotFound
//   - 400-499（除 404）: CacheValid.ClientError
//   - 500-599: CacheValid.ServerError
//   - 其他: 不缓存（返回 0）
func (p *Proxy) getCacheDuration(statusCode int) time.Duration {
	// 无 CacheValid 配置，使用 MaxAge
	if p.config.CacheValid == nil {
		return p.config.Cache.MaxAge
	}

	cv := p.config.CacheValid

	switch {
	case statusCode >= 200 && statusCode < 300:
		if cv.OK > 0 {
			return cv.OK
		}
		return p.config.Cache.MaxAge // 0 表示继承 MaxAge

	case statusCode == 301 || statusCode == 302:
		return cv.Redirect // 0 表示不缓存

	case statusCode == 404:
		return cv.NotFound

	case statusCode >= 400 && statusCode < 500:
		return cv.ClientError

	case statusCode >= 500:
		return cv.ServerError

	default:
		return 0 // 不缓存
	}
}
