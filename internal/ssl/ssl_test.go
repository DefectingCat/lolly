// Package ssl 提供 SSL/TLS 功能的测试。
//
// 该文件测试 SSL 模块的各项功能，包括：
//   - TLS 管理器创建和配置
//   - TLS 版本和加密套件解析
//   - 证书验证
//   - 多证书管理
//   - TLS 配置获取
//
// 作者：xfy
package ssl

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
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

func TestNewTLSManager(t *testing.T) {
	tests := []struct {
		cfg     *config.SSLConfig
		name    string
		errMsg  string
		wantErr bool
	}{
		{
			name:    "nil config",
			cfg:     nil,
			wantErr: true,
			errMsg:  "ssl config is nil",
		},
		{
			name: "empty cert path",
			cfg: &config.SSLConfig{
				Key: "key.pem",
			},
			wantErr: true,
			errMsg:  "certificate and key paths are required",
		},
		{
			name: "empty key path",
			cfg: &config.SSLConfig{
				Cert: "cert.pem",
			},
			wantErr: true,
			errMsg:  "certificate and key paths are required",
		},
		{
			name: "non-existent cert file",
			cfg: &config.SSLConfig{
				Cert: "/nonexistent/cert.pem",
				Key:  "/nonexistent/key.pem",
			},
			wantErr: true,
			errMsg:  "failed to load certificate",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewTLSManager(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewTLSManager() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.errMsg != "" {
				if !containsString(err.Error(), tt.errMsg) {
					t.Errorf("NewTLSManager() error = %v, want errMsg containing %v", err, tt.errMsg)
				}
			}
		})
	}
}

func TestNewTLSManagerWithCert(t *testing.T) {
	// Create temporary test certificate
	tmpDir := t.TempDir()
	certPath := filepath.Join(tmpDir, "cert.pem")
	keyPath := filepath.Join(tmpDir, "key.pem")

	// Generate a self-signed certificate for testing
	cert, key := generateTestCert(t)
	if err := os.WriteFile(certPath, cert, 0o644); err != nil {
		t.Fatalf("Failed to write cert: %v", err)
	}
	if err := os.WriteFile(keyPath, key, 0o600); err != nil {
		t.Fatalf("Failed to write key: %v", err)
	}

	cfg := &config.SSLConfig{
		Cert: certPath,
		Key:  keyPath,
	}

	manager, err := NewTLSManager(cfg)
	if err != nil {
		t.Fatalf("NewTLSManager() failed: %v", err)
	}

	if manager == nil {
		t.Fatal("Expected non-nil manager")
	}

	tlsCfg := manager.GetTLSConfig()
	if tlsCfg == nil {
		t.Fatal("Expected non-nil TLS config")
	}

	// Check TLS version defaults
	if tlsCfg.MinVersion != tls.VersionTLS12 {
		t.Errorf("Expected MinVersion TLS 1.2, got %v", tlsCfg.MinVersion)
	}

	// Check cipher suites are set
	if len(tlsCfg.CipherSuites) == 0 {
		t.Error("Expected cipher suites to be set")
	}
}

func TestParseTLSVersions(t *testing.T) {
	tests := []struct {
		name      string
		protocols []string
		wantMin   uint16
		wantMax   uint16
		wantErr   bool
	}{
		{
			name:      "TLS 1.2 only",
			protocols: []string{"TLSv1.2"},
			wantMin:   tls.VersionTLS12,
			wantMax:   tls.VersionTLS13,
		},
		{
			name:      "TLS 1.3 only",
			protocols: []string{"TLSv1.3"},
			wantMin:   tls.VersionTLS13,
			wantMax:   tls.VersionTLS13,
		},
		{
			name:      "TLS 1.2 and 1.3",
			protocols: []string{"TLSv1.2", "TLSv1.3"},
			wantMin:   tls.VersionTLS12,
			wantMax:   tls.VersionTLS13,
		},
		{
			name:      "insecure TLS 1.0",
			protocols: []string{"TLSv1.0"},
			wantErr:   true,
		},
		{
			name:      "insecure TLS 1.1",
			protocols: []string{"TLSv1.1"},
			wantErr:   true,
		},
		{
			name:      "unknown protocol",
			protocols: []string{"TLSv0.9"},
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			minVer, maxVer, err := sslutil.ParseTLSVersions(tt.protocols)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseTLSVersions() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if minVer != tt.wantMin {
					t.Errorf("parseTLSVersions() minVer = %v, want %v", minVer, tt.wantMin)
				}
				if maxVer != tt.wantMax {
					t.Errorf("parseTLSVersions() maxVer = %v, want %v", maxVer, tt.wantMax)
				}
			}
		})
	}
}

