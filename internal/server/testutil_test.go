// Package server 提供测试工具函数的测试。
package server

import (
	"errors"
	"net"
	"testing"
	"time"

	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/config"
	"rua.plus/lolly/internal/lua"
	"rua.plus/lolly/internal/ssl"
)

// TestMockFastServer_Serve 测试 MockFastServer.Serve 方法
func TestMockFastServer_Serve(t *testing.T) {
	t.Run("with custom ServeFunc", func(t *testing.T) {
		called := false
		mock := &MockFastServer{
			ServeFunc: func(ln net.Listener) error {
				called = true
				return nil
			},
		}

		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("failed to create listener: %v", err)
		}
		defer ln.Close()

		err = mock.Serve(ln)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if !called {
			t.Error("ServeFunc was not called")
		}
	})

	t.Run("without ServeFunc", func(t *testing.T) {
		mock := &MockFastServer{}

		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("failed to create listener: %v", err)
		}
		defer ln.Close()

		err = mock.Serve(ln)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("with error from ServeFunc", func(t *testing.T) {
		expectedErr := errors.New("serve error")
		mock := &MockFastServer{
			ServeFunc: func(ln net.Listener) error {
				return expectedErr
			},
		}

		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("failed to create listener: %v", err)
		}
		defer ln.Close()

		err = mock.Serve(ln)
		if err != expectedErr {
			t.Errorf("expected error %v, got %v", expectedErr, err)
		}
	})
}

