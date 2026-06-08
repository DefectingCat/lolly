package loadbalance

import (
	"sync"
	"testing"
	"time"
)

// TestLeastTime_BasicSelect 测试基本的响应时间选择。
// 两个目标，不同响应时间，应选择更快的目标。
func TestLeastTime_BasicSelect(t *testing.T) {
	t.Parallel()
	t.Run("选择响应时间最短的目标", func(_ *testing.T) {
		lt := NewLeastTime("last_byte", time.Millisecond)
		target1 := createHealthyTarget("http://backend1:8080", true)
		target2 := createHealthyTarget("http://backend2:8080", true)
		targets := []*Target{target1, target2}

		// 记录响应时间：backend1 慢，backend2 快
		lt.RecordResponseTime(target1, 10*time.Millisecond, 100*time.Millisecond)
		lt.RecordResponseTime(target2, 10*time.Millisecond, 10*time.Millisecond)

		got := lt.Select(targets)
		if got == nil {
			t.Fatal("Select() = nil, want non-nil")
		}
		if got.URL != "http://backend2:8080" {
			t.Errorf("Select() = %q, want %q", got.URL, "http://backend2:8080")
		}
	})

	t.Run("空目标", func(_ *testing.T) {
		lt := NewLeastTime("last_byte", time.Millisecond)
		got := lt.Select([]*Target{})
		if got != nil {
			t.Errorf("Select() = %v, want nil", got)
		}
	})

	t.Run("跳过不健康目标", func(_ *testing.T) {
		lt := NewLeastTime("last_byte", time.Millisecond)
		target1 := createHealthyTarget("http://backend1:8080", false)
		target2 := createHealthyTarget("http://backend2:8080", true)
		targets := []*Target{target1, target2}

		lt.RecordResponseTime(target2, 10*time.Millisecond, 100*time.Millisecond)

		got := lt.Select(targets)
		if got == nil {
			t.Fatal("Select() = nil, want non-nil")
		}
		if got.URL != "http://backend2:8080" {
			t.Errorf("Select() = %q, want %q", got.URL, "http://backend2:8080")
		}
	})
}

// TestLeastTime_NoStats 测试无统计信息的目标。
// 目标没有记录过响应时间时，应使用默认值选择。
func TestLeastTime_NoStats(t *testing.T) {
	t.Parallel()
	t.Run("无统计信息时使用默认值", func(_ *testing.T) {
		lt := NewLeastTime("last_byte", time.Millisecond)
		target1 := createHealthyTarget("http://backend1:8080", true)
		target2 := createHealthyTarget("http://backend2:8080", true)
		targets := []*Target{target1, target2}

		// 不记录任何响应时间
		got := lt.Select(targets)
		if got == nil {
			t.Fatal("Select() = nil, want non-nil")
		}
		// 使用默认值时，应返回第一个可用目标
		if got.URL != "http://backend1:8080" {
			t.Errorf("Select() = %q, want %q", got.URL, "http://backend1:8080")
		}
	})

	t.Run("部分目标有统计", func(_ *testing.T) {
		lt := NewLeastTime("last_byte", time.Millisecond)
		target1 := createHealthyTarget("http://backend1:8080", true)
		target2 := createHealthyTarget("http://backend2:8080", true)
		targets := []*Target{target1, target2}

		// 只记录一个目标的响应时间（非常快）
		lt.RecordResponseTime(target1, 1*time.Nanosecond, 1*time.Nanosecond)
		// target2 无统计，使用默认值（1ms = 1,000,000ns）

		got := lt.Select(targets)
		if got == nil {
			t.Fatal("Select() = nil, want non-nil")
		}
		// target1 的 EWMA 应该远小于默认值，所以选择 target1
		if got.URL != "http://backend1:8080" {
			t.Errorf("Select() = %q, want %q", got.URL, "http://backend1:8080")
		}
	})
}

