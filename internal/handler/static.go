// Package handler 提供 HTTP 请求处理器，包括路由、静态文件服务和零拷贝传输。
//
// 该文件包含静态文件服务相关的核心逻辑，包括：
//   - 静态文件请求处理
//   - 目录索引文件支持
//   - 文件缓存和零拷贝传输优化
//   - 预压缩文件支持
//
// 主要用途：
//
//	用于提供静态文件服务，支持缓存和零拷贝传输优化。
//
// 注意事项：
//   - 自动处理目录遍历攻击防护
//   - 支持多索引文件（如 index.html、index.htm）
//   - 支持预压缩 .gz 文件
//
// 作者：xfy
package handler

import (
	"mime"
	"os"
	"path/filepath"
	"strings"

	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/cache"
	"rua.plus/lolly/internal/middleware/compression"
)

// StaticHandler 静态文件处理器。
//
// 提供静态文件服务，支持目录索引、文件缓存和零拷贝传输。
type StaticHandler struct {
	// root 静态文件根目录
	root string

	// index 索引文件列表，当请求目录时依次查找
	index []string

	// useSendfile 是否启用零拷贝传输（大文件优化）
	useSendfile bool

	// fileCache 文件缓存实例（可选）
	fileCache *cache.FileCache

	// gzipStatic 预压缩文件支持（可选）
	gzipStatic *compression.GzipStatic
}

// NewStaticHandler 创建静态文件处理器
func NewStaticHandler(root string, index []string, useSendfile bool) *StaticHandler {
	return &StaticHandler{
		root:        root,
		index:       index,
		useSendfile: useSendfile,
	}
}

// SetFileCache 设置文件缓存
func (h *StaticHandler) SetFileCache(fc *cache.FileCache) {
	h.fileCache = fc
}

// SetGzipStatic 设置预压缩文件支持
func (h *StaticHandler) SetGzipStatic(enabled bool, extensions []string) {
	if enabled {
		h.gzipStatic = compression.NewGzipStatic(true, h.root, extensions)
	}
}

// Handle 处理静态文件请求
func (h *StaticHandler) Handle(ctx *fasthttp.RequestCtx) {
	path := string(ctx.Path())

	// 安全检查：防止目录遍历
	if strings.Contains(path, "..") {
		ctx.Error("Forbidden", fasthttp.StatusForbidden)
		return
	}

	// 拼接文件路径
	filePath := filepath.Join(h.root, path)

	// 检查文件/目录是否存在
	info, err := os.Stat(filePath)
	if err != nil {
		ctx.Error("Not Found", fasthttp.StatusNotFound)
		return
	}

	// 如果是目录，尝试索引文件
	if info.IsDir() {
		for _, idx := range h.index {
			idxPath := filepath.Join(filePath, idx)
			if idxInfo, err := os.Stat(idxPath); err == nil && !idxInfo.IsDir() {
				h.serveFile(ctx, idxPath, idxInfo)
				return
			}
		}
		ctx.Error("Forbidden", fasthttp.StatusForbidden)
		return
	}

	// 直接返回文件
	h.serveFile(ctx, filePath, info)
}

// serveFile 提供文件服务，支持缓存和零拷贝传输
func (h *StaticHandler) serveFile(ctx *fasthttp.RequestCtx, filePath string, info os.FileInfo) {
	// 尝试发送预压缩文件
	if h.gzipStatic != nil {
		relPath := strings.TrimPrefix(filePath, h.root)
		if h.gzipStatic.ServeFile(ctx, relPath) {
			return // 预压缩文件已发送
		}
	}

	// 尝试从缓存获取
	if h.fileCache != nil {
		if entry, ok := h.fileCache.Get(filePath); ok {
			// 检查文件是否被修改
			if entry.ModTime.Equal(info.ModTime()) {
				// 缓存命中且文件未修改
				ctx.Response.SetBody(entry.Data)
				ctx.Response.Header.SetContentType(mime.TypeByExtension(filepath.Ext(filePath)))
				return
			}
			// 文件已修改，删除旧缓存
			h.fileCache.Delete(filePath)
		}
	}

	// 大文件使用零拷贝传输
	if h.useSendfile && info.Size() >= MinSendfileSize {
		file, err := os.Open(filePath)
		if err == nil {
			defer file.Close()
			if err := SendFile(ctx, file, 0, info.Size()); err == nil {
				return
			}
			// sendfile 失败，fallback 到 ServeFile
		}
	}

	// 读取文件内容
	data, err := os.ReadFile(filePath)
	if err != nil {
		ctx.Error("Internal Server Error", fasthttp.StatusInternalServerError)
		return
	}

	// 存入缓存（仅对小文件缓存）
	if h.fileCache != nil && info.Size() < 1024*1024 { // < 1MB
		h.fileCache.Set(filePath, data, info.Size(), info.ModTime())
	}

	ctx.Response.SetBody(data)
	ctx.Response.Header.SetContentType(mime.TypeByExtension(filepath.Ext(filePath)))
}
