// Package http2 提供 HTTP/2 服务器测试。
//
// 该文件包含 HTTP/2 服务器的单元测试和集成测试：
//   - 服务器创建和配置测试
//   - ALPN 协议协商测试
//   - HTTP/1.1 fallback 测试
//
// 作者：xfy
package http2

import (
	"crypto/tls"
	"net"
	"testing"
	"time"

	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/config"
)

// TestNewServer 测试 HTTP/2 服务器创建。
func TestNewServer(t *testing.T) {
	tests := []struct {
		cfg       *config.HTTP2Config
		handler   fasthttp.RequestHandler
		tlsConfig *tls.Config
		name      string
		wantErr   bool
	}{
		{
			name: "有效配置",
			cfg: &config.HTTP2Config{
				Enabled:              true,
				MaxConcurrentStreams: 128,
				MaxHeaderListSize:    1048576,
				IdleTimeout:          120 * time.Second,
				PushEnabled:          false,
				H2CEnabled:           false,
			},
			handler:   func(_ *fasthttp.RequestCtx) {},
			tlsConfig: nil,
			wantErr:   false,
		},
		{
			name:    "默认配置",
			cfg:     &config.HTTP2Config{},
			handler: func(_ *fasthttp.RequestCtx) {},
			wantErr: false,
		},
		{
			name:    "nil配置",
			cfg:     nil,
			handler: func(_ *fasthttp.RequestCtx) {},
			wantErr: true,
		},
		{
			name: "nil handler",
			cfg: &config.HTTP2Config{
				Enabled: true,
			},
			handler: nil,
			wantErr: true,
		},
		{
			name: "自定义并发流数量",
			cfg: &config.HTTP2Config{
				Enabled:              true,
				MaxConcurrentStreams: 256,
			},
			handler: func(_ *fasthttp.RequestCtx) {},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, err := NewServer(tt.cfg, tt.handler, tt.tlsConfig)
			if tt.wantErr {
				if err == nil {
					t.Errorf("NewServer() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("NewServer() unexpected error: %v", err)
				return
			}
			if server == nil {
				t.Error("NewServer() returned nil server")
				return
			}

			// 验证配置正确应用
			if server.config != tt.cfg {
				t.Error("NewServer() config not set correctly")
			}
			if server.handler == nil {
				t.Error("NewServer() handler not set")
			}
		})
	}
}

// TestServerDefaultValues 测试服务器默认值。
func TestServerDefaultValues(t *testing.T) {
	cfg := &config.HTTP2Config{
		Enabled: true,
	}
	handler := func(_ *fasthttp.RequestCtx) {}

	server, err := NewServer(cfg, handler, nil)
	if err != nil {
		t.Fatalf("NewServer() error: %v", err)
	}

	// 验证默认并发流数量
	if server.http2Server.MaxConcurrentStreams == 0 {
		t.Error("Expected default MaxConcurrentStreams to be set")
	}

	// 验证默认空闲超时
	if server.http2Server.IdleTimeout == 0 {
		t.Error("Expected default IdleTimeout to be set")
	}
}

// TestServe_AcceptError 测试 Accept 错误处理。
func TestServe_AcceptError(t *testing.T) {
	cfg := &config.HTTP2Config{Enabled: true}
	server, err := NewServer(cfg, func(_ *fasthttp.RequestCtx) {}, nil)
	if err != nil {
		t.Fatalf("NewServer() error: %v", err)
	}

	// 创建一个已关闭的监听器
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}

	// 启动服务器
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Serve(ln)
	}()

	// 关闭监听器触发 Accept 错误
	if err := ln.Close(); err != nil {
		t.Logf("Failed to close listener: %v", err)
	}

	// 停止服务器
	_ = server.Stop()

	// 服务器应该正常退出
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Serve() unexpected error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("Serve() did not exit in time")
	}
}

// TestServe_AlreadyRunning 测试服务器重复启动。
func TestServe_AlreadyRunning(t *testing.T) {
	cfg := &config.HTTP2Config{Enabled: true}
	server, err := NewServer(cfg, func(_ *fasthttp.RequestCtx) {}, nil)
	if err != nil {
		t.Fatalf("NewServer() error: %v", err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	defer func() {
		if err := ln.Close(); err != nil {
			t.Logf("Failed to close listener: %v", err)
		}
	}()

	// 启动服务器
	go func() {
		_ = server.Serve(ln)
	}()

	// 等待服务器启动
	time.Sleep(50 * time.Millisecond)

	// 尝试再次启动
	err = server.Serve(ln)
	if err == nil {
		t.Error("Serve() should return error when already running")
	}

	// 停止服务器
	_ = server.Stop()
}

// TestStop_GracefulShutdownTimeout 测试优雅关闭超时。
func TestStop_GracefulShutdownTimeout(t *testing.T) {
	cfg := &config.HTTP2Config{
		Enabled:                 true,
		GracefulShutdownTimeout: 100 * time.Millisecond,
	}

	handler := func(ctx *fasthttp.RequestCtx) {
		// 模拟长时间处理
		time.Sleep(2 * time.Second)
		ctx.SetStatusCode(fasthttp.StatusOK)
	}

	server, err := NewServer(cfg, handler, nil)
	if err != nil {
		t.Fatalf("NewServer() error: %v", err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	defer func() {
		if err := ln.Close(); err != nil {
			t.Logf("Failed to close listener: %v", err)
		}
	}()

	// 启动服务器
	go func() {
		_ = server.Serve(ln)
	}()

	// 等待服务器启动
	time.Sleep(50 * time.Millisecond)

	// 停止服务器（应该超时）
	start := time.Now()
	_ = server.Stop()
	elapsed := time.Since(start)

	// 应该在超时后返回
	if elapsed > 500*time.Millisecond {
		t.Errorf("Stop() took too long: %v", elapsed)
	}
}

// TestStop_NotRunning 测试停止未运行的服务器。
func TestStop_NotRunning(t *testing.T) {
	cfg := &config.HTTP2Config{Enabled: true}
	server, err := NewServer(cfg, func(_ *fasthttp.RequestCtx) {}, nil)
	if err != nil {
		t.Fatalf("NewServer() error: %v", err)
	}

	// 停止未运行的服务器应该返回 nil
	err = server.Stop()
	if err != nil {
		t.Errorf("Stop() on non-running server should return nil, got: %v", err)
	}
}

// TestConnectionPool_CloseAll 测试连接池关闭所有连接。
func TestConnectionPool_CloseAll(t *testing.T) {
	pool := newConnectionPool()

	// 创建多个连接
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	defer func() {
		if err := ln.Close(); err != nil {
			t.Logf("Failed to close listener: %v", err)
		}
	}()

	conn1, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("Failed to dial: %v", err)
	}
	conn2, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("Failed to dial: %v", err)
	}

	pool.add("key1", conn1)
	pool.add("key1", conn2)

	// 关闭所有连接
	pool.closeAll()

	// 验证连接池已清空
	if count := len(pool.conns["key1"]); count != 0 {
		t.Errorf("Expected count 0 after closeAll, got %d", count)
	}
}

// TestConnectionPool_RemoveNonExistent 测试移除不存在的连接。
func TestConnectionPool_RemoveNonExistent(t *testing.T) {
	pool := newConnectionPool()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	defer func() {
		if err := ln.Close(); err != nil {
			t.Logf("Failed to close listener: %v", err)
		}
	}()

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("Failed to dial: %v", err)
	}
	defer func() {
		_ = conn.Close()
	}()

	// 移除不存在的 key/conn 组合不应 panic
	pool.remove("nonexistent", conn)
	pool.remove("key1", conn)
}
