// Package accesslog 提供访问日志中间件，记录每个请求的详细信息。
//
// 该文件包含访问日志相关的核心逻辑，包括：
//   - 请求方法和路径记录
//   - 响应状态码和大小记录
//   - 请求处理耗时记录
//
// 使用示例：
//
//	accessLog := accesslog.New(cfg.Logging)
//	chain := middleware.NewChain(accessLog)
//
// 作者：xfy
package accesslog

import (
	"time"

	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/config"
	"rua.plus/lolly/internal/logging"
)

// AccessLog 访问日志中间件，记录请求方法、路径、状态码、响应大小和处理时间。
type AccessLog struct {
	// logger 日志记录器实例，用于输出访问日志
	logger *logging.Logger
}

// New 创建访问日志中间件。
//
// 参数：
//   - cfg: 日志配置，包含输出路径、格式等设置
//
// 返回值：
//   - *AccessLog: 访问日志中间件实例
func New(cfg *config.LoggingConfig) *AccessLog {
	return &AccessLog{
		logger: logging.New(cfg),
	}
}

// Name 返回中间件名称。
//
// 返回值：
//   - string: 中间件名称 "accesslog"
func (a *AccessLog) Name() string {
	return "accesslog"
}

// Process 包装 handler，在请求处理后记录访问日志。
//
// 参数：
//   - next: 下一个请求处理器
//
// 返回值：
//   - fasthttp.RequestHandler: 包装后的请求处理器
func (a *AccessLog) Process(next fasthttp.RequestHandler) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		start := time.Now()
		next(ctx)
		duration := time.Since(start)
		a.logger.LogAccess(ctx, ctx.Response.StatusCode(), int64(len(ctx.Response.Body())), duration)
	}
}

// Close 关闭日志文件。
//
// 返回值：
//   - error: 关闭失败时返回错误，成功返回 nil
func (a *AccessLog) Close() error {
	return a.logger.Close()
}
