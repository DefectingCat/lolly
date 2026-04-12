// Package lua 提供 Lua 脚本嵌入能力
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

// LuaEngine 全局 Lua 引擎
// 每个 HTTP Server 实例持有一个 LuaEngine
//
// 类型命名说明：虽然 lua.LuaEngine 存在 stuttering，但保持此命名以：
// 1) 与 LuaContext/LuaCoroutine 保持一致的 API 命名风格
// 2) 明确区分 Lua 引擎与其他引擎类型
// 3) 保持向后兼容性
type LuaEngine struct {
	// 主 LState
	L *glua.LState

	// 调度器 LState（专用于定时器回调，线程隔离）
	schedulerLState *glua.LState

	// 配置
	config *Config

	// 字节码缓存
	codeCache *CodeCache

	// 协程管理
	activeCount   int32     // 活跃协程数
	maxCoroutines int       // 最大并发协程数
	coroutinePool sync.Pool // 协程对象池（注意：池中的协程已 dead，不可复用，仅复用内存）

	// 生命周期
	ctx    context.Context
	cancel context.CancelFunc

	// 共享字典管理器
	sharedDictManager *SharedDictManager

	// 定时器管理器
	timerManager *TimerManager

	// location 管理器
	locationManager *LocationManager

	// 统计
	stats EngineStats

	// 定时器回调队列（调度器 goroutine 专用）
	callbackQueue chan *CallbackEntry
}

// EngineStats 引擎统计信息
type EngineStats struct {
	CoroutinesCreated uint64
	CoroutinesClosed  uint64
	ScriptsExecuted   uint64
	ScriptsErrors     uint64
}

// NewEngine 创建 Lua 引擎
func NewEngine(config *Config) (*LuaEngine, error) {
	if config == nil {
		config = DefaultConfig()
	}

	// 创建主 LState
	L := glua.NewState(glua.Options{
		SkipOpenLibs: true, // 禁用默认库，手动加载安全库
	})

	// 加载安全的标准库
	glua.OpenBase(L)
	glua.OpenTable(L)
	glua.OpenString(L)
	glua.OpenMath(L)
	glua.OpenCoroutine(L) // 加载 coroutine 库支持 yield

	// 可选加载危险库
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
			New: func() interface{} {
				// 注意：这里只是创建空的协程对象结构
				// 实际的协程通过 L.NewThread() 创建
				return &LuaCoroutine{}
			},
		},
	}

	// 创建定时器管理器（需要在 engine 创建后初始化）
	engine.timerManager = NewTimerManager(engine)

	// 创建 location 管理器
	engine.locationManager = NewLocationManager()

	return engine, nil
}

// Close 关闭引擎
func (e *LuaEngine) Close() {
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
}

// NewCoroutine 创建临时协程
// 注意：协程在 ResumeOK 后变成 dead 状态，不能复用
func (e *LuaEngine) NewCoroutine(req *fasthttp.RequestCtx) (*LuaCoroutine, error) {
	// 检查并发限制
	current := atomic.AddInt32(&e.activeCount, 1)
	if current > int32(e.maxCoroutines) {
		atomic.AddInt32(&e.activeCount, -1)
		return nil, fmt.Errorf("max concurrent coroutines exceeded: %d/%d", current, e.maxCoroutines)
	}

	// 通过 NewThread 创建协程
	// 协程继承主 LState 的全局环境
	co, cancel := e.L.NewThread()
	if co == nil {
		atomic.AddInt32(&e.activeCount, -1)
		return nil, fmt.Errorf("failed to create coroutine")
	}

	// 从池中获取协程对象结构（复用内存，不复用协程状态）
	coro := e.coroutinePool.Get().(*LuaCoroutine)
	coro.Engine = e
	coro.Co = co
	coro.Cancel = cancel
	coro.RequestCtx = req
	coro.CreatedAt = time.Now()
	coro.ExecutionContext, coro.executionCancel = context.WithTimeout(e.ctx, e.config.MaxExecutionTime)

	// 设置 LState 的上下文，使 getRequestCtx 能够获取到 RequestCtx
	co.SetContext(req)

	atomic.AddUint64(&e.stats.CoroutinesCreated, 1)

	return coro, nil
}

