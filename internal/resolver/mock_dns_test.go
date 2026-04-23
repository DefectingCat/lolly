// Package resolver 提供 Mock DNS 测试。
//
// 该文件测试 DNS 解析器的核心功能：
//   - queryDNS 成功/超时/失败
//   - queryWithResolver 自定义服务器
//   - refreshLoop 启动/停止
//   - 缓存和 LRU 淘汰
//
// 作者：xfy
package resolver

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"rua.plus/lolly/internal/config"
)

// TestMockDNSResolverDisabled 测试禁用状态。
func TestMockDNSResolverDisabled(t *testing.T) {
	cfg := &config.ResolverConfig{
		Enabled: false,
	}

	resolver := New(cfg)
	if resolver == nil {
		t.Fatal("New() returned nil")
	}

	// 禁用状态下应返回错误
	_, err := resolver.LookupHost(context.Background(), "example.com")
	if err == nil {
		t.Error("Expected error for disabled resolver")
	}
}

// TestMockDNSResolverEnabled 测试启用状态。
func TestMockDNSResolverEnabled(t *testing.T) {
	cfg := &config.ResolverConfig{
		Enabled:   true,
		Valid:     30 * time.Second,
		Timeout:   5 * time.Second,
		CacheSize: 100,
	}

	resolver := New(cfg)
	if resolver == nil {
		t.Fatal("New() returned nil")
	}

	// 验证类型
	if _, ok := resolver.(*DNSResolver); !ok {
		t.Error("Expected DNSResolver type")
	}
}

// TestMockDNSResolverDefaultValues 测试默认值设置。
func TestMockDNSResolverDefaultValues(t *testing.T) {
	cfg := &config.ResolverConfig{
		Enabled: true,
		// 其他字段为零值
	}

	resolver := New(cfg).(*DNSResolver)

	// 验证默认值
	if resolver.config.Valid != 30*time.Second {
		t.Errorf("Expected default Valid 30s, got %v", resolver.config.Valid)
	}
	if resolver.config.Timeout != 5*time.Second {
		t.Errorf("Expected default Timeout 5s, got %v", resolver.config.Timeout)
	}
	if !resolver.config.IPv4 {
		t.Error("Expected IPv4 to be enabled by default")
	}
}

// TestMockDNSLookupHostIPAddress 测试直接 IP 地址。
func TestMockDNSLookupHostIPAddress(t *testing.T) {
	cfg := &config.ResolverConfig{
		Enabled: true,
	}

	resolver := New(cfg).(*DNSResolver)

	// 直接 IP 地址应直接返回
	ips, err := resolver.LookupHost(context.Background(), "127.0.0.1")
	if err != nil {
		t.Fatalf("LookupHost() error: %v", err)
	}
	if len(ips) != 1 || ips[0] != "127.0.0.1" {
		t.Errorf("Expected ['127.0.0.1'], got %v", ips)
	}

	// IPv6 地址
	ips, err = resolver.LookupHost(context.Background(), "::1")
	if err != nil {
		t.Fatalf("LookupHost() error for IPv6: %v", err)
	}
	if len(ips) != 1 || ips[0] != "::1" {
		t.Errorf("Expected ['::1'], got %v", ips)
	}
}

// TestMockDNSLookupHostWithCache 测试带缓存的解析。
func TestMockDNSLookupHostWithCache(t *testing.T) {
	cfg := &config.ResolverConfig{
		Enabled:   true,
		Valid:     30 * time.Second,
		CacheSize: 10,
	}

	resolver := New(cfg).(*DNSResolver)

	// 第一次查询（缓存未命中）
	ips1, err := resolver.LookupHostWithCache(context.Background(), "localhost")
	if err != nil {
		t.Fatalf("First LookupHostWithCache() error: %v", err)
	}

	// 第二次查询（缓存命中）
	ips2, err := resolver.LookupHostWithCache(context.Background(), "localhost")
	if err != nil {
		t.Fatalf("Second LookupHostWithCache() error: %v", err)
	}

	// 结果应一致
	if len(ips1) != len(ips2) {
		t.Errorf("Cache result mismatch: %v vs %v", ips1, ips2)
	}

	// 验证缓存命中
	stats := resolver.Stats()
	if stats.CacheHits < 1 {
		t.Error("Expected at least one cache hit")
	}
}

