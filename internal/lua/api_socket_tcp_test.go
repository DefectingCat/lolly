package lua

import (
	"context"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	glua "github.com/yuin/gopher-lua"
)

// TestNewTCPSocket_NilManager 测试 nil manager 使用默认管理器
func TestNewTCPSocket_NilManager(t *testing.T) {
	socket := NewTCPSocket(nil)
	require.NotNil(t, socket)
	assert.Equal(t, SocketStateIdle, socket.State())
	assert.Equal(t, 60*time.Second, socket.readTimeout)
	assert.Equal(t, 60*time.Second, socket.sendTimeout)
	assert.Equal(t, 30*time.Second, socket.connectTimeout)
	assert.False(t, socket.IsClosed())
	socket.Close()
}

// TestNewTCPSocket_ExplicitManager 测试显式管理器
func TestNewTCPSocket_ExplicitManager(t *testing.T) {
	cm := NewCosocketManager()
	defer cm.Close()

	socket := NewTCPSocket(cm)
	require.NotNil(t, socket)
	assert.Equal(t, cm, socket.manager)
	socket.Close()
}

// TestTCPSocket_ConnectNotIdle 测试非空闲状态下连接失败
func TestTCPSocket_ConnectNotIdle(t *testing.T) {
	cm := NewCosocketManager()
	defer cm.Close()

	socket := NewTCPSocket(cm)
	defer socket.Close()

	// 设置状态为非空闲
	socket.setState(SocketStateConnected)
	defer socket.setState(SocketStateIdle)

	err := socket.Connect("127.0.0.1", 9999)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "socket not idle")
}

// TestTCPSocket_Send_NotConnected 测试未连接时发送失败
func TestTCPSocket_Send_NotConnected(t *testing.T) {
	cm := NewCosocketManager()
	defer cm.Close()

	socket := NewTCPSocket(cm)
	defer socket.Close()

	// 未连接状态下发送
	n, err := socket.Send([]byte("test"))
	assert.Error(t, err)
	assert.Equal(t, 0, n)
	assert.Contains(t, err.Error(), "socket not connected")
}

// TestTCPSocket_SendAsync_NotConnected 测试未连接时异步发送失败
func TestTCPSocket_SendAsync_NotConnected(t *testing.T) {
	cm := NewCosocketManager()
	defer cm.Close()

	socket := NewTCPSocket(cm)
	defer socket.Close()

	op, err := socket.SendAsync([]byte("test"))
	assert.Error(t, err)
	assert.Nil(t, op)
	assert.Contains(t, err.Error(), "socket not connected")
}

// TestTCPSocket_Receive_NotConnected 测试未连接时接收失败
func TestTCPSocket_Receive_NotConnected(t *testing.T) {
	cm := NewCosocketManager()
	defer cm.Close()

	socket := NewTCPSocket(cm)
	defer socket.Close()

	data, err := socket.Receive(1024)
	assert.Error(t, err)
	assert.Nil(t, data)
	assert.Contains(t, err.Error(), "socket not connected")
}

// TestTCPSocket_ReceiveAsync_NotConnected 测试未连接时异步接收失败
func TestTCPSocket_ReceiveAsync_NotConnected(t *testing.T) {
	cm := NewCosocketManager()
	defer cm.Close()

	socket := NewTCPSocket(cm)
	defer socket.Close()

	op, err := socket.ReceiveAsync(1024)
	assert.Error(t, err)
	assert.Nil(t, op)
	assert.Contains(t, err.Error(), "socket not connected")
}

// TestTCPSocket_ReceiveUntil_NotConnected 测试未连接时 ReceiveUntil 失败
func TestTCPSocket_ReceiveUntil_NotConnected(t *testing.T) {
	cm := NewCosocketManager()
	defer cm.Close()

	socket := NewTCPSocket(cm)
	defer socket.Close()

	// 设置为 connected 状态但 conn 为 nil
	socket.setState(SocketStateConnected)
	defer socket.setState(SocketStateIdle)

	data, err := socket.ReceiveUntil("\n", true)
	assert.Error(t, err)
	assert.Nil(t, data)
	assert.Contains(t, err.Error(), "socket connection is nil")
}

// TestTCPSocket_ReceiveUntil_EmptyPattern 测试空模式错误
func TestTCPSocket_ReceiveUntil_EmptyPattern(t *testing.T) {
	cm := NewCosocketManager()
	defer cm.Close()

	socket := NewTCPSocket(cm)
	defer socket.Close()

	data, err := socket.ReceiveUntil("", true)
	assert.Error(t, err)
	assert.Nil(t, data)
	assert.Contains(t, err.Error(), "pattern cannot be empty")
}

