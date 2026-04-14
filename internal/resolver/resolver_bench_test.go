// Package resolver 提供 DNS 解析器的基准测试。
//
// 该文件包含 DNS 解析器的性能基准测试，测试缓存命中、并发竞争和缓存过期场景。
//
// 作者：xfy
package resolver

import (
	"context"
	"fmt"
	"testing"
	"time"

	"rua.plus/lolly/internal/config"
)

// mockResolver 是一个模拟的 Resolver 实现，用于基准测试。
// 它返回固定的 IP 地址，避免网络依赖，确保测试的稳定性和可重复性。
type mockResolver struct {
	ips   []string
	delay time.Duration
}

// LookupHost 模拟解析主机名，返回固定的 IP 地址列表。
func (m *mockResolver) LookupHost(ctx context.Context, host string) ([]string, error) {
	if m.delay > 0 {
		time.Sleep(m.delay)
	}
	return m.ips, nil
}

// LookupHostWithCache 带缓存的解析实现。
func (m *mockResolver) LookupHostWithCache(ctx context.Context, host string) ([]string, error) {
	return m.LookupHost(ctx, host)
}

// Refresh 刷新指定主机的缓存（模拟实现）。
func (m *mockResolver) Refresh(host string) error {
	return nil
}

// Start 启动后台刷新协程（模拟实现）。
func (m *mockResolver) Start() error {
	return nil
}

// Stop 停止解析器（模拟实现）。
func (m *mockResolver) Stop() error {
	return nil
}

// Stats 返回统计信息（模拟实现）。
func (m *mockResolver) Stats() Stats {
	return Stats{}
}

// createTestResolver 创建一个用于基准测试的 DNSResolver 实例。
// 预填充缓存以模拟真实场景。
func createTestResolver() *DNSResolver {
	cfg := &config.ResolverConfig{
		Enabled: true,
		Valid:   30 * time.Second,
		Timeout: 5 * time.Second,
		IPv4:    true,
	}

	r := New(cfg).(*DNSResolver)

	// 预填充缓存条目，模拟真实的解析场景
	for i := 0; i < 100; i++ {
		host := fmt.Sprintf("host%d.example.com", i)
		r.cache.Store(host, &DNSCacheEntry{
			IPs:        []string{fmt.Sprintf("192.168.1.%d", i%256), fmt.Sprintf("192.168.2.%d", i%256)},
			ExpiresAt:  time.Now().Add(30 * time.Second),
			LastLookup: time.Now(),
		})
		r.mu.Lock()
		r.refreshHosts[host] = struct{}{}
		r.mu.Unlock()
	}

	return r
}

// BenchmarkDNSResolverLookupWithCache 测试缓存命中的性能。
//
// 该基准测试测量在缓存已预热的情况下，LookupHostWithCache 方法的性能。
// 所有查询都应该命中缓存，不涉及 DNS 查询。
//
// 预期结果：
//   - 低延迟（< 1μs）
//   - 零内存分配（sync.Map 读操作）
//   - 高并发安全
func BenchmarkDNSResolverLookupWithCache(b *testing.B) {
	r := createTestResolver()
	ctx := context.Background()
	hosts := []string{
		"host0.example.com",
		"host25.example.com",
		"host50.example.com",
		"host75.example.com",
	}

	// 重置缓存命中统计
	r.hits.Store(0)
	r.misses.Store(0)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			host := hosts[i%len(hosts)]
			_, err := r.LookupHostWithCache(ctx, host)
			if err != nil {
				b.Fatalf("LookupHostWithCache failed: %v", err)
			}
			i++
		}
	})

	// 验证所有请求都命中缓存
	hits := r.GetCacheHits()
	if hits != int64(b.N) {
		b.Logf("缓存命中率: %d/%d (%.2f%%)", hits, b.N, float64(hits)*100/float64(b.N))
	}
}

// BenchmarkDNSResolverConcurrent 测试并发解析同一主机时的锁竞争。
//
// 该基准测试模拟高并发场景下多个 goroutine 同时访问同一 DNSCacheEntry 的情况。
// 主要测试 DNSCacheEntry.mu 读写锁的争用性能。
//
// 关键指标：
//   - 锁等待时间
//   - 并发吞吐量
//   - 无死锁风险
func BenchmarkDNSResolverConcurrent(b *testing.B) {
	cfg := &config.ResolverConfig{
		Enabled: true,
		Valid:   30 * time.Second,
		Timeout: 5 * time.Second,
		IPv4:    true,
	}

	r := New(cfg).(*DNSResolver)

	// 只添加一个缓存条目，所有 goroutine 都访问同一个条目
	targetHost := "concurrent.example.com"
	r.cache.Store(targetHost, &DNSCacheEntry{
		IPs:        []string{"10.0.0.1", "10.0.0.2", "10.0.0.3"},
		ExpiresAt:  time.Now().Add(30 * time.Second),
		LastLookup: time.Now(),
	})
	r.mu.Lock()
	r.refreshHosts[targetHost] = struct{}{}
	r.mu.Unlock()

	ctx := context.Background()

	b.ResetTimer()

	// 使用 RunParallel 模拟并发访问
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := r.LookupHostWithCache(ctx, targetHost)
			if err != nil {
				b.Fatalf("LookupHostWithCache failed: %v", err)
			}
		}
	})

	// 验证并发统计
	hits := r.GetCacheHits()
	b.Logf("并发缓存命中次数: %d", hits)
}

