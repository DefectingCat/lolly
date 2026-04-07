// Package stream 提供 TCP/UDP 流代理功能的测试。
//
// 该文件测试流代理模块的各项功能，包括：
//   - 服务器创建和初始化
//   - 上游配置和负载均衡
//   - TCP 和 UDP 监听
//   - 健康检查
//   - 连接统计
//
// 作者：xfy
package stream

import (
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewServer(t *testing.T) {
	s := NewServer()
	if s == nil {
		t.Fatal("Expected non-nil server")
	}
	if s.listeners == nil {
		t.Error("Expected initialized listeners map")
	}
	if s.upstreams == nil {
		t.Error("Expected initialized upstreams map")
	}
}

func TestAddUpstream(t *testing.T) {
	s := NewServer()

	targets := []TargetSpec{
		{Addr: "localhost:8001", Weight: 1},
		{Addr: "localhost:8002", Weight: 2},
	}

	hcSpec := HealthCheckSpec{
		Enabled:  false,
		Interval: 10 * time.Second,
		Timeout:  5 * time.Second,
	}

	err := s.AddUpstream("test", targets, "round_robin", hcSpec)
	if err != nil {
		t.Errorf("AddUpstream failed: %v", err)
	}

	if len(s.upstreams) != 1 {
		t.Errorf("Expected 1 upstream, got %d", len(s.upstreams))
	}

	up := s.upstreams["test"]
	if up == nil {
		t.Fatal("Expected non-nil upstream")
	}
	if len(up.targets) != 2 {
		t.Errorf("Expected 2 targets, got %d", len(up.targets))
	}
}

func TestRoundRobinBalancer(t *testing.T) {
	targets := []*Target{
		{addr: "localhost:8001"},
		{addr: "localhost:8002"},
		{addr: "localhost:8003"},
	}
	for _, target := range targets {
		target.healthy.Store(true)
	}

	rr := newRoundRobin()

	// 测试轮询
	results := make(map[string]int)
	for i := 0; i < 6; i++ {
		selected := rr.Select(targets)
		if selected == nil {
			t.Error("Expected non-nil target")
			continue
		}
		results[selected.addr]++
	}

	// 每个目标应该被选中 2 次
	for _, target := range targets {
		if results[target.addr] != 2 {
			t.Errorf("Expected %s to be selected 2 times, got %d", target.addr, results[target.addr])
		}
	}
}

func TestLeastConnBalancer(t *testing.T) {
	targets := []*Target{
		{addr: "localhost:8001", conns: 5},
		{addr: "localhost:8002", conns: 2},
		{addr: "localhost:8003", conns: 8},
	}
	for _, t := range targets {
		t.healthy.Store(true)
	}

	lc := newLeastConn()
	selected := lc.Select(targets)

	if selected == nil {
		t.Error("Expected non-nil target")
	} else if selected.addr != "localhost:8002" {
		t.Errorf("Expected localhost:8002 (least connections), got %s", selected.addr)
	}
}

func TestBalancerNoHealthyTargets(t *testing.T) {
	targets := []*Target{
		{addr: "localhost:8001"},
		{addr: "localhost:8002"},
	}
	// 不设置 healthy，默认为 false

	rr := newRoundRobin()
	selected := rr.Select(targets)
	if selected != nil {
		t.Error("Expected nil for no healthy targets")
	}

	lc := newLeastConn()
	selected = lc.Select(targets)
	if selected != nil {
		t.Error("Expected nil for no healthy targets")
	}
}

func TestWeightedRoundRobinBalancer(t *testing.T) {
	targets := []*Target{
		{addr: "localhost:8001", weight: 3},
		{addr: "localhost:8002", weight: 1},
	}
	for _, target := range targets {
		target.healthy.Store(true)
	}

	wrr := newWeightedRoundRobin()

	// 测试加权分布：3:1 比例
	results := make(map[string]int)
	for i := 0; i < 8; i++ {
		selected := wrr.Select(targets)
		if selected == nil {
			t.Error("Expected non-nil target")
			continue
		}
		results[selected.addr]++
	}

	// localhost:8001 应被选中 6 次，localhost:8002 应被选中 2 次
	if results["localhost:8001"] != 6 {
		t.Errorf("Expected localhost:8001 to be selected 6 times, got %d", results["localhost:8001"])
	}
	if results["localhost:8002"] != 2 {
		t.Errorf("Expected localhost:8002 to be selected 2 times, got %d", results["localhost:8002"])
	}
}

func TestIPHashBalancer(t *testing.T) {
	targets := []*Target{
		{addr: "localhost:8001"},
		{addr: "localhost:8002"},
		{addr: "localhost:8003"},
	}
	for _, target := range targets {
		target.healthy.Store(true)
	}

	ih := newIPHash()

	// 相同 IP 应始终选择同一目标
	ip1 := "192.168.1.1"
	selected1 := ih.(*ipHash).SelectByIP(targets, ip1)
	selected2 := ih.(*ipHash).SelectByIP(targets, ip1)

	if selected1 != selected2 {
		t.Error("Same IP should select same target")
	}

	// 不同 IP 可能选择不同目标
	ip2 := "10.0.0.1"
	selected3 := ih.(*ipHash).SelectByIP(targets, ip2)
	// 验证返回非空
	if selected3 == nil {
		t.Error("Expected non-nil target for different IP")
	}
}

func TestServerStats(t *testing.T) {
	s := NewServer()

	stats := s.Stats()
	if stats.Connections != 0 {
		t.Errorf("Expected 0 connections, got %d", stats.Connections)
	}
	if stats.Listeners != 0 {
		t.Errorf("Expected 0 listeners, got %d", stats.Listeners)
	}
}

func TestUpstreamSelect(t *testing.T) {
	u := &Upstream{
		targets: []*Target{
			{addr: "localhost:8001"},
			{addr: "localhost:8002"},
		},
		balancer: newRoundRobin(),
	}
	for _, t := range u.targets {
		t.healthy.Store(true)
	}

	selected := u.Select()
	if selected == nil {
		t.Error("Expected non-nil target")
	}
}

func TestTargetHealthy(t *testing.T) {
	target := &Target{
		addr:   "localhost:8001",
		weight: 1,
	}

	// 初始状态应该是不健康（默认为 false）
	if target.healthy.Load() {
		t.Error("新目标应该默认为不健康")
	}

	// 设置为健康
	target.healthy.Store(true)
	if !target.healthy.Load() {
		t.Error("目标应该被标记为健康")
	}

	// 设置为不健康
	target.healthy.Store(false)
	if target.healthy.Load() {
		t.Error("目标应该被标记为不健康")
	}
}

func TestHealthChecker(t *testing.T) {
	u := &Upstream{
		targets: []*Target{
			{addr: "localhost:99999"}, // 不存在的端口
		},
	}

	hc := &HealthChecker{
		upstream: u,
		interval: 1 * time.Second,
		timeout:  100 * time.Millisecond,
		stopCh:   make(chan struct{}),
	}

	// 执行一次检查
	hc.check()

	// 目标应该被标记为不健康
	if u.targets[0].healthy.Load() {
		t.Error("Expected target to be marked unhealthy")
	}
}

func TestHealthCheckerStartStop(t *testing.T) {
	u := &Upstream{
		targets: []*Target{
			{addr: "localhost:99998"}, // 不存在的端口
		},
	}

	hc := &HealthChecker{
		upstream: u,
		interval: 100 * time.Millisecond,
		timeout:  50 * time.Millisecond,
		stopCh:   make(chan struct{}),
	}

	// 启动健康检查
	go hc.Start()

	// 等待几次检查执行
	time.Sleep(250 * time.Millisecond)

	// 停止健康检查
	hc.Stop()
}

func TestConcurrentConnections(t *testing.T) {
	s := NewServer()

	targets := []TargetSpec{
		{Addr: "localhost:8001", Weight: 1},
	}
	_ = s.AddUpstream("test", targets, "round_robin", HealthCheckSpec{})

	// 并发增加连接数
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			atomic.AddInt64(&s.connCount, 1)
		}()
	}
	wg.Wait()

	if s.connCount != 100 {
		t.Errorf("Expected 100 connections, got %d", s.connCount)
	}
}

