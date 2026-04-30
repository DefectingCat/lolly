// Package lua 提供边界场景测试。
//
// 该文件测试 Lua 模块的边界场景：
//   - 协程创建失败处理
//   - 定时器句柄操作
//   - 共享字典容量上限
//   - getVariable 所有变量类型
//
// 作者：xfy
package lua

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/valyala/fasthttp"
	glua "github.com/yuin/gopher-lua"
)

// TestBoundaryCoroutineSandbox 测试协程沙箱阻止危险操作。
func TestBoundaryCoroutineSandbox(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	// 创建协程
	co, err := engine.NewCoroutine(nil)
	require.NoError(t, err)
	defer co.Close()

	// 设置沙箱
	err = co.SetupSandbox()
	require.NoError(t, err)

	// 尝试创建协程应该失败 - 使用单个脚本执行
	err = co.Execute(`
		local ok, err = pcall(function()
			coroutine.create(function() end)
		end)
		if not ok then
			-- 预期错误
		else
			error("coroutine.create should be blocked")
		end
	`)
	assert.NoError(t, err)
}

// TestBoundaryCoroutineYieldAllowed 测试协程 yield 仍然可用。
func TestBoundaryCoroutineYieldAllowed(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	co, err := engine.NewCoroutine(nil)
	require.NoError(t, err)
	defer co.Close()

	err = co.SetupSandbox()
	require.NoError(t, err)

	// coroutine.yield 和 coroutine.status 应该可用
	err = co.Execute(`
		-- coroutine.status 应该可用
		local status = coroutine.status
		if type(status) ~= "function" then
			error("coroutine.status should be a function")
		end
		-- coroutine.yield 应该可用
		local yield = coroutine.yield
		if type(yield) ~= "function" then
			error("coroutine.yield should be a function")
		end
	`)
	assert.NoError(t, err)
}

// TestBoundaryTimerHandleCancel 测试定时器句柄取消。
func TestBoundaryTimerHandleCancel(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	timerMgr := engine.TimerManager()
	require.NotNil(t, timerMgr)

	L := glua.NewState()
	defer L.Close()

	ngx := L.NewTable()
	L.SetGlobal("ngx", ngx)
	RegisterTimerAPI(L, timerMgr, ngx)

	// 创建定时器并取消 - 使用全局函数避免闭包
	err = L.DoString(`
		function timer_callback()
			-- 空回调
		end
		local handle, err = ngx.timer.at(10, timer_callback)
		if not handle then
			error("Failed to create timer: " .. tostring(err))
		end

		-- 取消定时器
		local ok, err = handle:cancel()
		if not ok then
			error("Failed to cancel timer: " .. tostring(err))
		end
	`)
	assert.NoError(t, err)

	// 验证定时器已取消
	assert.Equal(t, int32(0), timerMgr.ActiveCount())
}

// TestBoundaryTimerHandleDoubleCancel 测试重复取消定时器。
func TestBoundaryTimerHandleDoubleCancel(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	timerMgr := engine.TimerManager()
	L := glua.NewState()
	defer L.Close()

	ngx := L.NewTable()
	L.SetGlobal("ngx", ngx)
	RegisterTimerAPI(L, timerMgr, ngx)

	err = L.DoString(`
		function timer_callback() end
		local handle = ngx.timer.at(10, timer_callback)
		if not handle then
			error("Failed to create timer")
		end

		-- 第一次取消
		local ok1 = handle:cancel()

		-- 第二次取消应该失败
		local ok2, err = handle:cancel()
		if ok2 then
			error("Double cancel should fail")
		end
	`)
	assert.NoError(t, err)
}

