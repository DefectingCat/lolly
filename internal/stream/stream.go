// Package stream 提供 TCP/UDP Stream 代理功能。
//
// 该文件实现第四层（传输层）代理，支持 MySQL、PostgreSQL、DNS 等服务的代理转发。
// 与 HTTP 代理不同，Stream 代理不解析应用层协议，而是进行透明的双向数据转发。
//
// 主要功能：
//   - TCP 代理：支持 TCP 连接的代理转发
//   - UDP 代理：支持 UDP 数据报的代理转发
//   - 负载均衡：支持轮询和最少连接算法
//   - 健康检查：定期检查后端服务可用性
//   - 会话管理：UDP 会话自动过期清理
//
// 使用示例：
//
//	server := stream.NewServer()
//	err := server.AddUpstream("mysql", []stream.TargetSpec{
//	    {Addr: "db1:3306", Weight: 1},
//	    {Addr: "db2:3306", Weight: 2},
//	}, "round_robin", stream.HealthCheckSpec{Enabled: true})
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	server.ListenTCP(":3306")
//	server.Start()
//	defer server.Stop()
//
// 作者：xfy
package stream

import (
	"hash/fnv"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// Balancer Stream 代理（L4 层）负载均衡器接口。
//
// Stream Balancer 特性（区别于 HTTP Balancer）：
//   - 仅 Select(): 按算法策略选择健康目标
//   - 无 SelectExcluding(): Stream 代理无 failover 重试机制
//
// 语义差异说明：
//   - Stream 代理工作在传输层（L4），连接建立后直接转发数据，无应用层重试
//   - HTTP 代理工作在应用层（L7），支持 next_upstream 配置的失败重试
//   - 因此 Stream Balancer 接口签名更简单，仅需要 Select 方法
//   - HTTP Balancer 需要 SelectExcluding 用于排除失败节点
//   - 两种 Balancer 接口签名不同，不可合并
type Balancer interface {
	Select(targets []*Target) *Target
}

// roundRobin 简单轮询负载均衡器。
//
// 使用原子计数器实现线程安全的轮询选择，每次选择后计数器递增，
// 确保请求均匀分布到所有健康目标。
type roundRobin struct {
	counter uint64
}

// newRoundRobin 创建轮询均衡器。
//
// 返回实现了简单轮询算法的 Balancer 接口实例。
//
// 返回值：
//   - Balancer: 轮询负载均衡器实例
func newRoundRobin() Balancer {
	return &roundRobin{}
}

// Select 选择下一个目标。
//
// 从健康目标列表中按轮询顺序选择一个目标。
// 只选择标记为健康的目标，如果无健康目标则返回 nil。
//
// 参数：
//   - targets: 目标服务器列表
//
// 返回值：
//   - *Target: 选中的目标服务器，无可用目标时返回 nil
func (r *roundRobin) Select(targets []*Target) *Target {
	// 过滤健康目标
	healthy := make([]*Target, 0)
	for _, t := range targets {
		if t.healthy.Load() {
			healthy = append(healthy, t)
		}
	}
	if len(healthy) == 0 {
		return nil
	}
	idx := atomic.AddUint64(&r.counter, 1) - 1
	return healthy[idx%uint64(len(healthy))]
}

// leastConn 最少连接负载均衡器。
//
// 跟踪每个目标服务器的活跃连接数，总是选择连接数最少的目标。
// 适合连接持续时间差异较大的场景。
type leastConn struct{}

// newLeastConn 创建最少连接均衡器。
//
// 返回实现了最少连接算法的 Balancer 接口实例。
//
// 返回值：
//   - Balancer: 最少连接负载均衡器实例
func newLeastConn() Balancer {
	return &leastConn{}
}

// Select 选择连接最少的目标。
//
// 遍历所有健康目标，选择当前连接数最少的目标服务器。
// 只选择标记为健康的目标，如果无健康目标则返回 nil。
//
// 参数：
//   - targets: 目标服务器列表
//
// 返回值：
//   - *Target: 选中的目标服务器，无可用目标时返回 nil
func (l *leastConn) Select(targets []*Target) *Target {
	var selected *Target
	var minConns int64 = -1
	for _, t := range targets {
		if !t.healthy.Load() {
			continue
		}
		conns := atomic.LoadInt64(&t.conns)
		if selected == nil || conns < minConns {
			selected = t
			minConns = conns
		}
	}
	return selected
}

// weightedRoundRobin 加权轮询负载均衡器。
//
// 根据目标服务器的权重分配请求，权重高的目标获得更多请求。
// 使用原子计数器确保线程安全，支持不同权重的目标混合使用。
type weightedRoundRobin struct {
	counter uint64
}

// newWeightedRoundRobin 创建加权轮询均衡器。
//
// 返回实现了加权轮询算法的 Balancer 接口实例。
//
// 返回值：
//   - Balancer: 加权轮询负载均衡器实例
func newWeightedRoundRobin() Balancer {
	return &weightedRoundRobin{}
}

// Select 基于权重分布选择目标。
//
// 根据目标服务器的权重进行轮询选择，权重决定被选中的概率。
// 权重为 0 或负数的目标按权重 1 处理。
// 只选择标记为健康的目标，如果无健康目标则返回 nil。
//
// 参数：
//   - targets: 目标服务器列表
//
// 返回值：
//   - *Target: 选中的目标服务器，无可用目标时返回 nil
func (w *weightedRoundRobin) Select(targets []*Target) *Target {
	healthy := make([]*Target, 0)
	for _, t := range targets {
		if t.healthy.Load() {
			healthy = append(healthy, t)
		}
	}
	if len(healthy) == 0 {
		return nil
	}

	// 计算总权重
	totalWeight := 0
	for _, t := range healthy {
		if t.weight <= 0 {
			totalWeight++ // 最小权重为 1
		} else {
			totalWeight += t.weight
		}
	}

	// 使用原子计数器确定位置
	idx := atomic.AddUint64(&w.counter, 1) - 1
	pos := int(idx % uint64(totalWeight))

	// 找到对应位置的目标
	currentWeight := 0
	for _, t := range healthy {
		weight := t.weight
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

// ipHash IP 哈希负载均衡器。
//
// 基于客户端 IP 地址计算哈希值，将同一 IP 的请求分配到固定目标。
// 适合需要会话保持的场景，确保相同客户端总是被路由到同一服务器。
type ipHash struct{}

// newIPHash 创建 IP 哈希均衡器。
//
// 返回实现了 IP 哈希算法的 Balancer 接口实例。
//
// 返回值：
//   - Balancer: IP 哈希负载均衡器实例
func newIPHash() Balancer {
	return &ipHash{}
}

// Select 默认选择（IP Hash 需要具体 IP）。
//
// 当无客户端 IP 时，使用空字符串进行哈希计算。
// 通常应使用 SelectByIP 方法指定客户端 IP。
//
// 参数：
//   - targets: 目标服务器列表
//
// 返回值：
//   - *Target: 选中的目标服务器，无可用目标时返回 nil
func (i *ipHash) Select(targets []*Target) *Target {
	return i.SelectByIP(targets, "")
}

// SelectByIP 基于客户端 IP 哈希选择目标。
//
// 使用 FNV-64a 算法计算客户端 IP 的哈希值，对目标数取模选择目标。
// 同一 IP 总是映射到同一目标（除非目标列表变化）。
// 只选择标记为健康的目标，如果无健康目标则返回 nil。
//
// 参数：
//   - targets: 目标服务器列表
//   - clientIP: 客户端 IP 地址字符串
//
// 返回值：
//   - *Target: 选中的目标服务器，无可用目标时返回 nil
func (i *ipHash) SelectByIP(targets []*Target, clientIP string) *Target {
	healthy := make([]*Target, 0)
	for _, t := range targets {
		if t.healthy.Load() {
			healthy = append(healthy, t)
		}
	}
	if len(healthy) == 0 {
		return nil
	}

	// 使用 FNV-64a 哈希
	h := fnv.New64a()
	h.Write([]byte(clientIP))
	hash := h.Sum64()

	idx := hash % uint64(len(healthy))
	return healthy[idx]
}

// Server TCP/UDP Stream 代理服务器。
type Server struct {
	// listeners TCP 监听器映射，按 upstream 名称索引
	listeners map[string]net.Listener
	// udpServers UDP 服务器映射
	udpServers map[string]*udpServer
	// upstreams 上游配置映射
	upstreams map[string]*Upstream
	// connCount 当前连接数
	connCount int64
	// mu 读写锁，保护并发访问
	mu sync.RWMutex
	// running 运行状态标志
	running atomic.Bool
}

// Upstream Stream 上游配置。
type Upstream struct {
	balancer  Balancer
	healthChk *HealthChecker
	name      string
	targets   []*Target
	mu        sync.RWMutex
}

// Target Stream 代理（L4 层）的目标服务器。
//
// Stream Target 特性（区别于 HTTP Target）：
//   - 简单地址：仅支持 host:port 格式，无 URL 解析
//   - 无 DNS 缓存：直接连接目标地址，无需动态 DNS 解析
//   - 无 failover：Stream 代理无重试机制，仅记录健康状态
//
// 语义差异说明：
//   - Stream 代理工作在传输层（L4），直接转发 TCP/UDP 数据
//   - HTTP 代理工作在应用层（L7），需要 URL 解析和 DNS 动态解析
//   - 因此 Stream Target 保持简单结构，HTTP Target 需要 DNS 缓存等复杂功能
//   - 两种 Target 必须保持独立定义，不可合并
type Target struct {
	// addr 目标地址（host:port）
	addr string
	// weight 权重
	weight int
	// healthy 健康状态
	healthy atomic.Bool
	// conns 当前连接数
	conns int64
}

// HealthChecker Stream 健康检查器。
type HealthChecker struct {
	// upstream 所属上游
	upstream *Upstream
	// stopCh 停止信号通道
	stopCh chan struct{}
	// interval 检查间隔
	interval time.Duration
	// timeout 检查超时
	timeout time.Duration
}

// Config Stream 配置。
type Config struct {
	// Listen 监听地址
	Listen string
	// Protocol 协议类型（tcp 或 udp）
	Protocol string
	// Upstream 上游配置
	Upstream UpstreamSpec
}

// UpstreamSpec 上游配置规格。
type UpstreamSpec struct {
	Name        string
	LoadBalance string
	Targets     []TargetSpec
	HealthCheck HealthCheckSpec
}

// TargetSpec 目标配置规格。
type TargetSpec struct {
	// Addr 目标地址（host:port）
	Addr string
	// Weight 权重
	Weight int
}

// HealthCheckSpec 健康检查配置规格。
type HealthCheckSpec struct {
	// Interval 检查间隔
	Interval time.Duration
	// Timeout 检查超时
	Timeout time.Duration
	// Enabled 是否启用
	Enabled bool
}

// NewServer 创建 Stream 服务器。
func NewServer() *Server {
	return &Server{
		listeners:  make(map[string]net.Listener),
		udpServers: make(map[string]*udpServer),
		upstreams:  make(map[string]*Upstream),
	}
}

// AddUpstream 添加上游配置。
func (s *Server) AddUpstream(name string, targets []TargetSpec, lbType string, hcSpec HealthCheckSpec) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 创建目标列表
	tgts := make([]*Target, len(targets))
	for i, t := range targets {
		tgts[i] = &Target{
			addr:   t.Addr,
			weight: t.Weight,
		}
		tgts[i].healthy.Store(true) // 初始假设健康
	}

	// 创建负载均衡器
	var balancer Balancer
	switch lbType {
	case "round_robin", "":
		balancer = newRoundRobin()
	case "weighted_round_robin":
		balancer = newWeightedRoundRobin()
	case "least_conn":
		balancer = newLeastConn()
	case "ip_hash":
		balancer = newIPHash()
	default:
		balancer = newRoundRobin()
	}

	upstream := &Upstream{
		name:     name,
		targets:  tgts,
		balancer: balancer,
	}

	// 启动健康检查
	if hcSpec.Enabled {
		upstream.healthChk = &HealthChecker{
			upstream: upstream,
			interval: hcSpec.Interval,
			timeout:  hcSpec.Timeout,
			stopCh:   make(chan struct{}),
		}
		go upstream.healthChk.Start()
	}

	s.upstreams[name] = upstream
	return nil
}

// ListenTCP 开始监听 TCP 端口。
func (s *Server) ListenTCP(addr string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	s.listeners[addr] = listener
	return nil
}

// ListenUDP 开始监听 UDP 端口。
func (s *Server) ListenUDP(addr string, upstreamName string, timeout time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 查找上游配置
	upstream, exists := s.upstreams[upstreamName]
	if !exists {
		return io.ErrClosedPipe
	}

	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return err
	}

	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return err
	}

	// 创建 UDP 服务器
	udpSrv := newUDPServer(conn, upstream, timeout)
	s.udpServers[addr] = udpSrv

	return nil
}

