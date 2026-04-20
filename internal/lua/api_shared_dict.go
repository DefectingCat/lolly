// Package lua 提供 ngx.shared.DICT API 实现。
//
// 该文件实现共享内存字典的 Lua API，兼容 OpenResty/ngx_lua 语义。
// 包括：
//   - SharedDictManager：管理多个命名的 SharedDict 实例
//   - Lua 元表注册：支持 dict:get/set/add/replace/incr/delete 等方法
//   - dict:flush_all/flush_expired/get_keys/size/free_space 管理方法
//
// 安全说明：
//   - 共享字典仅支持字符串键值对
//   - incr 操作使用手动整数解析（不使用 strconv 依赖）
//   - Scheduler 模式下共享字典仍然可用
//
// 作者：xfy
package lua

import (
	"sync"
	"time"

	glua "github.com/yuin/gopher-lua"
)

// SharedDictManager 共享字典管理器。
//
// 管理多个命名的 SharedDict 实例，支持并发安全的创建、查询和清理操作。
type SharedDictManager struct {
	// dicts 字典名称到实例的映射
	dicts map[string]*SharedDict

	// mu 读写锁
	mu sync.RWMutex
}

// NewSharedDictManager 创建共享字典管理器实例。
//
// 返回值：
//   - *SharedDictManager: 初始化的管理器实例
func NewSharedDictManager() *SharedDictManager {
	return &SharedDictManager{
		dicts: make(map[string]*SharedDict),
	}
}

// CreateDict 创建或获取指定名称的共享字典。
//
// 如果字典已存在则直接返回，否则创建新字典。
//
// 参数：
//   - name: 字典名称
//   - maxItems: 最大条目数（LRU 淘汰阈值）
//
// 返回值：
//   - *SharedDict: 共享字典实例
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

// GetDict 获取指定名称的共享字典。
//
// 参数：
//   - name: 字典名称
//
// 返回值：
//   - *SharedDict: 字典实例，不存在时返回 nil
func (m *SharedDictManager) GetDict(name string) *SharedDict {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.dicts[name]
}

// Close 清理所有字典引用。
//
// 将 dicts 映射置为 nil，释放所有字典引用。
func (m *SharedDictManager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dicts = nil
}

// DictConfig 共享字典配置。
type DictConfig struct {
	// Name 字典名称
	Name string

	// MaxItems 最大条目数
	MaxItems int
}

// RegisterSharedDictAPI 注册 ngx.shared.DICT API 到 Lua 状态机。
//
// 在 ngx 表下创建 shared 子表和字典元表，
// 注册 get/set/add/replace/incr/delete/flush_all/flush_expired/get_keys/size/free_space 方法。
//
// 参数：
//   - L: Lua 状态
//   - manager: 共享字典管理器
//   - ngx: ngx 全局表
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

// checkSharedDict 从 Lua 调用参数中验证并获取 SharedDict 实例。
//
// 参数：
//   - L: Lua 状态
//
// 返回值：
//   - *SharedDict: 字典实例，类型不正确时引发 Lua 错误
func checkSharedDict(L *glua.LState) *SharedDict {
	ud := L.CheckUserData(1)
	dict, ok := ud.Value.(*SharedDict)
	if !ok {
		L.RaiseError("invalid shared dict")
	}
	return dict
}

