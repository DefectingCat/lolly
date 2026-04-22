package cache

import (
	"testing"
	"time"
)

func TestNewTieredCache(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &TieredCacheConfig{
		L2Config: &DiskCacheConfig{
			Path:   tmpDir,
			Levels: "1:2",
		},
		PromoteThreshold: 3,
		PromoteInterval:  100 * time.Millisecond,
	}

	tc, err := NewTieredCache(cfg)
	if err != nil {
		t.Fatalf("NewTieredCache failed: %v", err)
	}
	defer tc.Stop()

	// 等待 L2 懒加载完成
	<-tc.l2.loadCh

	if tc.l1 == nil {
		t.Error("l1 should not be nil")
	}
	if tc.l2 == nil {
		t.Error("l2 should not be nil")
	}
}

func TestTieredCacheSetGet(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &TieredCacheConfig{
		L2Config: &DiskCacheConfig{
			Path:   tmpDir,
			Levels: "1:2",
		},
	}

	tc, err := NewTieredCache(cfg)
	if err != nil {
		t.Fatalf("NewTieredCache failed: %v", err)
	}
	defer tc.Stop()

	<-tc.l2.loadCh

	// 设置缓存
	hashKey := uint64(12345)
	origKey := "GET:/api/test"
	data := []byte("test response data")

	tc.Set(hashKey, origKey, data, nil, 200, 10*time.Minute)

	// 等待 L2 异步写入完成
	time.Sleep(50 * time.Millisecond)

	// 获取缓存（应该从 L1 获取）
	entry, exists, stale := tc.Get(hashKey, origKey)
	if !exists {
		t.Fatal("cache entry not found")
	}
	if stale {
		t.Error("entry should not be stale")
	}
	if string(entry.Data) != string(data) {
		t.Errorf("Data = %q, want %q", entry.Data, data)
	}

	// 验证 L1 命中
	stats := tc.TieredCacheStats()
	if stats.L1Hits != 1 {
		t.Errorf("L1Hits = %d, want 1", stats.L1Hits)
	}
}

func TestTieredCacheL2Fallback(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &TieredCacheConfig{
		L2Config: &DiskCacheConfig{
			Path:   tmpDir,
			Levels: "1:2",
		},
	}

	tc, err := NewTieredCache(cfg)
	if err != nil {
		t.Fatalf("NewTieredCache failed: %v", err)
	}
	defer tc.Stop()

	<-tc.l2.loadCh

	// 直接写入 L2（绕过 L1）
	hashKey := uint64(12345)
	origKey := "GET:/api/test"
	data := []byte("test from l2")

	tc.l2.Set(hashKey, origKey, data, nil, 200, 10*time.Minute)
	time.Sleep(50 * time.Millisecond)

	// 获取缓存（应该从 L2 获取）
	entry, exists, stale := tc.Get(hashKey, origKey)
	if !exists {
		t.Fatal("cache entry not found")
	}
	if stale {
		t.Error("entry should not be stale")
	}
	if string(entry.Data) != string(data) {
		t.Errorf("Data = %q, want %q", entry.Data, data)
	}

	// 验证 L2 命中
	stats := tc.TieredCacheStats()
	if stats.L2Hits != 1 {
		t.Errorf("L2Hits = %d, want 1", stats.L2Hits)
	}
}

func TestTieredCacheDelete(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &TieredCacheConfig{
		L2Config: &DiskCacheConfig{
			Path:   tmpDir,
			Levels: "1:2",
		},
	}

	tc, err := NewTieredCache(cfg)
	if err != nil {
		t.Fatalf("NewTieredCache failed: %v", err)
	}
	defer tc.Stop()

	<-tc.l2.loadCh

	// 设置缓存
	hashKey := uint64(12345)
	origKey := "GET:/api/test"
	tc.Set(hashKey, origKey, []byte("test"), nil, 200, 10*time.Minute)
	time.Sleep(50 * time.Millisecond)

	// 删除
	if err := tc.Delete(hashKey); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// 验证 L1 和 L2 都已删除
	_, exists, _ := tc.l1.Get(hashKey, origKey)
	if exists {
		t.Error("entry should not exist in L1 after delete")
	}

	_, exists, _ = tc.l2.Get(hashKey, origKey)
	if exists {
		t.Error("entry should not exist in L2 after delete")
	}
}

func TestTieredCacheStale(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &TieredCacheConfig{
		L2Config: &DiskCacheConfig{
			Path:   tmpDir,
			Levels: "1:2",
		},
	}

	tc, err := NewTieredCache(cfg)
	if err != nil {
		t.Fatalf("NewTieredCache failed: %v", err)
	}
	defer tc.Stop()

	<-tc.l2.loadCh

	// 设置一个已过期的缓存
	hashKey := uint64(12345)
	origKey := "GET:/api/test"
	tc.l2.Set(hashKey, origKey, []byte("test"), nil, 200, 1*time.Millisecond)

	// 等待过期
	time.Sleep(10 * time.Millisecond)

	// 获取缓存
	_, exists, stale := tc.Get(hashKey, origKey)
	if !exists {
		t.Fatal("expired entry should still exist")
	}
	if !stale {
		t.Error("expired entry should be marked as stale")
	}
}