// Start 启动 Stream 服务器。
func (s *Server) Start() error {
	s.running.Store(true)

	s.mu.RLock()
	defer s.mu.RUnlock()

	// 启动 TCP 监听器
	for addr, listener := range s.listeners {
		go s.acceptLoop(addr, listener)
	}

	// 启动 UDP 服务器
	for _, udpSrv := range s.udpServers {
		go udpSrv.serve()
		go udpSrv.startCleanupTicker()
	}

	return nil
}

// acceptLoop 接受连接循环。
//
// 在单独的 goroutine 中运行，持续接受 TCP 连接。
// 当服务器停止时，该函数返回。
//
// 参数：
//   - addr: 监听地址
//   - listener: TCP 监听器实例
func (s *Server) acceptLoop(addr string, listener net.Listener) {
	for s.running.Load() {
		conn, err := listener.Accept()
		if err != nil {
			if !s.running.Load() {
				return // 正常关闭
			}
			continue
		}

		s.connCount++
		go s.handleConnection(conn, addr)
	}
}

// handleConnection 处理单个连接。
//
// 处理客户端连接的完整生命周期：
// 1. 选择上游目标
// 2. 建立到目标服务器的连接
// 3. 在客户端和目标之间双向转发数据
// 4. 处理连接关闭和错误
//
// 参数：
//   - clientConn: 客户端连接
//   - addr: 监听地址
func (s *Server) handleConnection(clientConn net.Conn, _ string) {
	defer func() {
		_ = clientConn.Close()
		s.connCount--
	}()

	s.mu.RLock()
	// 根据监听地址找到对应 upstream（简化：用第一个）
	var upstream *Upstream
	for _, up := range s.upstreams {
		upstream = up
		break
	}
	s.mu.RUnlock()

	if upstream == nil {
		return // 无上游配置
	}

	// 选择目标
	target := upstream.Select()
	if target == nil {
		return // 无可用目标
	}

	target.conns++
	defer func() { target.conns-- }()

	// 连接目标
	targetConn, err := net.DialTimeout("tcp", target.addr, 10*time.Second)
	if err != nil {
		target.healthy.Store(false)
		return
	}
	defer func() { _ = targetConn.Close() }()

	// 双向数据转发
	go func() { _, _ = io.Copy(targetConn, clientConn) }()
	_, _ = io.Copy(clientConn, targetConn)
}

