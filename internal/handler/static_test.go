// Package handler 提供静态文件处理器的测试。
package handler

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/valyala/fasthttp"
)

// newTestHandler 创建测试用的静态文件处理器
func newTestHandler(t *testing.T, root string) *StaticHandler {
	t.Helper()
	return NewStaticHandler(root, []string{"index.html", "index.htm"})
}

// newTestContext 创建测试用的 fasthttp 请求上下文
func newTestContext(t *testing.T, path string) *fasthttp.RequestCtx {
	t.Helper()
	var ctx fasthttp.RequestCtx
	ctx.Request.SetRequestURI(path)
	return &ctx
}

// TestStaticHandlerHandle 测试静态文件处理器
func TestStaticHandlerHandle(t *testing.T) {
	tests := []struct {
		name         string
		setup        func(t *testing.T, root string) // 在临时目录中设置测试文件
		path         string                          // 请求路径
		wantStatus   int                             // 期望的 HTTP 状态码
		wantContent  string                          // 期望的响应内容（可选）
		skipContent  bool                            // 是否跳过内容验证
	}{
		{
			name: "正常文件访问",
			setup: func(t *testing.T, root string) {
				t.Helper()
				content := "hello world"
				if err := os.WriteFile(filepath.Join(root, "test.txt"), []byte(content), 0644); err != nil {
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
				if err := os.MkdirAll(subDir, 0755); err != nil {
					t.Fatalf("创建子目录失败: %v", err)
				}
				content := "nested file content"
				if err := os.WriteFile(filepath.Join(subDir, "nested.txt"), []byte(content), 0644); err != nil {
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
				if err := os.MkdirAll(dir, 0755); err != nil {
					t.Fatalf("创建目录失败: %v", err)
				}
				content := "index page"
				if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte(content), 0644); err != nil {
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
				if err := os.MkdirAll(dir, 0755); err != nil {
					t.Fatalf("创建目录失败: %v", err)
				}
			},
			path:       "/noindex/",
			wantStatus: fasthttp.StatusForbidden,
			skipContent: true,
		},
		{
			name: "文件不存在",
			setup: func(t *testing.T, root string) {
				t.Helper()
				// 不创建任何文件
			},
			path:       "/nonexistent.txt",
			wantStatus: fasthttp.StatusNotFound,
			skipContent: true,
		},
		{
			name: "空路径访问根目录无索引",
			setup: func(t *testing.T, root string) {
				t.Helper()
				// root 目录没有索引文件
			},
			path:       "/",
			wantStatus: fasthttp.StatusForbidden,
			skipContent: true,
		},
		{
			name: "根目录有索引文件",
			setup: func(t *testing.T, root string) {
				t.Helper()
				content := "root index"
				if err := os.WriteFile(filepath.Join(root, "index.html"), []byte(content), 0644); err != nil {
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
		name        string
		setup       func(t *testing.T, root string)
		path        string
		wantStatus  int
		description string // 说明测试预期行为
	}{
		{
			name: "文件名包含双点 - 安全检查拦截",
			setup: func(t *testing.T, root string) {
				t.Helper()
				// 不创建任何文件
			},
			path:        "/file..txt",
			wantStatus:  fasthttp.StatusForbidden,
			description: "路径包含 '..' 字符串，触发安全检查返回 403",
		},
		{
			name: "路径末尾双点 - 安全检查拦截",
			setup: func(t *testing.T, root string) {
				t.Helper()
			},
			path:        "/foo/..",
			wantStatus:  fasthttp.StatusForbidden,
			description: "路径末尾包含 '..'，触发安全检查返回 403",
		},
		{
			name: "隐藏文件 .hidden - 文件不存在",
			setup: func(t *testing.T, root string) {
				t.Helper()
			},
			path:        "/.hidden",
			wantStatus:  fasthttp.StatusNotFound,
			description: "单点开头的隐藏文件不触发安全检查，文件不存在返回 404",
		},
		{
			name: "文件名包含多点 ...txt - 安全检查拦截",
			setup: func(t *testing.T, root string) {
				t.Helper()
			},
			path:        "/file...txt",
			wantStatus:  fasthttp.StatusForbidden,
			description: "包含连续多点（含 '..'）触发安全检查返回 403",
		},
		{
			name: "fasthttp 规范化后的路径 - 文件不存在",
			setup: func(t *testing.T, root string) {
				t.Helper()
				// fasthttp 将 /../secret.txt 规范化为 /secret.txt
			},
			path:        "/../secret.txt",
			wantStatus:  fasthttp.StatusNotFound,
			description: "fasthttp 自动规范化路径移除 ../，结果路径文件不存在返回 404",
		},
		{
			name: "URL 编码路径遍历 - fasthttp 规范化",
			setup: func(t *testing.T, root string) {
				t.Helper()
				// fasthttp 解码 %2e%2e 为 .. 并规范化路径
			},
			path:        "/%2e%2e/secret.txt",
			wantStatus:  fasthttp.StatusNotFound,
			description: "fasthttp 解码 URL 编码后规范化路径，文件不存在返回 404",
		},
		{
			name: "混合 URL 编码 - fasthttp 规范化",
			setup: func(t *testing.T, root string) {
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
				if err := os.WriteFile(filepath.Join(root, "bar.txt"), []byte("bar"), 0644); err != nil {
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
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("创建目录失败: %v", err)
		}

		// 创建两个索引文件
		if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("html content"), 0644); err != nil {
			t.Fatalf("创建 index.html 失败: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "index.htm"), []byte("htm content"), 0644); err != nil {
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
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("创建目录失败: %v", err)
		}

		// 仅创建 index.htm
		if err := os.WriteFile(filepath.Join(dir, "index.htm"), []byte("htm content"), 0644); err != nil {
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
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("创建目录失败: %v", err)
		}

		// 创建一个非索引文件
		if err := os.WriteFile(filepath.Join(dir, "other.txt"), []byte("other content"), 0644); err != nil {
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
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("创建目录失败: %v", err)
		}

		// 创建索引文件
		if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("index"), 0644); err != nil {
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
		handler := NewStaticHandler(root, index)

		if handler == nil {
			t.Fatal("NewStaticHandler() 返回 nil")
		}
		if handler.root != root {
			t.Errorf("handler.root = %q, want %q", handler.root, root)
		}
		if len(handler.index) != len(index) {
			t.Errorf("len(handler.index) = %d, want %d", len(handler.index), len(index))
		}
	})

	t.Run("空索引列表", func(t *testing.T) {
		handler := NewStaticHandler("/var/www", nil)
		if handler == nil {
			t.Fatal("NewStaticHandler() 返回 nil")
		}
		if handler.index != nil {
			t.Errorf("handler.index 应为 nil")
		}
	})
}