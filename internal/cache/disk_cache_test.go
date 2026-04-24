package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewDiskCache(t *testing.T) {
	// 创建临时目录
	tmpDir := t.TempDir()

	cfg := &DiskCacheConfig{
		Path:   tmpDir,
		Levels: "1:2",
	}

	dc, err := NewDiskCache(cfg)
	if err != nil {
		t.Fatalf("NewDiskCache failed: %v", err)
	}
	defer dc.Stop()

	// 等待懒加载完成
	<-dc.loadCh

	if dc.basePath != tmpDir {
		t.Errorf("basePath = %q, want %q", dc.basePath, tmpDir)
	}
}

func TestDiskCacheSetGet(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &DiskCacheConfig{
		Path:   tmpDir,
		Levels: "1:2",
	}

	dc, err := NewDiskCache(cfg)
	if err != nil {
		t.Fatalf("NewDiskCache failed: %v", err)
	}
	defer dc.Stop()

	// 等待懒加载完成
	<-dc.loadCh

	// 设置缓存
	hashKey := uint64(12345)
	origKey := "GET:/api/test"
	data := []byte("test response data")
	headers := map[string]string{"Content-Type": "application/json"}
	status := 200
	maxAge := 10 * time.Minute

	dc.Set(hashKey, origKey, data, headers, status, maxAge)

	// 获取缓存
	entry, exists, stale := dc.Get(hashKey, origKey)
	if !exists {
		t.Fatal("cache entry not found")
	}
	if stale {
		t.Error("entry should not be stale")
	}
	if string(entry.Data) != string(data) {
		t.Errorf("Data = %q, want %q", entry.Data, data)
	}
	if entry.Status != status {
		t.Errorf("Status = %d, want %d", entry.Status, status)
	}
	if entry.Headers["Content-Type"] != "application/json" {
		t.Errorf("Headers[Content-Type] = %q, want %q", entry.Headers["Content-Type"], "application/json")
	}
}

func TestDiskCacheDelete(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &DiskCacheConfig{
		Path:   tmpDir,
		Levels: "1:2",
	}

	dc, err := NewDiskCache(cfg)
	if err != nil {
		t.Fatalf("NewDiskCache failed: %v", err)
	}
	defer dc.Stop()

	<-dc.loadCh

	// 设置缓存
	hashKey := uint64(12345)
	origKey := "GET:/api/test"
	dc.Set(hashKey, origKey, []byte("test"), nil, 200, 10*time.Minute)

	// 验证存在
	_, exists, _ := dc.Get(hashKey, origKey)
	if !exists {
		t.Fatal("entry should exist before delete")
	}

	// 删除
	if err := dc.Delete(hashKey); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// 验证已删除
	_, exists, _ = dc.Get(hashKey, origKey)
	if exists {
		t.Error("entry should not exist after delete")
	}
}

func TestDiskCacheStale(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &DiskCacheConfig{
		Path:   tmpDir,
		Levels: "1:2",
	}

	dc, err := NewDiskCache(cfg)
	if err != nil {
		t.Fatalf("NewDiskCache failed: %v", err)
	}
	defer dc.Stop()

	<-dc.loadCh

	// 设置一个已过期的缓存
	hashKey := uint64(12345)
	origKey := "GET:/api/test"
	dc.Set(hashKey, origKey, []byte("test"), nil, 200, 1*time.Millisecond)

	// 等待过期
	time.Sleep(10 * time.Millisecond)

	// 获取缓存
	entry, exists, stale := dc.Get(hashKey, origKey)
	if !exists {
		t.Fatal("expired entry should still exist")
	}
	if !stale {
		t.Error("expired entry should be marked as stale")
	}
	if string(entry.Data) != "test" {
		t.Errorf("Data = %q, want %q", entry.Data, "test")
	}
}

func TestDiskCacheLevels(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &DiskCacheConfig{
		Path:   tmpDir,
		Levels: "1:2",
	}

	dc, err := NewDiskCache(cfg)
	if err != nil {
		t.Fatalf("NewDiskCache failed: %v", err)
	}
	defer dc.Stop()

	<-dc.loadCh

	// 设置缓存
	hashKey := uint64(0xabcdef1234567890)
	origKey := "GET:/api/test"
	dc.Set(hashKey, origKey, []byte("test"), nil, 200, 10*time.Minute)

	// 验证文件路径包含层级目录
	dataPath := dc.filePathFromHash(hashKey, "data")
	if filepath.Dir(dataPath) == tmpDir {
		t.Error("file should be in a subdirectory for levels=1:2")
	}

	// 验证文件存在
	if _, err := os.Stat(dataPath); os.IsNotExist(err) {
		t.Errorf("data file not found at %s", dataPath)
	}
}

