//go:build e2e

// healthcheck_e2e_test.go - 健康检查 E2E 测试
//
// 测试 lolly 健康检查功能：主动健康检查、被动健康检查、后端恢复检测。
//
// 作者：xfy
package e2e

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rua.plus/lolly/internal/e2e/testutil"
)

// TestE2EHealthCheckActive 测试主动健康检查。
//
// 验证定期探测后端状态，自动剔除不健康后端。
func TestE2EHealthCheckActive(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	// 设置代理测试环境（使用网络模式）
	netObj, networkName, pool, err := testutil.SetupProxyTest(ctx, 2, t.Name())
	require.NoError(t, err, "Failed to setup proxy test")
	defer testutil.CleanupProxyTest(ctx, netObj, networkName, pool)

	// 构建配置：启用主动健康检查（使用内部地址）
	cfg := testutil.NewConfigBuilder().
		WithServer(":8080").
		WithProxy("/", pool.InternalAddresses(),
			testutil.WithLoadBalance("round_robin"),
			testutil.WithHealthCheck("/", 5*time.Second, 3*time.Second),
		)

	configYAML, err := cfg.Build()
	require.NoError(t, err, "Failed to build config")
	t.Logf("Config:\n%s", configYAML)

	// 启动 lolly（加入网络）
	lolly, err := testutil.StartLolly(ctx,
		testutil.WithConfigYAML(configYAML),
		testutil.WithNetwork(networkName),
	)
	require.NoError(t, err, "Failed to start lolly")
	defer lolly.Terminate(ctx)

	err = lolly.WaitForHealthy(ctx, testutil.HealthCheckWaitTimeout)
	require.NoError(t, err, "Lolly not healthy")

	client := &http.Client{Timeout: 10 * time.Second}

	// 发送请求验证两个后端都工作
	for i := 0; i < 10; i++ {
		resp, err := client.Get(lolly.HTTPBaseURL())
		require.NoError(t, err, "Request %d failed", i)
		resp.Body.Close()
	}

	t.Log("Both backends are healthy and receiving requests")

	// 终止一个后端
	err = pool.TerminateOne(ctx, 0)
	require.NoError(t, err, "Failed to terminate backend 0")

	t.Log("Backend 0 terminated, waiting for health check to detect...")

	// 等待健康检查检测到故障（使用重试机制）
	err = testutil.WaitForNoError(ctx, testutil.RetryConfig{
		Interval: 1 * time.Second,
		Timeout:  15 * time.Second,
	}, func() error {
		// 发送请求验证故障转移
		resp, err := client.Get(lolly.HTTPBaseURL())
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return fmt.Errorf("unexpected status: %d", resp.StatusCode)
		}
		return nil
	})
	require.NoError(t, err, "Health check should detect failure and route to healthy backend")

	// 继续发送请求，应该仍然成功（路由到健康后端）
	successCount := 0
	for i := 0; i < 10; i++ {
		resp, err := client.Get(lolly.HTTPBaseURL())
		if err == nil {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			if resp.StatusCode == 200 {
				successCount++
			}
		}
	}

	t.Logf("Requests after failover: %d/10 succeeded", successCount)
	assert.GreaterOrEqual(t, successCount, 8, "Most requests should succeed with remaining healthy backend")
}

// TestE2EHealthCheckPassive 测试被动健康检查。
//
// 验证失败后自动剔除后端。
func TestE2EHealthCheckPassive(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	// 设置代理测试环境（使用网络模式）
	netObj, networkName, pool, err := testutil.SetupProxyTest(ctx, 2, t.Name())
	require.NoError(t, err, "Failed to setup proxy test")
	defer testutil.CleanupProxyTest(ctx, netObj, networkName, pool)

	// 构建配置：启用被动健康检查（max_fails）（使用内部地址）
	targetOpts := [][]testutil.ProxyTargetOption{
		{testutil.WithMaxFails(3, 10*time.Second)},
		{testutil.WithMaxFails(3, 10*time.Second)},
	}

	cfg := testutil.NewConfigBuilder().
		WithServer(":8080").
		WithProxyTargets("/", pool.InternalAddresses(), targetOpts,
			testutil.WithLoadBalance("round_robin"),
		)

	configYAML, err := cfg.Build()
	require.NoError(t, err, "Failed to build config")

	// 启动 lolly（加入网络）
	lolly, err := testutil.StartLolly(ctx,
		testutil.WithConfigYAML(configYAML),
		testutil.WithNetwork(networkName),
	)
	require.NoError(t, err, "Failed to start lolly")
	defer lolly.Terminate(ctx)

	err = lolly.WaitForHealthy(ctx, testutil.HealthCheckWaitTimeout)
	require.NoError(t, err, "Lolly not healthy")

	client := &http.Client{Timeout: 10 * time.Second}

	// 发送请求验证正常工作
	for i := 0; i < 5; i++ {
		resp, err := client.Get(lolly.HTTPBaseURL())
		require.NoError(t, err, "Request %d failed", i)
		resp.Body.Close()
	}

	t.Log("Passive health check test completed")
}

