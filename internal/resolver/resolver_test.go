package resolver

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"rua.plus/lolly/internal/config"
)

// TestNewResolver 测试解析器创建。
func TestNewResolver(t *testing.T) {
	// 测试启用状态
	cfg := &config.ResolverConfig{
		Enabled:   true,
		Addresses: []string{"8.8.8.8:53"},
		Valid:     30 * time.Second,
		Timeout:   5 * time.Second,
		IPv4:      true,
		IPv6:      false,
	}

	r := New(cfg)
	if r == nil {
		t.Fatal("New() should return non-nil resolver")
	}

	// 验证是 DNSResolver 类型
	dnsR, ok := r.(*DNSResolver)
	if !ok {
		t.Fatal("New() should return *DNSResolver when enabled")
	}

	if !dnsR.config.Enabled {
		t.Error("config.Enabled should be true")
	}

	// 测试禁用状态
	cfgDisabed := &config.ResolverConfig{
		Enabled: false,
	}
	rDisabled := New(cfgDisabed)
	if _, ok := rDisabled.(*noopResolver); !ok {
		t.Error("New() should return *noopResolver when disabled")
	}
}

// TestNewResolverDefaults 测试默认值设置。
func TestNewResolverDefaults(t *testing.T) {
	cfg := &config.ResolverConfig{
		Enabled:   true,
		Addresses: []string{"8.8.8.8:53"},
		// 不设置 Valid 和 Timeout
	}

	r := New(cfg).(*DNSResolver)
	if r.config.Valid != 30*time.Second {
		t.Errorf("expected default Valid=30s, got %v", r.config.Valid)
	}
	if r.config.Timeout != 5*time.Second {
		t.Errorf("expected default Timeout=5s, got %v", r.config.Timeout)
	}
}

// TestLookupHostWithIP 测试 IP 地址直接返回。
func TestLookupHostWithIP(t *testing.T) {
	cfg := &config.ResolverConfig{
		Enabled:   true,
		Addresses: []string{"8.8.8.8:53"},
		Valid:     30 * time.Second,
		Timeout:   5 * time.Second,
		IPv4:      true,
	}

	r := New(cfg).(*DNSResolver)

	// 测试 IPv4 地址直接返回
	ips, err := r.LookupHost(context.Background(), "127.0.0.1")
	if err != nil {
		t.Fatalf("LookupHost failed: %v", err)
	}
	if len(ips) != 1 || ips[0] != "127.0.0.1" {
		t.Errorf("expected [127.0.0.1], got %v", ips)
	}

	// 测试 IPv6 地址直接返回
	ips, err = r.LookupHost(context.Background(), "::1")
	if err != nil {
		t.Fatalf("LookupHost failed: %v", err)
	}
	if len(ips) != 1 || ips[0] != "::1" {
		t.Errorf("expected [::1], got %v", ips)
	}
}

// TestCache 测试缓存功能。
func TestCache(t *testing.T) {
	cfg := &config.ResolverConfig{
		Enabled:   true,
		Addresses: []string{}, // 空地址，使用系统 DNS
		Valid:     1 * time.Second,
		Timeout:   5 * time.Second,
		IPv4:      true,
	}

	r := New(cfg).(*DNSResolver)

	// 模拟缓存条目
	r.storeCache("test.example.com", &DNSCacheEntry{
		IPs:       []string{"192.168.1.1", "192.168.1.2"},
		ExpiresAt: time.Now().Add(1 * time.Minute),
	})
	r.mu.Lock()
	r.refreshHosts["test.example.com"] = struct{}{}
	r.mu.Unlock()

	// 测试缓存命中
	ctx := context.Background()
	ips, err := r.LookupHostWithCache(ctx, "test.example.com")
	if err != nil {
		t.Fatalf("LookupHostWithCache failed: %v", err)
	}
	if len(ips) != 2 {
		t.Errorf("expected 2 IPs, got %d", len(ips))
	}

	// 验证缓存命中统计
	if r.GetCacheHits() != 1 {
		t.Errorf("expected 1 cache hit, got %d", r.GetCacheHits())
	}

	// 测试缓存过期
	// 更新缓存条目为过期
	r.storeCache("test.example.com", &DNSCacheEntry{
		IPs:       []string{"192.168.1.1"},
		ExpiresAt: time.Now().Add(-1 * time.Second), // 已过期
	})

	// 由于使用系统 DNS，可能会失败，但应该尝试查询
	_, _ = r.LookupHostWithCache(ctx, "test.example.com")

	// 应该有缓存未命中（因为过期了）
	if r.GetCacheMisses() != 1 {
		t.Errorf("expected 1 cache miss, got %d", r.GetCacheMisses())
	}
}

