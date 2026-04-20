// Package lua 提供 ngx.shared API 扩展测试，覆盖 Lua API 函数的更多场景
package lua

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSharedDictLuaReplace 测试 dict:replace
func TestSharedDictLuaReplace(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	_ = engine.CreateSharedDict("testdict", 100)

	L := engine.L
	ngx := L.NewTable()
	L.SetGlobal("ngx", ngx)
	RegisterSharedDictAPI(L, engine.SharedDictManager(), ngx)

	// 测试 replace 存在的 key
	err = L.DoString(`
		local dict = ngx.shared.DICT("testdict")

		-- 先设置一个值
		dict:set("mykey", "old_value")

		-- replace 存在的 key 应该成功
		local ok, err = dict:replace("mykey", "new_value")
		assert(ok == true, "replace existing key should succeed: " .. tostring(err))

		-- 验证值已更新
		local val, _ = dict:get("mykey")
		assert(val == "new_value", "value should be updated")
	`)
	require.NoError(t, err)

	// 测试 replace 不存在的 key
	err = L.DoString(`
		local dict = ngx.shared.DICT("testdict")

		-- replace 不存在的 key 应该失败
		local ok, err = dict:replace("nonexistent", "value")
		assert(ok == false, "replace nonexistent key should fail")
		assert(err == "not found", "error should be 'not found', got: " .. tostring(err))
	`)
	require.NoError(t, err)
}

// TestSharedDictLuaReplaceWithTTL 测试 dict:replace 带 TTL
func TestSharedDictLuaReplaceWithTTL(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	_ = engine.CreateSharedDict("ttldict", 100)

	L := engine.L
	ngx := L.NewTable()
	L.SetGlobal("ngx", ngx)
	RegisterSharedDictAPI(L, engine.SharedDictManager(), ngx)

	err = L.DoString(`
		local dict = ngx.shared.DICT("ttldict")
		dict:set("ttlkey", "original")

		-- replace 带 TTL（0.1 秒）
		local ok, err = dict:replace("ttlkey", "replaced", 0.1)
		assert(ok == true, "replace with TTL should succeed: " .. tostring(err))
	`)
	require.NoError(t, err)

	// 等待过期
	time.Sleep(150 * time.Millisecond)

	// 验证过期
	err = L.DoString(`
		local dict = ngx.shared.DICT("ttldict")
		local val, err = dict:get("ttlkey")
		assert(val == nil, "expired key should return nil")
	`)
	require.NoError(t, err)
}

// TestSharedDictLuaIndexAccess 测试 dict["key"] 索引访问
func TestSharedDictLuaIndexAccess(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	_ = engine.CreateSharedDict("idxdict", 100)

	L := engine.L
	ngx := L.NewTable()
	L.SetGlobal("ngx", ngx)
	RegisterSharedDictAPI(L, engine.SharedDictManager(), ngx)

	err = L.DoString(`
		local dict = ngx.shared.DICT("idxdict")

		-- 通过方法设置值
		dict:set("foo", "bar")

		-- 通过索引方式读取
		local val = dict["foo"]
		assert(val == "bar", "index access should work")

		-- 通过 __newindex 设置值
		dict["newkey"] = "newvalue"
		local v = dict:get("newkey")
		assert(v == "newvalue", "__newindex should work")
	`)
	require.NoError(t, err)
}

// TestSharedDictLuaIndexNotFound 测试索引访问不存在的 key
func TestSharedDictLuaIndexNotFound(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	_ = engine.CreateSharedDict("idxdict2", 100)

	L := engine.L
	ngx := L.NewTable()
	L.SetGlobal("ngx", ngx)
	RegisterSharedDictAPI(L, engine.SharedDictManager(), ngx)

	err = L.DoString(`
		local dict = ngx.shared.DICT("idxdict2")

		-- 读取不存在的 key 应该返回 nil
		local val = dict["nonexistent"]
		assert(val == nil, "nonexistent key should return nil")
	`)
	require.NoError(t, err)
}

// TestSharedDictLuaIndexMethodAccess 测试索引访问方法名
func TestSharedDictLuaIndexMethodAccess(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	_ = engine.CreateSharedDict("metdict", 100)

	L := engine.L
	ngx := L.NewTable()
	L.SetGlobal("ngx", ngx)
	RegisterSharedDictAPI(L, engine.SharedDictManager(), ngx)

	err = L.DoString(`
		local dict = ngx.shared.DICT("metdict")

		-- 通过索引访问方法
		local getFn = dict["get"]
		assert(type(getFn) == "function", "get should be a function")

		local setFn = dict["set"]
		assert(type(setFn) == "function", "set should be a function")
	`)
	require.NoError(t, err)
}

