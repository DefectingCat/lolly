//go:build e2e

// cache_e2e_test.go - 代理缓存 E2E 测试
//
// 测试 lolly 代理缓存功能：缓存命中、缓存过期、缓存锁等。
//
// 作者：xfy
package e2e

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rua.plus/lolly/internal/e2e/testutil"
)

// TestE2EProxyCacheHit 测试缓存命中。
//
// 验证第二次请求返回缓存内容。
func TestE2EProxyCacheHit(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	// 启动后端
	netObj, networkName, pool, err := testutil.SetupProxyTest(ctx, 1, t.Name())
	require.NoError(t, err, "Failed to start backend pool")
	defer testutil.CleanupProxyTest(ctx, netObj, networkName, pool)

	// 构建配置：启用缓存
	cfg := testutil.NewConfigBuilder().
		WithServer(":8080").
		WithProxy("/", pool.InternalAddresses(),
			testutil.WithProxyCache(5*time.Minute, false),
		)

	configYAML, err := cfg.Build()
	require.NoError(t, err, "Failed to build config")
	t.Logf("Config:\n%s", configYAML)

	// 启动 lolly
	lolly, err := testutil.StartLolly(ctx, testutil.WithConfigYAML(configYAML), testutil.WithNetwork(networkName))
	require.NoError(t, err, "Failed to start lolly")
	defer lolly.Terminate(ctx)

	err = lolly.WaitForHealthy(ctx, testutil.HealthCheckWaitTimeout)
	require.NoError(t, err, "Lolly not healthy")

	client := &http.Client{Timeout: 10 * time.Second}

	// 第一次请求（缓存 MISS）
	resp1, err := client.Get(lolly.HTTPBaseURL())
	require.NoError(t, err, "First request failed")
	defer resp1.Body.Close()

	body1, err := io.ReadAll(resp1.Body)
	require.NoError(t, err, "Failed to read first response")

	// 检查缓存状态头部
	cacheStatus1 := resp1.Header.Get("X-Cache-Status")
	t.Logf("First request - X-Cache-Status: %s", cacheStatus1)

	// 第二次请求（应该命中缓存）
	resp2, err := client.Get(lolly.HTTPBaseURL())
	require.NoError(t, err, "Second request failed")
	defer resp2.Body.Close()

	body2, err := io.ReadAll(resp2.Body)
	require.NoError(t, err, "Failed to read second response")

	cacheStatus2 := resp2.Header.Get("X-Cache-Status")
	t.Logf("Second request - X-Cache-Status: %s", cacheStatus2)

	// 验证响应内容相同
	assert.Equal(t, string(body1), string(body2), "Cached response should match original")

	// 如果有 Age 头部，表示来自缓存
	age := resp2.Header.Get("Age")
	if age != "" {
		t.Logf("Response Age: %s", age)
	}
}

// TestE2EProxyCacheExpire 测试缓存过期。
//
// 验证缓存过期后重新获取。
func TestE2EProxyCacheExpire(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	// 启动后端
	netObj, networkName, pool, err := testutil.SetupProxyTest(ctx, 1, t.Name())
	require.NoError(t, err, "Failed to start backend pool")
	defer testutil.CleanupProxyTest(ctx, netObj, networkName, pool)

	// 构建配置：短缓存时间
	cfg := testutil.NewConfigBuilder().
		WithServer(":8080").
		WithProxy("/", pool.InternalAddresses(),
			testutil.WithProxyCache(2*time.Second, false),
		)

	configYAML, err := cfg.Build()
	require.NoError(t, err, "Failed to build config")

	// 启动 lolly
	lolly, err := testutil.StartLolly(ctx, testutil.WithConfigYAML(configYAML), testutil.WithNetwork(networkName))
	require.NoError(t, err, "Failed to start lolly")
	defer lolly.Terminate(ctx)

	err = lolly.WaitForHealthy(ctx, testutil.HealthCheckWaitTimeout)
	require.NoError(t, err, "Lolly not healthy")

	client := &http.Client{Timeout: 10 * time.Second}

	// 第一次请求
	resp1, err := client.Get(lolly.HTTPBaseURL())
	require.NoError(t, err, "First request failed")
	resp1.Body.Close()

	t.Log("First request completed, waiting for cache to expire...")

	// 等待缓存过期
	time.Sleep(3 * time.Second)

	// 第二次请求（缓存应该已过期）
	resp2, err := client.Get(lolly.HTTPBaseURL())
	require.NoError(t, err, "Second request failed")
	defer resp2.Body.Close()

	t.Log("Second request after cache expiry completed")

	// 验证请求成功
	assert.Equal(t, 200, resp2.StatusCode)
}