// TestIsCached 测试缓存状态检查。
func TestIsCached(t *testing.T) {
	cfg := &config.ResolverConfig{
		Enabled: true,
		Valid:   30 * time.Second,
	}

	r := New(cfg).(*DNSResolver)

	// 添加未过期的缓存
	r.storeCache("active.example.com", &DNSCacheEntry{
		IPs:       []string{"192.168.1.1"},
		ExpiresAt: time.Now().Add(1 * time.Minute),
	})

	// 添加已过期的缓存
	r.storeCache("expired.example.com", &DNSCacheEntry{
		IPs:       []string{"192.168.1.2"},
		ExpiresAt: time.Now().Add(-1 * time.Second),
	})

	if !r.IsCached("active.example.com") {
		t.Error("IsCached should return true for active entry")
	}
	if r.IsCached("expired.example.com") {
		t.Error("IsCached should return false for expired entry")
	}
	if r.IsCached("unknown.example.com") {
		t.Error("IsCached should return false for unknown entry")
	}
}

// TestCacheHitRate 测试缓存命中率计算。
func TestCacheHitRate(t *testing.T) {
	cfg := &config.ResolverConfig{
		Enabled: true,
		Valid:   30 * time.Second,
	}

	r := New(cfg).(*DNSResolver)

	// 初始命中率应为 0
	if r.GetHitRate() != 0 {
		t.Errorf("expected 0 hit rate, got %f", r.GetHitRate())
	}

	// 模拟缓存命中
	r.storeCache("test.example.com", &DNSCacheEntry{
		IPs:       []string{"192.168.1.1"},
		ExpiresAt: time.Now().Add(1 * time.Minute),
	})
	r.mu.Lock()
	r.refreshHosts["test.example.com"] = struct{}{}
	r.mu.Unlock()

	// 3 次命中
	for i := 0; i < 3; i++ {
		_, _ = r.LookupHostWithCache(context.Background(), "test.example.com")
	}

	// 1 次未命中（新域名）
	_, _ = r.LookupHostWithCache(context.Background(), "unknown.example.com")

	// 命中率应为 3/4 = 0.75
	hitRate := r.GetHitRate()
	if hitRate != 0.75 {
		t.Errorf("expected 0.75 hit rate, got %f", hitRate)
	}
}

// TestStats 测试统计信息。
func TestStats(t *testing.T) {
	cfg := &config.ResolverConfig{
		Enabled: true,
		Valid:   30 * time.Second,
	}

	r := New(cfg).(*DNSResolver)

	// 添加缓存条目
	r.storeCache("test1.example.com", &DNSCacheEntry{
		IPs:       []string{"192.168.1.1"},
		ExpiresAt: time.Now().Add(1 * time.Minute),
	})
	r.storeCache("test2.example.com", &DNSCacheEntry{
		IPs:       []string{"192.168.1.2"},
		ExpiresAt: time.Now().Add(1 * time.Minute),
	})
	r.mu.Lock()
	r.refreshHosts["test1.example.com"] = struct{}{}
	r.refreshHosts["test2.example.com"] = struct{}{}
	r.mu.Unlock()

	// 触发缓存命中
	_, _ = r.LookupHostWithCache(context.Background(), "test1.example.com")

	stats := r.Stats()
	if stats.CacheHits != 1 {
		t.Errorf("expected 1 cache hit, got %d", stats.CacheHits)
	}
	if stats.CacheEntries != 2 {
		t.Errorf("expected 2 cache entries, got %d", stats.CacheEntries)
	}
}

