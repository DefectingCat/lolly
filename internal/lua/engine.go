// Package lua 提供 Lua 脚本嵌入能力。
//
// 该文件包含 Lua 引擎的核心实现，包括：
//   - LuaEngine：全局引擎，每个 HTTP Server 实例持有一个
//   - LuaCoroutine：请求级临时协程，生命周期与请求绑定
//   - CodeCache：字节码缓存，支持 LRU 淘汰和文件变更检测
//   - 调度器：专用的 LState 用于定时器回调执行，实现线程隔离
//
// 架构设计：
//
//	采用 Server 级单 LState + 请求级临时协程架构。
//	所有请求共享一个主 LState 的全局环境，但各自拥有独立的协程状态，
//	确保请求间的数据隔离性和并发安全性。
//
// 主要用途：
//
//	用于在 fasthttp 服务中嵌入 Lua 脚本，实现动态请求处理、
//	负载均衡、响应过滤等可编程功能，兼容 OpenResty/ngx_lua API 语义。
//
// 注意事项：
//   - LuaEngine 非并发安全，NewEngine/Close 应在初始化/关闭阶段调用
//   - LuaCoroutine 为请求级独占，不可跨请求复用
//   - 协程在 ResumeOK 后变成 dead 状态，不能复用
//
// 作者：xfy
package lua

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/valyala/fasthttp"
	glua "github.com/yuin/gopher-lua"
)

// LuaEngine 全局 Lua 引擎。
//
// 每个 HTTP Server 实例持有一个 LuaEngine，负责：
//   - 管理主 LState（全局 Lua 状态机）
//   - 创建和回收请求级协程（LuaCoroutine）
//   - 管理字节码缓存（CodeCache）
//   - 管理共享字典、定时器、location 等子系统
//   - 提供调度器 LState 用于定时器回调的线程隔离执行
//
// 类型命名说明：虽然 lua.LuaEngine 存在 stuttering，但保持此命名以：
// 1) 与 LuaContext/LuaCoroutine 保持一致的 API 命名风格
// 2) 明确区分 Lua 引擎与其他引擎类型
// 3) 保持向后兼容性
type LuaEngine struct {
	// 主 LState，所有协程通过 NewThread 继承其全局环境
	L *glua.LState

	// 引擎配置
	config *Config

	// 字节码缓存
	codeCache *CodeCache

	// 协程池，复用 LuaCoroutine 结构体内存（不复用协程状态）
	coroutinePool sync.Pool

	// 共享字典管理器
	sharedDictManager *SharedDictManager

	// 定时器管理器
	timerManager *TimerManager

	// location 管理器（子请求）
	locationManager *LocationManager

	// 调度器 LState，用于执行定时器回调
	schedulerLState *glua.LState

	// 回调队列，定时器触发后将回调入队
	callbackQueue chan *CallbackEntry

	// 缓存：coroutine 库函数（避免并发读取 Engine.L）
	coroYieldFn  glua.LValue
	coroStatusFn glua.LValue

	// ngx 表注册锁（保护并发写入共享的全局 ngx 表）
	ngxRegisterMu sync.Mutex

	// 上下文及取消函数
	ctx    context.Context
	cancel context.CancelFunc

	// 并发控制
	maxCoroutines int
	activeCount   atomic.Int32

	// 引擎统计
	stats EngineStats
}

// EngineStats 引擎统计信息。
//
// 记录引擎运行期间的关键指标，用于监控和诊断。
// 所有字段均为原子操作，并发安全。
type EngineStats struct {
	// CoroutinesCreated 已创建的协程总数
	CoroutinesCreated uint64

	// CoroutinesClosed 已关闭的协程总数
	CoroutinesClosed uint64

	// ScriptsExecuted 成功执行的脚本总数
	ScriptsExecuted uint64

	// ScriptsErrors 执行出错的脚本总数
	ScriptsErrors uint64
}

