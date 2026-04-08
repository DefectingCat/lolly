// Package security 提供滑动窗口限流器的基准测试。
//
// 该文件测试近似模式和精确模式的滑动窗口限流性能。
//
// 作者：xfy
package security

import (
	"testing"
	"time"
)

// BenchmarkSlidingWindowAllow 测试近似模式滑动窗口 Allow 性能。
func BenchmarkSlidingWindowAllow(b *testing.B) {
	sw := NewSlidingWindowLimiter(time.Second, 10000, false)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sw.Allow("192.168.1.100")
	}
}

// BenchmarkSlidingWindowAllowPrecise 测试精确模式滑动窗口 Allow 性能。
func BenchmarkSlidingWindowAllowPrecise(b *testing.B) {
	sw := NewSlidingWindowLimiter(time.Second, 10000, true)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sw.Allow("192.168.1.100")
	}
}

// BenchmarkSlidingWindowAllowParallel 测试近似模式并发 Allow 性能。
func BenchmarkSlidingWindowAllowParallel(b *testing.B) {
	sw := NewSlidingWindowLimiter(time.Second, 100000, false)

	clients := make([]string, 10)
	for i := range clients {
		clients[i] = "192.168.1." + string(rune('0'+i))
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			sw.Allow(clients[i%10])
			i++
		}
	})
}

// BenchmarkSlidingWindowAllowPreciseParallel 测试精确模式并发 Allow 性能。
func BenchmarkSlidingWindowAllowPreciseParallel(b *testing.B) {
	sw := NewSlidingWindowLimiter(time.Second, 100000, true)

	clients := make([]string, 10)
	for i := range clients {
		clients[i] = "192.168.1." + string(rune('0'+i))
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			sw.Allow(clients[i%10])
			i++
		}
	})
}

// BenchmarkSlidingWindowCleanup 测试滑动窗口清理性能。
func BenchmarkSlidingWindowCleanup(b *testing.B) {
	sw := NewSlidingWindowLimiter(time.Second, 1000, false)

	// 预创建 100 个键
	for i := 0; i < 100; i++ {
		sw.Allow("192.168.0." + string(rune(i)))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sw.Cleanup(time.Hour)
	}
}

// BenchmarkSlidingWindowGetCount 测试获取计数性能。
func BenchmarkSlidingWindowGetCount(b *testing.B) {
	sw := NewSlidingWindowLimiter(time.Second, 10000, false)
	key := "192.168.1.100"

	// 预先添加一些请求
	for i := 0; i < 100; i++ {
		sw.Allow(key)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sw.GetCount(key)
	}
}

// BenchmarkSlidingWindowReset 测试重置性能。
func BenchmarkSlidingWindowReset(b *testing.B) {
	sw := NewSlidingWindowLimiter(time.Second, 10000, false)
	key := "192.168.1.100"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sw.Allow(key)
		sw.Reset(key)
	}
}

// BenchmarkSlidingWindowMultiKey 测试多键场景性能。
func BenchmarkSlidingWindowMultiKey(b *testing.B) {
	sw := NewSlidingWindowLimiter(time.Second, 10000, false)

	keys := make([]string, 100)
	for i := range keys {
		keys[i] = "192.168.0." + string(rune(i))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sw.Allow(keys[i%100])
	}
}

// BenchmarkSlidingWindowStats 测试获取统计信息性能。
func BenchmarkSlidingWindowStats(b *testing.B) {
	sw := NewSlidingWindowLimiter(time.Second, 10000, false)

	// 预创建一些键
	for i := 0; i < 50; i++ {
		sw.Allow("192.168.0." + string(rune(i)))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sw.GetStats()
	}
}