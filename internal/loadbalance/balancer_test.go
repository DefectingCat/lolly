// Package loadbalance provides load balancing algorithms for the Lolly HTTP server.
package loadbalance

import (
	"sync"
	"testing"
)

// createHealthyTarget 创建一个带有健康状态的目标（辅助函数）
func createHealthyTarget(url string, healthy bool) *Target {
	t := &Target{URL: url}
	t.Healthy.Store(healthy)
	return t
}

// TestRoundRobin_Select 测试轮询负载均衡选择器。
func TestRoundRobin_Select(t *testing.T) {
	t.Run("多目标轮询", func(t *testing.T) {
		rr := NewRoundRobin()
		targets := []*Target{
			createHealthyTarget("http://backend1:8080", true),
			createHealthyTarget("http://backend2:8080", true),
			createHealthyTarget("http://backend3:8080", true),
		}

		// 验证轮询顺序
		got1 := rr.Select(targets)
		got2 := rr.Select(targets)
		got3 := rr.Select(targets)
		got4 := rr.Select(targets)

		if got1.URL != "http://backend1:8080" {
			t.Errorf("第一次选择 = %q, want %q", got1.URL, "http://backend1:8080")
		}
		if got2.URL != "http://backend2:8080" {
			t.Errorf("第二次选择 = %q, want %q", got2.URL, "http://backend2:8080")
		}
		if got3.URL != "http://backend3:8080" {
			t.Errorf("第三次选择 = %q, want %q", got3.URL, "http://backend3:8080")
		}
		if got4.URL != "http://backend1:8080" {
			t.Errorf("第四次选择 = %q, want %q", got4.URL, "http://backend1:8080")
		}
	})

	t.Run("单目标", func(t *testing.T) {
		rr := NewRoundRobin()
		targets := []*Target{
			createHealthyTarget("http://backend1:8080", true),
		}

		got := rr.Select(targets)
		if got == nil {
			t.Fatal("Select() = nil, want non-nil")
		}
		if got.URL != "http://backend1:8080" {
			t.Errorf("Select() = %q, want %q", got.URL, "http://backend1:8080")
		}
	})

	t.Run("空目标", func(t *testing.T) {
		rr := NewRoundRobin()
		got := rr.Select([]*Target{})
		if got != nil {
			t.Errorf("Select() = %v, want nil", got)
		}
	})

	t.Run("跳过不健康目标", func(t *testing.T) {
		rr := NewRoundRobin()
		targets := []*Target{
			createHealthyTarget("http://backend1:8080", false),
			createHealthyTarget("http://backend2:8080", true),
			createHealthyTarget("http://backend3:8080", false),
		}

		got := rr.Select(targets)
		if got == nil {
			t.Fatal("Select() = nil, want non-nil")
		}
		if got.URL != "http://backend2:8080" {
			t.Errorf("Select() = %q, want %q", got.URL, "http://backend2:8080")
		}
	})

	t.Run("所有目标都不健康", func(t *testing.T) {
		rr := NewRoundRobin()
		targets := []*Target{
			createHealthyTarget("http://backend1:8080", false),
			createHealthyTarget("http://backend2:8080", false),
		}

		got := rr.Select(targets)
		if got != nil {
			t.Errorf("Select() = %v, want nil", got)
		}
	})

	t.Run("并发安全", func(t *testing.T) {
		rr := NewRoundRobin()
		targets := []*Target{
			createHealthyTarget("http://backend1:8080", true),
			createHealthyTarget("http://backend2:8080", true),
		}

		var wg sync.WaitGroup
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_ = rr.Select(targets)
			}()
		}
		wg.Wait()
	})
}

