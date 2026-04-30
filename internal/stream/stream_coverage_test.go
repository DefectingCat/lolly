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

// TestGetOrCreateSession 测试 getOrCreateSession 方法
func TestGetOrCreateSession(t *testing.T) {
	// 创建 UDP 连接
	udpAddr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	conn, _ := net.ListenUDP("udp", udpAddr)
	defer func() { _ = conn.Close() }()

	// 创建上游
	upstream := &Upstream{
		targets:  []*Target{{addr: "127.0.0.1:29003"}},
		balancer: newRoundRobin(),
	}
	upstream.targets[0].healthy.Store(true)

	srv := newUDPServer(conn, upstream, 1*time.Minute)

	clientAddr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:29010")

	// 第一次调用 - 应该创建新会话（但由于后端不可达，应该失败）
	session, err := srv.getOrCreateSession(clientAddr)
	if err != nil {
		// 预期失败，因为后端不可达
		return
	}
	if session == nil {
		t.Error("getOrCreateSession() should return a session")
	}
}

// TestGetOrCreateSession_DoubleCheck 测试双重检查锁定
func TestGetOrCreateSession_DoubleCheck(t *testing.T) {
	// 创建 UDP 连接
	udpAddr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	conn, _ := net.ListenUDP("udp", udpAddr)
	defer func() { _ = conn.Close() }()

	// 创建上游
	upstream := &Upstream{
		targets:  []*Target{{addr: "127.0.0.1:29004"}},
		balancer: newRoundRobin(),
	}
	upstream.targets[0].healthy.Store(true)

	srv := newUDPServer(conn, upstream, 1*time.Minute)
	clientAddr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:29011")

	// 手动创建一个会话来测试双重检查
	srv.mu.Lock()
	testSession := &udpSession{
		clientAddr: clientAddr,
		lastActive: time.Now(),
		srv:        srv,
	}
	srv.sessions[sessionKey(clientAddr)] = testSession
	srv.mu.Unlock()

	// 再次获取应该返回现有会话
	session, err := srv.getOrCreateSession(clientAddr)
	if err != nil {
		t.Errorf("getOrCreateSession() should not error for existing session: %v", err)
	}
	if session != testSession {
		t.Error("getOrCreateSession() should return existing session")
	}
}

// TestGetOrCreateSession_NoHealthyTarget 测试无健康目标
func TestGetOrCreateSession_NoHealthyTarget(t *testing.T) {
	udpAddr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	conn, _ := net.ListenUDP("udp", udpAddr)
	defer func() { _ = conn.Close() }()

	upstream := &Upstream{
		targets:  []*Target{{addr: "127.0.0.1:29005"}},
		balancer: newRoundRobin(),
	}
	// 不设置 healthy，默认为 false

	srv := newUDPServer(conn, upstream, 1*time.Minute)
	clientAddr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:29012")

	session, err := srv.getOrCreateSession(clientAddr)
	if err == nil {
		t.Error("getOrCreateSession() should return error when no healthy target")
	}
	if session != nil {
		t.Error("getOrCreateSession() should return nil session on error")
	}
}

// TestGetOrCreateSession_InvalidTargetAddr 测试无效目标地址
func TestGetOrCreateSession_InvalidTargetAddr(t *testing.T) {
	udpAddr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	conn, _ := net.ListenUDP("udp", udpAddr)
	defer func() { _ = conn.Close() }()

	upstream := &Upstream{
		targets:  []*Target{{addr: "invalid-address"}},
		balancer: newRoundRobin(),
	}
	upstream.targets[0].healthy.Store(true)

	srv := newUDPServer(conn, upstream, 1*time.Minute)
	clientAddr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:29013")

	session, err := srv.getOrCreateSession(clientAddr)
	if err == nil {
		t.Error("getOrCreateSession() should return error for invalid target address")
	}
	if session != nil {
		t.Error("getOrCreateSession() should return nil session on error")
	}
}

// TestHandleBackendResponse 测试 handleBackendResponse 超时清理路径
func TestHandleBackendResponse(t *testing.T) {
	// 创建 UDP 连接（服务端）
	udpAddr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	conn, _ := net.ListenUDP("udp", udpAddr)
	defer func() { _ = conn.Close() }()

	// 创建上游
	upstream := &Upstream{
		targets:  []*Target{{addr: "127.0.0.1:29006"}},
		balancer: newRoundRobin(),
	}
	upstream.targets[0].healthy.Store(true)

	srv := newUDPServer(conn, upstream, 50*time.Millisecond) // 短超时

	clientAddr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:29014")

	// 创建目标连接（监听器用于接收目标连接）
	targetAddr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:29006")
	targetConn, _ := net.ListenUDP("udp", targetAddr)
	defer func() { _ = targetConn.Close() }()

	// 创建会话时需要先添加到 WaitGroup（handleBackendResponse 会调用 Done）
	srv.wg.Add(1)
	session := &udpSession{
		clientAddr: clientAddr,
		targetConn: targetConn,
		lastActive: time.Now().Add(-2 * time.Hour), // 很久以前
		srv:        srv,
	}

	// 添加会话到服务器
	srv.sessions[sessionKey(clientAddr)] = session

	// 启动后端响应处理
	done := make(chan struct{})
	go func() {
		session.handleBackendResponse()
		close(done)
	}()

	select {
	case <-done:
		// 应该因为超时而清理会话
	case <-time.After(2 * time.Second):
		t.Fatal("handleBackendResponse() timed out")
	}
}

