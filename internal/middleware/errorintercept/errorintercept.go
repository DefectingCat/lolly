// Package errorintercept 提供 HTTP 错误拦截中间件，用于应用自定义错误页面。
//
// 该文件包含错误拦截相关的核心功能，包括：
//   - ErrorIntercept 中间件：拦截 HTTP 错误响应并应用自定义错误页面
//   - 错误状态码检测
//   - 错误页面内容替换
//
// 主要用途：
//
//	在 HTTP 响应返回错误状态码时，自动替换为预加载的自定义错误页面内容。
//
// 注意事项：
//   - 错误页面在启动时预加载，运行时不进行文件 I/O
//   - 支持可选的响应状态码覆盖
//   - 只拦截 4xx 和 5xx 错误状态码
//
// 作者：xfy
package errorintercept

import (
	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/handler"
)

// ErrorIntercept 错误拦截中间件。
//
// 拦截 HTTP 错误响应（4xx 和 5xx），并使用预加载的自定义错误页面内容替换响应。
type ErrorIntercept struct {
	// manager 错误页面管理器，用于获取预加载的错误页面内容
	manager *handler.ErrorPageManager
}

// New 创建错误拦截中间件。
//
// 参数：
//   - manager: 错误页面管理器（已预加载错误页面）
//
// 返回值：
//   - *ErrorIntercept: 创建的中间件实例
//
// 使用示例：
//
//	interceptor := errorintercept.New(errorPageManager)
func New(manager *handler.ErrorPageManager) *ErrorIntercept {
	return &ErrorIntercept{
		manager: manager,
	}
}

// Name 返回中间件名称。
//
// 返回值：
//   - string: 中间件名称
func (ei *ErrorIntercept) Name() string {
	return "ErrorIntercept"
}

// Process 实现中间件接口。
//
// 拦截错误状态码响应并应用自定义错误页面。
//
// 参数：
//   - next: 下一个请求处理器
//
// 返回值：
//   - fasthttp.RequestHandler: 包装后的请求处理器
func (ei *ErrorIntercept) Process(next fasthttp.RequestHandler) fasthttp.RequestHandler {
	// 如果没有配置错误页面，直接返回下一个处理器
	if ei.manager == nil || !ei.manager.IsConfigured() {
		return next
	}

	return func(ctx *fasthttp.RequestCtx) {
		// 先执行下一个处理器
		next(ctx)

		// 检查是否是错误状态码（4xx 或 5xx）
		statusCode := ctx.Response.StatusCode()
		if !isErrorStatusCode(statusCode) {
			return
		}

		// 查找对应的错误页面
		content, found, responseCode := ei.manager.GetPage(statusCode)
		if !found {
			return
		}

		// 替换响应内容为自定义错误页面
		ctx.Response.SetBody(content)
		ctx.Response.Header.SetContentType("text/html; charset=utf-8")

		// 如果配置了响应状态码覆盖，使用覆盖值
		if responseCode != statusCode {
			ctx.Response.SetStatusCode(responseCode)
		}
	}
}

// isErrorStatusCode 检查状态码是否为错误状态码。
//
// 参数：
//   - code: HTTP 状态码
//
// 返回值：
//   - bool: 是否为错误状态码（4xx 或 5xx）
func isErrorStatusCode(code int) bool {
	return code >= 400 && code < 600
}

// GetManager 返回错误页面管理器。
//
// 返回值：
//   - *handler.ErrorPageManager: 错误页面管理器
func (ei *ErrorIntercept) GetManager() *handler.ErrorPageManager {
	return ei.manager
}
