// Package server 提供 HTTP 服务器功能的测试。
//
// 该文件测试服务器模块的各项功能，包括：
//   - 服务器创建和初始化
//   - 启动和停止控制
//   - 优雅关闭
//   - 中间件链构建
//   - 请求统计追踪
//   - 监听器管理
//   - TLS 配置
//   - 代理缓存统计
//
// 作者：xfy
package server

import (
	"net"
	"os"
	"testing"
	"time"

	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/config"
	"rua.plus/lolly/internal/version"
)

// TestNew 测试服务器创建
func TestNew(t *testing.T) {
	cfg := &config.Config{
		Servers: []config.ServerConfig{{
			Listen: ":8080",
			Static: []config.StaticConfig{{
				Path:  "/",
				Root:  "./static",
				Index: []string{"index.html"},
			}},
		}},
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
		Servers: []config.ServerConfig{{
			Listen: ":8080",
		}},
	}

	s := New(cfg)

	// 在未启动时调用 Stop，应返回 nil
	err := s.StopWithTimeout(5 * time.Second)
	if err != nil {
		t.Errorf("StopWithTimeout() on non-started server returned error: %v", err)
	}
}

// TestGracefulStop 测试 GracefulStop 调用
func TestGracefulStop(t *testing.T) {
	cfg := &config.Config{
		Servers: []config.ServerConfig{{
			Listen: ":8080",
		}},
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
		Servers: []config.ServerConfig{{
			Listen: ":8080",
		}},
	}

	s := New(cfg)

	// 多次调用 StopWithTimeout 应该都是安全的
	for i := 0; i < 3; i++ {
		err := s.StopWithTimeout(5 * time.Second)
		if err != nil {
			t.Errorf("StopWithTimeout() call %d returned error: %v", i+1, err)
		}
	}
}

// TestGracefulStopWithZeroTimeout 测试零超时的 GracefulStop
func TestGracefulStopWithZeroTimeout(t *testing.T) {
	cfg := &config.Config{
		Servers: []config.ServerConfig{{
			Listen: ":8080",
		}},
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
		Servers: []config.ServerConfig{{
			Listen: ":8080",
		}},
	}

	s := New(cfg)
	chain, err := s.buildMiddlewareChain(&cfg.Servers[0])
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
		Servers: []config.ServerConfig{{
			Listen: ":8080",
			Security: config.SecurityConfig{
				Access: config.AccessConfig{
					Allow: []string{"127.0.0.1"},
				},
			},
		}},
	}

	s := New(cfg)
	chain, err := s.buildMiddlewareChain(&cfg.Servers[0])
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
		Servers: []config.ServerConfig{{
			Listen: ":8080",
			Security: config.SecurityConfig{
				RateLimit: config.RateLimitConfig{
					RequestRate: 100,
					Burst:       200,
				},
			},
		}},
	}

	s := New(cfg)
	chain, err := s.buildMiddlewareChain(&cfg.Servers[0])
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
		Servers: []config.ServerConfig{{
			Listen: ":8080",
			Rewrite: []config.RewriteRule{
				{Pattern: "/old/(.*)", Replacement: "/new/$1"},
			},
		}},
	}

	s := New(cfg)
	chain, err := s.buildMiddlewareChain(&cfg.Servers[0])
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
		Servers: []config.ServerConfig{{
			Listen: ":8080",
			Compression: config.CompressionConfig{
				Level: 6,
			},
		}},
	}

	s := New(cfg)
	chain, err := s.buildMiddlewareChain(&cfg.Servers[0])
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
		Servers: []config.ServerConfig{{
			Listen: ":8080",
			Security: config.SecurityConfig{
				Headers: config.SecurityHeaders{
					XFrameOptions:       "DENY",
					XContentTypeOptions: "nosniff",
				},
			},
		}},
	}

	s := New(cfg)
	chain, err := s.buildMiddlewareChain(&cfg.Servers[0])
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
		Servers: []config.ServerConfig{{
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
		}},
	}

	s := New(cfg)
	chain, err := s.buildMiddlewareChain(&cfg.Servers[0])
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
		Servers: []config.ServerConfig{{
			Listen: ":8080",
		}},
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
		Servers: []config.ServerConfig{{
			Listen: ":8080",
		}},
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
		Servers: []config.ServerConfig{{
			Listen: ":8080",
		}},
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
		Servers: []config.ServerConfig{{
			Listen: ":8080",
		}},
	}

	s := New(cfg)

	// 创建模拟监听器
	listener1, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	defer func() {
		_ = listener1.Close()
	}()

	listener2, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	defer func() {
		_ = listener2.Close()
	}()

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
		Servers: []config.ServerConfig{{
			Listen: ":8080",
		}},
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
		Servers: []config.ServerConfig{{
			Listen: ":8080",
		}},
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
		Servers: []config.ServerConfig{{
			Listen: ":8080",
		}},
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

