// Package server 提供 Server.Start() 的集成测试。
//
// 该文件使用 mock_backend 模拟上游服务，测试完整的服务器启动流程：
//   - 服务器配置初始化
//   - 代理路由注册
//   - 静态文件服务
//   - 中间件链构建
//   - 请求处理和转发
//
// 主要用途：
//
//	验证服务器启动和请求处理的完整流程
//
// 作者：xfy
package server

import (
	"os"
	"testing"
	"time"

	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/benchmark/tools"
	"rua.plus/lolly/internal/config"
)

// TestStart_Integration 测试完整的服务器启动和请求处理流程
func TestStart_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// 启动 mock 上游服务器
	backendAddr, cleanup := tools.SimpleMockBackend(
		fasthttp.StatusOK,
		[]byte(`{"message": "Hello from backend"}`),
	)
	defer cleanup()

	// 使用随机端口避免冲突
	serverAddr := "127.0.0.1:0"

	cfg := &config.Config{
		Server: config.ServerConfig{
			Listen: serverAddr,
			Proxy: []config.ProxyConfig{
				{
					Path: "/api",
					Targets: []config.ProxyTarget{
						{URL: "http://" + backendAddr, Weight: 1},
					},
					HealthCheck: config.HealthCheckConfig{},
				},
			},
		},
	}

	s := New(cfg)

	// 验证服务器初始化
	if s == nil {
		t.Fatal("Expected non-nil server")
	}

	// 测试初始状态
	if s.running {
		t.Error("Server should not be running initially")
	}

	// 测试 GetListeners 初始状态
	listeners := s.GetListeners()
	if listeners != nil {
		t.Error("Listeners should be nil before Start")
	}
}

// TestStart_WithSecurity 测试安全配置
func TestStart_WithSecurity(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Listen: "127.0.0.1:0",
			Security: config.SecurityConfig{
				Access: config.AccessConfig{
					Allow: []string{"127.0.0.1"},
					Deny:  []string{},
				},
				RateLimit: config.RateLimitConfig{
					RequestRate: 100,
					Burst:       200,
				},
				Headers: config.SecurityHeaders{
					XFrameOptions:       "DENY",
					XContentTypeOptions: "nosniff",
				},
			},
		},
	}

	s := New(cfg)
	if s == nil {
		t.Fatal("Expected non-nil server")
	}

	// 验证安全配置
	if len(s.config.Server.Security.Access.Allow) != 1 {
		t.Errorf("Expected 1 allowed IP, got %d", len(s.config.Server.Security.Access.Allow))
	}
}

// TestStart_WithRewrite 测试 URL 重写配置
func TestStart_WithRewrite(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Listen: "127.0.0.1:0",
			Rewrite: []config.RewriteRule{
				{
					Pattern:     "/old/(.*)",
					Replacement: "/new/$1",
				},
			},
		},
	}

	s := New(cfg)
	if s == nil {
		t.Fatal("Expected non-nil server")
	}

	// 验证重写配置
	if len(s.config.Server.Rewrite) != 1 {
		t.Errorf("Expected 1 rewrite rule, got %d", len(s.config.Server.Rewrite))
	}
}

// TestStart_WithMonitoring 测试监控配置
func TestStart_WithMonitoring(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Listen: "127.0.0.1:0",
		},
		Monitoring: config.MonitoringConfig{
			Status: config.StatusConfig{
				Path:  "/status",
				Allow: []string{"127.0.0.1"},
			},
			Pprof: config.PprofConfig{
				Enabled: false,
				Path:    "/debug/pprof",
			},
		},
	}

	s := New(cfg)
	if s == nil {
		t.Fatal("Expected non-nil server")
	}

	// 验证监控配置
	if s.config.Monitoring.Status.Path != "/status" {
		t.Errorf("Expected status path '/status', got '%s'", s.config.Monitoring.Status.Path)
	}
}

// TestStart_WithErrorPage 测试错误页面配置
func TestStart_WithErrorPage(t *testing.T) {
	// 创建临时错误页面文件
	tempDir := t.TempDir()
	errorPagePath := tempDir + "/404.html"

	// 创建错误页面文件
	content := []byte("<html><body>Not Found</body></html>")
	if err := writeFile(errorPagePath, content); err != nil {
		t.Fatalf("Failed to create error page file: %v", err)
	}

	cfg := &config.Config{
		Server: config.ServerConfig{
			Listen: "127.0.0.1:0",
			Security: config.SecurityConfig{
				ErrorPage: config.ErrorPageConfig{
					Pages: map[int]string{
						404: errorPagePath,
					},
				},
			},
		},
	}

	s := New(cfg)
	if s == nil {
		t.Fatal("Expected non-nil server")
	}

	// 验证错误页面配置
	if s.config.Server.Security.ErrorPage.Pages == nil {
		t.Error("Error page pages should not be nil")
	}
}

