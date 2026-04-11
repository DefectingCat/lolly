// Package server 提供 HTTP 服务器的核心实现，支持单服务器和虚拟主机两种运行模式。
//
// 该文件包含服务器相关的核心逻辑，包括：
//   - HTTP 服务器的创建和生命周期管理
//   - 中间件链的构建和应用
//   - 代理路由的注册和处理
//   - 静态文件服务的集成
//   - Goroutine 池的性能优化
//
// 主要用途：
//
//	用于启动和管理 HTTP 服务器，处理客户端请求并转发到上游服务或静态文件。
//
// 注意事项：
//   - 服务器支持优雅关闭和热升级
//   - 所有公开方法均为并发安全
//   - 使用前需确保配置已正确加载
//
// 作者：xfy
package server

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"strings"
	"sync/atomic"
	"time"

	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/cache"
	"rua.plus/lolly/internal/config"
	"rua.plus/lolly/internal/handler"
	"rua.plus/lolly/internal/loadbalance"
	"rua.plus/lolly/internal/logging"
	"rua.plus/lolly/internal/lua"
	"rua.plus/lolly/internal/middleware"
	"rua.plus/lolly/internal/middleware/accesslog"
	"rua.plus/lolly/internal/middleware/bodylimit"
	"rua.plus/lolly/internal/middleware/compression"
	"rua.plus/lolly/internal/middleware/errorintercept"
	"rua.plus/lolly/internal/middleware/rewrite"
	"rua.plus/lolly/internal/middleware/security"
	"rua.plus/lolly/internal/proxy"
	"rua.plus/lolly/internal/resolver"
	"rua.plus/lolly/internal/ssl"
)

// Server HTTP 服务器，封装 fasthttp.Server 并提供中间件链和生命周期管理。
//
// 该结构体是服务器的核心实体，负责：
//   - 管理配置和 fasthttp.Server 实例
//   - 构建和应用中间件链
//   - 维护健康检查器和访问日志中间件
//   - 可选的 Goroutine 池和文件缓存
//
// 注意事项：
//   - 创建后需调用 Start 方法启动服务器
//   - 关闭时建议使用 GracefulStop 实现优雅关闭
type Server struct {
	// config 服务器配置，包含监听地址、代理、静态文件、安全等配置
	config *config.Config

	// fastServer fasthttp 服务器实例，处理底层 HTTP 请求
	fastServer *fasthttp.Server

	// handler 最终的请求处理器，经过中间件链包装
	handler fasthttp.RequestHandler

	// running 服务器运行状态标志
	running bool

	// healthCheckers 健康检查器列表，用于检查代理目标健康状态
	healthCheckers []*proxy.HealthChecker

	// proxies 代理实例列表，用于收集缓存统计
	proxies []*proxy.Proxy

	// accessLogMiddleware 访问日志中间件，记录请求详细信息
	accessLogMiddleware *accesslog.AccessLog

	// errorPageManager 错误页面管理器（可选）
	errorPageManager *handler.ErrorPageManager

	// pool Goroutine 池，用于限制并发处理请求数（可选）
	pool *GoroutinePool

	// fileCache 文件缓存，用于缓存静态文件内容（可选）
	fileCache *cache.FileCache

	// startTime 服务器启动时间
	startTime time.Time

	// connections 当前活动连接数
	connections atomic.Int64

	// requests 总请求数
	requests atomic.Int64

	// bytesSent 发送的总字节数
	bytesSent atomic.Int64

	// bytesReceived 接收的总字节数
	bytesReceived atomic.Int64

	// tlsManager TLS 配置管理器（可选）
	tlsManager *ssl.TLSManager

	// listeners 保存的监听器列表，用于热升级
	listeners []net.Listener

	// resolver DNS 解析器（可选）
	resolver resolver.Resolver

	// luaEngine Lua 引擎（可选）
	luaEngine *lua.LuaEngine
}

// New 创建 HTTP 服务器实例。
//
// 根据提供的配置创建服务器对象，但不启动服务器。
// 服务器创建后需调用 Start 方法才能开始处理请求。
//
// 参数：
//   - cfg: 服务器配置对象，包含监听地址、代理、静态文件、安全等配置
//
// 返回值：
//   - *Server: 创建的服务器实例
func New(cfg *config.Config) *Server {
	return &Server{config: cfg}
}

