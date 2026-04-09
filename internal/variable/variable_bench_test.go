// Package variable 提供变量模块的基准测试。
//
// 该文件测试变量展开和 Pool 操作的性能。
//
// 作者：xfy
package variable

import (
	"net"
	"testing"

	"github.com/valyala/fasthttp"
)

// setupBenchmarkRequestCtx 创建用于基准测试的 fasthttp.RequestCtx。
func setupBenchmarkRequestCtx() *fasthttp.RequestCtx {
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod(fasthttp.MethodGet)
	ctx.Request.SetRequestURI("/test/path?foo=bar&baz=qux")
	ctx.Request.Header.SetHost("example.com")
	ctx.Init(&fasthttp.Request{}, &net.TCPAddr{
		IP:   net.ParseIP("192.168.1.100"),
		Port: 12345,
	}, nil)
	return ctx
}

// BenchmarkVariableExpandSimple 测试简单模板展开性能。
//
// 模板: "$remote_addr - $request_method"
func BenchmarkVariableExpandSimple(b *testing.B) {
	ctx := setupBenchmarkRequestCtx()
	vc := NewVariableContext(ctx)
	defer ReleaseVariableContext(vc)

	template := "$remote_addr - $request_method"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		vc.Expand(template)
	}
}

// BenchmarkVariableExpandComplex 测试复杂模板展开性能。
//
// 模拟 Nginx combined 日志格式:
// "$remote_addr - [$time_local] \"$request_method $uri $args\" $status $body_bytes_sent"
func BenchmarkVariableExpandComplex(b *testing.B) {
	ctx := setupBenchmarkRequestCtx()
	vc := NewVariableContext(ctx)
	vc.SetResponseInfo(200, 1024, 1000000) // status, bodySize, durationNs
	defer ReleaseVariableContext(vc)

	template := "$remote_addr - [$time_local] \"$request_method $uri $args\" $status $body_bytes_sent"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		vc.Expand(template)
	}
}

// BenchmarkVariableExpandMixed 测试混合 ${var} 和 $var 格式的展开性能。
func BenchmarkVariableExpandMixed(b *testing.B) {
	ctx := setupBenchmarkRequestCtx()
	vc := NewVariableContext(ctx)
	defer ReleaseVariableContext(vc)

	template := "${remote_addr} - $request_method ${uri}?${args}"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		vc.Expand(template)
	}
}

// BenchmarkVariableExpandNoVar 测试无变量模板的性能（快速路径）。
func BenchmarkVariableExpandNoVar(b *testing.B) {
	ctx := setupBenchmarkRequestCtx()
	vc := NewVariableContext(ctx)
	defer ReleaseVariableContext(vc)

	template := "This is a plain string with no variables"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		vc.Expand(template)
	}
}

// BenchmarkVariableContextPool 测试 Pool 获取释放性能。
func BenchmarkVariableContextPool(b *testing.B) {
	ctx := setupBenchmarkRequestCtx()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		vc := NewVariableContext(ctx)
		ReleaseVariableContext(vc)
	}
}

// BenchmarkVariableContextPoolParallel 测试并发 Pool 获取释放性能。
func BenchmarkVariableContextPoolParallel(b *testing.B) {
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		ctx := setupBenchmarkRequestCtx()
		for pb.Next() {
			vc := NewVariableContext(ctx)
			ReleaseVariableContext(vc)
		}
	})
}

// BenchmarkVariableGetCache 测试内置变量缓存命中性能。
//
// 首次获取变量会求值并缓存，后续获取命中缓存。
func BenchmarkVariableGetCache(b *testing.B) {
	ctx := setupBenchmarkRequestCtx()
	vc := NewVariableContext(ctx)
	defer ReleaseVariableContext(vc)

	// 预热缓存
	_, _ = vc.Get("remote_addr")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		vc.Get("remote_addr")
	}
}

// BenchmarkVariableGetNoCache 测试内置变量首次求值性能。
//
// 每次循环创建新的 VariableContext，模拟首次求值场景。
func BenchmarkVariableGetNoCache(b *testing.B) {
	ctx := setupBenchmarkRequestCtx()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		vc := NewVariableContext(ctx)
		vc.Get("remote_addr")
		ReleaseVariableContext(vc)
	}
}

// BenchmarkVariableGetMultiple 测试获取多个内置变量的性能。
func BenchmarkVariableGetMultiple(b *testing.B) {
	ctx := setupBenchmarkRequestCtx()
	vc := NewVariableContext(ctx)
	defer ReleaseVariableContext(vc)

	vars := []string{
		"remote_addr", "request_method", "uri", "args",
		"host", "request_uri", "scheme", "time_local",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, name := range vars {
			vc.Get(name)
		}
	}
}

// BenchmarkVariableSetAndGet 测试设置和获取自定义变量的性能。
func BenchmarkVariableSetAndGet(b *testing.B) {
	ctx := setupBenchmarkRequestCtx()
	vc := NewVariableContext(ctx)
	defer ReleaseVariableContext(vc)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		vc.Set("custom_var", "custom_value")
		vc.Get("custom_var")
	}
}

// BenchmarkExpandStringStaticWithLookup 测试静态展开函数的性能（使用自定义查找函数）。
func BenchmarkExpandStringStaticWithLookup(b *testing.B) {
	template := "$remote_addr - $request_method"
	lookup := func(name string) string {
		switch name {
		case "remote_addr":
			return "192.168.1.100"
		case "request_method":
			return "GET"
		default:
			return ""
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ExpandString(template, lookup)
	}
}

// BenchmarkVariableExpandLongTemplate 测试长模板展开性能。
//
// 模拟完整访问日志格式，约 200 字符。
func BenchmarkVariableExpandLongTemplate(b *testing.B) {
	ctx := setupBenchmarkRequestCtx()
	vc := NewVariableContext(ctx)
	vc.SetResponseInfo(200, 4096, 15000000)
	vc.SetServerName("api.example.com")
	defer ReleaseVariableContext(vc)

	template := "$remote_addr - [$time_local] \"$request_method $uri?$args\" $status $body_bytes_sent \"$http_referer\" \"$http_user_agent\" $request_time $server_name"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		vc.Expand(template)
	}
}
