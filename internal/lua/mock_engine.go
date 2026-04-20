// Package lua 提供 Lua 引擎的 Mock 实现，用于测试。
//
// 该文件提供 LuaEngine 和 LuaCoroutine 的 Mock 版本，通过函数指针
// 注入自定义行为，便于单元测试中模拟 Lua 脚本执行。
//
// 使用方式：
//   - 设置 ExecuteFunc 等字段来自定义方法行为
//   - 未设置的函数指针返回零值（stub 模式）
//
// 作者：xfy
package lua

import (
	"context"
	"time"

	"github.com/valyala/fasthttp"
	glua "github.com/yuin/gopher-lua"
)

// MockLuaEngine 是 LuaEngine 的 Mock 实现。
//
// 通过注入函数指针模拟 LuaEngine 的所有公开方法，
// 未注入的方法返回零值或 nil（stub 模式）。
type MockLuaEngine struct {
	// ExecuteFunc 模拟 Execute 方法
	ExecuteFunc func(script string) error

	// ExecuteFileFunc 模拟 ExecuteFile 方法
	ExecuteFileFunc func(path string) error

	// NewCoroutineFunc 模拟 NewCoroutine 方法
	NewCoroutineFunc func(ctx *fasthttp.RequestCtx) (*MockCoroutine, error)

	// CloseFunc 模拟 Close 方法
	CloseFunc func()

	// StatsFunc 模拟 Stats 方法
	StatsFunc func() EngineStats

	// ActiveCoroutinesFunc 模拟 ActiveCoroutines 方法
	ActiveCoroutinesFunc func() int32

	// CodeCacheFunc 模拟 CodeCache 方法
	CodeCacheFunc func() *CodeCache

	// SharedDictManagerFunc 模拟 SharedDictManager 方法
	SharedDictManagerFunc func() *SharedDictManager

	// TimerManagerFunc 模拟 TimerManager 方法
	TimerManagerFunc func() *TimerManager

	// LocationManagerFunc 模拟 LocationManager 方法
	LocationManagerFunc func() *LocationManager

	// CreateSharedDictFunc 模拟 CreateSharedDict 方法
	CreateSharedDictFunc func(name string, maxItems int) *SharedDict

	// InitSchedulerLStateFunc 模拟 InitSchedulerLState 方法
	InitSchedulerLStateFunc func() error

	// SchedulerLoopFunc 模拟 SchedulerLoop 方法
	SchedulerLoopFunc func()

	// EnqueueCallbackFunc 模拟 EnqueueCallback 方法
	EnqueueCallbackFunc func(entry *CallbackEntry) bool

	// CloseSchedulerFunc 模拟 CloseScheduler 方法
	CloseSchedulerFunc func()
}

// Execute 执行脚本（Mock）。
//
// 参数：
//   - script: Lua 脚本
//
// 返回值：
//   - error: ExecuteFunc 的结果，未注入时返回 nil
func (m *MockLuaEngine) Execute(script string) error {
	if m.ExecuteFunc != nil {
		return m.ExecuteFunc(script)
	}
	return nil // stub
}

// ExecuteFile 执行文件（Mock）。
//
// 参数：
//   - path: 脚本文件路径
//
// 返回值：
//   - error: ExecuteFileFunc 的结果，未注入时返回 nil
func (m *MockLuaEngine) ExecuteFile(path string) error {
	if m.ExecuteFileFunc != nil {
		return m.ExecuteFileFunc(path)
	}
	return nil // stub
}

// NewCoroutine 创建协程（Mock）。
//
// 参数：
//   - req: fasthttp 请求上下文
//
// 返回值：
//   - *MockCoroutine: 模拟协程
//   - error: NewCoroutineFunc 的结果
func (m *MockLuaEngine) NewCoroutine(req *fasthttp.RequestCtx) (*MockCoroutine, error) {
	if m.NewCoroutineFunc != nil {
		return m.NewCoroutineFunc(req)
	}
	return &MockCoroutine{}, nil
}

// Close 关闭引擎（Mock）。
func (m *MockLuaEngine) Close() {
	if m.CloseFunc != nil {
		m.CloseFunc()
	}
}

// Stats 返回统计（Mock）。
//
// 返回值：
//   - EngineStats: StatsFunc 的结果，未注入时返回零值
func (m *MockLuaEngine) Stats() EngineStats {
	if m.StatsFunc != nil {
		return m.StatsFunc()
	}
	return EngineStats{}
}

// ActiveCoroutines 返回活跃协程数（Mock）。
//
// 返回值：
//   - int32: ActiveCoroutinesFunc 的结果，未注入时返回 0
func (m *MockLuaEngine) ActiveCoroutines() int32 {
	if m.ActiveCoroutinesFunc != nil {
		return m.ActiveCoroutinesFunc()
	}
	return 0
}

// CodeCache 返回字节码缓存（Mock）。
//
// 返回值：
//   - *CodeCache: CodeCacheFunc 的结果，未注入时返回 nil
func (m *MockLuaEngine) CodeCache() *CodeCache {
	if m.CodeCacheFunc != nil {
		return m.CodeCacheFunc()
	}
	return nil
}

