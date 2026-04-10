// Package http2 提供 HTTP/2 适配器测试。
//
// 该文件包含 FastHTTPHandlerAdapter 的单元测试：
//   - 适配器创建和配置
//   - 请求转换测试
//   - 响应转换测试
//   - 流式请求体处理
//
// 作者：xfy
package http2

import (
	"bytes"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/valyala/fasthttp"
)

// TestNewFastHTTPHandlerAdapter 测试适配器创建。
func TestNewFastHTTPHandlerAdapter(t *testing.T) {
	handler := func(ctx *fasthttp.RequestCtx) {
		ctx.WriteString("Hello") //nolint:errcheck
	}

	adapter := NewFastHTTPHandlerAdapter(handler)
	if adapter == nil {
		t.Fatal("NewFastHTTPHandlerAdapter() returned nil")
	}

	if adapter.handler == nil {
		t.Error("Adapter handler not set")
	}
}

// TestFastHTTPHandlerAdapterServeHTTP 测试适配器处理 HTTP 请求。
func TestFastHTTPHandlerAdapterServeHTTP(t *testing.T) {
	handler := func(ctx *fasthttp.RequestCtx) {
		ctx.WriteString("Hello from fasthttp") //nolint:errcheck
		ctx.SetStatusCode(fasthttp.StatusOK)
	}

	adapter := NewFastHTTPHandlerAdapter(handler)

	// 创建测试请求
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Custom-Header", "custom-value")
	rec := httptest.NewRecorder()

	// 执行请求
	adapter.ServeHTTP(rec, req)

	// 验证响应
	if rec.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, rec.Code)
	}

	body := rec.Body.String()
	if body != "Hello from fasthttp" {
		t.Errorf("Expected body 'Hello from fasthttp', got '%s'", body)
	}
}

// TestFastHTTPHandlerAdapterWithRequestBody 测试带请求体的请求。
func TestFastHTTPHandlerAdapterWithRequestBody(t *testing.T) {
	handler := func(ctx *fasthttp.RequestCtx) {
		body := ctx.PostBody()
		ctx.Write(body) //nolint:errcheck
		ctx.SetStatusCode(fasthttp.StatusOK)
	}

	adapter := NewFastHTTPHandlerAdapter(handler)

	// 创建带请求体的测试请求
	body := []byte(`{"key":"value"}`)
	req := httptest.NewRequest(http.MethodPost, "/api", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	// 执行请求
	adapter.ServeHTTP(rec, req)

	// 验证响应
	if rec.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, rec.Code)
	}

	respBody := rec.Body.String()
	if respBody != string(body) {
		t.Errorf("Expected body '%s', got '%s'", string(body), respBody)
	}
}

// TestFastHTTPHandlerAdapterWithHeaders 测试请求头转换。
func TestFastHTTPHandlerAdapterWithHeaders(t *testing.T) {
	var receivedHeaders map[string]string

	handler := func(ctx *fasthttp.RequestCtx) {
		receivedHeaders = make(map[string]string)
		for key, value := range ctx.Request.Header.All() {
			receivedHeaders[string(key)] = string(value)
		}
		ctx.SetStatusCode(fasthttp.StatusOK)
	}

	adapter := NewFastHTTPHandlerAdapter(handler)

	// 创建带多个头部的测试请求
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer token123")
	req.Header.Set("X-Request-ID", "uuid-123")
	rec := httptest.NewRecorder()

	// 执行请求
	adapter.ServeHTTP(rec, req)

	// 验证接收到的头部
	if receivedHeaders == nil {
		t.Fatal("No headers received")
	}

	if _, ok := receivedHeaders["Accept"]; !ok {
		t.Error("Accept header not received")
	}
	if _, ok := receivedHeaders["Authorization"]; !ok {
		t.Error("Authorization header not received")
	}
}

// TestFastHTTPHandlerAdapterWithQueryString 测试查询字符串。
func TestFastHTTPHandlerAdapterWithQueryString(t *testing.T) {
	var receivedURI string

	handler := func(ctx *fasthttp.RequestCtx) {
		receivedURI = string(ctx.Request.URI().RequestURI())
		ctx.SetStatusCode(fasthttp.StatusOK)
	}

	adapter := NewFastHTTPHandlerAdapter(handler)

	// 创建带查询字符串的测试请求
	req := httptest.NewRequest(http.MethodGet, "/search?q=hello&page=1", nil)
	rec := httptest.NewRecorder()

	// 执行请求
	adapter.ServeHTTP(rec, req)

	// 验证 URI
	if receivedURI == "" {
		t.Error("Request URI not received")
	}
}

// TestFastHTTPHandlerAdapterErrorResponse 测试错误响应。
func TestFastHTTPHandlerAdapterErrorResponse(t *testing.T) {
	handler := func(ctx *fasthttp.RequestCtx) {
		ctx.Error("Not Found", fasthttp.StatusNotFound)
	}

	adapter := NewFastHTTPHandlerAdapter(handler)

	req := httptest.NewRequest(http.MethodGet, "/notfound", nil)
	rec := httptest.NewRecorder()

	adapter.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Expected status %d, got %d", http.StatusNotFound, rec.Code)
	}
}

