// Package stream 提供 TCP/UDP Stream 代理性能的基准测试。
//
// 该文件测试流代理模块的性能热点，包括：
//   - filterHealthy 健康目标过滤（slice 分配热点）
//   - UDP 会话缓冲区分配
//   - UDP 会话创建/获取（双重检查锁定）
//   - 负载均衡算法性能
//
// 作者：xfy
package stream

import (
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// BenchmarkStreamFilterHealthy 基准测试健康目标过滤性能。
//
// 热点：每次 Select 都会分配新的 healthy slice，
// 在高并发场景下造成频繁的内存分配和 GC 压力。
func BenchmarkStreamFilterHealthy(b *testing.B) {
	testCases := []struct {
		name         string
		targetCount  int
		healthyRatio float64 // 健康目标比例
	}{
		{"3_healthy", 3, 1.0},
		{"10_healthy_80", 10, 0.8},
		{"50_healthy_50", 50, 0.5},
		{"100_healthy_80", 100, 0.8},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			// 创建目标列表
			targets := make([]*Target, tc.targetCount)
			for i := 0; i < tc.targetCount; i++ {
				targets[i] = &Target{
					addr:   fmt.Sprintf("backend%d:8080", i),
					weight: 1,
				}
				// 按比例设置健康状态
				if float64(i) < float64(tc.targetCount)*tc.healthyRatio {
					targets[i].healthy.Store(true)
				}
			}

			balancer := newRoundRobin()

			b.ResetTimer()
			b.ReportAllocs()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					_ = balancer.Select(targets)
				}
			})
		})
	}
}

// BenchmarkStreamFilterHealthyPreallocated 测试预分配 slice 的性能改进。
//
// 通过复用 slice 避免每次分配新的内存。
func BenchmarkStreamFilterHealthyPreallocated(b *testing.B) {
	targetCount := 50
	healthyRatio := 0.8

	// 创建目标列表
	targets := make([]*Target, targetCount)
	for i := range targetCount {
		targets[i] = &Target{
			addr:   fmt.Sprintf("backend%d:8080", i),
			weight: 1,
		}
		if float64(i) < float64(targetCount)*healthyRatio {
			targets[i].healthy.Store(true)
		}
	}

	b.ResetTimer()
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		// 复用预分配的 buffer
		buffer := make([]*Target, 0, targetCount)
		for pb.Next() {
			// 清空 slice 但保留容量
			buffer = buffer[:0]
			for _, t := range targets {
				if t.healthy.Load() {
					buffer = append(buffer, t)
				}
			}
		}
	})
}

// BenchmarkUDPSessionAllocations 基准测试 UDP 会话缓冲区分配。
//
// 热点：UDP 会话创建时分配 65KB 缓冲区（make([]byte, 65535)），
// 在高 QPS 场景下造成大量内存分配。
func BenchmarkUDPSessionAllocations(b *testing.B) {
	testCases := []struct {
		name     string
		bufSize  int
		poolSize int
	}{
		{"no_pool_65k", 65535, 0},
		{"sync_pool_65k", 65535, 100},
		{"no_pool_16k", 16384, 0},
		{"sync_pool_16k", 16384, 100},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			var pool *sync.Pool
			if tc.poolSize > 0 {
				pool = &sync.Pool{
					New: func() any {
						return make([]byte, tc.bufSize)
					},
				}
				// 预填充 pool
				for i := 0; i < tc.poolSize; i++ {
					pool.Put(make([]byte, tc.bufSize))
				}
			}

			b.ResetTimer()
			b.ReportAllocs()
			for b.Loop() {
				if pool != nil {
					buf := pool.Get().([]byte)
					// 模拟使用
					_ = buf[0]
					pool.Put(buf)
				} else {
					// 直接分配
					_ = make([]byte, tc.bufSize)
				}
			}
		})
	}
}