// trackStats 包装处理器以统计请求数和数据传输量。
//
// 在每个请求处理前后更新统计字段：requests、bytesSent、bytesReceived。
//
// 参数：
//   - handler: 原始的请求处理器
//
// 返回值：
//   - fasthttp.RequestHandler: 包装后的处理器
func (s *Server) trackStats(handler fasthttp.RequestHandler) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		s.requests.Add(1)
		s.bytesReceived.Add(int64(len(ctx.Request.Body())))
		handler(ctx)
		s.bytesSent.Add(int64(len(ctx.Response.Body())))
	}
}

// GetListeners 获取服务器监听器列表。
//
// 返回当前服务器使用的监听器，用于热升级时传递给子进程。
//
// 返回值：
//   - []net.Listener: 监听器列表
func (s *Server) GetListeners() []net.Listener {
	return s.listeners
}

// SetListeners 设置服务器监听器列表。
//
// 用于热升级时，子进程从父进程继承监听器。
//
// 参数：
//   - listeners: 要设置的监听器列表
func (s *Server) SetListeners(listeners []net.Listener) {
	s.listeners = listeners
}

// GetTLSConfig 获取 TLS 配置。
//
// 返回服务器的 TLS 配置，用于 HTTP/3 等需要 TLS 的协议。
//
// 返回值：
//   - *tls.Config: TLS 配置对象
//   - error: 未配置 TLS 或配置无效时返回错误
func (s *Server) GetTLSConfig() (*tls.Config, error) {
	if s.tlsManager == nil {
		return nil, fmt.Errorf("TLS not configured")
	}
	return s.tlsManager.GetTLSConfig(), nil
}

// GetHandler 获取请求处理器。
//
// 返回服务器的请求处理器，用于 HTTP/3 等需要复用处理器的场景。
//
// 返回值：
//   - fasthttp.RequestHandler: 请求处理器
func (s *Server) GetHandler() fasthttp.RequestHandler {
	return s.handler
}

