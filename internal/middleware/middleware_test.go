// Package middleware 提供中间件链功能的测试。
//
// 该文件测试中间件链模块的各项功能，包括：
//   - 中间件链创建
//   - 空链处理
//   - 单中间件包装
//   - 多中间件执行顺序
//   - 中间件修改响应
//
// 作者：xfy
package middleware

import (
	"reflect"
	"testing"

	"github.com/valyala/fasthttp"
)

// testMiddleware 测试用中间件，记录执行顺序
type testMiddleware struct {
	name  string
	order *[]string
}

func (m *testMiddleware) Name() string {
	return m.name
}

func (m *testMiddleware) Process(next fasthttp.RequestHandler) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		*m.order = append(*m.order, m.name+"-enter")
		next(ctx)
		*m.order = append(*m.order, m.name+"-exit")
	}
}

// TestEmptyChain 测试空链直接返回原 handler
func TestEmptyChain(t *testing.T) {
	chain := NewChain()
	executed := false
	final := func(_ *fasthttp.RequestCtx) {
		executed = true
	}

	handler := chain.Apply(final)
	if handler == nil {
		t.Fatal("Apply returned nil handler for empty chain")
	}

	// 调用 handler
	var ctx fasthttp.RequestCtx
	handler(&ctx)

	if !executed {
		t.Error("final handler was not called")
	}
}

// TestSingleMiddleware 测试单中间件包装
func TestSingleMiddleware(t *testing.T) {
	var order []string

	mw := &testMiddleware{name: "mw1", order: &order}
	chain := NewChain(mw)

	final := func(_ *fasthttp.RequestCtx) {
		order = append(order, "final")
	}

	handler := chain.Apply(final)

	var ctx fasthttp.RequestCtx
	handler(&ctx)

	expected := []string{"mw1-enter", "final", "mw1-exit"}
	if !reflect.DeepEqual(order, expected) {
		t.Errorf("execution order = %v, want %v", order, expected)
	}
}

// TestMultipleMiddlewareOrder 测试多中间件逆序包装
// 逆序包装：最后添加的最先包装 final，因此执行顺序为 mw1 -> mw2 -> mw3 -> final -> mw3 -> mw2 -> mw1
// 即第一个添加的中间件最外层，最后添加的最内层
func TestMultipleMiddlewareOrder(t *testing.T) {
	var order []string

	mw1 := &testMiddleware{name: "mw1", order: &order}
	mw2 := &testMiddleware{name: "mw2", order: &order}
	mw3 := &testMiddleware{name: "mw3", order: &order}

	// 添加顺序：mw1, mw2, mw3
	chain := NewChain(mw1, mw2, mw3)

	final := func(_ *fasthttp.RequestCtx) {
		order = append(order, "final")
	}

	handler := chain.Apply(final)

	var ctx fasthttp.RequestCtx
	handler(&ctx)

	// 逆序包装：mw1 最外层，mw3 最内层
	expected := []string{
		"mw1-enter",
		"mw2-enter",
		"mw3-enter",
		"final",
		"mw3-exit",
		"mw2-exit",
		"mw1-exit",
	}

	if !reflect.DeepEqual(order, expected) {
		t.Errorf("execution order = %v, want %v", order, expected)
	}
}

// TestMiddlewareCanModifyResponse 测试中间件可修改响应
func TestMiddlewareCanModifyResponse(t *testing.T) {
	modifyingMiddleware := &modifyMiddleware{}

	chain := NewChain(modifyingMiddleware)

	final := func(ctx *fasthttp.RequestCtx) {
		ctx.SetBodyString("original")
	}

	handler := chain.Apply(final)

	var ctx fasthttp.RequestCtx
	handler(&ctx)

	body := string(ctx.Response.Body())
	expected := "original-modified"
	if body != expected {
		t.Errorf("response body = %q, want %q", body, expected)
	}
}

// modifyMiddleware 修改响应的中间件
type modifyMiddleware struct{}

func (m *modifyMiddleware) Name() string {
	return "modify"
}

func (m *modifyMiddleware) Process(next fasthttp.RequestHandler) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		next(ctx)
		// 在响应后追加内容
		ctx.SetBodyString(string(ctx.Response.Body()) + "-modified")
	}
}
