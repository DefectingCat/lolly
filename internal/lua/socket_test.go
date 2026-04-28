package lua

import (
	"context"
	"fmt"
	"net"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func init() {
	// 测试环境允许回环地址连接（SSRF 防护对 localhost mock 服务器放宽）
	DefaultCosocketManager.DisableSSRFGuard = true
	testingSSRFGuardDisabled = true
}

// mockEchoServer 模拟 echo 服务器
func mockEchoServer(t *testing.T, addr string) (net.Listener, func()) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("Failed to listen: %v", err)
	}

	var wg sync.WaitGroup
	stop := make(chan struct{})

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				select {
				case <-stop:
					return
				default:
					continue
				}
			}

			wg.Add(1)
			go func(c net.Conn) {
				defer wg.Done()
				buf := make([]byte, 4096)
				for {
					n, err := c.Read(buf)
					if err != nil {
						return
					}
					if n > 0 {
						if _, err := c.Write(buf[:n]); err != nil {
							return
						}
					}
				}
			}(conn)
		}
	}()

	cleanup := func() {
		close(stop)
		ln.Close()
		wg.Wait()
	}

	return ln, cleanup
}

// TestCosocketManager_Basic 测试基本功能
func TestCosocketManager_Basic(t *testing.T) {
	cm := NewCosocketManager()
	defer cm.Close()

	// 测试初始状态
	stats := cm.Stats()
	if stats.TotalOperations != 0 {
		t.Errorf("Expected 0 operations, got %d", stats.TotalOperations)
	}

	// 测试操作创建
	socket := NewTCPSocket(cm)
	defer socket.Close()

	op := cm.StartOperation(socket, OpConnect, 5*time.Second)
	if op == nil {
		t.Fatal("Expected non-nil operation")
	}

	if op.ID == 0 {
		t.Error("Expected non-zero operation ID")
	}

	if op.Type != OpConnect {
		t.Errorf("Expected OpConnect, got %v", op.Type)
	}

	// 测试统计
	stats = cm.Stats()
	if stats.TotalOperations != 1 {
		t.Errorf("Expected 1 operation, got %d", stats.TotalOperations)
	}
	if stats.ActiveOperations != 1 {
		t.Errorf("Expected 1 active operation, got %d", stats.ActiveOperations)
	}

	// 测试操作完成
	cm.CompleteOperation(op.ID, "done", nil)

	stats = cm.Stats()
	if stats.ActiveOperations != 0 {
		t.Errorf("Expected 0 active operations after complete, got %d", stats.ActiveOperations)
	}
}

// TestCosocketManager_Timeout 测试超时机制
func TestCosocketManager_Timeout(t *testing.T) {
	// 创建一个使用短清理间隔的管理器用于测试
	cm := NewCosocketManager()
	cm.SetDefaultTimeout(100 * time.Millisecond)
	cm.cleanupInterval = 50 * time.Millisecond
	cm.timeoutChecker.Reset(50 * time.Millisecond)
	defer cm.Close()

	socket := NewTCPSocket(cm)
	defer socket.Close()

	// 创建一个不完成的操作
	op := cm.StartOperation(socket, OpConnect, 100*time.Millisecond)

	// 等待超时清理
	time.Sleep(300 * time.Millisecond)

	// 检查操作是否超时完成
	if !op.IsCompleted() {
		t.Error("Expected operation to be completed due to timeout")
	}

	stats := cm.Stats()
	if stats.TimeoutOperations != 1 {
		t.Errorf("Expected 1 timeout operation, got %d", stats.TimeoutOperations)
	}
}

// TestTCPSocket_Connect 测试 TCP 连接
func TestTCPSocket_Connect(t *testing.T) {
	_, cleanup := mockEchoServer(t, "127.0.0.1:19999")
	defer cleanup()

	cm := NewCosocketManager()
	defer cm.Close()

	socket := NewTCPSocket(cm)
	defer socket.Close()

	// 测试连接
	err := socket.Connect("127.0.0.1", 19999)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	// 等待连接完成
	op := socket.currentOp
	if op != nil {
		result, err := op.Wait(context.Background())
		if err != nil {
			t.Fatalf("Connect wait failed: %v", err)
		}
		if result == nil {
			t.Fatal("Expected non-nil connection")
		}
	}

	if socket.State() != SocketStateConnected {
		t.Errorf("Expected state connected, got %v", socket.State())
	}
}

