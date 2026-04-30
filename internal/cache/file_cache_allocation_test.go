// Package cache 提供文件缓存分配热点追踪测试。
//
// 该文件追踪 FileCache.Set 的分配来源，验证池化优化效果。
//
// 测试场景：
//   - SetNew: 新建条目路径
//   - SetUpdate: 更新已存在条目路径
//   - SetEviction: 淘汰场景（缓存满载）
//
// 目标：池化后 ≤1 allocs/op
//
// 作者：xfy
package cache

import (
	"container/list"
	"fmt"
	"sync"
	"testing"
	"time"
)

// BenchmarkFileCacheSetAllocation_New 测试新建条目路径的分配。
//
// 热点分配来源：
//   - entry := c.entryPool.Get()  -> 池化后 0 allocs
//   - c.lruList.PushFront(entry)  -> list Element 分配
//   - c.entries[path] = entry     -> map 写入
func BenchmarkFileCacheSetAllocation_New(b *testing.B) {
	fc := NewFileCache(10000, 0, 1*time.Hour)

	data := []byte("test data content for benchmark")
	size := int64(len(data))

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; b.Loop(); i++ {
		path := fmt.Sprintf("/new/file%d.txt", i)
		fc.Set(path, data, size, time.Now())
	}
}

// BenchmarkFileCacheSetAllocation_Update 测试更新已存在条目路径的分配。
//
// 更新路径理论上 0 allocs（不涉及新条目分配）。
func BenchmarkFileCacheSetAllocation_Update(b *testing.B) {
	fc := NewFileCache(10000, 0, 1*time.Hour)

	// 预填充缓存
	data := []byte("test data content for benchmark")
	size := int64(len(data))
	for i := range 1000 {
		path := fmt.Sprintf("/update/file%d.txt", i)
		fc.Set(path, data, size, time.Now())
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; b.Loop(); i++ {
		// 循环更新已有条目
		path := fmt.Sprintf("/update/file%d.txt", i%1000)
		fc.Set(path, data, size, time.Now())
	}
}

// BenchmarkFileCacheSetAllocation_Eviction 测试淘汰场景的分配。
//
// 热点：
//   - 淘汰时 removeEntry -> entryPool.Put(entry)
//   - 新 Set 时 entryPool.Get() 复用
//   - 理论上 LRU Element 分配仍是热点
func BenchmarkFileCacheSetAllocation_Eviction(b *testing.B) {
	// 容量限制为 100，强制触发淘汰
	fc := NewFileCache(100, 0, 1*time.Hour)

	// 预填充到容量上限
	data := []byte("test data content for benchmark")
	size := int64(len(data))
	for i := range 100 {
		path := fmt.Sprintf("/evict/file%d.txt", i)
		fc.Set(path, data, size, time.Now())
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; b.Loop(); i++ {
		// 每个 Set 都触发淘汰
		path := fmt.Sprintf("/evict/new%d.txt", i)
		fc.Set(path, data, size, time.Now())
	}
}

// BenchmarkFileCacheSetAllocation_EvictionWithPool 测试淘汰+池化复用的分配。
//
// 验证 entryPool 在淘汰场景的复用效果。
func BenchmarkFileCacheSetAllocation_EvictionWithPool(b *testing.B) {
	fc := NewFileCache(100, 0, 1*time.Hour)

	data := []byte("test data content for benchmark")
	size := int64(len(data))

	// 预填充
	for i := range 100 {
		path := fmt.Sprintf("/pool/file%d.txt", i)
		fc.Set(path, data, size, time.Now())
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; b.Loop(); i++ {
		path := fmt.Sprintf("/pool/new%d.txt", i)
		fc.Set(path, data, size, time.Now())
	}
}

// BenchmarkFileCacheSetAllocation_MemoryLimit 测试内存限制淘汰的分配。
//
// 热点：evictIfNeeded 需遍历 LRU 链表淘汰多个条目。
func BenchmarkFileCacheSetAllocation_MemoryLimit(b *testing.B) {
	// 限制 1MB，每个条目 1KB，约 1000 条目
	fc := NewFileCache(0, 1024*1024, 1*time.Hour)

	data := make([]byte, 1024) // 1KB 每条目
	size := int64(len(data))

	// 预填充到接近上限
	for i := range 900 {
		path := fmt.Sprintf("/mem/file%d.txt", i)
		fc.Set(path, data, size, time.Now())
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; b.Loop(); i++ {
		path := fmt.Sprintf("/mem/new%d.txt", i)
		fc.Set(path, data, size, time.Now())
	}
}

// BenchmarkFileCacheSetAllocation_Concurrent 测试并发 Set 的分配。
//
// 并发场景下锁竞争可能影响池化效果。
func BenchmarkFileCacheSetAllocation_Concurrent(b *testing.B) {
	fc := NewFileCache(10000, 0, 1*time.Hour)

	data := []byte("concurrent test data")
	size := int64(len(data))

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			path := fmt.Sprintf("/conc/file%d.txt", i)
			fc.Set(path, data, size, time.Now())
			i++
		}
	})
}

// BenchmarkFileCacheSetAllocation_ConcurrentEviction 测试并发淘汰场景。
func BenchmarkFileCacheSetAllocation_ConcurrentEviction(b *testing.B) {
	fc := NewFileCache(100, 0, 1*time.Hour)

	data := []byte("eviction concurrent")
	size := int64(len(data))

	// 预填充
	for i := range 100 {
		path := fmt.Sprintf("/concevict/file%d.txt", i)
		fc.Set(path, data, size, time.Now())
	}

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			path := fmt.Sprintf("/concevict/new%d.txt", i)
			fc.Set(path, data, size, time.Now())
			i++
		}
	})
}

// BenchmarkFileCacheEntryPool_GetPut 测试 entryPool 直接操作。
//
// 验证 sync.Pool 的分配效果（应 0 allocs）。
func BenchmarkFileCacheEntryPool_GetPut(b *testing.B) {
	pool := &sync.Pool{
		New: func() any {
			return &FileEntry{}
		},
	}

	// 预填充池
	for range 100 {
		pool.Put(&FileEntry{})
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		entry := pool.Get().(*FileEntry)
		// 模拟使用
		entry.Path = "/test"
		entry.Data = nil
		pool.Put(entry)
	}
}

// BenchmarkFileCacheLRUList_PushFront 测试 LRU 链表操作分配。
//
// 热点：list.PushFront 分配 Element（约 1 allocs）。
func BenchmarkFileCacheLRUList_PushFront(b *testing.B) {
	lruList := list.New()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; b.Loop(); i++ {
		entry := &FileEntry{Path: fmt.Sprintf("/lru%d", i)}
		lruList.PushFront(entry)
		// 模拟淘汰移除
		if lruList.Len() > 100 {
			old := lruList.Back()
			lruList.Remove(old)
		}
	}
}
