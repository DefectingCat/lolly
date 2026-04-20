// Package lua 提供 Cosocket TCP API 实现。
//
// 该文件实现 ngx.socket.tcp 相关的 Lua API，兼容 OpenResty cosocket 语义。
// 包括：
//   - TCPSocket：TCP 连接封装，支持 connect/send/receive/close
//   - CosocketManager：异步操作生命周期管理（已在 socket_manager.go 中）
//   - 异步 yield/resume 机制（通过 handleCosocket* 系列函数）
//   - ReceiveUntil 模式匹配读取
//
// 特性：
//   - 操作状态机：Idle -> Connecting -> Connected -> Sending/Receiving -> Closed
//   - 超时检测：连接、发送、接收各自独立超时
//   - 原子操作标记完成，避免竞态条件
//
// 注意事项：
//   - TCPSocket 非并发安全，每个 Lua 协程独占一个 socket
//   - ReceiveUntil 有 1MB 缓冲区限制，防止内存耗尽
//   - Cosocket 的 yield 当前为同步模拟，待实现真正的非阻塞 yield
//
// 作者：xfy
package lua

import (
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	glua "github.com/yuin/gopher-lua"
)

// TCPSocket 封装 TCP 连接，提供同步和异步操作。
//
// 支持 connect、send、receive、receiveuntil、close 等操作，
// 兼容 OpenResty cosocket API 语义。
//
// 每个 socket 关联一个 CosocketManager 用于异步操作跟踪和超时检测。
// 状态转换通过 sync.RWMutex 保护。
type TCPSocket struct {
	// createdAt 创建时间
	createdAt time.Time

	// conn 底层 TCP 连接
	conn net.Conn

	// currentOp 当前进行中的异步操作
	currentOp *SocketOperation

	// manager 关联的 Cosocket 管理器
	manager *CosocketManager

	// addr 目标地址
	addr *net.TCPAddr

	// readTimeout 读取超时
	readTimeout time.Duration

	// sendTimeout 发送超时
	sendTimeout time.Duration

	// connectTimeout 连接超时
	connectTimeout time.Duration

	// state 当前 socket 状态
	state SocketState

	// mu 状态读写锁
	mu sync.RWMutex

	// closed 关闭标记（原子操作）
	closed int32
}

// NewTCPSocket 创建新的 TCP socket 实例。
//
// 参数：
//   - manager: Cosocket 管理器，为 nil 时使用默认全局管理器
//
// 返回值：
//   - *TCPSocket: 初始化的 socket 实例
func NewTCPSocket(manager *CosocketManager) *TCPSocket {
	if manager == nil {
		manager = DefaultCosocketManager
	}

	s := &TCPSocket{
		manager:        manager,
		state:          SocketStateIdle,
		readTimeout:    60 * time.Second,
		sendTimeout:    60 * time.Second,
		connectTimeout: 30 * time.Second,
		createdAt:      time.Now(),
	}

	manager.TrackSocketCreated()
	return s
}

// Connect 连接到指定地址（同步版本）。
//
// 发起 TCP 连接，并在后台 goroutine 中执行实际的 dial 操作。
// 连接结果通过 manager 的 SocketOperation 通知。
//
// 参数：
//   - host: 目标主机地址
//   - port: 目标端口号
//
// 返回值：
//   - error: 状态不正确或地址解析失败时返回错误
func (s *TCPSocket) Connect(host string, port int) error {
	s.mu.Lock()
	if s.state != SocketStateIdle {
		s.mu.Unlock()
		return fmt.Errorf("socket not idle, current state: %s", s.state)
	}
	s.state = SocketStateConnecting
	s.mu.Unlock()

	// 解析地址
	addr, err := s.manager.TCPAddr(host, port)
	if err != nil {
		s.setState(SocketStateError)
		return fmt.Errorf("resolve address: %w", err)
	}
	s.addr = addr

	// 开始操作
	op := s.manager.StartOperation(s, OpConnect, s.connectTimeout)
	s.currentOp = op

	// 在 goroutine 中执行连接
	go func() {
		defer func() {
			s.currentOp = nil
		}()

		dialer := &net.Dialer{
			Timeout: s.connectTimeout,
		}

		conn, err := dialer.DialContext(context.Background(), "tcp", addr.String())
		if err != nil {
			s.setState(SocketStateError)
			s.manager.CompleteOperation(op.ID, nil, fmt.Errorf("dial: %w", err))
			return
		}

		s.mu.Lock()
		s.conn = conn
		s.state = SocketStateConnected
		s.mu.Unlock()

		s.manager.CompleteOperation(op.ID, conn, nil)
	}()

	return nil
}

