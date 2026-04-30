// Package bodylimit 提供请求体大小限制中间件的基准测试。
//
// 作者：xfy
package bodylimit

import (
	"bytes"
	"testing"

	"github.com/valyala/fasthttp"
)

// BenchmarkBodyLimitProcess 基准测试限制检查（无超限情况）。
//
// 测试中间件处理正常请求（不触发限制）的性能。
func BenchmarkBodyLimitProcess(b *testing.B) {
	bl, err := New("1mb")
	if err != nil {
		b.Fatalf("创建中间件失败: %v", err)
	}

	nextHandler := func(ctx *fasthttp.RequestCtx) {
		ctx.SetStatusCode(fasthttp.StatusOK)
	}
	handler := bl.Process(nextHandler)

	// 准备请求上下文
	body := []byte("test body content")

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.Header.SetMethod("POST")
		ctx.Request.Header.SetContentLength(len(body))
		ctx.Request.SetBodyStream(bytes.NewReader(body), len(body))

		handler(ctx)
	}
}

// BenchmarkBodyLimitGetLimit 基准测试获取路径限制。
//
// 测试 GetLimit 方法的性能，该方法在每次请求时都会被调用。
func BenchmarkBodyLimitGetLimit(b *testing.B) {
	bl, err := New("1mb")
	if err != nil {
		b.Fatalf("创建中间件失败: %v", err)
	}

	// 添加多个路径配置
	paths := []string{
		"/api/v1/users",
		"/api/v1/posts",
		"/api/v2/upload",
		"/admin/settings",
		"/static/images",
	}
	limits := []string{"10kb", "100kb", "5mb", "50kb", "20mb"}

	for i, path := range paths {
		if err := bl.AddPathLimit(path, limits[i]); err != nil {
			b.Fatalf("添加路径限制失败: %v", err)
		}
	}

	testPaths := []string{
		"/api/v1/users/123",
		"/api/v2/upload/file",
		"/other/path",
		"/admin/settings/general",
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; b.Loop(); i++ {
		path := testPaths[i%len(testPaths)]
		_ = bl.GetLimit(path)
	}
}

// BenchmarkBodyLimitPathMatching 基准测试多路径配置匹配。
//
// 测试在有大量路径配置时 GetLimit 的性能（包括 RWMutex 锁开销）。
func BenchmarkBodyLimitPathMatching(b *testing.B) {
	bl, err := New("1mb")
	if err != nil {
		b.Fatalf("创建中间件失败: %v", err)
	}

	// 添加大量路径配置
	for i := range 100 {
		path := "/api/v" + string(rune('0'+i%10)) + "/resource" + string(rune('0'+i%10))
		size := "1mb"
		if i%3 == 0 {
			size = "10mb"
		} else if i%5 == 0 {
			size = "100kb"
		}
		if err := bl.AddPathLimit(path, size); err != nil {
			b.Fatalf("添加路径限制失败: %v", err)
		}
	}

	testPaths := []string{
		"/api/v1/resource1/123",
		"/api/v5/resource5/upload",
		"/other/path",
		"/api/v9/resource9/data",
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; b.Loop(); i++ {
		path := testPaths[i%len(testPaths)]
		_ = bl.GetLimit(path)
	}
}

// BenchmarkParseSize 基准测试大小字符串解析。
//
// 测试 ParseSize 函数解析各种大小字符串的性能。
func BenchmarkParseSize(b *testing.B) {
	sizes := []string{
		"1024",
		"1kb",
		"10kb",
		"1mb",
		"10mb",
		"1.5mb",
		"1gb",
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; b.Loop(); i++ {
		sizeStr := sizes[i%len(sizes)]
		_, err := ParseSize(sizeStr)
		if err != nil {
			b.Fatalf("ParseSize(%q) 失败: %v", sizeStr, err)
		}
	}
}
