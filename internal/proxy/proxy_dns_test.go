// Package proxy 提供 DNS 代理功能的测试。
//
// 该文件测试 proxy_dns.go 中的 DNS 相关功能，包括：
//   - DNS 解析器设置和获取
//   - DNS 缓存刷新机制
//   - HostClient 地址更新
//   - 错误处理场景
//
// 作者：xfy
package proxy

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"rua.plus/lolly/internal/config"
	"rua.plus/lolly/internal/loadbalance"
	"rua.plus/lolly/internal/resolver"
)

// mockResolver 实现 resolver.Resolver 接口的模拟解析器。
type mockResolver struct {
	mu                   sync.RWMutex
	lookupResults        map[string][]string
	lookupError          error
	startErr             error
	stopErr              error
	lookupHostCalls      int
	lookupWithCacheCalls int
	startCalls           int
	stopCalls            int
}

func (m *mockResolver) LookupHost(_ context.Context, host string) ([]string, error) {
	m.mu.Lock()
	m.lookupHostCalls++
	m.mu.Unlock()
	return m.resolve(host)
}

func (m *mockResolver) LookupHostWithCache(_ context.Context, host string) ([]string, error) {
	m.mu.Lock()
	m.lookupWithCacheCalls++
	m.mu.Unlock()
	return m.resolve(host)
}

func (m *mockResolver) resolve(host string) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.lookupError != nil {
		return nil, m.lookupError
	}
	if ips, ok := m.lookupResults[host]; ok {
		return ips, nil
	}
	return nil, errors.New("host not found in mock resolver")
}

func (m *mockResolver) Refresh(_ string) error {
	return nil
}

func (m *mockResolver) Start() error {
	m.mu.Lock()
	m.startCalls++
	m.mu.Unlock()
	return m.startErr
}

func (m *mockResolver) Stop() error {
	m.mu.Lock()
	m.stopCalls++
	m.mu.Unlock()
	return m.stopErr
}

func (m *mockResolver) Stats() resolver.Stats {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return resolver.Stats{
		CacheHits:   int64(m.lookupWithCacheCalls),
		CacheMisses: int64(m.lookupHostCalls),
	}
}

func (m *mockResolver) getLookupWithCacheCalls() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.lookupWithCacheCalls
}

// TestSetResolver 测试设置 DNS 解析器。
func TestSetResolver(t *testing.T) {
	cfg := &config.ProxyConfig{
		Path:        "/api",
		LoadBalance: "round_robin",
		Timeout:     config.ProxyTimeout{Connect: 5 * time.Second},
	}
	targets := []*loadbalance.Target{
		{URL: "http://localhost:8080"},
	}

	p, err := NewProxy(cfg, targets, nil, nil)
	if err != nil {
		t.Fatalf("NewProxy() error: %v", err)
	}

	// 初始状态 resolver 为 nil
	if p.resolver != nil {
		t.Error("resolver should be nil initially")
	}

	// 设置解析器
	mr := &mockResolver{}
	p.SetResolver(mr)

	if p.resolver != mr {
		t.Error("SetResolver() did not set resolver correctly")
	}
}

// TestGetResolverStats_NoResolver 测试没有解析器时返回空统计。
func TestGetResolverStats_NoResolver(t *testing.T) {
	cfg := &config.ProxyConfig{
		Path:        "/api",
		LoadBalance: "round_robin",
		Timeout:     config.ProxyTimeout{Connect: 5 * time.Second},
	}
	targets := []*loadbalance.Target{
		{URL: "http://localhost:8080"},
	}

	p, err := NewProxy(cfg, targets, nil, nil)
	if err != nil {
		t.Fatalf("NewProxy() error: %v", err)
	}

	stats := p.GetResolverStats()
	if stats.CacheHits != 0 || stats.CacheMisses != 0 {
		t.Errorf("expected empty stats, got %+v", stats)
	}
}

