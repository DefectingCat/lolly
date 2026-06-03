package limitrate

import (
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


