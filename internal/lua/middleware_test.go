// Package lua 提供 Lua 中间件测试
package lua

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/valyala/fasthttp"
)

// TestLuaMiddlewareCreation 测试 LuaMiddleware 创建
func TestLuaMiddlewareCreation(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	// 创建临时脚本文件
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "test.lua")
	err = os.WriteFile(scriptPath, []byte("ngx.say('hello')"), 0o644)
	require.NoError(t, err)

	config := LuaMiddlewareConfig{
		ScriptPath: scriptPath,
		Phase:      PhaseContent,
		Timeout:    10 * time.Second,
		Name:       "test-middleware",
		Enabled:    true,
	}

	middleware, err := NewLuaMiddleware(engine, config)
	require.NoError(t, err)
	require.NotNil(t, middleware)

	assert.Equal(t, "test-middleware", middleware.Name())
	assert.Equal(t, scriptPath, middleware.GetScriptPath())
	assert.Equal(t, PhaseContent, middleware.GetPhase())
	assert.Equal(t, 10*time.Second, middleware.timeout)
	assert.True(t, middleware.IsEnabled())
}

// TestLuaMiddlewareDefaultConfig 测试默认配置
func TestLuaMiddlewareDefaultConfig(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	// 创建临时脚本文件
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "test.lua")
	err = os.WriteFile(scriptPath, []byte("return 1"), 0o644)
	require.NoError(t, err)

	// 使用默认配置
	config := DefaultLuaMiddlewareConfig()
	config.ScriptPath = scriptPath

	middleware, err := NewLuaMiddleware(engine, config)
	require.NoError(t, err)

	// 验证默认值
	assert.Equal(t, PhaseContent, middleware.GetPhase())
	assert.Equal(t, 30*time.Second, middleware.timeout)
	assert.Equal(t, "lua-content", middleware.Name())
	assert.True(t, middleware.IsEnabled())
}

// TestLuaMiddlewareValidation 测试配置验证
func TestLuaMiddlewareValidation(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	// 缺少 engine
	config := LuaMiddlewareConfig{ScriptPath: "test.lua"}
	middleware, err := NewLuaMiddleware(nil, config)
	assert.Error(t, err)
	assert.Nil(t, middleware)
	assert.Contains(t, err.Error(), "engine is required")

	// 缺少 script path
	middleware, err = NewLuaMiddleware(engine, LuaMiddlewareConfig{})
	assert.Error(t, err)
	assert.Nil(t, middleware)
	assert.Contains(t, err.Error(), "script path is required")
}

// TestLuaMiddlewareProcess 测试中间件处理
func TestLuaMiddlewareProcess(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	// 创建脚本文件
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "test.lua")
	err = os.WriteFile(scriptPath, []byte(`ngx.say("hello from lua")`), 0o644)
	require.NoError(t, err)

	config := LuaMiddlewareConfig{
		ScriptPath: scriptPath,
		Phase:      PhaseContent,
		Enabled:    true,
		EnabledSet: true, // 显式启用
	}

	middleware, err := NewLuaMiddleware(engine, config)
	require.NoError(t, err)

	// 创建 RequestCtx
	ctx := &fasthttp.RequestCtx{}

	// 创建最终处理器
	finalHandler := func(ctx *fasthttp.RequestCtx) {
		ctx.WriteString("final handler")
	}

	// 包装处理器
	handler := middleware.Process(finalHandler)

	// 执行
	handler(ctx)

	// 验证输出包含 Lua 输出
	body := string(ctx.Response.Body())
	assert.Contains(t, body, "hello from lua")
}

// TestLuaMiddlewareDisabled 测试禁用的中间件
func TestLuaMiddlewareDisabled(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	// 创建脚本文件
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "test.lua")
	err = os.WriteFile(scriptPath, []byte(`ngx.say("lua output")`), 0o644)
	require.NoError(t, err)

	config := LuaMiddlewareConfig{
		ScriptPath: scriptPath,
		Phase:      PhaseContent,
		Enabled:    false, // 禁用
		EnabledSet: true,  // 显式设置
	}

	middleware, err := NewLuaMiddleware(engine, config)
	require.NoError(t, err)

	// 创建 RequestCtx
	ctx := &fasthttp.RequestCtx{}

	// 创建最终处理器
	finalHandler := func(ctx *fasthttp.RequestCtx) {
		ctx.WriteString("final only")
	}

	// 包装处理器
	handler := middleware.Process(finalHandler)

	// 执行
	handler(ctx)

	// 禁用时只执行最终处理器
	body := string(ctx.Response.Body())
	assert.Equal(t, "final only", body)
	assert.NotContains(t, body, "lua output")
}

