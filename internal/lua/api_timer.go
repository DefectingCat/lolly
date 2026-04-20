// Package lua 提供 ngx.timer API 实现。
//
// 该文件实现定时器相关的 Lua API，兼容 OpenResty/ngx_lua 语义。
// 包括：
//   - TimerManager：定时器管理器，支持延迟执行和取消
//   - TimerEntry：单个定时器条目，包含回调和参数
//   - TimerHandle：Lua 可见的定时器句柄（userdata）
//   - 调度器 goroutine：在专用 LState 中安全执行 Lua 回调
//
// 设计说明：
//   - 回调通过 FunctionProto 编译后在调度器 goroutine 中执行
//   - 不允许回调捕获 upvalue（闭包变量），防止内存泄漏
//   - 优雅关闭：等待回调队列排空，超时后放弃剩余回调
//
// 注意事项：
//   - 定时器回调中不可用 ngx.req/ngx.resp/ngx.var/ngx.ctx 等请求级 API
//   - 队列满时回调被丢弃（不重试）
//
// 作者：xfy
package lua

import (
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	glua "github.com/yuin/gopher-lua"
)

// CallbackEntry 回调队列条目，封装定时器触发的 Lua 回调。
//
// 包含编译后的 FunctionProto 和调用参数，
// 由 TimerManager 推入 engine 的 callbackQueue 供调度器执行。
type CallbackEntry struct {
	// proto 编译后的 Lua 函数原型
	proto *glua.FunctionProto

	// args 调用参数
	args []glua.LValue
}

// TimerManager 定时器管理器。
//
// 负责创建、取消和执行 Lua 定时器。
// 回调在专用调度器 goroutine 的 LState 中执行，实现线程隔离。
//
// 并发安全说明：
//   - mu 保护 timers 映射的读写
//   - queueMu 保护队列关闭状态
//   - active/stopping 使用 atomic 操作
type TimerManager struct {
	// timers 活跃定时器映射（ID -> 条目）
	timers map[uint64]*TimerEntry

	// engine 关联的 Lua 引擎
	engine *LuaEngine

	// callbackQueue 回调队列，推入调度器执行
	callbackQueue chan *CallbackEntry

	// schedulerDone 调度器 goroutine 退出信号
	schedulerDone chan struct{}

	// schedulerL 调度器专用 LState
	schedulerL *glua.LState

	// nextID 下一个定时器 ID（原子操作）
	nextID uint64

	// mu 定时器映射读写锁
	mu sync.Mutex

	// queueMu 队列状态锁
	queueMu sync.Mutex

	// active 活跃定时器计数（原子操作）
	active int32

	// stopping 停止标记（原子操作）
	stopping int32

	// queueClosed 队列是否已关闭
	queueClosed bool
}

// TimerEntry 定时器条目。
//
// 封装单个定时器的回调、参数、生命周期和取消信号。
type TimerEntry struct {
	// callback 原始回调函数
	callback *glua.LFunction

	// callbackProto 编译后的回调原型（无 upvalue 限制）
	callbackProto *glua.FunctionProto

	// timer 底层 time.Timer
	timer *time.Timer

	// cancel 取消信号通道
	cancel chan struct{}

	// done 完成信号通道
	done chan struct{}

	// args 回调参数
	args []glua.LValue

	// ID 定时器唯一标识
	id uint64

	// delay 延迟时间
	delay time.Duration
}

// TimerHandle 定时器句柄，暴露给 Lua 的 userdata。
//
// 通过该句柄可在 Lua 中取消定时器。
type TimerHandle struct {
	// manager 关联的定时器管理器
	manager *TimerManager

	// id 定时器 ID
	id uint64
}

