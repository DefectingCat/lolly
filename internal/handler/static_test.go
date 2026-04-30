// Package handler 提供静态文件处理器功能的测试。
//
// 该文件测试静态文件处理器模块的各项功能，包括：
//   - 正常文件访问
//   - 嵌套路径文件
//   - 目录索引文件
//   - 路径遍历安全检查
//   - 索引文件优先级
//   - 构造函数
//   - 文件缓存设置
//   - Gzip 静态文件设置
//   - HEAD 请求处理
//   - 预压缩文件支持
//   - 大文件处理
//   - 符号链接处理
//
// 作者：xfy
package handler

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/cache"
	"rua.plus/lolly/internal/testutil"
)

// newTestHandler 创建测试用的静态文件处理器。
//
// 参数：
//   - t: 测试上下文，用于标记 helper
//   - root: 静态文件根目录
//
// 返回值：
//   - *StaticHandler: 配置好索引文件的静态文件处理器（禁用 sendfile）
func newTestHandler(t *testing.T, root string) *StaticHandler {
	t.Helper()
	return NewStaticHandler(root, "/", []string{"index.html", "index.htm"}, false) // 测试时禁用 sendfile
}

// newTestContext 创建测试用的 fasthttp 请求上下文。
//
// 参数：
//   - t: 测试上下文，用于标记 helper
//   - path: 请求路径
//
// 返回值：
//   - *fasthttp.RequestCtx: 初始化好的请求上下文
func newTestContext(t *testing.T, path string) *fasthttp.RequestCtx {
	t.Helper()
	return testutil.NewRequestCtx("GET", path)
}

// TestStaticHandlerHandle 测试静态文件处理器
func TestStaticHandlerHandle(t *testing.T) {
	tests := []struct {
		setup       func(t *testing.T, root string)
		name        string
		path        string
		wantContent string
		wantStatus  int
		skipContent bool
	}{
		{
			name: "正常文件访问",
			setup: func(t *testing.T, root string) {
				t.Helper()
				content := "hello world"
				if err := os.WriteFile(filepath.Join(root, "test.txt"), []byte(content), 0o644); err != nil {
					t.Fatalf("创建测试文件失败: %v", err)
				}
			},
			path:        "/test.txt",
			wantStatus:  fasthttp.StatusOK,
			wantContent: "hello world",
		},
		{
			name: "嵌套路径文件",
			setup: func(t *testing.T, root string) {
				t.Helper()
				subDir := filepath.Join(root, "sub", "dir")
				if err := os.MkdirAll(subDir, 0o755); err != nil {
					t.Fatalf("创建子目录失败: %v", err)
				}
				content := "nested file content"
				if err := os.WriteFile(filepath.Join(subDir, "nested.txt"), []byte(content), 0o644); err != nil {
					t.Fatalf("创建嵌套文件失败: %v", err)
				}
			},
			path:        "/sub/dir/nested.txt",
			wantStatus:  fasthttp.StatusOK,
			wantContent: "nested file content",
		},
		{
			name: "目录带索引文件",
			setup: func(t *testing.T, root string) {
				t.Helper()
				dir := filepath.Join(root, "withindex")
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatalf("创建目录失败: %v", err)
				}
				content := "index page"
				if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte(content), 0o644); err != nil {
					t.Fatalf("创建索引文件失败: %v", err)
				}
			},
			path:        "/withindex/",
			wantStatus:  fasthttp.StatusOK,
			wantContent: "index page",
		},
		{
			name: "目录无索引文件",
			setup: func(t *testing.T, root string) {
				t.Helper()
				dir := filepath.Join(root, "noindex")
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatalf("创建目录失败: %v", err)
				}
			},
			path:        "/noindex/",
			wantStatus:  fasthttp.StatusForbidden,
			skipContent: true,
		},
		{
			name: "文件不存在",
			setup: func(_ *testing.T, _ string) {
				t.Helper()
				// 不创建任何文件
			},
			path:        "/nonexistent.txt",
			wantStatus:  fasthttp.StatusNotFound,
			skipContent: true,
		},
		{
			name: "空路径访问根目录无索引",
			setup: func(_ *testing.T, _ string) {
				t.Helper()
				// root 目录没有索引文件
			},
			path:        "/",
			wantStatus:  fasthttp.StatusForbidden,
			skipContent: true,
		},
		{
			name: "根目录有索引文件",
			setup: func(t *testing.T, root string) {
				t.Helper()
				content := "root index"
				if err := os.WriteFile(filepath.Join(root, "index.html"), []byte(content), 0o644); err != nil {
					t.Fatalf("创建根索引文件失败: %v", err)
				}
			},
			path:        "/",
			wantStatus:  fasthttp.StatusOK,
			wantContent: "root index",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 创建临时目录
			tmpDir := t.TempDir()

			// 设置测试文件
			tt.setup(t, tmpDir)

			// 创建处理器和上下文
			handler := newTestHandler(t, tmpDir)
			ctx := newTestContext(t, tt.path)

			// 执行请求
			handler.Handle(ctx)

			// 验证状态码
			if got := ctx.Response.StatusCode(); got != tt.wantStatus {
				t.Errorf("Handle() 状态码 = %d, want %d", got, tt.wantStatus)
			}

			// 验证内容（如果需要）
			if !tt.skipContent && tt.wantContent != "" {
				got := string(ctx.Response.Body())
				if got != tt.wantContent {
					t.Errorf("Handle() 内容 = %q, want %q", got, tt.wantContent)
				}
			}
		})
	}
}

// TestStaticHandlerHandle_PathTraversalSecurity 测试路径遍历安全检查
// 注意：fasthttp 会自动规范化路径，移除 ../ 组件
// 安全检查 strings.Contains(path, "..") 检测文件名中包含 ".." 的情况
func TestStaticHandlerHandle_PathTraversalSecurity(t *testing.T) {
	tests := []struct {
		setup       func(t *testing.T, root string)
		name        string
		path        string
		description string
		wantStatus  int
	}{
		{
			name: "文件名包含双点 - 安全检查拦截",
			setup: func(_ *testing.T, _ string) {
				t.Helper()
				// 不创建任何文件
			},
			path:        "/file..txt",
			wantStatus:  fasthttp.StatusForbidden,
			description: "路径包含 '..' 字符串，触发安全检查返回 403",
		},
		{
			name: "路径末尾双点 - 安全检查拦截",
			setup: func(_ *testing.T, _ string) {
				t.Helper()
			},
			path:        "/foo/..",
			wantStatus:  fasthttp.StatusForbidden,
			description: "路径末尾包含 '..'，触发安全检查返回 403",
		},
		{
			name: "隐藏文件 .hidden - 文件不存在",
			setup: func(_ *testing.T, _ string) {
				t.Helper()
			},
			path:        "/.hidden",
			wantStatus:  fasthttp.StatusNotFound,
			description: "单点开头的隐藏文件不触发安全检查，文件不存在返回 404",
		},
		{
			name: "文件名包含多点 ...txt - 安全检查拦截",
			setup: func(_ *testing.T, _ string) {
				t.Helper()
			},
			path:        "/file...txt",
			wantStatus:  fasthttp.StatusForbidden,
			description: "包含连续多点（含 '..'）触发安全检查返回 403",
		},
		{
			name: "fasthttp 规范化后的路径 - 文件不存在",
			setup: func(_ *testing.T, _ string) {
				t.Helper()
				// fasthttp 将 /../secret.txt 规范化为 /secret.txt
			},
			path:        "/../secret.txt",
			wantStatus:  fasthttp.StatusNotFound,
			description: "fasthttp 自动规范化路径移除 ../，结果路径文件不存在返回 404",
		},
		{
			name: "URL 编码路径遍历 - fasthttp 规范化",
			setup: func(_ *testing.T, _ string) {
				t.Helper()
				// fasthttp 解码 %2e%2e 为 .. 并规范化路径
			},
			path:        "/%2e%2e/secret.txt",
			wantStatus:  fasthttp.StatusNotFound,
			description: "fasthttp 解码 URL 编码后规范化路径，文件不存在返回 404",
		},
		{
			name: "混合 URL 编码 - fasthttp 规范化",
			setup: func(_ *testing.T, _ string) {
				t.Helper()
			},
			path:        "/%2e%2e%2fsecret.txt",
			wantStatus:  fasthttp.StatusNotFound,
			description: "fasthttp 解码并规范化路径，文件不存在返回 404",
		},
		{
			name: "路径中含 ../ - fasthttp 规范化",
			setup: func(t *testing.T, root string) {
				t.Helper()
				// 创建目标文件供测试
				if err := os.WriteFile(filepath.Join(root, "bar.txt"), []byte("bar"), 0o644); err != nil {
					t.Fatalf("创建文件失败: %v", err)
				}
			},
			path:        "/foo/../bar.txt",
			wantStatus:  fasthttp.StatusOK,
			description: "fasthttp 规范化 /foo/../bar.txt 为 /bar.txt，文件存在返回 200",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			tt.setup(t, tmpDir)

			handler := newTestHandler(t, tmpDir)
			ctx := newTestContext(t, tt.path)

			handler.Handle(ctx)

			if got := ctx.Response.StatusCode(); got != tt.wantStatus {
				t.Errorf("Handle() 状态码 = %d, want %d\n说明: %s", got, tt.wantStatus, tt.description)
			}
		})
	}
}