// dictIndex 字典 __index 元方法。
//
// 先检查是否为方法名（如 get、set 等），若是则返回方法；
// 否则作为 key 获取字典中的值。
func dictIndex(L *glua.LState) int {
	dict := checkSharedDict(L)

	key := L.CheckString(2)

	// 检查是否是方法

	ud, ok := L.Get(1).(*glua.LUserData)
	if !ok {
		L.RaiseError("expected userdata")
		return 0
	}
	methods := L.GetField(ud.Metatable, "methods")
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

// dictNewIndex 字典 __newindex 元方法。
//
// 处理 dict[key] = value 形式的赋值，设置永不过期的键值对。
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

// dictToString 字典 __tostring 元方法，返回 "ngx.shared.dict:{name}" 格式字符串。
func dictToString(L *glua.LState) int {
	dict := checkSharedDict(L)
	L.Push(glua.LString("ngx.shared.dict:" + dict.name))
	return 1
}

// dictGet 实现 dict:get(key) 方法。
//
// 获取指定键的值，支持过期检测。
//
// 返回值（推入 Lua 栈）：
//   - value: 键对应的值，不存在或过期时返回 nil
//   - flags: 标志位（当前始终返回 0）
//   - err: 错误信息（不存在时不返回）
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

// dictSet 实现 dict:set(key, value, exptime?, flags?) 方法。
//
// 设置键值对，支持可选过期时间（秒）。
//
// 返回值（推入 Lua 栈）：
//   - ok: true 表示设置成功
//   - err: 失败时的错误信息
func dictSet(L *glua.LState) int {
	dict := checkSharedDict(L)

	key := L.CheckString(2)
	value := L.CheckString(3)

	ttl := time.Duration(0)
	if L.GetTop() >= 4 {
		ttl = time.Duration(float64(L.CheckNumber(4)) * float64(time.Second))
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

// dictAdd 实现 dict:add(key, value, exptime?, flags?) 方法。
//
// 仅在键不存在时添加，存在时返回错误。
func dictAdd(L *glua.LState) int {
	dict := checkSharedDict(L)

	key := L.CheckString(2)
	value := L.CheckString(3)

	ttl := time.Duration(0)
	if L.GetTop() >= 4 {
		ttl = time.Duration(float64(L.CheckNumber(4)) * float64(time.Second))
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

// dictReplace 实现 dict:replace(key, value, exptime?, flags?) 方法。
//
// 仅在键存在且未过期时替换值，不存在时返回 "not found" 错误。
func dictReplace(L *glua.LState) int {
	dict := checkSharedDict(L)

	key := L.CheckString(2)
	value := L.CheckString(3)

	ttl := time.Duration(0)
	if L.GetTop() >= 4 {
		ttl = time.Duration(float64(L.CheckNumber(4)) * float64(time.Second))
	}

	// 检查是否存在（Get 返回: value, expired, error）
	// expired=true 表示存在但已过期，expired=false 表示不存在或存在且未过期
	// 需要区分不存在和存在的情况
	val, expired, _ := dict.Get(key)
	// 如果 val 为空且 expired 为 false，表示 key 不存在
	// 如果 expired 为 true，表示 key 存在但已过期（也算不存在）
	if val == "" && !expired {
		// key 不存在
		L.Push(glua.LFalse)
		L.Push(glua.LString("not found"))
		return 2
	}
	if expired {
		// key 存在但已过期，也算不存在
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

// dictIncr 实现 dict:incr(key, value) 方法。
//
// 将指定键的值作为整数自增，返回新值。
// 如果键不存在则创建初始值为 0 再自增。
// 如果值不是纯数字，返回 nil 和错误。
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
	if newValue == 0 {
		// Incr 返回 0 表示值不是纯数字字符串（非错误）
		// 在 Lua 中返回 nil 表示操作失败
		L.Push(glua.LNil)
		L.Push(glua.LString("not a number"))
		return 2
	}
	L.Push(glua.LNumber(newValue))
	return 1
}

// dictDelete 实现 dict:delete(key) 方法。
//
// 删除指定键。
func dictDelete(L *glua.LState) int {
	dict := checkSharedDict(L)

	key := L.CheckString(2)
	_ = dict.Delete(key) // Delete 返回错误，但在 Lua API 中忽略
	L.Push(glua.LTrue)
	return 1
}

// dictFlushAll 实现 dict:flush_all() 方法。
//
// 清空字典中的所有条目。
func dictFlushAll(L *glua.LState) int {
	dict := checkSharedDict(L)

	_ = dict.FlushAll() // FlushAll 返回错误，但在 Lua API 中忽略
	return 0
}

// dictFlushExpired 实现 dict:flush_expired(max_count?) 方法。
//
// 清除所有过期条目，返回被清除的数量。
func dictFlushExpired(L *glua.LState) int {
	dict := checkSharedDict(L)

	count := dict.FlushExpired()
	L.Push(glua.LNumber(count))
	return 1
}

// dictGetKeys 实现 dict:get_keys(max_count?) 方法。
//
// 获取字典中的所有键。当前实现返回空表（待完善）。
func dictGetKeys(L *glua.LState) int {
	checkSharedDict(L) // 验证参数但不使用返回值

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