func TestParseCipherSuites(t *testing.T) {
	tests := []struct {
		name    string
		ciphers []string
		wantLen int
		wantErr bool
	}{
		{
			name:    "valid cipher",
			ciphers: []string{"TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256"},
			wantLen: 1,
		},
		{
			name: "multiple valid ciphers",
			ciphers: []string{
				"TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256",
				"TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384",
			},
			wantLen: 2,
		},
		{
			name:    "unknown cipher",
			ciphers: []string{"TLS_UNKNOWN_CIPHER"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := sslutil.ParseCipherSuites(tt.ciphers)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseCipherSuites() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && len(result) != tt.wantLen {
				t.Errorf("parseCipherSuites() returned %d ciphers, want %d", len(result), tt.wantLen)
			}
		})
	}
}

func TestDefaultCipherSuites(t *testing.T) {
	suites := sslutil.DefaultCipherSuites()
	if len(suites) == 0 {
		t.Error("Expected non-empty default cipher suites")
	}

	// Check that all default ciphers are secure
	for _, suite := range suites {
		if sslutil.IsInsecureCipher(suite) {
			t.Errorf("Default cipher suite %v is insecure", suite)
		}
	}
}

func TestIsInsecureCipher(t *testing.T) {
	// Test known insecure ciphers
	insecureCiphers := []uint16{
		tls.TLS_RSA_WITH_RC4_128_SHA,
		tls.TLS_RSA_WITH_3DES_EDE_CBC_SHA,
	}

	for _, c := range insecureCiphers {
		if !sslutil.IsInsecureCipher(c) {
			t.Errorf("Expected cipher %v to be insecure", c)
		}
	}

	// Test secure ciphers
	secureCiphers := []uint16{
		tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
	}

	for _, c := range secureCiphers {
		if sslutil.IsInsecureCipher(c) {
			t.Errorf("Expected cipher %v to be secure", c)
		}
	}
}

func TestTLSManagerGetTLSConfigForHost(t *testing.T) {
	manager := &TLSManager{
		configs: make(map[string]*tls.Config),
	}

	// Add config for a host
	manager.configs["example.com"] = &tls.Config{
		ServerName: "example.com",
	}
	manager.defaultCfg = &tls.Config{
		ServerName: "default",
	}

	tests := []struct {
		name     string
		host     string
		wantName string
	}{
		{
			name:     "matching host",
			host:     "example.com",
			wantName: "example.com",
		},
		{
			name:     "host with port",
			host:     "example.com:443",
			wantName: "example.com",
		},
		{
			name:     "unknown host uses default",
			host:     "unknown.com",
			wantName: "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := manager.GetTLSConfigForHost(tt.host)
			if cfg == nil {
				t.Fatal("Expected non-nil config")
			}
			if cfg.ServerName != tt.wantName {
				t.Errorf("Expected ServerName %s, got %s", tt.wantName, cfg.ServerName)
			}
		})
	}
}

func TestValidateCertificate(t *testing.T) {
	t.Run("non-existent file", func(t *testing.T) {
		err := ValidateCertificate("/nonexistent/cert.pem")
		if err == nil {
			t.Error("Expected error for non-existent file")
		}
	})

	t.Run("valid file", func(t *testing.T) {
		tmpFile := filepath.Join(t.TempDir(), "cert.pem")
		if err := os.WriteFile(tmpFile, []byte("test"), 0o644); err != nil {
			t.Fatalf("Failed to create temp file: %v", err)
		}

		err := ValidateCertificate(tmpFile)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
	})
}

func TestValidateKey(t *testing.T) {
	t.Run("non-existent file", func(t *testing.T) {
		err := ValidateKey("/nonexistent/key.pem")
		if err == nil {
			t.Error("Expected error for non-existent file")
		}
	})

	t.Run("valid file", func(t *testing.T) {
		tmpFile := filepath.Join(t.TempDir(), "key.pem")
		if err := os.WriteFile(tmpFile, []byte("test"), 0o600); err != nil {
			t.Fatalf("Failed to create temp file: %v", err)
		}

		err := ValidateKey(tmpFile)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
	})
}

// generateTestCert generates a self-signed certificate for testing
func generateTestCert(t *testing.T) ([]byte, []byte) {
	t.Helper()

	// Generate ECDSA private key
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate private key: %v", err)
	}

	// Create certificate template
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Test"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost"},
	}

	// Create certificate
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("Failed to create certificate: %v", err)
	}

	// Encode certificate to PEM
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})

	// Encode private key to PEM
	keyDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		t.Fatalf("Failed to marshal private key: %v", err)
	}

	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: keyDER,
	})

	return certPEM, keyPEM
}

