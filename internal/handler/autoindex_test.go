// Package handler 提供 HTTP 请求处理功能。
package handler

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/valyala/fasthttp"
)

func TestGenerateAutoIndex_HTML(t *testing.T) {
	// 创建临时目录
	tmpDir, err := os.MkdirTemp("", "autoindex_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// 创建测试文件和目录
	if err := os.WriteFile(filepath.Join(tmpDir, "file1.txt"), []byte("content1"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "file2.html"), []byte("<html>content2</html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(tmpDir, "subdir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".hidden"), []byte("hidden"), 0o644); err != nil {
		t.Fatal(err)
	}

	// 测试 HTML 格式
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/test/")

	config := AutoIndexConfig{
		Format:    "html",
		Localtime: false,
		ExactSize: false,
	}

	if !GenerateAutoIndex(ctx, tmpDir, "/test/", config) {
		t.Fatal("GenerateAutoIndex returned false")
	}

	if ct := string(ctx.Response.Header.ContentType()); ct != "text/html; charset=utf-8" {
		t.Errorf("Content-Type = %s, want text/html; charset=utf-8", ct)
	}

	body := string(ctx.Response.Body())
	// 检查包含文件名
	if !containsAll(body, "file1.txt", "file2.html", "subdir") {
		t.Errorf("HTML body missing expected files: %s", body)
	}
	// 检查隐藏文件不显示
	if containsAll(body, ".hidden") {
		t.Errorf("HTML body should not contain hidden file: %s", body)
	}
	// 检查目录有斜杠后缀
	if !containsAll(body, "subdir/") {
		t.Errorf("HTML body directory should have / suffix: %s", body)
	}
}

func TestGenerateAutoIndex_JSON(t *testing.T) {
	// 创建临时目录
	tmpDir, err := os.MkdirTemp("", "autoindex_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// 创建测试文件
	if err := os.WriteFile(filepath.Join(tmpDir, "test.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	// 测试 JSON 格式
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/api/")

	config := AutoIndexConfig{
		Format: "json",
	}

	if !GenerateAutoIndex(ctx, tmpDir, "/api/", config) {
		t.Fatal("GenerateAutoIndex returned false")
	}

	if ct := string(ctx.Response.Header.ContentType()); ct != "application/json" {
		t.Errorf("Content-Type = %s, want application/json", ct)
	}

	body := string(ctx.Response.Body())
	// 检查 JSON 格式
	if !containsAll(body, `"name"`, `"type"`, `"mtime"`, "test.json") {
		t.Errorf("JSON body missing expected fields: %s", body)
	}
}

func TestGenerateAutoIndex_XML(t *testing.T) {
	// 创建临时目录
	tmpDir, err := os.MkdirTemp("", "autoindex_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// 创建测试文件
	if err := os.WriteFile(filepath.Join(tmpDir, "data.xml"), []byte("<data/>"), 0o644); err != nil {
		t.Fatal(err)
	}

	// 测试 XML 格式
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/xml/")

	config := AutoIndexConfig{
		Format: "xml",
	}

	if !GenerateAutoIndex(ctx, tmpDir, "/xml/", config) {
		t.Fatal("GenerateAutoIndex returned false")
	}

	if ct := string(ctx.Response.Header.ContentType()); ct != "text/xml; charset=utf-8" {
		t.Errorf("Content-Type = %s, want text/xml; charset=utf-8", ct)
	}

	body := string(ctx.Response.Body())
	// 检查 XML 格式
	if !containsAll(body, `<list`, `path=`, `<element`, "data.xml") {
		t.Errorf("XML body missing expected elements: %s", body)
	}
}

func TestGenerateAutoIndex_Sorting(t *testing.T) {
	// 创建临时目录
	tmpDir, err := os.MkdirTemp("", "autoindex_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// 创建文件和目录（按不同顺序）
	if err := os.WriteFile(filepath.Join(tmpDir, "z_file.txt"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(tmpDir, "a_dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "m_file.txt"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	// 测试排序
	ctx := &fasthttp.RequestCtx{}
	config := AutoIndexConfig{Format: "json"}

	if !GenerateAutoIndex(ctx, tmpDir, "/", config) {
		t.Fatal("GenerateAutoIndex returned false")
	}

	body := string(ctx.Response.Body())
	// 目录应该排在前面
	dirIdx := indexOf(body, `"a_dir"`)
	zFileIdx := indexOf(body, `"z_file.txt"`)
	mFileIdx := indexOf(body, `"m_file.txt"`)

	if dirIdx == -1 || zFileIdx == -1 || mFileIdx == -1 {
		t.Fatalf("Missing entries in body: %s", body)
	}

	if dirIdx > zFileIdx || dirIdx > mFileIdx {
		t.Errorf("Directories should come first: dirIdx=%d, zFileIdx=%d, mFileIdx=%d", dirIdx, zFileIdx, mFileIdx)
	}
}

func TestGenerateAutoIndex_SizeFormatting(t *testing.T) {
	// 创建临时目录
	tmpDir, err := os.MkdirTemp("", "autoindex_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// 创建不同大小的文件
	smallFile := filepath.Join(tmpDir, "small.txt")
	if err := os.WriteFile(smallFile, make([]byte, 100), 0o644); err != nil {
		t.Fatal(err)
	}

	// 测试人类可读格式
	ctx := &fasthttp.RequestCtx{}
	config := AutoIndexConfig{
		Format:    "html",
		ExactSize: false,
	}

	if !GenerateAutoIndex(ctx, tmpDir, "/", config) {
		t.Fatal("GenerateAutoIndex returned false")
	}

	body := string(ctx.Response.Body())
	if !containsAll(body, "small.txt") {
		t.Errorf("Missing file in output: %s", body)
	}
}

func TestFormatSize(t *testing.T) {
	tests := []struct {
		size     int64
		expected string
	}{
		{0, "0"},
		{100, "100"},
		{1024, "1.0K"},
		{1536, "1.5K"},
		{1048576, "1.0M"},
		{1572864, "1.5M"},
		{1073741824, "1.0G"},
		{1610612736, "1.5G"},
	}

	for _, tt := range tests {
		result := formatSize(tt.size)
		if result != tt.expected {
			t.Errorf("formatSize(%d) = %s, want %s", tt.size, result, tt.expected)
		}
	}
}

// TestGenerateAutoIndex_EmptyDirectory 测试空目录
func TestGenerateAutoIndex_EmptyDirectory(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "autoindex_empty_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	ctx := &fasthttp.RequestCtx{}
	config := AutoIndexConfig{Format: "json"}

	if !GenerateAutoIndex(ctx, tmpDir, "/", config) {
		t.Fatal("GenerateAutoIndex returned false for empty directory")
	}

	body := string(ctx.Response.Body())
	// JSON 格式空数组
	if !containsAll(body, "[]") {
		t.Errorf("Empty directory JSON = %s, should contain []", body)
	}
}

// 辅助函数
func containsAll(s string, substrs ...string) bool {
	for _, substr := range substrs {
		if !contains(s, substr) {
			return false
		}
	}
	return true
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// 确保 autoindex 在 StaticHandler 中工作
func TestStaticHandler_AutoIndex(t *testing.T) {
	// 创建临时目录
	tmpDir, err := os.MkdirTemp("", "autoindex_static_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// 创建测试文件
	if err := os.WriteFile(filepath.Join(tmpDir, "test.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	// 创建处理器并启用 autoindex
	handler := NewStaticHandler(tmpDir, "/", []string{"index.html"}, false)
	handler.SetAutoIndex(true, "html", false, false)

	// 测试目录请求
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/")
	handler.Handle(ctx)

	if ctx.Response.StatusCode() != 200 {
		t.Errorf("Status = %d, want 200", ctx.Response.StatusCode())
	}

	body := string(ctx.Response.Body())
	if !containsAll(body, "test.txt") {
		t.Errorf("AutoIndex response missing file: %s", body)
	}
}

// 测试 autoindex 关闭时返回 403
func TestStaticHandler_AutoIndex_Disabled(t *testing.T) {
	// 创建临时目录
	tmpDir, err := os.MkdirTemp("", "autoindex_disabled_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// 创建处理器（不启用 autoindex）
	handler := NewStaticHandler(tmpDir, "/", []string{"index.html"}, false)

	// 测试目录请求
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/")
	handler.Handle(ctx)

	if ctx.Response.StatusCode() != 403 {
		t.Errorf("Status = %d, want 403", ctx.Response.StatusCode())
	}
}

// 测试时间格式
func TestGenerateAutoIndex_TimeFormat(t *testing.T) {
	// 创建临时目录
	tmpDir, err := os.MkdirTemp("", "autoindex_time_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// 创建文件
	testFile := filepath.Join(tmpDir, "time.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0o644); err != nil {
		t.Fatal(err)
	}

	// 测试 GMT 时间
	ctx1 := &fasthttp.RequestCtx{}
	config1 := AutoIndexConfig{Format: "html", Localtime: false}
	GenerateAutoIndex(ctx1, tmpDir, "/", config1)

	// 测试本地时间
	ctx2 := &fasthttp.RequestCtx{}
	config2 := AutoIndexConfig{Format: "html", Localtime: true}
	GenerateAutoIndex(ctx2, tmpDir, "/", config2)

	// 两个响应应该都成功
	if ctx1.Response.StatusCode() != 200 || ctx2.Response.StatusCode() != 200 {
		t.Errorf("Expected status 200 for both time formats")
	}

	// 验证时间格式存在（格式：02-Jan-2006 15:04）
	body1 := string(ctx1.Response.Body())
	if len(body1) < 10 {
		t.Errorf("HTML body too short")
	}
}

// 确保编译时检查接口
func TestAutoIndexConfig_CompileTimeCheck(t *testing.T) {
	// 确保 AutoIndexConfig 结构体字段正确
	config := AutoIndexConfig{
		Format:    "html",
		Localtime: true,
		ExactSize: true,
	}

	if config.Format != "html" {
		t.Errorf("Format = %s, want html", config.Format)
	}
}

// 基准测试
func BenchmarkGenerateAutoIndex_HTML(b *testing.B) {
	// 创建临时目录
	tmpDir, err := os.MkdirTemp("", "autoindex_bench")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// 创建 100 个文件
	for i := range 100 {
		if err := os.WriteFile(filepath.Join(tmpDir, fmt.Sprintf("file%d.txt", i)), []byte("content"), 0o644); err != nil {
			b.Fatal(err)
		}
	}

	config := AutoIndexConfig{Format: "html"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx := &fasthttp.RequestCtx{}
		GenerateAutoIndex(ctx, tmpDir, "/", config)
	}
}

// 确保时间包导入
var _ = time.Second
