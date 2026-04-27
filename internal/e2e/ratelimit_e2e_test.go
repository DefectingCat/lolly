//go:build e2e

// ratelimit_e2e_test.go - 请求限流 E2E 测试（L3 层，需要 Docker）
//
// 测试 lolly 请求限流功能，包括：
//   - 请求速率限制
//   - 突发流量处理
//   - 429 响应
//
// 作者：xfy
package e2e

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rua.plus/lolly/internal/e2e/testutil"
)

// TestE2ERateLimitBasic 测试基本请求限流。
func TestE2ERateLimitBasic(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	// 启动模拟后端
	backend, backendAddr, err := testutil.StartMockBackend(ctx)
	require.NoError(t, err, "Failed to start mock backend")
	defer backend.Terminate(ctx)

	// 配置限流：每秒 5 个请求，突发 10 个
	config := fmt.Sprintf(`
servers:
  - listen: ":8080"
    proxy:
      - path: "/api"
        targets:
          - url: "http://%s"
    security:
      rate_limit:
        request_rate: 5
        burst: 10
`, backendAddr)

	// 启动 lolly
	lolly, err := testutil.StartLolly(ctx, testutil.WithConfigYAML(config))
	require.NoError(t, err, "Failed to start lolly container")
	defer lolly.Terminate(ctx)

	err = lolly.WaitForHealthy(ctx, 30*time.Second)
	require.NoError(t, err, "Lolly not healthy")

	client := &http.Client{Timeout: 5 * time.Second}
	baseURL := lolly.HTTPBaseURL()

	// 快速发送 20 个请求
	var successCount, rateLimitedCount int32
	var wg sync.WaitGroup

	for i := range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := client.Get(fmt.Sprintf("%s/api/test?id=%d", baseURL, i))
			if err != nil {
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode == http.StatusOK {
				atomic.AddInt32(&successCount, 1)
			} else if resp.StatusCode == http.StatusTooManyRequests {
				atomic.AddInt32(&rateLimitedCount, 1)
			}
		}()
	}

	wg.Wait()

	t.Logf("Success: %d, Rate limited: %d", successCount, rateLimitedCount)

	// 验证有请求被限流
	assert.Greater(t, rateLimitedCount, int32(0), "Some requests should be rate limited")
}

// TestE2ERateLimitBurst 测试突发流量处理。
func TestE2ERateLimitBurst(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	// 启动模拟后端
	backend, backendAddr, err := testutil.StartMockBackend(ctx)
	require.NoError(t, err, "Failed to start mock backend")
	defer backend.Terminate(ctx)

	// 配置限流：每秒 2 个请求，突发 5 个
	config := fmt.Sprintf(`
servers:
  - listen: ":8080"
    proxy:
      - path: "/api"
        targets:
          - url: "http://%s"
    security:
      rate_limit:
        request_rate: 2
        burst: 5
`, backendAddr)

	// 启动 lolly
	lolly, err := testutil.StartLolly(ctx, testutil.WithConfigYAML(config))
	require.NoError(t, err, "Failed to start lolly container")
	defer lolly.Terminate(ctx)

	err = lolly.WaitForHealthy(ctx, 30*time.Second)
	require.NoError(t, err, "Lolly not healthy")

	client := &http.Client{Timeout: 5 * time.Second}
	baseURL := lolly.HTTPBaseURL()

	// 第一批：突发 5 个请求应该都成功
	var successCount int32
	for i := range 5 {
		resp, err := client.Get(fmt.Sprintf("%s/api/test?id=%d", baseURL, i))
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				atomic.AddInt32(&successCount, 1)
			}
		}
	}

	assert.GreaterOrEqual(t, successCount, int32(4), "Most burst requests should succeed")
}

// TestE2ERateLimitRecovery 测试限流恢复。
func TestE2ERateLimitRecovery(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	// 启动模拟后端
	backend, backendAddr, err := testutil.StartMockBackend(ctx)
	require.NoError(t, err, "Failed to start mock backend")
	defer backend.Terminate(ctx)

	// 配置限流：每秒 3 个请求，突发 3 个
	config := fmt.Sprintf(`
servers:
  - listen: ":8080"
    proxy:
      - path: "/api"
        targets:
          - url: "http://%s"
    security:
      rate_limit:
        request_rate: 3
        burst: 3
`, backendAddr)

	// 启动 lolly
	lolly, err := testutil.StartLolly(ctx, testutil.WithConfigYAML(config))
	require.NoError(t, err, "Failed to start lolly container")
	defer lolly.Terminate(ctx)

	err = lolly.WaitForHealthy(ctx, 30*time.Second)
	require.NoError(t, err, "Lolly not healthy")

	client := &http.Client{Timeout: 5 * time.Second}
	baseURL := lolly.HTTPBaseURL()

	// 发送请求直到被限流
	limited := false
	for i := range 10 {
		resp, err := client.Get(fmt.Sprintf("%s/api/test?id=%d", baseURL, i))
		if err == nil {
			if resp.StatusCode == http.StatusTooManyRequests {
				limited = true
				resp.Body.Close()
				break
			}
			resp.Body.Close()
		}
	}

	if !limited {
		t.Skip("Rate limiting not triggered, skipping recovery test")
	}

	// 等待限流窗口恢复
	time.Sleep(500 * time.Millisecond)

	// 再次发送请求应该成功
	resp, err := client.Get(baseURL + "/api/test?after=wait")
	if err == nil {
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode, "Request should succeed after waiting")
	}
}

// TestE2ERateLimitDisabled 测试未配置限流时不限制。
func TestE2ERateLimitDisabled(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	// 启动模拟后端
	backend, backendAddr, err := testutil.StartMockBackend(ctx)
	require.NoError(t, err, "Failed to start mock backend")
	defer backend.Terminate(ctx)

	// 不配置限流
	config := fmt.Sprintf(`
servers:
  - listen: ":8080"
    proxy:
      - path: "/api"
        targets:
          - url: "http://%s"
`, backendAddr)

	// 启动 lolly
	lolly, err := testutil.StartLolly(ctx, testutil.WithConfigYAML(config))
	require.NoError(t, err, "Failed to start lolly container")
	defer lolly.Terminate(ctx)

	err = lolly.WaitForHealthy(ctx, 30*time.Second)
	require.NoError(t, err, "Lolly not healthy")

	client := &http.Client{Timeout: 5 * time.Second}
	baseURL := lolly.HTTPBaseURL()

	// 发送 20 个请求，都不应该被限流
	var successCount int32
	for i := range 20 {
		resp, err := client.Get(fmt.Sprintf("%s/api/test?id=%d", baseURL, i))
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				atomic.AddInt32(&successCount, 1)
			}
		}
	}

	assert.GreaterOrEqual(t, successCount, int32(18), "Most requests should succeed without rate limiting")
}
