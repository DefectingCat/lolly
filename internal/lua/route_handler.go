// Package lua 提供 Lua 脚本嵌入能力。
//
// 该文件包含 Lua 路由处理器的实现，用于将 Lua 脚本作为独立路由处理器执行。
// LuaRouteHandler 实现了 fasthttp.RequestHandler 接口，可以直接注册到路由。
//
// 作者：xfy
package lua

import (
	"fmt"
	"strings"
	"time"

	"github.com/valyala/fasthttp"
)

// LuaRouteHandler Lua 路由处理器。
//
// 将 Lua 脚本作为独立的 HTTP 路由处理器执行，支持：
//   - 独立的脚本路径和超时配置
//   - ngx.say/print 输出缓冲
//   - ngx.exit/ngx.redirect 特殊处理
//
// 该处理器直接注册到路由系统，不经过中间件链。
type LuaRouteHandler struct {
	// engine 所属 Lua 引擎
	engine *LuaEngine

	// scriptPath Lua 脚本路径
	scriptPath string

	// timeout 执行超时时间
	timeout time.Duration
}

// NewLuaRouteHandler 创建 Lua 路由处理器。
//
// 参数：
//   - engine: Lua 引擎实例
//   - scriptPath: Lua 脚本文件路径
//   - timeout: 执行超时时间（0 表示无限制）
//
// 返回值：
//   - *LuaRouteHandler: 路由处理器实例
func NewLuaRouteHandler(engine *LuaEngine, scriptPath string, timeout time.Duration) *LuaRouteHandler {
	return &LuaRouteHandler{
		engine:     engine,
		scriptPath: scriptPath,
		timeout:    timeout,
	}
}

// ServeHTTP 处理 HTTP 请求。
//
// 执行流程：
//  1. 创建请求级 LuaContext
//  2. 设置 Phase 为 PhaseContent
//  3. 初始化协程
//  4. 执行 Lua 脚本
//  5. 处理 ngx.exit/ngx.redirect 特殊退出
//  6. 刷新输出缓冲
//  7. 释放资源
//
// 错误处理：
//   - 协程初始化失败：返回 500 Internal Server Error
//   - 脚本执行失败（非 ngx.exit）：返回 500 Internal Server Error
//   - ngx.exit/ngx.redirect：正常退出，不视为错误
func (h *LuaRouteHandler) ServeHTTP(ctx *fasthttp.RequestCtx) {
	luaCtx := NewContext(h.engine, ctx)
	luaCtx.SetPhase(PhaseContent)

	if err := luaCtx.InitCoroutine(); err != nil {
		ctx.Error(fmt.Sprintf("lua coroutine init failed: %v", err), fasthttp.StatusInternalServerError)
		luaCtx.Release()
		return
	}

	err := luaCtx.ExecuteFile(h.scriptPath)

	// 检查是否为 ngx.exit 或 ngx.redirect 导致的"错误"
	// 这些实际上是正常的退出方式，不应视为错误
	isNgxExit := err != nil && (strings.Contains(err.Error(), "ngx.exit") ||
		strings.Contains(err.Error(), "ngx.redirect"))

	if isNgxExit {
		luaCtx.Exited = true
	}

	// 只有真正的错误才返回 500
	if err != nil && !isNgxExit && !luaCtx.Exited {
		ctx.Error(fmt.Sprintf("lua execution failed: %v", err), fasthttp.StatusInternalServerError)
	}

	luaCtx.FlushOutput()
	luaCtx.Release()
}