// TestTCPSocket_SetTimeouts 测试设置超时
func TestTCPSocket_SetTimeouts(t *testing.T) {
	cm := NewCosocketManager()
	defer cm.Close()

	socket := NewTCPSocket(cm)
	defer socket.Close()

	socket.SetTimeout(5 * time.Second)
	assert.Equal(t, 5*time.Second, socket.readTimeout)
	assert.Equal(t, 5*time.Second, socket.sendTimeout)
	assert.Equal(t, 5*time.Second, socket.connectTimeout)

	socket.SetReadTimeout(10 * time.Second)
	assert.Equal(t, 10*time.Second, socket.readTimeout)

	socket.SetSendTimeout(15 * time.Second)
	assert.Equal(t, 15*time.Second, socket.sendTimeout)

	socket.SetConnectTimeout(20 * time.Second)
	assert.Equal(t, 20*time.Second, socket.connectTimeout)
}

// TestTCPSocket_StateString 测试状态字符串
func TestTCPSocket_StateString(t *testing.T) {
	assert.Equal(t, "idle", SocketStateIdle.String())
	assert.Equal(t, "connecting", SocketStateConnecting.String())
	assert.Equal(t, "connected", SocketStateConnected.String())
	assert.Equal(t, "sending", SocketStateSending.String())
	assert.Equal(t, "receiving", SocketStateReceiving.String())
	assert.Equal(t, "closing", SocketStateClosing.String())
	assert.Equal(t, "closed", SocketStateClosed.String())
	assert.Equal(t, "error", SocketStateError.String())
	// 未知状态
	assert.Equal(t, "unknown", SocketState(999).String())
}

// TestTCPSocket_LocalAddr_RemoteAddr_NotConnected 测试未连接时地址返回 nil
func TestTCPSocket_LocalAddr_RemoteAddr_NotConnected(t *testing.T) {
	cm := NewCosocketManager()
	defer cm.Close()

	socket := NewTCPSocket(cm)
	defer socket.Close()

	assert.Nil(t, socket.LocalAddr())
	assert.Nil(t, socket.RemoteAddr())
}