// buildMiddlewareChain 构建中间件链。
//
// 根据服务器配置按顺序构建中间件链，顺序为：
//
//	AccessLog -> AccessControl -> RateLimiter -> BasicAuth -> Rewrite -> Compression -> SecurityHeaders
//
// 参数：
//   - serverCfg: 单个服务器的配置对象
//
// 返回值：
//   - *middleware.Chain: 构建完成的中间件链
//   - error: 构建过程中遇到的错误，如中间件创建失败
//
// 注意事项：
//   - 各中间件按顺序依次包装请求处理器
//   - 未配置的中间件不会添加到链中
func (s *Server) buildMiddlewareChain(serverCfg *config.ServerConfig) (*middleware.Chain, error) {
	var middlewares []middleware.Middleware

	// 1. AccessLog (已集成)
	s.accessLogMiddleware = accesslog.New(&s.config.Logging)
	middlewares = append(middlewares, s.accessLogMiddleware)

	// 2. Security: AccessControl (IP 访问控制)
	if len(serverCfg.Security.Access.Allow) > 0 || len(serverCfg.Security.Access.Deny) > 0 {
		ac, err := security.NewAccessControl(&serverCfg.Security.Access)
		if err != nil {
			return nil, fmt.Errorf("创建访问控制中间件失败: %w", err)
		}
		middlewares = append(middlewares, ac)
	}

	// 3. Security: RateLimiter (速率限制)
	if serverCfg.Security.RateLimit.RequestRate > 0 {
		rl, err := security.NewRateLimiter(&serverCfg.Security.RateLimit)
		if err != nil {
			return nil, fmt.Errorf("创建限流中间件失败: %w", err)
		}
		middlewares = append(middlewares, rl)
	}

	// 3.5 Security: ConnLimiter (连接数限制)
	if serverCfg.Security.RateLimit.ConnLimit > 0 {
		cl, err := security.NewConnLimiter(serverCfg.Security.RateLimit.ConnLimit, true, serverCfg.Security.RateLimit.Key)
		if err != nil {
			return nil, fmt.Errorf("创建连接限制中间件失败: %w", err)
		}
		middlewares = append(middlewares, cl.Middleware())
	}

	// 4. Security: BasicAuth (认证)
	if len(serverCfg.Security.Auth.Users) > 0 {
		auth, err := security.NewBasicAuth(&serverCfg.Security.Auth)
		if err != nil {
			return nil, fmt.Errorf("创建认证中间件失败: %w", err)
		}
		middlewares = append(middlewares, auth)
	}

	// 4.3 Security: AuthRequest (外部认证子请求)
	if serverCfg.Security.AuthRequest.Enabled && serverCfg.Security.AuthRequest.URI != "" {
		authReq, err := security.NewAuthRequest(serverCfg.Security.AuthRequest)
		if err != nil {
			return nil, fmt.Errorf("创建外部认证中间件失败: %w", err)
		}
		middlewares = append(middlewares, authReq)
	}

	// 4.5 BodyLimit (请求体大小限制)
	// 创建 bodylimit 中间件，使用全局配置或默认值
	bodyLimitMiddleware := bodylimit.NewWithDefault()
	if serverCfg.ClientMaxBodySize != "" {
		bl, err := bodylimit.New(serverCfg.ClientMaxBodySize)
		if err != nil {
			return nil, fmt.Errorf("创建请求体限制中间件失败: %w", err)
		}
		bodyLimitMiddleware = bl
	}
	// 添加路径级别的限制配置
	for i := range serverCfg.Proxy {
		if serverCfg.Proxy[i].ClientMaxBodySize != "" {
			if err := bodyLimitMiddleware.AddPathLimit(
				serverCfg.Proxy[i].Path,
				serverCfg.Proxy[i].ClientMaxBodySize,
			); err != nil {
				return nil, fmt.Errorf("添加路径请求体限制失败: %w", err)
			}
		}
	}
	middlewares = append(middlewares, bodyLimitMiddleware)

	// 5. Rewrite (URL 重写)
	if len(serverCfg.Rewrite) > 0 {
		rw, err := rewrite.New(serverCfg.Rewrite)
		if err != nil {
			return nil, fmt.Errorf("创建重写中间件失败: %w", err)
		}
		middlewares = append(middlewares, rw)
	}

	// 6. Compression (响应压缩)
	if serverCfg.Compression.Type != "" {
		comp, err := compression.New(&serverCfg.Compression)
		if err != nil {
			return nil, fmt.Errorf("创建压缩中间件失败: %w", err)
		}
		middlewares = append(middlewares, comp)
	}

	// 7. SecurityHeaders (安全头部)
	// 如果有任何安全头部配置，则启用
	if serverCfg.Security.Headers.XFrameOptions != "" ||
		serverCfg.Security.Headers.XContentTypeOptions != "" ||
		serverCfg.Security.Headers.ContentSecurityPolicy != "" ||
		serverCfg.Security.Headers.ReferrerPolicy != "" ||
		serverCfg.Security.Headers.PermissionsPolicy != "" {
		headers := security.NewHeadersWithHSTS(&serverCfg.Security.Headers, &serverCfg.SSL.HSTS)
		middlewares = append(middlewares, headers)
	}

	// 8. ErrorIntercept (错误页面拦截)
	// 如果配置了错误页面，添加错误拦截中间件
	if s.errorPageManager != nil && s.errorPageManager.IsConfigured() {
		ei := errorintercept.New(s.errorPageManager)
		middlewares = append(middlewares, ei)
	}

	// Lua 中间件（可选）
	if s.luaEngine != nil && serverCfg.Lua != nil && serverCfg.Lua.Enabled {
		luaMiddlewares, err := s.buildLuaMiddlewares(serverCfg.Lua)
		if err != nil {
			return nil, fmt.Errorf("创建 Lua 中间件失败: %w", err)
		}
		middlewares = append(middlewares, luaMiddlewares...)
	}

	return middleware.NewChain(middlewares...), nil
}

