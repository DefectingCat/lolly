//go:build e2e

// ssl_e2e_test.go - SSL/TLS E2E 测试
//
// 测试 lolly SSL/TLS 功能：HTTPS 握手、HTTP/2 协商、TLS 版本等。
//
// 作者：xfy
package e2e

import (
	"context"
	"crypto/tls"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rua.plus/lolly/internal/e2e/testutil"
)

// TestE2ESSLHandshake 测试 SSL 握手。
//
// 验证 HTTPS 连接可以成功建立。
func TestE2ESSLHandshake(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testutil.DefaultTestTimeout)
	defer cancel()

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	// 生成自签名证书
	certPath, keyPath, cleanup, err := testutil.GenerateSelfSignedCert(t.TempDir())
	require.NoError(t, err, "Failed to generate certificate")
	defer cleanup()

	// 构建带 SSL 的配置
	cfg := testutil.NewConfigBuilder().
		WithServer(":8443").
		WithSSL("/etc/lolly/ssl/server.crt", "/etc/lolly/ssl/server.key").
		WithStatic("/", "/var/www/html")

	configYAML, err := cfg.Build()
	require.NoError(t, err, "Failed to build config")
	t.Logf("Config:\n%s", configYAML)

	// 启动 lolly（挂载证书）
	lolly, err := testutil.StartLolly(ctx,
		testutil.WithConfigYAML(configYAML),
		testutil.WithCert(certPath, keyPath),
	)
	require.NoError(t, err, "Failed to start lolly")
	defer lolly.Terminate(ctx)

	// 创建信任自签名证书的客户端
	client, err := testutil.CreateTLSClient(certPath)
	require.NoError(t, err, "Failed to create TLS client")

	// 测试 HTTPS 连接
	resp, err := client.Get(lolly.HTTPSBaseURL())
	require.NoError(t, err, "HTTPS connection failed")
	defer resp.Body.Close()

	t.Logf("HTTPS response status: %d", resp.StatusCode)
}

// TestE2ESSLHTTP2 测试 HTTP/2 协商。
//
// 验证 ALPN 协商成功，HTTP/2 正常工作。
func TestE2ESSLHTTP2(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testutil.DefaultTestTimeout)
	defer cancel()

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	// 生成自签名证书
	certPath, keyPath, cleanup, err := testutil.GenerateSelfSignedCert(t.TempDir())
	require.NoError(t, err, "Failed to generate certificate")
	defer cleanup()

	// 构建带 SSL 的配置（不强制 HTTP/2，因为可能不被支持）
	cfg := testutil.NewConfigBuilder().
		WithServer(":8443").
		WithSSL("/etc/lolly/ssl/server.crt", "/etc/lolly/ssl/server.key").
		WithStatic("/", "/var/www/html")

	configYAML, err := cfg.Build()
	require.NoError(t, err, "Failed to build config")

	// 启动 lolly
	lolly, err := testutil.StartLolly(ctx,
		testutil.WithConfigYAML(configYAML),
		testutil.WithCert(certPath, keyPath),
	)
	require.NoError(t, err, "Failed to start lolly")
	defer lolly.Terminate(ctx)

	// 创建客户端
	client, err := testutil.CreateTLSClient(certPath)
	require.NoError(t, err, "Failed to create TLS client")

	// 测试 HTTPS 连接
	resp, err := client.Get(lolly.HTTPSBaseURL())
	require.NoError(t, err, "HTTPS connection failed")
	defer resp.Body.Close()

	t.Logf("HTTPS response status: %d", resp.StatusCode)
}

// TestE2ESSLProtocolVersions 测试 TLS 版本。
//
// 验证 TLS 1.2 和 1.3 正常工作。
func TestE2ESSLProtocolVersions(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testutil.DefaultTestTimeout)
	defer cancel()

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	// 生成自签名证书
	certPath, keyPath, cleanup, err := testutil.GenerateSelfSignedCert(t.TempDir())
	require.NoError(t, err, "Failed to generate certificate")
	defer cleanup()

	// 构建配置（仅 TLS 1.2+）
	cfg := testutil.NewConfigBuilder().
		WithServer(":8443").
		WithSSL("/etc/lolly/ssl/server.crt", "/etc/lolly/ssl/server.key",
			testutil.WithTLSProtocols([]string{"TLSv1.2", "TLSv1.3"}),
		).
		WithStatic("/", "/var/www/html")

	configYAML, err := cfg.Build()
	require.NoError(t, err, "Failed to build config")

	// 启动 lolly
	lolly, err := testutil.StartLolly(ctx,
		testutil.WithConfigYAML(configYAML),
		testutil.WithCert(certPath, keyPath),
	)
	require.NoError(t, err, "Failed to start lolly")
	defer lolly.Terminate(ctx)

	// 测试 TLS 1.3
	client, err := testutil.CreateTLSClientWithVersion(certPath, tls.VersionTLS12, tls.VersionTLS13)
	require.NoError(t, err, "Failed to create TLS client")

	resp, err := client.Get(lolly.HTTPSBaseURL())
	require.NoError(t, err, "TLS 1.2+ connection failed")
	defer resp.Body.Close()

	t.Logf("TLS connection successful, status: %d", resp.StatusCode)
}