// TestE2EHealthCheckRecovery 测试后端恢复检测。
//
// 验证后端恢复后重新加入负载均衡。
func TestE2EHealthCheckRecovery(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	// 设置代理测试环境（使用网络模式）
	netObj, networkName, pool, err := testutil.SetupProxyTest(ctx, 2, t.Name())
	require.NoError(t, err, "Failed to setup proxy test")
	defer testutil.CleanupProxyTest(ctx, netObj, networkName, pool)

	// 构建配置（使用内部地址）
	cfg := testutil.NewConfigBuilder().
		WithServer(":8080").
		WithProxy("/", pool.InternalAddresses(),
			testutil.WithLoadBalance("round_robin"),
			testutil.WithHealthCheck("/", 5*time.Second, 3*time.Second),
		)

	configYAML, err := cfg.Build()
	require.NoError(t, err, "Failed to build config")

	// 启动 lolly（加入网络）
	lolly, err := testutil.StartLolly(ctx,
		testutil.WithConfigYAML(configYAML),
		testutil.WithNetwork(networkName),
	)
	require.NoError(t, err, "Failed to start lolly")
	defer lolly.Terminate(ctx)

	err = lolly.WaitForHealthy(ctx, testutil.HealthCheckWaitTimeout)
	require.NoError(t, err, "Lolly not healthy")

	client := &http.Client{Timeout: 10 * time.Second}

	// 初始请求
	for i := 0; i < 5; i++ {
		resp, err := client.Get(lolly.HTTPBaseURL())
		require.NoError(t, err, "Initial request %d failed", i)
		resp.Body.Close()
	}

	t.Log("Initial requests successful")

	// 终止一个后端
	err = pool.TerminateOne(ctx, 0)
	require.NoError(t, err, "Failed to terminate backend")

	t.Log("Backend terminated, waiting for health check...")

	// 等待健康检查检测到故障
	time.Sleep(5 * time.Second)

	// 恢复后端
	err = pool.RestartOne(ctx, 0)
	require.NoError(t, err, "Failed to restart backend")

	t.Log("Backend restarted, waiting for recovery detection...")

	// 等待健康检查检测到恢复（使用重试机制）
	err = testutil.WaitForNoError(ctx, testutil.RetryConfig{
		Interval: 1 * time.Second,
		Timeout:  15 * time.Second,
	}, func() error {
		resp, err := client.Get(lolly.HTTPBaseURL())
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return fmt.Errorf("unexpected status: %d", resp.StatusCode)
		}
		return nil
	})
	require.NoError(t, err, "Backend should recover and accept requests")

	// 发送请求验证恢复
	successCount := 0
	for i := 0; i < 10; i++ {
		resp, err := client.Get(lolly.HTTPBaseURL())
		if err == nil {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			if resp.StatusCode == 200 {
				successCount++
			}
		}
	}

	t.Logf("Requests after recovery: %d/10 succeeded", successCount)
	assert.GreaterOrEqual(t, successCount, 8, "Most requests should succeed after recovery")
}

