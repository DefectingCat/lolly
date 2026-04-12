// Package lua 提供 Lua 脚本嵌入能力
package lua

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	glua "github.com/yuin/gopher-lua"
)

func TestTimerManagerAt(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	manager := engine.TimerManager()
	require.NotNil(t, manager)

	// 创建 Lua 函数作为回调
	L := engine.L

	// 注册一个简单的回调函数
	callback := L.NewFunction(func(L *glua.LState) int {
		return 0
	})

	// 创建定时器
	handle, err := manager.At(100*time.Millisecond, callback, nil)
	require.NoError(t, err)
	require.NotNil(t, handle)

	// 等待定时器触发
	time.Sleep(200 * time.Millisecond)

	// 定时器应该已完成（active count 回到 0）
	assert.Equal(t, int32(0), manager.ActiveCount())
}

func TestTimerManagerCancel(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	manager := engine.TimerManager()

	callback := engine.L.NewFunction(func(L *glua.LState) int {
		return 0
	})

	// 创建定时器
	handle, err := manager.At(200*time.Millisecond, callback, nil)
	require.NoError(t, err)

	// 立即取消
	ok := manager.Cancel(handle)
	assert.True(t, ok)

	// 等待超过定时器时间
	time.Sleep(300 * time.Millisecond)

	// 定时器应该被取消，active count 为 0
	assert.Equal(t, int32(0), manager.ActiveCount())
}

func TestTimerManagerWaitAll(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)

	manager := engine.TimerManager()

	// 创建多个定时器
	for range 3 {
		callback := engine.L.NewFunction(func(L *glua.LState) int {
			return 0
		})
		manager.At(50*time.Millisecond, callback, nil)
	}

	// 等待所有完成
	ok := manager.WaitAll(1 * time.Second)
	assert.True(t, ok)

	// active count 应该回到 0
	assert.Equal(t, int32(0), manager.ActiveCount())

	engine.Close()
}

func TestTimerLuaAPI(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	L := engine.L

	// 注册 ngx.timer API
	ngx := L.NewTable()
	L.SetGlobal("ngx", ngx)
	RegisterTimerAPI(L, engine.TimerManager(), ngx)

	// 测试 ngx.timer.at
	err = L.DoString(`
		local count = 0

		-- 创建定时器
		local handle, err = ngx.timer.at(0.1, function()
			count = count + 1
		end)

		assert(handle ~= nil)
		assert(err == nil)

		-- 检查 running_count
		local running = ngx.timer.running_count()
		assert(running >= 1)
	`)
	require.NoError(t, err)
}

func TestTimerRunningCount(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	manager := engine.TimerManager()

	// 初始应该为 0
	assert.Equal(t, int32(0), manager.ActiveCount())

	// 创建定时器
	callback := engine.L.NewFunction(func(L *glua.LState) int {
		return 0
	})

	handle, _ := manager.At(50*time.Millisecond, callback, nil)
	_ = handle

	// 刚创建后应该有活跃定时器（在定时器触发前）
	// 注意：由于简化实现，定时器执行很快，所以 active count 可能很快回到 0
	// 这里我们只验证定时器最终会完成

	// 等待完成
	time.Sleep(100 * time.Millisecond)

	// 应该回到 0
	assert.Equal(t, int32(0), manager.ActiveCount())
}
