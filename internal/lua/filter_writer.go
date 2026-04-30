// Package lua 提供 Lua 脚本嵌入能力。
//
// 该文件实现响应拦截器和延迟写入机制，用于 Lua header_filter/body_filter 阶段。
// 包括：
//   - ResponseInterceptor：延迟 header 写入，允许在发送前修改响应
//   - DelayedResponseWriter：包装 fasthttp.RequestCtx 提供延迟写入能力
//   - BufferedWriter：带缓冲区的写入器，支持自动刷新
//   - 对象池：ResponseInterceptorPool、bufferPool 减少 GC 压力
//
// 执行流程：
//  1. 启用拦截模式后，header 和 body 写入被延迟
//  2. HeaderFilter 阶段可执行 Lua 脚本修改响应头
//  3. BodyFilter 阶段可执行 Lua 脚本修改响应体
//  4. Flush 时应用所有修改并发送响应
//
// 注意事项：
//   - 流式 body（SetBodyStream）无法缓冲，header filter 在设置前应用
//   - 拦截器使用后必须调用 ReleaseResponseInterceptor 放回池中
//
// 作者：xfy
package lua

import (
	"io"
	"net"
	"sync"

	"github.com/valyala/fasthttp"
)

// ResponseInterceptor 响应拦截器。
//
// 用于延迟 header 写入，允许在 header/body_filter 阶段执行 Lua 脚本
// 修改响应内容后再发送。所有 header 修改、删除和 body 缓冲均在
// Flush 时统一应用。
//
// 线程安全：SetHeader 等方法使用 sync.RWMutex 保护。
type ResponseInterceptor struct {
	// ctx 关联的 fasthttp 请求上下文
	ctx *fasthttp.RequestCtx

	// headerFilterFunc header 过滤器回调（在 Flush 时执行）
	headerFilterFunc func() error

	// bodyFilterFunc body 过滤器回调（在 Flush 时执行）
	bodyFilterFunc func([]byte) ([]byte, error)

	// customHeaders 自定义 header 映射（延迟发送）
	customHeaders map[string]string

	// headersToDelete 需要删除的 header 列表
	headersToDelete []string

	// bodyBuffer 缓冲的 body 数据
	bodyBuffer []byte

	// statusCode 响应状态码
	statusCode int

	// mu 读写锁
	mu sync.RWMutex

	// headersWritten 标记 header 是否已发送
	headersWritten bool

	// intercepted 是否启用拦截模式
	intercepted bool
}

// NewResponseInterceptor 创建响应拦截器。
//
// 参数：
//   - ctx: fasthttp 请求上下文
//
// 返回值：
//   - *ResponseInterceptor: 初始化的拦截器实例
func NewResponseInterceptor(ctx *fasthttp.RequestCtx) *ResponseInterceptor {
	return &ResponseInterceptor{
		ctx:             ctx,
		statusCode:      200,
		customHeaders:   make(map[string]string),
		headersToDelete: make([]string, 0),
	}
}

// SetHeaderFilter 设置 header 过滤器回调。
//
// 参数：
//   - fn: 回调函数，在 Flush 时执行，返回非 nil error 将中断响应
func (ri *ResponseInterceptor) SetHeaderFilter(fn func() error) {
	ri.headerFilterFunc = fn
}

// SetBodyFilter 设置 body 过滤器回调。
//
// 参数：
//   - fn: 回调函数，接收原始 body，返回修改后的 body
func (ri *ResponseInterceptor) SetBodyFilter(fn func([]byte) ([]byte, error)) {
	ri.bodyFilterFunc = fn
}

// SetStatusCode 设置响应状态码（延迟到 Flush 时生效）。
//
// 参数：
//   - code: HTTP 状态码
func (ri *ResponseInterceptor) SetStatusCode(code int) {
	ri.statusCode = code
}

// GetStatusCode 获取当前状态码。
//
// 返回值：
//   - int: 当前设置的状态码
func (ri *ResponseInterceptor) GetStatusCode() int {
	return ri.statusCode
}

