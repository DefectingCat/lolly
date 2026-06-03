// Package ssl 提供 mTLS 客户端验证的单元测试。
//
// 测试覆盖：
//   - CA 证书池加载
//   - 验证模式解析
//   - 客户端证书验证
//   - CRL 检查
//   - 变量提取
//
// 作者：xfy
package ssl

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"rua.plus/lolly/internal/config"
	"rua.plus/lolly/internal/sslutil"
)

// generateTestCA 生成测试 CA 证书。
func generateTestCA(t *testing.T) (*x509.Certificate, *rsa.PrivateKey, []byte) {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("Failed to generate CA key: %v", err)
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "Test CA",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("Failed to create CA cert: %v", err)
	}

	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		t.Fatalf("Failed to parse CA cert: %v", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	return cert, key, certPEM
}

// generateTestClientCert 生成测试客户端证书，serial 参数指定序列号。
func generateTestClientCert(t *testing.T, caCert *x509.Certificate, caKey *rsa.PrivateKey, serial int64) (*x509.Certificate, *rsa.PrivateKey, []byte) {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("Failed to generate client key: %v", err)
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(serial),
		Subject: pkix.Name{
			CommonName:   "Test Client",
			Organization: []string{"Test Org"},
		},
		NotBefore:      time.Now(),
		NotAfter:       time.Now().Add(24 * time.Hour),
		KeyUsage:       x509.KeyUsageDigitalSignature,
		ExtKeyUsage:    []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		EmailAddresses: []string{"test@example.com"},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, caCert, &key.PublicKey, caKey)
	if err != nil {
		t.Fatalf("Failed to create client cert: %v", err)
	}

	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		t.Fatalf("Failed to parse client cert: %v", err)
	}

	return cert, key, certDER
}

// TestParseVerifyMode 测试验证模式解析。
func TestParseVerifyMode(t *testing.T) {
	tests := []struct {
		input    string
		expected ClientVerifyMode
		wantErr  bool
	}{
		{"off", VerifyOff, false},
		{"", VerifyOff, false},
		{"on", VerifyOn, false},
		{"optional", VerifyOptional, false},
		{"optional_no_ca", VerifyOptionalNoCA, false},
		{"invalid", VerifyOff, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			mode, err := ParseVerifyMode(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseVerifyMode(%q) expected error", tt.input)
				}
				return
			}
			if err != nil {
				t.Errorf("ParseVerifyMode(%q) unexpected error: %v", tt.input, err)
				return
			}
			if mode != tt.expected {
				t.Errorf("ParseVerifyMode(%q) = %v, want %v", tt.input, mode, tt.expected)
			}
		})
	}
}

// TestLoadCACertPool 测试 CA 证书池加载。
func TestLoadCACertPool(t *testing.T) {
	// 创建临时 CA 文件
	tempDir := t.TempDir()
	caFile := filepath.Join(tempDir, "ca.crt")

	_, _, caPEM := generateTestCA(t)
	if err := os.WriteFile(caFile, caPEM, 0o644); err != nil {
		t.Fatalf("Failed to write CA file: %v", err)
	}

	// 测试加载
	pool, err := sslutil.LoadCACertPool(caFile)
	if err != nil {
		t.Fatalf("LoadCACertPool() failed: %v", err)
	}
	if pool == nil {
		t.Fatal("LoadCACertPool() returned nil pool")
	}

	// 测试文件不存在
	_, err = sslutil.LoadCACertPool("/nonexistent/ca.crt")
	if err == nil {
		t.Error("LoadCACertPool() should fail for non-existent file")
	}

	// 测试无效证书
	invalidFile := filepath.Join(tempDir, "invalid.crt")
	if err := os.WriteFile(invalidFile, []byte("invalid data"), 0o644); err != nil {
		t.Fatalf("写入无效证书文件失败: %v", err)
	}
	_, err = sslutil.LoadCACertPool(invalidFile)
	if err == nil {
		t.Error("LoadCACertPool() should fail for invalid certificate")
	}
}

// TestClientVerifier_ConfigureTLS 测试 TLS 配置。
func TestClientVerifier_ConfigureTLS(t *testing.T) {
	// 创建临时 CA 文件
	tempDir := t.TempDir()
	caFile := filepath.Join(tempDir, "ca.crt")

	_, _, caPEM := generateTestCA(t)
	if err := os.WriteFile(caFile, caPEM, 0o644); err != nil {
		t.Fatalf("Failed to write CA file: %v", err)
	}

	verifier, err := NewClientVerifier(config.ClientVerifyConfig{
		Enabled:     true,
		Mode:        "on",
		ClientCA:    caFile,
		VerifyDepth: 2,
	})
	if err != nil {
		t.Fatalf("NewClientVerifier() failed: %v", err)
	}

	tlsCfg := &tls.Config{}
	verifier.ConfigureTLS(tlsCfg)

	if tlsCfg.ClientAuth != tls.RequireAndVerifyClientCert {
		t.Errorf("ClientAuth = %v, want %v", tlsCfg.ClientAuth, tls.RequireAndVerifyClientCert)
	}
	if tlsCfg.ClientCAs == nil {
		t.Error("ClientCAs should be set")
	}

	// 测试 nil 配置（不应 panic）
	verifier.ConfigureTLS(nil)
}

