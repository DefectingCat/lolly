// Package lua 提供 Lua 脚本嵌入能力。
//
// 该文件包含请求级 Lua 协程的实现，包括：
//   - Phase：请求处理阶段常量（对应 nginx 生命周期）
//   - LuaCoroutine：请求级临时协程，每个请求独立创建
//   - 沙箱机制：隔离用户脚本，防止全局污染和危险操作
//   - ngx API 注册：为每个协程注册完整的 ngx.* API
//
// 注意事项：
//   - 协程在 ResumeOK 后变成 dead 状态，不能复用
//   - 每个协程拥有独立的 _ENV 沙箱环境
//   - 协程库被安全替换，阻止用户创建嵌套协程
//
// 作者：xfy
package lua

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/valyala/fasthttp"
	glua "github.com/yuin/gopher-lua"
)

// Phase 处理阶段。
//
// 对应 nginx 请求处理生命周期，Lua 脚本可在这些阶段中执行。
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

// LuaCoroutine 请求级临时协程。
//
// 每个 HTTP 请求创建一个独立的 LuaCoroutine，负责：
//   - 执行用户 Lua 脚本
//   - 管理 ngx.* API 实例（req、resp、var、log 等）
//   - 处理 yield/resume 循环（支持 sleep、cosocket 等异步操作）
//   - 维护沙箱环境（独立 _ENV，受限 coroutine 库）
//
// 注意：协程在 ResumeOK 后变成 dead 状态，不能复用
//
// 类型命名说明：虽然 lua.LuaCoroutine 存在 stuttering，但保持此命名以：
// 1) 与 LuaEngine/LuaContext 保持一致的 API 命名风格
// 2) 明确区分 Lua 运行时协程与 Go 协程概念
// 3) 保持向后兼容性
type LuaCoroutine struct {
	// CreatedAt 协程创建时间
	CreatedAt time.Time

	// ExecutionContext 执行上下文（含超时控制）
	ExecutionContext context.Context

	// ngx.req API 实例
	ngxReqAPI *ngxReqAPI

	// 请求上下文
	RequestCtx *fasthttp.RequestCtx

	// 底层 Lua 协程（gopher-lua LState）
	Co *glua.LState

	// ngx.var API 实例
	ngxVarAPI *ngxVarAPI

	// ngx.resp API 实例
	ngxRespAPI *ngxRespAPI

	// ngx.log API 实例
	ngxLogAPI *ngxLogAPI

	// Cancel 协程取消函数
	Cancel context.CancelFunc

	// executionCancel 执行超时取消函数
	executionCancel context.CancelFunc

	// 所属引擎
	Engine *LuaEngine

	// 输出缓冲
	OutputBuffer []byte

	// 退出标记（ngx.exit 触发）
	Exited bool
}

// SetupSandbox 创建 per-request _ENV 沙箱。
//
// 每个请求创建独立的 _ENV 表，通过元表继承全局环境。
// 安全层：
//   - Layer 1 & 2: 替换 coroutine 库，阻止 create/wrap/resume/running
//   - Layer 3: 注册 ngx.* API（req、resp、var、ctx、log、socket、shared、timer、location）
//
// 注意事项：
//   - 阻止写入全局环境（__newindex 返回错误）
//   - 不修改引擎级全局表，避免并发竞态条件
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

// setupSecureCoroutineLib 创建安全的协程库替换。
//
// 移除原始 coroutine 库中的危险函数（create、wrap、resume、running），
// 仅保留安全的 yield 和 status 函数。
// 被拦截的函数返回友好错误消息，而非直接崩溃。
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

// setupNgxAPI 创建并注册 ngx API 到协程环境。
//
// 注册以下 API 子模块：
//   - ngx.req：请求头/URI/方法/请求体操作
//   - ngx.resp：响应状态码/头操作
//   - ngx.var：nginx 变量访问和自定义变量
//   - ngx.ctx：请求级上下文 table
//   - ngx.log：日志输出
//   - ngx.socket：TCP cosocket
//   - ngx.shared：共享内存字典
//   - ngx.timer：定时器
//   - ngx.location：子请求
func (c *LuaCoroutine) setupNgxAPI() {
	// 创建 ngx 表
	ngx := c.Co.NewTable()

	// 先设置到全局，让所有注册函数使用同一个 ngx 表
	c.Co.SetGlobal("ngx", ngx)

	// 注册 ngx.req API
	if c.RequestCtx != nil {
		reqAPI := newNgxReqAPI(c.RequestCtx)
		c.ngxReqAPI = reqAPI
		RegisterNgxReqAPI(c.Co, reqAPI, ngx)

		// 注册 ngx.resp API
		respAPI := newNgxRespAPI(c.RequestCtx)
		c.ngxRespAPI = respAPI
		RegisterNgxRespAPI(c.Co, respAPI)

		// 注册 ngx.log API (logger 为 nil 时禁用日志输出)
		// ngx.say/print/flush 直接写入 RequestCtx
		logAPI := newNgxLogAPI(c.RequestCtx, nil, nil)
		c.ngxLogAPI = logAPI
		RegisterNgxLogAPI(c.Co, logAPI)
	}

	// 注册 ngx.var API
	varAPI := newNgxVarAPI(c.RequestCtx)
	c.ngxVarAPI = varAPI
	RegisterNgxVarAPI(c.Co, varAPI, ngx)

	// 注册 ngx.ctx API
	RegisterNgxCtxAPI(c.Co, ngx)

	// 注册 ngx.socket API
	RegisterTCPSocketAPI(c.Co, c.Engine)

	// 注册 ngx.shared.DICT API
	RegisterSharedDictAPI(c.Co, c.Engine.SharedDictManager(), ngx)

	// 注册 ngx.timer API
	RegisterTimerAPI(c.Co, c.Engine.TimerManager(), ngx)

	// 注册 ngx.location API
	RegisterLocationAPI(c.Co, c.Engine.LocationManager(), ngx)
}

