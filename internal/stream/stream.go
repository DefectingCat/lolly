// Package stream 提供 TCP/UDP Stream 代理功能，支持 MySQL、DNS 等服务代理。
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
	listeners map[string]net.Listener
	upstreams map[string]*Upstream
	connCount int64 // 当前连接数
	mu        sync.RWMutex
	running   atomic.Bool
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
		listeners: make(map[string]net.Listener),
		upstreams: make(map[string]*Upstream),
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
func (s *Server) ListenUDP(addr string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return err
	}

	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return err
	}

	// UDP 用 UDPConn 而非 Listener，需要特殊处理
	s.listeners[addr] = &udpListener{conn: conn}
	return nil
}

// Start 启动 Stream 服务器。
func (s *Server) Start() error {
	s.running.Store(true)

	s.mu.RLock()
	defer s.mu.RUnlock()

	for addr, listener := range s.listeners {
		go s.acceptLoop(addr, listener)
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

	// 关闭所有监听器
	for _, listener := range s.listeners {
		listener.Close()
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
	return Stats{
		Connections: s.connCount,
		Listeners:   len(s.listeners),
		Upstreams:   len(s.upstreams),
	}
}

// Stats Stream 服务器统计。
type Stats struct {
	Connections int64
	Listeners   int
	Upstreams   int
}

// udpListener UDP 监听器包装。
type udpListener struct {
	conn *net.UDPConn
}

// Accept UDP 不支持 Accept，返回错误。
func (u *udpListener) Accept() (net.Conn, error) {
	return nil, io.EOF
}

// Close 关闭 UDP 连接。
func (u *udpListener) Close() error {
	return u.conn.Close()
}

// Addr 返回本地地址。
func (u *udpListener) Addr() net.Addr {
	return u.conn.LocalAddr()
}
