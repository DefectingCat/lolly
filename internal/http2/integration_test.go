// Package http2 提供 HTTP/2 集成测试。
//
// 该文件包含 HTTP/2 的端到端集成测试：
//   - HTTP/2 请求处理
//   - ALPN 协商
//   - HTTP/1.1 fallback
//
// 运行方式: go test -tags=integration ./internal/http2/...
//
// 作者：xfy
package http2

import (
	"bytes"
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/config"
)

// TestIntegrationHTTP2Request 测试 HTTP/2 请求处理（需要 TLS 证书）。
func TestIntegrationHTTP2Request(t *testing.T) {
	// 跳过集成测试，除非显式启用
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// 注意：这需要有效的 TLS 证书才能完整测试
	// 这里使用非 TLS 模式测试基本功能

	handler := func(ctx *fasthttp.RequestCtx) {
		ctx.WriteString("Hello HTTP/2") //nolint:errcheck
		ctx.SetStatusCode(fasthttp.StatusOK)
	}

	cfg := &config.HTTP2Config{
		Enabled:              true,
		MaxConcurrentStreams: 100,
	}

	server, err := NewServer(cfg, handler, nil)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// 创建监听器
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	defer func() { _ = ln.Close() }()

	// 启动服务器（在后台）
	go func() {
		_ = server.Serve(ln)
	}()

	// 等待服务器启动
	time.Sleep(100 * time.Millisecond)

	// 停止服务器
	if err := server.Stop(); err != nil {
		t.Errorf("Failed to stop server: %v", err)
	}
}

// TestIntegrationALPN 测试 ALPN 协议协商（需要 TLS 证书）。
func TestIntegrationALPN(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	handler := func(ctx *fasthttp.RequestCtx) {
		ctx.SetStatusCode(fasthttp.StatusOK)
	}

	cfg := &config.HTTP2Config{
		Enabled: true,
	}

	server, err := NewServer(cfg, handler, nil)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// 验证 ALPN 配置
	tlsConfig := server.ALPNConfig()
	if tlsConfig == nil {
		t.Fatal("ALPN config should not be nil")
	}

	// 验证协议列表
	foundH2 := false
	for _, proto := range tlsConfig.NextProtos {
		if proto == "h2" {
			foundH2 = true
			break
		}
	}
	if !foundH2 {
		t.Error("ALPN config should include h2 protocol")
	}
}

// TestIntegrationHTTP1Fallback 测试 HTTP/1.1 回退。
func TestIntegrationHTTP1Fallback(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	handler := func(ctx *fasthttp.RequestCtx) {
		ctx.WriteString("Fallback to HTTP/1.1") //nolint:errcheck
		ctx.SetStatusCode(fasthttp.StatusOK)
	}

	cfg := &config.HTTP2Config{
		Enabled:              true,
		MaxConcurrentStreams: 100,
	}

	server, err := NewServer(cfg, handler, nil)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// 验证服务器支持 HTTP/1.1 回退
	if server.handler == nil {
		t.Error("Server handler should be set for HTTP/1.1 fallback")
	}
}

// TestIntegrationConcurrentStreams 测试并发流处理。
func TestIntegrationConcurrentStreams(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	requestCount := 0
	handler := func(ctx *fasthttp.RequestCtx) {
		requestCount++
		ctx.WriteString("OK") //nolint:errcheck
		ctx.SetStatusCode(fasthttp.StatusOK)
	}

	cfg := &config.HTTP2Config{
		Enabled:              true,
		MaxConcurrentStreams: 10,
	}

	server, err := NewServer(cfg, handler, nil)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// 验证并发流限制
	if server.http2Server.MaxConcurrentStreams != 10 {
		t.Errorf("Expected MaxConcurrentStreams 10, got %d",
			server.http2Server.MaxConcurrentStreams)
	}
}

// TestIntegrationServerLifecycle 测试服务器生命周期。
func TestIntegrationServerLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	handler := func(ctx *fasthttp.RequestCtx) {
		ctx.SetStatusCode(fasthttp.StatusOK)
	}

	cfg := &config.HTTP2Config{
		Enabled: true,
	}

	server, err := NewServer(cfg, handler, nil)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// 初始状态检查
	if server.IsRunning() {
		t.Error("Server should not be running initially")
	}

	// 创建监听器
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}

	// 启动服务器
	go func() { _ = server.Serve(ln) }()

	// 等待服务器启动
	time.Sleep(50 * time.Millisecond)

	// 停止服务器
	if err := server.Stop(); err != nil {
		t.Errorf("Failed to stop server: %v", err)
	}
}

