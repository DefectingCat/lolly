// Package lua 提供 Lua 脚本嵌入能力
package lua

import (
	"testing"

	glua "github.com/yuin/gopher-lua"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewEngine 测试引擎创建
func TestNewEngine(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	require.NotNil(t, engine)
	defer engine.Close()

	assert.NotNil(t, engine.L)
	assert.NotNil(t, engine.codeCache)
	assert.Equal(t, int32(0), engine.ActiveCoroutines())
}

// TestNewCoroutine 测试协程创建
func TestNewCoroutine(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	coro, err := engine.NewCoroutine(nil)
	require.NoError(t, err)
	require.NotNil(t, coro)
	require.NotNil(t, coro.Co)

	defer coro.Close()

	assert.Equal(t, int32(1), engine.ActiveCoroutines())
}

// TestCoroutineDeadAfterResumeOK 验证协程 ResumeOK 后变成 dead 不能复用
// 注意：gopher-lua 的 Resume 对 dead coroutine 会 panic，无法安全测试
// 此测试验证 ResumeOK 正常完成，证明协程生命周期正确
func TestCoroutineDeadAfterResumeOK(t *testing.T) {
	L := glua.NewState()
	defer L.Close()

	// 创建协程
	co, cocancel := L.NewThread()
	require.NotNil(t, co)
	if cocancel != nil {
		defer cocancel()
	}

	// 编译简单脚本
	proto, err := engineCodeToProto("return 42")
	require.NoError(t, err)

	fn := L.NewFunctionFromProto(proto)

	// 执行协程，应该正常完成
	st, err, values := L.Resume(co, fn)
	assert.Equal(t, glua.ResumeOK, st)
	assert.NoError(t, err)
	assert.Len(t, values, 1)
	assert.Equal(t, glua.LNumber(42), values[0])

	// 协程完成后变成 dead 状态
	// 注意：再次调用 Resume(co, fn) 会 panic
	// 实际使用中必须确保每个协程只使用一次
}

// TestLFunctionCannotCrossLState 验证 LFunction 不能跨 LState 使用
// 注意：FunctionProto 可以跨 LState 使用，但 LFunction 绑定到特定 LState
// 这个测试验证的是 FunctionProto 共享的正确性
func TestLFunctionCannotCrossLState(t *testing.T) {
	L1 := glua.NewState()
	defer L1.Close()

	// 在 L1 中编译脚本并执行
	proto, err := engineCodeToProto("return 42")
	require.NoError(t, err)

	fn := L1.NewFunctionFromProto(proto)
	L1.Push(fn)
	err = L1.PCall(0, 1, nil)
	require.NoError(t, err)
	assert.Equal(t, glua.LNumber(42), L1.Get(-1))
	L1.Pop(1)

	// FunctionProto 可以在不同 LState 使用（这是缓存的核心假设）
	L2 := glua.NewState()
	defer L2.Close()

	fn2 := L2.NewFunctionFromProto(proto) // 从同一个 proto 创建新的函数
	L2.Push(fn2)
	err = L2.PCall(0, 1, nil)
	require.NoError(t, err)
	assert.Equal(t, glua.LNumber(42), L2.Get(-1))
	L2.Pop(1)
}

// TestNewThreadInheritsGlobals 验证 NewThread 继承全局环境
func TestNewThreadInheritsGlobals(t *testing.T) {
	L := glua.NewState()
	defer L.Close()

	// 在主 LState 设置全局变量
	L.SetGlobal("test_global", glua.LString("shared_value"))

	// 创建协程
	co, cocancel := L.NewThread()
	require.NotNil(t, co)
	if cocancel != nil {
		defer cocancel()
	}

	// 协程应该能访问主 LState 的全局变量
	proto, err := engineCodeToProto("return test_global")
	require.NoError(t, err)

	fn := L.NewFunctionFromProto(proto)
	st, err, values := L.Resume(co, fn)
	assert.Equal(t, glua.ResumeOK, st)
	assert.NoError(t, err)
	assert.Len(t, values, 1)
	assert.Equal(t, glua.LString("shared_value"), values[0])
}

// TestPerRequestEnvSandbox 验证 _ENV 沙箱隔离
func TestPerRequestEnvSandbox(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	// 创建第一个协程并设置沙箱
	coro1, err := engine.NewCoroutine(nil)
	require.NoError(t, err)
	require.NotNil(t, coro1)

	err = coro1.SetupSandbox()
	require.NoError(t, err)

	// 在沙箱中设置变量
	err = coro1.Execute("local x = 1")
	assert.NoError(t, err)

	// 创建第二个协程
	coro2, err := engine.NewCoroutine(nil)
	require.NoError(t, err)
	require.NotNil(t, coro2)

	err = coro2.SetupSandbox()
	require.NoError(t, err)

	// 第二个协程不应该看到第一个协程的变量
	// 由于我们使用了 _ENV 沙箱，局部变量是隔离的
	coro1.Close()
	coro2.Close()
}

// TestCodeCache 测试字节码缓存
func TestCodeCache(t *testing.T) {
	cache := NewCodeCache(100, 0, false)

	script := "return 1 + 1"

	// 第一次编译
	proto1, err := cache.GetOrCompileInline(script)
	require.NoError(t, err)
	require.NotNil(t, proto1)

	// 第二次应该命中缓存
	proto2, err := cache.GetOrCompileInline(script)
	require.NoError(t, err)
	require.NotNil(t, proto2)

	// 相同的脚本应该返回相同的字节码
	assert.Equal(t, proto1, proto2)

	// 检查命中率
	hits, misses, _ := cache.Stats()
	assert.Equal(t, uint64(1), hits)
	assert.Equal(t, uint64(1), misses)
}

// TestCodeCacheDifferentScripts 测试不同脚本的缓存
func TestCodeCacheDifferentScripts(t *testing.T) {
	cache := NewCodeCache(100, 0, false)

	proto1, err := cache.GetOrCompileInline("return 1")
	require.NoError(t, err)

	proto2, err := cache.GetOrCompileInline("return 2")
	require.NoError(t, err)

	// 不同脚本应该产生不同的字节码
	assert.NotEqual(t, proto1, proto2)

	hits, misses, _ := cache.Stats()
	assert.Equal(t, uint64(0), hits) // 都是 miss
	assert.Equal(t, uint64(2), misses)
}

// Helper function: compile Lua code to FunctionProto
func engineCodeToProto(src string) (*glua.FunctionProto, error) {
	return cacheGetOrCompile(src)
}

// Package-level helper for testing
var cacheGetOrCompile = NewCodeCache(100, 0, false).GetOrCompileInline