// TestGetResolverStats_WithResolver 测试有解析器时返回统计。
func TestGetResolverStats_WithResolver(t *testing.T) {
	cfg := &config.ProxyConfig{
		Path:        "/api",
		LoadBalance: "round_robin",
		Timeout:     config.ProxyTimeout{Connect: 5 * time.Second},
	}
	targets := []*loadbalance.Target{
		{URL: "http://localhost:8080"},
	}

	p, err := NewProxy(cfg, targets, nil, nil)
	if err != nil {
		t.Fatalf("NewProxy() error: %v", err)
	}

	mr := &mockResolver{}
	p.SetResolver(mr)

	// 调用几次以产生统计
	_, _ = mr.LookupHostWithCache(context.Background(), "example.com")
	_, _ = mr.LookupHostWithCache(context.Background(), "example.com")

	stats := p.GetResolverStats()
	if stats.CacheHits != 2 {
		t.Errorf("expected 2 cache hits, got %d", stats.CacheHits)
	}
}

// TestStartWithResolver 测试启动代理时解析器正确启动。
func TestStartWithResolver(t *testing.T) {
	cfg := &config.ProxyConfig{
		Path:        "/api",
		LoadBalance: "round_robin",
		Timeout:     config.ProxyTimeout{Connect: 5 * time.Second},
	}
	targets := []*loadbalance.Target{
		{URL: "http://localhost:8080"},
	}

	p, err := NewProxy(cfg, targets, nil, nil)
	if err != nil {
		t.Fatalf("NewProxy() error: %v", err)
	}

	mr := &mockResolver{}
	p.SetResolver(mr)

	err = p.Start()
	if err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	// 等待 goroutine 调度
	time.Sleep(10 * time.Millisecond)

	if mr.startCalls != 1 {
		t.Errorf("expected resolver Start called once, got %d", mr.startCalls)
	}

	// 清理
	_ = p.Stop()
}

// TestStartResolverFails 测试解析器启动失败时代理返回错误。
func TestStartResolverFails(t *testing.T) {
	cfg := &config.ProxyConfig{
		Path:        "/api",
		LoadBalance: "round_robin",
		Timeout:     config.ProxyTimeout{Connect: 5 * time.Second},
	}
	targets := []*loadbalance.Target{
		{URL: "http://localhost:8080"},
	}

	p, err := NewProxy(cfg, targets, nil, nil)
	if err != nil {
		t.Fatalf("NewProxy() error: %v", err)
	}

	mr := &mockResolver{startErr: errors.New("resolver start failed")}
	p.SetResolver(mr)

	err = p.Start()
	if err == nil {
		t.Fatal("Start() should return error when resolver fails")
	}

	if !contains(err.Error(), "failed to start resolver") {
		t.Errorf("expected error containing 'failed to start resolver', got %v", err)
	}
}

// TestStartIdempotent 测试 Start 是幂等的。
func TestStartIdempotent(t *testing.T) {
	cfg := &config.ProxyConfig{
		Path:        "/api",
		LoadBalance: "round_robin",
		Timeout:     config.ProxyTimeout{Connect: 5 * time.Second},
	}
	targets := []*loadbalance.Target{
		{URL: "http://localhost:8080"},
	}

	p, err := NewProxy(cfg, targets, nil, nil)
	if err != nil {
		t.Fatalf("NewProxy() error: %v", err)
	}

	mr := &mockResolver{}
	p.SetResolver(mr)

	// 第一次启动
	if err := p.Start(); err != nil {
		t.Fatalf("first Start() error: %v", err)
	}

	// 第二次启动不应重复调用 resolver.Start
	if err := p.Start(); err != nil {
		t.Fatalf("second Start() error: %v", err)
	}

	time.Sleep(10 * time.Millisecond)

	if mr.startCalls != 1 {
		t.Errorf("resolver.Start should only be called once, got %d", mr.startCalls)
	}

	_ = p.Stop()
}

