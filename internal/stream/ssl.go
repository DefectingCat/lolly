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
	"os"
	"sync"

	"rua.plus/lolly/internal/config"
)

// SSLManager 管理 Stream SSL/TLS 配置。
//
// 负责加载证书、配置 TLS 连接，支持服务端和客户端两种模式。
type SSLManager struct {
	// config SSL 配置
	config config.StreamSSLConfig

	// cert 服务器证书
	cert tls.Certificate

	// clientCAPool 客户端 CA 证书池（mTLS）
	clientCAPool *x509.CertPool

	// mu 保护并发访问
	mu sync.RWMutex
}

// ProxySSLManager 管理上游 SSL 连接。
//
// 负责创建到上游服务器的 TLS 连接，支持证书验证和客户端证书。
type ProxySSLManager struct {
	// config 代理 SSL 配置
	config config.StreamProxySSLConfig

	// cert 客户端证书
	cert tls.Certificate

	// rootCAPool 根 CA 证书池
	rootCAPool *x509.CertPool

	// mu 保护并发访问
	mu sync.RWMutex
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
		pool, err := loadCertPool(cfg.ClientCA)
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
		pool, err := loadCertPool(cfg.TrustedCA)
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
		tlsConfig.MinVersion = parseMinTLSVersion(m.config.Protocols)
	}

	// 设置加密套件
	if len(m.config.Ciphers) > 0 {
		tlsConfig.CipherSuites = parseCipherSuites(m.config.Ciphers)
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
		tlsConfig.MinVersion = parseMinTLSVersion(m.config.Protocols)
	}

	// 配置服务器证书验证
	if m.config.Verify && m.rootCAPool != nil {
		tlsConfig.RootCAs = m.rootCAPool
	} else if !m.config.Verify {
		// 跳过证书验证
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

// loadCertPool 从文件加载证书池。
//
// 参数：
//   - certFile: 证书文件路径
//
// 返回值：
//   - *x509.CertPool: 证书池
//   - error: 加载失败时返回错误
func loadCertPool(certFile string) (*x509.CertPool, error) {
	data, err := os.ReadFile(certFile)
	if err != nil {
		return nil, err
	}

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(data) {
		return nil, fmt.Errorf("failed to parse certificates from %s", certFile)
	}

	return pool, nil
}

// parseMinTLSVersion 解析最小 TLS 版本。
//
// 参数：
//   - protocols: 协议版本列表
//
// 返回值：
//   - uint16: TLS 版本常量
func parseMinTLSVersion(protocols []string) uint16 {
	for _, p := range protocols {
		switch p {
		case "TLSv1.3":
			return tls.VersionTLS13
		case "TLSv1.2":
			return tls.VersionTLS12
		}
	}
	return tls.VersionTLS12
}

// parseCipherSuites 解析加密套件列表。
//
// 参数：
//   - ciphers: 加密套件名称列表
//
// 返回值：
//   - []uint16: 加密套件 ID 列表
func parseCipherSuites(ciphers []string) []uint16 {
	var suites []uint16
	for _, c := range ciphers {
		if id, ok := cipherNameToID[c]; ok {
			suites = append(suites, id)
		}
	}
	if len(suites) == 0 {
		return nil // 使用默认值
	}
	return suites
}

// cipherNameToID 加密套件名称到 ID 的映射
var cipherNameToID = map[string]uint16{
	"ECDHE-RSA-AES128-GCM-SHA256":   tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
	"ECDHE-RSA-AES256-GCM-SHA384":   tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
	"ECDHE-ECDSA-AES128-GCM-SHA256": tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
	"ECDHE-ECDSA-AES256-GCM-SHA384": tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
	"ECDHE-RSA-CHACHA20-POLY1305":   tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
	"ECDHE-ECDSA-CHACHA20-POLY1305": tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
	"AES128-GCM-SHA256":             tls.TLS_AES_128_GCM_SHA256,
	"AES256-GCM-SHA384":             tls.TLS_AES_256_GCM_SHA384,
	"CHACHA20-POLY1305":             tls.TLS_CHACHA20_POLY1305_SHA256,
	"ECDHE-RSA-AES128-CBC-SHA":      tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
	"ECDHE-RSA-AES256-CBC-SHA":      tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
	"ECDHE-ECDSA-AES128-CBC-SHA":    tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA,
	"ECDHE-ECDSA-AES256-CBC-SHA":    tls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA,
	"RSA-AES128-GCM-SHA256":         tls.TLS_RSA_WITH_AES_128_GCM_SHA256,
	"RSA-AES256-GCM-SHA384":         tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
	"RSA-AES128-CBC-SHA":            tls.TLS_RSA_WITH_AES_128_CBC_SHA,
	"RSA-AES256-CBC-SHA":            tls.TLS_RSA_WITH_AES_256_CBC_SHA,
	"ECDHE-RSA-3DES-EDE-CBC-SHA":    tls.TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA,
	"RSA-3DES-EDE-CBC-SHA":          tls.TLS_RSA_WITH_3DES_EDE_CBC_SHA,
}
