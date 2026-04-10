// Package lua 提供 Lua 脚本嵌入能力
package lua

import (
	"io"
	"net"
	"sync"

	"github.com/valyala/fasthttp"
)

// ResponseInterceptor 响应拦截器
// 用于延迟 header 写入，允许在发送前修改响应
type ResponseInterceptor struct {
	// 原始请求上下文
	ctx *fasthttp.RequestCtx

	// Header 修改回调（Lua 执行）
	headerFilterFunc func() error

	// Body 修改回调（Lua 执行）
	bodyFilterFunc func([]byte) ([]byte, error)

	// 缓冲的 body 数据
	bodyBuffer []byte

	// 是否已写入 header
	headersWritten bool

	// 是否已拦截
	intercepted bool

	// 状态码（可修改）
	statusCode int

	// 自定义 header（可修改）
	customHeaders map[string]string

	// 需要删除的 header
	headersToDelete []string

	// 并发保护
	mu sync.RWMutex
}

// NewResponseInterceptor 创建响应拦截器
func NewResponseInterceptor(ctx *fasthttp.RequestCtx) *ResponseInterceptor {
	return &ResponseInterceptor{
		ctx:             ctx,
		statusCode:      200,
		customHeaders:   make(map[string]string),
		headersToDelete: make([]string, 0),
	}
}

// SetHeaderFilter 设置 header 过滤器回调
func (ri *ResponseInterceptor) SetHeaderFilter(fn func() error) {
	ri.headerFilterFunc = fn
}

// SetBodyFilter 设置 body 过滤器回调
func (ri *ResponseInterceptor) SetBodyFilter(fn func([]byte) ([]byte, error)) {
	ri.bodyFilterFunc = fn
}

// SetStatusCode 设置状态码（延迟生效）
func (ri *ResponseInterceptor) SetStatusCode(code int) {
	ri.statusCode = code
}

// GetStatusCode 获取当前状态码
func (ri *ResponseInterceptor) GetStatusCode() int {
	return ri.statusCode
}

// SetHeader 设置 header（延迟生效）
func (ri *ResponseInterceptor) SetHeader(key, value string) {
	ri.mu.Lock()
	defer ri.mu.Unlock()
	ri.customHeaders[key] = value
}

// GetHeader 获取原始 header 值
func (ri *ResponseInterceptor) GetHeader(key string) []byte {
	return ri.ctx.Response.Header.Peek(key)
}

// DelHeader 删除 header（延迟生效）
func (ri *ResponseInterceptor) DelHeader(key string) {
	ri.headersToDelete = append(ri.headersToDelete, key)
}

// Write 拦截写入操作（缓冲 body，延迟 header 发送）
func (ri *ResponseInterceptor) Write(p []byte) (int, error) {
	if !ri.intercepted {
		// 未启用拦截，直接写入
		return ri.ctx.Write(p)
	}

	// 缓冲 body 数据
	ri.bodyBuffer = append(ri.bodyBuffer, p...)
	return len(p), nil
}

// WriteString 写入字符串
func (ri *ResponseInterceptor) WriteString(s string) (int, error) {
	return ri.Write([]byte(s))
}

// SetBody 设置 body（延迟发送）
func (ri *ResponseInterceptor) SetBody(body []byte) {
	if !ri.intercepted {
		ri.ctx.SetBody(body)
		return
	}
	ri.bodyBuffer = body
}

// SetBodyString 设置字符串 body
func (ri *ResponseInterceptor) SetBodyString(body string) {
	ri.SetBody([]byte(body))
}

// Flush 执行 header/body filter 并发送响应
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

// Enable 启用拦截模式
func (ri *ResponseInterceptor) Enable() {
	ri.intercepted = true
}

// Disable 禁用拦截模式
func (ri *ResponseInterceptor) Disable() {
	ri.intercepted = false
}

// IsEnabled 检查是否启用拦截
func (ri *ResponseInterceptor) IsEnabled() bool {
	return ri.intercepted
}

// GetBufferedBody 获取当前缓冲的 body
func (ri *ResponseInterceptor) GetBufferedBody() []byte {
	return ri.bodyBuffer
}

// ClearBody 清空 body 缓冲
func (ri *ResponseInterceptor) ClearBody() {
	ri.bodyBuffer = nil
}

