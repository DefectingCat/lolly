// Package proxy 反向代理包，为 Lolly HTTP 服务器提供反向代理功能。
//
// 该文件提供上游 SSL/TLS 配置支持，包括自定义 CA 证书、
// 客户端证书（mTLS）、SNI 和 TLS 版本控制。
package proxy

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"os"
	"strings"

	"rua.plus/lolly/internal/config"
)

// TLS 版本字符串到 tls 常量的映射。
// 支持 TLSv1.0, TLSv1.1, TLSv1.2, TLSv1.3 格式（大小写不敏感）
var tlsVersionMap = map[string]uint16{
	"TLSV1.0": tls.VersionTLS10,
	"TLSV1.1": tls.VersionTLS11,
	"TLSV1.2": tls.VersionTLS12,
	"TLSV1.3": tls.VersionTLS13,
	"":        0, // 空字符串表示使用默认
}

// CreateTLSConfig 从 ProxySSLConfig 创建 tls.Config。
//
// 参数：
//   - cfg: 上游 SSL 配置
//   - defaultServerName: 默认 SNI 名称（从目标 URL 提取）
//
// 返回值：
//   - *tls.Config: TLS 配置对象
//   - error: 配置错误（证书加载失败等）
//
// 注意事项：
//   - cfg 为 nil 或 Enabled=false 时返回 nil
//   - TrustedCA 加载失败时返回错误
//   - ClientCert/ClientKey 加载失败时返回错误
func CreateTLSConfig(cfg *config.ProxySSLConfig, defaultServerName string) (*tls.Config, error) {
	if cfg == nil || !cfg.Enabled {
		return nil, nil
	}

	tlsCfg := &tls.Config{}

	// SNI 配置
	if cfg.ServerName != "" {
		tlsCfg.ServerName = cfg.ServerName
	} else if defaultServerName != "" {
		tlsCfg.ServerName = defaultServerName
	}

	// 跳过证书验证（仅测试环境）
	if cfg.InsecureSkipVerify {
		tlsCfg.InsecureSkipVerify = true
	}

	// CA 证书验证
	if cfg.TrustedCA != "" {
		caData, err := os.ReadFile(cfg.TrustedCA)
		if err != nil {
			return nil, errors.New("failed to read CA certificate: " + err.Error())
		}

		caPool := x509.NewCertPool()
		if !caPool.AppendCertsFromPEM(caData) {
			return nil, errors.New("failed to parse CA certificate")
		}
		tlsCfg.RootCAs = caPool
	}

	// 客户端证书（mTLS）
	if cfg.ClientCert != "" && cfg.ClientKey != "" {
		cert, err := tls.LoadX509KeyPair(cfg.ClientCert, cfg.ClientKey)
		if err != nil {
			return nil, errors.New("failed to load client certificate: " + err.Error())
		}
		tlsCfg.Certificates = []tls.Certificate{cert}
	}

	// TLS 版本配置
	if cfg.MinVersion != "" {
		version, ok := tlsVersionMap[strings.ToUpper(cfg.MinVersion)]
		if !ok {
			return nil, errors.New("invalid TLS min version: " + cfg.MinVersion)
		}
		tlsCfg.MinVersion = version
	}

	if cfg.MaxVersion != "" {
		version, ok := tlsVersionMap[strings.ToUpper(cfg.MaxVersion)]
		if !ok {
			return nil, errors.New("invalid TLS max version: " + cfg.MaxVersion)
		}
		tlsCfg.MaxVersion = version
	}

	return tlsCfg, nil
}