// Select 选择健康的上游目标。
func (u *Upstream) Select() *Target {
	u.mu.RLock()
	defer u.mu.RUnlock()

	// 获取健康目标列表
	healthyTargets := make([]*Target, 0)
	for _, t := range u.targets {
		if t.healthy.Load() {
			healthyTargets = append(healthyTargets, t)
		}
	}

	if len(healthyTargets) == 0 {
		return nil
	}

	// 使用负载均衡器选择
	return u.balancer.Select(healthyTargets)
}

// Start 启动健康检查。
func (h *HealthChecker) Start() {
	ticker := time.NewTicker(h.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			h.check()
		case <-h.stopCh:
			return
		}
	}
}

// check 执行健康检查。
//
// 尝试 TCP 连接到所有目标，根据连接结果更新健康状态。
// 连接成功标记为健康，连接失败标记为不健康。
func (h *HealthChecker) check() {
	for _, target := range h.upstream.targets {
		conn, err := net.DialTimeout("tcp", target.addr, h.timeout)
		if err != nil {
			target.healthy.Store(false)
		} else {
			_ = conn.Close()
			target.healthy.Store(true)
		}
	}
}

// Stop 停止健康检查。
func (h *HealthChecker) Stop() {
	close(h.stopCh)
}

// Stop 停止 Stream 服务器。
func (s *Server) Stop() error {
	s.running.Store(false)

	s.mu.Lock()
	defer s.mu.Unlock()

	// 关闭所有 TCP 监听器
	for _, listener := range s.listeners {
		_ = listener.Close()
	}

	// 停止所有 UDP 服务器
	for _, udpSrv := range s.udpServers {
		udpSrv.stop()
	}

	// 停止健康检查
	for _, upstream := range s.upstreams {
		if upstream.healthChk != nil {
			upstream.healthChk.Stop()
		}
	}

	return nil
}

