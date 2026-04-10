// Package resolver 提供 DNS 解析功能，支持缓存和后台刷新。
//
// 该文件包含 DNS 解析器统计指标相关的实现。
//
// 作者：xfy
package resolver

import (
	"time"
)

// StatsCollector 统计收集器接口。
type StatsCollector interface {
	// RecordHit 记录缓存命中
	RecordHit()
	// RecordMiss 记录缓存未命中
	RecordMiss()
	// RecordError 记录解析错误
	RecordError()
	// RecordLatency 记录解析延迟
	RecordLatency(latency time.Duration)
	// GetStats 获取当前统计
	GetStats() Stats
}

// ResetStats 重置所有统计信息。
func (r *DNSResolver) ResetStats() {
	r.hits.Store(0)
	r.misses.Store(0)
	r.errors.Store(0)
	r.latencyNs.Store(0)
	r.count.Store(0)
}

// GetCacheHits 返回缓存命中次数。
func (r *DNSResolver) GetCacheHits() int64 {
	return r.hits.Load()
}

// GetCacheMisses 返回缓存未命中次数。
func (r *DNSResolver) GetCacheMisses() int64 {
	return r.misses.Load()
}

// GetResolveErrors 返回解析错误次数。
func (r *DNSResolver) GetResolveErrors() int64 {
	return r.errors.Load()
}

// GetTotalQueries 返回总查询次数。
func (r *DNSResolver) GetTotalQueries() int64 {
	return r.hits.Load() + r.misses.Load()
}

// GetAverageLatency 返回平均解析延迟。
func (r *DNSResolver) GetAverageLatency() time.Duration {
	count := r.count.Load()
	if count == 0 {
		return 0
	}
	return time.Duration(r.latencyNs.Load() / count)
}