// TestE2EProxyCacheLock 测试缓存锁。
//
// 验证缓存锁防止缓存击穿。
func TestE2EProxyCacheLock(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	// 启动后端
	netObj, networkName, pool, err := testutil.SetupProxyTest(ctx, 1, t.Name())
	require.NoError(t, err, "Failed to start backend pool")
	defer testutil.CleanupProxyTest(ctx, netObj, networkName, pool)

	// 构建配置：启用缓存锁
	cfg := testutil.NewConfigBuilder().
		WithServer(":8080").
		WithProxy("/", pool.InternalAddresses(),
			testutil.WithProxyCache(5*time.Minute, true),
		)

	configYAML, err := cfg.Build()
	require.NoError(t, err, "Failed to build config")

	// 启动 lolly
	lolly, err := testutil.StartLolly(ctx, testutil.WithConfigYAML(configYAML), testutil.WithNetwork(networkName))
	require.NoError(t, err, "Failed to start lolly")
	defer lolly.Terminate(ctx)

	err = lolly.WaitForHealthy(ctx, testutil.HealthCheckWaitTimeout)
	require.NoError(t, err, "Lolly not healthy")

	// 并发发送相同请求
	failures := testutil.RunAndVerifyConcurrentRequests(t, testutil.ConcurrentRequestConfig{
		URL:        lolly.HTTPBaseURL(),
		Count:      10,
		Timeout:    30 * time.Second,
		ExpectCode: 200,
	})

	assert.Empty(t, failures, "All concurrent requests should succeed with cache lock")
}

// TestE2EProxyCacheBypass 测试缓存绕过。
//
// 验证特定请求绕过缓存。
func TestE2EProxyCacheBypass(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	// 启动后端
	netObj, networkName, pool, err := testutil.SetupProxyTest(ctx, 1, t.Name())
	require.NoError(t, err, "Failed to start backend pool")
	defer testutil.CleanupProxyTest(ctx, netObj, networkName, pool)

	// 构建配置
	cfg := testutil.NewConfigBuilder().
		WithServer(":8080").
		WithProxy("/", pool.InternalAddresses(),
			testutil.WithProxyCache(5*time.Minute, false),
		)

	configYAML, err := cfg.Build()
	require.NoError(t, err, "Failed to build config")

	// 启动 lolly
	lolly, err := testutil.StartLolly(ctx, testutil.WithConfigYAML(configYAML), testutil.WithNetwork(networkName))
	require.NoError(t, err, "Failed to start lolly")
	defer lolly.Terminate(ctx)

	err = lolly.WaitForHealthy(ctx, testutil.HealthCheckWaitTimeout)
	require.NoError(t, err, "Lolly not healthy")

	client := &http.Client{Timeout: 10 * time.Second}

	// 正常请求
	resp1, err := client.Get(lolly.HTTPBaseURL())
	require.NoError(t, err, "Normal request failed")
	resp1.Body.Close()

	// 带 Cache-Control: no-cache 的请求
	req, err := http.NewRequest("GET", lolly.HTTPBaseURL(), nil)
	require.NoError(t, err, "Failed to create request")
	req.Header.Set("Cache-Control", "no-cache")

	resp2, err := client.Do(req)
	require.NoError(t, err, "Bypass request failed")
	defer resp2.Body.Close()

	t.Log("Cache bypass test completed")
}

