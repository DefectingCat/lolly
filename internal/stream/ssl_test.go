// Package stream 提供 TCP/UDP Stream 代理功能。
//
// 该文件包含 Stream SSL/TLS 支持的单元测试。
//
// 作者：xfy
package stream

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"rua.plus/lolly/internal/sslutil"
)

// generateTestCertificate 生成测试用的自签名证书
func generateTestCertificate(t *testing.T, certFile, keyFile string) {
	t.Helper()

	// 创建证书模板
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost"},
	}

	// 生成私钥和证书
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("Failed to create certificate: %v", err)
	}

	// 写入证书文件
	certOut, err := os.Create(certFile)
	if err != nil {
		t.Fatalf("Failed to create cert file: %v", err)
	}
	defer func() { _ = certOut.Close() }()
	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: certDER}); err != nil {
		t.Fatalf("Failed to encode certificate: %v", err)
	}

	// 写入私钥文件
	keyOut, err := os.Create(keyFile)
	if err != nil {
		t.Fatalf("Failed to create key file: %v", err)
	}
	defer func() { _ = keyOut.Close() }()
	if err := pem.Encode(keyOut, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}); err != nil {
		t.Fatalf("Failed to encode key: %v", err)
	}
}













func TestParseMinTLSVersion(t *testing.T) {
	tests := []struct {
		protocols   []string
		wantVersion uint16
	}{
		{[]string{"TLSv1.3"}, tls.VersionTLS13},
		{[]string{"TLSv1.2"}, tls.VersionTLS12},
		{[]string{"TLSv1.2", "TLSv1.3"}, tls.VersionTLS12},
		{[]string{}, tls.VersionTLS12},
		{[]string{"Unknown"}, tls.VersionTLS12},
	}

	for _, tt := range tests {
		got := sslutil.ParseMinTLSVersion(tt.protocols)
		if got != tt.wantVersion {
			t.Errorf("parseMinTLSVersion(%v) = %v, want %v", tt.protocols, got, tt.wantVersion)
		}
	}
}

func TestParseCipherSuites(t *testing.T) {
	tests := []struct {
		name    string
		ciphers []string
		wantLen int
	}{
		{
			name:    "valid ciphers",
			ciphers: []string{"ECDHE-RSA-AES128-GCM-SHA256", "ECDHE-RSA-AES256-GCM-SHA384"},
			wantLen: 2,
		},
		{
			name:    "empty ciphers",
			ciphers: []string{},
			wantLen: 0, // returns nil for empty
		},
		{
			name:    "unknown ciphers",
			ciphers: []string{"UNKNOWN-CIPHER"},
			wantLen: 0, // returns nil for no valid ciphers
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sslutil.ParseCipherSuitesLenient(tt.ciphers)
			if tt.wantLen == 0 && got != nil {
				t.Errorf("Expected nil, got %v", got)
			} else if tt.wantLen > 0 && len(got) != tt.wantLen {
				t.Errorf("Expected %d ciphers, got %d", tt.wantLen, len(got))
			}
		})
	}
}

func TestLoadCertPool(t *testing.T) {
	t.Run("valid cert", func(t *testing.T) {
		tempDir := t.TempDir()
		certFile := filepath.Join(tempDir, "ca.crt")

		// 创建证书
		template := &x509.Certificate{
			SerialNumber: big.NewInt(1),
			NotBefore:    time.Now(),
			NotAfter:     time.Now().Add(24 * time.Hour),
			IsCA:         true,
			KeyUsage:     x509.KeyUsageCertSign,
		}

		key, _ := rsa.GenerateKey(rand.Reader, 2048)
		certDER, _ := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)

		certOut, err := os.Create(certFile)
		if err != nil {
			t.Fatalf("Failed to create cert file: %v", err)
		}
		defer func() { _ = certOut.Close() }()
		if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: certDER}); err != nil {
			t.Fatalf("Failed to encode certificate: %v", err)
		}

		pool, err := sslutil.LoadCertPool(certFile, "test")
		if err != nil {
			t.Fatalf("loadCertPool failed: %v", err)
		}
		if pool == nil {
			t.Error("Expected non-nil pool")
		}
	})

	t.Run("invalid path", func(t *testing.T) {
		_, err := sslutil.LoadCertPool("/nonexistent/cert.pem", "test")
		if err == nil {
			t.Error("Expected error for nonexistent file")
		}
	})

	t.Run("invalid content", func(t *testing.T) {
		tempDir := t.TempDir()
		certFile := filepath.Join(tempDir, "invalid.crt")
		if err := os.WriteFile(certFile, []byte("not a certificate"), 0o644); err != nil {
			t.Fatalf("写入无效证书文件失败: %v", err)
		}

		_, err := sslutil.LoadCertPool(certFile, "test")
		if err == nil {
			t.Error("Expected error for invalid certificate content")
		}
	})
}




