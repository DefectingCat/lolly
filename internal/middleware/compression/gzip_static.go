// Package compression 提供 gzip_static 预压缩文件支持。
//
// 该文件实现预压缩文件的检测和发送，避免实时压缩开销。
//
// 主要用途：
//
//	用于发送预压缩的 .gz 文件，减少服务器 CPU 开销，提升响应速度。
//
// 使用场景：
//   - 静态资源预先压缩（如 CSS、JS、HTML 文件）
//   - 构建时生成 .gz 文件
//   - 运行时直接发送预压缩文件
//
// 作者：xfy
package compression

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"

	"github.com/valyala/fasthttp"
)

// GzipStatic 预压缩文件支持。
//
// 检查是否存在预压缩的 .gz 文件，如果存在则直接发送，
// 避免实时压缩的 CPU 开销。
type GzipStatic struct {
	// enabled 是否启用
	enabled bool

	// root 静态文件根目录
	root string

	// extensions 支持的扩展名
	extensions []string
}

// NewGzipStatic 创建预压缩文件处理器。
//
// 参数：
//   - enabled: 是否启用预压缩支持
//   - root: 静态文件根目录
//   - extensions: 支持预压缩的文件扩展名，为空则使用默认值
func NewGzipStatic(enabled bool, root string, extensions []string) *GzipStatic {
	if len(extensions) == 0 {
		extensions = []string{".html", ".css", ".js", ".json", ".xml", ".svg", ".txt"}
	}
	return &GzipStatic{
		enabled:    enabled,
		root:       root,
		extensions: extensions,
	}
}

// ServeFile 发送预压缩文件（如果存在）。
//
// 检查是否存在对应的 .gz 文件，如果存在且客户端支持 gzip，
// 则发送预压缩文件。
//
// 参数：
//   - ctx: FastHTTP 请求上下文
//   - filePath: 请求的文件路径
//
// 返回值：
//   - bool: true 表示已发送预压缩文件，false 表示未发送
func (g *GzipStatic) ServeFile(ctx *fasthttp.RequestCtx, filePath string) bool {
	if !g.enabled {
		return false
	}

	// 检查客户端是否支持 gzip
	acceptEncoding := ctx.Request.Header.Peek("Accept-Encoding")
	if !bytes.Contains(acceptEncoding, []byte("gzip")) {
		return false
	}

	// 检查文件扩展名
	if !g.matchExtension(filePath) {
		return false
	}

	// 检查预压缩文件是否存在
	gzPath := filePath + ".gz"
	fullGzPath := filepath.Join(g.root, gzPath)

	// 安全检查：防止目录遍历
	if strings.Contains(gzPath, "..") {
		return false
	}

	// 检查文件是否存在
	if _, err := os.Stat(fullGzPath); err != nil {
		return false
	}

	// 发送预压缩文件
	ctx.Response.Header.Set("Content-Encoding", "gzip")
	ctx.Response.Header.Set("Vary", "Accept-Encoding")
	fasthttp.ServeFile(ctx, fullGzPath)
	return true
}

// TryServeFile 尝试发送预压缩文件的静态方法。
//
// 用于在静态文件处理器中调用。
//
// 参数：
//   - ctx: FastHTTP 请求上下文
//   - root: 静态文件根目录
//   - filePath: 请求的文件路径
//   - extensions: 支持的扩展名
//
// 返回值：
//   - bool: true 表示已发送预压缩文件
func TryServeFile(ctx *fasthttp.RequestCtx, root, filePath string, extensions []string) bool {
	g := NewGzipStatic(true, root, extensions)
	return g.ServeFile(ctx, filePath)
}

// matchExtension 检查文件扩展名是否匹配。
func (g *GzipStatic) matchExtension(filePath string) bool {
	ext := strings.ToLower(filepath.Ext(filePath))
	for _, e := range g.extensions {
		if strings.ToLower(e) == ext {
			return true
		}
	}
	return false
}

// Enabled 返回是否启用预压缩。
func (g *GzipStatic) Enabled() bool {
	return g.enabled
}

// Extensions 返回支持的扩展名列表。
func (g *GzipStatic) Extensions() []string {
	return g.extensions
}

// DefaultExtensions 返回默认支持的扩展名。
func DefaultExtensions() []string {
	return []string{".html", ".css", ".js", ".json", ".xml", ".svg", ".txt"}
}
