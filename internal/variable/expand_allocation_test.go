// Package variable 提供变量展开分配追踪测试。
//
// 该文件追踪 $variable 展开的分配来源。
//
// 测试场景：
//   - Simple: 热点变量展开，目标 ≤1 allocs/op
//   - Complex: 多变量模板
//   - NoVar: 无变量模板（0 allocs）
//
// 作者：xfy
package variable

import (
	"net"
	"testing"

	"github.com/valyala/fasthttp"
)

// setupAllocationTestCtx 创建测试上下文。
func setupAllocationTestCtx() *fasthttp.RequestCtx {
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod(fasthttp.MethodGet)
	ctx.Request.SetRequestURI("/api/users?id=123&sort=desc")
	ctx.Request.Header.SetHost("api.example.com")
	ctx.Init(&fasthttp.Request{}, &net.TCPAddr{
		IP:   net.ParseIP("10.0.0.50"),
		Port: 54321,
	}, nil)
	return ctx
}

// BenchmarkExpandAllocation_Simple 测试热点变量展开。
//
// 目标：≤1 allocs/op
func BenchmarkExpandAllocation_Simple(b *testing.B) {
	ctx := setupAllocationTestCtx()
	vc := NewContext(ctx)
	defer ReleaseContext(vc)

	template := "$remote_addr - $request_method $uri"

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = vc.Expand(template)
	}
}

// BenchmarkExpandAllocation_SingleVar 测试单个变量展开。
//
// 热点场景：日志格式中常见的单个变量。
func BenchmarkExpandAllocation_SingleVar(b *testing.B) {
	ctx := setupAllocationTestCtx()
	vc := NewContext(ctx)
	defer ReleaseContext(vc)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = vc.Expand("$remote_addr")
	}
}

// BenchmarkExpandAllocation_TwoVars 测试两个变量展开。
func BenchmarkExpandAllocation_TwoVars(b *testing.B) {
	ctx := setupAllocationTestCtx()
	vc := NewContext(ctx)
	defer ReleaseContext(vc)

	template := "$remote_addr $request_method"

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = vc.Expand(template)
	}
}

// BenchmarkExpandAllocation_Complex 测试复杂模板。
//
// 类似 Nginx combined 日志格式。
func BenchmarkExpandAllocation_Complex(b *testing.B) {
	ctx := setupAllocationTestCtx()
	vc := NewContext(ctx)
	defer ReleaseContext(vc)

	template := "$remote_addr - [$time_local] \"$request_method $uri $args\" $status $body_bytes_sent"

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = vc.Expand(template)
	}
}

// BenchmarkExpandAllocation_NoVar 测试无变量模板。
//
// 目标：0 allocs/op
func BenchmarkExpandAllocation_NoVar(b *testing.B) {
	ctx := setupAllocationTestCtx()
	vc := NewContext(ctx)
	defer ReleaseContext(vc)

	template := "static text without variables"

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = vc.Expand(template)
	}
}

// BenchmarkExpandAllocation_Brace 测试带花括号的变量。
func BenchmarkExpandAllocation_Brace(b *testing.B) {
	ctx := setupAllocationTestCtx()
	vc := NewContext(ctx)
	defer ReleaseContext(vc)

	template := "${remote_addr} - ${request_method}"

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = vc.Expand(template)
	}
}

// BenchmarkExpandAllocation_LookupOnly 测试纯查找（无展开）。
//
// 验证 Get 分配。
func BenchmarkExpandAllocation_LookupOnly(b *testing.B) {
	ctx := setupAllocationTestCtx()
	vc := NewContext(ctx)
	defer ReleaseContext(vc)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_, _ = vc.Get("remote_addr")
	}
}

// BenchmarkExpandAllocation_ContextPool 测试 Context 池复用。
//
// 验证 NewContext + ReleaseContext 分配效果。
func BenchmarkExpandAllocation_ContextPool(b *testing.B) {
	ctx := setupAllocationTestCtx()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		vc := NewContext(ctx)
		_ = vc.Expand("$remote_addr")
		ReleaseContext(vc)
	}
}

// BenchmarkExpandAllocation_ContextReuse 测试复用同一 Context。
//
// 对比池复用 vs 每次新建。
func BenchmarkExpandAllocation_ContextReuse(b *testing.B) {
	ctx := setupAllocationTestCtx()
	vc := NewContext(ctx)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = vc.Expand("$remote_addr")
	}

	ReleaseContext(vc)
}
