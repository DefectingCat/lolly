// Package lua 提供 ngx.var API 测试
package lua

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/valyala/fasthttp"
)

// TestNgxVarAPI 测试 ngx.var API 基础功能
func TestNgxVarAPI(t *testing.T) {
	// 创建 fasthttp 请求上下文
	var req fasthttp.Request
	req.Header.SetMethod("GET")
	req.Header.SetRequestURI("/test/path?foo=bar&baz=qux")
	req.Header.Set("Host", "example.com")
	req.Header.Set("User-Agent", "TestAgent")

	ctx := &fasthttp.RequestCtx{}
	ctx.Init(&req, nil, nil)

	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	// 创建协程
	coro, err := engine.NewCoroutine(ctx)
	require.NoError(t, err)
	defer coro.Close()

	// 设置沙箱（这会注册 ngx API）
	err = coro.SetupSandbox()
	require.NoError(t, err)

	// 测试 ngx.var 存在
	err = coro.Execute(`
		if type(ngx) ~= "table" then
			error("ngx is not a table")
		end
		if type(ngx.var) ~= "table" then
			error("ngx.var is not a table")
		end
	`)
	assert.NoError(t, err)
}

// TestNgxVarReadBuiltin 测试读取内置变量
func TestNgxVarReadBuiltin(t *testing.T) {
	// 创建 fasthttp 请求上下文
	var req fasthttp.Request
	req.Header.SetMethod("POST")
	req.Header.SetRequestURI("/api/test?name=value")
	req.Header.Set("Host", "test.example.com")
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Accept", "application/json")

	ctx := &fasthttp.RequestCtx{}
	ctx.Init(&req, nil, nil)

	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	coro, err := engine.NewCoroutine(ctx)
	require.NoError(t, err)
	defer coro.Close()

	err = coro.SetupSandbox()
	require.NoError(t, err)

	// 测试读取内置变量
	err = coro.Execute(`
		local method = ngx.var.request_method
		if method ~= "POST" then
			error("request_method should be POST, got: " .. tostring(method))
		end

		local uri = ngx.var.uri
		if uri ~= "/api/test" then
			error("uri should be /api/test, got: " .. tostring(uri))
		end

		local host = ngx.var.http_host
		if host ~= "test.example.com" then
			error("http_host should be test.example.com, got: " .. tostring(host))
		end

		local userAgent = ngx.var.http_user_agent
		if userAgent ~= "Mozilla/5.0" then
			error("http_user_agent should be Mozilla/5.0, got: " .. tostring(userAgent))
		end
	`)
	assert.NoError(t, err)
}

// TestNgxVarReadQueryArgs 测试读取查询参数
func TestNgxVarReadQueryArgs(t *testing.T) {
	var req fasthttp.Request
	req.Header.SetMethod("GET")
	req.Header.SetRequestURI("/search?q=lua&page=1")

	ctx := &fasthttp.RequestCtx{}
	ctx.Init(&req, nil, nil)

	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	coro, err := engine.NewCoroutine(ctx)
	require.NoError(t, err)
	defer coro.Close()

	err = coro.SetupSandbox()
	require.NoError(t, err)

	// 测试读取查询参数
	err = coro.Execute(`
		local q = ngx.var.arg_q
		if q ~= "lua" then
			error("arg_q should be 'lua', got: " .. tostring(q))
		end

		local page = ngx.var.arg_page
		if page ~= "1" then
			error("arg_page should be '1', got: " .. tostring(page))
		end

		local queryString = ngx.var.query_string
		if type(queryString) ~= "string" then
			error("query_string should be a string")
		end
	`)
	assert.NoError(t, err)
}