// BenchmarkUDPSessionGetOrCreate 基准测试 UDP 会话创建/获取性能。
//
// 热点：双重检查锁定模式在获取会话时的锁竞争，
// 高并发下 RLock -> Lock -> RLock 的升级路径造成性能瓶颈。
func BenchmarkUDPSessionGetOrCreate(b *testing.B) {
	// 创建 UDP 连接
	udpAddr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	conn, _ := net.ListenUDP("udp", udpAddr)
	defer conn.Close()

	// 创建上游
	upstream := &Upstream{
		targets:  []*Target{{addr: "127.0.0.1:19001"}},
		balancer: newRoundRobin(),
	}
	upstream.targets[0].healthy.Store(true)

	// 创建 UDP 服务器
	srv := newUDPServer(conn, upstream, 1*time.Minute)

	// 预创建一些客户端地址
	clientAddrs := make([]*net.UDPAddr, 100)
	for i := range 100 {
		clientAddrs[i], _ = net.ResolveUDPAddr("udp", fmt.Sprintf("127.0.0.1:%d", 20000+i))
	}

	b.ResetTimer()
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			// 模拟交替使用已有会话和创建新会话
			clientAddr := clientAddrs[i%len(clientAddrs)]
			srv.getOrCreateSession(clientAddr)
			i++
		}
	})
}

// BenchmarkUDPSessionGetOnly 基准测试纯获取会话性能。
//
// 测试已有会话的读取性能（只涉及 RLock）。
func BenchmarkUDPSessionGetOnly(b *testing.B) {
	// 创建 UDP 连接
	udpAddr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	conn, _ := net.ListenUDP("udp", udpAddr)
	defer conn.Close()

	// 创建上游
	upstream := &Upstream{
		targets:  []*Target{{addr: "127.0.0.1:19002"}},
		balancer: newRoundRobin(),
	}
	upstream.targets[0].healthy.Store(true)

	// 创建 UDP 服务器
	srv := newUDPServer(conn, upstream, 1*time.Minute)

	// 预创建会话
	clientAddrs := make([]*net.UDPAddr, 100)
	for i := range 100 {
		clientAddrs[i], _ = net.ResolveUDPAddr("udp", fmt.Sprintf("127.0.0.1:%d", 30000+i))
		// 手动创建会话
		targetAddr, _ := net.ResolveUDPAddr("udp", upstream.targets[0].addr)
		targetConn, _ := net.DialUDP("udp", nil, targetAddr)
		session := &udpSession{
			clientAddr: clientAddrs[i],
			targetConn: targetConn,
			lastActive: time.Now(),
			srv:        srv,
		}
		srv.sessions[sessionKey(clientAddrs[i])] = session
	}

	b.ResetTimer()
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			clientAddr := clientAddrs[i%len(clientAddrs)]
			srv.getSession(clientAddr)
			i++
		}
	})
}

// BenchmarkStreamBalancerSelect 基准测试各种负载均衡算法。
//
// 测试不同算法在高并发下的选择性能。
func BenchmarkStreamBalancerSelect(b *testing.B) {
	testCases := []struct {
		name        string
		balancer    string
		targetCount int
	}{
		{"round_robin_3", "round_robin", 3},
		{"round_robin_10", "round_robin", 10},
		{"round_robin_50", "round_robin", 50},
		{"weighted_round_robin_3", "weighted_round_robin", 3},
		{"weighted_round_robin_10", "weighted_round_robin", 10},
		{"least_conn_3", "least_conn", 3},
		{"least_conn_10", "least_conn", 10},
		{"ip_hash_3", "ip_hash", 3},
		{"ip_hash_10", "ip_hash", 10},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			// 创建目标
			targets := make([]*Target, tc.targetCount)
			for i := 0; i < tc.targetCount; i++ {
				targets[i] = &Target{
					addr:   fmt.Sprintf("backend%d:8080", i),
					weight: i + 1,
					conns:  int64(i * 10), // 模拟不同连接数
				}
				targets[i].healthy.Store(true)
			}

			// 创建均衡器
			var balancer Balancer
			switch tc.balancer {
			case "round_robin":
				balancer = newRoundRobin()
			case "weighted_round_robin":
				balancer = newWeightedRoundRobin()
			case "least_conn":
				balancer = newLeastConn()
			case "ip_hash":
				balancer = newIPHash()
			}

			b.ResetTimer()
			b.ReportAllocs()
			b.RunParallel(func(pb *testing.PB) {
				counter := uint64(0)
				for pb.Next() {
					if tc.balancer == "ip_hash" {
						// IP Hash 需要特定 IP
						idx := atomic.AddUint64(&counter, 1)
						_ = balancer.(*ipHash).SelectByIP(targets, fmt.Sprintf("192.168.1.%d", idx%255))
					} else {
						_ = balancer.Select(targets)
					}
				}
			})
		})
	}
}

