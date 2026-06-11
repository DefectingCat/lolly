// Package handler 提供 HTTP 请求处理器，包括路由、静态文件服务和零拷贝传输。
//
// 该文件实现 FileInfo 缓存，用于减少 os.Stat 调用。
// 替代原 fd 池设计，避免 fd 所有权问题。
//
// 设计说明：
//   - 使用 TTL-only 新鲜度策略：缓存命中时不验证 ModTime
//   - 理由：每次验证 ModTime 仍需 os.Stat 调用，违背缓存目的
//   - 风险：TTL 内文件修改可能返回旧 FileInfo，但静态文件通常不频繁修改
//   - 支持负缓存：缓存文件不存在的结果，避免重复 stat 不存在的路径
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
	fileInfoCacheMaxEntries   = 2000
	defaultFileInfoCacheTTL   = 10 * time.Second
	defaultFileNotFoundCacheTTL = 2 * time.Second
)

// fileInfoEntry FileInfo 缓存条目
type fileInfoEntry struct {
	path     string
	info     os.FileInfo
	notFound bool
	cachedAt time.Time
	element  *list.Element
}

// FileInfoCache FileInfo 缓存（O(1) LRU）
type FileInfoCache struct {
	entries     map[string]*fileInfoEntry
	lruList     *list.List
	ttl         time.Duration
	notFoundTTL time.Duration
	mu          sync.RWMutex
}

// NewFileInfoCache 创建新的 FileInfo 缓存
func NewFileInfoCache() *FileInfoCache {
	return &FileInfoCache{
		entries:     make(map[string]*fileInfoEntry),
		lruList:     list.New(),
		ttl:         defaultFileInfoCacheTTL,
		notFoundTTL: defaultFileNotFoundCacheTTL,
	}
}

// SetTTL 设置 FileInfo 缓存 TTL。
func (c *FileInfoCache) SetTTL(ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ttl = ttl
}

// Get 获取缓存的 FileInfo（向后兼容）
//
// 返回值：
//   - info: 缓存的 FileInfo
//   - ok: 是否命中缓存（仅对存在的文件返回 true）
func (c *FileInfoCache) Get(filePath string) (os.FileInfo, bool) {
	info, hit, exists := c.GetWithNotFound(filePath)
	if !hit || !exists {
		return nil, false
	}
	return info, true
}

// GetWithNotFound 获取缓存结果，包含负缓存（文件不存在）信息。
//
// 返回值：
//   - info: 缓存的 FileInfo（仅当 exists=true 时有效）
//   - hit:  是否命中缓存（包括正缓存和负缓存）
//   - exists: 文件是否存在（false 表示命中了负缓存）
func (c *FileInfoCache) GetWithNotFound(filePath string) (os.FileInfo, bool, bool) {
	c.mu.RLock()
	entry, ok := c.entries[filePath]
	if !ok {
		c.mu.RUnlock()
		return nil, false, false
	}

	ttl := c.ttl
	if ttl <= 0 {
		// ttl=0 表示禁用 fileInfoCache，总是返回未命中
		c.mu.RUnlock()
		return nil, false, false
	}
	if entry.notFound {
		if c.notFoundTTL > 0 {
			ttl = c.notFoundTTL
		} else {
			ttl = defaultFileNotFoundCacheTTL
		}
	}

	if time.Since(entry.cachedAt) > ttl {
		c.mu.RUnlock()
		c.mu.Lock()
		if e, ok := c.entries[filePath]; ok && time.Since(e.cachedAt) > ttl {
			c.lruList.Remove(e.element)
			delete(c.entries, filePath)
		}
		c.mu.Unlock()
		return nil, false, false
	}

	info := entry.info
	notFound := entry.notFound
	c.mu.RUnlock()
	return info, true, !notFound
}

// Set 缓存 FileInfo（向后兼容）
func (c *FileInfoCache) Set(filePath string, info os.FileInfo) {
	c.SetWithNotFound(filePath, info, false)
}

// SetWithNotFound 缓存 FileInfo，支持负缓存。
//
// 参数：
//   - filePath: 文件路径
//   - info:     FileInfo（notFound=true 时可为 nil）
//   - notFound: true 表示文件不存在
func (c *FileInfoCache) SetWithNotFound(filePath string, info os.FileInfo, notFound bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()

	// 已存在，更新
	if entry, ok := c.entries[filePath]; ok {
		entry.info = info
		entry.notFound = notFound
		entry.cachedAt = now
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
		notFound: notFound,
		cachedAt: now,
	}
	entry.element = c.lruList.PushFront(entry)
	c.entries[filePath] = entry
}

// FileInfoCacheStats FileInfo 缓存统计
type FileInfoCacheStats struct {
	Entries int
}
