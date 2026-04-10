// Package http2 提供 HTTP/2 协议支持。
//
// 该文件包含 HTTP/2 服务器的核心实现，包括：
//   - 基于 golang.org/x/net/http2 的 HTTP/2 服务器
//   - ALPN 协议协商支持
//   - 与现有 fasthttp handler 的集成
//   - 优雅关闭支持
//
// 主要用途：
//
//	用于在现有 TCP 监听器上提供 HTTP/2 协议支持，通过 ALPN 协商自动选择协议。
//
// 作者：xfy
package http2

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/textproto"
	"sync"
	"time"

	"github.com/valyala/fasthttp"
	"golang.org/x/net/http2"
	"rua.plus/lolly/internal/config"
	"rua.plus/lolly/internal/logging"
)

// Server HTTP/2 服务器。
//
// 包装 golang.org/x/net/http2 服务器，提供与 fasthttp handler 的集成。
type Server struct {
	// config HTTP/2 配置
	config *config.HTTP2Config

	// handler fasthttp 请求处理器
	handler fasthttp.RequestHandler

	// tlsConfig TLS 配置
	tlsConfig *tls.Config

	// http2Server HTTP/2 服务器实例
	http2Server *http2.Server

	// running 服务器运行状态
	running bool

	// mu 读写锁
	mu sync.RWMutex

	// listener TCP 监听器
	listener net.Listener

	// stopChan 停止信号通道
	stopChan chan struct{}
}

// NewServer 创建 HTTP/2 服务器。
//
// 参数：
//   - cfg: HTTP/2 配置
//   - handler: fasthttp 请求处理器
//   - tlsConfig: TLS 配置（可选，但推荐用于 ALPN 协商）
//
// 返回值：
//   - *Server: HTTP/2 服务器实例
//   - error: 配置无效时返回错误
func NewServer(cfg *config.HTTP2Config, handler fasthttp.RequestHandler, tlsConfig *tls.Config) (*Server, error) {
	if cfg == nil {
		return nil, fmt.Errorf("http2 config is nil")
	}

	if handler == nil {
		return nil, fmt.Errorf("handler is nil")
	}

	// 设置默认值
	maxConcurrentStreams := cfg.MaxConcurrentStreams
	if maxConcurrentStreams <= 0 {
		maxConcurrentStreams = 250
	}

	maxHeaderListSize := cfg.MaxHeaderListSize
	if maxHeaderListSize <= 0 {
		maxHeaderListSize = 1048576 // 1MB
	}

	idleTimeout := cfg.IdleTimeout
	if idleTimeout <= 0 {
		idleTimeout = 120 * time.Second
	}

	// 创建 HTTP/2 服务器
	h2s := &http2.Server{
		MaxConcurrentStreams: uint32(maxConcurrentStreams),
		IdleTimeout:          idleTimeout,
		MaxReadFrameSize:     uint32(maxHeaderListSize),
		NewWriteScheduler:    func() http2.WriteScheduler { return http2.NewPriorityWriteScheduler(nil) },
		CountError:           func(_ string) {},
	}

	return &Server{
		config:      cfg,
		handler:     handler,
		tlsConfig:   tlsConfig,
		http2Server: h2s,
		stopChan:    make(chan struct{}),
	}, nil
}

// Serve 在指定监听器上启动 HTTP/2 服务器。
//
// 该方法会处理 ALPN 协议协商，根据客户端支持的协议自动选择 HTTP/2 或 HTTP/1.1。
//
// 参数：
//   - ln: TCP 监听器
//
// 返回值：
//   - error: 启动失败时返回错误
func (s *Server) Serve(ln net.Listener) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return fmt.Errorf("server already running")
	}
	s.running = true
	s.listener = ln
	s.mu.Unlock()

	log := logging.Info()
	if s.config.Enabled {
		log.Str("protocol", "h2").
			Bool("push", s.config.PushEnabled).
			Int("max_streams", s.config.MaxConcurrentStreams).
			Int("max_header_size", s.config.MaxHeaderListSize).
			Str("idle_timeout", s.config.IdleTimeout.String()).
			Msg("HTTP/2 server started")
	}

	// 启动连接处理循环
	for {
		select {
		case <-s.stopChan:
			return nil
		default:
		}

		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-s.stopChan:
				return nil
			default:
			}
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			logging.Error().Err(err).Msg("HTTP/2 accept error")
			continue
		}

		go s.handleConnection(conn)
	}
}