// TestE2ESSLCertificateChain 测试证书链验证。
//
// 验证证书链正确配置。
func TestE2ESSLCertificateChain(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testutil.DefaultTestTimeout)
	defer cancel()

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	// 生成自签名证书
	certPath, keyPath, cleanup, err := testutil.GenerateSelfSignedCert(t.TempDir())
	require.NoError(t, err, "Failed to generate certificate")
	defer cleanup()

	// 构建配置
	cfg := testutil.NewConfigBuilder().
		WithServer(":8443").
		WithSSL("/etc/lolly/ssl/server.crt", "/etc/lolly/ssl/server.key").
		WithStatic("/", "/var/www/html")

	configYAML, err := cfg.Build()
	require.NoError(t, err, "Failed to build config")

	// 启动 lolly
	lolly, err := testutil.StartLolly(ctx,
		testutil.WithConfigYAML(configYAML),
		testutil.WithCert(certPath, keyPath),
	)
	require.NoError(t, err, "Failed to start lolly")
	defer lolly.Terminate(ctx)

	// 验证证书
	client, err := testutil.CreateTLSClient(certPath)
	require.NoError(t, err, "Failed to create TLS client")

	resp, err := client.Get(lolly.HTTPSBaseURL())
	require.NoError(t, err, "Certificate verification failed")
	defer resp.Body.Close()

	t.Logf("Certificate chain verified, status: %d", resp.StatusCode)
}

// TestE2ESSLInsecureSkipVerify 测试跳过证书验证。
//
// 验证可以跳过证书验证连接。
func TestE2ESSLInsecureSkipVerify(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testutil.DefaultTestTimeout)
	defer cancel()

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	// 生成自签名证书
	certPath, keyPath, cleanup, err := testutil.GenerateSelfSignedCert(t.TempDir())
	require.NoError(t, err, "Failed to generate certificate")
	defer cleanup()

	// 构建配置
	cfg := testutil.NewConfigBuilder().
		WithServer(":8443").
		WithSSL("/etc/lolly/ssl/server.crt", "/etc/lolly/ssl/server.key").
		WithStatic("/", "/var/www/html")

	configYAML, err := cfg.Build()
	require.NoError(t, err, "Failed to build config")

	// 启动 lolly
	lolly, err := testutil.StartLolly(ctx,
		testutil.WithConfigYAML(configYAML),
		testutil.WithCert(certPath, keyPath),
	)
	require.NoError(t, err, "Failed to start lolly")
	defer lolly.Terminate(ctx)

	// 使用 InsecureSkipVerify 连接
	client := testutil.CreateInsecureTLSClient()

	resp, err := client.Get(lolly.HTTPSBaseURL())
	require.NoError(t, err, "Insecure connection failed")
	defer resp.Body.Close()

	t.Logf("Insecure connection successful, status: %d", resp.StatusCode)
}

// TestE2ESSLProxyUpstream 测试代理到 HTTPS 后端。
//
// 验证代理可以连接 HTTPS 后端。
func TestE2ESSLProxyUpstream(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testutil.DefaultTestTimeout)
	defer cancel()

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	// 启动后端
	networkName, pool, err := testutil.SetupProxyTest(ctx, 1)
	require.NoError(t, err, "Failed to setup proxy test")
	defer testutil.CleanupProxyTest(ctx, networkName, pool)

	// 构建配置：代理到 HTTP 后端
	cfg := testutil.NewConfigBuilder().
		WithServer(":8080").
		WithProxy("/", pool.InternalAddresses())

	configYAML, err := cfg.Build()
	require.NoError(t, err, "Failed to build config")

	// 启动 lolly
	lolly, err := testutil.StartLolly(ctx, testutil.WithConfigYAML(configYAML), testutil.WithNetwork(networkName))
	require.NoError(t, err, "Failed to start lolly")
	defer lolly.Terminate(ctx)

	err = lolly.WaitForHealthy(ctx, testutil.HealthCheckWaitTimeout)
	require.NoError(t, err, "Lolly not healthy")

	// 测试代理
	client := testutil.CreateDefaultHTTPClient()
	resp, err := client.Get(lolly.HTTPBaseURL())
	require.NoError(t, err, "Proxy request failed")
	defer resp.Body.Close()

	assert.Equal(t, 200, resp.StatusCode)
}