// TestStopIdempotent 测试 Stop 是幂等的。
func TestStopIdempotent(t *testing.T) {
	cfg := &config.ProxyConfig{
		Path:        "/api",
		LoadBalance: "round_robin",
		Timeout:     config.ProxyTimeout{Connect: 5 * time.Second},
	}
	targets := []*loadbalance.Target{
		{URL: "http://localhost:8080"},
	}

	p, err := NewProxy(cfg, targets, nil, nil)
	if err != nil {
		t.Fatalf("NewProxy() error: %v", err)
	}

	mr := &mockResolver{}
	p.SetResolver(mr)

	if err := p.Start(); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	// 第一次停止
	if err := p.Stop(); err != nil {
		t.Fatalf("first Stop() error: %v", err)
	}

	// 第二次停止不应报错
	if err := p.Stop(); err != nil {
		t.Errorf("second Stop() should not error: %v", err)
	}

	if mr.stopCalls < 1 {
		t.Errorf("expected resolver Stop called at least once, got %d", mr.stopCalls)
	}
}

// TestStopWithoutResolver 测试没有解析器时停止代理。
func TestStopWithoutResolver(t *testing.T) {
	cfg := &config.ProxyConfig{
		Path:        "/api",
		LoadBalance: "round_robin",
		Timeout:     config.ProxyTimeout{Connect: 5 * time.Second},
	}
	targets := []*loadbalance.Target{
		{URL: "http://localhost:8080"},
	}

	p, err := NewProxy(cfg, targets, nil, nil)
	if err != nil {
		t.Fatalf("NewProxy() error: %v", err)
	}

	if err := p.Start(); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	// 没有解析器，停止不应报错
	if err := p.Stop(); err != nil {
		t.Errorf("Stop() without resolver should not error: %v", err)
	}
}

// TestRefreshDNS_Success 测试 DNS 刷新成功场景。
func TestRefreshDNS_Success(t *testing.T) {
	cfg := &config.ProxyConfig{
		Path:        "/api",
		LoadBalance: "round_robin",
		Timeout:     config.ProxyTimeout{Connect: 5 * time.Second},
	}
	targets := []*loadbalance.Target{
		{URL: "http://api.example.com:8080"},
	}

	p, err := NewProxy(cfg, targets, nil, nil)
	if err != nil {
		t.Fatalf("NewProxy() error: %v", err)
	}

	mr := &mockResolver{
		lookupResults: map[string][]string{
			"api.example.com": {"10.0.0.1", "10.0.0.2"},
		},
	}
	p.SetResolver(mr)

	// 确保目标需要解析（从未解析过）
	if !targets[0].NeedsResolve(30 * time.Second) {
		t.Fatal("target should need resolve initially")
	}

	// 执行刷新
	p.refreshDNS()

	// 验证解析器被调用
	if mr.getLookupWithCacheCalls() != 1 {
		t.Errorf("expected 1 lookup call, got %d", mr.getLookupWithCacheCalls())
	}

	// 验证目标 IP 已更新
	ips := targets[0].ResolvedIPs()
	if len(ips) != 2 || ips[0] != "10.0.0.1" || ips[1] != "10.0.0.2" {
		t.Errorf("expected IPs [10.0.0.1, 10.0.0.2], got %v", ips)
	}

	// 验证 HostClient 地址已更新
	p.mu.RLock()
	client := p.clients["http://api.example.com:8080"]
	p.mu.RUnlock()

	if client == nil {
		t.Fatal("HostClient not found")
	}
	expectedAddr := "10.0.0.1:8080"
	if client.Addr != expectedAddr {
		t.Errorf("HostClient.Addr = %q, want %q", client.Addr, expectedAddr)
	}
}

// TestRefreshDNS_LookupError 测试 DNS 刷新时查找失败场景。
func TestRefreshDNS_LookupError(t *testing.T) {
	cfg := &config.ProxyConfig{
		Path:        "/api",
		LoadBalance: "round_robin",
		Timeout:     config.ProxyTimeout{Connect: 5 * time.Second},
	}
	targets := []*loadbalance.Target{
		{URL: "http://api.example.com:8080"},
	}

	p, err := NewProxy(cfg, targets, nil, nil)
	if err != nil {
		t.Fatalf("NewProxy() error: %v", err)
	}

	mr := &mockResolver{
		lookupError: errors.New("DNS lookup failed"),
	}
	p.SetResolver(mr)

	// 执行刷新 - 不应 panic
	p.refreshDNS()

	// 解析器被调用
	if mr.getLookupWithCacheCalls() != 1 {
		t.Errorf("expected 1 lookup call, got %d", mr.getLookupWithCacheCalls())
	}

	// 目标 IP 不应被更新（仍为 nil）
	ips := targets[0].ResolvedIPs()
	if ips != nil {
		t.Errorf("expected nil IPs after failed lookup, got %v", ips)
	}
}