func TestUDPServer(t *testing.T) {
	s := NewServer()

	// 添加 UDP 上游配置
	targets := []TargetSpec{
		{Addr: "127.0.0.1:0", Weight: 1},
	}
	err := s.AddUpstream("udp_test", targets, "round_robin", HealthCheckSpec{})
	if err != nil {
		t.Fatalf("AddUpstream failed: %v", err)
	}

	// 测试 UDP 监听（使用 :0 让系统分配端口）
	err = s.ListenUDP("127.0.0.1:0", "udp_test", 1*time.Second)
	if err != nil {
		t.Fatalf("ListenUDP failed: %v", err)
	}

	// 验证 UDP 服务器已创建
	s.mu.RLock()
	if len(s.udpServers) != 1 {
		t.Errorf("Expected 1 UDP server, got %d", len(s.udpServers))
	}
	s.mu.RUnlock()

	// 测试 Stats 包含 UDP 监听器
	stats := s.Stats()
	if stats.Listeners != 1 {
		t.Errorf("Expected 1 listener in stats, got %d", stats.Listeners)
	}
}

func TestUDPServerInvalidUpstream(t *testing.T) {
	s := NewServer()

	// 尝试监听不存在的上游配置
	err := s.ListenUDP("127.0.0.1:0", "non_existent", 0)
	if err == nil {
		t.Error("Expected error for non-existent upstream")
	}
}