// TestWeightedRoundRobin_Select 测试加权轮询负载均衡选择器。
func TestWeightedRoundRobin_Select(t *testing.T) {
	t.Run("权重分配", func(t *testing.T) {
		wrr := NewWeightedRoundRobin()
		targets := []*Target{
			{URL: "http://backend1:8080", Weight: 1},
			{URL: "http://backend2:8080", Weight: 3},
		}
		targets[0].Healthy.Store(true)
		targets[1].Healthy.Store(true)

		// 统计选择次数
		counts := make(map[string]int)
		for i := 0; i < 400; i++ {
			got := wrr.Select(targets)
			if got == nil {
				t.Fatal("Select() = nil, want non-nil")
			}
			counts[got.URL]++
		}

		// 权重1:3，期望比例大约为1:3
		// 允许一定误差
		ratio := float64(counts["http://backend2:8080"]) / float64(counts["http://backend1:8080"])
		if ratio < 2.0 || ratio > 4.0 {
			t.Errorf("权重比例 = %f, 期望接近 3.0", ratio)
		}
	})

	t.Run("权重为0", func(t *testing.T) {
		wrr := NewWeightedRoundRobin()
		targets := []*Target{
			{URL: "http://backend1:8080", Weight: 0},
			{URL: "http://backend2:8080", Weight: 1},
		}
		targets[0].Healthy.Store(true)
		targets[1].Healthy.Store(true)

		// 权重为0的目标应该被当作权重为1处理
		counts := make(map[string]int)
		for i := 0; i < 100; i++ {
			got := wrr.Select(targets)
			if got == nil {
				t.Fatal("Select() = nil, want non-nil")
			}
			counts[got.URL]++
		}

		// 两个目标都应该被选中
		if counts["http://backend1:8080"] == 0 {
			t.Error("权重为0的目标从未被选中")
		}
		if counts["http://backend2:8080"] == 0 {
			t.Error("权重为1的目标从未被选中")
		}
	})

	t.Run("空目标", func(t *testing.T) {
		wrr := NewWeightedRoundRobin()
		got := wrr.Select([]*Target{})
		if got != nil {
			t.Errorf("Select() = %v, want nil", got)
		}
	})

	t.Run("所有目标权重为0或不健康", func(t *testing.T) {
		wrr := NewWeightedRoundRobin()
		targets := []*Target{
			{URL: "http://backend1:8080", Weight: 0},
			{URL: "http://backend2:8080", Weight: 0},
		}
		targets[0].Healthy.Store(false)
		targets[1].Healthy.Store(false)

		got := wrr.Select(targets)
		if got != nil {
			t.Errorf("Select() = %v, want nil", got)
		}
	})

	t.Run("跳过不健康目标", func(t *testing.T) {
		wrr := NewWeightedRoundRobin()
		targets := []*Target{
			{URL: "http://backend1:8080", Weight: 5},
			{URL: "http://backend2:8080", Weight: 1},
		}
		targets[0].Healthy.Store(false)
		targets[1].Healthy.Store(true)

		// 所有选择都应该落在健康目标上
		for i := 0; i < 50; i++ {
			got := wrr.Select(targets)
			if got == nil {
				t.Fatal("Select() = nil, want non-nil")
			}
			if got.URL != "http://backend2:8080" {
				t.Errorf("Select() = %q, want %q", got.URL, "http://backend2:8080")
			}
		}
	})
}