// TestRefreshDNS_NoResolver 测试没有解析器时刷新不执行任何操作。
func TestRefreshDNS_NoResolver(t *testing.T) {
	cfg := &config.ProxyConfig{
		Path:        "/api",
		LoadBalance: "round_robin",
		Timeout:     config.ProxyTimeout{Connect: 5 * time.Second},
	}
	targets := []*loadbalance.Target{
		{URL: "http://api.example.com:8080"},
	}

	p, err := NewProxy(cfg, targets, nil, nil)
	if err != nil {
		t.Fatalf("NewProxy() error: %v", err)
	}

	// 没有解析器，刷新应该直接返回
	p.refreshDNS()
	// 不应 panic
}

// TestRefreshDNS_IPAddressTarget 测试 IP 类型的目标不需要解析。
func TestRefreshDNS_IPAddressTarget(t *testing.T) {
	cfg := &config.ProxyConfig{
		Path:        "/api",
		LoadBalance: "round_robin",
		Timeout:     config.ProxyTimeout{Connect: 5 * time.Second},
	}
	// 使用 IP 地址而非域名
	targets := []*loadbalance.Target{
		{URL: "http://192.168.1.100:8080"},
	}

	p, err := NewProxy(cfg, targets, nil, nil)
	if err != nil {
		t.Fatalf("NewProxy() error: %v", err)
	}

	mr := &mockResolver{}
	p.SetResolver(mr)

	// IP 类型的目标不需要解析
	if targets[0].NeedsResolve(30 * time.Second) {
		t.Error("IP address target should not need resolve")
	}

	// 执行刷新
	p.refreshDNS()

	// 解析器不应被调用（目标不需要解析）
	if mr.getLookupWithCacheCalls() != 0 {
		t.Errorf("expected 0 lookup calls for IP target, got %d", mr.getLookupWithCacheCalls())
	}
}

// TestRefreshDNS_RecentlyResolved 测试最近已解析的目标不需要再次解析。
func TestRefreshDNS_RecentlyResolved(t *testing.T) {
	cfg := &config.ProxyConfig{
		Path:        "/api",
		LoadBalance: "round_robin",
		Timeout:     config.ProxyTimeout{Connect: 5 * time.Second},
	}
	targets := []*loadbalance.Target{
		{URL: "http://api.example.com:8080"},
	}

	p, err := NewProxy(cfg, targets, nil, nil)
	if err != nil {
		t.Fatalf("NewProxy() error: %v", err)
	}

	// 标记目标为已解析（刚刚）
	targets[0].SetResolvedIPs([]string{"10.0.0.1"})

	mr := &mockResolver{}
	p.SetResolver(mr)

	// TTL 内不需要再次解析
	if targets[0].NeedsResolve(30 * time.Second) {
		t.Error("recently resolved target should not need resolve")
	}

	// 执行刷新
	p.refreshDNS()

	// 解析器不应被调用
	if mr.getLookupWithCacheCalls() != 0 {
		t.Errorf("expected 0 lookup calls, got %d", mr.getLookupWithCacheCalls())
	}
}