// TestTCPSocket_SendReceive 测试发送接收
func TestTCPSocket_SendReceive(t *testing.T) {
	_, cleanup := mockEchoServer(t, "127.0.0.1:19998")
	defer cleanup()

	cm := NewCosocketManager()
	defer cm.Close()

	socket := NewTCPSocket(cm)
	defer socket.Close()

	// 连接
	if err := socket.Connect("127.0.0.1", 19998); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	// 等待连接完成
	time.Sleep(100 * time.Millisecond)

	// 发送数据
	testData := "Hello, Cosocket!"
	n, err := socket.Send([]byte(testData))
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}
	if n != len(testData) {
		t.Errorf("Expected %d bytes sent, got %d", len(testData), n)
	}

	// 接收数据
	received, err := socket.Receive(1024)
	if err != nil {
		t.Fatalf("Receive failed: %v", err)
	}
	if string(received) != testData {
		t.Errorf("Expected '%s', got '%s'", testData, string(received))
	}
}

// TestTCPSocket_AsyncOperations 测试异步操作
func TestTCPSocket_AsyncOperations(t *testing.T) {
	_, cleanup := mockEchoServer(t, "127.0.0.1:19997")
	defer cleanup()

	cm := NewCosocketManager()
	defer cm.Close()

	socket := NewTCPSocket(cm)
	defer socket.Close()

	// 测试异步连接
	err := socket.Connect("127.0.0.1", 19997)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	// 等待连接完成
	time.Sleep(100 * time.Millisecond)

	// 测试异步发送
	testData := "Async test"
	sendOp, err := socket.SendAsync([]byte(testData))
	if err != nil {
		t.Fatalf("SendAsync failed: %v", err)
	}

	result, err := sendOp.Wait(context.Background())
	if err != nil {
		t.Fatalf("Send wait failed: %v", err)
	}
	if n, ok := result.(int); !ok || n != len(testData) {
		t.Errorf("Expected %d bytes, got %v", len(testData), result)
	}

	// 测试异步接收
	recvOp, err := socket.ReceiveAsync(1024)
	if err != nil {
		t.Fatalf("ReceiveAsync failed: %v", err)
	}

	result, err = recvOp.Wait(context.Background())
	if err != nil {
		t.Fatalf("Receive wait failed: %v", err)
	}
	if data, ok := result.([]byte); !ok || string(data) != testData {
		t.Errorf("Expected '%s', got %v", testData, result)
	}
}

// TestTCPSocket_ReceiveUntil 测试接收直到特定模式
func TestTCPSocket_ReceiveUntil(t *testing.T) {
	_, cleanup := mockEchoServer(t, "127.0.0.1:19996")
	defer cleanup()

	cm := NewCosocketManager()
	defer cm.Close()

	socket := NewTCPSocket(cm)
	defer socket.Close()

	// 连接
	if err := socket.Connect("127.0.0.1", 19996); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	// 等待连接完成
	time.Sleep(100 * time.Millisecond)

	// 发送带换行的数据
	testData := "Line1\nLine2\nLine3\n"
	_, err := socket.Send([]byte(testData))
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	// 接收直到换行
	data, err := socket.ReceiveUntil("\n", true)
	if err != nil {
		t.Fatalf("ReceiveUntil failed: %v", err)
	}
	if string(data) != "Line1\n" {
		t.Errorf("Expected 'Line1\\n', got '%s'", string(data))
	}
}

// TestTCPSocket_Close 测试关闭
func TestTCPSocket_Close(t *testing.T) {
	cm := NewCosocketManager()
	defer cm.Close()

	socket := NewTCPSocket(cm)

	if socket.IsClosed() {
		t.Error("Socket should not be closed initially")
	}

	err := socket.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}

	if !socket.IsClosed() {
		t.Error("Socket should be closed")
	}

	// 重复关闭应该返回 nil
	err = socket.Close()
	if err != nil {
		t.Errorf("Second close should not error: %v", err)
	}
}

// TestTCPSocket_StateTransitions 测试状态转换
func TestTCPSocket_StateTransitions(t *testing.T) {
	_, cleanup := mockEchoServer(t, "127.0.0.1:19995")
	defer cleanup()

	cm := NewCosocketManager()
	defer cm.Close()

	socket := NewTCPSocket(cm)
	defer socket.Close()

	// 初始状态
	if socket.State() != SocketStateIdle {
		t.Errorf("Expected idle state, got %v", socket.State())
	}

	// 连接中
	socket.Connect("127.0.0.1", 19995)
	if socket.State() != SocketStateConnecting {
		t.Errorf("Expected connecting state, got %v", socket.State())
	}

	// 等待连接完成
	time.Sleep(100 * time.Millisecond)

	if socket.State() != SocketStateConnected {
		t.Errorf("Expected connected state, got %v", socket.State())
	}
}

