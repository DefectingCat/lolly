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
	"net"
	"strings"
	"sync"
	"time"

	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/cache"
	"rua.plus/lolly/internal/config"
	"rua.plus/lolly/internal/loadbalance"
	"rua.plus/lolly/internal/logging"
)

// Proxy 表示反向代理实例，负责将 HTTP 请求转发到后端目标。
// 它为每个后端目标管理连接池，并提供负载均衡功能。
type Proxy struct {
	targets       []*loadbalance.Target
	clients       map[string]*fasthttp.HostClient // key: target URL
	balancer      loadbalance.Balancer
	config        *config.ProxyConfig
	cache         *cache.ProxyCache // 代理缓存（可选）
	healthChecker *HealthChecker    // 健康检查器（用于被动检查）
	mu            sync.RWMutex
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
	addr := targetURL
	isTLS := false

	if strings.HasPrefix(targetURL, "http://") {
		addr = targetURL[7:]
	} else if strings.HasPrefix(targetURL, "https://") {
		addr = targetURL[8:]
		isTLS = true
	}

	// 如果存在路径则移除，只保留 host:port
	if idx := strings.Index(addr, "/"); idx != -1 {
		addr = addr[:idx]
	}

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
		RetryIf:                nil, // Disable automatic retries
		DisablePathNormalizing: false,
		SecureErrorLogMessage:  false,
	}

	return client
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
// 如果后端请求失败，返回相应的错误响应。
func (p *Proxy) ServeHTTP(ctx *fasthttp.RequestCtx) {
	// 使用负载均衡器选择目标
	target := p.selectTarget(ctx)
	if target == nil {
		ctx.Error("Bad Gateway: no healthy upstream", fasthttp.StatusBadGateway)
		return
	}

	// 获取所选目标的客户端
	client := p.getClient(target.URL)
	if client == nil {
		ctx.Error("Bad Gateway: upstream client unavailable", fasthttp.StatusBadGateway)
		return
	}

	// 增加连接计数（用于最少连接数负载均衡）
	loadbalance.IncrementConnections(target)
	defer loadbalance.DecrementConnections(target)

	// 检查是否为 WebSocket 升级请求
	if isWebSocketRequest(ctx) {
		p.handleWebSocket(ctx, target, client)
		return
	}

	// 准备请求
	req := &ctx.Request

	// 修改请求头
	p.modifyRequestHeaders(ctx, target)

	// 尝试从缓存获取（如果启用）
	if p.cache != nil {
		cacheKey := p.buildCacheKey(ctx)
		if entry, ok, stale := p.cache.Get(cacheKey); ok {
			// 缓存命中
			if !stale {
				// 新鲜缓存，直接返回
				p.writeCachedResponse(ctx, entry)
				return
			}
			// 过期缓存，尝试后台刷新，同时返回旧数据
			go p.backgroundRefresh(ctx, target, cacheKey)
			p.writeCachedResponse(ctx, entry)
			return
		}

		// 检查是否需要缓存锁（防止缓存击穿）
		if done := p.cache.AcquireLock(cacheKey); done != nil {
			// 有其他请求正在生成缓存，等待
			<-done
			// 重新尝试获取缓存
			if entry, ok, _ := p.cache.Get(cacheKey); ok {
				p.writeCachedResponse(ctx, entry)
				return
			}
		}
	}

	// 执行代理请求
	err := client.Do(req, &ctx.Response)
	if err != nil {
		// 被动健康检查：标记目标为不健康
		if p.healthChecker != nil {
			p.healthChecker.MarkUnhealthy(target)
		}

		// 释放缓存锁
		if p.cache != nil {
			p.cache.ReleaseLock(p.buildCacheKey(ctx), err)
		}

		// 处理不同类型的错误
		if errors.Is(err, fasthttp.ErrTimeout) {
			ctx.Error("Gateway Timeout", fasthttp.StatusGatewayTimeout)
		} else if errors.Is(err, fasthttp.ErrConnectionClosed) {
			ctx.Error("Bad Gateway: upstream connection closed", fasthttp.StatusBadGateway)
		} else {
			ctx.Error("Bad Gateway", fasthttp.StatusBadGateway)
		}
		return
	}

	// 存入缓存（如果启用且响应可缓存）
	if p.cache != nil {
		cacheKey := p.buildCacheKey(ctx)
		status := ctx.Response.StatusCode()
		if status >= 200 && status < 300 {
			// 提取响应头
			headers := make(map[string]string)
			for key, value := range ctx.Response.Header.All() {
				headers[string(key)] = string(value)
			}
			p.cache.Set(cacheKey, ctx.Response.Body(), headers, status, p.config.Cache.MaxAge)
		}
		p.cache.ReleaseLock(cacheKey, nil)
	}

	// 修改响应头
	p.modifyResponseHeaders(ctx)
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
		clientIP := getClientIP(ctx)
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

// extractHashKey 根据配置提取哈希键值。
func (p *Proxy) extractHashKey(ctx *fasthttp.RequestCtx, hashKey string) string {
	switch {
	case hashKey == "ip" || hashKey == "":
		return getClientIP(ctx)
	case hashKey == "uri":
		return string(ctx.RequestURI())
	case strings.HasPrefix(hashKey, "header:"):
		headerName := strings.TrimPrefix(hashKey, "header:")
		value := ctx.Request.Header.Peek(headerName)
		if len(value) > 0 {
			return string(value)
		}
		return getClientIP(ctx) // fallback to IP
	default:
		return getClientIP(ctx)
	}
}

// getClientIP 从请求上下文中提取客户端 IP 地址。
func getClientIP(ctx *fasthttp.RequestCtx) string {
	// 首先检查 X-Forwarded-For 请求头
	if xff := ctx.Request.Header.Peek("X-Forwarded-For"); len(xff) > 0 {
		ips := strings.Split(string(xff), ",")
		if len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}

	// 检查 X-Real-IP 请求头
	if xri := ctx.Request.Header.Peek("X-Real-IP"); len(xri) > 0 {
		return string(xri)
	}

	// 回退到 RemoteAddr
	if addr := ctx.RemoteAddr(); addr != nil {
		if tcpAddr, ok := addr.(*net.TCPAddr); ok {
			return tcpAddr.IP.String()
		}
		return addr.String()
	}

	return ""
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

	// 添加 X-Real-IP 请求头
	clientIP := getClientIP(ctx)
	if clientIP != "" {
		headers.Set("X-Real-IP", clientIP)
	}

	// 添加/追加 X-Forwarded-For 请求头
	existingXFF := headers.Peek("X-Forwarded-For")
	if len(existingXFF) > 0 {
		headers.Set("X-Forwarded-For", string(existingXFF)+", "+clientIP)
	} else {
		headers.Set("X-Forwarded-For", clientIP)
	}

	// 添加 X-Forwarded-Host 请求头
	host := string(ctx.Host())
	if host != "" {
		headers.Set("X-Forwarded-Host", host)
	}

	// 添加 X-Forwarded-Proto 请求头
	proto := "http"
	if ctx.IsTLS() {
		proto = "https"
	}
	headers.Set("X-Forwarded-Proto", proto)

	// 从配置设置自定义请求头
	if p.config.Headers.SetRequest != nil {
		for key, value := range p.config.Headers.SetRequest {
			headers.Set(key, value)
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
	// 从配置设置自定义响应头
	if p.config.Headers.SetResponse != nil {
		for key, value := range p.config.Headers.SetResponse {
			ctx.Response.Header.Set(key, value)
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

// handleWebSocket 处理 WebSocket 升级请求。
func (p *Proxy) handleWebSocket(ctx *fasthttp.RequestCtx, target *loadbalance.Target, client *fasthttp.HostClient) {
	timeout := p.config.Timeout.Connect
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	if err := ProxyWebSocket(ctx, target, timeout); err != nil {
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
func (p *Proxy) backgroundRefresh(ctx *fasthttp.RequestCtx, target *loadbalance.Target, cacheKey string) {
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
		p.cache.ReleaseLock(cacheKey, err)
		return
	}

	// 提取响应头
	headers := make(map[string]string)
	for key, value := range resp.Header.All() {
		headers[string(key)] = string(value)
	}

	// 更新缓存
	p.cache.Set(cacheKey, resp.Body(), headers, resp.StatusCode(), p.config.Cache.MaxAge)
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
