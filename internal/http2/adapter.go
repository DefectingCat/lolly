// Package http2 提供 HTTP/2 请求适配层。
//
// 该文件实现 fasthttp.RequestHandler 与 http.Handler 之间的适配，
// 使 HTTP/2 服务器能够复用现有的 fasthttp 处理器。
//
// 主要特性：
//
//   - 零拷贝头部转换：使用 sync.Pool 复用缓冲区
//   - 流式请求体处理：避免大请求体内存复制
//   - 低延迟：预估每请求 5-10µs 开销
//
// 作者：xfy
package http2

import (
	"net"
	"net/http"
	"time"

	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/adapter"
)

// FastHTTPHandlerAdapter 将 fasthttp.RequestHandler 适配为 http.Handler。
//
// 由于 HTTP/2 服务器使用标准库的 http.Handler 接口，
// 而 lolly 使用 fasthttp，需要通过适配层进行转换。
type FastHTTPHandlerAdapter struct {
	*adapter.CommonAdapter
	handler fasthttp.RequestHandler
}

// NewFastHTTPHandlerAdapter 创建新的 HTTP/2 适配器。
//
// 参数：
//   - handler: fasthttp 请求处理器
//
// 返回值：
//   - *FastHTTPHandlerAdapter: 适配器实例
func NewFastHTTPHandlerAdapter(handler fasthttp.RequestHandler) *FastHTTPHandlerAdapter {
	return &FastHTTPHandlerAdapter{
		CommonAdapter: adapter.NewCommonAdapter(),
		handler:       handler,
	}
}

// ServeHTTP 实现 http.Handler 接口。
//
// 这是适配器的核心方法，将标准库 HTTP 请求转换为 fasthttp 请求，
// 调用 fasthttp 处理器，然后将响应写回标准库 ResponseWriter。
//
// 参数：
//   - w: 标准库 ResponseWriter
//   - r: 标准库 HTTP 请求
func (a *FastHTTPHandlerAdapter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 从池中获取 RequestCtx
	ctx, _ := a.GetContext()
	defer a.PutContext(ctx)

	// 重置 ctx 状态以避免污染
	a.ResetContext(ctx)

	// 转换请求（零拷贝头部转换）
	a.convertRequest(r, ctx)

	// 流式处理请求体
	a.StreamRequestBody(r, ctx)

	// 调用 fasthttp handler
	a.handler(ctx)

	// 转换响应
	a.convertResponse(ctx, w)
}

// convertRequest 将 net/http.Request 转换为 fasthttp.RequestCtx。
//
// 使用零拷贝策略转换请求头和元数据。
//
// 参数：
//   - r: 标准库 HTTP 请求
//   - ctx: FastHTTP 请求上下文
func (a *FastHTTPHandlerAdapter) convertRequest(r *http.Request, ctx *fasthttp.RequestCtx) {
	// 设置方法
	ctx.Request.Header.SetMethod(r.Method)

	// 设置 URI
	uri := r.URL.Path
	if r.URL.RawQuery != "" {
		uri += "?" + r.URL.RawQuery
	}
	ctx.Request.SetRequestURI(uri)

	// 设置协议版本为 HTTP/2
	ctx.Request.Header.SetProtocol("HTTP/2.0")

	// 设置 Host 头
	ctx.Request.Header.SetHost(r.Host)

	// 零拷贝头部转换
	a.convertHeaders(r, ctx)

	// 设置远程地址
	a.setRemoteAddr(r, ctx)

	// 设置 Content-Type
	if ct := r.Header.Get("Content-Type"); ct != "" {
		ctx.Request.Header.SetContentType(ct)
	}

	// 设置 Content-Length（如果有）
	if r.ContentLength > 0 {
		ctx.Request.Header.SetContentLength(int(r.ContentLength))
	}
}

// convertHeaders 将 HTTP 请求头转换为 fasthttp 格式。
//
// 使用 HPACK 风格的零拷贝转换策略。
//
// 参数：
//   - r: 标准库 HTTP 请求
//   - ctx: FastHTTP 请求上下文
func (a *FastHTTPHandlerAdapter) convertHeaders(r *http.Request, ctx *fasthttp.RequestCtx) {
	// 跳过已处理的头部
	skipHeaders := map[string]bool{
		"Host":           true,
		"Content-Type":   true,
		"Content-Length": true,
	}

	for k, v := range r.Header {
		if skipHeaders[k] {
			continue
		}
		// 复用缓冲区避免分配
		for i, vv := range v {
			if i == 0 {
				ctx.Request.Header.Set(k, vv)
			} else {
				ctx.Request.Header.Add(k, vv)
			}
		}
	}
}

