// Package accesslog 提供访问日志中间件，记录每个请求的详细信息。
package accesslog

import (
	"time"

	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/config"
	"rua.plus/lolly/internal/logging"
)

// AccessLog 访问日志中间件，记录请求方法、路径、状态码、响应大小和处理时间。
type AccessLog struct {
	logger *logging.Logger
}

// New 创建访问日志中间件。
func New(cfg *config.LoggingConfig) *AccessLog {
	return &AccessLog{
		logger: logging.New(cfg),
	}
}

// Name 返回中间件名称。
func (a *AccessLog) Name() string {
	return "accesslog"
}

// Process 包装 handler，在请求处理后记录访问日志。
func (a *AccessLog) Process(next fasthttp.RequestHandler) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		start := time.Now()
		next(ctx)
		duration := time.Since(start)
		a.logger.LogAccess(ctx, ctx.Response.StatusCode(), int64(len(ctx.Response.Body())), duration)
	}
}

// Close 关闭日志文件。
func (a *AccessLog) Close() error {
	return a.logger.Close()
}