// Package server 提供 startMultiServerMode 集成测试。
//
// 该文件测试 startMultiServerMode 函数的各种配置场景，
// 包括多服务器配置、监听器创建、服务器启动等场景。
//
// 作者：xfy
package server

import (
	"os"
	"strings"
	"testing"
	"time"

	"rua.plus/lolly/internal/config"
)

// TestStartMultiServerMode_BasicConfig 测试基本的多服务器配置。
func TestStartMultiServerMode_BasicConfig(t *testing.T) {
	cfg := &config.Config{
		Servers: []config.ServerConfig{
			{Listen: "127.0.0.1:0"},
			{Listen: "127.0.0.1:0"},
		},
	}

	s := New(cfg)

	// 验证多服务器配置
	if len(s.config.Servers) != 2 {
		t.Errorf("expected 2 servers, got %d", len(s.config.Servers))
	}
}

// TestStartMultiServerMode_ThreeServers 测试三个服务器配置。
func TestStartMultiServerMode_ThreeServers(t *testing.T) {
	cfg := &config.Config{
		Servers: []config.ServerConfig{
			{Listen: "127.0.0.1:0"},
			{Listen: "127.0.0.1:0"},
			{Listen: "127.0.0.1:0"},
		},
	}

	s := New(cfg)

	if len(s.config.Servers) != 3 {
		t.Errorf("expected 3 servers, got %d", len(s.config.Servers))
	}
}

// TestStartMultiServerMode_WithProxy 测试带代理的多服务器配置。
func TestStartMultiServerMode_WithProxy(t *testing.T) {
	cfg := &config.Config{
		Servers: []config.ServerConfig{
			{
				Listen: "127.0.0.1:0",
				Proxy: []config.ProxyConfig{
					{
						Path: "/api",
						Targets: []config.ProxyTarget{
							{URL: "http://127.0.0.1:8081", Weight: 1},
						},
					},
				},
			},
			{
				Listen: "127.0.0.1:0",
				Proxy: []config.ProxyConfig{
					{
						Path: "/api",
						Targets: []config.ProxyTarget{
							{URL: "http://127.0.0.1:8082", Weight: 1},
						},
					},
				},
			},
		},
	}

	s := New(cfg)

	if len(s.config.Servers[0].Proxy) != 1 {
		t.Errorf("expected 1 proxy config for server 0, got %d", len(s.config.Servers[0].Proxy))
	}
	if len(s.config.Servers[1].Proxy) != 1 {
		t.Errorf("expected 1 proxy config for server 1, got %d", len(s.config.Servers[1].Proxy))
	}
}

// TestStartMultiServerMode_WithStaticFiles 测试带静态文件的多服务器配置。
func TestStartMultiServerMode_WithStaticFiles(t *testing.T) {
	tempDir := t.TempDir()

	cfg := &config.Config{
		Servers: []config.ServerConfig{
			{
				Listen: "127.0.0.1:0",
				Static: []config.StaticConfig{
					{
						Path:  "/static",
						Root:  tempDir,
						Index: []string{"index.html"},
					},
				},
			},
			{
				Listen: "127.0.0.1:0",
				Static: []config.StaticConfig{
					{
						Path:  "/assets",
						Root:  tempDir,
						Index: []string{"index.html"},
					},
				},
			},
		},
	}

	s := New(cfg)

	if len(s.config.Servers[0].Static) != 1 {
		t.Errorf("expected 1 static config for server 0, got %d", len(s.config.Servers[0].Static))
	}
	if len(s.config.Servers[1].Static) != 1 {
		t.Errorf("expected 1 static config for server 1, got %d", len(s.config.Servers[1].Static))
	}
}

