package handler

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFileInfoCache(t *testing.T) {
	// 创建临时文件
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(tmpFile, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	cache := NewFileInfoCache()

	t.Run("缓存未命中", func(t *testing.T) {
		info, ok := cache.Get(tmpFile)
		if ok {
			t.Error("未命中的缓存应返回 false")
		}
		if info != nil {
			t.Error("未命中时应返回 nil")
		}
	})

	t.Run("缓存命中", func(t *testing.T) {
		// 先获取真实 FileInfo
		realInfo, err := os.Stat(tmpFile)
		if err != nil {
			t.Fatal(err)
		}

		// 存入缓存
		cache.Set(tmpFile, realInfo)

		// 从缓存获取
		cachedInfo, ok := cache.Get(tmpFile)
		if !ok {
			t.Error("缓存应命中")
		}
		if cachedInfo == nil {
			t.Fatal("缓存命中时应返回非 nil")
		}
		if cachedInfo.Name() != realInfo.Name() {
			t.Errorf("Name() = %q, want %q", cachedInfo.Name(), realInfo.Name())
		}
		if cachedInfo.Size() != realInfo.Size() {
			t.Errorf("Size() = %d, want %d", cachedInfo.Size(), realInfo.Size())
		}
	})

	t.Run("删除缓存", func(t *testing.T) {
		cache.Delete(tmpFile)
		_, ok := cache.Get(tmpFile)
		if ok {
			t.Error("删除后缓存不应命中")
		}
	})

	t.Run("清空缓存", func(t *testing.T) {
		realInfo, _ := os.Stat(tmpFile)
		cache.Set(tmpFile, realInfo)
		cache.Set(filepath.Join(tmpDir, "other"), realInfo)

		cache.Clear()
		stats := cache.Stats()
		if stats.Entries != 0 {
			t.Errorf("清空后 Entries = %d, want 0", stats.Entries)
		}
	})
}

func TestFileInfoCacheTTL(t *testing.T) {
	// 创建临时文件
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(tmpFile, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	cache := NewFileInfoCache()
	realInfo, _ := os.Stat(tmpFile)

	// 存入缓存
	cache.Set(tmpFile, realInfo)

	// 立即获取应命中
	_, ok := cache.Get(tmpFile)
	if !ok {
		t.Error("立即获取应命中")
	}

	// 模拟过期：修改 cachedAt
	cache.mu.Lock()
	if entry, exists := cache.entries[tmpFile]; exists {
		entry.cachedAt = time.Now().Add(-fileInfoCacheTTL - time.Second)
	}
	cache.mu.Unlock()

	// 过期后应未命中
	_, ok = cache.Get(tmpFile)
	if ok {
		t.Error("过期后应未命中")
	}
}

func TestFileInfoCacheLRU(t *testing.T) {
	cache := NewFileInfoCache()

	// 创建测试文件信息
	tmpDir := t.TempDir()
	for i := range fileInfoCacheMaxEntries + 10 {
		tmpFile := filepath.Join(tmpDir, "test"+string(rune('0'+i%10))+".txt")
		os.WriteFile(tmpFile, []byte("hello"), 0o644)
		info, _ := os.Stat(tmpFile)
		cache.Set(tmpFile, info)
	}

	stats := cache.Stats()
	if stats.Entries > fileInfoCacheMaxEntries {
		t.Errorf("Entries = %d, should not exceed %d", stats.Entries, fileInfoCacheMaxEntries)
	}
}
