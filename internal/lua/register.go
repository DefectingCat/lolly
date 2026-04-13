// Package lua 提供 Lua API 注册辅助函数
package lua

import glua "github.com/yuin/gopher-lua"

// APIMethod 表示一个 Lua API 方法
type APIMethod struct {
	Name string
	Func func(*glua.LState) int
}

// RegisterAPIMethods 批量注册 API 方法到 Lua 表
func RegisterAPIMethods(L *glua.LState, tbl *glua.LTable, methods []APIMethod) {
	for _, m := range methods {
		tbl.RawSetString(m.Name, L.NewFunction(m.Func))
	}
}

// RegisterUnsafeAPI 注册不可用于 timer context 的安全桩
func RegisterUnsafeAPI(L *glua.LState, ngx *glua.LTable, apiName string, methods []string) {
	tbl := L.NewTable()
	for _, m := range methods {
		tbl.RawSetString(m, L.NewFunction(func(L *glua.LState) int {
			L.RaiseError("API %s.%s not available in timer callback context", apiName, m)
			return 0
		}))
	}
	ngx.RawSetString(apiName, tbl)
}
