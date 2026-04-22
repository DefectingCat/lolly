// Package server 提供 startSingleMode 集成测试。
//
// 该文件测试 startSingleMode 函数的各种配置场景，
// 包括静态文件、代理、监控端点、TLS 等配置的实际启动。
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

// TestStartSingleMode_Integration_WithStaticFiles 测试 startSingleMode 静态文件实际启动。
func TestStartSingleMode_Integration_WithStaticFiles(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	tempDir := t.TempDir()
	// 创建一个测试文件
	testFile := tempDir + "/index.html"
	if err := os.WriteFile(testFile, []byte("<html>test</html>"), 0o644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	cfg := &config.Config{
		Servers: []config.ServerConfig{{
			Listen: "127.0.0.1:0",
			Static: []config.StaticConfig{
				{
					Path:  "/static",
					Root:  tempDir,
					Index: []string{"index.html"},
				},
			},
		}},
	}

	s := New(cfg)

	// 在 goroutine 中启动服务器
	errCh := make(chan error, 1)
	go func() {
		if err := s.Start(); err != nil {
			errCh <- err
		}
	}()

	// 等待服务器启动
	time.Sleep(50 * time.Millisecond)

	// 停止服务器
	_ = s.GracefulStop(2 * time.Second)

	select {
	case err := <-errCh:
		// 服务器正常关闭会返回 nil 或 listener closed 错误
		if err != nil && !isExpectedServerErrorForIntegration(err) {
			t.Errorf("unexpected server error: %v", err)
		}
	default:
		// 服务器仍在运行，已通过 GracefulStop 关闭
	}
}

// TestStartSingleMode_Integration_WithProxy 测试 startSingleMode 代理实际启动。
func TestStartSingleMode_Integration_WithProxy(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	cfg := &config.Config{
		Servers: []config.ServerConfig{{
			Listen: "127.0.0.1:0",
			Proxy: []config.ProxyConfig{
				{
					Path: "/api",
					Targets: []config.ProxyTarget{
						{URL: "http://127.0.0.1:9999", Weight: 1}, // 不存在的后端
					},
				},
			},
		}},
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
		if err != nil && !isExpectedServerErrorForIntegration(err) {
			t.Errorf("unexpected server error: %v", err)
		}
	default:
	}
}

// TestStartSingleMode_Integration_WithMonitoring 测试 startSingleMode 监控端点实际启动。
func TestStartSingleMode_Integration_WithMonitoring(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	cfg := &config.Config{
		Servers: []config.ServerConfig{{
			Listen: "127.0.0.1:0",
		}},
		Monitoring: config.MonitoringConfig{
			Status: config.StatusConfig{
				Enabled: true,
				Path:    "/_status",
				Allow:   []string{"127.0.0.1"},
			},
			Pprof: config.PprofConfig{
				Enabled: true,
				Path:    "/debug/pprof",
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
		if err != nil && !isExpectedServerErrorForIntegration(err) {
			t.Errorf("unexpected server error: %v", err)
		}
	default:
	}
}

// TestStartSingleMode_Integration_WithCacheAPI 测试 startSingleMode 缓存 API 实际启动。
func TestStartSingleMode_Integration_WithCacheAPI(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	cfg := &config.Config{
		Servers: []config.ServerConfig{{
			Listen: "127.0.0.1:0",
			CacheAPI: &config.CacheAPIConfig{
				Enabled: true,
				Path:    "/_cache/purge",
			},
		}},
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
		if err != nil && !isExpectedServerErrorForIntegration(err) {
			t.Errorf("unexpected server error: %v", err)
		}
	default:
	}
}

// TestStartSingleMode_Integration_WithCompression 测试 startSingleMode 压缩配置实际启动。
func TestStartSingleMode_Integration_WithCompression(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	cfg := &config.Config{
		Servers: []config.ServerConfig{{
			Listen: "127.0.0.1:0",
			Compression: config.CompressionConfig{
				Type:  "gzip",
				Level: 6,
			},
		}},
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
		if err != nil && !isExpectedServerErrorForIntegration(err) {
			t.Errorf("unexpected server error: %v", err)
		}
	default:
	}
}

// TestStartSingleMode_Integration_WithSecurity 测试 startSingleMode 安全配置实际启动。
func TestStartSingleMode_Integration_WithSecurity(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	cfg := &config.Config{
		Servers: []config.ServerConfig{{
			Listen: "127.0.0.1:0",
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
		}},
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
		if err != nil && !isExpectedServerErrorForIntegration(err) {
			t.Errorf("unexpected server error: %v", err)
		}
	default:
	}
}

// TestStartSingleMode_Integration_WithRewrite 测试 startSingleMode 重写配置实际启动。
func TestStartSingleMode_Integration_WithRewrite(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	cfg := &config.Config{
		Servers: []config.ServerConfig{{
			Listen: "127.0.0.1:0",
			Rewrite: []config.RewriteRule{
				{Pattern: "^/old/(.*)$", Replacement: "/new/$1"},
			},
		}},
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
		if err != nil && !isExpectedServerErrorForIntegration(err) {
			t.Errorf("unexpected server error: %v", err)
		}
	default:
	}
}

// TestStartSingleMode_Integration_WithPerformance 测试 startSingleMode 性能配置实际启动。
func TestStartSingleMode_Integration_WithPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	cfg := &config.Config{
		Servers: []config.ServerConfig{{
			Listen: "127.0.0.1:0",
		}},
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
		if err != nil && !isExpectedServerErrorForIntegration(err) {
			t.Errorf("unexpected server error: %v", err)
		}
	default:
	}
}

// TestStartSingleMode_Integration_WithProxyLocationTypes 测试代理不同位置类型实际启动。
func TestStartSingleMode_Integration_WithProxyLocationTypes(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	cfg := &config.Config{
		Servers: []config.ServerConfig{{
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
				{
					Path:         "^/api/regex/(.*)$",
					LocationType: "regex",
					Targets:      []config.ProxyTarget{{URL: "http://127.0.0.1:9999", Weight: 1}},
				},
			},
		}},
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
		if err != nil && !isExpectedServerErrorForIntegration(err) {
			t.Errorf("unexpected server error: %v", err)
		}
	default:
	}
}

// TestStartSingleMode_Integration_WithStaticLocationTypes 测试静态文件不同位置类型实际启动。
func TestStartSingleMode_Integration_WithStaticLocationTypes(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	tempDir := t.TempDir()

	cfg := &config.Config{
		Servers: []config.ServerConfig{{
			Listen: "127.0.0.1:0",
			Static: []config.StaticConfig{
				{
					Path:         "/static/exact",
					Root:         tempDir,
					LocationType: "exact",
				},
				{
					Path:         "/static/priority",
					Root:         tempDir,
					LocationType: "prefix_priority",
				},
			},
		}},
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
		if err != nil && !isExpectedServerErrorForIntegration(err) {
			t.Errorf("unexpected server error: %v", err)
		}
	default:
	}
}

// TestStartSingleMode_Integration_WithHealthCheck 测试代理健康检查实际启动。
func TestStartSingleMode_Integration_WithHealthCheck(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	cfg := &config.Config{
		Servers: []config.ServerConfig{{
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
		}},
	}

	s := New(cfg)

	errCh := make(chan error, 1)
	go func() {
		if err := s.Start(); err != nil {
			errCh <- err
		}
	}()

	time.Sleep(100 * time.Millisecond) // 给健康检查一些时间启动
	_ = s.GracefulStop(2 * time.Second)

	select {
	case err := <-errCh:
		if err != nil && !isExpectedServerErrorForIntegration(err) {
			t.Errorf("unexpected server error: %v", err)
		}
	default:
	}
}

// TestStartSingleMode_Integration_WithMIMETypes 测试 MIME 类型配置实际启动。
func TestStartSingleMode_Integration_WithMIMETypes(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	cfg := &config.Config{
		Servers: []config.ServerConfig{{
			Listen: "127.0.0.1:0",
			Types: config.TypesConfig{
				Map: map[string]string{
					".wasm": "application/wasm",
				},
				DefaultType: "application/octet-stream",
			},
		}},
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
		if err != nil && !isExpectedServerErrorForIntegration(err) {
			t.Errorf("unexpected server error: %v", err)
		}
	default:
	}
}

// TestStartSingleMode_Integration_WithErrorPage 测试错误页面配置实际启动。
func TestStartSingleMode_Integration_WithErrorPage(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	tempDir := t.TempDir()
	errorPage := tempDir + "/404.html"
	if err := os.WriteFile(errorPage, []byte("<html>Not Found</html>"), 0o644); err != nil {
		t.Fatalf("failed to create error page: %v", err)
	}

	cfg := &config.Config{
		Servers: []config.ServerConfig{{
			Listen: "127.0.0.1:0",
			Security: config.SecurityConfig{
				ErrorPage: config.ErrorPageConfig{
					Pages:   map[int]string{404: errorPage},
					Default: errorPage,
				},
			},
		}},
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
		if err != nil && !isExpectedServerErrorForIntegration(err) {
			t.Errorf("unexpected server error: %v", err)
		}
	default:
	}
}

// TestStartSingleMode_Integration_WithConnLimiter 测试连接限制配置实际启动。
func TestStartSingleMode_Integration_WithConnLimiter(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	cfg := &config.Config{
		Servers: []config.ServerConfig{{
			Listen: "127.0.0.1:0",
			Security: config.SecurityConfig{
				RateLimit: config.RateLimitConfig{
					ConnLimit: 10,
				},
			},
		}},
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
		if err != nil && !isExpectedServerErrorForIntegration(err) {
			t.Errorf("unexpected server error: %v", err)
		}
	default:
	}
}

// TestStartSingleMode_Integration_WithAuthRequest 测试外部认证配置实际启动。
func TestStartSingleMode_Integration_WithAuthRequest(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	cfg := &config.Config{
		Servers: []config.ServerConfig{{
			Listen: "127.0.0.1:0",
			Security: config.SecurityConfig{
				AuthRequest: config.AuthRequestConfig{
					Enabled: true,
					URI:     "/auth/validate",
					Timeout: 5 * time.Second,
				},
			},
		}},
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
		if err != nil && !isExpectedServerErrorForIntegration(err) {
			t.Errorf("unexpected server error: %v", err)
		}
	default:
	}
}

// isExpectedServerErrorForIntegration 检查是否是预期的服务器关闭错误。
func isExpectedServerErrorForIntegration(err error) bool {
	if err == nil {
		return true
	}
	// fasthttp 服务器关闭时的常见错误
	errStr := err.Error()
	return strings.Contains(errStr, "closed") ||
		strings.Contains(errStr, "use of closed") ||
		strings.Contains(errStr, "listener closed")
}
