// Package lua 提供 Lua 脚本嵌入能力。
//
// 该文件定义 Lua 引擎的配置结构及其默认值，包括：
//   - 并发控制：最大协程数、超时设置、协程栈大小
//   - 缓存配置：字节码缓存大小与 TTL
//   - 安全选项：OS/IO/Load 库的启用控制
//   - 内存优化：协程栈收缩、池预热
//
// 注意事项：
//   - 安全相关的库（OS、IO、LoadLib）默认禁用，防止不安全操作
//   - 协程栈默认 64KB，最大 256KB，较小的栈可减少内存分配
//   - 最大执行时间与协程超时默认均为 30 秒
//
// 作者：xfy
package lua

import (
	"time"
)

// Config Lua 引擎配置。
//
// 控制引擎的并发、缓存、安全和内存行为。
type Config struct {
	// MaxConcurrentCoroutines 最大并发协程数（默认 1000）
	MaxConcurrentCoroutines int

	// CoroutineTimeout 单个协程执行超时（默认 30 秒）
	CoroutineTimeout time.Duration

	// CodeCacheSize 字节码缓存最大条目数（默认 1000）
	CodeCacheSize int

	// CodeCacheTTL 字节码缓存生存时间（默认 1 小时）
	CodeCacheTTL time.Duration

	// MaxExecutionTime 最大执行时间（默认 30 秒）
	MaxExecutionTime time.Duration

	// CoroutineStackSize 协程栈大小（默认 64KB，最大 256KB）
	CoroutineStackSize int

	// CoroutinePoolWarmup 协程池预热数量，启动时预创建的协程结构数
	CoroutinePoolWarmup int

	// EnableFileWatch 是否启用文件变更检测（文件修改后自动重新编译）
	EnableFileWatch bool

	// EnableOSLib 是否启用 Lua os 库（默认禁用，出于安全考虑）
	EnableOSLib bool

	// EnableIOLib 是否启用 Lua io 库（默认禁用，出于安全考虑）
	EnableIOLib bool

	// EnableLoadLib 是否启用 Lua loadfile/dofile（默认禁用，出于安全考虑）
	EnableLoadLib bool

	// MinimizeStackMemory 启用栈内存自动收缩以减少内存占用
	MinimizeStackMemory bool
}

// DefaultConfig 返回默认配置。
//
// 安全策略：
//   - os、io、loadlib 库默认禁用，防止文件系统访问和动态代码加载
//   - 文件变更检测默认启用，便于开发时热更新
//   - 协程栈默认 64KB 以节省内存
//
// 返回值：
//   - *Config: 预填充的默认配置实例
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
