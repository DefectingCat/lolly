//go:build e2e

// rewrite_e2e_test.go - URL 重写 E2E 测试（L3 层，需要 Docker）
//
// 测试 lolly URL 重写功能，包括：
//   - URL 重写
//   - 正则表达式重写
//   - 重定向 (301/302)
//   - 内部重定向
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

// TestE2ERewriteBasic 测试基本 URL 重写。
func TestE2ERewriteBasic(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	// 配置 URL 重写：/old/* -> /new/*
	config := `
servers:
  - listen: ":8080"
    static:
      - path: "/"
        root: "/var/www/html"
    rewrite:
      - pattern: "^/old/(.*)$"
        replacement: "/new/$1"
        flag: "last"
`

	// 启动 lolly
	lolly, err := testutil.StartLolly(ctx, testutil.WithConfigYAML(config))
	require.NoError(t, err, "Failed to start lolly container")
	defer lolly.Terminate(ctx)

	err = lolly.WaitForHealthy(ctx, testutil.HealthCheckWaitTimeout)
	require.NoError(t, err, "Lolly not healthy")

	client := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse // 不自动跟随重定向
	}}

	// 请求 /old/test 应该被重写到 /new/test
	resp, err := client.Get(lolly.HTTPBaseURL() + "/old/test")
	require.NoError(t, err)
	defer resp.Body.Close()

	t.Logf("Status: %d", resp.StatusCode)
	// 重写后可能返回 404（文件不存在），但不应该返回重定向
	assert.NotEqual(t, http.StatusMovedPermanently, resp.StatusCode)
	assert.NotEqual(t, http.StatusFound, resp.StatusCode)
}

// TestE2ERewriteRedirect 测试重写为重定向。
func TestE2ERewriteRedirect(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	// 配置重写为 302 重定向
	config := `
servers:
  - listen: ":8080"
    static:
      - path: "/"
        root: "/var/www/html"
    rewrite:
      - pattern: "^/old/(.*)$"
        replacement: "/new/$1"
        flag: "redirect"
`

	// 启动 lolly
	lolly, err := testutil.StartLolly(ctx, testutil.WithConfigYAML(config))
	require.NoError(t, err, "Failed to start lolly container")
	defer lolly.Terminate(ctx)

	err = lolly.WaitForHealthy(ctx, testutil.HealthCheckWaitTimeout)
	require.NoError(t, err, "Lolly not healthy")

	client := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}

	// 请求 /old/test 应该返回 302 重定向
	resp, err := client.Get(lolly.HTTPBaseURL() + "/old/test")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusFound, resp.StatusCode, "Should return 302 redirect")

	location := resp.Header.Get("Location")
	assert.Contains(t, location, "/new/test", "Location should contain /new/test")
	t.Logf("Location: %s", location)
}

// TestE2ERewritePermanent 测试永久重定向。
func TestE2ERewritePermanent(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	// 配置永久重定向
	config := `
servers:
  - listen: ":8080"
    static:
      - path: "/"
        root: "/var/www/html"
    rewrite:
      - pattern: "^/old/(.*)$"
        replacement: "/new/$1"
        flag: "permanent"
`

	// 启动 lolly
	lolly, err := testutil.StartLolly(ctx, testutil.WithConfigYAML(config))
	require.NoError(t, err, "Failed to start lolly container")
	defer lolly.Terminate(ctx)

	err = lolly.WaitForHealthy(ctx, testutil.HealthCheckWaitTimeout)
	require.NoError(t, err, "Lolly not healthy")

	client := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}

	// 请求 /old/test 应该返回 301 永久重定向
	resp, err := client.Get(lolly.HTTPBaseURL() + "/old/test")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusMovedPermanently, resp.StatusCode, "Should return 301 permanent redirect")
}

// TestE2ERewriteRegexCapture 测试正则表达式捕获组。
func TestE2ERewriteRegexCapture(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	// 配置复杂的正则重写
	config := `
servers:
  - listen: ":8080"
    static:
      - path: "/"
        root: "/var/www/html"
    rewrite:
      - pattern: "^/user/([0-9]+)/post/([0-9]+)$"
        replacement: "/posts/$1-$2"
        flag: "redirect"
`

	// 启动 lolly
	lolly, err := testutil.StartLolly(ctx, testutil.WithConfigYAML(config))
	require.NoError(t, err, "Failed to start lolly container")
	defer lolly.Terminate(ctx)

	err = lolly.WaitForHealthy(ctx, testutil.HealthCheckWaitTimeout)
	require.NoError(t, err, "Lolly not healthy")

	client := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}

	// 请求 /user/123/post/456 应该重定向到 /posts/123-456
	resp, err := client.Get(lolly.HTTPBaseURL() + "/user/123/post/456")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusFound, resp.StatusCode)

	location := resp.Header.Get("Location")
	assert.Contains(t, location, "/posts/123-456", "Location should contain /posts/123-456")
	t.Logf("Location: %s", location)
}

// TestE2ERewriteProxy 测试代理模式下的重写。
func TestE2ERewriteProxy(t *testing.T) {
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

	// 配置代理 + 重写
	config := fmt.Sprintf(`
servers:
  - listen: ":8080"
    proxy:
      - path: "/api"
        targets:
          - url: "http://%s"
    rewrite:
      - pattern: "^/api/v1/(.*)$"
        replacement: "/api/v2/$1"
        flag: "last"
`, backendAddr)

	// 启动 lolly
	lolly, err := testutil.StartLolly(ctx, testutil.WithConfigYAML(config))
	require.NoError(t, err, "Failed to start lolly container")
	defer lolly.Terminate(ctx)

	err = lolly.WaitForHealthy(ctx, testutil.HealthCheckWaitTimeout)
	require.NoError(t, err, "Lolly not healthy")

	client := &http.Client{Timeout: 10 * time.Second}

	// 请求 /api/v1/users 应该被重写到 /api/v2/users
	resp, err := client.Get(lolly.HTTPBaseURL() + "/api/v1/users")
	require.NoError(t, err)
	defer resp.Body.Close()

	t.Logf("Status: %d", resp.StatusCode)
	// 代理请求应该成功
	assert.NotEqual(t, http.StatusForbidden, resp.StatusCode)
}

// TestE2ERewriteNoMatch 测试不匹配时不重写。
func TestE2ERewriteNoMatch(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	// 配置重写规则
	config := `
servers:
  - listen: ":8080"
    static:
      - path: "/"
        root: "/var/www/html"
    rewrite:
      - pattern: "^/old/(.*)$"
        replacement: "/new/$1"
        flag: "redirect"
`

	// 启动 lolly
	lolly, err := testutil.StartLolly(ctx, testutil.WithConfigYAML(config))
	require.NoError(t, err, "Failed to start lolly container")
	defer lolly.Terminate(ctx)

	err = lolly.WaitForHealthy(ctx, testutil.HealthCheckWaitTimeout)
	require.NoError(t, err, "Lolly not healthy")

	client := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}

	// 请求 /other/test 不匹配规则，不应该重定向
	resp, err := client.Get(lolly.HTTPBaseURL() + "/other/test")
	require.NoError(t, err)
	defer resp.Body.Close()

	// 不应该返回重定向
	assert.NotEqual(t, http.StatusFound, resp.StatusCode)
	assert.NotEqual(t, http.StatusMovedPermanently, resp.StatusCode)
}
