// Package cache 提供分片缓存实现，用于高并发场景。
//
// 该文件实现分片文件缓存，将缓存分散到多个独立分片，
// 每个分片有自己的锁和 LRU 链表，减少锁竞争。
//
// 主要用途：
//
//	用于高并发场景下的文件缓存，减少单一 RWMutex 的竞争压力。
//
// 注意事项：
//   - 分片数固定为 16，按 path hash 选择分片
//   - 各分片独立 LRU 淘汰，无法跨分片协调
//   - 适合读多写少场景，写入仍会阻塞单个分片
//
// 作者：xfy
package cache

import (
	"container/list"
	"hash/fnv"
	"sync"
	"time"
)

const shardCount = 16

// FileCacheShard 单个缓存分片。
type FileCacheShard struct {
	entries     map[string]*FileEntry
	lruList     *list.List
	maxEntries  int64
	maxSize     int64
	inactive    time.Duration
	currentSize int64
	mu          sync.RWMutex
	entryPool   sync.Pool
}

// ShardedFileCache 分片文件缓存。
type ShardedFileCache struct {
	shards     [shardCount]*FileCacheShard
	maxEntries int64
	maxSize    int64
	inactive   time.Duration
}

// NewShardedFileCache 创建分片文件缓存实例。
func NewShardedFileCache(maxEntries, maxSize int64, inactive time.Duration) *ShardedFileCache {
	s := &ShardedFileCache{
		maxEntries: maxEntries,
		maxSize:    maxSize,
		inactive:   inactive,
	}

	perShardEntries := maxEntries / shardCount
	if perShardEntries == 0 {
		perShardEntries = maxEntries // 单分片容量
	}
	perShardSize := maxSize / shardCount
	if perShardSize == 0 {
		perShardSize = maxSize
	}

	for i := range shardCount {
		shard := &FileCacheShard{
			maxEntries: perShardEntries,
			maxSize:    perShardSize,
			inactive:   inactive,
			entries:    make(map[string]*FileEntry),
			lruList:    list.New(),
		}
		shard.entryPool = sync.Pool{
			New: func() any {
				return &FileEntry{}
			},
		}
		s.shards[i] = shard
	}
	return s
}

// getShard 根据 path hash 选择分片。
func (s *ShardedFileCache) getShard(path string) *FileCacheShard {
	h := fnv.New64a()
	h.Write([]byte(path))
	return s.shards[h.Sum64()%shardCount]
}

// Get 获取缓存的文件。
func (s *ShardedFileCache) Get(path string) (*FileEntry, bool) {
	return s.getShard(path).Get(path)
}

// Set 设置缓存条目。
func (s *ShardedFileCache) Set(path string, data []byte, size int64, modTime time.Time) error {
	return s.getShard(path).Set(path, data, size, modTime)
}

// Delete 删除缓存条目。
func (s *ShardedFileCache) Delete(path string) {
	s.getShard(path).Delete(path)
}

// Clear 清空所有分片缓存。
func (s *ShardedFileCache) Clear() {
	for _, shard := range s.shards {
		shard.Clear()
	}
}

// Stats 返回汇总统计信息。
func (s *ShardedFileCache) Stats() FileCacheStats {
	totalEntries := int64(0)
	totalSize := int64(0)
	for _, shard := range s.shards {
		stats := shard.Stats()
		totalEntries += stats.Entries
		totalSize += stats.Size
	}
	return FileCacheStats{
		Entries:    totalEntries,
		MaxEntries: s.maxEntries,
		Size:       totalSize,
		MaxSize:    s.maxSize,
	}
}

// --- FileCacheShard 方法（内联实现，避免依赖 FileCache）---

