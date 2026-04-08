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

// generateTestClientCert 生成测试客户端证书。
func generateTestClientCert(t *testing.T, caCert *x509.Certificate, caKey *rsa.PrivateKey) (*x509.Certificate, *rsa.PrivateKey, []byte) {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("Failed to generate client key: %v", err)
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(2),
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

// TestClientVerifyMode_TLSClientAuth 测试 TLS 认证类型映射。
func TestClientVerifyMode_TLSClientAuth(t *testing.T) {
	tests := []struct {
		mode     ClientVerifyMode
		expected tls.ClientAuthType
	}{
		{VerifyOff, tls.NoClientCert},
		{VerifyOn, tls.RequireAndVerifyClientCert},
		{VerifyOptional, tls.VerifyClientCertIfGiven},
		{VerifyOptionalNoCA, tls.RequestClientCert},
	}

	for _, tt := range tests {
		t.Run(tt.mode.String(), func(t *testing.T) {
			auth := tt.mode.TLSClientAuth()
			if auth != tt.expected {
				t.Errorf("TLSClientAuth() = %v, want %v", auth, tt.expected)
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
	if err := os.WriteFile(caFile, caPEM, 0644); err != nil {
		t.Fatalf("Failed to write CA file: %v", err)
	}

	// 测试加载
	pool, err := LoadCACertPool(caFile)
	if err != nil {
		t.Fatalf("LoadCACertPool() failed: %v", err)
	}
	if pool == nil {
		t.Fatal("LoadCACertPool() returned nil pool")
	}

	// 测试文件不存在
	_, err = LoadCACertPool("/nonexistent/ca.crt")
	if err == nil {
		t.Error("LoadCACertPool() should fail for non-existent file")
	}

	// 测试无效证书
	invalidFile := filepath.Join(tempDir, "invalid.crt")
	if err := os.WriteFile(invalidFile, []byte("invalid data"), 0644); err != nil {
		t.Fatalf("写入无效证书文件失败: %v", err)
	}
	_, err = LoadCACertPool(invalidFile)
	if err == nil {
		t.Error("LoadCACertPool() should fail for invalid certificate")
	}
}

// TestNewClientVerifier 测试创建客户端验证器。
func TestNewClientVerifier(t *testing.T) {
	// 测试禁用状态
	verifier, err := NewClientVerifier(config.ClientVerifyConfig{
		Enabled: false,
	})
	if err != nil {
		t.Fatalf("NewClientVerifier() failed for disabled config: %v", err)
	}
	if verifier.IsEnabled() {
		t.Error("Verifier should be disabled when Enabled=false")
	}

	// 测试启用但无 CA（应该失败）
	_, err = NewClientVerifier(config.ClientVerifyConfig{
		Enabled: true,
		Mode:    "on",
	})
	if err == nil {
		t.Error("NewClientVerifier() should fail without CA file")
	}
}

// TestNewClientVerifier_WithCA 测试带 CA 的验证器。
func TestNewClientVerifier_WithCA(t *testing.T) {
	// 创建临时 CA 文件
	tempDir := t.TempDir()
	caFile := filepath.Join(tempDir, "ca.crt")

	_, _, caPEM := generateTestCA(t)
	if err := os.WriteFile(caFile, caPEM, 0644); err != nil {
		t.Fatalf("Failed to write CA file: %v", err)
	}

	// 测试各种模式
	modes := []string{"on", "optional"}
	for _, mode := range modes {
		t.Run(mode, func(t *testing.T) {
			verifier, err := NewClientVerifier(config.ClientVerifyConfig{
				Enabled:  true,
				Mode:     mode,
				ClientCA: caFile,
			})
			if err != nil {
				t.Fatalf("NewClientVerifier() failed: %v", err)
			}
			if !verifier.IsEnabled() {
				t.Error("Verifier should be enabled")
			}
		})
	}
}

// TestClientVerifier_ConfigureTLS 测试 TLS 配置。
func TestClientVerifier_ConfigureTLS(t *testing.T) {
	// 创建临时 CA 文件
	tempDir := t.TempDir()
	caFile := filepath.Join(tempDir, "ca.crt")

	_, _, caPEM := generateTestCA(t)
	if err := os.WriteFile(caFile, caPEM, 0644); err != nil {
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

// TestGetClientCertInfo 测试证书信息提取。
func TestGetClientCertInfo(t *testing.T) {
	// 生成测试证书
	caCert, caKey, _ := generateTestCA(t)
	clientCert, _, _ := generateTestClientCert(t, caCert, caKey)

	// 创建模拟连接状态
	cs := &tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{clientCert},
	}

	info := GetClientCertInfo(cs)
	if info == nil {
		t.Fatal("GetClientCertInfo() returned nil")
	}

	if info.Serial == "" {
		t.Error("Serial should not be empty")
	}
	if info.Subject == "" {
		t.Error("Subject should not be empty")
	}
	if info.Issuer == "" {
		t.Error("Issuer should not be empty")
	}

	// 测试无证书
	emptyCs := &tls.ConnectionState{}
	info = GetClientCertInfo(emptyCs)
	if info != nil {
		t.Error("GetClientCertInfo() should return nil for no certificates")
	}
}

// TestGetClientCertInfo_Nil 测试 nil 输入。
func TestGetClientCertInfo_Nil(t *testing.T) {
	info := GetClientCertInfo(nil)
	if info != nil {
		t.Error("GetClientCertInfo(nil) should return nil")
	}
}

// BenchmarkLoadCACertPool 基准测试 CA 证书池加载。
func BenchmarkLoadCACertPool(b *testing.B) {
	tempDir := b.TempDir()
	caFile := filepath.Join(tempDir, "ca.crt")

	// 生成 CA
	_, _, caPEM := generateTestCA(&testing.T{})
	if err := os.WriteFile(caFile, caPEM, 0644); err != nil {
		b.Fatalf("写入 CA 文件失败: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := LoadCACertPool(caFile)
		if err != nil {
			b.Fatal(err)
		}
	}
}