// NewEngine 创建并初始化 Lua 引擎。
//
// 该函数执行以下初始化步骤：
//  1. 创建主 LState，配置栈大小和内存优化选项
//  2. 加载安全的标准库（base、table、string、math、coroutine）
//  3. 按需加载危险库（os、io），默认禁止 package 库
//  4. 初始化字节码缓存、共享字典、定时器、location 管理器
//  5. 执行协程池预热
//
// 参数：
//   - config: 引擎配置，为 nil 时使用 DefaultConfig()
//
// 返回值：
//   - *LuaEngine: 初始化完成的引擎实例
//   - error: 初始化失败时返回错误
//
// 使用示例：
//
//	engine, err := lua.NewEngine(nil) // 使用默认配置
//	if err != nil {
//	    // 处理初始化错误
//	}
//	defer engine.Close()
//
// 注意事项：
//   - 该方法应在服务启动阶段调用，不应在请求处理路径中调用
//   - 返回的引擎需要在使用完毕后调用 Close() 释放资源
func NewEngine(config *Config) (*LuaEngine, error) {
	if config == nil {
		config = DefaultConfig()
	}

	// 步骤1: 创建主 LState（使用优化后的栈配置）
	// 协程通过 NewThread 继承这些配置
	L := glua.NewState(glua.Options{
		SkipOpenLibs:        true, // 禁用默认库，手动加载安全库
		CallStackSize:       config.CoroutineStackSize,
		MinimizeStackMemory: config.MinimizeStackMemory,
	})

	// 步骤2: 加载安全的标准库
	glua.OpenBase(L)
	glua.OpenTable(L)
	glua.OpenString(L)
	glua.OpenMath(L)
	glua.OpenCoroutine(L) // 加载 coroutine 库支持 yield

	// 步骤3: 可选加载危险库
	if config.EnableOSLib {
		glua.OpenOs(L)
	}
	if config.EnableIOLib {
		glua.OpenIo(L)
	}
	// 注意：package 库默认不加载，禁止 require 外部模块

	ctx, cancel := context.WithCancel(context.Background())

	engine := &LuaEngine{
		L:                 L,
		config:            config,
		codeCache:         NewCodeCache(config.CodeCacheSize, config.CodeCacheTTL, config.EnableFileWatch),
		maxCoroutines:     config.MaxConcurrentCoroutines,
		ctx:               ctx,
		cancel:            cancel,
		sharedDictManager: NewSharedDictManager(),
		coroutinePool: sync.Pool{
			New: func() any {
				// 注意：这里只是创建空的协程对象结构
				// 实际的协程通过 L.NewThread() 创建
				return &LuaCoroutine{}
			},
		},
	}

	// 步骤4: 创建定时器管理器（需要在 engine 创建后初始化）
	engine.timerManager = NewTimerManager(engine)

	// 步骤5: 创建 location 管理器
	engine.locationManager = NewLocationManager()

	// 步骤6: 协程池预热：预创建 LuaCoroutine 结构体对象
	if config.CoroutinePoolWarmup > 0 {
		for i := 0; i < config.CoroutinePoolWarmup; i++ {
			engine.coroutinePool.Put(&LuaCoroutine{})
		}
	}

	// 步骤7: 缓存 coroutine 库的安全函数（避免并发读取 Engine.L）
	coroTable := L.GetGlobal("coroutine")
	if coroTable != glua.LNil {
		if tbl, ok := coroTable.(*glua.LTable); ok {
			engine.coroYieldFn = tbl.RawGetString("yield")
			engine.coroStatusFn = tbl.RawGetString("status")
		}
	}

	return engine, nil
}

