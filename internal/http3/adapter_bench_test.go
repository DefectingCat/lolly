// Package http3 提供 HTTP/3 适配器的基准测试。
//
// 该文件测试 HTTP/3 适配器的性能，包括：
//   - Handler 包装开销
//   - HTTP 请求到 fasthttp 请求的转换性能
//   - 不同大小 Body 的读取性能
//   - fasthttp 响应到 HTTP 响应的转换性能
//   - RequestCtx sync.Pool 复用效率
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

// BenchmarkAdapterWrap 测试 Handler 包装开销
//
// 该基准测试测量将 fasthttp.RequestHandler 包装为 http.Handler 的基本开销，
// 包括 ctxPool 获取和放回操作。
func BenchmarkAdapterWrap(b *testing.B) {
	adapter := NewAdapter()

	// 简单的 fasthttp handler
	handler := func(ctx *fasthttp.RequestCtx) {
		ctx.SetStatusCode(200)
		ctx.SetBodyString("OK")
	}

	httpHandler := adapter.Wrap(handler)

	// 创建请求
	req := &http.Request{
		Method: "GET",
		URL:    &url.URL{Path: "/test"},
		Host:   "localhost",
		Header: http.Header{},
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		rw := &mockResponseWriter{}
		httpHandler.ServeHTTP(rw, req)
	}
}

// BenchmarkAdapterConvertRequest 测试 HTTP -> fasthttp 头部转换性能
//
// 该基准测试测量 convertRequest 方法在仅有头部转换时的性能，
// 不包含 Body 读取的开销。
func BenchmarkAdapterConvertRequest(b *testing.B) {
	adapter := NewAdapter()

	// 创建包含多个头部的请求
	req := &http.Request{
		Method: "POST",
		URL:    &url.URL{Path: "/api/v1/users", RawQuery: "page=1&limit=10"},
		Host:   "api.example.com",
		Header: http.Header{
			"Content-Type":    []string{"application/json"},
			"Accept":          []string{"application/json"},
			"Authorization":   []string{"Bearer token123456"},
			"X-Request-ID":    []string{"req-123456789"},
			"X-User-Agent":    []string{"TestClient/1.0"},
			"Accept-Encoding": []string{"gzip, deflate"},
			"Cache-Control":   []string{"no-cache"},
			"Connection":      []string{"keep-alive"},
		},
		RemoteAddr: "192.168.1.100:12345",
	}

	ctx := &fasthttp.RequestCtx{}
	ctx.Init(&fasthttp.Request{}, nil, nil)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		ctx.Request.Reset()
		adapter.convertRequest(req, ctx)
	}
}

// BenchmarkAdapterConvertRequestBody_1KB 测试 1KB Body 读取性能
//
// 该基准测试测量 io.ReadAll 在 1KB Body 大小下的开销。
func BenchmarkAdapterConvertRequestBody_1KB(b *testing.B) {
	benchmarkAdapterConvertRequestBody(b, 1024)
}

// BenchmarkAdapterConvertRequestBody_10KB 测试 10KB Body 读取性能
//
// 该基准测试测量 io.ReadAll 在 10KB Body 大小下的开销。
func BenchmarkAdapterConvertRequestBody_10KB(b *testing.B) {
	benchmarkAdapterConvertRequestBody(b, 10*1024)
}

// BenchmarkAdapterConvertRequestBody_100KB 测试 100KB Body 读取性能
//
// 该基准测试测量 io.ReadAll 在 100KB Body 大小下的开销。
func BenchmarkAdapterConvertRequestBody_100KB(b *testing.B) {
	benchmarkAdapterConvertRequestBody(b, 100*1024)
}

// benchmarkAdapterConvertRequestBody 是 Body 读取性能的通用基准测试函数
//
// 参数：
//   - b: 测试对象
//   - bodySize: Body 大小（字节）
func benchmarkAdapterConvertRequestBody(b *testing.B, bodySize int) {
	adapter := NewAdapter()
	bodyData := make([]byte, bodySize)
	for i := range bodyData {
		bodyData[i] = byte('a' + (i % 26))
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		b.StopTimer()
		// 每次迭代创建新的 Body，模拟新的请求
		req := &http.Request{
			Method: "POST",
			URL:    &url.URL{Path: "/upload"},
			Host:   "localhost",
			Header: http.Header{
				"Content-Type": []string{"application/octet-stream"},
			},
			Body: io.NopCloser(bytes.NewReader(bodyData)),
		}
		ctx := &fasthttp.RequestCtx{}
		ctx.Init(&fasthttp.Request{}, nil, nil)
		b.StartTimer()

		adapter.convertRequest(req, ctx)
	}
}