// TestResetStats 测试统计重置。
func TestResetStats(t *testing.T) {
	cfg := &config.ResolverConfig{
		Enabled: true,
		Valid:   30 * time.Second,
	}

	r := New(cfg).(*DNSResolver)

	// 添加统计数据
	r.hits.Store(10)
	r.misses.Store(5)
	r.errors.Store(2)
	r.latencyNs.Store(1000000)
	r.count.Store(10)

	r.ResetStats()

	if r.GetCacheHits() != 0 {
		t.Errorf("expected 0 hits after reset, got %d", r.GetCacheHits())
	}
	if r.GetCacheMisses() != 0 {
		t.Errorf("expected 0 misses after reset, got %d", r.GetCacheMisses())
	}
	if r.GetResolveErrors() != 0 {
		t.Errorf("expected 0 errors after reset, got %d", r.GetResolveErrors())
	}
	if r.GetAverageLatency() != 0 {
		t.Errorf("expected 0 latency after reset, got %v", r.GetAverageLatency())
	}
}

// TestStartStop 测试启动和停止。
func TestStartStop(t *testing.T) {
	cfg := &config.ResolverConfig{
		Enabled:   true,
		Addresses: []string{"8.8.8.8:53"},
		Valid:     30 * time.Second,
	}

	r := New(cfg).(*DNSResolver)

	// 启动
	err := r.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if !r.started.Load() {
		t.Error("resolver should be started")
	}

	// 重复启动不应报错
	err = r.Start()
	if err != nil {
		t.Errorf("Start() should not error when already started: %v", err)
	}

	// 停止
	err = r.Stop()
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
	if r.started.Load() {
		t.Error("resolver should be stopped")
	}
}

// TestDeleteCacheEntry 测试删除缓存条目。
func TestDeleteCacheEntry(t *testing.T) {
	cfg := &config.ResolverConfig{
		Enabled: true,
		Valid:   30 * time.Second,
	}

	r := New(cfg).(*DNSResolver)

	// 添加缓存
	r.storeCache("test.example.com", &DNSCacheEntry{
		IPs:       []string{"192.168.1.1"},
		ExpiresAt: time.Now().Add(1 * time.Minute),
	})
	r.mu.Lock()
	r.refreshHosts["test.example.com"] = struct{}{}
	r.mu.Unlock()

	// 删除
	r.DeleteCacheEntry("test.example.com")

	// 验证删除
	if r.IsCached("test.example.com") {
		t.Error("cache entry should be deleted")
	}
}

// TestClearCache 测试清空缓存。
func TestClearCache(t *testing.T) {
	cfg := &config.ResolverConfig{
		Enabled: true,
		Valid:   30 * time.Second,
	}

	r := New(cfg).(*DNSResolver)

	// 添加多个缓存
	for i := 0; i < 5; i++ {
		host := fmt.Sprintf("test%d.example.com", i)
		r.storeCache(host, &DNSCacheEntry{
			IPs:       []string{fmt.Sprintf("192.168.1.%d", i)},
			ExpiresAt: time.Now().Add(1 * time.Minute),
		})
		r.mu.Lock()
		r.refreshHosts[host] = struct{}{}
		r.mu.Unlock()
	}

	// 清空
	r.ClearCache()

	// 验证
	stats := r.GetCacheStats()
	if stats.Entries != 0 {
		t.Errorf("expected 0 entries after clear, got %d", stats.Entries)
	}
}