// TestStaticHandlerHandle_IndexFallback 测试索引文件优先级
func TestStaticHandlerHandle_IndexFallback(t *testing.T) {
	t.Run("优先 index.html", func(t *testing.T) {
		tmpDir := t.TempDir()
		dir := filepath.Join(tmpDir, "testdir")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("创建目录失败: %v", err)
		}

		// 创建两个索引文件
		if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("html content"), 0o644); err != nil {
			t.Fatalf("创建 index.html 失败: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "index.htm"), []byte("htm content"), 0o644); err != nil {
			t.Fatalf("创建 index.htm 失败: %v", err)
		}

		handler := newTestHandler(t, tmpDir)
		ctx := newTestContext(t, "/testdir/")
		handler.Handle(ctx)

		if got := ctx.Response.StatusCode(); got != fasthttp.StatusOK {
			t.Errorf("状态码 = %d, want %d", got, fasthttp.StatusOK)
		}

		// 应返回 index.html 而非 index.htm
		got := string(ctx.Response.Body())
		if got != "html content" {
			t.Errorf("内容 = %q, want %q", got, "html content")
		}
	})

	t.Run("无 index.html 时使用 index.htm", func(t *testing.T) {
		tmpDir := t.TempDir()
		dir := filepath.Join(tmpDir, "testdir")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("创建目录失败: %v", err)
		}

		// 仅创建 index.htm
		if err := os.WriteFile(filepath.Join(dir, "index.htm"), []byte("htm content"), 0o644); err != nil {
			t.Fatalf("创建 index.htm 失败: %v", err)
		}

		handler := newTestHandler(t, tmpDir)
		ctx := newTestContext(t, "/testdir/")
		handler.Handle(ctx)

		if got := ctx.Response.StatusCode(); got != fasthttp.StatusOK {
			t.Errorf("状态码 = %d, want %d", got, fasthttp.StatusOK)
		}

		got := string(ctx.Response.Body())
		if got != "htm content" {
			t.Errorf("内容 = %q, want %q", got, "htm content")
		}
	})

	t.Run("无索引文件时返回 403", func(t *testing.T) {
		tmpDir := t.TempDir()
		dir := filepath.Join(tmpDir, "testdir")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("创建目录失败: %v", err)
		}

		// 创建一个非索引文件
		if err := os.WriteFile(filepath.Join(dir, "other.txt"), []byte("other content"), 0o644); err != nil {
			t.Fatalf("创建 other.txt 失败: %v", err)
		}

		handler := newTestHandler(t, tmpDir)
		ctx := newTestContext(t, "/testdir/")
		handler.Handle(ctx)

		if got := ctx.Response.StatusCode(); got != fasthttp.StatusForbidden {
			t.Errorf("状态码 = %d, want %d", got, fasthttp.StatusForbidden)
		}
	})

	t.Run("目录不带斜杠结尾", func(t *testing.T) {
		tmpDir := t.TempDir()
		dir := filepath.Join(tmpDir, "testdir")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("创建目录失败: %v", err)
		}

		// 创建索引文件
		if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("index"), 0o644); err != nil {
			t.Fatalf("创建 index.html 失败: %v", err)
		}

		handler := newTestHandler(t, tmpDir)
		ctx := newTestContext(t, "/testdir") // 不带斜杠
		handler.Handle(ctx)

		// 目录不带斜杠也应该能访问索引文件
		if got := ctx.Response.StatusCode(); got != fasthttp.StatusOK {
			t.Errorf("状态码 = %d, want %d", got, fasthttp.StatusOK)
		}
	})
}

// TestNewStaticHandler 测试静态文件处理器构造函数
func TestNewStaticHandler(t *testing.T) {
	t.Run("正常创建", func(t *testing.T) {
		root := "/var/www"
		index := []string{"index.html", "index.htm"}
		handler := NewStaticHandler(root, "/", index, true)

		if handler == nil {
			t.Fatal("NewStaticHandler() 返回 nil")
		}
		// root 被规范化为带尾部斜杠的形式
		expectedRoot := "/var/www/"
		if handler.root != expectedRoot {
			t.Errorf("handler.root = %q, want %q", handler.root, expectedRoot)
		}
		if len(handler.index) != len(index) {
			t.Errorf("len(handler.index) = %d, want %d", len(handler.index), len(index))
		}
	})

	t.Run("空索引列表", func(t *testing.T) {
		handler := NewStaticHandler("/var/www", "/", nil, false)
		if handler == nil {
			t.Fatal("NewStaticHandler() 返回 nil")
		}
		if handler.index != nil {
			t.Errorf("handler.index 应为 nil")
		}
	})
}

// TestStaticHandler_SetFileCache 测试设置文件缓存
func TestStaticHandler_SetFileCache(t *testing.T) {
	handler := NewStaticHandler("/var/www", "/", nil, false)

	// 设置 nil 缓存
	handler.SetFileCache(nil)
	if handler.fileCache != nil {
		t.Error("Expected nil fileCache")
	}

	// 设置非 nil 缓存（使用 mock 或简单验证）
	// 由于 FileCache 接口需要实现，这里主要验证不会 panic
}

// TestStaticHandler_SetGzipStatic 测试设置 Gzip 静态文件
func TestStaticHandler_SetGzipStatic(t *testing.T) {
	handler := NewStaticHandler("/var/www", "/", nil, false)

	// 启用 gzip
	handler.SetGzipStatic(true, nil, []string{".gz", ".gzip"})
	if handler.gzipStatic == nil {
		t.Error("Expected gzipStatic to be non-nil")
	}

	// 禁用 gzip
	handler.SetGzipStatic(false, nil, nil)
	// gzipStatic 保持不变（SetGzipStatic 只在 enabled=true 时设置）
}

// TestStaticHandler_Handle_HeadRequest 测试 HEAD 请求
func TestStaticHandler_Handle_HeadRequest(t *testing.T) {
	tmpDir := t.TempDir()
	content := "head request test"
	if err := os.WriteFile(filepath.Join(tmpDir, "test.txt"), []byte(content), 0o644); err != nil {
		t.Fatalf("创建测试文件失败: %v", err)
	}

	handler := newTestHandler(t, tmpDir)

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/test.txt")
	ctx.Request.Header.SetMethod("HEAD")

	handler.Handle(ctx)

	if got := ctx.Response.StatusCode(); got != fasthttp.StatusOK {
		t.Errorf("状态码 = %d, want %d", got, fasthttp.StatusOK)
	}

	// 验证 Content-Type 设置正确
	ct := string(ctx.Response.Header.ContentType())
	if ct == "" {
		t.Error("Expected Content-Type to be set")
	}
}

// TestStaticHandler_Handle_WithCache 测试带缓存的文件处理
func TestStaticHandler_Handle_WithCache(t *testing.T) {
	tmpDir := t.TempDir()
	content := "cached content test"
	if err := os.WriteFile(filepath.Join(tmpDir, "test.txt"), []byte(content), 0o644); err != nil {
		t.Fatalf("创建测试文件失败: %v", err)
	}

	handler := newTestHandler(t, tmpDir)
	// 设置 nil 缓存，验证不会 panic
	handler.SetFileCache(nil)

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/test.txt")

	handler.Handle(ctx)

	if got := ctx.Response.StatusCode(); got != fasthttp.StatusOK {
		t.Errorf("状态码 = %d, want %d", got, fasthttp.StatusOK)
	}
}

// TestStaticHandler_Handle_Precompressed 测试预压缩文件
func TestStaticHandler_Handle_Precompressed(t *testing.T) {
	tmpDir := t.TempDir()

	// 创建原始文件和 gzip 压缩版本
	content := "test content for gzip"
	if err := os.WriteFile(filepath.Join(tmpDir, "test.txt"), []byte(content), 0o644); err != nil {
		t.Fatalf("创建测试文件失败: %v", err)
	}

	// 创建 .gz 文件（模拟预压缩）
	gzContent := []byte("gzipped content")
	if err := os.WriteFile(filepath.Join(tmpDir, "test.txt.gz"), gzContent, 0o644); err != nil {
		t.Fatalf("创建 gzip 文件失败: %v", err)
	}

	handler := NewStaticHandler(tmpDir, "/", nil, false)
	handler.SetGzipStatic(true, nil, []string{".gz"})

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/test.txt")
	ctx.Request.Header.Set("Accept-Encoding", "gzip")

	handler.Handle(ctx)

	// 应返回预压缩内容
	if got := ctx.Response.StatusCode(); got != fasthttp.StatusOK {
		t.Errorf("状态码 = %d, want %d", got, fasthttp.StatusOK)
	}
}

