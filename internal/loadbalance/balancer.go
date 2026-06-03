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
	"net"
	"net/url"
	"sync"
	"sync/atomic"
	"time"
)

type filterContext struct {
	available  []*Target
	backups    []*Target
	excludeSet map[string]bool
}

var filterContextPool = sync.Pool{
	New: func() any {
		return &filterContext{
			available:  make([]*Target, 0, 64),
			backups:    make([]*Target, 0, 64),
			excludeSet: make(map[string]bool, 8),
		}
	},
}

func acquireFilterContext() *filterContext {
	return filterContextPool.Get().(*filterContext)
}

func releaseFilterContext(fc *filterContext) {
	fc.available = fc.available[:0]
	fc.backups = fc.backups[:0]
	for k := range fc.excludeSet {
		delete(fc.excludeSet, k)
	}
	filterContextPool.Put(fc)
}

func fnvHash64a(key string) uint64 {
	var h uint64 = 14695981039346656037
	for i := 0; i < len(key); i++ {
		h ^= uint64(key[i])
		h *= 1099511628211
	}
	return h
}

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
	resolvedIPs   atomic.Pointer[[]string]
	URL           string
	hostname      string
	VirtualHashes []uint64
	Weight        int
	Connections   int64
	lastResolved  atomic.Int64
	hostnameOnce  sync.Once
	Healthy       atomic.Bool

	// MaxConns 最大并发连接数，0 表示不限制
	MaxConns int64
	// MaxFails 最大失败次数，0 表示不检测
	MaxFails int64
	// FailTimeout 失败冷却时间
	FailTimeout time.Duration
	// Backup 备份服务器标记
	Backup bool
	// Down 永久不可用标记
	Down bool
	// ProxyURI 代理传递的 URI 路径
	ProxyURI string

	failCount   atomic.Int64
	failedUntil atomic.Int64

	// 慢启动相关字段
	// EffectiveWeight 当前有效权重（慢启动期间动态变化）
	EffectiveWeight atomic.Int64
	// SlowStart 慢启动时间（配置）
	SlowStart time.Duration `yaml:"slow_start"`
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
//
// 它按顺序将请求均匀分配到所有健康目标上，适合后端服务器性能相近的场景。
//
// 并发安全：counter 使用 atomic 操作，支持多 goroutine 并发调用。
type RoundRobin struct {
	// counter 请求计数器，原子递增，用于确定轮询位置
	counter atomic.Uint64
}

// NewRoundRobin 创建一个新的轮询负载均衡器。
//
// 该函数初始化一个无状态的 RoundRobin 实例，内部 counter 从零开始。
// 适合后端服务器性能相近、无需权重区分的场景。
//
// 返回值：
//   - *RoundRobin: 初始化的轮询均衡器实例
func NewRoundRobin() *RoundRobin {
	return &RoundRobin{}
}

// Select 选择轮询顺序中的下一个目标。
// 只考虑健康目标。如果没有健康目标则返回 nil。
func (r *RoundRobin) Select(targets []*Target) *Target {
	fc := acquireFilterContext()
	defer releaseFilterContext(fc)
	healthy := filterInto(fc, targets)
	if len(healthy) == 0 {
		return nil
	}
	idx := r.counter.Add(1) - 1
	return healthy[idx%uint64(len(healthy))]
}

// WeightedRoundRobin 实现权重轮询负载均衡。
//
// 权重越高的目标接收成比例更多的请求，适合后端服务器性能差异较大的场景。
//
// 并发安全：counter 使用 atomic 操作，支持多 goroutine 并发调用。
type WeightedRoundRobin struct {
	// counter 请求计数器，原子递增，用于确定权重分布位置
	counter atomic.Uint64
}

// NewWeightedRoundRobin 创建一个新的权重轮询负载均衡器。
//
// 该函数初始化一个 WeightedRoundRobin 实例，内部 counter 从零开始。
// 权重越高的目标接收成比例更多的请求，适合后端服务器性能差异较大的场景。
//
// 返回值：
//   - *WeightedRoundRobin: 初始化的权重轮询均衡器实例
func NewWeightedRoundRobin() *WeightedRoundRobin {
	return &WeightedRoundRobin{}
}

