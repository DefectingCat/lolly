// Package cache 提供缓存功能的测试。
//
// 该文件测试缓存模块的各项功能，包括：
//   - 文件缓存创建和配置
//   - 代理缓存规则和匹配
//   - 缓存设置和获取
//   - 过期和淘汰策略
//   - 路径匹配功能
//
// 作者：xfy
package cache

import (
	"hash/fnv"
	"testing"
	"time"
)

// hashKey 计算字符串的 FNV-64a 哈希值，用于测试。
func hashKey(s string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(s))
	return h.Sum64()
}

func TestNewFileCache(t *testing.T) {
	fc := NewFileCache(100, 1024*1024, 30*time.Second)
	if fc == nil {
		t.Error("Expected non-nil FileCache")
	}
}

func TestFileCacheSetGet(t *testing.T) {
	fc := NewFileCache(10, 1024, 1*time.Hour)

	path := "/test/file.txt"
	data := []byte("Hello, World!")

	err := fc.Set(path, data, int64(len(data)), time.Now())
	if err != nil {
		t.Errorf("Set() error: %v", err)
	}

	entry, ok := fc.Get(path)
	if !ok {
		t.Error("Expected to find cached entry")
	}
	if string(entry.Data) != "Hello, World!" {
		t.Errorf("Expected data 'Hello, World!', got %s", entry.Data)
	}
}

func TestFileCacheDelete(t *testing.T) {
	fc := NewFileCache(10, 1024, 1*time.Hour)

	_ = fc.Set("/test.txt", []byte("data"), 4, time.Now())

	fc.Delete("/test.txt")

	_, ok := fc.Get("/test.txt")
	if ok {
		t.Error("Expected entry to be deleted")
	}
}

func TestFileCacheLRUEviction(t *testing.T) {
	// 最大 3 个条目
	fc := NewFileCache(3, 0, 1*time.Hour)

	_ = fc.Set("/a", []byte("a"), 1, time.Now())
	_ = fc.Set("/b", []byte("b"), 1, time.Now())
	_ = fc.Set("/c", []byte("c"), 1, time.Now())

	// 再添加一个，应该淘汰 /a
	_ = fc.Set("/d", []byte("d"), 1, time.Now())

	_, ok := fc.Get("/a")
	if ok {
		t.Error("Expected /a to be evicted")
	}

	// b, c, d 应该还在
	for _, path := range []string{"b", "c", "d"} {
		_, ok := fc.Get("/" + path)
		if !ok {
			t.Errorf("Expected /%s to exist", path)
		}
	}
}

func TestAcquireLockWithTimeout(t *testing.T) {
	pc := NewProxyCache(nil, true, 0, 0, 0)
	key := hashKey("timeout-test")

	// 测试获取锁
	waitCh, timedOut := pc.AcquireLockWithTimeout(key, 100*time.Millisecond)
	if waitCh != nil || timedOut {
		t.Error("Expected to acquire lock immediately")
	}

	// 测试等待超时
	done := make(chan struct{})
	go func() {
		time.Sleep(200 * time.Millisecond)
		pc.ReleaseLock(key, nil)
		close(done)
	}()

	_, timedOut = pc.AcquireLockWithTimeout(key, 50*time.Millisecond)
	if !timedOut {
		t.Error("Expected timeout when waiting for lock")
	}
	<-done
}

func TestRefreshTTL(t *testing.T) {
	pc := NewProxyCache(nil, false, 0, 0, 0)
	key := hashKey("refresh-test")
	origKey := "refresh-test"

	pc.Set(key, origKey, []byte("data"), nil, 200, 10*time.Minute)

	newHeaders := map[string]string{
		"Last-Modified": "Wed, 21 Oct 2015 07:28:00 GMT",
		"ETag":          "\"abc123\"",
	}

	ok := pc.RefreshTTL(key, origKey, newHeaders)
	if !ok {
		t.Error("Expected RefreshTTL to succeed")
	}

	entry, _, _ := pc.Get(key, origKey)
	if entry.LastModified != newHeaders["Last-Modified"] {
		t.Errorf("Expected Last-Modified to be updated")
	}
	if entry.ETag != newHeaders["ETag"] {
		t.Errorf("Expected ETag to be updated")
	}
}