// TestStaticHandler_Handle_LargeFile 测试大文件处理
func TestStaticHandler_Handle_LargeFile(t *testing.T) {
	tmpDir := t.TempDir()

	// 创建一个较大的文件 (> 8KB 触发 sendfile 路径)
	largeContent := make([]byte, 16*1024)
	for i := range largeContent {
		largeContent[i] = byte(i % 256)
	}

	tmpFile := filepath.Join(tmpDir, "large.bin")
	if err := os.WriteFile(tmpFile, largeContent, 0o644); err != nil {
		t.Fatalf("创建大文件失败: %v", err)
	}

	// 使用 sendfile 启用的处理器
	handler := NewStaticHandler(tmpDir, "/", []string{"index.html"}, true)

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/large.bin")

	handler.Handle(ctx)

	if got := ctx.Response.StatusCode(); got != fasthttp.StatusOK {
		t.Errorf("状态码 = %d, want %d", got, fasthttp.StatusOK)
	}
}

// TestStaticHandler_Handle_Symlink 测试符号链接处理
func TestStaticHandler_Handle_Symlink(t *testing.T) {
	tmpDir := t.TempDir()

	// 创建目标文件
	targetContent := "symlink target"
	targetFile := filepath.Join(tmpDir, "target.txt")
	if err := os.WriteFile(targetFile, []byte(targetContent), 0o644); err != nil {
		t.Fatalf("创建目标文件失败: %v", err)
	}

	// 创建符号链接
	linkFile := filepath.Join(tmpDir, "link.txt")
	if err := os.Symlink(targetFile, linkFile); err != nil {
		t.Fatalf("创建符号链接失败: %v", err)
	}

	handler := newTestHandler(t, tmpDir)

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/link.txt")

	handler.Handle(ctx)

	// 符号链接应该能正常访问
	if got := ctx.Response.StatusCode(); got != fasthttp.StatusOK {
		t.Errorf("状态码 = %d, want %d", got, fasthttp.StatusOK)
	}
}

// TestStaticHandler_SetTryFiles 测试 SetTryFiles 配置设置
func TestStaticHandler_SetTryFiles(t *testing.T) {
	tests := []struct {
		name         string
		tryFiles     []string
		wantTryFiles []string
		tryFilesPass bool
		wantPass     bool
	}{
		{
			name:         "基本配置",
			tryFiles:     []string{"$uri", "$uri/", "/index.html"},
			tryFilesPass: false,
			wantTryFiles: []string{"$uri", "$uri/", "/index.html"},
			wantPass:     false,
		},
		{
			name:         "启用 tryFilesPass",
			tryFiles:     []string{"$uri", "/fallback.html"},
			tryFilesPass: true,
			wantTryFiles: []string{"$uri", "/fallback.html"},
			wantPass:     true,
		},
		{
			name:         "空配置",
			tryFiles:     []string{},
			tryFilesPass: false,
			wantTryFiles: []string{},
			wantPass:     false,
		},
		{
			name:         "单一项配置",
			tryFiles:     []string{"/app.html"},
			tryFilesPass: false,
			wantTryFiles: []string{"/app.html"},
			wantPass:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewStaticHandler("/var/www", "/", []string{"index.html"}, false)
			router := NewRouter()

			handler.SetTryFiles(tt.tryFiles, tt.tryFilesPass, router)

			// 验证配置
			if len(handler.tryFiles) != len(tt.wantTryFiles) {
				t.Errorf("tryFiles length = %d, want %d", len(handler.tryFiles), len(tt.wantTryFiles))
			}
			for i, v := range tt.wantTryFiles {
				if handler.tryFiles[i] != v {
					t.Errorf("tryFiles[%d] = %q, want %q", i, handler.tryFiles[i], v)
				}
			}
			if handler.tryFilesPass != tt.wantPass {
				t.Errorf("tryFilesPass = %v, want %v", handler.tryFilesPass, tt.wantPass)
			}
			if handler.router != router {
				t.Error("router 未正确设置")
			}
		})
	}
}

// TestStaticHandler_resolveTryFilePath 测试 resolveTryFilePath 占位符解析
func TestStaticHandler_resolveTryFilePath(t *testing.T) {
	handler := NewStaticHandler("/var/www", "/", []string{"index.html"}, false)

	tests := []struct {
		name       string
		tryFile    string
		relPath    string
		wantResult string
	}{
		{
			name:       "$uri 占位符",
			tryFile:    "$uri",
			relPath:    "/api/user",
			wantResult: "/api/user",
		},
		{
			name:       "$uri/ 占位符",
			tryFile:    "$uri/",
			relPath:    "/api/user",
			wantResult: "/api/user/",
		},
		{
			name:       "绝对路径",
			tryFile:    "/index.html",
			relPath:    "/api/user",
			wantResult: "index.html",
		},
		{
			name:       "普通文件名",
			tryFile:    "fallback.html",
			relPath:    "/api/user",
			wantResult: "fallback.html",
		},
		{
			name:       "根路径 $uri",
			tryFile:    "$uri",
			relPath:    "/",
			wantResult: "/",
		},
		{
			name:       "嵌套路径 $uri",
			tryFile:    "$uri",
			relPath:    "/assets/js/app.js",
			wantResult: "/assets/js/app.js",
		},
		{
			name:       "带查询风格路径",
			tryFile:    "$uri",
			relPath:    "/path/to/file.txt",
			wantResult: "/path/to/file.txt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := handler.resolveTryFilePath(tt.tryFile, tt.relPath)
			if got != tt.wantResult {
				t.Errorf("resolveTryFilePath(%q, %q) = %q, want %q", tt.tryFile, tt.relPath, got, tt.wantResult)
			}
		})
	}
}

// TestStaticHandler_handleTryFiles 测试 handleTryFiles 功能
func TestStaticHandler_handleTryFiles(t *testing.T) {
	tests := []struct {
		setup       func(t *testing.T, root string)
		name        string
		path        string
		wantContent string
		tryFiles    []string
		wantStatus  int
		skipContent bool
	}{
		{
			name: "$uri 找到文件",
			setup: func(t *testing.T, root string) {
				if err := os.WriteFile(filepath.Join(root, "app.js"), []byte("app content"), 0o644); err != nil {
					t.Fatalf("创建文件失败: %v", err)
				}
			},
			tryFiles:    []string{"$uri", "$uri/", "/index.html"},
			path:        "/app.js",
			wantStatus:  fasthttp.StatusOK,
			wantContent: "app content",
		},
		{
			name: "$uri 未找到回退到 $uri/",
			setup: func(t *testing.T, root string) {
				dir := filepath.Join(root, "assets")
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatalf("创建目录失败: %v", err)
				}
				if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("assets index"), 0o644); err != nil {
					t.Fatalf("创建索引文件失败: %v", err)
				}
			},
			tryFiles:    []string{"$uri", "$uri/", "/index.html"},
			path:        "/assets",
			wantStatus:  fasthttp.StatusOK,
			wantContent: "assets index",
		},
		{
			name: "回退到 fallback 文件",
			setup: func(t *testing.T, root string) {
				if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("spa fallback"), 0o644); err != nil {
					t.Fatalf("创建 fallback 文件失败: %v", err)
				}
			},
			tryFiles:    []string{"$uri", "$uri/", "/index.html"},
			path:        "/nonexistent",
			wantStatus:  fasthttp.StatusOK,
			wantContent: "spa fallback",
		},
		{
			name: "所有 try_files 都未找到",
			setup: func(_ *testing.T, _ string) {
				// 不创建任何文件
			},
			tryFiles:    []string{"$uri", "$uri/", "/index.html"},
			path:        "/nonexistent",
			wantStatus:  fasthttp.StatusNotFound,
			skipContent: true,
		},
		{
			name: "嵌套目录回退",
			setup: func(t *testing.T, root string) {
				if err := os.WriteFile(filepath.Join(root, "app.html"), []byte("app shell"), 0o644); err != nil {
					t.Fatalf("创建 fallback 文件失败: %v", err)
				}
			},
			tryFiles:    []string{"$uri", "/app.html"},
			path:        "/user/profile",
			wantStatus:  fasthttp.StatusOK,
			wantContent: "app shell",
		},
		{
			name: "路径前缀剥离",
			setup: func(t *testing.T, root string) {
				apiDir := filepath.Join(root, "api")
				if err := os.MkdirAll(apiDir, 0o755); err != nil {
					t.Fatalf("创建目录失败: %v", err)
				}
				if err := os.WriteFile(filepath.Join(apiDir, "data.json"), []byte("json data"), 0o644); err != nil {
					t.Fatalf("创建文件失败: %v", err)
				}
			},
			tryFiles:    []string{"$uri"},
			path:        "/static/api/data.json",
			wantStatus:  fasthttp.StatusNotFound, // 路径前缀剥离后找不到
			skipContent: true,
		},
		{
			name: "空 try_files 数组",
			setup: func(t *testing.T, root string) {
				if err := os.WriteFile(filepath.Join(root, "test.txt"), []byte("test"), 0o644); err != nil {
					t.Fatalf("创建文件失败: %v", err)
				}
			},
			tryFiles:    []string{},
			path:        "/test.txt",
			wantStatus:  fasthttp.StatusOK, // 空 try_files 走标准处理流程
			wantContent: "test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			tt.setup(t, tmpDir)

			handler := NewStaticHandler(tmpDir, "/", []string{"index.html"}, false)
			handler.SetTryFiles(tt.tryFiles, false, nil)

			ctx := newTestContext(t, tt.path)
			handler.Handle(ctx)

			if got := ctx.Response.StatusCode(); got != tt.wantStatus {
				t.Errorf("状态码 = %d, want %d", got, tt.wantStatus)
			}

			if !tt.skipContent && tt.wantContent != "" {
				got := string(ctx.Response.Body())
				if got != tt.wantContent {
					t.Errorf("内容 = %q, want %q", got, tt.wantContent)
				}
			}
		})
	}
}

