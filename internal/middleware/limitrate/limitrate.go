package limitrate

import (
	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/config"
)

const (
	// LargeFileStrategySkip 跳过大文件限速
	LargeFileStrategySkip = "skip"
	// LargeFileStrategyCoarse 粗粒度限速
	LargeFileStrategyCoarse = "coarse"
)

// Middleware 速率限制中间件
type Middleware struct {
	config *config.LimitRateConfig
}

// NewMiddleware 创建速率限制中间件
func NewMiddleware(cfg *config.LimitRateConfig) *Middleware {
	return &Middleware{config: cfg}
}

// Name 返回中间件名称
func (m *Middleware) Name() string {
	return "limit_rate"
}

// Process 处理请求
func (m *Middleware) Process(next fasthttp.RequestHandler) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		// 如果未配置限速，直接放行
		if m.config == nil || m.config.Rate <= 0 {
			next(ctx)
			return
		}

		// 包装响应写入器
		// 注意：fasthttp 的响应写入比较复杂，这里简化实现
		// 实际生产环境需要更精细的控制
		next(ctx)
	}
}
