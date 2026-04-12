// Package lua 提供 Lua 脚本嵌入能力
package lua

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSharedDictGetSet(t *testing.T) {
	dict := NewSharedDict("test", 100)

	// Set
	ok, err := dict.Set("key1", "value1", 0)
	require.NoError(t, err)
	assert.True(t, ok)

	// Get
	value, expired, err := dict.Get("key1")
	require.NoError(t, err)
	assert.False(t, expired)
	assert.Equal(t, "value1", value)

	// 不存在的 key
	value, expired, err = dict.Get("notexist")
	require.NoError(t, err)
	assert.False(t, expired)
	assert.Equal(t, "", value)
}

func TestSharedDictAdd(t *testing.T) {
	dict := NewSharedDict("test", 100)

	// Add 新 key
	ok, err := dict.Add("key1", "value1", 0)
	require.NoError(t, err)
	assert.True(t, ok)

	// Add 已存在的 key
	ok, err = dict.Add("key1", "value2", 0)
	require.NoError(t, err)
	assert.False(t, ok) // 已存在，返回 false

	// 验证值未被修改
	value, _, _ := dict.Get("key1")
	assert.Equal(t, "value1", value)
}

func TestSharedDictIncr(t *testing.T) {
	dict := NewSharedDict("test", 100)

	// Incr 不存在的 key，从 0 开始
	newValue, err := dict.Incr("counter", 5)
	require.NoError(t, err)
	assert.Equal(t, 5, newValue)

	// Incr 已存在的 key
	newValue, err = dict.Incr("counter", 3)
	require.NoError(t, err)
	assert.Equal(t, 8, newValue)

	// 轮减
	newValue, err = dict.Incr("counter", -2)
	require.NoError(t, err)
	assert.Equal(t, 6, newValue)
}

func TestSharedDictDelete(t *testing.T) {
	dict := NewSharedDict("test", 100)

	dict.Set("key1", "value1", 0)
	dict.Delete("key1")

	value, _, _ := dict.Get("key1")
	assert.Equal(t, "", value)

	// 删除不存在的 key 不会报错
	dict.Delete("notexist")
}

func TestSharedDictTTL(t *testing.T) {
	dict := NewSharedDict("test", 100)

	// Set 带 TTL
	ok, err := dict.Set("key1", "value1", 100*time.Millisecond)
	require.NoError(t, err)
	assert.True(t, ok)

	// 立即获取应该成功
	value, expired, err := dict.Get("key1")
	require.NoError(t, err)
	assert.False(t, expired)
	assert.Equal(t, "value1", value)

	// 等待过期
	time.Sleep(150 * time.Millisecond)

	// 过期后获取
	value, expired, err = dict.Get("key1")
	require.NoError(t, err)
	assert.True(t, expired)
	assert.Equal(t, "", value)
}

func TestSharedDictLRUEviction(t *testing.T) {
	dict := NewSharedDict("test", 3) // 只有 3 个容量

	// 添加 3 个条目
	dict.Set("key1", "value1", 0)
	dict.Set("key2", "value2", 0)
	dict.Set("key3", "value3", 0)

	// 使用 key1，使其成为最近使用
	dict.Get("key1")

	// 添加第 4 个条目，应该淘汰 key2（最久未使用）
	ok, err := dict.Set("key4", "value4", 0)
	require.NoError(t, err)
	assert.True(t, ok)

	// key1 应该还在
	value, _, _ := dict.Get("key1")
	assert.Equal(t, "value1", value)

	// key2 应该被淘汰
	value, _, _ = dict.Get("key2")
	assert.Equal(t, "", value)
}

func TestSharedDictFlushAll(t *testing.T) {
	dict := NewSharedDict("test", 100)

	dict.Set("key1", "value1", 0)
	dict.Set("key2", "value2", 0)

	dict.FlushAll()

	assert.Equal(t, 0, dict.Size())
}

func TestSharedDictFlushExpired(t *testing.T) {
	dict := NewSharedDict("test", 100)

	dict.Set("key1", "value1", 100*time.Millisecond)
	dict.Set("key2", "value2", 100*time.Millisecond)
	dict.Set("key3", "value3", 0) // 不过期

	// 立即清除应该返回 0
	count := dict.FlushExpired()
	assert.Equal(t, 0, count)

	// 等待过期
	time.Sleep(150 * time.Millisecond)

	count = dict.FlushExpired()
	assert.Equal(t, 2, count)

	// key3 应该还在
	assert.Equal(t, 1, dict.Size())
	value, _, _ := dict.Get("key3")
	assert.Equal(t, "value3", value)
}

func TestSharedDictSize(t *testing.T) {
	dict := NewSharedDict("test", 100)

	assert.Equal(t, 0, dict.Size())
	assert.Equal(t, 100, dict.FreeSlots())

	dict.Set("key1", "value1", 0)
	assert.Equal(t, 1, dict.Size())
	assert.Equal(t, 99, dict.FreeSlots())
}

func TestSharedDictManager(t *testing.T) {
	manager := NewSharedDictManager()

	// 创建字典
	dict1 := manager.CreateDict("dict1", 100)
	require.NotNil(t, dict1)

	// 再次获取同一个字典
	dict1Again := manager.GetDict("dict1")
	assert.Equal(t, dict1, dict1Again)

	// 创建另一个字典
	dict2 := manager.CreateDict("dict2", 200)
	require.NotNil(t, dict2)

	// 获取不存在的字典
	notexist := manager.GetDict("notexist")
	assert.Nil(t, notexist)

	// 关闭
	manager.Close()
}

func TestSharedDictLuaAPI(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	// 创建共享字典（通过 Lua API 测试，此处仅为初始化）
	_ = engine.CreateSharedDict("mydict", 100)

	// 测试 Lua 脚本
	L := engine.L

	// 手动注册 ngx.shared API（用于测试）
	ngx := L.NewTable()
	L.SetGlobal("ngx", ngx)
	RegisterSharedDictAPI(L, engine.SharedDictManager(), ngx)

	// 运行 Lua 脚本测试
	err = L.DoString(`
		local dict = ngx.shared.DICT("mydict")

		-- 测试 set/get
		dict:set("key1", "value1")
		local val, flags = dict:get("key1")
		assert(val == "value1")

		-- 测试 add
		local ok, err = dict:add("key2", "value2")
		assert(ok == true)

		-- 测试 add 已存在的 key
		ok, err = dict:add("key2", "value3")
		assert(ok == false)
		assert(err == "exists")

		-- 测试 incr
		local new_val, err = dict:incr("counter", 10)
		assert(new_val == 10)

		new_val, err = dict:incr("counter", 5)
		assert(new_val == 15)

		-- 测试 size
		local size = dict:size()
		assert(size >= 3)

		-- 测试 delete
		dict:delete("key1")
		local val, err = dict:get("key1")
		assert(val == nil)
	`)
	require.NoError(t, err)
}