// buildLuaMiddlewares 根据 Lua 配置创建中间件。
//
// 根据 Scripts 配置创建 LuaMiddleware 或 MultiPhaseLuaMiddleware。
// 支持单脚本和多阶段脚本配置。
//
// 参数：
//   - luaCfg: Lua 配置对象
//
// 返回值：
//   - []middleware.Middleware: 创建的中间件列表
//   - error: 创建过程中遇到的错误
func (s *Server) buildLuaMiddlewares(luaCfg *config.LuaMiddlewareConfig) ([]middleware.Middleware, error) {
	if s.luaEngine == nil {
		return nil, nil
	}

	// 按阶段分组脚本
	phaseScripts := make(map[string][]config.LuaScriptConfig)
	for _, script := range luaCfg.Scripts {
		// 默认启用
		enabled := script.Enabled
		if !enabled && script.Timeout == 0 && script.Path != "" {
			enabled = true // 零值时默认启用
		}
		if enabled {
			phaseScripts[script.Phase] = append(phaseScripts[script.Phase], script)
		}
	}

	var middlewares []middleware.Middleware

	// 为每个阶段创建中间件
	for phase, scripts := range phaseScripts {
		if len(scripts) == 0 {
			continue
		}

		// 单脚本：直接创建 LuaMiddleware
		if len(scripts) == 1 {
			script := scripts[0]
			luaPhase, err := lua.ParsePhase(phase)
			if err != nil {
				return nil, fmt.Errorf("无效的阶段 '%s': %w", phase, err)
			}

			timeout := script.Timeout
			if timeout == 0 {
				timeout = 30 * time.Second
			}

			cfg := lua.LuaMiddlewareConfig{
				ScriptPath: script.Path,
				Phase:      luaPhase,
				Timeout:    timeout,
				Name:       fmt.Sprintf("lua-%s", phase),
			}

			mw, err := lua.NewLuaMiddleware(s.luaEngine, cfg)
			if err != nil {
				return nil, fmt.Errorf("创建 Lua 中间件失败 (phase=%s): %w", phase, err)
			}

			middlewares = append(middlewares, mw)
		} else {
			// 多脚本：创建 MultiPhaseLuaMiddleware
			multi := lua.NewMultiPhaseLuaMiddleware(s.luaEngine, fmt.Sprintf("lua-multi-%s", phase))
			for _, script := range scripts {
				luaPhase, err := lua.ParsePhase(phase)
				if err != nil {
					return nil, fmt.Errorf("无效的阶段 '%s': %w", phase, err)
				}

				timeout := script.Timeout
				if timeout == 0 {
					timeout = 30 * time.Second
				}

				err = multi.AddPhase(luaPhase, script.Path, timeout)
				if err != nil {
					return nil, fmt.Errorf("添加 Lua 阶段失败 (phase=%s): %w", phase, err)
				}
			}

			middlewares = append(middlewares, multi)
		}
	}

	return middlewares, nil
}