func containsString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestGetTLSConfig 测试 TLS 配置获取
func TestGetTLSConfig(t *testing.T) {
	tmpDir := t.TempDir()
	certPath := filepath.Join(tmpDir, "cert.pem")
	keyPath := filepath.Join(tmpDir, "key.pem")

	// 生成自签名证书
	cert, key := generateTestCert(t)
	if err := os.WriteFile(certPath, cert, 0o644); err != nil {
		t.Fatalf("Failed to write cert: %v", err)
	}
	if err := os.WriteFile(keyPath, key, 0o600); err != nil {
		t.Fatalf("Failed to write key: %v", err)
	}

	cfg := &config.SSLConfig{
		Cert: certPath,
		Key:  keyPath,
	}

	manager, err := NewTLSManager(cfg)
	if err != nil {
		t.Fatalf("NewTLSManager() failed: %v", err)
	}
	defer manager.Close()

	// 验证返回非 nil 配置
	tlsCfg := manager.GetTLSConfig()
	if tlsCfg == nil {
		t.Fatal("Expected non-nil TLS config")
	}

	// 验证 TLS 版本设置
	if tlsCfg.MinVersion != tls.VersionTLS12 {
		t.Errorf("Expected MinVersion TLS 1.2, got %v", tlsCfg.MinVersion)
	}
}

// TestGetTLSConfig_WithProtocols 测试带协议配置的 TLS 配置获取
func TestGetTLSConfig_WithProtocols(t *testing.T) {
	tmpDir := t.TempDir()
	certPath := filepath.Join(tmpDir, "cert.pem")
	keyPath := filepath.Join(tmpDir, "key.pem")

	cert, key := generateTestCert(t)
	if err := os.WriteFile(certPath, cert, 0o644); err != nil {
		t.Fatalf("Failed to write cert: %v", err)
	}
	if err := os.WriteFile(keyPath, key, 0o600); err != nil {
		t.Fatalf("Failed to write key: %v", err)
	}

	cfg := &config.SSLConfig{
		Cert:      certPath,
		Key:       keyPath,
		Protocols: []string{"TLSv1.3"},
	}

	manager, err := NewTLSManager(cfg)
	if err != nil {
		t.Fatalf("NewTLSManager() failed: %v", err)
	}
	defer manager.Close()

	tlsCfg := manager.GetTLSConfig()
	if tlsCfg == nil {
		t.Fatal("Expected non-nil TLS config")
	}

	// 验证 TLS 1.3 设置
	if tlsCfg.MinVersion != tls.VersionTLS13 {
		t.Errorf("Expected MinVersion TLS 1.3, got %v", tlsCfg.MinVersion)
	}
	if tlsCfg.MaxVersion != tls.VersionTLS13 {
		t.Errorf("Expected MaxVersion TLS 1.3, got %v", tlsCfg.MaxVersion)
	}
}

// TestClose 测试 TLS 管理器关闭
func TestClose(t *testing.T) {
	tmpDir := t.TempDir()
	certPath := filepath.Join(tmpDir, "cert.pem")
	keyPath := filepath.Join(tmpDir, "key.pem")

	cert, key := generateTestCert(t)
	if err := os.WriteFile(certPath, cert, 0o644); err != nil {
		t.Fatalf("Failed to write cert: %v", err)
	}
	if err := os.WriteFile(keyPath, key, 0o600); err != nil {
		t.Fatalf("Failed to write key: %v", err)
	}

	cfg := &config.SSLConfig{
		Cert: certPath,
		Key:  keyPath,
	}

	manager, err := NewTLSManager(cfg)
	if err != nil {
		t.Fatalf("NewTLSManager() failed: %v", err)
	}

	// 第一次关闭
	manager.Close()

	// 验证多次调用 Close 是安全的
	manager.Close()
	manager.Close()
}

// TestNewTLSManager_Errors 测试错误场景
func TestNewTLSManager_Errors(t *testing.T) {
	tests := []struct {
		cfg     *config.SSLConfig
		name    string
		errMsg  string
		wantErr bool
	}{
		{
			name:    "nil config",
			cfg:     nil,
			wantErr: true,
			errMsg:  "ssl config is nil",
		},
		{
			name: "缺少证书路径",
			cfg: &config.SSLConfig{
				Key: "key.pem",
			},
			wantErr: true,
			errMsg:  "certificate and key paths are required",
		},
		{
			name: "缺少密钥路径",
			cfg: &config.SSLConfig{
				Cert: "cert.pem",
			},
			wantErr: true,
			errMsg:  "certificate and key paths are required",
		},
		{
			name: "无效证书文件",
			cfg: &config.SSLConfig{
				Cert: "/nonexistent/cert.pem",
				Key:  "/nonexistent/key.pem",
			},
			wantErr: true,
			errMsg:  "failed to load certificate",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewTLSManager(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewTLSManager() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.errMsg != "" {
				if !containsString(err.Error(), tt.errMsg) {
					t.Errorf("NewTLSManager() error = %v, want errMsg containing %v", err, tt.errMsg)
				}
			}
		})
	}
}

