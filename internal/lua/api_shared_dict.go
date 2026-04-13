// Package lua 提供 Lua 脚本嵌入能力
package lua

import (
	"sync"
	"time"

	glua "github.com/yuin/gopher-lua"
)

// SharedDictManager 共享字典管理器
// 管理多个命名的 SharedDict 实例
type SharedDictManager struct {
	dicts map[string]*SharedDict
	mu    sync.RWMutex
}

// NewSharedDictManager 创建字典管理器
func NewSharedDictManager() *SharedDictManager {
	return &SharedDictManager{
		dicts: make(map[string]*SharedDict),
	}
}

// CreateDict 创建或获取字典
func (m *SharedDictManager) CreateDict(name string, maxItems int) *SharedDict {
	m.mu.Lock()
	defer m.mu.Unlock()

	if dict, ok := m.dicts[name]; ok {
		return dict
	}

	dict := NewSharedDict(name, maxItems)
	m.dicts[name] = dict
	return dict
}

// GetDict 获取字典
func (m *SharedDictManager) GetDict(name string) *SharedDict {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.dicts[name]
}

// Close 清理所有字典
func (m *SharedDictManager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dicts = nil
}

// DictConfig 字典配置
type DictConfig struct {
	Name     string
	MaxItems int
}

// RegisterSharedDictAPI 注册 ngx.shared.DICT API
func RegisterSharedDictAPI(L *glua.LState, manager *SharedDictManager, ngx *glua.LTable) {
	// 创建 ngx.shared 表
	shared := L.NewTable()

	// ngx.shared.DICT - 返回字典 userdata
	L.SetField(shared, "DICT", L.NewFunction(func(L *glua.LState) int {
		name := L.CheckString(1)

		dict := manager.GetDict(name)
		if dict == nil {
			L.Push(glua.LNil)
			L.Push(glua.LString("shared dict not found: " + name))
			return 2
		}

		// 返回字典 userdata
		ud := L.NewUserData()
		ud.Value = dict
		L.SetMetatable(ud, L.GetTypeMetatable("ngx.shared.dict"))
		L.Push(ud)
		return 1
	}))

	L.SetField(ngx, "shared", shared)

	// 创建字典类型元表
	mt := L.NewTypeMetatable("ngx.shared.dict")
	L.SetField(mt, "__index", L.NewFunction(dictIndex))
	L.SetField(mt, "__newindex", L.NewFunction(dictNewIndex))
	L.SetField(mt, "__tostring", L.NewFunction(dictToString))

	// 注册字典方法
	methods := L.NewTable()
	L.SetField(methods, "get", L.NewFunction(dictGet))
	L.SetField(methods, "set", L.NewFunction(dictSet))
	L.SetField(methods, "add", L.NewFunction(dictAdd))
	L.SetField(methods, "replace", L.NewFunction(dictReplace))
	L.SetField(methods, "incr", L.NewFunction(dictIncr))
	L.SetField(methods, "delete", L.NewFunction(dictDelete))
	L.SetField(methods, "flush_all", L.NewFunction(dictFlushAll))
	L.SetField(methods, "flush_expired", L.NewFunction(dictFlushExpired))
	L.SetField(methods, "get_keys", L.NewFunction(dictGetKeys))
	L.SetField(methods, "size", L.NewFunction(dictSize))
	L.SetField(methods, "free_space", L.NewFunction(dictFreeSpace))
	L.SetField(mt, "methods", methods)
}

// checkSharedDict 检查并获取 SharedDict
func checkSharedDict(L *glua.LState) *SharedDict {
	ud := L.CheckUserData(1)
	dict, ok := ud.Value.(*SharedDict)
	if !ok {
		L.RaiseError("invalid shared dict")
	}
	return dict
}

// dictIndex 字典索引方法
func dictIndex(L *glua.LState) int {
	dict := checkSharedDict(L)

	key := L.CheckString(2)

	// 检查是否是方法
	//nolint:errcheck // 类型断言检查
	methods := L.GetField(L.Get(1).(*glua.LUserData).Metatable, "methods")
	if method := L.GetField(methods, key); method != glua.LNil {
		L.Push(method)
		return 1
	}

	// 否则作为 key 获取值
	value, expired, err := dict.Get(key)
	if err != nil {
		L.RaiseError("%s", err.Error())
		return 0
	}
	if expired {
		L.Push(glua.LNil)
		L.Push(glua.LString("expired"))
		return 2
	}
	if value == "" {
		L.Push(glua.LNil)
		return 1
	}
	L.Push(glua.LString(value))
	return 1
}

// dictNewIndex 字典设置方法
func dictNewIndex(L *glua.LState) int {
	dict := checkSharedDict(L)

	key := L.CheckString(2)
	value := L.CheckString(3)

	ok, err := dict.Set(key, value, 0)
	if err != nil {
		L.RaiseError("%s", err.Error())
		return 0
	}
	if !ok {
		L.Push(glua.LFalse)
		L.Push(glua.LString("no memory"))
		return 2
	}
	L.Push(glua.LTrue)
	return 1
}

// dictToString 字典字符串表示
func dictToString(L *glua.LState) int {
	dict := checkSharedDict(L)
	L.Push(glua.LString("ngx.shared.dict:" + dict.name))
	return 1
}