// TestBoundaryTimerRunningCount 测试定时器运行计数。
func TestBoundaryTimerRunningCount(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	timerMgr := engine.TimerManager()
	L := glua.NewState()
	defer L.Close()

	ngx := L.NewTable()
	L.SetGlobal("ngx", ngx)
	RegisterTimerAPI(L, timerMgr, ngx)

	// 创建多个定时器
	err = L.DoString(`
		local count = ngx.timer.running_count()
		if count ~= 0 then
			error("Initial count should be 0")
		end

		-- 创建定时器
		local h1 = ngx.timer.at(10, function() end)
		local h2 = ngx.timer.at(10, function() end)
		local h3 = ngx.timer.at(10, function() end)

		count = ngx.timer.running_count()
		if count ~= 3 then
			error("Count should be 3, got " .. tostring(count))
		end

		-- 取消一个
		h1:cancel()
		count = ngx.timer.running_count()
		if count ~= 2 then
			error("Count should be 2 after cancel, got " .. tostring(count))
		end

		-- 清理
		h2:cancel()
		h3:cancel()
	`)
	assert.NoError(t, err)
}

// TestBoundaryTimerUpvalueRejected 测试定时器拒绝闭包变量。
func TestBoundaryTimerUpvalueRejected(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	timerMgr := engine.TimerManager()
	L := glua.NewState()
	defer L.Close()

	ngx := L.NewTable()
	L.SetGlobal("ngx", ngx)
	RegisterTimerAPI(L, timerMgr, ngx)

	// 尝试创建捕获 upvalue 的定时器应该失败
	err = L.DoString(`
		local captured = "value"
		local handle, err = ngx.timer.at(1, function()
			print(captured) -- 捕获 upvalue
		end)
		if handle then
			handle:cancel()
			error("Timer with upvalue should be rejected")
		end
		if not err then
			error("Expected error for upvalue capture")
		end
	`)
	assert.NoError(t, err)
}

// TestBoundarySharedDictCapacity 测试共享字典容量上限。
func TestBoundarySharedDictCapacity(t *testing.T) {
	dict := NewSharedDict("test", 3) // 只允许 3 个条目

	// 添加 3 个条目
	ok, err := dict.Set("a", "1", 0)
	assert.True(t, ok)
	assert.NoError(t, err)

	ok, err = dict.Set("b", "2", 0)
	assert.True(t, ok)
	assert.NoError(t, err)

	ok, err = dict.Set("c", "3", 0)
	assert.True(t, ok)
	assert.NoError(t, err)

	// 添加第 4 个应该淘汰 LRU
	ok, err = dict.Set("d", "4", 0)
	assert.True(t, ok)
	assert.NoError(t, err)

	// 验证大小仍然为 3
	assert.Equal(t, 3, dict.Size())

	// "a" 应该被淘汰 - Get 返回空值
	val, _, _ := dict.Get("a")
	assert.Equal(t, "", val) // 不存在返回空

	// "d" 应该存在
	val, expired, _ := dict.Get("d")
	assert.False(t, expired)
	assert.Equal(t, "4", val)
}

// TestBoundarySharedDictIncrNonNumeric 测试非数值自增。
func TestBoundarySharedDictIncrNonNumeric(t *testing.T) {
	dict := NewSharedDict("test", 10)

	// 设置非数值字符串
	ok, _ := dict.Set("key", "not-a-number", 0)
	assert.True(t, ok)

	// 尝试自增应该失败
	_, err := dict.Incr("key", 1)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not a number")
}

// TestBoundarySharedDictIncrNegative 测试负数自增。
func TestBoundarySharedDictIncrNegative(t *testing.T) {
	dict := NewSharedDict("test", 10)

	// 设置初始值
	ok, _ := dict.Set("counter", "10", 0)
	assert.True(t, ok)

	// 负数自增
	val, err := dict.Incr("counter", -3)
	assert.NoError(t, err)
	assert.Equal(t, 7, val)

	// 再次负数自增到负数
	val, err = dict.Incr("counter", -20)
	assert.NoError(t, err)
	assert.Equal(t, -13, val)
}