// Select 基于权重分布选择目标。
// 只考虑健康目标。如果没有健康目标则返回 nil。
func (w *WeightedRoundRobin) Select(targets []*Target) *Target {
	fc := acquireFilterContext()
	defer releaseFilterContext(fc)
	healthy := filterInto(fc, targets)
	if len(healthy) == 0 {
		return nil
	}

	totalWeight := 0
	for _, t := range healthy {
		if t.Weight <= 0 {
			totalWeight++
		} else {
			totalWeight += t.Weight
		}
	}

	if totalWeight == 0 {
		return nil
	}

	idx := w.counter.Add(1) - 1
	pos := int(idx % uint64(totalWeight))

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

	return healthy[len(healthy)-1]
}

// LeastConnections 实现最少连接负载均衡。
//
// 它选择活跃连接数最少的目标，适合请求处理时间差异较大的场景。
//
// 该算法无状态设计，不维护内部计数器，直接读取目标连接数。
type LeastConnections struct{}

// NewLeastConnections 创建一个新的最少连接负载均衡器。
//
// 返回值：
//   - 初始化的 LeastConnections 实例
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
//
// 它将来自同一客户端 IP 的请求始终路由到同一目标，实现会话保持。
// 适合需要会话粘性的场景，如无状态后端的有状态请求处理。
type IPHash struct{}

// NewIPHash 创建一个新的 IP 哈希负载均衡器。
//
// 返回值：
//   - 初始化的 IPHash 实例
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
	fc := acquireFilterContext()
	defer releaseFilterContext(fc)
	healthy := filterInto(fc, targets)
	if len(healthy) == 0 {
		return nil
	}
	hash := fnvHash64a(clientIP)
	idx := hash % uint64(len(healthy))
	return healthy[idx]
}

// GetEffectiveWeight 获取目标的有效权重。
//
// 如果未配置慢启动或不在慢启动状态，返回配置权重。
// 慢启动期间，权重从 1 逐渐增加到配置权重。
func (t *Target) GetEffectiveWeight() int {
	ew := t.EffectiveWeight.Load()
	if ew == 0 {
		return t.Weight // 未配置慢启动时返回配置权重
	}
	return int(ew)
}

// IsAvailable 检查目标是否可用。
// 目标不可用的条件（优先级从高到低）：
//   - Healthy 为 false（硬性不可用，由健康检查器设置）
//   - Down 为 true（配置标记永久不可用）
//   - 超过 MaxConns 限制
//   - 失败冷却期内（failCount >= MaxFails 且未超过 FailTimeout）
func (t *Target) IsAvailable() bool {
	if !t.Healthy.Load() || t.Down {
		return false
	}
	if t.MaxConns > 0 && atomic.LoadInt64(&t.Connections) >= t.MaxConns {
		return false
	}
	if t.MaxFails > 0 {
		failCount := t.failCount.Load()
		if failCount >= t.MaxFails {
			failedUntil := t.failedUntil.Load()
			if time.Now().UnixNano() < failedUntil {
				return false
			}
			if failedUntil > 0 {
				t.failCount.Store(0)
				t.failedUntil.Store(0)
			}
		}
	}
	return true
}

// RecordFailure 记录一次失败。
// 使用互斥锁保护 failCount/failedUntil 的协调更新。
// 返回当前失败计数。
func (t *Target) RecordFailure() int64 {
	if t.MaxFails <= 0 {
		return 0
	}
	count := t.failCount.Add(1)
	if count >= t.MaxFails {
		timeout := t.FailTimeout
		if timeout <= 0 {
			timeout = 10 * time.Second
		}
		t.failedUntil.Store(time.Now().Add(timeout).UnixNano())
	}
	return count
}

// RecordSuccess 记录一次成功，重置软失败状态。
// 仅重置 failCount 和 failedUntil，不修改 Healthy（健康检查器权威）。
func (t *Target) RecordSuccess() {
	if t.MaxFails <= 0 {
		return
	}
	t.failCount.Store(0)
	t.failedUntil.Store(0)
}

// IsBackup 返回目标是否为备份服务器。
func (t *Target) IsBackup() bool {
	return t.Backup
}

