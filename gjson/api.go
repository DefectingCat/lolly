package gjson

import (
	"fmt"

	glua "github.com/yuin/gopher-lua"
)

// cfgEncodeSparseArray configures sparse array handling.
// Lua: gjson.encode_sparse_array([convert[, ratio[, safe]]])
// Returns current values when called without arguments.
func (g *GJSON) cfgEncodeSparseArray(L *glua.LState) int {
	n := L.GetTop()

	if n == 0 {
		// Return current values
		L.Push(glua.LBool(g.config.encodeSparseArray.convert))
		L.Push(glua.LNumber(g.config.encodeSparseArray.ratio))
		L.Push(glua.LNumber(g.config.encodeSparseArray.safe))
		return 3
	}

	// Set new values
	if n >= 1 {
		g.config.encodeSparseArray.convert = L.CheckBool(1)
	}
	if n >= 2 {
		g.config.encodeSparseArray.ratio = L.CheckInt(2)
		if g.config.encodeSparseArray.ratio < 0 {
			L.ArgError(2, "ratio must be >= 0")
		}
	}
	if n >= 3 {
		g.config.encodeSparseArray.safe = L.CheckInt(3)
		if g.config.encodeSparseArray.safe < 0 {
			L.ArgError(3, "safe must be >= 0")
		}
	}

	L.Push(glua.LBool(g.config.encodeSparseArray.convert))
	L.Push(glua.LNumber(g.config.encodeSparseArray.ratio))
	L.Push(glua.LNumber(g.config.encodeSparseArray.safe))
	return 3
}

// cfgEncodeMaxDepth configures the maximum nesting depth for encoding.
// Lua: gjson.encode_max_depth([depth])
func (g *GJSON) cfgEncodeMaxDepth(L *glua.LState) int {
	if L.GetTop() == 0 {
		L.Push(glua.LNumber(g.config.encodeMaxDepth))
		return 1
	}

	depth := L.CheckInt(1)
	if depth < 1 {
		L.ArgError(1, "max depth must be >= 1")
	}
	g.config.encodeMaxDepth = depth

	L.Push(glua.LNumber(g.config.encodeMaxDepth))
	return 1
}

// cfgDecodeMaxDepth configures the maximum nesting depth for decoding.
// Lua: gjson.decode_max_depth([depth])
func (g *GJSON) cfgDecodeMaxDepth(L *glua.LState) int {
	if L.GetTop() == 0 {
		L.Push(glua.LNumber(g.config.decodeMaxDepth))
		return 1
	}

	depth := L.CheckInt(1)
	if depth < 1 {
		L.ArgError(1, "max depth must be >= 1")
	}
	g.config.decodeMaxDepth = depth

	L.Push(glua.LNumber(g.config.decodeMaxDepth))
	return 1
}

// cfgEncodeNumberPrecision configures the number precision for encoding.
// Lua: gjson.encode_number_precision([precision])
func (g *GJSON) cfgEncodeNumberPrecision(L *glua.LState) int {
	if L.GetTop() == 0 {
		L.Push(glua.LNumber(g.config.encodeNumberPrecision))
		return 1
	}

	precision := L.CheckInt(1)
	if precision < 1 || precision > 14 {
		L.ArgError(1, "precision must be between 1 and 14")
	}
	g.config.encodeNumberPrecision = precision

	L.Push(glua.LNumber(g.config.encodeNumberPrecision))
	return 1
}

// cfgEncodeKeepBuffer configures whether to reuse the encoding buffer.
// Lua: gjson.encode_keep_buffer([keep])
func (g *GJSON) cfgEncodeKeepBuffer(L *glua.LState) int {
	if L.GetTop() == 0 {
		L.Push(glua.LBool(g.config.encodeKeepBuffer))
		return 1
	}

	g.config.encodeKeepBuffer = L.CheckBool(1)

	L.Push(glua.LBool(g.config.encodeKeepBuffer))
	return 1
}

// cfgEncodeSortKeys configures whether to sort object keys for stable output.
// Lua: gjson.encode_sort_keys([sort])
// Returns current value when called without arguments.
// When enabled, object keys are sorted alphabetically for deterministic output.
// When disabled (default), keys are output in arbitrary order for better performance.
//
//nolint:unused // 方法通过 instance.cfgEncodeSortKeys 方式注册到 Lua，linter 无法检测到使用
func (g *GJSON) cfgEncodeSortKeys(L *glua.LState) int {
	if L.GetTop() == 0 {
		L.Push(glua.LBool(g.config.encodeSortKeys))
		return 1
	}

	g.config.encodeSortKeys = L.CheckBool(1)

	L.Push(glua.LBool(g.config.encodeSortKeys))
	return 1
}

// encode is the Lua function for gjson.encode(value).
// Returns (json_string, nil) on success or (nil, error_message) on failure.
func (g *GJSON) encode(L *glua.LState) int {
	if L.GetTop() != 1 {
		L.ArgError(1, "expected 1 argument")
		return 0
	}

	value := L.Get(1)
	result, err := g.encodeValue(L, value, 0)
	if err != nil {
		L.Push(glua.LNil)
		L.Push(glua.LString(err.Error()))
		return 2
	}

	L.Push(glua.LString(result))
	return 1
}

// decode is the Lua function for gjson.decode(string).
// Returns (value, nil) on success or (nil, error_message) on failure.
func (g *GJSON) decode(L *glua.LState) int {
	if L.GetTop() != 1 {
		L.ArgError(1, "expected 1 argument")
		return 0
	}

	str := L.CheckString(1)
	if str == "" {
		L.Push(glua.LNil)
		L.Push(glua.LString("empty JSON string"))
		return 2
	}

	result, err := g.decodeValue(L, []byte(str), 0)
	if err != nil {
		L.Push(glua.LNil)
		L.Push(glua.LString(err.Error()))
		return 2
	}

	L.Push(result)
	return 1
}

// encodeValue encodes a Lua value to JSON with depth tracking.
func (g *GJSON) encodeValue(L *glua.LState, value glua.LValue, depth int) (string, error) {
	if depth > g.config.encodeMaxDepth {
		return "", fmt.Errorf("maximum nesting depth %d exceeded", g.config.encodeMaxDepth)
	}

	return encodeLuaValue(L, value, g.config, depth)
}

// decodeValue decodes JSON to a Lua value with depth tracking.
func (g *GJSON) decodeValue(L *glua.LState, data []byte, depth int) (glua.LValue, error) {
	if depth > g.config.decodeMaxDepth {
		return glua.LNil, fmt.Errorf("maximum nesting depth %d exceeded", g.config.decodeMaxDepth)
	}

	return decodeJSONValue(L, data, g.config, g.null, depth)
}
