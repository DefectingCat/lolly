// Package server 提供 HTTP 服务器的核心实现测试补充。
//
// 该文件补充以下测试覆盖：
//   - 多模式启动逻辑测试（single/vhost/multi_server/auto）
//   - 多服务器模式 shutdownServers 函数测试
//   - 监听器创建测试（TCP/Unix socket）
//   - StopWithTimeout 超时行为测试
//   - GracefulStop 超时行为测试
//   - 中间件链错误路径测试

package server

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/config"
	"rua.plus/lolly/internal/resolver"
)

// TestServer_GetMode_Single 测试单服务器模式
func TestServer_GetMode_Single(t *testing.T) {
	cfg := &config.Config{
		Mode: config.ServerModeSingle,
		Servers: []config.ServerConfig{{
			Listen: ":0",
		}},
	}

	s := New(cfg)
	if s.config.GetMode() != config.ServerModeSingle {
		t.Errorf("Expected mode single, got %s", s.config.GetMode())
	}
}

// TestServer_GetMode_VHost 测试虚拟主机模式
func TestServer_GetMode_VHost(t *testing.T) {
	cfg := &config.Config{
		Mode: config.ServerModeVHost,
		Servers: []config.ServerConfig{
			{Listen: ":0", Name: "host1.example.com"},
			{Listen: ":0", Name: "host2.example.com"},
		},
	}

	s := New(cfg)
	if s.config.GetMode() != config.ServerModeVHost {
		t.Errorf("Expected mode vhost, got %s", s.config.GetMode())
	}
}

// TestServer_GetMode_MultiServer 测试多服务器模式
func TestServer_GetMode_MultiServer(t *testing.T) {
	cfg := &config.Config{
		Mode: config.ServerModeMultiServer,
		Servers: []config.ServerConfig{
			{Listen: ":8080", Name: "server1"},
			{Listen: ":8081", Name: "server2"},
		},
	}

	s := New(cfg)
	if s.config.GetMode() != config.ServerModeMultiServer {
		t.Errorf("Expected mode multi_server, got %s", s.config.GetMode())
	}
}

// TestServer_GetMode_Auto 测试自动模式
func TestServer_GetMode_Auto(t *testing.T) {
	cfg := &config.Config{
		Mode: config.ServerModeAuto,
		Servers: []config.ServerConfig{{
			Listen: ":0",
		}},
	}

	s := New(cfg)
	mode := s.config.GetMode()
	if mode != config.ServerModeSingle {
		t.Errorf("Expected auto to resolve to single, got %s", mode)
	}
}

// TestShutdownServers_Empty 测试空服务器列表关闭
func TestShutdownServers_Empty(t *testing.T) {
	err := shutdownServers(nil, nil)
	if err != nil {
		t.Errorf("Expected nil error for empty servers, got %v", err)
	}
}

// TestShutdownServers_NilServer 测试含 nil 的服务器列表关闭
func TestShutdownServers_NilServer(t *testing.T) {
	servers := []*fasthttp.Server{nil, nil}
	err := shutdownServers(nil, servers)
	if err != nil {
		t.Errorf("Expected nil error with nil servers, got %v", err)
	}
}

// TestShutdownServers_Timeout 测试关闭超时
func TestShutdownServers_Timeout(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}

	fastSrv := &fasthttp.Server{
		Handler: func(ctx *fasthttp.RequestCtx) {
			select {}
		},
	}

	go func() {
		_ = fastSrv.Serve(ln)
	}()

	time.Sleep(50 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	err = shutdownServers(ctx, []*fasthttp.Server{fastSrv})
	// context 超时后 shutdownServers 会返回 ctx.Err()
	if err != nil && err != context.DeadlineExceeded {
		t.Errorf("Expected context.DeadlineExceeded or nil, got: %v", err)
	}

	_ = fastSrv.Shutdown()
	_ = ln.Close()
}

// TestStopWithTimeout_DefaultTimeout 测试零超时使用默认值
func TestStopWithTimeout_DefaultTimeout(t *testing.T) {
	cfg := &config.Config{
		Servers: []config.ServerConfig{{
			Listen: ":0",
		}},
	}

	s := New(cfg)

	err := s.StopWithTimeout(0)
	if err != nil {
		t.Errorf("StopWithTimeout(0) should succeed, got %v", err)
	}

	err = s.StopWithTimeout(-1 * time.Second)
	if err != nil {
		t.Errorf("StopWithTimeout(-1s) should succeed, got %v", err)
	}
}

