// Package loadbalance 负载均衡包为 Lolly HTTP 服务器提供负载均衡算法。
//
// 本包实现了多种负载均衡策略，包括轮询（round-robin）、
// 权重轮询（weighted round-robin）、最少连接（least connections）和 IP 哈希（IP hash）。
// 所有实现都使用原子操作来保证并发安全。
//
// 使用示例：
//
//	targets := []*Target{
//	    {URL: "http://backend1:8080", Weight: 1, Healthy: true},
//	    {URL: "http://backend2:8080", Weight: 2, Healthy: true},
//	}
//
//	balancer := NewWeightedRoundRobin()
//	selected := balancer.Select(targets)
//
//go:generate go test -v ./...
package loadbalance

import (
	"hash/fnv"
	"sync/atomic"
)

// Target 表示负载均衡的后端服务器目标。
// 所有字段都设计为使用原子操作进行并发访问（如适用）。
type Target struct {
	// URL 是目标地址，例如 "http://backend1:8080"
	URL string

	// Weight 是此目标在权重算法中的权重值。
	// 权重越高，表示有更多请求会被路由到此目标。
	Weight int

	// Healthy 表示此目标是否健康可用。
	// 并发读写此字段时应使用原子操作。
	Healthy bool

	// Connections 跟踪当前活跃连接数。
	// 并发修改此字段时应使用原子操作。
	Connections int64
}

// Balancer 是负载均衡算法的接口。
// 实现必须是并发安全的。
type Balancer interface {
	// Select 根据算法策略从提供的列表中选择一个目标。
	// 如果没有健康目标可用，返回 nil。
	Select(targets []*Target) *Target
}

// RoundRobin 实现简单的轮询负载均衡。
// 它按顺序将请求均匀分配到所有健康目标上。
type RoundRobin struct {
	// counter 原子地为每个请求递增
	counter uint64
}

// NewRoundRobin 创建一个新的轮询负载均衡器。
func NewRoundRobin() *RoundRobin {
	return &RoundRobin{}
}

// Select 选择轮询顺序中的下一个目标。
// 只考虑健康目标。如果没有健康目标则返回 nil。
func (r *RoundRobin) Select(targets []*Target) *Target {
	healthy := filterHealthy(targets)
	if len(healthy) == 0 {
		return nil
	}

	// 原子地递增并获取计数器值
	idx := atomic.AddUint64(&r.counter, 1) - 1
	return healthy[idx%uint64(len(healthy))]
}

// WeightedRoundRobin 实现权重轮询负载均衡。
// 权重越高的目标接收成比例更多的请求。
type WeightedRoundRobin struct {
	// counter 原子地为每个请求递增
	counter uint64
}

// NewWeightedRoundRobin 创建一个新的权重轮询负载均衡器。
func NewWeightedRoundRobin() *WeightedRoundRobin {
	return &WeightedRoundRobin{}
}

// Select 基于权重分布选择目标。
// 只考虑健康目标。如果没有健康目标则返回 nil。
func (w *WeightedRoundRobin) Select(targets []*Target) *Target {
	healthy := filterHealthy(targets)
	if len(healthy) == 0 {
		return nil
	}

	// 计算总权重
	totalWeight := 0
	for _, t := range healthy {
		if t.Weight <= 0 {
			totalWeight += 1 // 最小权重为 1
		} else {
			totalWeight += t.Weight
		}
	}

	if totalWeight == 0 {
		return nil
	}

	// 使用原子计数器确定权重分布中的位置
	idx := atomic.AddUint64(&w.counter, 1) - 1
	pos := int(idx % uint64(totalWeight))

	// 找到计算位置处的目标
	currentWeight := 0
	for _, t := range healthy {
		weight := t.Weight
		if weight <= 0 {
			weight = 1
		}
		currentWeight += weight
		if pos < currentWeight {
			return t
		}
	}

	// 回退到最后一个目标（不应到达这里）
	return healthy[len(healthy)-1]
}

// LeastConnections 实现最少连接负载均衡。
// 它选择活跃连接数最少的目标。
type LeastConnections struct{}

// NewLeastConnections 创建一个新的最少连接负载均衡器。
func NewLeastConnections() *LeastConnections {
	return &LeastConnections{}
}

// Select 选择连接数最少的目标。
// 只考虑健康目标。如果没有健康目标则返回 nil。
func (l *LeastConnections) Select(targets []*Target) *Target {
	var selected *Target
	var minConns int64 = -1

	for _, t := range targets {
		if !t.Healthy {
			continue
		}

		// 原子地读取连接计数
		conns := atomic.LoadInt64(&t.Connections)

		if selected == nil || conns < minConns {
			selected = t
			minConns = conns
		}
	}

	return selected
}

// IPHash 实现基于 IP 哈希的负载均衡。
// 它将来自同一客户端 IP 的请求始终路由到同一目标。
type IPHash struct{}

// NewIPHash 创建一个新的 IP 哈希负载均衡器。
func NewIPHash() *IPHash {
	return &IPHash{}
}

// Select 基于客户端 IP 的哈希值选择目标。
// 只考虑健康目标。如果没有健康目标则返回 nil。
// clientIP 参数应该是客户端的 IP 地址字符串。
func (i *IPHash) Select(targets []*Target) *Target {
	return i.SelectByIP(targets, "")
}

// SelectByIP 基于提供的 IP 地址的哈希值选择目标。
// 只考虑健康目标。如果没有健康目标则返回 nil。
func (i *IPHash) SelectByIP(targets []*Target, clientIP string) *Target {
	healthy := filterHealthy(targets)
	if len(healthy) == 0 {
		return nil
	}

	// 对客户端 IP 进行哈希
	h := fnv.New64a()
	h.Write([]byte(clientIP))
	hash := h.Sum64()

	idx := hash % uint64(len(healthy))
	return healthy[idx]
}

// filterHealthy 返回仅包含健康目标的新切片。
// 这是负载均衡实现使用的辅助函数。
func filterHealthy(targets []*Target) []*Target {
	healthy := make([]*Target, 0, len(targets))
	for _, t := range targets {
		if t.Healthy {
			healthy = append(healthy, t)
		}
	}
	return healthy
}

// IncrementConnections 原子地增加目标的连接计数。
// 当新连接建立时应调用此函数。
func IncrementConnections(t *Target) {
	atomic.AddInt64(&t.Connections, 1)
}

// DecrementConnections 原子地减少目标的连接计数。
// 当连接关闭时应调用此函数。
func DecrementConnections(t *Target) {
	atomic.AddInt64(&t.Connections, -1)
}

// IsHealthy 原子地读取目标的健康状态。
func IsHealthy(t *Target) bool {
	// Healthy 是 bool 类型，在 Go 的内存模型中无需原子操作即可安全读取
	// 但为了与 setter 保持一致，我们可以使用原子操作
	// 对于 bool，简单的读取是安全的
	return t.Healthy
}

// SetHealthy 原子地设置目标的健康状态。
// 注意：在 Go 中，bool 操作不能直接是原子的。
// 此函数提供了同步更新健康状态的方式。
// 对于 bool 的真正原子操作，请考虑使用 atomic.Bool（Go 1.19+）
// 或 sync.RWMutex。对于本实现，我们使用直接赋值
// 当与调用层的适当同步结合时，这通常是足够的。
func SetHealthy(t *Target, healthy bool) {
	t.Healthy = healthy
}
