package server

import (
	"net"
	"testing"
	"time"

	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/config"
)

// TestNew 测试服务器创建
func TestNew(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Listen: ":8080",
			Static: config.StaticConfig{
				Root:  "./static",
				Index: []string{"index.html"},
			},
		},
	}

	s := New(cfg)
	if s == nil {
		t.Fatal("New() returned nil, expected non-nil Server")
	}

	if s.config != cfg {
		t.Error("Server.config not set correctly")
	}

	if s.running {
		t.Error("Server.running should be false initially")
	}

	if s.fastServer != nil {
		t.Error("Server.fastServer should be nil before Start()")
	}
}

// TestStopWithoutServer 测试无服务器时调用 Stop
func TestStopWithoutServer(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Listen: ":8080",
		},
	}

	s := New(cfg)

	// 在未启动时调用 Stop，应返回 nil
	err := s.Stop()
	if err != nil {
		t.Errorf("Stop() on non-started server returned error: %v", err)
	}
}

// TestGracefulStop 测试 GracefulStop 调用
func TestGracefulStop(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Listen: ":8080",
		},
	}

	s := New(cfg)

	// 在未启动时调用 GracefulStop，应返回 nil
	err := s.GracefulStop(5 * time.Second)
	if err != nil {
		t.Errorf("GracefulStop() on non-started server returned error: %v", err)
	}
}

// TestStopAfterStop 测试多次调用 Stop
func TestStopAfterStop(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Listen: ":8080",
		},
	}

	s := New(cfg)

	// 多次调用 Stop 应该都是安全的
	for i := 0; i < 3; i++ {
		err := s.Stop()
		if err != nil {
			t.Errorf("Stop() call %d returned error: %v", i+1, err)
		}
	}
}

// TestGracefulStopWithZeroTimeout 测试零超时的 GracefulStop
func TestGracefulStopWithZeroTimeout(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Listen: ":8080",
		},
	}

	s := New(cfg)

	err := s.GracefulStop(0)
	if err != nil {
		t.Errorf("GracefulStop(0) returned error: %v", err)
	}
}

// TestBuildMiddlewareChain_AccessLog 测试访问日志中间件
func TestBuildMiddlewareChain_AccessLog(t *testing.T) {
	cfg := &config.Config{
		Logging: config.LoggingConfig{},
		Server: config.ServerConfig{
			Listen: ":8080",
		},
	}

	s := New(cfg)
	chain, err := s.buildMiddlewareChain(&cfg.Server)
	if err != nil {
		t.Errorf("buildMiddlewareChain failed: %v", err)
	}
	if chain == nil {
		t.Error("Expected non-nil chain")
	}
}

// TestBuildMiddlewareChain_AccessControl 测试访问控制中间件
func TestBuildMiddlewareChain_AccessControl(t *testing.T) {
	cfg := &config.Config{
		Logging: config.LoggingConfig{},
		Server: config.ServerConfig{
			Listen: ":8080",
			Security: config.SecurityConfig{
				Access: config.AccessConfig{
					Allow: []string{"127.0.0.1"},
				},
			},
		},
	}

	s := New(cfg)
	chain, err := s.buildMiddlewareChain(&cfg.Server)
	if err != nil {
		t.Errorf("buildMiddlewareChain failed: %v", err)
	}
	if chain == nil {
		t.Error("Expected non-nil chain")
	}
}

// TestBuildMiddlewareChain_RateLimiter 测试限流中间件
func TestBuildMiddlewareChain_RateLimiter(t *testing.T) {
	cfg := &config.Config{
		Logging: config.LoggingConfig{},
		Server: config.ServerConfig{
			Listen: ":8080",
			Security: config.SecurityConfig{
				RateLimit: config.RateLimitConfig{
					RequestRate: 100,
					Burst:       200,
				},
			},
		},
	}

	s := New(cfg)
	chain, err := s.buildMiddlewareChain(&cfg.Server)
	if err != nil {
		t.Errorf("buildMiddlewareChain failed: %v", err)
	}
	if chain == nil {
		t.Error("Expected non-nil chain")
	}
}