// TestLeastConnections_Select 测试最少连接负载均衡选择器。
func TestLeastConnections_Select(t *testing.T) {
	t.Run("选择最少连接", func(t *testing.T) {
		lc := NewLeastConnections()
		target1 := &Target{URL: "http://backend1:8080", Connections: 10}
		target1.Healthy.Store(true)
		target2 := &Target{URL: "http://backend2:8080", Connections: 5}
		target2.Healthy.Store(true)
		target3 := &Target{URL: "http://backend3:8080", Connections: 15}
		target3.Healthy.Store(true)
		targets := []*Target{target1, target2, target3}

		got := lc.Select(targets)
		if got == nil {
			t.Fatal("Select() = nil, want non-nil")
		}
		if got.URL != "http://backend2:8080" {
			t.Errorf("Select() = %q, want %q", got.URL, "http://backend2:8080")
		}
	})

	t.Run("连接数相等时选择第一个", func(t *testing.T) {
		lc := NewLeastConnections()
		targets := []*Target{
			{URL: "http://backend1:8080", Connections: 5},
			{URL: "http://backend2:8080", Connections: 5},
		}
		targets[0].Healthy.Store(true)
		targets[1].Healthy.Store(true)

		got := lc.Select(targets)
		if got == nil {
			t.Fatal("Select() = nil, want non-nil")
		}
		if got.URL != "http://backend1:8080" {
			t.Errorf("Select() = %q, want %q", got.URL, "http://backend1:8080")
		}
	})

	t.Run("空目标", func(t *testing.T) {
		lc := NewLeastConnections()
		got := lc.Select([]*Target{})
		if got != nil {
			t.Errorf("Select() = %v, want nil", got)
		}
	})

	t.Run("跳过不健康目标", func(t *testing.T) {
		lc := NewLeastConnections()
		targets := []*Target{
			{URL: "http://backend1:8080", Connections: 1},
			{URL: "http://backend2:8080", Connections: 10},
		}
		targets[0].Healthy.Store(false)
		targets[1].Healthy.Store(true)

		got := lc.Select(targets)
		if got == nil {
			t.Fatal("Select() = nil, want non-nil")
		}
		if got.URL != "http://backend2:8080" {
			t.Errorf("Select() = %q, want %q", got.URL, "http://backend2:8080")
		}
	})

	t.Run("所有目标都不健康", func(t *testing.T) {
		lc := NewLeastConnections()
		targets := []*Target{
			{URL: "http://backend1:8080", Connections: 1},
			{URL: "http://backend2:8080", Connections: 2},
		}
		targets[0].Healthy.Store(false)
		targets[1].Healthy.Store(false)

		got := lc.Select(targets)
		if got != nil {
			t.Errorf("Select() = %v, want nil", got)
		}
	})
}

// TestIPHash_Select 测试IP哈希负载均衡选择器。
func TestIPHash_Select(t *testing.T) {
	t.Run("相同IP返回相同目标", func(t *testing.T) {
		ih := NewIPHash()
		targets := []*Target{
			createHealthyTarget("http://backend1:8080", true),
			createHealthyTarget("http://backend2:8080", true),
			createHealthyTarget("http://backend3:8080", true),
		}

		// 使用相同的IP地址多次选择
		clientIP := "192.168.1.100"
		var firstSelection *Target
		for i := 0; i < 10; i++ {
			got := ih.SelectByIP(targets, clientIP)
			if got == nil {
				t.Fatal("SelectByIP() = nil, want non-nil")
			}
			if firstSelection == nil {
				firstSelection = got
			} else if got.URL != firstSelection.URL {
				t.Errorf("相同IP选择不同目标: 第一次=%q, 后续=%q", firstSelection.URL, got.URL)
			}
		}
	})

	t.Run("不同IP分配", func(t *testing.T) {
		ih := NewIPHash()
		targets := []*Target{
			createHealthyTarget("http://backend1:8080", true),
			createHealthyTarget("http://backend2:8080", true),
		}

		// 使用不同的IP地址
		ips := []string{"192.168.1.1", "192.168.1.2", "10.0.0.1", "10.0.0.2"}
		selections := make(map[string]string)
		for _, ip := range ips {
			got := ih.SelectByIP(targets, ip)
			if got == nil {
				t.Fatal("SelectByIP() = nil, want non-nil")
			}
			selections[ip] = got.URL
		}

		// 验证每个IP都有分配（不验证具体分配到哪个）
		for _, ip := range ips {
			if selections[ip] == "" {
				t.Errorf("IP %s 没有分配到目标", ip)
			}
		}
	})

	t.Run("空目标", func(t *testing.T) {
		ih := NewIPHash()
		got := ih.SelectByIP([]*Target{}, "192.168.1.1")
		if got != nil {
			t.Errorf("SelectByIP() = %v, want nil", got)
		}
	})

	t.Run("Select方法使用空IP", func(t *testing.T) {
		ih := NewIPHash()
		targets := []*Target{
			createHealthyTarget("http://backend1:8080", true),
		}

		got := ih.Select(targets)
		if got == nil {
			t.Fatal("Select() = nil, want non-nil")
		}
		if got.URL != "http://backend1:8080" {
			t.Errorf("Select() = %q, want %q", got.URL, "http://backend1:8080")
		}
	})

	t.Run("跳过不健康目标", func(t *testing.T) {
		ih := NewIPHash()
		targets := []*Target{
			createHealthyTarget("http://backend1:8080", false),
			createHealthyTarget("http://backend2:8080", true),
		}

		got := ih.SelectByIP(targets, "192.168.1.1")
		if got == nil {
			t.Fatal("SelectByIP() = nil, want non-nil")
		}
		if got.URL != "http://backend2:8080" {
			t.Errorf("SelectByIP() = %q, want %q", got.URL, "http://backend2:8080")
		}
	})
}