// SetHeader 设置 header（延迟到 Flush 时生效）。
//
// 参数：
//   - key: header 名称
//   - value: header 值
func (ri *ResponseInterceptor) SetHeader(key, value string) {
	ri.mu.Lock()
	defer ri.mu.Unlock()
	ri.customHeaders[key] = value
}

// GetHeader 获取原始 header 值（直接从响应读取）。
//
// 参数：
//   - key: header 名称
//
// 返回值：
//   - []byte: header 值
func (ri *ResponseInterceptor) GetHeader(key string) []byte {
	return ri.ctx.Response.Header.Peek(key)
}

// DelHeader 标记删除 header（延迟到 Flush 时生效）。
//
// 参数：
//   - key: 要删除的 header 名称
func (ri *ResponseInterceptor) DelHeader(key string) {
	ri.headersToDelete = append(ri.headersToDelete, key)
}

// Write 拦截写入操作（缓冲 body，延迟 header 发送）。
//
// 如果未启用拦截模式，直接写入 ctx。
//
// 参数：
//   - p: 要写入的数据
//
// 返回值：
//   - int: 写入字节数
//   - error: 写入错误
func (ri *ResponseInterceptor) Write(p []byte) (int, error) {
	if !ri.intercepted {
		// 未启用拦截，直接写入
		return ri.ctx.Write(p)
	}

	// 缓冲 body 数据
	ri.bodyBuffer = append(ri.bodyBuffer, p...)
	return len(p), nil
}

// WriteString 写入字符串。
//
// 参数：
//   - s: 要写入的字符串
//
// 返回值：
//   - int: 写入字节数
//   - error: 写入错误
func (ri *ResponseInterceptor) WriteString(s string) (int, error) {
	return ri.Write([]byte(s))
}

// SetBody 设置 body（延迟发送）。
//
// 参数：
//   - body: 响应体内容
func (ri *ResponseInterceptor) SetBody(body []byte) {
	if !ri.intercepted {
		ri.ctx.SetBody(body)
		return
	}
	ri.bodyBuffer = body
}

// SetBodyString 设置字符串 body。
//
// 参数：
//   - body: 响应体内容
func (ri *ResponseInterceptor) SetBodyString(body string) {
	ri.SetBody([]byte(body))
}

// Flush 执行 header/body filter 并发送响应。
//
// 执行顺序：
//  1. 执行 header filter 回调
//  2. 应用 header 修改和删除
//  3. 执行 body filter 回调
//  4. 发送最终响应
//
// 返回值：
//   - error: filter 执行失败时返回错误
func (ri *ResponseInterceptor) Flush() error {
	if ri.headersWritten {
		return nil // 已经发送过
	}
	ri.headersWritten = true

	// 1. 执行 header filter
	if ri.headerFilterFunc != nil {
		if err := ri.headerFilterFunc(); err != nil {
			return err
		}
	}

	// 2. 应用 header 修改
	ri.ctx.Response.SetStatusCode(ri.statusCode)
	for key, value := range ri.customHeaders {
		ri.ctx.Response.Header.Set(key, value)
	}
	for _, key := range ri.headersToDelete {
		ri.ctx.Response.Header.Del(key)
	}

	// 3. 执行 body filter
	body := ri.bodyBuffer
	if ri.bodyFilterFunc != nil && len(body) > 0 {
		modified, err := ri.bodyFilterFunc(body)
		if err != nil {
			return err
		}
		body = modified
	}

	// 4. 发送响应
	if len(body) > 0 {
		ri.ctx.SetBody(body)
	}

	return nil
}

// Enable 启用拦截模式。
func (ri *ResponseInterceptor) Enable() {
	ri.intercepted = true
}

// Disable 禁用拦截模式。
func (ri *ResponseInterceptor) Disable() {
	ri.intercepted = false
}

// IsEnabled 检查是否启用拦截。
//
// 返回值：
//   - bool: true 表示启用
func (ri *ResponseInterceptor) IsEnabled() bool {
	return ri.intercepted
}

// GetBufferedBody 获取当前缓冲的 body。
//
// 返回值：
//   - []byte: 缓冲的 body 数据
func (ri *ResponseInterceptor) GetBufferedBody() []byte {
	return ri.bodyBuffer
}

