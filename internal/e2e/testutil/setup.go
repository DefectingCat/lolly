//go:build e2e

// Package testutil 提供 E2E 测试的工具函数。
//
// 包含统一的测试环境设置函数。
//
// 作者：xfy
package testutil

import (
	"context"
	"net/http"
	"testing"
	"time"
)

// E2ETestEnv E2E 测试环境。
//
// 封装测试所需的资源和清理函数。
type E2ETestEnv struct {
	Ctx     context.Context
	Network string
	Pool    *BackendPool
	Lolly   *LollyContainer
	Client  *http.Client
	cleanup func()
}

// SetupE2ETest 设置 E2E 测试环境。
//
// 自动处理镜像检查、后端启动、lolly 启动和资源清理。
// 使用 t.Cleanup() 确保资源正确释放。
//
// 参数：
//   - t: 测试对象
//   - backendCount: 后端数量
//   - cfgBuilder: 配置构建函数，接收后端池返回 YAML 配置
//
// 使用示例：
//
//	env := testutil.SetupE2ETest(t, 2, func(pool *testutil.BackendPool) string {
//	    cfg := testutil.NewConfigBuilder().
//	        WithServer(":8080").
//	        WithProxy("/", pool.InternalAddresses())
//	    yaml, _ := cfg.Build()
//	    return yaml
//	})
//	defer env.Cleanup()
func SetupE2ETest(t *testing.T, backendCount int, cfgBuilder func(*BackendPool) string) *E2ETestEnv {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), DefaultTestTimeout)

	if !LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	network, pool, err := SetupProxyTest(ctx, backendCount)
	if err != nil {
		cancel()
		t.Fatalf("Failed to setup proxy test: %v", err)
	}

	cfgYAML := cfgBuilder(pool)

	lolly, err := StartLolly(ctx,
		WithConfigYAML(cfgYAML),
		WithNetwork(network),
	)
	if err != nil {
		CleanupProxyTest(ctx, network, pool)
		cancel()
		t.Fatalf("Failed to start lolly: %v", err)
	}

	if err := lolly.WaitForHealthy(ctx, HealthCheckWaitTimeout); err != nil {
		lolly.Terminate(ctx)
		CleanupProxyTest(ctx, network, pool)
		cancel()
		t.Fatalf("Lolly not healthy: %v", err)
	}

	env := &E2ETestEnv{
		Ctx:     ctx,
		Network: network,
		Pool:    pool,
		Lolly:   lolly,
		Client:  CreateDefaultHTTPClient(),
	}

	env.cleanup = func() {
		lolly.Terminate(ctx)
		CleanupProxyTest(ctx, network, pool)
		cancel()
	}

	t.Cleanup(env.Cleanup)

	return env
}

// SetupE2ETestWithTimeout 设置带自定义超时的 E2E 测试环境。
func SetupE2ETestWithTimeout(t *testing.T, backendCount int, timeout time.Duration, cfgBuilder func(*BackendPool) string) *E2ETestEnv {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)

	if !LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	network, pool, err := SetupProxyTest(ctx, backendCount)
	if err != nil {
		cancel()
		t.Fatalf("Failed to setup proxy test: %v", err)
	}

	cfgYAML := cfgBuilder(pool)

	lolly, err := StartLolly(ctx,
		WithConfigYAML(cfgYAML),
		WithNetwork(network),
	)
	if err != nil {
		CleanupProxyTest(ctx, network, pool)
		cancel()
		t.Fatalf("Failed to start lolly: %v", err)
	}

	if err := lolly.WaitForHealthy(ctx, HealthCheckWaitTimeout); err != nil {
		lolly.Terminate(ctx)
		CleanupProxyTest(ctx, network, pool)
		cancel()
		t.Fatalf("Lolly not healthy: %v", err)
	}

	env := &E2ETestEnv{
		Ctx:     ctx,
		Network: network,
		Pool:    pool,
		Lolly:   lolly,
		Client:  CreateDefaultHTTPClient(),
	}

	env.cleanup = func() {
		lolly.Terminate(ctx)
		CleanupProxyTest(ctx, network, pool)
		cancel()
	}

	t.Cleanup(env.Cleanup)

	return env
}

// Cleanup 手动清理资源。
func (e *E2ETestEnv) Cleanup() {
	if e.cleanup != nil {
		e.cleanup()
		e.cleanup = nil
	}
}

// HTTPURL 返回 lolly HTTP 地址。
func (e *E2ETestEnv) HTTPURL() string {
	return e.Lolly.HTTPBaseURL()
}

// HTTPSURL 返回 lolly HTTPS 地址。
func (e *E2ETestEnv) HTTPSURL() string {
	return e.Lolly.HTTPSBaseURL()
}

// SetupSSLTest 设置 SSL 测试环境。
//
// 用于 SSL/TLS 测试场景，自动生成证书。
func SetupSSLTest(t *testing.T, cfgBuilder func() string) (*LollyContainer, string, string) {
	t.Helper()

	ctx := context.Background()

	if !LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	// 生成自签名证书
	certPath, keyPath, cleanup, err := GenerateSelfSignedCert(t.TempDir())
	if err != nil {
		t.Fatalf("Failed to generate certificate: %v", err)
	}
	t.Cleanup(cleanup)

	cfgYAML := cfgBuilder()

	lolly, err := StartLolly(ctx,
		WithConfigYAML(cfgYAML),
		WithCert(certPath, keyPath),
	)
	if err != nil {
		t.Fatalf("Failed to start lolly: %v", err)
	}
	t.Cleanup(func() { lolly.Terminate(ctx) })

	if err := lolly.WaitForHealthy(ctx, HealthCheckWaitTimeout); err != nil {
		t.Fatalf("Lolly not healthy: %v", err)
	}

	return lolly, certPath, keyPath
}
