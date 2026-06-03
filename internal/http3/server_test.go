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

// TestStart_AlreadyRunning 测试启动已运行的服务器


// TestStart_InvalidListenAddress 测试无效监听地址