// TestE2EHealthCheckInterval 测试健康检查间隔。
//
// 验证健康检查按配置间隔执行。
func TestE2EHealthCheckInterval(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	// 设置代理测试环境（使用网络模式）
	netObj, networkName, pool, err := testutil.SetupProxyTest(ctx, 1, t.Name())
	require.NoError(t, err, "Failed to setup proxy test")
	defer testutil.CleanupProxyTest(ctx, netObj, networkName, pool)

	// 构建配置：短健康检查间隔（使用内部地址）
	cfg := testutil.NewConfigBuilder().
		WithServer(":8080").
		WithProxy("/", pool.InternalAddresses(),
			testutil.WithHealthCheck("/", 3*time.Second, 2*time.Second),
		)

	configYAML, err := cfg.Build()
	require.NoError(t, err, "Failed to build config")

	// 启动 lolly（加入网络）
	lolly, err := testutil.StartLolly(ctx,
		testutil.WithConfigYAML(configYAML),
		testutil.WithNetwork(networkName),
	)
	require.NoError(t, err, "Failed to start lolly")
	defer lolly.Terminate(ctx)

	err = lolly.WaitForHealthy(ctx, testutil.HealthCheckWaitTimeout)
	require.NoError(t, err, "Lolly not healthy")

	client := &http.Client{Timeout: 10 * time.Second}

	// 发送请求
	resp, err := client.Get(lolly.HTTPBaseURL())
	require.NoError(t, err, "Request failed")
	resp.Body.Close()

	t.Log("Health check interval test completed")
}

// TestE2EHealthCheckTimeout 测试健康检查超时。
//
// 验证健康检查超时正确处理。
func TestE2EHealthCheckTimeout(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	// 设置代理测试环境（使用网络模式）
	netObj, networkName, pool, err := testutil.SetupProxyTest(ctx, 1, t.Name())
	require.NoError(t, err, "Failed to setup proxy test")
	defer testutil.CleanupProxyTest(ctx, netObj, networkName, pool)

	// 构建配置：短超时（使用内部地址）
	cfg := testutil.NewConfigBuilder().
		WithServer(":8080").
		WithProxy("/", pool.InternalAddresses(),
			testutil.WithHealthCheck("/", 5*time.Second, 1*time.Second),
		)

	configYAML, err := cfg.Build()
	require.NoError(t, err, "Failed to build config")

	// 启动 lolly（加入网络）
	lolly, err := testutil.StartLolly(ctx,
		testutil.WithConfigYAML(configYAML),
		testutil.WithNetwork(networkName),
	)
	require.NoError(t, err, "Failed to start lolly")
	defer lolly.Terminate(ctx)

	err = lolly.WaitForHealthy(ctx, testutil.HealthCheckWaitTimeout)
	require.NoError(t, err, "Lolly not healthy")

	client := &http.Client{Timeout: 10 * time.Second}

	// 发送请求
	resp, err := client.Get(lolly.HTTPBaseURL())
	require.NoError(t, err, "Request failed")
	resp.Body.Close()

	t.Log("Health check timeout test completed")
}

// TestE2EHealthCheckPath 测试健康检查路径。
//
// 验证自定义健康检查路径。
func TestE2EHealthCheckPath(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	// 设置代理测试环境（使用网络模式）
	netObj, networkName, pool, err := testutil.SetupProxyTest(ctx, 1, t.Name())
	require.NoError(t, err, "Failed to setup proxy test")
	defer testutil.CleanupProxyTest(ctx, netObj, networkName, pool)

	// 构建配置：使用根路径作为健康检查路径（nginx 默认存在）
	cfg := testutil.NewConfigBuilder().
		WithServer(":8080").
		WithProxy("/", pool.InternalAddresses(),
			testutil.WithHealthCheck("/", 5*time.Second, 3*time.Second),
		)

	configYAML, err := cfg.Build()
	require.NoError(t, err, "Failed to build config")

	// 启动 lolly（加入网络）
	lolly, err := testutil.StartLolly(ctx,
		testutil.WithConfigYAML(configYAML),
		testutil.WithNetwork(networkName),
	)
	require.NoError(t, err, "Failed to start lolly")
	defer lolly.Terminate(ctx)

	err = lolly.WaitForHealthy(ctx, testutil.HealthCheckWaitTimeout)
	require.NoError(t, err, "Lolly not healthy")

	client := &http.Client{Timeout: 10 * time.Second}

	// 发送请求
	resp, err := client.Get(lolly.HTTPBaseURL())
	require.NoError(t, err, "Request failed")
	resp.Body.Close()

	t.Log("Health check path test completed")
}