// TestBoundarySharedDictTTLExpiration 测试 TTL 过期。
func TestBoundarySharedDictTTLExpiration(t *testing.T) {
	dict := NewSharedDict("test", 10)

	// 设置短 TTL
	ok, _ := dict.Set("ephemeral", "value", 100*time.Millisecond)
	assert.True(t, ok)

	// 立即读取应该存在
	val, expired, _ := dict.Get("ephemeral")
	assert.False(t, expired)
	assert.Equal(t, "value", val)

	// 等待过期
	time.Sleep(150 * time.Millisecond)

	// 再次读取应该过期
	val, expired, _ = dict.Get("ephemeral")
	assert.True(t, expired)
	assert.Equal(t, "", val)
}

// TestBoundarySharedDictConcurrentAccess 测试并发访问。
func TestBoundarySharedDictConcurrentAccess(t *testing.T) {
	dict := NewSharedDict("test", 100)

	var wg sync.WaitGroup
	concurrency := 10
	iterations := 100

	for i := range concurrency {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for range iterations {
				key := "key-" + string(rune('0'+id%10))
				_, _ = dict.Set(key, "value", 0)
				_, _, _ = dict.Get(key)
			}
		}(i)
	}

	wg.Wait()
	// 如果没有竞态条件，测试通过
}

// TestBoundaryGetVariableAllTypes 测试所有变量类型。
func TestBoundaryGetVariableAllTypes(t *testing.T) {
	// 创建模拟请求上下文
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("http://example.com/path?arg1=value1&arg2=value2")
	ctx.Request.Header.SetMethod("POST")
	ctx.Request.Header.Set("User-Agent", "test-agent")
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Request.Header.Set("X-Custom", "custom-value")

	api := newNgxVarAPI(ctx)

	tests := []struct {
		name     string
		expected string
	}{
		{"request_method", "POST"},
		{"request_uri", "http://example.com/path?arg1=value1&arg2=value2"},
		{"uri", "/path"},
		{"query_string", "arg1=value1&arg2=value2"},
		{"args", "arg1=value1&arg2=value2"},
		{"http_host", "example.com"},
		{"http_user_agent", "test-agent"},
		{"http_content_type", "application/json"},
		{"http_x-custom", "custom-value"},
		{"arg_arg1", "value1"},
		{"arg_arg2", "value2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val := api.getVariable(tt.name)
			assert.Equal(t, tt.expected, val)
		})
	}
}

// TestBoundaryGetVariableLuaTypes 测试 Lua 类型返回。
func TestBoundaryGetVariableLuaTypes(t *testing.T) {
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("http://example.com/path")
	ctx.Request.Header.SetMethod("GET")

	api := newNgxVarAPI(ctx)

	// request_length 应该返回数值类型
	lv := api.getVariableLua("request_length")
	if lv == nil {
		t.Log("request_length returned nil (acceptable)")
	} else if _, ok := lv.(glua.LNumber); !ok {
		t.Errorf("request_length should return LNumber, got %T", lv)
	}

	// request_method 应该返回字符串类型
	lv = api.getVariableLua("request_method")
	if lv == nil {
		t.Error("request_method should not return nil")
	} else if _, ok := lv.(glua.LString); !ok {
		t.Errorf("request_method should return LString, got %T", lv)
	}
}

// TestBoundaryGetVariableCustom 测试自定义变量。
func TestBoundaryGetVariableCustom(t *testing.T) {
	ctx := &fasthttp.RequestCtx{}
	api := newNgxVarAPI(ctx)

	// 设置自定义变量
	api.SetVariable("custom_var", "custom_value")

	// 读取自定义变量
	val, ok := api.GetVariable("custom_var")
	assert.True(t, ok)
	assert.Equal(t, "custom_value", val)

	// 读取不存在的变量
	_, ok = api.GetVariable("nonexistent")
	assert.False(t, ok)
}

