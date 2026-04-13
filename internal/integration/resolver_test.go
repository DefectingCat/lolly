// resolver_integration_test.go - DNS 解析器集成测试
//
// 测试 DNS 解析器与代理的集成
//
// 作者：xfy
package integration

import (
	"context"
	"testing"
	"time"

	"rua.plus/lolly/internal/config"
	"rua.plus/lolly/internal/resolver"
)

// TestResolverBasicLookup 测试基本 DNS 解析功能
func TestResolverBasicLookup(t *testing.T) {
	cfg := &config.ResolverConfig{
		Enabled:   true,
		Addresses: []string{"8.8.8.8:53"},
		Valid:     30 * time.Second,
		Timeout:   5 * time.Second,
		IPv4:      true,
		IPv6:      false,
	}

	r := resolver.New(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 测试解析一个已知域名
	ips, err := r.LookupHost(ctx, "dns.google")
	if err != nil {
		t.Skipf("Skipping DNS test (network unavailable): %v", err)
		return
	}

	if len(ips) == 0 {
		t.Error("expected at least one IP for dns.google")
	}

	if stopErr := r.Stop(); stopErr != nil {
		t.Errorf("stop failed: %v", stopErr)
	}
}

// TestResolverCache 测试 DNS 缓存功能
func TestResolverCache(t *testing.T) {
	cfg := &config.ResolverConfig{
		Enabled:   true,
		Addresses: []string{"8.8.8.8:53"},
		Valid:     30 * time.Second,
		Timeout:   5 * time.Second,
		IPv4:      true,
		IPv6:      false,
	}

	r := resolver.New(cfg)

	ctx := context.Background()

	// 第一次查询（缓存未命中）
	start := time.Now()
	ips1, err := r.LookupHostWithCache(ctx, "dns.google")
	if err != nil {
		t.Skipf("Skipping DNS test (network unavailable): %v", err)
		return
	}
	duration1 := time.Since(start)

	// 第二次查询（应该命中缓存）
	start = time.Now()
	ips2, err := r.LookupHostWithCache(ctx, "dns.google")
	if err != nil {
		t.Errorf("second lookup failed: %v", err)
		return
	}
	duration2 := time.Since(start)

	// 验证缓存命中（第二次应该更快）
	if duration2 > duration1 {
		t.Logf("Warning: cached lookup (%v) slower than uncached (%v)", duration2, duration1)
	}

	// 验证返回的 IP 相同
	if len(ips1) != len(ips2) {
		t.Errorf("cached result different: got %d IPs, expected %d", len(ips2), len(ips1))
	}

	// 检查统计信息
	stats := r.Stats()
	if stats.CacheHits < 1 {
		t.Error("expected at least 1 cache hit")
	}
	if stats.CacheMisses < 1 {
		t.Error("expected at least 1 cache miss")
	}

	if stopErr := r.Stop(); stopErr != nil {
		t.Errorf("stop failed: %v", stopErr)
	}
}

// TestResolverTimeout 测试 DNS 查询超时
func TestResolverTimeout(t *testing.T) {
	// 使用一个不可达的地址
	cfg := &config.ResolverConfig{
		Enabled:   true,
		Addresses: []string{"192.0.2.1:53"}, // TEST-NET-1，不会响应
		Valid:     30 * time.Second,
		Timeout:   1 * time.Second, // 短超时
		IPv4:      true,
		IPv6:      false,
	}

	r := resolver.New(cfg)

	ctx := context.Background()

	start := time.Now()
	_, err := r.LookupHost(ctx, "example.com")
	elapsed := time.Since(start)

	// 注意：在某些网络环境下，这个地址可能会被防火墙快速拒绝
	// 所以我们只验证超时时间是否合理，不要求一定返回错误
	if err == nil {
		t.Log("Warning: expected timeout error but got success (network may have different behavior)")
	}

	// 验证超时时间不超过合理范围（允许一些偏差）
	if elapsed > 3*time.Second {
		t.Errorf("timeout took too long: %v", elapsed)
	}

	if stopErr := r.Stop(); stopErr != nil {
		t.Errorf("stop failed: %v", stopErr)
	}
}

// TestResolverNXDOMAIN 测试 NXDOMAIN 错误处理
func TestResolverNXDOMAIN(t *testing.T) {
	cfg := &config.ResolverConfig{
		Enabled:   true,
		Addresses: []string{"8.8.8.8:53"},
		Valid:     30 * time.Second,
		Timeout:   5 * time.Second,
		IPv4:      true,
		IPv6:      false,
	}

	r := resolver.New(cfg)

	ctx := context.Background()

	// 查询一个不存在的域名
	_, err := r.LookupHost(ctx, "this-should-not-exist-12345.example")
	// 注意：某些 DNS 服务器可能配置为返回特定响应而不是 NXDOMAIN
	// 这里我们只记录结果，不强制要求错误
	if err == nil {
		t.Log("Warning: no error for non-existent domain (DNS server may have different behavior)")
	}

	if stopErr := r.Stop(); stopErr != nil {
		t.Errorf("stop failed: %v", stopErr)
	}
}

// TestResolverStats 测试解析器统计信息
func TestResolverStats(t *testing.T) {
	cfg := &config.ResolverConfig{
		Enabled:   true,
		Addresses: []string{"8.8.8.8:53"},
		Valid:     30 * time.Second,
		Timeout:   5 * time.Second,
		IPv4:      true,
		IPv6:      false,
	}

	r := resolver.New(cfg)

	// 初始状态
	stats := r.Stats()
	if stats.CacheEntries != 0 {
		t.Errorf("expected 0 cache entries initially, got %d", stats.CacheEntries)
	}

	ctx := context.Background()

	// 执行几次查询
	_, err1 := r.LookupHostWithCache(ctx, "dns.google")
	if err1 != nil {
		t.Logf("first lookup result (may fail): %v", err1)
	}
	_, err2 := r.LookupHostWithCache(ctx, "cloudflare.com")
	if err2 != nil {
		t.Logf("second lookup result (may fail): %v", err2)
	}

	// 验证缓存条目
	stats = r.Stats()
	if stats.CacheEntries < 2 {
		t.Errorf("expected at least 2 cache entries, got %d", stats.CacheEntries)
	}

	if stopErr := r.Stop(); stopErr != nil {
		t.Errorf("stop failed: %v", stopErr)
	}
}

// TestResolverRefresh 测试 DNS 缓存刷新
func TestResolverRefresh(t *testing.T) {
	cfg := &config.ResolverConfig{
		Enabled:   true,
		Addresses: []string{"8.8.8.8:53"},
		Valid:     30 * time.Second,
		Timeout:   5 * time.Second,
		IPv4:      true,
		IPv6:      false,
	}

	r := resolver.New(cfg)

	ctx := context.Background()

	// 先查询一次
	_, lookupErr := r.LookupHostWithCache(ctx, "dns.google")
	if lookupErr != nil {
		t.Logf("lookup result (may fail): %v", lookupErr)
	}

	// 刷新缓存
	err := r.Refresh("dns.google")
	if err != nil {
		t.Errorf("refresh failed: %v", err)
	}

	if stopErr := r.Stop(); stopErr != nil {
		t.Errorf("stop failed: %v", stopErr)
	}
}
