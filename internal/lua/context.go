// Package lua 提供 Lua 脚本嵌入能力。
//
// 该文件包含请求级 Lua 上下文的实现，包括：
//   - LuaContext：请求级上下文，管理协程生命周期和输出缓冲
//   - 对象池：sync.Pool 复用 LuaContext 实例，减少 GC 压力
//   - 变量存储：每个请求独立的变量存储空间
//
// 注意事项：
//   - LuaContext 从对象池获取，使用后必须调用 Release() 放回
//   - Release() 会重置所有可变状态，防止请求间污染
//
// 作者：xfy
package lua

import (
	"sync"

	"github.com/valyala/fasthttp"
)

// LuaContext 请求级 Lua 上下文。
//
// 每个 HTTP 请求对应一个 LuaContext，负责：
//   - 管理请求级 Lua 协程（LuaCoroutine）的生命周期
//   - 维护请求级变量存储（Variables）
//   - 缓冲 Lua 脚本的输出内容（OutputBuffer）
//   - 跟踪请求处理阶段（Phase）和退出状态（Exited）
//
// 类型命名说明：虽然 lua.LuaContext 存在 stuttering，但保持此命名以：
// 1) 与 LuaEngine/LuaCoroutine 保持一致的 API 命名风格
// 2) 明确区分 Lua 上下文与其他上下文类型（如 context.Context）
// 3) 保持向后兼容性
type LuaContext struct {
	// Engine 所属 Lua 引擎
	Engine *LuaEngine

	// Coroutine 请求级 Lua 协程
	Coroutine *LuaCoroutine

	// RequestCtx fasthttp 请求上下文
	RequestCtx *fasthttp.RequestCtx

	// Variables 自定义变量存储
	Variables map[string]string

	// OutputBuffer 输出缓冲，存储 ngx.say/print 的内容
	OutputBuffer []byte

	// Phase 当前处理阶段
	Phase Phase

	// Exited 是否已通过 ngx.exit 退出
	Exited bool
}

// luaContextPool LuaContext 对象池。
//
// 使用 sync.Pool 复用 LuaContext 实例，减少频繁创建/销毁带来的 GC 压力。
// 从池中获取的实例必须在 Release() 中完全重置状态。
var luaContextPool = sync.Pool{
	New: func() any {
		return &LuaContext{
			Variables: make(map[string]string),
		}
	},
}

// AcquireContext 从对象池中获取并初始化 LuaContext。
//
// 参数：
//   - engine: Lua 引擎实例
//   - req: fasthttp 请求上下文
//
// 返回值：
//   - *LuaContext: 已初始化的上下文实例
//
// 注意：使用后必须调用 Release() 放回池中。
func AcquireContext(engine *LuaEngine, req *fasthttp.RequestCtx) *LuaContext {
	v := luaContextPool.Get()
	lc, ok := v.(*LuaContext)
	if !ok {
		// Pool 的 New 函数返回 *LuaContext，类型断言不应失败
		// 如果失败说明 Pool 被错误使用，panic 是合理的
		panic("luaContextPool returned unexpected type")
	}
	lc.Engine = engine
	lc.RequestCtx = req
	lc.Phase = PhaseInit
	// Variables 和 OutputBuffer 已在 Release 中重置
	return lc
}

// NewContext 创建请求上下文（从池中获取）。
//
// 该函数是 AcquireContext 的别名，保持向后兼容。
func NewContext(engine *LuaEngine, req *fasthttp.RequestCtx) *LuaContext {
	return AcquireContext(engine, req)
}

// InitCoroutine 初始化请求级 Lua 协程。
//
// 从引擎创建新协程并设置沙箱环境。
// 如果协程已存在则跳过创建。
//
// 返回值：
//   - error: 协程创建或沙箱设置失败时返回错误
func (c *LuaContext) InitCoroutine() error {
	coro, err := c.Engine.NewCoroutine(c.RequestCtx)
	if err != nil {
		return err
	}
	c.Coroutine = coro
	return c.Coroutine.SetupSandbox()
}