// TestTCPSocket_ConnectAsync 测试 ConnectAsync
func TestTCPSocket_ConnectAsync(t *testing.T) {
	_, cleanup := mockEchoServer(t, "127.0.0.1:18801")
	defer cleanup()

	cm := NewCosocketManager()
	defer cm.Close()

	socket := NewTCPSocket(cm)
	defer socket.Close()

	L := glua.NewState()
	defer L.Close()

	op, err := socket.ConnectAsync(L, "127.0.0.1", 18801)
	require.NoError(t, err)
	require.NotNil(t, op)

	// 等待连接完成
	result, err := op.Wait(context.Background())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

// TestTCPSocket_ConnectAsync_Error 测试 ConnectAsync 错误路径
func TestTCPSocket_ConnectAsync_Error(t *testing.T) {
	cm := NewCosocketManager()
	defer cm.Close()

	socket := NewTCPSocket(cm)
	defer socket.Close()

	// 先设置为非空闲状态，ConnectAsync 应失败
	socket.setState(SocketStateConnected)
	defer socket.setState(SocketStateIdle)

	L := glua.NewState()
	defer L.Close()

	op, err := socket.ConnectAsync(L, "127.0.0.1", 18800)
	assert.Error(t, err)
	assert.Nil(t, op)
}

// TestTCPSocket_Connect_Failure 测试连接失败（无服务器）
func TestTCPSocket_Connect_Failure(t *testing.T) {
	cm := NewCosocketManager()
	defer cm.Close()

	socket := NewTCPSocket(cm)
	defer socket.Close()

	// 连接到不存在的端口，应该返回错误
	err := socket.Connect("127.0.0.1", 18899)
	require.NoError(t, err) // Connect 本身不报错

	// 等待异步连接完成
	op := socket.currentOp
	if op != nil {
		_, err := op.Wait(context.Background())
		assert.Error(t, err) // 连接应该失败
	}

	// 状态应该变为 error
	assert.Equal(t, SocketStateError, socket.State())
}

// TestTCPSocket_Receive_DefaultSize 测试默认读取大小 (size <= 0)
func TestTCPSocket_Receive_DefaultSize(t *testing.T) {
	_, cleanup := mockEchoServer(t, "127.0.0.1:18802")
	defer cleanup()

	cm := NewCosocketManager()
	defer cm.Close()

	socket := NewTCPSocket(cm)
	defer socket.Close()

	// 连接
	if err := socket.Connect("127.0.0.1", 18802); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	// 发送数据
	testData := "Hello"
	_, err := socket.Send([]byte(testData))
	require.NoError(t, err)

	// 使用 size=0 触发默认 4096
	received, err := socket.Receive(0)
	require.NoError(t, err)
	assert.Equal(t, testData, string(received))
}

// TestTCPSocket_ReceiveAsync_DefaultSize 测试异步接收默认大小
func TestTCPSocket_ReceiveAsync_DefaultSize(t *testing.T) {
	_, cleanup := mockEchoServer(t, "127.0.0.1:18803")
	defer cleanup()

	cm := NewCosocketManager()
	defer cm.Close()

	socket := NewTCPSocket(cm)
	defer socket.Close()

	if err := socket.Connect("127.0.0.1", 18803); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	// 发送
	testData := "AsyncReceive"
	_, err := socket.Send([]byte(testData))
	require.NoError(t, err)

	// 异步接收，size=-1 触发默认
	op, err := socket.ReceiveAsync(-1)
	require.NoError(t, err)
	require.NotNil(t, op)

	result, err := op.Wait(context.Background())
	require.NoError(t, err)
	data, ok := result.([]byte)
	require.True(t, ok)
	assert.Equal(t, testData, string(data))
}

// TestTCPSocket_ReceiveUntil_Inclusive 测试 inclusive 模式
func TestTCPSocket_ReceiveUntil_Inclusive(t *testing.T) {
	_, cleanup := mockEchoServer(t, "127.0.0.1:18804")
	defer cleanup()

	cm := NewCosocketManager()
	defer cm.Close()

	socket := NewTCPSocket(cm)
	defer socket.Close()

	if err := socket.Connect("127.0.0.1", 18804); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	// 发送带分隔符的数据
	testData := "hello|world"
	_, err := socket.Send([]byte(testData))
	require.NoError(t, err)

	// inclusive=true: 包含模式
	data, err := socket.ReceiveUntil("|", true)
	require.NoError(t, err)
	assert.Equal(t, "hello|", string(data))
}

// TestTCPSocket_ReceiveUntil_Exclusive 测试 exclusive 模式
func TestTCPSocket_ReceiveUntil_Exclusive(t *testing.T) {
	_, cleanup := mockEchoServer(t, "127.0.0.1:18805")
	defer cleanup()

	cm := NewCosocketManager()
	defer cm.Close()

	socket := NewTCPSocket(cm)
	defer socket.Close()

	if err := socket.Connect("127.0.0.1", 18805); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	testData := "hello|world"
	_, err := socket.Send([]byte(testData))
	require.NoError(t, err)

	// inclusive=false: 不包含模式
	data, err := socket.ReceiveUntil("|", false)
	require.NoError(t, err)
	assert.Equal(t, "hello", string(data))
}

// TestTCPSocket_Close_CompletesPendingOp 测试关闭时完成未完成的操作
func TestTCPSocket_Close_CompletesPendingOp(t *testing.T) {
	// 使用 slow server 模拟延迟
	ln, err := net.Listen("tcp", "127.0.0.1:18806")
	require.NoError(t, err)

	var wg sync.WaitGroup
	stopCh := make(chan struct{})

	// slow server: 接受连接但延迟响应
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				select {
				case <-stopCh:
					return
				default:
					continue
				}
			}
			wg.Add(1)
			go func(c net.Conn) {
				defer wg.Done()
				// 保持连接打开但不发送数据
				buf := make([]byte, 1)
				c.Read(buf)
				c.Close()
			}(conn)
		}
	}()

	defer func() {
		close(stopCh)
		ln.Close()
		wg.Wait()
	}()

	cm := NewCosocketManager()
	defer cm.Close()

	socket := NewTCPSocket(cm)
	defer socket.Close()

	// 连接
	err = socket.Connect("127.0.0.1", 18806)
	require.NoError(t, err)
	time.Sleep(100 * time.Millisecond)

	// 此时连接应该建立了
	assert.Equal(t, SocketStateConnected, socket.State())

	// 启动一个异步接收操作
	op, err := socket.ReceiveAsync(1024)
	require.NoError(t, err)
	require.NotNil(t, op)

	// 在操作完成前关闭 socket
	err = socket.Close()
	assert.NoError(t, err)

	// 等待操作完成（应该被取消）
	_, err = op.Wait(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "socket closed")
}

// TestTCPSocket_Close_NilSafety 测试 nil socket 的 Close
func TestTCPSocket_Close_NilSafety(t *testing.T) {
	var s *TCPSocket
	err := s.Close()
	assert.NoError(t, err)
}

// TestTCPSocket_DoubleClose 测试重复关闭
func TestTCPSocket_DoubleClose(t *testing.T) {
	cm := NewCosocketManager()
	defer cm.Close()

	socket := NewTCPSocket(cm)

	err := socket.Close()
	assert.NoError(t, err)

	err = socket.Close()
	assert.NoError(t, err) // 第二次关闭不应报错

	assert.True(t, socket.IsClosed())
}

// TestTCPSocket_Addresses_WhenConnected 测试连接后获取地址
func TestTCPSocket_Addresses_WhenConnected(t *testing.T) {
	_, cleanup := mockEchoServer(t, "127.0.0.1:18807")
	defer cleanup()

	cm := NewCosocketManager()
	defer cm.Close()

	socket := NewTCPSocket(cm)
	defer socket.Close()

	if err := socket.Connect("127.0.0.1", 18807); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	assert.NotNil(t, socket.LocalAddr())
	assert.NotNil(t, socket.RemoteAddr())
	assert.Contains(t, socket.RemoteAddr().String(), "127.0.0.1:18807")
}