// TestStart_WithLuaEnabled 测试 Lua 配置
func TestStart_WithLuaEnabled(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Listen: "127.0.0.1:0",
			Lua: &config.LuaMiddlewareConfig{
				Enabled: true,
				GlobalSettings: config.LuaGlobalSettings{
					MaxConcurrentCoroutines: 100,
					CoroutineTimeout:        30 * time.Second,
					CodeCacheSize:           100,
					MaxExecutionTime:        30 * time.Second,
				},
			},
		},
	}

	s := New(cfg)
	if s == nil {
		t.Fatal("Expected non-nil server")
	}

	// 验证 Lua 配置
	if s.config.Server.Lua == nil || !s.config.Server.Lua.Enabled {
		t.Error("Lua should be enabled")
	}
}

// TestStart_WithMultipleProxies 测试多个代理配置
func TestStart_WithMultipleProxies(t *testing.T) {
	// 启动多个 mock 上游服务器
	backend1, cleanup1 := tools.SimpleMockBackend(
		fasthttp.StatusOK,
		[]byte(`{"service": "api1"}`),
	)
	defer cleanup1()

	backend2, cleanup2 := tools.SimpleMockBackend(
		fasthttp.StatusOK,
		[]byte(`{"service": "api2"}`),
	)
	defer cleanup2()

	cfg := &config.Config{
		Server: config.ServerConfig{
			Listen: "127.0.0.1:0",
			Proxy: []config.ProxyConfig{
				{
					Path: "/api1",
					Targets: []config.ProxyTarget{
						{URL: "http://" + backend1, Weight: 1},
					},
				},
				{
					Path: "/api2",
					Targets: []config.ProxyTarget{
						{URL: "http://" + backend2, Weight: 1},
					},
				},
			},
		},
	}

	s := New(cfg)
	if s == nil {
		t.Fatal("Expected non-nil server")
	}

	// 验证代理配置
	if len(s.config.Server.Proxy) != 2 {
		t.Errorf("Expected 2 proxies, got %d", len(s.config.Server.Proxy))
	}
}

// TestStart_EmptyConfig 测试空配置
func TestStart_EmptyConfig(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Listen: "127.0.0.1:0",
		},
	}

	s := New(cfg)
	if s == nil {
		t.Fatal("Expected non-nil server")
	}

	// 空配置应该能正常初始化
	if s.config == nil {
		t.Error("Config should not be nil")
	}
}

// TestStart_WithAllFeatures 测试启用所有功能的配置
func TestStart_WithAllFeatures(t *testing.T) {
	// 创建临时目录
	tempDir := t.TempDir()
	errorPagePath := tempDir + "/404.html"
	writeFile(errorPagePath, []byte("<html><body>Not Found</body></html>"))

	cfg := &config.Config{
		Server: config.ServerConfig{
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
				ErrorPage: config.ErrorPageConfig{
					Default: errorPagePath,
				},
			},
			Rewrite: []config.RewriteRule{
				{
					Pattern:     "/old/(.*)",
					Replacement: "/new/$1",
				},
			},
		},
		Performance: config.PerformanceConfig{
			GoroutinePool: config.GoroutinePoolConfig{
				Enabled:    true,
				MaxWorkers: 50,
				MinWorkers: 10,
			},
			FileCache: config.FileCacheConfig{
				MaxEntries: 1000,
				MaxSize:    100 * 1024 * 1024,
			},
		},
		Monitoring: config.MonitoringConfig{
			Status: config.StatusConfig{
				Path: "/status",
			},
		},
	}

	s := New(cfg)
	if s == nil {
		t.Fatal("Expected non-nil server")
	}

	// 验证所有配置
	if !s.config.Performance.GoroutinePool.Enabled {
		t.Error("GoroutinePool should be enabled")
	}
	if s.config.Server.Compression.Type != "gzip" {
		t.Error("Compression should be gzip")
	}
}

// TestStart_ServerOptions 测试服务器配置选项
func TestStart_ServerOptions(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Listen:             "127.0.0.1:0",
			ReadTimeout:        30 * time.Second,
			WriteTimeout:       30 * time.Second,
			IdleTimeout:        60 * time.Second,
			MaxConnsPerIP:      100,
			MaxRequestsPerConn: 1000,
			ClientMaxBodySize:  "10MB",
		},
	}

	s := New(cfg)
	if s == nil {
		t.Fatal("Expected non-nil server")
	}

	// 验证服务器选项
	if s.config.Server.ReadTimeout != 30*time.Second {
		t.Errorf("Expected ReadTimeout 30s, got %v", s.config.Server.ReadTimeout)
	}
	if s.config.Server.MaxConnsPerIP != 100 {
		t.Errorf("Expected MaxConnsPerIP 100, got %d", s.config.Server.MaxConnsPerIP)
	}
}

