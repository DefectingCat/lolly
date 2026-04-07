// Package compression 提供 gzip_static 预压缩文件功能的测试。
//
// 该文件测试 gzip_static 模块的各项功能，包括：
//   - Brotli 和 Gzip 文件优先级
//   - Gzip 回退机制
//   - Accept-Encoding 头解析
//   - 扩展名检查
//   - 路径遍历防护
//   - Vary 头设置
//
// 作者：xfy
package compression

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/valyala/fasthttp"
)

// TestGzipStaticServeFile_BrotliPriority 测试 .br 文件优先于 .gz 文件
func TestGzipStaticServeFile_BrotliPriority(t *testing.T) {
	// 创建临时目录
	tmpDir := t.TempDir()

	// 创建 .br 和 .gz 文件
	brFile := filepath.Join(tmpDir, "test.js.br")
	gzFile := filepath.Join(tmpDir, "test.js.gz")

	if err := os.WriteFile(brFile, []byte("br content"), 0644); err != nil {
		t.Fatalf("创建 .br 文件失败: %v", err)
	}
	if err := os.WriteFile(gzFile, []byte("gz content"), 0644); err != nil {
		t.Fatalf("创建 .gz 文件失败: %v", err)
	}

	g := NewGzipStatic(true, tmpDir, nil)

	tests := []struct {
		name           string
		acceptEncoding string
		wantServed     bool
		wantEncoding   string
	}{
		{
			name:           "同时支持 br 和 gzip，优先返回 br",
			acceptEncoding: "br, gzip",
			wantServed:     true,
			wantEncoding:   "br",
		},
		{
			name:           "只支持 br",
			acceptEncoding: "br",
			wantServed:     true,
			wantEncoding:   "br",
		},
		{
			name:           "只支持 gzip",
			acceptEncoding: "gzip",
			wantServed:     true,
			wantEncoding:   "gzip",
		},
		{
			name:           "支持 deflate 和 gzip，返回 gzip",
			acceptEncoding: "deflate, gzip",
			wantServed:     true,
			wantEncoding:   "gzip",
		},
		{
			name:           "不支持任何编码",
			acceptEncoding: "",
			wantServed:     false,
			wantEncoding:   "",
		},
		{
			name:           "支持 br（大小写不敏感）",
			acceptEncoding: "BR",
			wantServed:     true,
			wantEncoding:   "br",
		},
		{
			name:           "支持 gzip（大小写不敏感）",
			acceptEncoding: "GZIP",
			wantServed:     true,
			wantEncoding:   "gzip",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &fasthttp.RequestCtx{}
			ctx.Request.Header.Set("Accept-Encoding", tt.acceptEncoding)

			served := g.ServeFile(ctx, "test.js")

			if served != tt.wantServed {
				t.Errorf("ServeFile() = %v, want %v", served, tt.wantServed)
			}

			if tt.wantServed {
				encoding := ctx.Response.Header.Peek("Content-Encoding")
				if string(encoding) != tt.wantEncoding {
					t.Errorf("Content-Encoding = %q, want %q", string(encoding), tt.wantEncoding)
				}
			}
		})
	}
}

// TestGzipStaticServeFile_GzipFallback 测试仅 .gz 存在时的回退
func TestGzipStaticServeFile_GzipFallback(t *testing.T) {
	// 创建临时目录
	tmpDir := t.TempDir()

	// 只创建 .gz 文件
	gzFile := filepath.Join(tmpDir, "test.css.gz")
	if err := os.WriteFile(gzFile, []byte("gz content"), 0644); err != nil {
		t.Fatalf("创建 .gz 文件失败: %v", err)
	}

	g := NewGzipStatic(true, tmpDir, nil)

	tests := []struct {
		name           string
		acceptEncoding string
		wantServed     bool
		wantEncoding   string
	}{
		{
			name:           "支持 br 但没有 .br 文件，回退到 gzip",
			acceptEncoding: "br, gzip",
			wantServed:     true,
			wantEncoding:   "gzip",
		},
		{
			name:           "只支持 gzip",
			acceptEncoding: "gzip",
			wantServed:     true,
			wantEncoding:   "gzip",
		},
		{
			name:           "只支持 br，没有 .br 文件，不服务",
			acceptEncoding: "br",
			wantServed:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &fasthttp.RequestCtx{}
			ctx.Request.Header.Set("Accept-Encoding", tt.acceptEncoding)

			served := g.ServeFile(ctx, "test.css")

			if served != tt.wantServed {
				t.Errorf("ServeFile() = %v, want %v", served, tt.wantServed)
			}

			if tt.wantServed {
				encoding := ctx.Response.Header.Peek("Content-Encoding")
				if string(encoding) != tt.wantEncoding {
					t.Errorf("Content-Encoding = %q, want %q", string(encoding), tt.wantEncoding)
				}
			}
		})
	}
}

