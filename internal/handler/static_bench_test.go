// Package handler 提供静态文件处理器的基准测试。
//
// 该文件测试文件查找、缓存命中/未命中、try_files 等场景的性能。
//
// 作者：xfy
package handler

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/cache"
)

// setupStaticTestDir 创建临时测试目录。
func setupStaticTestDir() (string, func()) {
	dir, err := os.MkdirTemp("", "static_bench_*")
	if err != nil {
		panic(err)
	}

	// 创建测试文件
	testFiles := map[string][]byte{
		"index.html":     []byte("<html><body>Index</body></html>"),
		"style.css":      make([]byte, 1024),    // 1KB
		"large.json":     make([]byte, 10*1024), // 10KB
		"nested/file.js": make([]byte, 5*1024),  // 5KB
	}

	for path, content := range testFiles {
		fullPath := filepath.Join(dir, path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			panic(err)
		}
		if err := os.WriteFile(fullPath, content, 0o644); err != nil {
			panic(err)
		}
	}

	cleanup := func() {
		_ = os.RemoveAll(dir)
	}

	return dir, cleanup
}

// BenchmarkStaticFileLookup 测试文件路径查找性能。
func BenchmarkStaticFileLookup(b *testing.B) {
	dir, cleanup := setupStaticTestDir()
	defer cleanup()

	handler := NewStaticHandler(dir, "/", []string{"index.html"}, false)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.SetRequestURI("/style.css")
		handler.Handle(ctx)
	}
}

// BenchmarkStaticFileCacheHit 测试缓存命中场景性能。
func BenchmarkStaticFileCacheHit(b *testing.B) {
	dir, cleanup := setupStaticTestDir()
	defer cleanup()

	fc := cache.NewFileCache(1000, 10*1024*1024, 0) // 1000 文件或 10MB
	handler := NewStaticHandler(dir, "/", []string{"index.html"}, false)
	handler.SetFileCache(fc)

	// 预热缓存
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/style.css")
	handler.Handle(ctx)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.SetRequestURI("/style.css")
		handler.Handle(ctx)
	}
}

// BenchmarkStaticFileCacheMiss_1KB 测试 1KB 文件缓存未命中场景性能。
func BenchmarkStaticFileCacheMiss_1KB(b *testing.B) {
	dir, cleanup := setupStaticTestDir()
	defer cleanup()

	handler := NewStaticHandler(dir, "/", []string{"index.html"}, false)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.SetRequestURI("/style.css")
		handler.Handle(ctx)
	}
}

// BenchmarkStaticFileCacheMiss_10KB 测试 10KB 文件缓存未命中场景性能。
func BenchmarkStaticFileCacheMiss_10KB(b *testing.B) {
	dir, cleanup := setupStaticTestDir()
	defer cleanup()

	handler := NewStaticHandler(dir, "/", []string{"index.html"}, false)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.SetRequestURI("/large.json")
		handler.Handle(ctx)
	}
}

// BenchmarkStaticTryFiles 测试 try_files 查找性能。
func BenchmarkStaticTryFiles(b *testing.B) {
	dir, cleanup := setupStaticTestDir()
	defer cleanup()

	handler := NewStaticHandler(dir, "/", []string{"index.html"}, false)
	handler.SetTryFiles([]string{"$uri", "$uri/", "/index.html"}, false, nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.SetRequestURI("/nonexistent/path")
		handler.Handle(ctx)
	}
}

// BenchmarkStaticIndex 测试索引文件查找性能。
func BenchmarkStaticIndex(b *testing.B) {
	dir, cleanup := setupStaticTestDir()
	defer cleanup()

	handler := NewStaticHandler(dir, "/", []string{"index.html", "index.htm"}, false)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.SetRequestURI("/")
		handler.Handle(ctx)
	}
}

// BenchmarkStaticNestedFile 测试嵌套文件查找性能。
func BenchmarkStaticNestedFile(b *testing.B) {
	dir, cleanup := setupStaticTestDir()
	defer cleanup()

	handler := NewStaticHandler(dir, "/", []string{"index.html"}, false)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.SetRequestURI("/nested/file.js")
		handler.Handle(ctx)
	}
}

// BenchmarkStaticFileNotFound 测试文件未找到场景性能。
func BenchmarkStaticFileNotFound(b *testing.B) {
	dir, cleanup := setupStaticTestDir()
	defer cleanup()

	handler := NewStaticHandler(dir, "/", []string{"index.html"}, false)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.SetRequestURI("/nonexistent/file.txt")
		handler.Handle(ctx)
	}
}

// BenchmarkStaticWithCacheParallel 测试带缓存的并发访问性能。
func BenchmarkStaticWithCacheParallel(b *testing.B) {
	dir, cleanup := setupStaticTestDir()
	defer cleanup()

	fc := cache.NewFileCache(1000, 10*1024*1024, 0)
	handler := NewStaticHandler(dir, "/", []string{"index.html"}, false)
	handler.SetFileCache(fc)

	paths := []string{"/style.css", "/large.json", "/nested/file.js", "/index.html"}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			ctx := &fasthttp.RequestCtx{}
			ctx.Request.SetRequestURI(paths[i%len(paths)])
			handler.Handle(ctx)
			i++
		}
	})
}

// BenchmarkStaticFileLookupWithAlias 测试 alias 模式下的文件查找性能。
func BenchmarkStaticFileLookupWithAlias(b *testing.B) {
	dir, cleanup := setupStaticTestDir()
	defer cleanup()

	handler := NewStaticHandlerWithAlias(dir+"/", "/static/", []string{"index.html"}, false)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.SetRequestURI("/static/style.css")
		handler.Handle(ctx)
	}
}