// TestClientVerifier_ConfigureTLS_Disabled 测试禁用验证器。
func TestClientVerifier_ConfigureTLS_Disabled(t *testing.T) {
	verifier, _ := NewClientVerifier(config.ClientVerifyConfig{
		Enabled: false,
	})

	tlsCfg := &tls.Config{}
	verifier.ConfigureTLS(tlsCfg)

	// 禁用时不应修改配置
	if tlsCfg.ClientAuth != 0 {
		t.Error("Disabled verifier should not modify TLS config")
	}
}

// generateTestCRL 生成测试 CRL。
func generateTestCRL(t *testing.T, caCert *x509.Certificate, caKey *rsa.PrivateKey, revokedSerials []*big.Int) []byte {
	t.Helper()

	template := &x509.RevocationList{
		Number:     big.NewInt(1),
		ThisUpdate: time.Now(),
		NextUpdate: time.Now().Add(24 * time.Hour),
		RevokedCertificateEntries: func() []x509.RevocationListEntry {
			entries := make([]x509.RevocationListEntry, len(revokedSerials))
			for i, serial := range revokedSerials {
				entries[i] = x509.RevocationListEntry{
					SerialNumber:   serial,
					RevocationTime: time.Now(),
				}
			}
			return entries
		}(),
	}

	crlDER, err := x509.CreateRevocationList(rand.Reader, template, caCert, caKey)
	if err != nil {
		t.Fatalf("Failed to create CRL: %v", err)
	}

	return pem.EncodeToMemory(&pem.Block{Type: "X509 CRL", Bytes: crlDER})
}

// TestLoadCRL 测试 CRL 加载。
func TestLoadCRL(t *testing.T) {
	// 生成测试 CA
	caCert, caKey, _ := generateTestCA(t)

	// 生成包含吊销证书的 CRL
	revokedSerial := big.NewInt(999)
	crlPEM := generateTestCRL(t, caCert, caKey, []*big.Int{revokedSerial})

	// 写入临时文件
	tempDir := t.TempDir()
	crlFile := filepath.Join(tempDir, "crl.pem")
	if err := os.WriteFile(crlFile, crlPEM, 0o644); err != nil {
		t.Fatalf("写入 CRL 文件失败: %v", err)
	}

	// 测试加载
	crl, err := LoadCRL(crlFile)
	if err != nil {
		t.Fatalf("LoadCRL() failed: %v", err)
	}
	if crl == nil {
		t.Fatal("LoadCRL() returned nil")
	}
	if len(crl.RevokedCertificateEntries) != 1 {
		t.Errorf("CRL should have 1 revoked certificate, got %d", len(crl.RevokedCertificateEntries))
	}

	// 测试文件不存在
	_, err = LoadCRL("/nonexistent/crl.pem")
	if err == nil {
		t.Error("LoadCRL() should fail for non-existent file")
	}

	// 测试无效 CRL
	invalidFile := filepath.Join(tempDir, "invalid.crl")
	if err := os.WriteFile(invalidFile, []byte("invalid data"), 0o644); err != nil {
		t.Fatalf("写入无效文件失败: %v", err)
	}
	_, err = LoadCRL(invalidFile)
	if err == nil {
		t.Error("LoadCRL() should fail for invalid CRL")
	}
}