// Stats 返回服务器统计信息。
func (s *Server) Stats() Stats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return Stats{
		Connections: s.connCount,
		Listeners:   len(s.listeners) + len(s.udpServers),
		Upstreams:   len(s.upstreams),
	}
}

// Stats Stream 服务器统计。
type Stats struct {
	// Connections 当前活跃连接数量
	Connections int64

	// Listeners 当前监听器数量（TCP + UDP）
	Listeners int

	// Upstreams 上游配置数量
	Upstreams int
}

// udpSession UDP 会话，管理客户端到后端的映射。
//
// 每个 UDP 会话代表一个客户端与一个后端目标的映射关系。
// 会话包含客户端地址、后端连接、最后活跃时间等信息。
// 会话在空闲超时后自动清理。
type udpSession struct {
	lastActive time.Time
	targetConn net.Conn
	clientAddr *net.UDPAddr
	srv        *udpServer
	mu         sync.RWMutex
	closeOnce  sync.Once
}

// udpServer UDP 服务器，管理多个客户端会话。
//
// 负责监听 UDP 端口，管理客户端会话的生命周期。
// 支持会话自动过期清理和优雅停止。
type udpServer struct {
	conn     *net.UDPConn
	sessions map[string]*udpSession
	upstream *Upstream
	stopCh   chan struct{}
	wg       sync.WaitGroup
	timeout  time.Duration
	mu       sync.RWMutex
	running  atomic.Bool
}

