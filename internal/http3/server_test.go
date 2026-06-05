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
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/config"
)

// newTestTLSConfig 创建用于测试的自签名 TLS 证书
func newTestTLSConfig(t *testing.T) *tls.Config {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Test"},
		},
		NotBefore:   time.Now(),
		NotAfter:    time.Now().Add(time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)

	cert := &tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  key,
	}

	return &tls.Config{
		Certificates: []tls.Certificate{*cert},
	}
}

// newTestHandler 创建测试用的 fasthttp handler
func newTestHandler() fasthttp.RequestHandler {
	return func(_ *fasthttp.RequestCtx) {}
}

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

	if server.running.Load() {
		t.Error("Server should not be running initially")
	}
}

// TestNewServer_TableDriven 使用表驱动测试各种配置组合
func TestNewServer_TableDriven(t *testing.T) {
	handler := newTestHandler()
	tlsConfig := newTestTLSConfig(t)

	tests := []struct {
		name      string
		cfg       *config.HTTP3Config
		handler   fasthttp.RequestHandler
		tlsConfig *tls.Config
		wantErr   bool
		errMsg    string
	}{
		{
			name:      "空配置",
			cfg:       nil,
			handler:   handler,
			tlsConfig: tlsConfig,
			wantErr:   true,
			errMsg:    "http3 config is nil",
		},
		{
			name: "空handler",
			cfg: &config.HTTP3Config{
				Enabled: true,
				Listen:  ":443",
			},
			handler:   nil,
			tlsConfig: tlsConfig,
			wantErr:   true,
			errMsg:    "handler is nil",
		},
		{
			name: "空TLS配置",
			cfg: &config.HTTP3Config{
				Enabled: true,
				Listen:  ":443",
			},
			handler:   handler,
			tlsConfig: nil,
			wantErr:   true,
			errMsg:    "tls config is required for HTTP/3",
		},
		{
			name: "完整配置",
			cfg: &config.HTTP3Config{
				Enabled:     true,
				Listen:      ":443",
				MaxStreams:  200,
				IdleTimeout: 30 * time.Second,
				Enable0RTT:  true,
			},
			handler:   handler,
			tlsConfig: tlsConfig,
			wantErr:   false,
		},
		{
			name: "最小配置",
			cfg: &config.HTTP3Config{
				Enabled: true,
			},
			handler:   handler,
			tlsConfig: tlsConfig,
			wantErr:   false,
		},
		{
			name: "禁用状态",
			cfg: &config.HTTP3Config{
				Enabled: false,
			},
			handler:   handler,
			tlsConfig: tlsConfig,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, err := NewServer(tt.cfg, tt.handler, tt.tlsConfig)
			if tt.wantErr {
				require.Error(t, err)
				assert.Equal(t, tt.errMsg, err.Error())
				assert.Nil(t, server)
			} else {
				require.NoError(t, err)
				require.NotNil(t, server)
				assert.Equal(t, tt.cfg, server.config)
				assert.NotNil(t, server.handler)
				assert.NotNil(t, server.adapter)
				assert.Equal(t, tt.tlsConfig, server.tlsConfig)
				assert.False(t, server.running.Load())
				assert.Nil(t, server.listener)
				assert.Nil(t, server.http3Server)
			}
		})
	}
}

// TestNewServer_VerifyInternalFields 验证创建后内部字段的值
func TestNewServer_VerifyInternalFields(t *testing.T) {
	cfg := &config.HTTP3Config{
		Enabled:     true,
		Listen:      ":8443",
		MaxStreams:  256,
		IdleTimeout: 60 * time.Second,
		Enable0RTT:  true,
	}
	handler := newTestHandler()
	tlsConfig := newTestTLSConfig(t)

	server, err := NewServer(cfg, handler, tlsConfig)
	require.NoError(t, err)
	require.NotNil(t, server)

	assert.Equal(t, cfg, server.config)
	assert.Equal(t, tlsConfig, server.tlsConfig)
	assert.NotNil(t, server.adapter)
	assert.False(t, server.running.Load())
	assert.Nil(t, server.listener)
	assert.Nil(t, server.http3Server)
}

