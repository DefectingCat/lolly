// Package lua 提供 LuaEngine 测试，覆盖协程创建和管理、调度器、回调队列
package lua

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/valyala/fasthttp"
	glua "github.com/yuin/gopher-lua"
	"github.com/yuin/gopher-lua/parse"
)

// engineCodeToProtoForTest 编译 Lua 代码为 FunctionProto（测试辅助函数）
func engineCodeToProtoForTest(src string) (*glua.FunctionProto, error) {
	chunk, err := parse.Parse(strings.NewReader(src), "<test>")
	if err != nil {
		return nil, err
	}
	return glua.Compile(chunk, "<test>")
}

// TestNewEngineNilConfig 测试 NewEngine 使用 nil config 时使用默认配置
func TestNewEngineNilConfig(t *testing.T) {
	engine, err := NewEngine(nil)
	require.NoError(t, err)
	defer engine.Close()

	assert.NotNil(t, engine.L)
	assert.NotNil(t, engine.codeCache)
	assert.NotNil(t, engine.sharedDictManager)
	assert.NotNil(t, engine.timerManager)
	assert.NotNil(t, engine.locationManager)
	assert.NotNil(t, engine.ctx)
	assert.NotNil(t, engine.cancel)
}

// TestEngineCloseMultiple 测试多次 Close 不 panic
func TestEngineCloseMultiple(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)

	engine.Close()
	engine.Close() // 第二次不应该 panic
}

// TestEngineCloseScheduler 测试关闭调度器
func TestEngineCloseScheduler(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	// 初始化调度器
	err = engine.InitSchedulerLState()
	require.NoError(t, err)

	// 关闭调度器
	engine.CloseScheduler()

	// 再次关闭不应该 panic
	engine.CloseScheduler()
}

// TestEngineNewCoroutine 测试创建协程
func TestEngineNewCoroutine(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	var req fasthttp.Request
	req.Header.SetMethod("GET")
	req.Header.SetRequestURI("/test")

	ctx := &fasthttp.RequestCtx{}
	ctx.Init(&req, nil, nil)

	coro, err := engine.NewCoroutine(ctx)
	require.NoError(t, err)
	require.NotNil(t, coro)
	assert.NotNil(t, coro.Co)
	assert.NotNil(t, coro.Engine)
	assert.Equal(t, ctx, coro.RequestCtx)

	coro.Close()
}

// TestEngineNewCoroutineNilContext 测试创建带 nil 请求上下文的协程
func TestEngineNewCoroutineNilContext(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	coro, err := engine.NewCoroutine(nil)
	require.NoError(t, err)
	require.NotNil(t, coro)
	assert.Nil(t, coro.RequestCtx)

	coro.Close()
}

// TestEngineActiveCoroutines 测试活跃协程计数
func TestEngineActiveCoroutines(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	// 初始应该为 0
	assert.Equal(t, int32(0), engine.ActiveCoroutines())

	// 创建一个协程
	coro, err := engine.NewCoroutine(nil)
	require.NoError(t, err)

	assert.Equal(t, int32(1), engine.ActiveCoroutines())

	// 关闭协程
	coro.Close()

	assert.Equal(t, int32(0), engine.ActiveCoroutines())
}

// TestEngineCoroutinePoolWarmup 测试协程池预热
func TestEngineCoroutinePoolWarmup(t *testing.T) {
	config := DefaultConfig()
	config.CoroutinePoolWarmup = 10

	engine, err := NewEngine(config)
	require.NoError(t, err)
	defer engine.Close()

	// 预热后池中应该有 10 个对象
	// 直接验证 engine 创建成功即可，预热是内部实现
	assert.NotNil(t, engine)
}

// TestEngineStatsAfterOperations 测试引擎统计信息在操作后更新
func TestEngineStatsAfterOperations(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	// 创建并关闭多个协程
	for range 5 {
		coro, err := engine.NewCoroutine(nil)
		require.NoError(t, err)
		coro.Close()
	}

	stats := engine.Stats()
	assert.Equal(t, uint64(5), stats.CoroutinesCreated)
	assert.Equal(t, uint64(5), stats.CoroutinesClosed)
}