// TestConcurrentAccess 测试并发访问。
func TestConcurrentAccess(t *testing.T) {
	cfg := &config.ResolverConfig{
		Enabled: true,
		Valid:   30 * time.Second,
	}

	r := New(cfg).(*DNSResolver)

	// 添加测试缓存
	r.storeCache("test.example.com", &DNSCacheEntry{
		IPs:       []string{"192.168.1.1"},
		ExpiresAt: time.Now().Add(1 * time.Minute),
	})
	r.mu.Lock()
	r.refreshHosts["test.example.com"] = struct{}{}
	r.mu.Unlock()

	// 并发读取
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = r.LookupHostWithCache(context.Background(), "test.example.com")
		}()
	}
	wg.Wait()

	// 验证没有竞争条件导致的 panic
	if r.GetCacheHits() != 100 {
		t.Errorf("expected 100 cache hits, got %d", r.GetCacheHits())
	}
}

// TestNoopResolver 测试空解析器。
func TestNoopResolver(t *testing.T) {
	nr := &noopResolver{}

	ctx := context.Background()

	_, err := nr.LookupHost(ctx, "example.com")
	if err == nil {
		t.Error("noopResolver.LookupHost should return error")
	}

	_, err = nr.LookupHostWithCache(ctx, "example.com")
	if err == nil {
		t.Error("noopResolver.LookupHostWithCache should return error")
	}

	if err := nr.Refresh("example.com"); err != nil {
		t.Error("noopResolver.Refresh should not return error")
	}

	if err := nr.Start(); err != nil {
		t.Error("noopResolver.Start should not return error")
	}

	if err := nr.Stop(); err != nil {
		t.Error("noopResolver.Stop should not return error")
	}

	stats := nr.Stats()
	if stats.CacheHits != 0 || stats.CacheMisses != 0 {
		t.Error("noopResolver.Stats should return empty stats")
	}
}

// TestResolverConfigValidate 测试配置验证。
func TestResolverConfigValidate(t *testing.T) {
	// 禁用状态不验证
	cfg := &config.ResolverConfig{Enabled: false}
	if err := cfg.Validate(); err != nil {
		t.Errorf("disabled resolver should pass validation: %v", err)
	}

	// 启用但没有地址
	cfg = &config.ResolverConfig{
		Enabled: true,
	}
	if err := cfg.Validate(); err == nil {
		t.Error("enabled resolver without addresses should fail")
	}

	// 有效配置
	cfg = &config.ResolverConfig{
		Enabled:   true,
		Addresses: []string{"8.8.8.8:53"},
		Valid:     30 * time.Second,
		Timeout:   5 * time.Second,
		IPv4:      true,
		IPv6:      false,
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("valid config should pass: %v", err)
	}

	// TTL 太短
	cfg = &config.ResolverConfig{
		Enabled:   true,
		Addresses: []string{"8.8.8.8:53"},
		Valid:     500 * time.Millisecond,
	}
	if err := cfg.Validate(); err == nil {
		t.Error("valid < 1s should fail")
	}

	// IPv4 和 IPv6 都禁用
	cfg = &config.ResolverConfig{
		Enabled:   true,
		Addresses: []string{"8.8.8.8:53"},
		IPv4:      false,
		IPv6:      false,
	}
	if err := cfg.Validate(); err == nil {
		t.Error("both IPv4 and IPv6 disabled should fail")
	}
}

// TestResolverConfigTTL 测试 TTL 方法。
func TestResolverConfigTTL(t *testing.T) {
	cfg := &config.ResolverConfig{
		Valid: 60 * time.Second,
	}
	if cfg.TTL() != 60*time.Second {
		t.Errorf("expected TTL=60s, got %v", cfg.TTL())
	}
}

// TestRefresh 测试刷新方法。
func TestRefresh(t *testing.T) {
	cfg := &config.ResolverConfig{
		Enabled: true,
		Valid:   30 * time.Second,
	}

	r := New(cfg).(*DNSResolver)

	// 测试 IP 地址直接返回（无 DNS 查询）
	err := r.Refresh("127.0.0.1")
	if err != nil {
		t.Errorf("Refresh for IP should succeed: %v", err)
	}
}