// TestRefreshDNS_ExpiredResolve 测试 TTL 过期后需要重新解析。
// 该测试验证 TTL 过期检查的正确性：
// - 短时间内（< TTL）不需要重新解析
// - 长时间后（> TTL）需要重新解析
func TestRefreshDNS_ExpiredResolve(t *testing.T) {
	cfg := &config.ProxyConfig{
		Path:        "/api",
		LoadBalance: "round_robin",
		Timeout:     config.ProxyTimeout{Connect: 5 * time.Second},
	}
	targets := []*loadbalance.Target{
		{URL: "http://api.example.com:8080"},
	}

	p, err := NewProxy(cfg, targets, nil, nil)
	if err != nil {
		t.Fatalf("NewProxy() error: %v", err)
	}

	mr := &mockResolver{
		lookupResults: map[string][]string{
			"api.example.com": {"10.0.0.2"},
		},
	}
	p.SetResolver(mr)

	// 首次解析 - 这会设置 lastResolved 为当前时间
	p.refreshDNS()

	if mr.getLookupWithCacheCalls() != 1 {
		t.Fatalf("expected 1 lookup call for initial resolve, got %d", mr.getLookupWithCacheCalls())
	}

	// 验证首次解析后的 IP
	ips := targets[0].ResolvedIPs()
	if len(ips) != 1 || ips[0] != "10.0.0.2" {
		t.Fatalf("expected IPs [10.0.0.2], got %v", ips)
	}

	// 立即调用 NeedsResolve：在默认 TTL（30s）内，不应需要重新解析
	if targets[0].NeedsResolve(30 * time.Second) {
		t.Error("target should NOT need resolve with 30s TTL immediately after resolving")
	}

	// 等待超过 150ms，然后测试短 TTL 下的过期行为
	time.Sleep(150 * time.Millisecond)

	// 使用短 TTL（100ms），此时目标应该过期需要重新解析
	if !targets[0].NeedsResolve(100 * time.Millisecond) {
		t.Fatal("target should need resolve with 100ms TTL after waiting 150ms")
	}
}

// TestUpdateHostClientAddr_HTTP 测试 HTTP 目标地址更新。
func TestUpdateHostClientAddr_HTTP(t *testing.T) {
	cfg := &config.ProxyConfig{
		Path:        "/api",
		LoadBalance: "round_robin",
		Timeout:     config.ProxyTimeout{Connect: 5 * time.Second},
	}
	targets := []*loadbalance.Target{
		{URL: "http://api.example.com:8080"},
	}

	p, err := NewProxy(cfg, targets, nil, nil)
	if err != nil {
		t.Fatalf("NewProxy() error: %v", err)
	}

	target := targets[0]
	newIP := "10.0.0.5"

	p.updateHostClientAddr(target, newIP)

	p.mu.RLock()
	client := p.clients["http://api.example.com:8080"]
	p.mu.RUnlock()

	if client == nil {
		t.Fatal("HostClient not found")
	}

	expected := "10.0.0.5:8080"
	if client.Addr != expected {
		t.Errorf("client.Addr = %q, want %q", client.Addr, expected)
	}
}

// TestUpdateHostClientAddr_HTTPS 测试 HTTPS 目标地址更新。
func TestUpdateHostClientAddr_HTTPS(t *testing.T) {
	cfg := &config.ProxyConfig{
		Path:        "/api",
		LoadBalance: "round_robin",
		Timeout:     config.ProxyTimeout{Connect: 5 * time.Second},
	}
	targets := []*loadbalance.Target{
		{URL: "https://api.example.com:8443"},
	}

	p, err := NewProxy(cfg, targets, nil, nil)
	if err != nil {
		t.Fatalf("NewProxy() error: %v", err)
	}

	target := targets[0]
	newIP := "10.0.0.10"

	p.updateHostClientAddr(target, newIP)

	p.mu.RLock()
	client := p.clients["https://api.example.com:8443"]
	p.mu.RUnlock()

	if client == nil {
		t.Fatal("HostClient not found")
	}

	expected := "10.0.0.10:8443"
	if client.Addr != expected {
		t.Errorf("client.Addr = %q, want %q", client.Addr, expected)
	}
}