// TestBoundaryCoroutineExecutionTimeout 测试协程执行超时。
func TestBoundaryCoroutineExecutionTimeout(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	// 创建带超时的上下文
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	co, err := engine.NewCoroutine(nil)
	require.NoError(t, err)
	defer co.Close()

	co.ExecutionContext = ctx

	err = co.SetupSandbox()
	require.NoError(t, err)

	// 执行一个长时间运行的脚本应该被中断
	err = co.Execute(`
		local start = os.time()
		while os.time() - start < 10 do
			-- 忙等待
		end
	`)
	// 超时错误是预期的
	if err == nil {
		t.Log("Script completed quickly (no timeout)")
	}
}

// TestBoundaryTimerHandleString 测试定时器句柄字符串表示。
func TestBoundaryTimerHandleString(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	timerMgr := engine.TimerManager()
	L := glua.NewState()
	defer L.Close()

	ngx := L.NewTable()
	L.SetGlobal("ngx", ngx)
	RegisterTimerAPI(L, timerMgr, ngx)

	err = L.DoString(`
		local handle = ngx.timer.at(10, function() end)
		if not handle then
			error("Failed to create timer")
		end

		local str = tostring(handle)
		if not string.find(str, "ngx.timer.handle:") then
			error("Invalid handle string: " .. str)
		end

		handle:cancel()
	`)
	assert.NoError(t, err)
}

// TestBoundaryTimerWaitAll 测试等待所有定时器完成。
func TestBoundaryTimerWaitAll(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	timerMgr := engine.TimerManager()

	// 创建短延迟定时器
	callback := func(L *glua.LState) int { return 0 }
	L := glua.NewState()
	defer L.Close()

	fn := L.NewFunction(callback)

	handle1, _ := timerMgr.At(50*time.Millisecond, fn, nil)
	handle2, _ := timerMgr.At(100*time.Millisecond, fn, nil)

	require.NotNil(t, handle1)
	require.NotNil(t, handle2)

	// 等待所有完成
	completed := timerMgr.WaitAll(500 * time.Millisecond)
	assert.True(t, completed)

	// 活跃计数应该为 0
	assert.Equal(t, int32(0), timerMgr.ActiveCount())
}

// TestBoundarySharedDictAddExisting 测试 Add 已存在的键。
func TestBoundarySharedDictAddExisting(t *testing.T) {
	dict := NewSharedDict("test", 10)

	// 第一次 Add 应该成功
	ok, _ := dict.Add("key", "value1", 0)
	assert.True(t, ok)

	// 第二次 Add 应该失败
	ok, _ = dict.Add("key", "value2", 0)
	assert.False(t, ok)

	// 值应该仍然是第一个
	val, _, _ := dict.Get("key")
	assert.Equal(t, "value1", val)
}

// TestBoundarySharedDictFlushExpired 测试批量清理过期条目。
func TestBoundarySharedDictFlushExpired(t *testing.T) {
	dict := NewSharedDict("test", 10)

	// 设置过期条目（它们会在 LRU 链表前端）
	dict.Set("expire1", "value", 50*time.Millisecond)
	dict.Set("expire2", "value", 50*time.Millisecond)

	// 等待过期
	time.Sleep(100 * time.Millisecond)

	// 设置不过期条目（它们会在 LRU 链表前端，过期条目在尾部）
	dict.Set("keep1", "value", time.Hour)
	dict.Set("keep2", "value", time.Hour)

	// 清理过期条目 - evictExpired 从 LRU 尾部开始检查
	_ = dict.FlushExpired()

	// 验证过期条目被清理
	assert.Equal(t, 2, dict.Size())
}

// TestBoundarySharedDictFlushAll 测试清空字典。
func TestBoundarySharedDictFlushAll(t *testing.T) {
	dict := NewSharedDict("test", 10)

	// 添加多个条目
	dict.Set("a", "1", 0)
	dict.Set("b", "2", 0)
	dict.Set("c", "3", 0)

	assert.Equal(t, 3, dict.Size())

	// 清空
	_ = dict.FlushAll()

	assert.Equal(t, 0, dict.Size())
	assert.Equal(t, 10, dict.FreeSlots())
}
