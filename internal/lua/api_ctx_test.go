// Package lua 提供 ngx.ctx API 测试
package lua

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/valyala/fasthttp"
	glua "github.com/yuin/gopher-lua"
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

// TestNgxCtxRequestIsolation 测试请求间上下文隔离
func TestNgxCtxRequestIsolation(t *testing.T) {
	var req1, req2 fasthttp.Request
	req1.Header.SetMethod("GET")
	req1.Header.SetRequestURI("/request1")
	req2.Header.SetMethod("GET")
	req2.Header.SetRequestURI("/request2")

	ctx1 := &fasthttp.RequestCtx{}
	ctx1.Init(&req1, nil, nil)
	ctx2 := &fasthttp.RequestCtx{}
	ctx2.Init(&req2, nil, nil)

	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	// 第一个请求：设置 ctx 值
	coro1, err := engine.NewCoroutine(ctx1)
	require.NoError(t, err)

	err = coro1.SetupSandbox()
	require.NoError(t, err)

	err = coro1.Execute(`
		ngx.ctx.request_id = 1
		ngx.ctx.message = "request1_data"
	`)
	assert.NoError(t, err)
	coro1.Close()

	// 第二个请求：验证 ctx 与其他请求隔离
	coro2, err := engine.NewCoroutine(ctx2)
	require.NoError(t, err)

	err = coro2.SetupSandbox()
	require.NoError(t, err)

	err = coro2.Execute(`
		-- 第一个请求的值不应该影响第二个请求
		if ngx.ctx.request_id ~= nil then
			error("ctx from another request should be isolated")
		end

		if ngx.ctx.message ~= nil then
			error("ctx from another request should be isolated")
		end

		-- 设置自己的值
		ngx.ctx.request_id = 2
		ngx.ctx.message = "request2_data"

		if ngx.ctx.request_id ~= 2 then
			error("request_id should be 2")
		end

		if ngx.ctx.message ~= "request2_data" then
			error("message should be 'request2_data'")
		end
	`)
	assert.NoError(t, err)
	coro2.Close()
}

// TestNgxCtxGoAPIAccess 测试 Go 层 API 访问
func TestNgxCtxGoAPIAccess(t *testing.T) {
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

	// 通过 Go 层 API 设置值
	api := coro.GetNgxVarAPI()
	require.NotNil(t, api)

	api.SetVariable("go_key", "go_value")

	// 验证 Lua 层可以读取，且 Lua 层设置的值 Go 层可见
	// 注意：在一个脚本中完成所有操作，因为协程执行后变为 dead 状态
	err = coro.Execute(`
		-- 验证 Go 层设置的值
		if ngx.var.go_key ~= "go_value" then
			error("value from Go layer should be accessible in Lua")
		end

		-- 从 Lua 层设置值
		ngx.var.lua_key = "lua_value"
	`)
	assert.NoError(t, err)

	// 验证 Lua 层设置的值 Go 层可见
	val, ok := api.GetVariable("lua_key")
	assert.True(t, ok)
	assert.Equal(t, "lua_value", val)
}

// TestNgxCtxScheduleUnsafeAPI 测试调度器上下文中的不安全 API
func TestNgxCtxScheduleUnsafeAPI(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	// 为 Scheduler LState 创建不安全的 ctx API
	L := glua.NewState()
	defer L.Close()

	ngx := L.NewTable()
	L.SetGlobal("ngx", ngx)

	// 注册调度器不安全的 ctx API
	RegisterSchedulerUnsafeCtxAPI(L, ngx)

	// 尝试访问 ctx 应该返回错误
	err = L.DoString(`
		local ok, msg = pcall(function()
			ngx.ctx.key = "value"
		end)
		if ok then
			error("writing to ngx.ctx in scheduler should fail")
		end
		if not string.match(msg, "not available in timer callback") then
			error("wrong error message: " .. msg)
		end
	`)
	assert.NoError(t, err)

	// 尝试读取 ctx 也应该返回错误
	err = L.DoString(`
		local ok, msg = pcall(function()
			local x = ngx.ctx.key
		end)
		if ok then
			error("reading from ngx.ctx in scheduler should fail")
		end
	`)
	assert.NoError(t, err)
}

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