// Start 启动 HTTP 服务器。
//
// 初始化日志系统、性能优化组件（Goroutine池、文件缓存），
// 根据配置选择单服务器模式或虚拟主机模式启动。
//
// 返回值：
//   - error: 启动过程中遇到的错误，如监听地址绑定失败
//
// 注意事项：
//   - 该方法会阻塞运行，直到服务器停止
//   - 调用前需确保配置已正确加载
//   - Goroutine池和文件缓存根据配置自动启用
func (s *Server) Start() error {
	logging.Init(s.config.Logging.Error.Level, true)

	// 记录启动时间
	s.startTime = time.Now()

	// 启用 GoroutinePool（如果配置）
	if s.config.Performance.GoroutinePool.Enabled {
		s.pool = NewGoroutinePool(PoolConfig{
			MaxWorkers:  s.config.Performance.GoroutinePool.MaxWorkers,
			MinWorkers:  s.config.Performance.GoroutinePool.MinWorkers,
			IdleTimeout: s.config.Performance.GoroutinePool.IdleTimeout,
		})
		s.pool.Start()
	}

	// 启用文件缓存（如果配置）
	if s.config.Performance.FileCache.MaxEntries > 0 || s.config.Performance.FileCache.MaxSize > 0 {
		s.fileCache = cache.NewFileCache(
			s.config.Performance.FileCache.MaxEntries,
			s.config.Performance.FileCache.MaxSize,
			s.config.Performance.FileCache.Inactive,
		)
	}

	// 预加载错误页面（如果配置）
	if s.config.Server.Security.ErrorPage.Pages != nil || s.config.Server.Security.ErrorPage.Default != "" {
		var err error
		s.errorPageManager, err = handler.NewErrorPageManager(&s.config.Server.Security.ErrorPage)
		if err != nil {
			// 检查是否是部分加载失败
			if _, ok := err.(*handler.PartialLoadError); ok {
				logging.Warn().Msg("部分错误页面加载失败: " + err.Error())
			} else {
				// 全部加载失败，阻止启动
				return fmt.Errorf("加载错误页面失败: %w", err)
			}
		}
	}

	// 初始化 Lua 引擎（如果配置）
	if s.config.Server.Lua != nil && s.config.Server.Lua.Enabled {
		engineCfg := &lua.Config{
			MaxConcurrentCoroutines: s.config.Server.Lua.GlobalSettings.MaxConcurrentCoroutines,
			CoroutineTimeout:        s.config.Server.Lua.GlobalSettings.CoroutineTimeout,
			CodeCacheSize:           s.config.Server.Lua.GlobalSettings.CodeCacheSize,
			CodeCacheTTL:            time.Hour, // 默认值
			EnableFileWatch:         s.config.Server.Lua.GlobalSettings.EnableFileWatch,
			MaxExecutionTime:        s.config.Server.Lua.GlobalSettings.MaxExecutionTime,
			EnableOSLib:             false, // 安全默认值
			EnableIOLib:             false,
			EnableLoadLib:           false,
		}
		// 设置默认值
		if engineCfg.MaxConcurrentCoroutines == 0 {
			engineCfg.MaxConcurrentCoroutines = 1000
		}
		if engineCfg.CoroutineTimeout == 0 {
			engineCfg.CoroutineTimeout = 30 * time.Second
		}
		if engineCfg.CodeCacheSize == 0 {
			engineCfg.CodeCacheSize = 1000
		}
		if engineCfg.MaxExecutionTime == 0 {
			engineCfg.MaxExecutionTime = 30 * time.Second
		}

		var err error
		s.luaEngine, err = lua.NewEngine(engineCfg)
		if err != nil {
			return fmt.Errorf("初始化 Lua 引擎失败: %w", err)
		}
		logging.Info().Msg("Lua 引擎已启动")
	}

	if s.config.HasServers() {
		return s.startVHostMode()
	}
	return s.startSingleMode()
}

// startSingleMode 单服务器模式启动。
//
// 在单服务器模式下，创建单一路由器，注册代理路由和静态文件服务，
// 应用中间件链后启动 fasthttp 服务器。
//
// 返回值：
//   - error: 启动过程中遇到的错误
//
// 注意事项：
//   - 静态文件服务作为 fallback 处理非代理路径的请求
//   - 使用零拷贝传输优化大文件传输
func (s *Server) startSingleMode() error {
	router := handler.NewRouter()

	// 注册状态监控端点（如果配置）
	if s.config.Monitoring.Status.Path != "" || len(s.config.Monitoring.Status.Allow) > 0 {
		statusHandler, err := NewStatusHandler(s, &s.config.Monitoring.Status)
		if err != nil {
			logging.Error().Msg("创建状态处理器失败: " + err.Error())
		} else {
			router.GET(statusHandler.Path(), statusHandler.ServeHTTP)
		}
	}

	// 注册 pprof 性能分析端点（如果配置）
	if s.config.Monitoring.Pprof.Enabled {
		pprofHandler, err := NewPprofHandler(&s.config.Monitoring.Pprof)
		if err != nil {
			logging.Error().Msg("创建 pprof 处理器失败: " + err.Error())
		} else {
			router.GET(pprofHandler.Path(), pprofHandler.ServeHTTP)
			router.GET(pprofHandler.Path()+"/{profile:*}", pprofHandler.ServeHTTP)
		}
	}

	// 注册代理路由
	s.registerProxyRoutes(router, &s.config.Server)

	// 静态文件服务
	s.registerStaticHandlers(router, &s.config.Server)

	// 构建中间件链
	chain, err := s.buildMiddlewareChain(&s.config.Server)
	if err != nil {
		return err
	}

	// 应用 GoroutinePool（如果启用）
	handler := chain.Apply(router.Handler())
	if s.pool != nil {
		handler = s.pool.WrapHandler(handler)
	}
	// 包装统计追踪
	handler = s.trackStats(handler)
	s.handler = handler

	s.fastServer = &fasthttp.Server{
		Name:               "lolly",
		Handler:            s.handler,
		ReadTimeout:        s.config.Server.ReadTimeout,
		WriteTimeout:       s.config.Server.WriteTimeout,
		IdleTimeout:        s.config.Server.IdleTimeout,
		MaxConnsPerIP:      s.config.Server.MaxConnsPerIP,
		MaxRequestsPerConn: s.config.Server.MaxRequestsPerConn,
	}

	s.running = true

	// 创建监听器并保存，用于热升级
	ln, err := net.Listen("tcp", s.config.Server.Listen)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}
	s.listeners = []net.Listener{ln}

	// 检查是否配置了 SSL/TLS
	if s.config.Server.SSL.Cert != "" && s.config.Server.SSL.Key != "" {
		var err error
		s.tlsManager, err = ssl.NewTLSManager(&s.config.Server.SSL)
		if err != nil {
			return fmt.Errorf("创建 TLS 管理器失败: %w", err)
		}
		s.fastServer.TLSConfig = s.tlsManager.GetTLSConfig()
		return s.fastServer.ServeTLS(ln, "", "")
	}

	return s.fastServer.Serve(ln)
}