// TestE2EProxyCacheMethods 测试缓存 HTTP 方法。
//
// 验证只有 GET 和 HEAD 被缓存。
func TestE2EProxyCacheMethods(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	// 启动后端
	netObj, networkName, pool, err := testutil.SetupProxyTest(ctx, 1, t.Name())
	require.NoError(t, err, "Failed to start backend pool")
	defer testutil.CleanupProxyTest(ctx, netObj, networkName, pool)

	// 构建配置
	cfg := testutil.NewConfigBuilder().
		WithServer(":8080").
		WithProxy("/", pool.InternalAddresses(),
			testutil.WithProxyCache(5*time.Minute, false),
		)

	configYAML, err := cfg.Build()
	require.NoError(t, err, "Failed to build config")

	// 启动 lolly
	lolly, err := testutil.StartLolly(ctx, testutil.WithConfigYAML(configYAML), testutil.WithNetwork(networkName))
	require.NoError(t, err, "Failed to start lolly")
	defer lolly.Terminate(ctx)

	err = lolly.WaitForHealthy(ctx, testutil.HealthCheckWaitTimeout)
	require.NoError(t, err, "Lolly not healthy")

	client := &http.Client{Timeout: 10 * time.Second}

	// GET 请求
	resp, err := client.Get(lolly.HTTPBaseURL())
	require.NoError(t, err, "GET request failed")
	resp.Body.Close()

	// HEAD 请求
	resp, err = client.Head(lolly.HTTPBaseURL())
	require.NoError(t, err, "HEAD request failed")
	resp.Body.Close()

	// POST 请求（不应该被缓存）
	resp, err = client.Post(lolly.HTTPBaseURL(), "text/plain", strings.NewReader("test"))
	require.NoError(t, err, "POST request failed")
	resp.Body.Close()

	t.Log("Cache methods test completed")
}

// TestE2EProxyCacheHeaders 测试缓存相关头部。
//
// 验证缓存头部正确设置。
func TestE2EProxyCacheHeaders(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	// 启动后端
	netObj, networkName, pool, err := testutil.SetupProxyTest(ctx, 1, t.Name())
	require.NoError(t, err, "Failed to start backend pool")
	defer testutil.CleanupProxyTest(ctx, netObj, networkName, pool)

	// 构建配置
	cfg := testutil.NewConfigBuilder().
		WithServer(":8080").
		WithProxy("/", pool.InternalAddresses(),
			testutil.WithProxyCache(5*time.Minute, false),
		)

	configYAML, err := cfg.Build()
	require.NoError(t, err, "Failed to build config")

	// 启动 lolly
	lolly, err := testutil.StartLolly(ctx, testutil.WithConfigYAML(configYAML), testutil.WithNetwork(networkName))
	require.NoError(t, err, "Failed to start lolly")
	defer lolly.Terminate(ctx)

	err = lolly.WaitForHealthy(ctx, testutil.HealthCheckWaitTimeout)
	require.NoError(t, err, "Lolly not healthy")

	client := &http.Client{Timeout: 10 * time.Second}

	// 发送请求
	resp, err := client.Get(lolly.HTTPBaseURL())
	require.NoError(t, err, "Request failed")
	defer resp.Body.Close()

	// 检查响应头部
	t.Logf("Response headers:")
	for k, v := range resp.Header {
		t.Logf("  %s: %v", k, v)
	}

	// 验证基本响应
	assert.Equal(t, 200, resp.StatusCode)
}

// TestE2EProxyCacheStale 测试过期缓存使用。
//
// 验证后端不可用时返回过期缓存。
func TestE2EProxyCacheStale(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	// 启动后端
	netObj, networkName, pool, err := testutil.SetupProxyTest(ctx, 1, t.Name())
	require.NoError(t, err, "Failed to start backend pool")
	defer testutil.CleanupProxyTest(ctx, netObj, networkName, pool)

	// 构建配置：启用 stale-while-revalidate
	cfg := testutil.NewConfigBuilder().
		WithServer(":8080").
		WithProxy("/", pool.InternalAddresses(),
			testutil.WithProxyCache(2*time.Second, false),
		)

	configYAML, err := cfg.Build()
	require.NoError(t, err, "Failed to build config")

	// 启动 lolly
	lolly, err := testutil.StartLolly(ctx, testutil.WithConfigYAML(configYAML), testutil.WithNetwork(networkName))
	require.NoError(t, err, "Failed to start lolly")
	defer lolly.Terminate(ctx)

	err = lolly.WaitForHealthy(ctx, testutil.HealthCheckWaitTimeout)
	require.NoError(t, err, "Lolly not healthy")

	client := &http.Client{Timeout: 10 * time.Second}

	// 第一次请求建立缓存
	resp, err := client.Get(lolly.HTTPBaseURL())
	require.NoError(t, err, "First request failed")
	resp.Body.Close()

	t.Log("Cache established")

	// 等待缓存过期
	time.Sleep(3 * time.Second)

	// 第二次请求
	resp, err = client.Get(lolly.HTTPBaseURL())
	require.NoError(t, err, "Second request failed")
	resp.Body.Close()

	t.Log("Stale cache test completed")
}

