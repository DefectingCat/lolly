package handler

import (
	"github.com/fasthttp/router"
	"github.com/valyala/fasthttp"
)

// Router 请求路由器
type Router struct {
	router *router.Router
}

// NewRouter 创建路由器
func NewRouter() *Router {
	return &Router{
		router: router.New(),
	}
}

// GET 注册 GET 路由
func (r *Router) GET(path string, handler fasthttp.RequestHandler) {
	r.router.GET(path, handler)
}

// POST 注册 POST 路由
func (r *Router) POST(path string, handler fasthttp.RequestHandler) {
	r.router.POST(path, handler)
}

// PUT 注册 PUT 路由
func (r *Router) PUT(path string, handler fasthttp.RequestHandler) {
	r.router.PUT(path, handler)
}

// DELETE 注册 DELETE 路由
func (r *Router) DELETE(path string, handler fasthttp.RequestHandler) {
	r.router.DELETE(path, handler)
}

// HEAD 注册 HEAD 路由
func (r *Router) HEAD(path string, handler fasthttp.RequestHandler) {
	r.router.HEAD(path, handler)
}

// Handler 返回路由处理器
func (r *Router) Handler() fasthttp.RequestHandler {
	return r.router.Handler
}