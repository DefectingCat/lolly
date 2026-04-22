// Package proxy 反向代理包，为 Lolly HTTP 服务器提供反向代理功能。
//
// 该包使用 fasthttp.HostClient 实现高性能反向代理，支持连接池和自动 keep-alive 管理。
// 支持负载均衡、WebSocket 转发、自定义请求头/响应头、上游 SSL/TLS、DNS 动态解析、
// 代理缓存、重定向改写和全面的超时配置。
//
// 主要功能：
//   - 多后端负载均衡：支持 round_robin、weighted_round_robin、least_conn、ip_hash、consistent_hash
//   - Lua 动态选择：通过 balancer_by_lua 脚本实现自定义负载均衡逻辑
//   - 故障转移：支持 next_upstream 配置，自动重试失败请求到其他健康目标
//   - WebSocket 代理：支持 ws:// 和 wss:// 协议的透明双向转发
//   - 上游 SSL/TLS：支持自定义 CA 证书、客户端证书（mTLS）、SNI 和 TLS 版本控制
//   - DNS 动态解析：支持后端域名自动解析、IP 缓存和定时刷新
//   - 代理缓存：支持响应缓存、缓存锁防击穿、后台刷新过期缓存
//   - 重定向改写：支持 default/custom/off 模式改写 Location 和 Refresh 响应头
//   - 健康检查：支持主动 HTTP 探测和被动失败标记
//   - 临时文件：大响应自动写入临时文件，避免内存溢出
//
// 主要用途：
//
//	用于将客户端 HTTP 请求代理转发到后端服务器集群，实现负载均衡、缓存加速、
//	协议转换等功能，适用于 API 网关、反向代理服务器等场景。
//
// 注意事项：
//   - Proxy 实例的公开方法均为并发安全
//   - 使用前需确保 targets 中至少有一个健康目标
//   - Lua 脚本执行有超时保护，默认 100ms
//
// 作者：xfy
//
//go:generate go test -v ./...
package proxy

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"net"
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
	// upstreamCache 上游缓存标识。
	// 用于标记请求可直接使用缓存响应，无需转发到上游。
	upstreamCache = "CACHE"

	// 负载均衡算法名称，与配置中的 LoadBalance 字段对应。
	lbRoundRobin         = "round_robin"          // 简单轮询
	lbWeightedRoundRobin = "weighted_round_robin" // 加权轮询
	lbLeastConn          = "least_conn"           // 最少连接
	lbIPHash             = "ip_hash"              // IP 哈希
	lbConsistentHash     = "consistent_hash"      // 一致性哈希
	lbRandom             = "random"               // 随机（Power of Two Choices）
)

// headersPool 复用缓存 headers map，减少分配。
// 预容量 20 覆盖大多数 HTTP 响应头数量。
// 注意：从 pool 获取的 map 使用后不能 Put 回 pool，
// 因为 cache.Set 存储了 map 引用。
var headersPool = sync.Pool{
	New: func() interface{} {
		return make(map[string]string, 20)
	},
}

