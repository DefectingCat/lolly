package gjson

import (
	"fmt"

	json "github.com/goccy/go-json"
	glua "github.com/yuin/gopher-lua"
)

// decodeJSONValue converts a JSON string to a Lua value.
func decodeJSONValue(L *glua.LState, data []byte, config *Config, nullValue *glua.LUserData, depth int) (glua.LValue, error) {
	// Parse JSON using go-json
	var result interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return glua.LNil, fmt.Errorf("JSON parse error: %w", err)
	}

	return convertToLua(L, result, config, nullValue, depth)
}

// convertToLua converts a Go value (from JSON) to a Lua value.
func convertToLua(L *glua.LState, value interface{}, config *Config, nullValue *glua.LUserData, depth int) (glua.LValue, error) {
	if depth > config.decodeMaxDepth {
		return glua.LNil, fmt.Errorf("maximum nesting depth %d exceeded", config.decodeMaxDepth)
	}

	if value == nil {
		// JSON null -> gjson.null
		return nullValue, nil
	}

	switch v := value.(type) {
	case bool:
		return glua.LBool(v), nil

	case float64:
		// Check if it's actually an integer
		if v == float64(int64(v)) {
			return glua.LNumber(int64(v)), nil
		}
		return glua.LNumber(v), nil

	case string:
		return glua.LString(v), nil

	case []interface{}:
		return convertArrayToLua(L, v, config, nullValue, depth)

	case map[string]interface{}:
		return convertObjectToLua(L, v, config, nullValue, depth)

	case json.Number:
		// Handle UseNumber case - parse the number string
		num, err := parseJSONNumber(string(v))
		if err != nil {
			return glua.LNil, err
		}
		return num, nil

	default:
		return glua.LNil, fmt.Errorf("unknown JSON type: %T", value)
	}
}

// convertArrayToLua converts a JSON array to a Lua table with integer keys (1-based).
func convertArrayToLua(L *glua.LState, arr []interface{}, config *Config, nullValue *glua.LUserData, depth int) (glua.LValue, error) {
	tbl := L.NewTable()

	for i, item := range arr {
		luaVal, err := convertToLua(L, item, config, nullValue, depth+1)
		if err != nil {
			return glua.LNil, err
		}
		// Lua arrays are 1-based
		tbl.RawSetInt(i+1, luaVal)
	}

	return tbl, nil
}

// convertObjectToLua converts a JSON object to a Lua table with string keys.
func convertObjectToLua(L *glua.LState, obj map[string]interface{}, config *Config, nullValue *glua.LUserData, depth int) (glua.LValue, error) {
	tbl := L.NewTable()

	for key, val := range obj {
		luaVal, err := convertToLua(L, val, config, nullValue, depth+1)
		if err != nil {
			return glua.LNil, err
		}
		tbl.RawSetString(key, luaVal)
	}

	return tbl, nil
}

// parseJSONNumber parses a JSON number string to a Lua number.
func parseJSONNumber(s string) (glua.LNumber, error) {
	// Try parsing as integer first
	var intVal int64
	if _, err := fmt.Sscanf(s, "%d", &intVal); err == nil {
		return glua.LNumber(intVal), nil
	}

	// Parse as float
	var floatVal float64
	if _, err := fmt.Sscanf(s, "%f", &floatVal); err == nil {
		return glua.LNumber(floatVal), nil
	}

	return glua.LNumber(0), fmt.Errorf("invalid number format: %s", s)
}