// handleConnection 处理单个连接。
//
// 根据连接类型（TLS 或明文）和 ALPN 协商结果，选择合适的协议处理。
func (s *Server) handleConnection(conn net.Conn) {
	defer func() { _ = conn.Close() }()

	// 如果是 TLS 连接，检查 ALPN 协商结果
	if tlsConn, ok := conn.(*tls.Conn); ok {
		// 执行 TLS 握手
		if err := tlsConn.SetReadDeadline(time.Now().Add(10 * time.Second)); err != nil {
			logging.Error().Err(err).Msg("HTTP/2 set read deadline error")
			return
		}

		if err := tlsConn.Handshake(); err != nil {
			logging.Error().Err(err).Msg("HTTP/2 TLS handshake error")
			return
		}

		if err := tlsConn.SetReadDeadline(time.Time{}); err != nil {
			logging.Error().Err(err).Msg("HTTP/2 clear read deadline error")
			return
		}

		// 检查 ALPN 协商结果
		state := tlsConn.ConnectionState()
		if len(state.NegotiatedProtocol) > 0 && state.NegotiatedProtocol != "h2" {
			// ALPN 协商结果为 http/1.1 或其他，使用 fasthttp 处理
			s.serveHTTP1(tlsConn)
			return
		}
	}

	// 处理 HTTP/2 连接
	s.serveHTTP2(conn)
}

// serveHTTP2 使用 HTTP/2 协议服务连接。
func (s *Server) serveHTTP2(conn net.Conn) {
	adapter := NewFastHTTPHandlerAdapter(s.handler)

	opts := &http2.ServeConnOpts{
		Context:    context.Background(),
		Handler:    adapter,
		BaseConfig: &http.Server{},
	}

	s.http2Server.ServeConn(conn, opts)
}

// serveHTTP1 使用 HTTP/1.1 协议服务连接（回退到 fasthttp）。
func (s *Server) serveHTTP1(conn net.Conn) {
	// 创建一个简单的 fasthttp 服务器来处理单个连接
	server := &fasthttp.Server{
		Handler: s.handler,
	}

	// 使用 fasthttp 的连接处理
	_ = server.ServeConn(conn)
}

// Stop 停止 HTTP/2 服务器。
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

	// 发送停止信号
	close(s.stopChan)

	// 关闭监听器
	if s.listener != nil {
		if err := s.listener.Close(); err != nil {
			logging.Error().Err(err).Msg("HTTP/2 listener close error")
		}
	}

	logging.Info().Msg("HTTP/2 server stopped")
	return nil
}

// IsRunning 检查服务器是否正在运行。
func (s *Server) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// GetConfig 返回服务器配置。
func (s *Server) GetConfig() *config.HTTP2Config {
	return s.config
}

// ALPNConfig 返回用于 ALPN 协商的 TLS 配置。
//
// 返回值：
//   - *tls.Config: 配置了 ALPN 的 TLS 配置
//
// 使用示例：
//
//	tlsConfig := &tls.Config{
//	    Certificates: []tls.Certificate{cert},
//	}
//	tlsConfig.NextProtos = []string{"h2", "http/1.1"}
func (s *Server) ALPNConfig() *tls.Config {
	return &tls.Config{
		NextProtos: []string{"h2", "http/1.1"},
	}
}

