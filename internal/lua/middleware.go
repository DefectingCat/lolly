// Package lua 提供 Lua 中间件实现
package lua

import (
	"fmt"
	"strings"
	"time"

	"github.com/valyala/fasthttp"
)

// LuaMiddleware Lua 中间件配置
type LuaMiddleware struct {
	engine     *LuaEngine
	scriptPath string
	name       string
	phase      Phase
	timeout    time.Duration
	enabled    bool
}

// LuaMiddlewareConfig Lua 中间件配置
type LuaMiddlewareConfig struct {
	ScriptPath string
	Name       string
	Phase      Phase
	Timeout    time.Duration
	Enabled    bool
	EnabledSet bool
}

// DefaultLuaMiddlewareConfig 默认配置
func DefaultLuaMiddlewareConfig() LuaMiddlewareConfig {
	return LuaMiddlewareConfig{
		Phase:   PhaseContent,
		Timeout: 30 * time.Second,
		Enabled: true,
	}
}

// NewLuaMiddleware 创建 Lua 中间件
func NewLuaMiddleware(engine *LuaEngine, config LuaMiddlewareConfig) (*LuaMiddleware, error) {
	if engine == nil {
		return nil, fmt.Errorf("lua engine is required")
	}

	if config.ScriptPath == "" {
		return nil, fmt.Errorf("script path is required")
	}

	// 设置默认值
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}

	// Enabled 默认值处理：
	// - EnabledSet 为 true 时，使用显式设置的 Enabled 值
	// - EnabledSet 为 false 时（零值），默认启用
	if !config.EnabledSet {
		config.Enabled = true
	}

	// 生成默认名称
	if config.Name == "" {
		config.Name = fmt.Sprintf("lua-%s", config.Phase.String())
	}

	return &LuaMiddleware{
		engine:     engine,
		scriptPath: config.ScriptPath,
		phase:      config.Phase,
		timeout:    config.Timeout,
		name:       config.Name,
		enabled:    config.Enabled,
	}, nil
}

// Name 返回中间件名称
func (m *LuaMiddleware) Name() string {
	return m.name
}

// Process 包装请求处理器
func (m *LuaMiddleware) Process(next fasthttp.RequestHandler) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		// 检查是否启用
		if !m.enabled {
			next(ctx)
			return
		}

		// 创建 Lua 上下文
		luaCtx := NewContext(m.engine, ctx)
		luaCtx.SetPhase(m.phase)

		// 初始化协程
		if err := luaCtx.InitCoroutine(); err != nil {
			// 协程创建失败，记录错误并继续
			ctx.Error(fmt.Sprintf("lua coroutine init failed: %v", err), fasthttp.StatusInternalServerError)
			luaCtx.Release()
			next(ctx)
			return
		}

		// 执行脚本
		err := luaCtx.ExecuteFile(m.scriptPath)

		// 检查是否为 ngx.exit/redirect 导致的终止（正常行为）
		// 这些 API 通过 RaiseError 终止执行，错误消息包含 "ngx.exit" 或 "ngx.redirect"
		isNgxExit := err != nil && (strings.Contains(err.Error(), "ngx.exit") ||
			strings.Contains(err.Error(), "ngx.redirect"))

		// 如果是 ngx.exit，手动设置 Exited 标记
		// 因为 setupNgxAPI 中 ngxLogAPI.luaCtx 为 nil，Exit() 只设置了 RequestCtx.StatusCode()
		if isNgxExit {
			luaCtx.Exited = true
		}

		// 只有非 ngx.exit/redirect 错误才设置错误响应
		if err != nil && !isNgxExit && !luaCtx.Exited {
			ctx.Error(fmt.Sprintf("lua execution failed: %v", err), fasthttp.StatusInternalServerError)
		}

		// 刷新输出缓冲
		luaCtx.FlushOutput()

		// 检查退出状态（在 Release 之前）
		exited := luaCtx.Exited

		// 释放资源
		luaCtx.Release()

		// 如果已退出，不再继续执行后续处理器
		if exited {
			return
		}

		// 继续执行后续处理器
		next(ctx)
	}
}