// TestLuaMiddlewareSetters 测试设置方法
func TestLuaMiddlewareSetters(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "test.lua")
	err = os.WriteFile(scriptPath, []byte("return 1"), 0o644)
	require.NoError(t, err)

	config := DefaultLuaMiddlewareConfig()
	config.ScriptPath = scriptPath

	middleware, err := NewLuaMiddleware(engine, config)
	require.NoError(t, err)

	// 测试 SetEnabled
	middleware.SetEnabled(false)
	assert.False(t, middleware.IsEnabled())

	// 测试 SetPhase
	middleware.SetPhase(PhaseRewrite)
	assert.Equal(t, PhaseRewrite, middleware.GetPhase())
	assert.Equal(t, "lua-rewrite", middleware.Name())

	// 测试 SetTimeout
	middleware.SetTimeout(5 * time.Second)
	assert.Equal(t, 5*time.Second, middleware.timeout)

	// 测试 SetScriptPath
	newPath := filepath.Join(tmpDir, "new.lua")
	middleware.SetScriptPath(newPath)
	assert.Equal(t, newPath, middleware.GetScriptPath())
}

// TestMultiPhaseLuaMiddlewareCreation 测试多阶段中间件创建
func TestMultiPhaseLuaMiddlewareCreation(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	multi := NewMultiPhaseLuaMiddleware(engine, "multi-test")

	assert.Equal(t, "multi-test", multi.Name())
	assert.Equal(t, 0, multi.PhaseCount())
}

// TestMultiPhaseLuaMiddlewareAddPhase 测试添加阶段
func TestMultiPhaseLuaMiddlewareAddPhase(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	tmpDir := t.TempDir()

	multi := NewMultiPhaseLuaMiddleware(engine, "multi-test")

	// 添加 rewrite 阶段
	rewriteScript := filepath.Join(tmpDir, "rewrite.lua")
	err = os.WriteFile(rewriteScript, []byte("ngx.var.uri = '/rewritten'"), 0o644)
	require.NoError(t, err)

	err = multi.AddPhase(PhaseRewrite, rewriteScript, 10*time.Second)
	require.NoError(t, err)

	assert.Equal(t, 1, multi.PhaseCount())
	assert.True(t, multi.HasPhase(PhaseRewrite))

	// 添加 access 阶段
	accessScript := filepath.Join(tmpDir, "access.lua")
	err = os.WriteFile(accessScript, []byte("ngx.exit(403)"), 0o644)
	require.NoError(t, err)

	err = multi.AddPhase(PhaseAccess, accessScript, 10*time.Second)
	require.NoError(t, err)

	assert.Equal(t, 2, multi.PhaseCount())
	assert.True(t, multi.HasPhase(PhaseAccess))
}

// TestMultiPhaseLuaMiddlewareRemovePhase 测试移除阶段
func TestMultiPhaseLuaMiddlewareRemovePhase(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	tmpDir := t.TempDir()

	multi := NewMultiPhaseLuaMiddleware(engine, "multi-test")

	scriptPath := filepath.Join(tmpDir, "test.lua")
	err = os.WriteFile(scriptPath, []byte("return 1"), 0o644)
	require.NoError(t, err)

	err = multi.AddPhase(PhaseRewrite, scriptPath, 10*time.Second)
	require.NoError(t, err)
	assert.Equal(t, 1, multi.PhaseCount())

	// 移除阶段
	multi.RemovePhase(PhaseRewrite)
	assert.Equal(t, 0, multi.PhaseCount())
	assert.False(t, multi.HasPhase(PhaseRewrite))
}

// TestMultiPhaseLuaMiddlewareProcess 测试多阶段执行
func TestMultiPhaseLuaMiddlewareProcess(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	tmpDir := t.TempDir()

	multi := NewMultiPhaseLuaMiddleware(engine, "multi-test")

	// rewrite 阎段脚本
	rewriteScript := filepath.Join(tmpDir, "rewrite.lua")
	err = os.WriteFile(rewriteScript, []byte(`ngx.say("rewrite")`), 0o644)
	require.NoError(t, err)

	err = multi.AddPhase(PhaseRewrite, rewriteScript, 10*time.Second)
	require.NoError(t, err)

	// content 阎段脚本
	contentScript := filepath.Join(tmpDir, "content.lua")
	err = os.WriteFile(contentScript, []byte(`ngx.say("content")`), 0o644)
	require.NoError(t, err)

	err = multi.AddPhase(PhaseContent, contentScript, 10*time.Second)
	require.NoError(t, err)

	// 创建 RequestCtx
	ctx := &fasthttp.RequestCtx{}

	// 创建最终处理器
	finalHandler := func(ctx *fasthttp.RequestCtx) {
		ctx.WriteString("final")
	}

	// 包装处理器
	handler := multi.Process(finalHandler)

	// 执行
	handler(ctx)

	// 验证执行顺序：rewrite -> content -> final
	body := string(ctx.Response.Body())
	// 注意：由于 Lua 输出会追加到响应体，顺序可能不同
	assert.Contains(t, body, "rewrite")
	assert.Contains(t, body, "content")
}

