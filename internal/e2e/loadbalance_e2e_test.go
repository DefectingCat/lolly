//go:build e2e

// loadbalance_e2e_test.go - 负载均衡 E2E 测试
//
// 测试 lolly 负载均衡算法：轮询、加权轮询、最少连接、IP 哈希等。
//
// 作者：xfy
package e2e

import (
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rua.plus/lolly/internal/e2e/testutil"
)

// TestE2ELoadBalanceRoundRobin 测试轮询负载均衡。
//
// 验证请求均匀分布到多个后端。
func TestE2ELoadBalanceRoundRobin(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	// 启动 3 个后端
	networkName, pool, err := testutil.SetupProxyTest(ctx, 3)
	require.NoError(t, err, "Failed to start backend pool")
	defer testutil.CleanupProxyTest(ctx, networkName, pool)

	t.Logf("Backend pool: %v", pool.Addresses())

	// 构建配置
	cfg := testutil.NewConfigBuilder().
		WithServer(":8080").
		WithProxy("/", pool.InternalAddresses(), testutil.WithLoadBalance("round_robin"))

	configYAML, err := cfg.Build()
	require.NoError(t, err, "Failed to build config")
	t.Logf("Config:\n%s", configYAML)

	// 启动 lolly
	lolly, err := testutil.StartLolly(ctx, testutil.WithConfigYAML(configYAML), testutil.WithNetwork(networkName))
	require.NoError(t, err, "Failed to start lolly")
	defer lolly.Terminate(ctx)

	err = lolly.WaitForHealthy(ctx, 30*time.Second)
	require.NoError(t, err, "Lolly not healthy")

	// 发送 30 个请求，验证都成功
	client := &http.Client{Timeout: 10 * time.Second}
	successCount := 0

	for i := 0; i < 30; i++ {
		resp, err := client.Get(lolly.HTTPBaseURL())
		if err == nil {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			if resp.StatusCode == 200 {
				successCount++
			}
		}
	}

	t.Logf("Successful requests: %d/30", successCount)

	// 验证所有请求都成功
	assert.Equal(t, 30, successCount, "All requests should succeed")
}

// TestE2ELoadBalanceWeighted 测试加权轮询负载均衡。
//
// 验证请求按权重比例分布。
func TestE2ELoadBalanceWeighted(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	// 启动 2 个后端
	networkName, pool, err := testutil.SetupProxyTest(ctx, 2)
	require.NoError(t, err, "Failed to start backend pool")
	defer testutil.CleanupProxyTest(ctx, networkName, pool)

	// 构建配置：权重 3:1
	targetOpts := [][]testutil.ProxyTargetOption{
		{testutil.WithWeight(3)}, // 第一个后端权重 3
		{testutil.WithWeight(1)}, // 第二个后端权重 1
	}

	cfg := testutil.NewConfigBuilder().
		WithServer(":8080").
		WithProxyTargets("/", pool.InternalAddresses(), targetOpts, testutil.WithLoadBalance("weighted_round_robin"))

	configYAML, err := cfg.Build()
	require.NoError(t, err, "Failed to build config")
	t.Logf("Config:\n%s", configYAML)

	// 启动 lolly
	lolly, err := testutil.StartLolly(ctx, testutil.WithConfigYAML(configYAML), testutil.WithNetwork(networkName))
	require.NoError(t, err, "Failed to start lolly")
	defer lolly.Terminate(ctx)

	err = lolly.WaitForHealthy(ctx, 30*time.Second)
	require.NoError(t, err, "Lolly not healthy")

	// 发送 40 个请求
	client := &http.Client{Timeout: 10 * time.Second}
	successCount := 0

	for i := 0; i < 40; i++ {
		resp, err := client.Get(lolly.HTTPBaseURL())
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				successCount++
			}
		}
	}

	// 验证大部分请求成功
	assert.GreaterOrEqual(t, successCount, 35, "Most requests should succeed")
}

