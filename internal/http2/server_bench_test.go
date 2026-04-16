// Package http2 提供 HTTP/2 服务器基准测试。
//
// 该文件测试 HTTP/2 服务器的性能，包括：
//   - 服务器创建开销
//   - HTTP/2 帧编码性能
//   - HPACK 头部压缩性能
//   - 流创建和管理开销
//   - 并发流处理吞吐量
//   - 完整请求往返延迟
//
// 作者：xfy
package http2

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/valyala/fasthttp"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
	"rua.plus/lolly/internal/config"
)

// BenchmarkHTTP2ServerStart 测试 HTTP/2 服务器启动开销
//
// 该基准测试测量从配置创建到 http2.Server 实例化的完整开销，
// 包括默认值填充、对象池初始化和结构体分配。
func BenchmarkHTTP2ServerStart(b *testing.B) {
	cfg := &config.HTTP2Config{
		Enabled:              true,
		MaxConcurrentStreams: 100,
		MaxHeaderListSize:    1048576,
		PushEnabled:          false,
	}
	handler := func(_ *fasthttp.RequestCtx) {}

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_, err := NewServer(cfg, handler, nil)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkHTTP2FrameEncoding 测试 HTTP/2 帧编码性能
//
// 该基准测试测量使用 golang.org/x/net/http2 的 Framer
// 编码不同类型 HTTP/2 帧的开销，包括 DATA、HEADERS 和
// SETTINGS 帧。
func BenchmarkHTTP2FrameEncoding(b *testing.B) {
	// 创建 Framer 实例（复用写入缓冲区）
	var buf bytes.Buffer
	framer := http2.NewFramer(&buf, nil)

	settings := []http2.Setting{
		{ID: http2.SettingMaxConcurrentStreams, Val: 100},
		{ID: http2.SettingInitialWindowSize, Val: 65535},
		{ID: http2.SettingMaxFrameSize, Val: 16384},
	}

	b.ResetTimer()
	b.ReportAllocs()

	b.Run("SettingsFrame", func(b *testing.B) {
		for b.Loop() {
			buf.Reset()
			if err := framer.WriteSettings(settings...); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("DataFrame", func(b *testing.B) {
		// 预创建帧载荷
		payload := make([]byte, 1024)
		for i := range payload {
			payload[i] = byte('a' + (i % 26))
		}

		b.ResetTimer()
		for b.Loop() {
			buf.Reset()
			if err := framer.WriteData(1, false, payload); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("DataFrame_Small", func(b *testing.B) {
		payload := []byte(`{"id":1,"name":"test"}`)

		b.ResetTimer()
		for b.Loop() {
			buf.Reset()
			if err := framer.WriteData(1, false, payload); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("DataFrame_Large", func(b *testing.B) {
		payload := bytes.Repeat([]byte("X"), 16384)

		b.ResetTimer()
		for b.Loop() {
			buf.Reset()
			if err := framer.WriteData(1, false, payload); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("PingFrame", func(b *testing.B) {
		data := [8]byte{1, 2, 3, 4, 5, 6, 7, 8}

		b.ResetTimer()
		for b.Loop() {
			buf.Reset()
			if err := framer.WritePing(false, data); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("RSTStreamFrame", func(b *testing.B) {
		b.ResetTimer()
		for b.Loop() {
			buf.Reset()
			if err := framer.WriteRSTStream(1, http2.ErrCodeCancel); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("WindowUpdateFrame", func(b *testing.B) {
		b.ResetTimer()
		for b.Loop() {
			buf.Reset()
			if err := framer.WriteWindowUpdate(1, 65535); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("GoAwayFrame", func(b *testing.B) {
		b.ResetTimer()
		for b.Loop() {
			buf.Reset()
			if err := framer.WriteGoAway(1, http2.ErrCodeNo, []byte("graceful shutdown")); err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkHTTP2HeadersEncoding 测试 HPACK 头部压缩性能
//
// 该基准测试测量使用 HPACK 编码器压缩 HTTP/2 请求头部的开销，
// 模拟不同头部数量和大小场景下的压缩性能。
func BenchmarkHTTP2HeadersEncoding(b *testing.B) {
	// 模拟真实 HTTP/2 请求头部
	commonHeaders := []hpack.HeaderField{
		{Name: ":method", Value: "GET"},
		{Name: ":scheme", Value: "https"},
		{Name: ":path", Value: "/api/v1/users"},
		{Name: ":authority", Value: "api.example.com"},
		{Name: "user-agent", Value: "Mozilla/5.0 (X11; Linux x86_64) Chrome/120.0"},
		{Name: "accept", Value: "application/json, text/plain, */*"},
		{Name: "accept-encoding", Value: "gzip, deflate, br"},
		{Name: "accept-language", Value: "en-US,en;q=0.9"},
	}

	authHeaders := append([]hpack.HeaderField(nil), commonHeaders...)
	authHeaders = append(authHeaders,
		hpack.HeaderField{Name: "authorization", Value: "Bearer eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0"},
		hpack.HeaderField{Name: "x-request-id", Value: "req-abc123-def456"},
		hpack.HeaderField{Name: "x-trace-id", Value: "trace-789xyz"},
		hpack.HeaderField{Name: "cookie", Value: "session=abc123; _ga=GA1.2.123456; _gid=GA1.2.789012"},
	)

	bodyHeaders := []hpack.HeaderField{
		{Name: ":method", Value: "POST"},
		{Name: ":scheme", Value: "https"},
		{Name: ":path", Value: "/api/v1/upload"},
		{Name: ":authority", Value: "api.example.com"},
		{Name: "content-type", Value: "multipart/form-data; boundary=----WebKitFormBoundary"},
		{Name: "content-length", Value: "1048576"},
	}

	b.ResetTimer()
	b.ReportAllocs()

	b.Run("CommonHeaders", func(b *testing.B) {
		var buf bytes.Buffer
		encoder := hpack.NewEncoder(&buf)

		for b.Loop() {
			buf.Reset()
			for _, hf := range commonHeaders {
				if err := encoder.WriteField(hf); err != nil {
					b.Fatal(err)
				}
			}
		}
	})

	b.Run("CommonHeaders_Parallel", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				var buf bytes.Buffer
				encoder := hpack.NewEncoder(&buf)
				for _, hf := range commonHeaders {
					if err := encoder.WriteField(hf); err != nil {
						b.Fatal(err)
					}
				}
			}
		})
	})

	b.Run("AuthHeaders", func(b *testing.B) {
		var buf bytes.Buffer
		encoder := hpack.NewEncoder(&buf)

		for b.Loop() {
			buf.Reset()
			for _, hf := range authHeaders {
				if err := encoder.WriteField(hf); err != nil {
					b.Fatal(err)
				}
			}
		}
	})

	b.Run("BodyHeaders", func(b *testing.B) {
		var buf bytes.Buffer
		encoder := hpack.NewEncoder(&buf)

		for b.Loop() {
			buf.Reset()
			for _, hf := range bodyHeaders {
				if err := encoder.WriteField(hf); err != nil {
					b.Fatal(err)
				}
			}
		}
	})

	b.Run("RepeatedHeaders", func(b *testing.B) {
		// 重复发送相同头部，测试 HPACK 静态表和动态表的效果
		var buf bytes.Buffer
		encoder := hpack.NewEncoder(&buf)

		for b.Loop() {
			buf.Reset()
			for _, hf := range commonHeaders {
				if err := encoder.WriteField(hf); err != nil {
					b.Fatal(err)
				}
			}
		}
	})
}

// BenchmarkHTTP2StreamCreate 测试流创建开销
//
// 该基准测试测量 HTTP/2 流创建和请求适配的核心开销，
// 包括 RequestCtx 从池中获取、请求转换和响应返回。
// 使用适配器直接模拟流处理流程，避免网络 I/O 不确定性。
func BenchmarkHTTP2StreamCreate(b *testing.B) {
	handler := func(ctx *fasthttp.RequestCtx) {
		ctx.SetStatusCode(fasthttp.StatusOK)
		ctx.WriteString("OK")
	}

	adapter := NewFastHTTPHandlerAdapter(handler)

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		req := httptest.NewRequest(http.MethodGet, "/stream/test", nil)
		req.Header.Set(":authority", "localhost")

		rec := httptest.NewRecorder()
		adapter.ServeHTTP(rec, req)
	}
}

// BenchmarkHTTP2ConcurrentStreams 测试并发流处理吞吐量
//
// 该基准测试使用 b.RunParallel 模拟多个客户端并发发送请求的场景，
// 测量服务器在多路复用下的吞吐量，通过复用适配器实例
// 模拟 HTTP/2 多流并发处理的实际开销。
func BenchmarkHTTP2ConcurrentStreams(b *testing.B) {
	handler := func(ctx *fasthttp.RequestCtx) {
		ctx.SetStatusCode(fasthttp.StatusOK)
		ctx.Response.Header.Set("Content-Type", "application/json")
		ctx.WriteString(`{"status":"ok"}`)
	}

	adapter := NewFastHTTPHandlerAdapter(handler)

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			req := httptest.NewRequest(http.MethodGet, "/api/concurrent", nil)
			req.Header.Set("Accept", "application/json")
			req.Header.Set("X-Stream-ID", "stream")

			rec := httptest.NewRecorder()
			adapter.ServeHTTP(rec, req)
		}
	})
}

// BenchmarkHTTP2RequestRoundTrip 测试完整请求往返性能
//
// 该基准测试模拟真实的 HTTP/2 请求处理流程：
//  1. 客户端创建连接并发送 HTTP/2 前导
//  2. 服务器接收并处理请求（通过适配器转换为 fasthttp）
//  3. 服务器返回响应
//  4. 客户端接收响应
//
// 注意：由于 Serve 方法会阻塞在 Accept 循环上，
// 本测试使用适配器层直接模拟完整流程，避免实际网络 I/O。
func BenchmarkHTTP2RequestRoundTrip(b *testing.B) {
	// 使用适配器直接测试完整的请求转换流程
	handler := func(ctx *fasthttp.RequestCtx) {
		ctx.SetStatusCode(fasthttp.StatusOK)
		ctx.Response.Header.Set("Content-Type", "application/json")
		ctx.Response.Header.Set("X-Request-ID", "test-id")
		ctx.WriteString(`{"message":"hello","timestamp":1234567890}`)
	}

	adapter := NewFastHTTPHandlerAdapter(handler)

	// 模拟经过 HPACK 编码后的 HTTP/2 请求
	req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "BenchmarkClient/1.0")
	req.Header.Set("X-Request-ID", "bench-req-001")

	rec := httptest.NewRecorder()

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		rec.Body.Reset()
		adapter.ServeHTTP(rec, req)
	}
}

// BenchmarkHTTP2RequestRoundTrip_WithBody 测试带请求体的完整往返
//
// 模拟 POST 请求，包含 JSON 请求体。
func BenchmarkHTTP2RequestRoundTrip_WithBody(b *testing.B) {
	handler := func(ctx *fasthttp.RequestCtx) {
		ctx.SetStatusCode(fasthttp.StatusOK)
		ctx.Response.Header.Set("Content-Type", "application/json")
		ctx.Write(ctx.PostBody())
	}

	adapter := NewFastHTTPHandlerAdapter(handler)

	body := []byte(`{"action":"update","data":{"id":12345,"name":"benchmark test","values":[1,2,3,4,5]}}`)

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/update", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.ContentLength = int64(len(body))

		rec := httptest.NewRecorder()
		adapter.ServeHTTP(rec, req)
	}
}

// BenchmarkHTTP2RequestRoundTrip_WithBody_Parallel 测试并发带请求体的完整往返
//
// 使用 b.RunParallel 模拟高并发 API 请求场景。
func BenchmarkHTTP2RequestRoundTrip_WithBody_Parallel(b *testing.B) {
	handler := func(ctx *fasthttp.RequestCtx) {
		ctx.SetStatusCode(fasthttp.StatusOK)
		ctx.Response.Header.Set("Content-Type", "application/json")
		ctx.Write(ctx.PostBody())
	}

	adapter := NewFastHTTPHandlerAdapter(handler)

	body := []byte(`{"action":"batch","items":[{"id":1},{"id":2},{"id":3}]}`)

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/batch", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.ContentLength = int64(len(body))

			rec := httptest.NewRecorder()
			adapter.ServeHTTP(rec, req)
		}
	})
}

// BenchmarkHTTP2SettingsValidation 测试设置验证性能
//
// 测量 ValidateSettings 函数的开销，用于了解
// 配置验证阶段的性能成本。
func BenchmarkHTTP2SettingsValidation(b *testing.B) {
	validSettings := Settings{
		HeaderTableSize:      4096,
		EnablePush:           true,
		MaxConcurrentStreams: 250,
		InitialWindowSize:    65535,
		MaxFrameSize:         16384,
		MaxHeaderListSize:    1048576,
	}

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		if err := ValidateSettings(validSettings); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkHTTP2AdapterWithHPACKHeaders 测试带 HPACK 编码头部的适配器性能
//
// 模拟 HTTP/2 场景下经过 HPACK 压缩和解压缩后的头部转换开销。
func BenchmarkHTTP2AdapterWithHPACKHeaders(b *testing.B) {
	handler := func(ctx *fasthttp.RequestCtx) {
		ctx.SetStatusCode(fasthttp.StatusOK)
		ctx.WriteString("OK")
	}

	adapter := NewFastHTTPHandlerAdapter(handler)

	// 预编码头部（模拟 HPACK 解码后）
	methods := []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete}
	paths := []string{"/api/v1/users", "/api/v1/orders", "/api/v1/products", "/api/v1/settings"}

	b.ResetTimer()
	b.ReportAllocs()

	var idx int
	for b.Loop() {
		method := methods[idx%len(methods)]
		path := paths[idx%len(paths)]
		idx++

		req := httptest.NewRequest(method, path, nil)
		req.Header.Set(":authority", "api.example.com")
		req.Header.Set("x-http2-stream-id", "1")

		rec := httptest.NewRecorder()
		adapter.ServeHTTP(rec, req)
	}
}
