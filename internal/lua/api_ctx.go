// Package lua 提供 ngx.ctx API 实现
package lua

import (
	glua "github.com/yuin/gopher-lua"
)

// RegisterNgxCtxAPI 在 Lua 状态机中注册 ngx.ctx API
// ngx.ctx 是一个普通的 Lua table，每请求独立，支持任意 Lua 值类型
func RegisterNgxCtxAPI(L *glua.LState, ngxTable *glua.LTable) {
	// 创建请求级的 ctx table
	ctxTable := L.NewTable()

	// 将 ngx.ctx 添加到 ngx 表
	ngxTable.RawSetString("ctx", ctxTable)
}

// RegisterSchedulerUnsafeCtxAPI 为 Scheduler LState 注册不安全的 ngx.ctx API
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

func luaSchedulerUnsafeCtx(L *glua.LState) int {
	L.RaiseError("API ngx.ctx not available in timer callback context")
	return 0
}
