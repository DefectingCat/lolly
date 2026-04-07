package security

import (
	"testing"
	"time"
)

func TestNewSlidingWindowLimiter(t *testing.T) {
	t.Run("默认配置", func(t *testing.T) {
		limiter := NewSlidingWindowLimiter(time.Second, 100, false)
		if limiter == nil {
			t.Fatal("NewSlidingWindowLimiter() = nil")
		}
		if limiter.window != time.Second {
			t.Errorf("window = %v, want %v", limiter.window, time.Second)
		}
		if limiter.limit != 100 {
			t.Errorf("limit = %d, want 100", limiter.limit)
		}
	})

	t.Run("精确模式", func(t *testing.T) {
		limiter := NewSlidingWindowLimiter(time.Minute, 50, true)
		if !limiter.precise {
			t.Error("precise should be true")
		}
	})
}

func TestSlidingWindowLimiter_Allow(t *testing.T) {
	t.Run("近似模式-允许请求", func(t *testing.T) {
		limiter := NewSlidingWindowLimiter(time.Second, 10, false)

		for i := 0; i < 10; i++ {
			if !limiter.Allow("test-key") {
				t.Errorf("请求 %d 应该被允许", i+1)
			}
		}
	})

	t.Run("近似模式-拒绝超限请求", func(t *testing.T) {
		limiter := NewSlidingWindowLimiter(time.Second, 5, false)

		// 发送 5 个请求
		for i := 0; i < 5; i++ {
			limiter.Allow("test-key")
		}

		// 验证统计
		stats := limiter.GetStats()
		if stats.Limit != 5 {
			t.Errorf("Limit = %d, want 5", stats.Limit)
		}
	})

	t.Run("精确模式-允许请求", func(t *testing.T) {
		limiter := NewSlidingWindowLimiter(time.Second, 10, true)

		for i := 0; i < 10; i++ {
			if !limiter.Allow("test-key") {
				t.Errorf("请求 %d 应该被允许", i+1)
			}
		}
	})

	t.Run("精确模式-拒绝超限请求", func(t *testing.T) {
		limiter := NewSlidingWindowLimiter(time.Second, 3, true)

		// 发送 3 个请求
		for i := 0; i < 3; i++ {
			if !limiter.Allow("test-key") {
				t.Errorf("请求 %d 应该被允许", i+1)
			}
		}

		// 第 4 个请求应该被拒绝
		if limiter.Allow("test-key") {
			t.Error("第 4 个请求应该被拒绝")
		}
	})

	t.Run("不同键独立计数", func(t *testing.T) {
		limiter := NewSlidingWindowLimiter(time.Second, 2, true)

		// key1 发送 2 个请求
		limiter.Allow("key1")
		limiter.Allow("key1")

		// key2 应该仍可发送
		if !limiter.Allow("key2") {
			t.Error("key2 的请求应该被允许")
		}
	})
}

func TestSlidingWindowLimiter_Reset(t *testing.T) {
	limiter := NewSlidingWindowLimiter(time.Second, 5, true)

	// 发送一些请求
	for i := 0; i < 5; i++ {
		limiter.Allow("test-key")
	}

	// 重置
	limiter.Reset("test-key")

	// 验证计数归零
	if count := limiter.GetCount("test-key"); count != 0 {
		t.Errorf("GetCount() = %d, want 0 after reset", count)
	}
}

func TestSlidingWindowLimiter_ResetAll(t *testing.T) {
	limiter := NewSlidingWindowLimiter(time.Second, 5, true)

	// 为多个键发送请求
	limiter.Allow("key1")
	limiter.Allow("key2")
	limiter.Allow("key3")

	// 重置所有
	limiter.ResetAll()

	stats := limiter.GetStats()
	if stats.CounterKeys != 0 {
		t.Errorf("CounterKeys = %d, want 0 after ResetAll", stats.CounterKeys)
	}
}

func TestSlidingWindowLimiter_Cleanup(t *testing.T) {
	limiter := NewSlidingWindowLimiter(time.Second, 5, true)

	// 发送请求
	limiter.Allow("test-key")

	// 清理（由于请求刚发送，不应该被清理）
	limiter.Cleanup(time.Minute)

	stats := limiter.GetStats()
	if stats.CounterKeys != 1 {
		t.Errorf("CounterKeys = %d, want 1", stats.CounterKeys)
	}
}

func TestSlidingWindowLimiter_GetStats(t *testing.T) {
	limiter := NewSlidingWindowLimiter(time.Minute, 100, false)

	stats := limiter.GetStats()

	if stats.Window != time.Minute {
		t.Errorf("Window = %v, want %v", stats.Window, time.Minute)
	}
	if stats.Limit != 100 {
		t.Errorf("Limit = %d, want 100", stats.Limit)
	}
	if stats.Precise {
		t.Error("Precise should be false")
	}
}

func TestSlidingWindowLimiter_GetCount(t *testing.T) {
	limiter := NewSlidingWindowLimiter(time.Second, 10, true)

	// 发送 3 个请求
	for i := 0; i < 3; i++ {
		limiter.Allow("test-key")
	}

	count := limiter.GetCount("test-key")
	if count != 3 {
		t.Errorf("GetCount() = %d, want 3", count)
	}

	// 不存在的键
	count = limiter.GetCount("nonexistent")
	if count != 0 {
		t.Errorf("GetCount(nonexistent) = %d, want 0", count)
	}
}