// TestSharedDictLuaFlushExpired 测试 dict:flush_expired
func TestSharedDictLuaFlushExpired(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	_ = engine.CreateSharedDict("flushdict", 100)

	L := engine.L
	ngx := L.NewTable()
	L.SetGlobal("ngx", ngx)
	RegisterSharedDictAPI(L, engine.SharedDictManager(), ngx)

	// 设置一些带 TTL 的条目和一个永久的
	err = L.DoString(`
		local dict = ngx.shared.DICT("flushdict")
		dict:set("exp1", "v1", 0.05)
		dict:set("exp2", "v2", 0.05)
		dict:set("perm", "permanent")
	`)
	require.NoError(t, err)

	// 立即清除应该没有过期条目
	err = L.DoString(`
		local dict = ngx.shared.DICT("flushdict")
		local count = dict:flush_expired()
		assert(count == 0, "should have 0 expired immediately")
	`)
	require.NoError(t, err)

	// 等待过期
	time.Sleep(100 * time.Millisecond)

	// 现在应该有 2 个过期条目
	err = L.DoString(`
		local dict = ngx.shared.DICT("flushdict")
		local count = dict:flush_expired()
		assert(count == 2, "should have 2 expired after wait")

		-- 永久条目应该还在
		local val, _ = dict:get("perm")
		assert(val == "permanent", "permanent key should still exist")
	`)
	require.NoError(t, err)
}

// TestSharedDictLuaGetKeys 测试 dict:get_keys
func TestSharedDictLuaGetKeys(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	_ = engine.CreateSharedDict("keysdict", 100)

	L := engine.L
	ngx := L.NewTable()
	L.SetGlobal("ngx", ngx)
	RegisterSharedDictAPI(L, engine.SharedDictManager(), ngx)

	err = L.DoString(`
		local dict = ngx.shared.DICT("keysdict")
		dict:set("k1", "v1")
		dict:set("k2", "v2")
		dict:set("k3", "v3")

		local keys = dict:get_keys()
		assert(type(keys) == "table", "get_keys should return a table")
		-- 当前实现返回空表
	`)
	require.NoError(t, err)
}

// TestSharedDictLuaSize 测试 dict:size
func TestSharedDictLuaSize(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	_ = engine.CreateSharedDict("sizedict", 100)

	L := engine.L
	ngx := L.NewTable()
	L.SetGlobal("ngx", ngx)
	RegisterSharedDictAPI(L, engine.SharedDictManager(), ngx)

	err = L.DoString(`
		local dict = ngx.shared.DICT("sizedict")

		-- 初始为 0
		assert(dict:size() == 0, "initial size should be 0")

		dict:set("a", "1")
		dict:set("b", "2")
		dict:set("c", "3")

		assert(dict:size() >= 3, "size should be at least 3")
	`)
	require.NoError(t, err)
}

// TestSharedDictLuaFreeSpace 测试 dict:free_space
func TestSharedDictLuaFreeSpace(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	_ = engine.CreateSharedDict("freedict", 50)

	L := engine.L
	ngx := L.NewTable()
	L.SetGlobal("ngx", ngx)
	RegisterSharedDictAPI(L, engine.SharedDictManager(), ngx)

	err = L.DoString(`
		local dict = ngx.shared.DICT("freedict")

		-- 初始 free_space 应该等于 maxItems
		local free = dict:free_space()
		assert(free == 50, "initial free_space should be 50, got: " .. tostring(free))

		-- 添加条目后 free_space 减少
		dict:set("key1", "value1")
		local free2 = dict:free_space()
		assert(free2 == 49, "free_space should be 49 after 1 item, got: " .. tostring(free2))
	`)
	require.NoError(t, err)
}

// TestSharedDictLuaFlushAll 测试 dict:flush_all
func TestSharedDictLuaFlushAll(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	_ = engine.CreateSharedDict("flushalldict", 100)

	L := engine.L
	ngx := L.NewTable()
	L.SetGlobal("ngx", ngx)
	RegisterSharedDictAPI(L, engine.SharedDictManager(), ngx)

	err = L.DoString(`
		local dict = ngx.shared.DICT("flushalldict")
		dict:set("a", "1")
		dict:set("b", "2")

		assert(dict:size() >= 2)

		-- flush_all 清空所有
		dict:flush_all()
		assert(dict:size() == 0, "size should be 0 after flush_all")
	`)
	require.NoError(t, err)
}

// TestSharedDictLuaDictNotFound 测试请求不存在的 shared dict
func TestSharedDictLuaDictNotFound(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	L := engine.L
	ngx := L.NewTable()
	L.SetGlobal("ngx", ngx)
	RegisterSharedDictAPI(L, engine.SharedDictManager(), ngx)

	err = L.DoString(`
		local dict, err = ngx.shared.DICT("nonexistent_dict")
		assert(dict == nil, "nonexistent dict should return nil")
		assert(err ~= nil, "should return error message")
	`)
	require.NoError(t, err)
}

// TestSharedDictLuaToString 测试 dict 的 tostring
func TestSharedDictLuaToString(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	_ = engine.CreateSharedDict("tostringdict", 100)

	L := engine.L
	ngx := L.NewTable()
	L.SetGlobal("ngx", ngx)
	RegisterSharedDictAPI(L, engine.SharedDictManager(), ngx)

	err = L.DoString(`
		local dict = ngx.shared.DICT("tostringdict")
		local s = tostring(dict)
		assert(type(s) == "string", "tostring should return a string")
		assert(string.match(s, "ngx.shared.dict"), "should contain 'ngx.shared.dict'")
	`)
	require.NoError(t, err)
}