// TestFastHTTPHandlerAdapterEmptyBody 测试空请求体。
func TestFastHTTPHandlerAdapterEmptyBody(t *testing.T) {
	handler := func(ctx *fasthttp.RequestCtx) {
		if len(ctx.Request.Body()) == 0 {
			ctx.WriteString("Empty body received") //nolint:errcheck
		}
		ctx.SetStatusCode(fasthttp.StatusOK)
	}

	adapter := NewFastHTTPHandlerAdapter(handler)

	req := httptest.NewRequest(http.MethodPost, "/upload", nil)
	rec := httptest.NewRecorder()

	adapter.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, rec.Code)
	}

	if rec.Body.String() != "Empty body received" {
		t.Error("Empty body not handled correctly")
	}
}

// TestWrapHandler 测试 WrapHandler 便捷函数。
func TestWrapHandler(t *testing.T) {
	handler := func(ctx *fasthttp.RequestCtx) {
		ctx.WriteString("Wrapped") //nolint:errcheck
	}

	wrapped := WrapHandler(handler)
	if wrapped == nil {
		t.Fatal("WrapHandler() returned nil")
	}

	// 验证它是一个 http.Handler
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	wrapped.ServeHTTP(rec, req)

	if rec.Body.String() != "Wrapped" {
		t.Error("WrapHandler did not work correctly")
	}
}

// TestWrapHandlerFunc 测试 WrapHandlerFunc 便捷函数。
func TestWrapHandlerFunc(t *testing.T) {
	fn := func(ctx *fasthttp.RequestCtx) {
		ctx.WriteString("Func wrapped") //nolint:errcheck
	}

	wrapped := WrapHandlerFunc(fn)
	if wrapped == nil {
		t.Fatal("WrapHandlerFunc() returned nil")
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	wrapped.ServeHTTP(rec, req)

	if rec.Body.String() != "Func wrapped" {
		t.Error("WrapHandlerFunc did not work correctly")
	}
}

// TestDefaultAdapterConfig 测试默认适配器配置。
func TestDefaultAdapterConfig(t *testing.T) {
	cfg := DefaultAdapterConfig()

	if cfg == nil {
		t.Fatal("DefaultAdapterConfig() returned nil")
	}

	if cfg.BufferSize <= 0 {
		t.Error("BufferSize should be positive")
	}

	if cfg.MaxBodySize <= 0 {
		t.Error("MaxBodySize should be positive")
	}

	if cfg.Timeout <= 0 {
		t.Error("Timeout should be positive")
	}
}

// TestNewConfigurableAdapter 测试可配置适配器。
func TestNewConfigurableAdapter(t *testing.T) {
	handler := func(ctx *fasthttp.RequestCtx) {
		ctx.WriteString("Configurable") //nolint:errcheck
	}

	cfg := DefaultAdapterConfig()
	adapter := NewConfigurableAdapter(handler, cfg)

	if adapter == nil {
		t.Fatal("NewConfigurableAdapter() returned nil")
	}

	if adapter.config != cfg {
		t.Error("Config not set correctly")
	}

	// 测试 nil 配置
	adapter2 := NewConfigurableAdapter(handler, nil)
	if adapter2 == nil {
		t.Fatal("NewConfigurableAdapter() with nil config returned nil")
	}
	if adapter2.config == nil {
		t.Error("Default config not applied")
	}
}

// TestAdapterWithLargeBody 测试大请求体处理。
func TestAdapterWithLargeBody(t *testing.T) {
	bodyReceived := false

	handler := func(ctx *fasthttp.RequestCtx) {
		body := ctx.PostBody()
		if len(body) > 1024 {
			bodyReceived = true
		}
		ctx.SetStatusCode(fasthttp.StatusOK)
	}

	adapter := NewFastHTTPHandlerAdapter(handler)

	// 创建大请求体
	largeBody := make([]byte, 100*1024) // 100KB
	for i := range largeBody {
		largeBody[i] = byte('a' + (i % 26))
	}

	req := httptest.NewRequest(http.MethodPost, "/upload", bytes.NewReader(largeBody))
	req.Header.Set("Content-Length", "102400")
	rec := httptest.NewRecorder()

	adapter.ServeHTTP(rec, req)

	if !bodyReceived {
		t.Error("Large body was not received correctly")
	}
}

// TestAdapterHTTPMethods 测试不同 HTTP 方法。
func TestAdapterHTTPMethods(t *testing.T) {
	methods := []string{
		http.MethodGet,
		http.MethodPost,
		http.MethodPut,
		http.MethodDelete,
		http.MethodPatch,
		http.MethodHead,
		http.MethodOptions,
	}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			var receivedMethod string

			handler := func(ctx *fasthttp.RequestCtx) {
				receivedMethod = string(ctx.Method())
				ctx.SetStatusCode(fasthttp.StatusOK)
			}

			adapter := NewFastHTTPHandlerAdapter(handler)

			req := httptest.NewRequest(method, "/test", nil)
			rec := httptest.NewRecorder()

			adapter.ServeHTTP(rec, req)

			if receivedMethod != method {
				t.Errorf("Expected method %s, got %s", method, receivedMethod)
			}
		})
	}
}