func TestTieredCachePromote(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &TieredCacheConfig{
		L2Config: &DiskCacheConfig{
			Path:   tmpDir,
			Levels: "1:2",
		},
		PromoteThreshold: 2,
		PromoteInterval:  50 * time.Millisecond,
	}

	tc, err := NewTieredCache(cfg)
	if err != nil {
		t.Fatalf("NewTieredCache failed: %v", err)
	}
	defer tc.Stop()

	<-tc.l2.loadCh

	// 直接写入 L2
	hashKey := uint64(12345)
	origKey := "GET:/api/test"
	data := []byte("test data")
	tc.l2.Set(hashKey, origKey, data, nil, 200, 10*time.Minute)
	time.Sleep(50 * time.Millisecond)

	// 访问两次（达到阈值）
	tc.Get(hashKey, origKey)
	tc.Get(hashKey, origKey)

	// 等待提升检查
	time.Sleep(100 * time.Millisecond)

	// 验证提升发生
	stats := tc.TieredCacheStats()
	if stats.Promotes == 0 {
		t.Error("promotes should be > 0 after reaching threshold")
	}
}

func TestTieredCacheStats(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &TieredCacheConfig{
		L2Config: &DiskCacheConfig{
			Path:   tmpDir,
			Levels: "1:2",
		},
	}

	tc, err := NewTieredCache(cfg)
	if err != nil {
		t.Fatalf("NewTieredCache failed: %v", err)
	}
	defer tc.Stop()

	<-tc.l2.loadCh

	// 设置缓存
	tc.Set(1, "key1", []byte("data1"), nil, 200, 10*time.Minute)
	tc.Set(2, "key2", []byte("data2"), nil, 200, 10*time.Minute)
	time.Sleep(50 * time.Millisecond)

	// 获取缓存
	tc.Get(1, "key1")          // L1 命中
	tc.Get(1, "key1")          // L1 命中
	tc.Get(999, "nonexistent") // 未命中

	stats := tc.TieredCacheStats()
	if stats.L1Hits != 2 {
		t.Errorf("L1Hits = %d, want 2", stats.L1Hits)
	}
	if stats.Misses != 1 {
		t.Errorf("Misses = %d, want 1", stats.Misses)
	}
}

func TestTieredCacheRestart(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &TieredCacheConfig{
		L2Config: &DiskCacheConfig{
			Path:   tmpDir,
			Levels: "1:2",
		},
	}

	// 第一个实例：写入数据
	tc1, err := NewTieredCache(cfg)
	if err != nil {
		t.Fatalf("NewTieredCache failed: %v", err)
	}
	<-tc1.l2.loadCh

	hashKey := uint64(12345)
	origKey := "GET:/api/test"
	data := []byte("persistent data")
	tc1.Set(hashKey, origKey, data, nil, 200, 10*time.Minute)
	time.Sleep(50 * time.Millisecond)
	tc1.Stop()

	// 第二个实例：读取数据（模拟重启）
	tc2, err := NewTieredCache(cfg)
	if err != nil {
		t.Fatalf("NewTieredCache (restart) failed: %v", err)
	}
	<-tc2.l2.loadCh
	defer tc2.Stop()

	// 验证数据从 L2 恢复
	entry, exists, _ := tc2.Get(hashKey, origKey)
	if !exists {
		t.Fatal("entry should exist after restart")
	}
	if string(entry.Data) != string(data) {
		t.Errorf("Data = %q, want %q", entry.Data, data)
	}

	// 验证是从 L2 获取的
	stats := tc2.TieredCacheStats()
	if stats.L2Hits != 1 {
		t.Errorf("L2Hits = %d, want 1", stats.L2Hits)
	}
}

func TestTieredCacheCacheStats(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &TieredCacheConfig{
		L2Config: &DiskCacheConfig{
			Path:   tmpDir,
			Levels: "1:2",
		},
	}

	tc, err := NewTieredCache(cfg)
	if err != nil {
		t.Fatalf("NewTieredCache failed: %v", err)
	}
	defer tc.Stop()

	<-tc.l2.loadCh

	// 设置缓存
	tc.Set(1, "key1", []byte("data1"), nil, 200, 10*time.Minute)
	time.Sleep(50 * time.Millisecond)

	// 获取缓存统计
	stats := tc.CacheStats()
	if stats.Entries < 1 {
		t.Errorf("Entries = %d, should be >= 1", stats.Entries)
	}
}
