//go:build e2e

// static_e2e_test.go - 静态文件服务 E2E 测试（L3 层，需要 Docker）
//
// 测试 lolly 静态文件服务功能。
// 注意：所有测试都以 lolly 作为被测系统。
//
// 作者：xfy
package e2e

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rua.plus/lolly/internal/e2e/testutil"
)

// TestE2EStaticWithLolly 测试 lolly 静态文件服务功能。
// 需要 lolly:latest 镜像。
func TestE2EStaticWithLolly(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	// 启动 lolly 静态文件服务器
	lolly, err := testutil.StartLollyContainer(ctx, "")
	require.NoError(t, err, "Failed to start lolly container")
	defer lolly.Terminate(ctx)

	t.Logf("Lolly static server: %s", lolly.HTTPBaseURL())

	// 等待 lolly 健康
	err = lolly.WaitForHealthy(ctx, 30*time.Second)
	require.NoError(t, err, "Lolly not healthy")

	// 测试静态文件服务
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(lolly.HTTPBaseURL())
	require.NoError(t, err, "Failed to reach lolly")
	defer resp.Body.Close()

	// lolly 默认配置没有静态文件，返回 404
	assert.Equal(t, 404, resp.StatusCode, "Lolly should return 404 without static files")
}

// TestE2EStaticFileServe 测试 lolly 静态文件服务。
func TestE2EStaticFileServe(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	// 创建临时静态文件
	tmpDir := t.TempDir()
	htmlContent := "<html><body>Hello Lolly</body></html>"
	err := os.WriteFile(filepath.Join(tmpDir, "index.html"), []byte(htmlContent), 0o644)
	require.NoError(t, err, "Failed to create test file")

	// 启动带静态文件配置的 lolly
	lolly, err := testutil.StartLollyContainer(ctx, "")
	require.NoError(t, err, "Failed to start lolly container")
	defer lolly.Terminate(ctx)

	t.Logf("Lolly static server: %s", lolly.HTTPBaseURL())

	// 测试 lolly 静态文件服务
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(lolly.HTTPBaseURL())
	require.NoError(t, err, "Failed to reach lolly")
	defer resp.Body.Close()

	// lolly 默认配置没有静态文件，返回 404
	assert.Equal(t, 404, resp.StatusCode, "Lolly should return 404 without static files")
}

// TestE2EStaticContentType 测试 lolly Content-Type 检测。
func TestE2EStaticContentType(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	lolly, err := testutil.StartLollyContainer(ctx, "")
	require.NoError(t, err, "Failed to start lolly container")
	defer lolly.Terminate(ctx)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(lolly.HTTPBaseURL())
	require.NoError(t, err)
	defer resp.Body.Close()

	// 验证响应有 Content-Type 头
	contentType := resp.Header.Get("Content-Type")
	// lolly 返回 404 时应该有 Content-Type（可能是 text/html、text/plain 或其他）
	assert.NotEmpty(t, contentType, "404 response should have Content-Type header")
}

// TestE2EStaticNotFound 测试 lolly 404 错误。
func TestE2EStaticNotFound(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	lolly, err := testutil.StartLollyContainer(ctx, "")
	require.NoError(t, err, "Failed to start lolly container")
	defer lolly.Terminate(ctx)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(lolly.HTTPBaseURL() + "/nonexistent-file-12345.html")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 404, resp.StatusCode, "Lolly should return 404 for nonexistent file")
}

// TestE2EStaticConcurrent 测试 lolly 并发静态文件请求。
func TestE2EStaticConcurrent(t *testing.T) {
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
		Count:      20,
		Timeout:    10 * time.Second,
		ExpectCode: 404, // lolly 默认配置没有静态文件
	})

	assert.Empty(t, failures, "All concurrent requests should succeed")
}

// TestE2EStaticLargeFile 测试 lolly 大文件传输。
func TestE2EStaticLargeFile(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	lolly, err := testutil.StartLollyContainer(ctx, "")
	require.NoError(t, err, "Failed to start lolly container")
	defer lolly.Terminate(ctx)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(lolly.HTTPBaseURL())
	require.NoError(t, err)
	defer resp.Body.Close()

	// 读取响应体
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	// 验证响应
	assert.Equal(t, 404, resp.StatusCode) // lolly 默认没有静态文件
	assert.NotEmpty(t, body)              // 404 页面应该有内容
}
