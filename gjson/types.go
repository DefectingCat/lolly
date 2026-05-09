package gjson

import (
	glua "github.com/yuin/gopher-lua"
)

// nullMarker is a sentinel type to identify gjson.null values.
type nullMarker struct{}

// GJSON represents a gjson instance with its own configuration.
type GJSON struct {
	config *Config
	null   *glua.LUserData
}

// createNull creates the gjson.null lightuserdata for the given LState.
func createNull(L *glua.LState) *glua.LUserData {
	ud := L.NewUserData()
	ud.Value = nullMarker{}
	return ud
}

// isNull checks if a Lua value is gjson.null.
func isNull(value glua.LValue) bool {
	if ud, ok := value.(*glua.LUserData); ok {
		_, isNullMarker := ud.Value.(nullMarker)
		return isNullMarker
	}
	return false
}

// gjsonNew creates a new GJSON instance with independent configuration.
// Lua: gjson.new() -> new_instance
func gjsonNew(L *glua.LState) int {
	// Create new instance with default config
	instance := &GJSON{
		config: defaultConfig(),
	}
	instance.null = createNull(L)

	// Create instance table
	tbl := L.NewTable()

	// Register methods (bound to this instance)
	L.SetField(tbl, "encode", L.NewFunction(instance.encode))
	L.SetField(tbl, "decode", L.NewFunction(instance.decode))
	L.SetField(tbl, "encode_sparse_array", L.NewFunction(instance.cfgEncodeSparseArray))
	L.SetField(tbl, "encode_max_depth", L.NewFunction(instance.cfgEncodeMaxDepth))
	L.SetField(tbl, "decode_max_depth", L.NewFunction(instance.cfgDecodeMaxDepth))
	L.SetField(tbl, "encode_number_precision", L.NewFunction(instance.cfgEncodeNumberPrecision))
	L.SetField(tbl, "encode_keep_buffer", L.NewFunction(instance.cfgEncodeKeepBuffer))
	L.SetField(tbl, "new", L.NewFunction(gjsonNew))

	// Set null
	L.SetField(tbl, "null", instance.null)

	// Set metadata
	L.SetField(tbl, "_NAME", glua.LString(ModuleName))
	L.SetField(tbl, "_VERSION", glua.LString(Version))

	L.Push(tbl)
	return 1
}