// TestNewTLSManager_InvalidCipher 测试无效加密套件
func TestNewTLSManager_InvalidCipher(t *testing.T) {
	tmpDir := t.TempDir()
	certPath := filepath.Join(tmpDir, "cert.pem")
	keyPath := filepath.Join(tmpDir, "key.pem")

	cert, key := generateTestCert(t)
	if err := os.WriteFile(certPath, cert, 0o644); err != nil {
		t.Fatalf("Failed to write cert: %v", err)
	}
	if err := os.WriteFile(keyPath, key, 0o600); err != nil {
		t.Fatalf("Failed to write key: %v", err)
	}

	cfg := &config.SSLConfig{
		Cert:    certPath,
		Key:     keyPath,
		Ciphers: []string{"TLS_UNKNOWN_CIPHER"},
	}

	_, err := NewTLSManager(cfg)
	if err == nil {
		t.Error("Expected error for invalid cipher suite")
	}
}

// TestNewTLSManager_InsecureCipher 测试不安全加密套件
func TestNewTLSManager_InsecureCipher(t *testing.T) {
	tmpDir := t.TempDir()
	certPath := filepath.Join(tmpDir, "cert.pem")
	keyPath := filepath.Join(tmpDir, "key.pem")

	cert, key := generateTestCert(t)
	if err := os.WriteFile(certPath, cert, 0o644); err != nil {
		t.Fatalf("Failed to write cert: %v", err)
	}
	if err := os.WriteFile(keyPath, key, 0o600); err != nil {
		t.Fatalf("Failed to write key: %v", err)
	}

	cfg := &config.SSLConfig{
		Cert:    certPath,
		Key:     keyPath,
		Ciphers: []string{"TLS_ECDHE_RSA_WITH_RC4_128_SHA"},
	}

	_, err := NewTLSManager(cfg)
	if err == nil {
		t.Error("Expected error for insecure cipher suite")
	}
}

// TestNewMultiTLSManager 测试多证书 TLS 管理器
func TestNewMultiTLSManager(t *testing.T) {
	tmpDir := t.TempDir()
	certPath := filepath.Join(tmpDir, "cert.pem")
	keyPath := filepath.Join(tmpDir, "key.pem")

	cert, key := generateTestCert(t)
	if err := os.WriteFile(certPath, cert, 0o644); err != nil {
		t.Fatalf("Failed to write cert: %v", err)
	}
	if err := os.WriteFile(keyPath, key, 0o600); err != nil {
		t.Fatalf("Failed to write key: %v", err)
	}

	configs := map[string]*config.SSLConfig{
		"example.com": {
			Cert: certPath,
			Key:  keyPath,
		},
	}

	manager, err := NewMultiTLSManager(configs, nil)
	if err != nil {
		t.Fatalf("NewMultiTLSManager() failed: %v", err)
	}
	defer manager.Close()

	// 验证配置已加载
	cfg := manager.GetTLSConfigForHost("example.com")
	if cfg == nil {
		t.Fatal("Expected non-nil config for example.com")
	}
}

// TestNewMultiTLSManager_EmptyConfigs 测试空配置
func TestNewMultiTLSManager_EmptyConfigs(t *testing.T) {
	_, err := NewMultiTLSManager(map[string]*config.SSLConfig{}, nil)
	if err == nil {
		t.Error("Expected error for empty configs")
	}
}

// TestNewMultiTLSManager_NilConfig 测试 nil 配置项
func TestNewMultiTLSManager_NilConfig(t *testing.T) {
	configs := map[string]*config.SSLConfig{
		"example.com": nil,
	}

	_, err := NewMultiTLSManager(configs, nil)
	if err == nil {
		t.Error("Expected error for nil config")
	}
}

// TestGetCertificate 测试证书获取回调
func TestGetCertificate(t *testing.T) {
	tmpDir := t.TempDir()
	certPath := filepath.Join(tmpDir, "cert.pem")
	keyPath := filepath.Join(tmpDir, "key.pem")

	cert, key := generateTestCert(t)
	if err := os.WriteFile(certPath, cert, 0o644); err != nil {
		t.Fatalf("Failed to write cert: %v", err)
	}
	if err := os.WriteFile(keyPath, key, 0o600); err != nil {
		t.Fatalf("Failed to write key: %v", err)
	}

	configs := map[string]*config.SSLConfig{
		"example.com": {
			Cert: certPath,
			Key:  keyPath,
		},
	}

	manager, err := NewMultiTLSManager(configs, nil)
	if err != nil {
		t.Fatalf("NewMultiTLSManager() failed: %v", err)
	}
	defer manager.Close()

	getCert := manager.GetCertificate()
	if getCert == nil {
		t.Fatal("Expected non-nil GetCertificate function")
	}

	// 测试获取存在的证书
	testHello := &tls.ClientHelloInfo{
		ServerName: "example.com",
	}
	certResult, err := getCert(testHello)
	if err != nil {
		t.Errorf("GetCertificate() error = %v", err)
	}
	if certResult == nil {
		t.Error("Expected non-nil certificate")
	}
}