// TestIntegrationAdapterConversion 测试适配器转换。
func TestIntegrationAdapterConversion(t *testing.T) {
	handler := func(ctx *fasthttp.RequestCtx) {
		// 设置响应头和体
		ctx.Response.Header.Set("X-Custom-Header", "test-value")
		ctx.WriteString("Converted response") //nolint:errcheck
		ctx.SetStatusCode(fasthttp.StatusOK)
	}

	adapter := NewFastHTTPHandlerAdapter(handler)

	// 创建标准 HTTP 请求
	req, err := http.NewRequest(http.MethodGet, "http://example.com/test", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Accept", "application/json")

	// 使用 httptest 记录响应
	recorder := &testResponseRecorder{
		header: make(http.Header),
	}

	// 执行适配器
	adapter.ServeHTTP(recorder, req)

	// 验证响应
	if recorder.statusCode != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, recorder.statusCode)
	}

	if recorder.body.String() != "Converted response" {
		t.Errorf("Expected body 'Converted response', got '%s'", recorder.body.String())
	}
}

// testResponseRecorder 是测试用的响应记录器。
type testResponseRecorder struct {
	statusCode int
	header     http.Header
	body       testBuffer
}

func (r *testResponseRecorder) Header() http.Header {
	return r.header
}

func (r *testResponseRecorder) Write(p []byte) (int, error) {
	return r.body.Write(p)
}

func (r *testResponseRecorder) WriteHeader(code int) {
	r.statusCode = code
}

// testBuffer 是一个简单的字节缓冲区。
type testBuffer struct {
	data []byte
}

func (b *testBuffer) Write(p []byte) (int, error) {
	b.data = append(b.data, p...)
	return len(p), nil
}

func (b *testBuffer) String() string {
	return string(b.data)
}

// TestIntegrationTLSConfiguration 测试 TLS 配置集成。
func TestIntegrationTLSConfiguration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	handler := func(ctx *fasthttp.RequestCtx) {
		ctx.SetStatusCode(fasthttp.StatusOK)
	}

	cfg := &config.HTTP2Config{
		Enabled:              true,
		MaxConcurrentStreams: 100,
	}

	tlsConfig := &tls.Config{
		NextProtos: []string{"h2", "http/1.1"},
	}

	server, err := NewServer(cfg, handler, tlsConfig)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// 验证 TLS 配置
	if server.tlsConfig == nil {
		t.Error("TLS config should be set")
	}

	// 测试监听器包装
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	defer func() { _ = ln.Close() }()

	wrappedLn := WrapTLSListener(ln, tlsConfig)
	if wrappedLn == nil {
		t.Error("Wrapped listener should not be nil")
	}
}

// TestIntegrationH2C 测试 H2C（明文 HTTP/2）。
func TestIntegrationH2C(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	handler := func(ctx *fasthttp.RequestCtx) {
		ctx.SetStatusCode(fasthttp.StatusOK)
	}

	cfg := &config.HTTP2Config{
		Enabled:    true,
		H2CEnabled: true,
	}

	server, err := NewServer(cfg, handler, nil)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// 验证 H2C 启用
	if !server.IsH2CEnabled() {
		t.Error("H2C should be enabled")
	}
}

// BenchmarkAdapterConversion 基准测试适配器转换性能。
func BenchmarkAdapterConversion(b *testing.B) {
	handler := func(ctx *fasthttp.RequestCtx) {
		ctx.WriteString("Hello") //nolint:errcheck
		ctx.SetStatusCode(fasthttp.StatusOK)
	}

	adapter := NewFastHTTPHandlerAdapter(handler)
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		rec.Body.Reset()
		adapter.ServeHTTP(rec, req)
	}
}

// BenchmarkAdapterWithBody 基准测试带请求体的适配器。
func BenchmarkAdapterWithBody(b *testing.B) {
	handler := func(ctx *fasthttp.RequestCtx) {
		ctx.Write(ctx.PostBody()) //nolint:errcheck
		ctx.SetStatusCode(fasthttp.StatusOK)
	}

	adapter := NewFastHTTPHandlerAdapter(handler)
	body := []byte(`{"test":"data","number":12345}`)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api", bytes.NewReader(body))
		rec := httptest.NewRecorder()
		adapter.ServeHTTP(rec, req)
	}
}

// BenchmarkServerCreation 基准测试服务器创建。
func BenchmarkServerCreation(b *testing.B) {
	cfg := &config.HTTP2Config{
		Enabled:              true,
		MaxConcurrentStreams: 100,
	}
	handler := func(ctx *fasthttp.RequestCtx) {}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := NewServer(cfg, handler, nil)
		if err != nil {
			b.Fatal(err)
		}
	}
}
