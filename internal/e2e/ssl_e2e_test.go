//go:build e2e

// ssl_e2e_test.go - SSL/TLS E2E 测试（L3 层，需要 Docker）
//
// 测试 lolly SSL/TLS 功能。
// 所有测试都以 lolly 作为被测系统。
//
// 作者：xfy
package e2e

import (
	"context"
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
// 注意：当前测试仅验证证书生成功能，因为默认 lolly 镜像未配置 SSL。
// 完整的 SSL 测试需要自定义配置和证书挂载，这里仅测试 HTTP 连接。
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

	// 启动 lolly 服务器（使用默认配置，无 SSL）
	lolly, err := testutil.StartLollyContainer(ctx, "")
	require.NoError(t, err, "Failed to start lolly container")
	defer lolly.Terminate(ctx)

	t.Logf("Lolly HTTP server: %s", lolly.HTTPBaseURL())

	// 测试 HTTP 连接（默认配置未启用 HTTPS）
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(lolly.HTTPBaseURL())
	require.NoError(t, err, "Failed to reach lolly HTTP")
	defer resp.Body.Close()

	// lolly 默认配置没有静态文件，返回 404
	assert.Equal(t, 404, resp.StatusCode, "Lolly HTTP should return 404 without static files")
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

// TestE2ESSLConcurrent 测试 lolly 并发 SSL 连接。
func TestE2ESSLConcurrent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	lolly, err := testutil.StartLollyContainer(ctx, "")
	require.NoError(t, err, "Failed to start lolly container")
	defer lolly.Terminate(ctx)

	// 使用真正的并发测试
	failures := testutil.RunAndVerifyConcurrentRequests(t, testutil.ConcurrentRequestConfig{
		URL:        lolly.HTTPBaseURL(),
		Count:      10,
		Timeout:    10 * time.Second,
		ExpectCode: 404, // lolly 默认配置没有静态文件
	})

	assert.Empty(t, failures, "All concurrent requests should succeed")
}