// releaseCoroutine 释放协程（内部方法）
func (e *LuaEngine) releaseCoroutine(coro *LuaCoroutine) {
	if coro == nil {
		return
	}

	// 取消执行上下文
	if coro.executionCancel != nil {
		coro.executionCancel()
	}

	// 取消协程
	if coro.Cancel != nil {
		coro.Cancel()
	}

	// 清理状态
	coro.Co = nil
	coro.Cancel = nil
	coro.RequestCtx = nil
	coro.ExecutionContext = nil
	coro.executionCancel = nil

	// 更新计数
	atomic.AddInt32(&e.activeCount, -1)
	atomic.AddUint64(&e.stats.CoroutinesClosed, 1)

	// 放回池中（仅复用 LuaCoroutine 结构体内存）
	e.coroutinePool.Put(coro)
}

// CodeCache 返回字节码缓存
func (e *LuaEngine) CodeCache() *CodeCache {
	return e.codeCache
}

// Stats 返回引擎统计
func (e *LuaEngine) Stats() EngineStats {
	return EngineStats{
		CoroutinesCreated: atomic.LoadUint64(&e.stats.CoroutinesCreated),
		CoroutinesClosed:  atomic.LoadUint64(&e.stats.CoroutinesClosed),
		ScriptsExecuted:   atomic.LoadUint64(&e.stats.ScriptsExecuted),
		ScriptsErrors:     atomic.LoadUint64(&e.stats.ScriptsErrors),
	}
}

// ActiveCoroutines 返回活跃协程数
func (e *LuaEngine) ActiveCoroutines() int32 {
	return atomic.LoadInt32(&e.activeCount)
}

// SharedDictManager 返回共享字典管理器
func (e *LuaEngine) SharedDictManager() *SharedDictManager {
	return e.sharedDictManager
}

// CreateSharedDict 创建共享字典
func (e *LuaEngine) CreateSharedDict(name string, maxItems int) *SharedDict {
	return e.sharedDictManager.CreateDict(name, maxItems)
}

// TimerManager 返回定时器管理器
func (e *LuaEngine) TimerManager() *TimerManager {
	return e.timerManager
}

// LocationManager 返回 location 管理器
func (e *LuaEngine) LocationManager() *LocationManager {
	return e.locationManager
}

// InitSchedulerLState 初始化调度器 LState
// 创建专用的 LState 用于定时器回调执行，线程隔离
func (e *LuaEngine) InitSchedulerLState() error {
	// 创建调度器 LState
	e.schedulerLState = glua.NewState(glua.Options{
		SkipOpenLibs: true, // 禁用默认库，手动加载安全库
	})

	// 加载安全的标准库
	glua.OpenBase(e.schedulerLState)
	glua.OpenTable(e.schedulerLState)
	glua.OpenString(e.schedulerLState)
	glua.OpenMath(e.schedulerLState)

	// 创建 ngx 表
	ngx := e.schedulerLState.NewTable()
	e.schedulerLState.SetGlobal("ngx", ngx)

	// 注册共享字典 API（与主引擎共享同一个管理器）
	RegisterSharedDictAPI(e.schedulerLState, e.sharedDictManager, ngx)

	// 注册日志 API
	RegisterNgxLogAPI(e.schedulerLState, nil)

	// 注册定时器 API（仅安全函数）
	RegisterTimerAPI(e.schedulerLState, e.timerManager, ngx)

	// 创建回调队列
	e.callbackQueue = make(chan *CallbackEntry, 1024)

	// 启动调度器 goroutine
	go e.SchedulerLoop()

	return nil
}

// SchedulerLoop 调度器循环
// 在独立的 goroutine 中运行，处理定时器回调
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

// executeCallback 执行定时器回调
func (e *LuaEngine) executeCallback(entry *CallbackEntry) {
	defer func() {
		if r := recover(); r != nil {
			// 捕获 panic，防止调度器崩溃
		}
	}()

	if e.schedulerLState == nil {
		return
	}

	// 从 FunctionProto 创建函数
	fn := e.schedulerLState.NewFunctionFromProto(entry.proto)

	// 调用回调函数（不添加额外的 fn 参数）
	err := e.schedulerLState.CallByParam(glua.P{
		Fn:      fn,
		NRet:    0,
		Protect: true,
	}, entry.args...)

	if err != nil {
		// 错误已在 Protect 模式下被捕获
	}
}

// EnqueueCallback 将回调加入调度队列
// 由 TimerManager 在定时器触发时调用
func (e *LuaEngine) EnqueueCallback(entry *CallbackEntry) bool {
	select {
	case e.callbackQueue <- entry:
		return true
	default:
		// 队列已满
		return false
	}
}

// CloseScheduler 关闭调度器
func (e *LuaEngine) CloseScheduler() {
	if e.callbackQueue != nil {
		close(e.callbackQueue)
	}
	if e.schedulerLState != nil {
		e.schedulerLState.Close()
		e.schedulerLState = nil
	}
}
