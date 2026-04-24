// Package cache 提供文件缓存和代理缓存功能。
//
// 该文件实现 TieredCache 分层缓存，支持：
//   - L1 内存缓存（热点数据）
//   - L2 磁盘缓存（持久化）
//   - 热点提升机制
//   - L2 过期检查
//
// 主要用途：
//
//	作为代理缓存的主要实现，平衡性能（L1）和容量（L2）。
//
// 作者：xfy
package cache

import (
	"sync"
	"sync/atomic"
	"time"
)

// TieredCacheConfig 分层缓存配置。
type TieredCacheConfig struct {
	// L1 配置
	L1MaxEntries int64
	L1MaxSize    int64

	// L2 配置
	L2Config *DiskCacheConfig

	// Stale 配置
	StaleIfError   time.Duration // 错误时使用过期缓存的窗口
	StaleIfTimeout time.Duration // 超时时使用过期缓存的窗口

	// 热点提升配置
	PromoteThreshold int           // 访问次数阈值，超过后提升到 L1
	PromoteInterval  time.Duration // 提升检查间隔
}

// TieredCache 分层缓存（L1 内存 + L2 磁盘）。
type TieredCache struct {
	l1       *ProxyCache // L1 内存缓存（热点数据）
	l2       *DiskCache  // L2 磁盘缓存（持久化）
	l1Ratio  float64     // L1 容量占 L2 的比例
	promoter *promoter   // 热点提升器
	stopCh   chan struct{}

	// 统计
	l1Hits   atomic.Int64
	l2Hits   atomic.Int64
	misses   atomic.Int64
	promotes atomic.Int64
}

// promoter 热点提升器。
type promoter struct {
	threshold int           // 访问次数阈值
	interval  time.Duration // 检查间隔
	accessMap map[uint64]*accessInfo
	mu        sync.RWMutex
}

// accessInfo 访问信息。
type accessInfo struct {
	count      int
	lastAccess time.Time
	origKey    string
}

// NewTieredCache 创建分层缓存实例。
func NewTieredCache(cfg *TieredCacheConfig) (*TieredCache, error) {
	// 创建 L1 内存缓存
	l1 := NewProxyCache(nil, true, 0, cfg.StaleIfError, cfg.StaleIfTimeout)

	// 创建 L2 磁盘缓存
	l2, err := NewDiskCache(cfg.L2Config)
	if err != nil {
		return nil, err
	}

	tc := &TieredCache{
		l1:      l1,
		l2:      l2,
		l1Ratio: 0.1, // 默认 L1 占 10%
		promoter: &promoter{
			threshold: cfg.PromoteThreshold,
			interval:  cfg.PromoteInterval,
			accessMap: make(map[uint64]*accessInfo),
		},
		stopCh: make(chan struct{}),
	}

	// 启动热点提升检查
	if tc.promoter.threshold > 0 {
		go tc.promoteLoop()
	}

	return tc, nil
}

// Get 获取缓存条目（实现 CacheBackend 接口）。
func (tc *TieredCache) Get(hashKey uint64, origKey string) (*ProxyCacheEntry, bool, bool) {
	// 1. 先查 L1
	entry, exists, stale := tc.l1.Get(hashKey, origKey)
	if exists {
		tc.l1Hits.Add(1)
		return entry, true, stale
	}

	// 2. 查 L2，必须验证 max_age
	entry, exists, stale = tc.l2.Get(hashKey, origKey)
	if !exists {
		tc.misses.Add(1)
		return nil, false, false
	}

	tc.l2Hits.Add(1)

	// 3. 记录访问（用于热点提升）
	tc.recordAccess(hashKey, origKey)

	// 4. L2 命中且未过期，异步提升到 L1
	if !stale {
		go tc.promoteToL1(hashKey, entry)
	}

	return entry, true, stale
}

// GetStale 在上游错误时获取可用的过期缓存。
//
// 先查 L1，再查 L2。
func (tc *TieredCache) GetStale(hashKey uint64, origKey string, isTimeout bool) (*ProxyCacheEntry, bool) {
	// 1. 先查 L1
	if entry, ok := tc.l1.GetStale(hashKey, origKey, isTimeout); ok {
		tc.l1Hits.Add(1)
		return entry, true
	}

	// 2. 查 L2
	if entry, ok := tc.l2.GetStale(hashKey, origKey, isTimeout); ok {
		tc.l2Hits.Add(1)
		return entry, true
	}

	tc.misses.Add(1)
	return nil, false
}

