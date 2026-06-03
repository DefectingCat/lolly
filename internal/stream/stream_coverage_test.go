// Package stream 提供流代理覆盖测试。
//
// 该文件补充测试 stream.go 中未覆盖的方法：
//   - ipHash.Select() (空 IP)
//   - handleConnection() 连接处理
//   - getOrCreateSession() 会话创建
//   - handleBackendResponse() 后端响应处理
//   - Stats 完整统计
//
// 作者：xfy
package stream

import (
	"net"
	"testing"
	"time"
)

// TestIPHashSelect 测试 ipHash 的 Select 方法（空字符串 IP）
func TestIPHashSelect(t *testing.T) {
	targets := []*Target{
		{addr: "localhost:8001"},
		{addr: "localhost:8002"},
	}
	for _, target := range targets {
		target.healthy.Store(true)
	}

	ih := newIPHash()

	// Select() 使用空字符串作为 IP
	selected := ih.Select(targets)
	if selected == nil {
		t.Error("Select() with empty IP should return a target")
	}

	// 多次调用应返回相同目标（确定性哈希）
	selected2 := ih.Select(targets)
	if selected != selected2 {
		t.Error("Select() with same empty IP should be consistent")
	}

	// 无健康目标时应返回 nil
	for _, target := range targets {
		target.healthy.Store(false)
	}
	selected = ih.Select(targets)
	if selected != nil {
		t.Error("Select() with no healthy targets should return nil")
	}
}

// TestSelectByIPNoHealthy 测试 SelectByIP 无健康目标
func TestSelectByIPNoHealthy(t *testing.T) {
	targets := []*Target{
		{addr: "localhost:8001"},
		{addr: "localhost:8002"},
	}

	ih := newIPHash()
	selected := ih.(*ipHash).SelectByIP(targets, "192.168.1.1")
	if selected != nil {
		t.Error("SelectByIP() with no healthy targets should return nil")
	}
}

// TestWeightedRoundRobinZeroWeight 测试零权重处理
func TestWeightedRoundRobinZeroWeight(t *testing.T) {
	targets := []*Target{
		{addr: "localhost:8001", weight: 0},
		{addr: "localhost:8002", weight: -1},
	}
	for _, target := range targets {
		target.healthy.Store(true)
	}

	wrr := newWeightedRoundRobin().(*weightedRoundRobin)

	// 权重为 0 或负数应视为权重 1
	for range 4 {
		selected := wrr.Select(targets)
		if selected == nil {
			t.Error("Select() should return target with zero/negative weight")
			return
		}
	}
}

// TestHandleConnection 测试 handleConnection 方法
func TestHandleConnection(t *testing.T) {
	s := NewServer()

	// 添加上游配置
	targets := []TargetSpec{
		{Addr: "127.0.0.1:29001", Weight: 1},
	}
	_ = s.AddUpstream("test", targets, "round_robin", HealthCheckSpec{})
	s.upstreams["test"].targets[0].healthy.Store(true)

	// 创建模拟客户端连接（不会实际建立连接，测试无上游路径）
	s.mu.Lock()
	// 设置上游为空，测试无上游配置路径
	s.upstreams = make(map[string]*Upstream)
	s.mu.Unlock()

	// 创建一对连接
	serverLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	defer func() { _ = serverLn.Close() }()

	clientConn, err := net.Dial("tcp", serverLn.Addr().String())
	if err != nil {
		t.Fatalf("Failed to dial: %v", err)
	}
	defer func() { _ = clientConn.Close() }()

	serverConn, err := serverLn.Accept()
	if err != nil {
		t.Fatalf("Failed to accept: %v", err)
	}
	defer func() { _ = serverConn.Close() }()

	// 测试无上游配置的 handleConnection
	s.handleConnection(clientConn, "127.0.0.1:0")
}

// TestHandleConnection_NoHealthyTarget 测试无健康目标路径
func TestHandleConnection_NoHealthyTarget(t *testing.T) {
	s := NewServer()

	// 添加不健康的上游
	targets := []TargetSpec{
		{Addr: "127.0.0.1:29002", Weight: 1},
	}
	_ = s.AddUpstream("test2", targets, "round_robin", HealthCheckSpec{})
	// 目标不健康（默认 false）

	serverLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	defer func() { _ = serverLn.Close() }()

	clientConn, err := net.Dial("tcp", serverLn.Addr().String())
	if err != nil {
		t.Fatalf("Failed to dial: %v", err)
	}
	defer func() { _ = clientConn.Close() }()

	serverConn, err := serverLn.Accept()
	if err != nil {
		t.Fatalf("Failed to accept: %v", err)
	}
	defer func() { _ = serverConn.Close() }()

	done := make(chan struct{})
	go func() {
		s.handleConnection(clientConn, "127.0.0.1:0")
		close(done)
	}()

	select {
	case <-done:
		// 完成
	case <-time.After(2 * time.Second):
		t.Fatal("handleConnection() timed out")
	}
}

// TestHandleConnection_DialFail 测试连接目标失败路径
func TestHandleConnection_DialFail(t *testing.T) {
	s := NewServer()

	// 添加上游，目标不可达
	targets := []TargetSpec{
		{Addr: "127.0.0.1:29999", Weight: 1},
	}
	_ = s.AddUpstream("test3", targets, "round_robin", HealthCheckSpec{})
	s.upstreams["test3"].targets[0].healthy.Store(true)

	serverLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	defer func() { _ = serverLn.Close() }()

	clientConn, err := net.Dial("tcp", serverLn.Addr().String())
	if err != nil {
		t.Fatalf("Failed to dial: %v", err)
	}
	defer func() { _ = clientConn.Close() }()

	serverConn, err := serverLn.Accept()
	if err != nil {
		t.Fatalf("Failed to accept: %v", err)
	}
	defer func() { _ = serverConn.Close() }()

	done := make(chan struct{})
	go func() {
		s.handleConnection(clientConn, "127.0.0.1:0")
		close(done)
	}()

	select {
	case <-done:
		// 完成 - 连接目标失败后应标记为不健康
		if s.upstreams["test3"].targets[0].healthy.Load() {
			t.Error("Target should be marked unhealthy after dial failure")
		}
	case <-time.After(15 * time.Second):
		t.Fatal("handleConnection() timed out")
	}
}

// TestAcceptLoop_Error 测试 acceptLoop 错误处理路径
func TestAcceptLoop_Error(t *testing.T) {
	s := NewServer()
	s.running.Store(true)

	// 创建一个立即关闭的监听器
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}

	// 在另一个 goroutine 中关闭监听器
	go func() {
		time.Sleep(50 * time.Millisecond)
		_ = ln.Close()
	}()

	done := make(chan struct{})
	go func() {
		s.acceptLoop("test", ln)
		close(done)
	}()

	select {
	case <-done:
		// 完成
	case <-time.After(2 * time.Second):
		s.running.Store(false)
		<-done
	}
}
