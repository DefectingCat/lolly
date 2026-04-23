// Package lua 提供 Scheduler 模式测试。
//
// 该文件测试 Lua Scheduler 模式的功能：
//   - setupSchedulerNgxAPI
//   - setSchedulerMode / IsSchedulerMode
//   - SchedulerUnsafeLocationAPI
//   - SchedulerUnsafeVarAPI
//   - SchedulerUnsafeCtxAPI
//
// 作者：xfy
package lua

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	glua "github.com/yuin/gopher-lua"
)

// TestSchedulerModeFlag 测试 Scheduler 模式标志。
func TestSchedulerModeFlag(t *testing.T) {
	L := glua.NewState()
	defer L.Close()

	// 初始状态不是 scheduler 模式
	if IsSchedulerMode(L) {
		t.Error("Initial state should not be scheduler mode")
	}

	// 设置 scheduler 模式
	setSchedulerMode(L, true)
	if !IsSchedulerMode(L) {
		t.Error("Should be in scheduler mode after setting")
	}

	// 关闭 scheduler 模式
	setSchedulerMode(L, false)
	if IsSchedulerMode(L) {
		t.Error("Should not be in scheduler mode after unsetting")
	}
}

// TestSetupSchedulerNgxAPI 测试 setupSchedulerNgxAPI 创建安全的 ngx API。
func TestSetupSchedulerNgxAPI(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	L := glua.NewState()
	defer L.Close()

	// 调用 setupSchedulerNgxAPI
	setupSchedulerNgxAPI(L, engine)

	// 验证 ngx 表存在
	ngx := L.GetGlobal("ngx")
	if ngx == glua.LNil {
		t.Fatal("ngx table should exist")
	}

	ngxTable, ok := ngx.(*glua.LTable)
	if !ok {
		t.Fatal("ngx should be a table")
	}

	// 验证 scheduler 模式已设置
	if !IsSchedulerMode(L) {
		t.Error("Should be in scheduler mode after setupSchedulerNgxAPI")
	}

	// 验证 ngx.shared 存在
	shared := ngxTable.RawGetString("shared")
	if shared == glua.LNil {
		t.Error("ngx.shared should exist")
	}

	// 验证 ngx.log 存在
	log := ngxTable.RawGetString("log")
	if log == glua.LNil {
		t.Error("ngx.log should exist")
	}

	// 验证 ngx.timer 存在
	timer := ngxTable.RawGetString("timer")
	if timer == glua.LNil {
		t.Error("ngx.timer should exist")
	}
}

// TestSchedulerUnsafeLocationAPI 测试 Scheduler 模式下 ngx.location 不可用。
func TestSchedulerUnsafeLocationAPI(t *testing.T) {
	L := glua.NewState()
	defer L.Close()

	ngx := L.NewTable()
	L.SetGlobal("ngx", ngx)

	// 注册不安全的 location API
	RegisterSchedulerUnsafeLocationAPI(L, ngx)

	// 验证 ngx.location 存在
	location := ngx.RawGetString("location")
	if location == glua.LNil {
		t.Fatal("ngx.location should exist")
	}

	// 尝试调用 ngx.location.capture 应该返回错误
	err := L.DoString(`
		local ok, err = pcall(function()
			ngx.location.capture("/test")
		end)
		if ok then
			error("ngx.location.capture should fail in scheduler mode")
		end
	`)
	assert.NoError(t, err)
}

// TestSchedulerUnsafeVarAPI 测试 Scheduler 模式下 ngx.var 不可用。
func TestSchedulerUnsafeVarAPI(t *testing.T) {
	L := glua.NewState()
	defer L.Close()

	ngx := L.NewTable()
	L.SetGlobal("ngx", ngx)

	// 注册不安全的 var API
	RegisterSchedulerUnsafeVarAPI(L, ngx)

	// 验证 ngx.var 存在
	v := ngx.RawGetString("var")
	if v == glua.LNil {
		t.Fatal("ngx.var should exist")
	}

	// 尝试读取 ngx.var 应该返回错误
	err := L.DoString(`
		local ok, err = pcall(function()
			local x = ngx.var.some_var
		end)
		if ok then
			error("reading ngx.var should fail in scheduler mode")
		end
	`)
	assert.NoError(t, err)

	// 尝试写入 ngx.var 应该返回错误
	err = L.DoString(`
		local ok, err = pcall(function()
			ngx.var.some_var = "value"
		end)
		if ok then
			error("writing ngx.var should fail in scheduler mode")
		end
	`)
	assert.NoError(t, err)
}