// TestBuildMiddlewareChain_Rewrite 测试重写中间件
func TestBuildMiddlewareChain_Rewrite(t *testing.T) {
	cfg := &config.Config{
		Logging: config.LoggingConfig{},
		Server: config.ServerConfig{
			Listen: ":8080",
			Rewrite: []config.RewriteRule{
				{Pattern: "/old/(.*)", Replacement: "/new/$1"},
			},
		},
	}

	s := New(cfg)
	chain, err := s.buildMiddlewareChain(&cfg.Server)
	if err != nil {
		t.Errorf("buildMiddlewareChain failed: %v", err)
	}
	if chain == nil {
		t.Error("Expected non-nil chain")
	}
}

// TestBuildMiddlewareChain_Compression 测试压缩中间件
func TestBuildMiddlewareChain_Compression(t *testing.T) {
	cfg := &config.Config{
		Logging: config.LoggingConfig{},
		Server: config.ServerConfig{
			Listen: ":8080",
			Compression: config.CompressionConfig{
				Level: 6,
			},
		},
	}

	s := New(cfg)
	chain, err := s.buildMiddlewareChain(&cfg.Server)
	if err != nil {
		t.Errorf("buildMiddlewareChain failed: %v", err)
	}
	if chain == nil {
		t.Error("Expected non-nil chain")
	}
}

// TestBuildMiddlewareChain_SecurityHeaders 测试安全头中间件
func TestBuildMiddlewareChain_SecurityHeaders(t *testing.T) {
	cfg := &config.Config{
		Logging: config.LoggingConfig{},
		Server: config.ServerConfig{
			Listen: ":8080",
			Security: config.SecurityConfig{
				Headers: config.SecurityHeaders{
					XFrameOptions:       "DENY",
					XContentTypeOptions: "nosniff",
				},
			},
		},
	}

	s := New(cfg)
	chain, err := s.buildMiddlewareChain(&cfg.Server)
	if err != nil {
		t.Errorf("buildMiddlewareChain failed: %v", err)
	}
	if chain == nil {
		t.Error("Expected non-nil chain")
	}
}

// TestBuildMiddlewareChain_AllMiddlewares 测试所有中间件组合
func TestBuildMiddlewareChain_AllMiddlewares(t *testing.T) {
	cfg := &config.Config{
		Logging: config.LoggingConfig{},
		Server: config.ServerConfig{
			Listen: ":8080",
			Security: config.SecurityConfig{
				Access: config.AccessConfig{
					Allow: []string{"127.0.0.1"},
				},
				RateLimit: config.RateLimitConfig{
					RequestRate: 100,
					Burst:       200,
				},
				Headers: config.SecurityHeaders{
					XFrameOptions: "DENY",
				},
			},
			Rewrite: []config.RewriteRule{
				{Pattern: "/old/(.*)", Replacement: "/new/$1"},
			},
			Compression: config.CompressionConfig{
				Level: 6,
			},
		},
	}

	s := New(cfg)
	chain, err := s.buildMiddlewareChain(&cfg.Server)
	if err != nil {
		t.Errorf("buildMiddlewareChain failed: %v", err)
	}
	if chain == nil {
		t.Error("Expected non-nil chain")
	}
}

