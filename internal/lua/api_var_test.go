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

// TestNgxVarAdditionalBuiltinVars 测试其他内置变量
func TestNgxVarAdditionalBuiltinVars(t *testing.T) {
	var req fasthttp.Request
	req.Header.SetMethod("DELETE")
	req.Header.SetRequestURI("/api/users?id=123&name=test")
	req.Header.Set("Host", "api.example.com")
	req.Header.Set("User-Agent", "TestClient/1.0")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer token123")

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

	// 测试其他内置变量
	err = coro.Execute(`
		-- URI 相关变量
		local request_uri = ngx.var.request_uri
		if request_uri ~= "/api/users?id=123&name=test" then
			error("request_uri mismatch, got: " .. tostring(request_uri))
		end

		local uri = ngx.var.uri
		if uri ~= "/api/users" then
			error("urishould be /api/users, got: " .. tostring(uri))
		end

		local document_uri = ngx.var.document_uri
		if document_uri ~= "/api/users" then
			error("document_uri should be /api/users, got: " .. tostring(document_uri))
		end

		-- 查询字符串
		local query_string = ngx.var.query_string
		if query_string ~= "id=123&name=test" then
			error("query_string mismatch, got: " .. tostring(query_string))
		end

		local args = ngx.var.args
		if args ~= "id=123&name=test" then
			error("args should match query_string, got: " .. tostring(args))
		end

		-- 请求头
		local accept = ngx.var.http_accept
		if accept ~= "application/json" then
			error("http_accept mismatch, got: " .. tostring(accept))
		end

		local contentType = ngx.var.http_content_type
		if contentType ~= "application/json" then
			error("http_content_type mismatch, got: " .. tostring(contentType))
		end

		local authorization = ngx.var.http_authorization
		if authorization ~= "Bearer token123" then
			error("http_authorization mismatch, got: " .. tostring(authorization))
		end

		-- 内置变量 map
		local vars = {
			"request_method", "request_uri", "uri", "document_uri",
			"query_string", "args", "http_host", "http_user_agent",
			"http_accept", "http_content_type"
		}
		for _, v in ipairs(vars) do
			local val = ngx.var[v]
			if type(val) ~= "string" then
				error("var " .. v .. " should be string, got: " .. type(val))
			end
		end
	`)
	assert.NoError(t, err)
}

// TestNgxVarDynamicArgsAccess 测试动态参数访问
func TestNgxVarDynamicArgsAccess(t *testing.T) {
	var req fasthttp.Request
	req.Header.SetMethod("GET")
	req.Header.SetRequestURI("/search?keyword=lua&category=programming&limit=10")

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

	// 测试动态参数访问
	err = coro.Execute(`
		-- 直接通过 arg_ 访问
		local keyword = ngx.var.arg_keyword
		if keyword ~= "lua" then
			error("arg_keyword should be 'lua', got: " .. tostring(keyword))
		end

		local category = ngx.var.arg_category
		if category ~= "programming" then
			error("arg_category should be 'programming', got: " .. tostring(category))
		end

		local limit = ngx.var.arg_limit
		if limit ~= "10" then
			error("arg_limit should be '10', got: " .. tostring(limit))
		end

		-- 使用动态键访问
		local keys = {"keyword", "category", "limit"}
		for i, k in ipairs(keys) do
			local val = ngx.var["arg_" .. k]
			if type(val) ~= "string" then
				error("dynamic arg access should return string")
			end
		end
	`)
	assert.NoError(t, err)
}

// TestNgxVarGoAPI 测试 Go 层 API 调用
func TestNgxVarGoAPI(t *testing.T) {
	var req fasthttp.Request
	req.Header.SetMethod("GET")
	req.Header.SetRequestURI("/test")

	ctx := &fasthttp.RequestCtx{}
	ctx.Init(&req, nil, nil)

	// 直接创建 API 实例并测试 Go 层 API
	api := newNgxVarAPI(ctx)
	require.NotNil(t, api)

	// 测试 SetVariable
	api.SetVariable("go_set_var", "value_from_go")
	value, ok := api.GetVariable("go_set_var")
	assert.True(t, ok)
	assert.Equal(t, "value_from_go", value)

	// 测试 GetVariable 不存在的变量
	value, ok = api.GetVariable("nonexistent")
	assert.False(t, ok)
	assert.Equal(t, "", value)

	// 测试覆盖：Go 设置，Go 读取验证
	api.SetVariable("cross_lang", "from_go")
	val, ok := api.GetVariable("cross_lang")
	assert.True(t, ok)
	assert.Equal(t, "from_go", val)

	// 测试覆盖：直接设置 store，Go 读取验证
	api.store["cross_lang2"] = "from_lua"
	value, ok = api.GetVariable("cross_lang2")
	assert.True(t, ok)
	assert.Equal(t, "from_lua", value)
}

