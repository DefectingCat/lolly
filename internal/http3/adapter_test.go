// Package http3 提供 HTTP/3 适配器功能的测试。
//
// 该文件测试 HTTP/3 适配器模块的各项功能，包括：
//   - 适配器创建和初始化
//   - HTTP 请求到 fasthttp 请求的转换
//   - fasthttp 响应到 HTTP 响应的转换
//   - 请求方法、URI、头部、请求体的转换
//   - 完整请求响应流程
//
// 作者：xfy
package http3

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"testing"

	"github.com/valyala/fasthttp"
)

// TestNewAdapter 测试适配器创建
func TestNewAdapter(t *testing.T) {
	adapter := NewAdapter()
	if adapter == nil {
		t.Fatal("Expected non-nil adapter")
	}

	// 测试 ctxPool 是否初始化
	ctx := adapter.ctxPool.Get().(*fasthttp.RequestCtx)
	if ctx == nil {
		t.Error("Expected non-nil RequestCtx from pool")
	}
	adapter.ctxPool.Put(ctx)
}

// TestWrap 测试 Wrap 函数基本功能
func TestWrap(t *testing.T) {
	adapter := NewAdapter()

	// 创建一个简单的 fasthttp handler
	handler := func(ctx *fasthttp.RequestCtx) {
		ctx.SetStatusCode(200)
		ctx.SetBodyString("Hello from fasthttp")
		ctx.Response.Header.Set("Content-Type", "text/plain")
	}

	httpHandler := adapter.Wrap(handler)
	if httpHandler == nil {
		t.Error("Expected non-nil http.Handler")
	}
}

// TestWrapHandler 测试 WrapHandler 函数
func TestWrapHandler(t *testing.T) {
	adapter := NewAdapter()

	handler := func(ctx *fasthttp.RequestCtx) {
		ctx.SetStatusCode(200)
		ctx.SetBodyString("test")
	}

	httpHandler := adapter.WrapHandler(handler)
	if httpHandler == nil {
		t.Error("Expected non-nil http.Handler")
	}
}

// TestConvertRequest_Method 测试请求方法转换
func TestConvertRequest_Method(t *testing.T) {
	adapter := NewAdapter()

	tests := []string{"GET", "POST", "PUT", "DELETE", "HEAD", "OPTIONS"}

	for _, method := range tests {
		t.Run(method, func(t *testing.T) {
			req := &http.Request{
				Method: method,
				URL:    &url.URL{Path: "/test"},
				Host:   "localhost",
			}

			ctx := &fasthttp.RequestCtx{}
			ctx.Init(&fasthttp.Request{}, nil, nil)

			adapter.convertRequest(req, ctx)

			if string(ctx.Method()) != method {
				t.Errorf("Expected method %s, got %s", method, ctx.Method())
			}
		})
	}
}

// TestConvertRequest_URI 测试 URI 转换
func TestConvertRequest_URI(t *testing.T) {
	adapter := NewAdapter()

	tests := []struct {
		name     string
		path     string
		query    string
		expected string
	}{
		{
			name:     "simple path",
			path:     "/test",
			query:    "",
			expected: "/test",
		},
		{
			name:     "path with query",
			path:     "/test",
			query:    "foo=bar",
			expected: "/test?foo=bar",
		},
		{
			name:     "path with multiple query params",
			path:     "/api/users",
			query:    "id=1&name=test",
			expected: "/api/users?id=1&name=test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &http.Request{
				Method: "GET",
				URL:    &url.URL{Path: tt.path, RawQuery: tt.query},
				Host:   "localhost",
			}

			ctx := &fasthttp.RequestCtx{}
			ctx.Init(&fasthttp.Request{}, nil, nil)

			adapter.convertRequest(req, ctx)

			if string(ctx.RequestURI()) != tt.expected {
				t.Errorf("Expected URI %s, got %s", tt.expected, ctx.RequestURI())
			}
		})
	}
}

// TestConvertRequest_Headers 测试头部转换
func TestConvertRequest_Headers(t *testing.T) {
	adapter := NewAdapter()

	req := &http.Request{
		Method: "GET",
		URL:    &url.URL{Path: "/test"},
		Host:   "example.com",
		Header: http.Header{
			"X-Custom-Header": []string{"value1", "value2"},
			"Content-Type":    []string{"application/json"},
			"Accept":          []string{"text/html"},
		},
	}

	ctx := &fasthttp.RequestCtx{}
	ctx.Init(&fasthttp.Request{}, nil, nil)

	adapter.convertRequest(req, ctx)

	// 检查 Host
	if string(ctx.Host()) != "example.com" {
		t.Errorf("Expected Host example.com, got %s", ctx.Host())
	}

	// 检查头部
	if string(ctx.Request.Header.Peek("Content-Type")) != "application/json" {
		t.Errorf("Expected Content-Type application/json, got %s", ctx.Request.Header.Peek("Content-Type"))
	}

	if string(ctx.Request.Header.Peek("Accept")) != "text/html" {
		t.Errorf("Expected Accept text/html, got %s", ctx.Request.Header.Peek("Accept"))
	}
}

// TestConvertRequest_Body 测试请求体转换
func TestConvertRequest_Body(t *testing.T) {
	adapter := NewAdapter()

	bodyContent := []byte("test request body")
	req := &http.Request{
		Method: "POST",
		URL:    &url.URL{Path: "/test"},
		Host:   "localhost",
		Body:   io.NopCloser(bytes.NewReader(bodyContent)),
	}

	ctx := &fasthttp.RequestCtx{}
	ctx.Init(&fasthttp.Request{}, nil, nil)

	adapter.convertRequest(req, ctx)

	if !bytes.Equal(ctx.Request.Body(), bodyContent) {
		t.Errorf("Expected body %s, got %s", bodyContent, ctx.Request.Body())
	}
}

