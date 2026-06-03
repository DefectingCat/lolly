// Package lua 提供 ngx.ctx API 测试
package lua

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/valyala/fasthttp"
)

// TestNgxCtxTableAPI 测试 table API 操作
func TestNgxCtxTableAPI(t *testing.T) {
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

	// 测试 table 操作函数
	err = coro.Execute(`
		-- 测试 pairs 遍历
		ngx.ctx.items = {a = 1, b = 2, c = 3}
		local count = 0
		for k, v in pairs(ngx.ctx.items) do
			count = count + 1
		end
		if count ~= 3 then
			error("items table should have 3 elements")
		end

		-- 测试 table.insert
		ngx.ctx.list = {}
		table.insert(ngx.ctx.list, 1)
		table.insert(ngx.ctx.list, 2)
		table.insert(ngx.ctx.list, 3)
		if ngx.ctx.list[1] ~= 1 or ngx.ctx.list[2] ~= 2 or ngx.ctx.list[3] ~= 3 then
			error("table.insert failed")
		end

		-- 测试 table.remove
		table.remove(ngx.ctx.list, 2)
		if #ngx.ctx.list ~= 2 or ngx.ctx.list[1] ~= 1 or ngx.ctx.list[2] ~= 3 then
			error("table.remove failed")
		end

		-- 测试 table.concat
		ngx.ctx.strlist = {"hello", "world", "test"}
		local joined = table.concat(ngx.ctx.strlist, ", ")
		if joined ~= "hello, world, test" then
			error("table.concat failed: " .. joined)
		end
	`)
	assert.NoError(t, err)
}

// TestNgxCtxLargeValues 测试大值存储
func TestNgxCtxLargeValues(t *testing.T) {
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

	// 测试大字符串和大 table（在一个脚本中完成所有操作）
	largeString := string(make([]byte, 10000)) // 10KB 字符串
	err = coro.Execute(`
		-- 测试大字符串
		ngx.ctx.large = "` + largeString + `"
		local val = ngx.ctx.large
		if type(val) ~= "string" then
			error("large value should be string")
		end

		-- 测试大 table
		ngx.ctx.bigtable = {}
		for i = 1, 1000 do
			ngx.ctx.bigtable[i] = i * 2
		end
		if #ngx.ctx.bigtable ~= 1000 then
			error("bigtable should have 1000 elements")
		end
	`)
	assert.NoError(t, err)
}

// TestNgxCtxTypeCoercion 测试类型转换
func TestNgxCtxTypeCoercion(t *testing.T) {
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

	// 测试数字字符串自动转换
	err = coro.Execute(`
		ngx.ctx.num = 42
		ngx.ctx.str = "123"

		-- 数字加字符串
		local result = ngx.ctx.num + tonumber(ngx.ctx.str)
		if result ~= 165 then
			error("type coercion failed: " .. tostring(result))
		end

		-- 字符串连接
		local concatenated = "value: " .. ngx.ctx.num
		if concatenated ~= "value: 42" then
			error("string concatenation failed: " .. concatenated)
		end
	`)
	assert.NoError(t, err)
}

// TestNgxCtxBooleanLogic 测试布尔逻辑
func TestNgxCtxBooleanLogic(t *testing.T) {
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

	err = coro.Execute(`
		ngx.ctx.a = true
		ngx.ctx.b = false

		-- and 操作
		if (ngx.ctx.a and ngx.ctx.b) ~= false then
			error("a and b should be false")
		end

		-- or 操作
		if (ngx.ctx.a or ngx.ctx.b) ~= true then
			error("a or b should be true")
		end

		-- not 操作
		if (not ngx.ctx.a) ~= false then
			error("not a should be false")
		end
	`)
	assert.NoError(t, err)
}
