// Package loadbalance 提供负载均衡算法实现。
//
// 该文件包含负载均衡相关的核心定义，包括：
//   - Target 目标结构体定义
//   - Balancer 接口定义
//   - ValidAlgorithms 有效算法列表
//
// 主要用途：
//
//	用于定义负载均衡的标准接口和目标结构，支持多种负载均衡算法。
//
// 注意事项：
//   - 所有实现必须并发安全
//   - 目标的健康状态使用 atomic.Bool
//
// 作者：xfy
package loadbalance

import (
	"hash/fnv"
	"net"
	"net/url"
	"sync"
	"sync/atomic"
	"time"
)

// Target 表示 HTTP 代理（L7 层）的负载均衡后端服务器目标。
//
// HTTP Target 特性（区别于 Stream Target）：
//   - URL 解析：支持完整 URL（如 http://backend:8080），包含协议、路径、查询参数
//   - DNS 动态解析：resolvedIPs 和 lastResolved 字段支持 DNS TTL 缓存和动态重解析
//   - Failover 支持：配合 Balancer.SelectExcluding 实现失败节点排除重试
//   - 一致性哈希：VirtualHashes 支持一致性哈希算法的虚拟节点
//
// 语义差异说明：
//   - HTTP 代理工作在应用层（L7），需要处理 URL 和 DNS 解析
//   - Stream 代理工作在传输层（L4），只需简单 host:port，无需 DNS 缓存
//   - 因此 HTTP Target 和 Stream Target 必须保持独立定义，不可合并
//
// 所有字段都设计为使用原子操作进行并发访问（如适用）。
type Target struct {
	URL           string
	hostname      string
	VirtualHashes []uint64
	resolvedIPs   atomic.Pointer[[]string]
	Weight        int
	Connections   int64
	lastResolved  atomic.Int64
	hostnameOnce  sync.Once
	Healthy       atomic.Bool
}