// SharedDictManager 返回共享字典管理器（Mock）。
//
// 返回值：
//   - *SharedDictManager: SharedDictManagerFunc 的结果，未注入时返回 nil
func (m *MockLuaEngine) SharedDictManager() *SharedDictManager {
	if m.SharedDictManagerFunc != nil {
		return m.SharedDictManagerFunc()
	}
	return nil
}

// TimerManager 返回定时器管理器（Mock）。
//
// 返回值：
//   - *TimerManager: TimerManagerFunc 的结果，未注入时返回 nil
func (m *MockLuaEngine) TimerManager() *TimerManager {
	if m.TimerManagerFunc != nil {
		return m.TimerManagerFunc()
	}
	return nil
}

// LocationManager 返回 location 管理器（Mock）。
//
// 返回值：
//   - *LocationManager: LocationManagerFunc 的结果，未注入时返回 nil
func (m *MockLuaEngine) LocationManager() *LocationManager {
	if m.LocationManagerFunc != nil {
		return m.LocationManagerFunc()
	}
	return nil
}

// CreateSharedDict 创建共享字典（Mock）。
//
// 参数：
//   - name: 字典名称
//   - maxItems: 最大条目数
//
// 返回值：
//   - *SharedDict: CreateSharedDictFunc 的结果，未注入时返回 nil
func (m *MockLuaEngine) CreateSharedDict(name string, maxItems int) *SharedDict {
	if m.CreateSharedDictFunc != nil {
		return m.CreateSharedDictFunc(name, maxItems)
	}
	return nil
}

// InitSchedulerLState 初始化调度器 LState（Mock）。
//
// 返回值：
//   - error: InitSchedulerLStateFunc 的结果，未注入时返回 nil
func (m *MockLuaEngine) InitSchedulerLState() error {
	if m.InitSchedulerLStateFunc != nil {
		return m.InitSchedulerLStateFunc()
	}
	return nil
}

// SchedulerLoop 调度器循环（Mock）。
func (m *MockLuaEngine) SchedulerLoop() {
	if m.SchedulerLoopFunc != nil {
		m.SchedulerLoopFunc()
	}
}

// EnqueueCallback 将回调加入调度队列（Mock）。
//
// 参数：
//   - entry: 回调条目
//
// 返回值：
//   - bool: EnqueueCallbackFunc 的结果，未注入时返回 false
func (m *MockLuaEngine) EnqueueCallback(entry *CallbackEntry) bool {
	if m.EnqueueCallbackFunc != nil {
		return m.EnqueueCallbackFunc(entry)
	}
	return false
}

// CloseScheduler 关闭调度器（Mock）。
func (m *MockLuaEngine) CloseScheduler() {
	if m.CloseSchedulerFunc != nil {
		m.CloseSchedulerFunc()
	}
}

// MockCoroutine 是 LuaCoroutine 的 Mock 实现。
//
// 通过注入函数指针模拟 LuaCoroutine 的核心方法，
// 同时包含模拟字段供测试验证。
type MockCoroutine struct {
	// ExecuteFunc 模拟 Execute 方法
	ExecuteFunc func(script string) error

	// ExecuteFileFunc 模拟 ExecuteFile 方法
	ExecuteFileFunc func(path string) error

	// SetupSandboxFunc 模拟 SetupSandbox 方法
	SetupSandboxFunc func() error

	// CloseFunc 模拟 Close 方法
	CloseFunc func()

	// HandleYieldFunc 模拟 handleYield 方法
	HandleYieldFunc func(values []glua.LValue) ([]glua.LValue, error)

	// CreatedAt 协程创建时间
	CreatedAt time.Time

	// ExecutionContext 执行上下文
	ExecutionContext context.Context

	// Engine 所属引擎
	Engine *MockLuaEngine

	// Co 底层 Lua 协程
	Co *glua.LState

	// Cancel 取消函数
	Cancel context.CancelFunc

	// RequestCtx fasthttp 请求上下文
	RequestCtx *fasthttp.RequestCtx

	// OutputBuffer 输出缓冲
	OutputBuffer []byte

	// Exited 退出标记
	Exited bool
}

// Execute 执行脚本（Mock）。
//
// 参数：
//   - script: Lua 脚本
//
// 返回值：
//   - error: ExecuteFunc 的结果，未注入时返回 nil
func (c *MockCoroutine) Execute(script string) error {
	if c.ExecuteFunc != nil {
		return c.ExecuteFunc(script)
	}
	return nil
}

// ExecuteFile 执行文件（Mock）。
//
// 参数：
//   - path: 脚本文件路径
//
// 返回值：
//   - error: ExecuteFileFunc 的结果，未注入时返回 nil
func (c *MockCoroutine) ExecuteFile(path string) error {
	if c.ExecuteFileFunc != nil {
		return c.ExecuteFileFunc(path)
	}
	return nil
}

// SetupSandbox 设置沙箱（Mock）。
//
// 返回值：
//   - error: SetupSandboxFunc 的结果，未注入时返回 nil
func (c *MockCoroutine) SetupSandbox() error {
	if c.SetupSandboxFunc != nil {
		return c.SetupSandboxFunc()
	}
	return nil
}

// Close 关闭协程（Mock）。
func (c *MockCoroutine) Close() {
	if c.CloseFunc != nil {
		c.CloseFunc()
	}
}