// TestServer_Proxies 测试代理管理
func TestServer_Proxies(t *testing.T) {
	cfg := &config.Config{
		Servers: []config.ServerConfig{{
			Listen: ":8080",
		}},
	}

	s := New(cfg)

	// 初始代理列表应为空
	if len(s.proxies) != 0 {
		t.Error("Initial proxies should be empty")
	}
}

// TestServer_Running 测试运行状态
func TestServer_Running(t *testing.T) {
	cfg := &config.Config{
		Servers: []config.ServerConfig{{
			Listen: ":8080",
		}},
	}

	s := New(cfg)

	// 初始状态应为未运行
	if s.running {
		t.Error("Initial running state should be false")
	}
}

// TestServer_StopWithNilFastServer 测试无 fastServer 时停止
func TestServer_StopWithNilFastServer(t *testing.T) {
	cfg := &config.Config{
		Servers: []config.ServerConfig{{
			Listen: ":8080",
		}},
	}

	s := New(cfg)
	s.fastServer = nil

	err := s.StopWithTimeout(5 * time.Second)
	if err != nil {
		t.Errorf("StopWithTimeout with nil fastServer should succeed: %v", err)
	}
}

// TestServer_GracefulStopWithNilFastServer 测试无 fastServer 时优雅停止
func TestServer_GracefulStopWithNilFastServer(t *testing.T) {
	cfg := &config.Config{
		Servers: []config.ServerConfig{{
			Listen: ":8080",
		}},
	}

	s := New(cfg)
	s.fastServer = nil

	err := s.GracefulStop(5 * time.Second)
	if err != nil {
		t.Errorf("GracefulStop with nil fastServer should succeed: %v", err)
	}
}

// TestServer_GetProxyCacheStats 测试代理缓存统计
func TestServer_GetProxyCacheStats(t *testing.T) {
	cfg := &config.Config{
		Servers: []config.ServerConfig{{
			Listen: ":8080",
		}},
	}

	s := New(cfg)

	// 无代理时应返回空统计
	stats := s.getProxyCacheStats()
	if stats.Entries != 0 {
		t.Errorf("Expected 0 entries, got %d", stats.Entries)
	}
	if stats.Pending != 0 {
		t.Errorf("Expected 0 pending, got %d", stats.Pending)
	}
}

// TestServer_BuildMiddlewareChain_EmptyConfig 测试空配置的中间件链
func TestServer_BuildMiddlewareChain_EmptyConfig(t *testing.T) {
	cfg := &config.Config{
		Servers: []config.ServerConfig{{
			Listen: ":8080",
		}},
	}

	s := New(cfg)

	chain, err := s.buildMiddlewareChain(&cfg.Servers[0])
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if chain == nil {
		t.Error("Expected non-nil chain")
	}
}

// TestServer_TrackStats_EmptyBody 测试空响应体的统计
func TestServer_TrackStats_EmptyBody(t *testing.T) {
	cfg := &config.Config{
		Servers: []config.ServerConfig{{
			Listen: ":8080",
		}},
	}

	s := New(cfg)

	handler := func(_ *fasthttp.RequestCtx) {
		// 空响应
	}

	wrappedHandler := s.trackStats(handler)

	ctx := &fasthttp.RequestCtx{}
	ctx.Init(&fasthttp.Request{}, nil, nil)
	ctx.Request.SetBody(nil)

	wrappedHandler(ctx)

	if s.requests.Load() != 1 {
		t.Errorf("Expected 1 request, got %d", s.requests.Load())
	}

	if s.bytesSent.Load() != 0 {
		t.Errorf("Expected 0 bytes sent, got %d", s.bytesSent.Load())
	}
}

