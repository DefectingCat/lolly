// Package lua 提供 Lua 脚本嵌入能力
// 采用 Server 级单 LState + 请求级临时协程架构
package lua

import (
	"time"
)

// Config Lua 引擎配置
type Config struct {
	// 协程配置
	MaxConcurrentCoroutines int           // 最大并发协程数（默认 1000）
	CoroutineTimeout        time.Duration // 协程执行超时（默认 30s）

	// 字节码缓存配置
	CodeCacheSize int           // 缓存条目数（默认 1000）
	CodeCacheTTL  time.Duration // 缓存过期时间（默认 1h）

	// 文件监控
	EnableFileWatch bool // 是否启用文件变更检测（默认 true）

	// 执行限制
	MaxExecutionTime time.Duration // 单脚本最大执行时间（默认 30s）

	// 安全设置
	EnableOSLib   bool // 是否加载 os 库（默认 false）
	EnableIOLib   bool // 是否加载 io 库（默认 false）
	EnableLoadLib bool // 是否允许 load/loadfile（默认 false）
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