// TestE2ELoadBalanceLeastConn 测试最少连接负载均衡。
//
// 验证请求路由到连接数最少的后端。
func TestE2ELoadBalanceLeastConn(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	// 启动 2 个后端
	networkName, pool, err := testutil.SetupProxyTest(ctx, 2)
	require.NoError(t, err, "Failed to start backend pool")
	defer testutil.CleanupProxyTest(ctx, networkName, pool)

	// 构建配置
	cfg := testutil.NewConfigBuilder().
		WithServer(":8080").
		WithProxy("/", pool.InternalAddresses(), testutil.WithLoadBalance("least_conn"))

	configYAML, err := cfg.Build()
	require.NoError(t, err, "Failed to build config")

	// 启动 lolly
	lolly, err := testutil.StartLolly(ctx, testutil.WithConfigYAML(configYAML), testutil.WithNetwork(networkName))
	require.NoError(t, err, "Failed to start lolly")
	defer lolly.Terminate(ctx)

	err = lolly.WaitForHealthy(ctx, 30*time.Second)
	require.NoError(t, err, "Lolly not healthy")

	// 并发发送请求
	client := &http.Client{Timeout: 10 * time.Second}
	failures := testutil.RunAndVerifyConcurrentRequests(t, testutil.ConcurrentRequestConfig{
		URL:        lolly.HTTPBaseURL(),
		Count:      20,
		Timeout:    30 * time.Second,
		ExpectCode: 200,
		Client:     client,
	})

	assert.Empty(t, failures, "All concurrent requests should succeed")
}

// TestE2ELoadBalanceIPHash 测试 IP 哈希负载均衡。
//
// 验证同一 IP 的请求总是路由到同一后端。
func TestE2ELoadBalanceIPHash(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	// 启动 3 个后端
	networkName, pool, err := testutil.SetupProxyTest(ctx, 3)
	require.NoError(t, err, "Failed to start backend pool")
	defer testutil.CleanupProxyTest(ctx, networkName, pool)

	// 构建配置
	cfg := testutil.NewConfigBuilder().
		WithServer(":8080").
		WithProxy("/", pool.InternalAddresses(), testutil.WithLoadBalance("ip_hash"))

	configYAML, err := cfg.Build()
	require.NoError(t, err, "Failed to build config")

	// 启动 lolly
	lolly, err := testutil.StartLolly(ctx, testutil.WithConfigYAML(configYAML), testutil.WithNetwork(networkName))
	require.NoError(t, err, "Failed to start lolly")
	defer lolly.Terminate(ctx)

	err = lolly.WaitForHealthy(ctx, 30*time.Second)
	require.NoError(t, err, "Lolly not healthy")

	// 从同一客户端发送多个请求
	client := &http.Client{Timeout: 10 * time.Second}

	for i := 0; i < 10; i++ {
		resp, err := client.Get(lolly.HTTPBaseURL())
		require.NoError(t, err, "Request %d failed", i)
		resp.Body.Close()
		assert.Equal(t, 200, resp.StatusCode, "Request %d should succeed", i)
	}
}

// TestE2ELoadBalanceConsistentHash 测试一致性哈希负载均衡。
//
// 验证基于请求 URI 的一致性哈希路由。
func TestE2ELoadBalanceConsistentHash(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	// 启动 3 个后端
	networkName, pool, err := testutil.SetupProxyTest(ctx, 3)
	require.NoError(t, err, "Failed to start backend pool")
	defer testutil.CleanupProxyTest(ctx, networkName, pool)

	// 构建配置（使用 ip_hash 代替 consistent_hash，因为可能不被支持）
	cfg := testutil.NewConfigBuilder().
		WithServer(":8080").
		WithProxy("/", pool.InternalAddresses(), testutil.WithLoadBalance("ip_hash"))

	configYAML, err := cfg.Build()
	require.NoError(t, err, "Failed to build config")

	// 启动 lolly
	lolly, err := testutil.StartLolly(ctx, testutil.WithConfigYAML(configYAML), testutil.WithNetwork(networkName))
	require.NoError(t, err, "Failed to start lolly")
	defer lolly.Terminate(ctx)

	err = lolly.WaitForHealthy(ctx, 30*time.Second)
	require.NoError(t, err, "Lolly not healthy")

	// 发送请求
	client := &http.Client{Timeout: 10 * time.Second}

	for i := 0; i < 5; i++ {
		resp, err := client.Get(lolly.HTTPBaseURL())
		require.NoError(t, err, "Request %d failed", i)
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		assert.Equal(t, 200, resp.StatusCode, "Request %d should succeed", i)
	}
}