// TestSchedulerUnsafeCtxAPI 测试 Scheduler 模式下 ngx.ctx 不可用。
func TestSchedulerUnsafeCtxAPISeparate(t *testing.T) {
	L := glua.NewState()
	defer L.Close()

	ngx := L.NewTable()
	L.SetGlobal("ngx", ngx)

	// 注册不安全的 ctx API
	RegisterSchedulerUnsafeCtxAPI(L, ngx)

	// 验证 ngx.ctx 存在
	ctx := ngx.RawGetString("ctx")
	if ctx == glua.LNil {
		t.Fatal("ngx.ctx should exist")
	}

	// 尝试读取 ngx.ctx 应该返回错误
	err := L.DoString(`
		local ok, err = pcall(function()
			local x = ngx.ctx.some_key
		end)
		if ok then
			error("reading ngx.ctx should fail in scheduler mode")
		end
	`)
	assert.NoError(t, err)

	// 尝试写入 ngx.ctx 应该返回错误
	err = L.DoString(`
		local ok, err = pcall(function()
			ngx.ctx.some_key = "value"
		end)
		if ok then
			error("writing ngx.ctx should fail in scheduler mode")
		end
	`)
	assert.NoError(t, err)
}

// TestSchedulerUnsafeLogAPI 测试 Scheduler 模式下 ngx.log 可用。
func TestSchedulerUnsafeLogAPI(t *testing.T) {
	L := glua.NewState()
	defer L.Close()

	ngx := L.NewTable()
	L.SetGlobal("ngx", ngx)

	// 注册 Scheduler 日志 API
	RegisterSchedulerLogAPI(L, ngx)

	// 验证 ngx.log 存在
	log := ngx.RawGetString("log")
	if log == glua.LNil {
		t.Fatal("ngx.log should exist")
	}

	// ngx.log 应该可以调用（不会报错）
	err := L.DoString(`
		ngx.log(ngx.ERR, "test log message")
	`)
	assert.NoError(t, err)
}

// TestSchedulerUnsafeReqAPI 测试 Scheduler 模式下 ngx.req 不可用。
func TestSchedulerUnsafeReqAPISeparate(t *testing.T) {
	L := glua.NewState()
	defer L.Close()

	ngx := L.NewTable()
	L.SetGlobal("ngx", ngx)

	// 注册不安全的 req API
	RegisterSchedulerUnsafeReqAPI(L, ngx)

	// 验证 ngx["ngx.req"] 存在（RegisterUnsafeAPI 使用完整名称作为键）
	req := ngx.RawGetString("ngx.req")
	if req == glua.LNil {
		t.Fatal("ngx[\"ngx.req\"] should exist")
	}

	// 尝试调用 ngx.req.get_method 应该返回错误
	err := L.DoString(`
		local ok, err = pcall(function()
			ngx["ngx.req"].get_method()
		end)
		if ok then
			error("ngx.req.get_method should fail in scheduler mode")
		end
	`)
	assert.NoError(t, err)
}

// TestSchedulerUnsafeRespAPI 测试 Scheduler 模式下 ngx.resp 不可用。
func TestSchedulerUnsafeRespAPISeparate(t *testing.T) {
	L := glua.NewState()
	defer L.Close()

	ngx := L.NewTable()
	L.SetGlobal("ngx", ngx)

	// 注册不安全的 resp API
	RegisterSchedulerUnsafeRespAPI(L, ngx)

	// 验证 ngx["ngx.resp"] 存在（RegisterUnsafeAPI 使用完整名称作为键）
	resp := ngx.RawGetString("ngx.resp")
	if resp == glua.LNil {
		t.Fatal("ngx[\"ngx.resp\"] should exist")
	}

	// 尝试调用 ngx.resp.get_headers 应该返回错误
	err := L.DoString(`
		local ok, err = pcall(function()
			ngx["ngx.resp"].get_headers()
		end)
		if ok then
			error("ngx.resp.get_headers should fail in scheduler mode")
		end
	`)
	assert.NoError(t, err)
}

// TestSchedulerSharedDictAvailable 测试 Scheduler 模式下 ngx.shared 可用。
func TestSchedulerSharedDictAvailable(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	// 创建共享字典
	engine.CreateSharedDict("test_dict", 100)

	L := glua.NewState()
	defer L.Close()

	// 调用 setupSchedulerNgxAPI
	setupSchedulerNgxAPI(L, engine)

	// 验证 ngx.shared.DICT 存在
	err = L.DoString(`
		if ngx.shared == nil then
			error("ngx.shared should exist")
		end
		local dict = ngx.shared.DICT("test_dict")
		if dict == nil then
			error("ngx.shared.DICT('test_dict') should return dict")
		end
	`)
	assert.NoError(t, err)
}

