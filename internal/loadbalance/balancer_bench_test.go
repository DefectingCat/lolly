// Package loadbalance 提供负载均衡算法的基准测试。
package loadbalance

import (
	"fmt"
	"sync/atomic"
	"testing"
)

// generateTargets 生成指定数量的健康目标用于基准测试。
func generateTargets(count int) []*Target {
	targets := make([]*Target, count)
	for i := 0; i < count; i++ {
		targets[i] = &Target{
			URL:    fmt.Sprintf("http://backend%d:8080", i),
			Weight: 1,
		}
		targets[i].Healthy.Store(true)
	}
	return targets
}

// generateWeightedTargets 生成带权重的目标用于基准测试。
func generateWeightedTargets(count int, weights []int) []*Target {
	targets := make([]*Target, count)
	for i := 0; i < count; i++ {
		weight := 1
		if i < len(weights) {
			weight = weights[i]
		}
		targets[i] = &Target{
			URL:    fmt.Sprintf("http://backend%d:8080", i),
			Weight: weight,
		}
		targets[i].Healthy.Store(true)
	}
	return targets
}

// BenchmarkRoundRobinSelect 基准测试轮询算法。
func BenchmarkRoundRobinSelect(b *testing.B) {
	testCases := []struct {
		name    string
		targets int
	}{
		{"3targets", 3},
		{"50targets", 50},
		{"200targets", 200},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			targets := generateTargets(tc.targets)
			rr := NewRoundRobin()

			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					rr.Select(targets)
				}
			})
		})
	}
}

// BenchmarkWeightedRoundRobin 基准测试加权轮询算法。
func BenchmarkWeightedRoundRobin(b *testing.B) {
	testCases := []struct {
		name    string
		targets int
		weights []int
	}{
		{"3targets_equal", 3, []int{1, 1, 1}},
		{"3targets_weighted", 3, []int{1, 5, 10}},
		{"50targets_equal", 50, nil},
		{"50targets_weighted", 50, []int{1, 2, 3, 4, 5, 1, 2, 3, 4, 5, 1, 2, 3, 4, 5, 1, 2, 3, 4, 5, 1, 2, 3, 4, 5, 1, 2, 3, 4, 5, 1, 2, 3, 4, 5, 1, 2, 3, 4, 5, 1, 2, 3, 4, 5, 1, 2, 3, 4, 5}},
		{"200targets_equal", 200, nil},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			targets := generateWeightedTargets(tc.targets, tc.weights)
			wrr := NewWeightedRoundRobin()

			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					wrr.Select(targets)
				}
			})
		})
	}
}

// BenchmarkConsistentHashSelect 基准测试一致性哈希算法。
func BenchmarkConsistentHashSelect(b *testing.B) {
	testCases := []struct {
		name         string
		targets      int
		virtualNodes int
	}{
		{"10targets_50vnodes", 10, 50},
		{"10targets_150vnodes", 10, 150},
		{"10targets_200vnodes", 10, 200},
		{"50targets_150vnodes", 50, 150},
		{"100targets_150vnodes", 100, 150},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			targets := generateTargets(tc.targets)
			ch := NewConsistentHash(tc.virtualNodes, "ip")
			ch.Rebuild(targets)

			keys := make([]string, 100)
			for i := 0; i < 100; i++ {
				keys[i] = fmt.Sprintf("192.168.1.%d", i)
			}

			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				i := uint64(0)
				for pb.Next() {
					keyIdx := atomic.AddUint64(&i, 1) % uint64(len(keys))
					ch.SelectByKey(targets, keys[keyIdx])
				}
			})
		})
	}
}

