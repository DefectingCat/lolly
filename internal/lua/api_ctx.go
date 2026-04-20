// Package lua 提供 ngx.ctx API 实现。
//
// 该文件实现 ngx.ctx 子模块，提供每请求独立的 Lua table 存储。
// ngx.ctx 是 OpenResty/ngx_lua 中的标准 API，允许在请求生命周期内
// 跨不同阶段（rewrite、access、content、log）共享数据。
//
// 注意事项：
//   - ngx.ctx 在 timer callback 上下文中不可用（通过 RegisterSchedulerUnsafeCtxAPI 拦截）
//
// 作者：xfy
package lua

import (
	glua "github.com/yuin/gopher-lua"
)

// RegisterNgxCtxAPI 在 Lua 状态机中注册 ngx.ctx API。
//
// ngx.ctx 是一个每请求独立的 Lua table，可通过 ngx.ctx.key 或 ngx.ctx[key]
// 读写任意 Lua 值类型（字符串、数字、table 等）。
//
// 参数：
//   - L: Lua 状态
//   - ngxTable: ngx 全局表
func RegisterNgxCtxAPI(L *glua.LState, ngxTable *glua.LTable) {
	// 创建请求级的 ctx table
	ctxTable := L.NewTable()

	// 将 ngx.ctx 添加到 ngx 表
	ngxTable.RawSetString("ctx", ctxTable)
}

// RegisterSchedulerUnsafeCtxAPI 为 Scheduler LState 注册不可用的 ngx.ctx API。
//
// 在 timer callback 等受限上下文中，ngx.ctx 不可用（无请求上下文）。
// 此函数将 ngx.ctx 的所有读写操作替换为返回错误的桩函数。
//
// 参数：
//   - L: Lua 状态
//   - ngx: ngx 全局表
func RegisterSchedulerUnsafeCtxAPI(L *glua.LState, ngx *glua.LTable) {
	ctxTable := L.NewTable()
	mt := L.NewTable()

	methods := []APIMethod{
		{Name: "__index", Func: luaSchedulerUnsafeCtx},
		{Name: "__newindex", Func: luaSchedulerUnsafeCtx},
	}
	RegisterAPIMethods(L, mt, methods)

	L.SetMetatable(ctxTable, mt)
	ngx.RawSetString("ctx", ctxTable)
}

// luaSchedulerUnsafeCtx 返回 scheduler 模式下 ngx.ctx 不可用的错误。
func luaSchedulerUnsafeCtx(L *glua.LState) int {
	L.RaiseError("API ngx.ctx not available in timer callback context")
	return 0
}
