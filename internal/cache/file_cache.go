// Package cache 提供文件缓存和代理缓存功能，支持 LRU 淘汰和缓存锁防击穿。
package cache

import (
	"container/list"
	"strings"
	"sync"
	"time"
)

// FileEntry 文件缓存条目。
type FileEntry struct {
	Path       string      // 文件路径
	Size       int64       // 文件大小
	ModTime    time.Time   // 修改时间
	LastAccess time.Time   // 最后访问时间
	Data       []byte      // 文件内容
	element    *list.Element // LRU 链表元素
}

// FileCache 文件缓存，支持 LRU 淘汰。
type FileCache struct {
	maxEntries int64           // 最大条目数
	maxSize    int64           // 内存上限（字节）
	inactive   time.Duration   // 未访问淘汰时间
	entries    map[string]*FileEntry
	lruList    *list.List      // LRU 链表
	mu         sync.RWMutex
	currentSize int64          // 当前内存使用
}

// NewFileCache 创建文件缓存。
func NewFileCache(maxEntries, maxSize int64, inactive time.Duration) *FileCache {
	return &FileCache{
		maxEntries: maxEntries,
		maxSize:    maxSize,
		inactive:   inactive,
		entries:    make(map[string]*FileEntry),
		lruList:    list.New(),
	}
}

// Get 获取缓存的文件。
func (c *FileCache) Get(path string) (*FileEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.entries[path]
	if !ok {
		return nil, false
	}

	// 检查是否过期
	if time.Since(entry.LastAccess) > c.inactive {
		c.removeEntry(entry)
		return nil, false
	}

	// 更新访问时间并移到 LRU 链表头部
	entry.LastAccess = time.Now()
	c.lruList.MoveToFront(entry.element)

	return entry, true
}

// Set 设置缓存条目。
func (c *FileCache) Set(path string, data []byte, size int64, modTime time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 检查是否已存在
	if entry, ok := c.entries[path]; ok {
		c.currentSize -= entry.Size
		entry.Data = data
		entry.Size = size
		entry.ModTime = modTime
		entry.LastAccess = time.Now()
		c.currentSize += size
		c.lruList.MoveToFront(entry.element)
		c.evictIfNeeded()
		return nil
	}

	// 创建新条目
	entry := &FileEntry{
		Path:       path,
		Data:       data,
		Size:       size,
		ModTime:    modTime,
		LastAccess: time.Now(),
	}
	entry.element = c.lruList.PushFront(entry)
	c.entries[path] = entry
	c.currentSize += size

	c.evictIfNeeded()
	return nil
}

// Delete 删除缓存条目。
func (c *FileCache) Delete(path string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if entry, ok := c.entries[path]; ok {
		c.removeEntry(entry)
	}
}

// removeEntry 内部删除条目（不加锁）。
func (c *FileCache) removeEntry(entry *FileEntry) {
	c.lruList.Remove(entry.element)
	delete(c.entries, entry.Path)
	c.currentSize -= entry.Size
}

// evictIfNeeded 根据限制淘汰条目。
func (c *FileCache) evictIfNeeded() {
	// 按条目数淘汰
	for c.lruList.Len() > int(c.maxEntries) && c.maxEntries > 0 {
		c.evictLRU()
	}

	// 按内存大小淘汰
	for c.currentSize > c.maxSize && c.maxSize > 0 {
		c.evictLRU()
	}
}

// evictLRU 淘汰最久未使用的条目。
func (c *FileCache) evictLRU() {
	if c.lruList.Len() == 0 {
		return
	}

	element := c.lruList.Back()
	if element == nil {
		return
	}

	entry := element.Value.(*FileEntry)
	c.removeEntry(entry)
}

// Clear 清空缓存。
func (c *FileCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries = make(map[string]*FileEntry)
	c.lruList = list.New()
	c.currentSize = 0
}

// Stats 返回缓存统计信息。
func (c *FileCache) Stats() FileCacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return FileCacheStats{
		Entries:     int64(len(c.entries)),
		MaxEntries:  c.maxEntries,
		Size:        c.currentSize,
		MaxSize:     c.maxSize,
	}
}

// FileCacheStats 文件缓存统计。
type FileCacheStats struct {
	Entries    int64
	MaxEntries int64
	Size       int64
	MaxSize    int64
}

// ProxyCacheRule 代理缓存规则。
type ProxyCacheRule struct {
	Path     string        // 匹配路径
	Methods  []string      // 可缓存的 HTTP 方法
	Statuses []int         // 可缓存的状态码
	MaxAge   time.Duration // 缓存有效期
}

// ProxyCacheEntry 代理缓存条目。
type ProxyCacheEntry struct {
	Key        string        // 缓存 key
	Data       []byte        // 响应体
	Headers    map[string]string // 响应头
	Status     int           // 状态码
	Created    time.Time     // 创建时间
	MaxAge     time.Duration // 有效期
}