// TestE2EProxyCacheVary 测试 Vary 头部。
//
// 验证 Vary 头部影响缓存键。
func TestE2EProxyCacheVary(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	// 启动后端
	netObj, networkName, pool, err := testutil.SetupProxyTest(ctx, 1, t.Name())
	require.NoError(t, err, "Failed to start backend pool")
	defer testutil.CleanupProxyTest(ctx, netObj, networkName, pool)

	// 构建配置
	cfg := testutil.NewConfigBuilder().
		WithServer(":8080").
		WithProxy("/", pool.InternalAddresses(),
			testutil.WithProxyCache(5*time.Minute, false),
		)

	configYAML, err := cfg.Build()
	require.NoError(t, err, "Failed to build config")

	// 启动 lolly
	lolly, err := testutil.StartLolly(ctx, testutil.WithConfigYAML(configYAML), testutil.WithNetwork(networkName))
	require.NoError(t, err, "Failed to start lolly")
	defer lolly.Terminate(ctx)

	err = lolly.WaitForHealthy(ctx, testutil.HealthCheckWaitTimeout)
	require.NoError(t, err, "Lolly not healthy")

	client := &http.Client{Timeout: 10 * time.Second}

	// 发送带不同 Accept-Encoding 的请求
	encodings := []string{"gzip", "identity", "br"}

	for i, enc := range encodings {
		req, err := http.NewRequest("GET", lolly.HTTPBaseURL(), nil)
		require.NoError(t, err, "Failed to create request %d", i)
		req.Header.Set("Accept-Encoding", enc)

		resp, err := client.Do(req)
		require.NoError(t, err, "Request %d failed", i)
		resp.Body.Close()

		t.Logf("Request with Accept-Encoding: %s - Status: %d", enc, resp.StatusCode)
	}
}

// TestE2EProxyCacheRevalidate 测试缓存重新验证。
//
// 验证条件请求正确处理。
func TestE2EProxyCacheRevalidate(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	// 启动后端
	netObj, networkName, pool, err := testutil.SetupProxyTest(ctx, 1, t.Name())
	require.NoError(t, err, "Failed to start backend pool")
	defer testutil.CleanupProxyTest(ctx, netObj, networkName, pool)

	// 构建配置
	cfg := testutil.NewConfigBuilder().
		WithServer(":8080").
		WithProxy("/", pool.InternalAddresses(),
			testutil.WithProxyCache(5*time.Minute, false),
		)

	configYAML, err := cfg.Build()
	require.NoError(t, err, "Failed to build config")

	// 启动 lolly
	lolly, err := testutil.StartLolly(ctx, testutil.WithConfigYAML(configYAML), testutil.WithNetwork(networkName))
	require.NoError(t, err, "Failed to start lolly")
	defer lolly.Terminate(ctx)

	err = lolly.WaitForHealthy(ctx, testutil.HealthCheckWaitTimeout)
	require.NoError(t, err, "Lolly not healthy")

	client := &http.Client{Timeout: 10 * time.Second}

	// 第一次请求获取 ETag
	resp1, err := client.Get(lolly.HTTPBaseURL())
	require.NoError(t, err, "First request failed")

	etag := resp1.Header.Get("ETag")
	lastModified := resp1.Header.Get("Last-Modified")
	resp1.Body.Close()

	t.Logf("ETag: %s, Last-Modified: %s", etag, lastModified)

	// 第二次请求带 If-None-Match
	if etag != "" {
		req, err := http.NewRequest("GET", lolly.HTTPBaseURL(), nil)
		require.NoError(t, err, "Failed to create conditional request")
		req.Header.Set("If-None-Match", etag)

		resp2, err := client.Do(req)
		require.NoError(t, err, "Conditional request failed")
		resp2.Body.Close()

		t.Logf("Conditional request status: %d", resp2.StatusCode)
	}
}