// TestStaticHandler_handleInternalRedirect 测试内部重定向功能
func TestStaticHandler_handleInternalRedirect(t *testing.T) {
	tests := []struct {
		setup        func(t *testing.T, root string)
		name         string
		path         string
		wantContent  string
		tryFiles     []string
		wantStatus   int
		tryFilesPass bool
		skipContent  bool
	}{
		{
			name: "tryFilesPass false 直接服务文件",
			setup: func(t *testing.T, root string) {
				if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("index content"), 0o644); err != nil {
					t.Fatalf("创建文件失败: %v", err)
				}
			},
			tryFiles:     []string{"$uri", "/index.html"},
			tryFilesPass: false,
			path:         "/nonexistent",
			wantStatus:   fasthttp.StatusOK,
			wantContent:  "index content",
		},
		{
			name: "tryFilesPass true 触发重定向",
			setup: func(t *testing.T, root string) {
				if err := os.WriteFile(filepath.Join(root, "fallback.txt"), []byte("fallback content"), 0o644); err != nil {
					t.Fatalf("创建文件失败: %v", err)
				}
			},
			tryFiles:     []string{"$uri", "/fallback.txt"},
			tryFilesPass: true,
			path:         "/nonexistent",
			wantStatus:   fasthttp.StatusOK,
			wantContent:  "fallback content",
		},
		{
			name: "内部重定向目标不存在",
			setup: func(_ *testing.T, _ string) {
				// 不创建 fallback 文件
			},
			tryFiles:     []string{"$uri", "/fallback.html"},
			tryFilesPass: false,
			path:         "/nonexistent",
			wantStatus:   fasthttp.StatusNotFound,
			skipContent:  true,
		},
		{
			name: "内部重定向目标是目录",
			setup: func(t *testing.T, root string) {
				dir := filepath.Join(root, "fallback")
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatalf("创建目录失败: %v", err)
				}
				// 在 fallback 目录中创建一个 index.html
				if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("fallback index"), 0o644); err != nil {
					t.Fatalf("创建 index.html 失败: %v", err)
				}
			},
			tryFiles:     []string{"$uri", "$uri/", "/fallback"},
			tryFilesPass: false,
			path:         "/nonexistent",
			wantStatus:   fasthttp.StatusOK,
			wantContent:  "fallback index",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			tt.setup(t, tmpDir)

			handler := NewStaticHandler(tmpDir, "/", []string{"index.html"}, false)
			router := NewRouter()
			handler.SetTryFiles(tt.tryFiles, tt.tryFilesPass, router)

			// 注册路由处理器用于测试 tryFilesPass 重定向
			if tt.tryFilesPass {
				router.GET("/{filepath:*}", func(ctx *fasthttp.RequestCtx) {
					// 通配符路由，可以匹配任何路径
					path := string(ctx.Path())
					// 从 root 读取文件
					filePath := filepath.Join(tmpDir, path[1:]) // 去掉开头的 /
					data, err := os.ReadFile(filePath)
					if err != nil {
						ctx.SetStatusCode(fasthttp.StatusNotFound)
						ctx.SetBodyString("Not Found")
						return
					}
					ctx.SetStatusCode(fasthttp.StatusOK)
					ctx.SetBody(data)
				})
			}

			ctx := newTestContext(t, tt.path)
			handler.Handle(ctx)

			if got := ctx.Response.StatusCode(); got != tt.wantStatus {
				t.Errorf("状态码 = %d, want %d", got, tt.wantStatus)
			}

			if !tt.skipContent && tt.wantContent != "" {
				got := string(ctx.Response.Body())
				if got != tt.wantContent {
					t.Errorf("内容 = %q, want %q", got, tt.wantContent)
				}
			}
		})
	}
}

// TestStaticHandler_TryFilesSPA 测试 SPA 场景下的 try_files
func TestStaticHandler_TryFilesSPA(t *testing.T) {
	tmpDir := t.TempDir()

	// 创建 SPA 文件结构
	// index.html - 主应用入口
	if err := os.WriteFile(filepath.Join(tmpDir, "index.html"), []byte("<!DOCTYPE html><html><body>SPA App</body></html>"), 0o644); err != nil {
		t.Fatalf("创建 index.html 失败: %v", err)
	}

	// 静态资源文件
	assetsDir := filepath.Join(tmpDir, "assets")
	if err := os.MkdirAll(assetsDir, 0o755); err != nil {
		t.Fatalf("创建 assets 目录失败: %v", err)
	}
	if err := os.WriteFile(filepath.Join(assetsDir, "app.js"), []byte("console.log('app')"), 0o644); err != nil {
		t.Fatalf("创建 app.js 失败: %v", err)
	}
	if err := os.WriteFile(filepath.Join(assetsDir, "style.css"), []byte("body { margin: 0 }"), 0o644); err != nil {
		t.Fatalf("创建 style.css 失败: %v", err)
	}

	handler := NewStaticHandler(tmpDir, "/", []string{"index.html"}, false)
	handler.SetTryFiles([]string{"$uri", "$uri/", "/index.html"}, false, nil)

	tests := []struct {
		name        string
		path        string
		wantContent string
		wantStatus  int
	}{
		{
			name:        "访问存在的静态资源",
			path:        "/assets/app.js",
			wantStatus:  fasthttp.StatusOK,
			wantContent: "console.log('app')",
		},
		{
			name:        "访问存在的 CSS 文件",
			path:        "/assets/style.css",
			wantStatus:  fasthttp.StatusOK,
			wantContent: "body { margin: 0 }",
		},
		{
			name:        "访问前端路由回退到 index.html",
			path:        "/dashboard",
			wantStatus:  fasthttp.StatusOK,
			wantContent: "<!DOCTYPE html><html><body>SPA App</body></html>",
		},
		{
			name:        "访问嵌套前端路由",
			path:        "/user/profile/settings",
			wantStatus:  fasthttp.StatusOK,
			wantContent: "<!DOCTYPE html><html><body>SPA App</body></html>",
		},
		{
			name:        "访问根路径",
			path:        "/",
			wantStatus:  fasthttp.StatusOK,
			wantContent: "<!DOCTYPE html><html><body>SPA App</body></html>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := newTestContext(t, tt.path)
			handler.Handle(ctx)

			if got := ctx.Response.StatusCode(); got != tt.wantStatus {
				t.Errorf("状态码 = %d, want %d", got, tt.wantStatus)
			}

			got := string(ctx.Response.Body())
			if got != tt.wantContent {
				t.Errorf("内容 = %q, want %q", got, tt.wantContent)
			}
		})
	}
}