func TestUDPServerStartAndStop(t *testing.T) {
	s := NewServer()

	// 添加上游
	targets := []TargetSpec{
		{Addr: "127.0.0.1:19001", Weight: 1},
	}
	_ = s.AddUpstream("udp_stop_test", targets, "round_robin", HealthCheckSpec{})

	// 监听 UDP
	err := s.ListenUDP("127.0.0.1:19000", "udp_stop_test", 500*time.Millisecond)
	if err != nil {
		t.Fatalf("ListenUDP failed: %v", err)
	}

	// 启动服务器
	err = s.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// 给服务器一点时间启动
	time.Sleep(50 * time.Millisecond)

	// 停止服务器
	err = s.Stop()
	if err != nil {
		t.Errorf("Stop failed: %v", err)
	}
}

func TestUDPSessionKey(t *testing.T) {
	addr1, _ := net.ResolveUDPAddr("udp", "127.0.0.1:1234")
	addr2, _ := net.ResolveUDPAddr("udp", "127.0.0.1:5678")
	addr3, _ := net.ResolveUDPAddr("udp", "127.0.0.1:1234")

	key1 := sessionKey(addr1)
	key2 := sessionKey(addr2)
	key3 := sessionKey(addr3)

	if key1 == key2 {
		t.Error("Different addresses should have different keys")
	}

	if key1 != key3 {
		t.Error("Same addresses should have same keys")
	}
}

func TestNewUDPServer(t *testing.T) {
	// 创建 UDP 连接
	udpAddr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	conn, _ := net.ListenUDP("udp", udpAddr)
	defer func() { _ = conn.Close() }()

	// 创建上游
	upstream := &Upstream{
		targets:  []*Target{{addr: "127.0.0.1:19002"}},
		balancer: newRoundRobin(),
	}

	// 测试默认超时
	srv := newUDPServer(conn, upstream, 0)
	if srv.timeout != 60*time.Second {
		t.Errorf("Expected default timeout 60s, got %v", srv.timeout)
	}

	// 测试自定义超时
	srv2 := newUDPServer(conn, upstream, 30*time.Second)
	if srv2.timeout != 30*time.Second {
		t.Errorf("Expected timeout 30s, got %v", srv2.timeout)
	}
}

func TestListenTCP(t *testing.T) {
	s := NewServer()

	// 使用 :0 让系统分配端口
	err := s.ListenTCP("127.0.0.1:0")
	if err != nil {
		t.Fatalf("ListenTCP failed: %v", err)
	}

	// 验证监听器已创建
	s.mu.RLock()
	if len(s.listeners) != 1 {
		t.Errorf("Expected 1 listener, got %d", len(s.listeners))
	}
	s.mu.RUnlock()

	// 验证 Stats
	stats := s.Stats()
	if stats.Listeners != 1 {
		t.Errorf("Expected 1 listener in stats, got %d", stats.Listeners)
	}
}