// BenchmarkStreamRoundRobinWithUnhealthy 基准测试轮询算法处理不健康目标。
//
// 测试当部分目标不健康时的过滤开销。
func BenchmarkStreamRoundRobinWithUnhealthy(b *testing.B) {
	testCases := []struct {
		name           string
		targetCount    int
		unhealthyCount int
	}{
		{"3_1_unhealthy", 3, 1},
		{"10_3_unhealthy", 10, 3},
		{"50_20_unhealthy", 50, 20},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			targets := make([]*Target, tc.targetCount)
			for i := 0; i < tc.targetCount; i++ {
				targets[i] = &Target{
					addr:   fmt.Sprintf("backend%d:8080", i),
					weight: 1,
				}
				// 标记部分目标为不健康
				targets[i].healthy.Store(i >= tc.unhealthyCount)
			}

			balancer := newRoundRobin()

			b.ResetTimer()
			b.ReportAllocs()
			for b.Loop() {
				_ = balancer.Select(targets)
			}
		})
	}
}

// BenchmarkStreamLeastConnWithVaryingConns 基准测试最少连接算法。
//
// 测试不同连接数分布下的选择性能。
func BenchmarkStreamLeastConnWithVaryingConns(b *testing.B) {
	testCases := []struct {
		name      string
		connsDist []int64 // 每个目标的连接数分布
	}{
		{"uniform", []int64{10, 10, 10}},
		{"varying", []int64{100, 50, 10}},
		{"extreme", []int64{1000, 10, 1}},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			targets := make([]*Target, len(tc.connsDist))
			for i, conns := range tc.connsDist {
				targets[i] = &Target{
					addr:   fmt.Sprintf("backend%d:8080", i),
					weight: 1,
					conns:  conns,
				}
				targets[i].healthy.Store(true)
			}

			balancer := newLeastConn()

			b.ResetTimer()
			b.ReportAllocs()
			for b.Loop() {
				_ = balancer.Select(targets)
			}
		})
	}
}

// BenchmarkStreamWeightedRoundRobinDistribution 基准测试加权轮询分布。
//
// 测试加权轮询在不同权重分布下的选择性能。
func BenchmarkStreamWeightedRoundRobinDistribution(b *testing.B) {
	testCases := []struct {
		name    string
		weights []int
	}{
		{"equal", []int{1, 1, 1}},
		{"linear", []int{1, 2, 3}},
		{"heavy", []int{1, 1, 10}},
		{"exponential", []int{1, 2, 4, 8}},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			targets := make([]*Target, len(tc.weights))
			for i, w := range tc.weights {
				targets[i] = &Target{
					addr:   fmt.Sprintf("backend%d:8080", i),
					weight: w,
				}
				targets[i].healthy.Store(true)
			}

			balancer := newWeightedRoundRobin()

			b.ResetTimer()
			b.ReportAllocs()
			for b.Loop() {
				_ = balancer.Select(targets)
			}
		})
	}
}

// BenchmarkStreamIPHashWithDifferentIPs 基准测试 IP Hash 算法。
//
// 测试不同 IP 数量下的哈希性能。
func BenchmarkStreamIPHashWithDifferentIPs(b *testing.B) {
	testCases := []struct {
		name        string
		ipCount     int
		targetCount int
	}{
		{"10_ips_3_targets", 10, 3},
		{"100_ips_3_targets", 100, 3},
		{"1000_ips_3_targets", 1000, 3},
		{"100_ips_10_targets", 100, 10},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			targets := make([]*Target, tc.targetCount)
			for i := 0; i < tc.targetCount; i++ {
				targets[i] = &Target{
					addr:   fmt.Sprintf("backend%d:8080", i),
					weight: 1,
				}
				targets[i].healthy.Store(true)
			}

			// 预生成 IP 列表
			ips := make([]string, tc.ipCount)
			for i := 0; i < tc.ipCount; i++ {
				ips[i] = fmt.Sprintf("192.168.%d.%d", i/256, i%256)
			}

			balancer := newIPHash().(*ipHash)

			b.ResetTimer()
			b.ReportAllocs()
			for b.Loop() {
				// 使用固定的 IP 测试
				_ = balancer.SelectByIP(targets, "192.168.1.1")
			}
		})
	}
}
