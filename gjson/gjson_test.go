package gjson

import (
	"testing"

	glua "github.com/yuin/gopher-lua"
)

func TestModuleLoad(t *testing.T) {
	L := glua.NewState()
	defer L.Close()

	Preload(L)

	err := L.DoString(`
		local gjson = require("gjson")
		assert(gjson._NAME == "gjson")
		assert(gjson._VERSION ~= nil)
	`)
	if err != nil {
		t.Fatalf("module load failed: %v", err)
	}
}

func TestEncodeBasicTypes(t *testing.T) {
	L := glua.NewState()
	defer L.Close()

	Preload(L)

	tests := []struct {
		name     string
		script   string
		expected string
	}{
		{"null", `return gjson.encode(gjson.null)`, "null"},
		{"nil", `return gjson.encode(nil)`, "null"},
		{"true", `return gjson.encode(true)`, "true"},
		{"false", `return gjson.encode(false)`, "false"},
		{"number", `return gjson.encode(42)`, "42"},
		{"string", `return gjson.encode("hello")`, `"hello"`},
		{"empty object", `return gjson.encode({})`, `{}`},
		{"empty array", `return gjson.encode({1})`, `[1]`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := L.DoString(`
				local gjson = require("gjson")
				result = ` + tt.script[7:] + `
			`)
			if err != nil {
				t.Fatalf("script failed: %v", err)
			}

			result := L.GetGlobal("result").String()
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestEncodeTable(t *testing.T) {
	L := glua.NewState()
	defer L.Close()

	Preload(L)

	err := L.DoString(`
		local gjson = require("gjson")

		-- Test array
		local arr = gjson.encode({1, 2, 3})
		assert(arr == "[1,2,3]", "array: " .. arr)

		-- Test object
		local obj = gjson.encode({name = "Alice", age = 30})
		assert(string.find(obj, '"name":"Alice"'), "object name: " .. obj)
		assert(string.find(obj, '"age":30'), "object age: " .. obj)

		-- Test nested
		local nested = gjson.encode({inner = {value = 123}})
		assert(string.find(nested, '"inner"'), "nested: " .. nested)
		assert(string.find(nested, '"value":123'), "nested value: " .. nested)
	`)
	if err != nil {
		t.Fatalf("test failed: %v", err)
	}
}

func TestDecodeBasicTypes(t *testing.T) {
	L := glua.NewState()
	defer L.Close()

	Preload(L)

	err := L.DoString(`
		local gjson = require("gjson")

		-- Test null
		local null_val = gjson.decode("null")
		assert(null_val == gjson.null, "null decode")

		-- Test boolean
		local bool_val = gjson.decode("true")
		assert(bool_val == true, "true decode")

		-- Test number
		local num_val = gjson.decode("42")
		assert(num_val == 42, "number decode")

		-- Test string
		local str_val = gjson.decode('"hello"')
		assert(str_val == "hello", "string decode")
	`)
	if err != nil {
		t.Fatalf("test failed: %v", err)
	}
}

func TestDecodeTable(t *testing.T) {
	L := glua.NewState()
	defer L.Close()

	Preload(L)

	err := L.DoString(`
		local gjson = require("gjson")

		-- Test array
		local arr = gjson.decode("[1,2,3]")
		assert(arr[1] == 1, "array index 1")
		assert(arr[2] == 2, "array index 2")
		assert(arr[3] == 3, "array index 3")

		-- Test object
		local obj = gjson.decode('{"name":"Alice","age":30}')
		assert(obj.name == "Alice", "object name")
		assert(obj.age == 30, "object age")
	`)
	if err != nil {
		t.Fatalf("test failed: %v", err)
	}
}

func TestRoundTrip(t *testing.T) {
	L := glua.NewState()
	defer L.Close()

	Preload(L)

	err := L.DoString(`
		local gjson = require("gjson")

		-- Test round trip
		local original = {name = "Bob", values = {1, 2, 3}, active = true}
		local encoded = gjson.encode(original)
		local decoded = gjson.decode(encoded)

		assert(decoded.name == "Bob", "round trip name")
		assert(decoded.values[1] == 1, "round trip values[1]")
		assert(decoded.values[2] == 2, "round trip values[2]")
		assert(decoded.values[3] == 3, "round trip values[3]")
		assert(decoded.active == true, "round trip active")
	`)
	if err != nil {
		t.Fatalf("test failed: %v", err)
	}
}

func TestErrorHandling(t *testing.T) {
	L := glua.NewState()
	defer L.Close()

	Preload(L)

	err := L.DoString(`
		local gjson = require("gjson")

		-- Test invalid JSON
		local result, err = gjson.decode("not valid json")
		assert(result == nil, "should return nil on error")
		assert(err ~= nil, "should return error message")

		-- Test empty string
		local result2, err2 = gjson.decode("")
		assert(result2 == nil, "should return nil on empty")
		assert(err2 ~= nil, "should return error on empty")
	`)
	if err != nil {
		t.Fatalf("test failed: %v", err)
	}
}

func TestNewInstance(t *testing.T) {
	L := glua.NewState()
	defer L.Close()

	Preload(L)

	err := L.DoString(`
		local gjson = require("gjson")

		-- Create new instance
		local inst = gjson.new()
		assert(inst._NAME == "gjson", "instance name")
		assert(inst.null ~= nil, "instance null")

		-- Test instance encode/decode
		local encoded = inst.encode({test = 123})
		local decoded = inst.decode(encoded)
		assert(decoded.test == 123, "instance round trip")
	`)
	if err != nil {
		t.Fatalf("test failed: %v", err)
	}
}

func TestConfigFunctions(t *testing.T) {
	L := glua.NewState()
	defer L.Close()

	Preload(L)

	err := L.DoString(`
		local gjson = require("gjson")

		-- Test encode_max_depth
		local depth = gjson.encode_max_depth(500)
		assert(depth == 500, "encode_max_depth set")
		assert(gjson.encode_max_depth() == 500, "encode_max_depth get")

		-- Test decode_max_depth
		local ddepth = gjson.decode_max_depth(500)
		assert(ddepth == 500, "decode_max_depth set")

		-- Test encode_number_precision
		local prec = gjson.encode_number_precision(10)
		assert(prec == 10, "encode_number_precision set")

		-- Test encode_sparse_array
		local convert, ratio, safe = gjson.encode_sparse_array(true, 3, 20)
		assert(convert == true, "sparse convert")
		assert(ratio == 3, "sparse ratio")
		assert(safe == 20, "sparse safe")
	`)
	if err != nil {
		t.Fatalf("test failed: %v", err)
	}
}

func TestEncodeDepthLimit(t *testing.T) {
	L := glua.NewState()
	defer L.Close()

	Preload(L)

	err := L.DoString(`
		local gjson = require("gjson")

		-- Set very low depth limit
		gjson.encode_max_depth(2)

		-- Create nested table
		local nested = {a = {b = {c = 1}}}

		-- Should fail due to depth limit
		local result, err = gjson.encode(nested)
		assert(result == nil, "should return nil on depth exceeded")
		assert(err ~= nil, "should return error on depth exceeded")
		assert(string.find(err, "depth"), "error should mention depth")
	`)
	if err != nil {
		t.Fatalf("test failed: %v", err)
	}
}

func TestDecodeDepthLimit(t *testing.T) {
	L := glua.NewState()
	defer L.Close()

	Preload(L)

	err := L.DoString(`
		local gjson = require("gjson")

		-- Set very low depth limit
		gjson.decode_max_depth(2)

		-- Deeply nested JSON
		local deep = '{"a":{"b":{"c":1}}}'

		-- Should fail due to depth limit
		local result, err = gjson.decode(deep)
		assert(result == nil, "should return nil on depth exceeded")
		assert(err ~= nil, "should return error on depth exceeded")
	`)
	if err != nil {
		t.Fatalf("test failed: %v", err)
	}
}

func TestEncodeUnencodableValue(t *testing.T) {
	L := glua.NewState()
	defer L.Close()

	Preload(L)

	err := L.DoString(`
		local gjson = require("gjson")

		-- Try to encode a function (should fail)
		local result, err = gjson.encode(function() end)
		assert(result == nil, "should return nil for function")
		assert(err ~= nil, "should return error for function")

		-- Try to encode table with function value (should fail)
		local result2, err2 = gjson.encode({func = function() end})
		assert(result2 == nil, "should return nil for table with function")
		assert(err2 ~= nil, "should return error for table with function")
	`)
	if err != nil {
		t.Fatalf("test failed: %v", err)
	}
}

func TestEncodeSparseArray(t *testing.T) {
	L := glua.NewState()
	defer L.Close()

	Preload(L)

	err := L.DoString(`
		local gjson = require("gjson")

		-- Create sparse array (missing index 2)
		local sparse = {}
		sparse[1] = "a"
		sparse[3] = "c"

		-- Default: convert sparse to object
		local result = gjson.encode(sparse)
		-- Should be encoded as object with numeric keys
		assert(string.find(result, '"1":"a"'), "should have key 1")
		assert(string.find(result, '"3":"c"'), "should have key 3")
	`)
	if err != nil {
		t.Fatalf("test failed: %v", err)
	}
}

func TestEncodeSortKeys(t *testing.T) {
	L := glua.NewState()
	defer L.Close()

	Preload(L)

	err := L.DoString(`
		local gjson = require("gjson")

		-- Test default (no sorting)
		local default_sort = gjson.encode_sort_keys()
		assert(default_sort == false, "default should be false")

		-- Enable sorting (returns new value)
		local new_val = gjson.encode_sort_keys(true)
		assert(new_val == true, "should return new value")
		assert(gjson.encode_sort_keys() == true, "should be true now")

		-- Test sorted output
		local data = {c = 3, a = 1, b = 2}
		local result = gjson.encode(data)
		-- Keys should be in alphabetical order: a, b, c
		local a_pos = string.find(result, '"a"')
		local b_pos = string.find(result, '"b"')
		local c_pos = string.find(result, '"c"')
		assert(a_pos < b_pos, "a should come before b")
		assert(b_pos < c_pos, "b should come before c")

		-- Disable sorting (back to fast mode)
		gjson.encode_sort_keys(false)
		assert(gjson.encode_sort_keys() == false, "should be false again")
	`)
	if err != nil {
		t.Fatalf("test failed: %v", err)
	}
}
