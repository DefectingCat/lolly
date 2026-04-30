// Package compression 提供压缩 writer 池化效果测试。
//
// 该文件对比新建 vs 池化复用的分配差异。
//
// 测试场景：
//   - NewWriter: 每次新建 gzip.Writer
//   - PoolWriter: 从 sync.Pool 获取复用
//
// 目标：池化后 ≤2 allocs/op
//
// 作者：xfy
package compression

import (
	"bytes"
	"compress/gzip"
	"sync"
	"testing"

	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/benchmark/tools"
	"rua.plus/lolly/internal/config"
)

// BenchmarkGzipPool_GetPut 测试 gzip.Writer 池直接操作。
//
// 验证 sync.Pool 的分配效果。
func BenchmarkGzipPool_GetPut(b *testing.B) {
	pool := &sync.Pool{
		New: func() any {
			return gzip.NewWriter(nil)
		},
	}

	buf := bytes.NewBuffer(nil)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		w := pool.Get().(*gzip.Writer)
		w.Reset(buf)
		// 模拟使用
		_, _ = w.Write([]byte("test data"))
		_ = w.Close()
		buf.Reset()
		pool.Put(w)
	}
}

// BenchmarkGzipWriter_New 测试每次新建 gzip.Writer。
//
// 对比基准：每次创建新 Writer。
func BenchmarkGzipWriter_New(b *testing.B) {
	data := tools.GenerateTestData(tools.Size1KB)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		buf := bytes.NewBuffer(nil)
		w := gzip.NewWriter(buf)
		_, _ = w.Write(data)
		_ = w.Close()
	}
}

// BenchmarkGzipWriter_Pool 测试池化复用 gzip.Writer。
//
// 目标：≤2 allocs/op
func BenchmarkGzipWriter_Pool(b *testing.B) {
	data := tools.GenerateTestData(tools.Size1KB)

	cfg := &config.CompressionConfig{
		Type:    "gzip",
		Level:   6,
		MinSize: 100,
		Types:   []string{"application/json"},
	}
	mw, _ := New(cfg)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = mw.compressWithPool(data, mw.gzipPool)
	}
}

// BenchmarkCompressionMiddleware_Pool 测试中间件池化效果。
//
// 模拟完整请求处理路径。
func BenchmarkCompressionMiddleware_Pool(b *testing.B) {
	cfg := &config.CompressionConfig{
		Type:    "gzip",
		Level:   6,
		MinSize: 100,
		Types:   []string{"text/html", "application/json"},
	}
	mw, _ := New(cfg)

	// 创建测试响应
	data := tools.GenerateTestData(tools.Size10KB)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.Header.SetMethod(fasthttp.MethodGet)
		ctx.Request.SetRequestURI("/test")
		ctx.Request.Header.Set("Accept-Encoding", "gzip")
		ctx.Response.Header.Set("Content-Type", "application/json")
		ctx.Response.SetBody(data)

		// 模拟中间件处理
		handler := mw.Process(func(ctx *fasthttp.RequestCtx) {
			ctx.Response.SetBody(data)
		})
		handler(ctx)
	}
}

// BenchmarkGzipCompress_Sizes 测试不同数据大小的池化效果。
func BenchmarkGzipCompress_Sizes(b *testing.B) {
	cfg := &config.CompressionConfig{
		Type:    "gzip",
		Level:   6,
		MinSize: 100,
	}
	mw, _ := New(cfg)

	sizes := []struct {
		name string
		data []byte
	}{
		{"100B", tools.GenerateTestData(100)},
		{"1KB", tools.GenerateTestData(tools.Size1KB)},
		{"10KB", tools.GenerateTestData(tools.Size10KB)},
		{"100KB", tools.GenerateTestData(tools.Size100KB)},
	}

	for _, tc := range sizes {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				_ = mw.compressWithPool(tc.data, mw.gzipPool)
			}
		})
	}
}