// newUDPServer 创建新的 UDP 服务器。
//
// 根据 UDP 连接、上游配置和超时时间创建 UDP 服务器实例。
// 如果超时时间小于等于 0，使用默认 60 秒。
//
// 参数：
//   - conn: UDP 连接
//   - upstream: 上游配置
//   - timeout: 会话空闲超时时间
//
// 返回值：
//   - *udpServer: 创建的 UDP 服务器实例
func newUDPServer(conn *net.UDPConn, upstream *Upstream, timeout time.Duration) *udpServer {
	if timeout <= 0 {
		timeout = 60 * time.Second // 默认 60 秒超时
	}
	return &udpServer{
		conn:     conn,
		sessions: make(map[string]*udpSession),
		upstream: upstream,
		timeout:  timeout,
		stopCh:   make(chan struct{}),
	}
}

// sessionKey 从 UDP 地址生成会话键。
//
// 使用 UDP 地址的字符串表示作为会话映射的唯一键。
//
// 参数：
//   - addr: UDP 地址
//
// 返回值：
//   - string: 会话键字符串
func sessionKey(addr *net.UDPAddr) string {
	return addr.String()
}

// getSession 获取现有会话（线程安全）。
//
// 根据客户端地址查找现有会话，如果找到则更新最后活跃时间。
// 如果会话不存在返回 nil。
//
// 参数：
//   - clientAddr: 客户端 UDP 地址
//
// 返回值：
//   - *udpSession: 找到的会话，不存在时返回 nil
func (s *udpServer) getSession(clientAddr *net.UDPAddr) *udpSession {
	s.mu.RLock()
	defer s.mu.RUnlock()

	session, exists := s.sessions[sessionKey(clientAddr)]
	if !exists {
		return nil
	}

	// 更新最后活动时间
	session.mu.Lock()
	session.lastActive = time.Now()
	session.mu.Unlock()

	return session
}