// ConnectAsync 异步连接（用于 Lua yield/resume）。
//
// 调用 Connect 并返回关联的 SocketOperation，供 Lua 协程 yield 等待。
//
// 返回值：
//   - *SocketOperation: 连接操作实例
//   - error: 连接失败时返回错误
func (s *TCPSocket) ConnectAsync(_ *glua.LState, host string, port int) (*SocketOperation, error) {
	err := s.Connect(host, port)
	if err != nil {
		return nil, err
	}
	return s.currentOp, nil
}

// Send 发送数据
func (s *TCPSocket) Send(data []byte) (int, error) {
	s.mu.RLock()
	if s.state != SocketStateConnected {
		s.mu.RUnlock()
		return 0, fmt.Errorf("socket not connected, current state: %s", s.state)
	}
	conn := s.conn
	s.mu.RUnlock()

	if conn == nil {
		return 0, fmt.Errorf("socket connection is nil")
	}

	// 设置写超时
	if err := conn.SetWriteDeadline(time.Now().Add(s.sendTimeout)); err != nil {
		return 0, fmt.Errorf("set write deadline: %w", err)
	}

	n, err := conn.Write(data)
	if err != nil {
		s.setState(SocketStateError)
		return n, fmt.Errorf("write: %w", err)
	}

	return n, nil
}

// SendAsync 异步发送（用于 Lua yield）
func (s *TCPSocket) SendAsync(data []byte) (*SocketOperation, error) {
	s.mu.RLock()
	if s.state != SocketStateConnected {
		s.mu.RUnlock()
		return nil, fmt.Errorf("socket not connected, current state: %s", s.state)
	}
	conn := s.conn
	s.mu.RUnlock()

	if conn == nil {
		return nil, fmt.Errorf("socket connection is nil")
	}

	// 开始操作
	op := s.manager.StartOperation(s, OpSend, s.sendTimeout)
	s.currentOp = op
	s.setState(SocketStateSending)

	// 在 goroutine 中执行发送
	go func() {
		defer func() {
			s.currentOp = nil
			s.setState(SocketStateConnected)
		}()

		// 设置写超时
		if err := conn.SetWriteDeadline(time.Now().Add(s.sendTimeout)); err != nil {
			s.manager.CompleteOperation(op.ID, 0, fmt.Errorf("set write deadline: %w", err))
			return
		}

		n, err := conn.Write(data)
		if err != nil {
			s.setState(SocketStateError)
			s.manager.CompleteOperation(op.ID, n, fmt.Errorf("write: %w", err))
			return
		}

		s.manager.CompleteOperation(op.ID, n, nil)
	}()

	return op, nil
}

// socketEOFError EOF错误字符串常量
const socketEOFError = "EOF"