// Close 关闭 Lua 引擎，释放所有资源。
//
// 关闭顺序：
//  1. 取消引擎上下文，通知所有子 goroutine 退出
//  2. 关闭定时器管理器（等待定时器回调排空）
//  3. 关闭共享字典管理器
//  4. 关闭主 LState
//
// 注意：该方法是幂等的，可安全调用多次。
func (e *LuaEngine) Close() {
	if e == nil || e.L == nil {
		return // 已关闭或 nil
	}

	e.cancel()
	if e.timerManager != nil {
		e.timerManager.Close()
	}
	if e.sharedDictManager != nil {
		e.sharedDictManager.Close()
	}
	if e.L != nil {
		e.L.Close()
	}
	// 标记为已关闭，防止重复关闭
	e.L = nil
}

// NewCoroutine 创建请求级临时协程。
//
// 该方法执行以下操作：
//  1. 检查并发限制，超过最大协程数时返回错误
//  2. 通过主 LState.NewThread() 创建底层 Lua 协程
//  3. 从对象池中获取 LuaCoroutine 结构体（复用内存）
//  4. 设置执行上下文（含超时控制）和请求上下文
//
// 参数：
//   - req: fasthttp 请求上下文，用于 API 访问（ngx.req、ngx.resp 等）
//
// 返回值：
//   - *LuaCoroutine: 新创建的协程实例
//   - error: 创建失败时返回错误（如超出并发限制）
//
// 注意事项：
//   - 协程在 ResumeOK 后变成 dead 状态，不能复用
//   - 使用完毕后必须调用 Close() 或 releaseCoroutine() 释放资源
func (e *LuaEngine) NewCoroutine(req *fasthttp.RequestCtx) (*LuaCoroutine, error) {
	// 步骤1: 检查并发限制
	current := e.activeCount.Add(1)
	if current > int32(e.maxCoroutines) {
		e.activeCount.Add(-1)
		return nil, fmt.Errorf("max concurrent coroutines exceeded: %d/%d", current, e.maxCoroutines)
	}

	// 步骤2: 通过 NewThread 创建协程
	// 协程继承主 LState 的全局环境
	co, cancel := e.L.NewThread()
	if co == nil {
		e.activeCount.Add(-1)
		return nil, fmt.Errorf("failed to create coroutine")
	}

	// 步骤3: 从池中获取协程对象结构（复用内存，不复用协程状态）
	coro, ok := e.coroutinePool.Get().(*LuaCoroutine)
	if !ok {
		coro = &LuaCoroutine{}
	}
	coro.Engine = e
	coro.Co = co
	coro.Cancel = cancel
	coro.RequestCtx = req
	coro.CreatedAt = time.Now()
	coro.ExecutionContext, coro.executionCancel = context.WithTimeout(e.ctx, e.config.MaxExecutionTime)

	// 步骤4: 设置 LState 的上下文为执行上下文（用于超时控制）
	// 注意：不直接使用 RequestCtx，因为 RequestCtx.Done() 依赖服务器连接
	// RequestCtx 通过 coro.RequestCtx 字段访问，而不是 L.Context()
	co.SetContext(coro.ExecutionContext)

	atomic.AddUint64(&e.stats.CoroutinesCreated, 1)

	return coro, nil
}

// releaseCoroutine 释放协程资源并放回对象池。
//
// 该方法执行以下清理操作：
//  1. 取消执行上下文和协程
//  2. 清空所有引用字段，防止内存泄漏
//  3. 更新活跃协程计数和关闭计数
//  4. 将 LuaCoroutine 结构体放回对象池（仅复用内存）
//
// 注意：这是内部方法，外部应通过 LuaCoroutine.Close() 间接调用。
func (e *LuaEngine) releaseCoroutine(coro *LuaCoroutine) {
	if coro == nil {
		return
	}

	// 步骤1: 取消执行上下文
	if coro.executionCancel != nil {
		coro.executionCancel()
	}

	// 步骤2: 取消协程
	if coro.Cancel != nil {
		coro.Cancel()
	}

	// 步骤3: 清理状态，防止内存泄漏
	coro.Co = nil
	coro.Cancel = nil
	coro.RequestCtx = nil
	coro.ExecutionContext = nil
	coro.executionCancel = nil

	// 步骤4: 更新计数
	e.activeCount.Add(-1)
	atomic.AddUint64(&e.stats.CoroutinesClosed, 1)

	// 步骤5: 放回池中（仅复用 LuaCoroutine 结构体内存）
	e.coroutinePool.Put(coro)
}