// TestLeastTime_HeaderMetric 测试 header 指标选择。
// 使用 "header" 指标时，应基于 header_time 而非 last_byte_time。
func TestLeastTime_HeaderMetric(t *testing.T) {
	t.Parallel()
	t.Run("header指标选择", func(_ *testing.T) {
		lt := NewLeastTime("header", time.Millisecond)
		target1 := createHealthyTarget("http://backend1:8080", true)
		target2 := createHealthyTarget("http://backend2:8080", true)
		targets := []*Target{target1, target2}

		// target1: header快但last_byte慢
		lt.RecordResponseTime(target1, 10*time.Millisecond, 100*time.Millisecond)
		// target2: header慢但last_byte快
		lt.RecordResponseTime(target2, 100*time.Millisecond, 10*time.Millisecond)

		// 使用 header 指标，应该选择 header_time 更小的 target1
		got := lt.Select(targets)
		if got == nil {
			t.Fatal("Select() = nil, want non-nil")
		}
		if got.URL != "http://backend1:8080" {
			t.Errorf("Select() = %q, want %q", got.URL, "http://backend1:8080")
		}
	})

	t.Run("last_byte指标选择", func(_ *testing.T) {
		lt := NewLeastTime("last_byte", time.Millisecond)
		target1 := createHealthyTarget("http://backend1:8080", true)
		target2 := createHealthyTarget("http://backend2:8080", true)
		targets := []*Target{target1, target2}

		// target1: header快但last_byte慢
		lt.RecordResponseTime(target1, 10*time.Millisecond, 100*time.Millisecond)
		// target2: header慢但last_byte快
		lt.RecordResponseTime(target2, 100*time.Millisecond, 10*time.Millisecond)

		// 使用 last_byte 指标，应该选择 last_byte_time 更小的 target2
		got := lt.Select(targets)
		if got == nil {
			t.Fatal("Select() = nil, want non-nil")
		}
		if got.URL != "http://backend2:8080" {
			t.Errorf("Select() = %q, want %q", got.URL, "http://backend2:8080")
		}
	})
}

// TestLeastTime_SelectExcluding 测试排除选择。
// 排除最快的目标后，应选择次快的目标。
func TestLeastTime_SelectExcluding(t *testing.T) {
	t.Parallel()
	t.Run("排除最快目标", func(_ *testing.T) {
		lt := NewLeastTime("last_byte", time.Millisecond)
		target1 := createHealthyTarget("http://backend1:8080", true)
		target2 := createHealthyTarget("http://backend2:8080", true)
		target3 := createHealthyTarget("http://backend3:8080", true)
		targets := []*Target{target1, target2, target3}

		// target1 最快
		lt.RecordResponseTime(target1, 10*time.Millisecond, 10*time.Millisecond)
		// target2 中等
		lt.RecordResponseTime(target2, 10*time.Millisecond, 50*time.Millisecond)
		// target3 最慢
		lt.RecordResponseTime(target3, 10*time.Millisecond, 100*time.Millisecond)

		// 排除最快的 target1
		excluded := []*Target{target1}
		got := lt.SelectExcluding(targets, excluded)
		if got == nil {
			t.Fatal("SelectExcluding() = nil, want non-nil")
		}
		// 应该选择次快的 target2
		if got.URL != "http://backend2:8080" {
			t.Errorf("SelectExcluding() = %q, want %q", got.URL, "http://backend2:8080")
		}
	})

	t.Run("排除所有目标", func(_ *testing.T) {
		lt := NewLeastTime("last_byte", time.Millisecond)
		target1 := createHealthyTarget("http://backend1:8080", true)
		target2 := createHealthyTarget("http://backend2:8080", true)
		targets := []*Target{target1, target2}
		excluded := []*Target{target1, target2}

		got := lt.SelectExcluding(targets, excluded)
		if got != nil {
			t.Errorf("SelectExcluding() = %v, want nil", got)
		}
	})

	t.Run("排除列表含nil", func(_ *testing.T) {
		lt := NewLeastTime("last_byte", time.Millisecond)
		target1 := createHealthyTarget("http://backend1:8080", true)
		target2 := createHealthyTarget("http://backend2:8080", true)
		targets := []*Target{target1, target2}

		lt.RecordResponseTime(target1, 10*time.Millisecond, 10*time.Millisecond)
		lt.RecordResponseTime(target2, 10*time.Millisecond, 50*time.Millisecond)

		excluded := []*Target{nil, target1}
		got := lt.SelectExcluding(targets, excluded)
		if got == nil {
			t.Fatal("SelectExcluding() = nil, want non-nil")
		}
		if got.URL == target1.URL {
			t.Errorf("选中了被排除的目标: %q", got.URL)
		}
	})
}

