package server

import (
	"testing"
	"time"

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