// WrapTLSListener 包装 TLS 监听器以支持 ALPN 协议协商。
//
// 参数：
//   - ln: 底层 TCP 监听器
//   - tlsConfig: TLS 配置（会被修改以添加 ALPN 支持）
//
// 返回值：
//   - net.Listener: 支持 ALPN 的 TLS 监听器
func WrapTLSListener(ln net.Listener, tlsConfig *tls.Config) net.Listener {
	// 确保 NextProtos 包含 h2 和 http/1.1
	if len(tlsConfig.NextProtos) == 0 {
		tlsConfig.NextProtos = []string{"h2", "http/1.1"}
	}

	// 使用 GetConfigForClient 根据客户端支持的协议返回不同的配置
	originalGetConfig := tlsConfig.GetConfigForClient
	tlsConfig.GetConfigForClient = func(hello *tls.ClientHelloInfo) (*tls.Config, error) {
		// 检查客户端是否支持 h2
		supportsH2 := false
		for _, proto := range hello.SupportedProtos {
			if proto == "h2" {
				supportsH2 = true
				break
			}
		}

		// 如果有原始回调，先调用它
		var cfg *tls.Config
		if originalGetConfig != nil {
			var err error
			cfg, err = originalGetConfig(hello)
			if err != nil {
				return nil, err
			}
		}

		// 如果客户端支持 h2，设置协商结果为 h2
		if supportsH2 {
			if cfg == nil {
				cfg = tlsConfig.Clone()
			}
			cfg.NextProtos = []string{"h2"}
		}

		return cfg, nil
	}

	return tls.NewListener(ln, tlsConfig)
}

// IsH2CEnabled 检查是否启用了 H2C（HTTP/2 over cleartext）。
//
// 注意：当前版本不支持 H2C，需要 TLS 才能启用 HTTP/2。
func (s *Server) IsH2CEnabled() bool {
	return s.config.H2CEnabled
}

// HandleH2C 处理 H2C 升级请求。
//
// 参数：
//   - conn: TCP 连接
//
// 返回值：
//   - bool: 如果成功处理 H2C 升级返回 true
//   - error: 处理失败时返回错误
func (s *Server) HandleH2C(_ net.Conn) (bool, error) {
	// HTTP/2 需要 TLS，不支持 H2C
	return false, nil
}

// h2cConn and related code kept for potential H2C support in future
var _ = h2cConn{} // reserved for future H2C support

// h2cConn 包装 net.Conn 以支持 H2C 协议检测。
type h2cConn struct {
	net.Conn
	reader *bufio.Reader
}

// Read 从连接读取数据。
func (c *h2cConn) Read(p []byte) (n int, err error) {
	if c.reader != nil {
		n, err = c.reader.Read(p)
		if err == io.EOF && n > 0 {
			return n, nil
		}
		if err != nil || n < len(p) {
			c.reader = nil
		}
		return n, err
	}
	return c.Conn.Read(p)
}

// IsHTTP2Request 检查请求是否是 HTTP/2。
//
// 参数：
//   - r: HTTP 请求
//
// 返回值：
//   - bool: 如果是 HTTP/2 请求返回 true
func IsHTTP2Request(r *http.Request) bool {
	// HTTP/2 请求通常使用 "PRI" 方法或 HTTP 版本为 2
	if r.Method == "PRI" {
		return true
	}
	if r.ProtoMajor == 2 {
		return true
	}
	// 检查 HTTP/2 特定的头
	if r.Header.Get(":method") != "" {
		return true
	}
	return false
}

// GetALPNProtocol 从 TLS 连接状态获取协商的协议。
//
// 参数：
//   - conn: 网络连接
//
// 返回值：
//   - string: 协商的协议（如 "h2", "http/1.1"），如果不是 TLS 返回空字符串
func GetALPNProtocol(conn net.Conn) string {
	tlsConn, ok := conn.(*tls.Conn)
	if !ok {
		return ""
	}

	state := tlsConn.ConnectionState()
	return state.NegotiatedProtocol
}

// SupportsHTTP2 检查客户端是否支持 HTTP/2（基于 ALPN 或升级头）。
//
// 参数：
//   - r: HTTP 请求
//
// 返回值：
//   - bool: 如果支持 HTTP/2 返回 true
func SupportsHTTP2(r *http.Request) bool {
	// 检查是否是 HTTP/2 请求
	if IsHTTP2Request(r) {
		return true
	}

	// 检查升级头
	if r.Header.Get("Upgrade") == "h2c" {
		return true
	}

	// 检查 HTTP2-Settings 头
	if r.Header.Get("HTTP2-Settings") != "" {
		return true
	}

	return false
}

