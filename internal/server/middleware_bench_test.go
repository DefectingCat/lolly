// Package server 提供中间件性能基准测试
//
// 该文件测试中间件链模块的性能，包括：
//   - 创建中间件链的开销
//   - Process 包装的开销
//   - 完整链执行的开销
//
// 用于评估中间件链在不同场景下的性能表现
package server

import (
	"testing"

	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/middleware"
)

// noopMiddleware 是一个空的中间件实现，用于基准测试
// 只记录进入和退出，不做任何实际操作
type noopMiddleware struct {
	name string
}

// Name 返回中间件名称
func (m *noopMiddleware) Name() string {
	return m.name
}

// Process 包装下一个请求处理器
// 直接调用下一个处理器，不做任何处理
func (m *noopMiddleware) Process(next fasthttp.RequestHandler) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		next(ctx)
	}
}

// BenchmarkMiddlewareNewChainApply 测试创建中间件链和应用的开销
//
// 该基准测试测量：
//   - 使用 NewChain() 创建链的开销
//   - 使用 Apply() 应用中间件链到最终处理器的开销
//
// 测试场景：包含 3 个中间件的链
func BenchmarkMiddlewareNewChainApply(b *testing.B) {
	// 创建 3 个空中间件
	mw1 := &noopMiddleware{name: "mw1"}
	mw2 := &noopMiddleware{name: "mw2"}
	mw3 := &noopMiddleware{name: "mw3"}

	// 最终处理器
	finalHandler := func(ctx *fasthttp.RequestCtx) {
		ctx.WriteString("ok") // nolint:errcheck
	}

	b.ResetTimer()
	for b.Loop() {
		// 创建链并应用
		chain := middleware.NewChain(mw1, mw2, mw3)
		_ = chain.Apply(finalHandler)
	}
}

// BenchmarkMiddlewareProcessChain 测试 Process 包装的开销
//
// 该基准测试测量单个中间件 Process 方法包装处理器的开销
// 关注单个中间件的包装性能，不涉及链的创建
func BenchmarkMiddlewareProcessChain(b *testing.B) {
	mw := &noopMiddleware{name: "benchmark"}

	// 最终处理器
	finalHandler := func(ctx *fasthttp.RequestCtx) {
		ctx.WriteString("ok") // nolint:errcheck
	}

	b.ResetTimer()
	for b.Loop() {
		// 只测量 Process 包装的开销
		_ = mw.Process(finalHandler)
	}
}

// BenchmarkMiddlewareChainExecution 测试完整中间件链的执行开销
//
// 该基准测试测量：
//   - 预创建的中间件链的执行性能
//   - 多中间件嵌套调用的开销
//   - 空 handler 情况下的链遍历成本
//
// 测试场景：3 个中间件的完整链执行
func BenchmarkMiddlewareChainExecution(b *testing.B) {
	// 创建 3 个空中间件
	mw1 := &noopMiddleware{name: "mw1"}
	mw2 := &noopMiddleware{name: "mw2"}
	mw3 := &noopMiddleware{name: "mw3"}

	// 创建链并应用
	chain := middleware.NewChain(mw1, mw2, mw3)

	// 最终处理器（空操作）
	finalHandler := func(ctx *fasthttp.RequestCtx) {
		// 空 handler，不做任何操作
	}

	handler := chain.Apply(finalHandler)
	ctx := &fasthttp.RequestCtx{}

	b.ResetTimer()
	for b.Loop() {
		// 执行完整的中间件链
		handler(ctx)
	}
}

// BenchmarkMiddlewareChainExecutionWithResponse 测试带响应的完整链执行
//
// 与 BenchmarkMiddlewareChainExecution 类似，但包含实际的响应写入操作
// 更接近实际使用场景
func BenchmarkMiddlewareChainExecutionWithResponse(b *testing.B) {
	mw1 := &noopMiddleware{name: "mw1"}
	mw2 := &noopMiddleware{name: "mw2"}
	mw3 := &noopMiddleware{name: "mw3"}

	chain := middleware.NewChain(mw1, mw2, mw3)

	finalHandler := func(ctx *fasthttp.RequestCtx) {
		ctx.WriteString("response") // nolint:errcheck
	}

	handler := chain.Apply(finalHandler)

	b.ResetTimer()
	for b.Loop() {
		ctx := &fasthttp.RequestCtx{}
		handler(ctx)
	}
}

// BenchmarkMiddlewareEmptyChain 测试空中间件链的性能
//
// 作为对照组，测量没有任何中间件时的基础开销
func BenchmarkMiddlewareEmptyChain(b *testing.B) {
	chain := middleware.NewChain()

	finalHandler := func(ctx *fasthttp.RequestCtx) {
		ctx.WriteString("ok") // nolint:errcheck
	}

	handler := chain.Apply(finalHandler)
	ctx := &fasthttp.RequestCtx{}

	b.ResetTimer()
	for b.Loop() {
		handler(ctx)
	}
}

// BenchmarkMiddlewareSingleMiddleware 测试单个中间件的开销
//
// 测量只有一个中间件时的性能，用于对比多中间件场景
func BenchmarkMiddlewareSingleMiddleware(b *testing.B) {
	mw := &noopMiddleware{name: "single"}
	chain := middleware.NewChain(mw)

	finalHandler := func(ctx *fasthttp.RequestCtx) {
		ctx.WriteString("ok") // nolint:errcheck
	}

	handler := chain.Apply(finalHandler)
	ctx := &fasthttp.RequestCtx{}

	b.ResetTimer()
	for b.Loop() {
		handler(ctx)
	}
}
