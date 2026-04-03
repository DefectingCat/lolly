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
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// Balancer 负载均衡器接口（stream 专用）。
type Balancer interface {
	Select(targets []*Target) *Target
}

// roundRobin 简单轮询。
type roundRobin struct {
	counter uint64
}

// newRoundRobin 创建轮询均衡器。
func newRoundRobin() Balancer {
	return &roundRobin{}
}

// Select 选择下一个目标。
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

// leastConn 最少连接。
type leastConn struct{}

// newLeastConn 创建最少连接均衡器。
func newLeastConn() Balancer {
	return &leastConn{}
}

// Select 选择连接最少的目标。
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

// Server TCP/UDP Stream 代理服务器。
type Server struct {
	listeners  map[string]net.Listener
	udpServers map[string]*udpServer
	upstreams  map[string]*Upstream
	connCount  int64 // 当前连接数
	mu         sync.RWMutex
	running    atomic.Bool
}

// Upstream Stream 上游配置。
type Upstream struct {
	name      string
	targets   []*Target
	balancer  Balancer
	healthChk *HealthChecker
	mu        sync.RWMutex
}

// Target Stream 目标服务器。
type Target struct {
	addr    string
	weight  int
	healthy atomic.Bool
	conns   int64 // 当前连接数
}

// HealthChecker Stream 健康检查器。
type HealthChecker struct {
	upstream *Upstream
	interval time.Duration
	timeout  time.Duration
	stopCh   chan struct{}
}

// Config Stream 配置。
type Config struct {
	Listen   string       // 监听地址
	Protocol string       // tcp 或 udp
	Upstream UpstreamSpec // 上游配置
}

// UpstreamSpec 上游配置规格。
type UpstreamSpec struct {
	Name        string
	Targets     []TargetSpec
	LoadBalance string
	HealthCheck HealthCheckSpec
}

// TargetSpec 目标配置规格。
type TargetSpec struct {
	Addr   string
	Weight int
}

// HealthCheckSpec 健康检查配置规格。
type HealthCheckSpec struct {
	Interval time.Duration
	Timeout  time.Duration
	Enabled  bool
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
	case "round_robin":
		balancer = newRoundRobin()
	case "least_conn":
		balancer = newLeastConn()
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
func (s *Server) handleConnection(clientConn net.Conn, addr string) {
	defer func() {
		clientConn.Close()
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
	defer targetConn.Close()

	// 双向数据转发
	go io.Copy(targetConn, clientConn)
	io.Copy(clientConn, targetConn)
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
func (h *HealthChecker) check() {
	for _, target := range h.upstream.targets {
		conn, err := net.DialTimeout("tcp", target.addr, h.timeout)
		if err != nil {
			target.healthy.Store(false)
		} else {
			conn.Close()
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
		listener.Close()
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
	Connections int64
	Listeners   int
	Upstreams   int
}

// udpSession UDP 会话，管理客户端到后端的映射
type udpSession struct {
	clientAddr *net.UDPAddr
	targetConn net.Conn
	lastActive time.Time
	mu         sync.RWMutex
	srv        *udpServer
	closeOnce  sync.Once
}

// udpServer UDP 服务器，管理多个客户端会话
type udpServer struct {
	conn     *net.UDPConn
	sessions map[string]*udpSession
	mu       sync.RWMutex
	running  atomic.Bool
	upstream *Upstream
	timeout  time.Duration
	stopCh   chan struct{}
	wg       sync.WaitGroup
}

// newUDPServer 创建新的 UDP 服务器
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

// sessionKey 从 UDP 地址生成会话键
func sessionKey(addr *net.UDPAddr) string {
	return addr.String()
}

// getSession 获取现有会话（线程安全）
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

// getOrCreateSession 获取或创建会话（线程安全）
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
	if session, exists := s.sessions[sessionKey(clientAddr)]; exists {
		session.mu.Lock()
		session.lastActive = time.Now()
		session.mu.Unlock()
		return session, nil
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

// removeSession 移除会话（线程安全）
func (s *udpServer) removeSession(clientAddr *net.UDPAddr) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := sessionKey(clientAddr)
	if session, exists := s.sessions[key]; exists {
		session.close()
		delete(s.sessions, key)
	}
}

// close 关闭会话
func (sess *udpSession) close() {
	sess.closeOnce.Do(func() {
		if sess.targetConn != nil {
			sess.targetConn.Close()
		}
	})
}

// handleBackendResponse 处理后端响应并转发回客户端
func (sess *udpSession) handleBackendResponse() {
	defer sess.srv.wg.Done()

	buf := make([]byte, 65535)
	for {
		// 设置读取超时
		sess.targetConn.SetReadDeadline(time.Now().Add(sess.srv.timeout))

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

// serve 启动 UDP 服务循环
func (s *udpServer) serve() {
	s.running.Store(true)

	buf := make([]byte, 65535)
	for s.running.Load() {
		// 设置读取超时，以便定期检查 stopCh
		s.conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))

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

// startCleanupTicker 启动定期清理过期会话的 ticker
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

// cleanupExpiredSessions 清理过期会话
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

// stop 停止 UDP 服务器
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
	s.conn.Close()
}
