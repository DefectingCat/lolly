// Package lua 提供 Lua 脚本嵌入能力
package lua

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/valyala/fasthttp"
)

func TestLocationManagerRegister(t *testing.T) {
	manager := NewLocationManager()
	require.NotNil(t, manager)

	// 注册 location
	handler := func(ctx *fasthttp.RequestCtx) {
		ctx.WriteString("test response")
	}
	manager.Register("/test", handler)

	// 验证注册成功
	manager.mu.Lock()
	_, ok := manager.handlers["/test"]
	manager.mu.Unlock()
	assert.True(t, ok)
}

func TestLocationManagerCapture(t *testing.T) {
	manager := NewLocationManager()

	// 注册 location
	handler := func(ctx *fasthttp.RequestCtx) {
		ctx.SetStatusCode(200)
		ctx.SetBodyString("hello from subrequest")
		ctx.Response.Header.Set("X-Custom", "value")
	}
	manager.Register("/api/sub", handler)

	// 创建父请求上下文
	parentCtx := &fasthttp.RequestCtx{}
	parentCtx.Request.SetRequestURI("/parent")

	// 执行子请求
	result, err := manager.Capture(parentCtx, "/api/sub", nil)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, 200, result.Status)
	assert.Equal(t, "hello from subrequest", string(result.Body))
	assert.Equal(t, "value", result.Headers["X-Custom"])
}

func TestLocationManagerCaptureNotFound(t *testing.T) {
	manager := NewLocationManager()

	parentCtx := &fasthttp.RequestCtx{}

	// 执行不存在的 location
	result, err := manager.Capture(parentCtx, "/notexist", nil)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, 404, result.Status)
}

func TestLocationManagerCaptureWithOptions(t *testing.T) {
	manager := NewLocationManager()

	// 注册 location
	handler := func(ctx *fasthttp.RequestCtx) {
		ctx.SetStatusCode(200)
		ctx.WriteString("method: " + string(ctx.Method()) + ", body: " + string(ctx.PostBody()))
	}
	manager.Register("/echo", handler)

	parentCtx := &fasthttp.RequestCtx{}
	parentCtx.Request.SetRequestURI("/parent")

	// 使用自定义选项
	opts := map[string]interface{}{
		"method": "POST",
		"body":   "test body",
	}

	result, err := manager.Capture(parentCtx, "/echo", opts)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, 200, result.Status)
	assert.Contains(t, string(result.Body), "method: POST")
	assert.Contains(t, string(result.Body), "body: test body")
}

func TestLocationLuaAPI(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	L := engine.L

	// 注册 ngx.location API
	ngx := L.NewTable()
	L.SetGlobal("ngx", ngx)
	RegisterLocationAPI(L, engine.LocationManager(), ngx)

	// 测试 ngx.location.capture
	err = L.DoString(`
		-- 创建模拟的 location 结果
		local result = ngx.location.capture("/test")

		-- 验证结果结构
		assert(result ~= nil)
		assert(result.status ~= nil)
		assert(result.body ~= nil)
	`)
	require.NoError(t, err)
}