// CodeCache 返回字节码缓存实例。
//
// 返回值：
//   - *CodeCache: 字节码缓存，用于脚本编译缓存
func (e *LuaEngine) CodeCache() *CodeCache {
	return e.codeCache
}

// Stats 返回引擎运行统计信息。
//
// 返回值：
//   - EngineStats: 包含创建/关闭协程数、执行/出错脚本数的统计快照
//
// 注意：返回值为快照副本，非实时引用。
func (e *LuaEngine) Stats() EngineStats {
	return EngineStats{
		CoroutinesCreated: atomic.LoadUint64(&e.stats.CoroutinesCreated),
		CoroutinesClosed:  atomic.LoadUint64(&e.stats.CoroutinesClosed),
		ScriptsExecuted:   atomic.LoadUint64(&e.stats.ScriptsExecuted),
		ScriptsErrors:     atomic.LoadUint64(&e.stats.ScriptsErrors),
	}
}

// ActiveCoroutines 返回当前活跃的协程数量。
//
// 返回值：
//   - int32: 当前正在执行的协程数
func (e *LuaEngine) ActiveCoroutines() int32 {
	return e.activeCount.Load()
}

// SharedDictManager 返回共享字典管理器实例。
//
// 返回值：
//   - *SharedDictManager: 用于管理多个命名的 SharedDict 实例
func (e *LuaEngine) SharedDictManager() *SharedDictManager {
	return e.sharedDictManager
}

// CreateSharedDict 创建或获取指定名称的共享字典。
//
// 参数：
//   - name: 字典名称
//   - maxItems: 字典最大条目数（LRU 淘汰阈值）
//
// 返回值：
//   - *SharedDict: 共享字典实例
func (e *LuaEngine) CreateSharedDict(name string, maxItems int) *SharedDict {
	return e.sharedDictManager.CreateDict(name, maxItems)
}

// TimerManager 返回定时器管理器实例。
//
// 返回值：
//   - *TimerManager: 用于管理 ngx.timer.* API 的定时器
func (e *LuaEngine) TimerManager() *TimerManager {
	return e.timerManager
}

// LocationManager 返回 location 管理器实例。
//
// 返回值：
//   - *LocationManager: 用于管理 ngx.location.capture 子请求
func (e *LuaEngine) LocationManager() *LocationManager {
	return e.locationManager
}

// InitSchedulerLState 初始化调度器 LState。
//
// 创建专用的 LState 用于定时器回调执行，实现与请求处理线程的隔离。
// 该调度器 LState 仅加载安全子集库，禁止危险操作。
//
// 初始化步骤：
//  1. 创建 LState（跳过默认库）
//  2. 加载安全库（base、table、string、math）
//  3. 注册安全的 API（ngx.shared、ngx.log、ngx.timer）
//  4. 创建回调队列（容量 1024）
//  5. 启动调度器 goroutine
//
// 返回值：
//   - error: 初始化失败时返回错误
//
// 注意事项：
//   - 该方法应在引擎启动后、定时器使用前调用
//   - 调度器 LState 与主 LState 共享同一个共享字典管理器
func (e *LuaEngine) InitSchedulerLState() error {
	// 步骤1: 创建调度器 LState
	e.schedulerLState = glua.NewState(glua.Options{
		SkipOpenLibs: true, // 禁用默认库，手动加载安全库
	})

	// 步骤2: 加载安全的标准库
	glua.OpenBase(e.schedulerLState)
	glua.OpenTable(e.schedulerLState)
	glua.OpenString(e.schedulerLState)
	glua.OpenMath(e.schedulerLState)

	// 步骤3: 创建 ngx 表并注册安全 API
	ngx := e.schedulerLState.NewTable()
	e.schedulerLState.SetGlobal("ngx", ngx)

	// 注册共享字典 API（与主引擎共享同一个管理器）
	RegisterSharedDictAPI(e.schedulerLState, e.sharedDictManager, ngx)

	// 注册日志 API
	RegisterNgxLogAPI(e.schedulerLState, nil)

	// 注册定时器 API（仅安全函数）
	RegisterTimerAPI(e.schedulerLState, e.timerManager, ngx)

	// 步骤4: 创建回调队列
	e.callbackQueue = make(chan *CallbackEntry, 1024)

	// 步骤5: 启动调度器 goroutine
	go e.SchedulerLoop()

	return nil
}