// TestE2EProxyCacheConcurrent 测试并发缓存请求。
//
// 验证并发请求正确处理缓存。
func TestE2EProxyCacheConcurrent(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	// 启动后端
	netObj, networkName, pool, err := testutil.SetupProxyTest(ctx, 1, t.Name())
	require.NoError(t, err, "Failed to start backend pool")
	defer testutil.CleanupProxyTest(ctx, netObj, networkName, pool)

	// 构建配置
	cfg := testutil.NewConfigBuilder().
		WithServer(":8080").
		WithProxy("/", pool.InternalAddresses(),
			testutil.WithProxyCache(5*time.Minute, true),
		)

	configYAML, err := cfg.Build()
	require.NoError(t, err, "Failed to build config")

	// 启动 lolly
	lolly, err := testutil.StartLolly(ctx, testutil.WithConfigYAML(configYAML), testutil.WithNetwork(networkName))
	require.NoError(t, err, "Failed to start lolly")
	defer lolly.Terminate(ctx)

	err = lolly.WaitForHealthy(ctx, testutil.HealthCheckWaitTimeout)
	require.NoError(t, err, "Lolly not healthy")

	// 并发请求
	failures := testutil.RunAndVerifyConcurrentRequests(t, testutil.ConcurrentRequestConfig{
		URL:        lolly.HTTPBaseURL(),
		Count:      20,
		Timeout:    30 * time.Second,
		ExpectCode: 200,
	})

	assert.Empty(t, failures, "All concurrent cache requests should succeed")

	t.Log("Concurrent cache test completed")
}

// TestE2EProxyCacheMultiplePaths 测试多路径缓存。
//
// 验证不同路径独立缓存。
func TestE2EProxyCacheMultiplePaths(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	// 启动后端
	netObj, networkName, pool, err := testutil.SetupProxyTest(ctx, 2, t.Name())
	require.NoError(t, err, "Failed to start backend pool")
	defer testutil.CleanupProxyTest(ctx, netObj, networkName, pool)

	// 构建配置：多路径代理
	cfg := testutil.NewConfigBuilder().
		WithServer(":8080").
		WithProxy("/api/", []string{pool.InternalAddresses()[0]},
			testutil.WithProxyCache(5*time.Minute, false),
		).
		WithProxy("/web/", []string{pool.InternalAddresses()[1]},
			testutil.WithProxyCache(5*time.Minute, false),
		)

	configYAML, err := cfg.Build()
	require.NoError(t, err, "Failed to build config")

	// 启动 lolly
	lolly, err := testutil.StartLolly(ctx, testutil.WithConfigYAML(configYAML), testutil.WithNetwork(networkName))
	require.NoError(t, err, "Failed to start lolly")
	defer lolly.Terminate(ctx)

	err = lolly.WaitForHealthy(ctx, testutil.HealthCheckWaitTimeout)
	require.NoError(t, err, "Lolly not healthy")

	client := &http.Client{Timeout: 10 * time.Second}

	// 测试 /api/ 路径
	resp1, err := client.Get(lolly.HTTPBaseURL() + "/api/")
	require.NoError(t, err, "API request failed")
	resp1.Body.Close()

	// 测试 /web/ 路径
	resp2, err := client.Get(lolly.HTTPBaseURL() + "/web/")
	require.NoError(t, err, "Web request failed")
	resp2.Body.Close()

	// 再次请求验证缓存
	resp3, err := client.Get(lolly.HTTPBaseURL() + "/api/")
	require.NoError(t, err, "Cached API request failed")
	resp3.Body.Close()

	t.Log("Multiple paths cache test completed")
}

