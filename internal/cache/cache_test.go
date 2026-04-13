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

	pc := NewProxyCache(rules, true, 60*time.Second)
	if pc == nil {
		t.Error("Expected non-nil ProxyCache")
	}
}

func TestProxyCacheSetGet(t *testing.T) {
	pc := NewProxyCache(nil, false, 0)

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
	pc := NewProxyCache(nil, false, 0)

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
	pc := NewProxyCache(nil, false, 200*time.Millisecond)

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
	pc := NewProxyCache(nil, true, 0)

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

	pc := NewProxyCache(rules, false, 0)

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
	pc := NewProxyCache(nil, false, 0)

	key := "key1"
	pc.Set(hashKey(key), key, []byte("data"), nil, 200, 10*time.Minute)
	pc.Delete(hashKey(key))

	_, ok, _ := pc.Get(hashKey(key), key)
	if ok {
		t.Error("Expected entry to be deleted")
	}
}

func TestProxyCacheClear(t *testing.T) {
	pc := NewProxyCache(nil, false, 0)

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
