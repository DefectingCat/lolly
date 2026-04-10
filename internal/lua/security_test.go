// Package lua 提供 Lua 脚本嵌入能力
package lua

import (
	"testing"

	glua "github.com/yuin/gopher-lua"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSandboxBlocksCoroutineCreate 验证协程创建被阻止
func TestSandboxBlocksCoroutineCreate(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	// 测试直接调用被阻止
	coro1, err := engine.NewCoroutine(nil)
	require.NoError(t, err)
	err = coro1.SetupSandbox()
	require.NoError(t, err)
	err = coro1.Execute(`coroutine.create(function() end)`)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "blocked")
	coro1.Close()

	// 测试通过 _G 访问被阻止（需要新协程，因为前一个已 dead）
	coro2, err := engine.NewCoroutine(nil)
	require.NoError(t, err)
	err = coro2.SetupSandbox()
	require.NoError(t, err)
	err = coro2.Execute(`_G.coroutine.create(function() end)`)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "blocked")
	coro2.Close()
}

// TestGlobalCoroutineBypassAttempt 验证 _G.coroutine 绕过尝试失败
func TestGlobalCoroutineBypassAttempt(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	// 测试 _G.coroutine.create 绕过
	coro1, err := engine.NewCoroutine(nil)
	require.NoError(t, err)
	err = coro1.SetupSandbox()
	require.NoError(t, err)
	err = coro1.Execute(`_G.coroutine.create(function() end)`)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "blocked")
	coro1.Close()

	// 测试字符串索引绕过
	coro2, err := engine.NewCoroutine(nil)
	require.NoError(t, err)
	err = coro2.SetupSandbox()
	require.NoError(t, err)
	err = coro2.Execute(`local c = _G["coroutine"]; c.create(function() end)`)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "blocked")
	coro2.Close()
}

// TestSandboxBlocksCoroutineWrap 验证 coroutine.wrap 被阻止
func TestSandboxBlocksCoroutineWrap(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	coro, err := engine.NewCoroutine(nil)
	require.NoError(t, err)
	defer coro.Close()

	err = coro.SetupSandbox()
	require.NoError(t, err)

	err = coro.Execute(`coroutine.wrap(function() end)`)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "blocked")
}

// TestSandboxPreservesYield 验证 yield 正常工作
func TestSandboxPreservesYield(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	coro, err := engine.NewCoroutine(nil)
	require.NoError(t, err)
	defer coro.Close()

	err = coro.SetupSandbox()
	require.NoError(t, err)

	// yield 应该正常工作（由引擎控制）
	err = coro.Execute(`coroutine.yield("sleep", 0.001)`)
	assert.NoError(t, err)
}

// TestSandboxPreservesStatus 验证 status 可用
func TestSandboxPreservesStatus(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	coro, err := engine.NewCoroutine(nil)
	require.NoError(t, err)
	defer coro.Close()

	err = coro.SetupSandbox()
	require.NoError(t, err)

	// status 应该可用
	err = coro.Execute(`local s = coroutine.status; return s`)
	assert.NoError(t, err)
}

// TestDebugLibraryNotLoaded 验证 debug 库未加载
func TestDebugLibraryNotLoaded(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	coro, err := engine.NewCoroutine(nil)
	require.NoError(t, err)
	defer coro.Close()

	// debug 库应该不存在
	debug := engine.L.GetGlobal("debug")
	assert.Equal(t, glua.LNil, debug, "debug library should not be loaded")

	// 尝试访问 debug.getregistry 应该失败
	err = coro.Execute(`return debug.getregistry()`)
	assert.Error(t, err)
}

// TestCoroutineRunningBlocked 验证 coroutine.running 被阻止（防止信息泄露）
func TestCoroutineRunningBlocked(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	coro, err := engine.NewCoroutine(nil)
	require.NoError(t, err)
	defer coro.Close()

	err = coro.SetupSandbox()
	require.NoError(t, err)

	err = coro.Execute(`coroutine.running()`)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "blocked")
}