// TestLeastTime_Concurrent 测试并发安全。
// 50 goroutines 记录响应时间，50 goroutines 选择目标。
func TestLeastTime_Concurrent(t *testing.T) {
	t.Parallel()
	lt := NewLeastTime("last_byte", time.Millisecond)
	targets := []*Target{
		createHealthyTarget("http://backend1:8080", true),
		createHealthyTarget("http://backend2:8080", true),
		createHealthyTarget("http://backend3:8080", true),
	}

	var wg sync.WaitGroup

	// 50 goroutines 记录响应时间
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			target := targets[idx%len(targets)]
			headerTime := time.Duration(10+idx) * time.Millisecond
			lastByteTime := time.Duration(50+idx) * time.Millisecond
			lt.RecordResponseTime(target, headerTime, lastByteTime)
		}(i)
	}

	// 50 goroutines 选择目标
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got := lt.Select(targets)
			if got == nil {
				t.Error("并发Select() = nil, want non-nil")
			}
		}()
	}

	wg.Wait()
}

// TestLeastTime_GetMetric 测试 GetMetric 方法。
func TestLeastTime_GetMetric(t *testing.T) {
	t.Parallel()
	t.Run("header metric", func(_ *testing.T) {
		lt := NewLeastTime("header", time.Millisecond)
		if lt.GetMetric() != "header" {
			t.Errorf("GetMetric() = %q, want %q", lt.GetMetric(), "header")
		}
	})

	t.Run("last_byte metric", func(_ *testing.T) {
		lt := NewLeastTime("last_byte", time.Millisecond)
		if lt.GetMetric() != "last_byte" {
			t.Errorf("GetMetric() = %q, want %q", lt.GetMetric(), "last_byte")
		}
	})

	t.Run("默认metric", func(_ *testing.T) {
		lt := NewLeastTime("", time.Millisecond)
		if lt.GetMetric() != "last_byte" {
			t.Errorf("GetMetric() = %q, want %q", lt.GetMetric(), "last_byte")
		}
	})
}

// TestLeastTime_BalancerInterface 验证 LeastTime 实现了 Balancer 接口。
func TestLeastTime_BalancerInterface(t *testing.T) {
	t.Parallel()
	var _ Balancer = (*LeastTime)(nil)
	var _ ResponseTimeRecorder = (*LeastTime)(nil)
}

// TestLeastTime_RecordResponseTimeNil 测试 RecordResponseTime 的 nil 处理。
func TestLeastTime_RecordResponseTimeNil(t *testing.T) {
	t.Parallel()
	lt := NewLeastTime("last_byte", time.Millisecond)

	// nil target 不应 panic
	lt.RecordResponseTime(nil, 10*time.Millisecond, 100*time.Millisecond)

	// nil Stats 不应 panic
	target := &Target{URL: "http://backend1:8080"}
	lt.RecordResponseTime(target, 10*time.Millisecond, 100*time.Millisecond)
}

// TestLeastTime_DefaultTimeValidation 测试默认值参数验证。
func TestLeastTime_DefaultTimeValidation(t *testing.T) {
	t.Parallel()
	t.Run("零值默认值", func(_ *testing.T) {
		lt := NewLeastTime("last_byte", 0)
		if lt.defaultTime != time.Millisecond {
			t.Errorf("defaultTime = %v, want %v", lt.defaultTime, time.Millisecond)
		}
	})

	t.Run("负值默认值", func(_ *testing.T) {
		lt := NewLeastTime("last_byte", -1*time.Millisecond)
		if lt.defaultTime != time.Millisecond {
			t.Errorf("defaultTime = %v, want %v", lt.defaultTime, time.Millisecond)
		}
	})
}