// TestStart_AlreadyRunning 测试启动已运行的服务器
func TestStart_AlreadyRunning(t *testing.T) {
	cfg := &config.HTTP3Config{
		Enabled: true,
		Listen:  ":0",
	}
	handler := newTestHandler()
	tlsConfig := newTestTLSConfig(t)

	server, err := NewServer(cfg, handler, tlsConfig)
	require.NoError(t, err)

	require.NoError(t, server.Start())
	t.Cleanup(func() { _ = server.Stop() })

	err = server.Start()
	require.Error(t, err)
	assert.Equal(t, "server already running", err.Error())
}

// TestStart_InvalidListenAddress 测试无效监听地址
func TestStart_InvalidListenAddress(t *testing.T) {
	tests := []struct {
		name    string
		listen  string
		wantErr bool
	}{
		{
			name:    "无效地址格式",
			listen:  "not-a-valid-address:999999999",
			wantErr: true,
		},
		{
			name:    "无效端口",
			listen:  ":999999999",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.HTTP3Config{
				Enabled: true,
				Listen:  tt.listen,
			}
			handler := newTestHandler()
			tlsConfig := newTestTLSConfig(t)

			server, err := NewServer(cfg, handler, tlsConfig)
			require.NoError(t, err)

			err = server.Start()
			if tt.wantErr {
				require.Error(t, err)
			} else {
				assert.NoError(t, err)
				_ = server.Stop()
			}
		})
	}
}

// TestStart_Success 测试成功启动服务器（使用随机端口）
func TestStart_Success(t *testing.T) {
	cfg := &config.HTTP3Config{
		Enabled:    true,
		Listen:     ":0",
		MaxStreams: 100,
	}
	handler := newTestHandler()
	tlsConfig := newTestTLSConfig(t)

	server, err := NewServer(cfg, handler, tlsConfig)
	require.NoError(t, err)

	require.NoError(t, server.Start())
	t.Cleanup(func() { _ = server.Stop() })

	assert.True(t, server.running.Load())
	assert.NotNil(t, server.listener)
	assert.NotNil(t, server.http3Server)
}

// TestStart_EmptyListenAddress 测试空监听地址时使用默认值
func TestStart_EmptyListenAddress(t *testing.T) {
	cfg := &config.HTTP3Config{
		Enabled: true,
		Listen:  "",
	}
	handler := newTestHandler()
	tlsConfig := newTestTLSConfig(t)

	server, err := NewServer(cfg, handler, tlsConfig)
	require.NoError(t, err)

	// 默认地址是 :443，可能需要权限，这里只验证逻辑不会 panic
	// 如果绑定失败是因为权限，则跳过
	err = server.Start()
	if err != nil {
		t.Skipf("无法绑定默认端口 :443（可能需要权限）: %v", err)
	}
	t.Cleanup(func() { _ = server.Stop() })
}

// TestStart_QUICConfigDefaults 测试 QUIC 配置默认值
func TestStart_QUICConfigDefaults(t *testing.T) {
	tests := []struct {
		name       string
		maxStreams int
		idle       time.Duration
		enable0RTT bool
	}{
		{
			name:       "零值使用默认 MaxStreams",
			maxStreams: 0,
			idle:       0,
			enable0RTT: false,
		},
		{
			name:       "自定义 MaxStreams",
			maxStreams: 500,
			idle:       60 * time.Second,
			enable0RTT: true,
		},
		{
			name:       "最小 MaxStreams",
			maxStreams: 1,
			idle:       5 * time.Second,
			enable0RTT: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.HTTP3Config{
				Enabled:     true,
				Listen:      ":0",
				MaxStreams:  tt.maxStreams,
				IdleTimeout: tt.idle,
				Enable0RTT:  tt.enable0RTT,
			}
			handler := newTestHandler()
			tlsConfig := newTestTLSConfig(t)

			server, err := NewServer(cfg, handler, tlsConfig)
			require.NoError(t, err)

			require.NoError(t, server.Start())
			t.Cleanup(func() { _ = server.Stop() })

			assert.True(t, server.running.Load())
			assert.NotNil(t, server.listener)
		})
	}
}