// NewTimerManager 创建定时器管理器实例。
//
// 初始化定时器映射、回调队列、调度器 LState，
// 并启动调度器 goroutine。
//
// 参数：
//   - engine: Lua 引擎实例
//
// 返回值：
//   - *TimerManager: 初始化的管理器实例
func NewTimerManager(engine *LuaEngine) *TimerManager {
	m := &TimerManager{
		timers:        make(map[uint64]*TimerEntry),
		engine:        engine,
		callbackQueue: make(chan *CallbackEntry, 1024),
		schedulerDone: make(chan struct{}),
	}

	// 创建专用调度器 LState
	m.schedulerL = glua.NewState(glua.Options{
		SkipOpenLibs: true,
	})
	glua.OpenBase(m.schedulerL)
	glua.OpenTable(m.schedulerL)
	glua.OpenString(m.schedulerL)
	glua.OpenMath(m.schedulerL)

	// 注册调度器可用的安全 API
	if engine != nil {
		RegisterSharedDictAPI(m.schedulerL, engine.SharedDictManager(), m.schedulerL.NewTable())
		RegisterNgxLogAPI(m.schedulerL, nil)
	}

	// 启动调度器 goroutine
	go m.schedulerLoop()

	return m
}

// At 创建延迟定时器。
//
// 在指定延迟后执行回调函数。回调在调度器 goroutine 的 LState 中执行。
//
// 参数：
//   - delay: 延迟时间
//   - callback: Lua 回调函数（不能捕获 upvalue）
//   - args: 回调参数
//
// 返回值：
//   - *TimerHandle: 定时器句柄
//   - error: 创建失败或服务器正在关闭时返回错误
//
// 安全说明：
//   - 回调不能捕获 upvalue（闭包变量），因为它们会在不同 goroutine 中执行
//   - 跨协程数据共享应使用 shared dict
func (m *TimerManager) At(delay time.Duration, callback *glua.LFunction, args []glua.LValue) (*TimerHandle, error) {
	if atomic.LoadInt32(&m.stopping) != 0 {
		return nil, nil // 服务器正在关闭，不接受新定时器
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	id := atomic.AddUint64(&m.nextID, 1)

	// 编译回调为 FunctionProto
	var proto *glua.FunctionProto
	if callback != nil && callback.Proto != nil {
		proto = callback.Proto
		// 拒绝带有 upvalue 的回调
		if proto.NumUpvalues > 0 {
			return nil, fmt.Errorf("timer callback cannot capture upvalues (closure variables); use shared dict instead")
		}
	}

	entry := &TimerEntry{
		id:            id,
		delay:         delay,
		callback:      callback,
		callbackProto: proto,
		args:          args,
		cancel:        make(chan struct{}),
		done:          make(chan struct{}),
	}

	// 设置定时器
	entry.timer = time.AfterFunc(delay, func() {
		m.executeTimer(entry)
	})

	m.timers[id] = entry
	atomic.AddInt32(&m.active, 1)

	return &TimerHandle{id: id, manager: m}, nil
}

// executeTimer 执行定时器回调。
//
// 定时器到期时由 time.AfterFunc 调用，将回调推入调度器队列。
// 检查取消信号，清理条目，并在队列满时丢弃回调。
func (m *TimerManager) executeTimer(entry *TimerEntry) {
	defer func() {
		atomic.AddInt32(&m.active, -1)
		close(entry.done)
	}()

	// 检查是否被取消
	select {
	case <-entry.cancel:
		return // 已取消
	default:
	}

	// 清理定时器条目
	m.mu.Lock()
	if m.timers != nil {
		delete(m.timers, entry.id)
	}
	m.mu.Unlock()

	// 将回调入队到调度器
	if entry.callbackProto != nil {
		cbEntry := &CallbackEntry{
			proto: entry.callbackProto,
			args:  entry.args,
		}
		m.queueMu.Lock()
		if m.queueClosed {
			m.queueMu.Unlock()
			return // 通道已关闭，放弃回调
		}
		select {
		case m.callbackQueue <- cbEntry:
			m.queueMu.Unlock()
		default:
			m.queueMu.Unlock()
			log.Printf("[lua] timer callback dropped: queue full")
		}
	}
}

// schedulerLoop 调度器循环，在专用 goroutine 中执行 Lua 回调。
//
// 从 callbackQueue 中读取回调条目，从 FunctionProto 重建 Lua 函数并执行。
// 队列关闭时退出循环。
func (m *TimerManager) schedulerLoop() {
	defer close(m.schedulerDone)

	for entry := range m.callbackQueue {
		// 从字节码重建函数并执行
		fn := m.schedulerL.NewFunctionFromProto(entry.proto)
		if fn == nil {
			log.Printf("[lua] timer callback: failed to create function from proto")
			continue
		}

		// 调用函数
		if err := m.schedulerL.CallByParam(glua.P{
			Fn:   fn,
			NRet: 0,
		}, entry.args...); err != nil {
			log.Printf("[lua] timer callback error: %v", err)
		}
	}
}

// Cancel 取消定时器。
//
// 停止底层 time.Timer，发送取消信号，清理定时器条目。
//
// 参数：
//   - handle: 定时器句柄
//
// 返回值：
//   - bool: true 表示成功取消，false 表示定时器不存在或已执行
func (m *TimerManager) Cancel(handle *TimerHandle) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	entry, ok := m.timers[handle.id]
	if !ok {
		return false // 定时器不存在或已执行
	}

	// 停止定时器
	if entry.timer != nil {
		entry.timer.Stop()
	}

	// 发送取消信号
	close(entry.cancel)

	// 清理
	delete(m.timers, entry.id)
	atomic.AddInt32(&m.active, -1)

	return true
}

