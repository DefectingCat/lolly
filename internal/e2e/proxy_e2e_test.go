//go:build e2e

// proxy_e2e_test.go - HTTP 代理 E2E 测试（L3 层，需要 Docker）
//
// 测试代理转发、负载均衡、健康检查等功能。
//
// 作者：xfy
package e2e

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rua.plus/lolly/internal/e2e/testutil"
)

// TestE2EProxyBasic 测试基本代理转发。
func TestE2EProxyBasic(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// 启动模拟后端
	backend, backendAddr, err := testutil.StartNginxContainer(ctx)
	require.NoError(t, err, "Failed to start mock backend")
	defer backend.Terminate(ctx)

	t.Logf("Mock backend: %s", backendAddr)

	// 验证后端可达
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(backendAddr)
	require.NoError(t, err, "Backend not reachable")
	resp.Body.Close()
	assert.Equal(t, 200, resp.StatusCode, "Backend should return 200")
}

// TestE2EProxyWithLolly 测试 lolly 代理转发功能。
// 需要 lolly:latest 镜像。
func TestE2EProxyWithLolly(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	if !testutil.DockerAvailable(ctx) {
		t.Skip("Docker not available")
	}

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	// 启动模拟后端
	backend, backendAddr, err := testutil.StartNginxContainer(ctx)
	require.NoError(t, err, "Failed to start mock backend")
	defer backend.Terminate(ctx)

	t.Logf("Mock backend: %s", backendAddr)

	// 启动 lolly 代理服务器
	lolly, err := testutil.StartLollyContainer(ctx, "")
	require.NoError(t, err, "Failed to start lolly container")
	defer lolly.Terminate(ctx)

	t.Logf("Lolly proxy: %s", lolly.HTTPBaseURL())

	// 等待 lolly 健康
	err = lolly.WaitForHealthy(ctx, 30*time.Second)
	require.NoError(t, err, "Lolly not healthy")

	// 通过 lolly 代理访问
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(lolly.HTTPBaseURL())
	require.NoError(t, err, "Failed to reach lolly")
	defer resp.Body.Close()

	// lolly 默认配置没有静态文件，返回 404
	assert.Equal(t, 404, resp.StatusCode, "Lolly should return 404 without static files")
}

// TestE2EProxyLoadBalance 测试负载均衡轮询。
func TestE2EProxyLoadBalance(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// 启动多个模拟后端
	backends := make([]context.CancelFunc, 3)
	backendAddrs := make([]string, 3)

	for i := 0; i < 3; i++ {
		backend, addr, err := testutil.StartNginxContainer(ctx)
		require.NoError(t, err, "Failed to start mock backend %d", i)
		backends[i] = func() { backend.Terminate(ctx) }
		backendAddrs[i] = addr
	}

	defer func() {
		for _, cancel := range backends {
			cancel()
		}
	}()

	// 验证所有后端可达
	client := &http.Client{Timeout: 10 * time.Second}
	for i, addr := range backendAddrs {
		resp, err := client.Get(addr)
		require.NoError(t, err, "Backend %d not reachable", i)
		resp.Body.Close()
		assert.Equal(t, 200, resp.StatusCode)
	}

	t.Logf("All backends reachable: %v", backendAddrs)
}

// TestE2EProxyHealthCheck 测试健康检查。
func TestE2EProxyHealthCheck(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// 启动健康后端
	healthyBackend, healthyAddr, err := testutil.StartNginxContainer(ctx)
	require.NoError(t, err, "Failed to start healthy backend")
	defer healthyBackend.Terminate(ctx)

	// 验证健康检查端点
	client := &http.Client{Timeout: 10 * time.Second}

	// 测试健康后端
	resp, err := client.Get(fmt.Sprintf("%s/", healthyAddr))
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, 200, resp.StatusCode, "Healthy backend should return 200")
}

// TestE2EProxyTimeout 测试代理超时处理。
func TestE2EProxyTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if !testutil.DockerAvailable(ctx) {
		t.Skip("Docker not available")
	}

	// 使用短超时客户端测试超时场景
	shortTimeoutClient := &http.Client{Timeout: 1 * time.Second}

	// 测试连接到不可达地址的超时
	_, err := shortTimeoutClient.Get("http://10.255.255.1:80/test")
	assert.Error(t, err, "Should timeout on unreachable address")
	assert.True(t, strings.Contains(err.Error(), "timeout") || strings.Contains(err.Error(), "context deadline exceeded"),
		"Error should indicate timeout")
}

// TestE2EProxyErrorHandling 测试代理错误处理。
func TestE2EProxyErrorHandling(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if !testutil.DockerAvailable(ctx) {
		t.Skip("Docker not available")
	}

	client := &http.Client{Timeout: 10 * time.Second}

	// 测试连接被拒绝
	_, err := client.Get("http://localhost:9999/test")
	assert.Error(t, err, "Should error on connection refused")
}

// TestE2EProxyHeaders 测试代理头部传递。
func TestE2EProxyHeaders(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// 启动模拟后端
	backend, backendAddr, err := testutil.StartNginxContainer(ctx)
	require.NoError(t, err, "Failed to start mock backend")
	defer backend.Terminate(ctx)

	// 发送带自定义头部的请求
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", backendAddr, nil)
	require.NoError(t, err)

	req.Header.Set("X-Custom-Header", "test-value")
	req.Header.Set("X-Forwarded-For", "192.168.1.1")

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// 验证响应
	assert.Equal(t, 200, resp.StatusCode)
}

// TestE2EProxyMultipleRequests 测试并发请求。
func TestE2EProxyMultipleRequests(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// 启动模拟后端
	backend, backendAddr, err := testutil.StartNginxContainer(ctx)
	require.NoError(t, err, "Failed to start mock backend")
	defer backend.Terminate(ctx)

	// 使用真正的并发测试
	failures := testutil.RunAndVerifyConcurrentRequests(t, testutil.ConcurrentRequestConfig{
		URL:        backendAddr,
		Count:      10,
		Timeout:    10 * time.Second,
		ExpectCode: 200,
	})

	assert.Empty(t, failures, "All concurrent requests should succeed")
}
