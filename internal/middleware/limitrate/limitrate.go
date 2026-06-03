// Package limitrate 提供基于令牌桶算法的请求速率限制功能。
//
// 包含速率限制器相关的逻辑，用于控制请求处理速率。
//
// 作者：xfy
package limitrate

import (
	"rua.plus/lolly/internal/config"
)

const (
	// LargeFileStrategySkip 大文件策略：跳过（不限制）。
	LargeFileStrategySkip = "skip"
	// LargeFileStrategyCoarse 大文件策略：粗略限制。
	LargeFileStrategyCoarse = "coarse"
)

// Middleware 速率限制中间件
type Middleware struct {
	config *config.LimitRateConfig
}