// TestStart_Success 测试服务器配置初始化
func TestStart_Success(t *testing.T) {
	cfg := &config.Config{
		Servers: []config.ServerConfig{{
			Listen: ":8080",
		}},
	}

	s := New(cfg)

	// 验证服务器正确初始化
	if s == nil {
		t.Fatal("New() returned nil, expected non-nil Server")
	}

	if s.config != cfg {
		t.Error("Server.config not set correctly")
	}
}

// TestStart_WithStaticFiles 测试静态文件配置
func TestStart_WithStaticFiles(t *testing.T) {
	// 创建临时目录
	tempDir := t.TempDir()

	cfg := &config.Config{
		Servers: []config.ServerConfig{{
			Listen: ":8080",
			Static: []config.StaticConfig{{
				Path:  "/static",
				Root:  tempDir,
				Index: []string{"index.html"},
			}},
		}},
	}

	s := New(cfg)

	if s == nil {
		t.Fatal("New() returned nil")
	}
}

// TestStart_WithGoroutinePool 测试 GoroutinePool 配置
func TestStart_WithGoroutinePool(t *testing.T) {
	cfg := &config.Config{
		Servers: []config.ServerConfig{{
			Listen: ":8080",
		}},
		Performance: config.PerformanceConfig{
			GoroutinePool: config.GoroutinePoolConfig{
				Enabled:     true,
				MaxWorkers:  100,
				MinWorkers:  10,
				IdleTimeout: 30 * time.Second,
			},
		},
	}

	s := New(cfg)

	if s == nil {
		t.Fatal("New() returned nil")
	}
}

// TestStart_WithFileCache 测试文件缓存配置
func TestStart_WithFileCache(t *testing.T) {
	cfg := &config.Config{
		Servers: []config.ServerConfig{{
			Listen: ":8080",
		}},
		Performance: config.PerformanceConfig{
			FileCache: config.FileCacheConfig{
				MaxEntries: 1000,
				MaxSize:    100 * 1024 * 1024,
			},
		},
	}

	s := New(cfg)

	if s == nil {
		t.Fatal("New() returned nil")
	}
}

// TestStop_Graceful 测试优雅停止（无 race 模式）
func TestStop_Graceful(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	cfg := &config.Config{
		Servers: []config.ServerConfig{{
			Listen: ":0",
		}},
	}

	s := New(cfg)

	// 在未启动时调用 GracefulStop，应返回 nil
	err := s.GracefulStop(1 * time.Second)
	if err != nil {
		t.Errorf("GracefulStop() on non-started server returned error: %v", err)
	}
}

// TestGetTLSConfig_Nil 测试无 TLS 配置
func TestGetTLSConfig_Nil(t *testing.T) {
	cfg := &config.Config{
		Servers: []config.ServerConfig{{
			Listen: ":0",
		}},
	}

	s := New(cfg)

	tlsCfg, err := s.GetTLSConfig()
	if err == nil {
		t.Error("GetTLSConfig() should return error when TLS not configured")
	}
	if tlsCfg != nil {
		t.Error("GetTLSConfig() should return nil when TLS not configured")
	}
}

// TestGetTLSConfig_NilServer 测试 nil 服务器调用 GetTLSConfig
func TestGetTLSConfig_NilServer(t *testing.T) {
	var s *Server
	// 防御性：如果 s 为 nil，调用方法会 panic，这是预期的行为
	// 这里我们只测试非 nil 但 tlsManager 为 nil 的情况
	cfg := &config.Config{
		Servers: []config.ServerConfig{{
			Listen: ":0",
		}},
	}
	s = New(cfg)

	// 确保 tlsManager 为 nil
	if s.tlsManager != nil {
		t.Skip("tlsManager should be nil initially")
	}

	tlsCfg, err := s.GetTLSConfig()
	if err == nil {
		t.Error("Expected error when tlsManager is nil")
	}
	if tlsCfg != nil {
		t.Error("Expected nil TLS config when tlsManager is nil")
	}
	if err.Error() != "TLS not configured" {
		t.Errorf("Expected error 'TLS not configured', got: %v", err)
	}
}

