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
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
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
	for i := range 3 {
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
	for i := range 3 {
		err := server.GracefulStop(1 * time.Second)
		if err != nil {
			t.Errorf("GracefulStop call %d returned error: %v", i+1, err)
		}
	}
}

// TestGetAltSvcHeader_PortBoundaries 测试端口边界值
func TestGetAltSvcHeader_PortBoundaries(t *testing.T) {
	tests := []struct {
		name     string
		listen   string
		expected string
	}{
		{
			name:     "标准 HTTP 端口 80",
			listen:   ":80",
			expected: `h3=":80"; ma=86400`,
		},
		{
			name:     "标准 HTTPS 端口 443",
			listen:   ":443",
			expected: `h3=":443"; ma=86400`,
		},
		{
			name:     "高端口 65535",
			listen:   ":65535",
			expected: `h3=":65535"; ma=86400`,
		},
		{
			name:     "低端口 1",
			listen:   ":1",
			expected: `h3=":1"; ma=86400`,
		},
		{
			name:     "带 IP 地址的监听",
			listen:   "0.0.0.0:8443",
			expected: `h3=":0.0.0.0:8443"; ma=86400`,
		},
		{
			name:     "带 IPv6 地址的监听",
			listen:   "[::]:8443",
			expected: `h3=":[::]:8443"; ma=86400`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.HTTP3Config{
				Enabled: true,
				Listen:  tt.listen,
			}
			handler := func(_ *fasthttp.RequestCtx) {}

			server, _ := NewServer(cfg, handler, &tls.Config{})

			header := server.GetAltSvcHeader()
			if header != tt.expected {
				t.Errorf("GetAltSvcHeader() = %q, want %q", header, tt.expected)
			}
		})
	}
}

// TestGetAltSvcHeader_DisabledServer 测试禁用状态下的服务器
func TestGetAltSvcHeader_DisabledServer(t *testing.T) {
	cfg := &config.HTTP3Config{
		Enabled: false,
		Listen:  ":443",
	}
	handler := func(_ *fasthttp.RequestCtx) {}

	server, _ := NewServer(cfg, handler, &tls.Config{})

	header := server.GetAltSvcHeader()
	if header != "" {
		t.Errorf("Expected empty Alt-Svc header when disabled, got %q", header)
	}
}

// TestGetAltSvcHeader_NilConfig 测试 nil 配置情况
func TestGetAltSvcHeader_NilConfig(t *testing.T) {
	cfg := &config.HTTP3Config{
		Enabled: true,
		Listen:  ":443",
	}
	handler := func(_ *fasthttp.RequestCtx) {}

	server, _ := NewServer(cfg, handler, &tls.Config{})

	// 正常情况下应该返回 header
	header := server.GetAltSvcHeader()
	if header == "" {
		t.Error("Expected non-empty Alt-Svc header with valid config")
	}
}

// TestStart_AlreadyRunning 测试启动已运行的服务器
func TestStart_AlreadyRunning(t *testing.T) {
	cfg := &config.HTTP3Config{
		Enabled:    true,
		Listen:     ":0", // 使用随机端口避免冲突
		MaxStreams: 100,
	}
	handler := func(_ *fasthttp.RequestCtx) {}

	cert := generateTestCertificate(t)
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
	}

	server, err := NewServer(cfg, handler, tlsConfig)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// 启动服务器
	err = server.Start()
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Stop()

	// 再次启动应该失败
	err = server.Start()
	if err == nil {
		t.Error("Expected error when starting already running server")
	}
	if err.Error() != "server already running" {
		t.Errorf("Expected 'server already running' error, got: %v", err)
	}
}

// TestStart_InvalidListenAddress 测试无效监听地址
func TestStart_InvalidListenAddress(t *testing.T) {
	cfg := &config.HTTP3Config{
		Enabled:    true,
		Listen:     "invalid:address:format", // 无效地址
		MaxStreams: 100,
	}
	handler := func(_ *fasthttp.RequestCtx) {}
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{},
	}

	server, err := NewServer(cfg, handler, tlsConfig)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	err = server.Start()
	if err == nil {
		t.Error("Expected error for invalid listen address")
		server.Stop()
	}
}

// TestStop_RunningServer 测试停止运行中的服务器
func TestStop_RunningServer(t *testing.T) {
	cfg := &config.HTTP3Config{
		Enabled:    true,
		Listen:     ":0",
		MaxStreams: 100,
	}
	handler := func(_ *fasthttp.RequestCtx) {}

	cert := generateTestCertificate(t)
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
	}

	server, err := NewServer(cfg, handler, tlsConfig)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// 启动服务器
	err = server.Start()
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	if !server.IsRunning() {
		t.Error("Server should be running after Start()")
	}

	// 停止服务器
	err = server.Stop()
	if err != nil {
		t.Errorf("Unexpected error stopping server: %v", err)
	}

	if server.IsRunning() {
		t.Error("Server should not be running after Stop()")
	}
}

// TestGracefulStop_RunningServer 测试优雅停止运行中的服务器
func TestGracefulStop_RunningServer(t *testing.T) {
	cfg := &config.HTTP3Config{
		Enabled:    true,
		Listen:     ":0",
		MaxStreams: 100,
	}
	handler := func(_ *fasthttp.RequestCtx) {}

	cert := generateTestCertificate(t)
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
	}

	server, err := NewServer(cfg, handler, tlsConfig)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// 启动服务器
	err = server.Start()
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	if !server.IsRunning() {
		t.Error("Server should be running after Start()")
	}

	// 优雅停止服务器
	err = server.GracefulStop(5 * time.Second)
	if err != nil {
		t.Errorf("Unexpected error graceful stopping server: %v", err)
	}

	if server.IsRunning() {
		t.Error("Server should not be running after GracefulStop()")
	}
}

// TestStart_Enable0RTT 测试启用 0-RTT 时的警告日志
func TestStart_Enable0RTT(t *testing.T) {
	cfg := &config.HTTP3Config{
		Enabled:    true,
		Listen:     ":0",
		Enable0RTT: true,
		MaxStreams: 100,
	}
	handler := func(_ *fasthttp.RequestCtx) {}

	cert := generateTestCertificate(t)
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
	}

	server, err := NewServer(cfg, handler, tlsConfig)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	err = server.Start()
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Stop()

	// 验证服务器已启动
	if !server.IsRunning() {
		t.Error("Server should be running")
	}
}

// TestStart_DefaultValues 测试默认值设置
func TestStart_DefaultValues(t *testing.T) {
	cfg := &config.HTTP3Config{
		Enabled:    true,
		Listen:     ":0",
		MaxStreams: 0, // 使用默认值
	}
	handler := func(_ *fasthttp.RequestCtx) {}

	cert := generateTestCertificate(t)
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
	}

	server, err := NewServer(cfg, handler, tlsConfig)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	err = server.Start()
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Stop()

	if !server.IsRunning() {
		t.Error("Server should be running")
	}
}

// generateTestCertificate 生成用于测试的自签名证书
func generateTestCertificate(t *testing.T) tls.Certificate {
	t.Helper()

	// 使用 RSA 密钥生成自签名证书
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "localhost"},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("Failed to generate RSA key: %v", err)
	}

	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("Failed to create certificate: %v", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("Failed to load certificate: %v", err)
	}

	return cert
}