// getOrCreateSession 获取或创建会话（线程安全）。
//
// 首先尝试获取现有会话，如果不存在则创建新会话：
// 1. 选择后端目标
// 2. 建立到目标的 UDP 连接
// 3. 创建会话并启动响应监听 goroutine
//
// 使用双重检查锁定模式避免重复创建会话。
//
// 参数：
//   - clientAddr: 客户端 UDP 地址
//
// 返回值：
//   - *udpSession: 获取或创建的会话
//   - error: 创建失败时返回错误
func (s *udpServer) getOrCreateSession(clientAddr *net.UDPAddr) (*udpSession, error) {
	// 先尝试获取现有会话
	session := s.getSession(clientAddr)
	if session != nil {
		return session, nil
	}

	// 需要创建新会话，获取写锁
	s.mu.Lock()
	defer s.mu.Unlock()

	// 双重检查：可能另一个 goroutine 已经创建了会话
	if existingSession, exists := s.sessions[sessionKey(clientAddr)]; exists {
		existingSession.mu.Lock()
		existingSession.lastActive = time.Now()
		existingSession.mu.Unlock()
		return existingSession, nil
	}

	// 选择后端目标
	target := s.upstream.Select()
	if target == nil {
		return nil, io.ErrClosedPipe
	}

	// 连接到后端（使用 UDP 连接）
	targetAddr, err := net.ResolveUDPAddr("udp", target.addr)
	if err != nil {
		return nil, err
	}

	targetConn, err := net.DialUDP("udp", nil, targetAddr)
	if err != nil {
		return nil, err
	}

	target.conns++

	// 创建新会话
	session = &udpSession{
		clientAddr: clientAddr,
		targetConn: targetConn,
		lastActive: time.Now(),
		srv:        s,
	}

	s.sessions[sessionKey(clientAddr)] = session

	// 启动后端响应监听
	s.wg.Add(1)
	go session.handleBackendResponse()

	return session, nil
}

// removeSession 移除会话（线程安全）。
//
// 关闭会话并删除会话映射。
//
// 参数：
//   - clientAddr: 客户端 UDP 地址
func (s *udpServer) removeSession(clientAddr *net.UDPAddr) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := sessionKey(clientAddr)
	if session, exists := s.sessions[key]; exists {
		session.close()
		delete(s.sessions, key)
	}
}

// close 关闭会话。
//
// 使用 sync.Once 确保会话只关闭一次。
// 关闭后端目标连接。
func (sess *udpSession) close() {
	sess.closeOnce.Do(func() {
		if sess.targetConn != nil {
			_ = sess.targetConn.Close()
		}
	})
}

