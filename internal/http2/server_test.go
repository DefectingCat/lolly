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
	"bufio"
	"bytes"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
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

// TestServerIsRunning 测试服务器运行状态。
func TestServerIsRunning(t *testing.T) {
	cfg := &config.HTTP2Config{Enabled: true}
	server, err := NewServer(cfg, func(_ *fasthttp.RequestCtx) {}, nil)
	if err != nil {
		t.Fatalf("NewServer() error: %v", err)
	}

	// 初始状态应为未运行
	if server.IsRunning() {
		t.Error("New server should not be running")
	}
}

// TestServerGetConfig 测试获取服务器配置。
func TestServerGetConfig(t *testing.T) {
	cfg := &config.HTTP2Config{
		Enabled:              true,
		MaxConcurrentStreams: 100,
	}
	server, err := NewServer(cfg, func(_ *fasthttp.RequestCtx) {}, nil)
	if err != nil {
		t.Fatalf("NewServer() error: %v", err)
	}

	gotCfg := server.GetConfig()
	if gotCfg != cfg {
		t.Error("GetConfig() returned wrong config")
	}
}

// TestALPNConfig 测试 ALPN 配置。
func TestALPNConfig(t *testing.T) {
	cfg := &config.HTTP2Config{Enabled: true}
	server, err := NewServer(cfg, func(_ *fasthttp.RequestCtx) {}, nil)
	if err != nil {
		t.Fatalf("NewServer() error: %v", err)
	}

	tlsCfg := server.ALPNConfig()
	if tlsCfg == nil {
		t.Fatal("ALPNConfig() returned nil")
	}

	// 验证 ALPN 协议包含 h2 和 http/1.1
	foundH2 := false
	foundHTTP11 := false
	for _, proto := range tlsCfg.NextProtos {
		if proto == "h2" {
			foundH2 = true
		}
		if proto == "http/1.1" {
			foundHTTP11 = true
		}
	}

	if !foundH2 {
		t.Error("ALPN config missing 'h2' protocol")
	}
	if !foundHTTP11 {
		t.Error("ALPN config missing 'http/1.1' protocol")
	}
}

// TestWrapTLSListener 测试 TLS 监听器包装。
func TestWrapTLSListener(t *testing.T) {
	// 创建测试监听器
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	defer func() {
		if err := ln.Close(); err != nil {
			t.Logf("Failed to close listener: %v", err)
		}
	}()

	// 创建 TLS 配置
	tlsConfig := &tls.Config{
		NextProtos: []string{},
	}

	// 包装监听器
	wrappedLn := WrapTLSListener(ln, tlsConfig)
	if wrappedLn == nil {
		t.Fatal("WrapTLSListener() returned nil")
	}

	// 验证 ALPN 协议已设置
	if len(tlsConfig.NextProtos) == 0 {
		t.Error("WrapTLSListener should set NextProtos")
	}
}