// TestAdapterRemoteAddr 测试远程地址设置。
func TestAdapterRemoteAddr(t *testing.T) {
	var remoteAddr net.Addr

	handler := func(ctx *fasthttp.RequestCtx) {
		remoteAddr = ctx.RemoteAddr()
		ctx.SetStatusCode(fasthttp.StatusOK)
	}

	adapter := NewFastHTTPHandlerAdapter(handler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	rec := httptest.NewRecorder()

	adapter.ServeHTTP(rec, req)

	if remoteAddr == nil {
		t.Error("RemoteAddr not set")
	}
}

// TestAdapterContentType 测试 Content-Type 处理。
func TestAdapterContentType(t *testing.T) {
	var contentType string

	handler := func(ctx *fasthttp.RequestCtx) {
		contentType = string(ctx.Request.Header.ContentType())
		ctx.SetStatusCode(fasthttp.StatusOK)
	}

	adapter := NewFastHTTPHandlerAdapter(handler)

	req := httptest.NewRequest(http.MethodPost, "/api", nil)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	adapter.ServeHTTP(rec, req)

	if contentType != "application/json" {
		t.Errorf("Expected Content-Type 'application/json', got '%s'", contentType)
	}
}

// TestAdapterResponseHeaders 测试响应头设置。
func TestAdapterResponseHeaders(t *testing.T) {
	handler := func(ctx *fasthttp.RequestCtx) {
		ctx.Response.Header.Set("X-Custom-Response", "custom-value")
		ctx.Response.Header.Set("Cache-Control", "no-cache")
		ctx.SetStatusCode(fasthttp.StatusOK)
	}

	adapter := NewFastHTTPHandlerAdapter(handler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	adapter.ServeHTTP(rec, req)

	if rec.Header().Get("X-Custom-Response") != "custom-value" {
		t.Error("Custom response header not set")
	}

	if rec.Header().Get("Cache-Control") != "no-cache" {
		t.Error("Cache-Control header not set")
	}
}

// TestAdapterConcurrentRequests 测试并发请求。
func TestAdapterConcurrentRequests(t *testing.T) {
	handler := func(ctx *fasthttp.RequestCtx) {
		// 模拟一些处理时间
		time.Sleep(1 * time.Millisecond)
		ctx.WriteString("OK") //nolint:errcheck
		ctx.SetStatusCode(fasthttp.StatusOK)
	}

	adapter := NewFastHTTPHandlerAdapter(handler)

	// 并发发送多个请求
	concurrency := 10
	done := make(chan bool, concurrency)

	for i := 0; i < concurrency; i++ {
		go func() {
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			rec := httptest.NewRecorder()
			adapter.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Errorf("Expected status %d, got %d", http.StatusOK, rec.Code)
			}
			done <- true
		}()
	}

	// 等待所有请求完成
	for i := 0; i < concurrency; i++ {
		<-done
	}
}

// mockReadCloser 是一个用于测试的模拟 io.ReadCloser。
type mockReadCloser struct {
	io.Reader
	closed bool
}

func (m *mockReadCloser) Close() error {
	m.closed = true
	return nil
}

// TestStreamRequestBody 测试流式请求体。
func TestStreamRequestBody(t *testing.T) {
	bodyContent := []byte("test body content")
	mock := &mockReadCloser{Reader: bytes.NewReader(bodyContent)}

	handler := func(ctx *fasthttp.RequestCtx) {
		ctx.SetStatusCode(fasthttp.StatusOK)
	}

	adapter := NewFastHTTPHandlerAdapter(handler)

	// 创建带有 mock body 的请求
	req, _ := http.NewRequest(http.MethodPost, "/test", mock)
	req.ContentLength = int64(len(bodyContent))
	rec := httptest.NewRecorder()

	adapter.ServeHTTP(rec, req)

	// 验证 body 被关闭
	if !mock.closed {
		t.Error("Request body was not closed")
	}
}

// TestAdapterPoolReuse 测试对象池复用。
func TestAdapterPoolReuse(_ *testing.T) {
	handler := func(ctx *fasthttp.RequestCtx) {
		ctx.WriteString("Test") //nolint:errcheck
		ctx.SetStatusCode(fasthttp.StatusOK)
	}

	adapter := NewFastHTTPHandlerAdapter(handler)

	// 发送多个请求，验证池复用
	for i := 0; i < 10; i++ {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()
		adapter.ServeHTTP(rec, req)
	}

	// 测试通过，没有 panic 表示池工作正常
}
