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
		t.Error("Expected non-nil server")
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
		t.Error("Expected non-nil upstream")
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

func TestUDPListener(t *testing.T) {
	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to resolve UDP address: %v", err)
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		t.Fatalf("Failed to listen UDP: %v", err)
	}
	defer conn.Close()

	ul := &udpListener{conn: conn}

	// 测试 Addr
	if ul.Addr() == nil {
		t.Error("Expected non-nil address")
	}

	// 测试 Close
	if err := ul.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}

	// 测试 Accept（应该返回 io.EOF）
	_, err = ul.Accept()
	if err == nil {
		t.Error("Expected error from Accept")
	}
}

func TestConcurrentConnections(t *testing.T) {
	s := NewServer()

	targets := []TargetSpec{
		{Addr: "localhost:8001", Weight: 1},
	}
	s.AddUpstream("test", targets, "round_robin", HealthCheckSpec{})

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
	s.AddUpstream("udp_stop_test", targets, "round_robin", HealthCheckSpec{})

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
	defer conn.Close()

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