// Get 获取缓存的文件。
func (sh *FileCacheShard) Get(path string) (*FileEntry, bool) {
	sh.mu.RLock()
	entry, ok := sh.entries[path]
	if !ok {
		sh.mu.RUnlock()
		return nil, false
	}

	// 检查是否过期
	if time.Since(entry.LastAccess) > sh.inactive {
		sh.mu.RUnlock()
		sh.mu.Lock()
		if entry, ok = sh.entries[path]; ok && time.Since(entry.LastAccess) > sh.inactive {
			sh.removeEntry(entry)
		}
		sh.mu.Unlock()
		return nil, false
	}

	sh.mu.RUnlock()

	// 更新访问时间（需写锁）
	sh.mu.Lock()
	if entry, ok = sh.entries[path]; ok && time.Since(entry.LastAccess) <= sh.inactive {
		entry.LastAccess = time.Now()
		sh.lruList.MoveToFront(entry.element)
	}
	sh.mu.Unlock()
	return entry, true
}

// Set 设置缓存条目。
func (sh *FileCacheShard) Set(path string, data []byte, size int64, modTime time.Time) error {
	sh.mu.Lock()
	defer sh.mu.Unlock()

	if entry, ok := sh.entries[path]; ok {
		sh.currentSize -= entry.Size
		entry.Data = data
		entry.Size = size
		entry.ModTime = modTime
		entry.CachedAt = time.Now()
		entry.LastAccess = time.Now()
		sh.currentSize += size
		sh.lruList.MoveToFront(entry.element)
		sh.evictIfNeeded()
		return nil
	}

	entry := sh.entryPool.Get().(*FileEntry)
	entry.Path = path
	entry.Data = data
	entry.Size = size
	entry.ModTime = modTime
	entry.CachedAt = time.Now()
	entry.LastAccess = time.Now()
	entry.element = sh.lruList.PushFront(entry)
	sh.entries[path] = entry
	sh.currentSize += size

	sh.evictIfNeeded()
	return nil
}

// Delete 删除缓存条目。
func (sh *FileCacheShard) Delete(path string) {
	sh.mu.Lock()
	defer sh.mu.Unlock()
	if entry, ok := sh.entries[path]; ok {
		sh.removeEntry(entry)
	}
}

// Clear 清空分片缓存。
func (sh *FileCacheShard) Clear() {
	sh.mu.Lock()
	defer sh.mu.Unlock()
	sh.entries = make(map[string]*FileEntry)
	sh.lruList = list.New()
	sh.currentSize = 0
}

// Stats 返回分片统计信息。
func (sh *FileCacheShard) Stats() FileCacheStats {
	sh.mu.RLock()
	defer sh.mu.RUnlock()
	return FileCacheStats{
		Entries:    int64(len(sh.entries)),
		MaxEntries: sh.maxEntries,
		Size:       sh.currentSize,
		MaxSize:    sh.maxSize,
	}
}

// removeEntry 内部删除条目（不加锁）。
func (sh *FileCacheShard) removeEntry(entry *FileEntry) {
	sh.lruList.Remove(entry.element)
	delete(sh.entries, entry.Path)
	sh.currentSize -= entry.Size
	// Reset entry 并放回池
	entry.Path = ""
	entry.Data = nil
	entry.Size = 0
	entry.ModTime = time.Time{}
	entry.CachedAt = time.Time{}
	entry.LastAccess = time.Time{}
	entry.element = nil
	sh.entryPool.Put(entry)
}

// evictIfNeeded 根据限制淘汰条目。
func (sh *FileCacheShard) evictIfNeeded() {
	for sh.lruList.Len() > int(sh.maxEntries) && sh.maxEntries > 0 {
		sh.evictLRU()
	}
	for sh.currentSize > sh.maxSize && sh.maxSize > 0 {
		sh.evictLRU()
	}
}

// evictLRU 淘汰最久未使用的条目。
func (sh *FileCacheShard) evictLRU() {
	if sh.lruList.Len() == 0 {
		return
	}
	element := sh.lruList.Back()
	if element == nil {
		return
	}
	entry, ok := element.Value.(*FileEntry)
	if !ok {
		return
	}
	sh.removeEntry(entry)
}