// startVHostMode 虚拟主机模式启动。
//
// 在虚拟主机模式下，为每个配置的服务器创建独立的路由器和中间件链，
// 通过虚拟主机管理器根据 Host 头分发请求。
//
// 返回值：
//   - error: 启动过程中遇到的错误
//
// 注意事项：
//   - 每个虚拟主机有独立的中间件配置
//   - 未匹配的 Host 头请求由默认主机处理
func (s *Server) startVHostMode() error {
	vhostMgr := NewVHostManager()

	for i := range s.config.Servers {
		router := handler.NewRouter()
		s.registerProxyRoutes(router, &s.config.Servers[i])

		// 静态文件
		s.registerStaticHandlers(router, &s.config.Servers[i])

		// 为每个虚拟主机构建独立的中间件链
		chain, err := s.buildMiddlewareChain(&s.config.Servers[i])
		if err != nil {
			return err
		}

		handler := chain.Apply(router.Handler())
		if s.pool != nil {
			handler = s.pool.WrapHandler(handler)
		}

		vhostMgr.AddHost(s.config.Servers[i].Name, handler)
	}

	// 默认主机
	if s.config.HasDefaultServer() {
		router := handler.NewRouter()

		// 注册状态监控端点（如果配置）
		if s.config.Monitoring.Status.Path != "" || len(s.config.Monitoring.Status.Allow) > 0 {
			statusHandler, err := NewStatusHandler(s, &s.config.Monitoring.Status)
			if err != nil {
				logging.Error().Msg("创建状态处理器失败: " + err.Error())
			} else {
				router.GET(statusHandler.Path(), statusHandler.ServeHTTP)
			}
		}

		// 注册 pprof 性能分析端点（如果配置）
		if s.config.Monitoring.Pprof.Enabled {
			pprofHandler, err := NewPprofHandler(&s.config.Monitoring.Pprof)
			if err != nil {
				logging.Error().Msg("创建 pprof 处理器失败: " + err.Error())
			} else {
				router.GET(pprofHandler.Path(), pprofHandler.ServeHTTP)
				router.GET(pprofHandler.Path()+"/{profile:*}", pprofHandler.ServeHTTP)
			}
		}

		s.registerProxyRoutes(router, &s.config.Server)

		// 静态文件
		s.registerStaticHandlers(router, &s.config.Server)

		chain, err := s.buildMiddlewareChain(&s.config.Server)
		if err != nil {
			return err
		}

		handler := chain.Apply(router.Handler())
		if s.pool != nil {
			handler = s.pool.WrapHandler(handler)
		}
		vhostMgr.SetDefault(handler)
	}

	s.handler = vhostMgr.Handler()
	// 包装统计追踪
	s.handler = s.trackStats(s.handler)

	s.fastServer = &fasthttp.Server{
		Name:               "lolly",
		Handler:            s.handler,
		ReadTimeout:        s.config.Server.ReadTimeout,
		WriteTimeout:       s.config.Server.WriteTimeout,
		IdleTimeout:        s.config.Server.IdleTimeout,
		MaxConnsPerIP:      s.config.Server.MaxConnsPerIP,
		MaxRequestsPerConn: s.config.Server.MaxRequestsPerConn,
	}

	s.running = true

	// 创建监听器并保存，用于热升级
	ln, err := net.Listen("tcp", s.config.Server.Listen)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}
	s.listeners = []net.Listener{ln}

	// 检查是否配置了 SSL/TLS
	if s.config.Server.SSL.Cert != "" && s.config.Server.SSL.Key != "" {
		var err error
		s.tlsManager, err = ssl.NewTLSManager(&s.config.Server.SSL)
		if err != nil {
			return fmt.Errorf("创建 TLS 管理器失败: %w", err)
		}
		s.fastServer.TLSConfig = s.tlsManager.GetTLSConfig()
		return s.fastServer.ServeTLS(ln, "", "")
	}

	return s.fastServer.Serve(ln)
}