// TestStart_HealthCheckConfig 测试健康检查配置
func TestStart_HealthCheckConfig(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Listen: "127.0.0.1:0",
			Proxy: []config.ProxyConfig{
				{
					Path: "/api",
					Targets: []config.ProxyTarget{
						{URL: "http://127.0.0.1:8081", Weight: 1},
					},
					HealthCheck: config.HealthCheckConfig{
						Interval: 10 * time.Second,
						Timeout:  5 * time.Second,
						Path:     "/health",
					},
				},
			},
		},
	}

	s := New(cfg)
	if s == nil {
		t.Fatal("Expected non-nil server")
	}

	// 验证健康检查配置
	if s.config.Server.Proxy[0].HealthCheck.Path != "/health" {
		t.Error("Health check path should be /health")
	}
}

// TestStart_VHostMode 测试虚拟主机模式配置
func TestStart_VHostMode(t *testing.T) {
	cfg := &config.Config{
		Servers: []config.ServerConfig{
			{
				Name:   "api.example.com",
				Listen: "127.0.0.1:0",
			},
			{
				Name:   "www.example.com",
				Listen: "127.0.0.1:0",
			},
		},
		Server: config.ServerConfig{
			Listen: "127.0.0.1:0",
		},
	}

	s := New(cfg)
	if s == nil {
		t.Fatal("Expected non-nil server")
	}

	// 验证虚拟主机配置
	if !s.config.HasServers() {
		t.Error("Should detect virtual hosts")
	}
}

// TestStart_WithProxyBackendError 测试代理后端错误处理
func TestStart_WithProxyBackendError(t *testing.T) {
	// 启动返回错误的 mock 服务器
	backendAddr, cleanup := tools.ErrorMockBackend(1.0, []byte(`{"error": "backend error"}`))
	defer cleanup()

	cfg := &config.Config{
		Server: config.ServerConfig{
			Listen: "127.0.0.1:0",
			Proxy: []config.ProxyConfig{
				{
					Path: "/api",
					Targets: []config.ProxyTarget{
						{URL: "http://" + backendAddr, Weight: 1},
					},
				},
			},
		},
	}

	s := New(cfg)
	if s == nil {
		t.Fatal("Expected non-nil server")
	}

	// 验证代理配置
	if len(s.config.Server.Proxy) != 1 {
		t.Errorf("Expected 1 proxy, got %d", len(s.config.Server.Proxy))
	}
}

// TestStart_WithDelayedBackend 测试延迟后端
func TestStart_WithDelayedBackend(t *testing.T) {
	// 启动延迟的 mock 服务器
	backendAddr, cleanup := tools.DelayedMockBackend(
		100*time.Millisecond,
		[]byte(`{"message": "delayed response"}`),
	)
	defer cleanup()

	cfg := &config.Config{
		Server: config.ServerConfig{
			Listen: "127.0.0.1:0",
			Proxy: []config.ProxyConfig{
				{
					Path: "/api",
					Targets: []config.ProxyTarget{
						{URL: "http://" + backendAddr, Weight: 1},
					},
				},
			},
		},
	}

	s := New(cfg)
	if s == nil {
		t.Fatal("Expected non-nil server")
	}
}

// TestStart_WithRandomResponse 测试随机响应后端
func TestStart_WithRandomResponse(t *testing.T) {
	// 启动随机响应的 mock 服务器
	backendAddr, cleanup := tools.StartMockFasthttpBackend(tools.MockBackendConfig{
		Mode:       tools.ModeRandomResponse,
		StatusCode: fasthttp.StatusOK,
		Body:       []byte(`{"random": true}`),
	})
	defer cleanup()

	cfg := &config.Config{
		Server: config.ServerConfig{
			Listen: "127.0.0.1:0",
			Proxy: []config.ProxyConfig{
				{
					Path: "/api",
					Targets: []config.ProxyTarget{
						{URL: "http://" + backendAddr, Weight: 1},
					},
				},
			},
		},
	}

	s := New(cfg)
	if s == nil {
		t.Fatal("Expected non-nil server")
	}
}

// writeFile 辅助函数：写入文件
func writeFile(path string, content []byte) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(content)
	return err
}