// setupSchedulerNgxAPI 为 Scheduler LState 创建安全的 ngx API
// 仅注册在 timer callback 中安全的 API：ngx.shared, ngx.log, ngx.timer
// Unsafe APIs (ngx.req, ngx.resp, ngx.var, ngx.ctx, ngx.location) 会返回错误
//
//lint:ignore U1000 This function is kept for potential future use
func setupSchedulerNgxAPI(L *glua.LState, engine *LuaEngine) {
	// 创建 ngx 表
	ngx := L.NewTable()
	L.SetGlobal("ngx", ngx)

	// 设置 scheduler 模式标志（通过 userdata）
	setSchedulerMode(L, true)

	// 注册安全的 ngx.log API（不依赖 RequestCtx）
	// TODO: worker-2 should implement RegisterSchedulerLogAPI
	RegisterNgxLogAPI(L, nil)

	// 注册安全的 ngx.shared.DICT API
	RegisterSharedDictAPI(L, engine.SharedDictManager(), ngx)

	// 注册 ngx.timer API（允许在 timer 中创建新 timer）
	RegisterTimerAPI(L, engine.TimerManager(), ngx)

	// 注册不安全的 API（会检查 scheduler 模式并返回错误）
	// TODO: worker-2 should implement these functions
	// RegisterSchedulerUnsafeReqAPI(L, ngx)
	// RegisterSchedulerUnsafeRespAPI(L, ngx)
	// RegisterSchedulerUnsafeVarAPI(L, ngx)
	// RegisterSchedulerUnsafeCtxAPI(L, ngx)
	RegisterSchedulerUnsafeLocationAPI(L, ngx)
}

// schedulerModeKey 用于在 LState 的全局表中存储 scheduler 模式标志
const schedulerModeKey = "__scheduler_mode__"

// setSchedulerMode 设置 LState 的 scheduler 模式标志
//
//lint:ignore U1000 This function is kept for potential future use // kept for potential future use
func setSchedulerMode(L *glua.LState, enabled bool) {
	L.SetGlobal(schedulerModeKey, glua.LBool(enabled))
}

// IsSchedulerMode 检查 LState 是否处于 scheduler 模式。
//
// 用于在 API 函数中判断是否在 timer callback 上下文中。
// timer callback 环境下某些 API（如 ngx.req、ngx.ctx）不可用。
//
// 返回值：
//   - bool: true 表示处于 scheduler/timer 模式
func IsSchedulerMode(L *glua.LState) bool {
	value := L.GetGlobal(schedulerModeKey)
	if value == glua.LNil {
		return false
	}
	if b, ok := value.(glua.LBool); ok {
		return bool(b)
	}
	return false
}

// Execute 在协程中执行 Lua 脚本（支持 Yield/Resume）。
//
// 该函数从代码缓存中获取或编译内联脚本，然后执行。
//
// 参数：
//   - script: Lua 源代码字符串
//
// 返回值：
//   - error: 编译或执行失败时返回错误
func (c *LuaCoroutine) Execute(script string) error {
	proto, err := c.Engine.codeCache.GetOrCompileInline(script)
	if err != nil {
		return fmt.Errorf("compile script: %w", err)
	}
	return c.executeProto(proto)
}

// ExecuteFile 执行文件中的 Lua 脚本。
//
// 参数：
//   - path: Lua 脚本文件路径
//
// 返回值：
//   - error: 编译或执行失败时返回错误
func (c *LuaCoroutine) ExecuteFile(path string) error {
	proto, err := c.Engine.codeCache.GetOrCompileFile(path)
	if err != nil {
		return fmt.Errorf("compile file: %w", err)
	}
	return c.executeProto(proto)
}

// executeProto 执行编译后的字节码，处理 yield/resume 循环。
//
// 该函数是协程执行的核心循环：
//  1. 从 FunctionProto 创建 Lua 函数
//  2. Resume 执行协程
//  3. 如果 yield，调用 handleYield 处理并继续 Resume
//  4. 如果 error，记录统计并返回错误
//  5. 如果正常结束，更新执行计数
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

// GetNgxVarAPI 获取 ngx.var API 实例（用于测试和 Go 层访问）
func (c *LuaCoroutine) GetNgxVarAPI() *ngxVarAPI {
	return c.ngxVarAPI
}

// GetNgxReqAPI 获取 ngx.req API 实例（用于测试和 Go 层访问）
func (c *LuaCoroutine) GetNgxReqAPI() *ngxReqAPI {
	return c.ngxReqAPI
}

// GetNgxRespAPI 获取 ngx.resp API 实例（用于测试和 Go 层访问）
func (c *LuaCoroutine) GetNgxRespAPI() *ngxRespAPI {
	return c.ngxRespAPI
}

// GetNgxLogAPI 获取 ngx.log API 实例（用于测试和 Go 层访问）
func (c *LuaCoroutine) GetNgxLogAPI() *ngxLogAPI {
	return c.ngxLogAPI
}
