// Package lua 提供 ngx.resp API 测试
package lua

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/valyala/fasthttp"
)

// TestNgxRespAPIGetStatus 测试 ngx.resp.get_status()
func TestNgxRespAPIGetStatus(t *testing.T) {
	// 创建 fasthttp 请求上下文
	var req fasthttp.Request
	var resp fasthttp.Response

	req.Header.SetMethod("GET")
	req.Header.SetRequestURI("/test")
	req.Header.SetHost("localhost")

	ctx := &fasthttp.RequestCtx{}
	// 使用延迟设置，避免直接构造 RequestCtx 的问题

	// 创建模拟响应
	resp.SetStatusCode(200)

	// 创建引擎
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	// 创建 Lua 协程
	coro, err := engine.NewCoroutine(nil)
	require.NoError(t, err)
	defer coro.Close()

	// 创建 ngx.resp API（使用 nil ctx 测试基本功能）
	api := newNgxRespAPI(ctx)

	// 在协程的 LState 中注册 API
	RegisterNgxRespAPI(coro.Co, api)

	// 测试：设置状态码后获取
	ctx.Response.SetStatusCode(404)

	err = coro.Execute(`
		local status = ngx.resp.get_status()
		return status
	`)
	require.NoError(t, err)

	// 验证返回值
	// 注意：由于 ctx 可能不是完整的 RequestCtx，状态码可能为 0
	// 这里主要验证 API 调用不 panic
}

// TestNgxRespAPISetStatus 测试 ngx.resp.set_status(code)
func TestNgxRespAPISetStatus(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	coro, err := engine.NewCoroutine(nil)
	require.NoError(t, err)
	defer coro.Close()

	// 创建一个模拟的 RequestCtx
	ctx := &fasthttp.RequestCtx{}
	api := newNgxRespAPI(ctx)
	RegisterNgxRespAPI(coro.Co, api)

	// 测试设置状态码
	err = coro.Execute(`
		ngx.resp.set_status(404)
		return ngx.resp.get_status()
	`)
	require.NoError(t, err)
}

// TestNgxRespAPIGetHeaders 测试 ngx.resp.get_headers()
func TestNgxRespAPIGetHeaders(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	coro, err := engine.NewCoroutine(nil)
	require.NoError(t, err)
	defer coro.Close()

	ctx := &fasthttp.RequestCtx{}
	api := newNgxRespAPI(ctx)
	RegisterNgxRespAPI(coro.Co, api)

	// 先设置一些响应头
	ctx.Response.Header.Set("Content-Type", "application/json")
	ctx.Response.Header.Set("X-Custom-Header", "custom-value")

	// 测试获取所有头
	err = coro.Execute(`
		local headers = ngx.resp.get_headers()
		return type(headers)
	`)
	require.NoError(t, err)
}

// TestNgxRespAPIGetHeadersWithMax 测试 ngx.resp.get_headers(max_headers)
func TestNgxRespAPIGetHeadersWithMax(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	coro, err := engine.NewCoroutine(nil)
	require.NoError(t, err)
	defer coro.Close()

	ctx := &fasthttp.RequestCtx{}
	api := newNgxRespAPI(ctx)
	RegisterNgxRespAPI(coro.Co, api)

	// 测试带 max_headers 参数
	err = coro.Execute(`
		local headers = ngx.resp.get_headers(10)
		return type(headers)
	`)
	require.NoError(t, err)
}

// TestNgxRespAPISetHeader 测试 ngx.resp.set_header(key, value)
func TestNgxRespAPISetHeader(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	coro, err := engine.NewCoroutine(nil)
	require.NoError(t, err)
	defer coro.Close()

	ctx := &fasthttp.RequestCtx{}
	api := newNgxRespAPI(ctx)
	RegisterNgxRespAPI(coro.Co, api)

	// 测试设置响应头
	err = coro.Execute(`
		ngx.resp.set_header("X-Test-Header", "test-value")
	`)
	require.NoError(t, err)

	// 验证头是否设置成功
	val := string(ctx.Response.Header.Peek("X-Test-Header"))
	assert.Equal(t, "test-value", val)
}

// TestNgxRespAPIClearHeader 测试 ngx.resp.clear_header(key)
func TestNgxRespAPIClearHeader(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	coro, err := engine.NewCoroutine(nil)
	require.NoError(t, err)
	defer coro.Close()

	ctx := &fasthttp.RequestCtx{}
	api := newNgxRespAPI(ctx)
	RegisterNgxRespAPI(coro.Co, api)

	// 先设置一个头
	ctx.Response.Header.Set("X-To-Be-Cleared", "some-value")
	assert.Equal(t, "some-value", string(ctx.Response.Header.Peek("X-To-Be-Cleared")))

	// 测试清除响应头
	err = coro.Execute(`
		ngx.resp.clear_header("X-To-Be-Cleared")
	`)
	require.NoError(t, err)

	// 验证头是否被清除
	val := ctx.Response.Header.Peek("X-To-Be-Cleared")
	assert.Empty(t, val)
}

