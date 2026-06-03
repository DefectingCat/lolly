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


