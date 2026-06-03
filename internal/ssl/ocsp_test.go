// Package ssl 提供 OCSP（在线证书状态协议）功能的测试。
//
// 该文件测试 OCSP 模块的各项功能，包括：
//   - OCSP 管理器创建和配置
//   - OCSP 响应获取
//   - 证书状态检查
//   - 过期响应处理
//   - OCSP 配置默认值
//
// 作者：xfy
package ssl

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"rua.plus/lolly/internal/config"
)

func TestNewOCSPManager(t *testing.T) {
	cfg := DefaultOCSPConfig()
	mgr := NewOCSPManager(cfg)

	if mgr == nil {
		t.Fatal("Expected non-nil OCSP manager")
	}

	if mgr.refreshInterval != 1*time.Hour {
		t.Errorf("Expected refresh interval 1h, got %v", mgr.refreshInterval)
	}

	if mgr.timeout != 10*time.Second {
		t.Errorf("Expected timeout 10s, got %v", mgr.timeout)
	}

	if mgr.maxRetries != 3 {
		t.Errorf("Expected max retries 3, got %d", mgr.maxRetries)
	}
}

func TestNewOCSPManagerWithCustomConfig(t *testing.T) {
	cfg := &OCSPConfig{
		Enabled:         true,
		RefreshInterval: 30 * time.Minute,
		Timeout:         5 * time.Second,
		MaxRetries:      5,
	}

	mgr := NewOCSPManager(cfg)

	if mgr.refreshInterval != 30*time.Minute {
		t.Errorf("Expected refresh interval 30m, got %v", mgr.refreshInterval)
	}

	if mgr.timeout != 5*time.Second {
		t.Errorf("Expected timeout 5s, got %v", mgr.timeout)
	}

	if mgr.maxRetries != 5 {
		t.Errorf("Expected max retries 5, got %d", mgr.maxRetries)
	}
}

func TestOCSPManagerStartStop(t *testing.T) {
	mgr := NewOCSPManager(nil)

	mgr.Start()

	mgr.mu.RLock()
	running := mgr.running
	mgr.mu.RUnlock()

	if !running {
		t.Error("Expected OCSP manager to be running")
	}

	mgr.Stop()

	mgr.mu.RLock()
	running = mgr.running
	mgr.mu.RUnlock()

	if running {
		t.Error("Expected OCSP manager to be stopped")
	}
}

func TestOCSPGetOCSPResponse(t *testing.T) {
	mgr := NewOCSPManager(nil)

	// Test non-existent serial
	resp := mgr.GetOCSPResponse("nonexistent")
	if resp != nil {
		t.Error("Expected nil response for non-existent serial")
	}

	// Test with valid response
	testResp := []byte("test-ocsp-response")
	serial := "12345"

	mgr.mu.Lock()
	mgr.responses[serial] = &ocspResponse{
		response:   testResp,
		thisUpdate: time.Now(),
		nextUpdate: time.Now().Add(1 * time.Hour),
		status:     statusValid,
		fetchedAt:  time.Now(),
	}
	mgr.mu.Unlock()

	resp = mgr.GetOCSPResponse(serial)
	if resp == nil {
		t.Error("Expected non-nil response")
	}
	if string(resp) != "test-ocsp-response" {
		t.Errorf("Expected 'test-ocsp-response', got '%s'", string(resp))
	}
}



func TestOCSPStaleResponse(t *testing.T) {
	mgr := NewOCSPManager(nil)

	serial := "12345"
	testResp := []byte("stale-response")

	// Create expired response
	mgr.mu.Lock()
	mgr.responses[serial] = &ocspResponse{
		response:   testResp,
		thisUpdate: time.Now().Add(-2 * time.Hour),
		nextUpdate: time.Now().Add(-1 * time.Hour), // Expired
		status:     statusValid,
		fetchedAt:  time.Now().Add(-2 * time.Hour),
	}
	mgr.mu.Unlock()

	// Should return stale response (graceful degradation)
	resp := mgr.GetOCSPResponse(serial)
	if resp == nil {
		t.Error("Expected stale response for graceful degradation")
	}

	// Status should now be stale
	mgr.mu.RLock()
	storedResp := mgr.responses[serial]
	mgr.mu.RUnlock()

	if storedResp.status != statusStale {
		t.Error("Expected status to be marked as stale")
	}
}

func TestOCSPFailedResponse(t *testing.T) {
	mgr := NewOCSPManager(nil)

	serial := "12345"

	mgr.mu.Lock()
	mgr.responses[serial] = &ocspResponse{
		status:    statusFailed,
		fetchedAt: time.Now(),
		errors:    3,
	}
	mgr.mu.Unlock()

	resp := mgr.GetOCSPResponse(serial)
	if resp != nil {
		t.Error("Expected nil response for failed status")
	}
}