// TestCacheStats 测试缓存统计。
func TestCacheStats(t *testing.T) {
	cfg := &config.ResolverConfig{
		Enabled: true,
		Valid:   1 * time.Second, // 短 TTL 用于测试过期
	}

	r := New(cfg).(*DNSResolver)

	// 添加活跃缓存
	r.storeCache("active.example.com", &DNSCacheEntry{
		IPs:       []string{"192.168.1.1"},
		ExpiresAt: time.Now().Add(1 * time.Minute),
	})

	// 添加过期缓存
	r.storeCache("expired.example.com", &DNSCacheEntry{
		IPs:       []string{"192.168.1.2"},
		ExpiresAt: time.Now().Add(-1 * time.Second),
	})
	r.mu.Lock()
	r.refreshHosts["active.example.com"] = struct{}{}
	r.refreshHosts["expired.example.com"] = struct{}{}
	r.mu.Unlock()

	// 设置命中/未命中统计
	r.hits.Store(10)
	r.misses.Store(5)

	stats := r.GetCacheStats()
	if stats.Hits != 10 {
		t.Errorf("expected 10 hits, got %d", stats.Hits)
	}
	if stats.Misses != 5 {
		t.Errorf("expected 5 misses, got %d", stats.Misses)
	}
	if stats.Entries != 2 {
		t.Errorf("expected 2 entries, got %d", stats.Entries)
	}
	if stats.Expired != 1 {
		t.Errorf("expected 1 expired, got %d", stats.Expired)
	}
}

// TestCacheSizeLimit 测试缓存大小限制。
func TestCacheSizeLimit(t *testing.T) {
	cfg := &config.ResolverConfig{
		Enabled:   true,
		Valid:     30 * time.Second,
		CacheSize: 3, // 限制 3 个条目
	}

	r := New(cfg).(*DNSResolver)

	// 添加 5 个缓存条目，应淘汰 2 个
	for i := 0; i < 5; i++ {
		host := fmt.Sprintf("host%d.example.com", i)
		r.storeCache(host, &DNSCacheEntry{
			IPs:       []string{fmt.Sprintf("192.168.1.%d", i)},
			ExpiresAt: time.Now().Add(1 * time.Minute),
		})
	}

	// 验证缓存条目数不超过限制
	stats := r.GetCacheStats()
	if stats.Entries > 3 {
		t.Errorf("expected at most 3 entries with CacheSize=3, got %d", stats.Entries)
	}

	// 验证最早添加的条目被淘汰（LRU）
	if r.IsCached("host0.example.com") {
		t.Error("host0.example.com should be evicted (oldest)")
	}
	if r.IsCached("host1.example.com") {
		t.Error("host1.example.com should be evicted (second oldest)")
	}

	// 验证最新添加的条目存在
	if !r.IsCached("host2.example.com") {
		t.Error("host2.example.com should be cached")
	}
	if !r.IsCached("host3.example.com") {
		t.Error("host3.example.com should be cached")
	}
	if !r.IsCached("host4.example.com") {
		t.Error("host4.example.com should be cached")
	}
}

// TestCacheSizeZero 测试 cache_size=0 时无限制。
func TestCacheSizeZero(t *testing.T) {
	cfg := &config.ResolverConfig{
		Enabled:   true,
		Valid:     30 * time.Second,
		CacheSize: 0, // 无限制
	}

	r := New(cfg).(*DNSResolver)

	// 添加大量缓存条目
	for i := 0; i < 100; i++ {
		host := fmt.Sprintf("host%d.example.com", i)
		r.storeCache(host, &DNSCacheEntry{
			IPs:       []string{fmt.Sprintf("192.168.1.%d", i%256)},
			ExpiresAt: time.Now().Add(1 * time.Minute),
		})
	}

	// 验证所有条目都存在
	stats := r.GetCacheStats()
	if stats.Entries != 100 {
		t.Errorf("expected 100 entries with CacheSize=0, got %d", stats.Entries)
	}
}