// TestStaticHandler_TryFilesWithPathPrefix 测试带路径前缀的 try_files
func TestStaticHandler_TryFilesWithPathPrefix(t *testing.T) {
	tmpDir := t.TempDir()

	// 创建 API 模拟文件
	apiDir := filepath.Join(tmpDir, "api")
	if err := os.MkdirAll(apiDir, 0o755); err != nil {
		t.Fatalf("创建 api 目录失败: %v", err)
	}
	if err := os.WriteFile(filepath.Join(apiDir, "users.json"), []byte("[]"), 0o644); err != nil {
		t.Fatalf("创建 users.json 失败: %v", err)
	}

	// 创建静态文件
	if err := os.WriteFile(filepath.Join(tmpDir, "index.html"), []byte("static index"), 0o644); err != nil {
		t.Fatalf("创建 index.html 失败: %v", err)
	}

	handler := NewStaticHandler(tmpDir, "/static", []string{"index.html"}, false)
	handler.SetTryFiles([]string{"$uri", "$uri/", "/index.html"}, false, nil)

	tests := []struct {
		name        string
		path        string
		wantContent string
		wantStatus  int
		skipContent bool
	}{
		{
			name:        "带前缀访问文件",
			path:        "/static/api/users.json",
			wantStatus:  fasthttp.StatusOK,
			wantContent: "[]",
		},
		{
			name:        "带前缀访问目录",
			path:        "/static/api/",
			wantStatus:  fasthttp.StatusOK, // 目录无索引文件，但会回退到 /index.html
			wantContent: "static index",
		},
		{
			name:        "前缀剥离后回退",
			path:        "/static/unknown",
			wantStatus:  fasthttp.StatusOK,
			wantContent: "static index",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := newTestContext(t, tt.path)
			handler.Handle(ctx)

			if got := ctx.Response.StatusCode(); got != tt.wantStatus {
				t.Errorf("状态码 = %d, want %d", got, tt.wantStatus)
			}

			if !tt.skipContent {
				got := string(ctx.Response.Body())
				if got != tt.wantContent {
					t.Errorf("内容 = %q, want %q", got, tt.wantContent)
				}
			}
		})
	}
}

// TestStaticHandler_Alias 测试 alias 路径替换功能
func TestStaticHandler_Alias(t *testing.T) {
	tests := []struct {
		setup       func(t *testing.T, aliasDir string)
		name        string
		alias       string
		pathPrefix  string
		path        string
		wantContent string
		wantStatus  int
		skipContent bool
	}{
		{
			name: "alias 基础替换",
			setup: func(t *testing.T, aliasDir string) {
				if err := os.WriteFile(filepath.Join(aliasDir, "logo.png"), []byte("png content"), 0o644); err != nil {
					t.Fatalf("创建文件失败: %v", err)
				}
			},
			alias:       "/alias/images/",
			pathPrefix:  "/images/",
			path:        "/images/logo.png",
			wantStatus:  fasthttp.StatusOK,
			wantContent: "png content",
		},
		{
			name: "alias 嵌套路径",
			setup: func(t *testing.T, aliasDir string) {
				subDir := filepath.Join(aliasDir, "icons")
				if err := os.MkdirAll(subDir, 0o755); err != nil {
					t.Fatalf("创建目录失败: %v", err)
				}
				if err := os.WriteFile(filepath.Join(subDir, "app.png"), []byte("app icon"), 0o644); err != nil {
					t.Fatalf("创建文件失败: %v", err)
				}
			},
			alias:       "/alias/img/",
			pathPrefix:  "/images/",
			path:        "/images/icons/app.png",
			wantStatus:  fasthttp.StatusOK,
			wantContent: "app icon",
		},
		{
			name: "alias 目录索引",
			setup: func(t *testing.T, aliasDir string) {
				if err := os.WriteFile(filepath.Join(aliasDir, "index.html"), []byte("alias index"), 0o644); err != nil {
					t.Fatalf("创建文件失败: %v", err)
				}
			},
			alias:       "/alias/images/",
			pathPrefix:  "/images/",
			path:        "/images/",
			wantStatus:  fasthttp.StatusOK,
			wantContent: "alias index",
		},
		{
			name: "alias 文件不存在",
			setup: func(_ *testing.T, _ string) {
				// 不创建任何文件
			},
			alias:       "/alias/images/",
			pathPrefix:  "/images/",
			path:        "/images/notfound.png",
			wantStatus:  fasthttp.StatusNotFound,
			skipContent: true,
		},
		{
			name: "root 与 alias 互斥 - 使用 alias",
			setup: func(t *testing.T, aliasDir string) {
				if err := os.WriteFile(filepath.Join(aliasDir, "file.txt"), []byte("from alias"), 0o644); err != nil {
					t.Fatalf("创建文件失败: %v", err)
				}
			},
			alias:       "/alias/images/",
			pathPrefix:  "/images/",
			path:        "/images/file.txt",
			wantStatus:  fasthttp.StatusOK,
			wantContent: "from alias",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 创建临时目录作为 alias 目录
			aliasDir := t.TempDir()
			tt.setup(t, aliasDir)

			// 创建处理器（使用 alias）
			handler := NewStaticHandlerWithAlias(aliasDir, tt.pathPrefix, []string{"index.html"}, false)

			ctx := newTestContext(t, tt.path)
			handler.Handle(ctx)

			if got := ctx.Response.StatusCode(); got != tt.wantStatus {
				t.Errorf("状态码 = %d, want %d", got, tt.wantStatus)
			}

			if !tt.skipContent && tt.wantContent != "" {
				got := string(ctx.Response.Body())
				if got != tt.wantContent {
					t.Errorf("内容 = %q, want %q", got, tt.wantContent)
				}
			}
		})
	}
}

// TestStaticHandler_AliasVsRoot 测试 alias 和 root 的行为区别
func TestStaticHandler_AliasVsRoot(t *testing.T) {
	// root 目录
	rootDir := t.TempDir()
	// alias 目录
	aliasDir := t.TempDir()

	// 在 root 创建子目录和文件
	imagesDir := filepath.Join(rootDir, "images")
	if err := os.MkdirAll(imagesDir, 0o755); err != nil {
		t.Fatalf("创建 images 目录失败: %v", err)
	}
	if err := os.WriteFile(filepath.Join(imagesDir, "logo.png"), []byte("from root"), 0o644); err != nil {
		t.Fatalf("创建 root 文件失败: %v", err)
	}
	if err := os.WriteFile(filepath.Join(aliasDir, "logo.png"), []byte("from alias"), 0o644); err != nil {
		t.Fatalf("创建 alias 文件失败: %v", err)
	}

	tests := []struct {
		name        string
		handler     *StaticHandler
		path        string
		wantContent string
	}{
		{
			name:        "root 模式：请求路径附加到 root",
			handler:     NewStaticHandler(rootDir, "/", []string{"index.html"}, false),
			path:        "/images/logo.png",
			wantContent: "from root", // /var/www/images/logo.png
		},
		{
			name:        "alias 模式：请求路径替换匹配部分",
			handler:     NewStaticHandlerWithAlias(aliasDir, "/images/", []string{"index.html"}, false),
			path:        "/images/logo.png",
			wantContent: "from alias", // /alias/logo.png（images/被替换）
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := newTestContext(t, tt.path)
			tt.handler.Handle(ctx)

			got := string(ctx.Response.Body())
			if got != tt.wantContent {
				t.Errorf("内容 = %q, want %q", got, tt.wantContent)
			}
		})
	}
}

// TestStaticHandler_SetAlias 测试 SetAlias 方法
func TestStaticHandler_SetAlias(t *testing.T) {
	t.Run("设置 alias", func(t *testing.T) {
		handler := NewStaticHandler("/root", "/", nil, false)

		if handler.GetAlias() != "" {
			t.Error("初始 alias 应为空")
		}

		handler.SetAlias("/alias")

		if handler.GetAlias() != "/alias" {
			t.Errorf("GetAlias() = %q, want %q", handler.GetAlias(), "/alias")
		}
		if handler.GetRoot() != "" {
			t.Error("设置 alias 后 root 应被清空")
		}
	})

	t.Run("设置 root 清除 alias", func(t *testing.T) {
		handler := NewStaticHandlerWithAlias("/alias", "/", nil, false)

		if handler.GetAlias() != "/alias" {
			t.Error("初始 alias 应为 /alias")
		}

		handler.SetRoot("/root")

		// root 被规范化为带尾部斜杠的形式
		expectedRoot := "/root/"
		if handler.GetRoot() != expectedRoot {
			t.Errorf("GetRoot() = %q, want %q", handler.GetRoot(), expectedRoot)
		}
		if handler.GetAlias() != "" {
			t.Error("设置 root 后 alias 应被清空")
		}
	})
}

// TestStaticHandler_AliasWithTryFiles 测试 alias 与 try_files 组合
func TestStaticHandler_AliasWithTryFiles(t *testing.T) {
	aliasDir := t.TempDir()

	// 创建测试文件
	if err := os.WriteFile(filepath.Join(aliasDir, "app.js"), []byte("app content"), 0o644); err != nil {
		t.Fatalf("创建文件失败: %v", err)
	}
	if err := os.WriteFile(filepath.Join(aliasDir, "index.html"), []byte("fallback"), 0o644); err != nil {
		t.Fatalf("创建文件失败: %v", err)
	}

	handler := NewStaticHandlerWithAlias(aliasDir, "/static/", []string{"index.html"}, false)
	handler.SetTryFiles([]string{"$uri", "/index.html"}, false, nil)

	tests := []struct {
		name        string
		path        string
		wantContent string
		wantStatus  int
	}{
		{
			name:        "找到文件",
			path:        "/static/app.js",
			wantStatus:  fasthttp.StatusOK,
			wantContent: "app content",
		},
		{
			name:        "回退到 index.html",
			path:        "/static/nonexistent",
			wantStatus:  fasthttp.StatusOK,
			wantContent: "fallback",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := newTestContext(t, tt.path)
			handler.Handle(ctx)

			if got := ctx.Response.StatusCode(); got != tt.wantStatus {
				t.Errorf("状态码 = %d, want %d", got, tt.wantStatus)
			}

			got := string(ctx.Response.Body())
			if got != tt.wantContent {
				t.Errorf("内容 = %q, want %q", got, tt.wantContent)
			}
		})
	}
}

