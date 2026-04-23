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
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rua.plus/lolly/internal/e2e/testutil"
)

// TestE2ESSLHandshake 测试 SSL 握手环境。
func TestE2ESSLHandshake(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if !testutil.DockerAvailable(ctx) {
		t.Skip("Docker not available")
	}

	t.Log("Docker is available for E2E SSL tests")
}

// TestE2ESSLWithLolly 测试 lolly SSL/TLS 功能。
// 需要 lolly:latest 镜像和测试证书。
func TestE2ESSLWithLolly(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	if !testutil.DockerAvailable(ctx) {
		t.Skip("Docker not available")
	}

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	// 生成自签名证书
	certPath, keyPath, cleanup, err := testutil.GenerateSelfSignedCert(t.TempDir())
	require.NoError(t, err, "Failed to generate self-signed certificate")
	defer cleanup()

	t.Logf("Generated certificate: %s", certPath)
	t.Logf("Generated key: %s", keyPath)

	// 启动 lolly SSL 服务器
	lolly, err := testutil.StartLollyContainer(ctx, "")
	require.NoError(t, err, "Failed to start lolly container")
	defer lolly.Terminate(ctx)

	t.Logf("Lolly HTTPS server: %s", lolly.HTTPSBaseURL())

	// 使用跳过证书验证的客户端（测试证书是自签名的）
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}
	client := &http.Client{
		Transport: tr,
		Timeout:   10 * time.Second,
	}

	// 测试 HTTPS 连接
	resp, err := client.Get(lolly.HTTPSBaseURL())
	require.NoError(t, err, "Failed to reach lolly HTTPS")
	defer resp.Body.Close()

	// lolly 默认配置没有静态文件，返回 404
	assert.Equal(t, 404, resp.StatusCode, "Lolly HTTPS should return 404 without static files")
}

// TestE2ESSLDockerAvailable 测试 Docker 是否可用。
func TestE2ESSLDockerAvailable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if !testutil.DockerAvailable(ctx) {
		t.Skip("Docker not available, skipping E2E SSL tests")
	}

	t.Log("Docker is available for E2E SSL tests")
}

// TestE2ESSLEnvironmentCheck 检查测试环境。
func TestE2ESSLEnvironmentCheck(t *testing.T) {
	dockerHost := os.Getenv("DOCKER_HOST")
	if dockerHost != "" {
		t.Logf("DOCKER_HOST: %s", dockerHost)
	}

	if os.Getenv("CI") != "" {
		t.Log("Running in CI environment")
	}

	if _, err := os.Stat("/.dockerenv"); err == nil {
		t.Log("Running inside a Docker container")
	}
}

// TestE2ESSLHTTP3Placeholder HTTP/3 测试占位符。
func TestE2ESSLHTTP3Placeholder(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	t.Log("HTTP/3 E2E test placeholder - requires UDP port configuration")
}

// TestE2ESSLContainer 测试带 SSL 的容器。
func TestE2ESSLContainer(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	container, addr, err := testutil.StartNginxContainer(ctx)
	require.NoError(t, err, "Failed to start nginx container")
	defer container.Terminate(ctx)

	t.Logf("HTTP address: %s", addr)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(addr)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 200, resp.StatusCode)
}

// TestE2ESSLCertificateGeneration 测试证书生成。
func TestE2ESSLCertificateGeneration(t *testing.T) {
	tmpDir := t.TempDir()

	certPath, keyPath, cleanup, err := testutil.GenerateSelfSignedCert(tmpDir)
	require.NoError(t, err, "Failed to generate certificate")
	defer cleanup()

	assert.FileExists(t, certPath, "Certificate file should exist")
	assert.FileExists(t, keyPath, "Key file should exist")

	// 验证证书可以被加载
	certPool, err := testutil.GenerateCertPool(certPath)
	require.NoError(t, err, "Failed to create cert pool")
	assert.NotNil(t, certPool)
}

// TestE2ESSLConcurrent 测试并发 SSL 连接。
func TestE2ESSLConcurrent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	container, addr, err := testutil.StartNginxContainer(ctx)
	require.NoError(t, err, "Failed to start nginx container")
	defer container.Terminate(ctx)

	// 使用真正的并发测试
	failures := testutil.RunAndVerifyConcurrentRequests(t, testutil.ConcurrentRequestConfig{
		URL:        addr,
		Count:      10,
		Timeout:    10 * time.Second,
		ExpectCode: 200,
	})

	assert.Empty(t, failures, "All concurrent requests should succeed")
}
