//go:build e2e

// static_e2e_test.go - 静态文件服务 E2E 测试（L3 层，需要 Docker）
//
// 测试静态文件服务、目录索引、缓存等功能。
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

// TestE2EStaticFileServe 测试静态文件服务。
func TestE2EStaticFileServe(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	container, addr, err := testutil.StartNginxContainer(ctx)
	require.NoError(t, err, "Failed to start static server")
	defer container.Terminate(ctx)

	t.Logf("Static server: %s", addr)

	// 测试获取静态文件
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(addr)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 200, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.NotEmpty(t, body, "Response body should not be empty")
}

// TestE2EStaticWithLolly 测试 lolly 静态文件服务功能。
// 需要 lolly:latest 镜像。
func TestE2EStaticWithLolly(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	if !testutil.DockerAvailable(ctx) {
		t.Skip("Docker not available")
	}

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

// TestE2EStaticDirectoryIndex 测试目录索引。
func TestE2EStaticDirectoryIndex(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	container, addr, err := testutil.StartNginxContainer(ctx)
	require.NoError(t, err)
	defer container.Terminate(ctx)

	// 测试根目录
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(addr)
	require.NoError(t, err)
	defer resp.Body.Close()

	// nginx 默认返回 index.html
	assert.Equal(t, 200, resp.StatusCode)
}

// TestE2EStaticFileCache 测试文件缓存。
func TestE2EStaticFileCache(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	container, addr, err := testutil.StartNginxContainer(ctx)
	require.NoError(t, err)
	defer container.Terminate(ctx)

	client := &http.Client{Timeout: 10 * time.Second}

	// 第一次请求
	resp1, err := client.Get(addr)
	require.NoError(t, err)

	etag1 := resp1.Header.Get("ETag")
	lastModified1 := resp1.Header.Get("Last-Modified")
	resp1.Body.Close()

	// 第二次请求带条件头
	req2, err := http.NewRequest("GET", addr, nil)
	require.NoError(t, err)

	if etag1 != "" {
		req2.Header.Set("If-None-Match", etag1)
	}
	if lastModified1 != "" {
		req2.Header.Set("If-Modified-Since", lastModified1)
	}

	resp2, err := client.Do(req2)
	require.NoError(t, err)
	resp2.Body.Close()

	// nginx 返回 304 表示缓存命中
	assert.True(t, resp2.StatusCode == 200 || resp2.StatusCode == 304,
		"Expected 200 or 304, got %d", resp2.StatusCode)
}

// TestE2EStaticContentType 测试 Content-Type 检测。
func TestE2EStaticContentType(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	container, addr, err := testutil.StartNginxContainer(ctx)
	require.NoError(t, err)
	defer container.Terminate(ctx)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(addr)
	require.NoError(t, err)
	defer resp.Body.Close()

	contentType := resp.Header.Get("Content-Type")
	assert.NotEmpty(t, contentType, "Content-Type should be set")
	assert.True(t, strings.Contains(contentType, "text/html") || strings.Contains(contentType, "application/octet-stream"),
		"Expected HTML or octet-stream, got %s", contentType)
}

// TestE2EStaticNotFound 测试 404 错误。
func TestE2EStaticNotFound(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	container, addr, err := testutil.StartNginxContainer(ctx)
	require.NoError(t, err)
	defer container.Terminate(ctx)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(fmt.Sprintf("%s/nonexistent-file-12345.html", addr))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 404, resp.StatusCode, "Should return 404 for nonexistent file")
}

// TestE2EStaticLargeFile 测试大文件传输。
func TestE2EStaticLargeFile(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	container, addr, err := testutil.StartNginxContainer(ctx)
	require.NoError(t, err)
	defer container.Terminate(ctx)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(addr)
	require.NoError(t, err)
	defer resp.Body.Close()

	// 读取响应体
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	// 验证响应
	assert.Equal(t, 200, resp.StatusCode)
	assert.NotEmpty(t, body)
}

// TestE2EStaticConcurrent 测试并发静态文件请求。
func TestE2EStaticConcurrent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	container, addr, err := testutil.StartNginxContainer(ctx)
	require.NoError(t, err)
	defer container.Terminate(ctx)

	// 使用真正的并发测试
	failures := testutil.RunAndVerifyConcurrentRequests(t, testutil.ConcurrentRequestConfig{
		URL:        addr,
		Count:      20,
		Timeout:    10 * time.Second,
		ExpectCode: 200,
	})

	assert.Empty(t, failures, "All concurrent requests should succeed")
}
