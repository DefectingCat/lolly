// Package lua 提供 Lua 脚本嵌入能力
package lua

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	glua "github.com/yuin/gopher-lua"
	"github.com/valyala/fasthttp"
)

// Phase 处理阶段
type Phase int

const (
	PhaseInit Phase = iota
	PhaseRewrite
	PhaseAccess
	PhaseContent
	PhaseLog
	PhaseHeaderFilter
	PhaseBodyFilter
)

func (p Phase) String() string {
	switch p {
	case PhaseInit:
		return "init"
	case PhaseRewrite:
		return "rewrite"
	case PhaseAccess:
		return "access"
	case PhaseContent:
		return "content"
	case PhaseLog:
		return "log"
	case PhaseHeaderFilter:
		return "header_filter"
	case PhaseBodyFilter:
		return "body_filter"
	default:
		return "unknown"
	}
}

// LuaCoroutine 请求级临时协程
// 注意：协程在 ResumeOK 后变成 dead 状态，不能复用
type LuaCoroutine struct {
	// 所属引擎
	Engine *LuaEngine

	// 协程 LState（通过 NewThread 创建）
	Co *glua.LState

	// 取消函数
	Cancel context.CancelFunc

	// 请求上下文
	RequestCtx *fasthttp.RequestCtx

	// 执行上下文
	ExecutionContext context.Context
	executionCancel  context.CancelFunc

	// 创建时间
	CreatedAt time.Time

	// 状态
	Exited bool // 是否已调用 exit

	// 输出缓冲
	OutputBuffer []byte
}

// SetupSandbox 创建 per-request _ENV 沙箱
// 每个请求创建独立的 _ENV 表，通过元表继承全局环境
func (c *LuaCoroutine) SetupSandbox() error {
	// 创建独立的 _ENV 表
	env := c.Co.NewTable()

	// 获取全局环境 - 使用 Engine 的主 LState 全局表
	// 协程通过 NewThread 继承了父 LState 的全局环境
	globals := c.Engine.L.GetGlobal("_G")

	// 设置元表，使未找到的变量从全局环境读取
	mt := c.Co.NewTable()
	mt.RawSetString("__index", globals)

	// 阻止写入全局环境（可选）
	readOnlyFn := c.Co.NewFunction(func(L *glua.LState) int {
		L.RaiseError("attempt to modify global table (read-only)")
		return 0
	})
	mt.RawSetString("__newindex", readOnlyFn)

	// 设置元表
	c.Co.SetMetatable(env, mt)

	// 将 _ENV 设置到协程
	c.Co.SetGlobal("_ENV", env)

	return nil
}

// Execute 在协程中执行 Lua 脚本（支持 Yield/Resume）
func (c *LuaCoroutine) Execute(script string) error {
	proto, err := c.Engine.codeCache.GetOrCompileInline(script)
	if err != nil {
		return fmt.Errorf("compile script: %w", err)
	}
	return c.executeProto(proto)
}

// ExecuteFile 执行文件脚本
func (c *LuaCoroutine) ExecuteFile(path string) error {
	proto, err := c.Engine.codeCache.GetOrCompileFile(path)
	if err != nil {
		return fmt.Errorf("compile file: %w", err)
	}
	return c.executeProto(proto)
}

// executeProto 执行编译后的字节码，处理 yield/resume 循环
func (c *LuaCoroutine) executeProto(proto *glua.FunctionProto) error {
	fn := c.Engine.L.NewFunctionFromProto(proto)
	st, execErr, values := c.Engine.L.Resume(c.Co, fn)

	for st == glua.ResumeYield {
		results, handleErr := c.handleYield(values)
		if handleErr != nil {
			return fmt.Errorf("handle yield: %w", handleErr)
		}
		st, execErr, values = c.Engine.L.Resume(c.Co, nil, results...)
	}

	if st == glua.ResumeError {
		atomic.AddUint64(&c.Engine.stats.ScriptsErrors, 1)
		return fmt.Errorf("lua execution error: %w", execErr)
	}

	atomic.AddUint64(&c.Engine.stats.ScriptsExecuted, 1)
	return nil
}

// handleYield 处理协程 yield
func (c *LuaCoroutine) handleYield(values []glua.LValue) ([]glua.LValue, error) {
	if len(values) == 0 {
		return nil, fmt.Errorf("yield without reason")
	}

	reason := glua.LVAsString(values[0])

	switch reason {
	case "sleep":
		return c.handleSleep(values[1:])
	default:
		return nil, fmt.Errorf("unknown yield reason: %s", reason)
	}
}

// handleSleep 处理 sleep yield
// 注意：此实现会阻塞当前 goroutine
func (c *LuaCoroutine) handleSleep(values []glua.LValue) ([]glua.LValue, error) {
	if len(values) == 0 {
		return nil, fmt.Errorf("sleep requires duration")
	}

	duration := float64(glua.LVAsNumber(values[0]))
	d := time.Duration(duration * float64(time.Second))

	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-timer.C:
		// sleep 完成，返回空结果
		return []glua.LValue{}, nil
	case <-c.ExecutionContext.Done():
		// 执行超时或取消
		return nil, fmt.Errorf("sleep interrupted: %w", c.ExecutionContext.Err())
	}
}

// Close 关闭协程
func (c *LuaCoroutine) Close() {
	c.Engine.releaseCoroutine(c)
}