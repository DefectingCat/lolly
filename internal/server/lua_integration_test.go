// Package server 提供 buildLuaMiddlewares 的 Mock 测试
package server

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"rua.plus/lolly/internal/config"
	"rua.plus/lolly/internal/lua"
)

// TestBuildLuaMiddlewares_NilEngine 测试 LuaEngine 为 nil 时
func TestBuildLuaMiddlewares_NilEngine(t *testing.T) {
	cfg := &config.Config{
		Servers: []config.ServerConfig{
			{
				Listen: ":8080",
			},
		},
	}
	s := New(cfg)

	// 确保 luaEngine 为 nil
	s.luaEngine = nil

	luaCfg := &config.LuaMiddlewareConfig{
		Enabled: true,
		Scripts: []config.LuaScriptConfig{
			{
				Path:    "/test/script.lua",
				Phase:   "rewrite",
				Enabled: true,
				Timeout: 30 * time.Second,
			},
		},
	}

	middlewares, err := s.buildLuaMiddlewares(luaCfg)
	require.NoError(t, err)
	assert.Nil(t, middlewares)
}

// TestBuildLuaMiddlewares_InvalidPhase 测试无效阶段
func TestBuildLuaMiddlewares_InvalidPhase(t *testing.T) {
	cfg := &config.Config{
		Servers: []config.ServerConfig{
			{
				Listen: ":8080",
			},
		},
	}
	s := New(cfg)

	// 使用真实的 LuaEngine 来测试无效阶段
	engine, err := lua.NewEngine(lua.DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	s.luaEngine = engine

	luaCfg := &config.LuaMiddlewareConfig{
		Enabled: true,
		Scripts: []config.LuaScriptConfig{
			{
				Path:    "/test/script.lua",
				Phase:   "invalid_phase",
				Enabled: true,
				Timeout: 30 * time.Second,
			},
		},
	}

	_, err = s.buildLuaMiddlewares(luaCfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid phase")
}

// TestBuildLuaMiddlewares_WithTimeout 测试超时配置
func TestBuildLuaMiddlewares_WithTimeout(t *testing.T) {
	cfg := &config.Config{
		Servers: []config.ServerConfig{
			{
				Listen: ":8080",
			},
		},
	}
	s := New(cfg)

	engine, err := lua.NewEngine(lua.DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	s.luaEngine = engine

	// 测试自定义超时
	customTimeout := 60 * time.Second
	luaCfg := &config.LuaMiddlewareConfig{
		Enabled: true,
		Scripts: []config.LuaScriptConfig{
			{
				Path:    "/test/script.lua",
				Phase:   "rewrite",
				Enabled: true,
				Timeout: customTimeout,
			},
		},
	}

	middlewares, err := s.buildLuaMiddlewares(luaCfg)
	require.NoError(t, err)
	assert.Len(t, middlewares, 1)
}

// TestBuildLuaMiddlewares_EmptyScripts 测试空脚本列表
func TestBuildLuaMiddlewares_EmptyScripts(t *testing.T) {
	cfg := &config.Config{
		Servers: []config.ServerConfig{
			{
				Listen: ":8080",
			},
		},
	}
	s := New(cfg)

	engine, err := lua.NewEngine(lua.DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	s.luaEngine = engine

	luaCfg := &config.LuaMiddlewareConfig{
		Enabled: true,
		Scripts: []config.LuaScriptConfig{},
	}

	middlewares, err := s.buildLuaMiddlewares(luaCfg)
	require.NoError(t, err)
	assert.Empty(t, middlewares)
}

// TestBuildLuaMiddlewares_DisabledLua 测试禁用 Lua 配置
// buildLuaMiddlewares 函数本身不检查 Enabled 字段，由调用者检查
func TestBuildLuaMiddlewares_DisabledLua(t *testing.T) {
	// 创建临时脚本文件
	tempDir := t.TempDir()
	scriptPath := tempDir + "/test.lua"
	err := os.WriteFile(scriptPath, []byte("-- test"), 0o644)
	require.NoError(t, err)

	cfg := &config.Config{
		Servers: []config.ServerConfig{
			{
				Listen: ":8080",
			},
		},
	}
	s := New(cfg)

	engine, err := lua.NewEngine(lua.DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	s.luaEngine = engine

	// Enabled=false 但 buildLuaMiddlewares 本身不检查这个字段
	// 它会正常处理 Scripts 中的脚本
	luaCfg := &config.LuaMiddlewareConfig{
		Enabled: false,
		Scripts: []config.LuaScriptConfig{
			{
				Path:    scriptPath,
				Phase:   "rewrite",
				Enabled: true,
				Timeout: 30 * time.Second,
			},
		},
	}

	// 由于 Enabled 检查在调用者处，buildLuaMiddlewares 会正常执行
	middlewares, err := s.buildLuaMiddlewares(luaCfg)
	require.NoError(t, err)
	// 脚本文件存在，应该能创建中间件
	assert.Len(t, middlewares, 1)
}

// TestBuildLuaMiddlewares_DisabledScript 测试禁用特定脚本
func TestBuildLuaMiddlewares_DisabledScript(t *testing.T) {
	// 创建临时脚本文件（只有一个启用的脚本）
	tempDir := t.TempDir()
	enabledScriptPath := tempDir + "/enabled.lua"
	err := os.WriteFile(enabledScriptPath, []byte("-- enabled script"), 0o644)
	require.NoError(t, err)

	cfg := &config.Config{
		Servers: []config.ServerConfig{
			{
				Listen: ":8080",
			},
		},
	}
	s := New(cfg)

	engine, err := lua.NewEngine(lua.DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	s.luaEngine = engine

	luaCfg := &config.LuaMiddlewareConfig{
		Enabled: true,
		Scripts: []config.LuaScriptConfig{
			{
				Path:    enabledScriptPath,
				Phase:   "rewrite",
				Enabled: true,
				Timeout: 30 * time.Second,
			},
			{
				Path:    "/test/disabled.lua",
				Phase:   "access",
				Enabled: false, // 禁用的脚本应该被过滤
				Timeout: 30 * time.Second,
			},
		},
	}

	// 只有一个启用的脚本，应该能正常创建中间件
	middlewares, err := s.buildLuaMiddlewares(luaCfg)
	require.NoError(t, err)
	// 只有 rewrite 阶段的脚本被创建
	assert.Len(t, middlewares, 1)
}

// TestBuildLuaMiddlewares_WithExistingScript 测试使用存在的脚本文件
func TestBuildLuaMiddlewares_WithExistingScript(t *testing.T) {
	// 创建临时脚本文件
	scriptContent := `-- test script
ngx.var.uri = "/test"
`
	tempDir := t.TempDir()
	scriptPath := tempDir + "/test.lua"
	err := os.WriteFile(scriptPath, []byte(scriptContent), 0o644)
	require.NoError(t, err)

	cfg := &config.Config{
		Servers: []config.ServerConfig{
			{
				Listen: ":8080",
			},
		},
	}
	s := New(cfg)

	engine, err := lua.NewEngine(lua.DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	s.luaEngine = engine

	luaCfg := &config.LuaMiddlewareConfig{
		Enabled: true,
		Scripts: []config.LuaScriptConfig{
			{
				Path:    scriptPath,
				Phase:   "rewrite",
				Enabled: true,
				Timeout: 30 * time.Second,
			},
		},
	}

	middlewares, err := s.buildLuaMiddlewares(luaCfg)
	require.NoError(t, err)
	assert.Len(t, middlewares, 1)
}

// TestBuildLuaMiddlewares_MultiplePhases 测试多个不同阶段的脚本
func TestBuildLuaMiddlewares_MultiplePhases(t *testing.T) {
	// 创建临时脚本文件
	tempDir := t.TempDir()

	scripts := map[string]string{
		"rewrite.lua": `-- rewrite script
ngx.var.uri = "/rewrite"
`,
		"access.lua": `-- access script
-- access control logic
`,
		"content.lua": `-- content script
ngx.say("hello")
`,
	}

	scriptPaths := make(map[string]string)
	for name, content := range scripts {
		path := tempDir + "/" + name
		err := os.WriteFile(path, []byte(content), 0o644)
		require.NoError(t, err)
		scriptPaths[name] = path
	}

	cfg := &config.Config{
		Servers: []config.ServerConfig{
			{
				Listen: ":8080",
			},
		},
	}
	s := New(cfg)

	engine, err := lua.NewEngine(lua.DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	s.luaEngine = engine

	luaCfg := &config.LuaMiddlewareConfig{
		Enabled: true,
		Scripts: []config.LuaScriptConfig{
			{
				Path:    scriptPaths["rewrite.lua"],
				Phase:   "rewrite",
				Enabled: true,
				Timeout: 30 * time.Second,
			},
			{
				Path:    scriptPaths["access.lua"],
				Phase:   "access",
				Enabled: true,
				Timeout: 30 * time.Second,
			},
			{
				Path:    scriptPaths["content.lua"],
				Phase:   "content",
				Enabled: true,
				Timeout: 30 * time.Second,
			},
		},
	}

	middlewares, err := s.buildLuaMiddlewares(luaCfg)
	require.NoError(t, err)
	// 三个阶段应该创建三个中间件
	assert.Len(t, middlewares, 3)
}

// TestBuildLuaMiddlewares_DefaultTimeout 测试默认超时值
func TestBuildLuaMiddlewares_DefaultTimeout(t *testing.T) {
	// 创建临时脚本文件
	tempDir := t.TempDir()
	scriptPath := tempDir + "/test.lua"
	err := os.WriteFile(scriptPath, []byte("-- test"), 0o644)
	require.NoError(t, err)

	cfg := &config.Config{
		Servers: []config.ServerConfig{
			{
				Listen: ":8080",
			},
		},
	}
	s := New(cfg)

	engine, err := lua.NewEngine(lua.DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	s.luaEngine = engine

	// 不设置 Timeout，使用默认值
	luaCfg := &config.LuaMiddlewareConfig{
		Enabled: true,
		Scripts: []config.LuaScriptConfig{
			{
				Path:    scriptPath,
				Phase:   "rewrite",
				Enabled: true,
				// Timeout 为 0，应该使用默认值 30s
				Timeout: 0,
			},
		},
	}

	middlewares, err := s.buildLuaMiddlewares(luaCfg)
	require.NoError(t, err)
	assert.Len(t, middlewares, 1)
}