// registerProxyRoutes 注册代理路由。
//
// 根据配置为路由器注册代理路径，创建代理处理器和健康检查器。
// 支持 GET、POST、PUT、DELETE、HEAD 等 HTTP 方法。
//
// 参数：
//   - router: 路由器实例，用于注册路由规则
//   - serverCfg: 服务器配置，包含代理目标、负载均衡、健康检查等设置
//
// 注意事项：
//   - 代理目标初始状态默认为健康
//   - 健康检查根据配置自动启动
func (s *Server) registerProxyRoutes(router *handler.Router, serverCfg *config.ServerConfig) {
	for i := range serverCfg.Proxy {
		proxyCfg := &serverCfg.Proxy[i]

		// 转换目标
		targets := make([]*loadbalance.Target, len(proxyCfg.Targets))
		for j, t := range proxyCfg.Targets {
			targets[j] = &loadbalance.Target{
				URL:    t.URL,
				Weight: t.Weight,
			}
			targets[j].Healthy.Store(true)
		}

		// 传递 Transport 配置
		p, err := proxy.NewProxy(proxyCfg, targets, &s.config.Performance.Transport)
		if err != nil {
			logging.Error().Msg("创建代理失败: " + err.Error())
			continue
		}

		// 设置 DNS 解析器（如果已配置）
		if s.resolver != nil {
			p.SetResolver(s.resolver)
			if err := p.Start(); err != nil {
				logging.Error().Err(err).Msg("启动代理失败")
			}
		}

		// 启动健康检查
		if proxyCfg.HealthCheck.Interval > 0 {
			hc := proxy.NewHealthChecker(targets, &proxyCfg.HealthCheck)
			hc.Start()
			s.healthCheckers = append(s.healthCheckers, hc)
			// 设置被动健康检查
			p.SetHealthChecker(hc)
		}

		// 保存代理实例用于缓存统计
		s.proxies = append(s.proxies, p)

		router.GET(proxyCfg.Path, p.ServeHTTP)
		router.POST(proxyCfg.Path, p.ServeHTTP)
		router.PUT(proxyCfg.Path, p.ServeHTTP)
		router.DELETE(proxyCfg.Path, p.ServeHTTP)
		router.HEAD(proxyCfg.Path, p.ServeHTTP)
	}
}