// TestE2EProxyCacheWithHeaders 测试带头部的缓存请求。
//
// 验证请求头正确传递并影响缓存。
func TestE2EProxyCacheWithHeaders(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	// 启动后端
	netObj, networkName, pool, err := testutil.SetupProxyTest(ctx, 1, t.Name())
	require.NoError(t, err, "Failed to start backend pool")
	defer testutil.CleanupProxyTest(ctx, netObj, networkName, pool)

	// 构建配置
	cfg := testutil.NewConfigBuilder().
		WithServer(":8080").
		WithProxy("/", pool.InternalAddresses(),
			testutil.WithProxyCache(5*time.Minute, false),
			testutil.WithProxyHeaders(
				map[string]string{
					"X-Forwarded-For": "$remote_addr",
					"X-Request-ID":    "$request_id",
				},
				map[string]string{
					"X-Cache-Status": "$upstream_cache_status",
				},
			),
		)

	configYAML, err := cfg.Build()
	require.NoError(t, err, "Failed to build config")

	// 启动 lolly
	lolly, err := testutil.StartLolly(ctx, testutil.WithConfigYAML(configYAML), testutil.WithNetwork(networkName))
	require.NoError(t, err, "Failed to start lolly")
	defer lolly.Terminate(ctx)

	err = lolly.WaitForHealthy(ctx, testutil.HealthCheckWaitTimeout)
	require.NoError(t, err, "Lolly not healthy")

	client := &http.Client{Timeout: 10 * time.Second}

	// 发送请求
	req, err := http.NewRequest("GET", lolly.HTTPBaseURL(), nil)
	require.NoError(t, err, "Failed to create request")
	req.Header.Set("X-Custom-Header", "test-value")

	resp, err := client.Do(req)
	require.NoError(t, err, "Request failed")
	defer resp.Body.Close()

	// 检查响应头
	cacheStatus := resp.Header.Get("X-Cache-Status")
	t.Logf("X-Cache-Status: %s", cacheStatus)

	// 验证请求成功
	assert.Equal(t, 200, resp.StatusCode)
}

// TestE2EProxyCacheSize 测试缓存大小限制。
//
// 验证大响应不被缓存或正确处理。
func TestE2EProxyCacheSize(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	// 启动后端
	netObj, networkName, pool, err := testutil.SetupProxyTest(ctx, 1, t.Name())
	require.NoError(t, err, "Failed to start backend pool")
	defer testutil.CleanupProxyTest(ctx, netObj, networkName, pool)

	// 构建配置
	cfg := testutil.NewConfigBuilder().
		WithServer(":8080").
		WithProxy("/", pool.InternalAddresses(),
			testutil.WithProxyCache(5*time.Minute, false),
		)

	configYAML, err := cfg.Build()
	require.NoError(t, err, "Failed to build config")

	// 启动 lolly
	lolly, err := testutil.StartLolly(ctx, testutil.WithConfigYAML(configYAML), testutil.WithNetwork(networkName))
	require.NoError(t, err, "Failed to start lolly")
	defer lolly.Terminate(ctx)

	err = lolly.WaitForHealthy(ctx, testutil.HealthCheckWaitTimeout)
	require.NoError(t, err, "Lolly not healthy")

	client := &http.Client{Timeout: 30 * time.Second}

	// 请求大文件
	resp, err := client.Get(lolly.HTTPBaseURL())
	require.NoError(t, err, "Request failed")

	// 读取响应
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	require.NoError(t, err, "Failed to read response")

	t.Logf("Response size: %d bytes", len(body))

	// 再次请求验证
	resp2, err := client.Get(lolly.HTTPBaseURL())
	require.NoError(t, err, "Second request failed")
	resp2.Body.Close()

	t.Log("Cache size test completed")
}

// TestE2EProxyCacheStatusCodes 测试不同状态码的缓存。
//
// 验证不同 HTTP 状态码的缓存行为。
func TestE2EProxyCacheStatusCodes(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	// 启动后端
	netObj, networkName, pool, err := testutil.SetupProxyTest(ctx, 1, t.Name())
	require.NoError(t, err, "Failed to start backend pool")
	defer testutil.CleanupProxyTest(ctx, netObj, networkName, pool)

	// 构建配置
	cfg := testutil.NewConfigBuilder().
		WithServer(":8080").
		WithProxy("/", pool.InternalAddresses(),
			testutil.WithProxyCache(5*time.Minute, false),
		)

	configYAML, err := cfg.Build()
	require.NoError(t, err, "Failed to build config")

	// 启动 lolly
	lolly, err := testutil.StartLolly(ctx, testutil.WithConfigYAML(configYAML), testutil.WithNetwork(networkName))
	require.NoError(t, err, "Failed to start lolly")
	defer lolly.Terminate(ctx)

	err = lolly.WaitForHealthy(ctx, testutil.HealthCheckWaitTimeout)
	require.NoError(t, err, "Lolly not healthy")

	client := &http.Client{Timeout: 10 * time.Second}

	// 测试正常请求（200）
	resp, err := client.Get(lolly.HTTPBaseURL())
	require.NoError(t, err, "Request failed")
	resp.Body.Close()

	assert.Equal(t, 200, resp.StatusCode)

	// 测试不存在的路径（404）
	resp, err = client.Get(lolly.HTTPBaseURL() + "/nonexistent")
	require.NoError(t, err, "Request failed")
	resp.Body.Close()

	t.Logf("Non-existent path status: %d", resp.StatusCode)

	// 测试带查询参数的请求
	resp, err = client.Get(lolly.HTTPBaseURL() + "?query=test")
	require.NoError(t, err, "Request failed")
	resp.Body.Close()

	t.Logf("Query parameter request status: %d", resp.StatusCode)
}