// Receive 接收数据
func (s *TCPSocket) Receive(size int) ([]byte, error) {
	s.mu.RLock()
	if s.state != SocketStateConnected {
		s.mu.RUnlock()
		return nil, fmt.Errorf("socket not connected, current state: %s", s.state)
	}
	conn := s.conn
	s.mu.RUnlock()

	if conn == nil {
		return nil, fmt.Errorf("socket connection is nil")
	}

	// 设置读超时
	if err := conn.SetReadDeadline(time.Now().Add(s.readTimeout)); err != nil {
		return nil, fmt.Errorf("set read deadline: %w", err)
	}

	// 默认读取大小
	if size <= 0 {
		size = 4096
	}

	buf := make([]byte, size)
	n, err := conn.Read(buf)
	if err != nil {
		if err.Error() == socketEOFError {
			return nil, nil // 连接关闭
		}
		s.setState(SocketStateError)
		return nil, fmt.Errorf("read: %w", err)
	}

	return buf[:n], nil
}

// ReceiveAsync 异步接收（用于 Lua yield）
func (s *TCPSocket) ReceiveAsync(size int) (*SocketOperation, error) {
	s.mu.RLock()
	if s.state != SocketStateConnected {
		s.mu.RUnlock()
		return nil, fmt.Errorf("socket not connected, current state: %s", s.state)
	}
	conn := s.conn
	s.mu.RUnlock()

	if conn == nil {
		return nil, fmt.Errorf("socket connection is nil")
	}

	// 开始操作
	op := s.manager.StartOperation(s, OpReceive, s.readTimeout)
	s.currentOp = op
	s.setState(SocketStateReceiving)

	// 在 goroutine 中执行接收
	go func() {
		defer func() {
			s.currentOp = nil
			s.setState(SocketStateConnected)
		}()

		// 设置读超时
		if err := conn.SetReadDeadline(time.Now().Add(s.readTimeout)); err != nil {
			s.manager.CompleteOperation(op.ID, nil, fmt.Errorf("set read deadline: %w", err))
			return
		}

		// 默认读取大小
		if size <= 0 {
			size = 4096
		}

		buf := make([]byte, size)
		n, err := conn.Read(buf)
		if err != nil {
			if err.Error() == socketEOFError {
				s.manager.CompleteOperation(op.ID, []byte{}, nil)
				return
			}
			s.setState(SocketStateError)
			s.manager.CompleteOperation(op.ID, nil, fmt.Errorf("read: %w", err))
			return
		}

		s.manager.CompleteOperation(op.ID, buf[:n], nil)
	}()

	return op, nil
}

// ReceiveUntil 读取直到特定模式
func (s *TCPSocket) ReceiveUntil(pattern string, inclusive bool) ([]byte, error) {
	if len(pattern) == 0 {
		return nil, fmt.Errorf("pattern cannot be empty")
	}

	s.mu.RLock()
	if s.state != SocketStateConnected {
		s.mu.RUnlock()
		return nil, fmt.Errorf("socket not connected, current state: %s", s.state)
	}
	conn := s.conn
	s.mu.RUnlock()

	if conn == nil {
		return nil, fmt.Errorf("socket connection is nil")
	}

	// 设置读超时
	if err := conn.SetReadDeadline(time.Now().Add(s.readTimeout)); err != nil {
		return nil, fmt.Errorf("set read deadline: %w", err)
	}

	// 使用带缓冲的读取
	var result []byte
	buf := make([]byte, 1)
	patternBytes := []byte(pattern)
	patternLen := len(patternBytes)

	for {
		n, err := conn.Read(buf)
		if err != nil {
			if err.Error() == socketEOFError {
				return result, nil
			}
			s.setState(SocketStateError)
			return result, fmt.Errorf("read: %w", err)
		}
		if n == 0 {
			continue
		}

		result = append(result, buf[0])

		// 检查是否匹配模式
		if len(result) >= patternLen {
			matched := true
			for i := 0; i < patternLen; i++ {
				if result[len(result)-patternLen+i] != patternBytes[i] {
					matched = false
					break
				}
			}
			if matched {
				if !inclusive {
					result = result[:len(result)-patternLen]
				}
				return result, nil
			}
		}

		// 防止无限增长
		if len(result) > 1024*1024 { // 1MB 限制
			return result, fmt.Errorf("receive buffer exceeded 1MB limit")
		}
	}
}

