// Package server 提供 HTTP 服务器的核心实现，支持单服务器、虚拟主机和多服务器三种运行模式。
//
// 包含服务器内部工具函数，用于处理服务器运行时逻辑。
//
// 作者：xfy
package server

import (
	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/utils"
)

const (
	// InternalRedirectKey 内部重定向标记
	InternalRedirectKey = utils.InternalRedirectKey
)

// SetInternalRedirect 标记请求为内部重定向
func SetInternalRedirect(ctx *fasthttp.RequestCtx, targetPath string) {
	utils.SetInternalRedirect(ctx, targetPath)
}

// IsInternalRedirect 检查是否为内部重定向
func IsInternalRedirect(ctx *fasthttp.RequestCtx) bool {
	return utils.IsInternalRedirect(ctx)
}

// GetInternalRedirectPath 获取内部重定向目标路径
func GetInternalRedirectPath(ctx *fasthttp.RequestCtx) string {
	return utils.GetInternalRedirectPath(ctx)
}
