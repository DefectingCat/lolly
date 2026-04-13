// Package http3 提供 HTTP/3 服务器功能的测试。
//
// 该文件测试 HTTP/3 服务器模块的各项功能，包括：
//   - 服务器创建和配置验证
//   - Alt-Svc 头部生成
//   - 服务器统计信息获取
//   - 运行状态检查
//   - 服务器停止和优雅停止
//
// 作者：xfy
package http3

import (
	"crypto/tls"
	"testing"
	"time"

	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/config"
)

// TestNewServer_NilConfig 测试空配置错误
func TestNewServer_NilConfig(t *testing.T) {
	handler := func(_ *fasthttp.RequestCtx) {}

	server, err := NewServer(nil, handler, &tls.Config{})

	if err == nil {
		t.Error("Expected error for nil config")
	}
	if server != nil {
		t.Error("Expected nil server for nil config")
	}
	if err.Error() != "http3 config is nil" {
		t.Errorf("Expected error message 'http3 config is nil', got: %v", err)
	}
}

// TestNewServer_NilHandler 测试空 handler 错误
func TestNewServer_NilHandler(t *testing.T) {
	cfg := &config.HTTP3Config{
		Enabled:    true,
		Listen:     ":443",
		Enable0RTT: true,
	}

	server, err := NewServer(cfg, nil, &tls.Config{})

	if err == nil {
		t.Error("Expected error for nil handler")
	}
	if server != nil {
		t.Error("Expected nil server for nil handler")
	}
	if err.Error() != "handler is nil" {
		t.Errorf("Expected error message 'handler is nil', got: %v", err)
	}
}

// TestNewServer_NilTLS 测试空 TLS 配置错误
func TestNewServer_NilTLS(t *testing.T) {
	cfg := &config.HTTP3Config{
		Enabled:    true,
		Listen:     ":443",
		Enable0RTT: true,
	}
	handler := func(_ *fasthttp.RequestCtx) {}

	server, err := NewServer(cfg, handler, nil)

	if err == nil {
		t.Error("Expected error for nil TLS config")
	}
	if server != nil {
		t.Error("Expected nil server for nil TLS config")
	}
	if err.Error() != "tls config is required for HTTP/3" {
		t.Errorf("Expected error message 'tls config is required for HTTP/3', got: %v", err)
	}
}

// TestNewServer_Success 测试成功创建服务器
func TestNewServer_Success(t *testing.T) {
	cfg := &config.HTTP3Config{
		Enabled:    true,
		Listen:     ":443",
		Enable0RTT: true,
		MaxStreams: 100,
	}
	handler := func(_ *fasthttp.RequestCtx) {}
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{},
	}

	server, err := NewServer(cfg, handler, tlsConfig)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if server == nil {
		t.Fatal("Expected non-nil server")
	}

	if server.config != cfg {
		t.Error("Config not set correctly")
	}

	if server.handler == nil {
		t.Error("Handler not set correctly")
	}

	if server.adapter == nil {
		t.Error("Adapter not initialized")
	}

	if server.tlsConfig != tlsConfig {
		t.Error("TLS config not set correctly")
	}

	if server.running {
		t.Error("Server should not be running initially")
	}
}

// TestGetAltSvcHeader_DefaultPort 测试默认端口 Alt-Svc 头
func TestGetAltSvcHeader_DefaultPort(t *testing.T) {
	cfg := &config.HTTP3Config{
		Enabled: true,
		Listen:  ":443",
	}
	handler := func(_ *fasthttp.RequestCtx) {}

	server, _ := NewServer(cfg, handler, &tls.Config{})

	header := server.GetAltSvcHeader()

	expected := `h3=":443"; ma=86400`
	if header != expected {
		t.Errorf("Expected Alt-Svc header '%s', got '%s'", expected, header)
	}
}

// TestGetAltSvcHeader_CustomPort 测试自定义端口 Alt-Svc 头
func TestGetAltSvcHeader_CustomPort(t *testing.T) {
	cfg := &config.HTTP3Config{
		Enabled: true,
		Listen:  ":8443",
	}
	handler := func(_ *fasthttp.RequestCtx) {}

	server, _ := NewServer(cfg, handler, &tls.Config{})

	header := server.GetAltSvcHeader()

	expected := `h3=":8443"; ma=86400`
	if header != expected {
		t.Errorf("Expected Alt-Svc header '%s', got '%s'", expected, header)
	}
}

// TestGetAltSvcHeader_Disabled 测试禁用 HTTP/3 时返回空
func TestGetAltSvcHeader_Disabled(t *testing.T) {
	cfg := &config.HTTP3Config{
		Enabled: false,
		Listen:  ":443",
	}
	handler := func(_ *fasthttp.RequestCtx) {}

	server, _ := NewServer(cfg, handler, &tls.Config{})

	header := server.GetAltSvcHeader()

	if header != "" {
		t.Errorf("Expected empty Alt-Svc header when disabled, got '%s'", header)
	}
}

// TestGetAltSvcHeader_EmptyListen 测试空监听地址时使用默认端口
func TestGetAltSvcHeader_EmptyListen(t *testing.T) {
	cfg := &config.HTTP3Config{
		Enabled: true,
		Listen:  "", // 空，使用默认 :443
	}
	handler := func(_ *fasthttp.RequestCtx) {}

	server, _ := NewServer(cfg, handler, &tls.Config{})

	header := server.GetAltSvcHeader()

	expected := `h3=":443"; ma=86400`
	if header != expected {
		t.Errorf("Expected Alt-Svc header '%s', got '%s'", expected, header)
	}
}