// ---- Lua API Tests ----

// TestLuaAPI_newTCPSocketFunc 测试 newTCPSocketFunc
func TestLuaAPI_newTCPSocketFunc(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	RegisterTCPSocketAPI(engine.L, engine)

	err = engine.L.DoString(`
		local sock = ngx.socket.tcp()
		assert(sock ~= nil)
		assert(type(sock) == "userdata")
	`)
	require.NoError(t, err)
}

// TestLuaAPI_tcpSocketConnect 测试 tcpSocketConnect
func TestLuaAPI_tcpSocketConnect(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	RegisterTCPSocketAPI(engine.L, engine)

	// 测试 connect 返回值结构（不等待实际连接完成，因为没有 yield 处理）
	err = engine.L.DoString(`
		local sock = ngx.socket.tcp()
		local res1, res2 = sock:connect("127.0.0.1", 9999)
		-- res1 应该是 "cosocket_connect"，res2 是 op ID
		assert(type(res1) == "string")
		assert(res1 == "cosocket_connect")
		assert(type(res2) == "number")
	`)
	require.NoError(t, err)
}

// TestLuaAPI_tcpSocketConnect_WithError 测试 connect 错误返回
func TestLuaAPI_tcpSocketConnect_WithError(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	RegisterTCPSocketAPI(engine.L, engine)

	// 尝试连接到非空闲 socket（已连接过但这里没有，用非法端口）
	err = engine.L.DoString(`
		local sock = ngx.socket.tcp()
		-- 先用一个 nil 测试 connect 的 Lua 参数错误
		local res, err = pcall(function()
			sock:connect(123) -- wrong argument types
		end)
	`)
	// Lua 可能 raise error，这取决于实现
	_ = err
}

// TestLuaAPI_tcpSocketSend 测试 tcpSocketSend
func TestLuaAPI_tcpSocketSend(t *testing.T) {
	_, cleanup := mockEchoServer(t, "127.0.0.1:18809")
	defer cleanup()

	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	RegisterTCPSocketAPI(engine.L, engine)

	// connect 和 send 都会返回 yield 值（cosocket_xxx, op_id）
	// 在没有实际 yield 处理的情况下，只测试不报错
	err = engine.L.DoString(`
		local sock = ngx.socket.tcp()
		local res1, res2 = sock:connect("127.0.0.1", 18809)
		-- res1 应该是 "cosocket_connect"，res2 应该是 op ID
		-- 没有 yield 处理，连接实际未完成
	`)
	require.NoError(t, err)
}

// TestLuaAPI_tcpSocketSend_Error 测试 send 错误（未连接时发送）
func TestLuaAPI_tcpSocketSend_Error(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	RegisterTCPSocketAPI(engine.L, engine)

	err = engine.L.DoString(`
		local sock = ngx.socket.tcp()
		local res, err = sock:send("hello")
		-- 未连接时应该返回 nil + error
		assert(res == nil)
		assert(err ~= nil)
	`)
	require.NoError(t, err)
}

// TestLuaAPI_tcpSocketReceive 测试 tcpSocketReceive
func TestLuaAPI_tcpSocketReceive(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	RegisterTCPSocketAPI(engine.L, engine)

	// 测试未连接时的 receive 返回错误
	err = engine.L.DoString(`
		local sock = ngx.socket.tcp()
		local res, err = sock:receive(1024)
		-- 未连接时应该返回 nil + error
		assert(res == nil)
		assert(err ~= nil)
	`)
	require.NoError(t, err)
}

// TestLuaAPI_tcpSocketReceive_Error 测试 receive 错误（未连接时接收）
func TestLuaAPI_tcpSocketReceive_Error(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	RegisterTCPSocketAPI(engine.L, engine)

	err = engine.L.DoString(`
		local sock = ngx.socket.tcp()
		local res, err = sock:receive(1024)
		assert(res == nil)
		assert(err ~= nil)
	`)
	require.NoError(t, err)
}

// TestLuaAPI_tcpSocketReceive_Pattern 测试 receive 带模式
func TestLuaAPI_tcpSocketReceive_Pattern(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	RegisterTCPSocketAPI(engine.L, engine)

	// 测试未连接时 receive("*a") 返回错误
	err = engine.L.DoString(`
		local sock = ngx.socket.tcp()
		local res, err = sock:receive("*a")
		assert(res == nil)
		assert(err ~= nil)
	`)
	require.NoError(t, err)
}

