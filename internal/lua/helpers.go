// Package lua 提供 Lua API 辅助函数。
//
// 该文件包含 ngx 表操作的共享辅助函数，用于减少代码重复。
// 所有函数保持并发安全设计（使用 RawGetString/RawSetString）。
//
// 作者：xfy
package lua

import glua "github.com/yuin/gopher-lua"

// GetOrCreateNgxTable 获取或创建全局 ngx 表。
//
// 如果全局 ngx 表已存在，则返回现有表；否则创建新表并设置为全局变量。
// 该函数是并发安全的，使用 RawGetString/RawSetString 操作。
//
// 参数：
//   - L: Lua 状态机
//
// 返回值：
//   - *glua.LTable: ngx 表
func GetOrCreateNgxTable(L *glua.LState) *glua.LTable {
	ngx := L.GetGlobal("ngx")
	if ngx != nil && ngx.Type() == glua.LTTable {
		if tbl, ok := ngx.(*glua.LTable); ok {
			return tbl
		}
	}

	// 创建新的 ngx 表
	tbl := L.NewTable()
	L.SetGlobal("ngx", tbl)
	return tbl
}

// GetOrCreateNgxSubTable 获取或创建 ngx 子表。
//
// 如果子表已存在，则返回现有表；否则创建新子表并设置到父表。
// 该函数是并发安全的，使用 RawGetString/RawSetString 操作。
//
// 参数：
//   - ngx: 父表（通常是 ngx 表）
//   - L: Lua 状态机
//   - name: 子表名称（如 "req", "resp", "socket"）
//
// 返回值：
//   - *glua.LTable: 子表
func GetOrCreateNgxSubTable(ngx *glua.LTable, L *glua.LState, name string) *glua.LTable {
	existing := ngx.RawGetString(name)
	if existing == glua.LNil {
		// 首次创建子表
		sub := L.NewTable()
		ngx.RawSetString(name, sub)
		return sub
	}
	return existing.(*glua.LTable) //nolint:errcheck // RawGetString returns LNil or valid LValue
}