// TestStartMultiServerMode_WithCacheAPI 测试带缓存 API 的多服务器配置。
func TestStartMultiServerMode_WithCacheAPI(t *testing.T) {
	cfg := &config.Config{
		Servers: []config.ServerConfig{
			{
				Listen: "127.0.0.1:0",
				CacheAPI: &config.CacheAPIConfig{
					Enabled: true,
					Path:    "/_cache/purge",
					Allow:   []string{"127.0.0.1"},
				},
			},
			{
				Listen: "127.0.0.1:0",
			},
		},
	}

	s := New(cfg)

	if s.config.Servers[0].CacheAPI == nil || !s.config.Servers[0].CacheAPI.Enabled {
		t.Error("expected cache API enabled on server 0")
	}
}

// TestStartMultiServerMode_WithMiddleware 测试带中间件的多服务器配置。
func TestStartMultiServerMode_WithMiddleware(t *testing.T) {
	cfg := &config.Config{
		Servers: []config.ServerConfig{
			{
				Listen: "127.0.0.1:0",
				Security: config.SecurityConfig{
					Access: config.AccessConfig{
						Allow: []string{"127.0.0.1"},
					},
					RateLimit: config.RateLimitConfig{
						RequestRate: 100,
						Burst:       200,
					},
				},
			},
			{
				Listen: "127.0.0.1:0",
				Security: config.SecurityConfig{
					Headers: config.SecurityHeaders{
						XFrameOptions: "DENY",
					},
				},
			},
		},
	}

	s := New(cfg)

	if len(s.config.Servers[0].Security.Access.Allow) != 1 {
		t.Errorf("expected 1 allow rule for server 0, got %d", len(s.config.Servers[0].Security.Access.Allow))
	}
	if s.config.Servers[1].Security.Headers.XFrameOptions != "DENY" {
		t.Errorf("expected XFrameOptions DENY for server 1, got %s", s.config.Servers[1].Security.Headers.XFrameOptions)
	}
}

// TestStartMultiServerMode_WithCompression 测试带压缩配置的多服务器配置。
func TestStartMultiServerMode_WithCompression(t *testing.T) {
	cfg := &config.Config{
		Servers: []config.ServerConfig{
			{
				Listen: "127.0.0.1:0",
				Compression: config.CompressionConfig{
					Type:  "gzip",
					Level: 6,
				},
			},
			{
				Listen: "127.0.0.1:0",
				Compression: config.CompressionConfig{
					Type:  "gzip",
					Level: 9,
				},
			},
		},
	}

	s := New(cfg)

	if s.config.Servers[0].Compression.Level != 6 {
		t.Errorf("expected compression level 6 for server 0, got %d", s.config.Servers[0].Compression.Level)
	}
	if s.config.Servers[1].Compression.Level != 9 {
		t.Errorf("expected compression level 9 for server 1, got %d", s.config.Servers[1].Compression.Level)
	}
}

// TestStartMultiServerMode_ServerOptions 测试服务器选项配置。
func TestStartMultiServerMode_ServerOptions(t *testing.T) {
	cfg := &config.Config{
		Servers: []config.ServerConfig{
			{
				Listen:             "127.0.0.1:0",
				ReadTimeout:        30 * time.Second,
				WriteTimeout:       30 * time.Second,
				IdleTimeout:        60 * time.Second,
				MaxConnsPerIP:      100,
				MaxRequestsPerConn: 1000,
			},
			{
				Listen:             "127.0.0.1:0",
				ReadTimeout:        15 * time.Second,
				WriteTimeout:       15 * time.Second,
				MaxConnsPerIP:      50,
				MaxRequestsPerConn: 500,
			},
		},
	}

	s := New(cfg)

	if s.config.Servers[0].ReadTimeout != 30*time.Second {
		t.Errorf("expected ReadTimeout 30s for server 0, got %v", s.config.Servers[0].ReadTimeout)
	}
	if s.config.Servers[1].MaxConnsPerIP != 50 {
		t.Errorf("expected MaxConnsPerIP 50 for server 1, got %d", s.config.Servers[1].MaxConnsPerIP)
	}
}