// dictGet 获取值
// dict:get(key) -> value, flags | nil, err
func dictGet(L *glua.LState) int {
	dict := checkSharedDict(L)

	key := L.CheckString(2)

	value, expired, err := dict.Get(key)
	if err != nil {
		L.Push(glua.LNil)
		L.Push(glua.LString(err.Error()))
		return 2
	}
	if expired {
		L.Push(glua.LNil)
		L.Push(glua.LString("expired"))
		return 2
	}
	if value == "" {
		L.Push(glua.LNil)
		return 1
	}
	L.Push(glua.LString(value))
	L.Push(glua.LNumber(0)) // flags（暂不支持）
	return 2
}

// dictSet 设置值
// dict:set(key, value, exptime?, flags?) -> ok, err
func dictSet(L *glua.LState) int {
	dict := checkSharedDict(L)

	key := L.CheckString(2)
	value := L.CheckString(3)

	ttl := time.Duration(0)
	if L.GetTop() >= 4 {
		ttl = time.Duration(L.CheckNumber(4)) * time.Second
	}

	// flags 参数暂不使用

	ok, err := dict.Set(key, value, ttl)
	if err != nil {
		L.Push(glua.LFalse)
		L.Push(glua.LString(err.Error()))
		return 2
	}
	if !ok {
		L.Push(glua.LFalse)
		L.Push(glua.LString("no memory"))
		return 2
	}
	L.Push(glua.LTrue)
	return 1
}

// dictAdd 添加值（不存在时）
// dict:add(key, value, exptime?, flags?) -> ok, err
func dictAdd(L *glua.LState) int {
	dict := checkSharedDict(L)

	key := L.CheckString(2)
	value := L.CheckString(3)

	ttl := time.Duration(0)
	if L.GetTop() >= 4 {
		ttl = time.Duration(L.CheckNumber(4)) * time.Second
	}

	ok, err := dict.Add(key, value, ttl)
	if err != nil {
		L.Push(glua.LFalse)
		L.Push(glua.LString(err.Error()))
		return 2
	}
	if !ok {
		L.Push(glua.LFalse)
		L.Push(glua.LString("exists"))
		return 2
	}
	L.Push(glua.LTrue)
	return 1
}

// dictReplace 替换值（存在时）
// dict:replace(key, value, exptime?, flags?) -> ok, err
func dictReplace(L *glua.LState) int {
	dict := checkSharedDict(L)

	key := L.CheckString(2)
	value := L.CheckString(3)

	ttl := time.Duration(0)
	if L.GetTop() >= 4 {
		ttl = time.Duration(L.CheckNumber(4)) * time.Second
	}

	// 检查是否存在
	_, expired, _ := dict.Get(key) //nolint:errcheck
	if expired {
		L.Push(glua.LFalse)
		L.Push(glua.LString("not found"))
		return 2
	}

	setOK, err := dict.Set(key, value, ttl)
	if err != nil {
		L.Push(glua.LFalse)
		L.Push(glua.LString(err.Error()))
		return 2
	}
	if !setOK {
		L.Push(glua.LFalse)
		L.Push(glua.LString("no memory"))
		return 2
	}
	L.Push(glua.LTrue)
	return 1
}

// dictIncr 自增数值
// dict:incr(key, value) -> new_value, err
func dictIncr(L *glua.LState) int {
	dict := checkSharedDict(L)

	key := L.CheckString(2)
	increment := int(L.CheckNumber(3))

	newValue, err := dict.Incr(key, increment)
	if err != nil {
		L.Push(glua.LNil)
		L.Push(glua.LString(err.Error()))
		return 2
	}
	L.Push(glua.LNumber(newValue))
	return 1
}

// dictDelete 删除条目
// dict:delete(key) -> ok
func dictDelete(L *glua.LState) int {
	dict := checkSharedDict(L)

	key := L.CheckString(2)
	dict.Delete(key) //nolint:errcheck
	L.Push(glua.LTrue)
	return 1
}

// dictFlushAll 清空字典
// dict:flush_all()
func dictFlushAll(L *glua.LState) int {
	dict := checkSharedDict(L)

	dict.FlushAll() //nolint:errcheck
	return 0
}

// dictFlushExpired 清除过期条目
// dict:flush_expired(max_count?) -> flushed_count
func dictFlushExpired(L *glua.LState) int {
	dict := checkSharedDict(L)

	count := dict.FlushExpired()
	L.Push(glua.LNumber(count))
	return 1
}

// dictGetKeys 获取所有键
// dict:get_keys(max_count?) -> keys
func dictGetKeys(L *glua.LState) int {
	//nolint:ineffassign,unused
	_ = checkSharedDict(L)

	// 暂不实现完整版，返回空表
	keys := L.NewTable()
	L.Push(keys)
	return 1
}

// dictSize 获取条目数
// dict:size() -> count
func dictSize(L *glua.LState) int {
	dict := checkSharedDict(L)

	L.Push(glua.LNumber(dict.Size()))
	return 1
}

// dictFreeSpace 获取剩余容量
// dict:free_space() -> slots
func dictFreeSpace(L *glua.LState) int {
	dict := checkSharedDict(L)

	L.Push(glua.LNumber(dict.FreeSlots()))
	return 1
}