// Balancer 是 HTTP 代理（L7 层）负载均衡算法的接口。
//
// HTTP Balancer 特性（区别于 Stream Balancer）：
//   - Select(): 标准选择方法，按算法策略选择健康目标
//   - SelectExcluding(): 故障转移支持，排除失败节点后选择替代目标
//
// 语义差异说明：
//   - HTTP 代理需要 failover 重试能力（next_upstream 配置），因此需要 SelectExcluding
//   - Stream 代理工作在传输层（L4），无重试机制，仅需要 Select 方法
//   - 因此 HTTP Balancer 和 Stream Balancer 接口签名不同，不可合并
//
// 实现必须是并发安全的。
type Balancer interface {
	// Select 根据算法策略从提供的列表中选择一个目标。
	// 如果没有健康目标可用，返回 nil。
	Select(targets []*Target) *Target

	// SelectExcluding 根据算法策略选择一个目标，排除指定的目标列表。
	// 用于故障转移场景，避免选择已失败的目标。
	// 如果除了排除列表外没有可用目标，返回 nil。
	SelectExcluding(targets []*Target, excluded []*Target) *Target
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
			totalWeight++ // 最小权重为 1
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
		if !t.Healthy.Load() {
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
		if t.Healthy.Load() {
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

// filterHealthyAndExclude 返回仅包含健康目标且不在排除列表中的新切片。
// 这是 SelectExcluding 使用的辅助函数。
func filterHealthyAndExclude(targets []*Target, excluded []*Target) []*Target {
	// 构建排除集合（使用 URL 作为键）
	excludeSet := make(map[string]bool, len(excluded))
	for _, t := range excluded {
		if t != nil {
			excludeSet[t.URL] = true
		}
	}

	// 过滤健康且不在排除列表中的目标
	available := make([]*Target, 0, len(targets))
	for _, t := range targets {
		if t.Healthy.Load() && !excludeSet[t.URL] {
			available = append(available, t)
		}
	}
	return available
}

// SelectExcluding 根据轮询策略选择一个目标，排除指定的目标列表。
// 只考虑健康且不在排除列表中的目标。
func (r *RoundRobin) SelectExcluding(targets []*Target, excluded []*Target) *Target {
	available := filterHealthyAndExclude(targets, excluded)
	if len(available) == 0 {
		return nil
	}

	// 原子地递增并获取计数器值
	idx := atomic.AddUint64(&r.counter, 1) - 1
	return available[idx%uint64(len(available))]
}

// SelectExcluding 根据权重分布选择目标，排除指定的目标列表。
// 只考虑健康且不在排除列表中的目标。
func (w *WeightedRoundRobin) SelectExcluding(targets []*Target, excluded []*Target) *Target {
	available := filterHealthyAndExclude(targets, excluded)
	if len(available) == 0 {
		return nil
	}

	// 计算总权重
	totalWeight := 0
	for _, t := range available {
		if t.Weight <= 0 {
			totalWeight++ // 最小权重为 1
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
	for _, t := range available {
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
	return available[len(available)-1]
}

// SelectExcluding 选择连接数最少的目标，排除指定的目标列表。
// 只考虑健康且不在排除列表中的目标。
func (l *LeastConnections) SelectExcluding(targets []*Target, excluded []*Target) *Target {
	// 构建排除集合
	excludeSet := make(map[string]bool, len(excluded))
	for _, t := range excluded {
		if t != nil {
			excludeSet[t.URL] = true
		}
	}

	var selected *Target
	var minConns int64 = -1

	for _, t := range targets {
		if !t.Healthy.Load() || excludeSet[t.URL] {
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

// SelectExcluding 基于客户端 IP 的哈希值选择目标，排除指定的目标列表。
// 只考虑健康且不在排除列表中的目标。
func (i *IPHash) SelectExcluding(targets []*Target, excluded []*Target) *Target {
	return i.SelectExcludingByIP(targets, excluded, "")
}

// SelectExcludingByIP 基于提供的 IP 地址的哈希值选择目标，排除指定的目标列表。
// 只考虑健康且不在排除列表中的目标。
func (i *IPHash) SelectExcludingByIP(targets []*Target, excluded []*Target, clientIP string) *Target {
	available := filterHealthyAndExclude(targets, excluded)
	if len(available) == 0 {
		return nil
	}

	// 对客户端 IP 进行哈希
	h := fnv.New64a()
	h.Write([]byte(clientIP))
	hash := h.Sum64()

	idx := hash % uint64(len(available))
	return available[idx]
}

// Hostname 返回目标主机名（从 URL 提取）。
// 使用 sync.Once 确保线程安全，只初始化一次。
func (t *Target) Hostname() string {
	t.hostnameOnce.Do(t.initHostname)
	return t.hostname
}

// ResolvedIPs 返回解析后的 IP 列表。
// 如果未解析过，返回 nil。
func (t *Target) ResolvedIPs() []string {
	ips := t.resolvedIPs.Load()
	if ips == nil {
		return nil
	}
	return *ips
}

// SetResolvedIPs 设置解析后的 IP 列表，并更新最后解析时间。
func (t *Target) SetResolvedIPs(ips []string) {
	// 创建副本避免外部修改
	ipsCopy := make([]string, len(ips))
	copy(ipsCopy, ips)
	t.resolvedIPs.Store(&ipsCopy)
	t.lastResolved.Store(time.Now().UnixNano())
}

// NeedsResolve 检查是否需要重新解析。
// 如果 hostname 是 IP 地址，返回 false。
// 如果从未解析过或超过 TTL，返回 true。
func (t *Target) NeedsResolve(ttl time.Duration) bool {
	host := t.Hostname()

	// IP 类型的 URL 不需要解析
	if net.ParseIP(host) != nil {
		return false
	}

	last := t.lastResolved.Load()
	if last == 0 {
		return true // 首次解析
	}

	return time.Since(time.Unix(0, last)) > ttl
}

// initHostname 从 URL 中提取并缓存主机名。
// 必须在 Target 创建后调用一次。
func (t *Target) initHostname() {
	u, err := url.Parse(t.URL)
	if err != nil {
		// 解析失败，使用整个 URL 作为 hostname
		t.hostname = t.URL
		return
	}

	// 提取主机名（去掉端口）
	host := u.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		t.hostname = h
	} else {
		t.hostname = host
	}
}

// NewTargetFromConfig 从配置创建 Target（推荐入口）。
// 自动初始化 hostname 和 Healthy 状态。
func NewTargetFromConfig(url string, weight int) *Target {
	t := &Target{
		URL:    url,
		Weight: weight,
	}
	t.initHostname()
	t.Healthy.Store(true)
	return t
}

// LastResolved 返回最后解析时间。
func (t *Target) LastResolved() time.Time {
	nano := t.lastResolved.Load()
	if nano == 0 {
		return time.Time{}
	}
	return time.Unix(0, nano)
}