// TestConnectionsAtomic 测试连接数的原子操作。
func TestConnectionsAtomic(t *testing.T) {
	t.Run("IncrementConnections", func(t *testing.T) {
		target := &Target{URL: "http://backend1:8080", Connections: 0}
		target.Healthy.Store(true)

		IncrementConnections(target)
		if target.Connections != 1 {
			t.Errorf("Connections = %d, want 1", target.Connections)
		}

		IncrementConnections(target)
		if target.Connections != 2 {
			t.Errorf("Connections = %d, want 2", target.Connections)
		}
	})

	t.Run("DecrementConnections", func(t *testing.T) {
		target := &Target{URL: "http://backend1:8080", Connections: 5}
		target.Healthy.Store(true)

		DecrementConnections(target)
		if target.Connections != 4 {
			t.Errorf("Connections = %d, want 4", target.Connections)
		}

		DecrementConnections(target)
		if target.Connections != 3 {
			t.Errorf("Connections = %d, want 3", target.Connections)
		}
	})

	t.Run("并发IncrementConnections", func(t *testing.T) {
		target := &Target{URL: "http://backend1:8080", Connections: 0}
		target.Healthy.Store(true)

		var wg sync.WaitGroup
		for i := 0; i < 1000; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				IncrementConnections(target)
			}()
		}
		wg.Wait()

		if target.Connections != 1000 {
			t.Errorf("Connections = %d, want 1000", target.Connections)
		}
	})

	t.Run("并发DecrementConnections", func(t *testing.T) {
		target := &Target{URL: "http://backend1:8080", Connections: 1000}
		target.Healthy.Store(true)

		var wg sync.WaitGroup
		for i := 0; i < 1000; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				DecrementConnections(target)
			}()
		}
		wg.Wait()

		if target.Connections != 0 {
			t.Errorf("Connections = %d, want 0", target.Connections)
		}
	})

	t.Run("混合增减操作", func(t *testing.T) {
		target := &Target{URL: "http://backend1:8080", Connections: 100}
		target.Healthy.Store(true)

		var wg sync.WaitGroup
		// 500个增加
		for i := 0; i < 500; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				IncrementConnections(target)
			}()
		}
		// 300个减少
		for i := 0; i < 300; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				DecrementConnections(target)
			}()
		}
		wg.Wait()

		// 100 + 500 - 300 = 300
		if target.Connections != 300 {
			t.Errorf("Connections = %d, want 300", target.Connections)
		}
	})

	t.Run("允许负值", func(t *testing.T) {
		target := &Target{URL: "http://backend1:8080", Connections: 0}
		target.Healthy.Store(true)

		DecrementConnections(target)
		if target.Connections != -1 {
			t.Errorf("Connections = %d, want -1", target.Connections)
		}
	})
}