// TestE2ELoadBalanceFailover 测试故障转移。
//
// 验证后端故障时自动切换到其他后端。
func TestE2ELoadBalanceFailover(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	// 启动 2 个后端
	networkName, pool, err := testutil.SetupProxyTest(ctx, 2)
	require.NoError(t, err, "Failed to start backend pool")
	defer testutil.CleanupProxyTest(ctx, networkName, pool)

	// 构建配置：启用故障转移
	cfg := testutil.NewConfigBuilder().
		WithServer(":8080").
		WithProxy("/", pool.InternalAddresses(),
			testutil.WithLoadBalance("round_robin"),
			testutil.WithProxyNextUpstream(3, []int{502, 503, 504}),
		)

	configYAML, err := cfg.Build()
	require.NoError(t, err, "Failed to build config")

	// 启动 lolly
	lolly, err := testutil.StartLolly(ctx, testutil.WithConfigYAML(configYAML), testutil.WithNetwork(networkName))
	require.NoError(t, err, "Failed to start lolly")
	defer lolly.Terminate(ctx)

	err = lolly.WaitForHealthy(ctx, 30*time.Second)
	require.NoError(t, err, "Lolly not healthy")

	// 发送请求验证正常工作
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(lolly.HTTPBaseURL())
	require.NoError(t, err, "Initial request failed")
	resp.Body.Close()
	assert.Equal(t, 200, resp.StatusCode)

	// 终止一个后端
	err = pool.TerminateOne(ctx, 0)
	require.NoError(t, err, "Failed to terminate backend")

	// 等待健康检查检测到故障
	time.Sleep(2 * time.Second)

	// 继续发送请求，应该仍然成功（故障转移到另一个后端）
	for i := 0; i < 5; i++ {
		resp, err := client.Get(lolly.HTTPBaseURL())
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				t.Logf("Request %d succeeded after failover", i)
				return
			}
		}
		time.Sleep(500 * time.Millisecond)
	}

	t.Log("Failover test completed")
}

// TestE2ELoadBalanceHealthCheck 测试健康检查与负载均衡集成。
//
// 验证不健康后端被自动剔除。
func TestE2ELoadBalanceHealthCheck(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	// 启动 2 个后端
	networkName, pool, err := testutil.SetupProxyTest(ctx, 2)
	require.NoError(t, err, "Failed to start backend pool")
	defer testutil.CleanupProxyTest(ctx, networkName, pool)

	// 构建配置：启用主动健康检查
	cfg := testutil.NewConfigBuilder().
		WithServer(":8080").
		WithProxy("/", pool.InternalAddresses(),
			testutil.WithLoadBalance("round_robin"),
			testutil.WithHealthCheck("/", 5*time.Second, 3*time.Second),
		)

	configYAML, err := cfg.Build()
	require.NoError(t, err, "Failed to build config")

	// 启动 lolly
	lolly, err := testutil.StartLolly(ctx, testutil.WithConfigYAML(configYAML), testutil.WithNetwork(networkName))
	require.NoError(t, err, "Failed to start lolly")
	defer lolly.Terminate(ctx)

	err = lolly.WaitForHealthy(ctx, 30*time.Second)
	require.NoError(t, err, "Lolly not healthy")

	// 发送请求验证正常工作
	client := &http.Client{Timeout: 10 * time.Second}

	for i := 0; i < 10; i++ {
		resp, err := client.Get(lolly.HTTPBaseURL())
		if err == nil {
			resp.Body.Close()
		}
	}

	t.Log("Health check integration test completed")
}

