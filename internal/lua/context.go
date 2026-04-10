// Package lua 提供 Lua 脚本嵌入能力
package lua

import (
	"github.com/valyala/fasthttp"
)

// LuaContext 请求级 Lua 上下文
type LuaContext struct {
	// 引擎引用
	Engine *LuaEngine

	// 协程
	Coroutine *LuaCoroutine

	// HTTP 请求上下文
	RequestCtx *fasthttp.RequestCtx

	// 当前阶段
	Phase Phase

	// 变量存储（ngx.var 实现）
	Variables map[string]string

	// 输出缓冲
	OutputBuffer []byte

	// 是否已退出
	Exited bool
}

// NewContext 创建请求上下文
func NewContext(engine *LuaEngine, req *fasthttp.RequestCtx) *LuaContext {
	return &LuaContext{
		Engine:     engine,
		RequestCtx: req,
		Variables:  make(map[string]string),
		Phase:      PhaseInit,
	}
}

// InitCoroutine 初始化协程
func (c *LuaContext) InitCoroutine() error {
	coro, err := c.Engine.NewCoroutine(c.RequestCtx)
	if err != nil {
		return err
	}
	c.Coroutine = coro
	return c.Coroutine.SetupSandbox()
}

// Execute 执行 Lua 脚本
func (c *LuaContext) Execute(script string) error {
	if c.Coroutine == nil {
		if err := c.InitCoroutine(); err != nil {
			return err
		}
	}
	return c.Coroutine.Execute(script)
}

// ExecuteFile 执行文件脚本
func (c *LuaContext) ExecuteFile(path string) error {
	if c.Coroutine == nil {
		if err := c.InitCoroutine(); err != nil {
			return err
		}
	}
	return c.Coroutine.ExecuteFile(path)
}

// SetPhase 设置当前阶段
func (c *LuaContext) SetPhase(phase Phase) {
	c.Phase = phase
}

// GetPhase 获取当前阶段
func (c *LuaContext) GetPhase() Phase {
	return c.Phase
}

// GetVariable 获取变量
func (c *LuaContext) GetVariable(name string) (string, bool) {
	val, ok := c.Variables[name]
	return val, ok
}

// SetVariable 设置变量
func (c *LuaContext) SetVariable(name, value string) {
	c.Variables[name] = value
}

// Write 输出内容
func (c *LuaContext) Write(data []byte) {
	c.OutputBuffer = append(c.OutputBuffer, data...)
}

// Say 输出内容并换行
func (c *LuaContext) Say(data string) {
	c.OutputBuffer = append(c.OutputBuffer, data...)
	c.OutputBuffer = append(c.OutputBuffer, '\n')
}

// Exit 退出请求处理
func (c *LuaContext) Exit(code int) {
	c.Exited = true
	c.RequestCtx.SetStatusCode(code)
}

// Release 释放资源
func (c *LuaContext) Release() {
	if c.Coroutine != nil {
		c.Coroutine.Close()
		c.Coroutine = nil
	}
	c.Variables = nil
	c.OutputBuffer = nil
}

// FlushOutput 刷新输出到响应
func (c *LuaContext) FlushOutput() {
	if len(c.OutputBuffer) > 0 && c.RequestCtx != nil {
		c.RequestCtx.Write(c.OutputBuffer)
		c.OutputBuffer = c.OutputBuffer[:0]
	}
}