// TestStart_MultipleStartsAndStops 测试多次启停
func TestStart_MultipleStartsAndStops(t *testing.T) {
	cfg := &config.HTTP3Config{
		Enabled: true,
		Listen:  ":0",
	}
	handler := newTestHandler()
	tlsConfig := newTestTLSConfig(t)

	server, err := NewServer(cfg, handler, tlsConfig)
	require.NoError(t, err)

	// 第一次启动
	require.NoError(t, server.Start())
	assert.True(t, server.running.Load())

	// 停止
	require.NoError(t, server.Stop())
	assert.False(t, server.running.Load())

	// 第二次启动（使用新端口，因为旧端口可能还在释放中）
	server.config.Listen = ":0"
	require.NoError(t, server.Start())
	t.Cleanup(func() { _ = server.Stop() })
	assert.True(t, server.running.Load())
}

// TestStop_NotRunning 测试停止未运行的服务器
func TestStop_NotRunning(t *testing.T) {
	cfg := &config.HTTP3Config{
		Enabled: true,
		Listen:  ":0",
	}
	handler := newTestHandler()
	tlsConfig := newTestTLSConfig(t)

	server, err := NewServer(cfg, handler, tlsConfig)
	require.NoError(t, err)

	assert.False(t, server.running.Load())

	err = server.Stop()
	require.NoError(t, err)
	assert.False(t, server.running.Load())
}

// TestStop_Running 测试停止正在运行的服务器
func TestStop_Running(t *testing.T) {
	cfg := &config.HTTP3Config{
		Enabled: true,
		Listen:  ":0",
	}
	handler := newTestHandler()
	tlsConfig := newTestTLSConfig(t)

	server, err := NewServer(cfg, handler, tlsConfig)
	require.NoError(t, err)

	require.NoError(t, server.Start())
	assert.True(t, server.running.Load())

	require.NoError(t, server.Stop())
	assert.False(t, server.running.Load())
}

// TestStop_CalledMultipleTimes 测试多次停止不会报错
func TestStop_CalledMultipleTimes(t *testing.T) {
	cfg := &config.HTTP3Config{
		Enabled: true,
		Listen:  ":0",
	}
	handler := newTestHandler()
	tlsConfig := newTestTLSConfig(t)

	server, err := NewServer(cfg, handler, tlsConfig)
	require.NoError(t, err)

	require.NoError(t, server.Start())

	require.NoError(t, server.Stop())
	require.NoError(t, server.Stop())
	require.NoError(t, server.Stop())

	assert.False(t, server.running.Load())
}

// TestStartStop_Lifecycle 测试完整的生命周期
func TestStartStop_Lifecycle(t *testing.T) {
	cfg := &config.HTTP3Config{
		Enabled:     true,
		Listen:      ":0",
		MaxStreams:  50,
		IdleTimeout: 30 * time.Second,
		Enable0RTT:  false,
	}
	handler := newTestHandler()
	tlsConfig := newTestTLSConfig(t)

	server, err := NewServer(cfg, handler, tlsConfig)
	require.NoError(t, err)

	assert.False(t, server.running.Load())
	assert.Nil(t, server.listener)
	assert.Nil(t, server.http3Server)

	require.NoError(t, server.Start())

	assert.True(t, server.running.Load())
	assert.NotNil(t, server.listener)
	assert.NotNil(t, server.http3Server)

	require.NoError(t, server.Stop())

	assert.False(t, server.running.Load())
}