// TestStartMultiServerMode_Integration_Basic 测试多服务器模式基本启动。
func TestStartMultiServerMode_Integration_Basic(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	cfg := &config.Config{
		Mode: config.ServerModeMultiServer,
		Servers: []config.ServerConfig{
			{Listen: "127.0.0.1:0"},
			{Listen: "127.0.0.1:0"},
		},
	}

	s := New(cfg)

	errCh := make(chan error, 1)
	go func() {
		if err := s.Start(); err != nil {
			errCh <- err
		}
	}()

	time.Sleep(50 * time.Millisecond)
	_ = s.GracefulStop(2 * time.Second)

	select {
	case err := <-errCh:
		if err != nil && !isExpectedMultiServerError(err) {
			t.Errorf("unexpected server error: %v", err)
		}
	default:
	}
}

// TestStartMultiServerMode_Integration_WithProxy 测试多服务器模式带代理启动。
func TestStartMultiServerMode_Integration_WithProxy(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	cfg := &config.Config{
		Mode: config.ServerModeMultiServer,
		Servers: []config.ServerConfig{
			{
				Listen: "127.0.0.1:0",
				Proxy: []config.ProxyConfig{
					{
						Path: "/api",
						Targets: []config.ProxyTarget{
							{URL: "http://127.0.0.1:9999", Weight: 1},
						},
					},
				},
			},
			{
				Listen: "127.0.0.1:0",
				Proxy: []config.ProxyConfig{
					{
						Path: "/api",
						Targets: []config.ProxyTarget{
							{URL: "http://127.0.0.1:9998", Weight: 1},
						},
					},
				},
			},
		},
	}

	s := New(cfg)

	errCh := make(chan error, 1)
	go func() {
		if err := s.Start(); err != nil {
			errCh <- err
		}
	}()

	time.Sleep(50 * time.Millisecond)
	_ = s.GracefulStop(2 * time.Second)

	select {
	case err := <-errCh:
		if err != nil && !isExpectedMultiServerError(err) {
			t.Errorf("unexpected server error: %v", err)
		}
	default:
	}
}

// TestStartMultiServerMode_Integration_WithStaticFiles 测试多服务器模式带静态文件启动。
func TestStartMultiServerMode_Integration_WithStaticFiles(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	tempDir := t.TempDir()

	cfg := &config.Config{
		Mode: config.ServerModeMultiServer,
		Servers: []config.ServerConfig{
			{
				Listen: "127.0.0.1:0",
				Static: []config.StaticConfig{
					{
						Path:  "/static",
						Root:  tempDir,
						Index: []string{"index.html"},
					},
				},
			},
			{
				Listen: "127.0.0.1:0",
				Static: []config.StaticConfig{
					{
						Path:  "/assets",
						Root:  tempDir,
						Index: []string{"index.html"},
					},
				},
			},
		},
	}

	s := New(cfg)

	errCh := make(chan error, 1)
	go func() {
		if err := s.Start(); err != nil {
			errCh <- err
		}
	}()

	time.Sleep(50 * time.Millisecond)
	_ = s.GracefulStop(2 * time.Second)

	select {
	case err := <-errCh:
		if err != nil && !isExpectedMultiServerError(err) {
			t.Errorf("unexpected server error: %v", err)
		}
	default:
	}
}

// TestStartMultiServerMode_Integration_WithCacheAPI 测试多服务器模式带缓存 API 启动。
func TestStartMultiServerMode_Integration_WithCacheAPI(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	cfg := &config.Config{
		Mode: config.ServerModeMultiServer,
		Servers: []config.ServerConfig{
			{
				Listen: "127.0.0.1:0",
				CacheAPI: &config.CacheAPIConfig{
					Enabled: true,
					Path:    "/_cache/purge",
				},
			},
			{
				Listen: "127.0.0.1:0",
			},
		},
	}

	s := New(cfg)

	errCh := make(chan error, 1)
	go func() {
		if err := s.Start(); err != nil {
			errCh <- err
		}
	}()

	time.Sleep(50 * time.Millisecond)
	_ = s.GracefulStop(2 * time.Second)

	select {
	case err := <-errCh:
		if err != nil && !isExpectedMultiServerError(err) {
			t.Errorf("unexpected server error: %v", err)
		}
	default:
	}
}

