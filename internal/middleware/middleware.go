// Package middleware 提供 HTTP 中间件的基础接口和链式处理功能。
//
// 该文件包含中间件相关的核心定义，包括：
//   - Middleware 接口：定义中间件的标准方法
//   - Chain 结构体：实现中间件的链式调用
//
// 主要用途：
//   用于构建和管理 HTTP 请求处理中间件链，支持灵活的组合和顺序控制。
//
// 注意事项：
//   - 中间件按逆序包装，确保执行顺序与添加顺序一致
//   - 所有中间件应实现 Middleware 接口
//
// 作者：xfy
package middleware

import "github.com/valyala/fasthttp"

// Middleware 中间件接口，定义中间件的标准方法。
//
// 所有中间件必须实现此接口，提供名称和请求处理方法。
type Middleware interface {
	// Name 返回中间件名称，用于日志和调试。
	Name() string

	// Process 包装下一个请求处理器，返回包装后的处理器。
	//
	// 参数：
	//   - next: 下一个请求处理器
	//
	// 返回值：
	//   - fasthttp.RequestHandler: 包装后的请求处理器
	Process(next fasthttp.RequestHandler) fasthttp.RequestHandler
}

// Chain 中间件链，管理多个中间件的链式调用。
//
// 中间件按添加顺序执行，通过逆序包装实现。
type Chain struct {
	// middlewares 中间件列表，按添加顺序存储
	middlewares []Middleware
}

// NewChain 创建中间件链。
//
// 根据提供的中间件创建中间件链，中间件按传入顺序执行。
//
// 参数：
//   - middlewares: 中间件列表，按执行顺序传入
//
// 返回值：
//   - *Chain: 创建的中间件链
func NewChain(middlewares ...Middleware) *Chain {
	return &Chain{middlewares: middlewares}
}

// Apply 应用中间件链。
//
// 将中间件链应用到最终请求处理器上，通过逆序包装确保
// 中间件按添加顺序执行。
//
// 参数：
//   - final: 最终的请求处理器
//
// 返回值：
//   - fasthttp.RequestHandler: 包装后的请求处理器
//
// 执行顺序：
//   如果中间件链为 [A, B, C]，最终处理器为 H，则执行顺序为：
//   A -> B -> C -> H -> C -> B -> A
func (c *Chain) Apply(final fasthttp.RequestHandler) fasthttp.RequestHandler {
	handler := final
	for i := len(c.middlewares) - 1; i >= 0; i-- {
		handler = c.middlewares[i].Process(handler)
	}
	return handler
}
