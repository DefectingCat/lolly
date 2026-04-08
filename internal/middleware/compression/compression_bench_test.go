// Package compression 提供压缩中间件的基准测试。
//
// 该文件测试 gzip/brotli 压缩性能、Pool 复用效率和组件级性能。
//
// 作者：xfy
package compression

import (
	"testing"

	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/benchmark/tools"
	"rua.plus/lolly/internal/config"
)

// BenchmarkGzipCompress_1KB 测试 gzip 压缩 1KB 数据的性能。
func BenchmarkGzipCompress_1KB(b *testing.B) {
	cfg := &config.CompressionConfig{
		Type:    "gzip",
		Level:   6,
		MinSize: 100,
		Types:   []string{"text/html", "application/json"},
	}
	mw, _ := New(cfg)

	data := tools.GenerateTestData(tools.Size1KB)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mw.compressGzip(data)
	}
}

// BenchmarkGzipCompress_10KB 测试 gzip 压缩 10KB 数据的性能。
func BenchmarkGzipCompress_10KB(b *testing.B) {
	cfg := &config.CompressionConfig{
		Type:    "gzip",
		Level:   6,
		MinSize: 100,
	}
	mw, _ := New(cfg)

	data := tools.GenerateTestData(tools.Size10KB)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mw.compressGzip(data)
	}
}

// BenchmarkGzipCompress_100KB 测试 gzip 压缩 100KB 数据的性能。
func BenchmarkGzipCompress_100KB(b *testing.B) {
	cfg := &config.CompressionConfig{
		Type:    "gzip",
		Level:   6,
		MinSize: 100,
	}
	mw, _ := New(cfg)

	data := tools.GenerateTestData(tools.Size100KB)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mw.compressGzip(data)
	}
}

// BenchmarkBrotliCompress_1KB 测试 brotli 压缩 1KB 数据的性能。
func BenchmarkBrotliCompress_1KB(b *testing.B) {
	cfg := &config.CompressionConfig{
		Type:    "brotli",
		Level:   6,
		MinSize: 100,
	}
	mw, _ := New(cfg)

	data := tools.GenerateTestData(tools.Size1KB)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mw.compressBrotli(data)
	}
}

// BenchmarkBrotliCompress_10KB 测试 brotli 压缩 10KB 数据的性能。
func BenchmarkBrotliCompress_10KB(b *testing.B) {
	cfg := &config.CompressionConfig{
		Type:    "brotli",
		Level:   6,
		MinSize: 100,
	}
	mw, _ := New(cfg)

	data := tools.GenerateTestData(tools.Size10KB)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mw.compressBrotli(data)
	}
}

// BenchmarkCompressionPool 测试压缩 Pool 复用效率。
//
// 模拟实际使用场景，反复从 Pool 获取和归还压缩器。
func BenchmarkCompressionPool(b *testing.B) {
	cfg := &config.CompressionConfig{
		Type:    "gzip",
		Level:   6,
		MinSize: 100,
	}
	mw, _ := New(cfg)

	data := tools.GenerateTestData(tools.Size1KB)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mw.compressGzip(data)
	}
}

// BenchmarkCompressionMiddleware 组件级测试：测量压缩中间件本身的开销。
//
// 使用 mockHandler 排除下游处理的影响。
func BenchmarkCompressionMiddleware(b *testing.B) {
	cfg := &config.CompressionConfig{
		Type:    "gzip",
		Level:   6,
		MinSize: 100,
		Types: []string{
			"text/html", "text/css", "application/json",
		},
	}
	mw, _ := New(cfg)

	// 创建 10KB 响应数据
	responseBody := tools.GenerateTestData(tools.Size10KB)

	// Mock handler 返回固定响应
	mockHandler := func(ctx *fasthttp.RequestCtx) {
		ctx.SetStatusCode(fasthttp.StatusOK)
		ctx.SetContentType("application/json")
		ctx.SetBody(responseBody)
	}

	handler := mw.Process(mockHandler)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.Header.SetMethod(fasthttp.MethodGet)
		ctx.Request.SetRequestURI("/api/test")
		ctx.Request.Header.Set("Accept-Encoding", "gzip")
		handler(ctx)
	}
}

// BenchmarkCompressionMiddlewareNoCompress 测试无需压缩场景的性能开销。
//
// 当客户端不支持压缩时，中间件应几乎无额外开销。
func BenchmarkCompressionMiddlewareNoCompress(b *testing.B) {
	cfg := &config.CompressionConfig{
		Type:    "gzip",
		Level:   6,
		MinSize: 100,
	}
	mw, _ := New(cfg)

	responseBody := tools.GenerateTestData(tools.Size10KB)

	mockHandler := func(ctx *fasthttp.RequestCtx) {
		ctx.SetStatusCode(fasthttp.StatusOK)
		ctx.SetContentType("application/json")
		ctx.SetBody(responseBody)
	}

	handler := mw.Process(mockHandler)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.Header.SetMethod(fasthttp.MethodGet)
		ctx.Request.SetRequestURI("/api/test")
		// 不设置 Accept-Encoding 头
		handler(ctx)
	}
}

// BenchmarkIsCompressible 测试 MIME 类型检查性能。
func BenchmarkIsCompressible(b *testing.B) {
	cfg := &config.CompressionConfig{
		Type:  "gzip",
		Level: 6,
		Types: []string{
			"text/html", "text/css", "text/javascript",
			"application/json", "application/javascript",
		},
	}
	mw, _ := New(cfg)

	contentTypes := []string{
		"application/json",
		"text/html; charset=utf-8",
		"image/png",
		"application/octet-stream",
		"text/css",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, ct := range contentTypes {
			mw.isCompressible(ct)
		}
	}
}

// BenchmarkCompressionLevelComparison 比较不同压缩级别的性能。
func BenchmarkCompressionLevelComparison(b *testing.B) {
	data := tools.GenerateTestData(tools.Size10KB)

	b.Run("Level1", func(b *testing.B) {
		cfg := &config.CompressionConfig{Type: "gzip", Level: 1}
		mw, _ := New(cfg)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			mw.compressGzip(data)
		}
	})

	b.Run("Level6", func(b *testing.B) {
		cfg := &config.CompressionConfig{Type: "gzip", Level: 6}
		mw, _ := New(cfg)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			mw.compressGzip(data)
		}
	})

	b.Run("Level9", func(b *testing.B) {
		cfg := &config.CompressionConfig{Type: "gzip", Level: 9}
		mw, _ := New(cfg)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			mw.compressGzip(data)
		}
	})
}

// BenchmarkCompressionMiddlewareParallel 测试并发场景下的中间件性能。
func BenchmarkCompressionMiddlewareParallel(b *testing.B) {
	cfg := &config.CompressionConfig{
		Type:    "gzip",
		Level:   6,
		MinSize: 100,
		Types:   []string{"application/json"},
	}
	mw, _ := New(cfg)

	responseBody := tools.GenerateTestData(tools.Size10KB)

	mockHandler := func(ctx *fasthttp.RequestCtx) {
		ctx.SetStatusCode(fasthttp.StatusOK)
		ctx.SetContentType("application/json")
		ctx.SetBody(responseBody)
	}

	handler := mw.Process(mockHandler)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			ctx := &fasthttp.RequestCtx{}
			ctx.Request.Header.SetMethod(fasthttp.MethodGet)
			ctx.Request.SetRequestURI("/api/test")
			ctx.Request.Header.Set("Accept-Encoding", "gzip")
			handler(ctx)
		}
	})
}