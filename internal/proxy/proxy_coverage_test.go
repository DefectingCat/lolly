// Package proxy 提供反向代理覆盖测试，补充 proxy.go 中未覆盖的方法。
//
// 该文件测试代理模块的以下功能：
//   - selectTargetExcluding 排除已失败目标的选择
//   - extractHashKey 哈希键提取
//   - buildCacheKeyHash / buildCacheKeyHashValue 缓存键计算
//   - writeCachedResponse 缓存响应写入
//   - GetCache / GetCacheStats 缓存访问
//   - getCacheDuration 不同状态码的缓存时间
//   - redirect_rewrite 相关功能
//
// 作者：xfy
package proxy

import (
	"testing"
	"time"

	"github.com/valyala/fasthttp"
	"github.com/valyala/fasthttp/fasthttputil"
	"rua.plus/lolly/internal/cache"
	"rua.plus/lolly/internal/config"
	"rua.plus/lolly/internal/loadbalance"
	"rua.plus/lolly/internal/testutil"
)

// TestSelectTargetExcluding 测试排除失败目标的目标选择
func TestSelectTargetExcluding(t *testing.T) {
	cfg := &config.ProxyConfig{
		Path:        "/api",
		LoadBalance: "round_robin",
		Timeout:     config.ProxyTimeout{Connect: 5 * time.Second},
	}

	targets := []*loadbalance.Target{
		{URL: "http://backend1:8080"},
		{URL: "http://backend2:8080"},
		{URL: "http://backend3:8080"},
	}
	for _, target := range targets {
		target.Healthy.Store(true)
	}

	p, err := NewProxy(cfg, targets, nil, nil)
	if err != nil {
		t.Fatalf("NewProxy() error: %v", err)
	}

	ctx := testutil.NewRequestCtx("GET", "/api/test")

	// 排除第一个目标，应该选择第二个
	excluded := []*loadbalance.Target{targets[0]}
	selected := p.selectTargetExcluding(ctx, excluded)
	if selected == nil {
		t.Fatal("selectTargetExcluding() returned nil")
	}
	if selected.URL == "http://backend1:8080" {
		t.Error("selectTargetExcluding() should not select excluded target")
	}

	// 排除所有目标，应该返回 nil
	allExcluded := []*loadbalance.Target{targets[0], targets[1], targets[2]}
	selected = p.selectTargetExcluding(ctx, allExcluded)
	if selected != nil {
		t.Error("selectTargetExcluding() should return nil when all excluded")
	}

	// 空目标列表
	p2, _ := NewProxy(cfg, []*loadbalance.Target{{URL: "http://a:1"}}, nil, nil)
	p2.targets = nil
	selected = p2.selectTargetExcluding(ctx, nil)
	if selected != nil {
		t.Error("selectTargetExcluding() should return nil for empty targets")
	}
}

// TestSelectTargetExcluding_IPHash 测试 IP Hash 排除选择
func TestSelectTargetExcluding_IPHash(t *testing.T) {
	cfg := &config.ProxyConfig{
		Path:        "/api",
		LoadBalance: "ip_hash",
		Timeout:     config.ProxyTimeout{Connect: 5 * time.Second},
	}

	targets := []*loadbalance.Target{
		{URL: "http://backend1:8080"},
		{URL: "http://backend2:8080"},
		{URL: "http://backend3:8080"},
	}
	for _, target := range targets {
		target.Healthy.Store(true)
	}

	p, err := NewProxy(cfg, targets, nil, nil)
	if err != nil {
		t.Fatalf("NewProxy() error: %v", err)
	}

	ctx := testutil.NewRequestCtxWithHeader("GET", "/api/test", map[string]string{
		"X-Forwarded-For": "192.168.1.1",
	})

	// 获取第一次选择
	first := p.selectTarget(ctx)
	if first == nil {
		t.Fatal("selectTarget() returned nil")
	}

	// 排除第一次选择的目标
	excluded := []*loadbalance.Target{first}
	second := p.selectTargetExcluding(ctx, excluded)
	if second == nil {
		t.Fatal("selectTargetExcluding() returned nil")
	}
	if second.URL == first.URL {
		t.Errorf("selectTargetExcluding() should not select same target %s", first.URL)
	}
}

