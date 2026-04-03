package server

import (
	"context"
	"time"

	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/config"
	"rua.plus/lolly/internal/handler"
	"rua.plus/lolly/internal/loadbalance"
	"rua.plus/lolly/internal/logging"
	"rua.plus/lolly/internal/middleware"
	"rua.plus/lolly/internal/middleware/accesslog"
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
}

// New 创建服务器
func New(cfg *config.Config) *Server {
	return &Server{config: cfg}
}

// Start 启动服务器
func (s *Server) Start() error {
	logging.Init(s.config.Logging.Error.Level, true)

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
	staticHandler := handler.NewStaticHandler(
		s.config.Server.Static.Root,
		s.config.Server.Static.Index,
	)
	router.GET("/{filepath:*}", staticHandler.Handle)
	router.HEAD("/{filepath:*}", staticHandler.Handle)

	// 创建访问日志中间件
	s.accessLogMiddleware = accesslog.New(&s.config.Logging)

	chain := middleware.NewChain(s.accessLogMiddleware)
	s.handler = chain.Apply(router.Handler())

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

	// 创建访问日志中间件（共享给所有虚拟主机）
	s.accessLogMiddleware = accesslog.New(&s.config.Logging)
	chain := middleware.NewChain(s.accessLogMiddleware)

	for i := range s.config.Servers {
		router := handler.NewRouter()
		s.registerProxyRoutes(router, &s.config.Servers[i])

		// 静态文件
		staticHandler := handler.NewStaticHandler(
			s.config.Servers[i].Static.Root,
			s.config.Servers[i].Static.Index,
		)
		router.GET("/{filepath:*}", staticHandler.Handle)
		router.HEAD("/{filepath:*}", staticHandler.Handle)

		vhostMgr.AddHost(s.config.Servers[i].Name, chain.Apply(router.Handler()))
	}

	// 默认主机
	if s.config.HasDefaultServer() {
		router := handler.NewRouter()
		s.registerProxyRoutes(router, &s.config.Server)
		staticHandler := handler.NewStaticHandler(
			s.config.Server.Static.Root,
			s.config.Server.Static.Index,
		)
		router.GET("/{filepath:*}", staticHandler.Handle)
		vhostMgr.SetDefault(chain.Apply(router.Handler()))
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