// BenchmarkAdapterConvertResponse 测试 fasthttp -> HTTP 响应转换性能
//
// 该基准测试测量 convertResponse 方法的性能，包括：
//   - 状态码提取
//   - 响应头部复制
//   - 响应体写入
func BenchmarkAdapterConvertResponse(b *testing.B) {
	adapter := NewAdapter()

	// 预创建响应内容
	responseBody := []byte(`{"status":"success","data":{"id":12345,"name":"test","items":[1,2,3,4,5]}}`)

	ctx := &fasthttp.RequestCtx{}
	ctx.Init(&fasthttp.Request{}, nil, nil)
	ctx.SetStatusCode(200)
	ctx.Response.Header.Set("Content-Type", "application/json")
	ctx.Response.Header.Set("X-Response-ID", "resp-123456")
	ctx.Response.Header.Set("Cache-Control", "no-store")
	ctx.SetBody(responseBody)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		b.StopTimer()
		rw := &mockResponseWriter{}
		b.StartTimer()

		adapter.convertResponse(ctx, rw)
	}
}

// BenchmarkAdapterCtxPool 测试 RequestCtx sync.Pool 复用效率
//
// 该基准测试比较使用 sync.Pool 复用 RequestCtx 与每次创建新对象的性能差异。
func BenchmarkAdapterCtxPool(b *testing.B) {
	b.Run("WithPool", func(b *testing.B) {
		adapter := NewAdapter()
		handler := func(ctx *fasthttp.RequestCtx) {
			ctx.SetStatusCode(200)
		}
		httpHandler := adapter.Wrap(handler)
		req := &http.Request{
			Method: "GET",
			URL:    &url.URL{Path: "/"},
			Host:   "localhost",
		}

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			rw := &mockResponseWriter{}
			httpHandler.ServeHTTP(rw, req)
		}
	})

	b.Run("WithoutPool", func(b *testing.B) {
		handler := func(ctx *fasthttp.RequestCtx) {
			ctx.SetStatusCode(200)
		}
		req := &http.Request{
			Method: "GET",
			URL:    &url.URL{Path: "/"},
			Host:   "localhost",
		}

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			rw := &mockResponseWriter{}
			// 每次创建新的 ctx，不使用 Pool
			ctx := &fasthttp.RequestCtx{}
			ctx.Init(&fasthttp.Request{}, nil, nil)
			ctx.Request.Header.SetMethod(req.Method)
			ctx.Request.SetRequestURI(req.URL.Path)
			ctx.Request.Header.SetHost(req.Host)
			handler(ctx)
			// 模拟响应写入
			if ctx.Response.StatusCode() == 0 {
				rw.status = 200
			} else {
				rw.status = ctx.Response.StatusCode()
			}
		}
	})
}

// BenchmarkAdapterConvertRequest_Parallel 测试并发环境下的请求转换性能
//
// 该基准测试使用 b.RunParallel 模拟并发请求场景。
func BenchmarkAdapterConvertRequest_Parallel(b *testing.B) {
	adapter := NewAdapter()

	req := &http.Request{
		Method: "POST",
		URL:    &url.URL{Path: "/api/data", RawQuery: "key=value"},
		Host:   "localhost",
		Header: http.Header{
			"Content-Type": []string{"application/json"},
			"X-Request-ID": []string{"parallel-test"},
		},
	}

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			ctx := &fasthttp.RequestCtx{}
			ctx.Init(&fasthttp.Request{}, nil, nil)
			adapter.convertRequest(req, ctx)
		}
	})
}

// BenchmarkAdapterFullRoundTrip 测试完整的请求-响应往返性能
//
// 该基准测试模拟真实的 HTTP/3 请求处理流程，包括：
//   - 请求转换
//   - fasthttp handler 执行
//   - 响应转换
func BenchmarkAdapterFullRoundTrip(b *testing.B) {
	adapter := NewAdapter()

	// 模拟真实的 API handler
	apiHandler := func(ctx *fasthttp.RequestCtx) {
		// 模拟一些业务逻辑
		method := string(ctx.Method())
		path := string(ctx.Path())

		if method == "GET" && string(path) == "/api/users" {
			ctx.SetStatusCode(200)
			ctx.Response.Header.Set("Content-Type", "application/json")
			ctx.SetBodyString(`{"users":[{"id":1,"name":"Alice"},{"id":2,"name":"Bob"}]}`)
		} else {
			ctx.SetStatusCode(404)
			ctx.SetBodyString(`{"error":"not found"}`)
		}
	}

	httpHandler := adapter.Wrap(apiHandler)

	req := &http.Request{
		Method: "GET",
		URL:    &url.URL{Path: "/api/users"},
		Host:   "api.example.com",
		Header: http.Header{
			"Accept": []string{"application/json"},
		},
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		rw := &mockResponseWriter{}
		httpHandler.ServeHTTP(rw, req)
	}
}