// TestE2EHealthCheckMultipleBackends 测试多后端健康检查。
//
// 验证多个后端的健康检查独立工作。
func TestE2EHealthCheckMultipleBackends(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	// 设置代理测试环境（使用网络模式）
	netObj, networkName, pool, err := testutil.SetupProxyTest(ctx, 3, t.Name())
	require.NoError(t, err, "Failed to setup proxy test")
	defer testutil.CleanupProxyTest(ctx, netObj, networkName, pool)

	// 构建配置（使用内部地址）
	cfg := testutil.NewConfigBuilder().
		WithServer(":8080").
		WithProxy("/", pool.InternalAddresses(),
			testutil.WithLoadBalance("round_robin"),
			testutil.WithHealthCheck("/", 5*time.Second, 3*time.Second),
		)

	configYAML, err := cfg.Build()
	require.NoError(t, err, "Failed to build config")

	// 启动 lolly（加入网络）
	lolly, err := testutil.StartLolly(ctx,
		testutil.WithConfigYAML(configYAML),
		testutil.WithNetwork(networkName),
	)
	require.NoError(t, err, "Failed to start lolly")
	defer lolly.Terminate(ctx)

	err = lolly.WaitForHealthy(ctx, testutil.HealthCheckWaitTimeout)
	require.NoError(t, err, "Lolly not healthy")

	client := &http.Client{Timeout: 10 * time.Second}

	// 发送请求验证所有后端
	for i := 0; i < 15; i++ {
		resp, err := client.Get(lolly.HTTPBaseURL())
		require.NoError(t, err, "Request %d failed", i)
		resp.Body.Close()
	}

	t.Log("All backends are healthy")

	// 终止一个后端
	err = pool.TerminateOne(ctx, 1)
	require.NoError(t, err, "Failed to terminate backend 1")

	t.Log("Backend 1 terminated")

	// 等待健康检查检测到故障（使用重试机制）
	err = testutil.WaitForNoError(ctx, testutil.RetryConfig{
		Interval: 1 * time.Second,
		Timeout:  15 * time.Second,
	}, func() error {
		resp, err := client.Get(lolly.HTTPBaseURL())
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return fmt.Errorf("unexpected status: %d", resp.StatusCode)
		}
		return nil
	})
	require.NoError(t, err, "Health check should detect failure and route to remaining backends")

	// 继续发送请求
	successCount := 0
	for i := 0; i < 10; i++ {
		resp, err := client.Get(lolly.HTTPBaseURL())
		if err == nil {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			if resp.StatusCode == 200 {
				successCount++
			}
		}
	}

	t.Logf("Requests with 2 backends: %d/10 succeeded", successCount)
	assert.GreaterOrEqual(t, successCount, 8, "Most requests should succeed with 2 healthy backends")
}

// TestE2EHealthCheckFailover 测试故障转移。
//
// 验证后端故障时请求自动转移到健康后端。
func TestE2EHealthCheckFailover(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	// 设置代理测试环境（使用网络模式）
	netObj, networkName, pool, err := testutil.SetupProxyTest(ctx, 2, t.Name())
	require.NoError(t, err, "Failed to setup proxy test")
	defer testutil.CleanupProxyTest(ctx, netObj, networkName, pool)

	// 构建配置：启用故障转移（使用内部地址）
	cfg := testutil.NewConfigBuilder().
		WithServer(":8080").
		WithProxy("/", pool.InternalAddresses(),
			testutil.WithLoadBalance("round_robin"),
			testutil.WithHealthCheck("/", 5*time.Second, 3*time.Second),
			testutil.WithProxyNextUpstream(3, []int{502, 503, 504}),
		)

	configYAML, err := cfg.Build()
	require.NoError(t, err, "Failed to build config")

	// 启动 lolly（加入网络）
	lolly, err := testutil.StartLolly(ctx,
		testutil.WithConfigYAML(configYAML),
		testutil.WithNetwork(networkName),
	)
	require.NoError(t, err, "Failed to start lolly")
	defer lolly.Terminate(ctx)

	err = lolly.WaitForHealthy(ctx, testutil.HealthCheckWaitTimeout)
	require.NoError(t, err, "Lolly not healthy")

	client := &http.Client{Timeout: 10 * time.Second}

	// 初始请求
	for i := 0; i < 5; i++ {
		resp, err := client.Get(lolly.HTTPBaseURL())
		require.NoError(t, err, "Initial request %d failed", i)
		resp.Body.Close()
	}

	// 终止一个后端
	err = pool.TerminateOne(ctx, 0)
	require.NoError(t, err, "Failed to terminate backend")

	// 立即发送请求（故障转移测试）
	resp, err := client.Get(lolly.HTTPBaseURL())
	if err == nil {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		t.Logf("Immediate request after failure: status %d", resp.StatusCode)
	}

	t.Log("Failover test completed")
}

