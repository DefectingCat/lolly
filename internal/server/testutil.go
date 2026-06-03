// Package server 提供测试工具函数和依赖注入支持
package server

import (
	"crypto/tls"
	"net"
	"time"

	"github.com/valyala/fasthttp"
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




// TestDependencies 包含测试时可注入的依赖
// 使用具体指针类型，允许注入 Mock 实现
type TestDependencies struct {
	LuaEngine  *lua.LuaEngine
	TLSManager *ssl.TLSManager
}



// TestServerOptions 测试服务器的可选配置
type TestServerOptions struct {
	MockFastServer    *MockFastServer
	CustomHandler     fasthttp.RequestHandler
	SkipListener      bool
	DisableMiddleware bool
}