// TestMockDNSCacheExpiration 测试缓存过期。
func TestMockDNSCacheExpiration(t *testing.T) {
	cfg := &config.ResolverConfig{
		Enabled:   true,
		Valid:     100 * time.Millisecond, // 短 TTL
		CacheSize: 10,
	}

	resolver := New(cfg).(*DNSResolver)

	// 第一次查询
	_, err := resolver.LookupHostWithCache(context.Background(), "localhost")
	if err != nil {
		t.Fatalf("First lookup error: %v", err)
	}

	// 等待缓存过期
	time.Sleep(150 * time.Millisecond)

	// 第二次查询应重新解析
	_, err = resolver.LookupHostWithCache(context.Background(), "localhost")
	if err != nil {
		t.Fatalf("Second lookup error: %v", err)
	}

	// 验证有两次缓存未命中
	stats := resolver.Stats()
	if stats.CacheMisses < 2 {
		t.Errorf("Expected at least 2 cache misses, got %d", stats.CacheMisses)
	}
}

// TestMockDNSLRUEviction 测试 LRU 淘汰。
func TestMockDNSLRUEviction(t *testing.T) {
	cfg := &config.ResolverConfig{
		Enabled:   true,
		Valid:     30 * time.Second,
		CacheSize: 3, // 只缓存 3 个
	}

	resolver := New(cfg).(*DNSResolver)

	// 添加 3 个条目
	hosts := []string{"host1.local", "host2.local", "host3.local"}
	for _, host := range hosts {
		resolver.storeCache(host, &DNSCacheEntry{
			IPs:       []string{"127.0.0.1"},
			ExpiresAt: time.Now().Add(time.Hour),
		})
	}

	// 验证缓存大小
	resolver.mu.RLock()
	cacheLen := len(resolver.cache)
	resolver.mu.RUnlock()
	if cacheLen != 3 {
		t.Errorf("Expected 3 cache entries, got %d", cacheLen)
	}

	// 添加第 4 个条目，应淘汰最旧的
	resolver.storeCache("host4.local", &DNSCacheEntry{
		IPs:       []string{"127.0.0.1"},
		ExpiresAt: time.Now().Add(time.Hour),
	})

	resolver.mu.RLock()
	cacheLen = len(resolver.cache)
	_, exists := resolver.cache["host1.local"]
	resolver.mu.RUnlock()

	if cacheLen > 3 {
		t.Errorf("Cache should not exceed 3 entries, got %d", cacheLen)
	}
	if exists {
		t.Error("Oldest entry should have been evicted")
	}
}

// TestMockDNSRefreshLoop 测试后台刷新循环。
func TestMockDNSRefreshLoop(t *testing.T) {
	cfg := &config.ResolverConfig{
		Enabled:   true,
		Valid:     200 * time.Millisecond,
		Timeout:   1 * time.Second,
		CacheSize: 10,
	}

	resolver := New(cfg).(*DNSResolver)

	// 启动刷新循环
	if err := resolver.Start(); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	// 验证已启动
	if !resolver.started.Load() {
		t.Error("Resolver should be started")
	}

	// 添加一个需要刷新的主机
	resolver.mu.Lock()
	resolver.refreshHosts["localhost"] = struct{}{}
	resolver.mu.Unlock()

	// 等待刷新
	time.Sleep(300 * time.Millisecond)

	// 停止
	if err := resolver.Stop(); err != nil {
		t.Fatalf("Stop() error: %v", err)
	}

	// 验证已停止
	if resolver.started.Load() {
		t.Error("Resolver should be stopped")
	}
}

// TestMockDNSStartIdempotent 测试重复启动。
func TestMockDNSStartIdempotent(t *testing.T) {
	cfg := &config.ResolverConfig{
		Enabled: true,
	}

	resolver := New(cfg).(*DNSResolver)

	// 第一次启动
	if err := resolver.Start(); err != nil {
		t.Fatalf("First Start() error: %v", err)
	}

	// 第二次启动应无操作
	if err := resolver.Start(); err != nil {
		t.Fatalf("Second Start() error: %v", err)
	}

	// 清理
	_ = resolver.Stop()
}