// SchedulerLoop 调度器循环。
//
// 在独立的 goroutine 中运行，持续监听回调队列和引擎上下文：
//   - 从 callbackQueue 接收定时器回调并执行
//   - 监听 ctx.Done() 信号，引擎关闭时退出循环
//
// 注意：该方法由 InitSchedulerLState 自动启动，不应手动调用。
func (e *LuaEngine) SchedulerLoop() {
	for {
		select {
		case entry, ok := <-e.callbackQueue:
			if !ok {
				// 通道已关闭，退出调度器
				return
			}
			e.executeCallback(entry)

		case <-e.ctx.Done():
			// 引擎关闭信号
			return
		}
	}
}

// executeCallback 执行单个定时器回调。
//
// 该函数从 FunctionProto 重建 Lua 函数并在调度器 LState 中调用。
// 使用 Protect 模式捕获执行错误，防止回调 panic 导致调度器崩溃。
//
// 参数：
//   - entry: 回调队列条目，包含编译后的 FunctionProto 和参数
//
// 注意事项：
//   - 使用 defer+recover 捕获 panic，保护调度器稳定性
//   - 错误在 Protect 模式下被 gopher-lua 内部捕获，不向外传播
func (e *LuaEngine) executeCallback(entry *CallbackEntry) {
	defer func() {
		if r := recover(); r != nil {
			// 捕获 panic，防止调度器崩溃
			_ = r
		}
	}()

	if e.schedulerLState == nil {
		return
	}

	// 从 FunctionProto 创建函数
	fn := e.schedulerLState.NewFunctionFromProto(entry.proto)

	// 调用回调函数（不添加额外的 fn 参数）
	_ = e.schedulerLState.CallByParam(glua.P{
		Fn:      fn,
		NRet:    0,
		Protect: true,
	}, entry.args...)
	// 错误已在 Protect 模式下被捕获
}

// EnqueueCallback 将回调加入调度队列。
//
// 由 TimerManager 在定时器触发时调用，将回调推入 callbackQueue。
//
// 参数：
//   - entry: 回调条目
//
// 返回值：
//   - bool: true 表示入队成功，false 表示队列已满（回调被丢弃）
//
// 注意事项：
//   - 使用非阻塞发送，队列满时直接返回 false
//   - 丢弃的回调不会被重试
func (e *LuaEngine) EnqueueCallback(entry *CallbackEntry) bool {
	select {
	case e.callbackQueue <- entry:
		return true
	default:
		// 队列已满
		return false
	}
}

// CloseScheduler 关闭调度器。
//
// 执行以下操作：
//  1. 关闭回调队列（阻止新回调入队）
//  2. 关闭调度器 LState
//
// 注意：该方法是幂等的，可安全调用多次。
func (e *LuaEngine) CloseScheduler() {
	if e.callbackQueue != nil {
		close(e.callbackQueue)
		e.callbackQueue = nil
	}
	if e.schedulerLState != nil {
		e.schedulerLState.Close()
		e.schedulerLState = nil
	}
}
