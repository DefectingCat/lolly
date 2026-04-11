// Package lua 提供 Lua 脚本嵌入能力
package lua

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/valyala/fasthttp"
	glua "github.com/yuin/gopher-lua"
)

// Phase 处理阶段
type Phase int

// 处理阶段常量，对应 nginx 请求处理生命周期
const (
	// PhaseInit 初始化阶段
	PhaseInit Phase = iota
	// PhaseRewrite 重写阶段
	PhaseRewrite
	// PhaseAccess 访问控制阶段
	PhaseAccess
	// PhaseContent 内容生成阶段
	PhaseContent
	// PhaseLog 日志记录阶段
	PhaseLog
	// PhaseHeaderFilter 响应头过滤阶段
	PhaseHeaderFilter
	// PhaseBodyFilter 响应体过滤阶段
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
//
// 类型命名说明：虽然 lua.LuaCoroutine 存在 stuttering，但保持此命名以：
// 1) 与 LuaEngine/LuaContext 保持一致的 API 命名风格
// 2) 明确区分 Lua 运行时协程与 Go 协程概念
// 3) 保持向后兼容性
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

	// Layer 1 & 2: 设置安全的协程库（移除危险函数）
	c.setupSecureCoroutineLib()

	// Layer 3: 设置 ngx API
	c.setupNgxAPI()

	return nil
}

// setupSecureCoroutineLib 创建安全的协程库替换
// 移除 coroutine.create/wrap/resume，仅保留 yield/status
func (c *LuaCoroutine) setupSecureCoroutineLib() {
	// 获取原始 coroutine 表
	originalCoroutine := c.Engine.L.GetGlobal("coroutine")
	if originalCoroutine == glua.LNil {
		return // coroutine 库未加载
	}

	origTable, ok := originalCoroutine.(*glua.LTable)
	if !ok {
		return
	}

	// 创建安全的 coroutine 表
	safeCoroutine := c.Co.NewTable()

	// 仅保留安全的函数：yield 和 status
	if yield := origTable.RawGetString("yield"); yield != glua.LNil {
		safeCoroutine.RawSetString("yield", yield)
	}
	if status := origTable.RawGetString("status"); status != glua.LNil {
		safeCoroutine.RawSetString("status", status)
	}

	// 拦截函数 - 返回友好错误
	blockFn := c.Co.NewFunction(func(L *glua.LState) int {
		L.RaiseError("coroutine creation is blocked in sandbox (use engine-provided coroutine instead)")
		return 0
	})
	safeCoroutine.RawSetString("create", blockFn)
	safeCoroutine.RawSetString("wrap", blockFn)
	safeCoroutine.RawSetString("resume", blockFn)
	safeCoroutine.RawSetString("running", blockFn) // 防止信息泄露

	// 替换协程的 coroutine 全局变量
	c.Co.SetGlobal("coroutine", safeCoroutine)

	// 注意：不修改引擎级全局表 origTable，避免并发竞态条件
	// _G.coroutine 的访问通过沙箱的 __index 元表机制被隔离
	// 因为协程继承的是引擎全局环境，而我们在协程级别设置了独立的 coroutine 表
}

// setupNgxAPI 创建 ngx API
// 注册 ngx.req、ngx.resp、ngx.var、ngx.ctx、ngx.log 和 ngx.socket API
func (c *LuaCoroutine) setupNgxAPI() {
	// 创建 ngx 表
	ngx := c.Co.NewTable()

	// 先设置到全局，让所有注册函数使用同一个 ngx 表
	c.Co.SetGlobal("ngx", ngx)

	// 注册 ngx.req API
	if c.RequestCtx != nil {
		reqAPI := newNgxReqAPI(c.RequestCtx)
		RegisterNgxReqAPI(c.Co, reqAPI, ngx)

		// 注册 ngx.resp API
		respAPI := newNgxRespAPI(c.RequestCtx)
		RegisterNgxRespAPI(c.Co, respAPI)

		// 注册 ngx.log API (logger 为 nil 时禁用日志输出)
		// ngx.say/print/flush 直接写入 RequestCtx
		logAPI := newNgxLogAPI(c.RequestCtx, nil, nil)
		RegisterNgxLogAPI(c.Co, logAPI)
	}

	// 注册 ngx.var API
	varAPI := newNgxVarAPI(c.RequestCtx)
	RegisterNgxVarAPI(c.Co, varAPI, ngx)

	// 注册 ngx.ctx API
	RegisterNgxCtxAPI(c.Co, ngx)

	// 注册 ngx.socket API
	RegisterTCPSocketAPI(c.Co, c.Engine)
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
