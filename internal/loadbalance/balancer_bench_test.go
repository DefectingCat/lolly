// Package loadbalance 提供负载均衡算法的基准测试。
//
// 该文件测试各负载均衡算法在不同规模下的性能表现，包括：
//   - 轮询算法（RoundRobin）在不同目标数量下的吞吐量
//   - 加权轮询算法（WeightedRoundRobin）在等权重/变权重场景下的性能
//   - 一致性哈希算法（ConsistentHash）在不同虚拟节点数下的选择和重建开销
//   - 一致性哈希排除选择（SelectExcludingByKey）的性能
//   - 最少连接（LeastConnections）和 IP 哈希（IPHash）的基准性能
//   - 所有算法的横向对比测试
//
// 主要用途：
//
//	用于评估负载均衡算法在不同规模下的性能特征，指导算法选型和参数调优。
//
// 注意事项：
//   - 基准测试使用并行模式（RunParallel），结果受 CPU 核心数影响
//   - 测试前需确保目标均为健康状态
//
// 作者：xfy
package loadbalance

import (
	"fmt"
	"sync/atomic"
	"testing"
)

// generateTargets 生成指定数量的健康目标，用于基准测试。
//
// 参数：
//   - count: 目标数量
//
// 返回值：
//   - 包含 count 个健康目标的切片，权重均为 1
func generateTargets(count int) []*Target {
	targets := make([]*Target, count)
	for i := range count {
		targets[i] = &Target{
			URL:    fmt.Sprintf("http://backend%d:8080", i),
			Weight: 1,
		}
		targets[i].Healthy.Store(true)
	}
	return targets
}

// generateWeightedTargets 生成带指定权重的健康目标，用于基准测试。
//
// 参数：
//   - count: 目标数量
//   - weights: 权重列表，长度不足时默认权重为 1
//
// 返回值：
//   - 包含 count 个健康目标的切片，按 weights 分配权重
func generateWeightedTargets(count int, weights []int) []*Target {
	targets := make([]*Target, count)
	for i := range count {
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

// BenchmarkRoundRobinSelect 基准测试轮询算法在不同目标数量下的性能。
//
// 测试用例：
//   - 3 个目标：小规模场景
//   - 50 个目标：中等规模场景
//   - 200 个目标：大规模场景
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

// BenchmarkWeightedRoundRobin 基准测试加权轮询算法在等权重和变权重场景下的性能。
//
// 测试用例：
//   - 等权重：所有目标权重相同（1:1:1）
//   - 变权重：目标权重差异较大（1:5:10）
//   - 不同规模：3/50/200 个目标
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

// BenchmarkConsistentHashSelect 基准测试一致性哈希算法在不同虚拟节点数下的选择性能。
//
// 测试用例：
//   - 10 个目标 + 50/150/200 个虚拟节点
//   - 50/100 个目标 + 150 个虚拟节点
//
// 验证虚拟节点数量对哈希环遍历性能的影响。
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
			for i := range 100 {
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

// BenchmarkLeastConnSelect 基准测试最少连接算法在不同目标数量下的性能。
//
// 测试用例：
//   - 3/50/200 个目标，连接数按递增方式分配（模拟负载差异）
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
			for i := range 100 {
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
