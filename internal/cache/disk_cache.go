// Package cache 提供文件缓存和代理缓存功能。
//
// 该文件实现 DiskCache 磁盘缓存后端，支持：
//   - 目录层级配置（levels=1:2）
//   - 原子写入策略（.tmp → .data）
//   - 懒加载（后台加载元数据，不阻塞启动）
//   - CRC32 校验和验证数据完整性
//
// 主要用途：
//
//	作为 L2 缓存层，持久化代理响应到磁盘，支持服务重启后恢复。
//
// 作者：xfy
package cache

import (
	"encoding/json"
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

// DiskCacheConfig 磁盘缓存配置。
type DiskCacheConfig struct {
	// Path 缓存根目录
	Path string

	// Levels 目录层级，如 "1:2" 表示两级目录
	Levels string

	// MaxSize 最大缓存大小（字节）
	MaxSize int64

	// Inactive 未访问淘汰时间
	Inactive time.Duration

	// StaleIfError 错误时使用过期缓存的窗口
	StaleIfError time.Duration

	// StaleIfTimeout 超时时使用过期缓存的窗口
	StaleIfTimeout time.Duration
}

// DiskCache 磁盘缓存实现。
type DiskCache struct {
	basePath       string
	levels         []int
	maxSize        int64
	inactive       time.Duration
	staleIfError   time.Duration
	staleIfTimeout time.Duration
	currentSize    atomic.Int64
	entries        map[uint64]*DiskCacheMeta
	mu             sync.RWMutex
	stopCh         chan struct{}

	// 懒加载相关
	loaded atomic.Bool
	loadCh chan struct{}

	// 统计
	hitCount  atomic.Int64
	missCount atomic.Int64
	evictions atomic.Int64
}

// DiskCacheMeta 磁盘缓存元数据。
type DiskCacheMeta struct {
	HashKey uint64            `json:"hash_key"`
	OrigKey string            `json:"orig_key"`
	Created time.Time         `json:"created"`
	MaxAge  time.Duration     `json:"max_age"`
	Status  int               `json:"status"`
	Size    int64             `json:"size"`
	Headers map[string]string `json:"headers,omitempty"`
	CRC32   uint32            `json:"crc32"`
}

// DiskCacheEntry 磁盘缓存条目（包含数据）。
type DiskCacheEntry struct {
	*DiskCacheMeta
	Data []byte
}

// NewDiskCache 创建磁盘缓存实例（懒加载模式）。
func NewDiskCache(cfg *DiskCacheConfig) (*DiskCache, error) {
	if cfg.Path == "" {
		return nil, fmt.Errorf("disk cache path is required")
	}

	// 确保目录存在
	if err := os.MkdirAll(cfg.Path, 0o755); err != nil {
		return nil, fmt.Errorf("create cache directory: %w", err)
	}

	dc := &DiskCache{
		basePath:       cfg.Path,
		levels:         parseLevels(cfg.Levels),
		maxSize:        cfg.MaxSize,
		inactive:       cfg.Inactive,
		staleIfError:   cfg.StaleIfError,
		staleIfTimeout: cfg.StaleIfTimeout,
		entries:        make(map[uint64]*DiskCacheMeta),
		loadCh:         make(chan struct{}),
		stopCh:         make(chan struct{}),
	}

	// 启动后台加载，不阻塞服务启动
	go dc.lazyLoad()

	return dc, nil
}

// parseLevels 解析目录层级配置。
// 支持格式：""（无层级）、"1"（一级）、"1:2"（两级）
func parseLevels(levels string) []int {
	if levels == "" {
		return nil
	}

	var result []int
	start := 0
	for i := 0; i <= len(levels); i++ {
		if i == len(levels) || levels[i] == ':' {
			var level int
			for j := start; j < i; j++ {
				level = level*10 + int(levels[j]-'0')
			}
			if level > 0 {
				result = append(result, level)
			}
			start = i + 1
		}
	}
	return result
}

// lazyLoad 后台加载缓存元数据。
func (dc *DiskCache) lazyLoad() {
	defer close(dc.loadCh)

	// 扫描目录加载元数据（不加载实际数据）
	_ = filepath.Walk(dc.basePath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}

		// 只处理 .meta 文件
		if filepath.Ext(path) != ".meta" {
			return nil
		}

		meta := dc.loadMetaFile(path)
		if meta != nil {
			dc.mu.Lock()
			dc.entries[meta.HashKey] = meta
			dc.mu.Unlock()
			dc.currentSize.Add(meta.Size)
		}

		return nil
	})

	dc.loaded.Store(true)
}

// loadMetaFile 加载元数据文件。
func (dc *DiskCache) loadMetaFile(path string) *DiskCacheMeta {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	var meta DiskCacheMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil
	}

	return &meta
}