func TestSetValidationHeaders(t *testing.T) {
	pc := NewProxyCache(nil, false, 0, 0, 0)
	key := hashKey("validation-test")
	origKey := "validation-test"

	pc.Set(key, origKey, []byte("data"), nil, 200, 10*time.Minute)

	ok := pc.SetValidationHeaders(key, origKey, "Mon, 01 Jan 2024 00:00:00 GMT", "\"xyz789\"")
	if !ok {
		t.Error("Expected SetValidationHeaders to succeed")
	}

	entry, _, _ := pc.Get(key, origKey)
	if entry.LastModified != "Mon, 01 Jan 2024 00:00:00 GMT" {
		t.Errorf("Expected LastModified to be set")
	}
	if entry.ETag != "\"xyz789\"" {
		t.Errorf("Expected ETag to be set")
	}
}

func TestMatchRulePathVariants(t *testing.T) {
	tests := []struct {
		name     string
		rulePath string
		reqPath  string
		want     bool
	}{
		{"prefix_match", "/api/", "/api/users", true},
		{"prefix_no_match", "/api/", "/other", false},
		{"wildcard_match", "/static/*", "/static/css/style.css", true},
		{"wildcard_no_match", "/static/*", "/api/users", false},
		{"exact_match", "/api", "/api", true},
		{"prefix_with_slash", "/api", "/api/users", true},
		{"prefix_with_query", "/api", "/api?query=value", true},
		{"no_match_similar", "/api", "/apiother", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rules := []ProxyCacheRule{
				{Path: tt.rulePath, Methods: []string{"GET"}, MaxAge: time.Minute},
			}
			pc := NewProxyCache(rules, false, 0, 0, 0)
			rule := pc.MatchRule(tt.reqPath, "GET", 0)
			if (rule != nil) != tt.want {
				t.Errorf("MatchRule(%s, %s) = %v, want %v", tt.rulePath, tt.reqPath, rule != nil, tt.want)
			}
		})
	}
}

func TestMinUsesThreshold(t *testing.T) {
	pc := NewProxyCache(nil, false, 0, 0, 0)
	key := hashKey("minuses-test")
	origKey := "minuses-test"

	// 首次设置
	pc.Set(key, origKey, []byte("data"), nil, 200, 10*time.Minute)

	// 首次 Get，Uses = 1
	entry, _, _ := pc.Get(key, origKey)
	if entry.Uses.Load() != 1 {
		t.Errorf("Expected Uses=1 after first Get, got %d", entry.Uses.Load())
	}

	// 第二次 Get，Uses = 2
	pc.Get(key, origKey)
	if entry.Uses.Load() != 2 {
		t.Errorf("Expected Uses=2 after second Get, got %d", entry.Uses.Load())
	}
}

func TestFileCacheSizeEviction(t *testing.T) {
	// 最大 10 字节
	fc := NewFileCache(0, 10, 1*time.Hour)

	_ = fc.Set("/a", []byte("12345"), 5, time.Now())
	_ = fc.Set("/b", []byte("12345"), 5, time.Now())

	// 再添加 6 字节，应该淘汰一个
	_ = fc.Set("/c", []byte("123456"), 6, time.Now())

	stats := fc.Stats()
	if stats.Size > 10 {
		t.Errorf("Expected size <= 10, got %d", stats.Size)
	}
}

func TestFileCacheInactiveEviction(t *testing.T) {
	fc := NewFileCache(10, 1024, 100*time.Millisecond)

	_ = fc.Set("/test", []byte("data"), 4, time.Now())

	// 立即获取应该成功
	_, ok := fc.Get("/test")
	if !ok {
		t.Error("Expected entry to exist")
	}

	// 等待过期
	time.Sleep(150 * time.Millisecond)

	// 再次获取应该失败（因过期被删除）
	_, ok = fc.Get("/test")
	if ok {
		t.Error("Expected entry to be expired")
	}
}

