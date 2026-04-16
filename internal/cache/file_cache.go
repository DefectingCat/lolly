// Package cache 提供文件缓存和代理缓存功能，支持 LRU 淘汰和缓存锁防击穿。
//
// 该文件包含缓存相关的核心逻辑，包括：
//   - 文件缓存实现，支持 LRU 淘汰策略
//   - 代理响应缓存，支持缓存锁防止缓存击穿
//   - 缓存统计和生命周期管理
//
// 主要用途：
//
//	用于缓存静态文件内容和代理响应，减少磁盘 I/O 和上游请求，提升服务性能。
//
// 注意事项：
//   - 文件缓存支持按条目数和内存大小双重限制
//   - 代理缓存支持过期缓存复用（stale）机制
//   - 所有公开方法均为并发安全
//
// 作者：xfy
package cache

import (
	"container/list"
	"slices"
	"strings"
	"sync"
	"time"
)

// FileEntry 文件缓存条目，存储单个文件的缓存信息。
type FileEntry struct {
	ModTime    time.Time
	CachedAt   time.Time // 缓存时间，用于 TTL 验证（新鲜度）
	LastAccess time.Time
	element    *list.Element
	Path       string
	Data       []byte
	Size       int64
}

// FileCache 文件缓存，支持 LRU 淘汰策略。
//
// 该结构体实现了基于内存的文件缓存，支持按条目数和内存大小限制进行淘汰。
// 使用 LRU（最近最少使用）算法决定淘汰顺序。
//
// 注意事项：
//   - 所有方法均为并发安全
//   - 支持过期时间自动淘汰
type FileCache struct {
	entries     map[string]*FileEntry
	lruList     *list.List
	maxEntries  int64
	maxSize     int64
	inactive    time.Duration
	currentSize int64
	mu          sync.RWMutex
}

// NewFileCache 创建文件缓存实例。
//
// 根据指定的条目数限制、内存大小限制和过期时间创建缓存。
//
// 参数：
//   - maxEntries: 最大缓存条目数，设为 0 表示不限制
//   - maxSize: 内存使用上限（字节），设为 0 表示不限制
//   - inactive: 未访问淘汰时间，超过此时间未访问的条目将被淘汰
//
// 返回值：
//   - *FileCache: 创建的文件缓存实例
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
//
// 根据文件路径查找缓存条目，如果找到且未过期则返回。
// 访问时会更新条目的访问时间并移动到 LRU 链表头部。
//
// 参数：
//   - path: 文件路径，作为缓存键
//
// 返回值：
//   - *FileEntry: 缓存条目，包含文件内容和元数据
//   - bool: 是否找到有效缓存
func (c *FileCache) Get(path string) (*FileEntry, bool) {
	c.mu.Lock()

	entry, ok := c.entries[path]
	if !ok {
		c.mu.Unlock()
		return nil, false
	}

	// 检查是否过期
	if time.Since(entry.LastAccess) > c.inactive {
		c.removeEntry(entry)
		c.mu.Unlock()
		return nil, false
	}

	// 迁移处理: CachedAt 为零值时视为刚刚缓存（旧条目）
	// 在锁内执行，确保并发安全
	if entry.CachedAt.IsZero() {
		entry.CachedAt = time.Now()
	}

	// 更新访问时间并移到 LRU 链表头部
	entry.LastAccess = time.Now()
	c.lruList.MoveToFront(entry.element)
	c.mu.Unlock()
	return entry, true
}

// Set 设置缓存条目。
//
// 将文件内容存入缓存，如果缓存已存在则更新。
// 存入后检查是否需要触发 LRU 淘汰。
//
// 参数：
//   - path: 文件路径，作为缓存键
//   - data: 文件内容字节
//   - size: 文件大小（字节）
//   - modTime: 文件最后修改时间
//
// 返回值：
//   - error: 当前实现始终返回 nil
func (c *FileCache) Set(path string, data []byte, size int64, modTime time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 检查是否已存在
	if entry, ok := c.entries[path]; ok {
		c.currentSize -= entry.Size
		entry.Data = data
		entry.Size = size
		entry.ModTime = modTime
		entry.CachedAt = time.Now() // 更新缓存时间
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
		CachedAt:   time.Now(), // 设置缓存时间
		LastAccess: time.Now(),
	}
	entry.element = c.lruList.PushFront(entry)
	c.entries[path] = entry
	c.currentSize += size

	c.evictIfNeeded()
	return nil
}

// Delete 删除缓存条目。
//
// 根据文件路径删除对应的缓存条目。
//
// 参数：
//   - path: 文件路径，作为缓存键
func (c *FileCache) Delete(path string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if entry, ok := c.entries[path]; ok {
		c.removeEntry(entry)
	}
}

// RefreshCachedAt 更新 CachedAt 并移动 LRU 位置。
//
// 用于 TTL 过期但文件未修改时刷新缓存时间。
//
// 参数：
//   - path: 文件路径，作为缓存键
func (c *FileCache) RefreshCachedAt(path string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if entry, ok := c.entries[path]; ok {
		entry.CachedAt = time.Now()
		entry.LastAccess = time.Now()
		c.lruList.MoveToFront(entry.element)
	}
}

// removeEntry 内部删除条目（不加锁）。
//
// 从 LRU 链表和条目映射中移除指定条目，更新当前内存使用量。
// 调用此方法前必须已持有写锁。
//
// 参数：
//   - entry: 要删除的缓存条目
func (c *FileCache) removeEntry(entry *FileEntry) {
	c.lruList.Remove(entry.element)
	delete(c.entries, entry.Path)
	c.currentSize -= entry.Size
}