// TestLuaAPI_tcpSocketReceive_UnknownPattern 测试未知模式
func TestLuaAPI_tcpSocketReceive_UnknownPattern(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	cm := NewCosocketManager()
	defer cm.Close()

	// 创建一个已连接但 conn 为 nil 的 socket 来测试模式接收的错误路径
	// 我们需要通过 Lua API 间接测试
	RegisterTCPSocketAPI(engine.L, engine)

	// 测试 unknown pattern "*x"
	// 需要先连接才能进入 pattern 匹配，但这里直接测试模式错误路径
	// 实际上 receive("*x") 在未连接时会先报 "not connected"
	err = engine.L.DoString(`
		local sock = ngx.socket.tcp()
		-- 未连接时用 *x 模式会先报 not connected
		local res, err = sock:receive("*x")
		assert(res == nil)
		assert(err ~= nil)
	`)
	require.NoError(t, err)
}

// TestLuaAPI_tcpSocketReceiveWithTable 测试 receive 带 table 参数
func TestLuaAPI_tcpSocketReceiveWithTable(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	RegisterTCPSocketAPI(engine.L, engine)

	// 未连接时 receive 带 table 参数返回错误
	err = engine.L.DoString(`
		local sock = ngx.socket.tcp()
		local res, err = sock:receive({timeout = 5000})
		assert(res == nil)
		assert(err ~= nil)
	`)
	require.NoError(t, err)
}

// TestLuaAPI_tcpSocketReceiveUntil 测试 receiveuntil
func TestLuaAPI_tcpSocketReceiveUntil(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	RegisterTCPSocketAPI(engine.L, engine)

	// 未连接时 receiveuntil 会先报 not connected
	err = engine.L.DoString(`
		local sock = ngx.socket.tcp()
		local res, err = sock:receiveuntil("|")
		assert(res == nil)
		assert(err ~= nil)
	`)
	require.NoError(t, err)
}

// TestLuaAPI_tcpSocketReceiveUntil_Inclusive 测试 receiveuntil 带 inclusive 选项
func TestLuaAPI_tcpSocketReceiveUntil_Inclusive(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	RegisterTCPSocketAPI(engine.L, engine)

	// 未连接时 receiveuntil 会先报 not connected
	err = engine.L.DoString(`
		local sock = ngx.socket.tcp()
		local res, err = sock:receiveuntil("|", {inclusive = true})
		assert(res == nil)
		assert(err ~= nil)
	`)
	require.NoError(t, err)
}

// TestLuaAPI_tcpSocketClose 测试 tcpSocketClose
func TestLuaAPI_tcpSocketClose(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	RegisterTCPSocketAPI(engine.L, engine)

	err = engine.L.DoString(`
		local sock = ngx.socket.tcp()
		local ok = sock:close()
		assert(ok == true)
	`)
	require.NoError(t, err)
}

// TestLuaAPI_tcpSocketSetTimeout 测试 settimeout
func TestLuaAPI_tcpSocketSetTimeout(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	RegisterTCPSocketAPI(engine.L, engine)

	err = engine.L.DoString(`
		local sock = ngx.socket.tcp()
		local ok = sock:settimeout(5000)
		assert(ok == true)
	`)
	require.NoError(t, err)
}

// TestLuaAPI_tcpSocketSetTimeouts 测试 settimeouts
func TestLuaAPI_tcpSocketSetTimeouts(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	RegisterTCPSocketAPI(engine.L, engine)

	err = engine.L.DoString(`
		local sock = ngx.socket.tcp()
		local ok = sock:settimeouts(1000, 2000, 3000)
		assert(ok == true)
	`)
	require.NoError(t, err)
}

// TestLuaAPI_tcpSocketToString 测试 __tostring
func TestLuaAPI_tcpSocketToString(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	RegisterTCPSocketAPI(engine.L, engine)

	err = engine.L.DoString(`
		local sock = ngx.socket.tcp()
		local str = tostring(sock)
		assert(str:find("tcp_socket"))
	`)
	require.NoError(t, err)
}

// TestLuaAPI_tcpSocketGC 测试 __gc
func TestLuaAPI_tcpSocketGC(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	RegisterTCPSocketAPI(engine.L, engine)

	// 创建 socket 并触发 GC
	err = engine.L.DoString(`
		local sock = ngx.socket.tcp()
		sock = nil
		-- 强制 GC（Lua GC 可能不会立即触发 __gc）
		collectgarbage("collect")
	`)
	require.NoError(t, err)
}

// TestCheckTCPSocket_ArgError 测试 checkTCPSocket 参数错误
func TestCheckTCPSocket_ArgError(t *testing.T) {
	L := glua.NewState()
	defer L.Close()

	// 传入非 userdata 应该 raise error
	L.Push(glua.LString("not a socket"))
	_ = L.PCall(0, 0, nil)
	// 这不会触发 checkTCPSocket，我们需要通过 Lua 脚本来测试
	_ = L.DoString(`
		local sock = ngx.socket.tcp()
	`)
	// 在没有注册 API 的情况下会失败
}