// WaitAll 等待所有定时器完成。
//
// 设置停止标志（拒绝新定时器），轮询等待活跃计数器归零。
// 超时后强制取消所有剩余定时器。
//
// 参数：
//   - timeout: 最大等待时间
//
// 返回值：
//   - bool: true 表示所有定时器已正常完成，false 表示超时
func (m *TimerManager) WaitAll(timeout time.Duration) bool {
	// 设置停止标志
	atomic.StoreInt32(&m.stopping, 1)

	// 等待所有定时器完成
	start := time.Now()
	for atomic.LoadInt32(&m.active) > 0 {
		if time.Since(start) > timeout {
			// 超时，强制取消所有
			m.mu.Lock()
			for _, entry := range m.timers {
				if entry.timer != nil {
					entry.timer.Stop()
				}
				close(entry.cancel)
			}
			m.timers = make(map[uint64]*TimerEntry)
			m.mu.Unlock()
			return false
		}
		time.Sleep(10 * time.Millisecond)
	}

	return true
}

// Close 关闭定时器管理器。
//
// 执行顺序：
//  1. 设置停止标志，拒绝新定时器
//  2. 优雅关闭：等待回调队列排空（5 秒超时）
//  3. 关闭调度器 LState
//
// 注意：该方法是幂等的，可安全调用多次。
func (m *TimerManager) Close() {
	if m == nil || atomic.LoadInt32(&m.stopping) != 0 {
		return // 已关闭或 nil
	}

	// 1. 停止接受新定时器
	atomic.StoreInt32(&m.stopping, 1)

	// 2. 优雅关闭：等待回调队列排空
	m.gracefulShutdown(5 * time.Second)

	// 3. 关闭调度器 LState
	if m.schedulerL != nil {
		m.schedulerL.Close()
		m.schedulerL = nil
	}
}

// gracefulShutdown 优雅关闭定时器管理器。
//
// 关闭回调队列，等待调度器 goroutine 退出。
// 超时后记录被丢弃的回调数量。
//
// 参数：
//   - timeout: 最大等待时间
func (m *TimerManager) gracefulShutdown(timeout time.Duration) {
	m.queueMu.Lock()
	m.queueClosed = true
	close(m.callbackQueue)
	m.queueMu.Unlock()

	// 等待调度器 goroutine 退出
	select {
	case <-m.schedulerDone:
	case <-time.After(timeout):
		abandoned := len(m.callbackQueue)
		if abandoned > 0 {
			log.Printf("[lua] shutdown timeout: %d callbacks abandoned", abandoned)
		}
	}
}

// ActiveCount 返回当前活跃定时器数量。
//
// 返回值：
//   - int32: 活跃定时器数量
func (m *TimerManager) ActiveCount() int32 {
	return atomic.LoadInt32(&m.active)
}

