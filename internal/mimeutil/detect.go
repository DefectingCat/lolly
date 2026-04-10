// Package mimeutil 提供 MIME 类型检测工具。
package mimeutil

import (
	"mime"
	"path/filepath"
	"strings"
)

// mimeOverrides 补充 Go 标准库缺失或错误的 MIME 类型映射。
// 使用包本地映射而非 mime.AddExtensionType，避免全局副作用。
//
// 注意: 部分扩展名 Go 返回错误类型而非缺失:
//   - .otf: Go 映射到 OpenDocument 公式模板，应为字体格式
//   - .webm: Go 返回 audio/webm，但 webm 可包含视频
var mimeOverrides = map[string]string{
	".eot":         "application/vnd.ms-fontobject", // 缺失
	".otf":         "font/otf",                      // Go 返回错误类型
	".webmanifest": "application/manifest+json",     // 缺失
	".map":         "application/json",              // 缺失
	".webm":        "video/webm",                    // Go 返回 audio/webm
	// 注意: Go 1.26.2+ 已正确支持 .mjs, .avif, .woff, .woff2
}

// DetectContentType 检测文件的 MIME 类型。
//
// 优先使用包本地映射，回退到 Go 标准库 mime.TypeByExtension。
// 自动处理扩展名大小写问题。
//
// 参数:
//   - filePath: 文件路径
//
// 返回值:
//   - string: MIME 类型，未知类型返回空字符串
func DetectContentType(filePath string) string {
	ext := strings.ToLower(filepath.Ext(filePath))
	if mime, ok := mimeOverrides[ext]; ok {
		return mime
	}
	return mime.TypeByExtension(ext)
}