// TestNewStaticHandlerWithAlias 测试 NewStaticHandlerWithAlias 构造函数
func TestNewStaticHandlerWithAlias(t *testing.T) {
	handler := NewStaticHandlerWithAlias("/var/www/img/", "/images/", []string{"index.html"}, true)

	if handler == nil {
		t.Fatal("NewStaticHandlerWithAlias() 返回 nil")
	}
	if handler.GetAlias() != "/var/www/img/" {
		t.Errorf("GetAlias() = %q, want %q", handler.GetAlias(), "/var/www/img/")
	}
	if handler.GetRoot() != "" {
		t.Errorf("GetRoot() = %q, want 空字符串", handler.GetRoot())
	}
}

// TestStaticHandler_TryFilesEdgeCases 测试 try_files 边界情况
func TestStaticHandler_TryFilesEdgeCases(t *testing.T) {
	tmpDir := t.TempDir()

	// 创建测试文件
	if err := os.WriteFile(filepath.Join(tmpDir, "file with spaces.txt"), []byte("spaces"), 0o644); err != nil {
		t.Fatalf("创建带空格文件失败: %v", err)
	}

	handler := NewStaticHandler(tmpDir, "/", []string{"index.html"}, false)
	handler.SetTryFiles([]string{"$uri", "/index.html"}, false, nil)

	tests := []struct {
		name       string
		path       string
		wantStatus int
	}{
		{
			name:       "路径遍历攻击被阻止 - fasthttp 规范化",
			path:       "/../secret",
			wantStatus: fasthttp.StatusNotFound, // fasthttp 规范化为 /secret，文件不存在返回 404
		},
		{
			name:       "双点号在路径中被阻止",
			path:       "/file..name",
			wantStatus: fasthttp.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := newTestContext(t, tt.path)
			handler.Handle(ctx)

			if got := ctx.Response.StatusCode(); got != tt.wantStatus {
				t.Errorf("状态码 = %d, want %d", got, tt.wantStatus)
			}
		})
	}
}

// TestStaticHandler_LargeFileContentType 测试大文件 sendfile 路径的 Content-Type
func TestStaticHandler_LargeFileContentType(t *testing.T) {
	tmpDir := t.TempDir()

	// 创建大于 8KB 的文件
	largeContent := make([]byte, 16*1024)
	for i := range largeContent {
		largeContent[i] = byte(i % 256)
	}

	tests := []struct {
		ext      string
		expected string
	}{
		{".css", "text/css; charset=utf-8"},
		{".js", "text/javascript; charset=utf-8"},
		{".webmanifest", "application/manifest+json"},
		{".webm", "video/webm"},
		{".otf", "font/otf"},
	}

	for _, tc := range tests {
		t.Run(tc.ext, func(t *testing.T) {
			filePath := filepath.Join(tmpDir, "large"+tc.ext)
			if err := os.WriteFile(filePath, largeContent, 0o644); err != nil {
				t.Fatalf("创建文件失败: %v", err)
			}

			handler := NewStaticHandler(tmpDir, "/", nil, true) // 启用 sendfile
			ctx := &fasthttp.RequestCtx{}
			ctx.Request.SetRequestURI("/large" + tc.ext)

			handler.Handle(ctx)

			if got := ctx.Response.StatusCode(); got != fasthttp.StatusOK {
				t.Errorf("状态码 = %d, want %d", got, fasthttp.StatusOK)
			}

			ct := string(ctx.Response.Header.ContentType())
			if ct != tc.expected {
				t.Errorf("Content-Type = %q, want %q", ct, tc.expected)
			}
		})
	}
}

// TestResolveTryFilePathWithDynamicSuffix 测试动态后缀解析
func TestResolveTryFilePathWithDynamicSuffix(t *testing.T) {
	handler := NewStaticHandler("/var/www", "/", []string{"index.html"}, false)

	tests := []struct {
		name       string
		tryFile    string
		relPath    string
		wantResult string
	}{
		// 基本占位符
		{name: "$uri", tryFile: "$uri", relPath: "/api/user", wantResult: "/api/user"},
		{name: "$uri/", tryFile: "$uri/", relPath: "/api/user", wantResult: "/api/user/"},

		// 动态后缀 - 正常路径
		{name: "$uri.html 正常", tryFile: "$uri.html", relPath: "/about", wantResult: "/about.html"},
		{name: "$uri.json 正常", tryFile: "$uri.json", relPath: "/api/data", wantResult: "/api/data.json"},
		{name: "$uri.css 正常", tryFile: "$uri.css", relPath: "/styles/main", wantResult: "/styles/main.css"},

		// 动态后缀 - 根路径边界（返回空字符串）
		{name: "$uri.html 根路径", tryFile: "$uri.html", relPath: "/", wantResult: ""},
		{name: "$uri.json 根路径", tryFile: "$uri.json", relPath: "/", wantResult: ""},

		// 动态后缀 - 子目录路径（正常处理）
		{name: "$uri.html 子目录", tryFile: "$uri.html", relPath: "/api/", wantResult: "/api/.html"},
		{name: "$uri.json 子目录", tryFile: "$uri.json", relPath: "/v1/", wantResult: "/v1/.json"},

		// 绝对路径
		{name: "绝对路径", tryFile: "/index.html", relPath: "/api/user", wantResult: "index.html"},
		{name: "绝对路径嵌套", tryFile: "/fallback/app.html", relPath: "/any", wantResult: "fallback/app.html"},

		// 相对路径
		{name: "相对路径", tryFile: "fallback.html", relPath: "/api/user", wantResult: "fallback.html"},
		{name: "相对路径带连字符", tryFile: "app-shell.html", relPath: "/any", wantResult: "app-shell.html"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := handler.resolveTryFilePath(tt.tryFile, tt.relPath)
			if got != tt.wantResult {
				t.Errorf("resolveTryFilePath(%q, %q) = %q, want %q",
					tt.tryFile, tt.relPath, got, tt.wantResult)
			}
		})
	}
}