// Proxy 表示反向代理实例，负责将 HTTP 请求转发到后端目标。
//
// 它为每个后端目标管理连接池（HostClient），并提供负载均衡、
// 缓存、健康检查、Lua 动态选择等功能。
//
// 注意事项：
//   - 所有公开方法均为并发安全
//   - 使用前需确保 targets 中至少有一个健康目标
type Proxy struct {
	balancer         loadbalance.Balancer            // 主负载均衡器
	fallbackBalancer loadbalance.Balancer            // Lua 失败时的备用均衡器
	resolver         resolver.Resolver               // DNS 解析器
	clients          map[string]*fasthttp.HostClient // 后端连接池，key 为 target URL
	config           *config.ProxyConfig             // 代理配置
	cache            *cache.ProxyCache               // 代理缓存
	healthChecker    *HealthChecker                  // 健康检查器
	luaEngine        *lua.LuaEngine                  // Lua 引擎，用于 balancer_by_lua 功能
	redirectRewriter *RedirectRewriter               // 重定向改写器
	stopCh           chan struct{}                   // 停止信号通道
	targets          []*loadbalance.Target           // 后端目标列表
	mu               sync.RWMutex                    // 保护并发访问的读写锁
	started          atomic.Bool                     // 代理启动标志
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

		client := createHostClient(target.URL, cfg.Timeout, transportCfg, cfg.ProxySSL, cfg.ProxyBind, cfg.Buffering)
		clientKey := target.URL
		if cfg.ProxyBind != "" {
			clientKey = target.URL + "|" + cfg.ProxyBind
		}
		p.clients[clientKey] = client
	}

	// 初始化代理缓存（如果启用）
	if cfg.Cache.Enabled {
		rules := make([]cache.ProxyCacheRule, 0)
		if cfg.Cache.MaxAge > 0 {
			// 使用配置中的方法，若为空则使用默认值 GET, HEAD (nginx 默认行为)
			methods := cfg.Cache.Methods
			if len(methods) == 0 {
				methods = []string{"GET", "HEAD"}
			}
			rules = append(rules, cache.ProxyCacheRule{
				Path:     cfg.Path,
				Methods:  methods,
				Statuses: nil, // nil = 所有可缓存状态码 (由 getCacheDuration 处理)
				MaxAge:   cfg.Cache.MaxAge,
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

// createBalancerByName 根据算法名称创建负载均衡器。
//
// 支持的算法：
//   - round_robin: 简单轮询，按顺序选择目标
//   - weighted_round_robin: 加权轮询，按权重比例分配
//   - least_conn: 最少连接，选择当前连接数最少的目标
//   - ip_hash: IP 哈希，同一客户端 IP 固定选择同一目标
//   - consistent_hash: 一致性哈希，支持虚拟节点和自定义 hash_key
//
// 参数：
//   - name: 算法名称
//   - cfg: 代理配置，用于获取虚拟节点数和 hash_key
//
// 返回值：
//   - loadbalance.Balancer: 创建的负载均衡器实例
//   - error: 不支持的算法时返回错误
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
	case lbRandom:
		return loadbalance.NewRandom(), nil
	default:
		return nil, errors.New("unsupported load balance algorithm: " + name)
	}
}

// SetHealthChecker 设置健康检查器用于被动健康检查。
//
// 当代理请求失败时，将调用健康检查器的 MarkUnhealthy 方法，
// 将失败的目标标记为不健康，避免后续请求继续路由到该目标。
//
// 参数：
//   - hc: 健康检查器实例，nil 时禁用被动健康检查
func (p *Proxy) SetHealthChecker(hc *HealthChecker) {
	p.healthChecker = hc
}

// createBalancer 根据配置中指定的算法名称创建负载均衡器。
// 是对 createBalancerByName 的便捷封装。
//
// 参数：
//   - cfg: 代理配置，从 cfg.LoadBalance 读取算法名称
//
// 返回值：
//   - loadbalance.Balancer: 创建的负载均衡器实例
//   - error: 不支持的算法时返回错误
func createBalancer(cfg *config.ProxyConfig) (loadbalance.Balancer, error) {
	return createBalancerByName(cfg.LoadBalance, cfg)
}

// createHostClient 为指定的后端目标 URL 创建 fasthttp.HostClient。
//
// 从目标 URL 解析地址和 TLS 标志，应用 Transport 连接池配置
// （空闲连接超时、最大连接数），以及上游 SSL 配置。
//
// 参数：
//   - targetURL: 后端目标 URL（如 http://backend:8080）
//   - timeout: 代理超时配置（读写超时、连接超时）
//   - transportCfg: 可选的 Transport 连接池配置，nil 时使用默认值
//   - sslCfg: 可选的上游 SSL 配置
//
// 返回值：
//   - *fasthttp.HostClient: 配置完成的 HostClient 实例
func createHostClient(targetURL string, timeout config.ProxyTimeout, transportCfg *config.TransportConfig, sslCfg *config.ProxySSLConfig, proxyBind string, buffering *config.ProxyBufferingConfig) *fasthttp.HostClient {
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

	// ProxyBind：使用指定本地地址作为出站连接源
	if proxyBind != "" {
		localAddr := proxyBind
		dialTimeout := client.MaxConnWaitTimeout
		if dialTimeout <= 0 {
			dialTimeout = 30 * time.Second
		}
		client.Dial = func(addr string) (net.Conn, error) {
			dialer := &net.Dialer{
				LocalAddr: &net.TCPAddr{IP: net.ParseIP(localAddr)},
				Timeout:   dialTimeout,
			}
			return dialer.Dial("tcp", addr)
		}
	}

	// Buffering 控制
	if buffering != nil && buffering.Mode == "off" {
		client.StreamResponseBody = true
	}
	if buffering != nil && buffering.BufferSize > 0 {
		client.ReadBufferSize = buffering.BufferSize
		client.WriteBufferSize = buffering.BufferSize
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

// UpstreamTiming 记录上游请求的各个时间戳。
//
// 用于捕获连接建立、首字节接收、响应完成等关键时间点，
// 计算连接时间、首字节时间和总响应时间，供日志和监控使用。
type UpstreamTiming struct {
	start          time.Time // 请求开始时间
	connectStart   time.Time // 连接开始时间
	connectEnd     time.Time // 连接完成时间
	headerReceived time.Time // 接收到响应头的时间
	responseEnd    time.Time // 响应完成时间
}

// NewUpstreamTiming 创建并初始化上游计时器。
// 自动记录请求开始时间。
//
// 返回值：
//   - *UpstreamTiming: 初始化的计时器实例
func NewUpstreamTiming() *UpstreamTiming {
	return &UpstreamTiming{
		start: time.Now(),
	}
}

// MarkConnectStart 标记连接开始时间点。
func (t *UpstreamTiming) MarkConnectStart() {
	t.connectStart = time.Now()
}

// MarkConnectEnd 标记连接完成时间点。
func (t *UpstreamTiming) MarkConnectEnd() {
	t.connectEnd = time.Now()
}

// MarkHeaderReceived 标记接收到响应头时间点。
func (t *UpstreamTiming) MarkHeaderReceived() {
	t.headerReceived = time.Now()
}

// MarkResponseEnd 标记响应完成时间点。
func (t *UpstreamTiming) MarkResponseEnd() {
	t.responseEnd = time.Now()
}

// GetConnectTime 获取连接建立耗时（秒）。
// 如果连接开始或结束时间未记录，返回 0。
//
// 返回值：
//   - float64: 连接耗时，单位为秒
func (t *UpstreamTiming) GetConnectTime() float64 {
	if t.connectStart.IsZero() || t.connectEnd.IsZero() {
		return 0
	}
	return t.connectEnd.Sub(t.connectStart).Seconds()
}

// GetHeaderTime 获取首字节响应时间（秒）。
// 计算从连接完成到接收到响应头的耗时。
// 如果任一时间点未记录，返回 0。
//
// 返回值：
//   - float64: 首字节耗时，单位为秒
func (t *UpstreamTiming) GetHeaderTime() float64 {
	if t.connectEnd.IsZero() || t.headerReceived.IsZero() {
		return 0
	}
	return t.headerReceived.Sub(t.connectEnd).Seconds()
}

// GetResponseTime 获取总响应时间（秒）。
// 计算从连接完成到响应完成的耗时。
// 如果任一时间点未记录，返回 0。
//
// 返回值：
//   - float64: 响应耗时，单位为秒
func (t *UpstreamTiming) GetResponseTime() float64 {
	if t.connectEnd.IsZero() || t.responseEnd.IsZero() {
		return 0
	}
	return t.responseEnd.Sub(t.connectEnd).Seconds()
}

// FinalizeUpstreamVars 在请求处理结束时设置上游变量到变量上下文。
//
// 该函数应在 ServeHTTP 的 defer 中调用，用于计算并设置以下变量：
//   - upstream_addr: 上游服务器地址
//   - upstream_status: 上游响应状态码
//   - upstream_response_time: 响应耗时
//   - upstream_connect_time: 连接耗时
//   - upstream_header_time: 首字节耗时
//
// 参数：
//   - vc: 变量上下文，用于存储上游变量
//   - upstreamAddr: 上游服务器地址
//   - upstreamStatus: 上游响应状态码
//   - timing: 时间记录器
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
		// SAFETY: lifetime=ephemeral - consumed immediately by SetRequestURIBytes
		path := ctx.URI().Path()
		query := ctx.URI().QueryString()

		// ProxyURI 语义：当 target.ProxyURI 设置时，替换请求路径
		// 这实现了 nginx proxy_pass URI 传递语义：
		//   proxy_pass http://backend/v2/ → 请求路径替换为 /v2/
		if target.ProxyURI != "" {
			path = []byte(target.ProxyURI)
		}

		targetURI := make([]byte, 0, len(target.URL)+len(path)+len(query)+1)
		targetURI = append(targetURI, target.URL...)
		targetURI = append(targetURI, path...)
		if len(query) > 0 {
			targetURI = append(targetURI, '?')
			targetURI = append(targetURI, query...)
		}
		req.SetRequestURIBytes(targetURI)

		// DEBUG: 打印请求头
		logging.Debug().Msgf("[PROXY] 请求准备完成: Host=%s, URI=%s, targetURI=%s",
			string(req.Header.Host()), string(req.RequestURI()), targetURI)

		// 尝试从缓存获取（如果启用）
		if p.cache != nil && attempt == 0 {
			// 检查请求方法是否允许缓存
			method := string(ctx.Request.Header.Method())
			path := string(ctx.Request.URI().Path())
			rule := p.cache.MatchRule(path, method, 0)
			if rule != nil {
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
					if !p.config.Cache.BackgroundUpdateDisable {
						entry.Updating.Store(true)
						go func() {
							defer entry.Updating.Store(false)
							p.backgroundRefresh(ctx, target, hashKey, origKey)
						}()
					}
					upstreamAddr = upstreamCache
					upstreamStatus = entry.Status

					p.writeCachedResponse(ctx, entry)
					if p.redirectRewriter != nil {
						p.redirectRewriter.RewriteRefreshOnly(&ctx.Response, ctx, upstreamCache, originalClientHost)
					}
					return
				}

				// 检查是否需要缓存锁（防止缓存击穿）
				timeout := p.config.Cache.CacheLockTimeout
				if timeout == 0 && p.config.Cache.CacheLock {
					timeout = 5 * time.Second // nginx 默认 5s
				}
				waitCh, timedOut := p.cache.AcquireLockWithTimeout(hashKey, timeout)
				if !timedOut && waitCh != nil {
					// 有其他请求正在生成缓存，等待
					loadbalance.DecrementConnections(target)
					<-waitCh
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
				// timedOut 或获得锁：继续执行代理请求
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
				hashKey := p.buildCacheKeyHashValue(ctx)
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

		// 记录成功，重置软失败状态
		target.RecordSuccess()

		// 检测 X-Accel-Redirect 头，支持内部重定向
		if redirectPath := ctx.Response.Header.Peek("X-Accel-Redirect"); len(redirectPath) > 0 {
			utils.SetInternalRedirect(ctx, string(redirectPath))
			ctx.Request.SetRequestURI(string(redirectPath))
			return
		}

		// 检查响应状态码是否需要重试
		statusCode := ctx.Response.StatusCode()
		upstreamStatus = statusCode

		shouldRetry := slices.Contains(httpCodes, statusCode)

		if shouldRetry {
			// 释放缓存锁
			if p.cache != nil && attempt == 0 {
				hashKey := p.buildCacheKeyHashValue(ctx)
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
			// 再次检查方法是否允许缓存
			method := string(ctx.Request.Header.Method())
			path := string(ctx.Request.URI().Path())
			if rule := p.cache.MatchRule(path, method, statusCode); rule == nil {
				// 方法或状态码不在允许列表中，不缓存
				return
			}

			hashKey, origKey := p.buildCacheKeyHash(ctx)
			if statusCode >= 200 && statusCode < 300 {
				// 检查 MinUses 阈值
				if entry, ok, _ := p.cache.Get(hashKey, origKey); ok {
					minUses := p.config.Cache.MinUses
					if minUses > 0 && entry.Uses.Load() < int32(minUses) {
						p.cache.ReleaseLock(hashKey, nil)
						return
					}
				}

				// 提取响应头（使用 pool 复用 map）
				headers, ok := headersPool.Get().(map[string]string)
				if !ok {
					headers = make(map[string]string, 20)
				}
				for k := range headers {
					delete(headers, k)
				}
				// 构建忽略头部查找表（大小写不敏感）
				ignoreSet := make(map[string]bool, len(p.config.Cache.CacheIgnoreHeaders))
				for _, h := range p.config.Cache.CacheIgnoreHeaders {
					ignoreSet[strings.ToLower(h)] = true
				}

				var lastModified, etag string
				for key, value := range ctx.Response.Header.All() {
					headerName := strings.ToLower(string(key))
					if ignoreSet[headerName] {
						continue
					}
					headers[string(key)] = string(value)

					switch headerName {
					case "last-modified":
						lastModified = string(value)
					case "etag":
						etag = string(value)
					}
				}
				p.cache.Set(hashKey, origKey, ctx.Response.Body(), headers, statusCode, p.getCacheDuration(statusCode))
				if lastModified != "" || etag != "" {
					p.cache.SetValidationHeaders(hashKey, origKey, lastModified, etag)
				}
				// 注意：不能 Put 回 pool，因为 cache.Set 存储了 map 引用
				// 后续 writeCachedResponse 会读取该 map
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

// selectTarget 使用配置的负载均衡器选择后端目标。
//
// 选择优先级：
//  1. 如果启用了 Lua balancer，先尝试 Lua 脚本选择
//  2. Lua 选择失败时，使用 fallback 算法
//  3. 否则使用传统负载均衡算法
//
// 参数：
//   - ctx: FastHTTP 请求上下文，用于提取客户端 IP 等信息
//
// 返回值：
//   - *loadbalance.Target: 选中的后端目标，无可用目标时返回 nil
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

// modifyRequestHeaders 在转发请求到后端之前修改请求头。
//
// 执行以下操作：
//  1. 设置 Host header 为目标主机地址
//  2. 提取并设置 X-Forwarded-For、X-Real-IP、X-Forwarded-Host、X-Forwarded-Proto
//  3. 应用自定义请求头配置（支持变量展开）
//  4. 移除配置的请求头
//
// 参数：
//   - ctx: FastHTTP 请求上下文
//   - target: 选中的后端目标
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
//
// 应用自定义响应头配置，支持变量展开（如 $upstream_addr、$status 等）。
//
// 参数：
//   - ctx: FastHTTP 请求上下文
func (p *Proxy) modifyResponseHeaders(ctx *fasthttp.RequestCtx) {
	respHeaders := &ctx.Response.Header

	// 构建 PassResponse 集合（多处使用）
	passSet := make(map[string]bool, len(p.config.Headers.PassResponse))
	for _, h := range p.config.Headers.PassResponse {
		passSet[h] = true
	}

	// PassResponse 白名单模式：仅传递列出的头部
	if len(passSet) > 0 {
		var toDelete []string
		for key := range respHeaders.All() {
			if !passSet[string(key)] {
				toDelete = append(toDelete, string(key))
			}
		}
		for _, k := range toDelete {
			respHeaders.Del(k)
		}
	}

	// HideResponse：移除指定的响应头（PassResponse 优先，跳过已传递的头部）
	for _, key := range p.config.Headers.HideResponse {
		if !passSet[key] {
			respHeaders.Del(key)
		}
	}

	// IgnoreHeaders：从请求和响应中移除（PassResponse 优先）
	for _, key := range p.config.Headers.IgnoreHeaders {
		ctx.Request.Header.Del(key)
		if !passSet[key] {
			respHeaders.Del(key)
		}
	}

	// Cookie 域/路径重写
	if p.config.Headers.CookieDomain != "" || p.config.Headers.CookiePath != "" {
		p.rewriteCookies(respHeaders)
	}

	// 从配置设置自定义响应头（支持变量展开）
	if p.config.Headers.SetResponse != nil {
		vc := variable.NewContext(ctx)
		defer variable.ReleaseContext(vc)
		for key, value := range p.config.Headers.SetResponse {
			expanded := vc.Expand(value)
			respHeaders.Set(key, expanded)
		}
	}
}

// rewriteCookies 重写响应中 Set-Cookie 头的 domain 和 path。
func (p *Proxy) rewriteCookies(respHeaders *fasthttp.ResponseHeader) {
	cookieDomain := p.config.Headers.CookieDomain
	cookiePath := p.config.Headers.CookiePath
	if cookieDomain == "" && cookiePath == "" {
		return
	}

	cookies := make([]string, 0, respHeaders.Len())
	for _, value := range respHeaders.Cookies() {
		cookie := string(value)
		if cookieDomain != "" {
			cookie = rewriteCookieAttr(cookie, "Domain", cookieDomain)
		}
		if cookiePath != "" {
			cookie = rewriteCookieAttr(cookie, "Path", cookiePath)
		}
		cookies = append(cookies, cookie)
	}

	if len(cookies) > 0 {
		respHeaders.Del("Set-Cookie")
		for _, c := range cookies {
			respHeaders.Add("Set-Cookie", c)
		}
	}
}

// rewriteCookieAttr 替换 Cookie 字符串中指定属性的值（大小写不敏感）。
func rewriteCookieAttr(cookie, attr, newValue string) string {
	prefix := attr + "="
	lower := strings.ToLower(cookie)
	idx := strings.Index(lower, strings.ToLower(prefix))
	if idx == -1 {
		return cookie
	}

	start := idx + len(prefix)
	end := start
	for end < len(cookie) && cookie[end] != ';' && cookie[end] != ' ' {
		end++
	}

	return cookie[:start] + newValue + cookie[end:]
}

// isWebSocketRequest 检查请求是否为 WebSocket 升级请求。
//
// 通过检查 Connection 和 Upgrade 请求头判断：
//   - Connection 头需包含 "upgrade"（不区分大小写）
//   - Upgrade 头需等于 "websocket"（不区分大小写）
//
// 参数：
//   - ctx: FastHTTP 请求上下文
//
// 返回值：
//   - bool: true 表示是 WebSocket 升级请求
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

// UpdateTargets 更新代理的后端目标列表并重新初始化连接池。
//
// 清除旧的 HostClient 连接池，为每个新目标创建新的连接。
// 适用于动态配置更新场景（如热重载配置）。
//
// 参数：
//   - targets: 新的后端目标列表
//
// 返回值：
//   - error: 目标列表为空时返回错误
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

		client := createHostClient(target.URL, p.config.Timeout, nil, p.config.ProxySSL, p.config.ProxyBind, p.config.Buffering)
		clientKey := target.URL
		if p.config.ProxyBind != "" {
			clientKey = target.URL + "|" + p.config.ProxyBind
		}
		p.clients[clientKey] = client
	}

	p.targets = targets
	return nil
}

// GetTargets 返回当前的后端目标列表。
//
// 返回值：
//   - []*loadbalance.Target: 后端目标列表
func (p *Proxy) GetTargets() []*loadbalance.Target {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.targets
}

// GetConfig 返回代理的配置。
//
// 返回值：
//   - *config.ProxyConfig: 代理配置
func (p *Proxy) GetConfig() *config.ProxyConfig {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.config
}

// buildCacheKey 构建缓存键字符串。
//
// 使用请求方法和完整请求 URI 作为缓存键。
// 该函数保留用于日志记录和调试场景。
//
// 参数：
//   - ctx: FastHTTP 请求上下文
//
// 返回值：
//   - string: 缓存键（格式 "METHOD:URI"）
func (p *Proxy) buildCacheKey(ctx *fasthttp.RequestCtx) string {
	// 使用请求方法和路径作为缓存键
	return string(ctx.Request.Header.Method()) + ":" + string(ctx.Request.URI().RequestURI())
}

// buildCacheKeyHash 使用 FNV-64a 计算缓存键的 uint64 哈希值。
// 返回哈希值和原始字符串键。
// 注意：此函数会先构建字符串键再哈希，存在双重分配。
// 对于只需要哈希值的场景，使用 buildCacheKeyHashValue 代替。
func (p *Proxy) buildCacheKeyHash(ctx *fasthttp.RequestCtx) (uint64, string) {
	// 构建原始 key
	origKey := p.buildCacheKey(ctx)

	// 使用 FNV-64a 计算哈希
	h := fnv.New64a()
	h.Write([]byte(origKey))
	return h.Sum64(), origKey
}

// buildCacheKeyHashValue 直接计算缓存键的哈希值，零字符串分配。
// 用于只需要哈希值而不需要原始键的场景。
func (p *Proxy) buildCacheKeyHashValue(ctx *fasthttp.RequestCtx) uint64 {
	h := fnv.New64a()
	h.Write(ctx.Request.Header.Method())
	h.Write([]byte(":"))
	h.Write(ctx.Request.URI().RequestURI())
	return h.Sum64()
}

// writeCachedResponse 将缓存的响应写入 FastHTTP 响应上下文。
//
// 设置响应体、状态码、响应头，并添加 X-Cache: HIT 头标记缓存命中。
//
// 参数：
//   - ctx: FastHTTP 请求上下文
//   - entry: 缓存条目，包含响应数据和元数据
func (p *Proxy) writeCachedResponse(ctx *fasthttp.RequestCtx, entry *cache.ProxyCacheEntry) {
	ctx.Response.SetBody(entry.Data)
	ctx.Response.SetStatusCode(entry.Status)
	for key, value := range entry.Headers {
		ctx.Response.Header.Set(key, value)
	}
	ctx.Response.Header.Set("X-Cache", "HIT")
}

// backgroundRefresh 在后台异步刷新缓存条目。
//
// 向对应的上游目标发送请求，获取最新响应并更新缓存。
// 该方法在独立 goroutine 中运行，不阻塞主请求流程。
//
// 参数：
//   - ctx: 原始 FastHTTP 请求上下文（仅用于复制请求信息）
//   - target: 要刷新的后端目标
//   - hashKey: 缓存哈希键
//   - origKey: 缓存原始键
func (p *Proxy) backgroundRefresh(ctx *fasthttp.RequestCtx, target *loadbalance.Target, hashKey uint64, origKey string) {
	// 创建新的请求上下文副本
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	// 复制原始请求
	ctx.Request.CopyTo(req)

	// 如果启用 Revalidate，添加条件请求头
	if p.config.Cache.Revalidate {
		if entry, ok, _ := p.cache.Get(hashKey, origKey); ok {
			if entry.LastModified != "" {
				req.Header.Set("If-Modified-Since", entry.LastModified)
			}
			if entry.ETag != "" {
				req.Header.Set("If-None-Match", entry.ETag)
			}
		}
	}

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

	// 处理 304 Not Modified 响应
	if resp.StatusCode() == 304 {
		newHeaders := make(map[string]string)
		if lm := resp.Header.Peek("Last-Modified"); len(lm) > 0 {
			newHeaders["Last-Modified"] = string(lm)
		}
		if et := resp.Header.Peek("ETag"); len(et) > 0 {
			newHeaders["ETag"] = string(et)
		}
		p.cache.RefreshTTL(hashKey, origKey, newHeaders)
		return
	}

	// 提取响应头（使用 pool 复用 map）
	headers, ok := headersPool.Get().(map[string]string)
	if !ok {
		headers = make(map[string]string, 20)
	}
	for k := range headers {
		delete(headers, k)
	}
	for key, value := range resp.Header.All() {
		headers[string(key)] = string(value)
	}

	// 更新缓存
	p.cache.Set(hashKey, origKey, resp.Body(), headers, resp.StatusCode(), p.getCacheDuration(resp.StatusCode()))
}

// GetCache 返回代理的 ProxyCache 实例（用于 purge handler）。
// 如果缓存未启用，返回 nil。
func (p *Proxy) GetCache() *cache.ProxyCache {
	return p.cache
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

// extractHostFromURL 从 URL 字符串中提取 host:port 部分。
//
// 移除 http:// 或 https:// 协议前缀，以及路径部分，
// 仅保留主机名和端口（如 "example.com:8080"）。
//
// 参数：
//   - urlStr: 完整 URL 字符串
//
// 返回值：
//   - string: host:port 格式的主机地址
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