// TestNgxVarRequestMethodAccess 测试各种请求方法
func TestNgxVarRequestMethodAccess(t *testing.T) {
	methods := []string{"GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS"}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			var req fasthttp.Request
			req.Header.SetMethod(method)
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
				local method = ngx.var.request_method
				if method ~= "` + method + `" then
					error("request_method should be '` + method + `', got: " .. tostring(method))
				end
			`)
			assert.NoError(t, err)
		})
	}
}

// TestNgxVarMixedAccessPatterns 测试混合访问模式
func TestNgxVarMixedAccessPatterns(t *testing.T) {
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

	// 测试混合访问模式
	err = coro.Execute(`
		-- 点号访问设置
		ngx.var.test1 = "value1"
		-- 索引访问读取
		local val1 = ngx.var["test1"]
		if val1 ~= "value1" then
			error("mixed access 1 failed")
		end

		-- 索引访问设置
		ngx.var["test2"] = "value2"
		-- 点号访问读取
		local val2 = ngx.var.test2
		if val2 ~= "value2" then
			error("mixed access 2 failed")
		end

		-- 循环访问
		for i = 1, 3 do
			ngx.var["dynamic_" .. i] = "val_" .. i
		end

		for i = 1, 3 do
			local v = ngx.var["dynamic_" .. i]
			if v ~= "val_" .. i then
				error("dynamic loop failed for i=" .. i)
			end
		end
	`)
	assert.NoError(t, err)
}

// TestNgxVarSpecialHeaders 测试特殊请求头
func TestNgxVarSpecialHeaders(t *testing.T) {
	var req fasthttp.Request
	req.Header.SetMethod("GET")
	req.Header.SetRequestURI("/test")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Referer", "https://example.com")

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
		-- 测试带连字符的头
		local acceptEncoding = ngx.var.http_accept_encoding
		if acceptEncoding ~= "gzip, deflate, br" then
			error("http_accept_encoding mismatch")
		end

		local acceptLanguage = ngx.var.http_accept_language
		if acceptLanguage ~= "en-US,en;q=0.9" then
			error("http_accept_language mismatch")
		end

		local connection = ngx.var.http_connection
		if connection ~= "keep-alive" then
			error("http_connection mismatch")
		end

		local referer = ngx.var.http_referer
		if referer ~= "https://example.com" then
			error("http_referer mismatch")
		end

		-- 测试也可以通过下划线访问
		local enc2 = ngx.var["http_accept_encoding"]
		if enc2 ~= acceptEncoding then
			error("http_accept_encoding via index mismatch")
		end
	`)
	assert.NoError(t, err)
}

// TestNgxVarEmptyAndNil 测试空值和 nil 处理
func TestNgxVarEmptyAndNil(t *testing.T) {
	var req fasthttp.Request
	req.Header.SetMethod("GET")
	req.Header.SetRequestURI("/")

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
		-- 未设置的参数应该返回空字符串或 nil
		local empty = ngx.var.arg_nonexistent
		-- arg_ 对不存在的参数通常返回空字符串

		-- 自定义变量设为空字符串
		ngx.var.empty_string = ""
		local val = ngx.var.empty_string
		if val ~= "" then
			error("empty_string should be empty")
		end

		-- 覆盖为空值
		ngx.var.test = "value"
		ngx.var.test = nil  -- Lua 的 nil 在 __newindex 中会被转换
		-- 实现中 nil 会被转换为空字符串
	`)
	assert.NoError(t, err)
}

// TestNgxVarRequestBodyAccess 测试请求体相关变量
func TestNgxVarRequestBodyAccess(t *testing.T) {
	var req fasthttp.Request
	req.Header.SetMethod("POST")
	req.Header.SetRequestURI("/upload")
	req.Header.SetContentType("application/octet-stream")
	req.SetBody([]byte("test body content"))

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
		-- 测试请求长度
		local length = ngx.var.request_length
		if type(length) ~= "number" then
			error("request_length should be a number")
		end

		-- 测试内容类型
		local contentType = ngx.var.http_content_type
		if contentType ~= "application/octet-stream" then
			error("content_type mismatch")
		end
	`)
	assert.NoError(t, err)
}
