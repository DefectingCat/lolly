// Package http3 提供 HTTP/3 请求适配层。
//
// 该文件实现 fasthttp.RequestHandler 与 http.Handler 之间的适配，
// 使 HTTP/3 服务器能够复用现有的 fasthttp 处理器。
//
// 主要用途：
//
//	将 quic-go 的 http.Handler 接口适配为 fasthttp.RequestHandler。
//
// 作者：xfy
package http3

import (
	"io"
	"net"
	"net/http"

	"github.com/valyala/fasthttp"
)

// Adapter 将 fasthttp.RequestHandler 适配为 http.Handler。
//
// 由于 quic-go 使用标准库的 http.Handler 接口，
// 而 lolly 使用 fasthttp，需要通过适配层进行转换。
type Adapter struct {
}

// NewAdapter 创建新的适配器。
func NewAdapter() *Adapter {
	return &Adapter{}
}

// Wrap 包装 fasthttp handler 为 http.Handler。
//
// 将 http.Request 转换为 fasthttp.RequestCtx，
// 调用 fasthttp handler，然后将响应写回 http.ResponseWriter。
//
// 参数：
//   - handler: fasthttp 请求处理器
//
// 返回值：
//   - http.Handler: 标准库兼容的 HTTP 处理器
func (a *Adapter) Wrap(handler fasthttp.RequestHandler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 直接创建 RequestCtx
		ctx := &fasthttp.RequestCtx{}

		// 初始化 ctx（fasthttp 的 RequestCtx 需要 Init 方法）
		ctx.Init(&fasthttp.Request{}, nil, nil)

		// 转换请求
		a.convertRequest(r, ctx)

		// 设置 ResponseWriter 用于后续写入
		ctx.SetUserValue("http3_response_writer", w)

		// 调用 fasthttp handler
		handler(ctx)

		// 转换响应
		a.convertResponse(ctx, w)
	})
}

// convertRequest 将 net/http.Request 转换为 fasthttp.RequestCtx。
//
// 参数：
//   - r: 标准库 HTTP 请求
//   - ctx: FastHTTP 请求上下文
func (a *Adapter) convertRequest(r *http.Request, ctx *fasthttp.RequestCtx) {
	// 设置方法
	ctx.Request.Header.SetMethod(r.Method)

	// 设置 URI
	uri := r.URL.Path
	if r.URL.RawQuery != "" {
		uri += "?" + r.URL.RawQuery
	}
	ctx.Request.SetRequestURI(uri)

	// 设置 Host 头
	ctx.Request.Header.SetHost(r.Host)

	// 复制头部
	for k, v := range r.Header {
		for _, vv := range v {
			ctx.Request.Header.Add(k, vv)
		}
	}

	// 设置请求体
	if r.Body != nil {
		body, err := io.ReadAll(r.Body)
		if err == nil {
			ctx.Request.SetBody(body)
		}
		_ = r.Body.Close()
	}

	// 设置远程地址
	if r.RemoteAddr != "" {
		if addr, err := net.ResolveTCPAddr("tcp", r.RemoteAddr); err == nil {
			ctx.SetRemoteAddr(addr)
		}
	}

	// 设置协议版本
	ctx.Request.Header.SetProtocol("HTTP/3")
}

// convertResponse 将 fasthttp.RequestCtx 响应写入 http.ResponseWriter。
//
// 参数：
//   - ctx: FastHTTP 请求上下文
//   - w: 标准库 ResponseWriter
func (a *Adapter) convertResponse(ctx *fasthttp.RequestCtx, w http.ResponseWriter) {
	// 设置状态码
	statusCode := ctx.Response.StatusCode()
	if statusCode == 0 {
		statusCode = 200
	}

	// 复制响应头
	for k, v := range ctx.Response.Header.All() {
		w.Header().Add(string(k), string(v))
	}

	// 写入状态码
	w.WriteHeader(statusCode)

	// 写入响应体
	body := ctx.Response.Body()
	if len(body) > 0 {
		_, _ = w.Write(body)
	}
}

// WrapHandler 包装特定的 fasthttp handler。
//
// 返回一个可以直接用于 http3.Server 的 http.Handler。
//
// 参数：
//   - handler: fasthttp 请求处理器
//
// 返回值：
//   - http.Handler: 标准库兼容的处理器
func (a *Adapter) WrapHandler(handler fasthttp.RequestHandler) http.Handler {
	return a.Wrap(handler)
}