// DelayedResponseWriter 延迟响应写入器
// 包装 fasthttp.RequestCtx 提供延迟写入能力
type DelayedResponseWriter struct {
	ctx         *fasthttp.RequestCtx
	interceptor *ResponseInterceptor
	pool        *sync.Pool
}

// NewDelayedResponseWriter 创建延迟响应写入器
func NewDelayedResponseWriter(ctx *fasthttp.RequestCtx) *DelayedResponseWriter {
	return &DelayedResponseWriter{
		ctx:         ctx,
		interceptor: NewResponseInterceptor(ctx),
	}
}

// EnableFilterPhase 启用 filter phase
func (drw *DelayedResponseWriter) EnableFilterPhase() {
	drw.interceptor.Enable()
}

// DisableFilterPhase 禁用 filter phase
func (drw *DelayedResponseWriter) DisableFilterPhase() {
	drw.interceptor.Disable()
}

// GetInterceptor 获取响应拦截器
func (drw *DelayedResponseWriter) GetInterceptor() *ResponseInterceptor {
	return drw.interceptor
}

// HeaderFilter 执行 header filter 阶段
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

// BodyFilter 执行 body filter 阶段
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

// Flush 刷新响应
func (drw *DelayedResponseWriter) Flush() error {
	return drw.interceptor.Flush()
}

// Write 实现 io.Writer
func (drw *DelayedResponseWriter) Write(p []byte) (int, error) {
	return drw.interceptor.Write(p)
}

// WriteString 写入字符串
func (drw *DelayedResponseWriter) WriteString(s string) (int, error) {
	return drw.interceptor.WriteString(s)
}

// SetStatusCode 设置状态码
func (drw *DelayedResponseWriter) SetStatusCode(code int) {
	drw.interceptor.SetStatusCode(code)
}

// SetBody 设置 body
func (drw *DelayedResponseWriter) SetBody(body []byte) {
	drw.interceptor.SetBody(body)
}

// SetBodyString 设置字符串 body
func (drw *DelayedResponseWriter) SetBodyString(body string) {
	drw.interceptor.SetBodyString(body)
}

// SetHeader 设置 header
func (drw *DelayedResponseWriter) SetHeader(key, value string) {
	drw.interceptor.SetHeader(key, value)
}

// GetHeader 获取 header
func (drw *DelayedResponseWriter) GetHeader(key string) []byte {
	return drw.interceptor.GetHeader(key)
}

// DelHeader 删除 header
func (drw *DelayedResponseWriter) DelHeader(key string) {
	drw.interceptor.DelHeader(key)
}

// ResponseInterceptorPool 响应拦截器池
var ResponseInterceptorPool = sync.Pool{
	New: func() interface{} {
		return &ResponseInterceptor{}
	},
}