func TestFileCacheClear(t *testing.T) {
	fc := NewFileCache(10, 1024, 1*time.Hour)

	_ = fc.Set("/a", []byte("a"), 1, time.Now())
	_ = fc.Set("/b", []byte("b"), 1, time.Now())

	fc.Clear()

	stats := fc.Stats()
	if stats.Entries != 0 {
		t.Errorf("Expected 0 entries after clear, got %d", stats.Entries)
	}
}

func TestFileCacheStats(t *testing.T) {
	fc := NewFileCache(100, 1024, 1*time.Hour)

	_ = fc.Set("/a", []byte("12345"), 5, time.Now())
	_ = fc.Set("/b", []byte("12345"), 5, time.Now())

	stats := fc.Stats()
	if stats.Entries != 2 {
		t.Errorf("Expected 2 entries, got %d", stats.Entries)
	}
	if stats.Size != 10 {
		t.Errorf("Expected size 10, got %d", stats.Size)
	}
}

func TestNewProxyCache(t *testing.T) {
	rules := []ProxyCacheRule{
		{Path: "/api/", Methods: []string{"GET"}, MaxAge: 10 * time.Minute},
	}

	pc := NewProxyCache(rules, true, 60*time.Second, 0, 0)
	if pc == nil {
		t.Error("Expected non-nil ProxyCache")
	}
}

func TestProxyCacheSetGet(t *testing.T) {
	pc := NewProxyCache(nil, false, 0, 0, 0)

	key := "test-key"
	data := []byte("response body")
	headers := map[string]string{"Content-Type": "application/json"}

	pc.Set(hashKey(key), key, data, headers, 200, 10*time.Minute)

	entry, ok, stale := pc.Get(hashKey(key), key)
	if !ok {
		t.Error("Expected to find cached entry")
	}
	if stale {
		t.Error("Expected entry to be fresh")
	}
	if string(entry.Data) != "response body" {
		t.Errorf("Expected data 'response body', got %s", entry.Data)
	}
	if entry.Status != 200 {
		t.Errorf("Expected status 200, got %d", entry.Status)
	}
}

func TestProxyCacheExpiration(t *testing.T) {
	pc := NewProxyCache(nil, false, 0, 0, 0)

	key := "expire-test"
	pc.Set(hashKey(key), key, []byte("data"), nil, 200, 100*time.Millisecond)

	// 立即获取应该成功
	_, ok, _ := pc.Get(hashKey(key), key)
	if !ok {
		t.Error("Expected entry to exist")
	}

	// 等待过期
	time.Sleep(150 * time.Millisecond)

	_, ok, _ = pc.Get(hashKey(key), key)
	if ok {
		t.Error("Expected entry to be expired")
	}
}

func TestProxyCacheStaleWhileRevalidate(t *testing.T) {
	pc := NewProxyCache(nil, false, 200*time.Millisecond, 0, 0)

	key := "stale-test"
	pc.Set(hashKey(key), key, []byte("data"), nil, 200, 100*time.Millisecond)

	// 等待过期但仍在 stale 时间内
	time.Sleep(150 * time.Millisecond)

	entry, ok, stale := pc.Get(hashKey(key), key)
	if !ok {
		t.Error("Expected stale entry to be usable")
	}
	if !stale {
		t.Error("Expected entry to be marked as stale")
	}
	if entry == nil {
		t.Error("Expected stale entry data")
	}
}

func TestProxyCacheLock(t *testing.T) {
	pc := NewProxyCache(nil, true, 0, 0, 0)

	key := "lock-test"

	// 获取锁
	ch := pc.AcquireLock(hashKey(key))
	if ch != nil {
		t.Error("Expected to acquire lock (nil chan)")
	}

	// 第二次获取应该返回等待 chan
	ch2 := pc.AcquireLock(hashKey(key))
	if ch2 == nil {
		t.Error("Expected waiting chan when lock is held")
	}

	// 设置缓存并释放锁
	pc.Set(hashKey(key), key, []byte("data"), nil, 200, 10*time.Minute)

	// 现在应该能获取缓存
	_, ok, _ := pc.Get(hashKey(key), key)
	if !ok {
		t.Error("Expected cache entry after lock release")
	}
}

