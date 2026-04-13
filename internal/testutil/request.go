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
