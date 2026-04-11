// Package lua 提供 Lua 中间件配置测试
package lua

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMiddlewareConfigValidation 测试配置验证
func TestMiddlewareConfigValidation(t *testing.T) {
	// 禁用时验证跳过
	cfg := DefaultMiddlewareConfig()
	require.NoError(t, cfg.Validate())

	// 启用但无脚本也允许
	cfg.Enabled = true
	require.NoError(t, cfg.Validate())

	// 有效脚本配置
	cfg.Scripts = []ScriptConfig{
		{Path: "/scripts/test.lua", Phase: "rewrite", Timeout: 10 * time.Second},
	}
	require.NoError(t, cfg.Validate())
}

// TestMiddlewareConfigInvalidPhase 测试无效阶段
func TestMiddlewareConfigInvalidPhase(t *testing.T) {
	cfg := DefaultMiddlewareConfig()
	cfg.Enabled = true
	cfg.Scripts = []ScriptConfig{
		{Path: "/scripts/test.lua", Phase: "invalid_phase"},
	}

	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid phase")
}

// TestMiddlewareConfigMissingPath 测试缺少路径
func TestMiddlewareConfigMissingPath(t *testing.T) {
	cfg := DefaultMiddlewareConfig()
	cfg.Enabled = true
	cfg.Scripts = []ScriptConfig{
		{Phase: "rewrite"},
	}

	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "path is required")
}

// TestMiddlewareConfigInvalidTimeout 测试无效超时
func TestMiddlewareConfigInvalidTimeout(t *testing.T) {
	cfg := DefaultMiddlewareConfig()
	cfg.Enabled = true
	cfg.Scripts = []ScriptConfig{
		{Path: "/scripts/test.lua", Phase: "rewrite", Timeout: 500 * time.Millisecond},
	}

	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "timeout must be at least 1s")
}

// TestParsePhase 测试阶段解析
func TestParsePhase(t *testing.T) {
	tests := []struct {
		input    string
		expected Phase
		hasError bool
	}{
		{"rewrite", PhaseRewrite, false},
		{"access", PhaseAccess, false},
		{"content", PhaseContent, false},
		{"log", PhaseLog, false},
		{"header_filter", PhaseHeaderFilter, false},
		{"body_filter", PhaseBodyFilter, false},
		{"invalid", PhaseInit, true},
		{"", PhaseInit, true},
	}

	for _, tt := range tests {
		phase, err := ParsePhase(tt.input)
		if tt.hasError {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
			assert.Equal(t, tt.expected, phase)
		}
	}
}

// TestGlobalLuaSettingsToEngineConfig 测试转换为引擎配置
func TestGlobalLuaSettingsToEngineConfig(t *testing.T) {
	settings := GlobalLuaSettings{
		MaxConcurrentCoroutines: 500,
		CoroutineTimeout:        20 * time.Second,
		CodeCacheSize:           200,
		EnableFileWatch:         false,
		MaxExecutionTime:        10 * time.Second,
	}

	engineCfg := settings.ToEngineConfig()
	assert.Equal(t, 500, engineCfg.MaxConcurrentCoroutines)
	assert.Equal(t, 20*time.Second, engineCfg.CoroutineTimeout)
	assert.Equal(t, 200, engineCfg.CodeCacheSize)
	assert.False(t, engineCfg.EnableFileWatch)
	assert.Equal(t, 10*time.Second, engineCfg.MaxExecutionTime)
	assert.False(t, engineCfg.EnableOSLib) // 安全默认值
	assert.False(t, engineCfg.EnableIOLib)
	assert.False(t, engineCfg.EnableLoadLib)
}

// TestMiddlewareConfigGlobalSettingsValidation 测试全局设置验证
func TestMiddlewareConfigGlobalSettingsValidation(t *testing.T) {
	cfg := DefaultMiddlewareConfig()
	cfg.Enabled = true
	cfg.GlobalSettings.MaxConcurrentCoroutines = 0

	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "max_concurrent_coroutines must be at least 1")
}
