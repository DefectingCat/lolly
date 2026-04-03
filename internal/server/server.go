package server

import (
	"context"
	"fmt"
	"time"

	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/cache"
	"rua.plus/lolly/internal/config"
	"rua.plus/lolly/internal/handler"
	"rua.plus/lolly/internal/loadbalance"
	"rua.plus/lolly/internal/logging"
	"rua.plus/lolly/internal/middleware"
	"rua.plus/lolly/internal/middleware/accesslog"
	"rua.plus/lolly/internal/middleware/compression"
	"rua.plus/lolly/internal/middleware/rewrite"
	"rua.plus/lolly/internal/middleware/security"
	"rua.plus/lolly/internal/proxy"
)

// Server HTTP 服务器
type Server struct {
	config              *config.Config
	fastServer          *fasthttp.Server
	handler             fasthttp.RequestHandler
	running             bool
	healthCheckers      []*proxy.HealthChecker
	accessLogMiddleware *accesslog.AccessLog
	pool                *GoroutinePool  // Goroutine 池（可选）
	fileCache           *cache.FileCache // 文件缓存（可选）
}

// New 创建服务器
func New(cfg *config.Config) *Server {
	return &Server{config: cfg}
}

// buildMiddlewareChain 构建中间件链
// 按顺序：AccessLog -> AccessControl -> RateLimiter -> BasicAuth -> Rewrite -> Compression -> SecurityHeaders
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

	// 4. Security: BasicAuth (认证)
	if len(serverCfg.Security.Auth.Users) > 0 {
		auth, err := security.NewBasicAuth(&serverCfg.Security.Auth)
		if err != nil {
			return nil, fmt.Errorf("创建认证中间件失败: %w", err)
		}
		middlewares = append(middlewares, auth)
	}

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
		headers := security.NewSecurityHeaders(&serverCfg.Security.Headers)
		middlewares = append(middlewares, headers)
	}

	return middleware.NewChain(middlewares...), nil
}

// Start 启动服务器
func (s *Server) Start() error {
	logging.Init(s.config.Logging.Error.Level, true)

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

	if s.config.HasServers() {
		return s.startVHostMode()
	}
	return s.startSingleMode()
}

// startSingleMode 单服务器模式
func (s *Server) startSingleMode() error {
	router := handler.NewRouter()

	// 注册代理路由
	s.registerProxyRoutes(router, &s.config.Server)

	// 静态文件服务（作为 fallback）
	// 启用零拷贝传输优化（大文件使用 sendfile）
	staticHandler := handler.NewStaticHandler(
		s.config.Server.Static.Root,
		s.config.Server.Static.Index,
		true, // useSendfile
	)
	// 设置文件缓存
	if s.fileCache != nil {
		staticHandler.SetFileCache(s.fileCache)
	}
	router.GET("/{filepath:*}", staticHandler.Handle)
	router.HEAD("/{filepath:*}", staticHandler.Handle)

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
	return s.fastServer.ListenAndServe(s.config.Server.Listen)
}

// startVHostMode 虚拟主机模式
func (s *Server) startVHostMode() error {
	vhostMgr := NewVHostManager()

	for i := range s.config.Servers {
		router := handler.NewRouter()
		s.registerProxyRoutes(router, &s.config.Servers[i])

		// 静态文件
		staticHandler := handler.NewStaticHandler(
			s.config.Servers[i].Static.Root,
			s.config.Servers[i].Static.Index,
			true, // useSendfile
		)
		if s.fileCache != nil {
			staticHandler.SetFileCache(s.fileCache)
		}
		router.GET("/{filepath:*}", staticHandler.Handle)
		router.HEAD("/{filepath:*}", staticHandler.Handle)

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
		s.registerProxyRoutes(router, &s.config.Server)
		staticHandler := handler.NewStaticHandler(
			s.config.Server.Static.Root,
			s.config.Server.Static.Index,
			true, // useSendfile
		)
		if s.fileCache != nil {
			staticHandler.SetFileCache(s.fileCache)
		}
		router.GET("/{filepath:*}", staticHandler.Handle)

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
	return s.fastServer.ListenAndServe(s.config.Server.Listen)
}

// registerProxyRoutes 注册代理路由
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

		p, err := proxy.NewProxy(proxyCfg, targets)
		if err != nil {
			logging.Error().Msg("创建代理失败: " + err.Error())
			continue
		}

		// 启动健康检查
		if proxyCfg.HealthCheck.Interval > 0 {
			hc := proxy.NewHealthChecker(targets, &proxyCfg.HealthCheck)
			hc.Start()
			s.healthCheckers = append(s.healthCheckers, hc)
		}

		router.GET(proxyCfg.Path, p.ServeHTTP)
		router.POST(proxyCfg.Path, p.ServeHTTP)
		router.PUT(proxyCfg.Path, p.ServeHTTP)
		router.DELETE(proxyCfg.Path, p.ServeHTTP)
		router.HEAD(proxyCfg.Path, p.ServeHTTP)
	}
}

// Stop 快速停止服务器
func (s *Server) Stop() error {
	s.running = false

	// 停止健康检查器
	for _, hc := range s.healthCheckers {
		hc.Stop()
	}

	// 关闭访问日志
	if s.accessLogMiddleware != nil {
		s.accessLogMiddleware.Close()
	}

	if s.fastServer != nil {
		return s.fastServer.Shutdown()
	}
	return nil
}

// GracefulStop 优雅停止
func (s *Server) GracefulStop(timeout time.Duration) error {
	s.running = false

	// 停止健康检查器
	for _, hc := range s.healthCheckers {
		hc.Stop()
	}

	// 关闭访问日志
	if s.accessLogMiddleware != nil {
		s.accessLogMiddleware.Close()
	}

	if s.fastServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		done := make(chan struct{})
		go func() {
			s.fastServer.Shutdown()
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
