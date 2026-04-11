// Package lua 提供 Lua 中间件配置
package lua

import (
	"fmt"
	"time"
)

// MiddlewareConfig Lua 中间件配置（配置文件格式）
type MiddlewareConfig struct {
	// Enabled 是否启用 Lua 中间件
	Enabled bool `yaml:"enabled"`

	// Scripts 脚本配置列表
	Scripts []ScriptConfig `yaml:"scripts"`

	// GlobalSettings 全局设置
	GlobalSettings GlobalLuaSettings `yaml:"global_settings"`
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

	// EnableFileWatch 启用文件变更检测
	EnableFileWatch bool `yaml:"enable_file_watch"`

	// MaxExecutionTime 单脚本最大执行时间
	MaxExecutionTime time.Duration `yaml:"max_execution_time"`
}

// DefaultMiddlewareConfig 默认 Lua 中间件配置
func DefaultMiddlewareConfig() *MiddlewareConfig {
	return &MiddlewareConfig{
		Enabled: false,
		Scripts: []ScriptConfig{},
		GlobalSettings: GlobalLuaSettings{
			MaxConcurrentCoroutines: 1000,
			CoroutineTimeout:        30 * time.Second,
			CodeCacheSize:           1000,
			EnableFileWatch:         true,
			MaxExecutionTime:        30 * time.Second,
		},
	}
}

// Validate 验证 Lua 中间件配置
func (c *MiddlewareConfig) Validate() error {
	if !c.Enabled {
		return nil
	}

	// 验证脚本配置
	for i, script := range c.Scripts {
		if script.Path == "" {
			return fmt.Errorf("scripts[%d].path is required", i)
		}

		// 验证 Phase 值
		if err := validatePhase(script.Phase); err != nil {
			return fmt.Errorf("scripts[%d]: %w", i, err)
		}

		// 验证超时时间
		if script.Timeout > 0 && script.Timeout < time.Second {
			return fmt.Errorf("scripts[%d].timeout must be at least 1s", i)
		}
	}

	// 验证全局设置
	if c.GlobalSettings.MaxConcurrentCoroutines < 1 {
		return fmt.Errorf("global_settings.max_concurrent_coroutines must be at least 1")
	}

	if c.GlobalSettings.CoroutineTimeout > 0 && c.GlobalSettings.CoroutineTimeout < time.Second {
		return fmt.Errorf("global_settings.coroutine_timeout must be at least 1s")
	}

	return nil
}

// validatePhase 验证阶段值
func validatePhase(phase string) error {
	if phase == "" {
		return fmt.Errorf("phase is required")
	}

	validPhases := map[string]bool{
		"rewrite":       true,
		"access":        true,
		"content":       true,
		"log":           true,
		"header_filter": true,
		"body_filter":   true,
	}

	if !validPhases[phase] {
		return fmt.Errorf("invalid phase '%s', must be one of: rewrite, access, content, log, header_filter, body_filter", phase)
	}

	return nil
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

// ToEngineConfig 将全局设置转换为引擎配置
func (s *GlobalLuaSettings) ToEngineConfig() *Config {
	return &Config{
		MaxConcurrentCoroutines: s.MaxConcurrentCoroutines,
		CoroutineTimeout:        s.CoroutineTimeout,
		CodeCacheSize:           s.CodeCacheSize,
		CodeCacheTTL:            time.Hour, // 默认值
		EnableFileWatch:         s.EnableFileWatch,
		MaxExecutionTime:        s.MaxExecutionTime,
		EnableOSLib:             false, // 安全默认值
		EnableIOLib:             false,
		EnableLoadLib:           false,
	}
}