// Close 关闭 socket
func (s *TCPSocket) Close() error {
	if s == nil {
		return nil
	}
	if !atomic.CompareAndSwapInt32(&s.closed, 0, 1) {
		return nil // 已经关闭
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// 取消当前操作
	if s.currentOp != nil && !s.currentOp.IsCompleted() && s.manager != nil {
		s.manager.CompleteOperation(s.currentOp.ID, nil, fmt.Errorf("socket closed"))
		s.currentOp = nil
	}

	// 关闭连接
	if s.conn != nil {
		s.conn.Close()
		s.conn = nil
	}

	s.state = SocketStateClosed
	if s.manager != nil {
		s.manager.TrackSocketClosed()
	}

	return nil
}

// SetTimeout 设置超时
func (s *TCPSocket) SetTimeout(timeout time.Duration) {
	s.readTimeout = timeout
	s.sendTimeout = timeout
	s.connectTimeout = timeout
}

// SetReadTimeout 设置读取超时
func (s *TCPSocket) SetReadTimeout(timeout time.Duration) {
	s.readTimeout = timeout
}

// SetSendTimeout 设置发送超时
func (s *TCPSocket) SetSendTimeout(timeout time.Duration) {
	s.sendTimeout = timeout
}

// SetConnectTimeout 设置连接超时
func (s *TCPSocket) SetConnectTimeout(timeout time.Duration) {
	s.connectTimeout = timeout
}

// State 获取当前状态
func (s *TCPSocket) State() SocketState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state
}

// setState 设置状态
func (s *TCPSocket) setState(state SocketState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state = state
}

// IsClosed 检查是否已关闭
func (s *TCPSocket) IsClosed() bool {
	return atomic.LoadInt32(&s.closed) == 1
}

// LocalAddr 获取本地地址
func (s *TCPSocket) LocalAddr() net.Addr {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.conn != nil {
		return s.conn.LocalAddr()
	}
	return nil
}

// RemoteAddr 获取远程地址
func (s *TCPSocket) RemoteAddr() net.Addr {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.conn != nil {
		return s.conn.RemoteAddr()
	}
	return nil
}

// -------------------- Lua API --------------------

// tcpSocketMT TCP socket 元表名称
const tcpSocketMT = "tcp_socket"

// RegisterTCPSocketAPI 注册 TCP socket API
func RegisterTCPSocketAPI(L *glua.LState, engine *LuaEngine) {
	// 创建 ngx.socket 表
	socket := L.NewTable()

	// ngx.socket.tcp()
	socket.RawSetString("tcp", L.NewFunction(newTCPSocketFunc(engine)))

	// 确保 ngx 表存在
	ngx := L.GetGlobal("ngx")
	var ngxTbl *glua.LTable
	if tbl, ok := ngx.(*glua.LTable); ok {
		ngxTbl = tbl
	} else {
		// 创建 ngx 表
		ngxTbl = L.NewTable()
		L.SetGlobal("ngx", ngxTbl)
	}
	ngxTbl.RawSetString("socket", socket)

	// 注册元表
	registerTCPSocketMetaTable(L)
}

// registerTCPSocketMetaTable 注册 TCP socket 元表
func registerTCPSocketMetaTable(L *glua.LState) {
	mt := L.NewTable()

	// __index
	index := L.NewTable()
	index.RawSetString("connect", L.NewFunction(tcpSocketConnect))
	index.RawSetString("send", L.NewFunction(tcpSocketSend))
	index.RawSetString("receive", L.NewFunction(tcpSocketReceive))
	index.RawSetString("receiveuntil", L.NewFunction(tcpSocketReceiveUntil))
	index.RawSetString("close", L.NewFunction(tcpSocketClose))
	index.RawSetString("settimeout", L.NewFunction(tcpSocketSetTimeout))
	index.RawSetString("settimeouts", L.NewFunction(tcpSocketSetTimeouts))

	mt.RawSetString("__index", index)
	mt.RawSetString("__gc", L.NewFunction(tcpSocketGC))
	mt.RawSetString("__tostring", L.NewFunction(tcpSocketToString))

	L.SetMetatable(L.NewTable(), mt)
	L.SetGlobal(tcpSocketMT, mt)
}

