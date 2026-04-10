// Package resolver 提供 DNS 解析功能，支持缓存和后台刷新。
//
// 该文件包含 DNS 解析器统计指标相关的实现，包括：
//   - 缓存命中/未命中次数统计
//   - 解析错误次数统计
//   - 解析延迟统计
//   - 统计数据查询接口
//
// 主要用途：
//
//	用于监控 DNS 解析器的性能指标，支持实时获取缓存命中率、
//	平均延迟等关键指标，便于系统性能分析和调优。
//
// 注意事项：
//   - 所有统计方法均为原子操作，并发安全
//   - 延迟统计使用纳秒级精度累加
//
// 作者：xfy
package resolver

import (
	"time"
)

// StatsCollector 定义 DNS 解析器统计收集器的接口。
//
// 该接口用于抽象统计数据的收集和查询操作，支持：
//   - 记录缓存命中和未命中事件
//   - 记录解析错误
//   - 记录解析延迟
//   - 获取汇总统计数据
//
// 实现要求：
//   - 所有方法必须是并发安全的
//   - 统计数据的更新应使用原子操作
//   - GetStats 返回的数据应反映当前时刻的快照
//
// 使用场景：
//
//	通常由 DNSResolver 实现此接口，供监控系统或管理接口调用。
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

// ResetStats 重置所有统计信息为初始值。
//
// 将缓存命中次数、缓存未命中次数、解析错误次数、
// 总延迟时间和查询次数全部清零。
//
// 注意事项：
//   - 该操作会清除所有历史统计数据，请谨慎使用
//   - 使用原子操作确保并发安全
func (r *DNSResolver) ResetStats() {
	r.hits.Store(0)
	r.misses.Store(0)
	r.errors.Store(0)
	r.latencyNs.Store(0)
	r.count.Store(0)
}

// GetCacheHits 返回缓存命中次数。
//
// 缓存命中表示请求的数据已存在于缓存中，无需进行实际的 DNS 查询。
// 该指标可用于评估缓存的有效性。
//
// 返回值：
//   - 缓存命中次数的累计值（int64 类型）
func (r *DNSResolver) GetCacheHits() int64 {
	return r.hits.Load()
}

// GetCacheMisses 返回缓存未命中次数。
//
// 缓存未命中表示请求的数据不在缓存中，需要进行实际的 DNS 查询。
// 结合缓存命中次数可计算缓存命中率：命中率 = 命中次数 / (命中次数 + 未命中次数)。
//
// 返回值：
//   - 缓存未命中次数的累计值（int64 类型）
func (r *DNSResolver) GetCacheMisses() int64 {
	return r.misses.Load()
}

// GetResolveErrors 返回解析错误次数。
//
// 解析错误表示 DNS 查询过程中发生的各种错误，包括网络错误、
// 超时、无效响应等。该指标可用于监控 DNS 服务的健康状态。
//
// 返回值：
//   - 解析错误次数的累计值（int64 类型）
func (r *DNSResolver) GetResolveErrors() int64 {
	return r.errors.Load()
}

// GetTotalQueries 返回总查询次数。
//
// 总查询次数等于缓存命中次数与缓存未命中次数之和。
// 该指标反映 DNS 解析器处理的所有请求总数。
//
// 返回值：
//   - 总查询次数（int64 类型）
func (r *DNSResolver) GetTotalQueries() int64 {
	return r.hits.Load() + r.misses.Load()
}

// GetAverageLatency 返回平均解析延迟。
//
// 平均延迟通过总延迟时间除以查询次数计算得出。
// 该指标反映 DNS 解析的平均响应时间，单位为 time.Duration。
// 当没有查询记录时返回 0。
//
// 返回值：
//   - 平均解析延迟（time.Duration 类型）
//
// 注意事项：
//   - 首次调用或 ResetStats 后若未进行任何查询，返回 0
//   - 延迟统计包括缓存命中和缓存未命中的请求
func (r *DNSResolver) GetAverageLatency() time.Duration {
	count := r.count.Load()
	if count == 0 {
		return 0
	}
	return time.Duration(r.latencyNs.Load() / count)
}
