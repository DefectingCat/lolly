// Package accesslog 提供访问日志中间件的基准测试。
//
// 该文件测试日志记录的性能开销。
//
// 作者：xfy
package accesslog

import (
	"testing"

	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/config"
)

// BenchmarkAccessLogProcess 测试访问日志中间件处理性能。
func BenchmarkAccessLogProcess(b *testing.B) {
	cfg := &config.LoggingConfig{
		Access: config.AccessLogConfig{
			Path:   "/dev/null",
			Format: "combined",
		},
	}
	al := New(cfg)
	defer func() { _ = al.Close() }()

	mockHandler := func(ctx *fasthttp.RequestCtx) {
		ctx.SetStatusCode(fasthttp.StatusOK)
		ctx.SetBodyString("Hello, World!")
	}

	handler := al.Process(mockHandler)

	b.ResetTimer()
	for b.Loop() {
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.Header.SetMethod(fasthttp.MethodGet)
		ctx.Request.SetRequestURI("/api/test")
		ctx.Request.Header.SetHost("example.com")
		handler(ctx)
	}
}

// BenchmarkAccessLogProcessParallel 测试并发场景下的访问日志性能。
func BenchmarkAccessLogProcessParallel(b *testing.B) {
	cfg := &config.LoggingConfig{
		Access: config.AccessLogConfig{
			Path:   "/dev/null",
			Format: "combined",
		},
	}
	al := New(cfg)
	defer func() { _ = al.Close() }()

	mockHandler := func(ctx *fasthttp.RequestCtx) {
		ctx.SetStatusCode(fasthttp.StatusOK)
		ctx.SetBodyString("OK")
	}

	handler := al.Process(mockHandler)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			ctx := &fasthttp.RequestCtx{}
			ctx.Request.Header.SetMethod(fasthttp.MethodGet)
			ctx.Request.SetRequestURI("/api/test")
			handler(ctx)
		}
	})
}