func TestProxyCacheMatchRule(t *testing.T) {
	rules := []ProxyCacheRule{
		{Path: "/api/", Methods: []string{"GET"}, Statuses: []int{200}, MaxAge: 10 * time.Minute},
		{Path: "/static/*", Methods: []string{"GET"}, MaxAge: 1 * time.Hour},
	}

	pc := NewProxyCache(rules, false, 0, 0, 0)

	tests := []struct {
		path   string
		method string
		status int
		want   bool
	}{
		{"api/users", "GET", 200, true},
		{"api/users", "POST", 200, false}, // POST 不在 Methods
		{"api/users", "GET", 404, false},  // 404 不在 Statuses
		{"static/css/style.css", "GET", 200, true},
		{"other/path", "GET", 200, false}, // 不匹配任何规则
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			// 添加前缀 / 到 path
			fullPath := "/" + tt.path
			rule := pc.MatchRule(fullPath, tt.method, tt.status)
			if (rule != nil) != tt.want {
				t.Errorf("MatchRule(%s, %s, %d) want %v", fullPath, tt.method, tt.status, tt.want)
			}
		})
	}
}

func TestProxyCacheDelete(t *testing.T) {
	pc := NewProxyCache(nil, false, 0, 0, 0)

	key := "key1"
	pc.Set(hashKey(key), key, []byte("data"), nil, 200, 10*time.Minute)
	pc.Delete(hashKey(key))

	_, ok, _ := pc.Get(hashKey(key), key)
	if ok {
		t.Error("Expected entry to be deleted")
	}
}

func TestProxyCacheClear(t *testing.T) {
	pc := NewProxyCache(nil, false, 0, 0, 0)

	pc.Set(hashKey("a"), "a", []byte("a"), nil, 200, 10*time.Minute)
	pc.Set(hashKey("b"), "b", []byte("b"), nil, 200, 10*time.Minute)

	pc.Clear()

	stats := pc.Stats()
	if stats.Entries != 0 {
		t.Errorf("Expected 0 entries, got %d", stats.Entries)
	}
}

func TestPathMatch(t *testing.T) {
	tests := []struct {
		pattern string
		path    string
		want    bool
	}{
		{"*", "/anything", true},
		{"api/*", "/api/users", true},
		{"api/*", "/api/", true},
		{"api/*", "/other", false},
		{"/exact", "/exact", true},
		{"/exact", "/exact/other", false},
	}

	for _, tt := range tests {
		t.Run(tt.pattern+"_"+tt.path, func(t *testing.T) {
			// 添加前缀 / 如果 pattern 没有
			pattern := tt.pattern
			if pattern[0] != '/' && pattern != "*" {
				pattern = "/" + pattern
			}
			result := MatchPattern(pattern, tt.path)
			if result != tt.want {
				t.Errorf("MatchPattern(%s, %s) = %v, want %v", pattern, tt.path, result, tt.want)
			}
		})
	}
}

func TestProxyCacheGetStaleIfError(t *testing.T) {
	pc := NewProxyCache(nil, false, 0, 200*time.Millisecond, 0)

	key := "stale-error-test"
	pc.Set(hashKey(key), key, []byte("data"), nil, 200, 100*time.Millisecond)

	// 等待过期但仍在 stale_if_error 窗口内
	time.Sleep(150 * time.Millisecond)

	// isTimeout=false，应该使用 staleIfError 窗口
	entry, ok := pc.GetStale(hashKey(key), key, false)
	if !ok {
		t.Error("stale entry should be usable on error")
	}
	if entry == nil {
		t.Error("expected stale entry data")
	}
	if string(entry.Data) != "data" {
		t.Errorf("entry.Data = %q, want %q", entry.Data, "data")
	}

	// isTimeout=true，staleIfTimeout=0，不应该可用
	if _, ok2 := pc.GetStale(hashKey(key), key, true); ok2 {
		t.Error("stale entry should NOT be usable on timeout when staleIfTimeout=0")
	}
}