// TestMockFastServer_ServeTLS 测试 MockFastServer.ServeTLS 方法
func TestMockFastServer_ServeTLS(t *testing.T) {
	t.Run("with custom ServeTLSFunc", func(t *testing.T) {
		called := false
		mock := &MockFastServer{
			ServeTLSFunc: func(ln net.Listener, certFile, keyFile string) error {
				called = true
				if certFile != "cert.pem" {
					t.Errorf("expected certFile cert.pem, got %s", certFile)
				}
				if keyFile != "key.pem" {
					t.Errorf("expected keyFile key.pem, got %s", keyFile)
				}
				return nil
			},
		}

		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("failed to create listener: %v", err)
		}
		defer ln.Close()

		err = mock.ServeTLS(ln, "cert.pem", "key.pem")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if !called {
			t.Error("ServeTLSFunc was not called")
		}
	})

	t.Run("without ServeTLSFunc", func(t *testing.T) {
		mock := &MockFastServer{}

		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("failed to create listener: %v", err)
		}
		defer ln.Close()

		err = mock.ServeTLS(ln, "cert.pem", "key.pem")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

// TestMockFastServer_Shutdown 测试 MockFastServer.Shutdown 方法
func TestMockFastServer_Shutdown(t *testing.T) {
	t.Run("with custom ShutdownFunc", func(t *testing.T) {
		called := false
		mock := &MockFastServer{
			ShutdownFunc: func() error {
				called = true
				return nil
			},
		}

		err := mock.Shutdown()
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if !called {
			t.Error("ShutdownFunc was not called")
		}
	})

	t.Run("without ShutdownFunc", func(t *testing.T) {
		mock := &MockFastServer{}

		err := mock.Shutdown()
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("with error from ShutdownFunc", func(t *testing.T) {
		expectedErr := errors.New("shutdown error")
		mock := &MockFastServer{
			ShutdownFunc: func() error {
				return expectedErr
			},
		}

		err := mock.Shutdown()
		if err != expectedErr {
			t.Errorf("expected error %v, got %v", expectedErr, err)
		}
	})
}

// TestNewServerForTesting 测试 NewServerForTesting 函数
func TestNewServerForTesting(t *testing.T) {
	t.Run("with nil deps", func(t *testing.T) {
		cfg := &config.Config{
			Servers: []config.ServerConfig{{
				Listen: ":8080",
			}},
		}

		s := NewServerForTesting(cfg, nil)
		if s == nil {
			t.Fatal("expected non-nil server")
		}
		if s.config != cfg {
			t.Error("config not set correctly")
		}
	})

	t.Run("with lua engine", func(t *testing.T) {
		cfg := &config.Config{
			Servers: []config.ServerConfig{{
				Listen: ":8080",
			}},
		}

		luaEngine := &lua.LuaEngine{}
		deps := &TestDependencies{
			LuaEngine: luaEngine,
		}

		s := NewServerForTesting(cfg, deps)
		if s == nil {
			t.Fatal("expected non-nil server")
		}
		if s.luaEngine != luaEngine {
			t.Error("lua engine not set correctly")
		}
	})

	t.Run("with TLS manager", func(t *testing.T) {
		cfg := &config.Config{
			Servers: []config.ServerConfig{{
				Listen: ":8080",
			}},
		}

		tlsManager := &ssl.TLSManager{}
		deps := &TestDependencies{
			TLSManager: tlsManager,
		}

		s := NewServerForTesting(cfg, deps)
		if s == nil {
			t.Fatal("expected non-nil server")
		}
		if s.tlsManager != tlsManager {
			t.Error("TLS manager not set correctly")
		}
	})

	t.Run("with all deps", func(t *testing.T) {
		cfg := &config.Config{
			Servers: []config.ServerConfig{{
				Listen: ":8080",
			}},
		}

		luaEngine := &lua.LuaEngine{}
		tlsManager := &ssl.TLSManager{}
		deps := &TestDependencies{
			LuaEngine:  luaEngine,
			TLSManager: tlsManager,
		}

		s := NewServerForTesting(cfg, deps)
		if s == nil {
			t.Fatal("expected non-nil server")
		}
		if s.luaEngine != luaEngine {
			t.Error("lua engine not set correctly")
		}
		if s.tlsManager != tlsManager {
			t.Error("TLS manager not set correctly")
		}
	})
}

// TestNewTestServerWithOptions 测试 NewTestServerWithOptions 函数
func TestNewTestServerWithOptions(t *testing.T) {
	t.Run("with nil opts", func(t *testing.T) {
		cfg := &config.Config{
			Servers: []config.ServerConfig{{
				Listen: ":8080",
			}},
		}

		s := NewTestServerWithOptions(cfg, nil)
		if s == nil {
			t.Fatal("expected non-nil server")
		}
		if s.config != cfg {
			t.Error("config not set correctly")
		}
	})

	t.Run("with custom handler", func(t *testing.T) {
		cfg := &config.Config{
			Servers: []config.ServerConfig{{
				Listen: ":8080",
			}},
		}

		customHandler := func(ctx *fasthttp.RequestCtx) {
			ctx.SetBodyString("custom response")
		}

		opts := &TestServerOptions{
			CustomHandler: customHandler,
		}

		s := NewTestServerWithOptions(cfg, opts)
		if s == nil {
			t.Fatal("expected non-nil server")
		}
		if s.handler == nil {
			t.Error("handler should be set")
		}
	})

	t.Run("with empty opts", func(t *testing.T) {
		cfg := &config.Config{
			Servers: []config.ServerConfig{{
				Listen: ":8080",
			}},
		}

		opts := &TestServerOptions{}

		s := NewTestServerWithOptions(cfg, opts)
		if s == nil {
			t.Fatal("expected non-nil server")
		}
	})

	t.Run("with mock fast server", func(t *testing.T) {
		cfg := &config.Config{
			Servers: []config.ServerConfig{{
				Listen: ":8080",
			}},
		}

		opts := &TestServerOptions{
			MockFastServer: &MockFastServer{
				Name: "test-server",
			},
		}

		s := NewTestServerWithOptions(cfg, opts)
		if s == nil {
			t.Fatal("expected non-nil server")
		}
	})
}

// TestMustStartTestServer 测试 MustStartTestServer 函数
func TestMustStartTestServer(t *testing.T) {
	t.Run("basic server start", func(t *testing.T) {
		cfg := &config.Config{
			Servers: []config.ServerConfig{{
				Listen: "127.0.0.1:0", // 随机端口
			}},
		}

		s := MustStartTestServer(cfg)
		if s == nil {
			t.Fatal("expected non-nil server")
		}

		// 给服务器一点时间启动
		time.Sleep(20 * time.Millisecond)

		// 停止服务器
		_ = s.StopWithTimeout(1 * time.Second)
	})

	t.Run("with empty listen address", func(t *testing.T) {
		cfg := &config.Config{
			Servers: []config.ServerConfig{{
				Listen: "",
			}},
		}

		s := MustStartTestServer(cfg)
		if s == nil {
			t.Fatal("expected non-nil server")
		}

		// 给服务器一点时间启动
		time.Sleep(20 * time.Millisecond)

		// 停止服务器
		_ = s.StopWithTimeout(1 * time.Second)
	})

	t.Run("with default port", func(t *testing.T) {
		cfg := &config.Config{
			Servers: []config.ServerConfig{{
				Listen: ":80",
			}},
		}

		s := MustStartTestServer(cfg)
		if s == nil {
			t.Fatal("expected non-nil server")
		}

		// 给服务器一点时间启动
		time.Sleep(20 * time.Millisecond)

		// 停止服务器
		_ = s.StopWithTimeout(1 * time.Second)
	})

	t.Run("with multiple servers", func(t *testing.T) {
		cfg := &config.Config{
			Servers: []config.ServerConfig{
				{Listen: "127.0.0.1:0"},
				{Listen: "127.0.0.1:0"},
			},
		}

		s := MustStartTestServer(cfg)
		if s == nil {
			t.Fatal("expected non-nil server")
		}

		// 给服务器一点时间启动
		time.Sleep(20 * time.Millisecond)

		// 停止服务器
		_ = s.StopWithTimeout(1 * time.Second)
	})
}

// TestTestDependencies 测试 TestDependencies 结构体
func TestTestDependencies(t *testing.T) {
	t.Run("empty dependencies", func(t *testing.T) {
		deps := &TestDependencies{}
		if deps.LuaEngine != nil {
			t.Error("LuaEngine should be nil")
		}
		if deps.TLSManager != nil {
			t.Error("TLSManager should be nil")
		}
	})

	t.Run("with lua engine only", func(t *testing.T) {
		luaEngine := &lua.LuaEngine{}
		deps := &TestDependencies{
			LuaEngine: luaEngine,
		}
		if deps.LuaEngine != luaEngine {
			t.Error("LuaEngine not set correctly")
		}
	})
}

// TestTestServerOptions 测试 TestServerOptions 结构体
func TestTestServerOptions(t *testing.T) {
	t.Run("empty options", func(t *testing.T) {
		opts := &TestServerOptions{}
		if opts.MockFastServer != nil {
			t.Error("MockFastServer should be nil")
		}
		if opts.CustomHandler != nil {
			t.Error("CustomHandler should be nil")
		}
		if opts.SkipListener {
			t.Error("SkipListener should be false")
		}
		if opts.DisableMiddleware {
			t.Error("DisableMiddleware should be false")
		}
	})

	t.Run("with all options", func(t *testing.T) {
		mock := &MockFastServer{Name: "test"}
		handler := func(ctx *fasthttp.RequestCtx) {}

		opts := &TestServerOptions{
			MockFastServer:    mock,
			CustomHandler:     handler,
			SkipListener:      true,
			DisableMiddleware: true,
		}

		if opts.MockFastServer != mock {
			t.Error("MockFastServer not set correctly")
		}
		if opts.CustomHandler == nil {
			t.Error("CustomHandler should be set")
		}
		if !opts.SkipListener {
			t.Error("SkipListener should be true")
		}
		if !opts.DisableMiddleware {
			t.Error("DisableMiddleware should be true")
		}
	})
}

// TestMockFastServer_Fields 测试 MockFastServer 字段
func TestMockFastServer_Fields(t *testing.T) {
	mock := &MockFastServer{
		Name:               "test-server",
		ReadTimeout:        10 * time.Second,
		WriteTimeout:       20 * time.Second,
		IdleTimeout:        30 * time.Second,
		MaxConnsPerIP:      100,
		MaxRequestsPerConn: 1000,
		CloseOnShutdown:    true,
	}

	if mock.Name != "test-server" {
		t.Errorf("expected Name test-server, got %s", mock.Name)
	}
	if mock.ReadTimeout != 10*time.Second {
		t.Errorf("expected ReadTimeout 10s, got %v", mock.ReadTimeout)
	}
	if mock.WriteTimeout != 20*time.Second {
		t.Errorf("expected WriteTimeout 20s, got %v", mock.WriteTimeout)
	}
	if mock.IdleTimeout != 30*time.Second {
		t.Errorf("expected IdleTimeout 30s, got %v", mock.IdleTimeout)
	}
	if mock.MaxConnsPerIP != 100 {
		t.Errorf("expected MaxConnsPerIP 100, got %d", mock.MaxConnsPerIP)
	}
	if mock.MaxRequestsPerConn != 1000 {
		t.Errorf("expected MaxRequestsPerConn 1000, got %d", mock.MaxRequestsPerConn)
	}
	if !mock.CloseOnShutdown {
		t.Error("CloseOnShutdown should be true")
	}
}