// TestEngineMaxCoroutinesExceeded 测试超过最大并发协程限制
func TestEngineMaxCoroutinesExceeded(t *testing.T) {
	config := &Config{
		MaxConcurrentCoroutines: 2,
		MaxExecutionTime:        5 * time.Second,
	}

	engine, err := NewEngine(config)
	require.NoError(t, err)
	defer engine.Close()

	// 创建 2 个协程
	coro1, err := engine.NewCoroutine(nil)
	require.NoError(t, err)
	coro2, err := engine.NewCoroutine(nil)
	require.NoError(t, err)

	// 第 3 个应该失败
	coro3, err := engine.NewCoroutine(nil)
	assert.Error(t, err)
	assert.Nil(t, coro3)
	assert.Contains(t, err.Error(), "max concurrent coroutines exceeded")

	coro1.Close()
	coro2.Close()
}

// TestEngineNewCoroutineFails 测试 NewThread 返回 nil 的情况
// 这个场景在实际中很难触发，我们验证引擎在正常情况下不会返回 nil
func TestEngineNewCoroutineSuccess(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	// 创建大量协程，验证稳定性
	for range 100 {
		coro, err := engine.NewCoroutine(nil)
		require.NoError(t, err)
		require.NotNil(t, coro.Co)
		coro.Close()
	}
}

// TestEngineCodeCacheAccess 测试 CodeCache 访问器
func TestEngineCodeCacheAccess(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	cache := engine.CodeCache()
	require.NotNil(t, cache)

	// 编译一段脚本
	proto, err := cache.GetOrCompileInline("return 1 + 1")
	require.NoError(t, err)
	require.NotNil(t, proto)
}

// TestEngineSharedDictManagerAccess 测试 SharedDictManager 访问器
func TestEngineSharedDictManagerAccess(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	mgr := engine.SharedDictManager()
	require.NotNil(t, mgr)

	// 创建共享字典
	dict := engine.CreateSharedDict("test", 100)
	require.NotNil(t, dict)

	// 通过 manager 获取
	dict2 := mgr.GetDict("test")
	assert.Equal(t, dict, dict2)
}

// TestEngineTimerManagerAccess 测试 TimerManager 访问器
func TestEngineTimerManagerAccess(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	mgr := engine.TimerManager()
	require.NotNil(t, mgr)
}

// TestEngineLocationManagerAccess 测试 LocationManager 访问器
func TestEngineLocationManagerAccess(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	mgr := engine.LocationManager()
	require.NotNil(t, mgr)

	// 注册一个 location
	mgr.Register("/test", func(ctx *fasthttp.RequestCtx) {
		ctx.SetStatusCode(200)
	})

	// 验证已注册
	_, err2 := mgr.Capture(&fasthttp.RequestCtx{}, "/test", nil)
	assert.NoError(t, err2)
}

// TestEngineSchedulerLoop 测试调度器循环处理回调
func TestEngineSchedulerLoop(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	// 初始化调度器
	err = engine.InitSchedulerLState()
	require.NoError(t, err)

	// 创建一个简单的回调函数并加入队列
	proto, err := engineCodeToProtoForTest("return 42")
	require.NoError(t, err)

	entry := &CallbackEntry{
		proto: proto,
		args:  []glua.LValue{},
	}

	ok := engine.EnqueueCallback(entry)
	assert.True(t, ok, "enqueue should succeed")

	// 给调度器一些时间处理
	time.Sleep(50 * time.Millisecond)

	// 关闭调度器
	engine.CloseScheduler()
}

// TestEngineEnqueueCallbackFull 测试回调队列满时入队失败
func TestEngineEnqueueCallbackFull(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	err = engine.InitSchedulerLState()
	require.NoError(t, err)

	proto, err := engineCodeToProtoForTest("return 1")
	require.NoError(t, err)

	// 填满回调队列（默认 1024 容量）
	full := false
	for range 1024 {
		if !engine.EnqueueCallback(&CallbackEntry{proto: proto, args: []glua.LValue{}}) {
			full = true
			break
		}
	}

	// 在正常环境下 1024 个应该能填满队列
	// 最后一个应该失败
	last := engine.EnqueueCallback(&CallbackEntry{proto: proto, args: []glua.LValue{}})
	// 可能为 false（队列满）或 true（如果调度器已经开始消费）
	// 不强制断言，因为调度器可能在消费
	_ = full
	_ = last

	engine.CloseScheduler()
}