// AcquireResponseInterceptor 从池中获取拦截器
func AcquireResponseInterceptor(ctx *fasthttp.RequestCtx) *ResponseInterceptor {
	ri := ResponseInterceptorPool.Get().(*ResponseInterceptor)
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

// ReleaseResponseInterceptor 释放拦截器回池
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

// responseWriterWrapper 适配 fasthttp.ResponseWriter 接口
type responseWriterWrapper struct {
	interceptor *ResponseInterceptor
}

func (w *responseWriterWrapper) Write(p []byte) (n int, err error) {
	return w.interceptor.Write(p)
}

func (w *responseWriterWrapper) Header() map[string][]string {
	// fasthttp 不兼容 http.Header，返回 nil
	return nil
}

func (w *responseWriterWrapper) WriteHeader(statusCode int) {
	w.interceptor.SetStatusCode(statusCode)
}

// Hijack 支持连接劫持（用于 WebSocket）
func (drw *DelayedResponseWriter) Hijack(handler fasthttp.HijackHandler) {
	drw.ctx.Hijack(handler)
}

// Hijacked 检查是否已劫持
func (drw *DelayedResponseWriter) Hijacked() bool {
	return drw.ctx.Hijacked()
}

// LocalAddr 获取本地地址
func (drw *DelayedResponseWriter) LocalAddr() net.Addr {
	return drw.ctx.LocalAddr()
}

// RemoteAddr 获取远程地址
func (drw *DelayedResponseWriter) RemoteAddr() net.Addr {
	return drw.ctx.RemoteAddr()
}

// SetConnectionClose 设置连接关闭
func (drw *DelayedResponseWriter) SetConnectionClose() {
	drw.ctx.Response.SetConnectionClose()
}

// BodyWriter 返回 body 写入器
func (drw *DelayedResponseWriter) BodyWriter() io.Writer {
	return &responseWriterAdapter{interceptor: drw.interceptor}
}

// responseWriterAdapter 适配 io.Writer
type responseWriterAdapter struct {
	interceptor *ResponseInterceptor
}

func (rwa *responseWriterAdapter) Write(p []byte) (n int, err error) {
	return rwa.interceptor.Write(p)
}

// ResponseStats 响应统计信息
type ResponseStats struct {
	BufferedBytes   int
	HeadersModified int
	HeadersDeleted  int
	BodyModified    bool
	StatusCode      int
}

// GetStats 获取响应统计
func (drw *DelayedResponseWriter) GetStats() ResponseStats {
	return ResponseStats{
		BufferedBytes:   len(drw.interceptor.bodyBuffer),
		HeadersModified: len(drw.interceptor.customHeaders),
		HeadersDeleted:  len(drw.interceptor.headersToDelete),
		BodyModified:    drw.interceptor.bodyFilterFunc != nil,
		StatusCode:      drw.interceptor.statusCode,
	}
}

// IsBodyBuffered 检查 body 是否被缓冲
func (drw *DelayedResponseWriter) IsBodyBuffered() bool {
	return len(drw.interceptor.bodyBuffer) > 0
}

// GetBufferedBodySize 获取缓冲的 body 大小
func (drw *DelayedResponseWriter) GetBufferedBodySize() int {
	return len(drw.interceptor.bodyBuffer)
}

// Reset 重置写入器状态
func (drw *DelayedResponseWriter) Reset() {
	drw.interceptor.bodyBuffer = nil
	drw.interceptor.headersWritten = false
	drw.interceptor.statusCode = 200
	drw.interceptor.customHeaders = make(map[string]string)
	drw.interceptor.headersToDelete = make([]string, 0)
}

// SetBodyStream 设置 body 流
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

// SendFile 发送文件
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

// Redirect 重定向
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

// bufferPool body 缓冲区池
var bufferPool = sync.Pool{
	New: func() interface{} {
		return make([]byte, 0, 4096) // 4KB 初始容量
	},
}

// acquireBuffer 获取缓冲区
func acquireBuffer() []byte {
	return bufferPool.Get().([]byte)
}

// releaseBuffer 释放缓冲区
func releaseBuffer(buf []byte) {
	if buf != nil && cap(buf) <= 65536 { // 只回收小缓冲区
		bufferPool.Put(buf[:0])
	}
}

// BufferedWriter 带缓冲的写入器
type BufferedWriter struct {
	buf       []byte
	size      int
	flushFunc func([]byte) error
	maxSize   int
	autoFlush bool
}

// NewBufferedWriter 创建缓冲写入器
func NewBufferedWriter(maxSize int, flushFunc func([]byte) error) *BufferedWriter {
	return &BufferedWriter{
		buf:       acquireBuffer(),
		maxSize:   maxSize,
		flushFunc: flushFunc,
		autoFlush: true,
	}
}

// Write 写入数据
func (bw *BufferedWriter) Write(p []byte) (int, error) {
	if bw.buf == nil {
		bw.buf = acquireBuffer()
	}

	// 检查是否需要扩容
	if len(bw.buf)+len(p) > cap(bw.buf) {
		// 扩容
		newCap := cap(bw.buf) * 2
		if newCap < len(bw.buf)+len(p) {
			newCap = len(bw.buf) + len(p)
		}
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

// Flush 刷新缓冲区
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

// Close 关闭写入器
func (bw *BufferedWriter) Close() error {
	err := bw.Flush()
	if bw.buf != nil {
		releaseBuffer(bw.buf)
		bw.buf = nil
	}
	return err
}

// Size 返回当前缓冲区大小
func (bw *BufferedWriter) Size() int {
	return len(bw.buf)
}

// Bytes 返回当前缓冲区内容（不消费）
func (bw *BufferedWriter) Bytes() []byte {
	return bw.buf
}
