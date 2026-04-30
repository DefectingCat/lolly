// Package stream 提供 TCP/UDP Stream 代理功能。
//
// 该文件实现 Stream 模块的 SSL/TLS 支持，包括：
//   - 服务端 TLS 终端
//   - 客户端 TLS 连接（上游 SSL）
//   - 证书加载和配置
//   - mTLS 客户端证书验证
//
// 作者：xfy
package stream

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"sync"

	"rua.plus/lolly/internal/config"
	"rua.plus/lolly/internal/logging"
	"rua.plus/lolly/internal/sslutil"
)

// SSLManager 管理 Stream SSL/TLS 配置。
//
// 负责加载证书、配置 TLS 连接，支持服务端和客户端两种模式。
type SSLManager struct {
	cert         tls.Certificate
	clientCAPool *x509.CertPool
	config       config.StreamSSLConfig
	mu           sync.RWMutex
}

// ProxySSLManager 管理上游 SSL 连接。
//
// 负责创建到上游服务器的 TLS 连接，支持证书验证和客户端证书。
type ProxySSLManager struct {
	cert       tls.Certificate
	rootCAPool *x509.CertPool
	config     config.StreamProxySSLConfig
	mu         sync.RWMutex
}

// NewSSLManager 创建 Stream SSL 管理器。
//
// 参数：
//   - cfg: SSL 配置
//
// 返回值：
//   - *SSLManager: SSL 管理器实例
//   - error: 证书加载失败时返回错误
func NewSSLManager(cfg config.StreamSSLConfig) (*SSLManager, error) {
	if !cfg.Enabled {
		return &SSLManager{config: cfg}, nil
	}

	// 加载服务器证书
	cert, err := tls.LoadX509KeyPair(cfg.Cert, cfg.Key)
	if err != nil {
		return nil, fmt.Errorf("failed to load server certificate: %w", err)
	}

	mgr := &SSLManager{
		config: cfg,
		cert:   cert,
	}

	// 加载客户端 CA 证书（mTLS）
	if cfg.ClientCA != "" {
		pool, err := sslutil.LoadCertPool(cfg.ClientCA, "client CA")
		if err != nil {
			return nil, fmt.Errorf("failed to load client CA: %w", err)
		}
		mgr.clientCAPool = pool
	}

	return mgr, nil
}

// NewProxySSLManager 创建上游 SSL 管理器。
//
// 参数：
//   - cfg: 代理 SSL 配置
//
// 返回值：
//   - *ProxySSLManager: 代理 SSL 管理器实例
//   - error: 证书加载失败时返回错误
func NewProxySSLManager(cfg config.StreamProxySSLConfig) (*ProxySSLManager, error) {
	if !cfg.Enabled {
		return &ProxySSLManager{config: cfg}, nil
	}

	mgr := &ProxySSLManager{config: cfg}

	// 加载客户端证书（mTLS）
	if cfg.Cert != "" && cfg.Key != "" {
		cert, err := tls.LoadX509KeyPair(cfg.Cert, cfg.Key)
		if err != nil {
			return nil, fmt.Errorf("failed to load client certificate: %w", err)
		}
		mgr.cert = cert
	}

	// 加载信任的 CA 证书
	if cfg.TrustedCA != "" {
		pool, err := sslutil.LoadCertPool(cfg.TrustedCA, "trusted CA")
		if err != nil {
			return nil, fmt.Errorf("failed to load trusted CA: %w", err)
		}
		mgr.rootCAPool = pool
	}

	return mgr, nil
}

// GetTLSConfig 获取服务端 TLS 配置。
//
// 返回值：
//   - *tls.Config: TLS 配置对象
func (m *SSLManager) GetTLSConfig() *tls.Config {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.config.Enabled {
		return nil
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{m.cert},
		MinVersion:   tls.VersionTLS12,
	}

	// 设置协议版本
	if len(m.config.Protocols) > 0 {
		tlsConfig.MinVersion = sslutil.ParseMinTLSVersion(m.config.Protocols)
	}

	// 设置加密套件
	if len(m.config.Ciphers) > 0 {
		tlsConfig.CipherSuites = sslutil.ParseCipherSuitesLenient(m.config.Ciphers)
	}

	// 配置客户端证书验证（mTLS）
	if m.clientCAPool != nil {
		tlsConfig.ClientCAs = m.clientCAPool
		tlsConfig.ClientAuth = tls.VerifyClientCertIfGiven
	}

	return tlsConfig
}

// GetClientTLSConfig 获取客户端 TLS 配置。
//
// 用于连接上游服务器。
//
// 参数：
//   - serverName: 服务器名称（用于 SNI）
//
// 返回值：
//   - *tls.Config: TLS 配置对象
func (m *ProxySSLManager) GetClientTLSConfig(serverName string) *tls.Config {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.config.Enabled {
		return nil
	}

	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}

	// 设置服务器名称（SNI）
	if m.config.ServerName != "" {
		tlsConfig.ServerName = m.config.ServerName
	} else if serverName != "" {
		tlsConfig.ServerName = serverName
	}

	// 设置客户端证书
	if m.cert.Certificate != nil {
		tlsConfig.Certificates = []tls.Certificate{m.cert}
	}

	// 设置协议版本
	if len(m.config.Protocols) > 0 {
		tlsConfig.MinVersion = sslutil.ParseMinTLSVersion(m.config.Protocols)
	}

	// 配置服务器证书验证
	if m.config.Verify && m.rootCAPool != nil {
		tlsConfig.RootCAs = m.rootCAPool
	} else if !m.config.Verify {
		// 跳过证书验证
		logging.Warn().Msg("SSL证书验证已禁用，连接可能遭受中间人攻击")
		tlsConfig.InsecureSkipVerify = true
	}

	// 会话复用
	if m.config.SessionReuse {
		tlsConfig.ClientSessionCache = tls.NewLRUClientSessionCache(100)
	}

	return tlsConfig
}

// IsEnabled 检查是否启用 SSL。
func (m *SSLManager) IsEnabled() bool {
	return m.config.Enabled
}

// IsEnabled 检查是否启用代理 SSL。
func (m *ProxySSLManager) IsEnabled() bool {
	return m.config.Enabled
}
