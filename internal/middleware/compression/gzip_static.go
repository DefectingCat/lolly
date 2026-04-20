// Package compression 提供预压缩静态文件支持。
//
// 该文件实现预压缩文件的检测和发送，避免实时压缩开销。
//
// 主要功能：
//   - 检测 .gz 和 .br 预压缩文件
//   - 按优先级选择编码（brotli > gzip）
//   - 客户端 Accept-Encoding 协商
//   - 扩展名过滤，仅对文本类资源启用预压缩
//
// 主要用途：
//
//	用于发送预压缩的静态资源文件，减少服务器 CPU 开销，提升响应速度。
//
// 使用场景：
//   - 静态资源预先压缩（如 CSS、JS、HTML 文件）
//   - 构建时生成 .gz 或 .br 文件
//   - 运行时直接发送预压缩文件，避免实时压缩
//
// 注意事项：
//   - 需要客户端支持对应的 Content-Encoding
//   - 预压缩文件需要与源文件放在同一目录
//
// 作者：xfy
package compression

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/mimeutil"
)

// GzipStatic 预压缩文件支持中间件。
//
// 检查是否存在预压缩的 .gz 或 .br 文件，如果存在且客户端支持对应编码，
// 则直接发送，避免实时压缩的 CPU 开销。
type GzipStatic struct {
	// root 静态文件根目录路径
	root string
	// precompressedExtensions 预压缩文件扩展名列表（如 .br, .gz）
	precompressedExtensions []string
	// extensions 支持预压缩的源文件扩展名列表（如 .html, .css, .js）
	extensions []string
	// enabled 是否启用预压缩支持
	enabled bool
}

// NewGzipStatic 创建预压缩文件处理器。
//
// 根据配置创建预压缩文件处理器实例，设置默认扩展名列表。
//
// 参数：
//   - enabled: 是否启用预压缩支持
//   - root: 静态文件根目录路径
//   - extensions: 支持预压缩的文件扩展名列表，为空则使用默认值
//
// 返回值：
//   - *GzipStatic: 创建的预压缩文件处理器
func NewGzipStatic(enabled bool, root string, extensions []string) *GzipStatic {
	if len(extensions) == 0 {
		extensions = []string{".html", ".css", ".js", ".json", ".xml", ".svg", ".txt"}
	}
	return &GzipStatic{
		enabled:                 enabled,
		root:                    root,
		extensions:              extensions,
		precompressedExtensions: []string{".br", ".gz"},
	}
}

// ServeFile 发送预压缩文件（如果存在）。
//
// 检查是否存在对应的 .br 或 .gz 文件，按优先级（br > gzip）选择：
// 1. 如果客户端支持 br 且 .br 文件存在，返回 .br 文件
// 2. 否则如果客户端支持 gzip 且 .gz 文件存在，返回 .gz 文件
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

	// 检查文件扩展名
	if !g.matchExtension(filePath) {
		return false
	}

	// 安全检查：防止目录遍历
	if strings.Contains(filePath, "..") {
		return false
	}

	// 获取 Accept-Encoding 头
	acceptEncoding := ctx.Request.Header.Peek("Accept-Encoding")

	// 按优先级检查预压缩文件
	for _, ext := range g.precompressedExtensions {
		// 检查客户端是否支持该编码
		if !supportsEncoding(acceptEncoding, ext) {
			continue
		}

		// 构建预压缩文件路径
		compressedPath := filePath + ext
		fullPath := filepath.Join(g.root, compressedPath)

		// 检查文件是否存在
		if _, err := os.Stat(fullPath); err != nil {
			continue
		}

		// 设置 Content-Encoding 头
		switch ext {
		case ".br":
			ctx.Response.Header.Set("Content-Encoding", "br")
		case ".gz":
			ctx.Response.Header.Set("Content-Encoding", "gzip")
		}
		ctx.Response.Header.Set("Vary", "Accept-Encoding")
		// 设置原始文件的 Content-Type
		// filePath 是原始文件路径 (如 "test.js")，直接使用即可
		ctx.Response.Header.SetContentType(mimeutil.DetectContentType(filePath))

		fasthttp.ServeFile(ctx, fullPath)
		return true
	}

	return false
}

// matchExtension 检查文件扩展名是否在支持的预压缩扩展名列表中。
//
// 参数：
//   - filePath: 文件路径
//
// 返回值：
//   - bool: true 表示文件扩展名匹配支持列表
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
//
// 返回值：
//   - bool: true 表示已启用预压缩支持
func (g *GzipStatic) Enabled() bool {
	return g.enabled
}

// Extensions 返回支持的源文件扩展名列表。
//
// 返回值：
//   - []string: 支持预压缩的源文件扩展名列表
func (g *GzipStatic) Extensions() []string {
	return g.extensions
}

// supportsEncoding 检查客户端是否支持指定编码。
//
// 通过检查 Accept-Encoding 请求头中是否包含指定编码名称。
// 忽略 q-value（质量因子），固定优先级由遍历顺序决定。
//
// 参数：
//   - acceptEncoding: Accept-Encoding 请求头值
//   - ext: 预压缩文件扩展名（如 ".br"、".gz"）
//
// 返回值：
//   - bool: true 表示客户端支持该编码
func supportsEncoding(acceptEncoding []byte, ext string) bool {
	if len(acceptEncoding) == 0 {
		return false
	}
	enc := strings.ToLower(string(acceptEncoding))

	switch ext {
	case ".br":
		return strings.Contains(enc, "br")
	case ".gz":
		return strings.Contains(enc, "gzip")
	default:
		return false
	}
}
