package gjson

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	json "github.com/goccy/go-json"
	glua "github.com/yuin/gopher-lua"
)

// encodeLuaValue converts a Lua value to a JSON string.
func encodeLuaValue(L *glua.LState, value glua.LValue, config *Config, depth int) (string, error) {
	switch value.Type() {
	case glua.LTNil:
		return "null", nil

	case glua.LTBool:
		if value == glua.LTrue {
			return "true", nil
		}
		return "false", nil

	case glua.LTNumber:
		num := float64(value.(glua.LNumber)) //nolint:errcheck // switch case guarantees type
		return formatNumber(num, config.encodeNumberPrecision), nil

	case glua.LTString:
		str := string(value.(glua.LString)) //nolint:errcheck // switch case guarantees type
		// Use go-json for proper string escaping
		result, err := json.Marshal(str)
		if err != nil {
			return "", fmt.Errorf("failed to encode string: %w", err)
		}
		return string(result), nil

	case glua.LTTable:
		return encodeTable(L, value.(*glua.LTable), config, depth) //nolint:errcheck // switch case guarantees type

	case glua.LTUserData:
		if isNull(value) {
			return "null", nil
		}
		return "", fmt.Errorf("cannot encode userdata (not gjson.null)")

	case glua.LTFunction:
		return "", fmt.Errorf("cannot encode function")

	case glua.LTThread:
		return "", fmt.Errorf("cannot encode thread")

	case glua.LTChannel:
		return "", fmt.Errorf("cannot encode channel")

	default:
		return "", fmt.Errorf("cannot encode unknown type: %s", value.Type())
	}
}

// encodeTable converts a Lua table to JSON (array or object).
func encodeTable(L *glua.LState, tbl *glua.LTable, config *Config, depth int) (string, error) {
	// Check depth limit
	if depth >= config.encodeMaxDepth {
		return "", fmt.Errorf("maximum nesting depth %d exceeded", config.encodeMaxDepth)
	}

	// Determine if table is an array or object
	isArray, maxIndex, _ := checkArrayType(tbl, config)

	if isArray {
		return encodeArray(L, tbl, maxIndex, config, depth)
	}

	return encodeObject(L, tbl, config, depth)
}

// checkArrayType determines if a table should be encoded as an array.
// Returns: (isArray, maxIndex, count)
func checkArrayType(tbl *glua.LTable, config *Config) (bool, int, int) {
	maxIndex := 0
	count := 0
	hasStringKey := false

	tbl.ForEach(func(key, _ glua.LValue) {
		switch k := key.(type) {
		case glua.LNumber:
			// Protect against integer overflow - only accept positive integers within int range
			floatIdx := float64(k)
			if floatIdx < 1 || floatIdx > float64(int(^uint(0)>>1)) {
				hasStringKey = true // Treat out-of-range as object key
				return
			}
			idx := int(k)
			if idx > maxIndex {
				maxIndex = idx
			}
			count++
		case glua.LString:
			hasStringKey = true
		}
	})

	// If there are string keys, it's an object
	if hasStringKey {
		return false, maxIndex, count
	}

	// Empty table is encoded as empty object (lua-cjson behavior)
	if count == 0 {
		return false, 0, 0
	}

	// Check for sparse array
	// Sparse condition: ratio > 0 && maxIndex > safe && maxIndex > count * ratio
	if config.encodeSparseArray.ratio > 0 &&
		maxIndex > config.encodeSparseArray.safe &&
		maxIndex > count*config.encodeSparseArray.ratio {
		// Sparse array detected
		if !config.encodeSparseArray.convert {
			// Would return error, but we return false to indicate object encoding
			return false, maxIndex, count
		}
		// Convert to object
		return false, maxIndex, count
	}

	// Check if keys are sequential starting from 1
	// Use MaxN() for quick check
	maxN := tbl.MaxN()
	if maxN == count && maxN == maxIndex {
		return true, maxIndex, count
	}

	// Non-sequential keys -> object
	return false, maxIndex, count
}

// encodeArray encodes a Lua table as a JSON array.
func encodeArray(L *glua.LState, tbl *glua.LTable, maxIndex int, config *Config, depth int) (string, error) {
	elements := make([]string, 0, maxIndex)

	for i := 1; i <= maxIndex; i++ {
		val := tbl.RawGetInt(i)
		if val == glua.LNil {
			// Missing element in sparse array - encode as null
			elements = append(elements, "null")
			continue
		}

		encoded, err := encodeLuaValue(L, val, config, depth+1)
		if err != nil {
			return "", err
		}
		elements = append(elements, encoded)
	}

	return "[" + strings.Join(elements, ",") + "]", nil
}

// encodeObject encodes a Lua table as a JSON object.
func encodeObject(L *glua.LState, tbl *glua.LTable, config *Config, depth int) (string, error) {
	// 快速路径：不需要排序时直接编码
	if !config.encodeSortKeys {
		elements := make([]string, 0)
		var encodeErr error
		tbl.ForEach(func(key, value glua.LValue) {
			if encodeErr != nil {
				return
			}

			// Encode key
			var keyStr string
			switch k := key.(type) {
			case glua.LString:
				encoded, _ := json.Marshal(string(k))
				keyStr = string(encoded)
			case glua.LNumber:
				keyStr = formatNumber(float64(k), config.encodeNumberPrecision)
				keyStr = "\"" + keyStr + "\""
			default:
				return
			}

			// Encode value
			valStr, err := encodeLuaValue(L, value, config, depth+1)
			if err != nil {
				encodeErr = err
				return
			}

			elements = append(elements, keyStr+":"+valStr)
		})

		if encodeErr != nil {
			return "", encodeErr
		}

		return "{" + strings.Join(elements, ",") + "}", nil
	}

	// 排序路径：收集所有键值对后排序
	type kv struct {
		key   string
		value glua.LValue
	}
	pairs := make([]kv, 0)

	var encodeErr error
	tbl.ForEach(func(key, value glua.LValue) {
		if encodeErr != nil {
			return
		}

		// Encode key
		var keyStr string
		switch k := key.(type) {
		case glua.LString:
			encoded, _ := json.Marshal(string(k))
			keyStr = string(encoded)
		case glua.LNumber:
			keyStr = formatNumber(float64(k), config.encodeNumberPrecision)
			keyStr = "\"" + keyStr + "\""
		default:
			return
		}

		pairs = append(pairs, kv{key: keyStr, value: value})
	})

	if encodeErr != nil {
		return "", encodeErr
	}

	// 按键排序（保证输出顺序稳定）
	sort.Slice(pairs, func(i, j int) bool {
		return pairs[i].key < pairs[j].key
	})

	// 编码值
	elements := make([]string, 0, len(pairs))
	for _, p := range pairs {
		valStr, err := encodeLuaValue(L, p.value, config, depth+1)
		if err != nil {
			return "", err
		}
		elements = append(elements, p.key+":"+valStr)
	}

	return "{" + strings.Join(elements, ",") + "}", nil
}

// formatNumber formats a number with the specified precision.
func formatNumber(n float64, precision int) string {
	if precision <= 0 {
		precision = 14
	}
	if precision > 14 {
		precision = 14
	}

	// Check if it's an integer
	if n == float64(int64(n)) && n >= -9007199254740992 && n <= 9007199254740992 {
		return strconv.FormatInt(int64(n), 10)
	}

	// Use 'g' format for floating point
	return strconv.FormatFloat(n, 'g', precision, 64)
}