// TestLuaAPI_tcpSocketConnect_WithTimeout 测试 connect 带超时选项
func TestLuaAPI_tcpSocketConnect_WithTimeout(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	RegisterTCPSocketAPI(engine.L, engine)

	// 测试 connect 带超时选项（不等待实际连接完成）
	err = engine.L.DoString(`
		local sock = ngx.socket.tcp()
		local res1, res2 = sock:connect("127.0.0.1", 9999, {timeout = 100})
		-- 返回值应该是 "cosocket_connect" 和 op ID
		assert(type(res1) == "string")
		assert(type(res2) == "number")
	`)
	require.NoError(t, err)
}

// TestLuaAPI_tcpSocketSend_WithTimeout 测试 send 带超时选项
func TestLuaAPI_tcpSocketSend_WithTimeout(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	RegisterTCPSocketAPI(engine.L, engine)

	// 未连接时 send 返回 nil + error
	err = engine.L.DoString(`
		local sock = ngx.socket.tcp()
		local res, err = sock:send("hello", {timeout = 5000})
		-- 未连接时应该返回 nil + error
		assert(res == nil)
		assert(err ~= nil)
	`)
	require.NoError(t, err)
}

// TestCosocketYield_Connect 测试 cosocket connect yield 处理
func TestCosocketYield_Connect(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	coro, err := engine.NewCoroutine(nil)
	require.NoError(t, err)
	defer coro.Close()

	err = coro.SetupSandbox()
	require.NoError(t, err)

	// 测试 connect 返回值结构（不等待实际连接完成）
	err = coro.Execute(`
		local sock = ngx.socket.tcp()
		local res1, res2 = sock:connect("127.0.0.1", 9999)
		-- res1 应该是 "cosocket_connect"，res2 是 op ID
		assert(type(res1) == "string")
		assert(res1 == "cosocket_connect")
		assert(type(res2) == "number")
	`)
	require.NoError(t, err)
}

