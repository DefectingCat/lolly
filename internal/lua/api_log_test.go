// Package lua 提供 ngx.log API 测试
package lua

import (
	"bytes"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/testutil"
)

// mockRequestCtxForLog 创建模拟的 RequestCtx
func mockRequestCtxForLog() *fasthttp.RequestCtx {
	return testutil.NewRequestCtx("GET", "/")
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
	L := engine.GetLStateForTest()
	RegisterNgxLogAPI(L, api)

	// 测试日志 - 不应 panic
	err = L.DoString(`
		ngx.log(ngx.INFO, "message without logger")
	`)
	assert.NoError(t, err)
}

// TestNgxLogEmergDoesNotExit 验证 ngx.EMERG 不会导致进程退出。
func TestNgxLogEmergDoesNotExit(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	ctx := mockRequestCtxForLog()
	luaCtx := NewContext(engine, ctx)

	var buf bytes.Buffer
	logger := zerolog.New(&buf)

	api := newNgxLogAPI(ctx, luaCtx, &logger)
	L := engine.GetLStateForTest()
	RegisterNgxLogAPI(L, api)

	// 不应 panic 或退出进程
	err = L.DoString(`
		ngx.log(ngx.EMERG, "emergency message")
		ngx.log(ngx.ALERT, "alert message")
		ngx.log(ngx.CRIT, "critical message")
	`)
	require.NoError(t, err)

	output := buf.String()
	if !strings.Contains(output, "emergency message") {
		t.Error("expected emerg message to be logged")
	}
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
	L := engine.GetLStateForTest()
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
