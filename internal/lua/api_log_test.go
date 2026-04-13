// Package lua 提供 ngx.log API 测试
package lua

import (
	"bytes"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/valyala/fasthttp"
	glua "github.com/yuin/gopher-lua"

	"rua.plus/lolly/internal/testutil"
)

// mockRequestCtxForLog 创建模拟的 RequestCtx
func mockRequestCtxForLog() *fasthttp.RequestCtx {
	return testutil.NewRequestCtx("GET", "/")
}

// TestNgxLogLevelConstants 测试日志级别常量
func TestNgxLogLevelConstants(t *testing.T) {
	// 验证日志级别常量值
	assert.Equal(t, 0, LogStderr)
	assert.Equal(t, 1, LogEmerg)
	assert.Equal(t, 2, LogAlert)
	assert.Equal(t, 3, LogCrit)
	assert.Equal(t, 4, LogErr)
	assert.Equal(t, 5, LogWarn)
	assert.Equal(t, 6, LogNotice)
	assert.Equal(t, 7, LogInfo)
	assert.Equal(t, 8, LogDebug)
}

// TestNgxHTTPStatusConstants 测试 HTTP 状态码常量
func TestNgxHTTPStatusConstants(t *testing.T) {
	// 验证常用 HTTP 状态码
	assert.Equal(t, 200, HTTPOK)
	assert.Equal(t, 201, HTTPCreated)
	assert.Equal(t, 301, HTTPMovedPermanently)
	assert.Equal(t, 302, HTTPFound)
	assert.Equal(t, 303, HTTPSeeOther)
	assert.Equal(t, 307, HTTPTemporaryRedirect)
	assert.Equal(t, 308, HTTPPermanentRedirect)
	assert.Equal(t, 400, HTTPBadRequest)
	assert.Equal(t, 404, HTTPNotFound)
	assert.Equal(t, 500, HTTPInternalServerError)
}

// TestNgxLogAPIRegistration 测试 API 注册
func TestNgxLogAPIRegistration(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	// 创建测试请求上下文
	ctx := mockRequestCtxForLog()
	luaCtx := NewContext(engine, ctx)

	// 创建 zerolog logger
	var buf bytes.Buffer
	logger := zerolog.New(&buf)

	// 创建 API 实例
	api := newNgxLogAPI(ctx, luaCtx, &logger)

	// 在 Lua 状态机中注册 API
	L := engine.L
	RegisterNgxLogAPI(L, api)

	// 验证 ngx 表已创建
	ngx := L.GetGlobal("ngx")
	require.NotEqual(t, nil, ngx)

	// 验证日志级别常量已注册
	ngxTable := ngx.(*glua.LTable)
	assert.Equal(t, glua.LNumber(LogStderr), ngxTable.RawGetString("STDERR"))
	assert.Equal(t, glua.LNumber(LogEmerg), ngxTable.RawGetString("EMERG"))
	assert.Equal(t, glua.LNumber(LogErr), ngxTable.RawGetString("ERR"))
	assert.Equal(t, glua.LNumber(LogWarn), ngxTable.RawGetString("WARN"))
	assert.Equal(t, glua.LNumber(LogInfo), ngxTable.RawGetString("INFO"))
	assert.Equal(t, glua.LNumber(LogDebug), ngxTable.RawGetString("DEBUG"))

	// 验证 HTTP 状态码常量已注册
	assert.Equal(t, glua.LNumber(HTTPOK), ngxTable.RawGetString("HTTP_OK"))
	assert.Equal(t, glua.LNumber(HTTPNotFound), ngxTable.RawGetString("HTTP_NOT_FOUND"))
	assert.Equal(t, glua.LNumber(HTTPInternalServerError), ngxTable.RawGetString("HTTP_INTERNAL_SERVER_ERROR"))

	// 验证函数已注册
	assert.NotEqual(t, glua.LNil, ngxTable.RawGetString("log"))
	assert.NotEqual(t, glua.LNil, ngxTable.RawGetString("say"))
	assert.NotEqual(t, glua.LNil, ngxTable.RawGetString("print"))
	assert.NotEqual(t, glua.LNil, ngxTable.RawGetString("flush"))
	assert.NotEqual(t, glua.LNil, ngxTable.RawGetString("exit"))
	assert.NotEqual(t, glua.LNil, ngxTable.RawGetString("redirect"))
}

