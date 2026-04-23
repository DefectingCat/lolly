//go:build e2e

// Package testutil 提供 E2E 测试的工具函数。
package testutil

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"
)

// GenerateSelfSignedCert 生成自签名证书和私钥。
//
// 返回证书路径、私钥路径、清理函数和错误。
// 清理函数会删除生成的文件。
func GenerateSelfSignedCert(tmpDir string) (certPath, keyPath string, cleanup func(), err error) {
	// 生成 ECDSA 私钥
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", nil, fmt.Errorf("failed to generate private key: %w", err)
	}

	// 生成序列号
	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return "", "", nil, fmt.Errorf("failed to generate serial number: %w", err)
	}

	// 创建证书模板
	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Lolly E2E Test"},
			CommonName:   "localhost",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost"},
		IPAddresses:           nil,
	}

	// 生成证书
	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return "", "", nil, fmt.Errorf("failed to create certificate: %w", err)
	}

	// 确保目录存在
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		return "", "", nil, fmt.Errorf("failed to create temp dir: %w", err)
	}

	// 写入证书文件
	certPath = filepath.Join(tmpDir, "cert.pem")
	certFile, err := os.Create(certPath)
	if err != nil {
		return "", "", nil, fmt.Errorf("failed to create cert file: %w", err)
	}
	if err := pem.Encode(certFile, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes}); err != nil {
		certFile.Close()
		return "", "", nil, fmt.Errorf("failed to write cert: %w", err)
	}
	certFile.Close()

	// 写入私钥文件
	keyPath = filepath.Join(tmpDir, "key.pem")
	keyFile, err := os.Create(keyPath)
	if err != nil {
		return "", "", nil, fmt.Errorf("failed to create key file: %w", err)
	}
	privBytes, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		keyFile.Close()
		return "", "", nil, fmt.Errorf("failed to marshal private key: %w", err)
	}
	if err := pem.Encode(keyFile, &pem.Block{Type: "EC PRIVATE KEY", Bytes: privBytes}); err != nil {
		keyFile.Close()
		return "", "", nil, fmt.Errorf("failed to write key: %w", err)
	}
	keyFile.Close()

	// 清理函数
	cleanup = func() {
		os.Remove(certPath)
		os.Remove(keyPath)
	}

	return certPath, keyPath, cleanup, nil
}

// GenerateCertPool 从证书文件创建 x509.CertPool。
func GenerateCertPool(certPath string) (*x509.CertPool, error) {
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read cert file: %w", err)
	}

	certPool := x509.NewCertPool()
	if !certPool.AppendCertsFromPEM(certPEM) {
		return nil, fmt.Errorf("failed to append cert to pool")
	}

	return certPool, nil
}

// GenerateTLSConfig 生成客户端 TLS 配置，信任指定的证书。
func GenerateTLSConfig(certPath string) (*TLSConfig, error) {
	certPool, err := GenerateCertPool(certPath)
	if err != nil {
		return nil, err
	}

	return &TLSConfig{
		RootCAs: certPool,
	}, nil
}

// TLSConfig 简化的 TLS 配置。
type TLSConfig struct {
	RootCAs *x509.CertPool
}