// TestStopWithTimeout_MultiServerMode 测试多服务器模式停止
func TestStopWithTimeout_MultiServerMode(t *testing.T) {
	cfg := &config.Config{
		Mode: config.ServerModeMultiServer,
		Servers: []config.ServerConfig{
			{Listen: ":0", Name: "server1"},
			{Listen: ":0", Name: "server2"},
		},
	}

	s := New(cfg)

	err := s.StopWithTimeout(1 * time.Second)
	if err != nil {
		t.Errorf("StopWithTimeout on non-started multi-server should succeed: %v", err)
	}
}

// TestGracefulStop_Timeout 测试优雅停止超时
func TestGracefulStop_Timeout(t *testing.T) {
	cfg := &config.Config{
		Servers: []config.ServerConfig{{
			Listen: ":0",
		}},
	}

	s := New(cfg)

	s.fastServer = &fasthttp.Server{
		Handler: func(ctx *fasthttp.RequestCtx) {
			ctx.SetBodyString("ok")
		},
	}
	s.running = true

	err := s.GracefulStop(100 * time.Millisecond)
	if err != nil {
		t.Errorf("GracefulStop should succeed: %v", err)
	}
}

// TestServer_SetUpgradeManager 测试设置升级管理器
func TestServer_SetUpgradeManager(t *testing.T) {
	cfg := &config.Config{
		Servers: []config.ServerConfig{{
			Listen: ":0",
		}},
	}

	s := New(cfg)
	mgr := NewUpgradeManager(s)

	s.SetUpgradeManager(mgr)
	if s.upgradeManager != mgr {
		t.Error("upgradeManager not set correctly")
	}
}

// mockResolver 用于测试的 mock DNS 解析器
type mockResolver struct{}

func (m *mockResolver) LookupHost(ctx context.Context, host string) ([]string, error) {
	return []string{"127.0.0.1"}, nil
}

func (m *mockResolver) LookupHostWithCache(ctx context.Context, host string) ([]string, error) {
	return []string{"127.0.0.1"}, nil
}

func (m *mockResolver) Refresh(host string) error { return nil }
func (m *mockResolver) Start() error              { return nil }
func (m *mockResolver) Stop() error               { return nil }
func (m *mockResolver) Stats() resolver.Stats     { return resolver.Stats{} }

// TestServer_Resolver 测试 DNS 解析器设置
func TestServer_Resolver(t *testing.T) {
	cfg := &config.Config{
		Servers: []config.ServerConfig{{
			Listen: ":0",
		}},
	}

	s := New(cfg)

	if s.GetResolver() != nil {
		t.Error("Expected nil resolver initially")
	}

	mockRes := &mockResolver{}
	s.SetResolver(mockRes)

	if s.GetResolver() == nil {
		t.Error("Resolver not set correctly")
	}
}

// TestCreateListener_TCP 测试 TCP 监听器创建
func TestCreateListener_TCP(t *testing.T) {
	cfg := &config.Config{
		Servers: []config.ServerConfig{{
			Listen: ":0",
		}},
	}

	s := New(cfg)
	serverCfg := &cfg.Servers[0]

	ln, err := s.createListener(serverCfg)
	if err != nil {
		t.Fatalf("createListener failed: %v", err)
	}
	defer func() { _ = ln.Close() }()

	if ln == nil {
		t.Fatal("Expected non-nil listener")
	}

	addr := ln.Addr().(*net.TCPAddr)
	if addr.Port == 0 {
		t.Error("Expected non-zero port")
	}
}

// TestCreateListener_InvalidTCP 测试无效 TCP 监听地址
func TestCreateListener_InvalidTCP(t *testing.T) {
	cfg := &config.Config{
		Servers: []config.ServerConfig{{
			Listen: "invalid:address:format",
		}},
	}

	s := New(cfg)
	_, err := s.createListener(&cfg.Servers[0])
	if err == nil {
		t.Error("Expected error for invalid listen address")
	}
}

