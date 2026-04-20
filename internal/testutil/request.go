// Package testutil 提供测试辅助工具函数。
//
// 该文件包含请求上下文创建相关的辅助函数，用于单元测试：
//   - 创建测试用的 fasthttp.RequestCtx
//   - 支持 method、path、body、header 配置
//
// 主要用途：
//
//	简化单元测试中请求上下文的创建，避免重复代码。
//
// 注意事项：
//   - 仅用于测试，不应在生产代码中使用
//
// 作者：xfy
package testutil

import "github.com/valyala/fasthttp"

// NewRequestCtx 创建测试用的请求上下文
func NewRequestCtx(method, path string) *fasthttp.RequestCtx {
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod(method)
	ctx.Request.SetRequestURI(path)
	return ctx
}

// NewRequestCtxWithBody 创建带 body 的测试请求上下文
func NewRequestCtxWithBody(method, path, body string) *fasthttp.RequestCtx {
	ctx := NewRequestCtx(method, path)
	ctx.Request.SetBodyString(body)
	return ctx
}

// NewRequestCtxWithHeader 创建带 header 的测试请求上下文
func NewRequestCtxWithHeader(method, path string, headers map[string]string) *fasthttp.RequestCtx {
	ctx := NewRequestCtx(method, path)
	for k, v := range headers {
		ctx.Request.Header.Set(k, v)
	}
	return ctx
}
