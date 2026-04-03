package middleware

import "github.com/valyala/fasthttp"

// Middleware 中间件接口
type Middleware interface {
	Name() string
	Process(next fasthttp.RequestHandler) fasthttp.RequestHandler
}

// Chain 中间件链
type Chain struct {
	middlewares []Middleware
}

// NewChain 创建中间件链
func NewChain(middlewares ...Middleware) *Chain {
	return &Chain{middlewares: middlewares}
}

// Apply 应用中间件链（逆序包装）
func (c *Chain) Apply(final fasthttp.RequestHandler) fasthttp.RequestHandler {
	handler := final
	for i := len(c.middlewares) - 1; i >= 0; i-- {
		handler = c.middlewares[i].Process(handler)
	}
	return handler
}