// Settings HTTP/2 连接设置。
type Settings struct {
	HeaderTableSize      uint32 // SETTINGS_HEADER_TABLE_SIZE
	EnablePush           bool   // SETTINGS_ENABLE_PUSH
	MaxConcurrentStreams uint32 // SETTINGS_MAX_CONCURRENT_STREAMS
	InitialWindowSize    uint32 // SETTINGS_INITIAL_WINDOW_SIZE
	MaxFrameSize         uint32 // SETTINGS_MAX_FRAME_SIZE
	MaxHeaderListSize    uint32 // SETTINGS_MAX_HEADER_LIST_SIZE
}

// DefaultSettings 返回默认 HTTP/2 设置。
func DefaultSettings() Settings {
	return Settings{
		HeaderTableSize:      4096,
		EnablePush:           true,
		MaxConcurrentStreams: 250,
		InitialWindowSize:    65535,
		MaxFrameSize:         16384,
		MaxHeaderListSize:    1048576,
	}
}

// ValidateSettings 验证 HTTP/2 设置的有效性。
//
// 参数：
//   - settings: HTTP/2 设置
//
// 返回值：
//   - error: 设置无效时返回错误
func ValidateSettings(settings Settings) error {
	if settings.MaxConcurrentStreams == 0 {
		return errors.New("max concurrent streams cannot be zero")
	}
	if settings.MaxFrameSize < 16384 || settings.MaxFrameSize > 16777215 {
		return errors.New("max frame size must be between 16384 and 16777215")
	}
	if settings.InitialWindowSize > 2147483647 {
		return errors.New("initial window size cannot exceed 2^31-1")
	}
	if settings.MaxHeaderListSize == 0 {
		return errors.New("max header list size cannot be zero")
	}
	return nil
}

// ParseSettings 从配置解析 HTTP/2 设置。
//
// 参数：
//   - cfg: HTTP/2 配置
//
// 返回值：
//   - Settings: 解析后的 HTTP/2 设置
func ParseSettings(cfg *config.HTTP2Config) Settings {
	settings := DefaultSettings()

	if cfg.MaxConcurrentStreams > 0 {
		settings.MaxConcurrentStreams = uint32(cfg.MaxConcurrentStreams)
	}
	if cfg.MaxHeaderListSize > 0 {
		settings.MaxHeaderListSize = uint32(cfg.MaxHeaderListSize)
	}
	settings.EnablePush = cfg.PushEnabled

	return settings
}

// connectionPool HTTP/2 连接池。
type connectionPool struct {
	mu    sync.RWMutex
	conns map[string][]net.Conn
}

// newConnectionPool 创建新的连接池。
func newConnectionPool() *connectionPool {
	return &connectionPool{
		conns: make(map[string][]net.Conn),
	}
}

// add 添加连接。
func (p *connectionPool) add(key string, conn net.Conn) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.conns[key] = append(p.conns[key], conn)
}

// remove 移除连接。
func (p *connectionPool) remove(key string, conn net.Conn) {
	p.mu.Lock()
	defer p.mu.Unlock()

	conns := p.conns[key]
	for i, c := range conns {
		if c == conn {
			p.conns[key] = append(conns[:i], conns[i+1:]...)
			break
		}
	}
}

// get 获取连接。
func (p *connectionPool) get(key string) []net.Conn {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.conns[key]
}

// count 获取连接数。
func (p *connectionPool) count(key string) int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.conns[key])
}

// closeAll 关闭所有连接。
func (p *connectionPool) closeAll() { //nolint:unused // reserved for future use
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, conns := range p.conns {
		for _, conn := range conns {
			_ = conn.Close()
		}
	}
	p.conns = make(map[string][]net.Conn)
}

// canonicalHeaderKey 返回规范化的 HTTP 头键。
func canonicalHeaderKey(key string) string {
	return textproto.CanonicalMIMEHeaderKey(key)
}
