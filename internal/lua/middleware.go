// Package lua 提供 Lua 中间件实现。
//
// 该文件包含 fasthttp 中间件的 Lua 集成，包括：
//   - LuaMiddleware：单阶段 Lua 中间件
//   - MultiPhaseLuaMiddleware：多阶段 Lua 中间件，支持在请求生命周期不同阶段执行不同脚本
//
// 中间件执行顺序（从外到内）：
//
//	rewrite -> access -> content -> header_filter -> body_filter -> log
//
// 注意事项：
//   - 中间件在协程创建失败时记录错误并继续执行下一处理器
//   - ngx.exit/ngx.redirect 导致的终止视为正常行为，不返回 500 错误
//
// 作者：xfy
package lua

import (
	"fmt"
	"strings"
	"time"

	"github.com/valyala/fasthttp"
)

// LuaMiddleware 单阶段 Lua 中间件。
//
// 包装 fasthttp.RequestHandler，在执行实际请求处理前运行指定的 Lua 脚本。
// 脚本在独立的协程中执行，拥有自己的沙箱环境。
type LuaMiddleware struct {
	// engine Lua 引擎实例
	engine *LuaEngine

	// scriptPath Lua 脚本文件路径
	scriptPath string

	// name 中间件名称
	name string

	// phase 执行阶段
	phase Phase

	// timeout 执行超时（当前未使用超时控制）
	timeout time.Duration

	// enabled 是否启用
	enabled bool
}

// LuaMiddlewareConfig Lua 中间件配置参数。
type LuaMiddlewareConfig struct {
	// ScriptPath Lua 脚本文件路径（必填）
	ScriptPath string

	// Name 中间件名称（为空时自动生成）
	Name string

	// Phase 执行阶段（默认为 PhaseContent）
	Phase Phase

	// Timeout 执行超时（默认为 30 秒）
	Timeout time.Duration

	// Enabled 是否启用（默认 true）
	Enabled bool

	// EnabledSet 是否显式设置了 Enabled（用于区分零值和显式 false）
	EnabledSet bool
}

// DefaultLuaMiddlewareConfig 返回 Lua 中间件的默认配置。
//
// 该函数提供一组合理的默认值，适用于大多数场景：
//   - Phase: PhaseContent（内容阶段执行）
//   - Timeout: 30 秒
//   - Enabled: true（默认启用）
//
// 返回值：
//   - LuaMiddlewareConfig: 包含默认值的配置结构体
func DefaultLuaMiddlewareConfig() LuaMiddlewareConfig {
	return LuaMiddlewareConfig{
		Phase:   PhaseContent,
		Timeout: 30 * time.Second,
		Enabled: true,
	}
}

// NewLuaMiddleware 创建 Lua 中间件实例。
//
// 参数：
//   - engine: Lua 引擎实例
//   - config: 中间件配置
//
// 返回值：
//   - *LuaMiddleware: 中间件实例
//   - error: 参数验证失败时返回错误
//
// 注意事项：
//   - ScriptPath 不能为空
//   - engine 不能为 nil
//   - Timeout 为零时自动设置为 30 秒默认值
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

// Name 返回中间件名称。
func (m *LuaMiddleware) Name() string {
	return m.name
}

// Process 包装请求处理器，注入 Lua 脚本执行逻辑。
//
// 执行流程：
//  1. 检查中间件是否启用，未启用则直接调用 next
//  2. 创建 Lua 上下文并设置阶段
//  3. 初始化协程（失败时记录错误并继续）
//  4. 执行 Lua 脚本文件
//  5. 处理 ngx.exit/ngx.redirect 导致的终止（视为正常行为）
//  6. 非 ngx.exit 错误时设置 500 响应
//  7. 刷新输出缓冲
//  8. 如果脚本调用了 ngx.exit，不继续执行 next
//  9. 否则继续执行后续处理器
//
// 返回值：
//   - fasthttp.RequestHandler: 包装后的处理器
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

// MultiPhaseLuaMiddleware 多阶段 Lua 中间件。
//
// 支持在不同请求处理阶段执行不同的 Lua 脚本。
// 阶段按逆序包装，确保执行顺序为：
//
//	rewrite -> access -> content -> header_filter -> body_filter -> log
type MultiPhaseLuaMiddleware struct {
	// engine Lua 引擎实例
	engine *LuaEngine

	// phases 各阶段对应的单阶段中间件
	phases map[Phase]*LuaMiddleware

	// name 中间件名称
	name string
}

// NewMultiPhaseLuaMiddleware 创建多阶段 Lua 中间件。
//
// 参数：
//   - engine: Lua 引擎实例
//   - name: 中间件名称
//
// 返回值：
//   - *MultiPhaseLuaMiddleware: 初始化的多阶段中间件
func NewMultiPhaseLuaMiddleware(engine *LuaEngine, name string) *MultiPhaseLuaMiddleware {
	return &MultiPhaseLuaMiddleware{
		engine: engine,
		phases: make(map[Phase]*LuaMiddleware),
		name:   name,
	}
}

// Name 返回中间件名称。
func (m *MultiPhaseLuaMiddleware) Name() string {
	return m.name
}

// AddPhase 为指定阶段添加 Lua 脚本。
//
// 参数：
//   - phase: 处理阶段
//   - scriptPath: 脚本文件路径
//   - timeout: 执行超时
//
// 返回值：
//   - error: 中间件创建失败时返回错误
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

// Process 包装请求处理器，按逆序添加各阶段中间件。
//
// 执行顺序（从先到后）：
//
//	rewrite -> access -> content -> header_filter -> body_filter -> log
//
// 通过在包装链中逆序注册（从 log 开始），确保实际执行时先执行 rewrite。
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