// TestCosocketManager_Concurrent 测试并发操作
func TestCosocketManager_Concurrent(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping concurrent test in short mode")
	}

	_, cleanup := mockEchoServer(t, "127.0.0.1:19994")
	defer cleanup()

	cm := NewCosocketManager()
	defer cm.Close()

	const numSockets = 1000
	const numGoroutines = 100

	var wg sync.WaitGroup
	errors := make(chan error, numSockets)
	var completed int32

	// 并发创建 socket 和连接
	for i := range numGoroutines {
		wg.Add(1)
		go func(start int) {
			defer wg.Done()
			for j := range numSockets / numGoroutines {
				socket := NewTCPSocket(cm)
				if err := socket.Connect("127.0.0.1", 19994); err != nil {
					errors <- fmt.Errorf("connect failed: %v", err)
					socket.Close()
					continue
				}

				// 等待连接
				time.Sleep(50 * time.Millisecond)

				// 发送数据
				data := fmt.Sprintf("Test%d", start+j)
				if _, err := socket.Send([]byte(data)); err != nil {
					errors <- fmt.Errorf("send failed: %v", err)
					socket.Close()
					continue
				}

				socket.Close()
				atomic.AddInt32(&completed, 1)
			}
		}(i * (numSockets / numGoroutines))
	}

	wg.Wait()

	// 检查错误
	close(errors)
	errCount := 0
	for err := range errors {
		t.Logf("Error: %v", err)
		errCount++
	}

	t.Logf("Completed: %d/%d, Errors: %d", completed, numSockets, errCount)

	// 检查统计
	stats := cm.Stats()
	t.Logf("Stats: %+v", stats)
}

// TestCosocketManager_MemoryLeak 测试内存泄漏
func TestCosocketManager_MemoryLeak(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping memory leak test in short mode")
	}

	_, cleanup := mockEchoServer(t, "127.0.0.1:19993")
	defer cleanup()

	cm := NewCosocketManager()
	defer cm.Close()

	// 记录初始 goroutine 数
	initialGoroutines := runtime.NumGoroutine()

	// 创建和关闭大量 socket
	for range 10000 {
		socket := NewTCPSocket(cm)
		// 使用同步连接避免竞态
		socket.Connect("127.0.0.1", 19993)
		time.Sleep(time.Millisecond) // 给连接时间完成
		socket.Close()
	}

	// 强制 GC
	runtime.GC()
	time.Sleep(100 * time.Millisecond)

	// 检查 goroutine 数量
	finalGoroutines := runtime.NumGoroutine()
	t.Logf("Initial goroutines: %d, Final goroutines: %d", initialGoroutines, finalGoroutines)

	// 允许一定的波动
	if finalGoroutines > initialGoroutines+100 {
		t.Errorf("Possible goroutine leak: started with %d, ended with %d", initialGoroutines, finalGoroutines)
	}

	// 检查统计
	stats := cm.Stats()
	if stats.ActiveSockets > 100 {
		t.Errorf("Active sockets leak: %d", stats.ActiveSockets)
	}
	if stats.ActiveOperations > 100 {
		t.Errorf("Active operations leak: %d", stats.ActiveOperations)
	}
}

// TestCosocketManager_LongRunning 测试长时间运行
func TestCosocketManager_LongRunning(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping long running test in short mode")
	}

	_, cleanup := mockEchoServer(t, "127.0.0.1:19992")
	defer cleanup()

	cm := NewCosocketManager()
	defer cm.Close()

	duration := 10 * time.Second // 缩短到 10 秒进行测试
	interval := 100 * time.Millisecond

	var totalOps int32
	start := time.Now()

	for time.Since(start) < duration {
		socket := NewTCPSocket(cm)
		if err := socket.Connect("127.0.0.1", 19992); err != nil {
			t.Logf("Connect error: %v", err)
			socket.Close()
			continue
		}

		// 等待连接
		time.Sleep(50 * time.Millisecond)

		// 发送接收
		if _, err := socket.Send([]byte("test")); err == nil {
			socket.Receive(1024)
		}

		socket.Close()
		atomic.AddInt32(&totalOps, 1)
		time.Sleep(interval)
	}

	elapsed := time.Since(start)
	t.Logf("Completed %d operations in %v", totalOps, elapsed)

	// 检查最终统计
	stats := cm.Stats()
	t.Logf("Final stats: %+v", stats)

	if stats.ActiveSockets > 0 {
		t.Errorf("Expected 0 active sockets, got %d", stats.ActiveSockets)
	}
	if stats.ActiveOperations > 0 {
		t.Errorf("Expected 0 active operations, got %d", stats.ActiveOperations)
	}
}

// BenchmarkCosocket_Connect 基准测试：连接
func BenchmarkCosocket_Connect(b *testing.B) {
	_, cleanup := mockEchoServer(nil, "127.0.0.1:19991")
	defer cleanup()

	cm := NewCosocketManager()
	defer cm.Close()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			socket := NewTCPSocket(cm)
			socket.Connect("127.0.0.1", 19991)
			time.Sleep(10 * time.Millisecond)
			socket.Close()
		}
	})
}