// TestStartMultiServerMode_Integration_WithHealthCheck 测试多服务器模式带健康检查启动。
func TestStartMultiServerMode_Integration_WithHealthCheck(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	cfg := &config.Config{
		Mode: config.ServerModeMultiServer,
		Servers: []config.ServerConfig{
			{
				Listen: "127.0.0.1:0",
				Proxy: []config.ProxyConfig{
					{
						Path: "/api",
						Targets: []config.ProxyTarget{
							{URL: "http://127.0.0.1:9999", Weight: 1},
						},
						HealthCheck: config.HealthCheckConfig{
							Interval: 1 * time.Second,
							Timeout:  500 * time.Millisecond,
							Path:     "/health",
						},
					},
				},
			},
			{
				Listen: "127.0.0.1:0",
			},
		},
	}

	s := New(cfg)

	errCh := make(chan error, 1)
	go func() {
		if err := s.Start(); err != nil {
			errCh <- err
		}
	}()

	time.Sleep(100 * time.Millisecond)
	_ = s.GracefulStop(2 * time.Second)

	select {
	case err := <-errCh:
		if err != nil && !isExpectedMultiServerError(err) {
			t.Errorf("unexpected server error: %v", err)
		}
	default:
	}
}

// TestStartMultiServerMode_Integration_WithMiddleware 测试多服务器模式带中间件启动。
func TestStartMultiServerMode_Integration_WithMiddleware(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	cfg := &config.Config{
		Mode: config.ServerModeMultiServer,
		Servers: []config.ServerConfig{
			{
				Listen: "127.0.0.1:0",
				Security: config.SecurityConfig{
					Access: config.AccessConfig{
						Allow: []string{"127.0.0.1"},
					},
					RateLimit: config.RateLimitConfig{
						RequestRate: 100,
						Burst:       200,
					},
				},
			},
			{
				Listen: "127.0.0.1:0",
				Security: config.SecurityConfig{
					Headers: config.SecurityHeaders{
						XFrameOptions: "DENY",
					},
				},
			},
		},
	}

	s := New(cfg)

	errCh := make(chan error, 1)
	go func() {
		if err := s.Start(); err != nil {
			errCh <- err
		}
	}()

	time.Sleep(50 * time.Millisecond)
	_ = s.GracefulStop(2 * time.Second)

	select {
	case err := <-errCh:
		if err != nil && !isExpectedMultiServerError(err) {
			t.Errorf("unexpected server error: %v", err)
		}
	default:
	}
}

// TestStartMultiServerMode_Integration_WithPerformance 测试多服务器模式带性能配置启动。
func TestStartMultiServerMode_Integration_WithPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	cfg := &config.Config{
		Mode: config.ServerModeMultiServer,
		Servers: []config.ServerConfig{
			{Listen: "127.0.0.1:0"},
			{Listen: "127.0.0.1:0"},
		},
		Performance: config.PerformanceConfig{
			GoroutinePool: config.GoroutinePoolConfig{
				Enabled:     true,
				MaxWorkers:  50,
				MinWorkers:  5,
				IdleTimeout: 10 * time.Second,
			},
			FileCache: config.FileCacheConfig{
				MaxEntries: 1000,
				MaxSize:    10 * 1024 * 1024,
			},
		},
	}

	s := New(cfg)

	errCh := make(chan error, 1)
	go func() {
		if err := s.Start(); err != nil {
			errCh <- err
		}
	}()

	time.Sleep(50 * time.Millisecond)
	_ = s.GracefulStop(2 * time.Second)

	select {
	case err := <-errCh:
		if err != nil && !isExpectedMultiServerError(err) {
			t.Errorf("unexpected server error: %v", err)
		}
	default:
	}
}