// evictIfNeeded 根据限制淘汰条目。
//
// 检查当前缓存是否超过条目数或内存大小限制，
// 如果超过则调用 evictLRU 淘汰最久未使用的条目。
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
//
// 从 LRU 链表尾部移除条目并删除。
// 如果链表为空则不执行任何操作。
func (c *FileCache) evictLRU() {
	if c.lruList.Len() == 0 {
		return
	}

	element := c.lruList.Back()
	if element == nil {
		return
	}

	entry, ok := element.Value.(*FileEntry)
	if !ok {
		return
	}
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
		Entries:    int64(len(c.entries)),
		MaxEntries: c.maxEntries,
		Size:       c.currentSize,
		MaxSize:    c.maxSize,
	}
}

// FileCacheStats 文件缓存统计。
type FileCacheStats struct {
	// Entries 当前缓存条目数量
	Entries int64

	// MaxEntries 最大缓存条目数限制
	MaxEntries int64

	// Size 当前缓存使用的内存大小（字节）
	Size int64

	// MaxSize 最大内存使用限制（字节）
	MaxSize int64
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
	Created time.Time
	Headers map[string]string
	Key     string
	OrigKey string
	Data    []byte
	Status  int
	MaxAge  time.Duration
}

// ProxyCache 代理响应缓存，支持缓存锁防击穿。
type ProxyCache struct {
	entries   map[uint64]*ProxyCacheEntry
	pending   map[uint64]*pendingRequest
	rules     []ProxyCacheRule
	staleTime time.Duration
	mu        sync.RWMutex
	cacheLock bool
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
		entries:   make(map[uint64]*ProxyCacheEntry),
		cacheLock: cacheLock,
		pending:   make(map[uint64]*pendingRequest),
		staleTime: staleTime,
	}
}

// Get 获取缓存的代理响应。
// hashKey 是 uint64 哈希值，origKey 是原始 key 用于碰撞验证。
func (c *ProxyCache) Get(hashKey uint64, origKey string) (*ProxyCacheEntry, bool, bool) {
	c.mu.RLock()
	entry, ok := c.entries[hashKey]
	c.mu.RUnlock()

	if !ok {
		return nil, false, false
	}

	// 双重验证：检查原始 key 是否匹配（防止哈希碰撞）
	if entry.OrigKey != origKey {
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
func (c *ProxyCache) Set(hashKey uint64, origKey string, data []byte, headers map[string]string, status int, maxAge time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[hashKey] = &ProxyCacheEntry{
		Key:     origKey, // 存储原始 key 作为 Key 字段（保持兼容性）
		OrigKey: origKey,
		Data:    data,
		Headers: headers,
		Status:  status,
		Created: time.Now(),
		MaxAge:  maxAge,
	}

	// 如果有等待的请求，通知它们
	if pending, ok := c.pending[hashKey]; ok {
		pending.err = nil
		close(pending.done)
		delete(c.pending, hashKey)
	}
}

// AcquireLock 获取缓存生成锁（防止击穿）。
// 如果返回 nil，表示获得锁，应该去生成缓存。
// 如果返回 chan，表示有其他请求正在生成，应该等待。
func (c *ProxyCache) AcquireLock(hashKey uint64) <-chan struct{} {
	if !c.cacheLock {
		return nil // 不使用缓存锁
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// 检查是否已有缓存
	if _, ok := c.entries[hashKey]; ok {
		return nil
	}

	// 检查是否有 pending 请求
	if pending, ok := c.pending[hashKey]; ok {
		return pending.done // 等待现有请求
	}

	// 创建新的 pending 请求
	pending := &pendingRequest{
		done: make(chan struct{}),
	}
	c.pending[hashKey] = pending
	return nil // 获得锁，应该生成缓存
}

// ReleaseLock 释放缓存生成锁。
func (c *ProxyCache) ReleaseLock(hashKey uint64, err error) {
	if !c.cacheLock {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if pending, ok := c.pending[hashKey]; ok {
		pending.err = err
		close(pending.done)
		delete(c.pending, hashKey)
	}
}

// MatchRule 检查请求是否匹配缓存规则。
func (c *ProxyCache) MatchRule(path, method string, status int) *ProxyCacheRule {
	for _, rule := range c.rules {
		// 检查路径匹配（简单前缀匹配）
		if rule.Path != "" && !MatchPattern(rule.Path, path) {
			continue
		}

		// 检查方法
		if len(rule.Methods) > 0 && !slices.Contains(rule.Methods, method) {
			continue
		}

		// 检查状态码
		if len(rule.Statuses) > 0 && !slices.Contains(rule.Statuses, status) {
			continue
		}

		return &rule
	}
	return nil
}

// Delete 删除缓存条目。
func (c *ProxyCache) Delete(hashKey uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, hashKey)
}

// DeleteByPatternWithMethod 按通配符模式删除缓存条目。
// method 过滤：检查 entry.OrigKey 是否以 "method:" 前缀开头。
// 空 method 匹配所有条目。
func (c *ProxyCache) DeleteByPatternWithMethod(pattern string, method string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	deleted := 0
	for hashKey, entry := range c.entries {
		if MatchPattern(pattern, entry.OrigKey) {
			if method == "" || strings.HasPrefix(entry.OrigKey, method+":") {
				delete(c.entries, hashKey)
				deleted++
			}
		}
	}
	return deleted
}

// Clear 清空代理缓存。
func (c *ProxyCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[uint64]*ProxyCacheEntry)
	c.pending = make(map[uint64]*pendingRequest)
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
	// Entries 当前缓存条目数量
	Entries int

	// Pending 正在等待缓存生成的请求数量
	Pending int
}