// TestNgxVarWriteCustom 测试设置自定义变量
func TestNgxVarWriteCustom(t *testing.T) {
	var req fasthttp.Request
	req.Header.SetMethod("GET")
	req.Header.SetRequestURI("/test")

	ctx := &fasthttp.RequestCtx{}
	ctx.Init(&req, nil, nil)

	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	coro, err := engine.NewCoroutine(ctx)
	require.NoError(t, err)
	defer coro.Close()

	err = coro.SetupSandbox()
	require.NoError(t, err)

	// 测试设置和读取自定义变量
	err = coro.Execute(`
		-- 设置自定义变量
		ngx.var.my_custom_var = "hello world"
		ngx.var.another_var = "12345"

		-- 读取自定义变量
		local val1 = ngx.var.my_custom_var
		if val1 ~= "hello world" then
			error("my_custom_var should be 'hello world', got: " .. tostring(val1))
		end

		local val2 = ngx.var.another_var
		if val2 ~= "12345" then
			error("another_var should be '12345', got: " .. tostring(val2))
		end

		-- 覆盖已存在的变量
		ngx.var.my_custom_var = "updated"
		local val3 = ngx.var.my_custom_var
		if val3 ~= "updated" then
			error("my_custom_var should be 'updated', got: " .. tostring(val3))
		end
	`)
	assert.NoError(t, err)
}

// TestNgxVarIndexAccess 测试索引访问方式 ngx.var[key]
func TestNgxVarIndexAccess(t *testing.T) {
	var req fasthttp.Request
	req.Header.SetMethod("GET")
	req.Header.SetRequestURI("/test")

	ctx := &fasthttp.RequestCtx{}
	ctx.Init(&req, nil, nil)

	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	coro, err := engine.NewCoroutine(ctx)
	require.NoError(t, err)
	defer coro.Close()

	err = coro.SetupSandbox()
	require.NoError(t, err)

	// 测试索引访问
	err = coro.Execute(`
		-- 使用索引方式设置变量
		ngx.var["dynamic_key"] = "dynamic_value"

		-- 使用索引方式读取变量
		local val = ngx.var["dynamic_key"]
		if val ~= "dynamic_value" then
			error("dynamic_key should be 'dynamic_value', got: " .. tostring(val))
		end

		-- 混合访问方式
		ngx.var.mixed = "mixed_value"
		local mixed = ngx.var["mixed"]
		if mixed ~= "mixed_value" then
			error("mixed should be 'mixed_value', got: " .. tostring(mixed))
		end
	`)
	assert.NoError(t, err)
}

// TestNgxVarNilRequestCtx 测试无请求上下文的情况
func TestNgxVarNilRequestCtx(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	// 创建没有请求上下文的协程
	coro, err := engine.NewCoroutine(nil)
	require.NoError(t, err)
	defer coro.Close()

	err = coro.SetupSandbox()
	require.NoError(t, err)

	// 测试在无请求上下文时访问 ngx.var
	err = coro.Execute(`
		-- 应该返回空字符串或 nil
		local method = ngx.var.request_method
		-- 可以接受空字符串或 nil

		-- 但自定义变量仍然可以设置
		ngx.var.test = "value"
		local val = ngx.var.test
		if val ~= "value" then
			error("custom var should be settable even without request ctx")
		end
	`)
	assert.NoError(t, err)
}

// TestNgxVarUndefined 测试未定义变量
func TestNgxVarUndefined(t *testing.T) {
	var req fasthttp.Request
	req.Header.SetMethod("GET")
	req.Header.SetRequestURI("/test")

	ctx := &fasthttp.RequestCtx{}
	ctx.Init(&req, nil, nil)

	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	coro, err := engine.NewCoroutine(ctx)
	require.NoError(t, err)
	defer coro.Close()

	err = coro.SetupSandbox()
	require.NoError(t, err)

	// 测试读取未定义变量
	err = coro.Execute(`
		local undefined = ngx.var.undefined_var_name
		if undefined ~= nil then
			error("undefined var should be nil, got: " .. tostring(undefined))
		end
	`)
	assert.NoError(t, err)
}