// TestSelectTargetExcluding_ConsistentHash 测试一致性哈希排除选择
func TestSelectTargetExcluding_ConsistentHash(t *testing.T) {
	cfg := &config.ProxyConfig{
		Path:         "/api",
		LoadBalance:  "consistent_hash",
		HashKey:      "uri",
		VirtualNodes: 150,
		Timeout:      config.ProxyTimeout{Connect: 5 * time.Second},
	}

	targets := []*loadbalance.Target{
		{URL: "http://backend1:8080"},
		{URL: "http://backend2:8080"},
	}
	for _, target := range targets {
		target.Healthy.Store(true)
	}

	p, err := NewProxy(cfg, targets, nil, nil)
	if err != nil {
		t.Fatalf("NewProxy() error: %v", err)
	}

	ctx := testutil.NewRequestCtx("GET", "/api/test")

	// 排除一个目标，应该还能选到另一个
	excluded := []*loadbalance.Target{targets[0]}
	selected := p.selectTargetExcluding(ctx, excluded)
	if selected == nil {
		t.Error("selectTargetExcluding() should return remaining target")
	}
}

// TestExtractHashKey 测试哈希键提取
func TestExtractHashKey(t *testing.T) {
	tests := []struct {
		name     string
		hashKey  string
		headers  map[string]string
		expected string
	}{
		{
			name:     "ip hash key",
			hashKey:  "ip",
			headers:  map[string]string{"X-Forwarded-For": "10.0.0.1"},
			expected: "10.0.0.1",
		},
		{
			name:     "empty hash key defaults to ip",
			hashKey:  "",
			headers:  map[string]string{"X-Forwarded-For": "10.0.0.2"},
			expected: "10.0.0.2",
		},
		{
			name:     "uri hash key",
			hashKey:  "uri",
			headers:  nil,
			expected: "/api/test",
		},
		{
			name:     "header hash key - found",
			hashKey:  "header:X-Custom-ID",
			headers:  map[string]string{"X-Custom-ID": "abc123"},
			expected: "abc123",
		},
		{
			name:     "header hash key - fallback to ip",
			hashKey:  "header:X-Missing",
			headers:  map[string]string{"X-Forwarded-For": "10.0.0.3"},
			expected: "10.0.0.3",
		},
		{
			name:     "unknown hash key defaults to ip",
			hashKey:  "unknown",
			headers:  map[string]string{"X-Forwarded-For": "10.0.0.4"},
			expected: "10.0.0.4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.ProxyConfig{
				Path:        "/api",
				LoadBalance: "consistent_hash",
				HashKey:     tt.hashKey,
				Timeout:     config.ProxyTimeout{Connect: 5 * time.Second},
			}

			targets := []*loadbalance.Target{{URL: "http://localhost:8080"}}
			p, err := NewProxy(cfg, targets, nil, nil)
			if err != nil {
				t.Fatalf("NewProxy() error: %v", err)
			}

			ctx := testutil.NewRequestCtxWithHeader("GET", "/api/test", tt.headers)
			result := p.extractHashKey(ctx, tt.hashKey)
			if result != tt.expected {
				t.Errorf("extractHashKey() = %q, want %q", result, tt.expected)
			}
		})
	}
}

// TestBuildCacheKeyHash 测试缓存键哈希计算
func TestBuildCacheKeyHash(t *testing.T) {
	cfg := &config.ProxyConfig{
		Path:        "/api",
		LoadBalance: "round_robin",
		Timeout:     config.ProxyTimeout{Connect: 5 * time.Second},
	}
	targets := []*loadbalance.Target{{URL: "http://localhost:8080"}}
	p, err := NewProxy(cfg, targets, nil, nil)
	if err != nil {
		t.Fatalf("NewProxy() error: %v", err)
	}

	ctx := testutil.NewRequestCtx("GET", "/api/test")

	hashKey, origKey := p.buildCacheKeyHash(ctx)
	if hashKey == 0 {
		t.Error("buildCacheKeyHash() should return non-zero hash")
	}
	if origKey == "" {
		t.Error("buildCacheKeyHash() should return non-empty origKey")
	}

	// 相同请求应产生相同哈希
	ctx2 := testutil.NewRequestCtx("GET", "/api/test")
	hashKey2, _ := p.buildCacheKeyHash(ctx2)
	if hashKey != hashKey2 {
		t.Error("Same request should produce same hash")
	}

	// 不同请求应产生不同哈希
	ctx3 := testutil.NewRequestCtx("POST", "/api/other")
	hashKey3, _ := p.buildCacheKeyHash(ctx3)
	if hashKey == hashKey3 {
		t.Error("Different request should produce different hash")
	}
}