// newTCPSocketFunc 创建 TCP socket
func newTCPSocketFunc(_ *LuaEngine) func(*glua.LState) int {
	return func(L *glua.LState) int {
		socket := NewTCPSocket(DefaultCosocketManager)

		// 创建 userdata
		ud := L.NewUserData()
		ud.Value = socket
		// 类型断言检查
		mt, ok := L.GetGlobal(tcpSocketMT).(*glua.LTable)
		if ok {
			L.SetMetatable(ud, mt)
		}

		L.Push(ud)
		return 1
	}
}

// checkTCPSocket 检查并获取 TCP socket
func checkTCPSocket(L *glua.LState, n int) *TCPSocket {
	ud := L.CheckUserData(n)
	if socket, ok := ud.Value.(*TCPSocket); ok {
		return socket
	}
	L.ArgError(n, "tcp socket expected")
	return nil
}

// tcpSocketConnect tcpsock:connect()
func tcpSocketConnect(L *glua.LState) int {
	socket := checkTCPSocket(L, 1)
	host := L.CheckString(2)
	port := L.CheckInt(3)

	opts := L.OptTable(4, nil)
	timeout := 30000 // 默认 30 秒
	if opts != nil {
		if t := opts.RawGetString("timeout"); t != glua.LNil {
			timeout = int(glua.LVAsNumber(t))
		}
	}

	// 设置超时
	socket.SetConnectTimeout(time.Duration(timeout) * time.Millisecond)

	// 开始异步连接
	op, err := socket.ConnectAsync(L, host, port)
	if err != nil {
		L.Push(glua.LNil)
		L.Push(glua.LString(err.Error()))
		return 2
	}

	// yield 等待连接完成
	L.Push(glua.LString("cosocket_connect"))
	L.Push(glua.LNumber(op.ID))
	// TODO: 实现真正的非阻塞 yield，目前使用同步模拟
	return 2
}

// tcpSocketSend tcpsock:send()
func tcpSocketSend(L *glua.LState) int {
	socket := checkTCPSocket(L, 1)
	data := L.CheckString(2)

	opts := L.OptTable(3, nil)
	timeout := 60000 // 默认 60 秒
	if opts != nil {
		if t := opts.RawGetString("timeout"); t != glua.LNil {
			timeout = int(glua.LVAsNumber(t))
		}
	}

	// 设置超时
	socket.SetSendTimeout(time.Duration(timeout) * time.Millisecond)

	// 开始异步发送
	op, err := socket.SendAsync([]byte(data))
	if err != nil {
		L.Push(glua.LNil)
		L.Push(glua.LString(err.Error()))
		return 2
	}

	// yield 等待发送完成
	L.Push(glua.LString("cosocket_send"))
	L.Push(glua.LNumber(op.ID))
	// TODO: 实现真正的非阻塞 yield，目前使用同步模拟
	return 2
}

