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
	"errors"
	"fmt"
	"hash/fnv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/cache"
	"rua.plus/lolly/internal/config"
	"rua.plus/lolly/internal/loadbalance"
	"rua.plus/lolly/internal/logging"
	"rua.plus/lolly/internal/netutil"
	"rua.plus/lolly/internal/resolver"
	"rua.plus/lolly/internal/utils"
	"rua.plus/lolly/internal/variable"
)

const (
	// upstreamCache 上游缓存标识
	// 用于标记请求可直接使用缓存响应，无需转发到上游
	upstreamCache = "CACHE"

	// protoHTTPS 使用 HTTPS 协议
	// 标记与上游目标通信时使用 HTTPS 加密传输
	protoHTTPS = "https"
)

// Proxy 表示反向代理实例，负责将 HTTP 请求转发到后端目标。
//
// 它为每个后端目标管理连接池，并提供负载均衡功能。
//
// 注意事项：
//   - 所有公开方法均为并发安全
//   - 使用前需确保 targets 中至少有一个健康目标
type Proxy struct {
	balancer      loadbalance.Balancer
	resolver      resolver.Resolver
	clients       map[string]*fasthttp.HostClient
	config        *config.ProxyConfig
	cache         *cache.ProxyCache
	healthChecker *HealthChecker
	stopCh        chan struct{}
	targets       []*loadbalance.Target
	mu            sync.RWMutex
	started       atomic.Bool
}

// NewProxy 使用给定的配置和后台目标创建一个新的反向代理实例。
// 它根据配置初始化负载均衡器，并为每个后端目标创建 HostClient。
//
// 参数：
//   - cfg: 代理配置，包括超时时间、请求头和负载均衡策略
//   - targets: 要代理请求的后端目标列表
//   - transportCfg: 可选的 Transport 连接池配置，nil 时使用默认值
//
// 返回值：
//   - *Proxy: 配置完成并可处理请求的代理实例
//   - error: 初始化失败时非空（无效配置、没有健康目标等）
func NewProxy(cfg *config.ProxyConfig, targets []*loadbalance.Target, transportCfg *config.TransportConfig) (*Proxy, error) {
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

	p := &Proxy{
		targets:  targets,
		clients:  make(map[string]*fasthttp.HostClient),
		balancer: balancer,
		config:   cfg,
		stopCh:   make(chan struct{}),
	}

	// 为每个后端目标初始化 HostClient
	for _, target := range targets {
		if target.URL == "" {
			continue
		}

		client := createHostClient(target.URL, cfg.Timeout, transportCfg)
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

	return p, nil
}

// SetHealthChecker 设置健康检查器用于被动健康检查。
// 当代理请求失败时，将调用健康检查器的 MarkUnhealthy 方法。
func (p *Proxy) SetHealthChecker(hc *HealthChecker) {
	p.healthChecker = hc
}

// createBalancer 根据配置的算法创建负载均衡器。
func createBalancer(cfg *config.ProxyConfig) (loadbalance.Balancer, error) {
	switch cfg.LoadBalance {
	case "round_robin", "":
		return loadbalance.NewRoundRobin(), nil
	case "weighted_round_robin":
		return loadbalance.NewWeightedRoundRobin(), nil
	case "least_conn":
		return loadbalance.NewLeastConnections(), nil
	case "ip_hash":
		return loadbalance.NewIPHash(), nil
	case "consistent_hash":
		virtualNodes := cfg.VirtualNodes
		if virtualNodes <= 0 {
			virtualNodes = 150
		}
		return loadbalance.NewConsistentHash(virtualNodes, cfg.HashKey), nil
	default:
		return nil, errors.New("unsupported load balance algorithm: " + cfg.LoadBalance)
	}
}

// createHostClient 为后台目标 URL 创建 fasthttp.HostClient。
func createHostClient(targetURL string, timeout config.ProxyTimeout, transportCfg *config.TransportConfig) *fasthttp.HostClient {
	// 从目标 URL 解析主机和协议
	addr, isTLS := netutil.ParseTargetURL(targetURL, false)

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

		// 获取所选目标的客户端
		client := p.getClient(target.URL)
		if client == nil {
			// 标记为不健康并继续尝试下一个
			if p.healthChecker != nil {
				p.healthChecker.MarkUnhealthy(target)
			}
			continue
		}

		// 增加连接计数（用于最少连接数负载均衡）
		loadbalance.IncrementConnections(target)

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
					return
				}
				// 过期缓存，尝试后台刷新，同时返回旧数据

				go p.backgroundRefresh(ctx, target, hashKey, origKey)
				upstreamAddr = "CACHE"
				upstreamStatus = entry.Status

				p.writeCachedResponse(ctx, entry)
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

		shouldRetry := false
		for _, code := range httpCodes {
			if statusCode == code {
				shouldRetry = true
				break
			}
		}

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
				p.cache.Set(hashKey, origKey, ctx.Response.Body(), headers, statusCode, p.config.Cache.MaxAge)
			}
			p.cache.ReleaseLock(hashKey, nil)
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

// selectTarget 使用配置的负载均衡器选择后端目标。
// 对于 IP 哈希负载均衡，从请求中提取客户端 IP。
// 对于一致性哈希，根据配置的 hash_key 选择目标。
// 如果没有可用的健康目标则返回 nil。
func (p *Proxy) selectTarget(ctx *fasthttp.RequestCtx) *loadbalance.Target {
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
func (p *Proxy) modifyRequestHeaders(ctx *fasthttp.RequestCtx, _ *loadbalance.Target) {
	headers := &ctx.Request.Header

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

// handleWebSocket 处理 WebSocket 升级请求（保留用于兼容性，实际逻辑在 ServeHTTP 中）
//
//nolint:unused // 保留用于未来 WebSocket 功能扩展
func (p *Proxy) handleWebSocket(ctx *fasthttp.RequestCtx, target *loadbalance.Target, _ *fasthttp.HostClient) {
	timeout := p.config.Timeout.Connect
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	if err := WebSocket(ctx, target, timeout); err != nil {
		logging.Error().Msgf("WebSocket proxy error: %v", err)
	}
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

		client := createHostClient(target.URL, p.config.Timeout, nil)
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
	p.cache.Set(hashKey, origKey, resp.Body(), headers, resp.StatusCode(), p.config.Cache.MaxAge)
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