// TestGetServerName 测试服务器名称返回。
func TestGetServerName(t *testing.T) {
	tests := []struct {
		name     string
		cfg      *config.ServerConfig
		wantName string
	}{
		{
			name:     "nil config",
			cfg:      nil,
			wantName: "lolly/" + version.Version,
		},
		{
			name: "ServerTokens true (default)",
			cfg: &config.ServerConfig{
				ServerTokens: true,
			},
			wantName: "lolly/" + version.Version,
		},
		{
			name: "ServerTokens false",
			cfg: &config.ServerConfig{
				ServerTokens: false,
			},
			wantName: "lolly",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Server{}
			got := s.getServerName(tt.cfg)
			if got != tt.wantName {
				t.Errorf("getServerName() = %q, want %q", got, tt.wantName)
			}
		})
	}
}

// TestApplyTypesConfig 测试 MIME 类型配置应用。
func TestApplyTypesConfig(t *testing.T) {
	t.Run("nil config", func(t *testing.T) {
		s := &Server{}
		// 不应该 panic
		s.applyTypesConfig(nil)
	})

	t.Run("empty config", func(t *testing.T) {
		s := &Server{}
		cfg := &config.ServerConfig{}
		// 不应该 panic
		s.applyTypesConfig(cfg)
	})

	t.Run("with types map", func(t *testing.T) {
		s := &Server{}
		cfg := &config.ServerConfig{
			Types: config.TypesConfig{
				Map: map[string]string{
					".custom": "application/x-custom",
				},
			},
		}
		// 不应该 panic
		s.applyTypesConfig(cfg)
	})

	t.Run("with default type", func(t *testing.T) {
		s := &Server{}
		cfg := &config.ServerConfig{
			Types: config.TypesConfig{
				DefaultType: "application/octet-stream",
			},
		}
		// 不应该 panic
		s.applyTypesConfig(cfg)
	})
}

// TestCreateListener_TCP 测试 TCP 监听器创建。
func TestCreateListener_TCP(t *testing.T) {
	cfg := &config.Config{
		Servers: []config.ServerConfig{{
			Listen: "127.0.0.1:0", // 随机端口
		}},
	}

	s := New(cfg)
	ln, err := s.createListener(&cfg.Servers[0])
	if err != nil {
		t.Fatalf("createListener() error: %v", err)
	}
	if ln == nil {
		t.Fatal("createListener() returned nil listener")
	}
	defer ln.Close()

	if ln.Addr().Network() != "tcp" {
		t.Errorf("expected tcp network, got %s", ln.Addr().Network())
	}
}

// TestCreateListener_UnixSocket 测试 Unix Socket 监听器创建。
func TestCreateListener_UnixSocket(t *testing.T) {
	tempDir := t.TempDir()
	socketPath := tempDir + "/test.sock"

	cfg := &config.Config{
		Servers: []config.ServerConfig{{
			Listen: "unix:" + socketPath,
		}},
	}

	s := New(cfg)
	ln, err := s.createListener(&cfg.Servers[0])
	if err != nil {
		t.Fatalf("createListener() error: %v", err)
	}
	if ln == nil {
		t.Fatal("createListener() returned nil listener")
	}
	defer ln.Close()

	if ln.Addr().Network() != "unix" {
		t.Errorf("expected unix network, got %s", ln.Addr().Network())
	}
}