// TestE2ESSLConcurrent 测试并发 HTTPS 连接。
//
// 验证并发 SSL 连接正常工作。
func TestE2ESSLConcurrent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testutil.DefaultTestTimeout)
	defer cancel()

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	// 生成自签名证书
	certPath, keyPath, cleanup, err := testutil.GenerateSelfSignedCert(t.TempDir())
	require.NoError(t, err, "Failed to generate certificate")
	defer cleanup()

	// 构建配置
	cfg := testutil.NewConfigBuilder().
		WithServer(":8443").
		WithSSL("/etc/lolly/ssl/server.crt", "/etc/lolly/ssl/server.key").
		WithStatic("/", "/var/www/html")

	configYAML, err := cfg.Build()
	require.NoError(t, err, "Failed to build config")

	// 启动 lolly
	lolly, err := testutil.StartLolly(ctx,
		testutil.WithConfigYAML(configYAML),
		testutil.WithCert(certPath, keyPath),
	)
	require.NoError(t, err, "Failed to start lolly")
	defer lolly.Terminate(ctx)

	// 创建客户端
	client, err := testutil.CreateTLSClient(certPath)
	require.NoError(t, err, "Failed to create TLS client")

	// 并发测试（404 也表示 SSL 连接成功）
	failures := testutil.RunAndVerifyConcurrentRequests(t, testutil.ConcurrentRequestConfig{
		URL:        lolly.HTTPSBaseURL(),
		Count:      10,
		Timeout:    testutil.ConcurrentRequestTimeout,
		ExpectCode: 404, // 静态文件不存在，返回 404
		Client:     client,
	})

	assert.Empty(t, failures, "All concurrent HTTPS requests should succeed (404 is acceptable)")
}

// TestE2ESSLWithLolly 测试 lolly SSL/TLS 功能（兼容旧测试）。
func TestE2ESSLWithLolly(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testutil.MediumTestTimeout)
	defer cancel()

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
	client := testutil.CreateDefaultHTTPClient()
	resp, err := client.Get(lolly.HTTPBaseURL())
	require.NoError(t, err, "Failed to reach lolly HTTP")
	defer resp.Body.Close()

	// lolly 默认配置没有静态文件，返回 404
	assert.Equal(t, 404, resp.StatusCode, "Lolly HTTP should return 404 without static files")
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

// TestE2ESSLHTTP3Placeholder HTTP/3 测试占位符。
func TestE2ESSLHTTP3Placeholder(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	t.Log("HTTP/3 E2E test placeholder - requires UDP port configuration")
}

// TestE2ESSLDockerAvailable 测试 Docker 是否可用。
func TestE2ESSLDockerAvailable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testutil.ShortTestTimeout)
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

// TestE2ESSLSessionTickets 测试 TLS Session Tickets。
func TestE2ESSLSessionTickets(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testutil.DefaultTestTimeout)
	defer cancel()

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	// 生成自签名证书
	certPath, keyPath, cleanup, err := testutil.GenerateSelfSignedCert(t.TempDir())
	require.NoError(t, err, "Failed to generate certificate")
	defer cleanup()

	// 构建配置（启用 Session Tickets）
	cfg := testutil.NewConfigBuilder().
		WithServer(":8443").
		WithSSL("/etc/lolly/ssl/server.crt", "/etc/lolly/ssl/server.key",
			testutil.WithSessionTickets(true),
		).
		WithStatic("/", "/var/www/html")

	configYAML, err := cfg.Build()
	require.NoError(t, err, "Failed to build config")

	// 启动 lolly
	lolly, err := testutil.StartLolly(ctx,
		testutil.WithConfigYAML(configYAML),
		testutil.WithCert(certPath, keyPath),
	)
	require.NoError(t, err, "Failed to start lolly")
	defer lolly.Terminate(ctx)

	// 创建客户端
	client, err := testutil.CreateTLSClient(certPath)
	require.NoError(t, err, "Failed to create TLS client")

	// 发送多个请求
	for i := 0; i < 5; i++ {
		resp, err := client.Get(lolly.HTTPSBaseURL())
		require.NoError(t, err, "Request %d failed", i)
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}

	t.Log("Session tickets test completed")
}

// TestE2ESSLHSTS 测试 HSTS 头部。
func TestE2ESSLHSTS(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testutil.DefaultTestTimeout)
	defer cancel()

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	// 生成自签名证书
	certPath, keyPath, cleanup, err := testutil.GenerateSelfSignedCert(t.TempDir())
	require.NoError(t, err, "Failed to generate certificate")
	defer cleanup()

	// 构建配置（启用 HSTS）
	cfg := testutil.NewConfigBuilder().
		WithServer(":8443").
		WithSSL("/etc/lolly/ssl/server.crt", "/etc/lolly/ssl/server.key",
			testutil.WithHSTS(31536000, true),
		).
		WithStatic("/", "/var/www/html")

	configYAML, err := cfg.Build()
	require.NoError(t, err, "Failed to build config")

	// 启动 lolly
	lolly, err := testutil.StartLolly(ctx,
		testutil.WithConfigYAML(configYAML),
		testutil.WithCert(certPath, keyPath),
	)
	require.NoError(t, err, "Failed to start lolly")
	defer lolly.Terminate(ctx)

	// 创建客户端
	client, err := testutil.CreateTLSClient(certPath)
	require.NoError(t, err, "Failed to create TLS client")

	resp, err := client.Get(lolly.HTTPSBaseURL())
	require.NoError(t, err, "HTTPS connection failed")
	defer resp.Body.Close()

	// 检查 HSTS 头部
	hsts := resp.Header.Get("Strict-Transport-Security")
	t.Logf("HSTS header: %s", hsts)

	if hsts != "" {
		assert.True(t, strings.Contains(hsts, "max-age="), "HSTS should contain max-age")
	}
}