func TestProxyCacheGetStaleIfTimeout(t *testing.T) {
	pc := NewProxyCache(nil, false, 0, 0, 300*time.Millisecond)

	key := "stale-timeout-test"
	pc.Set(hashKey(key), key, []byte("data"), nil, 200, 100*time.Millisecond)

	// 等待过期但仍在 stale_if_timeout 窗口内
	time.Sleep(250 * time.Millisecond)

	// isTimeout=true，应该使用 staleIfTimeout 窗口
	entry, ok := pc.GetStale(hashKey(key), key, true)
	if !ok {
		t.Error("stale entry should be usable on timeout")
	}
	if entry == nil {
		t.Error("expected stale entry data")
	}

	// isTimeout=false，staleIfError=0，不应该可用
	if _, ok2 := pc.GetStale(hashKey(key), key, false); ok2 {
		t.Error("stale entry should NOT be usable on error when staleIfError=0")
	}
}

func TestProxyCacheGetStaleExpired(t *testing.T) {
	pc := NewProxyCache(nil, false, 0, 100*time.Millisecond, 100*time.Millisecond)

	key := "stale-expired-test"
	pc.Set(hashKey(key), key, []byte("data"), nil, 200, 50*time.Millisecond)

	// 等待超过 stale 窗口
	time.Sleep(200 * time.Millisecond)

	// 两种情况都不应该可用
	if _, ok := pc.GetStale(hashKey(key), key, false); ok {
		t.Error("stale entry should NOT be usable after stale window expired")
	}

	if _, ok2 := pc.GetStale(hashKey(key), key, true); ok2 {
		t.Error("stale entry should NOT be usable on timeout after stale window expired")
	}
}

func TestProxyCacheGetStaleNotExpired(t *testing.T) {
	pc := NewProxyCache(nil, false, 0, 100*time.Millisecond, 100*time.Millisecond)

	key := "stale-fresh-test"
	pc.Set(hashKey(key), key, []byte("data"), nil, 200, 200*time.Millisecond)

	// 未过期，两种情况都应该可用（返回新鲜数据）
	entry, ok := pc.GetStale(hashKey(key), key, false)
	if !ok {
		t.Error("fresh entry should be usable")
	}
	if string(entry.Data) != "data" {
		t.Errorf("entry.Data = %q, want %q", entry.Data, "data")
	}

	if _, ok2 := pc.GetStale(hashKey(key), key, true); !ok2 {
		t.Error("fresh entry should be usable on timeout")
	}
}

// TestFileCacheRefreshCachedAt 测试 RefreshCachedAt 方法。
func TestFileCacheRefreshCachedAt(t *testing.T) {
	fc := NewFileCache(10, 1024, 1*time.Hour)

	path := "/test/refresh.txt"
	data := []byte("test data")

	// 设置缓存
	_ = fc.Set(path, data, int64(len(data)), time.Now())

	// 获取原始 CachedAt 时间
	entry, ok := fc.Get(path)
	if !ok {
		t.Fatal("Expected to find cached entry")
	}
	originalCachedAt := entry.CachedAt

	// 等待一小段时间
	time.Sleep(10 * time.Millisecond)

	// 刷新 CachedAt
	fc.RefreshCachedAt(path)

	// 再次获取，验证 CachedAt 已更新
	entry, ok = fc.Get(path)
	if !ok {
		t.Fatal("Expected to find cached entry after refresh")
	}
	if !entry.CachedAt.After(originalCachedAt) {
		t.Errorf("CachedAt not updated: %v <= %v", entry.CachedAt, originalCachedAt)
	}
}

// TestFileCacheRefreshCachedAtNonExistent 测试刷新不存在的条目。
func TestFileCacheRefreshCachedAtNonExistent(t *testing.T) {
	fc := NewFileCache(10, 1024, 1*time.Hour)

	// 刷新不存在的条目应该静默忽略
	fc.RefreshCachedAt("/nonexistent/path")

	// 验证没有副作用
	stats := fc.Stats()
	if stats.Entries != 0 {
		t.Errorf("Expected 0 entries, got %d", stats.Entries)
	}
}