// TestStartMultiServerMode_Integration_ThreeServers 测试三服务器模式启动。
func TestStartMultiServerMode_Integration_ThreeServers(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	cfg := &config.Config{
		Mode: config.ServerModeMultiServer,
		Servers: []config.ServerConfig{
			{Listen: "127.0.0.1:0"},
			{Listen: "127.0.0.1:0"},
			{Listen: "127.0.0.1:0"},
		},
	}

	s := New(cfg)

	errCh := make(chan error, 1)
	go func() {
		if err := s.Start(); err != nil {
			errCh <- err
		}
	}()

	time.Sleep(50 * time.Millisecond)
	_ = s.GracefulStop(2 * time.Second)

	select {
	case err := <-errCh:
		if err != nil && !isExpectedMultiServerError(err) {
			t.Errorf("unexpected server error: %v", err)
		}
	default:
	}
}

// TestStartMultiServerMode_Integration_WithCompression 测试多服务器模式带压缩启动。
func TestStartMultiServerMode_Integration_WithCompression(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	cfg := &config.Config{
		Mode: config.ServerModeMultiServer,
		Servers: []config.ServerConfig{
			{
				Listen: "127.0.0.1:0",
				Compression: config.CompressionConfig{
					Type:  "gzip",
					Level: 6,
				},
			},
			{
				Listen: "127.0.0.1:0",
				Compression: config.CompressionConfig{
					Type:  "gzip",
					Level: 9,
				},
			},
		},
	}

	s := New(cfg)

	errCh := make(chan error, 1)
	go func() {
		if err := s.Start(); err != nil {
			errCh <- err
		}
	}()

	time.Sleep(50 * time.Millisecond)
	_ = s.GracefulStop(2 * time.Second)

	select {
	case err := <-errCh:
		if err != nil && !isExpectedMultiServerError(err) {
			t.Errorf("unexpected server error: %v", err)
		}
	default:
	}
}

// TestStartMultiServerMode_Integration_WithRewrite 测试多服务器模式带重写启动。
func TestStartMultiServerMode_Integration_WithRewrite(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	cfg := &config.Config{
		Mode: config.ServerModeMultiServer,
		Servers: []config.ServerConfig{
			{
				Listen: "127.0.0.1:0",
				Rewrite: []config.RewriteRule{
					{Pattern: "^/old/(.*)$", Replacement: "/new/$1"},
				},
			},
			{
				Listen: "127.0.0.1:0",
			},
		},
	}

	s := New(cfg)

	errCh := make(chan error, 1)
	go func() {
		if err := s.Start(); err != nil {
			errCh <- err
		}
	}()

	time.Sleep(50 * time.Millisecond)
	_ = s.GracefulStop(2 * time.Second)

	select {
	case err := <-errCh:
		if err != nil && !isExpectedMultiServerError(err) {
			t.Errorf("unexpected server error: %v", err)
		}
	default:
	}
}

// TestStartMultiServerMode_Integration_WithConnLimiter 测试多服务器模式带连接限制启动。
func TestStartMultiServerMode_Integration_WithConnLimiter(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	cfg := &config.Config{
		Mode: config.ServerModeMultiServer,
		Servers: []config.ServerConfig{
			{
				Listen: "127.0.0.1:0",
				Security: config.SecurityConfig{
					RateLimit: config.RateLimitConfig{
						ConnLimit: 10,
					},
				},
			},
			{
				Listen: "127.0.0.1:0",
			},
		},
	}

	s := New(cfg)

	errCh := make(chan error, 1)
	go func() {
		if err := s.Start(); err != nil {
			errCh <- err
		}
	}()

	time.Sleep(50 * time.Millisecond)
	_ = s.GracefulStop(2 * time.Second)

	select {
	case err := <-errCh:
		if err != nil && !isExpectedMultiServerError(err) {
			t.Errorf("unexpected server error: %v", err)
		}
	default:
	}
}