// TestSchedulerTimerAvailable 测试 Scheduler 模式下 ngx.timer 可用。
func TestSchedulerTimerAvailable(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	L := glua.NewState()
	defer L.Close()

	// 调用 setupSchedulerNgxAPI
	setupSchedulerNgxAPI(L, engine)

	// 验证 ngx.timer 存在
	err = L.DoString(`
		if ngx.timer == nil then
			error("ngx.timer should exist")
		end
		if ngx.timer.at == nil then
			error("ngx.timer.at should exist")
		end
	`)
	assert.NoError(t, err)
}

// TestSchedulerModeIsolation 测试 Scheduler 模式与普通模式隔离。
func TestSchedulerModeIsolation(t *testing.T) {
	// 创建两个 LState，一个在 scheduler 模式，一个不在
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	// 普通 LState
	normalL := glua.NewState()
	defer normalL.Close()

	// Scheduler LState
	schedulerL := glua.NewState()
	defer schedulerL.Close()

	setupSchedulerNgxAPI(schedulerL, engine)

	// 验证普通 LState 不是 scheduler 模式
	if IsSchedulerMode(normalL) {
		t.Error("Normal LState should not be in scheduler mode")
	}

	// 验证 Scheduler LState 是 scheduler 模式
	if !IsSchedulerMode(schedulerL) {
		t.Error("Scheduler LState should be in scheduler mode")
	}
}

// TestSchedulerErrorMessages 测试 Scheduler 模式下错误消息格式。
func TestSchedulerErrorMessages(t *testing.T) {
	L := glua.NewState()
	defer L.Close()

	ngx := L.NewTable()
	L.SetGlobal("ngx", ngx)

	RegisterSchedulerUnsafeLocationAPI(L, ngx)
	RegisterSchedulerUnsafeVarAPI(L, ngx)
	RegisterSchedulerUnsafeCtxAPI(L, ngx)

	// 测试 location 错误消息
	err := L.DoString(`
		local ok, err = pcall(function()
			ngx.location.capture("/test")
		end)
		if not string.find(err, "not available") then
			error("Error message should mention 'not available': " .. tostring(err))
		end
	`)
	assert.NoError(t, err)

	// 测试 var 错误消息
	err = L.DoString(`
		local ok, err = pcall(function()
			local x = ngx.var.test
		end)
		if not string.find(err, "not available") then
			error("Error message should mention 'not available': " .. tostring(err))
		end
	`)
	assert.NoError(t, err)

	// 测试 ctx 错误消息
	err = L.DoString(`
		local ok, err = pcall(function()
			local x = ngx.ctx.test
		end)
		if not string.find(err, "not available") then
			error("Error message should mention 'not available': " .. tostring(err))
		end
	`)
	assert.NoError(t, err)
}

// TestSchedulerMultipleUnsafeCalls 测试多次调用不安全 API。
func TestSchedulerMultipleUnsafeCalls(t *testing.T) {
	L := glua.NewState()
	defer L.Close()

	ngx := L.NewTable()
	L.SetGlobal("ngx", ngx)

	RegisterSchedulerUnsafeVarAPI(L, ngx)

	// 多次调用都应该失败
	err := L.DoString(`
		for i = 1, 5 do
			local ok, err = pcall(function()
				ngx.var["key" .. i] = "value" .. i
			end)
			if ok then
				error("Call " .. i .. " should have failed")
			end
		end
	`)
	assert.NoError(t, err)
}

// TestSchedulerNilValueHandling 测试 Scheduler 模式下处理 nil 值。
func TestSchedulerNilValueHandling(t *testing.T) {
	L := glua.NewState()
	defer L.Close()

	ngx := L.NewTable()
	L.SetGlobal("ngx", ngx)

	RegisterSchedulerUnsafeVarAPI(L, ngx)

	// 尝试读取 nil 键
	err := L.DoString(`
		local ok, err = pcall(function()
			local x = ngx.var[nil]
		end)
		if ok then
			error("Reading nil key should fail")
		end
	`)
	assert.NoError(t, err)
}

// TestSchedulerConcurrentAccess 测试并发访问 Scheduler API。
func TestSchedulerConcurrentAccess(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	// 创建多个 LState 并发访问
	done := make(chan bool, 10)

	for i := 0; i < 10; i++ {
		go func() {
			L := glua.NewState()
			defer L.Close()

			setupSchedulerNgxAPI(L, engine)

			// 验证 scheduler 模式
			if !IsSchedulerMode(L) {
				t.Error("Should be in scheduler mode")
			}

			done <- true
		}()
	}

	// 等待所有 goroutine 完成
	for i := 0; i < 10; i++ {
		<-done
	}
}
