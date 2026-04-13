// Package lua 提供 Lua 脚本嵌入能力
package lua

import (
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	glua "github.com/yuin/gopher-lua"
)

// CallbackEntry 回调队列条目
type CallbackEntry struct {
	proto *glua.FunctionProto
	args  []glua.LValue
}

// TimerManager 定时器管理器
type TimerManager struct {
	timers        map[uint64]*TimerEntry
	engine        *LuaEngine
	callbackQueue chan *CallbackEntry
	schedulerDone chan struct{}
	schedulerL    *glua.LState
	nextID        uint64
	mu            sync.Mutex
	queueMu       sync.Mutex
	active        int32
	stopping      int32
	queueClosed   bool
}

// TimerEntry 定时器条目
type TimerEntry struct {
	callback      *glua.LFunction
	callbackProto *glua.FunctionProto
	timer         *time.Timer
	cancel        chan struct{}
	done          chan struct{}
	args          []glua.LValue
	id            uint64
	delay         time.Duration
}

// TimerHandle 定时器句柄（Lua userdata）
type TimerHandle struct {
	manager *TimerManager
	id      uint64
}

// NewTimerManager 创建定时器管理器
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

// At 创建定时器
// 返回定时器句柄和错误
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

// executeTimer 执行定时器回调
// 通过 channel 将回调调度到调度器 goroutine 执行
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

// schedulerLoop 调度器循环，在专用 goroutine 中执行 Lua 回调
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

// Cancel 取消定时器
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

// WaitAll 等待所有定时器完成
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

// Close 关闭定时器管理器
func (m *TimerManager) Close() {
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

// gracefulShutdown 优雅关闭：排空回调队列，超时后放弃
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

// ActiveCount 返回活跃定时器数
func (m *TimerManager) ActiveCount() int32 {
	return atomic.LoadInt32(&m.active)
}

// RegisterTimerAPI 注册 ngx.timer API
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
	//nolint:errcheck // 类型断言检查
	methods := L.GetField(L.Get(1).(*glua.LUserData).Metatable, "methods")
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
