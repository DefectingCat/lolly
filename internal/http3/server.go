// Package http3 提供 HTTP/3 (QUIC) 协议支持。
//
// 该文件包含 HTTP/3 服务器的核心实现，包括：
//   - 基于 quic-go 的 HTTP/3 服务器
//   - 支持 0-RTT 连接
//   - 优雅关闭支持
//   - 与现有 fasthttp handler 的集成
//
// 主要用途：
//
//	用于提供 HTTP/3 协议支持，提升网站性能和用户体验。
//
// 作者：xfy
package http3

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/quic-go/quic-go"
	quichttp3 "github.com/quic-go/quic-go/http3"
	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/config"
	"rua.plus/lolly/internal/logging"
)

// Server HTTP/3 服务器。
//
// 使用 QUIC 协议提供 HTTP/3 服务，与现有的 TCP 服务器并行运行。
type Server struct {
	// config HTTP/3 配置
	config *config.HTTP3Config

	// http3Server HTTP/3 服务器实例
	http3Server *quichttp3.Server

	// handler fasthttp 请求处理器
	handler fasthttp.RequestHandler

	// adapter 请求适配器
	adapter *Adapter

	// tlsConfig TLS 配置
	tlsConfig *tls.Config

	// listener QUIC 监听器
	listener *quic.EarlyListener

	// running 服务器运行状态
	running bool

	// mu 读写锁
	mu sync.RWMutex
}

// NewServer 创建 HTTP/3 服务器。
//
// 参数：
//   - cfg: HTTP/3 配置
//   - handler: fasthttp 请求处理器
//   - tlsConfig: TLS 配置（必须）
//
// 返回值：
//   - *Server: HTTP/3 服务器实例
//   - error: 配置无效时返回错误
func NewServer(cfg *config.HTTP3Config, handler fasthttp.RequestHandler, tlsConfig *tls.Config) (*Server, error) {
	if cfg == nil {
		return nil, fmt.Errorf("http3 config is nil")
	}

	if handler == nil {
		return nil, fmt.Errorf("handler is nil")
	}

	if tlsConfig == nil {
		return nil, fmt.Errorf("tls config is required for HTTP/3")
	}

	adapter := NewAdapter()

	return &Server{
		config:    cfg,
		handler:   handler,
		adapter:   adapter,
		tlsConfig: tlsConfig,
	}, nil
}

// Start 启动 HTTP/3 服务器。
//
// 创建 UDP 监听器并开始接受 QUIC 连接。
//
// 返回值：
//   - error: 启动失败时返回错误
func (s *Server) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return fmt.Errorf("server already running")
	}

	// 创建 QUIC 配置
	quicConfig := &quic.Config{
		MaxIncomingStreams: int64(s.config.MaxStreams),
		MaxIdleTimeout:     s.config.IdleTimeout,
		KeepAlivePeriod:    30 * time.Second,
	}

	// 设置默认值
	if quicConfig.MaxIncomingStreams == 0 {
		quicConfig.MaxIncomingStreams = 100
	}
	if quicConfig.MaxIdleTimeout == 0 {
		quicConfig.MaxIdleTimeout = 30 * time.Second
	}

	// 创建 UDP 监听器
	listenAddr := s.config.Listen
	if listenAddr == "" {
		listenAddr = ":443"
	}

	udpAddr, err := net.ResolveUDPAddr("udp", listenAddr)
	if err != nil {
		return fmt.Errorf("failed to resolve UDP address: %w", err)
	}

	udpConn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return fmt.Errorf("failed to listen UDP: %w", err)
	}

	// 创建 QUIC 监听器
	s.listener, err = quic.ListenEarly(udpConn, s.tlsConfig, quicConfig)
	if err != nil {
		_ = udpConn.Close()
		return fmt.Errorf("failed to listen QUIC: %w", err)
	}

	// 创建 HTTP/3 服务器
	s.http3Server = &quichttp3.Server{
		Handler: s.adapter.Wrap(s.handler),
	}

	s.running = true

	logging.Info().
		Str("listen", listenAddr).
		Bool("0rtt", s.config.Enable0RTT).
		Msg("HTTP/3 server started")

	// 开始服务
	go func() {
		if err := s.http3Server.ServeListener(s.listener); err != nil {
			s.mu.RLock()
			running := s.running
			s.mu.RUnlock()

			if running {
				logging.Error().Err(err).Msg("HTTP/3 server error")
			}
		}
	}()

	return nil
}

// Stop 停止 HTTP/3 服务器。
//
// 优雅关闭服务器，等待现有连接完成。
//
// 返回值：
//   - error: 关闭失败时返回错误
func (s *Server) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return nil
	}

	s.running = false

	if s.http3Server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := s.http3Server.Close(); err != nil {
			logging.Error().Err(err).Msg("HTTP/3 server close error")
		}

		// 等待服务完全停止
		<-ctx.Done()
	}

	logging.Info().Msg("HTTP/3 server stopped")
	return nil
}

// GracefulStop 优雅停止服务器。
//
// 等待指定时间让现有连接完成。
//
// 参数：
//   - timeout: 等待超时时间
func (s *Server) GracefulStop(timeout time.Duration) error {
	s.mu.Lock()
	s.running = false
	s.mu.Unlock()

	if s.http3Server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		done := make(chan struct{})
		go func() {
			_ = s.http3Server.Close()
			close(done)
		}()

		select {
		case <-done:
			logging.Info().Msg("HTTP/3 server graceful stop completed")
		case <-ctx.Done():
			logging.Warn().Msg("HTTP/3 server graceful stop timeout")
		}
	}

	return nil
}

// IsRunning 检查服务器是否正在运行。
func (s *Server) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// GetAltSvcHeader 返回 Alt-Svc 响应头值。
//
// 用于告知客户端可以使用 HTTP/3。
//
// 返回值：
//   - string: Alt-Svc 头值，如 `h3=":443"; ma=86400`
func (s *Server) GetAltSvcHeader() string {
	if s.config == nil || !s.config.Enabled {
		return ""
	}

	listen := s.config.Listen
	if listen == "" {
		listen = ":443"
	}

	// 移除前导冒号，保留端口
	port := listen
	if port[0] == ':' {
		port = port[1:]
	}

	return fmt.Sprintf(`h3=":%s"; ma=86400`, port)
}

// Stats 返回服务器统计信息。
type Stats struct {
	Running    bool   // 是否运行中
	Listen     string // 监听地址
	Enable0RTT bool   // 是否启用 0-RTT
	MaxStreams int    // 最大并发流
}

// GetStats 返回服务器统计信息。
func (s *Server) GetStats() Stats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return Stats{
		Running:    s.running,
		Listen:     s.config.Listen,
		Enable0RTT: s.config.Enable0RTT,
		MaxStreams: s.config.MaxStreams,
	}
}
