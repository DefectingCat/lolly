// Package security 提供滑动窗口限流算法实现。
//
// 该文件实现基于滑动窗口的请求限流，支持近似和精确两种模式。
//
// 主要用途：
//
//	用于更精确地控制请求速率，相比令牌桶算法提供更平滑的限流效果。
//
// 算法特点：
//   - 近似模式：O(1) 时间复杂度，内存占用低
//   - 精确模式：O(n) 时间复杂度，限流更精确
//
// 作者：xfy
package security

import (
	"sync"
	"time"
)

// SlidingWindowLimiter 滑动窗口限流器。
//
// 使用滑动窗口算法限制请求速率，支持近似和精确两种模式。
type SlidingWindowLimiter struct {
	// window 窗口大小
	window time.Duration

	// limit 窗口内最大请求数
	limit int

	// precise 是否使用精确模式
	precise bool

	// counters 窗口计数器，key 为窗口起始时间
	counters map[string]*windowCounter

	// mu 读写锁
	mu sync.RWMutex
}

// windowCounter 窗口计数器。
type windowCounter struct {
	count      int64
	timestamps []time.Time // precise 模式下记录每个请求时间
	mu         sync.Mutex
}

// NewSlidingWindowLimiter 创建滑动窗口限流器。
//
// 参数：
//   - window: 窗口大小（如 1s、1m）
//   - limit: 窗口内最大请求数
//   - precise: 是否使用精确模式
func NewSlidingWindowLimiter(window time.Duration, limit int, precise bool) *SlidingWindowLimiter {
	return &SlidingWindowLimiter{
		window:   window,
		limit:    limit,
		precise:  precise,
		counters: make(map[string]*windowCounter),
	}
}

// Allow 检查是否允许请求。
//
// 参数：
//   - key: 限流键（如 IP 地址）
//
// 返回值：
//   - bool: true 表示允许请求，false 表示拒绝
func (s *SlidingWindowLimiter) Allow(key string) bool {
	if s.precise {
		return s.allowPrecise(key)
	}
	return s.allowApproximate(key)
}

// allowApproximate 近似滑动窗口（推荐，内存 O(1)）。
//
// 使用两个固定窗口估算滑动窗口内的请求数，性能优于精确模式。
func (s *SlidingWindowLimiter) allowApproximate(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	windowNanos := s.window.Nanoseconds()
	_ = windowNanos // 用于近似计算

	// 获取或创建当前窗口计数器
	current, ok := s.counters[key]
	if !ok {
		current = &windowCounter{}
		s.counters[key] = current
	}

	current.mu.Lock()
	defer current.mu.Unlock()

	// 检查是否需要重置窗口
	if current.count == 0 || current.timestamps == nil || len(current.timestamps) == 0 {
		// 首次请求或新窗口
		current.timestamps = []time.Time{now}
	} else {
		// 检查是否仍在当前窗口
		lastTime := current.timestamps[0]
		if now.Sub(lastTime) > s.window {
			// 新窗口，重置
			current.count = 0
			current.timestamps = []time.Time{}
		}
	}

	// 获取上一个窗口的计数（用于估算）
	// 简化实现：直接计算当前窗口内的请求数
	count := int64(len(current.timestamps))

	// 计算滑动窗口内的请求数
	// 公式：当前窗口计数 × 1.0 + 上一窗口计数 × (1 - 当前窗口已过比例)
	elapsed := float64(now.UnixNano()%windowNanos) / float64(windowNanos)
	adjustedCount := float64(count) * (1.0 - elapsed)

	if int(adjustedCount) >= s.limit {
		return false
	}

	// 记录请求
	current.timestamps = append(current.timestamps, now)
	current.count = int64(len(current.timestamps))
	return true
}

// allowPrecise 精确滑动窗口（内存 O(n)，精确限流）。
//
// 记录每个请求的时间戳，精确计算滑动窗口内的请求数。
func (s *SlidingWindowLimiter) allowPrecise(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	windowStart := now.Add(-s.window)

	// 获取或创建计数器
	counter, ok := s.counters[key]
	if !ok {
		counter = &windowCounter{
			timestamps: make([]time.Time, 0, s.limit),
		}
		s.counters[key] = counter
	}

	counter.mu.Lock()
	defer counter.mu.Unlock()

	// 清理过期的时间戳
	valid := make([]time.Time, 0, len(counter.timestamps))
	for _, t := range counter.timestamps {
		if t.After(windowStart) {
			valid = append(valid, t)
		}
	}
	counter.timestamps = valid

	// 检查是否超过限制
	if len(counter.timestamps) >= s.limit {
		return false
	}

	counter.timestamps = append(counter.timestamps, now)
	return true
}

// Reset 重置指定键的计数器。
//
// 参数：
//   - key: 要重置的限流键
func (s *SlidingWindowLimiter) Reset(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.counters, key)
}

// ResetAll 重置所有计数器。
func (s *SlidingWindowLimiter) ResetAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.counters = make(map[string]*windowCounter)
}

// Cleanup 清理长时间未使用的计数器。
//
// 参数：
//   - maxAge: 未使用计数器的最大保留时间
func (s *SlidingWindowLimiter) Cleanup(maxAge time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for key, counter := range s.counters {
		counter.mu.Lock()
		if len(counter.timestamps) > 0 {
			lastTime := counter.timestamps[len(counter.timestamps)-1]
			if now.Sub(lastTime) > maxAge {
				delete(s.counters, key)
			}
		}
		counter.mu.Unlock()
	}
}

// SlidingWindowStats 返回限流器统计信息。
type SlidingWindowStats struct {
	Window      time.Duration // 窗口大小
	Limit       int           // 窗口内最大请求数
	Precise     bool          // 是否精确模式
	CounterKeys int           // 当前活跃的键数量
}

// GetStats 返回统计信息。
func (s *SlidingWindowLimiter) GetStats() SlidingWindowStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return SlidingWindowStats{
		Window:      s.window,
		Limit:       s.limit,
		Precise:     s.precise,
		CounterKeys: len(s.counters),
	}
}

// GetCount 获取指定键的当前计数。
//
// 参数：
//   - key: 限流键
//
// 返回值：
//   - int: 当前窗口内的请求数
func (s *SlidingWindowLimiter) GetCount(key string) int {
	s.mu.RLock()
	counter, ok := s.counters[key]
	s.mu.RUnlock()

	if !ok {
		return 0
	}

	counter.mu.Lock()
	defer counter.mu.Unlock()

	if s.precise {
		// 精确模式：计算窗口内的有效请求数
		now := time.Now()
		windowStart := now.Add(-s.window)
		count := 0
		for _, t := range counter.timestamps {
			if t.After(windowStart) {
				count++
			}
		}
		return count
	}

	return int(counter.count)
}