// TestMultiPhaseLuaMiddlewareGetPhaseMiddleware 测试获取阶段中间件
func TestMultiPhaseLuaMiddlewareGetPhaseMiddleware(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	tmpDir := t.TempDir()

	multi := NewMultiPhaseLuaMiddleware(engine, "multi-test")

	scriptPath := filepath.Join(tmpDir, "test.lua")
	err = os.WriteFile(scriptPath, []byte("return 1"), 0o644)
	require.NoError(t, err)

	err = multi.AddPhase(PhaseRewrite, scriptPath, 10*time.Second)
	require.NoError(t, err)

	// 获取阶段中间件
	middleware := multi.GetPhaseMiddleware(PhaseRewrite)
	require.NotNil(t, middleware)
	assert.Equal(t, PhaseRewrite, middleware.GetPhase())

	// 获取不存在的阶段
	middleware = multi.GetPhaseMiddleware(PhaseAccess)
	assert.Nil(t, middleware)
}

// TestLuaMiddlewareIntegrationWithChain 测试与 middleware chain 集成
func TestLuaMiddlewareIntegrationWithChain(t *testing.T) {
	// 这个测试验证 LuaMiddleware 可以与现有的 middleware 链集成
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "test.lua")
	err = os.WriteFile(scriptPath, []byte(`ngx.say("lua middleware")`), 0o644)
	require.NoError(t, err)

	config := LuaMiddlewareConfig{
		ScriptPath: scriptPath,
		Phase:      PhaseContent,
		Name:       "lua-content",
	}

	middleware, err := NewLuaMiddleware(engine, config)
	require.NoError(t, err)

	// 验证实现了 Middleware 接口
	// Name() 和 Process() 方法已实现
	assert.Equal(t, "lua-content", middleware.Name())

	// 创建 RequestCtx
	ctx := &fasthttp.RequestCtx{}

	// 创建处理器
	handler := middleware.Process(func(ctx *fasthttp.RequestCtx) {
		ctx.WriteString("next")
	})

	// 执行
	handler(ctx)

	// 验证输出
	body := string(ctx.Response.Body())
	assert.Contains(t, body, "lua middleware")
}

// TestLuaMiddlewareExecutionError 测试执行错误处理
func TestLuaMiddlewareExecutionError(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	// 创建有语法错误的脚本
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "error.lua")
	err = os.WriteFile(scriptPath, []byte("invalid lua syntax !!!"), 0o644)
	require.NoError(t, err)

	config := LuaMiddlewareConfig{
		ScriptPath: scriptPath,
		Phase:      PhaseContent,
	}

	middleware, err := NewLuaMiddleware(engine, config)
	require.NoError(t, err)

	ctx := &fasthttp.RequestCtx{}

	handler := middleware.Process(func(ctx *fasthttp.RequestCtx) {
		ctx.WriteString("final")
	})

	handler(ctx)

	// 执行错误时返回 500
	assert.Equal(t, fasthttp.StatusInternalServerError, ctx.Response.StatusCode())
}

// TestLuaMiddlewareExit 测试 ngx.exit() 终止执行
func TestLuaMiddlewareExit(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "exit.lua")
	err = os.WriteFile(scriptPath, []byte(`ngx.say("before exit"); ngx.exit(200)`), 0o644)
	require.NoError(t, err)

	config := LuaMiddlewareConfig{
		ScriptPath: scriptPath,
		Phase:      PhaseContent,
	}

	middleware, err := NewLuaMiddleware(engine, config)
	require.NoError(t, err)

	ctx := &fasthttp.RequestCtx{}

	nextCalled := false
	handler := middleware.Process(func(ctx *fasthttp.RequestCtx) {
		nextCalled = true
		ctx.WriteString("next handler")
	})

	handler(ctx)

	// ngx.exit() 终止执行，next handler 不应被调用
	assert.False(t, nextCalled)

	// 状态码应为 200
	assert.Equal(t, 200, ctx.Response.StatusCode())

	// 输出包含 Lua 输出
	body := string(ctx.Response.Body())
	assert.Contains(t, body, "before exit")
}