// Set 设置缓存条目（实现 CacheBackend 接口）。
func (tc *TieredCache) Set(hashKey uint64, origKey string, data []byte, headers map[string]string, status int, maxAge time.Duration) {
	// 同时写入 L1 和 L2
	tc.l1.Set(hashKey, origKey, data, headers, status, maxAge)

	// L2 异步写入（在 goroutine 中执行）
	go tc.l2.Set(hashKey, origKey, data, headers, status, maxAge)
}

// Delete 删除缓存条目（实现 CacheBackend 接口）。
func (tc *TieredCache) Delete(hashKey uint64) error {
	_ = tc.l1.Delete(hashKey)
	return tc.l2.Delete(hashKey)
}

// CacheStats 返回缓存统计信息（实现 CacheBackend 接口）。
func (tc *TieredCache) CacheStats() CacheStats {
	l1Stats := tc.l1.CacheStats()
	l2Stats := tc.l2.CacheStats()

	return CacheStats{
		Entries:   l1Stats.Entries + l2Stats.Entries,
		Size:      l1Stats.Size + l2Stats.Size,
		HitCount:  tc.l1Hits.Load() + tc.l2Hits.Load(),
		MissCount: tc.misses.Load(),
		Evictions: l2Stats.Evictions,
	}
}

// TieredCacheStats 返回分层缓存详细统计。
func (tc *TieredCache) TieredCacheStats() TieredCacheStats {
	return TieredCacheStats{
		L1Hits:    tc.l1Hits.Load(),
		L2Hits:    tc.l2Hits.Load(),
		Misses:    tc.misses.Load(),
		Promotes:  tc.promotes.Load(),
		L1Entries: tc.l1.CacheStats().Entries,
		L2Entries: tc.l2.CacheStats().Entries,
	}
}

// TieredCacheStats 分层缓存详细统计。
type TieredCacheStats struct {
	L1Hits    int64
	L2Hits    int64
	Misses    int64
	Promotes  int64
	L1Entries int64
	L2Entries int64
}

// Stop 停止分层缓存。
func (tc *TieredCache) Stop() {
	close(tc.stopCh)
	if tc.l2 != nil {
		tc.l2.Stop()
	}
}

// recordAccess 记录访问（用于热点提升）。
func (tc *TieredCache) recordAccess(hashKey uint64, origKey string) {
	if tc.promoter.threshold <= 0 {
		return
	}

	tc.promoter.mu.Lock()
	defer tc.promoter.mu.Unlock()

	info, exists := tc.promoter.accessMap[hashKey]
	if !exists {
		info = &accessInfo{origKey: origKey}
		tc.promoter.accessMap[hashKey] = info
	}
	info.count++
	info.lastAccess = time.Now()
}

// promoteToL1 提升条目到 L1。
func (tc *TieredCache) promoteToL1(hashKey uint64, entry *ProxyCacheEntry) {
	tc.l1.Set(hashKey, entry.OrigKey, entry.Data, entry.Headers, entry.Status, entry.MaxAge)
	tc.promotes.Add(1)
}

// promoteLoop 热点提升检查循环。
func (tc *TieredCache) promoteLoop() {
	ticker := time.NewTicker(tc.promoter.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			tc.checkAndPromote()
		case <-tc.stopCh:
			return
		}
	}
}

// checkAndPromote 检查并提升热点数据。
func (tc *TieredCache) checkAndPromote() {
	tc.promoter.mu.Lock()
	defer tc.promoter.mu.Unlock()

	for hashKey, info := range tc.promoter.accessMap {
		if info.count >= tc.promoter.threshold {
			// 从 L2 获取并提升到 L1
			entry, exists, _ := tc.l2.Get(hashKey, info.origKey)
			if exists {
				tc.promoteToL1(hashKey, entry)
			}
			// 重置计数
			info.count = 0
		}
	}
}
