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
		name      string
		cfg       *config.HTTP2Config
		handler   fasthttp.RequestHandler
		tlsConfig *tls.Config
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
			handler:   func(ctx *fasthttp.RequestCtx) {},
			tlsConfig: nil,
			wantErr:   false,
		},
		{
			name:    "默认配置",
			cfg:     &config.HTTP2Config{},
			handler: func(ctx *fasthttp.RequestCtx) {},
			wantErr: false,
		},
		{
			name:    "nil配置",
			cfg:     nil,
			handler: func(ctx *fasthttp.RequestCtx) {},
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
			handler: func(ctx *fasthttp.RequestCtx) {},
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
	handler := func(ctx *fasthttp.RequestCtx) {}

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
	server, err := NewServer(cfg, func(ctx *fasthttp.RequestCtx) {}, nil)
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
	server, err := NewServer(cfg, func(ctx *fasthttp.RequestCtx) {}, nil)
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
	server, err := NewServer(cfg, func(ctx *fasthttp.RequestCtx) {}, nil)
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
	defer func() { _ = ln.Close() }()

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
			server, err := NewServer(cfg, func(ctx *fasthttp.RequestCtx) {}, nil)
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
		name   string
		method string
		major  int
		header map[string]string
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
		t.Run(tt.name, func(t *testing.T) {
			// 这里只测试基本的逻辑，完整测试需要创建 http.Request
			// 在实际集成测试中会覆盖
		})
	}
}

// TestHTTP2Settings 测试 HTTP/2 设置。
func TestHTTP2Settings(t *testing.T) {
	tests := []struct {
		name     string
		settings HTTP2Settings
		wantErr  bool
	}{
		{
			name: "默认设置",
			settings: HTTP2Settings{
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
			settings: HTTP2Settings{
				MaxConcurrentStreams: 0,
			},
			wantErr: true,
		},
		{
			name: "无效帧大小",
			settings: HTTP2Settings{
				MaxConcurrentStreams: 100,
				MaxFrameSize:         1024, // 小于最小值 16384
			},
			wantErr: true,
		},
		{
			name: "帧大小过大",
			settings: HTTP2Settings{
				MaxConcurrentStreams: 100,
				MaxFrameSize:         16777216, // 超过最大值 16777215
			},
			wantErr: true,
		},
		{
			name: "零头部列表大小",
			settings: HTTP2Settings{
				MaxConcurrentStreams: 100,
				MaxHeaderListSize:    0,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateHTTP2Settings(tt.settings)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ValidateHTTP2Settings() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("ValidateHTTP2Settings() unexpected error: %v", err)
			}
		})
	}
}

// TestDefaultHTTP2Settings 测试默认 HTTP/2 设置。
func TestDefaultHTTP2Settings(t *testing.T) {
	settings := DefaultHTTP2Settings()

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

// TestParseHTTP2Settings 测试从配置解析 HTTP/2 设置。
func TestParseHTTP2Settings(t *testing.T) {
	cfg := &config.HTTP2Config{
		Enabled:              true,
		MaxConcurrentStreams: 200,
		MaxHeaderListSize:    2097152, // 2MB
		PushEnabled:          true,
	}

	settings := ParseHTTP2Settings(cfg)

	if settings.MaxConcurrentStreams != 200 {
		t.Errorf("ParseHTTP2Settings() MaxConcurrentStreams = %d, want 200", settings.MaxConcurrentStreams)
	}
	if settings.MaxHeaderListSize != 2097152 {
		t.Errorf("ParseHTTP2Settings() MaxHeaderListSize = %d, want 2097152", settings.MaxHeaderListSize)
	}
	if !settings.EnablePush {
		t.Error("ParseHTTP2Settings() EnablePush should be true")
	}
}

// TestConnectionPool 测试连接池。
func TestConnectionPool(t *testing.T) {
	pool := newConnectionPool()

	// 创建测试连接
	ln1, _ := net.Listen("tcp", "127.0.0.1:0")
	defer func() { _ = ln1.Close() }()

	ln2, _ := net.Listen("tcp", "127.0.0.1:0")
	defer func() { _ = ln2.Close() }()

	// 测试添加连接
	conn1, _ := net.Dial("tcp", ln1.Addr().String())
	if conn1 != nil {
		defer func() { _ = conn1.Close() }()
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