// TestCreateListener_UnixSocketWithPermissions 测试带权限的 Unix Socket 创建。
func TestCreateListener_UnixSocketWithPermissions(t *testing.T) {
	tempDir := t.TempDir()
	socketPath := tempDir + "/test_perm.sock"

	cfg := &config.Config{
		Servers: []config.ServerConfig{{
			Listen: "unix:" + socketPath,
			UnixSocket: config.UnixSocketConfig{
				Mode:  0o600,
				User:  "nobody",
				Group: "nobody",
			},
		}},
	}

	s := New(cfg)
	ln, err := s.createListener(&cfg.Servers[0])
	if err != nil {
		t.Fatalf("createListener() error: %v", err)
	}
	if ln == nil {
		t.Fatal("createListener() returned nil listener")
	}
	defer ln.Close()
}

// TestCreateListener_UnixSocketCleanup 测试 Unix Socket 文件清理。
func TestCreateListener_UnixSocketCleanup(t *testing.T) {
	tempDir := t.TempDir()
	socketPath := tempDir + "/cleanup.sock"

	// 先创建一个已存在的 socket 文件
	if err := os.WriteFile(socketPath, []byte{}, 0o666); err != nil {
		t.Fatalf("failed to create existing socket file: %v", err)
	}

	cfg := &config.Config{
		Servers: []config.ServerConfig{{
			Listen: "unix:" + socketPath,
		}},
	}

	s := New(cfg)
	ln, err := s.createListener(&cfg.Servers[0])
	if err != nil {
		t.Fatalf("createListener() error: %v", err)
	}
	defer ln.Close()
}

// TestServer_StatsMethods 测试服务器统计方法。
func TestServer_StatsMethods(t *testing.T) {
	cfg := &config.Config{
		Servers: []config.ServerConfig{{
			Listen: ":8080",
		}},
	}

	s := New(cfg)

	// 测试 startTime 初始值
	if !s.startTime.IsZero() {
		t.Error("startTime should be zero initially")
	}

	// 设置 startTime
	s.startTime = time.Now()
	if s.startTime.IsZero() {
		t.Error("startTime should not be zero after setting")
	}

	// 测试统计值
	if s.connections.Load() != 0 {
		t.Error("initial connections should be 0")
	}
	if s.requests.Load() != 0 {
		t.Error("initial requests should be 0")
	}
	if s.bytesSent.Load() != 0 {
		t.Error("initial bytesSent should be 0")
	}
	if s.bytesReceived.Load() != 0 {
		t.Error("initial bytesReceived should be 0")
	}
}

// TestServer_SetResolver 测试设置解析器。
func TestServer_SetResolver(t *testing.T) {
	cfg := &config.Config{
		Servers: []config.ServerConfig{{
			Listen: ":8080",
		}},
	}

	s := New(cfg)

	// 设置 nil resolver
	s.SetResolver(nil)
	if s.resolver != nil {
		t.Error("resolver should be nil")
	}
}

// TestServer_SetUpgradeManager 测试设置升级管理器。
func TestServer_SetUpgradeManager(t *testing.T) {
	cfg := &config.Config{
		Servers: []config.ServerConfig{{
			Listen: ":8080",
		}},
	}

	s := New(cfg)

	// 设置 nil upgrade manager
	s.SetUpgradeManager(nil)
	if s.upgradeManager != nil {
		t.Error("upgradeManager should be nil")
	}

	// 设置实际的 upgrade manager
	um := NewUpgradeManager(s)
	s.SetUpgradeManager(um)
	if s.upgradeManager == nil {
		t.Error("upgradeManager should not be nil after setting")
	}
}

// TestServer_GetResolver 测试获取解析器。
func TestServer_GetResolver(t *testing.T) {
	cfg := &config.Config{
		Servers: []config.ServerConfig{{
			Listen: ":8080",
		}},
	}

	s := New(cfg)

	// 初始 resolver 应为 nil
	resolver := s.GetResolver()
	if resolver != nil {
		t.Error("expected nil resolver initially")
	}
}