// TestStartMultiServerMode_Integration_MixedConfigs 测试多服务器模式混合配置启动。
func TestStartMultiServerMode_Integration_MixedConfigs(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	tempDir := t.TempDir()

	cfg := &config.Config{
		Mode: config.ServerModeMultiServer,
		Servers: []config.ServerConfig{
			{
				Listen: "127.0.0.1:0",
				Proxy: []config.ProxyConfig{
					{
						Path: "/api",
						Targets: []config.ProxyTarget{
							{URL: "http://127.0.0.1:9999", Weight: 1},
						},
					},
				},
				CacheAPI: &config.CacheAPIConfig{
					Enabled: true,
					Path:    "/_cache/purge",
				},
			},
			{
				Listen: "127.0.0.1:0",
				Static: []config.StaticConfig{
					{
						Path:  "/static",
						Root:  tempDir,
						Index: []string{"index.html"},
					},
				},
				Compression: config.CompressionConfig{
					Type:  "gzip",
					Level: 6,
				},
			},
			{
				Listen: "127.0.0.1:0",
				Security: config.SecurityConfig{
					Access: config.AccessConfig{
						Allow: []string{"127.0.0.1"},
					},
				},
			},
		},
	}

	s := New(cfg)

	errCh := make(chan error, 1)
	go func() {
		if err := s.Start(); err != nil {
			errCh <- err
		}
	}()

	time.Sleep(50 * time.Millisecond)
	_ = s.GracefulStop(2 * time.Second)

	select {
	case err := <-errCh:
		if err != nil && !isExpectedMultiServerError(err) {
			t.Errorf("unexpected server error: %v", err)
		}
	default:
	}
}

// TestStartMultiServerMode_GracefulUpgradeFallback 测试热升级模式回退。
func TestStartMultiServerMode_GracefulUpgradeFallback(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// 设置热升级环境变量
	originalValue := os.Getenv("GRACEFUL_UPGRADE")
	defer os.Setenv("GRACEFUL_UPGRADE", originalValue)

	os.Setenv("GRACEFUL_UPGRADE", "1")

	cfg := &config.Config{
		Mode: config.ServerModeMultiServer,
		Servers: []config.ServerConfig{
			{Listen: "127.0.0.1:0"},
			{Listen: "127.0.0.1:0"},
		},
	}

	s := New(cfg)

	errCh := make(chan error, 1)
	go func() {
		if err := s.Start(); err != nil {
			errCh <- err
		}
	}()

	time.Sleep(50 * time.Millisecond)
	_ = s.GracefulStop(2 * time.Second)

	select {
	case err := <-errCh:
		if err != nil && !isExpectedMultiServerError(err) {
			t.Errorf("unexpected server error: %v", err)
		}
	default:
	}
}

// TestStartMultiServerMode_WithUnixSocket 测试 Unix Socket 配置。
func TestStartMultiServerMode_WithUnixSocket(t *testing.T) {
	tempDir := t.TempDir()

	cfg := &config.Config{
		Servers: []config.ServerConfig{
			{
				Listen: "unix:" + tempDir + "/test1.sock",
			},
			{
				Listen: "unix:" + tempDir + "/test2.sock",
			},
		},
	}

	s := New(cfg)

	if !strings.HasPrefix(s.config.Servers[0].Listen, "unix:") {
		t.Errorf("expected unix socket for server 0, got %s", s.config.Servers[0].Listen)
	}
	if !strings.HasPrefix(s.config.Servers[1].Listen, "unix:") {
		t.Errorf("expected unix socket for server 1, got %s", s.config.Servers[1].Listen)
	}
}

// TestStartMultiServerMode_WithDifferentListens 测试不同监听地址配置。
func TestStartMultiServerMode_WithDifferentListens(t *testing.T) {
	cfg := &config.Config{
		Servers: []config.ServerConfig{
			{Listen: "127.0.0.1:0"},
			{Listen: "0.0.0.0:0"},
		},
	}

	s := New(cfg)

	if s.config.Servers[0].Listen != "127.0.0.1:0" {
		t.Errorf("expected listen 127.0.0.1:0 for server 0, got %s", s.config.Servers[0].Listen)
	}
	if s.config.Servers[1].Listen != "0.0.0.0:0" {
		t.Errorf("expected listen 0.0.0.0:0 for server 1, got %s", s.config.Servers[1].Listen)
	}
}

