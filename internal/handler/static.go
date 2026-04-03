package handler

import (
	"mime"
	"os"
	"path/filepath"
	"strings"

	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/cache"
)

// StaticHandler 静态文件处理器
type StaticHandler struct {
	root        string
	index       []string
	useSendfile bool          // 是否启用零拷贝传输
	fileCache   *cache.FileCache // 文件缓存（可选）
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