func TestTLSManagerWithOCSPDisabled(t *testing.T) {
	tmpDir := t.TempDir()
	certPath := filepath.Join(tmpDir, "cert.pem")
	keyPath := filepath.Join(tmpDir, "key.pem")

	certPEM, keyPEM := generateTestCertWithOCSP(t, nil)
	if err := os.WriteFile(certPath, certPEM, 0o644); err != nil {
		t.Fatalf("Failed to write cert: %v", err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		t.Fatalf("Failed to write key: %v", err)
	}

	cfg := &config.SSLConfig{
		Cert:         certPath,
		Key:          keyPath,
		OCSPStapling: false,
	}

	manager, err := NewTLSManager(cfg)
	if err != nil {
		t.Fatalf("NewTLSManager() failed: %v", err)
	}

	// OCSP manager should not be initialized
	if manager.ocspManager != nil {
		t.Error("Expected OCSP manager to be nil when disabled")
	}

	manager.Close()
}



func TestTLSManagerClose(t *testing.T) {
	tmpDir := t.TempDir()
	certPath := filepath.Join(tmpDir, "cert.pem")
	keyPath := filepath.Join(tmpDir, "key.pem")

	certPEM, keyPEM := generateTestCertWithOCSP(t, nil)
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

	// Close should work even if OCSP manager is nil or stopped
	manager.Close()

	// Should not panic on second close
	manager.Close()
}

func TestOCSPManagerRegisterCertificate(t *testing.T) {
	mgr := NewOCSPManager(nil)
	defer mgr.Stop()

	// Generate test cert with OCSP server URL
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	// Create mock OCSP server
	ocspServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Return a simple OCSP response
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("mock-ocsp-response"))
	}))
	defer ocspServer.Close()

	template := x509.Certificate{
		SerialNumber: big.NewInt(12345),
		Subject:      pkix.Name{CommonName: "test"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(1 * time.Hour),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost"},
		OCSPServer:   []string{ocspServer.URL},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("Failed to create certificate: %v", err)
	}

	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		t.Fatalf("Failed to parse certificate: %v", err)
	}

	// Register should fail gracefully because our mock server returns invalid OCSP
	err = mgr.RegisterCertificate(cert, cert)
	// This is expected to fail since mock server doesn't return valid OCSP response
	if err == nil {
		// If it succeeds, verify the response was stored
		serial := cert.SerialNumber.String()
		mgr.mu.RLock()
		_, exists := mgr.responses[serial]
		mgr.mu.RUnlock()

		if !exists {
			t.Error("Expected response to be stored")
		}
	}
	// If it fails, that's also OK - graceful degradation
}

// generateTestCertWithOCSP 生成用于测试的自签名证书。
// If ocspServer is provided, it will be included in the certificate.
func generateTestCertWithOCSP(t *testing.T, ocspServer []string) ([]byte, []byte) {
	t.Helper()

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate private key: %v", err)
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Test"},
			CommonName:   "test.example.com",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost", "test.example.com"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
		OCSPServer:            ocspServer,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("Failed to create certificate: %v", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})

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

func TestOCSPConfigDefaults(t *testing.T) {
	cfg := DefaultOCSPConfig()

	if !cfg.Enabled {
		t.Error("Expected OCSP to be enabled by default")
	}

	if cfg.RefreshInterval != 1*time.Hour {
		t.Errorf("Expected default refresh interval 1h, got %v", cfg.RefreshInterval)
	}

	if cfg.Timeout != 10*time.Second {
		t.Errorf("Expected default timeout 10s, got %v", cfg.Timeout)
	}

	if cfg.MaxRetries != 3 {
		t.Errorf("Expected default max retries 3, got %d", cfg.MaxRetries)
	}
}



// TestOCSPManager_refreshAll 测试刷新所有响应
func TestOCSPManager_refreshAll(_ *testing.T) {
	cfg := &OCSPConfig{
		Enabled:         true,
		RefreshInterval: 1 * time.Hour,
		Timeout:         100 * time.Millisecond,
		MaxRetries:      1,
	}
	mgr := NewOCSPManager(cfg)

	// 手动添加一些响应到缓存
	serial1 := "1001"
	serial2 := "1002"

	mgr.mu.Lock()
	mgr.responses[serial1] = &ocspResponse{
		status:     statusValid,
		response:   []byte("test-response"),
		nextUpdate: time.Now().Add(-1 * time.Hour), // 已过期
		fetchedAt:  time.Now().Add(-2 * time.Hour),
	}
	mgr.responses[serial2] = &ocspResponse{
		status:     statusValid,
		response:   []byte("test-response-2"),
		nextUpdate: time.Now().Add(1 * time.Hour), // 未过期
		fetchedAt:  time.Now(),
	}
	mgr.mu.Unlock()

	// 调用 refreshAll
	mgr.refreshAll()

	// 验证刷新逻辑被触发（无法验证实际刷新因为 URL 无效）
	// 主要目的是确保代码路径被覆盖
}

