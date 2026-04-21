// Package loadbalance 提供负载均衡算法的测试。
//
// 该文件测试负载均衡模块的各项功能，包括：
//   - 轮询算法
//   - 加权轮询算法
//   - 最少连接算法
//   - IP 哈希算法
//   - 一致性哈希算法
//
// 作者：xfy
package loadbalance

import (
	"net"
	"sync"
	"testing"
	"time"
)

// createHealthyTarget 创建并返回一个指定 URL 和健康状态的 Target 实例。
//
// 该辅助函数简化测试中 Target 的创建过程，自动初始化 Healthy 状态。
//
// 参数：
//   - url: 目标 URL 地址
//   - healthy: 健康状态，true 表示健康，false 表示不健康
//
// 返回值：
//   - 初始化完成的 Target 指针
func createHealthyTarget(url string, healthy bool) *Target {
	t := &Target{URL: url}
	t.Healthy.Store(healthy)
	return t
}

// TestRoundRobin_Select 测试轮询负载均衡选择器。
func TestRoundRobin_Select(t *testing.T) {
	t.Run("多目标轮询", func(_ *testing.T) {
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

	t.Run("单目标", func(_ *testing.T) {
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

	t.Run("空目标", func(_ *testing.T) {
		rr := NewRoundRobin()
		got := rr.Select([]*Target{})
		if got != nil {
			t.Errorf("Select() = %v, want nil", got)
		}
	})

	t.Run("跳过不健康目标", func(_ *testing.T) {
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

	t.Run("所有目标都不健康", func(_ *testing.T) {
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

	t.Run("并发安全", func(_ *testing.T) {
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
	t.Run("权重分配", func(_ *testing.T) {
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

	t.Run("权重为0", func(_ *testing.T) {
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

	t.Run("空目标", func(_ *testing.T) {
		wrr := NewWeightedRoundRobin()
		got := wrr.Select([]*Target{})
		if got != nil {
			t.Errorf("Select() = %v, want nil", got)
		}
	})

	t.Run("所有目标权重为0或不健康", func(_ *testing.T) {
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

	t.Run("跳过不健康目标", func(_ *testing.T) {
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
	t.Run("选择最少连接", func(_ *testing.T) {
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

	t.Run("连接数相等时选择第一个", func(_ *testing.T) {
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

	t.Run("空目标", func(_ *testing.T) {
		lc := NewLeastConnections()
		got := lc.Select([]*Target{})
		if got != nil {
			t.Errorf("Select() = %v, want nil", got)
		}
	})

	t.Run("跳过不健康目标", func(_ *testing.T) {
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

	t.Run("所有目标都不健康", func(_ *testing.T) {
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
	t.Run("相同IP返回相同目标", func(_ *testing.T) {
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

	t.Run("不同IP分配", func(_ *testing.T) {
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

	t.Run("空目标", func(_ *testing.T) {
		ih := NewIPHash()
		got := ih.SelectByIP([]*Target{}, "192.168.1.1")
		if got != nil {
			t.Errorf("SelectByIP() = %v, want nil", got)
		}
	})

	t.Run("Select方法使用空IP", func(_ *testing.T) {
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

	t.Run("跳过不健康目标", func(_ *testing.T) {
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
	t.Run("IncrementConnections", func(_ *testing.T) {
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

	t.Run("DecrementConnections", func(_ *testing.T) {
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

	t.Run("并发IncrementConnections", func(_ *testing.T) {
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

	t.Run("并发DecrementConnections", func(_ *testing.T) {
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

	t.Run("混合增减操作", func(_ *testing.T) {
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

	t.Run("允许负值", func(_ *testing.T) {
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
	t.Run("IsHealthy", func(_ *testing.T) {
		tests := []struct {
			target *Target
			name   string
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
			t.Run(tt.name, func(_ *testing.T) {
				got := tt.target.Healthy.Load()
				if got != tt.want {
					t.Errorf("Healthy.Load() = %v, want %v", got, tt.want)
				}
			})
		}
	})

	t.Run("SetHealthy", func(_ *testing.T) {
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
	t.Run("过滤健康目标", func(_ *testing.T) {
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

	t.Run("全部健康", func(_ *testing.T) {
		targets := []*Target{
			createHealthyTarget("http://backend1:8080", true),
			createHealthyTarget("http://backend2:8080", true),
		}

		got := filterHealthy(targets)
		if len(got) != 2 {
			t.Errorf("len(filterHealthy) = %d, want 2", len(got))
		}
	})

	t.Run("全部不健康", func(_ *testing.T) {
		targets := []*Target{
			createHealthyTarget("http://backend1:8080", false),
			createHealthyTarget("http://backend2:8080", false),
		}

		got := filterHealthy(targets)
		if len(got) != 0 {
			t.Errorf("len(filterHealthy) = %d, want 0", len(got))
		}
	})

	t.Run("空切片", func(_ *testing.T) {
		got := filterHealthy([]*Target{})
		if len(got) != 0 {
			t.Errorf("len(filterHealthy) = %d, want 0", len(got))
		}
	})

	t.Run("nil切片", func(_ *testing.T) {
		got := filterHealthy(nil)
		if len(got) != 0 {
			t.Errorf("len(filterHealthy) = %d, want 0", len(got))
		}
	})
}

// TestBalancerInterface 测试各种负载均衡器都实现了Balancer接口。
func TestBalancerInterface(t *testing.T) {
	tests := []struct {
		balancer Balancer
		name     string
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
		t.Run(tt.name, func(_ *testing.T) {
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

// TestConsistentHash 测试一致性哈希负载均衡器。
func TestConsistentHash(t *testing.T) {
	t.Run("创建默认配置", func(_ *testing.T) {
		ch := NewConsistentHash(0, "ip")
		if ch == nil {
			t.Fatal("NewConsistentHash() = nil")
		}
		if ch.GetVirtualNodes() != 150 {
			t.Errorf("GetVirtualNodes() = %d, want 150", ch.GetVirtualNodes())
		}
		if ch.GetHashKey() != "ip" {
			t.Errorf("GetHashKey() = %q, want %q", ch.GetHashKey(), "ip")
		}
	})

	t.Run("自定义虚拟节点数", func(_ *testing.T) {
		ch := NewConsistentHash(200, "uri")
		if ch.GetVirtualNodes() != 200 {
			t.Errorf("GetVirtualNodes() = %d, want 200", ch.GetVirtualNodes())
		}
	})

	t.Run("SelectByKey 空目标", func(_ *testing.T) {
		ch := NewConsistentHash(150, "ip")
		got := ch.SelectByKey([]*Target{}, "192.168.1.1")
		if got != nil {
			t.Errorf("SelectByKey() = %v, want nil", got)
		}
	})

	t.Run("SelectByKey 单目标", func(_ *testing.T) {
		ch := NewConsistentHash(150, "ip")
		targets := []*Target{
			createHealthyTarget("http://backend1:8080", true),
		}

		got := ch.SelectByKey(targets, "192.168.1.1")
		if got == nil {
			t.Fatal("SelectByKey() = nil")
		}
		if got.URL != "http://backend1:8080" {
			t.Errorf("SelectByKey() = %q, want %q", got.URL, "http://backend1:8080")
		}
	})

	t.Run("SelectByKey 多目标相同键", func(_ *testing.T) {
		ch := NewConsistentHash(150, "ip")
		targets := []*Target{
			createHealthyTarget("http://backend1:8080", true),
			createHealthyTarget("http://backend2:8080", true),
			createHealthyTarget("http://backend3:8080", true),
		}

		// 相同的键应该选择相同的目标
		key := "192.168.1.100"
		first := ch.SelectByKey(targets, key)
		for i := 0; i < 10; i++ {
			got := ch.SelectByKey(targets, key)
			if got == nil {
				t.Fatal("SelectByKey() = nil")
			}
			if got.URL != first.URL {
				t.Errorf("相同键选择不同目标: first=%q, got=%q", first.URL, got.URL)
			}
		}
	})

	t.Run("GetStats", func(_ *testing.T) {
		ch := NewConsistentHash(100, "ip")
		targets := []*Target{
			createHealthyTarget("http://backend1:8080", true),
			createHealthyTarget("http://backend2:8080", true),
		}

		ch.Rebuild(targets)
		stats := ch.GetStats()

		if stats.VirtualNodes != 100 {
			t.Errorf("VirtualNodes = %d, want 100", stats.VirtualNodes)
		}
		if stats.CircleSize != 200 { // 2 targets * 100 nodes
			t.Errorf("CircleSize = %d, want 200", stats.CircleSize)
		}
	})

	t.Run("Rebuild 跳过不健康目标", func(_ *testing.T) {
		ch := NewConsistentHash(10, "ip")
		targets := []*Target{
			createHealthyTarget("http://backend1:8080", false),
			createHealthyTarget("http://backend2:8080", true),
		}

		ch.Rebuild(targets)
		stats := ch.GetStats()

		if stats.CircleSize != 10 { // 只有1个健康目标 * 10 nodes
			t.Errorf("CircleSize = %d, want 10", stats.CircleSize)
		}
	})
}

// TestConsistentHashSelectExcludingByKey 测试一致性哈希排除选择功能。
func TestConsistentHashSelectExcludingByKey(t *testing.T) {
	t.Run("空排除列表", func(_ *testing.T) {
		ch := NewConsistentHash(150, "ip")
		targets := []*Target{
			createHealthyTarget("http://backend1:8080", true),
			createHealthyTarget("http://backend2:8080", true),
			createHealthyTarget("http://backend3:8080", true),
		}
		ch.Rebuild(targets)

		key := "192.168.1.100"
		got := ch.SelectExcludingByKey(targets, []*Target{}, key)

		if got == nil {
			t.Fatal("SelectExcludingByKey() = nil, want non-nil")
		}

		// 验证正常选择行为
		got2 := ch.SelectExcludingByKey(targets, nil, key)
		if got2 == nil {
			t.Fatal("SelectExcludingByKey() with nil = nil, want non-nil")
		}
		if got.URL != got2.URL {
			t.Errorf("空排除和nil排除应该返回相同结果: empty=%q, nil=%q", got.URL, got2.URL)
		}
	})

	t.Run("部分排除", func(_ *testing.T) {
		ch := NewConsistentHash(150, "ip")
		targets := []*Target{
			createHealthyTarget("http://backend1:8080", true),
			createHealthyTarget("http://backend2:8080", true),
			createHealthyTarget("http://backend3:8080", true),
		}
		ch.Rebuild(targets)

		// 排除第一个目标
		excluded := []*Target{targets[0]}
		key := "192.168.1.100"

		// 多次选择，验证不会选中排除的目标
		for i := 0; i < 100; i++ {
			got := ch.SelectExcludingByKey(targets, excluded, key)
			if got == nil {
				t.Fatal("SelectExcludingByKey() = nil, want non-nil")
			}
			if got.URL == targets[0].URL {
				t.Errorf("选中了被排除的目标: %q", got.URL)
			}
		}
	})

	t.Run("全部排除", func(_ *testing.T) {
		ch := NewConsistentHash(150, "ip")
		targets := []*Target{
			createHealthyTarget("http://backend1:8080", true),
			createHealthyTarget("http://backend2:8080", true),
		}
		ch.Rebuild(targets)

		// 排除所有目标
		excluded := []*Target{targets[0], targets[1]}
		key := "192.168.1.100"

		got := ch.SelectExcludingByKey(targets, excluded, key)
		if got != nil {
			t.Errorf("SelectExcludingByKey() = %q, want nil (all excluded)", got.URL)
		}
	})

	t.Run("排除包含nil目标", func(_ *testing.T) {
		ch := NewConsistentHash(150, "ip")
		targets := []*Target{
			createHealthyTarget("http://backend1:8080", true),
			createHealthyTarget("http://backend2:8080", true),
		}
		ch.Rebuild(targets)

		// 排除列表中包含nil
		excluded := []*Target{nil, targets[0]}
		key := "192.168.1.100"

		got := ch.SelectExcludingByKey(targets, excluded, key)
		if got == nil {
			t.Fatal("SelectExcludingByKey() = nil, want non-nil")
		}
		if got.URL == targets[0].URL {
			t.Errorf("选中了被排除的目标: %q", got.URL)
		}
	})

	t.Run("并发安全", func(_ *testing.T) {
		ch := NewConsistentHash(150, "ip")
		targets := []*Target{
			createHealthyTarget("http://backend1:8080", true),
			createHealthyTarget("http://backend2:8080", true),
			createHealthyTarget("http://backend3:8080", true),
		}
		ch.Rebuild(targets)

		excluded := []*Target{targets[0]}
		key := "192.168.1.100"

		var wg sync.WaitGroup
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < 100; j++ {
					got := ch.SelectExcludingByKey(targets, excluded, key)
					if got != nil && got.URL == targets[0].URL {
						t.Errorf("并发时选中了被排除的目标: %q", got.URL)
					}
				}
			}()
		}
		wg.Wait()
	})

	t.Run("相同键一致性", func(_ *testing.T) {
		ch := NewConsistentHash(150, "ip")
		targets := []*Target{
			createHealthyTarget("http://backend1:8080", true),
			createHealthyTarget("http://backend2:8080", true),
			createHealthyTarget("http://backend3:8080", true),
		}
		ch.Rebuild(targets)

		excluded := []*Target{targets[0]}
		key := "192.168.1.100"

		// 相同键应该始终返回相同的目标
		var firstSelection *Target
		for i := 0; i < 50; i++ {
			got := ch.SelectExcludingByKey(targets, excluded, key)
			if got == nil {
				t.Fatal("SelectExcludingByKey() = nil, want non-nil")
			}
			if firstSelection == nil {
				firstSelection = got
			} else if got.URL != firstSelection.URL {
				t.Errorf("相同键选择不同目标: first=%q, got=%q", firstSelection.URL, got.URL)
			}
		}
	})
}

func TestIsValidAlgorithm(t *testing.T) {
	tests := []struct {
		name      string
		algorithm string
		want      bool
	}{
		{"round_robin", "round_robin", true},
		{"weighted_round_robin", "weighted_round_robin", true},
		{"least_conn", "least_conn", true},
		{"ip_hash", "ip_hash", true},
		{"consistent_hash", "consistent_hash", true},
		{"invalid", "invalid", false},
		{"empty", "", true}, // 空字符串有效（使用默认值）
		{"unknown", "unknown-algorithm", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(_ *testing.T) {
			got := IsValidAlgorithm(tt.algorithm)
			if got != tt.want {
				t.Errorf("IsValidAlgorithm(%q) = %v, want %v", tt.algorithm, got, tt.want)
			}
		})
	}
}

// TestRoundRobin_SelectExcluding 测试轮询排除选择功能。
func TestRoundRobin_SelectExcluding(t *testing.T) {
	t.Run("空排除列表", func(_ *testing.T) {
		rr := NewRoundRobin()
		targets := []*Target{
			createHealthyTarget("http://backend1:8080", true),
			createHealthyTarget("http://backend2:8080", true),
		}

		got := rr.SelectExcluding(targets, nil)
		if got == nil {
			t.Fatal("SelectExcluding() = nil, want non-nil")
		}
	})

	t.Run("排除部分目标", func(_ *testing.T) {
		rr := NewRoundRobin()
		targets := []*Target{
			createHealthyTarget("http://backend1:8080", true),
			createHealthyTarget("http://backend2:8080", true),
			createHealthyTarget("http://backend3:8080", true),
		}
		excluded := []*Target{targets[0], targets[1]}

		got := rr.SelectExcluding(targets, excluded)
		if got == nil {
			t.Fatal("SelectExcluding() = nil, want non-nil")
		}
		if got.URL != "http://backend3:8080" {
			t.Errorf("SelectExcluding() = %q, want %q", got.URL, "http://backend3:8080")
		}
	})

	t.Run("排除所有目标", func(_ *testing.T) {
		rr := NewRoundRobin()
		targets := []*Target{
			createHealthyTarget("http://backend1:8080", true),
			createHealthyTarget("http://backend2:8080", true),
		}
		excluded := []*Target{targets[0], targets[1]}

		got := rr.SelectExcluding(targets, excluded)
		if got != nil {
			t.Errorf("SelectExcluding() = %v, want nil", got)
		}
	})

	t.Run("排除列表含nil", func(_ *testing.T) {
		rr := NewRoundRobin()
		targets := []*Target{
			createHealthyTarget("http://backend1:8080", true),
			createHealthyTarget("http://backend2:8080", true),
		}
		excluded := []*Target{nil, targets[0]}

		got := rr.SelectExcluding(targets, excluded)
		if got == nil {
			t.Fatal("SelectExcluding() = nil, want non-nil")
		}
		if got.URL == targets[0].URL {
			t.Errorf("选中了被排除的目标: %q", got.URL)
		}
	})

	t.Run("并发安全", func(_ *testing.T) {
		rr := NewRoundRobin()
		targets := []*Target{
			createHealthyTarget("http://backend1:8080", true),
			createHealthyTarget("http://backend2:8080", true),
			createHealthyTarget("http://backend3:8080", true),
		}
		excluded := []*Target{targets[0]}

		var wg sync.WaitGroup
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				got := rr.SelectExcluding(targets, excluded)
				if got != nil && got.URL == targets[0].URL {
					t.Errorf("选中了被排除的目标: %q", got.URL)
				}
			}()
		}
		wg.Wait()
	})

	t.Run("排除不健康目标外再排除一个", func(_ *testing.T) {
		rr := NewRoundRobin()
		targets := []*Target{
			createHealthyTarget("http://backend1:8080", false),
			createHealthyTarget("http://backend2:8080", true),
			createHealthyTarget("http://backend3:8080", true),
		}
		excluded := []*Target{targets[1]}

		got := rr.SelectExcluding(targets, excluded)
		if got == nil {
			t.Fatal("SelectExcluding() = nil, want non-nil")
		}
		if got.URL != "http://backend3:8080" {
			t.Errorf("SelectExcluding() = %q, want %q", got.URL, "http://backend3:8080")
		}
	})
}

// TestWeightedRoundRobin_SelectExcluding 测试加权轮询排除选择功能。
func TestWeightedRoundRobin_SelectExcluding(t *testing.T) {
	t.Run("排除高权重目标", func(_ *testing.T) {
		wrr := NewWeightedRoundRobin()
		targets := []*Target{
			createHealthyTarget("http://backend1:8080", true),
			createHealthyTarget("http://backend2:8080", true),
		}
		targets[0].Weight = 1
		targets[1].Weight = 5
		excluded := []*Target{targets[1]}

		// 排除高权重目标后，只应选低权重目标
		for i := 0; i < 20; i++ {
			got := wrr.SelectExcluding(targets, excluded)
			if got == nil {
				t.Fatal("SelectExcluding() = nil, want non-nil")
			}
			if got.URL != "http://backend1:8080" {
				t.Errorf("SelectExcluding() = %q, want %q", got.URL, "http://backend1:8080")
			}
		}
	})

	t.Run("排除所有健康目标", func(_ *testing.T) {
		wrr := NewWeightedRoundRobin()
		targets := []*Target{
			createHealthyTarget("http://backend1:8080", true),
			createHealthyTarget("http://backend2:8080", true),
		}
		excluded := []*Target{targets[0], targets[1]}

		got := wrr.SelectExcluding(targets, excluded)
		if got != nil {
			t.Errorf("SelectExcluding() = %v, want nil", got)
		}
	})

	t.Run("排除列表含nil", func(_ *testing.T) {
		wrr := NewWeightedRoundRobin()
		targets := []*Target{
			createHealthyTarget("http://backend1:8080", true),
			createHealthyTarget("http://backend2:8080", true),
		}
		targets[0].Weight = 1
		targets[1].Weight = 1
		excluded := []*Target{nil, targets[0]}

		got := wrr.SelectExcluding(targets, excluded)
		if got == nil {
			t.Fatal("SelectExcluding() = nil, want non-nil")
		}
		if got.URL == targets[0].URL {
			t.Errorf("选中了被排除的目标: %q", got.URL)
		}
	})
}

// TestLeastConnections_SelectExcluding 测试最少连接排除选择功能。
func TestLeastConnections_SelectExcluding(t *testing.T) {
	t.Run("排除连接最少的目标", func(_ *testing.T) {
		lc := NewLeastConnections()
		targets := []*Target{
			createHealthyTarget("http://backend1:8080", true),
			createHealthyTarget("http://backend2:8080", true),
			createHealthyTarget("http://backend3:8080", true),
		}
		targets[0].Connections = 1
		targets[1].Connections = 10
		targets[2].Connections = 5
		excluded := []*Target{targets[0]}

		got := lc.SelectExcluding(targets, excluded)
		if got == nil {
			t.Fatal("SelectExcluding() = nil, want non-nil")
		}
		// 排除最少连接的后，应选连接数次少的
		if got.URL != "http://backend3:8080" {
			t.Errorf("SelectExcluding() = %q, want %q", got.URL, "http://backend3:8080")
		}
	})

	t.Run("排除列表含nil", func(_ *testing.T) {
		lc := NewLeastConnections()
		targets := []*Target{
			createHealthyTarget("http://backend1:8080", true),
			createHealthyTarget("http://backend2:8080", true),
		}
		targets[0].Connections = 5
		targets[1].Connections = 3
		excluded := []*Target{nil, targets[0]}

		got := lc.SelectExcluding(targets, excluded)
		if got == nil {
			t.Fatal("SelectExcluding() = nil, want non-nil")
		}
		if got.URL == targets[0].URL {
			t.Errorf("选中了被排除的目标: %q", got.URL)
		}
	})

	t.Run("全部排除", func(_ *testing.T) {
		lc := NewLeastConnections()
		targets := []*Target{
			createHealthyTarget("http://backend1:8080", true),
			createHealthyTarget("http://backend2:8080", true),
		}
		excluded := []*Target{targets[0], targets[1]}

		got := lc.SelectExcluding(targets, excluded)
		if got != nil {
			t.Errorf("SelectExcluding() = %v, want nil", got)
		}
	})
}

// TestIPHash_SelectExcluding 测试IP哈希排除选择功能。
func TestIPHash_SelectExcluding(t *testing.T) {
	t.Run("排除命中目标", func(_ *testing.T) {
		ih := NewIPHash()
		targets := []*Target{
			createHealthyTarget("http://backend1:8080", true),
			createHealthyTarget("http://backend2:8080", true),
			createHealthyTarget("http://backend3:8080", true),
		}

		// 找到该IP会命中的目标
		clientIP := "192.168.1.100"
		first := ih.SelectByIP(targets, clientIP)
		if first == nil {
			t.Fatal("SelectByIP() = nil, want non-nil")
		}

		// 排除该目标
		excluded := []*Target{first}
		got := ih.SelectExcludingByIP(targets, excluded, clientIP)
		if got == nil {
			t.Fatal("SelectExcludingByIP() = nil, want non-nil")
		}
		if got.URL == first.URL {
			t.Errorf("SelectExcludingByIP() 选中了被排除的目标: %q", got.URL)
		}
	})

	t.Run("排除所有目标", func(_ *testing.T) {
		ih := NewIPHash()
		targets := []*Target{
			createHealthyTarget("http://backend1:8080", true),
			createHealthyTarget("http://backend2:8080", true),
		}
		excluded := []*Target{targets[0], targets[1]}

		got := ih.SelectExcludingByIP(targets, excluded, "10.0.0.1")
		if got != nil {
			t.Errorf("SelectExcludingByIP() = %v, want nil", got)
		}
	})

	t.Run("SelectExcluding使用空IP", func(_ *testing.T) {
		ih := NewIPHash()
		targets := []*Target{
			createHealthyTarget("http://backend1:8080", true),
			createHealthyTarget("http://backend2:8080", true),
		}
		excluded := []*Target{targets[0]}

		got := ih.SelectExcluding(targets, excluded)
		if got == nil {
			t.Fatal("SelectExcluding() = nil, want non-nil")
		}
	})

	t.Run("排除列表含nil", func(_ *testing.T) {
		ih := NewIPHash()
		targets := []*Target{
			createHealthyTarget("http://backend1:8080", true),
			createHealthyTarget("http://backend2:8080", true),
		}
		excluded := []*Target{nil, targets[0]}

		got := ih.SelectExcludingByIP(targets, excluded, "192.168.1.1")
		if got == nil {
			t.Fatal("SelectExcludingByIP() = nil, want non-nil")
		}
		if got.URL == targets[0].URL {
			t.Errorf("选中了被排除的目标: %q", got.URL)
		}
	})
}

// TestFilterHealthyAndExclude 测试filterHealthyAndExclude辅助函数。
func TestFilterHealthyAndExclude(t *testing.T) {
	t.Run("基本过滤和排除", func(_ *testing.T) {
		targets := []*Target{
			createHealthyTarget("http://backend1:8080", true),
			createHealthyTarget("http://backend2:8080", false),
			createHealthyTarget("http://backend3:8080", true),
		}
		excluded := []*Target{targets[0]}

		got := filterHealthyAndExclude(targets, excluded)
		if len(got) != 1 {
			t.Fatalf("len = %d, want 1", len(got))
		}
		if got[0].URL != "http://backend3:8080" {
			t.Errorf("got = %q, want %q", got[0].URL, "http://backend3:8080")
		}
	})

	t.Run("空排除列表", func(_ *testing.T) {
		targets := []*Target{
			createHealthyTarget("http://backend1:8080", true),
			createHealthyTarget("http://backend2:8080", true),
		}

		got := filterHealthyAndExclude(targets, nil)
		if len(got) != 2 {
			t.Fatalf("len = %d, want 2", len(got))
		}
	})

	t.Run("空目标列表", func(_ *testing.T) {
		got := filterHealthyAndExclude(nil, []*Target{})
		if len(got) != 0 {
			t.Errorf("len = %d, want 0", len(got))
		}
	})

	t.Run("排除列表含nil", func(_ *testing.T) {
		targets := []*Target{
			createHealthyTarget("http://backend1:8080", true),
		}
		excluded := []*Target{nil}

		got := filterHealthyAndExclude(targets, excluded)
		if len(got) != 1 {
			t.Fatalf("len = %d, want 1", len(got))
		}
	})
}

// TestTarget_Hostname 测试Target.Hostname方法。
func TestTarget_Hostname(t *testing.T) {
	t.Run("从URL提取主机名", func(_ *testing.T) {
		target := NewTargetFromConfig("http://example.com:8080/api", 1, 0, 0, 0, false, false, "")
		got := target.Hostname()
		if got != "example.com" {
			t.Errorf("Hostname() = %q, want %q", got, "example.com")
		}
	})

	t.Run("无端口URL", func(_ *testing.T) {
		target := NewTargetFromConfig("http://example.com/api", 1, 0, 0, 0, false, false, "")
		got := target.Hostname()
		if got != "example.com" {
			t.Errorf("Hostname() = %q, want %q", got, "example.com")
		}
	})

	t.Run("无效URL主机名为空", func(_ *testing.T) {
		target := &Target{URL: "not-a-valid-url"}
		target.initHostname()
		got := target.Hostname()
		// url.Parse("not-a-valid-url") 解析为纯路径URL，Host为空
		if got != "" {
			t.Errorf("Hostname() = %q, want empty string", got)
		}
	})

	t.Run("缓存行为", func(_ *testing.T) {
		target := NewTargetFromConfig("http://example.com:8080", 1, 0, 0, 0, false, false, "")
		_ = target.Hostname()
		_ = target.Hostname() // 第二次应使用缓存
		got := target.Hostname()
		if got != "example.com" {
			t.Errorf("Hostname() = %q, want %q", got, "example.com")
		}
	})
}

// TestTarget_ResolvedIPs 测试Target.ResolvedIPs和SetResolvedIPs方法。
func TestTarget_ResolvedIPs(t *testing.T) {
	t.Run("未设置时返回nil", func(_ *testing.T) {
		target := &Target{URL: "http://example.com"}
		got := target.ResolvedIPs()
		if got != nil {
			t.Errorf("ResolvedIPs() = %v, want nil", got)
		}
	})

	t.Run("设置后返回副本", func(_ *testing.T) {
		target := &Target{URL: "http://example.com"}
		ips := []string{"192.168.1.1", "192.168.1.2"}
		target.SetResolvedIPs(ips)

		got := target.ResolvedIPs()
		if len(got) != 2 {
			t.Fatalf("len = %d, want 2", len(got))
		}
		if got[0] != "192.168.1.1" || got[1] != "192.168.1.2" {
			t.Errorf("ResolvedIPs() = %v, want %v", got, ips)
		}

		// 修改原切片不影响内部存储
		ips[0] = "10.0.0.1"
		got2 := target.ResolvedIPs()
		if got2[0] != "192.168.1.1" {
			t.Errorf("ResolvedIPs() 受外部修改影响 = %q, want %q", got2[0], "192.168.1.1")
		}
	})

	t.Run("设置空列表", func(_ *testing.T) {
		target := &Target{URL: "http://example.com"}
		target.SetResolvedIPs([]string{})
		got := target.ResolvedIPs()
		if len(got) != 0 {
			t.Errorf("ResolvedIPs() = %v, want empty", got)
		}
	})
}

// TestTarget_NeedsResolve 测试Target.NeedsResolve方法。
func TestTarget_NeedsResolve(t *testing.T) {
	t.Run("IP地址不需要解析", func(_ *testing.T) {
		target := NewTargetFromConfig("http://192.168.1.1:8080", 1, 0, 0, 0, false, false, "")
		if target.NeedsResolve(time.Minute) {
			t.Error("IP地址URL不需要解析")
		}
	})

	t.Run("首次解析需要", func(_ *testing.T) {
		target := NewTargetFromConfig("http://example.com:8080", 1, 0, 0, 0, false, false, "")
		if !target.NeedsResolve(time.Minute) {
			t.Error("首次解析应该需要")
		}
	})

	t.Run("TTL未过期不需要", func(_ *testing.T) {
		target := NewTargetFromConfig("http://example.com:8080", 1, 0, 0, 0, false, false, "")
		target.SetResolvedIPs([]string{"192.168.1.1"})
		// TTL为1小时，刚设置过，不应过期
		if target.NeedsResolve(time.Hour) {
			t.Error("TTL未过期不应该需要解析")
		}
	})

	t.Run("TTL过期需要", func(_ *testing.T) {
		target := NewTargetFromConfig("http://example.com:8080", 1, 0, 0, 0, false, false, "")
		target.SetResolvedIPs([]string{"192.168.1.1"})
		// 使用极短的TTL模拟过期
		if !target.NeedsResolve(time.Nanosecond) {
			t.Error("TTL过期应该需要解析")
		}
	})
}

// TestTarget_LastResolved 测试Target.LastResolved方法。
func TestTarget_LastResolved(t *testing.T) {
	t.Run("未设置时返回零值", func(_ *testing.T) {
		target := &Target{URL: "http://example.com"}
		got := target.LastResolved()
		if !got.IsZero() {
			t.Errorf("LastResolved() = %v, want zero", got)
		}
	})

	t.Run("设置后返回时间", func(_ *testing.T) {
		target := &Target{URL: "http://example.com"}
		before := time.Now()
		target.SetResolvedIPs([]string{"192.168.1.1"})
		after := time.Now()

		got := target.LastResolved()
		if got.Before(before) || got.After(after) {
			t.Errorf("LastResolved() = %v, 应该在 %v 和 %v 之间", got, before, after)
		}
	})
}

// TestNewTargetFromConfig 测试NewTargetFromConfig函数。
func TestNewTargetFromConfig(t *testing.T) {
	t.Run("创建健康目标", func(_ *testing.T) {
		target := NewTargetFromConfig("http://backend:8080", 5, 0, 0, 0, false, false, "")
		if target.URL != "http://backend:8080" {
			t.Errorf("URL = %q, want %q", target.URL, "http://backend:8080")
		}
		if target.Weight != 5 {
			t.Errorf("Weight = %d, want 5", target.Weight)
		}
		if !target.Healthy.Load() {
			t.Error("Healthy 应为 true")
		}
	})

	t.Run("主机名自动初始化", func(_ *testing.T) {
		target := NewTargetFromConfig("http://example.com:9090", 1, 0, 0, 0, false, false, "")
		got := target.Hostname()
		if got != "example.com" {
			t.Errorf("Hostname() = %q, want %q", got, "example.com")
		}
	})
}

// TestConsistentHash_PrecomputeHashes 测试预计算哈希功能。
func TestConsistentHash_PrecomputeHashes(t *testing.T) {
	t.Run("预计算哈希值", func(_ *testing.T) {
		ch := NewConsistentHash(50, "ip")
		targets := []*Target{
			createHealthyTarget("http://backend1:8080", true),
			createHealthyTarget("http://backend2:8080", true),
		}

		ch.PrecomputeHashes(targets, 50)

		for _, target := range targets {
			if len(target.VirtualHashes) != 50 {
				t.Errorf("VirtualHashes len = %d, want 50", len(target.VirtualHashes))
			}
		}
	})

	t.Run("跳过已预计算的目标", func(_ *testing.T) {
		ch := NewConsistentHash(10, "ip")
		target := createHealthyTarget("http://backend1:8080", true)
		// 预计算一次
		ch.PrecomputeHashes([]*Target{target}, 10)
		firstHashes := make([]uint64, len(target.VirtualHashes))
		copy(firstHashes, target.VirtualHashes)

		// 再次预计算相同数量
		ch.PrecomputeHashes([]*Target{target}, 10)

		// 哈希值应保持不变
		for i, h := range target.VirtualHashes {
			if h != firstHashes[i] {
				t.Errorf("预计算改变了已有哈希值")
				break
			}
		}
	})

	t.Run("默认虚拟节点数", func(_ *testing.T) {
		ch := NewConsistentHash(0, "ip")
		targets := []*Target{
			createHealthyTarget("http://backend1:8080", true),
		}

		ch.PrecomputeHashes(targets, 0)

		if len(targets[0].VirtualHashes) != 150 {
			t.Errorf("VirtualHashes len = %d, want 150 (default)", len(targets[0].VirtualHashes))
		}
	})
}

// TestConsistentHash_SelectExcluding 测试一致性哈希SelectExcluding方法。
func TestConsistentHash_SelectExcluding(t *testing.T) {
	t.Run("委托给SelectExcludingByKey", func(_ *testing.T) {
		ch := NewConsistentHash(100, "ip")
		targets := []*Target{
			createHealthyTarget("http://backend1:8080", true),
			createHealthyTarget("http://backend2:8080", true),
		}
		ch.Rebuild(targets)

		excluded := []*Target{targets[0]}
		got := ch.SelectExcluding(targets, excluded)
		if got == nil {
			t.Fatal("SelectExcluding() = nil, want non-nil")
		}
		if got.URL == targets[0].URL {
			t.Errorf("选中了被排除的目标: %q", got.URL)
		}
	})
}

// TestLeastConnections_ConcurrentSelection 测试最少连接并发选择。
func TestLeastConnections_ConcurrentSelection(t *testing.T) {
	targets := []*Target{
		createHealthyTarget("http://backend1:8080", true),
		createHealthyTarget("http://backend2:8080", true),
		createHealthyTarget("http://backend3:8080", true),
	}
	lc := NewLeastConnections()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got := lc.Select(targets)
			if got == nil {
				t.Error("并发Select() = nil, want non-nil")
			}
		}()
	}
	wg.Wait()
}

// TestWeightedRoundRobin_ConcurrentSelection 测试加权轮询并发选择。
func TestWeightedRoundRobin_ConcurrentSelection(t *testing.T) {
	targets := []*Target{
		createHealthyTarget("http://backend1:8080", true),
		createHealthyTarget("http://backend2:8080", true),
	}
	targets[0].Weight = 3
	targets[1].Weight = 1
	wrr := NewWeightedRoundRobin()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got := wrr.Select(targets)
			if got == nil {
				t.Error("并发Select() = nil, want non-nil")
			}
		}()
	}
	wg.Wait()
}

// TestIPHash_ConcurrentSelection 测试IP哈希并发选择。
func TestIPHash_ConcurrentSelection(t *testing.T) {
	targets := []*Target{
		createHealthyTarget("http://backend1:8080", true),
		createHealthyTarget("http://backend2:8080", true),
	}
	ih := NewIPHash()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(ip string) {
			defer wg.Done()
			got := ih.SelectByIP(targets, ip)
			if got == nil {
				t.Error("并发SelectByIP() = nil, want non-nil")
			}
		}(net.IP{192, 168, 1, byte(i)}.String())
	}
	wg.Wait()
}

// TestTarget_Hostname_IPURL 测试纯IP地址URL的主机名提取。
func TestTarget_Hostname_IPURL(t *testing.T) {
	target := NewTargetFromConfig("http://10.0.0.1:8080", 1, 0, 0, 0, false, false, "")
	got := target.Hostname()
	if got != "10.0.0.1" {
		t.Errorf("Hostname() = %q, want %q", got, "10.0.0.1")
	}
}

// TestConsistentHash_SelectByKey_EmptyKey 测试空键选择行为。
func TestConsistentHash_SelectByKey_EmptyKey(t *testing.T) {
	ch := NewConsistentHash(100, "ip")
	targets := []*Target{
		createHealthyTarget("http://backend1:8080", true),
	}

	// 空键也应该能选择（不会panic）
	got := ch.SelectByKey(targets, "")
	if got == nil {
		t.Fatal("SelectByKey(\"\") = nil, want non-nil")
	}
}

// TestConsistentHash_RebuildWithAllUnhealthy 测试所有目标不健康时重建。
func TestConsistentHash_RebuildWithAllUnhealthy(t *testing.T) {
	ch := NewConsistentHash(10, "ip")
	targets := []*Target{
		createHealthyTarget("http://backend1:8080", false),
		createHealthyTarget("http://backend2:8080", false),
	}

	ch.Rebuild(targets)
	stats := ch.GetStats()

	if stats.CircleSize != 0 {
		t.Errorf("CircleSize = %d, want 0", stats.CircleSize)
	}
	if stats.SortedHashes != 0 {
		t.Errorf("SortedHashes = %d, want 0", stats.SortedHashes)
	}
}

// TestConsistentHash_GetStats 测试统计信息完整性。
func TestConsistentHash_GetStats(t *testing.T) {
	ch := NewConsistentHash(50, "uri")
	targets := []*Target{
		createHealthyTarget("http://backend1:8080", true),
		createHealthyTarget("http://backend2:8080", true),
		createHealthyTarget("http://backend3:8080", true),
	}

	ch.Rebuild(targets)
	stats := ch.GetStats()

	if stats.VirtualNodes != 50 {
		t.Errorf("VirtualNodes = %d, want 50", stats.VirtualNodes)
	}
	if stats.CircleSize != 150 { // 3 targets * 50 nodes
		t.Errorf("CircleSize = %d, want 150", stats.CircleSize)
	}
	if stats.SortedHashes != 150 {
		t.Errorf("SortedHashes = %d, want 150", stats.SortedHashes)
	}
}

// TestConsistentHash_GetHashKey 测试哈希键配置获取。
func TestConsistentHash_GetHashKey(t *testing.T) {
	tests := []struct {
		name    string
		hashKey string
		want    string
	}{
		{"ip", "ip", "ip"},
		{"uri", "uri", "uri"},
		{"header", "header:X-Forwarded-For", "header:X-Forwarded-For"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(_ *testing.T) {
			ch := NewConsistentHash(100, tt.hashKey)
			if ch.GetHashKey() != tt.want {
				t.Errorf("GetHashKey() = %q, want %q", ch.GetHashKey(), tt.want)
			}
		})
	}
}

// TestSelectExcluding_DynamicRebuild 测试SelectByKey触发的动态重建。
func TestSelectExcluding_DynamicRebuild(t *testing.T) {
	ch := NewConsistentHash(10, "ip")
	targets := []*Target{
		createHealthyTarget("http://backend1:8080", true),
		createHealthyTarget("http://backend2:8080", true),
	}

	// 不手动Rebuild，SelectByKey应自动触发
	got := ch.SelectByKey(targets, "10.0.0.1")
	if got == nil {
		t.Fatal("SelectByKey() = nil after auto-rebuild, want non-nil")
	}
}

// TestWeightedRoundRobin_NegativeWeight 测试负权重行为。
func TestWeightedRoundRobin_NegativeWeight(t *testing.T) {
	wrr := NewWeightedRoundRobin()
	targets := []*Target{
		createHealthyTarget("http://backend1:8080", true),
		createHealthyTarget("http://backend2:8080", true),
	}
	targets[0].Weight = -5
	targets[1].Weight = 3

	// 负权重要被当作1处理
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
		t.Error("负权重目标从未被选中")
	}
	if counts["http://backend2:8080"] == 0 {
		t.Error("正权重目标从未被选中")
	}
}

// TestRoundRobin_NilTargets 测试nil切片输入。
func TestRoundRobin_NilTargets(t *testing.T) {
	rr := NewRoundRobin()
	got := rr.Select(nil)
	if got != nil {
		t.Errorf("Select(nil) = %v, want nil", got)
	}
}

// TestLeastConnections_NilTargets 测试nil切片输入。
func TestLeastConnections_NilTargets(t *testing.T) {
	lc := NewLeastConnections()
	got := lc.Select(nil)
	if got != nil {
		t.Errorf("Select(nil) = %v, want nil", got)
	}
}

func TestTargetIsAvailable(t *testing.T) {
	t.Run("healthy target is available", func(t *testing.T) {
		target := NewTargetFromConfig("http://localhost:8080", 1, 0, 0, 0, false, false, "")
		if !target.IsAvailable() {
			t.Error("healthy target should be available")
		}
	})

	t.Run("down target is not available", func(t *testing.T) {
		target := NewTargetFromConfig("http://localhost:8080", 1, 0, 0, 0, false, true, "")
		if target.IsAvailable() {
			t.Error("down target should not be available")
		}
	})

	t.Run("unhealthy target is not available", func(t *testing.T) {
		target := NewTargetFromConfig("http://localhost:8080", 1, 0, 0, 0, false, false, "")
		target.Healthy.Store(false)
		if target.IsAvailable() {
			t.Error("unhealthy target should not be available")
		}
	})

	t.Run("max connections exceeded", func(t *testing.T) {
		target := NewTargetFromConfig("http://localhost:8080", 1, 2, 0, 0, false, false, "")
		IncrementConnections(target)
		IncrementConnections(target)
		if target.IsAvailable() {
			t.Error("target at max connections should not be available")
		}
	})

	t.Run("within max connections", func(t *testing.T) {
		target := NewTargetFromConfig("http://localhost:8080", 1, 2, 0, 0, false, false, "")
		IncrementConnections(target)
		if !target.IsAvailable() {
			t.Error("target under max connections should be available")
		}
	})
}

func TestTargetRecordFailure(t *testing.T) {
	t.Run("record failure increments count", func(t *testing.T) {
		target := NewTargetFromConfig("http://localhost:8080", 1, 0, 3, 10*time.Second, false, false, "")
		count := target.RecordFailure()
		if count != 1 {
			t.Errorf("expected fail count 1, got %d", count)
		}
	})

	t.Run("target becomes unavailable after max fails", func(t *testing.T) {
		target := NewTargetFromConfig("http://localhost:8080", 1, 0, 2, 10*time.Second, false, false, "")
		target.RecordFailure()
		target.RecordFailure()
		if target.IsAvailable() {
			t.Error("target should be unavailable after reaching max fails")
		}
	})

	t.Run("default fail timeout is 10s", func(t *testing.T) {
		target := NewTargetFromConfig("http://localhost:8080", 1, 0, 1, 0, false, false, "")
		target.RecordFailure()
		if target.IsAvailable() {
			t.Error("target should be unavailable with default timeout")
		}
	})
}

func TestTargetRecordSuccess(t *testing.T) {
	t.Run("record success resets fail count", func(t *testing.T) {
		target := NewTargetFromConfig("http://localhost:8080", 1, 0, 3, 10*time.Second, false, false, "")
		target.RecordFailure()
		target.RecordFailure()
		target.RecordSuccess()
		if target.failCount.Load() != 0 {
			t.Error("fail count should be reset after success")
		}
		if !target.IsAvailable() {
			t.Error("target should be available after success resets cooldown")
		}
	})
}

func TestFilterHealthyBackup(t *testing.T) {
	t.Run("prefers non-backup targets", func(t *testing.T) {
		primary := NewTargetFromConfig("http://primary:8080", 1, 0, 0, 0, false, false, "")
		backup := NewTargetFromConfig("http://backup:8080", 1, 0, 0, 0, true, false, "")
		targets := []*Target{primary, backup}

		result := filterHealthy(targets)
		if len(result) != 1 || result[0].URL != "http://primary:8080" {
			t.Error("should prefer non-backup target")
		}
	})

	t.Run("falls back to backup when primary unavailable", func(t *testing.T) {
		primary := NewTargetFromConfig("http://primary:8080", 1, 0, 0, 0, false, false, "")
		primary.Healthy.Store(false)
		backup := NewTargetFromConfig("http://backup:8080", 1, 0, 0, 0, true, false, "")
		targets := []*Target{primary, backup}

		result := filterHealthy(targets)
		if len(result) != 1 || result[0].URL != "http://backup:8080" {
			t.Error("should fall back to backup target")
		}
	})
}

func TestRandomBalancer(t *testing.T) {
	t.Run("selects from available targets", func(t *testing.T) {
		targets := []*Target{
			NewTargetFromConfig("http://a:8080", 1, 0, 0, 0, false, false, ""),
			NewTargetFromConfig("http://b:8080", 1, 0, 0, 0, false, false, ""),
		}
		b := NewRandom()
		selected := b.Select(targets)
		if selected == nil {
			t.Error("should select a target")
		}
	})

	t.Run("returns nil when no available targets", func(t *testing.T) {
		targets := []*Target{
			NewTargetFromConfig("http://a:8080", 1, 0, 0, 0, false, true, ""),
		}
		b := NewRandom()
		selected := b.Select(targets)
		if selected != nil {
			t.Error("should return nil for no available targets")
		}
	})

	t.Run("select excluding works", func(t *testing.T) {
		targets := []*Target{
			NewTargetFromConfig("http://a:8080", 1, 0, 0, 0, false, false, ""),
			NewTargetFromConfig("http://b:8080", 1, 0, 0, 0, false, false, ""),
		}
		b := NewRandom()
		selected := b.SelectExcluding(targets, targets[:1])
		if selected == nil || selected.URL == "http://a:8080" {
			t.Error("should exclude first target")
		}
	})
}