// TestConvertRequest_RemoteAddr 测试远程地址转换
func TestConvertRequest_RemoteAddr(t *testing.T) {
	adapter := NewAdapter()

	req := &http.Request{
		Method:     "GET",
		URL:        &url.URL{Path: "/test"},
		Host:       "localhost",
		RemoteAddr: "192.168.1.1:8080",
	}

	ctx := &fasthttp.RequestCtx{}
	ctx.Init(&fasthttp.Request{}, nil, nil)

	adapter.convertRequest(req, ctx)

	remoteAddr := ctx.RemoteAddr()
	if remoteAddr == nil {
		t.Error("Expected non-nil remote address")
	} else {
		if remoteAddr.String() != "192.168.1.1:8080" {
			t.Errorf("Expected remote addr 192.168.1.1:8080, got %s", remoteAddr.String())
		}
	}
}

// TestConvertResponse_Status 测试响应状态码转换
func TestConvertResponse_Status(t *testing.T) {
	adapter := NewAdapter()

	tests := []int{200, 201, 400, 404, 500}

	for _, status := range tests {
		t.Run(string(rune(status)), func(t *testing.T) {
			ctx := &fasthttp.RequestCtx{}
			ctx.Init(&fasthttp.Request{}, nil, nil)
			ctx.SetStatusCode(status)

			// 创建 mock ResponseWriter
			rw := &mockResponseWriter{}

			adapter.convertResponse(ctx, rw)

			if rw.status != status {
				t.Errorf("Expected status %d, got %d", status, rw.status)
			}
		})
	}
}

// TestConvertResponse_DefaultStatus 测试默认状态码
func TestConvertResponse_DefaultStatus(t *testing.T) {
	adapter := NewAdapter()

	ctx := &fasthttp.RequestCtx{}
	ctx.Init(&fasthttp.Request{}, nil, nil)
	// 不设置状态码

	rw := &mockResponseWriter{}

	adapter.convertResponse(ctx, rw)

	// 默认应该是 200
	if rw.status != 200 {
		t.Errorf("Expected default status 200, got %d", rw.status)
	}
}

// TestConvertResponse_Headers 测试响应头部转换
func TestConvertResponse_Headers(t *testing.T) {
	adapter := NewAdapter()

	ctx := &fasthttp.RequestCtx{}
	ctx.Init(&fasthttp.Request{}, nil, nil)
	ctx.Response.Header.Set("Content-Type", "application/json")
	ctx.Response.Header.Set("X-Custom", "value")

	rw := &mockResponseWriter{}

	adapter.convertResponse(ctx, rw)

	if rw.header.Get("Content-Type") != "application/json" {
		t.Errorf("Expected Content-Type application/json, got %s", rw.header.Get("Content-Type"))
	}

	if rw.header.Get("X-Custom") != "value" {
		t.Errorf("Expected X-Custom value, got %s", rw.header.Get("X-Custom"))
	}
}

// TestConvertResponse_Body 测试响应体转换
func TestConvertResponse_Body(t *testing.T) {
	adapter := NewAdapter()

	bodyContent := []byte("response body content")

	ctx := &fasthttp.RequestCtx{}
	ctx.Init(&fasthttp.Request{}, nil, nil)
	ctx.SetBody(bodyContent)

	rw := &mockResponseWriter{}

	adapter.convertResponse(ctx, rw)

	if !bytes.Equal(rw.body, bodyContent) {
		t.Errorf("Expected body %s, got %s", bodyContent, rw.body)
	}
}

// TestWrap_RoundTrip 完整流程测试
func TestWrap_RoundTrip(t *testing.T) {
	adapter := NewAdapter()

	// fasthttp handler
	fastHandler := func(ctx *fasthttp.RequestCtx) {
		// 检查请求
		if string(ctx.Method()) != "POST" {
			t.Errorf("Expected POST method, got %s", ctx.Method())
		}

		// 设置响应
		ctx.SetStatusCode(201)
		ctx.SetBodyString("Created")
		ctx.Response.Header.Set("X-Response-Header", "test-value")
	}

	httpHandler := adapter.Wrap(fastHandler)

	// 创建请求
	req := &http.Request{
		Method: "POST",
		URL:    &url.URL{Path: "/create"},
		Host:   "localhost",
		Header: http.Header{
			"Content-Type": []string{"application/json"},
		},
		Body: io.NopCloser(bytes.NewReader([]byte("request data"))),
	}

	// 创建 mock ResponseWriter
	rw := &mockResponseWriter{}

	// 执行
	httpHandler.ServeHTTP(rw, req)

	// 检查响应
	if rw.status != 201 {
		t.Errorf("Expected status 201, got %d", rw.status)
	}

	if rw.header.Get("X-Response-Header") != "test-value" {
		t.Errorf("Expected X-Response-Header test-value, got %s", rw.header.Get("X-Response-Header"))
	}

	if string(rw.body) != "Created" {
		t.Errorf("Expected body 'Created', got %s", rw.body)
	}
}

// mockResponseWriter 用于测试的 mock ResponseWriter
type mockResponseWriter struct {
	header http.Header
	body   []byte
	status int
}

func (m *mockResponseWriter) Header() http.Header {
	if m.header == nil {
		m.header = make(http.Header)
	}
	return m.header
}

func (m *mockResponseWriter) WriteHeader(statusCode int) {
	m.status = statusCode
}

func (m *mockResponseWriter) Write(data []byte) (int, error) {
	m.body = append(m.body, data...)
	if m.status == 0 {
		m.status = 200
	}
	return len(data), nil
}
