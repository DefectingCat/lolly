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
	"fmt"
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
	if err := os.WriteFile(caFile, caPEM, 0o644); err != nil {
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

// TestGetClientCertInfo 测试证书信息提取。
func TestGetClientCertInfo(t *testing.T) {
	// 生成测试证书
	caCert, caKey, _ := generateTestCA(t)
	clientCert, _, _ := generateTestClientCert(t, caCert, caKey, 2)

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

// TestGetMode 测试获取验证模式。
func TestGetMode(t *testing.T) {
	tests := []struct {
		name     string
		mode     string
		expected ClientVerifyMode
	}{
		{"off", "off", VerifyOff},
		{"on", "on", VerifyOn},
		{"optional", "optional", VerifyOptional},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			caFile := filepath.Join(tempDir, "ca.crt")
			_, _, caPEM := generateTestCA(t)
			if err := os.WriteFile(caFile, caPEM, 0o644); err != nil {
				t.Fatalf("写入 CA 文件失败: %v", err)
			}

			var cfg config.ClientVerifyConfig
			if tt.mode != "off" {
				cfg = config.ClientVerifyConfig{
					Enabled:  true,
					Mode:     tt.mode,
					ClientCA: caFile,
				}
			} else {
				cfg = config.ClientVerifyConfig{Enabled: false}
			}

			verifier, err := NewClientVerifier(cfg)
			if err != nil {
				t.Fatalf("NewClientVerifier() failed: %v", err)
			}

			if verifier.GetMode() != tt.expected {
				t.Errorf("GetMode() = %v, want %v", verifier.GetMode(), tt.expected)
			}
		})
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

// TestCheckCRL 测试 CRL 检查。
func TestCheckCRL(t *testing.T) {
	// 生成测试 CA
	caCert, caKey, _ := generateTestCA(t)

	// 生成将被吊销的客户端证书（序列号100）
	revokedCert, _, _ := generateTestClientCert(t, caCert, caKey, 100)

	// 生成有效客户端证书（序列号200，不会被吊销）
	validCert, _, _ := generateTestClientCert(t, caCert, caKey, 200)

	// 生成包含吊销证书的 CRL
	crlPEM := generateTestCRL(t, caCert, caKey, []*big.Int{revokedCert.SerialNumber})

	// 写入临时文件
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

	// 创建带 CRL 的验证器
	verifier, err := NewClientVerifier(config.ClientVerifyConfig{
		Enabled:  true,
		Mode:     "on",
		ClientCA: caFile,
		CRL:      crlFile,
	})
	if err != nil {
		t.Fatalf("NewClientVerifier() failed: %v", err)
	}

	// 测试检查有效证书
	err = verifier.ValidateClientCertificate(validCert)
	if err != nil {
		t.Errorf("CheckCRL() should pass for valid cert: %v", err)
	}

	// 测试检查吊销证书
	err = verifier.ValidateClientCertificate(revokedCert)
	if err == nil {
		t.Error("CheckCRL() should fail for revoked cert")
	}
}

// TestCheckCRL_EmptyCRL 测试空 CRL。
func TestCheckCRL_EmptyCRL(t *testing.T) {
	caCert, caKey, _ := generateTestCA(t)
	clientCert, _, _ := generateTestClientCert(t, caCert, caKey, 50)

	// 生成空 CRL（无吊销证书）
	crlPEM := generateTestCRL(t, caCert, caKey, nil)

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

	verifier, err := NewClientVerifier(config.ClientVerifyConfig{
		Enabled:  true,
		Mode:     "on",
		ClientCA: caFile,
		CRL:      crlFile,
	})
	if err != nil {
		t.Fatalf("NewClientVerifier() failed: %v", err)
	}

	// 空列表应通过所有证书
	err = verifier.ValidateClientCertificate(clientCert)
	if err != nil {
		t.Errorf("CheckCRL() should pass with empty CRL: %v", err)
	}
}

// TestValidateClientCertificate 测试手动验证客户端证书。
func TestValidateClientCertificate(t *testing.T) {
	// 测试禁用验证器
	verifier, _ := NewClientVerifier(config.ClientVerifyConfig{Enabled: false})

	err := verifier.ValidateClientCertificate(nil)
	if err != nil {
		t.Errorf("Disabled verifier should accept nil cert: %v", err)
	}

	// 测试启用验证器（on 模式）
	tempDir := t.TempDir()
	caFile := filepath.Join(tempDir, "ca.crt")
	_, _, caPEM := generateTestCA(t)
	if err := os.WriteFile(caFile, caPEM, 0o644); err != nil {
		t.Fatalf("写入 CA 文件失败: %v", err)
	}

	verifier, _ = NewClientVerifier(config.ClientVerifyConfig{
		Enabled:  true,
		Mode:     "on",
		ClientCA: caFile,
	})

	// nil 证书在 on 模式应失败
	err = verifier.ValidateClientCertificate(nil)
	if err == nil {
		t.Error("ValidateClientCertificate(nil) should fail in 'on' mode")
	}

	// 有效证书应通过
	caCert, caKey, _ := generateTestCA(t)
	clientCert, _, _ := generateTestClientCert(t, caCert, caKey, 30)
	err = verifier.ValidateClientCertificate(clientCert)
	if err != nil {
		t.Errorf("ValidateClientCertificate() should pass for valid cert: %v", err)
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

// TestFingerprint_Nil 测试 nil 证书的指纹。
func TestFingerprint_Nil(t *testing.T) {
	result := fingerprint(nil)
	if result != "" {
		t.Errorf("Expected empty string for nil cert, got: %s", result)
	}
}

// TestFingerprint_Valid 测试有效证书的指纹。
func TestFingerprint_Valid(t *testing.T) {
	caCert, _, _ := generateTestCA(t)
	result := fingerprint(caCert)
	if result == "" {
		t.Error("Expected non-empty fingerprint for valid cert")
	}
	// 指纹应该是证书 Raw 的十六进制表示
	expected := fmt.Sprintf("%x", caCert.Raw)
	if result != expected {
		t.Error("Fingerprint should match certificate Raw hex")
	}
}

// TestClientVerifyMode_String 测试验证模式字符串表示。
func TestClientVerifyMode_String(t *testing.T) {
	tests := []struct {
		mode     ClientVerifyMode
		expected string
	}{
		{VerifyOff, "off"},
		{VerifyOn, "on"},
		{VerifyOptional, "optional"},
		{VerifyOptionalNoCA, "optional_no_ca"},
		{ClientVerifyMode(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.mode.String(); got != tt.expected {
				t.Errorf("String() = %q, want %q", got, tt.expected)
			}
		})
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

// TestNewClientVerifier_WithCRL 测试带 CRL 的验证器。
func TestNewClientVerifier_WithCRL(t *testing.T) {
	tempDir := t.TempDir()
	caFile := filepath.Join(tempDir, "ca.crt")
	crlFile := filepath.Join(tempDir, "crl.pem")

	caCert, caKey, caPEM := generateTestCA(t)
	if err := os.WriteFile(caFile, caPEM, 0o644); err != nil {
		t.Fatalf("Failed to write CA file: %v", err)
	}

	// 生成空 CRL
	crlPEM := generateTestCRL(t, caCert, caKey, nil)
	if err := os.WriteFile(crlFile, crlPEM, 0o644); err != nil {
		t.Fatalf("Failed to write CRL file: %v", err)
	}

	verifier, err := NewClientVerifier(config.ClientVerifyConfig{
		Enabled:  true,
		Mode:     "on",
		ClientCA: caFile,
		CRL:      crlFile,
	})
	if err != nil {
		t.Fatalf("NewClientVerifier() failed: %v", err)
	}
	if !verifier.IsEnabled() {
		t.Error("Verifier should be enabled")
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

// TestClientVerifier_ValidateClientCertificate_WithCRL 测试带 CRL 的证书验证。
func TestClientVerifier_ValidateClientCertificate_WithCRL(t *testing.T) {
	tempDir := t.TempDir()
	caFile := filepath.Join(tempDir, "ca.crt")
	crlFile := filepath.Join(tempDir, "crl.pem")

	caCert, caKey, caPEM := generateTestCA(t)
	if err := os.WriteFile(caFile, caPEM, 0o644); err != nil {
		t.Fatalf("Failed to write CA file: %v", err)
	}

	// 生成客户端证书
	clientCert, _, _ := generateTestClientCert(t, caCert, caKey, 600)

	// 生成包含吊销证书的 CRL
	crlPEM := generateTestCRL(t, caCert, caKey, []*big.Int{clientCert.SerialNumber})
	if err := os.WriteFile(crlFile, crlPEM, 0o644); err != nil {
		t.Fatalf("Failed to write CRL file: %v", err)
	}

	verifier, err := NewClientVerifier(config.ClientVerifyConfig{
		Enabled:  true,
		Mode:     "on",
		ClientCA: caFile,
		CRL:      crlFile,
	})
	if err != nil {
		t.Fatalf("NewClientVerifier() failed: %v", err)
	}

	// 验证吊销证书应失败
	err = verifier.ValidateClientCertificate(clientCert)
	if err == nil {
		t.Error("Expected error for revoked certificate")
	}
}

// TestGetClientCertInfo_WithEmail 测试带邮件地址的证书信息提取。
func TestGetClientCertInfo_WithEmail(t *testing.T) {
	caCert, caKey, _ := generateTestCA(t)
	clientCert, _, _ := generateTestClientCert(t, caCert, caKey, 700)

	cs := &tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{clientCert},
	}

	info := GetClientCertInfo(cs)
	if info == nil {
		t.Fatal("GetClientCertInfo() returned nil")
	}
	if len(info.Email) == 0 {
		t.Error("Expected email addresses in cert info")
	}
	if info.Email[0] != "test@example.com" {
		t.Errorf("Expected email test@example.com, got %s", info.Email[0])
	}
}
