// Package lua 提供 ngx.ctx API 实现
package lua

import (
	glua "github.com/yuin/gopher-lua"
)

// ngxCtxAPI ngx.ctx API 实现
type ngxCtxAPI struct {
	// 每个请求独立的 ctx 表
	// 存储在协程的全局变量中
	ctxTable *glua.LTable
}

// newNgxCtxAPI 创建 ngx.ctx API 实例
func newNgxCtxAPI() *ngxCtxAPI {
	return &ngxCtxAPI{}
}

// RegisterNgxCtxAPI 在 Lua 状态机中注册 ngx.ctx API
// ngx.ctx 是一个普通的 Lua table，每请求独立，支持任意 Lua 值类型
func RegisterNgxCtxAPI(L *glua.LState, ngxTable *glua.LTable) {
	// 创建请求级的 ctx table
	ctxTable := L.NewTable()

	// 将 ngx.ctx 添加到 ngx 表
	ngxTable.RawSetString("ctx", ctxTable)
}

// GetCtxTable 获取 ctx table（用于内部访问）
func (api *ngxCtxAPI) GetCtxTable(L *glua.LState) *glua.LTable {
	ngx := L.GetGlobal("ngx")
	if ngx == glua.LNil {
		return nil
	}
	if ngxTable, ok := ngx.(*glua.LTable); ok {
		ctx := ngxTable.RawGetString("ctx")
		if ctxTable, ok := ctx.(*glua.LTable); ok {
			return ctxTable
		}
	}
	return nil
}

// SetValue 在 Go 层设置 ctx 值
func (api *ngxCtxAPI) SetValue(L *glua.LState, key string, value glua.LValue) {
	tb := api.GetCtxTable(L)
	if tb != nil {
		tb.RawSetString(key, value)
	}
}

// GetValue 在 Go 层获取 ctx 值
func (api *ngxCtxAPI) GetValue(L *glua.LState, key string) glua.LValue {
	tb := api.GetCtxTable(L)
	if tb != nil {
		return tb.RawGetString(key)
	}
	return glua.LNil
}

// RegisterSchedulerUnsafeCtxAPI 为 Scheduler LState 注册不安全的 ngx.ctx API
func RegisterSchedulerUnsafeCtxAPI(L *glua.LState, ngx *glua.LTable) {
	ctxTable := L.NewTable()
	mt := L.NewTable()
	mt.RawSetString("__index", L.NewFunction(luaSchedulerUnsafeCtx))
	mt.RawSetString("__newindex", L.NewFunction(luaSchedulerUnsafeCtx))
	L.SetMetatable(ctxTable, mt)
	ngx.RawSetString("ctx", ctxTable)
}

func luaSchedulerUnsafeCtx(L *glua.LState) int {
	L.RaiseError("API ngx.ctx not available in timer callback context")
	return 0
}
