// Package rewrite URL 重写中间件基准测试
package rewrite

import (
	"testing"

	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/config"
)

// BenchmarkRewriteProcess 基准测试：正则匹配 + 替换
// 测试单个重写规则的性能
func BenchmarkRewriteProcess(b *testing.B) {
	m, err := New([]config.RewriteRule{
		{Pattern: "^/api/v1/(.*)$", Replacement: "/api/v2/$1", Flag: "last"},
	})
	if err != nil {
		b.Fatalf("New() error: %v", err)
	}

	nextHandler := func(ctx *fasthttp.RequestCtx) {
		ctx.WriteString("OK")
	}
	handler := m.Process(nextHandler)

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.SetRequestURI("/api/v1/users")
		handler(ctx)
	}
}

// BenchmarkRewriteMultipleRules 基准测试：多规则匹配（最多 10 个）
// 测试规则列表的遍历和匹配性能
func BenchmarkRewriteMultipleRules(b *testing.B) {
	// 创建 10 个规则，只有最后一个匹配
	rules := []config.RewriteRule{
		{Pattern: "^/a/(.*)$", Replacement: "/x/$1", Flag: "last"},
		{Pattern: "^/b/(.*)$", Replacement: "/x/$1", Flag: "last"},
		{Pattern: "^/c/(.*)$", Replacement: "/x/$1", Flag: "last"},
		{Pattern: "^/d/(.*)$", Replacement: "/x/$1", Flag: "last"},
		{Pattern: "^/e/(.*)$", Replacement: "/x/$1", Flag: "last"},
		{Pattern: "^/f/(.*)$", Replacement: "/x/$1", Flag: "last"},
		{Pattern: "^/g/(.*)$", Replacement: "/x/$1", Flag: "last"},
		{Pattern: "^/h/(.*)$", Replacement: "/x/$1", Flag: "last"},
		{Pattern: "^/i/(.*)$", Replacement: "/x/$1", Flag: "last"},
		{Pattern: "^/j/(.*)$", Replacement: "/x/$1", Flag: "last"},
	}

	m, err := New(rules)
	if err != nil {
		b.Fatalf("New() error: %v", err)
	}

	nextHandler := func(ctx *fasthttp.RequestCtx) {
		ctx.WriteString("OK")
	}
	handler := m.Process(nextHandler)

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.SetRequestURI("/j/resource")
		handler(ctx)
	}
}

// BenchmarkRewriteWithVariableExpand 基准测试：带变量展开
// 测试变量展开对性能的影响
func BenchmarkRewriteWithVariableExpand(b *testing.B) {
	// 在替换字符串中使用变量
	m, err := New([]config.RewriteRule{
		{Pattern: "^/api/(.*)$", Replacement: "/proxy/${host}/$1", Flag: "last"},
	})
	if err != nil {
		b.Fatalf("New() error: %v", err)
	}

	nextHandler := func(ctx *fasthttp.RequestCtx) {
		ctx.WriteString("OK")
	}
	handler := m.Process(nextHandler)

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.SetRequestURI("/api/data")
		// 设置 Host 头以供变量展开使用
		ctx.Request.Header.Set("Host", "example.com")
		handler(ctx)
	}
}

// BenchmarkRewriteFlagLast 基准测试：FlagLast 循环检测
// 测试 FlagLast 重新扫描的性能
func BenchmarkRewriteFlagLast(b *testing.B) {
	// 创建两条规则形成链式重写
	// /v1/* -> /v2/* -> /v3/*
	m, err := New([]config.RewriteRule{
		{Pattern: "^/v1/(.*)$", Replacement: "/v2/$1", Flag: "last"},
		{Pattern: "^/v2/(.*)$", Replacement: "/v3/$1", Flag: "last"},
	})
	if err != nil {
		b.Fatalf("New() error: %v", err)
	}

	nextHandler := func(ctx *fasthttp.RequestCtx) {
		ctx.WriteString("OK")
	}
	handler := m.Process(nextHandler)

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.SetRequestURI("/v1/resource")
		handler(ctx)
	}
}

// BenchmarkRewriteNoMatch 基准测试：无匹配情况
// 测试当没有规则匹配时的性能
func BenchmarkRewriteNoMatch(b *testing.B) {
	m, err := New([]config.RewriteRule{
		{Pattern: "^/api/(.*)$", Replacement: "/proxy/$1", Flag: "last"},
	})
	if err != nil {
		b.Fatalf("New() error: %v", err)
	}

	nextHandler := func(ctx *fasthttp.RequestCtx) {
		ctx.WriteString("OK")
	}
	handler := m.Process(nextHandler)

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.SetRequestURI("/static/file.txt")
		handler(ctx)
	}
}

// BenchmarkRewriteComplexPattern 基准测试：复杂正则表达式
// 测试复杂正则匹配的性能
func BenchmarkRewriteComplexPattern(b *testing.B) {
	// 使用更复杂的正则表达式
	m, err := New([]config.RewriteRule{
		{Pattern: "^/api/v\\d+/(users|posts|comments)/(\\d+)/(profile|settings)$", Replacement: "/internal/$1/$2/$3", Flag: "last"},
	})
	if err != nil {
		b.Fatalf("New() error: %v", err)
	}

	nextHandler := func(ctx *fasthttp.RequestCtx) {
		ctx.WriteString("OK")
	}
	handler := m.Process(nextHandler)

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.SetRequestURI("/api/v1/users/123/profile")
		handler(ctx)
	}
}

// BenchmarkRewriteRedirect 基准测试：重定向标志
// 测试重定向响应的性能
func BenchmarkRewriteRedirect(b *testing.B) {
	m, err := New([]config.RewriteRule{
		{Pattern: "^/old/(.*)$", Replacement: "/new/$1", Flag: "redirect"},
	})
	if err != nil {
		b.Fatalf("New() error: %v", err)
	}

	nextHandler := func(ctx *fasthttp.RequestCtx) {
		ctx.WriteString("OK")
	}
	handler := m.Process(nextHandler)

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.SetRequestURI("/old/page")
		handler(ctx)
	}
}

// BenchmarkRewriteBreak 基准测试：Break 标志
// 测试 Break 标志提前终止的性能
func BenchmarkRewriteBreak(b *testing.B) {
	m, err := New([]config.RewriteRule{
		{Pattern: "^/api/(.*)$", Replacement: "/internal/$1", Flag: "break"},
		{Pattern: "^/internal/(.*)$", Replacement: "/final/$1", Flag: "last"},
	})
	if err != nil {
		b.Fatalf("New() error: %v", err)
	}

	nextHandler := func(ctx *fasthttp.RequestCtx) {
		ctx.WriteString("OK")
	}
	handler := m.Process(nextHandler)

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.SetRequestURI("/api/data")
		handler(ctx)
	}
}

// BenchmarkRewriteMultipleCaptures 基准测试：多捕获组
// 测试多个捕获组的替换性能
func BenchmarkRewriteMultipleCaptures(b *testing.B) {
	m, err := New([]config.RewriteRule{
		{Pattern: "^/(\\d{4})/(\\d{2})/(\\d{2})/(.*)$", Replacement: "/archive/$1-$2-$3/$4", Flag: "last"},
	})
	if err != nil {
		b.Fatalf("New() error: %v", err)
	}

	nextHandler := func(ctx *fasthttp.RequestCtx) {
		ctx.WriteString("OK")
	}
	handler := m.Process(nextHandler)

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.SetRequestURI("/2024/03/15/article")
		handler(ctx)
	}
}
