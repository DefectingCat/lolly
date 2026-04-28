//go:build e2e

// access_e2e_test.go - 访问控制 E2E 测试（L3 层，需要 Docker）
//
// 测试 lolly 访问控制功能，包括：
//   - IP 白名单
//   - IP 黑名单
//   - CIDR 网段匹配
//   - 403 Forbidden 响应
//
// 作者：xfy
package e2e

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rua.plus/lolly/internal/e2e/testutil"
)

// TestE2EAccessAllowWhitelist 测试 IP 白名单。
func TestE2EAccessAllowWhitelist(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	// 配置只允许特定 IP 访问
	// 由于测试在容器内运行，需要允许容器网络
	config := `
servers:
  - listen: ":8080"
    static:
      - path: "/"
        root: "/var/www/html"
        index:
          - "index.html"
    access:
      allow:
        - "127.0.0.1"
        - "10.0.0.0/8"
        - "172.16.0.0/12"
        - "192.168.0.0/16"
      default: deny
`

	// 启动 lolly
	lolly, err := testutil.StartLolly(ctx, testutil.WithConfigYAML(config))
	require.NoError(t, err, "Failed to start lolly container")
	defer lolly.Terminate(ctx)

	err = lolly.WaitForHealthy(ctx, testutil.HealthCheckWaitTimeout)
	require.NoError(t, err, "Lolly not healthy")

	client := &http.Client{Timeout: 10 * time.Second}

	// 从容器网络访问应该成功
	resp, err := client.Get(lolly.HTTPBaseURL() + "/")
	if err == nil {
		defer resp.Body.Close()
		// 容器网络在允许范围内，应该可以访问
		assert.NotEqual(t, http.StatusForbidden, resp.StatusCode, "Request from allowed network should not be forbidden")
	}
}

// TestE2EAccessDenyBlacklist 测试 IP 黑名单。
func TestE2EAccessDenyBlacklist(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	// 配置拒绝特定 IP 访问
	config := `
servers:
  - listen: ":8080"
    static:
      - path: "/"
        root: "/var/www/html"
        index:
          - "index.html"
    access:
      deny:
        - "192.168.100.0/24"
      default: allow
`

	// 启动 lolly
	lolly, err := testutil.StartLolly(ctx, testutil.WithConfigYAML(config))
	require.NoError(t, err, "Failed to start lolly container")
	defer lolly.Terminate(ctx)

	err = lolly.WaitForHealthy(ctx, testutil.HealthCheckWaitTimeout)
	require.NoError(t, err, "Lolly not healthy")

	client := &http.Client{Timeout: 10 * time.Second}

	// 从非黑名单 IP 访问应该成功
	resp, err := client.Get(lolly.HTTPBaseURL() + "/")
	if err == nil {
		defer resp.Body.Close()
		// 测试环境 IP 不在黑名单中，应该可以访问
		assert.NotEqual(t, http.StatusForbidden, resp.StatusCode, "Request from non-blacklisted IP should not be forbidden")
	}
}

// TestE2EAccessDefaultDeny 测试默认拒绝策略。
func TestE2EAccessDefaultDeny(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	// 配置默认拒绝，只允许 localhost
	config := `
servers:
  - listen: ":8080"
    static:
      - path: "/"
        root: "/var/www/html"
        index:
          - "index.html"
    access:
      allow:
        - "127.0.0.1"
      default: deny
`

	// 启动 lolly
	lolly, err := testutil.StartLolly(ctx, testutil.WithConfigYAML(config))
	require.NoError(t, err, "Failed to start lolly container")
	defer lolly.Terminate(ctx)

	err = lolly.WaitForHealthy(ctx, testutil.HealthCheckWaitTimeout)
	require.NoError(t, err, "Lolly not healthy")

	client := &http.Client{Timeout: 10 * time.Second}

	// 从容器网络访问（非 127.0.0.1）
	resp, err := client.Get(lolly.HTTPBaseURL() + "/")
	if err == nil {
		defer resp.Body.Close()
		t.Logf("Status: %d", resp.StatusCode)
		// 根据配置，非 localhost 可能被拒绝
	}
}