// Get 获取缓存条目（实现 CacheBackend 接口）。
func (dc *DiskCache) Get(hashKey uint64, origKey string) (*ProxyCacheEntry, bool, bool) {
	// 如果未完成加载，等待加载完成或超时
	if !dc.loaded.Load() {
		select {
		case <-dc.loadCh:
			// 加载完成，继续
		case <-time.After(100 * time.Millisecond):
			// 超时，返回未命中（避免阻塞请求）
			dc.missCount.Add(1)
			return nil, false, false
		}
	}

	dc.mu.RLock()
	meta, exists := dc.entries[hashKey]
	dc.mu.RUnlock()

	if !exists {
		dc.missCount.Add(1)
		return nil, false, false
	}

	// 双重验证：检查原始 key 是否匹配
	if meta.OrigKey != origKey {
		dc.missCount.Add(1)
		return nil, false, false
	}

	// 读取数据文件
	dataPath := dc.filePathFromHash(hashKey, "data")
	data, err := os.ReadFile(dataPath)
	if err != nil {
		dc.missCount.Add(1)
		return nil, false, false
	}

	// 验证 CRC32
	if meta.CRC32 != 0 {
		if crc32.ChecksumIEEE(data) != meta.CRC32 {
			// 数据损坏，删除条目
			_ = dc.Delete(hashKey)
			dc.missCount.Add(1)
			return nil, false, false
		}
	}

	// 检查是否过期
	now := time.Now()
	expiresAt := meta.Created.Add(meta.MaxAge)
	stale := now.After(expiresAt)

	dc.hitCount.Add(1)

	// 转换为 ProxyCacheEntry
	entry := &ProxyCacheEntry{
		Key:     meta.OrigKey,
		OrigKey: meta.OrigKey,
		Data:    data,
		Headers: meta.Headers,
		Status:  meta.Status,
		Created: meta.Created,
		MaxAge:  meta.MaxAge,
	}

	return entry, true, stale
}

// GetStale 在上游错误时获取可用的过期缓存。
//
// 与 Get 不同，GetStale 只在错误发生时使用，根据错误类型检查对应的 stale 窗口。
func (dc *DiskCache) GetStale(hashKey uint64, origKey string, isTimeout bool) (*ProxyCacheEntry, bool) {
	// 等待懒加载完成
	if !dc.loaded.Load() {
		select {
		case <-dc.loadCh:
		case <-time.After(100 * time.Millisecond):
			return nil, false
		}
	}

	dc.mu.RLock()
	meta, ok := dc.entries[hashKey]
	dc.mu.RUnlock()

	if !ok {
		return nil, false
	}

	// 双重验证：检查原始 key 是否匹配
	if meta.OrigKey != origKey {
		return nil, false
	}

	// 读取数据文件
	dataPath := dc.filePathFromHash(hashKey, "data")
	data, err := os.ReadFile(dataPath)
	if err != nil {
		return nil, false
	}

	// 验证 CRC32
	crc := crc32.ChecksumIEEE(data)
	if crc != meta.CRC32 {
		return nil, false
	}

	now := time.Now()
	expiresAt := meta.Created.Add(meta.MaxAge)

	// 未过期，直接返回
	if !now.After(expiresAt) {
		entry := &ProxyCacheEntry{
			Key:     meta.OrigKey,
			OrigKey: meta.OrigKey,
			Data:    data,
			Headers: meta.Headers,
			Status:  meta.Status,
			Created: meta.Created,
			MaxAge:  meta.MaxAge,
		}
		return entry, true
	}

	// 已过期，检查 stale 窗口
	var staleWindow time.Duration
	if isTimeout {
		staleWindow = dc.staleIfTimeout
	} else {
		staleWindow = dc.staleIfError
	}

	if staleWindow <= 0 {
		return nil, false
	}

	// 检查是否在 stale 窗口内
	if now.Sub(expiresAt) > staleWindow {
		return nil, false
	}

	entry := &ProxyCacheEntry{
		Key:     meta.OrigKey,
		OrigKey: meta.OrigKey,
		Data:    data,
		Headers: meta.Headers,
		Status:  meta.Status,
		Created: meta.Created,
		MaxAge:  meta.MaxAge,
	}

	return entry, true
}

