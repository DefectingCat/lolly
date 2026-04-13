// Package lua 提供 Lua 引擎的 Mock 实现，用于测试
package lua

import (
	"context"
	"time"

	"github.com/valyala/fasthttp"
	glua "github.com/yuin/gopher-lua"
)

// MockLuaEngine 是 LuaEngine 的 Mock 实现
type MockLuaEngine struct {
	ExecuteFunc             func(script string) error
	ExecuteFileFunc         func(path string) error
	NewCoroutineFunc        func(ctx *fasthttp.RequestCtx) (*MockCoroutine, error)
	CloseFunc               func()
	StatsFunc               func() EngineStats
	ActiveCoroutinesFunc    func() int32
	CodeCacheFunc           func() *CodeCache
	SharedDictManagerFunc   func() *SharedDictManager
	TimerManagerFunc        func() *TimerManager
	LocationManagerFunc     func() *LocationManager
	CreateSharedDictFunc    func(name string, maxItems int) *SharedDict
	InitSchedulerLStateFunc func() error
	SchedulerLoopFunc       func()
	EnqueueCallbackFunc     func(entry *CallbackEntry) bool
	CloseSchedulerFunc      func()
}

// Execute 执行脚本
func (m *MockLuaEngine) Execute(script string) error {
	if m.ExecuteFunc != nil {
		return m.ExecuteFunc(script)
	}
	return nil // stub
}

// ExecuteFile 执行文件
func (m *MockLuaEngine) ExecuteFile(path string) error {
	if m.ExecuteFileFunc != nil {
		return m.ExecuteFileFunc(path)
	}
	return nil // stub
}

// NewCoroutine 创建协程
func (m *MockLuaEngine) NewCoroutine(req *fasthttp.RequestCtx) (*MockCoroutine, error) {
	if m.NewCoroutineFunc != nil {
		return m.NewCoroutineFunc(req)
	}
	return &MockCoroutine{}, nil
}

// Close 关闭引擎
func (m *MockLuaEngine) Close() {
	if m.CloseFunc != nil {
		m.CloseFunc()
	}
}

// Stats 返回统计
func (m *MockLuaEngine) Stats() EngineStats {
	if m.StatsFunc != nil {
		return m.StatsFunc()
	}
	return EngineStats{}
}

// ActiveCoroutines 返回活跃协程数
func (m *MockLuaEngine) ActiveCoroutines() int32 {
	if m.ActiveCoroutinesFunc != nil {
		return m.ActiveCoroutinesFunc()
	}
	return 0
}

// CodeCache 返回字节码缓存
func (m *MockLuaEngine) CodeCache() *CodeCache {
	if m.CodeCacheFunc != nil {
		return m.CodeCacheFunc()
	}
	return nil
}

// SharedDictManager 返回共享字典管理器
func (m *MockLuaEngine) SharedDictManager() *SharedDictManager {
	if m.SharedDictManagerFunc != nil {
		return m.SharedDictManagerFunc()
	}
	return nil
}

// TimerManager 返回定时器管理器
func (m *MockLuaEngine) TimerManager() *TimerManager {
	if m.TimerManagerFunc != nil {
		return m.TimerManagerFunc()
	}
	return nil
}

// LocationManager 返回 location 管理器
func (m *MockLuaEngine) LocationManager() *LocationManager {
	if m.LocationManagerFunc != nil {
		return m.LocationManagerFunc()
	}
	return nil
}

// CreateSharedDict 创建共享字典
func (m *MockLuaEngine) CreateSharedDict(name string, maxItems int) *SharedDict {
	if m.CreateSharedDictFunc != nil {
		return m.CreateSharedDictFunc(name, maxItems)
	}
	return nil
}

// InitSchedulerLState 初始化调度器 LState
func (m *MockLuaEngine) InitSchedulerLState() error {
	if m.InitSchedulerLStateFunc != nil {
		return m.InitSchedulerLStateFunc()
	}
	return nil
}

// SchedulerLoop 调度器循环
func (m *MockLuaEngine) SchedulerLoop() {
	if m.SchedulerLoopFunc != nil {
		m.SchedulerLoopFunc()
	}
}

// EnqueueCallback 将回调加入调度队列
func (m *MockLuaEngine) EnqueueCallback(entry *CallbackEntry) bool {
	if m.EnqueueCallbackFunc != nil {
		return m.EnqueueCallbackFunc(entry)
	}
	return false
}

// CloseScheduler 关闭调度器
func (m *MockLuaEngine) CloseScheduler() {
	if m.CloseSchedulerFunc != nil {
		m.CloseSchedulerFunc()
	}
}

// MockCoroutine 是 LuaCoroutine 的 Mock 实现
type MockCoroutine struct {
	ExecuteFunc      func(script string) error
	ExecuteFileFunc  func(path string) error
	SetupSandboxFunc func() error
	CloseFunc        func()
	HandleYieldFunc  func(values []glua.LValue) ([]glua.LValue, error)

	// 模拟字段
	CreatedAt        time.Time
	ExecutionContext context.Context
	Engine           *MockLuaEngine
	Co               *glua.LState
	Cancel           context.CancelFunc
	RequestCtx       *fasthttp.RequestCtx
	OutputBuffer     []byte
	Exited           bool
}

// Execute 执行脚本
func (c *MockCoroutine) Execute(script string) error {
	if c.ExecuteFunc != nil {
		return c.ExecuteFunc(script)
	}
	return nil
}

// ExecuteFile 执行文件
func (c *MockCoroutine) ExecuteFile(path string) error {
	if c.ExecuteFileFunc != nil {
		return c.ExecuteFileFunc(path)
	}
	return nil
}

// SetupSandbox 设置沙箱
func (c *MockCoroutine) SetupSandbox() error {
	if c.SetupSandboxFunc != nil {
		return c.SetupSandboxFunc()
	}
	return nil
}

// Close 关闭协程
func (c *MockCoroutine) Close() {
	if c.CloseFunc != nil {
		c.CloseFunc()
	}
}