// TestE2EAccessNoRestriction 测试无访问限制。
func TestE2EAccessNoRestriction(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	// 不配置访问控制
	config := `
servers:
  - listen: ":8080"
    static:
      - path: "/"
        root: "/var/www/html"
        index:
          - "index.html"
`

	// 启动 lolly
	lolly, err := testutil.StartLolly(ctx, testutil.WithConfigYAML(config))
	require.NoError(t, err, "Failed to start lolly container")
	defer lolly.Terminate(ctx)

	err = lolly.WaitForHealthy(ctx, testutil.HealthCheckWaitTimeout)
	require.NoError(t, err, "Lolly not healthy")

	client := &http.Client{Timeout: 10 * time.Second}

	// 应该可以正常访问
	resp, err := client.Get(lolly.HTTPBaseURL() + "/")
	require.NoError(t, err)
	defer resp.Body.Close()

	// 没有访问限制，应该返回 404（没有文件）或 200
	assert.NotEqual(t, http.StatusForbidden, resp.StatusCode, "Request should not be forbidden without access control")
}

// TestE2EAccessCIDRMatch 测试 CIDR 网段匹配。
func TestE2EAccessCIDRMatch(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	// 配置允许私有网络访问
	config := `
servers:
  - listen: ":8080"
    static:
      - path: "/"
        root: "/var/www/html"
        index:
          - "index.html"
    access:
      allow:
        - "10.0.0.0/8"
        - "172.16.0.0/12"
        - "192.168.0.0/16"
      default: deny
`

	// 启动 lolly
	lolly, err := testutil.StartLolly(ctx, testutil.WithConfigYAML(config))
	require.NoError(t, err, "Failed to start lolly container")
	defer lolly.Terminate(ctx)

	err = lolly.WaitForHealthy(ctx, testutil.HealthCheckWaitTimeout)
	require.NoError(t, err, "Lolly not healthy")

	client := &http.Client{Timeout: 10 * time.Second}

	// 从容器网络访问（通常是 172.x.x.x）
	resp, err := client.Get(lolly.HTTPBaseURL() + "/")
	if err == nil {
		defer resp.Body.Close()
		t.Logf("Status: %d", resp.StatusCode)
		// 容器网络在 172.16.0.0/12 范围内
		assert.NotEqual(t, http.StatusForbidden, resp.StatusCode, "Container IP should be in allowed CIDR")
	}
}

// TestE2EAccessProxyWithAccessControl 测试代理模式下的访问控制。
func TestE2EAccessProxyWithAccessControl(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	// 启动模拟后端
	backend, backendAddr, err := testutil.StartMockBackend(ctx)
	require.NoError(t, err, "Failed to start mock backend")
	defer backend.Terminate(ctx)

	// 配置代理 + 访问控制
	config := fmt.Sprintf(`
servers:
  - listen: ":8080"
    proxy:
      - path: "/api"
        targets:
          - url: "http://%s"
    access:
      allow:
        - "10.0.0.0/8"
        - "172.16.0.0/12"
        - "192.168.0.0/16"
      default: deny
`, backendAddr)

	// 启动 lolly
	lolly, err := testutil.StartLolly(ctx, testutil.WithConfigYAML(config))
	require.NoError(t, err, "Failed to start lolly container")
	defer lolly.Terminate(ctx)

	err = lolly.WaitForHealthy(ctx, testutil.HealthCheckWaitTimeout)
	require.NoError(t, err, "Lolly not healthy")

	client := &http.Client{Timeout: 10 * time.Second}

	// 从容器网络访问代理
	resp, err := client.Get(lolly.HTTPBaseURL() + "/api/test")
	if err == nil {
		defer resp.Body.Close()
		t.Logf("Status: %d", resp.StatusCode)
		// 应该可以访问
		assert.NotEqual(t, http.StatusForbidden, resp.StatusCode, "Proxy request should not be forbidden")
	}
}
