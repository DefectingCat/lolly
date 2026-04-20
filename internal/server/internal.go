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