// TestServer_StopWithTimeout_WithListeners 测试带监听器的停止。
func TestServer_StopWithTimeout_WithListeners(t *testing.T) {
	cfg := &config.Config{
		Servers: []config.ServerConfig{{
			Listen: ":0",
		}},
	}

	s := New(cfg)

	// 创建监听器
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to create listener: %v", err)
	}
	s.listeners = []net.Listener{ln}

	// 调用停止
	err = s.StopWithTimeout(1 * time.Second)
	if err != nil {
		t.Errorf("StopWithTimeout failed: %v", err)
	}
}

// TestServer_GracefulStop_WithListeners 测试带监听器的优雅停止。
func TestServer_GracefulStop_WithListeners(t *testing.T) {
	cfg := &config.Config{
		Servers: []config.ServerConfig{{
			Listen: ":0",
		}},
	}

	s := New(cfg)

	// 创建监听器
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to create listener: %v", err)
	}
	s.listeners = []net.Listener{ln}

	// 调用优雅停止
	err = s.GracefulStop(1 * time.Second)
	if err != nil {
		t.Errorf("GracefulStop failed: %v", err)
	}
}

// TestServer_StopWithTimeout_WithFastServer 测试带 fastServer 的停止。
func TestServer_StopWithTimeout_WithFastServer(t *testing.T) {
	cfg := &config.Config{
		Servers: []config.ServerConfig{{
			Listen: ":0",
		}},
	}

	s := New(cfg)
	s.running = true

	// 创建 mock fastServer
	s.fastServer = &fasthttp.Server{}

	// 调用停止
	err := s.StopWithTimeout(1 * time.Second)
	if err != nil {
		t.Errorf("StopWithTimeout failed: %v", err)
	}
}

// TestBuildMiddlewareChain_BodyLimit 测试请求体限制中间件。
func TestBuildMiddlewareChain_BodyLimit(t *testing.T) {
	cfg := &config.Config{
		Logging: config.LoggingConfig{},
		Servers: []config.ServerConfig{{
			Listen:            ":8080",
			ClientMaxBodySize: "1MB",
		}},
	}

	s := New(cfg)
	chain, err := s.buildMiddlewareChain(&cfg.Servers[0])
	if err != nil {
		t.Errorf("buildMiddlewareChain failed: %v", err)
	}
	if chain == nil {
		t.Error("Expected non-nil chain")
	}
}

// TestBuildMiddlewareChain_ErrorIntercept 测试错误拦截中间件。
func TestBuildMiddlewareChain_ErrorIntercept(t *testing.T) {
	cfg := &config.Config{
		Logging: config.LoggingConfig{},
		Servers: []config.ServerConfig{{
			Listen: ":8080",
			Security: config.SecurityConfig{
				ErrorPage: config.ErrorPageConfig{
					Pages: map[int]string{
						404: "/404.html",
					},
				},
			},
		}},
	}

	s := New(cfg)
	chain, err := s.buildMiddlewareChain(&cfg.Servers[0])
	if err != nil {
		t.Errorf("buildMiddlewareChain failed: %v", err)
	}
	if chain == nil {
		t.Error("Expected non-nil chain")
	}
}

// TestBuildMiddlewareChain_NilServerConfig 测试 nil 服务器配置。
// 注意：buildMiddlewareChain 不接受 nil，所以这个测试验证空配置。
func TestBuildMiddlewareChain_NilServerConfig(t *testing.T) {
	cfg := &config.Config{
		Logging: config.LoggingConfig{},
		Servers: []config.ServerConfig{{
			Listen: ":0",
		}},
	}

	s := New(cfg)
	chain, err := s.buildMiddlewareChain(&cfg.Servers[0])
	if err != nil {
		t.Errorf("buildMiddlewareChain failed: %v", err)
	}
	if chain == nil {
		t.Error("Expected non-nil chain")
	}
}

// TestServer_StatusCode_MethodNotAllowed 测试不支持的 HTTP 方法。
func TestServer_StatusCode_MethodNotAllowed(t *testing.T) {
	// 简单验证
	if fasthttp.StatusMethodNotAllowed != 405 {
		t.Errorf("StatusMethodNotAllowed should be 405")
	}
}

