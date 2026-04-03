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
		t.Error("Expected non-nil adapter")
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

// TestFastHTTPHandler 测试反向转换
func TestFastHTTPHandler(t *testing.T) {
	// 创建标准库 handler
	stdHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(200)
		w.Write([]byte("Hello from std http"))
	})

	ctx := &fasthttp.RequestCtx{}
	ctx.Init(&fasthttp.Request{}, nil, nil)
	ctx.Request.SetRequestURI("/test")
	ctx.Request.Header.SetMethod("GET")

	FastHTTPHandler(stdHandler, ctx)

	if ctx.Response.StatusCode() != 200 {
		t.Errorf("Expected status 200, got %d", ctx.Response.StatusCode())
	}

	if string(ctx.Response.Body()) != "Hello from std http" {
		t.Errorf("Expected body 'Hello from std http', got %s", ctx.Response.Body())
	}
}

// TestConvertToHTTPRequest 测试转换为标准库请求
func TestConvertToHTTPRequest(t *testing.T) {
	ctx := &fasthttp.RequestCtx{}
	ctx.Init(&fasthttp.Request{}, nil, nil)
	ctx.Request.SetRequestURI("/path?query=value")
	ctx.Request.Header.SetMethod("POST")
	ctx.Request.Header.SetHost("example.com")
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Request.SetBody([]byte("test body"))

	r := convertToHTTPRequest(ctx)

	if r.Method != "POST" {
		t.Errorf("Expected Method POST, got %s", r.Method)
	}

	if r.Host != "example.com" {
		t.Errorf("Expected Host example.com, got %s", r.Host)
	}

	if r.URL.Path != "/path" {
		t.Errorf("Expected Path /path, got %s", r.URL.Path)
	}

	if r.URL.RawQuery != "query=value" {
		t.Errorf("Expected RawQuery query=value, got %s", r.URL.RawQuery)
	}

	if r.Proto != "HTTP/3" {
		t.Errorf("Expected Proto HTTP/3, got %s", r.Proto)
	}

	if r.ProtoMajor != 3 || r.ProtoMinor != 0 {
		t.Errorf("Expected Proto 3.0, got %d.%d", r.ProtoMajor, r.ProtoMinor)
	}

	// 检查头部
	if r.Header.Get("Content-Type") != "application/json" {
		t.Errorf("Expected Content-Type application/json, got %s", r.Header.Get("Content-Type"))
	}

	// 检查请求体
	body, _ := io.ReadAll(r.Body)
	if string(body) != "test body" {
		t.Errorf("Expected body 'test body', got %s", body)
	}
}

// TestFastHTTPResponseWriter_Write 测试 Write 方法
func TestFastHTTPResponseWriter_Write(t *testing.T) {
	ctx := &fasthttp.RequestCtx{}
	ctx.Init(&fasthttp.Request{}, nil, nil)

	rw := &fastHTTPResponseWriter{ctx: ctx}

	n, err := rw.Write([]byte("test content"))
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if n != len("test content") {
		t.Errorf("Expected written %d, got %d", len("test content"), n)
	}

	// 检查状态码被自动设置
	if rw.status != http.StatusOK {
		t.Errorf("Expected auto-set status 200, got %d", rw.status)
	}
}

// TestFastHTTPResponseWriter_WriteHeader 测试 WriteHeader 方法
func TestFastHTTPResponseWriter_WriteHeader(t *testing.T) {
	ctx := &fasthttp.RequestCtx{}
	ctx.Init(&fasthttp.Request{}, nil, nil)

	rw := &fastHTTPResponseWriter{ctx: ctx}

	rw.Header().Set("X-Custom", "value")
	rw.WriteHeader(404)

	if rw.status != 404 {
		t.Errorf("Expected status 404, got %d", rw.status)
	}

	if rw.written != true {
		t.Error("Expected written flag to be true")
	}

	// 再次调用应该被忽略
	rw.WriteHeader(500)
	if rw.status != 404 {
		t.Errorf("Expected status to remain 404, got %d", rw.status)
	}
}

// TestFastHTTPResponseWriter_Header 测试 Header 方法
func TestFastHTTPResponseWriter_Header(t *testing.T) {
	ctx := &fasthttp.RequestCtx{}
	ctx.Init(&fasthttp.Request{}, nil, nil)

	rw := &fastHTTPResponseWriter{ctx: ctx}

	h := rw.Header()
	if h == nil {
		t.Error("Expected non-nil header")
	}

	h.Set("Content-Type", "text/html")
	if rw.Header().Get("Content-Type") != "text/html" {
		t.Errorf("Expected Content-Type text/html, got %s", rw.Header().Get("Content-Type"))
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
	status int
	header http.Header
	body   []byte
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
