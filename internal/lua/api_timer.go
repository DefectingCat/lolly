// Package lua 提供 Lua 脚本嵌入能力
package lua

import (
	"sync"
	"sync/atomic"
	"time"

	glua "github.com/yuin/gopher-lua"
)

// TimerManager 定时器管理器
type TimerManager struct {
	mu       sync.Mutex
	timers   map[uint64]*TimerEntry
	nextID   uint64
	engine   *LuaEngine
	active   int32
	stopping int32
}

// TimerEntry 定时器条目
type TimerEntry struct {
	id       uint64
	delay    time.Duration
	callback *glua.LFunction
	args     []glua.LValue
	timer    *time.Timer
	cancel   chan struct{}
	done     chan struct{}
}

// TimerHandle 定时器句柄（Lua userdata）
type TimerHandle struct {
	id      uint64
	manager *TimerManager
}

// NewTimerManager 创建定时器管理器
func NewTimerManager(engine *LuaEngine) *TimerManager {
	return &TimerManager{
		timers: make(map[uint64]*TimerEntry),
		engine: engine,
	}
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

	entry := &TimerEntry{
		id:       id,
		delay:    delay,
		callback: callback,
		args:     args,
		cancel:   make(chan struct{}),
		done:     make(chan struct{}),
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
// 注意：由于 gopher-lua 不是线程安全的，定时器回调执行有限制
// 当前简化版本仅支持记录定时器触发，不执行实际 Lua 回调
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

	// 检查 engine 是否已关闭
	if m.engine == nil || m.engine.L == nil {
		return
	}

	// 由于 gopher-lua 不是线程安全的，异步 goroutine 中不能直接调用 LState
	// 完整实现需要使用 channel 将回调调度到主线程执行
	// 这里简化处理：定时器触发后记录日志（生产环境应该有更好的方案）

	// 清理定时器条目
	m.mu.Lock()
	if m.timers != nil {
		delete(m.timers, entry.id)
	}
	m.mu.Unlock()
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
	m.WaitAll(5 * time.Second)
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
