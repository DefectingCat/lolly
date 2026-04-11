// Package lua 提供 ngx.ctx API 测试
package lua

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/valyala/fasthttp"
)

// TestNgxCtxAPI 测试 ngx.ctx API 基础功能
func TestNgxCtxAPI(t *testing.T) {
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

	// 测试 ngx.ctx 存在且是一个 table
	err = coro.Execute(`
		if type(ngx) ~= "table" then
			error("ngx is not a table")
		end
		if type(ngx.ctx) ~= "table" then
			error("ngx.ctx is not a table")
		end
	`)
	assert.NoError(t, err)
}

// TestNgxCtxStringValue 测试存储字符串值
func TestNgxCtxStringValue(t *testing.T) {
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

	// 测试字符串存储
	err = coro.Execute(`
		ngx.ctx.message = "hello world"
		local msg = ngx.ctx.message
		if msg ~= "hello world" then
			error("string value mismatch: " .. tostring(msg))
		end
	`)
	assert.NoError(t, err)
}

// TestNgxCtxNumberValue 测试存储数字值
func TestNgxCtxNumberValue(t *testing.T) {
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

	// 测试数字存储
	err = coro.Execute(`
		ngx.ctx.count = 42
		ngx.ctx.pi = 3.14159

		local count = ngx.ctx.count
		local pi = ngx.ctx.pi

		if count ~= 42 then
			error("count should be 42, got: " .. tostring(count))
		end

		if math.abs(pi - 3.14159) > 0.00001 then
			error("pi should be 3.14159, got: " .. tostring(pi))
		end
	`)
	assert.NoError(t, err)
}

// TestNgxCtxTableValue 测试存储 table 值
func TestNgxCtxTableValue(t *testing.T) {
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

	// 测试 table 存储
	err = coro.Execute(`
		ngx.ctx.data = {
			name = "test",
			items = {1, 2, 3, 4, 5}
		}

		local data = ngx.ctx.data
		if type(data) ~= "table" then
			error("data should be a table")
		end

		if data.name ~= "test" then
			error("data.name should be 'test'")
		end

		if type(data.items) ~= "table" then
			error("data.items should be a table")
		end

		if data.items[1] ~= 1 then
			error("data.items[1] should be 1")
		end

		if data.items[5] ~= 5 then
			error("data.items[5] should be 5")
		end
	`)
	assert.NoError(t, err)
}

// TestNgxCtxFunctionValue 测试存储函数值
func TestNgxCtxFunctionValue(t *testing.T) {
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

	// 测试函数存储
	err = coro.Execute(`
		ngx.ctx.handler = function(x)
			return x * 2
		end

		local handler = ngx.ctx.handler
		if type(handler) ~= "function" then
			error("handler should be a function")
		end

		local result = handler(21)
		if result ~= 42 then
			error("handler(21) should return 42, got: " .. tostring(result))
		end
	`)
	assert.NoError(t, err)
}

// TestNgxCtxBooleanValue 测试存储布尔值
func TestNgxCtxBooleanValue(t *testing.T) {
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

	// 测试布尔值存储
	err = coro.Execute(`
		ngx.ctx.enabled = true
		ngx.ctx.disabled = false

		if ngx.ctx.enabled ~= true then
			error("enabled should be true")
		end

		if ngx.ctx.disabled ~= false then
			error("disabled should be false")
		end
	`)
	assert.NoError(t, err)
}

// TestNgxCtxNilValue 测试存储和读取 nil 值
func TestNgxCtxNilValue(t *testing.T) {
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

	// 测试 nil 值
	err = coro.Execute(`
		ngx.ctx.nothing = nil
		local val = ngx.ctx.nothing
		if val ~= nil then
			error("nothing should be nil")
		end

		-- 读取不存在的键
		local missing = ngx.ctx.missing_key
		if missing ~= nil then
			error("missing_key should be nil")
		end
	`)
	assert.NoError(t, err)
}

// TestNgxCtxMultipleScripts 测试在同一个脚本中读写 ngx.ctx
// 注意：协程在执行后变成 dead 状态，不能多次执行
func TestNgxCtxMultipleScripts(t *testing.T) {
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

	// 在同一个脚本中设置和读取值（协程只能执行一次）
	err = coro.Execute(`
		-- 设置值
		ngx.ctx.shared_value = "shared"
		ngx.ctx.counter = 1

		-- 读取并验证值
		local val = ngx.ctx.shared_value
		if val ~= "shared" then
			error("shared_value should be 'shared'")
		end

		-- 修改值
		ngx.ctx.counter = ngx.ctx.counter + 1
		if ngx.ctx.counter ~= 2 then
			error("counter should be 2")
		end
	`)
	assert.NoError(t, err)
}

// TestNgxCtxNestedTable 测试嵌套 table
func TestNgxCtxNestedTable(t *testing.T) {
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

	// 测试嵌套 table
	err = coro.Execute(`
		ngx.ctx.config = {
			database = {
				host = "localhost",
				port = 5432
			},
			cache = {
				ttl = 3600
			}
		}

		local host = ngx.ctx.config.database.host
		if host ~= "localhost" then
			error("config.database.host should be 'localhost'")
		end

		local port = ngx.ctx.config.database.port
		if port ~= 5432 then
			error("config.database.port should be 5432")
		end

		ngx.ctx.config.database.port = 3306
		if ngx.ctx.config.database.port ~= 3306 then
			error("config.database.port should be updated to 3306")
		end
	`)
	assert.NoError(t, err)
}