// tcpSocketReceive tcpsock:receive()
func tcpSocketReceive(L *glua.LState) int {
	socket := checkTCPSocket(L, 1)

	// 解析参数
	var size int
	opts := L.NewTable()

	if L.GetTop() >= 2 {
		switch v := L.Get(2).(type) {
		case glua.LNumber:
			size = int(v)
		case *glua.LTable:
			opts = v
		default:
			// 检查是否为字符串类型
			if L.Get(2).Type() == glua.LTString {
				// 接收特定模式
				return tcpSocketReceivePattern(L, socket, L.CheckString(2), opts)
			}
		}
	}

	if L.GetTop() >= 3 {
		if t, ok := L.Get(3).(*glua.LTable); ok {
			opts = t
		}
	}

	// 获取超时
	timeout := 60000 // 默认 60 秒
	if t := opts.RawGetString("timeout"); t != glua.LNil {
		timeout = int(glua.LVAsNumber(t))
	}

	// 设置超时
	socket.SetReadTimeout(time.Duration(timeout) * time.Millisecond)

	// 开始异步接收
	op, err := socket.ReceiveAsync(size)
	if err != nil {
		L.Push(glua.LNil)
		L.Push(glua.LString(err.Error()))
		return 2
	}

	// yield 等待接收完成
	L.Push(glua.LString("cosocket_receive"))
	L.Push(glua.LNumber(op.ID))
	// TODO: 实现真正的非阻塞 yield，目前使用同步模拟
	return 2
}

// tcpSocketReceivePattern 按模式接收
func tcpSocketReceivePattern(L *glua.LState, socket *TCPSocket, pattern string, _ *glua.LTable) int {
	switch pattern {
	case "*l":
		// 读取一行
		data, err := socket.ReceiveUntil("\n", true)
		if err != nil {
			L.Push(glua.LNil)
			L.Push(glua.LString(err.Error()))
			return 2
		}
		L.Push(glua.LString(string(data)))
		return 1
	case "*a":
		// 读取所有（这里简化为读取最大 64KB）
		data, err := socket.Receive(64 * 1024)
		if err != nil {
			L.Push(glua.LNil)
			L.Push(glua.LString(err.Error()))
			return 2
		}
		L.Push(glua.LString(string(data)))
		return 1
	default:
		L.Push(glua.LNil)
		L.Push(glua.LString("unknown pattern: " + pattern))
		return 2
	}
}

// tcpSocketReceiveUntil tcpsock:receiveuntil()
func tcpSocketReceiveUntil(L *glua.LState) int {
	socket := checkTCPSocket(L, 1)
	pattern := L.CheckString(2)

	opts := L.OptTable(3, nil)
	inclusive := false
	if opts != nil {
		if inc := opts.RawGetString("inclusive"); inc != glua.LNil {
			inclusive = glua.LVAsBool(inc)
		}
	}

	data, err := socket.ReceiveUntil(pattern, inclusive)
	if err != nil {
		L.Push(glua.LNil)
		L.Push(glua.LString(err.Error()))
		return 2
	}

	// 创建迭代器函数
	iter := L.NewFunction(func(L *glua.LState) int {
		if len(data) == 0 {
			L.Push(glua.LNil)
			return 1
		}
		L.Push(glua.LString(string(data)))
		data = nil // 清空，下次返回 nil
		return 1
	})

	L.Push(iter)
	return 1
}

// tcpSocketClose tcpsock:close()
func tcpSocketClose(L *glua.LState) int {
	socket := checkTCPSocket(L, 1)
	if err := socket.Close(); err != nil {
		L.Push(glua.LFalse)
		L.Push(glua.LString(err.Error()))
		return 2
	}
	L.Push(glua.LTrue)
	return 1
}

// tcpSocketSetTimeout tcpsock:settimeout()
func tcpSocketSetTimeout(L *glua.LState) int {
	socket := checkTCPSocket(L, 1)
	timeout := L.CheckNumber(2)
	socket.SetTimeout(time.Duration(timeout) * time.Millisecond)
	L.Push(glua.LTrue)
	return 1
}

// tcpSocketSetTimeouts tcpsock:settimeouts()
func tcpSocketSetTimeouts(L *glua.LState) int {
	socket := checkTCPSocket(L, 1)
	connectTimeout := L.CheckNumber(2)
	sendTimeout := L.CheckNumber(3)
	readTimeout := L.CheckNumber(4)

	socket.SetConnectTimeout(time.Duration(connectTimeout) * time.Millisecond)
	socket.SetSendTimeout(time.Duration(sendTimeout) * time.Millisecond)
	socket.SetReadTimeout(time.Duration(readTimeout) * time.Millisecond)

	L.Push(glua.LTrue)
	return 1
}