// BenchmarkCosocket_SendReceive 基准测试：发送接收
func BenchmarkCosocket_SendReceive(b *testing.B) {
	_, cleanup := mockEchoServer(nil, "127.0.0.1:19990")
	defer cleanup()

	cm := NewCosocketManager()
	defer cm.Close()

	// 预先连接
	socket := NewTCPSocket(cm)
	socket.Connect("127.0.0.1", 19990)
	time.Sleep(100 * time.Millisecond)

	b.ResetTimer()
	for b.Loop() {
		socket.Send([]byte("benchmark"))
		socket.Receive(1024)
	}
	b.StopTimer()

	socket.Close()
}

// TestLuaAPI_TCPSocket 测试 Lua API
func TestLuaAPI_TCPSocket(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping Lua API test in short mode")
	}

	// 创建引擎
	engine, err := NewEngine(nil)
	if err != nil {
		t.Fatalf("Failed to create engine: %v", err)
	}
	defer engine.Close()

	// 注册 TCP socket API
	RegisterTCPSocketAPI(engine.L, engine)

	// 测试创建 socket
	script := `
		local sock = ngx.socket.tcp()
		if not sock then
			return nil, "failed to create socket"
		end
		return "ok"
	`

	coro, err := engine.NewCoroutine(nil)
	if err != nil {
		t.Fatalf("Failed to create coroutine: %v", err)
	}
	defer coro.Close()

	if err := coro.SetupSandbox(); err != nil {
		t.Fatalf("Failed to setup sandbox: %v", err)
	}

	err = coro.Execute(script)
	if err != nil {
		t.Errorf("Execute failed: %v", err)
	}
}

// TestCosocketManager_Stress 压力测试
func TestCosocketManager_Stress(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	// 创建多个 echo 服务器
	ports := []int{19980, 19981, 19982, 19983}
	cleanups := make([]func(), len(ports))
	for i, port := range ports {
		_, cleanups[i] = mockEchoServer(t, fmt.Sprintf("127.0.0.1:%d", port))
	}
	defer func() {
		for _, c := range cleanups {
			c()
		}
	}()

	cm := NewCosocketManager()
	defer cm.Close()

	const totalConnections = 1000
	const concurrency = 100

	var wg sync.WaitGroup
	var successCount int32
	var errorCount int32
	var latencySum int64

	start := time.Now()

	// 使用信号量限制并发
	sem := make(chan struct{}, concurrency)

	for i := range totalConnections {
		wg.Add(1)
		sem <- struct{}{} // 获取信号量

		go func(idx int) {
			defer wg.Done()
			defer func() { <-sem }() // 释放信号量

			port := ports[idx%len(ports)]
			socket := NewTCPSocket(cm)

			opStart := time.Now()
			err := socket.Connect("127.0.0.1", port)
			if err != nil {
				atomic.AddInt32(&errorCount, 1)
				socket.Close()
				return
			}

			// 等待连接状态就绪（最多 50ms）
			for range 10 {
				if socket.State() == SocketStateConnected {
					break
				}
				time.Sleep(5 * time.Millisecond)
			}

			// 简单数据交换
			if _, err := socket.Send([]byte("hello")); err == nil {
				socket.Receive(1024)
			}

			socket.Close()

			latency := time.Since(opStart).Milliseconds()
			atomic.AddInt64(&latencySum, latency)
			atomic.AddInt32(&successCount, 1)
		}(i)
	}

	wg.Wait()
	elapsed := time.Since(start)

	t.Logf("Stress test completed:")
	t.Logf("  Total: %d, Success: %d, Errors: %d", totalConnections, successCount, errorCount)
	t.Logf("  Duration: %v", elapsed)
	t.Logf("  RPS: %.2f", float64(totalConnections)/elapsed.Seconds())
	if successCount > 0 {
		t.Logf("  Avg Latency: %dms", latencySum/int64(successCount))
	}

	// 内存检查
	var m runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m)
	t.Logf("  Memory: %.2f MB", float64(m.HeapAlloc)/(1024*1024))

	// 验证没有资源泄漏
	stats := cm.Stats()
	t.Logf("  Active sockets: %d, Active operations: %d", stats.ActiveSockets, stats.ActiveOperations)

	if errorCount > totalConnections/10 { // 允许 10% 错误率
		t.Errorf("Too many errors: %d", errorCount)
	}

	if stats.ActiveSockets > 100 {
		t.Errorf("Socket leak detected: %d active sockets", stats.ActiveSockets)
	}
}