func TestDiskCacheMaxSize(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &DiskCacheConfig{
		Path:    tmpDir,
		Levels:  "1:2",
		MaxSize: 100, // 很小的限制
	}

	dc, err := NewDiskCache(cfg)
	if err != nil {
		t.Fatalf("NewDiskCache failed: %v", err)
	}
	defer dc.Stop()

	<-dc.loadCh

	// 设置多个缓存条目
	for i := range 10 {
		hashKey := uint64(i)
		origKey := "GET:/api/test"
		dc.Set(hashKey, origKey, []byte("test data that is longer than 10 bytes"), nil, 200, 10*time.Minute)
	}

	// 等待淘汰完成（淘汰是异步的）
	time.Sleep(500 * time.Millisecond)

	// 验证淘汰发生（Evictions > 0）
	stats := dc.CacheStats()
	if stats.Evictions == 0 {
		t.Error("Evictions should be > 0 when MaxSize is exceeded")
	}
}

func TestDiskCacheStats(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &DiskCacheConfig{
		Path:   tmpDir,
		Levels: "1:2",
	}

	dc, err := NewDiskCache(cfg)
	if err != nil {
		t.Fatalf("NewDiskCache failed: %v", err)
	}
	defer dc.Stop()

	<-dc.loadCh

	// 初始统计
	stats := dc.CacheStats()
	if stats.Entries != 0 {
		t.Errorf("initial Entries = %d, want 0", stats.Entries)
	}

	// 设置缓存
	dc.Set(1, "key1", []byte("data1"), nil, 200, 10*time.Minute)
	dc.Set(2, "key2", []byte("data2"), nil, 200, 10*time.Minute)

	stats = dc.CacheStats()
	if stats.Entries != 2 {
		t.Errorf("Entries = %d, want 2", stats.Entries)
	}

	// 获取缓存（命中）
	dc.Get(1, "key1")
	stats = dc.CacheStats()
	if stats.HitCount != 1 {
		t.Errorf("HitCount = %d, want 1", stats.HitCount)
	}

	// 获取不存在的缓存（未命中）
	dc.Get(999, "nonexistent")
	stats = dc.CacheStats()
	if stats.MissCount != 1 {
		t.Errorf("MissCount = %d, want 1", stats.MissCount)
	}
}

func TestDiskCacheCRC32(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &DiskCacheConfig{
		Path:   tmpDir,
		Levels: "1:2",
	}

	dc, err := NewDiskCache(cfg)
	if err != nil {
		t.Fatalf("NewDiskCache failed: %v", err)
	}
	defer dc.Stop()

	<-dc.loadCh

	// 设置缓存
	hashKey := uint64(12345)
	origKey := "GET:/api/test"
	data := []byte("test data for crc32")
	dc.Set(hashKey, origKey, data, nil, 200, 10*time.Minute)

	// 获取缓存验证 CRC32
	entry, exists, _ := dc.Get(hashKey, origKey)
	if !exists {
		t.Fatal("entry not found")
	}
	if string(entry.Data) != string(data) {
		t.Errorf("Data mismatch")
	}
}

func TestParseLevels(t *testing.T) {
	tests := []struct {
		input    string
		expected []int
	}{
		{"", nil},
		{"1", []int{1}},
		{"1:2", []int{1, 2}},
		{"2:2:2", []int{2, 2, 2}},
	}

	for _, tt := range tests {
		result := parseLevels(tt.input)
		if len(result) != len(tt.expected) {
			t.Errorf("parseLevels(%q) = %v, want %v", tt.input, result, tt.expected)
			continue
		}
		for i, v := range result {
			if v != tt.expected[i] {
				t.Errorf("parseLevels(%q)[%d] = %d, want %d", tt.input, i, v, tt.expected[i])
			}
		}
	}
}

func TestDiskCacheLazyLoad(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &DiskCacheConfig{
		Path:   tmpDir,
		Levels: "1:2",
	}

	dc, err := NewDiskCache(cfg)
	if err != nil {
		t.Fatalf("NewDiskCache failed: %v", err)
	}
	defer dc.Stop()

	// 在懒加载完成前，loaded 应该是 false
	// 但由于加载很快，我们无法可靠测试这个状态
	// 所以我们等待加载完成
	<-dc.loadCh

	if !dc.loaded.Load() {
		t.Error("loaded should be true after lazyLoad completes")
	}
}

