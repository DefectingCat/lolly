// Package tools 提供基准测试工具函数。
//
// 该文件提供标准化的基准测试上下文构造器，用于：
//   - 快速创建模拟的 fasthttp.RequestCtx
//   - 标准化测试数据大小
//   - 简化组件级基准测试编写
//
// 作者：xfy
package tools

import (
	"bytes"
	"math/rand"
	"net"
	"strconv"
	"time"

	"github.com/valyala/fasthttp"
)

// BenchmarkContext 提供标准化的基准测试上下文。
//
// 包含构造模拟请求所需的所有配置。
type BenchmarkContext struct {
	// randSrc 随机数生成器
	randSrc *rand.Rand

	// RequestSize 请求数据大小
	RequestSize TestDataSize

	// ResponseSize 响应数据大小
	ResponseSize TestDataSize

	// Concurrency 并发级别（用于并行测试）
	Concurrency int
}

// NewBenchmarkContext 创建基准测试上下文。
//
// 参数:
//   - reqSize: 请求数据大小
//   - respSize: 响应数据大小
//   - concurrency: 并发级别
//
// 返回值:
//   - *BenchmarkContext: 配置好的基准测试上下文
func NewBenchmarkContext(reqSize, respSize TestDataSize, concurrency int) *BenchmarkContext {
	return &BenchmarkContext{
		RequestSize:  reqSize,
		ResponseSize: respSize,
		Concurrency:  concurrency,
		randSrc:      rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// MockRequestCtx 构造模拟的 fasthttp.RequestCtx。
//
// 创建一个可用于基准测试的请求上下文，包含：
//   - 请求方法和路径
//   - 请求头
//   - 请求体
//   - 远程地址
//
// 参数:
//   - method: HTTP 方法 (GET, POST, etc.)
//   - path: 请求路径
//
// 返回值:
//   - *fasthttp.RequestCtx: 模拟的请求上下文
func (bc *BenchmarkContext) MockRequestCtx(method, path string) *fasthttp.RequestCtx {
	ctx := &fasthttp.RequestCtx{}

	// 设置请求行
	ctx.Request.Header.SetMethod(method)
	ctx.Request.SetRequestURI(path)

	// 设置远程地址
	ctx.Init(&fasthttp.Request{}, &net.TCPAddr{
		IP:   net.ParseIP("192.168.1.100"),
		Port: 12345,
	}, nil)

	// 设置请求体
	if bc.RequestSize > 0 {
		body := GenerateTestData(bc.RequestSize)
		ctx.Request.SetBody(body)
	}

	return ctx
}

// MockRequestCtxWithHeaders 构造带自定义请求头的模拟上下文。
//
// 参数:
//   - method: HTTP 方法
//   - path: 请求路径
//   - headers: 请求头键值对
//
// 返回值:
//   - *fasthttp.RequestCtx: 模拟的请求上下文
func (bc *BenchmarkContext) MockRequestCtxWithHeaders(method, path string, headers map[string]string) *fasthttp.RequestCtx {
	ctx := bc.MockRequestCtx(method, path)

	// 设置请求头
	for key, value := range headers {
		ctx.Request.Header.Set(key, value)
	}

	return ctx
}

// MockRequestCtxWithBody 构造带自定义请求体的模拟上下文。
//
// 参数:
//   - method: HTTP 方法
//   - path: 请求路径
//   - body: 请求体内容
//
// 返回值:
//   - *fasthttp.RequestCtx: 模拟的请求上下文
func (bc *BenchmarkContext) MockRequestCtxWithBody(method, path string, body []byte) *fasthttp.RequestCtx {
	ctx := bc.MockRequestCtx(method, path)
	ctx.Request.SetBody(body)
	return ctx
}

// MockRequestCtxWithIP 构造带指定 IP 的模拟上下文。
//
// 参数:
//   - method: HTTP 方法
//   - path: 请求路径
//   - ip: 客户端 IP 地址
//
// 返回值:
//   - *fasthttp.RequestCtx: 模拟的请求上下文
func (bc *BenchmarkContext) MockRequestCtxWithIP(method, path, ip string) *fasthttp.RequestCtx {
	ctx := &fasthttp.RequestCtx{}

	ctx.Request.Header.SetMethod(method)
	ctx.Request.SetRequestURI(path)

	ctx.Init(&fasthttp.Request{}, &net.TCPAddr{
		IP:   net.ParseIP(ip),
		Port: 12345,
	}, nil)

	return ctx
}

// MockResponse 构造模拟响应数据。
//
// 根据 ResponseSize 生成响应体。
//
// 返回值:
//   - []byte: 模拟的响应数据
func (bc *BenchmarkContext) MockResponse() []byte {
	return GenerateTestData(bc.ResponseSize)
}

// MockResponseWithContentType 构造带 Content-Type 的模拟响应。
//
// 参数:
//   - statusCode: HTTP 状态码
//   - contentType: 内容类型
//
// 返回值:
//   - []byte: 模拟的响应数据
func (bc *BenchmarkContext) MockResponseWithContentType(_, _ string) []byte {
	// 返回响应体，调用者负责设置状态码和 Content-Type
	return GenerateTestData(bc.ResponseSize)
}

// RandomIP 生成随机 IP 地址。
//
// 用于测试多客户端场景。
//
// 返回值:
//   - string: 随机 IP 地址
func (bc *BenchmarkContext) RandomIP() string {
	return "192.168." + strconv.Itoa(bc.randSrc.Intn(256)) + "." + strconv.Itoa(bc.randSrc.Intn(256))
}

// RandomPath 生成随机请求路径。
//
// 参数:
//   - prefix: 路径前缀
//
// 返回值:
//   - string: 随机路径
func (bc *BenchmarkContext) RandomPath(prefix string) string {
	return prefix + "/" + strconv.Itoa(bc.randSrc.Intn(10000))
}

// MockJSONRequest 构造 JSON 格式的模拟请求。
//
// 参数:
//   - path: 请求路径
//   - data: JSON 数据
//
// 返回值:
//   - *fasthttp.RequestCtx: 模拟的请求上下文
func (bc *BenchmarkContext) MockJSONRequest(path string, data []byte) *fasthttp.RequestCtx {
	ctx := bc.MockRequestCtx("POST", path)
	ctx.Request.Header.SetContentType("application/json")
	ctx.Request.SetBody(data)
	return ctx
}

// MockFormRequest 构造表单格式的模拟请求。
//
// 参数:
//   - path: 请求路径
//   - form: 表单数据键值对
//
// 返回值:
//   - *fasthttp.RequestCtx: 模拟的请求上下文
func (bc *BenchmarkContext) MockFormRequest(path string, form map[string]string) *fasthttp.RequestCtx {
	ctx := bc.MockRequestCtx("POST", path)
	ctx.Request.Header.SetContentType("application/x-www-form-urlencoded")

	// 构造表单数据
	var buf bytes.Buffer
	first := true
	for key, value := range form {
		if !first {
			buf.WriteByte('&')
		}
		buf.WriteString(key)
		buf.WriteByte('=')
		buf.WriteString(value)
		first = false
	}
	ctx.Request.SetBody(buf.Bytes())

	return ctx
}

// SetResponse 设置模拟上下文的响应。
//
// 用于测试响应处理逻辑。
//
// 参数:
//   - ctx: 请求上下文
//   - statusCode: HTTP 状态码
//   - body: 响应体
func (bc *BenchmarkContext) SetResponse(ctx *fasthttp.RequestCtx, statusCode int, body []byte) {
	ctx.Response.SetStatusCode(statusCode)
	ctx.Response.SetBody(body)
}

// SetJSONResponse 设置 JSON 格式的模拟响应。
//
// 参数:
//   - ctx: 请求上下文
//   - statusCode: HTTP 状态码
//   - json: JSON 数据
func (bc *BenchmarkContext) SetJSONResponse(ctx *fasthttp.RequestCtx, statusCode int, json []byte) {
	ctx.Response.SetStatusCode(statusCode)
	ctx.Response.Header.SetContentType("application/json")
	ctx.Response.SetBody(json)
}

// DefaultBenchmarkContext 返回默认的基准测试上下文。
//
// 配置:
//   - RequestSize: 1KB
//   - ResponseSize: 1KB
//   - Concurrency: 1
//
// 返回值:
//   - *BenchmarkContext: 默认配置的上下文
func DefaultBenchmarkContext() *BenchmarkContext {
	return NewBenchmarkContext(Size1KB, Size1KB, 1)
}

// SmallRequestContext 返回小请求的基准测试上下文。
//
// 配置:
//   - RequestSize: 100B
//   - ResponseSize: 100B
//
// 返回值:
//   - *BenchmarkContext: 小请求配置的上下文
func SmallRequestContext() *BenchmarkContext {
	return NewBenchmarkContext(100, 100, 1)
}

// LargeRequestContext 返回大请求的基准测试上下文。
//
// 配置:
//   - RequestSize: 100KB
//   - ResponseSize: 100KB
//
// 返回值:
//   - *BenchmarkContext: 大请求配置的上下文
func LargeRequestContext() *BenchmarkContext {
	return NewBenchmarkContext(Size100KB, Size100KB, 1)
}

// HighConcurrencyContext 返回高并发基准测试上下文。
//
// 配置:
//   - Concurrency: 100
//
// 返回值:
//   - *BenchmarkContext: 高并发配置的上下文
func HighConcurrencyContext() *BenchmarkContext {
	return NewBenchmarkContext(Size1KB, Size1KB, 100)
}

// BenchmarkContextPool 提供基准测试上下文的池。
//
// 用于减少内存分配开销。
type BenchmarkContextPool struct {
	pool chan *BenchmarkContext
}

// NewBenchmarkContextPool 创建基准测试上下文池。
//
// 参数:
//   - size: 池大小
//
// 返回值:
//   - *BenchmarkContextPool: 上下文池
func NewBenchmarkContextPool(size int) *BenchmarkContextPool {
	p := &BenchmarkContextPool{
		pool: make(chan *BenchmarkContext, size),
	}
	// 预填充池
	for range size {
		p.pool <- DefaultBenchmarkContext()
	}
	return p
}

// Get 从池中获取基准测试上下文。
//
// 返回值:
//   - *BenchmarkContext: 基准测试上下文
func (p *BenchmarkContextPool) Get() *BenchmarkContext {
	select {
	case ctx := <-p.pool:
		return ctx
	default:
		return DefaultBenchmarkContext()
	}
}

// Put 将基准测试上下文放回池中。
//
// 参数:
//   - ctx: 基准测试上下文
func (p *BenchmarkContextPool) Put(ctx *BenchmarkContext) {
	select {
	case p.pool <- ctx:
	default:
		// 池已满，丢弃
	}
}
