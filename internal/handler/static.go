package handler

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/valyala/fasthttp"
)

// StaticHandler 静态文件处理器
type StaticHandler struct {
	root  string
	index []string
}

// NewStaticHandler 创建静态文件处理器
func NewStaticHandler(root string, index []string) *StaticHandler {
	return &StaticHandler{root: root, index: index}
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
			if _, err := os.Stat(idxPath); err == nil {
				fasthttp.ServeFile(ctx, idxPath)
				return
			}
		}
		ctx.Error("Forbidden", fasthttp.StatusForbidden)
		return
	}

	// 直接返回文件
	fasthttp.ServeFile(ctx, filePath)
}