// TestLRUEvictionOrder 测试 LRU 淘汰顺序。
func TestLRUEvictionOrder(t *testing.T) {
	cfg := &config.ResolverConfig{
		Enabled:   true,
		Valid:     30 * time.Second,
		CacheSize: 3,
	}

	r := New(cfg).(*DNSResolver)

	// 添加 3 个条目填满缓存
	r.storeCache("a.example.com", &DNSCacheEntry{
		IPs:       []string{"192.168.1.1"},
		ExpiresAt: time.Now().Add(1 * time.Minute),
	})
	r.storeCache("b.example.com", &DNSCacheEntry{
		IPs:       []string{"192.168.1.2"},
		ExpiresAt: time.Now().Add(1 * time.Minute),
	})
	r.storeCache("c.example.com", &DNSCacheEntry{
		IPs:       []string{"192.168.1.3"},
		ExpiresAt: time.Now().Add(1 * time.Minute),
	})

	// 访问 a.example.com 使其变为最新
	r.storeCache("a.example.com", &DNSCacheEntry{
		IPs:       []string{"192.168.1.1"},
		ExpiresAt: time.Now().Add(1 * time.Minute),
	})

	// 添加新条目，应淘汰 b.example.com（最久未使用）
	r.storeCache("d.example.com", &DNSCacheEntry{
		IPs:       []string{"192.168.1.4"},
		ExpiresAt: time.Now().Add(1 * time.Minute),
	})

	// 验证淘汰顺序
	if r.IsCached("b.example.com") {
		t.Error("b.example.com should be evicted (least recently used)")
	}
	if !r.IsCached("a.example.com") {
		t.Error("a.example.com should be cached (recently accessed)")
	}
	if !r.IsCached("c.example.com") {
		t.Error("c.example.com should be cached")
	}
	if !r.IsCached("d.example.com") {
		t.Error("d.example.com should be cached (newly added)")
	}
}

// TestCacheUpdatePreservesOrder 测试更新已存在条目不触发淘汰。
func TestCacheUpdatePreservesOrder(t *testing.T) {
	cfg := &config.ResolverConfig{
		Enabled:   true,
		Valid:     30 * time.Second,
		CacheSize: 3,
	}

	r := New(cfg).(*DNSResolver)

	// 添加 3 个条目填满缓存
	r.storeCache("a.example.com", &DNSCacheEntry{
		IPs:       []string{"192.168.1.1"},
		ExpiresAt: time.Now().Add(1 * time.Minute),
	})
	r.storeCache("b.example.com", &DNSCacheEntry{
		IPs:       []string{"192.168.1.2"},
		ExpiresAt: time.Now().Add(1 * time.Minute),
	})
	r.storeCache("c.example.com", &DNSCacheEntry{
		IPs:       []string{"192.168.1.3"},
		ExpiresAt: time.Now().Add(1 * time.Minute),
	})

	// 更新已存在的条目（不应触发淘汰）
	r.storeCache("b.example.com", &DNSCacheEntry{
		IPs:       []string{"192.168.1.20"}, // 新 IP
		ExpiresAt: time.Now().Add(1 * time.Minute),
	})

	// 验证所有条目仍然存在
	stats := r.GetCacheStats()
	if stats.Entries != 3 {
		t.Errorf("expected 3 entries after update, got %d", stats.Entries)
	}

	// 验证更新生效
	entry, ok := r.GetCacheEntry("b.example.com")
	if !ok {
		t.Fatal("b.example.com should exist")
	}
	if len(entry.IPs) != 1 || entry.IPs[0] != "192.168.1.20" {
		t.Errorf("expected IP 192.168.1.20, got %v", entry.IPs)
	}
}