// TestNgxLogAPILog 测试 ngx.log API
func TestNgxLogAPILog(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	ctx := mockRequestCtxForLog()
	luaCtx := NewContext(engine, ctx)

	var buf bytes.Buffer
	logger := zerolog.New(&buf)

	api := newNgxLogAPI(ctx, luaCtx, &logger)
	L := engine.L
	RegisterNgxLogAPI(L, api)

	// 测试日志输出
	err = L.DoString(`
		ngx.log(ngx.INFO, "test message")
	`)
	// 日志调用不应返回错误
	assert.NoError(t, err)

	// 验证日志内容
	logOutput := buf.String()
	assert.Contains(t, logOutput, "test message")
}

// TestNgxLogAPISay 测试 ngx.say API
func TestNgxLogAPISay(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	ctx := mockRequestCtxForLog()
	luaCtx := NewContext(engine, ctx)

	var buf bytes.Buffer
	logger := zerolog.New(&buf)

	api := newNgxLogAPI(ctx, luaCtx, &logger)
	L := engine.L
	RegisterNgxLogAPI(L, api)

	// 测试 say 输出
	err = L.DoString(`
		ngx.say("Hello, ")
		ngx.say("World!")
	`)
	require.NoError(t, err)

	// 验证输出缓冲包含换行符
	assert.Equal(t, "Hello, \nWorld!\n", string(luaCtx.OutputBuffer))
}

// TestNgxLogAPIPrint 测试 ngx.print API
func TestNgxLogAPIPrint(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	ctx := mockRequestCtxForLog()
	luaCtx := NewContext(engine, ctx)

	var buf bytes.Buffer
	logger := zerolog.New(&buf)

	api := newNgxLogAPI(ctx, luaCtx, &logger)
	L := engine.L
	RegisterNgxLogAPI(L, api)

	// 测试 print 输出
	err = L.DoString(`
		ngx.print("Hello")
		ngx.print(", ")
		ngx.print("World")
	`)
	require.NoError(t, err)

	// 验证输出缓冲不包含换行符
	assert.Equal(t, "Hello, World", string(luaCtx.OutputBuffer))
}

// TestNgxLogAPIFlush 测试 ngx.flush API
func TestNgxLogAPIFlush(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	ctx := mockRequestCtxForLog()
	luaCtx := NewContext(engine, ctx)

	var buf bytes.Buffer
	logger := zerolog.New(&buf)

	api := newNgxLogAPI(ctx, luaCtx, &logger)
	L := engine.L
	RegisterNgxLogAPI(L, api)

	// 测试 flush
	err = L.DoString(`
		ngx.print("before flush")
		local ok = ngx.flush()
		assert(ok == true, "flush should return true")
	`)
	require.NoError(t, err)
}

// TestNgxLogAPIExit 测试 ngx.exit API
func TestNgxLogAPIExit(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	ctx := mockRequestCtxForLog()
	luaCtx := NewContext(engine, ctx)

	var buf bytes.Buffer
	logger := zerolog.New(&buf)

	api := newNgxLogAPI(ctx, luaCtx, &logger)
	L := engine.L
	RegisterNgxLogAPI(L, api)

	// 测试 exit - 应该抛出错误
	err = L.DoString(`
		ngx.exit(ngx.HTTP_OK)
	`)
	// exit 应该返回错误
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ngx.exit")

	// 验证状态码已设置
	assert.Equal(t, 200, ctx.Response.StatusCode())
}

// TestNgxLogAPIRedirect 测试 ngx.redirect API
func TestNgxLogAPIRedirect(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	ctx := mockRequestCtxForLog()
	luaCtx := NewContext(engine, ctx)

	var buf bytes.Buffer
	logger := zerolog.New(&buf)

	api := newNgxLogAPI(ctx, luaCtx, &logger)
	L := engine.L
	RegisterNgxLogAPI(L, api)

	// 测试 redirect - 默认状态码 302
	err = L.DoString(`
		ngx.redirect("/new/path")
	`)
	// redirect 应该返回错误以终止执行
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ngx.redirect")

	// 验证重定向头和状态码
	assert.Equal(t, 302, ctx.Response.StatusCode())
	assert.Equal(t, "/new/path", string(ctx.Response.Header.Peek("Location")))
}