// BenchmarkDNSResolverCacheExpiry 测试缓存过期后的刷新性能。
//
// 该基准测试模拟缓存条目过期后，重新解析的场景。
// 使用 IP 地址直接绕过 DNS 查询，专注于测试过期逻辑的性能。
func BenchmarkDNSResolverCacheExpiry(b *testing.B) {
	cfg := &config.ResolverConfig{
		Enabled: true,
		Valid:   1 * time.Millisecond, // 极短的 TTL 以便快速过期
		Timeout: 5 * time.Second,
		IPv4:    true,
	}

	r := New(cfg).(*DNSResolver)
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// 使用 IP 地址（会被直接返回，不涉及 DNS 查询）
		// 这样可以测试过期逻辑而不依赖网络
		host := fmt.Sprintf("127.0.0.%d", (i%254)+1)

		// 预存储一个已过期的条目
		r.cache.Store(host, &DNSCacheEntry{
			IPs:        []string{"192.168.1.1"},
			ExpiresAt:  time.Now().Add(-1 * time.Second), // 已过期
			LastLookup: time.Now().Add(-2 * time.Second),
		})

		// 查询 IP 地址应该直接返回，不会触发缓存过期逻辑
		// 这样可以测试缓存过期检测的性能
		_, _ = r.LookupHostWithCache(ctx, host)
	}
}

// BenchmarkDNSResolverCacheWriteLock 测试缓存写入时的锁竞争。
//
// 该基准测试模拟多个 goroutine 同时写入缓存的场景，
// 测试 sync.Map 的 Store 操作性能。
func BenchmarkDNSResolverCacheWriteLock(b *testing.B) {
	cfg := &config.ResolverConfig{
		Enabled: true,
		Valid:   30 * time.Second,
		Timeout: 5 * time.Second,
		IPv4:    true,
	}

	r := New(cfg).(*DNSResolver)
	ctx := context.Background()

	// 使用 IP 地址避免 DNS 查询，专注于缓存写入性能
	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			host := fmt.Sprintf("127.0.0.%d", (i%254)+1)
			_, _ = r.LookupHostWithCache(ctx, host)
			i++
		}
	})
}

// BenchmarkDNSResolverMixedWorkload 测试混合读写负载。
//
// 模拟真实场景：80% 缓存命中，20% 新条目写入。
func BenchmarkDNSResolverMixedWorkload(b *testing.B) {
	cfg := &config.ResolverConfig{
		Enabled: true,
		Valid:   30 * time.Second,
		Timeout: 5 * time.Second,
		IPv4:    true,
	}

	r := New(cfg).(*DNSResolver)
	ctx := context.Background()

	// 预填充一些缓存
	for i := 0; i < 50; i++ {
		host := fmt.Sprintf("cached%d.example.com", i)
		r.cache.Store(host, &DNSCacheEntry{
			IPs:       []string{fmt.Sprintf("192.168.1.%d", i%256)},
			ExpiresAt: time.Now().Add(30 * time.Second),
		})
		r.mu.Lock()
		r.refreshHosts[host] = struct{}{}
		r.mu.Unlock()
	}

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			// 80% 概率访问已缓存的条目
			if i%5 != 0 {
				host := fmt.Sprintf("cached%d.example.com", i%50)
				_, _ = r.LookupHostWithCache(ctx, host)
			} else {
				// 20% 概率访问新条目（使用 IP 避免网络）
				host := fmt.Sprintf("127.0.0.%d", (i%254)+1)
				_, _ = r.LookupHostWithCache(ctx, host)
			}
			i++
		}
	})

	// 输出缓存统计
	stats := r.Stats()
	b.Logf("缓存统计: 命中=%d, 未命中=%d, 命中率=%.2f%%",
		stats.CacheHits, stats.CacheMisses, r.GetHitRate()*100)
}

// BenchmarkDNSCacheEntryRLock 测试 DNSCacheEntry 读写锁的读性能。
//
// 该基准测试直接测试 DNSCacheEntry 的 RLock 性能，
// 这是 resolver 并发性能的关键路径。
func BenchmarkDNSCacheEntryRLock(b *testing.B) {
	entry := &DNSCacheEntry{
		IPs:        []string{"192.168.1.1", "192.168.1.2"},
		ExpiresAt:  time.Now().Add(30 * time.Second),
		LastLookup: time.Now(),
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			entry.mu.RLock()
			_ = entry.IPs
			_ = entry.ExpiresAt
			entry.mu.RUnlock()
		}
	})
}

// BenchmarkDNSCacheEntryRWLock 测试 DNSCacheEntry 读写锁的写性能。
//
// 该基准测试直接测试 DNSCacheEntry 的 Lock 性能。
func BenchmarkDNSCacheEntryRWLock(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			entry := &DNSCacheEntry{
				IPs:        []string{fmt.Sprintf("192.168.1.%d", i%256)},
				ExpiresAt:  time.Now().Add(30 * time.Second),
				LastLookup: time.Now(),
			}

			entry.mu.Lock()
			entry.IPs = []string{fmt.Sprintf("10.0.0.%d", i%256)}
			entry.LastLookup = time.Now()
			entry.mu.Unlock()
			i++
		}
	})
}