// TestAddCertificate 测试添加证书
func TestAddCertificate(t *testing.T) {
	tmpDir := t.TempDir()
	certPath := filepath.Join(tmpDir, "cert.pem")
	keyPath := filepath.Join(tmpDir, "key.pem")

	cert, key := generateTestCert(t)
	if err := os.WriteFile(certPath, cert, 0o644); err != nil {
		t.Fatalf("Failed to write cert: %v", err)
	}
	if err := os.WriteFile(keyPath, key, 0o600); err != nil {
		t.Fatalf("Failed to write key: %v", err)
	}

	// 创建带默认配置的管理器
	cfg := &config.SSLConfig{
		Cert: certPath,
		Key:  keyPath,
	}

	manager, err := NewTLSManager(cfg)
	if err != nil {
		t.Fatalf("NewTLSManager() failed: %v", err)
	}
	defer manager.Close()

	// 测试添加新证书
	err = manager.AddCertificate("newhost.com", cfg)
	if err != nil {
		t.Errorf("AddCertificate() error = %v", err)
	}

	// 验证新证书已添加
	hostCfg := manager.GetTLSConfigForHost("newhost.com")
	if hostCfg == nil {
		t.Error("Expected config for newhost.com")
	}
}

// TestAddCertificate_Error 测试添加证书错误
func TestAddCertificate_Error(t *testing.T) {
	tmpDir := t.TempDir()
	certPath := filepath.Join(tmpDir, "cert.pem")
	keyPath := filepath.Join(tmpDir, "key.pem")

	cert, key := generateTestCert(t)
	if err := os.WriteFile(certPath, cert, 0o644); err != nil {
		t.Fatalf("Failed to write cert: %v", err)
	}
	if err := os.WriteFile(keyPath, key, 0o600); err != nil {
		t.Fatalf("Failed to write key: %v", err)
	}

	cfg := &config.SSLConfig{
		Cert: certPath,
		Key:  keyPath,
	}

	manager, err := NewTLSManager(cfg)
	if err != nil {
		t.Fatalf("NewTLSManager() failed: %v", err)
	}
	defer manager.Close()

	// 测试 nil 配置
	err = manager.AddCertificate("test.com", nil)
	if err == nil {
		t.Error("Expected error for nil config")
	}
}

// TestRemoveCertificate 测试移除证书
func TestRemoveCertificate(t *testing.T) {
	tmpDir := t.TempDir()
	certPath := filepath.Join(tmpDir, "cert.pem")
	keyPath := filepath.Join(tmpDir, "key.pem")

	cert, key := generateTestCert(t)
	if err := os.WriteFile(certPath, cert, 0o644); err != nil {
		t.Fatalf("Failed to write cert: %v", err)
	}
	if err := os.WriteFile(keyPath, key, 0o600); err != nil {
		t.Fatalf("Failed to write key: %v", err)
	}

	configs := map[string]*config.SSLConfig{
		"example.com": {
			Cert: certPath,
			Key:  keyPath,
		},
	}

	// 创建一个默认配置
	defaultCfg := &config.SSLConfig{
		Cert: certPath,
		Key:  keyPath,
	}

	manager, err := NewMultiTLSManager(configs, defaultCfg)
	if err != nil {
		t.Fatalf("NewMultiTLSManager() failed: %v", err)
	}
	defer manager.Close()

	// 移除证书
	manager.RemoveCertificate("example.com")

	// 验证证书已移除（应返回默认配置）
	cfg := manager.GetTLSConfigForHost("example.com")
	if cfg == nil {
		t.Error("Expected default config after removal")
	}
}

// TestGetOCSPStatus_NoManager 测试无 OCSP 管理器时的状态
func TestGetOCSPStatus_NoManager(t *testing.T) {
	tmpDir := t.TempDir()
	certPath := filepath.Join(tmpDir, "cert.pem")
	keyPath := filepath.Join(tmpDir, "key.pem")

	cert, key := generateTestCert(t)
	if err := os.WriteFile(certPath, cert, 0o644); err != nil {
		t.Fatalf("Failed to write cert: %v", err)
	}
	if err := os.WriteFile(keyPath, key, 0o600); err != nil {
		t.Fatalf("Failed to write key: %v", err)
	}

	cfg := &config.SSLConfig{
		Cert:         certPath,
		Key:          keyPath,
		OCSPStapling: false, // 禁用 OCSP
	}

	manager, err := NewTLSManager(cfg)
	if err != nil {
		t.Fatalf("NewTLSManager() failed: %v", err)
	}
	defer manager.Close()

	status := manager.GetOCSPStatus()
	if status == nil {
		t.Error("Expected non-nil status map")
	}
	if len(status) != 0 {
		t.Errorf("Expected empty status, got %d entries", len(status))
	}
}