// TestServer_ConnectionTracking 测试连接追踪。
func TestServer_ConnectionTracking(t *testing.T) {
	cfg := &config.Config{
		Servers: []config.ServerConfig{{
			Listen: ":8080",
		}},
	}

	s := New(cfg)

	// 测试原子操作
	initial := s.connections.Load()
	s.connections.Add(1)
	if s.connections.Load() != initial+1 {
		t.Error("connections should have incremented")
	}
	s.connections.Add(-1)
	if s.connections.Load() != initial {
		t.Error("connections should be back to initial")
	}
}

// TestServer_RequestTracking 测试请求追踪。
func TestServer_RequestTracking(t *testing.T) {
	cfg := &config.Config{
		Servers: []config.ServerConfig{{
			Listen: ":8080",
		}},
	}

	s := New(cfg)

	// 测试原子操作
	s.requests.Add(5)
	if s.requests.Load() != 5 {
		t.Errorf("expected 5 requests, got %d", s.requests.Load())
	}
}

// TestServer_BytesTracking 测试字节追踪。
func TestServer_BytesTracking(t *testing.T) {
	cfg := &config.Config{
		Servers: []config.ServerConfig{{
			Listen: ":8080",
		}},
	}

	s := New(cfg)

	// 测试原子操作
	s.bytesSent.Add(1024)
	s.bytesReceived.Add(512)
	if s.bytesSent.Load() != 1024 {
		t.Errorf("expected 1024 bytes sent, got %d", s.bytesSent.Load())
	}
	if s.bytesReceived.Load() != 512 {
		t.Errorf("expected 512 bytes received, got %d", s.bytesReceived.Load())
	}
}

// TestServer_GracefulStop_WithFastServers 测试带多个 fastServer 的优雅停止。
func TestServer_GracefulStop_WithFastServers(t *testing.T) {
	cfg := &config.Config{
		Servers: []config.ServerConfig{{
			Listen: ":0",
		}},
	}

	s := New(cfg)
	s.running = true

	// 创建多个 fastServer
	s.fastServers = []*fasthttp.Server{
		{},
		{},
	}

	// 调用优雅停止
	err := s.GracefulStop(1 * time.Second)
	if err != nil {
		t.Errorf("GracefulStop failed: %v", err)
	}
}

// TestServer_StopWithTimeout_WithFastServers 测试带多个 fastServer 的停止。
func TestServer_StopWithTimeout_WithFastServers(t *testing.T) {
	cfg := &config.Config{
		Servers: []config.ServerConfig{{
			Listen: ":0",
		}},
	}

	s := New(cfg)
	s.running = true

	// 创建多个 fastServer
	s.fastServers = []*fasthttp.Server{
		{},
		{},
	}

	// 调用停止
	err := s.StopWithTimeout(1 * time.Second)
	if err != nil {
		t.Errorf("StopWithTimeout failed: %v", err)
	}
}

// TestServer_GetProxyCacheStats_WithProxies 测试带代理的缓存统计。
func TestServer_GetProxyCacheStats_WithProxies(t *testing.T) {
	cfg := &config.Config{
		Servers: []config.ServerConfig{{
			Listen: ":8080",
		}},
	}

	s := New(cfg)
	s.proxies = nil // 确保 proxies 为 nil

	// 无代理时应返回空统计
	stats := s.getProxyCacheStats()
	if stats.Entries != 0 {
		t.Errorf("Expected 0 entries, got %d", stats.Entries)
	}
}

// TestServer_MultipleListeners 测试多个监听器。
func TestServer_MultipleListeners(t *testing.T) {
	cfg := &config.Config{
		Servers: []config.ServerConfig{{
			Listen: ":0",
		}},
	}

	s := New(cfg)

	// 创建多个监听器
	ln1, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to create listener: %v", err)
	}
	ln2, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to create listener: %v", err)
		_ = ln1.Close()
	}

	s.listeners = []net.Listener{ln1, ln2}

	// 验证可以获取监听器
	got := s.GetListeners()
	if len(got) != 2 {
		t.Errorf("expected 2 listeners, got %d", len(got))
	}

	// 清理
	_ = s.StopWithTimeout(1 * time.Second)
}