// TestTrackStats 测试请求统计追踪
func TestTrackStats(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Listen: ":8080",
		},
	}

	s := New(cfg)

	// 初始统计应该为 0
	if s.requests.Load() != 0 {
		t.Error("Initial requests should be 0")
	}
	if s.bytesSent.Load() != 0 {
		t.Error("Initial bytesSent should be 0")
	}
	if s.bytesReceived.Load() != 0 {
		t.Error("Initial bytesReceived should be 0")
	}

	// 创建测试 handler
	handler := func(ctx *fasthttp.RequestCtx) {
		ctx.SetBodyString("response body")
	}

	// 包装 handler
	wrappedHandler := s.trackStats(handler)

	// 创建测试请求上下文
	ctx := &fasthttp.RequestCtx{}
	ctx.Init(&fasthttp.Request{}, nil, nil)
	ctx.Request.SetBody([]byte("request body"))

	// 执行
	wrappedHandler(ctx)

	// 验证统计
	if s.requests.Load() != 1 {
		t.Errorf("Expected 1 request, got %d", s.requests.Load())
	}

	if s.bytesReceived.Load() != int64(len("request body")) {
		t.Errorf("Expected bytesReceived %d, got %d", len("request body"), s.bytesReceived.Load())
	}

	if s.bytesSent.Load() != int64(len("response body")) {
		t.Errorf("Expected bytesSent %d, got %d", len("response body"), s.bytesSent.Load())
	}
}

// TestTrackStats_MultipleRequests 测试多次请求统计
func TestTrackStats_MultipleRequests(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Listen: ":8080",
		},
	}

	s := New(cfg)

	handler := func(ctx *fasthttp.RequestCtx) {
		ctx.SetBodyString("ok")
	}

	wrappedHandler := s.trackStats(handler)

	// 执行多次请求
	for i := 0; i < 10; i++ {
		ctx := &fasthttp.RequestCtx{}
		ctx.Init(&fasthttp.Request{}, nil, nil)
		wrappedHandler(ctx)
	}

	if s.requests.Load() != 10 {
		t.Errorf("Expected 10 requests, got %d", s.requests.Load())
	}
}

// TestGetListeners_Empty 测试空监听器列表
func TestGetListeners_Empty(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Listen: ":8080",
		},
	}

	s := New(cfg)

	listeners := s.GetListeners()
	if listeners != nil {
		t.Errorf("Expected nil listeners, got %v", listeners)
	}
}

// TestSetListeners 测试设置监听器
func TestSetListeners(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Listen: ":8080",
		},
	}

	s := New(cfg)

	// 创建模拟监听器
	listener1, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	defer func() { _ = listener1.Close() }()

	listener2, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	defer func() { _ = listener2.Close() }()

	listeners := []net.Listener{listener1, listener2}
	s.SetListeners(listeners)

	// 验证设置成功
	got := s.GetListeners()
	if len(got) != 2 {
		t.Errorf("Expected 2 listeners, got %d", len(got))
	}
}

// TestGetTLSConfig_NotConfigured 测试未配置 TLS
func TestGetTLSConfig_NotConfigured(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Listen: ":8080",
		},
	}

	s := New(cfg)

	tlsConfig, err := s.GetTLSConfig()
	if err == nil {
		t.Error("Expected error for unconfigured TLS")
	}
	if tlsConfig != nil {
		t.Error("Expected nil TLS config")
	}
	if err.Error() != "TLS not configured" {
		t.Errorf("Expected error 'TLS not configured', got: %v", err)
	}
}

// TestGetHandler 测试获取 handler
func TestGetHandler(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Listen: ":8080",
		},
	}

	s := New(cfg)

	// 初始 handler 应该为 nil
	handler := s.GetHandler()
	if handler != nil {
		t.Error("Expected nil handler initially")
	}

	// 设置一个 handler
	testHandler := func(ctx *fasthttp.RequestCtx) {
		ctx.SetBodyString("test")
	}
	s.handler = testHandler

	// 验证获取成功
	got := s.GetHandler()
	if got == nil {
		t.Error("Expected non-nil handler after setting")
	}
}

// TestServer_Connections 测试连接统计
func TestServer_Connections(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Listen: ":8080",
		},
	}

	s := New(cfg)

	// 初始连接数应该为 0
	if s.connections.Load() != 0 {
		t.Error("Initial connections should be 0")
	}

	// 增加
	s.connections.Add(1)
	if s.connections.Load() != 1 {
		t.Errorf("Expected 1 connection, got %d", s.connections.Load())
	}

	// 减少
	s.connections.Add(-1)
	if s.connections.Load() != 0 {
		t.Errorf("Expected 0 connections, got %d", s.connections.Load())
	}
}