// TestStartMultiServerMode_Integration_WithErrorPage 测试多服务器模式带错误页面启动。
func TestStartMultiServerMode_Integration_WithErrorPage(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	tempDir := t.TempDir()
	errorPage := tempDir + "/404.html"
	if err := os.WriteFile(errorPage, []byte("<html>Not Found</html>"), 0o644); err != nil {
		t.Fatalf("failed to create error page: %v", err)
	}

	cfg := &config.Config{
		Mode: config.ServerModeMultiServer,
		Servers: []config.ServerConfig{
			{
				Listen: "127.0.0.1:0",
				Security: config.SecurityConfig{
					ErrorPage: config.ErrorPageConfig{
						Pages:   map[int]string{404: errorPage},
						Default: errorPage,
					},
				},
			},
			{
				Listen: "127.0.0.1:0",
			},
		},
	}

	s := New(cfg)

	errCh := make(chan error, 1)
	go func() {
		if err := s.Start(); err != nil {
			errCh <- err
		}
	}()

	time.Sleep(50 * time.Millisecond)
	_ = s.GracefulStop(2 * time.Second)

	select {
	case err := <-errCh:
		if err != nil && !isExpectedMultiServerError(err) {
			t.Errorf("unexpected server error: %v", err)
		}
	default:
	}
}

// TestStartMultiServerMode_Integration_WithMIMETypes 测试多服务器模式带 MIME 类型启动。
func TestStartMultiServerMode_Integration_WithMIMETypes(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	cfg := &config.Config{
		Mode: config.ServerModeMultiServer,
		Servers: []config.ServerConfig{
			{
				Listen: "127.0.0.1:0",
				Types: config.TypesConfig{
					Map: map[string]string{
						".wasm": "application/wasm",
					},
					DefaultType: "application/octet-stream",
				},
			},
			{
				Listen: "127.0.0.1:0",
			},
		},
	}

	s := New(cfg)

	errCh := make(chan error, 1)
	go func() {
		if err := s.Start(); err != nil {
			errCh <- err
		}
	}()

	time.Sleep(50 * time.Millisecond)
	_ = s.GracefulStop(2 * time.Second)

	select {
	case err := <-errCh:
		if err != nil && !isExpectedMultiServerError(err) {
			t.Errorf("unexpected server error: %v", err)
		}
	default:
	}
}

// TestStartMultiServerMode_WithServerNames 测试带服务器名称的多服务器配置。
func TestStartMultiServerMode_WithServerNames(t *testing.T) {
	cfg := &config.Config{
		Servers: []config.ServerConfig{
			{
				Name:        "server1",
				Listen:      "127.0.0.1:0",
				ServerNames: []string{"example.com", "www.example.com"},
			},
			{
				Name:        "server2",
				Listen:      "127.0.0.1:0",
				ServerNames: []string{"api.example.com"},
			},
		},
	}

	s := New(cfg)

	if s.config.Servers[0].Name != "server1" {
		t.Errorf("expected name server1, got %s", s.config.Servers[0].Name)
	}
	if len(s.config.Servers[0].ServerNames) != 2 {
		t.Errorf("expected 2 server names for server 0, got %d", len(s.config.Servers[0].ServerNames))
	}
	if s.config.Servers[1].Name != "server2" {
		t.Errorf("expected name server2, got %s", s.config.Servers[1].Name)
	}
}