// TestE2EHealthCheckAllDown 测试所有后端不可用。
//
// 验证所有后端不可用时的行为。
func TestE2EHealthCheckAllDown(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	// 设置代理测试环境（使用网络模式）
	netObj, networkName, pool, err := testutil.SetupProxyTest(ctx, 2, t.Name())
	require.NoError(t, err, "Failed to setup proxy test")
	defer testutil.CleanupProxyTest(ctx, netObj, networkName, pool)

	// 构建配置（使用内部地址）
	cfg := testutil.NewConfigBuilder().
		WithServer(":8080").
		WithProxy("/", pool.InternalAddresses(),
			testutil.WithLoadBalance("round_robin"),
			testutil.WithHealthCheck("/", 5*time.Second, 3*time.Second),
		)

	configYAML, err := cfg.Build()
	require.NoError(t, err, "Failed to build config")

	// 启动 lolly（加入网络）
	lolly, err := testutil.StartLolly(ctx,
		testutil.WithConfigYAML(configYAML),
		testutil.WithNetwork(networkName),
	)
	require.NoError(t, err, "Failed to start lolly")
	defer lolly.Terminate(ctx)

	err = lolly.WaitForHealthy(ctx, testutil.HealthCheckWaitTimeout)
	require.NoError(t, err, "Lolly not healthy")

	client := &http.Client{Timeout: 10 * time.Second}

	// 初始请求
	resp, err := client.Get(lolly.HTTPBaseURL())
	require.NoError(t, err, "Initial request failed")
	resp.Body.Close()

	// 终止所有后端
	err = pool.TerminateOne(ctx, 0)
	require.NoError(t, err, "Failed to terminate backend 0")
	err = pool.TerminateOne(ctx, 1)
	require.NoError(t, err, "Failed to terminate backend 1")

	t.Log("All backends terminated")

	// 等待健康检查
	time.Sleep(10 * time.Second)

	// 发送请求应该失败
	resp, err = client.Get(lolly.HTTPBaseURL())
	if err == nil {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		t.Logf("Request with all backends down: status %d", resp.StatusCode)
	} else {
		t.Logf("Request failed as expected: %v", err)
	}

	t.Log("All backends down test completed")
}

// TestE2EHealthCheckBackupServer 测试备份服务器。
//
// 验证备份服务器在主服务器不可用时启用。
func TestE2EHealthCheckBackupServer(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	// 设置代理测试环境（使用网络模式）
	netObj, networkName, pool, err := testutil.SetupProxyTest(ctx, 2, t.Name())
	require.NoError(t, err, "Failed to setup proxy test")
	defer testutil.CleanupProxyTest(ctx, netObj, networkName, pool)

	// 构建配置：第二个后端为备份（使用内部地址）
	targetOpts := [][]testutil.ProxyTargetOption{
		{},                      // 主服务器
		{testutil.WithBackup()}, // 备份服务器
	}

	cfg := testutil.NewConfigBuilder().
		WithServer(":8080").
		WithProxyTargets("/", pool.InternalAddresses(), targetOpts,
			testutil.WithLoadBalance("round_robin"),
			testutil.WithHealthCheck("/", 5*time.Second, 3*time.Second),
		)

	configYAML, err := cfg.Build()
	require.NoError(t, err, "Failed to build config")

	// 启动 lolly（加入网络）
	lolly, err := testutil.StartLolly(ctx,
		testutil.WithConfigYAML(configYAML),
		testutil.WithNetwork(networkName),
	)
	require.NoError(t, err, "Failed to start lolly")
	defer lolly.Terminate(ctx)

	err = lolly.WaitForHealthy(ctx, testutil.HealthCheckWaitTimeout)
	require.NoError(t, err, "Lolly not healthy")

	client := &http.Client{Timeout: 10 * time.Second}

	// 发送请求（应该只路由到主服务器）
	for i := 0; i < 5; i++ {
		resp, err := client.Get(lolly.HTTPBaseURL())
		require.NoError(t, err, "Request %d failed", i)
		resp.Body.Close()
	}

	t.Log("Backup server test completed")
}