// TestBuildCacheKeyHashValue 测试零分配缓存键哈希
func TestBuildCacheKeyHashValue(t *testing.T) {
	cfg := &config.ProxyConfig{
		Path:        "/api",
		LoadBalance: "round_robin",
		Timeout:     config.ProxyTimeout{Connect: 5 * time.Second},
	}
	targets := []*loadbalance.Target{{URL: "http://localhost:8080"}}
	p, err := NewProxy(cfg, targets, nil, nil)
	if err != nil {
		t.Fatalf("NewProxy() error: %v", err)
	}

	ctx := testutil.NewRequestCtx("GET", "/api/test")

	hashValue := p.buildCacheKeyHashValue(ctx)
	if hashValue == 0 {
		t.Error("buildCacheKeyHashValue() should return non-zero hash")
	}

	// 应该与 buildCacheKeyHash 结果一致
	hashKey, _ := p.buildCacheKeyHash(ctx)
	if hashValue != hashKey {
		t.Error("buildCacheKeyHashValue() should match buildCacheKeyHash()")
	}
}

// TestWriteCachedResponse 测试缓存响应写入
func TestWriteCachedResponse(t *testing.T) {
	cfg := &config.ProxyConfig{
		Path:        "/api",
		LoadBalance: "round_robin",
		Timeout:     config.ProxyTimeout{Connect: 5 * time.Second},
	}
	targets := []*loadbalance.Target{{URL: "http://localhost:8080"}}
	p, err := NewProxy(cfg, targets, nil, nil)
	if err != nil {
		t.Fatalf("NewProxy() error: %v", err)
	}

	// 手动创建一个 Response 用于验证 writeCachedResponse 写入正确
	ctx := testutil.NewRequestCtx("GET", "/api/test")

	entry := &cache.ProxyCacheEntry{
		Data:    []byte("cached body"),
		Headers: map[string]string{"Content-Type": "text/html", "X-Cached": "true"},
		Status:  200,
	}

	p.writeCachedResponse(ctx, entry)

	if ctx.Response.StatusCode() != 200 {
		t.Errorf("writeCachedResponse() status = %d, want 200", ctx.Response.StatusCode())
	}
	if string(ctx.Response.Body()) != "cached body" {
		t.Errorf("writeCachedResponse() body = %q, want %q", string(ctx.Response.Body()), "cached body")
	}
	ct := string(ctx.Response.Header.Peek("Content-Type"))
	if ct != "text/html" {
		t.Errorf("writeCachedResponse() Content-Type = %q, want %q", ct, "text/html")
	}
	xc := string(ctx.Response.Header.Peek("X-Cache"))
	if xc != "HIT" {
		t.Errorf("writeCachedResponse() X-Cache = %q, want HIT", xc)
	}
}

// TestGetCache 测试 GetCache 方法
func TestGetCache(t *testing.T) {
	// 启用缓存时
	cfg := &config.ProxyConfig{
		Path:        "/api",
		LoadBalance: "round_robin",
		Timeout:     config.ProxyTimeout{Connect: 5 * time.Second},
		Cache: config.ProxyCacheConfig{
			Enabled: true,
			MaxAge:  10 * time.Second,
		},
	}
	targets := []*loadbalance.Target{{URL: "http://localhost:8080"}}
	p, err := NewProxy(cfg, targets, nil, nil)
	if err != nil {
		t.Fatalf("NewProxy() error: %v", err)
	}

	c := p.GetCache()
	if c == nil {
		t.Error("GetCache() should return non-nil when cache enabled")
	}

	// 禁用缓存时
	cfg2 := &config.ProxyConfig{
		Path:        "/api",
		LoadBalance: "round_robin",
		Timeout:     config.ProxyTimeout{Connect: 5 * time.Second},
	}
	p2, _ := NewProxy(cfg2, targets, nil, nil)
	c2 := p2.GetCache()
	if c2 != nil {
		t.Error("GetCache() should return nil when cache disabled")
	}
}

