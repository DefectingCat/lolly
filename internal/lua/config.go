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
	CoroutineStackSize      int  // 协程栈大小（默认64，最大256）
	CoroutinePoolWarmup     int  // 协程池预热数量，启动时预创建
	EnableFileWatch         bool // 1
	EnableOSLib             bool // 1
	EnableIOLib             bool // 1
	EnableLoadLib           bool // 1
	MinimizeStackMemory     bool // 启用栈内存自动收缩以减少内存占用
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
		CoroutineStackSize:      64, // 优化：较小的栈减少内存分配
		MinimizeStackMemory:     true,
		CoroutinePoolWarmup:     4, // 预热4个协程结构
	}
}