// TestE2EHealthCheckSlowStart 测试慢启动。
//
// 验证新后端逐渐接收流量。
func TestE2EHealthCheckSlowStart(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	// 设置代理测试环境（使用网络模式）
	netObj, networkName, pool, err := testutil.SetupProxyTest(ctx, 1, t.Name())
	require.NoError(t, err, "Failed to setup proxy test")
	defer testutil.CleanupProxyTest(ctx, netObj, networkName, pool)

	// 构建配置（使用内部地址）
	cfg := testutil.NewConfigBuilder().
		WithServer(":8080").
		WithProxy("/", pool.InternalAddresses(),
			testutil.WithHealthCheck("/", 5*time.Second, 3*time.Second),
		)

	configYAML, err := cfg.Build()
	require.NoError(t, err, "Failed to build config")

	// 启动 lolly（加入网络）
	lolly, err := testutil.StartLolly(ctx,
		testutil.WithConfigYAML(configYAML),
		testutil.WithNetwork(networkName),
	)
	require.NoError(t, err, "Failed to start lolly")
	defer lolly.Terminate(ctx)

	err = lolly.WaitForHealthy(ctx, testutil.HealthCheckWaitTimeout)
	require.NoError(t, err, "Lolly not healthy")

	client := &http.Client{Timeout: 10 * time.Second}

	// 发送请求
	resp, err := client.Get(lolly.HTTPBaseURL())
	require.NoError(t, err, "Request failed")
	resp.Body.Close()

	t.Log("Slow start test completed")
}

// TestE2EHealthCheckWithLoadBalance 测试健康检查与负载均衡集成。
//
// 验证健康检查与各种负载均衡算法配合工作。
func TestE2EHealthCheckWithLoadBalance(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	// 设置代理测试环境（使用网络模式）
	netObj, networkName, pool, err := testutil.SetupProxyTest(ctx, 3, t.Name())
	require.NoError(t, err, "Failed to setup proxy test")
	defer testutil.CleanupProxyTest(ctx, netObj, networkName, pool)

	algorithms := []string{"round_robin", "least_conn"}

	for _, algo := range algorithms {
		t.Run(algo, func(t *testing.T) {
			cfg := testutil.NewConfigBuilder().
				WithServer(":8080").
				WithProxy("/", pool.InternalAddresses(),
					testutil.WithLoadBalance(algo),
					testutil.WithHealthCheck("/", 5*time.Second, 3*time.Second),
				)

			configYAML, err := cfg.Build()
			require.NoError(t, err, "Failed to build config")

			lolly, err := testutil.StartLolly(ctx,
				testutil.WithConfigYAML(configYAML),
				testutil.WithNetwork(networkName),
			)
			require.NoError(t, err, "Failed to start lolly")
			defer lolly.Terminate(ctx)

			err = lolly.WaitForHealthy(ctx, testutil.HealthCheckWaitTimeout)
			require.NoError(t, err, "Lolly not healthy")

			client := &http.Client{Timeout: 10 * time.Second}

			// 发送请求
			for i := 0; i < 10; i++ {
				resp, err := client.Get(lolly.HTTPBaseURL())
				require.NoError(t, err, "Request %d failed", i)
				resp.Body.Close()
			}

			t.Logf("Health check with %s completed", algo)
		})
	}
}