// TestGzipStaticServeFile_AcceptEncodingParsing 测试 Accept-Encoding 头解析
func TestGzipStaticServeFile_AcceptEncodingParsing(t *testing.T) {
	tmpDir := t.TempDir()

	// 创建测试文件
	brFile := filepath.Join(tmpDir, "test.html.br")
	if err := os.WriteFile(brFile, []byte("br content"), 0644); err != nil {
		t.Fatalf("创建 .br 文件失败: %v", err)
	}

	g := NewGzipStatic(true, tmpDir, nil)

	tests := []struct {
		name           string
		acceptEncoding string
		wantSupported  bool
	}{
		{
			name:           "包含 br",
			acceptEncoding: "gzip, deflate, br",
			wantSupported:  true,
		},
		{
			name:           "br 在中间",
			acceptEncoding: "gzip, br, deflate",
			wantSupported:  true,
		},
		{
			name:           "br 在最后",
			acceptEncoding: "gzip, deflate,br",
			wantSupported:  true,
		},
		{
			name:           "包含空格",
			acceptEncoding: "gzip, br , deflate",
			wantSupported:  true,
		},
		{
			name:           "q-value 支持",
			acceptEncoding: "br;q=0.9, gzip;q=0.8",
			wantSupported:  true,
		},
		{
			name:           "没有 br",
			acceptEncoding: "gzip, deflate",
			wantSupported:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &fasthttp.RequestCtx{}
			ctx.Request.Header.Set("Accept-Encoding", tt.acceptEncoding)

			served := g.ServeFile(ctx, "test.html")

			if served != tt.wantSupported {
				t.Errorf("ServeFile() = %v, want %v", served, tt.wantSupported)
			}
		})
	}
}

// TestGzipStaticServeFile_Disabled 测试禁用时不服务
func TestGzipStaticServeFile_Disabled(t *testing.T) {
	tmpDir := t.TempDir()

	// 创建测试文件
	brFile := filepath.Join(tmpDir, "test.js.br")
	if err := os.WriteFile(brFile, []byte("br content"), 0644); err != nil {
		t.Fatalf("创建 .br 文件失败: %v", err)
	}

	// 禁用的 GzipStatic
	g := NewGzipStatic(false, tmpDir, nil)

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.Set("Accept-Encoding", "br")

	served := g.ServeFile(ctx, "test.js")

	if served {
		t.Error("禁用时不应服务文件")
	}
}

// TestGzipStaticServeFile_InvalidExtension 测试无效扩展名
func TestGzipStaticServeFile_InvalidExtension(t *testing.T) {
	tmpDir := t.TempDir()

	// 创建测试文件
	brFile := filepath.Join(tmpDir, "test.exe.br")
	if err := os.WriteFile(brFile, []byte("br content"), 0644); err != nil {
		t.Fatalf("创建 .br 文件失败: %v", err)
	}

	g := NewGzipStatic(true, tmpDir, nil)

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.Set("Accept-Encoding", "br")

	served := g.ServeFile(ctx, "test.exe")

	if served {
		t.Error("无效扩展名不应服务文件")
	}
}

// TestGzipStaticServeFile_PathTraversal 测试路径遍历防护
func TestGzipStaticServeFile_PathTraversal(t *testing.T) {
	tmpDir := t.TempDir()

	// 创建测试文件
	brFile := filepath.Join(tmpDir, "test.js.br")
	if err := os.WriteFile(brFile, []byte("br content"), 0644); err != nil {
		t.Fatalf("创建 .br 文件失败: %v", err)
	}

	g := NewGzipStatic(true, tmpDir, nil)

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.Set("Accept-Encoding", "br")

	// 尝试路径遍历
	served := g.ServeFile(ctx, "../test.js")

	if served {
		t.Error("路径遍历应被阻止")
	}
}

// TestGzipStaticServeFile_VaryHeader 测试 Vary 头设置
func TestGzipStaticServeFile_VaryHeader(t *testing.T) {
	tmpDir := t.TempDir()

	// 创建测试文件
	brFile := filepath.Join(tmpDir, "test.js.br")
	if err := os.WriteFile(brFile, []byte("br content"), 0644); err != nil {
		t.Fatalf("创建 .br 文件失败: %v", err)
	}

	g := NewGzipStatic(true, tmpDir, nil)

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.Set("Accept-Encoding", "br")

	g.ServeFile(ctx, "test.js")

	vary := ctx.Response.Header.Peek("Vary")
	if string(vary) != "Accept-Encoding" {
		t.Errorf("Vary 头 = %q, want %q", string(vary), "Accept-Encoding")
	}
}

// TestNewGzipStatic_DefaultExtensions 测试默认扩展名
func TestNewGzipStatic_DefaultExtensions(t *testing.T) {
	g := NewGzipStatic(true, "/tmp", nil)

	expected := []string{".html", ".css", ".js", ".json", ".xml", ".svg", ".txt"}
	got := g.Extensions()

	if len(got) != len(expected) {
		t.Errorf("扩展名数量 = %d, want %d", len(got), len(expected))
	}

	for i, ext := range expected {
		if got[i] != ext {
			t.Errorf("扩展名[%d] = %q, want %q", i, got[i], ext)
		}
	}
}