// TestParsePEMChain 测试 PEM 证书链解析
func TestParsePEMChain(t *testing.T) {
	// 测试有效的 PEM 数据
	certPEM, _ := generateTestCert(t)
	certs := parsePEMChain(certPEM)
	if len(certs) == 0 {
		t.Error("Expected at least one certificate from valid PEM")
	}

	// 测试空数据
	emptyCerts := parsePEMChain([]byte{})
	if len(emptyCerts) != 0 {
		t.Error("Expected no certificates from empty data")
	}

	// 测试无效 PEM 数据
	invalidCerts := parsePEMChain([]byte("not valid pem"))
	if len(invalidCerts) != 0 {
		t.Error("Expected no certificates from invalid PEM")
	}
}

// TestExtractPEMBlock 测试 PEM 块提取
func TestExtractPEMBlock(t *testing.T) {
	// 测试有效的证书块
	certPEM, _ := generateTestCert(t)
	block, rest := extractPEMBlock(certPEM)
	if block == nil {
		t.Error("Expected non-nil block from valid PEM")
	}
	if len(block) == 0 {
		t.Error("Expected non-empty block")
	}
	_ = rest

	// 测试空数据
	block, _ = extractPEMBlock([]byte{})
	if block != nil {
		t.Error("Expected nil block from empty data")
	}

	// 测试无结束标记的数据
	invalidData := []byte("-----BEGIN CERTIFICATE-----\nsome data without end")
	block, _ = extractPEMBlock(invalidData)
	if block != nil {
		t.Error("Expected nil block from incomplete PEM")
	}

	// 测试无开始标记的数据
	noStartData := []byte("some data\n-----END CERTIFICATE-----")
	block, _ = extractPEMBlock(noStartData)
	if block != nil {
		t.Error("Expected nil block from data without start marker")
	}
}

// TestFindMarker 测试标记查找
func TestFindMarker(t *testing.T) {
	data := []byte("prefix-----BEGIN CERTIFICATE-----suffix")
	marker := []byte("-----BEGIN CERTIFICATE-----")

	idx := findMarker(data, marker)
	if idx != 6 {
		t.Errorf("Expected index 6, got %d", idx)
	}

	// 测试不存在的标记
	idx = findMarker(data, []byte("NOTFOUND"))
	if idx != -1 {
		t.Errorf("Expected -1 for not found marker, got %d", idx)
	}

	// 测试空数据
	idx = findMarker([]byte{}, marker)
	if idx != -1 {
		t.Errorf("Expected -1 for empty data, got %d", idx)
	}
}

// TestMatchMarker 测试标记匹配
func TestMatchMarker(t *testing.T) {
	data := []byte("-----BEGIN CERTIFICATE-----suffix")
	marker := []byte("-----BEGIN CERTIFICATE-----")

	if !matchMarker(data, marker) {
		t.Error("Expected true for matching marker")
	}

	// 测试不匹配
	if matchMarker(data, []byte("-----END CERTIFICATE-----")) {
		t.Error("Expected false for non-matching marker")
	}

	// 测试数据长度小于标记
	shortData := []byte("short")
	if matchMarker(shortData, marker) {
		t.Error("Expected false when data is shorter than marker")
	}
}

// TestGetCertificate_NoCertificate 测试无证书时的错误情况
func TestGetCertificate_NoCertificate(t *testing.T) {
	manager := &TLSManager{
		configs: make(map[string]*tls.Config),
	}

	getCert := manager.GetCertificate()
	if getCert == nil {
		t.Fatal("Expected non-nil GetCertificate function")
	}

	// 测试未知服务器名且无默认证书
	testHello := &tls.ClientHelloInfo{
		ServerName: "unknown.com",
	}
	certResult, err := getCert(testHello)
	if err == nil {
		t.Error("Expected error when no certificate available")
	}
	if certResult != nil {
		t.Error("Expected nil certificate")
	}
}

