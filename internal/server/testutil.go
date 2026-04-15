// Package server 提供测试工具函数和依赖注入支持
package server

import (
	"crypto/tls"
	"net"
	"time"

	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/config"
	"rua.plus/lolly/internal/lua"
	"rua.plus/lolly/internal/ssl"
)

// MockFastServer 是 fasthttp.Server 的 Mock 包装
// 定义在此文件以便 TestServerOptions 可以引用
type MockFastServer struct {
	Handler            fasthttp.RequestHandler
	TLSConfig          *tls.Config
	ServeFunc          func(ln net.Listener) error
	ServeTLSFunc       func(ln net.Listener, certFile, keyFile string) error
	ShutdownFunc       func() error
	Name               string
	ReadTimeout        time.Duration
	WriteTimeout       time.Duration
	IdleTimeout        time.Duration
	MaxConnsPerIP      int
	MaxRequestsPerConn int
	CloseOnShutdown    bool
}

// Serve 启动服务
func (m *MockFastServer) Serve(ln net.Listener) error {
	if m.ServeFunc != nil {
		return m.ServeFunc(ln)
	}
	return nil
}

// ServeTLS 启动 TLS 服务
func (m *MockFastServer) ServeTLS(ln net.Listener, certFile, keyFile string) error {
	if m.ServeTLSFunc != nil {
		return m.ServeTLSFunc(ln, certFile, keyFile)
	}
	return nil
}

// Shutdown 关闭服务器
func (m *MockFastServer) Shutdown() error {
	if m.ShutdownFunc != nil {
		return m.ShutdownFunc()
	}
	return nil
}

// TestDependencies 包含测试时可注入的依赖
// 使用具体指针类型，允许注入 Mock 实现
type TestDependencies struct {
	LuaEngine  *lua.LuaEngine
	TLSManager *ssl.TLSManager
}

// NewServerForTesting 创建用于测试的服务器实例
// 允许注入 Mock 依赖，不改变生产 API
func NewServerForTesting(cfg *config.Config, deps *TestDependencies) *Server {
	s := New(cfg)
	if deps != nil {
		if deps.LuaEngine != nil {
			s.luaEngine = deps.LuaEngine
		}
		if deps.TLSManager != nil {
			s.tlsManager = deps.TLSManager
		}
	}
	return s
}

// TestServerOptions 测试服务器的可选配置
type TestServerOptions struct {
	MockFastServer    *MockFastServer
	CustomHandler     fasthttp.RequestHandler
	SkipListener      bool
	DisableMiddleware bool
}

// NewTestServerWithOptions 使用选项创建测试服务器
func NewTestServerWithOptions(cfg *config.Config, opts *TestServerOptions) *Server {
	s := New(cfg)

	if opts != nil {
		// 可以在这里应用各种测试选项
		if opts.CustomHandler != nil {
			s.handler = opts.CustomHandler
		}
	}

	return s
}

// MustStartTestServer 启动测试服务器，失败时 panic
// 主要用于集成测试
func MustStartTestServer(cfg *config.Config) *Server {
	s := New(cfg)
	// 在测试环境中使用随机端口避免冲突
	listenAddr := ""
	if len(cfg.Servers) > 0 {
		listenAddr = cfg.Servers[0].Listen
	} else {
		listenAddr = cfg.Server.Listen
	}
	if listenAddr == "" || listenAddr == ":80" {
		if len(cfg.Servers) > 0 {
			cfg.Servers[0].Listen = "127.0.0.1:0"
		} else {
			cfg.Server.Listen = "127.0.0.1:0"
		}
	}

	// 使用 goroutine 启动服务器以避免阻塞
	go func() {
		if err := s.Start(); err != nil {
			// 测试服务器启动失败记录日志
			panic("failed to start test server: " + err.Error())
		}
	}()

	// 给服务器一点时间启动
	time.Sleep(10 * time.Millisecond)

	return s
}