// handleBackendResponse 处理后端响应并转发回客户端。
//
// 在单独的 goroutine 中运行，持续监听后端响应：
// 1. 读取后端数据
// 2. 转发到客户端
// 3. 处理超时和错误
//
// 当会话超时、连接错误或服务器停止时返回。
func (sess *udpSession) handleBackendResponse() {
	defer sess.srv.wg.Done()

	buf := make([]byte, 65535)
	for {
		// 设置读取超时
		_ = sess.targetConn.SetReadDeadline(time.Now().Add(sess.srv.timeout))

		n, err := sess.targetConn.Read(buf)
		if err != nil {
			// 超时或其他错误，检查是否需要关闭
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				// 检查是否超过空闲超时
				sess.mu.RLock()
				lastActive := sess.lastActive
				sess.mu.RUnlock()

				if time.Since(lastActive) >= sess.srv.timeout {
					sess.srv.removeSession(sess.clientAddr)
					return
				}
				continue
			}
			// 其他错误，关闭会话
			sess.srv.removeSession(sess.clientAddr)
			return
		}

		// 更新活动时间
		sess.mu.Lock()
		sess.lastActive = time.Now()
		sess.mu.Unlock()

		// 发送回客户端
		_, err = sess.srv.conn.WriteToUDP(buf[:n], sess.clientAddr)
		if err != nil {
			// 写入客户端失败，关闭会话
			sess.srv.removeSession(sess.clientAddr)
			return
		}
	}
}

// serve 启动 UDP 服务循环。
//
// 在单独的 goroutine 中运行，持续监听 UDP 数据报：
// 1. 接收客户端数据报
// 2. 获取或创建会话
// 3. 转发数据到后端
//
// 当服务器停止时返回。
func (s *udpServer) serve() {
	s.running.Store(true)

	buf := make([]byte, 65535)
	for s.running.Load() {
		// 设置读取超时，以便定期检查 stopCh
		_ = s.conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))

		n, clientAddr, err := s.conn.ReadFromUDP(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				// 检查是否需要停止
				select {
				case <-s.stopCh:
					return
				default:
					continue
				}
			}
			continue
		}

		// 获取或创建会话
		session, err := s.getOrCreateSession(clientAddr)
		if err != nil {
			continue
		}

		// 转发数据到后端
		_, err = session.targetConn.Write(buf[:n])
		if err != nil {
			// 写入失败，移除会话
			s.removeSession(clientAddr)
		}
	}
}

// startCleanupTicker 启动定期清理过期会话的 ticker。
//
// 每 10 秒执行一次清理，移除空闲超时的会话。
// 当服务器停止时返回。
func (s *udpServer) startCleanupTicker() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.cleanupExpiredSessions()
		case <-s.stopCh:
			return
		}
	}
}

// cleanupExpiredSessions 清理过期会话。
//
// 遍历所有会话，移除空闲时间超过 timeout 的会话。
// 必须在持有写锁的情况下调用。
func (s *udpServer) cleanupExpiredSessions() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for key, session := range s.sessions {
		session.mu.RLock()
		lastActive := session.lastActive
		session.mu.RUnlock()

		if now.Sub(lastActive) >= s.timeout {
			session.close()
			delete(s.sessions, key)
		}
	}
}

// stop 停止 UDP 服务器。
//
// 设置停止标志，关闭所有会话，等待 goroutine 结束，关闭连接。
func (s *udpServer) stop() {
	s.running.Store(false)
	close(s.stopCh)

	// 关闭所有会话
	s.mu.Lock()
	for _, session := range s.sessions {
		session.close()
	}
	s.sessions = make(map[string]*udpSession)
	s.mu.Unlock()

	// 等待所有 goroutine 结束
	s.wg.Wait()

	// 关闭连接
	_ = s.conn.Close()
}