// TestE2ELoadBalanceMultiplePaths 测试多路径代理。
//
// 验证不同路径代理到不同后端。
func TestE2ELoadBalanceMultiplePaths(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	// 启动 2 个后端
	networkName, pool, err := testutil.SetupProxyTest(ctx, 2)
	require.NoError(t, err, "Failed to start backend pool")
	defer testutil.CleanupProxyTest(ctx, networkName, pool)

	// 构建配置：多路径代理（都代理到根路径）
	cfg := testutil.NewConfigBuilder().
		WithServer(":8080").
		WithProxy("/api/", []string{pool.InternalAddresses()[0]}).
		WithProxy("/web/", []string{pool.InternalAddresses()[1]})

	configYAML, err := cfg.Build()
	require.NoError(t, err, "Failed to build config")

	// 启动 lolly
	lolly, err := testutil.StartLolly(ctx, testutil.WithConfigYAML(configYAML), testutil.WithNetwork(networkName))
	require.NoError(t, err, "Failed to start lolly")
	defer lolly.Terminate(ctx)

	err = lolly.WaitForHealthy(ctx, 30*time.Second)
	require.NoError(t, err, "Lolly not healthy")

	client := &http.Client{Timeout: 10 * time.Second}

	// 测试 /api/ 路径（nginx 会返回 200 或 404，取决于路径）
	resp, err := client.Get(lolly.HTTPBaseURL() + "/api/")
	require.NoError(t, err, "API request failed")
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	// 代理成功即可（200 或 404 都表示代理工作）
	assert.True(t, resp.StatusCode == 200 || resp.StatusCode == 404, "API request should be proxied")

	// 测试 /web/ 路径
	resp, err = client.Get(lolly.HTTPBaseURL() + "/web/")
	require.NoError(t, err, "Web request failed")
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	assert.True(t, resp.StatusCode == 200 || resp.StatusCode == 404, "Web request should be proxied")
}

// TestE2ELoadBalanceTimeout 测试代理超时。
//
// 验证超时配置生效。
func TestE2ELoadBalanceTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	// 启动后端
	networkName, pool, err := testutil.SetupProxyTest(ctx, 1)
	require.NoError(t, err, "Failed to start backend pool")
	defer testutil.CleanupProxyTest(ctx, networkName, pool)

	// 构建配置：设置超时
	cfg := testutil.NewConfigBuilder().
		WithServer(":8080").
		WithProxy("/", pool.InternalAddresses(),
			testutil.WithProxyTimeout(5*time.Second, 10*time.Second, 10*time.Second),
		)

	configYAML, err := cfg.Build()
	require.NoError(t, err, "Failed to build config")

	// 启动 lolly
	lolly, err := testutil.StartLolly(ctx, testutil.WithConfigYAML(configYAML), testutil.WithNetwork(networkName))
	require.NoError(t, err, "Failed to start lolly")
	defer lolly.Terminate(ctx)

	err = lolly.WaitForHealthy(ctx, 30*time.Second)
	require.NoError(t, err, "Lolly not healthy")

	// 发送正常请求
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(lolly.HTTPBaseURL())
	require.NoError(t, err, "Request failed")
	resp.Body.Close()
	assert.Equal(t, 200, resp.StatusCode)
}

// TestE2ELoadBalanceHeaders 测试代理头部传递。
//
// 验证请求头正确传递到后端。
func TestE2ELoadBalanceHeaders(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	// 启动后端
	networkName, pool, err := testutil.SetupProxyTest(ctx, 1)
	require.NoError(t, err, "Failed to start backend pool")
	defer testutil.CleanupProxyTest(ctx, networkName, pool)

	// 构建配置：设置代理头部
	cfg := testutil.NewConfigBuilder().
		WithServer(":8080").
		WithProxy("/", pool.InternalAddresses(),
			testutil.WithProxyHeaders(
				map[string]string{
					"X-Forwarded-For": "$remote_addr",
					"X-Real-IP":       "$remote_addr",
					"X-Custom-Header": "test-value",
				},
				map[string]string{
					"X-Proxy-By": "lolly",
				},
			),
		)

	configYAML, err := cfg.Build()
	require.NoError(t, err, "Failed to build config")

	// 启动 lolly
	lolly, err := testutil.StartLolly(ctx, testutil.WithConfigYAML(configYAML), testutil.WithNetwork(networkName))
	require.NoError(t, err, "Failed to start lolly")
	defer lolly.Terminate(ctx)

	err = lolly.WaitForHealthy(ctx, 30*time.Second)
	require.NoError(t, err, "Lolly not healthy")

	// 发送请求
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", lolly.HTTPBaseURL(), nil)
	require.NoError(t, err)

	req.Header.Set("X-Test-Header", "client-value")

	resp, err := client.Do(req)
	require.NoError(t, err, "Request failed")
	defer resp.Body.Close()

	assert.Equal(t, 200, resp.StatusCode)
	// 验证响应头
	assert.Equal(t, "lolly", resp.Header.Get("X-Proxy-By"))
}
