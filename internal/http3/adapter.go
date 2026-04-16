// Package http3 提供 HTTP/3 请求适配层。
//
// 该文件实现 fasthttp.RequestHandler 与 http.Handler 之间的适配，
// 使 HTTP/3 服务器能够复用现有的 fasthttp 处理器。
//
// 主要特性：
//
//   - 流式请求体处理：对于大请求体使用流式读取避免内存峰值
//   - 阈值控制：64KB 以下全量读取，以上使用流式处理
//
// 作者：xfy
package http3

import (
	"io"
	"net"
	"net/http"
	"sync"

	"github.com/valyala/fasthttp"
)

const (
	// bodySizeThreshold 是请求体大小阈值，超过此值使用流式处理
	bodySizeThreshold = 64 * 1024 // 64KB
)

// Adapter 将 fasthttp.RequestHandler 适配为 http.Handler。
//
// 由于 quic-go 使用标准库的 http.Handler 接口，
// 而 lolly 使用 fasthttp，需要通过适配层进行转换。
type Adapter struct {
	// ctxPool 用于复用 fasthttp.RequestCtx 对象
	ctxPool sync.Pool

	// bufferPool 用于复用字节缓冲区（流式处理优化）
	bufferPool sync.Pool
}

// NewAdapter 创建新的适配器。
func NewAdapter() *Adapter {
	return &Adapter{
		ctxPool: sync.Pool{
			New: func() interface{} {
				return &fasthttp.RequestCtx{}
			},
		},
		bufferPool: sync.Pool{
			New: func() interface{} {
				buf := make([]byte, 4096) // 4KB 初始缓冲区
				return &buf
			},
		},
	}
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
		// 从池中获取 RequestCtx
		ctx, ok := a.ctxPool.Get().(*fasthttp.RequestCtx)
		if !ok {
			// 如果类型断言失败，创建新的上下文（不应该发生，但为了安全）
			ctx = &fasthttp.RequestCtx{}
		}
		defer a.ctxPool.Put(ctx)

		// 重置 ctx 状态以避免污染
		a.resetContext(ctx)

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

// resetContext 重置 fasthttp.RequestCtx 状态。
//
// 参数：
//   - ctx: 需要重置的上下文
func (a *Adapter) resetContext(ctx *fasthttp.RequestCtx) {
	// 清空请求头
	ctx.Request.Header.DisableNormalizing()
	ctx.Request.Reset()
	ctx.Response.Reset()
	ctx.SetUserValueBytes(nil, nil)
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

	// 设置请求体（使用流式处理优化）
	a.streamRequestBody(r, ctx)

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

// streamRequestBody 流式读取请求体到 fasthttp。
//
// 对于小于等于 64KB 的请求体，直接读取到内存；
// 对于大于 64KB 的请求体，使用流式缓冲区避免内存峰值。
//
// 参数：
//   - r: 标准库 HTTP 请求
//   - ctx: FastHTTP 请求上下文
func (a *Adapter) streamRequestBody(r *http.Request, ctx *fasthttp.RequestCtx) {
	if r.Body == nil || r.Body == http.NoBody {
		return
	}

	defer func() {
		_ = r.Body.Close()
	}()

	// 小请求体（<=64KB）：直接读取到内存
	if r.ContentLength > 0 && r.ContentLength <= bodySizeThreshold {
		body, err := io.ReadAll(r.Body)
		if err == nil {
			ctx.Request.SetBody(body)
		}
		return
	}

	// 大请求体（>64KB 或未知长度）：使用流式缓冲区
	// 如果已知 ContentLength，预分配精确大小的缓冲区
	var body []byte
	if r.ContentLength > 0 {
		body = make([]byte, 0, r.ContentLength)
	}

	// 从 pool 获取缓冲区进行分块读取
	bufPtr, ok := a.bufferPool.Get().(*[]byte)
	if !ok {
		buf := make([]byte, 4096)
		bufPtr = &buf
	}
	defer a.bufferPool.Put(bufPtr)

	buf := *bufPtr

	for {
		n, err := r.Body.Read(buf)
		if n > 0 {
			body = append(body, buf[:n]...)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}
	}

	if len(body) > 0 {
		ctx.Request.SetBody(body)
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