// TestGetCacheStats 测试 GetCacheStats 方法
func TestGetCacheStats(t *testing.T) {
	// 启用缓存时
	cfg := &config.ProxyConfig{
		Path:        "/api",
		LoadBalance: "round_robin",
		Timeout:     config.ProxyTimeout{Connect: 5 * time.Second},
		Cache: config.ProxyCacheConfig{
			Enabled: true,
			MaxAge:  10 * time.Second,
		},
	}
	targets := []*loadbalance.Target{{URL: "http://localhost:8080"}}
	p, err := NewProxy(cfg, targets, nil, nil)
	if err != nil {
		t.Fatalf("NewProxy() error: %v", err)
	}

	stats := p.GetCacheStats()
	if stats == nil {
		t.Error("GetCacheStats() should return non-nil when cache enabled")
	}

	// 禁用缓存时
	cfg2 := &config.ProxyConfig{
		Path:        "/api",
		LoadBalance: "round_robin",
		Timeout:     config.ProxyTimeout{Connect: 5 * time.Second},
	}
	p2, _ := NewProxy(cfg2, targets, nil, nil)
	stats2 := p2.GetCacheStats()
	if stats2 != nil {
		t.Error("GetCacheStats() should return nil when cache disabled")
	}
}

// TestGetCacheDuration 测试不同状态码的缓存时间计算
func TestGetCacheDuration(t *testing.T) {
	tests := []struct {
		name       string
		cacheValid *config.ProxyCacheValidConfig
		maxAge     time.Duration
		statusCode int
		expected   time.Duration
	}{
		{
			name:       "no CacheValid config uses MaxAge",
			maxAge:     5 * time.Minute,
			statusCode: 200,
			expected:   5 * time.Minute,
		},
		{
			name: "2xx with CacheValid.OK set",
			cacheValid: &config.ProxyCacheValidConfig{
				OK: 10 * time.Minute,
			},
			statusCode: 200,
			expected:   10 * time.Minute,
		},
		{
			name: "2xx with CacheValid.OK=0 inherits MaxAge",
			cacheValid: &config.ProxyCacheValidConfig{
				OK: 0,
			},
			maxAge:     3 * time.Minute,
			statusCode: 201,
			expected:   3 * time.Minute,
		},
		{
			name: "301 redirect",
			cacheValid: &config.ProxyCacheValidConfig{
				Redirect: 1 * time.Hour,
			},
			statusCode: 301,
			expected:   1 * time.Hour,
		},
		{
			name: "302 redirect",
			cacheValid: &config.ProxyCacheValidConfig{
				Redirect: 30 * time.Minute,
			},
			statusCode: 302,
			expected:   30 * time.Minute,
		},
		{
			name: "302 with zero Redirect means no cache",
			cacheValid: &config.ProxyCacheValidConfig{
				Redirect: 0,
			},
			statusCode: 302,
			expected:   0,
		},
		{
			name: "404",
			cacheValid: &config.ProxyCacheValidConfig{
				NotFound: 1 * time.Minute,
			},
			statusCode: 404,
			expected:   1 * time.Minute,
		},
		{
			name: "4xx client error",
			cacheValid: &config.ProxyCacheValidConfig{
				ClientError: 30 * time.Second,
			},
			statusCode: 400,
			expected:   30 * time.Second,
		},
		{
			name: "5xx server error",
			cacheValid: &config.ProxyCacheValidConfig{
				ServerError: 0,
			},
			statusCode: 500,
			expected:   0,
		},
		{
			name:       "other status code",
			statusCode: 100,
			expected:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.ProxyConfig{
				Path:        "/api",
				LoadBalance: "round_robin",
				Timeout:     config.ProxyTimeout{Connect: 5 * time.Second},
				Cache: config.ProxyCacheConfig{
					Enabled: true,
					MaxAge:  tt.maxAge,
				},
				CacheValid: tt.cacheValid,
			}
			targets := []*loadbalance.Target{{URL: "http://localhost:8080"}}
			p, err := NewProxy(cfg, targets, nil, nil)
			if err != nil {
				t.Fatalf("NewProxy() error: %v", err)
			}

			duration := p.getCacheDuration(tt.statusCode)
			if duration != tt.expected {
				t.Errorf("getCacheDuration(%d) = %v, want %v", tt.statusCode, duration, tt.expected)
			}
		})
	}
}