// TestUpdateHostClientAddr_DefaultPort 测试没有端口时使用默认端口。
func TestUpdateHostClientAddr_DefaultPort(t *testing.T) {
	tests := []struct {
		name         string
		targetURL    string
		newIP        string
		expectedAddr string
	}{
		{
			name:         "HTTP 默认端口 80",
			targetURL:    "http://api.example.com",
			newIP:        "10.0.0.1",
			expectedAddr: "10.0.0.1:80",
		},
		{
			name:         "HTTPS 默认端口 443",
			targetURL:    "https://api.example.com",
			newIP:        "10.0.0.2",
			expectedAddr: "10.0.0.2:443",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.ProxyConfig{
				Path:        "/api",
				LoadBalance: "round_robin",
				Timeout:     config.ProxyTimeout{Connect: 5 * time.Second},
			}
			targets := []*loadbalance.Target{
				{URL: tt.targetURL},
			}

			p, err := NewProxy(cfg, targets, nil, nil)
			if err != nil {
				t.Fatalf("NewProxy() error: %v", err)
			}

			p.updateHostClientAddr(targets[0], tt.newIP)

			p.mu.RLock()
			client := p.clients[tt.targetURL]
			p.mu.RUnlock()

			if client == nil {
				t.Fatalf("HostClient not found for %s", tt.targetURL)
			}

			if client.Addr != tt.expectedAddr {
				t.Errorf("client.Addr = %q, want %q", client.Addr, tt.expectedAddr)
			}
		})
	}
}

// TestUpdateHostClientAddr_NonExistentTarget 测试不存在的目标不更新。
func TestUpdateHostClientAddr_NonExistentTarget(t *testing.T) {
	cfg := &config.ProxyConfig{
		Path:        "/api",
		LoadBalance: "round_robin",
		Timeout:     config.ProxyTimeout{Connect: 5 * time.Second},
	}
	targets := []*loadbalance.Target{
		{URL: "http://api.example.com:8080"},
	}

	p, err := NewProxy(cfg, targets, nil, nil)
	if err != nil {
		t.Fatalf("NewProxy() error: %v", err)
	}

	// 创建一个不在 clients 中的目标
	unknownTarget := &loadbalance.Target{URL: "http://unknown.example.com:9090"}

	// 不应 panic
	p.updateHostClientAddr(unknownTarget, "10.0.0.1")

	// 原有的客户端不应受影响
	p.mu.RLock()
	client := p.clients["http://api.example.com:8080"]
	p.mu.RUnlock()

	// 注意：HostClient.Addr 存储的是 host:port 格式，不含协议前缀
	expected := "api.example.com:8080"
	if client.Addr != expected {
		t.Errorf("client.Addr changed unexpectedly to %q, want %q", client.Addr, expected)
	}
}

// TestGetResolverTTL 测试 TTL 获取。
func TestGetResolverTTL(t *testing.T) {
	cfg := &config.ProxyConfig{
		Path:        "/api",
		LoadBalance: "round_robin",
		Timeout:     config.ProxyTimeout{Connect: 5 * time.Second},
	}
	targets := []*loadbalance.Target{
		{URL: "http://localhost:8080"},
	}

	p, err := NewProxy(cfg, targets, nil, nil)
	if err != nil {
		t.Fatalf("NewProxy() error: %v", err)
	}

	// 没有解析器时返回 0
	ttl := p.getResolverTTL()
	if ttl != 0 {
		t.Errorf("expected TTL=0 without resolver, got %v", ttl)
	}

	// 有解析器时返回默认值 30s
	mr := &mockResolver{}
	p.SetResolver(mr)

	ttl = p.getResolverTTL()
	if ttl != 30*time.Second {
		t.Errorf("expected TTL=30s with resolver, got %v", ttl)
	}
}

// TestDNSRefreshLoop_StartStop 测试 DNS 刷新循环的启动和停止。
func TestDNSRefreshLoop_StartStop(t *testing.T) {
	cfg := &config.ProxyConfig{
		Path:        "/api",
		LoadBalance: "round_robin",
		Timeout:     config.ProxyTimeout{Connect: 5 * time.Second},
	}
	targets := []*loadbalance.Target{
		{URL: "http://api.example.com:8080"},
	}

	p, err := NewProxy(cfg, targets, nil, nil)
	if err != nil {
		t.Fatalf("NewProxy() error: %v", err)
	}

	mr := &mockResolver{
		lookupResults: map[string][]string{
			"api.example.com": {"10.0.0.1"},
		},
	}
	p.SetResolver(mr)

	if err := p.Start(); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	// 等待 goroutine 启动和可能的 ticker 触发
	time.Sleep(20 * time.Millisecond)

	// 停止代理
	if err := p.Stop(); err != nil {
		t.Fatalf("Stop() error: %v", err)
	}

	// 停止后代理状态应为 false
	if p.started.Load() {
		t.Error("proxy should be stopped")
	}
}

