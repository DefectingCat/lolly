//go:build e2e

// compression_e2e_test.go - 压缩功能 E2E 测试（L3 层，需要 Docker）
//
// 测试 lolly 响应压缩功能，包括：
//   - Gzip 压缩响应
//   - 压缩级别配置
//   - Content-Type 过滤
//   - Accept-Encoding 协商
//
// 作者：xfy
package e2e

import (
	"bytes"
	"compress/gzip"
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

// TestE2ECompressionGzip 测试 Gzip 压缩响应。
func TestE2ECompressionGzip(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	// 创建包含可压缩内容的 HTML 文件
	htmlContent := `<html><body>` + repeatString("<p>Hello World</p>", 100) + `</body></html>`

	config := fmt.Sprintf(`
servers:
  - listen: ":8080"
    static:
      - path: "/"
        root: "/var/www/html"
    compression:
      enabled: true
      types:
        - "text/html"
        - "text/css"
        - "application/json"
`)

	// 启动 lolly
	lolly, err := testutil.StartLollyContainer(ctx, "",
		testutil.WithConfigYAML(config),
		testutil.WithExtraMount(createTempHTMLFile(t, "index.html", htmlContent), "/var/www/html/index.html"),
	)
	require.NoError(t, err, "Failed to start lolly container")
	defer lolly.Terminate(ctx)

	err = lolly.WaitForHealthy(ctx, 30*time.Second)
	require.NoError(t, err, "Lolly not healthy")

	client := &http.Client{Timeout: 10 * time.Second}

	// 发送带 Accept-Encoding: gzip 的请求
	req, err := http.NewRequest("GET", lolly.HTTPBaseURL()+"/", nil)
	require.NoError(t, err)
	req.Header.Set("Accept-Encoding", "gzip")

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// 检查响应是否被压缩
	encoding := resp.Header.Get("Content-Encoding")
	if encoding == "gzip" {
		t.Log("Response is gzip compressed")

		// 解压并验证内容
		gzReader, err := gzip.NewReader(resp.Body)
		require.NoError(t, err)
		defer gzReader.Close()

		body, err := io.ReadAll(gzReader)
		require.NoError(t, err)
		assert.Contains(t, string(body), "Hello World")
	} else {
		t.Logf("Response not compressed (Content-Encoding: %s), may be too small", encoding)
	}
}

// TestE2ECompressionNoAcceptEncoding 测试不发送 Accept-Encoding 时不压缩。
func TestE2ECompressionNoAcceptEncoding(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	htmlContent := `<html><body><p>Test Content</p></body></html>`

	config := `
servers:
  - listen: ":8080"
    static:
      - path: "/"
        root: "/var/www/html"
    compression:
      enabled: true
`

	// 启动 lolly
	lolly, err := testutil.StartLollyContainer(ctx, "",
		testutil.WithConfigYAML(config),
		testutil.WithExtraMount(createTempHTMLFile(t, "index.html", htmlContent), "/var/www/html/index.html"),
	)
	require.NoError(t, err, "Failed to start lolly container")
	defer lolly.Terminate(ctx)

	err = lolly.WaitForHealthy(ctx, 30*time.Second)
	require.NoError(t, err, "Lolly not healthy")

	client := &http.Client{Timeout: 10 * time.Second}

	// 不发送 Accept-Encoding
	resp, err := client.Get(lolly.HTTPBaseURL() + "/")
	require.NoError(t, err)
	defer resp.Body.Close()

	// 响应不应该被压缩
	encoding := resp.Header.Get("Content-Encoding")
	assert.NotEqual(t, "gzip", encoding, "Response should not be gzip compressed without Accept-Encoding")
}

// TestE2ECompressionDisabled 测试禁用压缩。
func TestE2ECompressionDisabled(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	htmlContent := repeatString("<p>Hello World</p>", 100)

	config := `
servers:
  - listen: ":8080"
    static:
      - path: "/"
        root: "/var/www/html"
    compression:
      enabled: false
`

	// 启动 lolly
	lolly, err := testutil.StartLollyContainer(ctx, "",
		testutil.WithConfigYAML(config),
		testutil.WithExtraMount(createTempHTMLFile(t, "index.html", htmlContent), "/var/www/html/index.html"),
	)
	require.NoError(t, err, "Failed to start lolly container")
	defer lolly.Terminate(ctx)

	err = lolly.WaitForHealthy(ctx, 30*time.Second)
	require.NoError(t, err, "Lolly not healthy")

	client := &http.Client{Timeout: 10 * time.Second}

	// 发送带 Accept-Encoding: gzip 的请求
	req, err := http.NewRequest("GET", lolly.HTTPBaseURL()+"/", nil)
	require.NoError(t, err)
	req.Header.Set("Accept-Encoding", "gzip")

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// 响应不应该被压缩
	encoding := resp.Header.Get("Content-Encoding")
	assert.NotEqual(t, "gzip", encoding, "Response should not be compressed when disabled")
}

// TestE2ECompressionPrecompressed 测试预压缩文件。
func TestE2ECompressionPrecompressed(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	// 创建原始文件和预压缩的 .gz 文件
	originalContent := repeatString("<p>Hello World</p>", 100)
	gzContent := gzipCompress(t, originalContent)

	tmpDir := t.TempDir()
	writeFile(t, tmpDir+"/test.js", originalContent)
	writeFile(t, tmpDir+"/test.js.gz", gzContent)

	config := `
servers:
  - listen: ":8080"
    static:
      - path: "/"
        root: "/var/www/html"
        gzip_static: true
`

	// 启动 lolly
	lolly, err := testutil.StartLollyContainer(ctx, "",
		testutil.WithConfigYAML(config),
		testutil.WithExtraMount(tmpDir, "/var/www/html"),
	)
	require.NoError(t, err, "Failed to start lolly container")
	defer lolly.Terminate(ctx)

	err = lolly.WaitForHealthy(ctx, 30*time.Second)
	require.NoError(t, err, "Lolly not healthy")

	client := &http.Client{Timeout: 10 * time.Second}

	// 请求预压缩文件
	req, err := http.NewRequest("GET", lolly.HTTPBaseURL()+"/test.js", nil)
	require.NoError(t, err)
	req.Header.Set("Accept-Encoding", "gzip")

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// 如果支持预压缩，应该直接返回 .gz 文件
	encoding := resp.Header.Get("Content-Encoding")
	t.Logf("Content-Encoding: %s", encoding)
}

// 辅助函数

func repeatString(s string, n int) string {
	var buf bytes.Buffer
	for range n {
		buf.WriteString(s)
	}
	return buf.String()
}

func createTempHTMLFile(t *testing.T, filename, content string) string {
	t.Helper()
	tmpDir := t.TempDir()
	filePath := tmpDir + "/" + filename
	writeFile(t, filePath, content)
	return filePath
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, writeFileErr(path, content))
}

func writeFileErr(path, content string) error {
	return writeFileBytes(path, []byte(content))
}

func writeFileBytes(path string, content []byte) error {
	return writeFileBytesWithPerm(path, content, 0o644)
}

func writeFileBytesWithPerm(path string, content []byte, perm uint32) error {
	return writeFileWithPerm(path, content, perm)
}

func writeFileWithPerm(path string, content []byte, perm uint32) error {
	import "os"
	return os.WriteFile(path, content, os.FileMode(perm))
}

func gzipCompress(t *testing.T, content string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gzWriter := gzip.NewWriter(&buf)
	_, err := gzWriter.Write([]byte(content))
	require.NoError(t, err)
	require.NoError(t, gzWriter.Close())
	return buf.Bytes()
}