// RegisterTimerAPI 注册 ngx.timer API 到 Lua 状态机。
//
// 在 ngx 表下创建 timer 子表，注册以下方法：
//   - at(delay, callback, ...)：创建延迟定时器，返回 userdata 句柄
//   - running_count()：返回活跃定时器数量
//
// 同时创建定时器句柄元表，支持 cancel 方法。
//
// 参数：
//   - L: Lua 状态
//   - manager: 定时器管理器实例
//   - ngx: ngx 全局表
func RegisterTimerAPI(L *glua.LState, manager *TimerManager, ngx *glua.LTable) {
	// 创建 ngx.timer 表
	timer := L.NewTable()

	// ngx.timer.at(delay, callback, ...)
	L.SetField(timer, "at", L.NewFunction(func(L *glua.LState) int {
		// 检查参数
		delay := float64(L.CheckNumber(1))
		callback := L.CheckFunction(2)

		// 收集额外参数
		args := []glua.LValue{}
		for i := 3; i <= L.GetTop(); i++ {
			args = append(args, L.Get(i))
		}

		// 创建定时器
		handle, err := manager.At(time.Duration(delay)*time.Second, callback, args)
		if err != nil {
			L.Push(glua.LNil)
			L.Push(glua.LString(err.Error()))
			return 2
		}
		if handle == nil {
			L.Push(glua.LNil)
			L.Push(glua.LString("server shutting down"))
			return 2
		}

		// 返回定时器句柄
		ud := L.NewUserData()
		ud.Value = handle
		L.SetMetatable(ud, L.GetTypeMetatable("ngx.timer.handle"))
		L.Push(ud)
		return 1
	}))

	// ngx.timer.running_count()
	L.SetField(timer, "running_count", L.NewFunction(func(L *glua.LState) int {
		L.Push(glua.LNumber(manager.ActiveCount()))
		return 1
	}))

	L.SetField(ngx, "timer", timer)

	// 创建定时器句柄元表
	mt := L.NewTypeMetatable("ngx.timer.handle")
	L.SetField(mt, "__index", L.NewFunction(timerHandleIndex))
	L.SetField(mt, "__tostring", L.NewFunction(timerHandleToString))

	// 注册方法
	methods := L.NewTable()
	L.SetField(methods, "cancel", L.NewFunction(timerHandleCancel))
	L.SetField(mt, "methods", methods)
}

// timerHandleIndex 定时器句柄索引
func timerHandleIndex(L *glua.LState) int {
	ud := L.CheckUserData(1)
	_, ok := ud.Value.(*TimerHandle)
	if !ok {
		L.RaiseError("invalid timer handle")
		return 0
	}

	// 检查是否是方法
	// 类型断言检查 - 使用已声明的 ud
	userdata, ok := L.Get(1).(*glua.LUserData)
	if !ok {
		L.Push(glua.LNil)
		return 1
	}
	methods := L.GetField(userdata.Metatable, "methods")
	if method := L.GetField(methods, L.CheckString(2)); method != glua.LNil {
		L.Push(method)
		return 1
	}

	L.Push(glua.LNil)
	return 1
}

// timerHandleToString 定时器句柄字符串表示
func timerHandleToString(L *glua.LState) int {
	ud := L.CheckUserData(1)
	handle, ok := ud.Value.(*TimerHandle)
	if !ok {
		L.Push(glua.LString("invalid timer handle"))
		return 1
	}
	L.Push(glua.LString("ngx.timer.handle:" + uint64ToStr(handle.id)))
	return 1
}

// timerHandleCancel 取消定时器
func timerHandleCancel(L *glua.LState) int {
	ud := L.CheckUserData(1)
	handle, ok := ud.Value.(*TimerHandle)
	if !ok {
		L.RaiseError("invalid timer handle")
		return 0
	}

	if handle.manager == nil {
		L.Push(glua.LFalse)
		L.Push(glua.LString("timer manager not available"))
		return 2
	}

	ok = handle.manager.Cancel(handle)
	if ok {
		L.Push(glua.LTrue)
		return 1
	}
	L.Push(glua.LFalse)
	L.Push(glua.LString("timer not found or already executed"))
	return 2
}

// uint64ToStr 整数转字符串
func uint64ToStr(n uint64) string {
	if n == 0 {
		return "0"
	}

	var buf []byte
	for n > 0 {
		buf = append(buf, byte('0'+n%10))
		n /= 10
	}

	// 反转
	for i, j := 0, len(buf)-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
	}

	return string(buf)
}
