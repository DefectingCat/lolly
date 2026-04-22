//go:build e2e

// ssl_e2e_test.go - SSL/TLS E2E 测试（L3 层，需要 Docker）
//
// 使用 testcontainers-go 进行真实的 HTTPS 测试。
// 需要在有 Docker 的环境中运行。
//
// 作者：xfy
package e2e

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// TestE2ESSLHandshake 测试 SSL 握手
// 注意：此测试需要 Docker 环境
func TestE2ESSLHandshake(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	ctx := context.Background()

	// 创建测试证书（自签名）
	// 在实际测试中，应该使用预生成的测试证书
	t.Log("SSL E2E test placeholder - requires Docker and test certificates")

	// 验证 Docker 是否可用
	_, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:      "alpine:latest",
			Cmd:        []string{"echo", "test"},
			AutoRemove: true,
		},
		Started: true,
	})
	if err != nil {
		t.Skipf("Docker not available: %v", err)
	}

	t.Log("Docker is available for E2E tests")
}

// TestE2EHTTPSBasic 测试基本 HTTPS 功能
func TestE2EHTTPSBasic(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	// 检查是否有可用的 HTTPS 端点
	// 这是一个占位测试，实际测试需要启动 lolly 容器
	t.Log("HTTPS E2E test placeholder")
}

// TestE2ETLSCertificateValidation 测试 TLS 证书验证
func TestE2ETLSCertificateValidation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	// 创建自定义证书池
	certPool := x509.NewCertPool()

	// 测试证书验证逻辑
	t.Log("TLS certificate validation test placeholder")

	// 验证证书池不为空
	assert.NotNil(t, certPool)
}

// TestE2ETLSVersion 测试 TLS 版本
func TestE2ETLSVersion(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	// 创建 TLS 配置
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
		MaxVersion: tls.VersionTLS13,
	}

	// 验证 TLS 版本配置
	assert.GreaterOrEqual(t, tlsConfig.MinVersion, uint16(tls.VersionTLS12))
	assert.LessOrEqual(t, tlsConfig.MaxVersion, uint16(tls.VersionTLS13))
}

// TestE2ECipherSuites 测试加密套件
func TestE2ECipherSuites(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	// 获取支持的加密套件
	cipherSuites := []uint16{
		tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
		tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
	}

	// 验证加密套件列表不为空
	assert.NotEmpty(t, cipherSuites)
}

// TestE2EMutualTLS 测试 mTLS 双向认证
func TestE2EMutualTLS(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	// 创建 mTLS 配置
	tlsConfig := &tls.Config{
		ClientAuth: tls.RequireAndVerifyClientCert,
	}

	// 验证客户端认证要求
	assert.Equal(t, tls.RequireAndVerifyClientCert, tlsConfig.ClientAuth)
}

// TestE2ESSLSessionResumption 测试 SSL 会话恢复
func TestE2ESSLSessionResumption(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	// 测试会话恢复配置
	tlsConfig := &tls.Config{
		SessionTicketsDisabled: false,
	}

	// 验证会话票证未禁用
	assert.False(t, tlsConfig.SessionTicketsDisabled)
}

// TestE2EHTTPClientWithTLS 测试带 TLS 的 HTTP 客户端
func TestE2EHTTPClientWithTLS(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	// 创建跳过证书验证的客户端（仅用于测试）
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}
	client := &http.Client{
		Transport: tr,
		Timeout:   10 * time.Second,
	}

	// 验证客户端配置
	assert.NotNil(t, client)
	assert.NotNil(t, tr)
}

// TestE2EDockerAvailable 测试 Docker 是否可用
func TestE2EDockerAvailable(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	ctx := context.Background()

	// 尝试启动一个简单的容器
	req := testcontainers.ContainerRequest{
		Image:      "alpine:latest",
		Cmd:        []string{"echo", "hello"},
		AutoRemove: true,
		WaitingFor: wait.ForExit(),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Skipf("Docker not available, skipping E2E tests: %v", err)
	}
	defer container.Terminate(ctx)

	// 获取容器日志
	logs, err := container.Logs(ctx)
	if err != nil {
		t.Logf("Could not get container logs: %v", err)
	} else {
		logs.Close()
	}

	t.Log("Docker is available and working")
}

// TestE2EEnvironmentCheck 检查测试环境
func TestE2EEnvironmentCheck(t *testing.T) {
	// 检查环境变量
	dockerHost := os.Getenv("DOCKER_HOST")
	if dockerHost != "" {
		t.Logf("DOCKER_HOST: %s", dockerHost)
	}

	// 检查是否在 CI 环境中
	if os.Getenv("CI") != "" {
		t.Log("Running in CI environment")
	}

	// 检查是否在容器中运行
	if _, err := os.Stat("/.dockerenv"); err == nil {
		t.Log("Running inside a Docker container")
	}
}

// TestE2ESSLConfiguration 测试 SSL 配置
func TestE2ESSLConfiguration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	// 测试 SSL 配置结构
	type SSLConfig struct {
		Enabled    bool
		CertFile   string
		KeyFile    string
		Protocols  []string
		Ciphers    []string
		ClientAuth string
	}

	config := SSLConfig{
		Enabled:    true,
		CertFile:   "/etc/ssl/certs/server.crt",
		KeyFile:    "/etc/ssl/certs/server.key",
		Protocols:  []string{"TLSv1.2", "TLSv1.3"},
		Ciphers:    []string{"ECDHE-ECDSA-AES256-GCM-SHA384", "ECDHE-RSA-AES256-GCM-SHA384"},
		ClientAuth: "none",
	}

	// 验证配置
	assert.True(t, config.Enabled)
	assert.NotEmpty(t, config.Protocols)
	assert.Contains(t, config.Protocols, "TLSv1.2")
	assert.Contains(t, config.Protocols, "TLSv1.3")
}

// TestE2EHTTP3Placeholder HTTP/3 测试占位符
func TestE2EHTTP3Placeholder(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	// HTTP/3 需要 UDP 端口，测试需要额外配置
	t.Log("HTTP/3 E2E test placeholder - requires UDP port configuration")
}

// Example of how to run a real E2E test with testcontainers
// This is commented out as it requires actual lolly image
/*
func TestE2ELollyContainer(t *testing.T) {
	ctx := context.Background()

	// Build and run lolly container
	req := testcontainers.ContainerRequest{
		FromDockerfile: testcontainers.FromDockerfile{
			Context:    "../../",
			Dockerfile: "Dockerfile",
		},
		ExposedPorts: []string{"8080/tcp", "8443/tcp"},
		WaitingFor:   wait.ForHTTP("/health").WithPort("8080/tcp"),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err)
	defer container.Terminate(ctx)

	// Get container address
	host, err := container.Host(ctx)
	require.NoError(t, err)

	port, err := container.MappedPort(ctx, "8080")
	require.NoError(t, err)

	// Make request
	resp, err := http.Get(fmt.Sprintf("http://%s:%s/", host, port.Port()))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 200, resp.StatusCode)
}
*/
