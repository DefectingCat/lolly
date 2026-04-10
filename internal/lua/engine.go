// Package lua 提供 Lua 脚本嵌入能力
package lua

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	glua "github.com/yuin/gopher-lua"
	"github.com/valyala/fasthttp"
)

// LuaEngine 全局 Lua 引擎
// 每个 HTTP Server 实例持有一个 LuaEngine
type LuaEngine struct {
	// 主 LState
	L *glua.LState

	// 配置
	config *Config

	// 字节码缓存
	codeCache *CodeCache

	// 协程管理
	activeCount   int32         // 活跃协程数
	maxCoroutines int           // 最大并发协程数
	coroutinePool sync.Pool     // 协程对象池（注意：池中的协程已 dead，不可复用，仅复用内存）

	// 生命周期
	ctx    context.Context
	cancel context.CancelFunc

	// 统计
	stats EngineStats
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
		L:             L,
		config:        config,
		codeCache:     NewCodeCache(config.CodeCacheSize, config.CodeCacheTTL, config.EnableFileWatch),
		maxCoroutines: config.MaxConcurrentCoroutines,
		ctx:           ctx,
		cancel:        cancel,
		coroutinePool: sync.Pool{
			New: func() interface{} {
				// 注意：这里只是创建空的协程对象结构
				// 实际的协程通过 L.NewThread() 创建
				return &LuaCoroutine{}
			},
		},
	}

	return engine, nil
}

// Close 关闭引擎
func (e *LuaEngine) Close() {
	e.cancel()
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