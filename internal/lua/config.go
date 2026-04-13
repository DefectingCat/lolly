// Package lua 提供 Lua 脚本嵌入能力
// 采用 Server 级单 LState + 请求级临时协程架构
package lua

import (
	"time"
)

// Config Lua 引擎配置
type Config struct {
	MaxConcurrentCoroutines int
	CoroutineTimeout        time.Duration
	CodeCacheSize           int
	CodeCacheTTL            time.Duration
	MaxExecutionTime        time.Duration
	EnableFileWatch         bool
	EnableOSLib             bool
	EnableIOLib             bool
	EnableLoadLib           bool
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		MaxConcurrentCoroutines: 1000,
		CoroutineTimeout:        30 * time.Second,
		CodeCacheSize:           1000,
		CodeCacheTTL:            time.Hour,
		EnableFileWatch:         true,
		MaxExecutionTime:        30 * time.Second,
		EnableOSLib:             false,
		EnableIOLib:             false,
		EnableLoadLib:           false,
	}
}