// TestMockDNSStopIdempotent 测试重复停止。
func TestMockDNSStopIdempotent(t *testing.T) {
	cfg := &config.ResolverConfig{
		Enabled: true,
	}

	resolver := New(cfg).(*DNSResolver)

	// 未启动时停止
	if err := resolver.Stop(); err != nil {
		t.Fatalf("Stop() on non-started resolver error: %v", err)
	}

	// 启动
	_ = resolver.Start()

	// 第一次停止
	if err := resolver.Stop(); err != nil {
		t.Fatalf("First Stop() error: %v", err)
	}

	// 第二次停止应无操作
	if err := resolver.Stop(); err != nil {
		t.Fatalf("Second Stop() error: %v", err)
	}
}

// TestMockDNSStatsFunc 测试统计信息。
func TestMockDNSStatsFunc(t *testing.T) {
	cfg := &config.ResolverConfig{
		Enabled:   true,
		CacheSize: 10,
	}

	resolver := New(cfg).(*DNSResolver)

	// 初始统计
	stats := resolver.Stats()
	if stats.CacheHits != 0 || stats.CacheMisses != 0 {
		t.Error("Initial stats should be zero")
	}

	// 执行查询
	_, _ = resolver.LookupHostWithCache(context.Background(), "localhost")
	_, _ = resolver.LookupHostWithCache(context.Background(), "localhost")

	stats = resolver.Stats()
	if stats.CacheMisses < 1 {
		t.Error("Expected at least one cache miss")
	}
	if stats.CacheHits < 1 {
		t.Error("Expected at least one cache hit")
	}
}

// TestMockDNSQueryDNSTimeout 测试 DNS 查询超时。
func TestMockDNSQueryDNSTimeout(t *testing.T) {
	cfg := &config.ResolverConfig{
		Enabled:   true,
		Timeout:   100 * time.Millisecond,
		Addresses: []string{"127.0.0.1:53535"}, // 不存在的 DNS 服务器
		CacheSize: 10,
	}

	resolver := New(cfg).(*DNSResolver)

	// 使用短超时 context
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := resolver.LookupHost(ctx, "example.com")
	elapsed := time.Since(start)

	// 应该超时
	if err == nil {
		t.Error("Expected timeout error")
	}
	if elapsed > 200*time.Millisecond {
		t.Errorf("Query took too long: %v", elapsed)
	}
}

// TestMockDNSQueryWithResolverSystemDefault 测试使用系统默认 DNS。
func TestMockDNSQueryWithResolverSystemDefault(t *testing.T) {
	cfg := &config.ResolverConfig{
		Enabled:   true,
		IPv4:      true,
		CacheSize: 10,
	}

	resolver := New(cfg).(*DNSResolver)

	// 不指定 DNS 服务器，使用系统默认
	ips, err := resolver.queryWithResolver(context.Background(), "localhost", "")
	if err != nil {
		t.Fatalf("queryWithResolver() error: %v", err)
	}
	if len(ips) == 0 {
		t.Error("Expected at least one IP for localhost")
	}
}

// TestMockDNSConcurrentAccess 测试并发访问。
func TestMockDNSConcurrentAccess(t *testing.T) {
	cfg := &config.ResolverConfig{
		Enabled:   true,
		CacheSize: 100,
	}

	resolver := New(cfg).(*DNSResolver)

	var wg sync.WaitGroup
	concurrency := 10
	iterations := 100

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				_, _ = resolver.LookupHostWithCache(context.Background(), "localhost")
			}
		}()
	}

	wg.Wait()

	stats := resolver.Stats()
	totalOps := stats.CacheHits + stats.CacheMisses
	if totalOps < int64(concurrency*iterations/2) {
		t.Errorf("Expected more operations, got hits=%d, misses=%d", stats.CacheHits, stats.CacheMisses)
	}
}