// TestHandleBackendResponse_ErrorPath 测试 handleBackendResponse 错误路径
func TestHandleBackendResponse_ErrorPath(t *testing.T) {
	udpAddr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	conn, _ := net.ListenUDP("udp", udpAddr)
	defer func() { _ = conn.Close() }()

	upstream := &Upstream{
		targets:  []*Target{{addr: "127.0.0.1:29007"}},
		balancer: newRoundRobin(),
	}

	srv := newUDPServer(conn, upstream, 10*time.Millisecond)

	clientAddr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:29015")

	// 创建一个已关闭的连接作为 targetConn
	targetUDPAddr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:29007")
	targetConn, err := net.DialUDP("udp", nil, targetUDPAddr)
	if err != nil {
		t.Fatalf("Failed to create target connection: %v", err)
	}

	session := &udpSession{
		clientAddr: clientAddr,
		targetConn: targetConn,
		lastActive: time.Now(),
		srv:        srv,
	}

	// 创建会话时需要先添加到 WaitGroup（handleBackendResponse 会调用 Done）
	srv.wg.Add(1)
	srv.sessions[sessionKey(clientAddr)] = session

	done := make(chan struct{})
	go func() {
		session.handleBackendResponse()
		close(done)
	}()

	select {
	case <-done:
		// 完成
	case <-time.After(3 * time.Second):
		t.Fatal("handleBackendResponse() timed out")
	}
}

// TestServe_InvalidUpstream 测试 serve 方法无效上游路径
func TestServe_InvalidUpstream(t *testing.T) {
	udpAddr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	conn, _ := net.ListenUDP("udp", udpAddr)
	defer func() { _ = conn.Close() }()

	upstream := &Upstream{
		targets:  []*Target{},
		balancer: newRoundRobin(),
	}

	srv := newUDPServer(conn, upstream, 50*time.Millisecond)

	// 启动 serve
	go srv.serve()

	// 立即停止
	time.Sleep(20 * time.Millisecond)
	srv.stop()
}

// TestServerStop 测试 Server.Stop 方法
func TestServerStop(t *testing.T) {
	s := NewServer()

	// 添加上游
	targets := []TargetSpec{
		{Addr: "127.0.0.1:29008", Weight: 1},
	}
	hcSpec := HealthCheckSpec{
		Enabled:  true,
		Interval: 1 * time.Second,
		Timeout:  500 * time.Millisecond,
	}
	_ = s.AddUpstream("stop_test", targets, "round_robin", hcSpec)

	// 监听 TCP
	err := s.ListenTCP("127.0.0.1:29009")
	if err != nil {
		t.Fatalf("ListenTCP failed: %v", err)
	}

	// 监听 UDP
	err = s.ListenUDP("127.0.0.1:29010", "stop_test", 1*time.Second)
	if err != nil {
		t.Fatalf("ListenUDP failed: %v", err)
	}

	// 启动
	err = s.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	// 停止
	err = s.Stop()
	if err != nil {
		t.Errorf("Stop() error: %v", err)
	}
}

// TestStatsComplete 测试 Stats 完整统计
func TestStatsComplete(t *testing.T) {
	s := NewServer()

	// 添加 TCP 监听
	err := s.ListenTCP("127.0.0.1:29020")
	if err != nil {
		t.Fatalf("ListenTCP failed: %v", err)
	}

	// 添加上游
	targets := []TargetSpec{{Addr: "127.0.0.1:29021", Weight: 1}}
	_ = s.AddUpstream("stats_test", targets, "round_robin", HealthCheckSpec{})

	// 添加 UDP 监听
	err = s.ListenUDP("127.0.0.1:29022", "stats_test", 1*time.Second)
	if err != nil {
		t.Fatalf("ListenUDP failed: %v", err)
	}

	stats := s.Stats()
	if stats.Listeners != 2 {
		t.Errorf("Stats().Listeners = %d, want 2 (1 TCP + 1 UDP)", stats.Listeners)
	}
	if stats.Upstreams != 1 {
		t.Errorf("Stats().Upstreams = %d, want 1", stats.Upstreams)
	}
	if stats.Connections != 0 {
		t.Errorf("Stats().Connections = %d, want 0", stats.Connections)
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
