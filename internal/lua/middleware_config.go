// Package lua 提供 Lua 中间件配置解析与验证。
//
// 该文件定义从配置文件（YAML）加载的中间件配置结构，包括：
//   - MiddlewareConfig：完整的中间件配置（含脚本列表、全局设置、启用标记）
//   - ScriptConfig：单个脚本的配置（路径、阶段、超时、启用标记）
//   - GlobalLuaSettings：全局 Lua 引擎设置（并发、缓存、栈大小等）
//   - ParsePhase：字符串到 Phase 常量的转换
//   - ToEngineConfig：将配置文件的设置转换为引擎配置
//
// 注意事项：
//   - Phase 值必须为：rewrite、access、content、log、header_filter、body_filter
//   - 超时时间至少为 1 秒
//   - 最大并发协程数至少为 1
//
// 作者：xfy
package lua

import (
	"fmt"
	"time"
)

// MiddlewareConfig Lua 中间件配置（配置文件格式）
type MiddlewareConfig struct {
	Scripts        []ScriptConfig    `yaml:"scripts"`
	GlobalSettings GlobalLuaSettings `yaml:"global_settings"`
	Enabled        bool              `yaml:"enabled"`
}

// ScriptConfig 单个脚本配置
type ScriptConfig struct {
	// Path 脚本路径
	Path string `yaml:"path"`

	// Phase 执行阶段
	// 可选值：rewrite、access、content、log、header_filter、body_filter
	Phase string `yaml:"phase"`

	// Timeout 执行超时
	Timeout time.Duration `yaml:"timeout"`

	// Enabled 是否启用此脚本（默认 true）
	Enabled bool `yaml:"enabled"`
}

// GlobalLuaSettings 全局 Lua 设置
type GlobalLuaSettings struct {
	// MaxConcurrentCoroutines 最大并发协程数
	MaxConcurrentCoroutines int `yaml:"max_concurrent_coroutines"`

	// CoroutineTimeout 协程执行超时
	CoroutineTimeout time.Duration `yaml:"coroutine_timeout"`

	// CodeCacheSize 字节码缓存条目数
	CodeCacheSize int `yaml:"code_cache_size"`

	// MaxExecutionTime 单脚本最大执行时间
	MaxExecutionTime time.Duration `yaml:"max_execution_time"`

	// CoroutineStackSize 协程栈大小（默认64，最大256）
	// 较小的栈减少内存分配，适用于简单脚本
	CoroutineStackSize int `yaml:"coroutine_stack_size"`

	// CoroutinePoolWarmup 协程池预热数量，启动时预创建
	CoroutinePoolWarmup int `yaml:"coroutine_pool_warmup"`

	// EnableFileWatch 启用文件变更检测
	EnableFileWatch bool `yaml:"enable_file_watch"`

	// MinimizeStackMemory 启用栈内存自动收缩以减少内存占用
	MinimizeStackMemory bool `yaml:"minimize_stack_memory"`
}

// ParsePhase 将字符串转换为 Phase 常量
func ParsePhase(s string) (Phase, error) {
	switch s {
	case "rewrite":
		return PhaseRewrite, nil
	case "access":
		return PhaseAccess, nil
	case "content":
		return PhaseContent, nil
	case "log":
		return PhaseLog, nil
	case "header_filter":
		return PhaseHeaderFilter, nil
	case "body_filter":
		return PhaseBodyFilter, nil
	default:
		return PhaseInit, fmt.Errorf("unknown phase: %s", s)
	}
}