// TestHealthStatus 测试健康状态操作。
func TestHealthStatus(t *testing.T) {
	t.Run("IsHealthy", func(t *testing.T) {
		tests := []struct {
			name   string
			target *Target
			want   bool
		}{
			{
				name:   "健康目标",
				target: createHealthyTarget("http://backend1:8080", true),
				want:   true,
			},
			{
				name:   "不健康目标",
				target: createHealthyTarget("http://backend1:8080", false),
				want:   false,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got := tt.target.Healthy.Load()
				if got != tt.want {
					t.Errorf("Healthy.Load() = %v, want %v", got, tt.want)
				}
			})
		}
	})

	t.Run("SetHealthy", func(t *testing.T) {
		target := &Target{URL: "http://backend1:8080"}
		target.Healthy.Store(true)

		// 设置为不健康
		target.Healthy.Store(false)
		if target.Healthy.Load() {
			t.Error("Store(false) 后期望 Load = false, 但 got true")
		}

		// 设置为健康
		target.Healthy.Store(true)
		if !target.Healthy.Load() {
			t.Error("Store(true) 后期望 Load = true, 但 got false")
		}
	})
}

// TestFilterHealthy 测试filterHealthy辅助函数。
func TestFilterHealthy(t *testing.T) {
	t.Run("过滤健康目标", func(t *testing.T) {
		targets := []*Target{
			createHealthyTarget("http://backend1:8080", true),
			createHealthyTarget("http://backend2:8080", false),
			createHealthyTarget("http://backend3:8080", true),
			createHealthyTarget("http://backend4:8080", false),
		}

		got := filterHealthy(targets)
		if len(got) != 2 {
			t.Errorf("len(filterHealthy) = %d, want 2", len(got))
		}

		// 验证返回的都是健康目标
		for _, target := range got {
			if !target.Healthy.Load() {
				t.Errorf("filterHealthy 返回了不健康目标: %q", target.URL)
			}
		}
	})

	t.Run("全部健康", func(t *testing.T) {
		targets := []*Target{
			createHealthyTarget("http://backend1:8080", true),
			createHealthyTarget("http://backend2:8080", true),
		}

		got := filterHealthy(targets)
		if len(got) != 2 {
			t.Errorf("len(filterHealthy) = %d, want 2", len(got))
		}
	})

	t.Run("全部不健康", func(t *testing.T) {
		targets := []*Target{
			createHealthyTarget("http://backend1:8080", false),
			createHealthyTarget("http://backend2:8080", false),
		}

		got := filterHealthy(targets)
		if len(got) != 0 {
			t.Errorf("len(filterHealthy) = %d, want 0", len(got))
		}
	})

	t.Run("空切片", func(t *testing.T) {
		got := filterHealthy([]*Target{})
		if len(got) != 0 {
			t.Errorf("len(filterHealthy) = %d, want 0", len(got))
		}
	})

	t.Run("nil切片", func(t *testing.T) {
		got := filterHealthy(nil)
		if len(got) != 0 {
			t.Errorf("len(filterHealthy) = %d, want 0", len(got))
		}
	})
}

// TestBalancerInterface 测试各种负载均衡器都实现了Balancer接口。
func TestBalancerInterface(t *testing.T) {
	tests := []struct {
		name     string
		balancer Balancer
	}{
		{
			name:     "RoundRobin",
			balancer: NewRoundRobin(),
		},
		{
			name:     "WeightedRoundRobin",
			balancer: NewWeightedRoundRobin(),
		},
		{
			name:     "LeastConnections",
			balancer: NewLeastConnections(),
		},
		{
			name:     "IPHash",
			balancer: NewIPHash(),
		},
	}

	targets := []*Target{
		createHealthyTarget("http://backend1:8080", true),
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.balancer.Select(targets)
			if got == nil {
				t.Fatal("Select() = nil, want non-nil")
			}
			if got.URL != "http://backend1:8080" {
				t.Errorf("Select() = %q, want %q", got.URL, "http://backend1:8080")
			}
		})
	}
}