// TestNgxLogAPIRedirectWithStatus 测试带状态码的 redirect
func TestNgxLogAPIRedirectWithStatus(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	ctx := mockRequestCtxForLog()
	luaCtx := NewContext(engine, ctx)

	var buf bytes.Buffer
	logger := zerolog.New(&buf)

	api := newNgxLogAPI(ctx, luaCtx, &logger)
	L := engine.L
	RegisterNgxLogAPI(L, api)

	// 测试 redirect - 301
	err = L.DoString(`
		ngx.redirect("/permanent/path", ngx.HTTP_MOVED_PERMANENTLY)
	`)
	assert.Error(t, err)

	// 验证状态码
	assert.Equal(t, 301, ctx.Response.StatusCode())
}

// TestNgxLogAPIRedirectInvalidStatus 测试无效状态码的 redirect
func TestNgxLogAPIRedirectInvalidStatus(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	ctx := mockRequestCtxForLog()
	luaCtx := NewContext(engine, ctx)

	var buf bytes.Buffer
	logger := zerolog.New(&buf)

	api := newNgxLogAPI(ctx, luaCtx, &logger)
	L := engine.L
	RegisterNgxLogAPI(L, api)

	// 测试 redirect - 无效状态码 (400)
	err = L.DoString(`
		ngx.redirect("/path", 400)
	`)
	// 应该返回参数错误
	assert.Error(t, err)
}

// TestNgxLogAPILogLevels 测试不同日志级别
func TestNgxLogAPILogLevels(t *testing.T) {
	testCases := []struct {
		level    int
		expected zerolog.Level
	}{
		{LogEmerg, zerolog.FatalLevel},
		{LogAlert, zerolog.FatalLevel},
		{LogCrit, zerolog.FatalLevel},
		{LogErr, zerolog.ErrorLevel},
		{LogWarn, zerolog.WarnLevel},
		{LogNotice, zerolog.InfoLevel},
		{LogInfo, zerolog.InfoLevel},
		{LogDebug, zerolog.DebugLevel},
		{999, zerolog.InfoLevel}, // 未知级别默认为 Info
	}

	for _, tc := range testCases {
		result := LogLevelToZerolog(tc.level)
		assert.Equal(t, tc.expected, result, "level %d should map to %v", tc.level, tc.expected)
	}
}

// TestNgxLogAPIWithoutLogger 测试无 logger 的情况
func TestNgxLogAPIWithoutLogger(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	ctx := mockRequestCtxForLog()
	luaCtx := NewContext(engine, ctx)

	// 创建无 logger 的 API
	api := newNgxLogAPI(ctx, luaCtx, nil)
	L := engine.L
	RegisterNgxLogAPI(L, api)

	// 测试日志 - 不应 panic
	err = L.DoString(`
		ngx.log(ngx.INFO, "message without logger")
	`)
	assert.NoError(t, err)
}

// TestNgxLogAPIIntegration 集成测试
func TestNgxLogAPIIntegration(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	ctx := mockRequestCtxForLog()
	luaCtx := NewContext(engine, ctx)

	var buf bytes.Buffer
	logger := zerolog.New(&buf)

	api := newNgxLogAPI(ctx, luaCtx, &logger)
	L := engine.L
	RegisterNgxLogAPI(L, api)

	// 综合测试
	err = L.DoString(`
		-- 记录日志
		ngx.log(ngx.INFO, "Starting request")

		-- 输出内容
		ngx.say("Line 1")
		ngx.print("Line 2")
		ngx.say("")

		-- 使用常量
		ngx.say("HTTP OK: " .. ngx.HTTP_OK)
		ngx.say("HTTP NOT FOUND: " .. ngx.HTTP_NOT_FOUND)
	`)
	require.NoError(t, err)

	// 验证输出
	output := string(luaCtx.OutputBuffer)
	assert.Contains(t, output, "Line 1")
	assert.Contains(t, output, "Line 2")
	assert.Contains(t, output, "HTTP OK: 200")
	assert.Contains(t, output, "HTTP NOT FOUND: 404")
}