// ProxyCache 代理响应缓存，支持缓存锁防击穿。
type ProxyCache struct {
	rules      []ProxyCacheRule
	entries    map[string]*ProxyCacheEntry
	mu         sync.RWMutex
	cacheLock  bool          // 缓存锁开关
	pending    map[string]*pendingRequest // 正在生成的缓存项
	staleTime  time.Duration // 过期缓存复用时间
}

// pendingRequest 等待中的缓存请求。
type pendingRequest struct {
	done chan struct{} // 完成信号
	err  error         // 生成结果
}

// NewProxyCache 创建代理缓存。
func NewProxyCache(rules []ProxyCacheRule, cacheLock bool, staleTime time.Duration) *ProxyCache {
	return &ProxyCache{
		rules:     rules,
		entries:   make(map[string]*ProxyCacheEntry),
		cacheLock: cacheLock,
		pending:   make(map[string]*pendingRequest),
		staleTime: staleTime,
	}
}

// Get 获取缓存的代理响应。
func (c *ProxyCache) Get(key string) (*ProxyCacheEntry, bool, bool) {
	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()

	if !ok {
		return nil, false, false
	}

	// 检查是否过期
	now := time.Now()
	expired := now.Sub(entry.Created) > entry.MaxAge

	if expired {
		// 检查是否可以使用过期缓存
		if c.staleTime > 0 && now.Sub(entry.Created) <= entry.MaxAge+c.staleTime {
			return entry, true, true // stale but usable
		}
		return nil, false, false
	}

	return entry, true, false
}

// Set 设置代理缓存条目。
func (c *ProxyCache) Set(key string, data []byte, headers map[string]string, status int, maxAge time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[key] = &ProxyCacheEntry{
		Key:     key,
		Data:    data,
		Headers: headers,
		Status:  status,
		Created: time.Now(),
		MaxAge:  maxAge,
	}

	// 如果有等待的请求，通知它们
	if pending, ok := c.pending[key]; ok {
		pending.err = nil
		close(pending.done)
		delete(c.pending, key)
	}
}

// AcquireLock 获取缓存生成锁（防止击穿）。
// 如果返回 nil，表示获得锁，应该去生成缓存。
// 如果返回 chan，表示有其他请求正在生成，应该等待。
func (c *ProxyCache) AcquireLock(key string) <-chan struct{} {
	if !c.cacheLock {
		return nil // 不使用缓存锁
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// 检查是否已有缓存
	if _, ok := c.entries[key]; ok {
		return nil
	}

	// 检查是否有 pending 请求
	if pending, ok := c.pending[key]; ok {
		return pending.done // 等待现有请求
	}

	// 创建新的 pending 请求
	pending := &pendingRequest{
		done: make(chan struct{}),
	}
	c.pending[key] = pending
	return nil // 获得锁，应该生成缓存
}

// ReleaseLock 释放缓存生成锁。
func (c *ProxyCache) ReleaseLock(key string, err error) {
	if !c.cacheLock {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if pending, ok := c.pending[key]; ok {
		pending.err = err
		close(pending.done)
		delete(c.pending, key)
	}
}

// MatchRule 检查请求是否匹配缓存规则。
func (c *ProxyCache) MatchRule(path, method string, status int) *ProxyCacheRule {
	for _, rule := range c.rules {
		// 检查路径匹配（简单前缀匹配）
		if rule.Path != "" && !pathMatch(rule.Path, path) {
			continue
		}

		// 检查方法
		if len(rule.Methods) > 0 && !contains(rule.Methods, method) {
			continue
		}

		// 检查状态码
		if len(rule.Statuses) > 0 && !containsInt(rule.Statuses, status) {
			continue
		}

		return &rule
	}
	return nil
}

// pathMatch 路径匹配（支持前缀和精确匹配）。
func pathMatch(pattern, path string) bool {
	if pattern == "*" {
		return true
	}
	// 通配符匹配
	if pattern[len(pattern)-1] == '*' {
		prefix := pattern[:len(pattern)-1]
		return strings.HasPrefix(path, prefix)
	}
	// 前缀匹配（pattern 以 / 结尾）
	if pattern[len(pattern)-1] == '/' {
		return strings.HasPrefix(path, pattern)
	}
	// 精确匹配
	return path == pattern
}

// contains 检查字符串切片是否包含某值。
func contains(slice []string, val string) bool {
	for _, s := range slice {
		if s == val {
			return true
		}
	}
	return false
}

// containsInt 检查整数切片是否包含某值。
func containsInt(slice []int, val int) bool {
	for _, i := range slice {
		if i == val {
			return true
		}
	}
	return false
}

// Delete 删除缓存条目。
func (c *ProxyCache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, key)
}

// Clear 清空代理缓存。
func (c *ProxyCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]*ProxyCacheEntry)
	c.pending = make(map[string]*pendingRequest)
}

// Stats 返回代理缓存统计。
func (c *ProxyCache) Stats() ProxyCacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return ProxyCacheStats{
		Entries: len(c.entries),
		Pending: len(c.pending),
	}
}

// ProxyCacheStats 代理缓存统计。
type ProxyCacheStats struct {
	Entries int
	Pending int
}