// TestMultipleTargets_Refresh 测试多目标刷新。
func TestMultipleTargets_Refresh(t *testing.T) {
	cfg := &config.ProxyConfig{
		Path:        "/api",
		LoadBalance: "round_robin",
		Timeout:     config.ProxyTimeout{Connect: 5 * time.Second},
	}
	targets := []*loadbalance.Target{
		{URL: "http://api1.example.com:8080"},
		{URL: "http://192.168.1.100:8080"}, // IP 类型，不需要解析
		{URL: "http://api2.example.com:9090"},
	}

	p, err := NewProxy(cfg, targets, nil, nil)
	if err != nil {
		t.Fatalf("NewProxy() error: %v", err)
	}

	mr := &mockResolver{
		lookupResults: map[string][]string{
			"api1.example.com": {"10.0.1.1"},
			"api2.example.com": {"10.0.2.1", "10.0.2.2"},
		},
	}
	p.SetResolver(mr)

	// 执行刷新
	p.refreshDNS()

	// 两个域名目标被解析（IP 类型跳过）
	if mr.getLookupWithCacheCalls() != 2 {
		t.Errorf("expected 2 lookup calls for 2 domain targets, got %d", mr.getLookupWithCacheCalls())
	}

	// 验证第一个目标
	ips1 := targets[0].ResolvedIPs()
	if len(ips1) != 1 || ips1[0] != "10.0.1.1" {
		t.Errorf("target1 IPs = %v, want [10.0.1.1]", ips1)
	}

	// 验证 IP 类型目标无变化
	ipsIP := targets[1].ResolvedIPs()
	if ipsIP != nil {
		t.Errorf("IP target should have no resolved IPs, got %v", ipsIP)
	}

	// 验证第三个目标
	ips2 := targets[2].ResolvedIPs()
	if len(ips2) != 2 || ips2[0] != "10.0.2.1" || ips2[1] != "10.0.2.2" {
		t.Errorf("target3 IPs = %v, want [10.0.2.1, 10.0.2.2]", ips2)
	}

	// 验证 HostClient 地址
	p.mu.RLock()
	client1 := p.clients["http://api1.example.com:8080"]
	client2 := p.clients["http://api2.example.com:9090"]
	p.mu.RUnlock()

	if client1 == nil || client1.Addr != "10.0.1.1:8080" {
		t.Errorf("client1 Addr = %q, want '10.0.1.1:8080'", client1.Addr)
	}
	if client2 == nil || client2.Addr != "10.0.2.1:9090" {
		t.Errorf("client2 Addr = %q, want '10.0.2.1:9090'", client2.Addr)
	}
}

// TestStopResolverFails 测试停止解析器失败时返回错误。
func TestStopResolverFails(t *testing.T) {
	cfg := &config.ProxyConfig{
		Path:        "/api",
		LoadBalance: "round_robin",
		Timeout:     config.ProxyTimeout{Connect: 5 * time.Second},
	}
	targets := []*loadbalance.Target{
		{URL: "http://localhost:8080"},
	}

	p, err := NewProxy(cfg, targets, nil, nil)
	if err != nil {
		t.Fatalf("NewProxy() error: %v", err)
	}

	mr := &mockResolver{stopErr: errors.New("resolver stop failed")}
	p.SetResolver(mr)

	if err := p.Start(); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	err = p.Stop()
	if err == nil {
		t.Fatal("Stop() should return error when resolver fails")
	}

	if !contains(err.Error(), "failed to stop resolver") {
		t.Errorf("expected error containing 'failed to stop resolver', got %v", err)
	}
}
