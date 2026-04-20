// Package lua 提供 Lua API 注册辅助函数。
//
// 该文件提供批量注册 API 方法到 Lua 表的工具函数，包括：
//   - RegisterAPIMethods：批量注册方法到 Lua 表
//   - RegisterUnsafeAPI：注册不可用于 timer context 的安全桩函数
//
// 注意事项：
//   - RegisterUnsafeAPI 用于在 timer callback 等受限上下文中，
//     将不可用的 API 替换为返回错误的桩函数
//
// 作者：xfy
package lua

import glua "github.com/yuin/gopher-lua"

// APIMethod 表示一个 Lua API 方法。
type APIMethod struct {
	// Name 方法名（在 Lua 表中暴露的名称）
	Name string

	// Func Lua 函数实现
	Func func(*glua.LState) int
}

// RegisterAPIMethods 批量注册 API 方法到 Lua 表。
//
// 遍历方法列表，将每个方法注册为 Lua 函数并设置到目标表中。
//
// 参数：
//   - L: Lua 状态
//   - tbl: 目标 Lua 表
//   - methods: 要注册的方法列表
func RegisterAPIMethods(L *glua.LState, tbl *glua.LTable, methods []APIMethod) {
	for _, m := range methods {
		tbl.RawSetString(m.Name, L.NewFunction(m.Func))
	}
}

// RegisterUnsafeAPI 注册不可用于 timer context 的安全桩。
//
// 在 timer callback 等受限上下文中，某些 API（如 ngx.req、ngx.resp）不可用。
// 此函数将指定 API 的所有方法替换为返回错误的桩函数。
//
// 参数：
//   - L: Lua 状态
//   - ngx: ngx 表（父表）
//   - apiName: API 子模块名称（如 "req"、"resp"）
//   - methods: 要替换为桩的方法名列表
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