// ClearBody 清空 body 缓冲。
func (ri *ResponseInterceptor) ClearBody() {
	ri.bodyBuffer = nil
}

// DelayedResponseWriter 延迟响应写入器。
//
// 包装 fasthttp.RequestCtx 和 ResponseInterceptor，提供延迟写入能力。
// 用于 Lua header_filter/body_filter 阶段的响应拦截和修改。
type DelayedResponseWriter struct {
	// ctx 关联的 fasthttp 请求上下文
	ctx *fasthttp.RequestCtx

	// interceptor 响应拦截器
	interceptor *ResponseInterceptor
}

// NewDelayedResponseWriter 创建延迟响应写入器。
//
// 参数：
//   - ctx: fasthttp 请求上下文
//
// 返回值：
//   - *DelayedResponseWriter: 初始化的写入器实例
func NewDelayedResponseWriter(ctx *fasthttp.RequestCtx) *DelayedResponseWriter {
	return &DelayedResponseWriter{
		ctx:         ctx,
		interceptor: NewResponseInterceptor(ctx),
	}
}

// EnableFilterPhase 启用 filter phase（启动拦截模式）。
func (drw *DelayedResponseWriter) EnableFilterPhase() {
	drw.interceptor.Enable()
}

// DisableFilterPhase 禁用 filter phase。
func (drw *DelayedResponseWriter) DisableFilterPhase() {
	drw.interceptor.Disable()
}

// GetInterceptor 获取响应拦截器。
//
// 返回值：
//   - *ResponseInterceptor: 关联的拦截器
func (drw *DelayedResponseWriter) GetInterceptor() *ResponseInterceptor {
	return drw.interceptor
}

// HeaderFilter 注册 header filter 阶段的 Lua 脚本。
//
// 参数：
//   - script: Lua 脚本
//   - luaCtx: Lua 上下文
//
// 返回值：
//   - error: 脚本执行失败时返回错误
func (drw *DelayedResponseWriter) HeaderFilter(script string, luaCtx *LuaContext) error {
	if !drw.interceptor.IsEnabled() {
		return nil
	}

	luaCtx.SetPhase(PhaseHeaderFilter)
	drw.interceptor.SetHeaderFilter(func() error {
		return luaCtx.Execute(script)
	})
	return nil
}

// BodyFilter 注册 body filter 阶段的 Lua 脚本。
//
// 参数：
//   - script: Lua 脚本
//   - luaCtx: Lua 上下文
//
// 返回值：
//   - error: 脚本执行失败时返回错误
func (drw *DelayedResponseWriter) BodyFilter(script string, luaCtx *LuaContext) error {
	if !drw.interceptor.IsEnabled() {
		return nil
	}

	luaCtx.SetPhase(PhaseBodyFilter)
	drw.interceptor.SetBodyFilter(func(body []byte) ([]byte, error) {
		// 将 body 设置到 Lua 上下文中
		luaCtx.OutputBuffer = body
		if err := luaCtx.Execute(script); err != nil {
			return nil, err
		}
		return luaCtx.OutputBuffer, nil
	})
	return nil
}

// Flush 刷新响应（执行 filter 并发送）。
//
// 返回值：
//   - error: 刷新失败时返回错误
func (drw *DelayedResponseWriter) Flush() error {
	return drw.interceptor.Flush()
}

// Write 实现 io.Writer 接口。
//
// 参数：
//   - p: 要写入的数据
//
// 返回值：
//   - int: 写入字节数
//   - error: 写入错误
func (drw *DelayedResponseWriter) Write(p []byte) (int, error) {
	return drw.interceptor.Write(p)
}

// WriteString 写入字符串。
//
// 参数：
//   - s: 要写入的字符串
//
// 返回值：
//   - int: 写入字节数
//   - error: 写入错误
func (drw *DelayedResponseWriter) WriteString(s string) (int, error) {
	return drw.interceptor.WriteString(s)
}

// SetStatusCode 设置状态码。
func (drw *DelayedResponseWriter) SetStatusCode(code int) {
	drw.interceptor.SetStatusCode(code)
}