// Execute 执行 Lua 脚本字符串。
//
// 如果协程未初始化，会先自动调用 InitCoroutine()。
//
// 参数：
//   - script: Lua 源代码字符串
//
// 返回值：
//   - error: 编译或执行失败时返回错误
func (c *LuaContext) Execute(script string) error {
	if c.Coroutine == nil {
		if err := c.InitCoroutine(); err != nil {
			return err
		}
	}
	return c.Coroutine.Execute(script)
}

// ExecuteFile 执行 Lua 脚本文件。
//
// 如果协程未初始化，会先自动调用 InitCoroutine()。
//
// 参数：
//   - path: Lua 脚本文件路径
//
// 返回值：
//   - error: 编译或执行失败时返回错误
func (c *LuaContext) ExecuteFile(path string) error {
	if c.Coroutine == nil {
		if err := c.InitCoroutine(); err != nil {
			return err
		}
	}
	return c.Coroutine.ExecuteFile(path)
}

// SetPhase 设置当前请求处理阶段。
func (c *LuaContext) SetPhase(phase Phase) {
	c.Phase = phase
}

// GetPhase 获取当前请求处理阶段。
func (c *LuaContext) GetPhase() Phase {
	return c.Phase
}

// GetVariable 获取自定义变量的值。
//
// 返回值：
//   - string: 变量值，不存在时返回空字符串
//   - bool: 是否存在
func (c *LuaContext) GetVariable(name string) (string, bool) {
	val, ok := c.Variables[name]
	return val, ok
}

// SetVariable 设置自定义变量的值。
//
// 参数：
//   - name: 变量名
//   - value: 变量值
func (c *LuaContext) SetVariable(name, value string) {
	c.Variables[name] = value
}

// Write 将数据追加到输出缓冲区。
//
// 数据不会立即发送到客户端，需调用 FlushOutput() 才会刷新。
func (c *LuaContext) Write(data []byte) {
	c.OutputBuffer = append(c.OutputBuffer, data...)
}

// Say 将数据追加到输出缓冲区并附加换行符。
//
// 等效于 Write(data) + Write("\n")。
func (c *LuaContext) Say(data string) {
	c.OutputBuffer = append(c.OutputBuffer, data...)
	c.OutputBuffer = append(c.OutputBuffer, '\n')
}

// Exit 标记请求处理已退出，并设置 HTTP 状态码。
//
// 参数：
//   - code: HTTP 状态码
//
// 注意：调用此函数后，Existed 标记为 true，后续不会继续执行中间件链。
func (c *LuaContext) Exit(code int) {
	c.Exited = true
	c.RequestCtx.SetStatusCode(code)
}

// Release 释放协程资源、重置所有可变状态，并将上下文放回对象池。
//
// 该方法必须在请求处理结束时调用。
// 重置操作包括：
//  1. 关闭并清空协程引用
//  2. 清空 Variables map
//  3. 截断 OutputBuffer
//  4. 重置 Phase、Exited 标记
//  5. 清空 Engine 和 RequestCtx 引用
func (c *LuaContext) Release() {
	if c.Coroutine != nil {
		c.Coroutine.Close()
		c.Coroutine = nil
	}

	// 重置所有可变状态，防止请求间污染
	for k := range c.Variables {
		delete(c.Variables, k)
	}
	c.OutputBuffer = c.OutputBuffer[:0]
	c.Phase = PhaseInit
	c.Exited = false
	c.Engine = nil
	c.RequestCtx = nil

	luaContextPool.Put(c)
}

// FlushOutput 将输出缓冲区内容写入 HTTP 响应并清空缓冲。
//
// 如果缓冲区为空或 RequestCtx 为 nil，则不执行任何操作。
// 注意：写入错误被忽略，因为此阶段出错时无法向客户端报告。
func (c *LuaContext) FlushOutput() {
	if len(c.OutputBuffer) > 0 && c.RequestCtx != nil {
		// Write 返回写入的字节数和可能的错误
		// 在响应刷新场景中，我们选择忽略错误，因为：
		// 1. fasthttp.RequestCtx.Write 内部已经处理了连接状态
		// 2. 此阶段出错时请求处理已完成，无法向客户端报告
		_, _ = c.RequestCtx.Write(c.OutputBuffer)
		c.OutputBuffer = c.OutputBuffer[:0]
	}
}
