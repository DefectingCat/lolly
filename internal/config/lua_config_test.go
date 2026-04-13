// Package config 提供 Lua 配置测试
package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLuaMiddlewareConfigValidation 测试 Lua 配置验证
func TestLuaMiddlewareConfigValidation(t *testing.T) {
	// 未配置时跳过验证
	cfg := &LuaMiddlewareConfig{}
	require.NoError(t, validateLua(cfg))

	// 禁用时跳过验证
	cfg = &LuaMiddlewareConfig{Enabled: false}
	require.NoError(t, validateLua(cfg))

	// 启用但无脚本也允许
	cfg = &LuaMiddlewareConfig{Enabled: true}
	require.NoError(t, validateLua(cfg))

	// 有效配置
	cfg = &LuaMiddlewareConfig{
		Enabled: true,
		Scripts: []LuaScriptConfig{
			{Path: "/scripts/test.lua", Phase: "rewrite", Timeout: 10 * time.Second},
		},
		GlobalSettings: LuaGlobalSettings{
			MaxConcurrentCoroutines: 1000,
			CoroutineTimeout:        30 * time.Second,
			CodeCacheSize:           100,
			EnableFileWatch:         true,
			MaxExecutionTime:        30 * time.Second,
		},
	}
	require.NoError(t, validateLua(cfg))
}

// TestLuaMiddlewareConfigInvalidPhase 测试无效阶段
func TestLuaMiddlewareConfigInvalidPhase(t *testing.T) {
	cfg := &LuaMiddlewareConfig{
		Enabled: true,
		Scripts: []LuaScriptConfig{
			{Path: "/scripts/test.lua", Phase: "invalid_phase"},
		},
	}

	err := validateLua(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "scripts[0].phase 无效")
}

// TestLuaMiddlewareConfigMissingPath 测试缺少路径
func TestLuaMiddlewareConfigMissingPath(t *testing.T) {
	cfg := &LuaMiddlewareConfig{
		Enabled: true,
		Scripts: []LuaScriptConfig{
			{Phase: "rewrite"},
		},
	}

	err := validateLua(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "scripts[0].path 必填")
}

// TestLuaMiddlewareConfigNegativeTimeout 测试负超时
func TestLuaMiddlewareConfigNegativeTimeout(t *testing.T) {
	cfg := &LuaMiddlewareConfig{
		Enabled: true,
		Scripts: []LuaScriptConfig{
			{Path: "/scripts/test.lua", Phase: "rewrite", Timeout: -5 * time.Second},
		},
	}

	err := validateLua(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "timeout 不能为负数")
}

// TestLuaMiddlewareConfigGlobalSettingsValidation 测试全局设置验证
func TestLuaMiddlewareConfigGlobalSettingsValidation(t *testing.T) {
	// MaxConcurrentCoroutines 为负
	cfg := &LuaMiddlewareConfig{
		Enabled: true,
		GlobalSettings: LuaGlobalSettings{
			MaxConcurrentCoroutines: -1,
		},
	}
	err := validateLua(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "max_concurrent_coroutines 不能为负数")

	// CoroutineTimeout 为负
	cfg = &LuaMiddlewareConfig{
		Enabled: true,
		GlobalSettings: LuaGlobalSettings{
			CoroutineTimeout: -1 * time.Second,
		},
	}
	err = validateLua(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "coroutine_timeout 不能为负数")
}

// TestServerConfigLuaField 测试 ServerConfig 包含 Lua 字段
func TestServerConfigLuaField(t *testing.T) {
	cfg := &ServerConfig{
		Lua: &LuaMiddlewareConfig{
			Enabled: true,
			Scripts: []LuaScriptConfig{
				{Path: "/scripts/auth.lua", Phase: "access"},
			},
		},
	}

	require.NotNil(t, cfg.Lua)
	assert.True(t, cfg.Lua.Enabled)
	assert.Len(t, cfg.Lua.Scripts, 1)
	assert.Equal(t, "/scripts/auth.lua", cfg.Lua.Scripts[0].Path)
	assert.Equal(t, "access", cfg.Lua.Scripts[0].Phase)
}

// TestValidateServerWithLuaConfig 测试服务器验证包含 Lua
func TestValidateServerWithLuaConfig(t *testing.T) {
	// 有效 Lua 配置
	cfg := &ServerConfig{
		Listen: ":8080",
		Lua: &LuaMiddlewareConfig{
			Enabled: true,
			Scripts: []LuaScriptConfig{
				{Path: "/scripts/test.lua", Phase: "content"},
			},
		},
	}
	require.NoError(t, validateServer(cfg, false))

	// 无效 Lua 配置
	cfg = &ServerConfig{
		Listen: ":8080",
		Lua: &LuaMiddlewareConfig{
			Enabled: true,
			Scripts: []LuaScriptConfig{
				{Path: "", Phase: "content"}, // 空路径
			},
		},
	}
	err := validateServer(cfg, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "lua:")
}
