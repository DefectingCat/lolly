// Package resolver 提供 DNS 解析功能，支持缓存和后台刷新。
//
// 该文件包含 DNS 缓存相关的实现。
//
// 作者：xfy
package resolver

import (
	"time"
)

// CacheStats 返回缓存统计信息。
type CacheStats struct {
	Hits    int64 // 缓存命中次数
	Misses  int64 // 缓存未命中次数
	Entries int   // 当前缓存条目数
	Expired int   // 过期条目数
}

// GetCacheStats 返回当前缓存统计信息。
// 这是一个辅助函数，用于测试和监控。
func (r *DNSResolver) GetCacheStats() CacheStats {
	hits := r.hits.Load()
	misses := r.misses.Load()

	// 统计缓存条目
	var entries, expired int
	now := time.Now()
	r.mu.RLock()
	for _, entry := range r.cache {
		entries++
		entry.mu.RLock()
		if now.After(entry.ExpiresAt) {
			expired++
		}
		entry.mu.RUnlock()
	}
	r.mu.RUnlock()

	return CacheStats{
		Hits:    hits,
		Misses:  misses,
		Entries: entries,
		Expired: expired,
	}
}

// GetCacheEntry 获取指定主机的缓存条目（用于测试）。
func (r *DNSResolver) GetCacheEntry(host string) (*DNSCacheEntry, bool) {
	r.mu.RLock()
	entry, ok := r.cache[host]
	r.mu.RUnlock()
	if !ok {
		return nil, false
	}
	return entry, true
}

// DeleteCacheEntry 删除指定主机的缓存条目。
func (r *DNSResolver) DeleteCacheEntry(host string) {
	r.mu.Lock()
	delete(r.cache, host)
	// 从 LRU 链表中移除
	for i, h := range r.lruOrder {
		if h == host {
			r.lruOrder = append(r.lruOrder[:i], r.lruOrder[i+1:]...)
			break
		}
	}
	delete(r.refreshHosts, host)
	r.mu.Unlock()
}

// ClearCache 清空所有缓存。
func (r *DNSResolver) ClearCache() {
	r.mu.Lock()
	r.cache = make(map[string]*DNSCacheEntry)
	r.lruOrder = make([]string, 0, r.config.CacheSize)
	r.refreshHosts = make(map[string]struct{})
	r.mu.Unlock()
}

// GetHitRate 返回缓存命中率。
func (r *DNSResolver) GetHitRate() float64 {
	hits := r.hits.Load()
	misses := r.misses.Load()
	total := hits + misses
	if total == 0 {
		return 0
	}
	return float64(hits) / float64(total)
}

// IsCached 检查指定主机是否在缓存中且未过期。
func (r *DNSResolver) IsCached(host string) bool {
	r.mu.RLock()
	entry, ok := r.cache[host]
	r.mu.RUnlock()
	if !ok {
		return false
	}
	entry.mu.RLock()
	expiresAt := entry.ExpiresAt
	entry.mu.RUnlock()
	return time.Now().Before(expiresAt)
}