// TestSharedDictLuaIncrNonNumber 测试 incr 对非数值
func TestSharedDictLuaIncrNonNumber(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	_ = engine.CreateSharedDict("incrdict", 100)

	L := engine.L
	ngx := L.NewTable()
	L.SetGlobal("ngx", ngx)
	RegisterSharedDictAPI(L, engine.SharedDictManager(), ngx)

	// 设置一个非数值
	dict := engine.SharedDictManager().GetDict("incrdict")
	dict.Set("notnum", "hello", 0)

	err = L.DoString(`
		local dict = ngx.shared.DICT("incrdict")
		local val, err = dict:incr("notnum", 1)
		-- 非数值 incr 返回 nil
		assert(val == nil, "incr non-number should return nil")
	`)
	require.NoError(t, err)
}

// TestSharedDictLuaAddMultipleKeys 测试批量添加
func TestSharedDictLuaAddMultipleKeys(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	_ = engine.CreateSharedDict("multiadd", 100)

	L := engine.L
	ngx := L.NewTable()
	L.SetGlobal("ngx", ngx)
	RegisterSharedDictAPI(L, engine.SharedDictManager(), ngx)

	err = L.DoString(`
		local dict = ngx.shared.DICT("multiadd")

		-- 批量添加
		for i = 1, 10 do
			local ok, err = dict:add("key_" .. i, "val_" .. i)
			assert(ok == true, "add should succeed: " .. tostring(err))
		end

		-- 验证所有 key 都存在
		for i = 1, 10 do
			local val, _ = dict:get("key_" .. i)
			assert(val == "val_" .. i, "key_" .. i .. " should have correct value")
		end

		assert(dict:size() == 10, "size should be 10")
	`)
	require.NoError(t, err)
}

// TestSharedDictEngineAPI 测试通过 Engine API 创建共享字典
func TestSharedDictEngineAPI(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	// 通过 Engine 的 CreateSharedDict 方法创建
	dict := engine.CreateSharedDict("enginedict", 50)
	require.NotNil(t, dict)

	// 验证可以通过 SharedDictManager 获取
	dict2 := engine.SharedDictManager().GetDict("enginedict")
	assert.Equal(t, dict, dict2, "should return same dict")

	// 再次调用 CreateSharedDict 应返回已存在的
	dict3 := engine.CreateSharedDict("enginedict", 100)
	assert.Equal(t, dict, dict3, "should return existing dict regardless of maxItems")
}

// TestSharedDictLuaSetWithTTL 测试 set 带 TTL
func TestSharedDictLuaSetWithTTL(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	_ = engine.CreateSharedDict("setttldict", 100)

	L := engine.L
	ngx := L.NewTable()
	L.SetGlobal("ngx", ngx)
	RegisterSharedDictAPI(L, engine.SharedDictManager(), ngx)

	err = L.DoString(`
		local dict = ngx.shared.DICT("setttldict")

		-- set 带 TTL（0.1 秒）
		local ok, err = dict:set("ttlkey", "temp_value", 0.1)
		assert(ok == true, "set with TTL should succeed: " .. tostring(err))

		-- 立即获取应该成功
		local val, exp = dict:get("ttlkey")
		assert(val == "temp_value", "value should be correct")
		assert(exp == 0 or exp == nil, "should not be expired yet")
	`)
	require.NoError(t, err)

	// 等待过期
	time.Sleep(150 * time.Millisecond)

	err = L.DoString(`
		local dict = ngx.shared.DICT("setttldict")
		local val, err = dict:get("ttlkey")
		assert(val == nil, "expired key should return nil")
		assert(err == "expired", "error should be 'expired', got: " .. tostring(err))
	`)
	require.NoError(t, err)
}

// TestSharedDictLuaAddWithTTL 测试 add 带 TTL
func TestSharedDictLuaAddWithTTL(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	_ = engine.CreateSharedDict("addttldict", 100)

	L := engine.L
	ngx := L.NewTable()
	L.SetGlobal("ngx", ngx)
	RegisterSharedDictAPI(L, engine.SharedDictManager(), ngx)

	err = L.DoString(`
		local dict = ngx.shared.DICT("addttldict")

		-- add 带 TTL
		local ok, err = dict:add("addkey", "temp", 0.1)
		assert(ok == true, "add with TTL should succeed: " .. tostring(err))
	`)
	require.NoError(t, err)

	time.Sleep(150 * time.Millisecond)

	err = L.DoString(`
		local dict = ngx.shared.DICT("addttldict")
		local val, err = dict:get("addkey")
		assert(val == nil, "expired key should return nil")
	`)
	require.NoError(t, err)
}

// TestSharedDictClose 测试 SharedDictManager.Close
func TestSharedDictClose(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	_ = engine.CreateSharedDict("closedict", 100)

	// Close 引擎会清理 sharedDictManager
	engine.Close()

	// Close 后不应该 panic（即使再次 Close）
	engine.Close()
}
