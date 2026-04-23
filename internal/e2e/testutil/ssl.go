//go:build e2e

// Package testutil 提供 E2E 测试的工具函数。
//
// 包含 SSL/TLS 测试辅助函数。
//
// 作者：xfy
package testutil

import (
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"os"
	"time"
)

// CreateTLSClient 创建信任指定证书的 HTTPS 客户端。
//
// 参数：
//   - certPath: CA 证书文件路径
//
// 返回配置好的 HTTP 客户端，信任指定的证书。
func CreateTLSClient(certPath string) (*http.Client, error) {
	caCert, err := os.ReadFile(certPath)
	if err != nil {
		return nil, err
	}

	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)

	return &http.Client{
		Timeout: DefaultClientTimeout,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs: caCertPool,
			},
		},
	}, nil
}

// CreateTLSClientWithVersion 创建带版本限制的 HTTPS 客户端。
//
// 参数：
//   - certPath: CA 证书文件路径
//   - minVersion: 最小 TLS 版本
//   - maxVersion: 最大 TLS 版本
func CreateTLSClientWithVersion(certPath string, minVersion, maxVersion uint16) (*http.Client, error) {
	caCert, err := os.ReadFile(certPath)
	if err != nil {
		return nil, err
	}

	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)

	return &http.Client{
		Timeout: DefaultClientTimeout,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs:    caCertPool,
				MinVersion: minVersion,
				MaxVersion: maxVersion,
			},
		},
	}, nil
}

// CreateInsecureTLSClient 创建跳过证书验证的 HTTPS 客户端。
//
// 用于测试自签名证书场景，不应在生产环境使用。
func CreateInsecureTLSClient() *http.Client {
	return &http.Client{
		Timeout: DefaultClientTimeout,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}
}

// CreateDefaultHTTPClient 创建默认 HTTP 客户端。
//
// 用于非 SSL 测试场景。
func CreateDefaultHTTPClient() *http.Client {
	return &http.Client{
		Timeout: DefaultClientTimeout,
	}
}

// CreateHTTPClientWithTimeout 创建带自定义超时的 HTTP 客户端。
func CreateHTTPClientWithTimeout(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
	}
}