// TestProxyCacheDeleteByPatternWithMethod 测试按模式和方法的删除。
func TestProxyCacheDeleteByPatternWithMethod(t *testing.T) {
	pc := NewProxyCache(nil, false, 0, 0, 0)

	// 添加多个缓存条目，带不同方法前缀
	pc.Set(hashKey("GET:/api/users"), "GET:/api/users", []byte("users"), nil, 200, 10*time.Minute)
	pc.Set(hashKey("POST:/api/users"), "POST:/api/users", []byte("create"), nil, 200, 10*time.Minute)
	pc.Set(hashKey("GET:/api/posts"), "GET:/api/posts", []byte("posts"), nil, 200, 10*time.Minute)
	pc.Set(hashKey("DELETE:/api/users/1"), "DELETE:/api/users/1", []byte("delete"), nil, 200, 10*time.Minute)

	// 删除所有 GET:/api/users* 的条目（模式匹配 OrigKey，包含方法前缀）
	deleted := pc.DeleteByPatternWithMethod("GET:/api/users*", "GET")
	if deleted != 1 {
		t.Errorf("Expected 1 deleted, got %d", deleted)
	}

	// 验证 GET:/api/users 被删除
	if _, ok, _ := pc.Get(hashKey("GET:/api/users"), "GET:/api/users"); ok {
		t.Error("GET:/api/users should be deleted")
	}

	// 验证 POST:/api/users 还在
	if _, ok, _ := pc.Get(hashKey("POST:/api/users"), "POST:/api/users"); !ok {
		t.Error("POST:/api/users should still exist")
	}

	// 验证 GET:/api/posts 还在
	if _, ok, _ := pc.Get(hashKey("GET:/api/posts"), "GET:/api/posts"); !ok {
		t.Error("GET:/api/posts should still exist")
	}
}

// TestProxyCacheDeleteByPatternAllMethods 测试删除所有方法。
func TestProxyCacheDeleteByPatternAllMethods(t *testing.T) {
	pc := NewProxyCache(nil, false, 0, 0, 0)

	// 添加多个缓存条目
	pc.Set(hashKey("GET:/api/test"), "GET:/api/test", []byte("get"), nil, 200, 10*time.Minute)
	pc.Set(hashKey("POST:/api/test"), "POST:/api/test", []byte("post"), nil, 200, 10*time.Minute)
	pc.Set(hashKey("PUT:/api/test"), "PUT:/api/test", []byte("put"), nil, 200, 10*time.Minute)

	// 删除所有 *:/api/test* 的条目（不限制方法，使用 * 通配符）
	deleted := pc.DeleteByPatternWithMethod("*", "")
	if deleted != 3 {
		t.Errorf("Expected 3 deleted, got %d", deleted)
	}
}

// TestProxyCacheDeleteByPatternNoMatch 测试无匹配删除。
func TestProxyCacheDeleteByPatternNoMatch(t *testing.T) {
	pc := NewProxyCache(nil, false, 0, 0, 0)

	pc.Set(hashKey("GET:/api/users"), "GET:/api/users", []byte("users"), nil, 200, 10*time.Minute)

	// 删除不匹配的模式
	deleted := pc.DeleteByPatternWithMethod("/other/*", "")
	if deleted != 0 {
		t.Errorf("Expected 0 deleted, got %d", deleted)
	}

	// 验证原条目还在
	if _, ok, _ := pc.Get(hashKey("GET:/api/users"), "GET:/api/users"); !ok {
		t.Error("Original entry should still exist")
	}
}

// TestProxyCacheStatsToCacheStats 测试 ProxyCacheStats 转换。
func TestProxyCacheStatsToCacheStats(t *testing.T) {
	stats := ProxyCacheStats{
		Entries: 10,
		Pending: 2,
	}

	cacheStats := stats.ToCacheStats()

	if cacheStats.Entries != 10 {
		t.Errorf("Entries = %d, want 10", cacheStats.Entries)
	}
}

// TestProxyCacheCacheStatsMethod 测试 CacheStats 方法。
func TestProxyCacheCacheStatsMethod(t *testing.T) {
	pc := NewProxyCache(nil, false, 0, 0, 0)

	// 添加缓存条目
	pc.Set(hashKey("key1"), "key1", []byte("data1"), nil, 200, 10*time.Minute)
	pc.Set(hashKey("key2"), "key2", []byte("data2"), nil, 200, 10*time.Minute)

	stats := pc.CacheStats()

	if stats.Entries != 2 {
		t.Errorf("Entries = %d, want 2", stats.Entries)
	}
}