// TestStaticHandler_TryFilesWithDynamicSuffix 测试动态后缀集成
func TestStaticHandler_TryFilesWithDynamicSuffix(t *testing.T) {
	tmpDir := t.TempDir()

	// 创建测试文件
	if err := os.WriteFile(filepath.Join(tmpDir, "about.html"), []byte("about page"), 0o644); err != nil {
		t.Fatalf("创建文件失败: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(tmpDir, "api"), 0o755); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "api", "data.json"), []byte("{\"data\":true}"), 0o644); err != nil {
		t.Fatalf("创建文件失败: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "index.html"), []byte("fallback"), 0o644); err != nil {
		t.Fatalf("创建文件失败: %v", err)
	}

	handler := NewStaticHandler(tmpDir, "/", []string{"index.html"}, false)
	handler.SetTryFiles([]string{"$uri", "$uri.html", "/index.html"}, false, nil)

	tests := []struct {
		name        string
		path        string
		wantContent string
		wantStatus  int
	}{
		{
			name:        "找到 $uri.html",
			path:        "/about",
			wantStatus:  fasthttp.StatusOK,
			wantContent: "about page",
		},
		{
			name:        "回退到 /index.html",
			path:        "/nonexistent",
			wantStatus:  fasthttp.StatusOK,
			wantContent: "fallback",
		},
		{
			name:        "根路径回退到 /index.html",
			path:        "/",
			wantStatus:  fasthttp.StatusOK,
			wantContent: "fallback",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := newTestContext(t, tt.path)
			handler.Handle(ctx)

			if got := ctx.Response.StatusCode(); got != tt.wantStatus {
				t.Errorf("状态码 = %d, want %d", got, tt.wantStatus)
			}

			if got := string(ctx.Response.Body()); got != tt.wantContent {
				t.Errorf("内容 = %q, want %q", got, tt.wantContent)
			}
		})
	}
}

// TestStaticHandler_TryFilesRootPathFallback 测试根路径回退
func TestStaticHandler_TryFilesRootPathFallback(t *testing.T) {
	tmpDir := t.TempDir()

	// 创建 index.html 作为根路径回退
	if err := os.WriteFile(filepath.Join(tmpDir, "index.html"), []byte("root fallback"), 0o644); err != nil {
		t.Fatalf("创建文件失败: %v", err)
	}

	// 注意：不创建 /.html 文件，测试根路径边界情况
	handler := NewStaticHandler(tmpDir, "/", []string{"index.html"}, false)
	handler.SetTryFiles([]string{"$uri", "$uri.html", "/index.html"}, false, nil)

	ctx := newTestContext(t, "/")
	handler.Handle(ctx)

	// 验证根路径请求正确回退到 /index.html
	if got := ctx.Response.StatusCode(); got != fasthttp.StatusOK {
		t.Errorf("状态码 = %d, want %d", got, fasthttp.StatusOK)
	}

	if got := string(ctx.Response.Body()); got != "root fallback" {
		t.Errorf("内容 = %q, want %q", got, "root fallback")
	}
}

// TestStaticHandler_SetSymlinkCheck 测试 SetSymlinkCheck 方法
func TestStaticHandler_SetSymlinkCheck(t *testing.T) {
	handler := NewStaticHandler("/var/www", "/", nil, false)

	if handler.symlinkCheck {
		t.Error("初始 symlinkCheck 应为 false")
	}

	handler.SetSymlinkCheck(true)
	if !handler.symlinkCheck {
		t.Error("SetSymlinkCheck(true) 后 symlinkCheck 应为 true")
	}

	handler.SetSymlinkCheck(false)
	if handler.symlinkCheck {
		t.Error("SetSymlinkCheck(false) 后 symlinkCheck 应为 false")
	}
}

// TestStaticHandler_SetInternal 测试 SetInternal 方法
func TestStaticHandler_SetInternal(t *testing.T) {
	handler := NewStaticHandler("/var/www", "/", nil, false)

	if handler.internal {
		t.Error("初始 internal 应为 false")
	}

	handler.SetInternal(true)
	if !handler.internal {
		t.Error("SetInternal(true) 后 internal 应为 true")
	}

	handler.SetInternal(false)
	if handler.internal {
		t.Error("SetInternal(false) 后 internal 应为 false")
	}
}

// TestStaticHandler_SetCacheTTL 测试 SetCacheTTL 方法
func TestStaticHandler_SetCacheTTL(t *testing.T) {
	handler := NewStaticHandler("/var/www", "/", nil, false)

	if handler.cacheTTL != 0 {
		t.Error("初始 cacheTTL 应为 0")
	}

	handler.SetCacheTTL(5 * time.Second)
	if handler.cacheTTL != 5*time.Second {
		t.Errorf("SetCacheTTL 后 cacheTTL = %v, want %v", handler.cacheTTL, 5*time.Second)
	}

	handler.SetCacheTTL(0)
	if handler.cacheTTL != 0 {
		t.Error("SetCacheTTL(0) 后 cacheTTL 应为 0")
	}
}

// TestStaticHandler_InternalRestriction 测试 internal 访问限制
func TestStaticHandler_InternalRestriction(t *testing.T) {
	tmpDir := t.TempDir()
	content := "internal content"
	if err := os.WriteFile(filepath.Join(tmpDir, "test.txt"), []byte(content), 0o644); err != nil {
		t.Fatalf("创建测试文件失败: %v", err)
	}

	handler := newTestHandler(t, tmpDir)
	handler.SetInternal(true)

	t.Run("外部请求返回 404", func(t *testing.T) {
		ctx := newTestContext(t, "/test.txt")
		handler.Handle(ctx)

		if got := ctx.Response.StatusCode(); got != fasthttp.StatusNotFound {
			t.Errorf("状态码 = %d, want %d", got, fasthttp.StatusNotFound)
		}
	})

	t.Run("内部重定向允许访问", func(t *testing.T) {
		ctx := newTestContext(t, "/test.txt")
		// 标记为内部重定向
		ctx.SetUserValue("__internal_redirect__", "/test.txt")
		handler.Handle(ctx)

		if got := ctx.Response.StatusCode(); got != fasthttp.StatusOK {
			t.Errorf("状态码 = %d, want %d", got, fasthttp.StatusOK)
		}
	})
}

// TestStaticHandler_ValidateSymlink 测试符号链接验证
func TestStaticHandler_ValidateSymlink(t *testing.T) {
	tmpDir := t.TempDir()

	// 创建目标文件和目录
	targetDir := filepath.Join(tmpDir, "target")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatalf("创建目标目录失败: %v", err)
	}
	targetFile := filepath.Join(targetDir, "secret.txt")
	if err := os.WriteFile(targetFile, []byte("secret content"), 0o644); err != nil {
		t.Fatalf("创建目标文件失败: %v", err)
	}

	// 创建允许的根目录
	allowedDir := filepath.Join(tmpDir, "allowed")
	if err := os.MkdirAll(allowedDir, 0o755); err != nil {
		t.Fatalf("创建允许目录失败: %v", err)
	}

	t.Run("安全符号链接 - 在允许范围内", func(t *testing.T) {
		// 在允许目录内创建符号链接
		linkFile := filepath.Join(allowedDir, "link.txt")
		allowedTarget := filepath.Join(allowedDir, "actual.txt")
		if err := os.WriteFile(allowedTarget, []byte("allowed content"), 0o644); err != nil {
			t.Fatalf("创建实际文件失败: %v", err)
		}
		if err := os.Symlink(allowedTarget, linkFile); err != nil {
			t.Fatalf("创建符号链接失败: %v", err)
		}

		handler := NewStaticHandler(allowedDir, "/", nil, false)
		handler.SetSymlinkCheck(true)

		ctx := newTestContext(t, "/link.txt")
		handler.Handle(ctx)

		if got := ctx.Response.StatusCode(); got != fasthttp.StatusOK {
			t.Errorf("状态码 = %d, want %d", got, fasthttp.StatusOK)
		}
	})

	t.Run("不安全符号链接 - 指向允许范围外", func(t *testing.T) {
		// 创建指向允许目录外的符号链接
		unsafeLink := filepath.Join(allowedDir, "unsafe.txt")
		if err := os.Symlink(targetFile, unsafeLink); err != nil {
			t.Fatalf("创建不安全符号链接失败: %v", err)
		}

		handler := NewStaticHandler(allowedDir, "/", nil, false)
		handler.SetSymlinkCheck(true)

		ctx := newTestContext(t, "/unsafe.txt")
		handler.Handle(ctx)

		if got := ctx.Response.StatusCode(); got != fasthttp.StatusForbidden {
			t.Errorf("状态码 = %d, want %d", got, fasthttp.StatusForbidden)
		}
	})

	t.Run("普通文件 - 非符号链接", func(t *testing.T) {
		normalFile := filepath.Join(allowedDir, "normal.txt")
		if err := os.WriteFile(normalFile, []byte("normal content"), 0o644); err != nil {
			t.Fatalf("创建普通文件失败: %v", err)
		}

		handler := NewStaticHandler(allowedDir, "/", nil, false)
		handler.SetSymlinkCheck(true)

		ctx := newTestContext(t, "/normal.txt")
		handler.Handle(ctx)

		if got := ctx.Response.StatusCode(); got != fasthttp.StatusOK {
			t.Errorf("状态码 = %d, want %d", got, fasthttp.StatusOK)
		}
	})

	t.Run("未启用符号链接检查", func(t *testing.T) {
		// 创建指向允许目录外的符号链接
		externalLink := filepath.Join(allowedDir, "external.txt")
		if err := os.Symlink(targetFile, externalLink); err != nil {
			t.Fatalf("创建外部符号链接失败: %v", err)
		}

		handler := NewStaticHandler(allowedDir, "/", nil, false)
		// 不启用符号链接检查

		ctx := newTestContext(t, "/external.txt")
		handler.Handle(ctx)

		// 未启用检查时，可以访问符号链接
		if got := ctx.Response.StatusCode(); got != fasthttp.StatusOK {
			t.Errorf("状态码 = %d, want %d", got, fasthttp.StatusOK)
		}
	})
}

// TestStaticHandler_ValidateSymlink_WithAlias 测试 alias 模式下的符号链接验证
func TestStaticHandler_ValidateSymlink_WithAlias(t *testing.T) {
	tmpDir := t.TempDir()

	// 创建 alias 目录
	aliasDir := filepath.Join(tmpDir, "alias")
	if err := os.MkdirAll(aliasDir, 0o755); err != nil {
		t.Fatalf("创建 alias 目录失败: %v", err)
	}

	// 在 alias 目录内创建文件和符号链接
	actualFile := filepath.Join(aliasDir, "actual.txt")
	if err := os.WriteFile(actualFile, []byte("actual content"), 0o644); err != nil {
		t.Fatalf("创建实际文件失败: %v", err)
	}

	linkFile := filepath.Join(aliasDir, "link.txt")
	if err := os.Symlink(actualFile, linkFile); err != nil {
		t.Fatalf("创建符号链接失败: %v", err)
	}

	handler := NewStaticHandlerWithAlias(aliasDir, "/static/", nil, false)
	handler.SetSymlinkCheck(true)

	ctx := newTestContext(t, "/static/link.txt")
	handler.Handle(ctx)

	if got := ctx.Response.StatusCode(); got != fasthttp.StatusOK {
		t.Errorf("状态码 = %d, want %d", got, fasthttp.StatusOK)
	}
}