// TestStartMultiServerMode_WithProxyLocationTypes 测试代理不同位置类型配置。
func TestStartMultiServerMode_WithProxyLocationTypes(t *testing.T) {
	cfg := &config.Config{
		Servers: []config.ServerConfig{
			{
				Listen: "127.0.0.1:0",
				Proxy: []config.ProxyConfig{
					{
						Path:         "/api/exact",
						LocationType: "exact",
						Targets:      []config.ProxyTarget{{URL: "http://127.0.0.1:9999", Weight: 1}},
					},
					{
						Path:         "/api/priority",
						LocationType: "prefix_priority",
						Targets:      []config.ProxyTarget{{URL: "http://127.0.0.1:9999", Weight: 1}},
					},
				},
			},
			{
				Listen: "127.0.0.1:0",
				Proxy: []config.ProxyConfig{
					{
						Path:         "^/api/regex/(.*)$",
						LocationType: "regex",
						Targets:      []config.ProxyTarget{{URL: "http://127.0.0.1:9999", Weight: 1}},
					},
				},
			},
		},
	}

	s := New(cfg)

	if s.config.Servers[0].Proxy[0].LocationType != "exact" {
		t.Errorf("expected exact location type, got %s", s.config.Servers[0].Proxy[0].LocationType)
	}
	if s.config.Servers[0].Proxy[1].LocationType != "prefix_priority" {
		t.Errorf("expected prefix_priority location type, got %s", s.config.Servers[0].Proxy[1].LocationType)
	}
	if s.config.Servers[1].Proxy[0].LocationType != "regex" {
		t.Errorf("expected regex location type, got %s", s.config.Servers[1].Proxy[0].LocationType)
	}
}

// TestStartMultiServerMode_Integration_WithAuthRequest 测试多服务器模式带外部认证启动。
func TestStartMultiServerMode_Integration_WithAuthRequest(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	cfg := &config.Config{
		Mode: config.ServerModeMultiServer,
		Servers: []config.ServerConfig{
			{
				Listen: "127.0.0.1:0",
				Security: config.SecurityConfig{
					AuthRequest: config.AuthRequestConfig{
						Enabled: true,
						URI:     "/auth/validate",
						Timeout: 5 * time.Second,
					},
				},
			},
			{
				Listen: "127.0.0.1:0",
			},
		},
	}

	s := New(cfg)

	errCh := make(chan error, 1)
	go func() {
		if err := s.Start(); err != nil {
			errCh <- err
		}
	}()

	time.Sleep(50 * time.Millisecond)
	_ = s.GracefulStop(2 * time.Second)

	select {
	case err := <-errCh:
		if err != nil && !isExpectedMultiServerError(err) {
			t.Errorf("unexpected server error: %v", err)
		}
	default:
	}
}

// TestStartMultiServerMode_ServerTokens 测试 ServerTokens 配置。
func TestStartMultiServerMode_ServerTokens(t *testing.T) {
	cfg := &config.Config{
		Servers: []config.ServerConfig{
			{
				Listen:       "127.0.0.1:0",
				ServerTokens: false,
			},
			{
				Listen:       "127.0.0.1:0",
				ServerTokens: true,
			},
		},
	}

	s := New(cfg)

	if s.config.Servers[0].ServerTokens {
		t.Error("expected ServerTokens false for server 0")
	}
	if !s.config.Servers[1].ServerTokens {
		t.Error("expected ServerTokens true for server 1")
	}
}

// TestStartMultiServerMode_Integration_WithDefaultServer 测试多服务器模式带默认服务器启动。
func TestStartMultiServerMode_Integration_WithDefaultServer(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	cfg := &config.Config{
		Mode: config.ServerModeMultiServer,
		Servers: []config.ServerConfig{
			{
				Name:        "default",
				Listen:      "127.0.0.1:0",
				Default:     true,
				ServerNames: []string{"_"},
			},
			{
				Name:        "api",
				Listen:      "127.0.0.1:0",
				ServerNames: []string{"api.example.com"},
			},
		},
	}

	s := New(cfg)

	errCh := make(chan error, 1)
	go func() {
		if err := s.Start(); err != nil {
			errCh <- err
		}
	}()

	time.Sleep(50 * time.Millisecond)
	_ = s.GracefulStop(2 * time.Second)

	select {
	case err := <-errCh:
		if err != nil && !isExpectedMultiServerError(err) {
			t.Errorf("unexpected server error: %v", err)
		}
	default:
	}
}

// isExpectedMultiServerError 检查是否是预期的多服务器关闭错误。
func isExpectedMultiServerError(err error) bool {
	if err == nil {
		return true
	}
	errStr := err.Error()
	return strings.Contains(errStr, "closed") ||
		strings.Contains(errStr, "use of closed") ||
		strings.Contains(errStr, "listener closed")
}