// TestListenerManagement 测试监听器管理
func TestListenerManagement(t *testing.T) {
	cfg := &config.Config{
		Servers: []config.ServerConfig{{
			Listen: ":0",
		}},
	}

	s := New(cfg)

	ln1, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener 1: %v", err)
	}
	defer func() { _ = ln1.Close() }()

	ln2, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener 2: %v", err)
	}
	defer func() { _ = ln2.Close() }()

	s.SetListeners([]net.Listener{ln1, ln2})

	listeners := s.GetListeners()
	if len(listeners) != 2 {
		t.Errorf("Expected 2 listeners, got %d", len(listeners))
	}
}

// TestStart_WithGoroutinePoolAndFileCache 测试同时启用 GoroutinePool 和 FileCache
func TestStart_WithGoroutinePoolAndFileCache(t *testing.T) {
	cfg := &config.Config{
		Servers: []config.ServerConfig{{
			Listen: ":0",
		}},
		Performance: config.PerformanceConfig{
			GoroutinePool: config.GoroutinePoolConfig{
				Enabled:    true,
				MaxWorkers: 50,
				MinWorkers: 5,
			},
			FileCache: config.FileCacheConfig{
				MaxEntries: 500,
				MaxSize:    50 * 1024 * 1024,
			},
		},
	}

	s := New(cfg)
	if s == nil {
		t.Fatal("New() returned nil")
	}

	if s.config.Performance.GoroutinePool.Enabled != true {
		t.Error("GoroutinePool should be enabled")
	}
}

// TestServer_GetHandler_NilThenSet 测试 handler 的 nil 到设置
func TestServer_GetHandler_NilThenSet(t *testing.T) {
	cfg := &config.Config{
		Servers: []config.ServerConfig{{
			Listen: ":0",
		}},
	}

	s := New(cfg)

	if s.GetHandler() != nil {
		t.Error("Expected nil handler initially")
	}

	testHandler := func(ctx *fasthttp.RequestCtx) {
		ctx.SetBodyString("test handler response")
	}
	s.handler = testHandler

	got := s.GetHandler()
	if got == nil {
		t.Error("Expected non-nil handler after setting")
	}

	ctx := &fasthttp.RequestCtx{}
	got(ctx)
	if string(ctx.Response.Body()) != "test handler response" {
		t.Errorf("Handler response = %q, want %q", string(ctx.Response.Body()), "test handler response")
	}
}

// TestServer_TrackStats_Concurrent 测试并发统计追踪
func TestServer_TrackStats_Concurrent(t *testing.T) {
	cfg := &config.Config{
		Servers: []config.ServerConfig{{
			Listen: ":0",
		}},
	}

	s := New(cfg)

	handler := func(ctx *fasthttp.RequestCtx) {
		ctx.SetBodyString("ok")
	}

	wrappedHandler := s.trackStats(handler)

	const numGoroutines = 100
	done := make(chan bool, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			ctx := &fasthttp.RequestCtx{}
			ctx.Init(&fasthttp.Request{}, nil, nil)
			wrappedHandler(ctx)
			done <- true
		}()
	}

	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	if s.requests.Load() != int64(numGoroutines) {
		t.Errorf("Expected %d requests, got %d", numGoroutines, s.requests.Load())
	}
}

// TestBuildMiddlewareChain_BodyLimit 测试请求体限制中间件
func TestBuildMiddlewareChain_BodyLimit(t *testing.T) {
	cfg := &config.Config{
		Logging: config.LoggingConfig{},
		Servers: []config.ServerConfig{{
			Listen: ":0",
			Proxy: []config.ProxyConfig{{
				Path:              "/api/",
				ClientMaxBodySize: "1MB",
			}},
			ClientMaxBodySize: "10MB",
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

// TestBuildMiddlewareChain_BodyLimit_Invalid 测试无效的请求体限制
func TestBuildMiddlewareChain_BodyLimit_Invalid(t *testing.T) {
	cfg := &config.Config{
		Logging: config.LoggingConfig{},
		Servers: []config.ServerConfig{{
			Listen:            ":0",
			ClientMaxBodySize: "invalid_size",
		}},
	}

	s := New(cfg)
	_, err := s.buildMiddlewareChain(&cfg.Servers[0])
	if err == nil {
		t.Error("Expected error for invalid body limit size")
	}
}