// SetBody 设置 body。
func (drw *DelayedResponseWriter) SetBody(body []byte) {
	drw.interceptor.SetBody(body)
}

// SetBodyString 设置字符串 body。
func (drw *DelayedResponseWriter) SetBodyString(body string) {
	drw.interceptor.SetBodyString(body)
}

// SetHeader 设置 header。
func (drw *DelayedResponseWriter) SetHeader(key, value string) {
	drw.interceptor.SetHeader(key, value)
}

// GetHeader 获取 header。
func (drw *DelayedResponseWriter) GetHeader(key string) []byte {
	return drw.interceptor.GetHeader(key)
}

// DelHeader 删除 header。
func (drw *DelayedResponseWriter) DelHeader(key string) {
	drw.interceptor.DelHeader(key)
}

// ResponseInterceptorPool 响应拦截器对象池。
var ResponseInterceptorPool = sync.Pool{
	New: func() any {
		return &ResponseInterceptor{}
	},
}

// AcquireResponseInterceptor 从池中获取拦截器并初始化。
//
// 参数：
//   - ctx: fasthttp 请求上下文
//
// 返回值：
//   - *ResponseInterceptor: 初始化后的拦截器
func AcquireResponseInterceptor(ctx *fasthttp.RequestCtx) *ResponseInterceptor {
	ri, ok := ResponseInterceptorPool.Get().(*ResponseInterceptor)
	if !ok {
		ri = &ResponseInterceptor{}
	}
	ri.ctx = ctx
	ri.statusCode = 200
	ri.customHeaders = make(map[string]string)
	ri.headersToDelete = make([]string, 0)
	ri.bodyBuffer = nil
	ri.headersWritten = false
	ri.intercepted = true
	ri.headerFilterFunc = nil
	ri.bodyFilterFunc = nil
	return ri
}

// ReleaseResponseInterceptor 释放拦截器回池。
//
// 清理所有引用和回调，防止内存泄漏。
func ReleaseResponseInterceptor(ri *ResponseInterceptor) {
	if ri == nil {
		return
	}
	// 清理状态
	ri.ctx = nil
	ri.headerFilterFunc = nil
	ri.bodyFilterFunc = nil
	ri.bodyBuffer = nil
	ri.customHeaders = nil
	ri.headersToDelete = nil
	ResponseInterceptorPool.Put(ri)
}

// Hijack 支持连接劫持（用于 WebSocket）。
//
// 参数：
//   - handler: 劫持后的处理函数
func (drw *DelayedResponseWriter) Hijack(handler fasthttp.HijackHandler) {
	drw.ctx.Hijack(handler)
}

// Hijacked 检查是否已劫持。
//
// 返回值：
//   - bool: true 表示已劫持
func (drw *DelayedResponseWriter) Hijacked() bool {
	return drw.ctx.Hijacked()
}

// LocalAddr 获取本地地址。
//
// 返回值：
//   - net.Addr: 本地网络地址
func (drw *DelayedResponseWriter) LocalAddr() net.Addr {
	return drw.ctx.LocalAddr()
}

// RemoteAddr 获取远程地址。
//
// 返回值：
//   - net.Addr: 远程网络地址
func (drw *DelayedResponseWriter) RemoteAddr() net.Addr {
	return drw.ctx.RemoteAddr()
}

// SetConnectionClose 设置响应头 Connection: close。
func (drw *DelayedResponseWriter) SetConnectionClose() {
	drw.ctx.Response.SetConnectionClose()
}

// BodyWriter 返回 body 写入器（适配 io.Writer）。
//
// 返回值：
//   - io.Writer: body 写入器
func (drw *DelayedResponseWriter) BodyWriter() io.Writer {
	return &responseWriterAdapter{interceptor: drw.interceptor}
}

// responseWriterAdapter 将 ResponseInterceptor 适配为 io.Writer 接口。
type responseWriterAdapter struct {
	interceptor *ResponseInterceptor
}

// Write 实现 io.Writer 接口。
func (rwa *responseWriterAdapter) Write(p []byte) (n int, err error) {
	return rwa.interceptor.Write(p)
}