// TestStaticHandler_ValidateSymlink_NoRootOrAlias 测试无 root/alias 时符号链接验证
func TestStaticHandler_ValidateSymlink_NoRootOrAlias(t *testing.T) {
	tmpDir := t.TempDir()

	// 创建文件和符号链接
	targetFile := filepath.Join(tmpDir, "target.txt")
	if err := os.WriteFile(targetFile, []byte("target"), 0o644); err != nil {
		t.Fatalf("创建目标文件失败: %v", err)
	}

	linkFile := filepath.Join(tmpDir, "link.txt")
	if err := os.Symlink(targetFile, linkFile); err != nil {
		t.Fatalf("创建符号链接失败: %v", err)
	}

	// 创建无 root/alias 的处理器
	handler := NewStaticHandler("", "/", nil, false)
	handler.SetSymlinkCheck(true)

	// 直接调用 validateSymlink
	err := handler.validateSymlink(linkFile)
	if err == nil {
		t.Error("无 root/alias 时验证符号链接应返回错误")
	}
}

// TestStaticHandler_Handle_WithCacheTTL 测试带 TTL 的缓存处理
func TestStaticHandler_Handle_WithCacheTTL(t *testing.T) {
	tmpDir := t.TempDir()
	content := "cached with ttl"
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte(content), 0o644); err != nil {
		t.Fatalf("创建测试文件失败: %v", err)
	}

	// 创建带缓存的处理器
	handler := newTestHandler(t, tmpDir)
	handler.SetFileCache(cache.NewFileCache(100, 1024*1024, time.Hour))
	handler.SetCacheTTL(5 * time.Second)

	// 第一次请求，填充缓存
	ctx1 := newTestContext(t, "/test.txt")
	handler.Handle(ctx1)

	if got := ctx1.Response.StatusCode(); got != fasthttp.StatusOK {
		t.Errorf("第一次请求状态码 = %d, want %d", got, fasthttp.StatusOK)
	}

	// 第二次请求，应该命中缓存
	ctx2 := newTestContext(t, "/test.txt")
	handler.Handle(ctx2)

	if got := ctx2.Response.StatusCode(); got != fasthttp.StatusOK {
		t.Errorf("第二次请求状态码 = %d, want %d", got, fasthttp.StatusOK)
	}

	// 内容应该一致
	if string(ctx1.Response.Body()) != string(ctx2.Response.Body()) {
		t.Error("缓存内容不一致")
	}
}

// TestStaticHandler_Handle_CacheTTLExpired 测试 TTL 过期后重新验证
func TestStaticHandler_Handle_CacheTTLExpired(t *testing.T) {
	tmpDir := t.TempDir()
	content := "initial content"
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte(content), 0o644); err != nil {
		t.Fatalf("创建测试文件失败: %v", err)
	}

	// 创建带缓存的处理器，TTL 设置很短
	handler := newTestHandler(t, tmpDir)
	handler.SetFileCache(cache.NewFileCache(100, 1024*1024, time.Hour))
	handler.SetCacheTTL(100 * time.Millisecond)

	// 第一次请求，填充缓存
	ctx1 := newTestContext(t, "/test.txt")
	handler.Handle(ctx1)

	// 等待 TTL 过期
	time.Sleep(150 * time.Millisecond)

	// 修改文件
	newContent := "updated content"
	if err := os.WriteFile(testFile, []byte(newContent), 0o644); err != nil {
		t.Fatalf("更新文件失败: %v", err)
	}

	// 第二次请求，TTL 过期后应该重新读取
	ctx2 := newTestContext(t, "/test.txt")
	handler.Handle(ctx2)

	if got := string(ctx2.Response.Body()); got != newContent {
		t.Errorf("TTL 过期后内容 = %q, want %q", got, newContent)
	}
}

// TestStaticHandler_Handle_CacheModTimeChanged 测试文件修改后缓存更新
func TestStaticHandler_Handle_CacheModTimeChanged(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	initialContent := "initial"
	if err := os.WriteFile(testFile, []byte(initialContent), 0o644); err != nil {
		t.Fatalf("创建测试文件失败: %v", err)
	}

	// 创建带缓存的处理器
	handler := newTestHandler(t, tmpDir)
	handler.SetFileCache(cache.NewFileCache(100, 1024*1024, time.Hour))

	// 第一次请求，填充缓存
	ctx1 := newTestContext(t, "/test.txt")
	handler.Handle(ctx1)

	// 修改文件
	time.Sleep(10 * time.Millisecond) // 确保 ModTime 变化
	newContent := "modified"
	if err := os.WriteFile(testFile, []byte(newContent), 0o644); err != nil {
		t.Fatalf("修改文件失败: %v", err)
	}

	// 第二次请求，应该检测到文件变化并更新缓存
	ctx2 := newTestContext(t, "/test.txt")
	handler.Handle(ctx2)

	if got := string(ctx2.Response.Body()); got != newContent {
		t.Errorf("修改后内容 = %q, want %q", got, newContent)
	}
}

// TestStaticHandler_SetExpires 测试 SetExpires 方法。
func TestStaticHandler_SetExpires(t *testing.T) {
	tmpDir := t.TempDir()
	handler := newTestHandler(t, tmpDir)

	// 默认为空
	if handler.expires != "" {
		t.Errorf("默认 expires = %q, want empty", handler.expires)
	}

	// 设置 expires
	handler.SetExpires("30d")
	if handler.expires != "30d" {
		t.Errorf("expires = %q, want 30d", handler.expires)
	}

	// 设置 off
	handler.SetExpires("off")
	if handler.expires != "off" {
		t.Errorf("expires = %q, want off", handler.expires)
	}

	// 设置 max
	handler.SetExpires("max")
	if handler.expires != "max" {
		t.Errorf("expires = %q, want max", handler.expires)
	}
}

// TestSetCacheHeaders 测试 setCacheHeaders 方法。
func TestSetCacheHeaders(t *testing.T) {
	tests := []struct {
		name           string
		expires        string
		wantCacheCtrl  string
		wantExpiresSet bool // 是否设置 Expires 头
	}{
		{
			name:          "empty_expires",
			expires:       "",
			wantCacheCtrl: "",
		},
		{
			name:          "off_expires",
			expires:       "off",
			wantCacheCtrl: "",
		},
		{
			name:           "epoch_expires",
			expires:        "epoch",
			wantCacheCtrl:  "no-cache, no-store, must-revalidate",
			wantExpiresSet: true,
		},
		{
			name:           "max_expires",
			expires:        "max",
			wantCacheCtrl:  "public, max-age=315360000, immutable",
			wantExpiresSet: true,
		},
		{
			name:           "duration_expires",
			expires:        "1h",
			wantCacheCtrl:  "public, max-age=3600",
			wantExpiresSet: true,
		},
		{
			name:           "complex_duration",
			expires:        "1d1h",
			wantCacheCtrl:  "public, max-age=90000",
			wantExpiresSet: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			handler := newTestHandler(t, tmpDir)
			handler.SetExpires(tt.expires)

			ctx := newTestContext(t, "/test.txt")
			handler.setCacheHeaders(ctx)

			cacheCtrl := string(ctx.Response.Header.Peek("Cache-Control"))
			if tt.wantCacheCtrl == "" {
				if cacheCtrl != "" {
					t.Errorf("Cache-Control = %q, want empty", cacheCtrl)
				}
			} else {
				if cacheCtrl != tt.wantCacheCtrl {
					t.Errorf("Cache-Control = %q, want %q", cacheCtrl, tt.wantCacheCtrl)
				}
			}

			expires := string(ctx.Response.Header.Peek("Expires"))
			if tt.wantExpiresSet && expires == "" {
				t.Error("Expected Expires header to be set")
			}
		})
	}
}

// TestParseExpires 测试 parseExpires 函数。
func TestParseExpires(t *testing.T) {
	tests := []struct {
		name     string
		expires  string
		wantSecs int64
	}{
		{"empty", "", 0},
		{"off", "off", 0},
		{"max", "max", 315360000},
		{"epoch", "epoch", -1},
		{"seconds", "30s", 30},
		{"minutes", "5m", 300},
		{"hours", "2h", 7200},
		{"days", "1d", 86400},
		{"complex", "1d1h1m1s", 90061},
		{"multiple_days", "30d", 2592000},
		{"mixed", "7d12h", 648000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseExpires(tt.expires)
			if got != tt.wantSecs {
				t.Errorf("parseExpires(%q) = %d, want %d", tt.expires, got, tt.wantSecs)
			}
		})
	}
}
