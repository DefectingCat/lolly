package utils

import "github.com/valyala/fasthttp"

const (
	// InternalRedirectKey 内部重定向标记
	InternalRedirectKey = "__internal_redirect__"
)

// SetInternalRedirect 标记请求为内部重定向
func SetInternalRedirect(ctx *fasthttp.RequestCtx, targetPath string) {
	ctx.SetUserValue(InternalRedirectKey, targetPath)
}

// IsInternalRedirect 检查是否为内部重定向
func IsInternalRedirect(ctx *fasthttp.RequestCtx) bool {
	return ctx.UserValue(InternalRedirectKey) != nil
}

// GetInternalRedirectPath 获取内部重定向目标路径
func GetInternalRedirectPath(ctx *fasthttp.RequestCtx) string {
	if v := ctx.UserValue(InternalRedirectKey); v != nil {
		if path, ok := v.(string); ok {
			return path
		}
	}
	return ""
}