// TestGetConfigForClientWithOCSP 测试 OCSP 配置回调
func TestGetConfigForClientWithOCSP(t *testing.T) {
	tmpDir := t.TempDir()
	certPath := filepath.Join(tmpDir, "cert.pem")
	keyPath := filepath.Join(tmpDir, "key.pem")

	// 生成带有 OCSP 服务器的证书
	certPEM, keyPEM := generateTestCertWithOCSP(t, []string{"http://ocsp.example.com"})
	if err := os.WriteFile(certPath, certPEM, 0o644); err != nil {
		t.Fatalf("Failed to write cert: %v", err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		t.Fatalf("Failed to write key: %v", err)
	}

	cfg := &config.SSLConfig{
		Cert:         certPath,
		Key:          keyPath,
		OCSPStapling: true,
	}

	manager, err := NewTLSManager(cfg)
	if err != nil {
		t.Fatalf("NewTLSManager() failed: %v", err)
	}
	defer manager.Close()

	// 测试 GetConfigForClient 回调
	testHello := &tls.ClientHelloInfo{
		ServerName: "localhost",
	}
	tlsCfg, err := manager.getConfigForClientWithOCSP(testHello)
	if err != nil {
		t.Errorf("getConfigForClientWithOCSP() error = %v", err)
	}
	if tlsCfg == nil {
		t.Error("Expected non-nil TLS config")
	}

	// 测试空 ServerName
	emptyHello := &tls.ClientHelloInfo{
		ServerName: "",
	}
	if _, err := manager.getConfigForClientWithOCSP(emptyHello); err != nil {
		t.Errorf("getConfigForClientWithOCSP() error with empty ServerName = %v", err)
	}
}

// TestLoadCertificate_WithCertChain 测试带证书链的加载
func TestLoadCertificate_WithCertChain(t *testing.T) {
	tmpDir := t.TempDir()
	certPath := filepath.Join(tmpDir, "cert.pem")
	keyPath := filepath.Join(tmpDir, "key.pem")
	chainPath := filepath.Join(tmpDir, "chain.pem")

	// 生成主证书
	certPEM, keyPEM := generateTestCert(t)
	if err := os.WriteFile(certPath, certPEM, 0o644); err != nil {
		t.Fatalf("Failed to write cert: %v", err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		t.Fatalf("Failed to write key: %v", err)
	}

	// 生成证书链（使用另一个测试证书）
	chainCert, _ := generateTestCert(t)
	if err := os.WriteFile(chainPath, chainCert, 0o644); err != nil {
		t.Fatalf("Failed to write chain: %v", err)
	}

	// 测试加载带证书链的证书
	cert, err := loadCertificate(certPath, keyPath, chainPath)
	if err != nil {
		t.Fatalf("loadCertificate() error = %v", err)
	}
	if len(cert.Certificate) < 2 {
		t.Errorf("Expected at least 2 certificates in chain, got %d", len(cert.Certificate))
	}
}

// TestLoadCertificate_InvalidChain 测试无效证书链
func TestLoadCertificate_InvalidChain(t *testing.T) {
	tmpDir := t.TempDir()
	certPath := filepath.Join(tmpDir, "cert.pem")
	keyPath := filepath.Join(tmpDir, "key.pem")

	certPEM, keyPEM := generateTestCert(t)
	if err := os.WriteFile(certPath, certPEM, 0o644); err != nil {
		t.Fatalf("Failed to write cert: %v", err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		t.Fatalf("Failed to write key: %v", err)
	}

	// 测试不存在的证书链文件
	_, err := loadCertificate(certPath, keyPath, "/nonexistent/chain.pem")
	if err == nil {
		t.Error("Expected error for non-existent chain file")
	}
}

// TestCreateTLSConfig_NilConfig 测试 nil 配置
func TestCreateTLSConfig_NilConfig(t *testing.T) {
	_, err := createTLSConfig(nil)
	if err == nil {
		t.Error("Expected error for nil config")
	}
}

// TestNewTLSManager_WithSessionTickets 测试启用 Session Tickets
func TestNewTLSManager_WithSessionTickets(t *testing.T) {
	tmpDir := t.TempDir()
	certPath := filepath.Join(tmpDir, "cert.pem")
	keyPath := filepath.Join(tmpDir, "key.pem")
	ticketKeyPath := filepath.Join(tmpDir, "ticket.key")

	cert, key := generateTestCert(t)
	if err := os.WriteFile(certPath, cert, 0o644); err != nil {
		t.Fatalf("Failed to write cert: %v", err)
	}
	if err := os.WriteFile(keyPath, key, 0o600); err != nil {
		t.Fatalf("Failed to write key: %v", err)
	}

	cfg := &config.SSLConfig{
		Cert: certPath,
		Key:  keyPath,
		SessionTickets: config.SessionTicketsConfig{
			Enabled:        true,
			KeyFile:        ticketKeyPath,
			RotateInterval: time.Hour,
			RetainKeys:     3,
		},
	}

	manager, err := NewTLSManager(cfg)
	if err != nil {
		t.Fatalf("NewTLSManager() failed: %v", err)
	}
	defer manager.Close()

	// 验证 Session Ticket 管理器已初始化
	manager.mu.RLock()
	stm := manager.sessionTicketMgr
	manager.mu.RUnlock()

	if stm == nil {
		t.Error("Expected session ticket manager to be initialized")
	}
}

// TestNewTLSManager_WithClientVerify 测试启用客户端验证
func TestNewTLSManager_WithClientVerify(t *testing.T) {
	tmpDir := t.TempDir()
	certPath := filepath.Join(tmpDir, "cert.pem")
	keyPath := filepath.Join(tmpDir, "key.pem")
	caPath := filepath.Join(tmpDir, "ca.pem")

	cert, key := generateTestCert(t)
	if err := os.WriteFile(certPath, cert, 0o644); err != nil {
		t.Fatalf("Failed to write cert: %v", err)
	}
	if err := os.WriteFile(keyPath, key, 0o600); err != nil {
		t.Fatalf("Failed to write key: %v", err)
	}

	// 创建 CA 证书
	_, _, caPEM := generateTestCA(t)
	if err := os.WriteFile(caPath, caPEM, 0o644); err != nil {
		t.Fatalf("Failed to write CA: %v", err)
	}

	cfg := &config.SSLConfig{
		Cert: certPath,
		Key:  keyPath,
		ClientVerify: config.ClientVerifyConfig{
			Enabled:     true,
			Mode:        "on",
			ClientCA:    caPath,
			VerifyDepth: 3,
		},
	}

	manager, err := NewTLSManager(cfg)
	if err != nil {
		t.Fatalf("NewTLSManager() failed: %v", err)
	}
	defer manager.Close()

	// 验证客户端验证器已初始化
	manager.mu.RLock()
	cv := manager.clientVerifier
	manager.mu.RUnlock()

	if cv == nil {
		t.Error("Expected client verifier to be initialized")
	}
}

// TestNewTLSManager_WithInvalidClientCA 测试无效的客户端 CA
func TestNewTLSManager_WithInvalidClientCA(t *testing.T) {
	tmpDir := t.TempDir()
	certPath := filepath.Join(tmpDir, "cert.pem")
	keyPath := filepath.Join(tmpDir, "key.pem")

	cert, key := generateTestCert(t)
	if err := os.WriteFile(certPath, cert, 0o644); err != nil {
		t.Fatalf("Failed to write cert: %v", err)
	}
	if err := os.WriteFile(keyPath, key, 0o600); err != nil {
		t.Fatalf("Failed to write key: %v", err)
	}

	cfg := &config.SSLConfig{
		Cert: certPath,
		Key:  keyPath,
		ClientVerify: config.ClientVerifyConfig{
			Enabled:  true,
			Mode:     "on",
			ClientCA: "/nonexistent/ca.pem",
		},
	}

	// 客户端验证配置失败不阻止 TLS 工作
	manager, err := NewTLSManager(cfg)
	if err != nil {
		t.Fatalf("NewTLSManager() should not fail for invalid client CA: %v", err)
	}
	defer manager.Close()

	// 客户端验证器应未初始化
	manager.mu.RLock()
	cv := manager.clientVerifier
	manager.mu.RUnlock()

	if cv != nil {
		t.Error("Expected client verifier to be nil for invalid CA")
	}
}

// TestNewTLSManager_WithOCSPAndIssuer 测试带颁发者证书的 OCSP
func TestNewTLSManager_WithOCSPAndIssuer(t *testing.T) {
	tmpDir := t.TempDir()
	certPath := filepath.Join(tmpDir, "cert.pem")
	keyPath := filepath.Join(tmpDir, "key.pem")
	chainPath := filepath.Join(tmpDir, "chain.pem")

	// 生成带 OCSP 服务器的证书
	certPEM, keyPEM := generateTestCertWithOCSP(t, []string{"http://ocsp.example.com"})
	if err := os.WriteFile(certPath, certPEM, 0o644); err != nil {
		t.Fatalf("Failed to write cert: %v", err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		t.Fatalf("Failed to write key: %v", err)
	}

	// 生成证书链（颁发者证书）
	chainCert, _ := generateTestCert(t)
	if err := os.WriteFile(chainPath, chainCert, 0o644); err != nil {
		t.Fatalf("Failed to write chain: %v", err)
	}

	cfg := &config.SSLConfig{
		Cert:         certPath,
		Key:          keyPath,
		CertChain:    chainPath,
		OCSPStapling: true,
	}

	manager, err := NewTLSManager(cfg)
	if err != nil {
		t.Fatalf("NewTLSManager() failed: %v", err)
	}
	defer manager.Close()

	// 验证 OCSP 管理器已初始化
	manager.mu.RLock()
	om := manager.ocspManager
	manager.mu.RUnlock()

	if om == nil {
		t.Error("Expected OCSP manager to be initialized")
	}
}