// BenchmarkConsistentHashRebuild 基准测试一致性哈希环重建性能。
func BenchmarkConsistentHashRebuild(b *testing.B) {
	testCases := []struct {
		name         string
		targets      int
		virtualNodes int
	}{
		{"10targets_150vnodes", 10, 150},
		{"50targets_150vnodes", 50, 150},
		{"100targets_150vnodes", 100, 150},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			targets := generateTargets(tc.targets)
			ch := NewConsistentHash(tc.virtualNodes, "ip")

			b.ResetTimer()
			for b.Loop() {
				ch.Rebuild(targets)
			}
		})
	}
}

// BenchmarkConsistentHashSelectExcluding 基准测试一致性哈希排除选择算法。
func BenchmarkConsistentHashSelectExcluding(b *testing.B) {
	testCases := []struct {
		name         string
		targets      int
		virtualNodes int
		excludeCount int
	}{
		{"50targets_150vnodes_exclude5", 50, 150, 5},
		{"50targets_150vnodes_exclude10", 50, 150, 10},
		{"100targets_150vnodes_exclude5", 100, 150, 5},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			targets := generateTargets(tc.targets)
			ch := NewConsistentHash(tc.virtualNodes, "ip")

			// 预计算所有目标的虚拟节点哈希
			ch.PrecomputeHashes(targets, tc.virtualNodes)
			ch.Rebuild(targets)

			excluded := targets[:tc.excludeCount]
			key := "test-request-key"

			b.ResetTimer()
			for b.Loop() {
				ch.SelectExcludingByKey(targets, excluded, key)
			}
		})
	}
}

func BenchmarkLeastConnSelect(b *testing.B) {
	testCases := []struct {
		name    string
		targets int
	}{
		{"3targets", 3},
		{"50targets", 50},
		{"200targets", 200},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			targets := generateTargets(tc.targets)
			// 设置不同的连接数以模拟真实场景
			for i, t := range targets {
				t.Connections = int64(i * 10)
			}
			lc := NewLeastConnections()

			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					lc.Select(targets)
				}
			})
		})
	}
}

// BenchmarkIPHashSelect 基准测试 IP 哈希算法。
func BenchmarkIPHashSelect(b *testing.B) {
	testCases := []struct {
		name    string
		targets int
	}{
		{"3targets", 3},
		{"50targets", 50},
		{"200targets", 200},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			targets := generateTargets(tc.targets)
			iph := NewIPHash()

			ips := make([]string, 100)
			for i := 0; i < 100; i++ {
				ips[i] = fmt.Sprintf("192.168.1.%d", i)
			}

			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				i := uint64(0)
				for pb.Next() {
					ipIdx := atomic.AddUint64(&i, 1) % uint64(len(ips))
					iph.SelectByIP(targets, ips[ipIdx])
				}
			})
		})
	}
}

// BenchmarkAllBalancers 对比所有负载均衡算法的性能。
func BenchmarkAllBalancers(b *testing.B) {
	targets := generateTargets(50)
	weightedTargets := generateWeightedTargets(50, nil)

	b.Run("RoundRobin", func(b *testing.B) {
		rr := NewRoundRobin()
		b.ResetTimer()
		for b.Loop() {
			rr.Select(targets)
		}
	})

	b.Run("WeightedRoundRobin", func(b *testing.B) {
		wrr := NewWeightedRoundRobin()
		b.ResetTimer()
		for b.Loop() {
			wrr.Select(weightedTargets)
		}
	})

	b.Run("LeastConnections", func(b *testing.B) {
		lc := NewLeastConnections()
		b.ResetTimer()
		for b.Loop() {
			lc.Select(targets)
		}
	})

	b.Run("IPHash", func(b *testing.B) {
		iph := NewIPHash()
		b.ResetTimer()
		for b.Loop() {
			iph.SelectByIP(targets, "192.168.1.100")
		}
	})

	b.Run("ConsistentHash", func(b *testing.B) {
		ch := NewConsistentHash(150, "ip")
		ch.Rebuild(targets)
		b.ResetTimer()
		for b.Loop() {
			ch.SelectByKey(targets, "192.168.1.100")
		}
	})
}