// ResponseStats 响应统计信息。
type ResponseStats struct {
	// BufferedBytes 缓冲的 body 字节数
	BufferedBytes int

	// HeadersModified 修改的 header 数量
	HeadersModified int

	// HeadersDeleted 删除的 header 数量
	HeadersDeleted int

	// BodyModified body 是否被修改
	BodyModified bool

	// StatusCode 响应状态码
	StatusCode int
}

// GetStats 获取响应统计信息。
//
// 返回值：
//   - ResponseStats: 当前统计快照
func (drw *DelayedResponseWriter) GetStats() ResponseStats {
	return ResponseStats{
		BufferedBytes:   len(drw.interceptor.bodyBuffer),
		HeadersModified: len(drw.interceptor.customHeaders),
		HeadersDeleted:  len(drw.interceptor.headersToDelete),
		BodyModified:    drw.interceptor.bodyFilterFunc != nil,
		StatusCode:      drw.interceptor.statusCode,
	}
}

// IsBodyBuffered 检查 body 是否被缓冲。
//
// 返回值：
//   - bool: true 表示有缓冲数据
func (drw *DelayedResponseWriter) IsBodyBuffered() bool {
	return len(drw.interceptor.bodyBuffer) > 0
}

// GetBufferedBodySize 获取缓冲的 body 大小。
//
// 返回值：
//   - int: 缓冲字节数
func (drw *DelayedResponseWriter) GetBufferedBodySize() int {
	return len(drw.interceptor.bodyBuffer)
}

// Reset 重置写入器状态。
func (drw *DelayedResponseWriter) Reset() {
	drw.interceptor.bodyBuffer = nil
	drw.interceptor.headersWritten = false
	drw.interceptor.statusCode = 200
	drw.interceptor.customHeaders = make(map[string]string)
	drw.interceptor.headersToDelete = make([]string, 0)
}

// SetBodyStream 设置 body 流。
//
// 流式 body 无法缓冲，在设置前应用 header filter。
//
// 参数：
//   - bodyStream: body 数据源
//   - bodySize: body 大小（-1 表示未知）
func (drw *DelayedResponseWriter) SetBodyStream(bodyStream io.Reader, bodySize int) {
	if !drw.interceptor.IsEnabled() {
		drw.ctx.SetBodyStream(bodyStream, bodySize)
		return
	}
	// 流式 body 无法缓冲，直接设置
	// 但在设置前应用 header filter
	if drw.interceptor.headerFilterFunc != nil {
		_ = drw.interceptor.headerFilterFunc()
	}
	drw.ctx.SetBodyStream(bodyStream, bodySize)
	drw.interceptor.headersWritten = true
}

// SendFile 发送文件。
//
// 在发送前应用 header filter 和自定义 header。
//
// 参数：
//   - path: 文件路径
//
// 返回值：
//   - error: 发送失败时返回错误
func (drw *DelayedResponseWriter) SendFile(path string) error {
	if !drw.interceptor.IsEnabled() {
		drw.ctx.SendFile(path)
		return nil
	}
	// 文件发送前应用 header filter
	if drw.interceptor.headerFilterFunc != nil {
		if err := drw.interceptor.headerFilterFunc(); err != nil {
			return err
		}
	}
	// 应用修改的 headers
	drw.ctx.Response.SetStatusCode(drw.interceptor.statusCode)
	for key, value := range drw.interceptor.customHeaders {
		drw.ctx.Response.Header.Set(key, value)
	}
	for _, key := range drw.interceptor.headersToDelete {
		drw.ctx.Response.Header.Del(key)
	}
	drw.ctx.SendFile(path)
	drw.interceptor.headersWritten = true
	return nil
}

// Redirect 重定向。
//
// 在重定向前应用 header filter。
//
// 参数：
//   - uri: 目标 URI
//   - statusCode: HTTP 重定向状态码
func (drw *DelayedResponseWriter) Redirect(uri string, statusCode int) {
	if !drw.interceptor.IsEnabled() {
		drw.ctx.Redirect(uri, statusCode)
		return
	}
	// 重定向前应用 header filter
	if drw.interceptor.headerFilterFunc != nil {
		_ = drw.interceptor.headerFilterFunc()
	}
	drw.ctx.Redirect(uri, statusCode)
	drw.interceptor.headersWritten = true
}