// TestBackgroundRefresh 测试后台缓存刷新（标记为 skip 因为需要真实网络）
func TestBackgroundRefresh(t *testing.T) {
	t.Skip("skipping: requires real network connection and is timing-sensitive")
	ln := fasthttputil.NewInmemoryListener()
	defer func() { _ = ln.Close() }()

	go func() {
		s := &fasthttp.Server{
			Handler: func(ctx *fasthttp.RequestCtx) {
				ctx.SetStatusCode(200)
				ctx.SetBodyString("refreshed")
				ctx.Response.Header.Set("Content-Type", "text/plain")
			},
		}
		_ = s.Serve(ln)
	}()
	time.Sleep(10 * time.Millisecond)

	addr := ln.Addr().String()

	cfg := &config.ProxyConfig{
		Path:        "/api",
		LoadBalance: "round_robin",
		Timeout:     config.ProxyTimeout{Connect: 5 * time.Second},
		Cache: config.ProxyCacheConfig{
			Enabled: true,
			MaxAge:  10 * time.Second,
		},
	}
	targets := []*loadbalance.Target{
		{URL: "http://" + addr},
	}
	targets[0].Healthy.Store(true)

	p, err := NewProxy(cfg, targets, nil, nil)
	if err != nil {
		t.Fatalf("NewProxy() error: %v", err)
	}

	ctx := testutil.NewRequestCtx("GET", "/api/test")

	// 设置缓存锁
	hashKey := uint64(12345)
	p.cache.AcquireLock(hashKey)

	// 调用后台刷新（它会执行实际请求来刷新缓存）
	done := make(chan struct{})
	go func() {
		p.backgroundRefresh(ctx, targets[0], hashKey, "/api/test")
		close(done)
	}()

	// 等待完成
	select {
	case <-done:
		// 完成
	case <-time.After(2 * time.Second):
		t.Fatal("backgroundRefresh() timed out")
	}
}

// TestBackgroundRefresh_NoClient 测试后台刷新时客户端不存在的情况
func TestBackgroundRefresh_NoClient(t *testing.T) {
	cfg := &config.ProxyConfig{
		Path:        "/api",
		LoadBalance: "round_robin",
		Timeout:     config.ProxyTimeout{Connect: 5 * time.Second},
		Cache: config.ProxyCacheConfig{
			Enabled: true,
			MaxAge:  10 * time.Second,
		},
	}
	targets := []*loadbalance.Target{
		{URL: "http://nonexistent:9999"},
	}
	targets[0].Healthy.Store(true)

	p, err := NewProxy(cfg, targets, nil, nil)
	if err != nil {
		t.Fatalf("NewProxy() error: %v", err)
	}

	// 移除客户端
	p.mu.Lock()
	delete(p.clients, targets[0].URL)
	p.mu.Unlock()

	ctx := testutil.NewRequestCtx("GET", "/api/test")
	hashKey := uint64(99999)
	p.cache.AcquireLock(hashKey)

	// 应该不会 panic，直接返回
	p.backgroundRefresh(ctx, targets[0], hashKey, "/api/test")
}