func TestServerStartStopWithTCP(t *testing.T) {
	s := NewServer()

	// 添加上游
	targets := []TargetSpec{
		{Addr: "127.0.0.1:19003", Weight: 1},
	}
	_ = s.AddUpstream("tcp_test", targets, "round_robin", HealthCheckSpec{})

	// 监听 TCP
	err := s.ListenTCP("127.0.0.1:19004")
	if err != nil {
		t.Fatalf("ListenTCP failed: %v", err)
	}

	// 启动服务器
	err = s.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// 给服务器一点时间启动
	time.Sleep(50 * time.Millisecond)

	// 停止服务器
	err = s.Stop()
	if err != nil {
		t.Errorf("Stop failed: %v", err)
	}
}

func TestRoundRobinBalancerWithSingleTarget(t *testing.T) {
	rb := newRoundRobin()
	targets := []*Target{
		{addr: "backend1:8080"},
	}
	targets[0].healthy.Store(true)

	// 测试单个健康目标
	for i := 0; i < 5; i++ {
		target := rb.Select(targets)
		if target == nil {
			t.Error("Expected non-nil target")
			continue
		}
		if target.addr != "backend1:8080" {
			t.Errorf("Expected backend1:8080, got %s", target.addr)
		}
	}
}

func TestLeastConnBalancerWithTie(t *testing.T) {
	lc := newLeastConn()
	targets := []*Target{
		{addr: "backend1:8080", conns: 5},
		{addr: "backend2:8080", conns: 5},
		{addr: "backend3:8080", conns: 5},
	}
	for _, t := range targets {
		t.healthy.Store(true)
	}

	// 当连接数相同时，应该选择第一个
	selected := lc.Select(targets)
	if selected == nil {
		t.Error("Expected non-nil target")
	}
}

func TestAddUpstreamWithLeastConn(t *testing.T) {
	s := NewServer()

	targets := []TargetSpec{
		{Addr: "localhost:8001", Weight: 1},
		{Addr: "localhost:8002", Weight: 2},
	}

	err := s.AddUpstream("least_conn_test", targets, "least_conn", HealthCheckSpec{})
	if err != nil {
		t.Errorf("AddUpstream failed: %v", err)
	}

	up := s.upstreams["least_conn_test"]
	if up == nil {
		t.Fatal("Expected non-nil upstream")
	}

	// 验证使用的是最少连接均衡器
	_, ok := up.balancer.(*leastConn)
	if !ok {
		t.Error("Expected leastConn balancer")
	}
}

func TestAddUpstreamWithHealthCheck(t *testing.T) {
	s := NewServer()

	targets := []TargetSpec{
		{Addr: "localhost:8001", Weight: 1},
	}

	hcSpec := HealthCheckSpec{
		Enabled:  true,
		Interval: 1 * time.Second,
		Timeout:  500 * time.Millisecond,
	}

	err := s.AddUpstream("hc_test", targets, "round_robin", hcSpec)
	if err != nil {
		t.Errorf("AddUpstream failed: %v", err)
	}

	up := s.upstreams["hc_test"]
	if up == nil {
		t.Fatal("Expected non-nil upstream")
	}

	if up.healthChk == nil {
		t.Error("Expected health checker to be initialized")
	}

	// 停止健康检查
	if up.healthChk != nil {
		up.healthChk.Stop()
	}
}

func TestUpstreamSelectNoHealthy(t *testing.T) {
	u := &Upstream{
		targets: []*Target{
			{addr: "localhost:8001"},
			{addr: "localhost:8002"},
		},
		balancer: newRoundRobin(),
	}
	// 不设置 healthy，默认为 false

	selected := u.Select()
	if selected != nil {
		t.Error("Expected nil for no healthy targets")
	}
}

func TestCleanupExpiredSessions(t *testing.T) {
	// 创建 UDP 连接
	udpAddr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	conn, _ := net.ListenUDP("udp", udpAddr)
	defer func() { _ = conn.Close() }()

	// 创建上游
	upstream := &Upstream{
		targets:  []*Target{{addr: "127.0.0.1:19005"}},
		balancer: newRoundRobin(),
	}
	upstream.targets[0].healthy.Store(true)

	// 创建 UDP 服务器，设置很短的超时时间
	srv := newUDPServer(conn, upstream, 1*time.Millisecond)

	// 创建模拟会话
	clientAddr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:12345")
	session := &udpSession{
		clientAddr: clientAddr,
		lastActive: time.Now().Add(-1 * time.Hour), // 很久以前的活动
	}
	srv.sessions[sessionKey(clientAddr)] = session

	// 执行清理
	srv.cleanupExpiredSessions()

	// 验证会话已被清理
	srv.mu.RLock()
	if len(srv.sessions) != 0 {
		t.Errorf("Expected 0 sessions after cleanup, got %d", len(srv.sessions))
	}
	srv.mu.RUnlock()
}