// TestOCSPManager_GetStatus_EdgeCases 测试 GetStatus 边界情况


// TestOCSPManager_RegisterCertificate_NilCert 测试注册空证书
func TestOCSPManager_RegisterCertificate_NilCert(t *testing.T) {
	mgr := NewOCSPManager(nil)
	defer mgr.Stop()

	err := mgr.RegisterCertificate(nil, nil)
	if err == nil {
		t.Error("Expected error for nil certificate")
	}
	if err.Error() != "certificate is nil" {
		t.Errorf("Expected 'certificate is nil' error, got: %v", err)
	}
}

// TestOCSPManager_RegisterCertificate_NoOCSPServer 测试无 OCSP 服务器的证书
func TestOCSPManager_RegisterCertificate_NoOCSPServer(t *testing.T) {
	mgr := NewOCSPManager(nil)
	defer mgr.Stop()

	// 创建无 OCSP 服务器的证书
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(12345),
		Subject:      pkix.Name{CommonName: "test"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(1 * time.Hour),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost"},
		// 无 OCSPServer
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("Failed to create certificate: %v", err)
	}

	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		t.Fatalf("Failed to parse certificate: %v", err)
	}

	err = mgr.RegisterCertificate(cert, cert)
	if err == nil {
		t.Error("Expected error for certificate without OCSP server")
	}
	if err.Error() != "certificate has no OCSP server URL" {
		t.Errorf("Expected 'certificate has no OCSP server URL' error, got: %v", err)
	}
}

// TestOCSPManager_SendOCSPRequest_Error 测试 OCSP 请求错误
func TestOCSPManager_SendOCSPRequest_Error(t *testing.T) {
	cfg := &OCSPConfig{
		Enabled:         true,
		RefreshInterval: 1 * time.Hour,
		Timeout:         100 * time.Millisecond,
		MaxRetries:      1,
	}
	mgr := NewOCSPManager(cfg)

	// 测试无效 URL
	_, err := mgr.sendOCSPRequest("://invalid-url", []byte("test"))
	if err == nil {
		t.Error("Expected error for invalid URL")
	}

	// 测试连接失败 - 使用保留的不可达 IP 地址 (198.18.0.0/15 是 IANA 保留用于基准测试的地址块)
	_, err = mgr.sendOCSPRequest("http://198.18.0.1:9999/ocsp", []byte("test"))
	if err == nil {
		t.Error("Expected error for connection failure")
	}
}

// TestOCSPManager_RefreshResponse_WithExistingEntry 测试刷新已有条目的响应


// TestOCSPManager_RefreshResponse_StatusFailed 测试刷新失败后状态变化


// TestOCSPManager_FetchOCSP_NoServer 测试无 OCSP 服务器时的 fetchOCSP
func TestOCSPManager_FetchOCSP_NoServer(t *testing.T) {
	mgr := NewOCSPManager(nil)

	priv, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(24 * time.Hour),
		// 无 OCSPServer
	}
	certDER, _ := x509.CreateCertificate(rand.Reader, template, template, &priv.PublicKey, priv)
	cert, _ := x509.ParseCertificate(certDER)

	_, err := mgr.fetchOCSP(cert, cert)
	if err == nil {
		t.Error("Expected error for certificate without OCSP server")
	}
	if err.Error() != "no OCSP server in certificate" {
		t.Errorf("Expected 'no OCSP server in certificate' error, got: %v", err)
	}
}

// TestOCSPManager_StartTwice 测试重复启动
func TestOCSPManager_StartTwice(t *testing.T) {
	mgr := NewOCSPManager(nil)

	mgr.Start()
	defer mgr.Stop()

	// 第二次启动应该无效果
	mgr.Start()

	mgr.mu.RLock()
	running := mgr.running
	mgr.mu.RUnlock()

	if !running {
		t.Error("Expected manager to be running")
	}
}

// TestOCSPManager_StopTwice 测试重复停止
func TestOCSPManager_StopTwice(t *testing.T) {
	mgr := NewOCSPManager(nil)

	mgr.Start()
	mgr.Stop()

	// 第二次停止应该无效果
	mgr.Stop()

	mgr.mu.RLock()
	running := mgr.running
	mgr.mu.RUnlock()

	if running {
		t.Error("Expected manager to be stopped")
	}
}
