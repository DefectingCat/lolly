package loadbalance

import (
	"time"
)

// ResponseTimeRecorder 响应时间记录接口。
type ResponseTimeRecorder interface {
	RecordResponseTime(target *Target, headerTime, lastByteTime time.Duration)
}

// LeastTime 基于响应时间 EWMA 的负载均衡器。
type LeastTime struct {
	metric      string
	defaultTime time.Duration
}

// NewLeastTime 创建一个新的基于响应时间的负载均衡器。
//
// 参数：
//   - metric: 选择指标，"header" 表示使用首字节时间，其他值使用完整响应时间
//   - defaultTime: 无统计信息时的默认响应时间，必须 > 0
func NewLeastTime(metric string, defaultTime time.Duration) *LeastTime {
	if metric != "header" {
		metric = "last_byte"
	}
	if defaultTime <= 0 {
		defaultTime = time.Millisecond
	}
	return &LeastTime{
		metric:      metric,
		defaultTime: defaultTime,
	}
}

// Select 根据响应时间 EWMA 选择一个目标。
// 只考虑可用目标。如果没有可用目标则返回 nil。
func (l *LeastTime) Select(targets []*Target) *Target {
	fc := acquireFilterContext()
	defer releaseFilterContext(fc)
	available := filterInto(fc, targets)
	return l.selectFrom(available)
}

// SelectExcluding 根据响应时间 EWMA 选择一个目标，排除指定的目标列表。
// 用于故障转移场景，避免选择已失败的目标。
func (l *LeastTime) SelectExcluding(targets []*Target, excluded []*Target) *Target {
	fc := acquireFilterContext()
	defer releaseFilterContext(fc)
	available := filterIntoExcluding(fc, targets, excluded)
	return l.selectFrom(available)
}

// selectFrom 从可用目标列表中选择响应时间最短的目标。
func (l *LeastTime) selectFrom(available []*Target) *Target {
	if len(available) == 0 {
		return nil
	}

	var selected *Target
	var minTime int64 = -1
	defaultNano := l.defaultTime.Nanoseconds()

	for _, t := range available {
		var currentTime int64
		if t.Stats != nil {
			if l.metric == "header" {
				currentTime = int64(t.Stats.HeaderTime())
			} else {
				currentTime = int64(t.Stats.LastByteTime())
			}
		}

		if currentTime == 0 {
			currentTime = defaultNano
		}

		if selected == nil || currentTime < minTime {
			selected = t
			minTime = currentTime
		}
	}

	return selected
}

// RecordResponseTime 记录目标服务器的响应时间。
// 更新目标的 EWMA 统计信息。
func (l *LeastTime) RecordResponseTime(target *Target, headerTime, lastByteTime time.Duration) {
	if target != nil && target.Stats != nil {
		target.Stats.Record(headerTime, lastByteTime)
	}
}

// GetMetric 返回当前使用的响应时间指标。
func (l *LeastTime) GetMetric() string {
	return l.metric
}

var (
	_ Balancer             = (*LeastTime)(nil)
	_ ResponseTimeRecorder = (*LeastTime)(nil)
)
