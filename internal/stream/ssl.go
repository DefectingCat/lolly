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
	"sync"

	"rua.plus/lolly/internal/config"
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


