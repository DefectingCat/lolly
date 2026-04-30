// Package handler 提供 HTTP 请求处理器，包括路由、静态文件服务和零拷贝传输。
//
// 该文件实现 FileInfo 缓存，用于减少 os.Stat 调用。
// 替代原 fd 池设计，避免 fd 所有权问题。
//
// 设计说明：
//   - 使用 TTL-only 新鲜度策略：缓存命中时不验证 ModTime
//   - 理由：每次验证 ModTime 仍需 os.Stat 调用，违背缓存目的
//   - 风险：TTL 内文件修改可能返回旧 FileInfo，但静态文件通常不频繁修改
//
// 作者：xfy
package handler

import (
	"container/list"
	"os"
	"sync"
	"time"
)

const (
	fileInfoCacheMaxEntries = 2000
	fileInfoCacheTTL        = 10 * time.Second
)

// fileInfoEntry FileInfo 缓存条目
type fileInfoEntry struct {
	path     string
	info     os.FileInfo
	cachedAt time.Time
	element  *list.Element
}

// FileInfoCache FileInfo 缓存（O(1) LRU）
type FileInfoCache struct {
	entries map[string]*fileInfoEntry
	lruList *list.List
	mu      sync.RWMutex
}

// NewFileInfoCache 创建 FileInfo 缓存
func NewFileInfoCache() *FileInfoCache {
	return &FileInfoCache{
		entries: make(map[string]*fileInfoEntry, fileInfoCacheMaxEntries),
		lruList: list.New(),
	}
}

// Get 获取缓存的 FileInfo
func (c *FileInfoCache) Get(filePath string) (os.FileInfo, bool) {
	c.mu.RLock()
	entry, ok := c.entries[filePath]
	if !ok {
		c.mu.RUnlock()
		return nil, false
	}

	// 检查 TTL（只读检查）
	if time.Since(entry.cachedAt) > fileInfoCacheTTL {
		c.mu.RUnlock()
		// 升级为写锁删除过期条目
		c.mu.Lock()
		// double-check：可能已被其他请求删除或更新
		if entry, ok := c.entries[filePath]; ok && time.Since(entry.cachedAt) > fileInfoCacheTTL {
			c.lruList.Remove(entry.element)
			delete(c.entries, filePath)
		}
		c.mu.Unlock()
		return nil, false
	}

	c.mu.RUnlock()

	// LRU 移动需要写锁
	c.mu.Lock()
	// double-check：条目可能已被删除
	if entry, ok := c.entries[filePath]; ok {
		c.lruList.MoveToFront(entry.element)
		c.mu.Unlock()
		return entry.info, true
	}
	c.mu.Unlock()
	return nil, false
}

// Set 缓存 FileInfo
func (c *FileInfoCache) Set(filePath string, info os.FileInfo) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 已存在，更新
	if entry, ok := c.entries[filePath]; ok {
		entry.info = info
		entry.cachedAt = time.Now()
		c.lruList.MoveToFront(entry.element)
		return
	}

	// 淘汰最久未用的
	if c.lruList.Len() >= fileInfoCacheMaxEntries {
		if oldest := c.lruList.Back(); oldest != nil {
			if entry, ok := oldest.Value.(*fileInfoEntry); ok {
				delete(c.entries, entry.path)
			}
			c.lruList.Remove(oldest)
		}
	}

	// 插入新条目
	entry := &fileInfoEntry{
		path:     filePath,
		info:     info,
		cachedAt: time.Now(),
	}
	entry.element = c.lruList.PushFront(entry)
	c.entries[filePath] = entry
}

// Delete 删除缓存条目
func (c *FileInfoCache) Delete(filePath string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if entry, ok := c.entries[filePath]; ok {
		c.lruList.Remove(entry.element)
		delete(c.entries, filePath)
	}
}

// Clear 清空缓存
func (c *FileInfoCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries = make(map[string]*fileInfoEntry, fileInfoCacheMaxEntries)
	c.lruList = list.New()
}

// Stats 返回缓存统计
func (c *FileInfoCache) Stats() FileInfoCacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return FileInfoCacheStats{
		Entries: len(c.entries),
	}
}

// FileInfoCacheStats FileInfo 缓存统计
type FileInfoCacheStats struct {
	Entries int
}
