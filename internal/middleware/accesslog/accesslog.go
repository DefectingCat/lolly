// Package accesslog 提供访问日志中间件，记录每个请求的详细信息。
//
// 该文件包含访问日志相关的核心逻辑，包括：
//   - 请求方法和路径记录
//   - 响应状态码和大小记录
//   - 请求处理耗时记录
//   - 访问日志采样（按 sample_rate 比例记录成功请求）
//
// 使用示例：
//
//	accessLog := accesslog.New(cfg.Logging)
//	chain := middleware.NewChain(accessLog)
//
// 作者：xfy
package accesslog

import (
	"sync/atomic"
	"time"

	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/config"
	"rua.plus/lolly/internal/logging"
)

// AccessLog 访问日志中间件，记录请求方法、路径、状态码、响应大小和处理时间。
type AccessLog struct {
	// logger 日志记录器实例，用于输出访问日志
	logger *logging.Logger

	// sampleRate 采样率，范围 0.0-1.0
	// 1.0 表示记录所有请求
	sampleRate float64

	// sampleCounter 原子计数器，用于确定性采样
	// 每请求原子自增，与 1/sampleRate 取模决定是否记录
	sampleCounter atomic.Uint64

	// sampleInterval 采样间隔，由 sampleRate 计算得出
	// 例如 sampleRate=0.1 时 interval=10，每 10 个请求记录 1 个
	sampleInterval uint64
}

// New 创建访问日志中间件。
//
// 参数：
//   - cfg: 日志配置，包含输出路径、格式等设置
//
// 返回值：
//   - *AccessLog: 访问日志中间件实例
func New(cfg *config.LoggingConfig) *AccessLog {
	sampleRate := cfg.Access.SampleRate
	// sampleRate=0 明确表示禁用访问日志
	// sampleRate<0 或 >1 修正为 1.0（全量记录）
	if sampleRate < 0.0 || sampleRate > 1.0 {
		sampleRate = 1.0
	}

	var sampleInterval uint64 = 1
	if sampleRate > 0.0 && sampleRate < 1.0 {
		// 使用 1000 作为基数以提高精度，例如 0.123 -> 间隔约 8
		sampleInterval = uint64((1.0 / sampleRate) + 0.5)
		if sampleInterval < 1 {
			sampleInterval = 1
		}
	}

	return &AccessLog{
		logger:         logging.New(cfg),
		sampleRate:     sampleRate,
		sampleInterval: sampleInterval,
	}
}

// Name 返回中间件名称。
//
// 返回值：
//   - string: 中间件名称 "accesslog"
func (a *AccessLog) Name() string {
	return "accesslog"
}

// shouldLog 判断当前请求是否应记录访问日志。
//
// 规则：
//   - 5xx 服务器错误始终记录（便于排查错误）
//   - sampleRate=0 时不记录 2xx/3xx/4xx
//   - 采样率为 1.0 时始终记录
//   - 其他情况按 sampleRate 采样
//
// 使用原子计数器实现无锁、零分配采样。
func (a *AccessLog) shouldLog(status int) bool {
	// 5xx 服务器错误始终记录
	if status >= 500 {
		return true
	}
	if a.sampleRate == 0.0 {
		return false
	}
	if a.sampleRate >= 1.0 {
		return true
	}
	// 确定性采样：每 sampleInterval 个请求记录一个
	return a.sampleCounter.Add(1)%a.sampleInterval == 1
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
		status := ctx.Response.StatusCode()
		if !a.shouldLog(status) {
			return
		}
		duration := time.Since(start)
		a.logger.LogAccess(ctx, status, int64(len(ctx.Response.Body())), duration)
	}
}

// Close 关闭日志文件。
//
// 返回值：
//   - error: 关闭失败时返回错误，成功返回 nil
func (a *AccessLog) Close() error {
	return a.logger.Close()
}