func filterInto(fc *filterContext, targets []*Target) []*Target {
	for _, t := range targets {
		if !t.IsAvailable() {
			continue
		}
		if t.IsBackup() {
			fc.backups = append(fc.backups, t)
		} else {
			fc.available = append(fc.available, t)
		}
	}
	if len(fc.available) > 0 {
		return fc.available
	}
	return fc.backups
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

func filterIntoExcluding(fc *filterContext, targets []*Target, excluded []*Target) []*Target {
	for _, t := range excluded {
		if t != nil {
			fc.excludeSet[t.URL] = true
		}
	}
	for _, t := range targets {
		if !t.IsAvailable() || fc.excludeSet[t.URL] {
			continue
		}
		if t.IsBackup() {
			fc.backups = append(fc.backups, t)
		} else {
			fc.available = append(fc.available, t)
		}
	}
	if len(fc.available) > 0 {
		return fc.available
	}
	return fc.backups
}

// SelectExcluding 根据轮询策略选择一个目标，排除指定的目标列表。
// 只考虑健康且不在排除列表中的目标。
func (r *RoundRobin) SelectExcluding(targets []*Target, excluded []*Target) *Target {
	fc := acquireFilterContext()
	defer releaseFilterContext(fc)
	available := filterIntoExcluding(fc, targets, excluded)
	if len(available) == 0 {
		return nil
	}
	idx := r.counter.Add(1) - 1
	return available[idx%uint64(len(available))]
}

// SelectExcluding 根据权重分布选择目标，排除指定的目标列表。
// 只考虑健康且不在排除列表中的目标。
func (w *WeightedRoundRobin) SelectExcluding(targets []*Target, excluded []*Target) *Target {
	fc := acquireFilterContext()
	defer releaseFilterContext(fc)
	available := filterIntoExcluding(fc, targets, excluded)
	if len(available) == 0 {
		return nil
	}

	totalWeight := 0
	for _, t := range available {
		if t.Weight <= 0 {
			totalWeight++
		} else {
			totalWeight += t.Weight
		}
	}

	if totalWeight == 0 {
		return nil
	}

	idx := w.counter.Add(1) - 1
	pos := int(idx % uint64(totalWeight))

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

	return available[len(available)-1]
}

// SelectExcluding 选择连接数最少的目标，排除指定的目标列表。
// 优先选择非备份目标，仅当无可用非备份目标时选择备份目标。
func (l *LeastConnections) SelectExcluding(targets []*Target, excluded []*Target) *Target {
	fc := acquireFilterContext()
	defer releaseFilterContext(fc)
	for _, t := range excluded {
		if t != nil {
			fc.excludeSet[t.URL] = true
		}
	}

	var selected *Target
	var selectedBackup *Target
	var minConns int64 = -1
	var minBackupConns int64 = -1

	for _, t := range targets {
		if !t.IsAvailable() || fc.excludeSet[t.URL] {
			continue
		}
		conns := atomic.LoadInt64(&t.Connections)
		if t.IsBackup() {
			if selectedBackup == nil || conns < minBackupConns {
				selectedBackup = t
				minBackupConns = conns
			}
		} else {
			if selected == nil || conns < minConns {
				selected = t
				minConns = conns
			}
		}
	}

	if selected != nil {
		return selected
	}
	return selectedBackup
}

// SelectExcluding 基于客户端 IP 的哈希值选择目标，排除指定的目标列表。
// 只考虑健康且不在排除列表中的目标。
func (i *IPHash) SelectExcluding(targets []*Target, excluded []*Target) *Target {
	return i.SelectExcludingByIP(targets, excluded, "")
}

// SelectExcludingByIP 基于提供的 IP 地址的哈希值选择目标，排除指定的目标列表。
// 只考虑健康且不在排除列表中的目标。
func (i *IPHash) SelectExcludingByIP(targets []*Target, excluded []*Target, clientIP string) *Target {
	fc := acquireFilterContext()
	defer releaseFilterContext(fc)
	available := filterIntoExcluding(fc, targets, excluded)
	if len(available) == 0 {
		return nil
	}
	hash := fnvHash64a(clientIP)
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
// 自动初始化 hostname 和 Healthy 状态，设置上游参数。
func NewTargetFromConfig(url string, weight int, maxConns int64, maxFails int64, failTimeout time.Duration, backup bool, down bool, proxyURI string) *Target {
	t := &Target{
		URL:         url,
		Weight:      weight,
		MaxConns:    maxConns,
		MaxFails:    maxFails,
		FailTimeout: failTimeout,
		Backup:      backup,
		Down:        down,
		ProxyURI:    proxyURI,
	}
	t.initHostname()
	if !down {
		t.Healthy.Store(true)
	}
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