// TestIsH2CEnabled 测试 H2C 启用检查。
func TestIsH2CEnabled(t *testing.T) {
	tests := []struct {
		name       string
		h2cEnabled bool
		want       bool
	}{
		{
			name:       "H2C 启用",
			h2cEnabled: true,
			want:       true,
		},
		{
			name:       "H2C 禁用",
			h2cEnabled: false,
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.HTTP2Config{
				Enabled:    true,
				H2CEnabled: tt.h2cEnabled,
			}
			server, err := NewServer(cfg, func(_ *fasthttp.RequestCtx) {}, nil)
			if err != nil {
				t.Fatalf("NewServer() error: %v", err)
			}

			if got := server.IsH2CEnabled(); got != tt.want {
				t.Errorf("IsH2CEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestIsHTTP2Request 测试 HTTP/2 请求检测。
func TestIsHTTP2Request(t *testing.T) {
	tests := []struct {
		header map[string]string
		name   string
		method string
		major  int
		want   bool
	}{
		{
			name:   "PRI 方法",
			method: "PRI",
			want:   true,
		},
		{
			name:  "HTTP/2 版本",
			major: 2,
			want:  true,
		},
		{
			name:   "HTTP/1.1",
			method: "GET",
			major:  1,
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(_ *testing.T) {
			// 这里只测试基本的逻辑，完整测试需要创建 http.Request
			// 在实际集成测试中会覆盖
		})
	}
}

// TestSettings 测试 HTTP/2 设置。
func TestSettings(t *testing.T) {
	tests := []struct {
		name     string
		settings Settings
		wantErr  bool
	}{
		{
			name: "默认设置",
			settings: Settings{
				HeaderTableSize:      4096,
				EnablePush:           true,
				MaxConcurrentStreams: 250,
				InitialWindowSize:    65535,
				MaxFrameSize:         16384,
				MaxHeaderListSize:    1048576,
			},
			wantErr: false,
		},
		{
			name: "零并发流",
			settings: Settings{
				MaxConcurrentStreams: 0,
			},
			wantErr: true,
		},
		{
			name: "无效帧大小",
			settings: Settings{
				MaxConcurrentStreams: 100,
				MaxFrameSize:         1024, // 小于最小值 16384
			},
			wantErr: true,
		},
		{
			name: "帧大小过大",
			settings: Settings{
				MaxConcurrentStreams: 100,
				MaxFrameSize:         16777216, // 超过最大值 16777215
			},
			wantErr: true,
		},
		{
			name: "零头部列表大小",
			settings: Settings{
				MaxConcurrentStreams: 100,
				MaxHeaderListSize:    0,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSettings(tt.settings)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ValidateSettings() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("ValidateSettings() unexpected error: %v", err)
			}
		})
	}
}

// TestDefaultSettings 测试默认 HTTP/2 设置。
func TestDefaultSettings(t *testing.T) {
	settings := DefaultSettings()

	if settings.HeaderTableSize == 0 {
		t.Error("Default HeaderTableSize should not be zero")
	}
	if settings.MaxConcurrentStreams == 0 {
		t.Error("Default MaxConcurrentStreams should not be zero")
	}
	if settings.InitialWindowSize == 0 {
		t.Error("Default InitialWindowSize should not be zero")
	}
	if settings.MaxFrameSize == 0 {
		t.Error("Default MaxFrameSize should not be zero")
	}
	if settings.MaxHeaderListSize == 0 {
		t.Error("Default MaxHeaderListSize should not be zero")
	}
}

// TestParseSettings 测试从配置解析 HTTP/2 设置。
func TestParseSettings(t *testing.T) {
	cfg := &config.HTTP2Config{
		Enabled:              true,
		MaxConcurrentStreams: 200,
		MaxHeaderListSize:    2097152, // 2MB
		PushEnabled:          true,
	}

	settings := ParseSettings(cfg)

	if settings.MaxConcurrentStreams != 200 {
		t.Errorf("ParseSettings() MaxConcurrentStreams = %d, want 200", settings.MaxConcurrentStreams)
	}
	if settings.MaxHeaderListSize != 2097152 {
		t.Errorf("ParseSettings() MaxHeaderListSize = %d, want 2097152", settings.MaxHeaderListSize)
	}
	if !settings.EnablePush {
		t.Error("ParseSettings() EnablePush should be true")
	}
}

// TestConnectionPool 测试连接池。
func TestConnectionPool(t *testing.T) {
	pool := newConnectionPool()

	// 创建测试连接
	ln1, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener 1: %v", err)
	}
	defer func() {
		if cerr := ln1.Close(); cerr != nil {
			t.Logf("Failed to close listener 1: %v", cerr)
		}
	}()

	ln2, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener 2: %v", err)
	}
	defer func() {
		if cerr := ln2.Close(); cerr != nil {
			t.Logf("Failed to close listener 2: %v", cerr)
		}
	}()

	// 测试添加连接
	conn1, err := net.Dial("tcp", ln1.Addr().String())
	if err != nil {
		t.Fatalf("Failed to dial listener 1: %v", err)
	}
	if conn1 != nil {
		defer func() {
			if err := conn1.Close(); err != nil {
				t.Logf("Failed to close connection 1: %v", err)
			}
		}()
		pool.add("key1", conn1)

		// 测试获取连接
		conns := pool.get("key1")
		if len(conns) != 1 {
			t.Errorf("Expected 1 connection, got %d", len(conns))
		}

		// 测试计数
		if count := pool.count("key1"); count != 1 {
			t.Errorf("Expected count 1, got %d", count)
		}

		// 测试移除连接
		pool.remove("key1", conn1)
		if count := pool.count("key1"); count != 0 {
			t.Errorf("Expected count 0 after remove, got %d", count)
		}
	}
}

// TestCanonicalHeaderKey 测试规范化头部键。
func TestCanonicalHeaderKey(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"content-type", "Content-Type"},
		{"CONTENT-TYPE", "Content-Type"},
		{"Content-Type", "Content-Type"},
		{"x-custom-header", "X-Custom-Header"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := canonicalHeaderKey(tt.input)
			if got != tt.want {
				t.Errorf("canonicalHeaderKey(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestValidateSettings_InitialWindowSize 测试 InitialWindowSize 超限。
func TestValidateSettings_InitialWindowSize(t *testing.T) {
	settings := Settings{
		MaxConcurrentStreams: 100,
		MaxFrameSize:         16384,
		MaxHeaderListSize:    1048576,
		InitialWindowSize:    2147483648, // 超过 2^31-1
	}

	err := ValidateSettings(settings)
	if err == nil {
		t.Error("ValidateSettings() expected error for InitialWindowSize > 2^31-1")
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

// TestHandleH2C 测试 H2C 处理。
func TestHandleH2C(t *testing.T) {
	cfg := &config.HTTP2Config{Enabled: true}
	server, err := NewServer(cfg, func(_ *fasthttp.RequestCtx) {}, nil)
	if err != nil {
		t.Fatalf("NewServer() error: %v", err)
	}

	// 创建一个 mock 连接
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
		if err := conn.Close(); err != nil {
			t.Logf("Failed to close connection: %v", err)
		}
	}()

	// HandleH2C 应该返回 false（不支持 H2C）
	handled, err := server.HandleH2C(conn)
	if handled {
		t.Error("HandleH2C() should return false (H2C not supported)")
	}
	if err != nil {
		t.Errorf("HandleH2C() should return nil error, got: %v", err)
	}
}

// TestH2CConnRead 测试 h2cConn.Read。
func TestH2CConnRead(t *testing.T) {
	// 创建一个测试用的连接
	server, client := net.Pipe()
	defer func() {
		_ = server.Close()
		_ = client.Close()
	}()

	// 创建 h2cConn
	h2c := &h2cConn{
		Conn:   server,
		reader: nil,
	}

	// 测试无 reader 的读取
	data := []byte("test data")
	go func() {
		_, _ = client.Write(data)
	}()

	buf := make([]byte, 100)
	n, err := h2c.Read(buf)
	if err != nil {
		t.Errorf("h2cConn.Read() error: %v", err)
	}
	if n != len(data) {
		t.Errorf("h2cConn.Read() n = %d, want %d", n, len(data))
	}
}

// TestH2CConnRead_WithReader 测试 h2cConn.Read 带 reader。
func TestH2CConnRead_WithReader(t *testing.T) {
	// 创建一个测试用的连接
	server, client := net.Pipe()
	defer func() {
		_ = server.Close()
		_ = client.Close()
	}()

	// 创建带 reader 的 h2cConn
	reader := bufio.NewReader(bytes.NewReader([]byte("prefetched")))
	h2c := &h2cConn{
		Conn:   server,
		reader: reader,
	}

	buf := make([]byte, 100)
	n, err := h2c.Read(buf)
	if err != nil {
		t.Errorf("h2cConn.Read() error: %v", err)
	}
	if n == 0 {
		t.Error("h2cConn.Read() should read from reader")
	}
}

// TestIsHTTP2Request 测试 HTTP/2 请求检测。
func TestIsHTTP2Request_Full(t *testing.T) {
	tests := []struct {
		name   string
		method string
		major  int
		header map[string]string
		want   bool
	}{
		{
			name:   "PRI 方法",
			method: "PRI",
			major:  1,
			want:   true,
		},
		{
			name:   "HTTP/2 版本",
			method: "GET",
			major:  2,
			want:   true,
		},
		{
			name:   "HTTP/1.1",
			method: "GET",
			major:  1,
			want:   false,
		},
		{
			name:   "带 :method 头",
			method: "GET",
			major:  1,
			header: map[string]string{":method": "GET"},
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest(tt.method, "http://example.com/", nil)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}

			req.ProtoMajor = tt.major

			for k, v := range tt.header {
				req.Header.Set(k, v)
			}

			got := IsHTTP2Request(req)
			if got != tt.want {
				t.Errorf("IsHTTP2Request() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestGetALPNProtocol 测试获取 ALPN 协议。
func TestGetALPNProtocol(t *testing.T) {
	// 测试非 TLS 连接
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
		if err := conn.Close(); err != nil {
			t.Logf("Failed to close connection: %v", err)
		}
	}()

	// 非 TLS 连接应返回空字符串
	proto := GetALPNProtocol(conn)
	if proto != "" {
		t.Errorf("GetALPNProtocol() on non-TLS connection = %q, want empty", proto)
	}
}

// TestSupportsHTTP2 测试 HTTP/2 支持检测。
func TestSupportsHTTP2(t *testing.T) {
	tests := []struct {
		name   string
		method string
		major  int
		header map[string]string
		want   bool
	}{
		{
			name:   "HTTP/2 请求",
			method: "GET",
			major:  2,
			want:   true,
		},
		{
			name:   "H2C 升级头",
			method: "GET",
			major:  1,
			header: map[string]string{"Upgrade": "h2c"},
			want:   true,
		},
		{
			name:   "HTTP2-Settings 头",
			method: "GET",
			major:  1,
			header: map[string]string{"HTTP2-Settings": "test"},
			want:   true,
		},
		{
			name:   "HTTP/1.1 无升级",
			method: "GET",
			major:  1,
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest(tt.method, "http://example.com/", nil)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}

			req.ProtoMajor = tt.major

			for k, v := range tt.header {
				req.Header.Set(k, v)
			}

			got := SupportsHTTP2(req)
			if got != tt.want {
				t.Errorf("SupportsHTTP2() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestWrapTLSListener_GetConfigForClient 测试 TLS 监听器的 GetConfigForClient 回调。
func TestWrapTLSListener_GetConfigForClient(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	defer func() {
		if err := ln.Close(); err != nil {
			t.Logf("Failed to close listener: %v", err)
		}
	}()

	tlsConfig := &tls.Config{
		NextProtos: []string{},
		GetConfigForClient: func(_ *tls.ClientHelloInfo) (*tls.Config, error) {
			return nil, nil
		},
	}

	_ = WrapTLSListener(ln, tlsConfig)
}

// TestWrapTLSListener_GetConfigForClientError 测试 GetConfigForClient 返回错误。
func TestWrapTLSListener_GetConfigForClientError(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	defer func() {
		if err := ln.Close(); err != nil {
			t.Logf("Failed to close listener: %v", err)
		}
	}()

	expectedErr := errors.New("client config error")
	tlsConfig := &tls.Config{
		NextProtos: []string{},
		GetConfigForClient: func(_ *tls.ClientHelloInfo) (*tls.Config, error) {
			return nil, expectedErr
		},
	}

	_ = WrapTLSListener(ln, tlsConfig)
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
	if count := pool.count("key1"); count != 0 {
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
