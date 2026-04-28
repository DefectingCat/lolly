// Package adapter 提供 HTTP/2 和 HTTP/3 适配器的共享组件。
//
// 该包提取了两个适配器中通用的功能，避免代码重复：
//
//   - 共享的 bufferPool singleton（零拷贝优化）
//   - 统一的请求体处理阈值
//   - 通用的上下文重置逻辑
//   - 流式请求体读取
//
// 关键设计决策：
//
//   1. bufferPool 使用 singleton 模式，ctxPool 保持独立
//   2. CommonAdapter 不包含 ConvertResponse（HTTP/2/HTTP/3 行为不同）
//   3. 阈值常量统一，避免 HTTP/2 inline 和 HTTP/3 constant 不一致
//
// 作者：xfy
package adapter

import (
	"io"
	"net/http"
	"sync"

	"github.com/valyala/fasthttp"
)

// DefaultBodyThreshold 是请求体大小阈值，超过此值使用流式处理。
//
// 64KB 是经过测试的平衡点：
//   - 小于此值：直接读取到内存，避免 pool 开销
//   - 大于此值：使用流式缓冲区，避免大内存分配
const DefaultBodyThreshold = 64 * 1024 // 64KB

// bufferPoolInstance 是全局共享的缓冲区池 singleton。
//
// 使用 singleton 模式避免多个适配器实例创建多个 pool，
// 提高内存复用效率。该 pool 被 HTTP/2 和 HTTP/3 适配器共享。
var bufferPoolInstance = &sync.Pool{
	New: func() interface{} {
		buf := make([]byte, 4096) // 4KB 初始缓冲区
		return &buf
	},
}

// SharedBufferPool 返回全局共享的缓冲区池实例。
//
// HTTP/2 和 HTTP/3 适配器都使用此 pool 来复用字节缓冲区，
// 避免大请求体处理时的频繁内存分配。
//
// 返回值：
//   - *sync.Pool: 全局缓冲区池实例
func SharedBufferPool() *sync.Pool {
	return bufferPoolInstance
}

// CommonAdapter 提供 HTTP/2 和 HTTP/3 适配器的共享基础结构。
//
// 该结构体提取了两个适配器共用的字段和方法，
// 但不包含 ConvertResponse（HTTP/2 和 HTTP/3 的响应转换逻辑不同）。
type CommonAdapter struct {
	// CtxPool 用于复用 fasthttp.RequestCtx 对象
	// 每个协议适配器实例独立维护自己的 ctxPool
	CtxPool sync.Pool
}

// NewCommonAdapter 创建新的共享适配器实例。
//
// 初始化 CommonAdapter，设置 ctxPool 的 New 函数。
// bufferPool 使用全局 singleton，不需要在实例中存储。
//
// 返回值：
//   - *CommonAdapter: 初始化的共享适配器实例
func NewCommonAdapter() *CommonAdapter {
	return &CommonAdapter{
		CtxPool: sync.Pool{
			New: func() interface{} {
				return &fasthttp.RequestCtx{}
			},
		},
	}
}

// ResetContext 重置 fasthttp.RequestCtx 状态。
//
// 从 pool 获取的 ctx 可能带有之前请求的残留状态，
// 必须在每次使用前调用此方法进行清理。
//
// 参数：
//   - ctx: 需要重置的 fasthttp 请求上下文
func (a *CommonAdapter) ResetContext(ctx *fasthttp.RequestCtx) {
	// 禁用头部规范化以保持原始大小写
	ctx.Request.Header.DisableNormalizing()
	// 重置请求和响应状态
	ctx.Request.Reset()
	ctx.Response.Reset()
	// 清除用户自定义值
	ctx.SetUserValueBytes(nil, nil)
}

// StreamRequestBody 流式读取 HTTP 请求体到 fasthttp。
//
// 对于小于等于 DefaultBodyThreshold（64KB）的请求体，直接读取到内存；
// 对于大于阈值的请求体，使用共享 bufferPool 进行流式处理，避免内存峰值。
//
// 参数：
//   - r: 标准库的 HTTP 请求
//   - ctx: fasthttp 请求上下文，用于存储读取的请求体
func (a *CommonAdapter) StreamRequestBody(r *http.Request, ctx *fasthttp.RequestCtx) {
	if r.Body == nil || r.Body == http.NoBody {
		return
	}

	defer func() {
		_ = r.Body.Close()
	}()

	// 小请求体：直接读取到内存（<= 64KB）
	if r.ContentLength > 0 && r.ContentLength <= DefaultBodyThreshold {
		body, err := io.ReadAll(r.Body)
		if err == nil {
			ctx.Request.SetBody(body)
		}
		return
	}

	// 大请求体：使用流式缓冲区（> 64KB 或未知长度）
	// 从全局 pool 获取缓冲区
	bufPtr, ok := bufferPoolInstance.Get().(*[]byte)
	if !ok {
		// 如果类型断言失败，创建新的缓冲区（不应该发生）
		buf := make([]byte, 4096)
		bufPtr = &buf
	}
	defer bufferPoolInstance.Put(bufPtr)

	buf := *bufPtr
	var body []byte

	// 如果已知 ContentLength，预分配精确大小的缓冲区
	if r.ContentLength > 0 {
		body = make([]byte, 0, r.ContentLength)
	}

	// 分块读取请求体
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

// GetContext 从 pool 获取一个 fasthttp.RequestCtx。
//
// 使用 pool 复用 RequestCtx 对象，减少 GC 压力。
// 获取的 ctx 必须通过 ResetContext 重置后才能使用。
//
// 返回值：
//   - *fasthttp.RequestCtx: fasthttp 请求上下文
//   - bool: 如果为 false，表示类型断言失败，ctx 是新创建的
func (a *CommonAdapter) GetContext() (*fasthttp.RequestCtx, bool) {
	ctx, ok := a.CtxPool.Get().(*fasthttp.RequestCtx)
	if !ok {
		ctx = &fasthttp.RequestCtx{}
	}
	return ctx, ok
}

// PutContext 将 fasthttp.RequestCtx 放回 pool。
//
// 在放回 pool 前应该调用 ResetContext 清理状态。
//
// 参数：
//   - ctx: 要放回 pool 的 fasthttp 请求上下文
func (a *CommonAdapter) PutContext(ctx *fasthttp.RequestCtx) {
	a.CtxPool.Put(ctx)
}