// Stop 快速停止服务器。
//
// 立即停止服务器，不等待正在处理的请求完成。
// 停止所有健康检查器和访问日志中间件。
//
// 返回值：
//   - error: 停止过程中遇到的错误
//
// 注意事项：
//   - 对于生产环境，建议使用 GracefulStop 实现优雅关闭
func (s *Server) Stop() error {
	s.running = false

	// 停止 Goroutine 池
	if s.pool != nil {
		s.pool.Stop()
	}

	// 停止健康检查器
	for _, hc := range s.healthCheckers {
		hc.Stop()
	}

	// 关闭访问日志
	if s.accessLogMiddleware != nil {
		_ = s.accessLogMiddleware.Close()
	}

	// 关闭 TLS 管理器
	if s.tlsManager != nil {
		s.tlsManager.Close()
	}

	// 关闭 Lua 引擎
	if s.luaEngine != nil {
		s.luaEngine.Close()
		logging.Info().Msg("Lua 引擎已关闭")
	}

	if s.fastServer != nil {
		return s.fastServer.Shutdown()
	}
	return nil
}

// GracefulStop 优雅停止服务器。
//
// 等待正在处理的请求完成后再停止服务器，确保连接正常关闭。
// 如果超时时间到达仍有请求未完成，将返回超时错误。
//
// 参数：
//   - timeout: 优雅关闭的最大等待时间
//
// 返回值：
//   - error: 停止过程中遇到的错误，超时返回 context.DeadlineExceeded
//
// 注意事项：
//   - 推荐在生产环境使用此方法关闭服务器
//   - 超时后会强制关闭，可能导致部分请求中断
func (s *Server) GracefulStop(timeout time.Duration) error {
	s.running = false

	// 停止 Goroutine 池
	if s.pool != nil {
		s.pool.Stop()
	}

	// 停止健康检查器
	for _, hc := range s.healthCheckers {
		hc.Stop()
	}

	// 关闭访问日志
	if s.accessLogMiddleware != nil {
		_ = s.accessLogMiddleware.Close()
	}

	// 关闭 TLS 管理器
	if s.tlsManager != nil {
		s.tlsManager.Close()
	}

	// 关闭 Lua 引擎
	if s.luaEngine != nil {
		s.luaEngine.Close()
		logging.Info().Msg("Lua 引擎已关闭")
	}

	if s.fastServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		done := make(chan struct{})
		go func() {
			_ = s.fastServer.Shutdown()
			close(done)
		}()

		select {
		case <-done:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

// getProxyCacheStats 收集所有代理缓存的统计信息。
func (s *Server) getProxyCacheStats() ProxyCacheStats {
	var total ProxyCacheStats
	for _, p := range s.proxies {
		if stats := p.GetCacheStats(); stats != nil {
			total.Entries += stats.Entries
			total.Pending += stats.Pending
		}
	}
	return total
}

// registerStaticHandlers 注册静态文件处理器。
//
// 为路由器注册静态文件服务，支持多个静态目录、文件缓存和预压缩文件。
//
// 参数：
//   - router: 路由器实例，用于注册路由规则
//   - cfg: 服务器配置，包含静态文件和压缩设置
func (s *Server) registerStaticHandlers(router *handler.Router, cfg *config.ServerConfig) {
	for _, static := range cfg.Static {
		path := static.Path
		if path == "" {
			path = "/"
		}

		staticHandler := handler.NewStaticHandler(
			static.Root,
			path,
			static.Index,
			true, // useSendfile
		)
		if s.fileCache != nil {
			staticHandler.SetFileCache(s.fileCache)
		}
		if cfg.Compression.GzipStatic {
			staticHandler.SetGzipStatic(true, cfg.Compression.GzipStaticExtensions)
		}

		// 设置 try_files 配置
		if len(static.TryFiles) > 0 {
			// 注意：tryFilesPass 需要路由器支持，当前实现传入 nil
			// 如果 tryFilesPass 为 true，需要额外处理
			staticHandler.SetTryFiles(static.TryFiles, static.TryFilesPass, router)
		}

		// 注册路由：确保路径以 / 结尾
		routePath := path
		if !strings.HasSuffix(routePath, "/") {
			routePath += "/"
		}
		router.GET(routePath+"{filepath:*}", staticHandler.Handle)
		router.HEAD(routePath+"{filepath:*}", staticHandler.Handle)
	}
}

// SetResolver 设置 DNS 解析器。
func (s *Server) SetResolver(r resolver.Resolver) {
	s.resolver = r
}

// GetResolver 返回 DNS 解析器。
func (s *Server) GetResolver() resolver.Resolver {
	return s.resolver
}