// TestVerifyConnection 测试连接验证。
func TestVerifyConnection(t *testing.T) {
	// 生成测试 CA 和证书
	caCert, caKey, _ := generateTestCA(t)
	validCert, _, _ := generateTestClientCert(t, caCert, caKey, 300)
	revokedCert, _, _ := generateTestClientCert(t, caCert, caKey, 400)

	// 生成包含吊销证书的 CRL
	crlPEM := generateTestCRL(t, caCert, caKey, []*big.Int{revokedCert.SerialNumber})

	tempDir := t.TempDir()
	crlFile := filepath.Join(tempDir, "crl.pem")
	caFile := filepath.Join(tempDir, "ca.crt")
	_, _, caPEM := generateTestCA(t)
	if err := os.WriteFile(crlFile, crlPEM, 0o644); err != nil {
		t.Fatalf("写入 CRL 文件失败: %v", err)
	}
	if err := os.WriteFile(caFile, caPEM, 0o644); err != nil {
		t.Fatalf("写入 CA 文件失败: %v", err)
	}

	// 测试带 CRL 和深度限制的验证器
	verifier, err := NewClientVerifier(config.ClientVerifyConfig{
		Enabled:     true,
		Mode:        "on",
		ClientCA:    caFile,
		CRL:         crlFile,
		VerifyDepth: 3,
	})
	if err != nil {
		t.Fatalf("NewClientVerifier() failed: %v", err)
	}

	// 配置 TLS 以设置 VerifyConnection 回调
	tlsCfg := &tls.Config{}
	verifier.ConfigureTLS(tlsCfg)

	if tlsCfg.VerifyConnection == nil {
		t.Fatal("VerifyConnection should be set when VerifyDepth > 0")
	}

	// 测试有效证书连接
	validCS := &tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{validCert},
	}
	err = tlsCfg.VerifyConnection(*validCS)
	if err != nil {
		t.Errorf("VerifyConnection() should pass for valid cert: %v", err)
	}

	// 测试吊销证书连接
	revokedCS := &tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{revokedCert},
	}
	err = tlsCfg.VerifyConnection(*revokedCS)
	if err == nil {
		t.Error("VerifyConnection() should fail for revoked cert")
	}
}

// TestVerifyConnection_DepthLimit 测试证书链深度限制。
func TestVerifyConnection_DepthLimit(t *testing.T) {
	caCert, caKey, _ := generateTestCA(t)
	clientCert, _, _ := generateTestClientCert(t, caCert, caKey, 500)

	tempDir := t.TempDir()
	caFile := filepath.Join(tempDir, "ca.crt")
	_, _, caPEM := generateTestCA(t)
	if err := os.WriteFile(caFile, caPEM, 0o644); err != nil {
		t.Fatalf("写入 CA 文件失败: %v", err)
	}

	// 测试深度限制为 1
	verifier, _ := NewClientVerifier(config.ClientVerifyConfig{
		Enabled:     true,
		Mode:        "on",
		ClientCA:    caFile,
		VerifyDepth: 1,
	})

	tlsCfg := &tls.Config{}
	verifier.ConfigureTLS(tlsCfg)

	// 单个证书应通过
	cs := &tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{clientCert},
	}
	err := tlsCfg.VerifyConnection(*cs)
	if err != nil {
		t.Errorf("VerifyConnection() should pass for single cert with depth 1: %v", err)
	}

	// 多个证书应失败（链太长）
	longChain := &tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{clientCert, caCert},
	}
	err = tlsCfg.VerifyConnection(*longChain)
	if err == nil {
		t.Error("VerifyConnection() should fail for chain exceeding depth limit")
	}
}

// BenchmarkLoadCACertPool 基准测试 CA 证书池加载。
func BenchmarkLoadCACertPool(b *testing.B) {
	tempDir := b.TempDir()
	caFile := filepath.Join(tempDir, "ca.crt")

	// 生成 CA
	_, _, caPEM := generateTestCA(&testing.T{})
	if err := os.WriteFile(caFile, caPEM, 0o644); err != nil {
		b.Fatalf("写入 CA 文件失败: %v", err)
	}

	b.ResetTimer()
	for b.Loop() {
		_, err := sslutil.LoadCACertPool(caFile)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// TestNewClientVerifier_InvalidMode 测试无效验证模式。
func TestNewClientVerifier_InvalidMode(t *testing.T) {
	_, err := NewClientVerifier(config.ClientVerifyConfig{
		Enabled: true,
		Mode:    "invalid_mode",
	})
	if err == nil {
		t.Error("Expected error for invalid verify mode")
	}
}

// TestNewClientVerifier_InvalidCRL 测试无效 CRL 文件。
func TestNewClientVerifier_InvalidCRL(t *testing.T) {
	tempDir := t.TempDir()
	caFile := filepath.Join(tempDir, "ca.crt")
	crlFile := filepath.Join(tempDir, "invalid.crl")

	_, _, caPEM := generateTestCA(t)
	if err := os.WriteFile(caFile, caPEM, 0o644); err != nil {
		t.Fatalf("Failed to write CA file: %v", err)
	}
	if err := os.WriteFile(crlFile, []byte("invalid crl data"), 0o644); err != nil {
		t.Fatalf("Failed to write invalid CRL file: %v", err)
	}

	_, err := NewClientVerifier(config.ClientVerifyConfig{
		Enabled:  true,
		Mode:     "on",
		ClientCA: caFile,
		CRL:      crlFile,
	})
	if err == nil {
		t.Error("Expected error for invalid CRL file")
	}
}