func TestUDPServerStop(t *testing.T) {
	// 创建 UDP 连接
	udpAddr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	conn, _ := net.ListenUDP("udp", udpAddr)

	// 创建上游
	upstream := &Upstream{
		targets:  []*Target{{addr: "127.0.0.1:19006"}},
		balancer: newRoundRobin(),
	}

	// 创建 UDP 服务器
	srv := newUDPServer(conn, upstream, 1*time.Second)

	// 添加一个模拟会话
	clientAddr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:12346")
	session := &udpSession{
		clientAddr: clientAddr,
		lastActive: time.Now(),
	}
	srv.sessions[sessionKey(clientAddr)] = session

	// 停止服务器
	srv.stop()

	// 验证会话已被清理
	srv.mu.RLock()
	if len(srv.sessions) != 0 {
		t.Errorf("Expected 0 sessions after stop, got %d", len(srv.sessions))
	}
	srv.mu.RUnlock()
}

func TestUDPSessionOperations(t *testing.T) {
	// 创建 UDP 连接
	udpAddr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	conn, _ := net.ListenUDP("udp", udpAddr)
	defer func() { _ = conn.Close() }()

	// 创建目标服务器（用于模拟连接）
	targetAddr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:19007")
	targetConn, _ := net.ListenUDP("udp", targetAddr)
	defer func() { _ = targetConn.Close() }()

	// 创建上游
	upstream := &Upstream{
		targets:  []*Target{{addr: "127.0.0.1:19007"}},
		balancer: newRoundRobin(),
	}
	upstream.targets[0].healthy.Store(true)

	// 创建 UDP 服务器
	srv := newUDPServer(conn, upstream, 1*time.Minute)

	// 测试 getSession - 不存在的会话
	clientAddr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:12347")
	session := srv.getSession(clientAddr)
	if session != nil {
		t.Error("Expected nil for non-existent session")
	}

	// 创建模拟会话
	testSession := &udpSession{
		clientAddr: clientAddr,
		lastActive: time.Now(),
		srv:        srv,
	}
	srv.sessions[sessionKey(clientAddr)] = testSession

	// 测试 getSession - 存在的会话
	session = srv.getSession(clientAddr)
	if session == nil {
		t.Error("Expected non-nil session")
	}

	// 测试 removeSession
	srv.removeSession(clientAddr)
	session = srv.getSession(clientAddr)
	if session != nil {
		t.Error("Expected nil after removeSession")
	}
}

func TestUDPSessionClose(t *testing.T) {
	// 创建两个 UDP 连接用于测试
	udpAddr1, _ := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	conn1, _ := net.ListenUDP("udp", udpAddr1)

	udpAddr2, _ := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	conn2, _ := net.ListenUDP("udp", udpAddr2)

	// 创建会话
	session := &udpSession{
		clientAddr: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12348},
		targetConn: conn2,
		lastActive: time.Now(),
	}

	// 测试 close - 应该能正常关闭
	session.close()

	// 第二次调用 close 不应该出错（使用 sync.Once）
	session.close()

	_ = conn1.Close()
}

func TestHealthCheckerCheckWithHealthyTarget(t *testing.T) {
	// 启动一个临时的 TCP 服务器
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	defer func() { _ = listener.Close() }()

	// 在后台运行服务器
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()

	addr := listener.Addr().String()

	u := &Upstream{
		targets: []*Target{
			{addr: addr},
		},
	}
	// 初始设置为不健康
	u.targets[0].healthy.Store(false)

	hc := &HealthChecker{
		upstream: u,
		interval: 1 * time.Second,
		timeout:  100 * time.Millisecond,
		stopCh:   make(chan struct{}),
	}

	// 执行健康检查
	hc.check()

	// 目标应该被标记为健康（因为服务器在运行）
	if !u.targets[0].healthy.Load() {
		t.Error("Expected target to be marked healthy")
	}
}