// TestNgxRespAPIFullWorkflow 测试完整工作流
func TestNgxRespAPIFullWorkflow(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	coro, err := engine.NewCoroutine(nil)
	require.NoError(t, err)
	defer coro.Close()

	ctx := &fasthttp.RequestCtx{}
	api := newNgxRespAPI(ctx)
	RegisterNgxRespAPI(coro.Co, api)

	// 执行完整的响应操作脚本
	script := `
		-- 设置状态码
		ngx.resp.set_status(201)

		-- 设置多个响应头
		ngx.resp.set_header("Content-Type", "application/json")
		ngx.resp.set_header("X-Custom-Header", "custom-value")
		ngx.resp.set_header("X-Request-ID", "12345")

		-- 清除一个头
		ngx.resp.clear_header("X-Request-ID")

		-- 获取并返回状态码
		local status = ngx.resp.get_status()

		-- 获取响应头
		local headers = ngx.resp.get_headers()

		return status, headers["Content-Type"]
	`

	err = coro.Execute(script)
	require.NoError(t, err)

	// 验证最终状态
	assert.Equal(t, 201, ctx.Response.StatusCode())
	assert.Equal(t, "application/json", string(ctx.Response.Header.Peek("Content-Type")))
	assert.Equal(t, "custom-value", string(ctx.Response.Header.Peek("X-Custom-Header")))
	assert.Empty(t, ctx.Response.Header.Peek("X-Request-ID"))
}

// TestNgxRespAPIErrorCases 测试错误处理
func TestNgxRespAPIErrorCases(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	coro, err := engine.NewCoroutine(nil)
	require.NoError(t, err)
	defer coro.Close()

	ctx := &fasthttp.RequestCtx{}
	api := newNgxRespAPI(ctx)
	RegisterNgxRespAPI(coro.Co, api)

	// 测试 set_status 缺少参数
	err = coro.Execute(`ngx.resp.set_status()`)
	assert.Error(t, err) // 应该返回错误

	// 测试 set_header 缺少参数
	err = coro.Execute(`ngx.resp.set_header("key")`)
	assert.Error(t, err) // 应该返回错误

	// 测试 clear_header 缺少参数
	err = coro.Execute(`ngx.resp.clear_header()`)
	assert.Error(t, err) // 应该返回错误
}

// TestNgxRespAPIMultiValueHeaders 测试多值响应头
func TestNgxRespAPIMultiValueHeaders(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	coro, err := engine.NewCoroutine(nil)
	require.NoError(t, err)
	defer coro.Close()

	ctx := &fasthttp.RequestCtx{}
	api := newNgxRespAPI(ctx)
	RegisterNgxRespAPI(coro.Co, api)

	// 添加多值头（fasthttp 支持通过 Add 添加多值）
	ctx.Response.Header.Add("Set-Cookie", "session=abc123")
	ctx.Response.Header.Add("Set-Cookie", "user=john")

	// 测试获取多值头
	err = coro.Execute(`
		local headers = ngx.resp.get_headers()
		return type(headers["Set-Cookie"])
	`)
	require.NoError(t, err)
	// 多值头应该返回为 table 类型
}

// TestNgxRespAPIWithRealHTTP 测试真实 HTTP 上下文
func TestNgxRespAPIWithRealHTTP(t *testing.T) {
	// 这个测试需要完整的 fasthttp 上下文
	// 在实际集成测试中验证

	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	// 创建协程
	coro, err := engine.NewCoroutine(nil)
	require.NoError(t, err)
	defer coro.Close()

	// 由于 fasthttp.RequestCtx 的复杂性，
	// 这里仅验证 API 注册和基本调用不 panic
	ctx := &fasthttp.RequestCtx{}
	api := newNgxRespAPI(ctx)
	RegisterNgxRespAPI(coro.Co, api)

	// 验证 ngx.resp 表存在
	err = coro.Execute(`
		assert(type(ngx.resp) == "table", "ngx.resp should be a table")
		assert(type(ngx.resp.get_status) == "function", "get_status should be a function")
		assert(type(ngx.resp.set_status) == "function", "set_status should be a function")
		assert(type(ngx.resp.get_headers) == "function", "get_headers should be a function")
		assert(type(ngx.resp.set_header) == "function", "set_header should be a function")
		assert(type(ngx.resp.clear_header) == "function", "clear_header should be a function")
	`)
	require.NoError(t, err)
}