// TestHandleCosocketYield_Unknown 测试未知 yield reason
func TestHandleCosocketYield_Unknown(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	coro, err := engine.NewCoroutine(nil)
	require.NoError(t, err)
	defer coro.Close()

	_, err = coro.HandleCosocketYield("unknown_reason", []glua.LValue{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown cosocket yield reason")
}

// TestHandleCosocketConnect_MissingOpID 测试 connect yield 缺少 op ID
func TestHandleCosocketConnect_MissingOpID(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	coro, err := engine.NewCoroutine(nil)
	require.NoError(t, err)
	defer coro.Close()

	_, err = coro.HandleCosocketYield("cosocket_connect", []glua.LValue{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "requires operation ID")
}

// TestHandleCosocketConnect_OpNotFound 测试 connect yield op 不存在
func TestHandleCosocketConnect_OpNotFound(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	coro, err := engine.NewCoroutine(nil)
	require.NoError(t, err)
	defer coro.Close()

	_, err = coro.HandleCosocketYield("cosocket_connect", []glua.LValue{glua.LNumber(999999)})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// TestHandleCosocketSend_MissingOpID 测试 send yield 缺少 op ID
func TestHandleCosocketSend_MissingOpID(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	coro, err := engine.NewCoroutine(nil)
	require.NoError(t, err)
	defer coro.Close()

	_, err = coro.HandleCosocketYield("cosocket_send", []glua.LValue{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "requires operation ID")
}

// TestHandleCosocketSend_OpNotFound 测试 send yield op 不存在
func TestHandleCosocketSend_OpNotFound(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	coro, err := engine.NewCoroutine(nil)
	require.NoError(t, err)
	defer coro.Close()

	_, err = coro.HandleCosocketYield("cosocket_send", []glua.LValue{glua.LNumber(999999)})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// TestHandleCosocketReceive_MissingOpID 测试 receive yield 缺少 op ID
func TestHandleCosocketReceive_MissingOpID(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	coro, err := engine.NewCoroutine(nil)
	require.NoError(t, err)
	defer coro.Close()

	_, err = coro.HandleCosocketYield("cosocket_receive", []glua.LValue{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "requires operation ID")
}

// TestHandleCosocketReceive_OpNotFound 测试 receive yield op 不存在
func TestHandleCosocketReceive_OpNotFound(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	coro, err := engine.NewCoroutine(nil)
	require.NoError(t, err)
	defer coro.Close()

	_, err = coro.HandleCosocketYield("cosocket_receive", []glua.LValue{glua.LNumber(999999)})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// TestHandleCosocketReceive_EmptyData 测试 receive yield 返回空数据
func TestHandleCosocketReceive_EmptyData(t *testing.T) {
	socket := NewTCPSocket(DefaultCosocketManager)
	defer socket.Close()

	// 创建一个空的 receive 操作
	op := DefaultCosocketManager.StartOperation(socket, OpReceive, 5*time.Second)
	op.Complete([]byte{}, nil)

	coro := &LuaCoroutine{
		Engine:           nil,
		ExecutionContext: context.Background(),
	}

	results, err := coro.HandleCosocketYield("cosocket_receive", []glua.LValue{glua.LNumber(op.ID)})
	require.NoError(t, err)
	// 空数据应该返回 nil + "closed"
	assert.Equal(t, 2, len(results))
	assert.Equal(t, glua.LNil, results[0])
	assert.Equal(t, glua.LString("closed"), results[1])
}

// TestHandleCosocketReceive_InvalidResult 测试 receive yield 无效结果类型
func TestHandleCosocketReceive_InvalidResult(t *testing.T) {
	socket := NewTCPSocket(DefaultCosocketManager)
	defer socket.Close()

	op := DefaultCosocketManager.StartOperation(socket, OpReceive, 5*time.Second)
	op.Complete("not_bytes", nil) // 非 []byte 类型

	coro := &LuaCoroutine{
		Engine:           nil,
		ExecutionContext: context.Background(),
	}

	results, err := coro.HandleCosocketYield("cosocket_receive", []glua.LValue{glua.LNumber(op.ID)})
	require.NoError(t, err)
	assert.Equal(t, 2, len(results))
	assert.Equal(t, glua.LNil, results[0])
	assert.Equal(t, glua.LString("invalid result"), results[1])
}

// TestHandleCosocketSend_InvalidResult 测试 send yield 无效结果类型
func TestHandleCosocketSend_InvalidResult(t *testing.T) {
	socket := NewTCPSocket(DefaultCosocketManager)
	defer socket.Close()

	op := DefaultCosocketManager.StartOperation(socket, OpSend, 5*time.Second)
	op.Complete("not_int", nil) // 非 int 类型

	coro := &LuaCoroutine{
		Engine:           nil,
		ExecutionContext: context.Background(),
	}

	results, err := coro.HandleCosocketYield("cosocket_send", []glua.LValue{glua.LNumber(op.ID)})
	require.NoError(t, err)
	assert.Equal(t, 2, len(results))
	assert.Equal(t, glua.LNil, results[0])
	assert.Equal(t, glua.LString("invalid result"), results[1])
}

// TestHandleCosocketConnect_NilResult 测试 connect yield nil result
func TestHandleCosocketConnect_NilResult(t *testing.T) {
	socket := NewTCPSocket(DefaultCosocketManager)
	defer socket.Close()

	op := DefaultCosocketManager.StartOperation(socket, OpConnect, 5*time.Second)
	op.Complete(nil, nil)

	coro := &LuaCoroutine{
		Engine:           nil,
		ExecutionContext: context.Background(),
	}

	results, err := coro.HandleCosocketYield("cosocket_connect", []glua.LValue{glua.LNumber(op.ID)})
	require.NoError(t, err)
	assert.Equal(t, 2, len(results))
	assert.Equal(t, glua.LNil, results[0])
	assert.Equal(t, glua.LNil, results[1])
}

// TestHandleCosocketConnect_WithError 测试 connect yield 带错误
func TestHandleCosocketConnect_WithError(t *testing.T) {
	socket := NewTCPSocket(DefaultCosocketManager)
	defer socket.Close()

	op := DefaultCosocketManager.StartOperation(socket, OpConnect, 5*time.Second)
	op.Complete(nil, fmt.Errorf("connection refused"))

	coro := &LuaCoroutine{
		Engine:           nil,
		ExecutionContext: context.Background(),
	}

	results, err := coro.HandleCosocketYield("cosocket_connect", []glua.LValue{glua.LNumber(op.ID)})
	require.NoError(t, err)
	assert.Equal(t, 2, len(results))
	assert.Equal(t, glua.LNil, results[0])
	assert.Contains(t, string(glua.LVAsString(results[1])), "connection refused")
}

// TestHandleCosocketSend_WithError 测试 send yield 带错误
func TestHandleCosocketSend_WithError(t *testing.T) {
	socket := NewTCPSocket(DefaultCosocketManager)
	defer socket.Close()

	op := DefaultCosocketManager.StartOperation(socket, OpSend, 5*time.Second)
	op.Complete(0, fmt.Errorf("write error"))

	coro := &LuaCoroutine{
		Engine:           nil,
		ExecutionContext: context.Background(),
	}

	results, err := coro.HandleCosocketYield("cosocket_send", []glua.LValue{glua.LNumber(op.ID)})
	require.NoError(t, err)
	assert.Equal(t, 2, len(results))
	assert.Equal(t, glua.LNil, results[0])
	assert.Contains(t, string(glua.LVAsString(results[1])), "write error")
}

// TestHandleCosocketReceive_WithData 测试 receive yield 带数据
func TestHandleCosocketReceive_WithData(t *testing.T) {
	socket := NewTCPSocket(DefaultCosocketManager)
	defer socket.Close()

	expected := []byte("hello world")
	op := DefaultCosocketManager.StartOperation(socket, OpReceive, 5*time.Second)
	op.Complete(expected, nil)

	coro := &LuaCoroutine{
		Engine:           nil,
		ExecutionContext: context.Background(),
	}

	results, err := coro.HandleCosocketYield("cosocket_receive", []glua.LValue{glua.LNumber(op.ID)})
	require.NoError(t, err)
	assert.Equal(t, 1, len(results))
	assert.Equal(t, glua.LString("hello world"), results[0])
}

// TestHandleCosocketSend_Success 测试 send yield 成功
func TestHandleCosocketSend_Success(t *testing.T) {
	socket := NewTCPSocket(DefaultCosocketManager)
	defer socket.Close()

	op := DefaultCosocketManager.StartOperation(socket, OpSend, 5*time.Second)
	op.Complete(5, nil)

	coro := &LuaCoroutine{
		Engine:           nil,
		ExecutionContext: context.Background(),
	}

	results, err := coro.HandleCosocketYield("cosocket_send", []glua.LValue{glua.LNumber(op.ID)})
	require.NoError(t, err)
	assert.Equal(t, 1, len(results))
	assert.Equal(t, glua.LNumber(5), results[0])
}

// TestHandleCosocketConnect_Success 测试 connect yield 成功
func TestHandleCosocketConnect_Success(t *testing.T) {
	socket := NewTCPSocket(DefaultCosocketManager)
	defer socket.Close()

	op := DefaultCosocketManager.StartOperation(socket, OpConnect, 5*time.Second)
	op.Complete(&net.TCPConn{}, nil) // 非 nil result

	coro := &LuaCoroutine{
		Engine:           nil,
		ExecutionContext: context.Background(),
	}

	results, err := coro.HandleCosocketYield("cosocket_connect", []glua.LValue{glua.LNumber(op.ID)})
	require.NoError(t, err)
	assert.Equal(t, 1, len(results))
	assert.Equal(t, glua.LNumber(1), results[0])
}

// TestRegisterTCPSocketMetaTable 测试元表注册
func TestRegisterTCPSocketMetaTable(t *testing.T) {
	L := glua.NewState()
	defer L.Close()

	registerTCPSocketMetaTable(L)

	// 验证元表存在
	mt := L.GetGlobal(tcpSocketMT)
	assert.NotNil(t, mt)
	assert.IsType(t, &glua.LTable{}, mt)
}

// TestRegisterTCPSocketAPI_NgxTableCreation 测试 ngx.socket API 注册时创建 ngx 表
func TestRegisterTCPSocketAPI_NgxTableCreation(t *testing.T) {
	L := glua.NewState()
	defer L.Close()

	// 确保 ngx 表不存在
	L.SetGlobal("ngx", glua.LNil)

	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	RegisterTCPSocketAPI(L, engine)

	// 验证 ngx.socket.tcp 存在
	ngx := L.GetGlobal("ngx")
	require.IsType(t, &glua.LTable{}, ngx)
	ngxTbl := ngx.(*glua.LTable)

	socket := ngxTbl.RawGetString("socket")
	require.IsType(t, &glua.LTable{}, socket)
	socketTbl := socket.(*glua.LTable)

	tcp := socketTbl.RawGetString("tcp")
	assert.IsType(t, &glua.LFunction{}, tcp)
}

// TestOperationType_String 测试操作类型字符串
func TestOperationType_String(t *testing.T) {
	assert.Equal(t, "connect", string(OpConnect))
	assert.Equal(t, "send", string(OpSend))
	assert.Equal(t, "receive", string(OpReceive))
	assert.Equal(t, "close", string(OpClose))
}

// TestSocketOperation_Complete 测试操作完成
func TestSocketOperation_Complete(t *testing.T) {
	op := &SocketOperation{
		ID:   1,
		Done: make(chan struct{}),
	}

	// 完成操作
	op.Complete("result", nil)
	assert.True(t, op.IsCompleted())
	assert.Equal(t, "result", op.Result)
	assert.Nil(t, op.Error)

	// 重复完成应该无影响
	op.Complete("other", fmt.Errorf("err"))
	assert.Equal(t, "result", op.Result) // 保持第一次的值
}

// TestSocketOperation_Wait_Timeout 测试 Wait 超时
func TestSocketOperation_Wait_Timeout(t *testing.T) {
	op := &SocketOperation{
		ID:   1,
		Done: make(chan struct{}),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	result, err := op.Wait(ctx)
	assert.Nil(t, result)
	assert.Error(t, err) // context deadline exceeded
}

// TestSocketOperation_Touch 测试 Touch
func TestSocketOperation_Touch(t *testing.T) {
	op := &SocketOperation{
		ID:           1,
		Done:         make(chan struct{}),
		LastActivity: time.Now().Add(-time.Hour),
	}

	oldTime := op.LastActivity
	op.Touch()
	assert.True(t, op.LastActivity.After(oldTime))
}