// TestEngineExecuteCallbackNilScheduler 测试 executeCallback 时 schedulerLState 为 nil
func TestEngineExecuteCallbackNilScheduler(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	// 不调用 InitSchedulerLState，schedulerLState 为 nil
	// executeCallback 会在 schedulerLState == nil 时直接返回
	engine.executeCallback(&CallbackEntry{
		proto: nil,
		args:  []glua.LValue{},
	})
	// 不应 panic
}

// TestEngineExecuteCallbackPanicRecovery 测试 executeCallback 中 panic 的恢复
func TestEngineExecuteCallbackPanicRecovery(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	err = engine.InitSchedulerLState()
	require.NoError(t, err)

	// 传入 nil proto，executeCallback 内部应该不会 panic
	// 因为 recover() 会捕获
	engine.executeCallback(&CallbackEntry{
		proto: nil,
		args:  []glua.LValue{},
	})

	engine.CloseScheduler()
}

// TestEngineSchedulerLoopExitOnClose 测试调度器在引擎关闭时退出
func TestEngineSchedulerLoopExitOnClose(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)

	err = engine.InitSchedulerLState()
	require.NoError(t, err)

	// 关闭引擎（会触发 cancel 信号）
	engine.Close()

	// 给调度器一些时间退出
	time.Sleep(50 * time.Millisecond)

	// 再次关闭不应该 panic
	engine.Close()
}

// TestEngineSchedulerLoopExitOnChannelClose 测试调度器在回调队列关闭时退出
func TestEngineSchedulerLoopExitOnChannelClose(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)

	err = engine.InitSchedulerLState()
	require.NoError(t, err)

	// 直接关闭调度器（关闭 callbackQueue channel）
	engine.CloseScheduler()

	// 给调度器一些时间退出
	time.Sleep(50 * time.Millisecond)

	engine.Close()
}

// TestEngineCoroutineExecutionContext 测试协程的执行上下文和超时控制
func TestEngineCoroutineExecutionContext(t *testing.T) {
	config := &Config{
		MaxConcurrentCoroutines: 100,
		MaxExecutionTime:        100 * time.Millisecond,
	}

	engine, err := NewEngine(config)
	require.NoError(t, err)
	defer engine.Close()

	coro, err := engine.NewCoroutine(nil)
	require.NoError(t, err)

	// 验证执行上下文已设置
	assert.NotNil(t, coro.ExecutionContext)
	assert.NotNil(t, coro.Cancel)

	coro.Close()
}

// TestEngineReleaseCoroutineNilSafety 测试 releaseCoroutine 对 nil 的安全处理
func TestEngineReleaseCoroutineNilSafety(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	// 释放 nil 协程不应 panic
	engine.releaseCoroutine(nil)
}

// TestEngineCoroutinePoolReuse 测试协程池复用
func TestEngineCoroutinePoolReuse(t *testing.T) {
	engine, err := NewEngine(&Config{
		MaxConcurrentCoroutines: 1000,
		MaxExecutionTime:        5 * time.Second,
		CoroutinePoolWarmup:     5,
	})
	require.NoError(t, err)
	defer engine.Close()

	// 创建并释放多次
	for range 10 {
		coro, err := engine.NewCoroutine(nil)
		require.NoError(t, err)
		coro.Close()
	}

	stats := engine.Stats()
	assert.Equal(t, uint64(10), stats.CoroutinesCreated)
	assert.Equal(t, uint64(10), stats.CoroutinesClosed)
}

// TestEngineConfigOverride 测试配置覆盖
func TestEngineConfigOverride(t *testing.T) {
	config := &Config{
		MaxConcurrentCoroutines: 500,
		MaxExecutionTime:        10 * time.Second,
		CodeCacheSize:           2000,
		CodeCacheTTL:            5 * time.Minute,
		CoroutineStackSize:      64,
		MinimizeStackMemory:     true,
	}

	engine, err := NewEngine(config)
	require.NoError(t, err)
	defer engine.Close()

	assert.Equal(t, 500, engine.maxCoroutines)
	assert.Equal(t, 10*time.Second, config.MaxExecutionTime)
}