// TestGetStats 测试获取统计信息
func TestGetStats(t *testing.T) {
	cfg := &config.HTTP3Config{
		Enabled:    true,
		Listen:     ":8443",
		Enable0RTT: true,
		MaxStreams: 200,
	}
	handler := func(_ *fasthttp.RequestCtx) {}

	server, _ := NewServer(cfg, handler, &tls.Config{})

	stats := server.GetStats()

	if stats.Running {
		t.Error("Expected Running to be false initially")
	}

	if stats.Listen != ":8443" {
		t.Errorf("Expected Listen ':8443', got '%s'", stats.Listen)
	}

	if !stats.Enable0RTT {
		t.Error("Expected Enable0RTT to be true")
	}

	if stats.MaxStreams != 200 {
		t.Errorf("Expected MaxStreams 200, got %d", stats.MaxStreams)
	}
}

// TestIsRunning 测试运行状态检查
func TestIsRunning(t *testing.T) {
	cfg := &config.HTTP3Config{
		Enabled: true,
		Listen:  ":443",
	}
	handler := func(_ *fasthttp.RequestCtx) {}

	server, _ := NewServer(cfg, handler, &tls.Config{})

	// 初始状态应该是 false
	if server.IsRunning() {
		t.Error("Expected IsRunning to be false initially")
	}

	// 手动设置运行状态（不启动真实服务器）
	server.mu.Lock()
	server.running = true
	server.mu.Unlock()

	if !server.IsRunning() {
		t.Error("Expected IsRunning to be true after setting")
	}

	// 再次设置为 false
	server.mu.Lock()
	server.running = false
	server.mu.Unlock()

	if server.IsRunning() {
		t.Error("Expected IsRunning to be false after unsetting")
	}
}

// TestStop_NotRunning 测试停止未运行的服务器
func TestStop_NotRunning(t *testing.T) {
	cfg := &config.HTTP3Config{
		Enabled: true,
		Listen:  ":443",
	}
	handler := func(_ *fasthttp.RequestCtx) {}

	server, _ := NewServer(cfg, handler, &tls.Config{})

	// 服务器未启动，Stop 应该返回 nil
	err := server.Stop()
	if err != nil {
		t.Errorf("Expected nil error when stopping non-running server, got: %v", err)
	}
}

// TestGracefulStop_NotRunning 测试优雅停止未运行的服务器
func TestGracefulStop_NotRunning(t *testing.T) {
	cfg := &config.HTTP3Config{
		Enabled: true,
		Listen:  ":443",
	}
	handler := func(_ *fasthttp.RequestCtx) {}

	server, _ := NewServer(cfg, handler, &tls.Config{})

	// 服务器未启动，GracefulStop 应该返回 nil
	err := server.GracefulStop(5 * time.Second)
	if err != nil {
		t.Errorf("Expected nil error when graceful stopping non-running server, got: %v", err)
	}
}

// TestGracefulStop_WithTimeout 测试优雅停止超时
func TestGracefulStop_WithTimeout(t *testing.T) {
	cfg := &config.HTTP3Config{
		Enabled: true,
		Listen:  ":443",
	}
	handler := func(_ *fasthttp.RequestCtx) {}

	server, _ := NewServer(cfg, handler, &tls.Config{})

	// 测试不同超时值
	tests := []struct {
		name    string
		timeout time.Duration
	}{
		{"zero timeout", 0},
		{"short timeout", 100 * time.Millisecond},
		{"long timeout", 30 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := server.GracefulStop(tt.timeout)
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

// TestServer_MultipleStop 测试多次调用 Stop
func TestServer_MultipleStop(t *testing.T) {
	cfg := &config.HTTP3Config{
		Enabled: true,
		Listen:  ":443",
	}
	handler := func(_ *fasthttp.RequestCtx) {}

	server, _ := NewServer(cfg, handler, &tls.Config{})

	// 多次调用 Stop 应该都是安全的
	for i := 0; i < 3; i++ {
		err := server.Stop()
		if err != nil {
			t.Errorf("Stop call %d returned error: %v", i+1, err)
		}
	}
}

// TestServer_MultipleGracefulStop 测试多次调用 GracefulStop
func TestServer_MultipleGracefulStop(t *testing.T) {
	cfg := &config.HTTP3Config{
		Enabled: true,
		Listen:  ":443",
	}
	handler := func(_ *fasthttp.RequestCtx) {}

	server, _ := NewServer(cfg, handler, &tls.Config{})

	// 多次调用 GracefulStop 应该都是安全的
	for i := 0; i < 3; i++ {
		err := server.GracefulStop(1 * time.Second)
		if err != nil {
			t.Errorf("GracefulStop call %d returned error: %v", i+1, err)
		}
	}
}

// TestStats_Struct 测试 Stats 结构体
func TestStats_Struct(t *testing.T) {
	stats := Stats{
		Running:    true,
		Listen:     ":443",
		Enable0RTT: true,
		MaxStreams: 100,
	}

	if !stats.Running {
		t.Error("Expected Running true")
	}
	if stats.Listen != ":443" {
		t.Errorf("Expected Listen ':443', got '%s'", stats.Listen)
	}
	if !stats.Enable0RTT {
		t.Error("Expected Enable0RTT true")
	}
	if stats.MaxStreams != 100 {
		t.Errorf("Expected MaxStreams 100, got %d", stats.MaxStreams)
	}
}
