// Package gjson provides a high-performance JSON encoding/decoding library for gopher-lua.
//
// This package is API-compatible with lua-cjson, allowing easy migration from OpenResty.
// It uses goccy/go-json as the underlying JSON engine for maximum performance.
//
// Basic usage:
//
//	L := glua.NewState()
//	defer L.Close()
//	gjson.Preload(L)
//
//	err := L.DoString(`
//	    local gjson = require("gjson")
//	    local data = {name = "Alice", age = 30}
//	    local json_str = gjson.encode(data)
//	    local decoded = gjson.decode(json_str)
//	`)
//
// The package supports:
//   - Full lua-cjson API compatibility
//   - Sparse array detection and handling
//   - Maximum nesting depth control
//   - Number precision control
//   - Independent configuration instances via gjson.new()
//
// 作者：xfy
package gjson

import (
	glua "github.com/yuin/gopher-lua"
)

const (
	// ModuleName 模块名称。
	ModuleName = "gjson"

	// Version 模块版本号。
	Version = "1.0.0"
)

// Preload registers the gjson module as a preload in the given LState.
// This allows Lua scripts to use `local gjson = require("gjson")`.
func Preload(L *glua.LState) {
	L.PreloadModule(ModuleName, Loader)
}

// Loader is the module loader function called by require("gjson").
func Loader(L *glua.LState) int {
	// Create the gjson module table
	mod := L.NewTable()

	// Create default instance
	instance := &GJSON{
		config: defaultConfig(),
		null:   createNull(L),
	}

	// Register module functions (bound to default instance)
	L.SetField(mod, "encode", L.NewFunction(instance.encode))
	L.SetField(mod, "decode", L.NewFunction(instance.decode))
	L.SetField(mod, "encode_sparse_array", L.NewFunction(instance.cfgEncodeSparseArray))
	L.SetField(mod, "encode_max_depth", L.NewFunction(instance.cfgEncodeMaxDepth))
	L.SetField(mod, "decode_max_depth", L.NewFunction(instance.cfgDecodeMaxDepth))
	L.SetField(mod, "encode_number_precision", L.NewFunction(instance.cfgEncodeNumberPrecision))
	L.SetField(mod, "encode_keep_buffer", L.NewFunction(instance.cfgEncodeKeepBuffer))
	L.SetField(mod, "encode_sort_keys", L.NewFunction(instance.cfgEncodeSortKeys))
	L.SetField(mod, "new", L.NewFunction(gjsonNew))

	// Set gjson.null (lightuserdata representing JSON null)
	L.SetField(mod, "null", instance.null)

	// Set module metadata
	L.SetField(mod, "_NAME", glua.LString(ModuleName))
	L.SetField(mod, "_VERSION", glua.LString(Version))

	// Push the module table
	L.Push(mod)
	return 1
}