// Set 设置缓存条目（实现 CacheBackend 接口）。
func (dc *DiskCache) Set(hashKey uint64, origKey string, data []byte, headers map[string]string, status int, maxAge time.Duration) {
	// 计算文件路径
	dataPath := dc.filePathFromHash(hashKey, "data")
	metaPath := dc.filePathFromHash(hashKey, "meta")

	// 确保目录存在
	dir := filepath.Dir(dataPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}

	// 计算 CRC32
	crc := crc32.ChecksumIEEE(data)

	// 创建元数据
	meta := &DiskCacheMeta{
		HashKey: hashKey,
		OrigKey: origKey,
		Created: time.Now(),
		MaxAge:  maxAge,
		Status:  status,
		Size:    int64(len(data)),
		Headers: headers,
		CRC32:   crc,
	}

	// 原子写入数据文件：先写临时文件，再重命名
	tmpDataPath := dataPath + ".tmp"
	if err := os.WriteFile(tmpDataPath, data, 0o644); err != nil {
		return
	}
	if err := os.Rename(tmpDataPath, dataPath); err != nil {
		os.Remove(tmpDataPath)
		return
	}

	// 写入元数据文件
	metaData, err := json.Marshal(meta)
	if err != nil {
		return
	}
	tmpMetaPath := metaPath + ".tmp"
	if err := os.WriteFile(tmpMetaPath, metaData, 0o644); err != nil {
		return
	}
	if err := os.Rename(tmpMetaPath, metaPath); err != nil {
		os.Remove(tmpMetaPath)
		return
	}

	// 更新内存索引
	dc.mu.Lock()
	oldMeta, existed := dc.entries[hashKey]
	dc.entries[hashKey] = meta
	dc.mu.Unlock()

	// 更新大小统计
	if existed && oldMeta != nil {
		dc.currentSize.Add(-oldMeta.Size)
	}
	dc.currentSize.Add(meta.Size)

	// 检查是否需要淘汰
	if dc.maxSize > 0 && dc.currentSize.Load() > dc.maxSize {
		go dc.evict()
	}
}

// Delete 删除缓存条目（实现 CacheBackend 接口）。
func (dc *DiskCache) Delete(hashKey uint64) error {
	dc.mu.Lock()
	meta, exists := dc.entries[hashKey]
	if exists {
		delete(dc.entries, hashKey)
		dc.currentSize.Add(-meta.Size)
		dc.evictions.Add(1)
	}
	dc.mu.Unlock()

	if !exists {
		return nil
	}

	// 删除文件
	dataPath := dc.filePathFromHash(hashKey, "data")
	metaPath := dc.filePathFromHash(hashKey, "meta")
	os.Remove(dataPath)
	os.Remove(metaPath)

	return nil
}

// CacheStats 返回缓存统计信息（实现 CacheBackend 接口）。
func (dc *DiskCache) CacheStats() CacheStats {
	dc.mu.RLock()
	entries := int64(len(dc.entries))
	dc.mu.RUnlock()

	return CacheStats{
		Entries:   entries,
		Size:      dc.currentSize.Load(),
		HitCount:  dc.hitCount.Load(),
		MissCount: dc.missCount.Load(),
		Evictions: dc.evictions.Load(),
	}
}

// Stop 停止磁盘缓存。
func (dc *DiskCache) Stop() {
	close(dc.stopCh)
}

// filePathFromHash 根据哈希值计算文件路径。
func (dc *DiskCache) filePathFromHash(hashKey uint64, ext string) string {
	// 将哈希值转换为十六进制字符串
	hashStr := fmt.Sprintf("%016x", hashKey)

	if len(dc.levels) == 0 {
		return filepath.Join(dc.basePath, hashStr+"."+ext)
	}

	// 根据层级构建路径
	parts := []string{dc.basePath}
	offset := len(hashStr)
	for _, level := range dc.levels {
		if offset < level {
			break
		}
		offset -= level
		parts = append(parts, hashStr[offset:offset+level])
	}
	parts = append(parts, hashStr+"."+ext)

	return filepath.Join(parts...)
}

// evict 淘汰旧条目。
func (dc *DiskCache) evict() {
	// 简单策略：删除最旧的条目直到大小低于阈值
	targetSize := dc.maxSize * 9 / 10 // 淘汰到 90%

	for dc.currentSize.Load() > targetSize {
		dc.mu.Lock()
		// 找到最旧的条目
		var oldestKey uint64
		var oldestTime time.Time
		for key, meta := range dc.entries {
			if oldestKey == 0 || meta.Created.Before(oldestTime) {
				oldestKey = key
				oldestTime = meta.Created
			}
		}

		if oldestKey == 0 {
			dc.mu.Unlock()
			break
		}

		meta := dc.entries[oldestKey]
		delete(dc.entries, oldestKey)
		dc.currentSize.Add(-meta.Size)
		dc.evictions.Add(1)
		dc.mu.Unlock()

		// 删除文件
		dataPath := dc.filePathFromHash(oldestKey, "data")
		metaPath := dc.filePathFromHash(oldestKey, "meta")
		os.Remove(dataPath)
		os.Remove(metaPath)
	}
}