// TestServeHTTP_CacheHit 测试缓存命中路径
func TestServeHTTP_CacheHit(t *testing.T) {
	cfg := &config.ProxyConfig{
		Path:        "/api",
		LoadBalance: "round_robin",
		Timeout:     config.ProxyTimeout{Connect: 5 * time.Second},
		Cache: config.ProxyCacheConfig{
			Enabled: true,
			MaxAge:  10 * time.Second,
		},
	}
	targets := []*loadbalance.Target{{URL: "http://localhost:8080"}}
	targets[0].Healthy.Store(true)

	p, err := NewProxy(cfg, targets, nil, nil)
	if err != nil {
		t.Fatalf("NewProxy() error: %v", err)
	}

	// 预填充缓存
	ctx := testutil.NewRequestCtx("GET", "/api/cached")
	hashKey, origKey := p.buildCacheKeyHash(ctx)
	p.cache.Set(hashKey, origKey, []byte("cached!"), map[string]string{
		"Content-Type": "text/plain",
	}, 200, 10*time.Second)

	// 执行请求
	p.ServeHTTP(ctx)

	// 应该返回缓存的响应
	if ctx.Response.StatusCode() != 200 {
		t.Errorf("ServeHTTP() status = %d, want 200", ctx.Response.StatusCode())
	}
	if string(ctx.Response.Body()) != "cached!" {
		t.Errorf("ServeHTTP() body = %q, want %q", string(ctx.Response.Body()), "cached!")
	}
	xc := string(ctx.Response.Header.Peek("X-Cache"))
	if xc != "HIT" {
		t.Errorf("ServeHTTP() X-Cache = %q, want HIT", xc)
	}
}

// TestServeHTTP_ClientNil 测试客户端为 nil 时的行为
func TestServeHTTP_ClientNil(t *testing.T) {
	cfg := &config.ProxyConfig{
		Path:        "/api",
		LoadBalance: "round_robin",
		Timeout:     config.ProxyTimeout{Connect: 5 * time.Second},
		NextUpstream: config.NextUpstreamConfig{
			Tries: 2,
		},
	}
	targets := []*loadbalance.Target{
		{URL: "http://backend1:8080"},
		{URL: "http://backend2:8080"},
	}
	for _, target := range targets {
		target.Healthy.Store(true)
	}

	p, err := NewProxy(cfg, targets, nil, nil)
	if err != nil {
		t.Fatalf("NewProxy() error: %v", err)
	}

	// 移除所有客户端
	p.mu.Lock()
	p.clients = make(map[string]*fasthttp.HostClient)
	p.mu.Unlock()

	ctx := testutil.NewRequestCtx("GET", "/api/test")
	p.ServeHTTP(ctx)

	// 所有客户端都不存在，应该返回 502
	if ctx.Response.StatusCode() != fasthttp.StatusBadGateway {
		t.Errorf("ServeHTTP() status = %d, want 502", ctx.Response.StatusCode())
	}
}

// TestServeHTTP_WithRedirectRewrite 测试带 redirect_rewrite 的缓存命中
func TestServeHTTP_WithRedirectRewrite_CacheHit(t *testing.T) {
	cfg := &config.ProxyConfig{
		Path:        "/api",
		LoadBalance: "round_robin",
		Timeout:     config.ProxyTimeout{Connect: 5 * time.Second},
		Cache: config.ProxyCacheConfig{
			Enabled: true,
			MaxAge:  10 * time.Second,
		},
		RedirectRewrite: &config.RedirectRewriteConfig{
			Mode: "off", // 关闭改写
		},
	}
	targets := []*loadbalance.Target{{URL: "http://localhost:8080"}}
	targets[0].Healthy.Store(true)

	p, err := NewProxy(cfg, targets, nil, nil)
	if err != nil {
		t.Fatalf("NewProxy() error: %v", err)
	}

	ctx := testutil.NewRequestCtx("GET", "/api/test")
	hashKey, origKey := p.buildCacheKeyHash(ctx)
	p.cache.Set(hashKey, origKey, []byte("ok"), map[string]string{
		"Content-Type": "text/plain",
	}, 200, 10*time.Second)

	p.ServeHTTP(ctx)

	if ctx.Response.StatusCode() != 200 {
		t.Errorf("ServeHTTP() status = %d, want 200", ctx.Response.StatusCode())
	}
}