// tcpSocketGC __gc 元方法
func tcpSocketGC(L *glua.LState) int {
	socket := checkTCPSocket(L, 1)
	socket.Close()
	return 0
}

// tcpSocketToString __tostring 元方法
func tcpSocketToString(L *glua.LState) int {
	socket := checkTCPSocket(L, 1)
	state := socket.State()
	L.Push(glua.LString(fmt.Sprintf("tcp_socket(%s)", state)))
	return 1
}

// HandleCosocketYield 处理 cosocket yield
func (c *LuaCoroutine) HandleCosocketYield(reason string, values []glua.LValue) ([]glua.LValue, error) {
	switch reason {
	case "cosocket_connect":
		return c.handleCosocketConnect(values)
	case "cosocket_send":
		return c.handleCosocketSend(values)
	case "cosocket_receive":
		return c.handleCosocketReceive(values)
	default:
		return nil, fmt.Errorf("unknown cosocket yield reason: %s", reason)
	}
}

// handleCosocketConnect 处理连接 yield
func (c *LuaCoroutine) handleCosocketConnect(values []glua.LValue) ([]glua.LValue, error) {
	if len(values) == 0 {
		return nil, fmt.Errorf("cosocket_connect requires operation ID")
	}

	opID := uint64(glua.LVAsNumber(values[0]))
	op := DefaultCosocketManager.GetOperation(opID)
	if op == nil {
		return nil, fmt.Errorf("operation %d not found", opID)
	}

	// 等待操作完成
	result, err := op.Wait(c.ExecutionContext)
	if err != nil {
		return []glua.LValue{glua.LNil, glua.LString(err.Error())}, nil
	}

	if result == nil {
		return []glua.LValue{glua.LNil, glua.LNil}, nil
	}

	_ = result // 连接成功，返回 1
	return []glua.LValue{glua.LNumber(1)}, nil
}

// handleCosocketSend 处理发送 yield
func (c *LuaCoroutine) handleCosocketSend(values []glua.LValue) ([]glua.LValue, error) {
	if len(values) == 0 {
		return nil, fmt.Errorf("cosocket_send requires operation ID")
	}

	opID := uint64(glua.LVAsNumber(values[0]))
	op := DefaultCosocketManager.GetOperation(opID)
	if op == nil {
		return nil, fmt.Errorf("operation %d not found", opID)
	}

	// 等待操作完成
	result, err := op.Wait(c.ExecutionContext)
	if err != nil {
		return []glua.LValue{glua.LNil, glua.LString(err.Error())}, nil
	}

	if n, ok := result.(int); ok {
		return []glua.LValue{glua.LNumber(n)}, nil
	}

	return []glua.LValue{glua.LNil, glua.LString("invalid result")}, nil
}

// handleCosocketReceive 处理接收 yield
func (c *LuaCoroutine) handleCosocketReceive(values []glua.LValue) ([]glua.LValue, error) {
	if len(values) == 0 {
		return nil, fmt.Errorf("cosocket_receive requires operation ID")
	}

	opID := uint64(glua.LVAsNumber(values[0]))
	op := DefaultCosocketManager.GetOperation(opID)
	if op == nil {
		return nil, fmt.Errorf("operation %d not found", opID)
	}

	// 等待操作完成
	result, err := op.Wait(c.ExecutionContext)
	if err != nil {
		return []glua.LValue{glua.LNil, glua.LString(err.Error())}, nil
	}

	if data, ok := result.([]byte); ok {
		if len(data) == 0 {
			return []glua.LValue{glua.LNil, glua.LString("closed")}, nil
		}
		return []glua.LValue{glua.LString(string(data))}, nil
	}

	return []glua.LValue{glua.LNil, glua.LString("invalid result")}, nil
}