func TestDiskCacheRestart(t *testing.T) {
	tmpDir := t.TempDir()

	// 第一个实例：写入数据
	cfg := &DiskCacheConfig{
		Path:   tmpDir,
		Levels: "1:2",
	}

	dc1, err := NewDiskCache(cfg)
	if err != nil {
		t.Fatalf("NewDiskCache failed: %v", err)
	}
	<-dc1.loadCh

	hashKey := uint64(12345)
	origKey := "GET:/api/test"
	data := []byte("persistent data")
	dc1.Set(hashKey, origKey, data, nil, 200, 10*time.Minute)
	dc1.Stop()

	// 第二个实例：读取数据（模拟重启）
	dc2, err := NewDiskCache(cfg)
	if err != nil {
		t.Fatalf("NewDiskCache (restart) failed: %v", err)
	}
	<-dc2.loadCh
	defer dc2.Stop()

	// 验证数据恢复
	entry, exists, _ := dc2.Get(hashKey, origKey)
	if !exists {
		t.Fatal("entry should exist after restart")
	}
	if string(entry.Data) != string(data) {
		t.Errorf("Data = %q, want %q", entry.Data, data)
	}
}

func TestDiskCacheGetStaleIfError(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &DiskCacheConfig{
		Path:           tmpDir,
		Levels:         "1:2",
		StaleIfError:   200 * time.Millisecond,
		StaleIfTimeout: 0,
	}

	dc, err := NewDiskCache(cfg)
	if err != nil {
		t.Fatalf("NewDiskCache failed: %v", err)
	}
	defer dc.Stop()
	<-dc.loadCh

	hashKey := uint64(12345)
	origKey := "GET:/api/test"
	dc.Set(hashKey, origKey, []byte("data"), nil, 200, 100*time.Millisecond)

	// 等待过期但仍在 stale_if_error 窗口内
	time.Sleep(150 * time.Millisecond)

	// isTimeout=false，应该使用 staleIfError 窗口
	entry, ok := dc.GetStale(hashKey, origKey, false)
	if !ok {
		t.Error("stale entry should be usable on error")
	}
	if entry == nil || string(entry.Data) != "data" {
		t.Errorf("entry.Data = %v, want %q", entry, "data")
	}

	// isTimeout=true，staleIfTimeout=0，不应该可用
	if _, ok2 := dc.GetStale(hashKey, origKey, true); ok2 {
		t.Error("stale entry should NOT be usable on timeout when staleIfTimeout=0")
	}
}

func TestDiskCacheGetStaleIfTimeout(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &DiskCacheConfig{
		Path:           tmpDir,
		Levels:         "1:2",
		StaleIfError:   0,
		StaleIfTimeout: 300 * time.Millisecond,
	}

	dc, err := NewDiskCache(cfg)
	if err != nil {
		t.Fatalf("NewDiskCache failed: %v", err)
	}
	defer dc.Stop()
	<-dc.loadCh

	hashKey := uint64(12345)
	origKey := "GET:/api/test"
	dc.Set(hashKey, origKey, []byte("data"), nil, 200, 100*time.Millisecond)

	// 等待过期但仍在 stale_if_timeout 窗口内
	time.Sleep(250 * time.Millisecond)

	// isTimeout=true，应该使用 staleIfTimeout 窗口
	entry, ok := dc.GetStale(hashKey, origKey, true)
	if !ok {
		t.Error("stale entry should be usable on timeout")
	}
	if entry == nil {
		t.Error("expected stale entry data")
	}

	// isTimeout=false，staleIfError=0，不应该可用
	if _, ok2 := dc.GetStale(hashKey, origKey, false); ok2 {
		t.Error("stale entry should NOT be usable on error when staleIfError=0")
	}
}

func TestDiskCacheGetStaleExpired(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &DiskCacheConfig{
		Path:           tmpDir,
		Levels:         "1:2",
		StaleIfError:   100 * time.Millisecond,
		StaleIfTimeout: 100 * time.Millisecond,
	}

	dc, err := NewDiskCache(cfg)
	if err != nil {
		t.Fatalf("NewDiskCache failed: %v", err)
	}
	defer dc.Stop()
	<-dc.loadCh

	hashKey := uint64(12345)
	origKey := "GET:/api/test"
	dc.Set(hashKey, origKey, []byte("data"), nil, 200, 50*time.Millisecond)

	// 等待超过 stale 窗口
	time.Sleep(200 * time.Millisecond)

	if _, ok := dc.GetStale(hashKey, origKey, false); ok {
		t.Error("stale entry should NOT be usable after stale window expired")
	}

	if _, ok2 := dc.GetStale(hashKey, origKey, true); ok2 {
		t.Error("stale entry should NOT be usable on timeout after stale window expired")
	}
}
