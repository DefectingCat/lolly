package server

import (
	"time"

	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/config"
	"rua.plus/lolly/internal/handler"
	"rua.plus/lolly/internal/logging"
	"rua.plus/lolly/internal/middleware"
)

// Server HTTP 服务器
type Server struct {
	config     *config.Config
	fastServer *fasthttp.Server
	handler    fasthttp.RequestHandler
	running    bool
}

// New 创建服务器
func New(cfg *config.Config) *Server {
	return &Server{config: cfg}
}

// Start 启动服务器
func (s *Server) Start() error {
	// 初始化日志
	logging.Init(s.config.Logging.Error.Level, true)

	// 创建路由
	router := handler.NewRouter()

	// 静态文件服务
	staticHandler := handler.NewStaticHandler(
		s.config.Server.Static.Root,
		s.config.Server.Static.Index,
	)

	// 注册路由 - 处理所有路径
	router.GET("/{filepath:*}", staticHandler.Handle)
	router.HEAD("/{filepath:*}", staticHandler.Handle)

	// 应用中间件
	chain := middleware.NewChain()
	s.handler = chain.Apply(router.Handler())

	// 创建 fasthttp 服务器
	s.fastServer = &fasthttp.Server{
		Handler:            s.handler,
		ReadTimeout:        30 * time.Second,
		WriteTimeout:       30 * time.Second,
		IdleTimeout:        120 * time.Second,
		MaxConnsPerIP:      1000,
		MaxRequestsPerConn: 10000,
	}

	s.running = true
	return s.fastServer.ListenAndServe(s.config.Server.Listen)
}

// Stop 快速停止服务器
func (s *Server) Stop() error {
	s.running = false
	if s.fastServer != nil {
		return s.fastServer.Shutdown()
	}
	return nil
}

// GracefulStop 优雅停止
func (s *Server) GracefulStop(timeout time.Duration) error {
	return s.Stop()
}