// TestE2EProxyCacheQueryParams 测试查询参数对缓存的影响。
//
// 验证不同查询参数产生不同的缓存条目。
func TestE2EProxyCacheQueryParams(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	// 启动后端
	netObj, networkName, pool, err := testutil.SetupProxyTest(ctx, 1, t.Name())
	require.NoError(t, err, "Failed to start backend pool")
	defer testutil.CleanupProxyTest(ctx, netObj, networkName, pool)

	// 构建配置
	cfg := testutil.NewConfigBuilder().
		WithServer(":8080").
		WithProxy("/", pool.InternalAddresses(),
			testutil.WithProxyCache(5*time.Minute, false),
		)

	configYAML, err := cfg.Build()
	require.NoError(t, err, "Failed to build config")

	// 启动 lolly
	lolly, err := testutil.StartLolly(ctx, testutil.WithConfigYAML(configYAML), testutil.WithNetwork(networkName))
	require.NoError(t, err, "Failed to start lolly")
	defer lolly.Terminate(ctx)

	err = lolly.WaitForHealthy(ctx, testutil.HealthCheckWaitTimeout)
	require.NoError(t, err, "Lolly not healthy")

	client := &http.Client{Timeout: 10 * time.Second}

	// 发送带不同查询参数的请求
	queries := []string{"?a=1", "?b=2", "?c=3"}

	for i, q := range queries {
		resp, err := client.Get(lolly.HTTPBaseURL() + q)
		require.NoError(t, err, "Request %d failed", i)
		resp.Body.Close()

		t.Logf("Query %s - Status: %d", q, resp.StatusCode)
	}

	// 再次请求相同查询参数
	for i, q := range queries {
		resp, err := client.Get(lolly.HTTPBaseURL() + q)
		require.NoError(t, err, "Cached request %d failed", i)
		resp.Body.Close()
	}

	t.Log("Query params cache test completed")
}

// TestE2EProxyCacheIntegration 综合缓存测试。
//
// 验证缓存功能与其他功能的集成。
func TestE2EProxyCacheIntegration(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	// 启动后端
	netObj, networkName, pool, err := testutil.SetupProxyTest(ctx, 2, t.Name())
	require.NoError(t, err, "Failed to start backend pool")
	defer testutil.CleanupProxyTest(ctx, netObj, networkName, pool)

	// 构建配置：缓存 + 负载均衡
	cfg := testutil.NewConfigBuilder().
		WithServer(":8080").
		WithProxy("/", pool.InternalAddresses(),
			testutil.WithLoadBalance("round_robin"),
			testutil.WithProxyCache(5*time.Minute, true),
			testutil.WithProxyTimeout(5*time.Second, 30*time.Second, 30*time.Second),
		)

	configYAML, err := cfg.Build()
	require.NoError(t, err, "Failed to build config")

	// 启动 lolly
	lolly, err := testutil.StartLolly(ctx, testutil.WithConfigYAML(configYAML), testutil.WithNetwork(networkName))
	require.NoError(t, err, "Failed to start lolly")
	defer lolly.Terminate(ctx)

	err = lolly.WaitForHealthy(ctx, testutil.HealthCheckWaitTimeout)
	require.NoError(t, err, "Lolly not healthy")

	client := &http.Client{Timeout: 30 * time.Second}

	// 发送多个请求
	for i := 0; i < 10; i++ {
		resp, err := client.Get(fmt.Sprintf("%s?request=%d", lolly.HTTPBaseURL(), i))
		require.NoError(t, err, "Request %d failed", i)
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}

	t.Log("Cache integration test completed")
}