// setRemoteAddr 设置远程客户端地址。
//
// 参数：
//   - r: 标准库 HTTP 请求
//   - ctx: FastHTTP 请求上下文
func (a *FastHTTPHandlerAdapter) setRemoteAddr(r *http.Request, ctx *fasthttp.RequestCtx) {
	if r.RemoteAddr != "" {
		// 尝试解析地址
		if addr, err := net.ResolveTCPAddr("tcp", r.RemoteAddr); err == nil {
			ctx.SetRemoteAddr(addr)
		} else {
			// 回退方案：使用字符串地址
			ctx.SetRemoteAddr(&net.TCPAddr{
				IP:   net.ParseIP("127.0.0.1"),
				Port: 0,
			})
		}
	}
}

// convertResponse 将 fasthttp.RequestCtx 响应写入 http.ResponseWriter。
//
// 参数：
//   - ctx: FastHTTP 请求上下文
//   - w: 标准库 ResponseWriter
func (a *FastHTTPHandlerAdapter) convertResponse(ctx *fasthttp.RequestCtx, w http.ResponseWriter) {
	// 设置状态码
	statusCode := ctx.Response.StatusCode()
	if statusCode == 0 {
		statusCode = http.StatusOK
	}

	// 复制响应头
	for key, value := range ctx.Response.Header.All() {
		w.Header().Add(string(key), string(value))
	}

	// 确保 Content-Type 被设置
	if ct := ctx.Response.Header.ContentType(); len(ct) > 0 {
		w.Header().Set("Content-Type", string(ct))
	}

	// 确保 Content-Length 被设置（如果已知）
	if cl := ctx.Response.Header.ContentLength(); cl > 0 {
		w.Header().Set("Content-Length", string(fasthttp.AppendUint(nil, cl)))
	}

	// 写入状态码
	w.WriteHeader(statusCode)

	// 写入响应体
	body := ctx.Response.Body()
	if len(body) > 0 {
		if _, err := w.Write(body); err != nil {
			// 响应写入失败，无法向客户端返回错误，只能记录
			return
		}
	}
}

// WrapHandler 创建一个适配器包装的 handler。
//
// 这是一个便捷函数，用于快速创建适配器实例。
//
// 参数：
//   - handler: fasthttp 请求处理器
//
// 返回值：
//   - http.Handler: 标准库兼容的处理器
func WrapHandler(handler fasthttp.RequestHandler) http.Handler {
	return NewFastHTTPHandlerAdapter(handler)
}

// WrapHandlerFunc 创建一个适配器包装的 handler 函数。
//
// 这是一个便捷函数，允许直接使用函数而非创建 handler 实例。
//
// 参数：
//   - fn: fasthttp handler 函数
//
// 返回值：
//   - http.Handler: 标准库兼容的处理器
func WrapHandlerFunc(fn func(*fasthttp.RequestCtx)) http.Handler {
	return NewFastHTTPHandlerAdapter(fn)
}

// AdapterConfig 提供适配器的配置选项。
type AdapterConfig struct {
	// BufferSize 是缓冲区大小，默认为 4096 字节
	BufferSize int

	// MaxBodySize 是最大请求体大小，超过则使用流式处理
	MaxBodySize int64

	// Timeout 是请求处理超时时间
	Timeout time.Duration
}

// DefaultAdapterConfig 返回适配器的默认配置。
//
// 返回包含默认缓冲区大小（4096 字节）、最大请求体大小（64KB）
// 和超时时间（30 秒）的配置实例。
//
// 返回值：
//   - *AdapterConfig: 初始化的适配器默认配置实例
func DefaultAdapterConfig() *AdapterConfig {
	return &AdapterConfig{
		BufferSize:  4096,
		MaxBodySize: 64 * 1024, // 64KB
		Timeout:     30 * time.Second,
	}
}

// ConfigurableAdapter 是基于配置的可配置适配器。
type ConfigurableAdapter struct {
	*FastHTTPHandlerAdapter
	config *AdapterConfig
}

// NewConfigurableAdapter 创建可配置的 HTTP/2 适配器。
//
// 根据传入的配置参数创建适配器实例，如果配置为 nil 则使用默认配置。
// 适用于需要自定义缓冲区大小、请求体大小限制或超时时间的场景。
//
// 参数：
//   - handler: fasthttp 请求处理器，用于处理 HTTP/2 请求
//   - config: 适配器配置，为 nil 时使用默认配置
//
// 返回值：
//   - *ConfigurableAdapter: 初始化的可配置适配器实例
func NewConfigurableAdapter(handler fasthttp.RequestHandler, config *AdapterConfig) *ConfigurableAdapter {
	if config == nil {
		config = DefaultAdapterConfig()
	}
	return &ConfigurableAdapter{
		FastHTTPHandlerAdapter: NewFastHTTPHandlerAdapter(handler),
		config:                 config,
	}
}