// TestMockDNSMoveToFrontLocked 测试 LRU 移动到前端。
func TestMockDNSMoveToFrontLocked(t *testing.T) {
	cfg := &config.ResolverConfig{
		Enabled:   true,
		CacheSize: 10,
	}

	resolver := New(cfg).(*DNSResolver)

	// 设置缓存
	resolver.cache = map[string]*DNSCacheEntry{
		"a": {},
		"b": {},
		"c": {},
	}
	resolver.lruOrder = []string{"a", "b", "c"}

	// 移动 "a" 到前端
	resolver.moveToFrontLocked("a")

	// 验证顺序
	expected := []string{"b", "c", "a"}
	for i, v := range expected {
		if resolver.lruOrder[i] != v {
			t.Errorf("LRU order[%d] = %s, want %s", i, resolver.lruOrder[i], v)
		}
	}
}

// TestMockDNSNoopResolver 测试空实现解析器。
func TestMockDNSNoopResolver(t *testing.T) {
	var r Resolver = &noopResolver{}

	// LookupHost 应返回错误
	_, err := r.LookupHost(context.Background(), "example.com")
	if err == nil {
		t.Error("Expected error from noopResolver.LookupHost")
	}

	// LookupHostWithCache 应返回错误
	_, err = r.LookupHostWithCache(context.Background(), "example.com")
	if err == nil {
		t.Error("Expected error from noopResolver.LookupHostWithCache")
	}

	// Refresh 应返回 nil
	if err := r.Refresh("example.com"); err != nil {
		t.Errorf("noopResolver.Refresh() error: %v", err)
	}

	// Start 应返回 nil
	if err := r.Start(); err != nil {
		t.Errorf("noopResolver.Start() error: %v", err)
	}

	// Stop 应返回 nil
	if err := r.Stop(); err != nil {
		t.Errorf("noopResolver.Stop() error: %v", err)
	}

	// Stats 应返回空
	stats := r.Stats()
	if stats.CacheHits != 0 || stats.CacheMisses != 0 {
		t.Error("noopResolver.Stats() should return zero values")
	}
}

// TestMockDNSCacheEntryConcurrency 测试缓存条目并发访问。
func TestMockDNSCacheEntryConcurrency(t *testing.T) {
	entry := &DNSCacheEntry{
		IPs:       []string{"127.0.0.1", "127.0.0.2"},
		ExpiresAt: time.Now().Add(time.Hour),
	}

	var wg sync.WaitGroup
	var readCount atomic.Int64

	// 并发读取
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			entry.mu.RLock()
			_ = entry.IPs
			_ = entry.ExpiresAt
			entry.mu.RUnlock()
			readCount.Add(1)
		}()
	}

	wg.Wait()

	if readCount.Load() != 100 {
		t.Errorf("Expected 100 reads, got %d", readCount.Load())
	}
}

// TestMockDNSIPv6QueryFallback 测试 IPv6 查询失败降级。
func TestMockDNSIPv6QueryFallback(t *testing.T) {
	cfg := &config.ResolverConfig{
		Enabled:   true,
		IPv4:      true,
		IPv6:      true,
		CacheSize: 10,
	}

	resolver := New(cfg).(*DNSResolver)

	// 查询 localhost（通常有 IPv4）
	ips, err := resolver.LookupHost(context.Background(), "localhost")
	if err != nil {
		t.Fatalf("LookupHost() error: %v", err)
	}

	// 应该至少有 IPv4 地址
	if len(ips) == 0 {
		t.Error("Expected at least one IP address")
	}
}

// TestMockDNSDoRefresh 测试刷新操作。
func TestMockDNSDoRefresh(t *testing.T) {
	cfg := &config.ResolverConfig{
		Enabled:   true,
		Valid:     30 * time.Second,
		Timeout:   1 * time.Second,
		CacheSize: 10,
	}

	resolver := New(cfg).(*DNSResolver)

	// 添加需要刷新的主机
	resolver.mu.Lock()
	resolver.refreshHosts["localhost"] = struct{}{}
	resolver.refreshHosts["127.0.0.1"] = struct{}{} // IP 地址会被跳过
	resolver.mu.Unlock()

	// 执行刷新
	resolver.doRefresh()

	// 验证刷新列表仍然存在
	resolver.mu.RLock()
	_, exists := resolver.refreshHosts["localhost"]
	resolver.mu.RUnlock()

	if !exists {
		t.Error("Refresh hosts should still contain localhost")
	}
}