// TestNewGzipStatic_CustomExtensions 测试自定义扩展名
func TestNewGzipStatic_CustomExtensions(t *testing.T) {
	custom := []string{".custom", ".ext"}
	g := NewGzipStatic(true, "/tmp", custom)

	got := g.Extensions()
	if len(got) != 2 || got[0] != ".custom" || got[1] != ".ext" {
		t.Errorf("自定义扩展名 = %v, want %v", got, custom)
	}
}

// TestGzipStatic_Enabled 测试 Enabled 方法
func TestGzipStatic_Enabled(t *testing.T) {
	g1 := NewGzipStatic(true, "/tmp", nil)
	if !g1.Enabled() {
		t.Error("Enabled() = false, want true")
	}

	g2 := NewGzipStatic(false, "/tmp", nil)
	if g2.Enabled() {
		t.Error("Enabled() = true, want false")
	}
}

// TestDefaultExtensions 测试 DefaultExtensions 函数
func TestDefaultExtensions(t *testing.T) {
	expected := []string{".html", ".css", ".js", ".json", ".xml", ".svg", ".txt"}
	got := DefaultExtensions()

	if len(got) != len(expected) {
		t.Errorf("默认扩展名数量 = %d, want %d", len(got), len(expected))
	}

	for i, ext := range expected {
		if got[i] != ext {
			t.Errorf("默认扩展名[%d] = %q, want %q", i, got[i], ext)
		}
	}
}

// TestSupportsEncoding 测试 supportsEncoding 函数
func TestSupportsEncoding(t *testing.T) {
	tests := []struct {
		name           string
		acceptEncoding string
		wantEncoding   string
		wantSupported  bool
	}{
		{
			name:           "支持 br",
			acceptEncoding: "br",
			wantEncoding:   ".br",
			wantSupported:  true,
		},
		{
			name:           "支持 gzip",
			acceptEncoding: "gzip",
			wantEncoding:   ".gz",
			wantSupported:  true,
		},
		{
			name:           "不支持",
			acceptEncoding: "deflate",
			wantEncoding:   ".br",
			wantSupported:  false,
		},
		{
			name:           "空 Accept-Encoding",
			acceptEncoding: "",
			wantEncoding:   ".br",
			wantSupported:  false,
		},
		{
			name:           "br 在中间",
			acceptEncoding: "gzip, br, deflate",
			wantEncoding:   ".br",
			wantSupported:  true,
		},
		{
			name:           "gzip 在中间",
			acceptEncoding: "br, gzip, deflate",
			wantEncoding:   ".gz",
			wantSupported:  true,
		},
		{
			name:           "大小写不敏感",
			acceptEncoding: "BR, GZIP",
			wantEncoding:   ".br",
			wantSupported:  true,
		},
		{
			name:           "未知扩展名",
			acceptEncoding: "br, gzip",
			wantEncoding:   ".unknown",
			wantSupported:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			supported := supportsEncoding([]byte(tt.acceptEncoding), tt.wantEncoding)
			if supported != tt.wantSupported {
				t.Errorf("supportsEncoding(%q, %q) = %v, want %v",
					tt.acceptEncoding, tt.wantEncoding, supported, tt.wantSupported)
			}
		})
	}
}

// TestTryServeFile 测试 TryServeFile 静态方法
func TestTryServeFile(t *testing.T) {
	tmpDir := t.TempDir()

	// 创建测试文件
	brFile := filepath.Join(tmpDir, "test.js.br")
	if err := os.WriteFile(brFile, []byte("br content"), 0644); err != nil {
		t.Fatalf("创建 .br 文件失败: %v", err)
	}

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.Set("Accept-Encoding", "br")

	served := TryServeFile(ctx, tmpDir, "test.js", nil)

	if !served {
		t.Error("TryServeFile() = false, want true")
	}

	encoding := ctx.Response.Header.Peek("Content-Encoding")
	if string(encoding) != "br" {
		t.Errorf("Content-Encoding = %q, want %q", string(encoding), "br")
	}
}

// TestGzipStatic_PrecompressedExtensions 测试预压缩扩展名优先级
func TestGzipStatic_PrecompressedExtensions(t *testing.T) {
	g := NewGzipStatic(true, "/tmp", nil)

	// 验证默认预压缩扩展名顺序
	expected := []string{".br", ".gz"}
	if len(g.precompressedExtensions) != len(expected) {
		t.Errorf("预压缩扩展名数量 = %d, want %d", len(g.precompressedExtensions), len(expected))
	}

	for i, ext := range expected {
		if g.precompressedExtensions[i] != ext {
			t.Errorf("预压缩扩展名[%d] = %q, want %q", i, g.precompressedExtensions[i], ext)
		}
	}
}