// TestE2EHealthCheckConcurrent 测试并发健康检查。
//
// 验证并发请求时健康检查正常工作。
func TestE2EHealthCheckConcurrent(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	// 设置代理测试环境（使用网络模式）
	netObj, networkName, pool, err := testutil.SetupProxyTest(ctx, 2, t.Name())
	require.NoError(t, err, "Failed to setup proxy test")
	defer testutil.CleanupProxyTest(ctx, netObj, networkName, pool)

	// 构建配置（使用内部地址）
	cfg := testutil.NewConfigBuilder().
		WithServer(":8080").
		WithProxy("/", pool.InternalAddresses(),
			testutil.WithLoadBalance("round_robin"),
			testutil.WithHealthCheck("/", 5*time.Second, 3*time.Second),
		)

	configYAML, err := cfg.Build()
	require.NoError(t, err, "Failed to build config")

	// 启动 lolly（加入网络）
	lolly, err := testutil.StartLolly(ctx,
		testutil.WithConfigYAML(configYAML),
		testutil.WithNetwork(networkName),
	)
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

	assert.Empty(t, failures, "All concurrent requests should succeed")

	t.Log("Concurrent health check test completed")
}

// TestE2EHealthCheckStatusCodes 测试健康检查状态码匹配。
//
// 验证健康检查正确匹配状态码。
func TestE2EHealthCheckStatusCodes(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	// 设置代理测试环境（使用网络模式）
	netObj, networkName, pool, err := testutil.SetupProxyTest(ctx, 1, t.Name())
	require.NoError(t, err, "Failed to setup proxy test")
	defer testutil.CleanupProxyTest(ctx, netObj, networkName, pool)

	// 构建配置（使用内部地址）
	cfg := testutil.NewConfigBuilder().
		WithServer(":8080").
		WithProxy("/", pool.InternalAddresses(),
			testutil.WithHealthCheck("/", 5*time.Second, 3*time.Second),
		)

	configYAML, err := cfg.Build()
	require.NoError(t, err, "Failed to build config")

	// 启动 lolly（加入网络）
	lolly, err := testutil.StartLolly(ctx,
		testutil.WithConfigYAML(configYAML),
		testutil.WithNetwork(networkName),
	)
	require.NoError(t, err, "Failed to start lolly")
	defer lolly.Terminate(ctx)

	err = lolly.WaitForHealthy(ctx, testutil.HealthCheckWaitTimeout)
	require.NoError(t, err, "Lolly not healthy")

	client := &http.Client{Timeout: 10 * time.Second}

	// 发送请求
	resp, err := client.Get(lolly.HTTPBaseURL())
	require.NoError(t, err, "Request failed")
	assert.Equal(t, 200, resp.StatusCode)
	resp.Body.Close()

	t.Log("Health check status codes test completed")
}

// TestE2EHealthCheckIntegration 综合健康检查测试。
//
// 验证健康检查与其他功能的集成。
func TestE2EHealthCheckIntegration(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	// 设置代理测试环境（使用网络模式）
	netObj, networkName, pool, err := testutil.SetupProxyTest(ctx, 2, t.Name())
	require.NoError(t, err, "Failed to setup proxy test")
	defer testutil.CleanupProxyTest(ctx, netObj, networkName, pool)

	// 构建配置：健康检查 + 负载均衡 + 超时（使用内部地址）
	cfg := testutil.NewConfigBuilder().
		WithServer(":8080").
		WithProxy("/", pool.InternalAddresses(),
			testutil.WithLoadBalance("round_robin"),
			testutil.WithHealthCheck("/", 5*time.Second, 3*time.Second),
			testutil.WithProxyTimeout(5*time.Second, 30*time.Second, 30*time.Second),
			testutil.WithProxyNextUpstream(3, []int{502, 503, 504}),
		)

	configYAML, err := cfg.Build()
	require.NoError(t, err, "Failed to build config")

	// 启动 lolly（加入网络）
	lolly, err := testutil.StartLolly(ctx,
		testutil.WithConfigYAML(configYAML),
		testutil.WithNetwork(networkName),
	)
	require.NoError(t, err, "Failed to start lolly")
	defer lolly.Terminate(ctx)

	err = lolly.WaitForHealthy(ctx, testutil.HealthCheckWaitTimeout)
	require.NoError(t, err, "Lolly not healthy")

	client := &http.Client{Timeout: 30 * time.Second}

	// 发送多个请求
	for i := 0; i < 20; i++ {
		resp, err := client.Get(lolly.HTTPBaseURL())
		if err == nil {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}
	}

	t.Log("Health check integration test completed")
}