// SetEnabled 设置启用状态
func (m *LuaMiddleware) SetEnabled(enabled bool) {
	m.enabled = enabled
}

// SetPhase 设置执行阶段
func (m *LuaMiddleware) SetPhase(phase Phase) {
	m.phase = phase
	m.name = fmt.Sprintf("lua-%s", phase.String())
}

// SetTimeout 设置超时时间
func (m *LuaMiddleware) SetTimeout(timeout time.Duration) {
	m.timeout = timeout
}

// SetScriptPath 设置脚本路径
func (m *LuaMiddleware) SetScriptPath(path string) {
	m.scriptPath = path
}

// GetPhase 获取执行阶段
func (m *LuaMiddleware) GetPhase() Phase {
	return m.phase
}

// GetScriptPath 获取脚本路径
func (m *LuaMiddleware) GetScriptPath() string {
	return m.scriptPath
}

// IsEnabled 检查是否启用
func (m *LuaMiddleware) IsEnabled() bool {
	return m.enabled
}

// MultiPhaseLuaMiddleware 多阶段 Lua 中间件
// 支持在不同阶段执行不同的脚本
type MultiPhaseLuaMiddleware struct {
	// Lua 引擎
	engine *LuaEngine

	// 各阶段脚本配置
	phases map[Phase]*LuaMiddleware

	// 名称
	name string
}

// NewMultiPhaseLuaMiddleware 创建多阶段 Lua 中间件
func NewMultiPhaseLuaMiddleware(engine *LuaEngine, name string) *MultiPhaseLuaMiddleware {
	return &MultiPhaseLuaMiddleware{
		engine: engine,
		phases: make(map[Phase]*LuaMiddleware),
		name:   name,
	}
}

// Name 返回中间件名称
func (m *MultiPhaseLuaMiddleware) Name() string {
	return m.name
}

// AddPhase 添加阶段脚本
func (m *MultiPhaseLuaMiddleware) AddPhase(phase Phase, scriptPath string, timeout time.Duration) error {
	config := LuaMiddlewareConfig{
		ScriptPath: scriptPath,
		Phase:      phase,
		Timeout:    timeout,
		Name:       fmt.Sprintf("%s-%s", m.name, phase.String()),
		Enabled:    true, // 多阶段配置默认启用
		EnabledSet: true, // 显式设置
	}

	middleware, err := NewLuaMiddleware(m.engine, config)
	if err != nil {
		return err
	}

	m.phases[phase] = middleware
	return nil
}

// Process 包装请求处理器
func (m *MultiPhaseLuaMiddleware) Process(next fasthttp.RequestHandler) fasthttp.RequestHandler {
	handler := next

	// 按逆序包装，确保执行顺序正确
	// log -> body_filter -> header_filter -> content -> access -> rewrite
	phaseOrder := []Phase{
		PhaseLog,
		PhaseBodyFilter,
		PhaseHeaderFilter,
		PhaseContent,
		PhaseAccess,
		PhaseRewrite,
	}

	for _, phase := range phaseOrder {
		if middleware, ok := m.phases[phase]; ok {
			handler = middleware.Process(handler)
		}
	}

	return handler
}

// GetPhaseMiddleware 获取指定阶段的中间件
func (m *MultiPhaseLuaMiddleware) GetPhaseMiddleware(phase Phase) *LuaMiddleware {
	return m.phases[phase]
}

// RemovePhase 移除阶段脚本
func (m *MultiPhaseLuaMiddleware) RemovePhase(phase Phase) {
	delete(m.phases, phase)
}

// HasPhase 检查是否有指定阶段的脚本
func (m *MultiPhaseLuaMiddleware) HasPhase(phase Phase) bool {
	return m.phases[phase] != nil
}

// PhaseCount 返回阶段数量
func (m *MultiPhaseLuaMiddleware) PhaseCount() int {
	return len(m.phases)
}