// bufferPool body 缓冲区对象池。
var bufferPool = sync.Pool{
	New: func() any {
		buf := make([]byte, 0, 4096) // 4KB 初始容量
		return &buf
	},
}

// acquireBuffer 获取缓冲区。
//
// 返回值：
//   - []byte: 可复用的缓冲区
func acquireBuffer() []byte {
	buf, ok := bufferPool.Get().(*[]byte)
	if !ok {
		return []byte{}
	}
	return *buf
}

// releaseBuffer 释放缓冲区回池。
//
// 只回收容量不超过 64KB 的缓冲区，避免池过大。
func releaseBuffer(buf []byte) {
	if buf != nil && cap(buf) <= 65536 { // 只回收小缓冲区
		buf = buf[:0]
		bufferPool.Put(&buf)
	}
}

// BufferedWriter 带缓冲的写入器。
//
// 支持自动刷新（达到 maxSize 时自动调用 flushFunc）和手动刷新。
// 使用对象池分配底层缓冲区。
type BufferedWriter struct {
	// flushFunc 刷新回调
	flushFunc func([]byte) error

	// buf 缓冲区
	buf []byte

	// maxSize 自动刷新的最大大小
	maxSize int

	// autoFlush 是否启用自动刷新
	autoFlush bool
}

// NewBufferedWriter 创建缓冲写入器。
//
// 参数：
//   - maxSize: 触发自动刷新的最大缓冲区大小
//   - flushFunc: 刷新回调函数
//
// 返回值：
//   - *BufferedWriter: 初始化的写入器
func NewBufferedWriter(maxSize int, flushFunc func([]byte) error) *BufferedWriter {
	return &BufferedWriter{
		buf:       acquireBuffer(),
		maxSize:   maxSize,
		flushFunc: flushFunc,
		autoFlush: true,
	}
}

// Write 写入数据到缓冲区。
//
// 如果缓冲区不足，自动扩容。如果启用 autoFlush 且达到 maxSize，自动刷新。
//
// 参数：
//   - p: 要写入的数据
//
// 返回值：
//   - int: 写入字节数
//   - error: 刷新失败时返回错误
func (bw *BufferedWriter) Write(p []byte) (int, error) {
	if bw.buf == nil {
		bw.buf = acquireBuffer()
	}

	// 检查是否需要扩容
	if len(bw.buf)+len(p) > cap(bw.buf) {
		// 扩容
		newCap := max(cap(bw.buf)*2, len(bw.buf)+len(p))
		newBuf := make([]byte, len(bw.buf), newCap)
		copy(newBuf, bw.buf)
		releaseBuffer(bw.buf)
		bw.buf = newBuf
	}

	bw.buf = append(bw.buf, p...)

	// 自动刷新检查
	if bw.autoFlush && bw.maxSize > 0 && len(bw.buf) >= bw.maxSize {
		if err := bw.Flush(); err != nil {
			return len(p), err
		}
	}

	return len(p), nil
}

// Flush 刷新缓冲区。
//
// 返回值：
//   - error: 刷新失败时返回错误
func (bw *BufferedWriter) Flush() error {
	if bw.flushFunc == nil || len(bw.buf) == 0 {
		return nil
	}
	if err := bw.flushFunc(bw.buf); err != nil {
		return err
	}
	bw.buf = bw.buf[:0]
	return nil
}

// Close 关闭写入器，刷新剩余数据并回收缓冲区。
//
// 返回值：
//   - error: 刷新失败时返回错误
func (bw *BufferedWriter) Close() error {
	err := bw.Flush()
	if bw.buf != nil {
		releaseBuffer(bw.buf)
		bw.buf = nil
	}
	return err
}

// Size 返回当前缓冲区大小。
//
// 返回值：
//   - int: 缓冲区字节数
func (bw *BufferedWriter) Size() int {
	return len(bw.buf)
}

// Bytes 返回当前缓冲区内容（不消费）。
//
// 返回值：
//   - []byte: 缓冲区内容
func (bw *BufferedWriter) Bytes() []byte {
	return bw.buf
}
