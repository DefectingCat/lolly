// Package lua 提供 Lua 脚本嵌入能力。
//
// 该文件实现共享内存字典（SharedDict），包括：
//   - SharedDict：并发安全的 key-value 存储，带 LRU 淘汰策略
//   - 过期机制：支持 TTL 过期，惰性删除 + 主动清理
//   - 数值操作：Incr 支持原子自增
//
// 注意事项：
//   - 所有公开方法均为并发安全（使用 sync.Mutex）
//   - 容量满时优先淘汰过期条目，其次淘汰 LRU 最久未使用的条目
//
// 作者：xfy
package lua

import (
	"container/list"
	"fmt"
	"sync"
	"time"
)

// SharedDict 共享内存字典。
//
// 提供并发安全的 key-value 存储，兼容 ngx.shared.DICT API。
// 特性：
//   - 支持 TTL 过期（惰性删除 + FlushExpired 主动清理）
//   - LRU 淘汰策略（容量满时淘汰最久未使用的条目）
//   - 所有公开方法均为并发安全
type SharedDict struct {
	// data 键值存储映射
	data map[string]*sharedDictEntry

	// lruList LRU 链表，用于淘汰策略
	lruList *list.List

	// 字典名称
	name string

	// 最大条目数
	maxItems int

	// 互斥锁
	mu sync.Mutex
}

// sharedDictEntry 字典条目。
//
// 存储单个 key-value 对及其过期信息和 LRU 链表引用。
type sharedDictEntry struct {
	// expiredAt 过期时间，零值表示永不过期
	expiredAt time.Time

	// element LRU 链表中的节点引用
	element *list.Element

	// 键名
	key string

	// 值（字符串类型）
	value string
}

// NewSharedDict 创建新的共享字典实例。
//
// 参数：
//   - name: 字典名称（用于标识）
//   - maxItems: 最大条目数（达到上限时触发 LRU 淘汰）
//
// 返回值：
//   - *SharedDict: 初始化的字典实例
func NewSharedDict(name string, maxItems int) *SharedDict {
	return &SharedDict{
		name:     name,
		maxItems: maxItems,
		data:     make(map[string]*sharedDictEntry),
		lruList:  list.New(),
	}
}

// Get 获取指定键的值。
//
// 返回值：
//   - value: 存储的值，不存在或过期时返回空字符串
//   - expired: 是否存在但已过期（true=存在但过期，false=不存在或未过期）
//   - err: 错误信息（当前实现始终返回 nil）
//
// 注意：访问成功时会更新 LRU 位置（移到链表前端）。
func (d *SharedDict) Get(key string) (string, bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	entry, ok := d.data[key]
	if !ok {
		return "", false, nil // 不存在
	}

	// 检查过期
	if !entry.expiredAt.IsZero() && time.Now().After(entry.expiredAt) {
		// 已过期，删除并返回
		d.deleteEntry(entry)
		return "", true, nil // 存在但已过期
	}

	// 更新 LRU - 移到前端
	d.lruList.MoveToFront(entry.element)

	return entry.value, false, nil
}

// Set 设置键值对。
//
// 如果键已存在，更新其值和过期时间。
// 如果是新键且容量已满，先尝试淘汰过期条目，再淘汰 LRU 条目。
//
// 参数：
//   - key: 键名
//   - value: 值
//   - ttl: 过期时间，零值表示永不过期
//
// 返回值：
//   - ok: true 表示设置成功，false 表示容量满且无法淘汰
//   - err: 错误信息（当前实现始终返回 nil）
func (d *SharedDict) Set(key, value string, ttl time.Duration) (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// 检查是否已存在
	if entry, ok := d.data[key]; ok {
		// 更新现有条目
		entry.value = value
		if ttl > 0 {
			entry.expiredAt = time.Now().Add(ttl)
		} else {
			entry.expiredAt = time.Time{} // 清除过期时间
		}
		d.lruList.MoveToFront(entry.element)
		return true, nil
	}

	// 新条目，检查容量
	if len(d.data) >= d.maxItems {
		// 尝试淘汰过期条目
		d.evictExpired()
		if len(d.data) >= d.maxItems {
			// 淘汰 LRU 最久未使用的条目
			if !d.evictLRU() {
				return false, nil // 无法淘汰（字典为空？）
			}
		}
	}

	// 创建新条目
	expiredAt := time.Time{}
	if ttl > 0 {
		expiredAt = time.Now().Add(ttl)
	}

	element := d.lruList.PushFront(key)
	entry := &sharedDictEntry{
		key:       key,
		value:     value,
		expiredAt: expiredAt,
		element:   element,
	}
	d.data[key] = entry

	return true, nil
}

// Add 添加键值对（仅在键不存在时设置）。
//
// 与 Set 的区别：如果键已存在（包括已过期的条目），Add 会返回 false。
//
// 参数：
//   - key: 键名
//   - value: 值
//   - ttl: 过期时间
//
// 返回值：
//   - ok: true 表示添加成功，false 表示已存在或容量满
//   - err: 错误信息
func (d *SharedDict) Add(key, value string, ttl time.Duration) (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// 检查是否已存在（包括过期的也算存在）
	if _, ok := d.data[key]; ok {
		return false, nil // 已存在
	}

	// 检查容量并淘汰
	if len(d.data) >= d.maxItems {
		d.evictExpired()
		if len(d.data) >= d.maxItems {
			if !d.evictLRU() {
				return false, nil
			}
		}
	}

	// 创建新条目
	expiredAt := time.Time{}
	if ttl > 0 {
		expiredAt = time.Now().Add(ttl)
	}

	element := d.lruList.PushFront(key)
	entry := &sharedDictEntry{
		key:       key,
		value:     value,
		expiredAt: expiredAt,
		element:   element,
	}
	d.data[key] = entry

	return true, nil
}

// Incr 将指定键的值作为整数自增。
//
// 如果键不存在，创建初始值为 0 后再自增。
// 如果值不是纯数字字符串，返回 0（非错误）。
//
// 参数：
//   - key: 键名
//   - increment: 自增量（可为负数）
//
// 返回值：
//   - newValue: 自增后的值
//   - err: 错误信息（当前实现始终返回 nil）
func (d *SharedDict) Incr(key string, increment int) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	entry, ok := d.data[key]
	if !ok {
		// 不存在，创建初始值
		if len(d.data) >= d.maxItems {
			d.evictExpired()
			if len(d.data) >= d.maxItems {
				if !d.evictLRU() {
					return 0, nil // 无法创建
				}
			}
		}

		element := d.lruList.PushFront(key)
		entry = &sharedDictEntry{
			key:     key,
			value:   "0",
			element: element,
		}
		d.data[key] = entry
	}

	// 解析数值
	var current int
	for _, c := range entry.value {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("not a number")
		}
		current = current*10 + int(c-'0')
	}

	newValue := current + increment
	entry.value = intToStr(newValue)
	d.lruList.MoveToFront(entry.element)

	return newValue, nil
}

// Delete 删除指定键。
func (d *SharedDict) Delete(key string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if entry, ok := d.data[key]; ok {
		d.deleteEntry(entry)
	}
	return nil
}

// FlushAll 清空字典中的所有条目。
func (d *SharedDict) FlushAll() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.data = make(map[string]*sharedDictEntry)
	d.lruList = list.New()
	return nil
}

// FlushExpired 清除所有过期条目。
//
// 返回值：
//   - int: 被清除的条目数
func (d *SharedDict) FlushExpired() int {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.evictExpired()
}

// Size 返回当前条目数（包括已过期的条目）。
func (d *SharedDict) Size() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.data)
}

// FreeSlots 返回剩余可添加的条目数。
func (d *SharedDict) FreeSlots() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.maxItems - len(d.data)
}

// deleteEntry 删除指定条目（内部方法，需已持有锁）。
func (d *SharedDict) deleteEntry(entry *sharedDictEntry) {
	d.lruList.Remove(entry.element)
	delete(d.data, entry.key)
}

// evictExpired 淘汰所有过期条目（内部方法，需已持有锁）。
//
// 从 LRU 链表尾部（最久未使用）开始扫描，删除过期条目。
//
// 返回值：
//   - int: 被淘汰的条目数
func (d *SharedDict) evictExpired() int {
	now := time.Now()
	count := 0

	// 从 LRU 链表尾部（最久未使用）开始检查
	for elem := d.lruList.Back(); elem != nil; {
		// 类型断言检查
		key, ok := elem.Value.(string)
		if !ok {
			// 类型不正确，移除元素
			next := elem.Prev()
			d.lruList.Remove(elem)
			elem = next
			continue
		}
		entry, ok := d.data[key]
		if !ok {
			// 数据不一致，跳过
			next := elem.Prev()
			d.lruList.Remove(elem)
			elem = next
			continue
		}

		if !entry.expiredAt.IsZero() && now.After(entry.expiredAt) {
			// 已过期，删除
			d.deleteEntry(entry)
			count++
			elem = d.lruList.Back() // 重新从尾部开始
		} else {
			break // 未过期，停止（链表顺序保证前面都是未过期的）
		}
	}

	return count
}

// evictLRU 淘汰 LRU 最久未使用的条目（内部方法，需已持有锁）。
//
// 返回值：
//   - bool: true 表示成功淘汰，false 表示链表为空
func (d *SharedDict) evictLRU() bool {
	if d.lruList.Len() == 0 {
		return false
	}

	elem := d.lruList.Back()
	if elem == nil {
		return false
	}

	// 类型断言检查
	key, ok := elem.Value.(string)
	if !ok {
		// 类型不正确，移除链表元素
		d.lruList.Remove(elem)
		return d.evictLRU()
	}
	entry, ok := d.data[key]
	if ok {
		d.deleteEntry(entry)
		return true
	}

	// 数据不一致，移除链表元素并重试
	d.lruList.Remove(elem)
	return d.evictLRU()
}

// intToStr 整数转字符串（简单实现，避免 strconv 依赖）
func intToStr(n int) string {
	if n == 0 {
		return "0"
	}

	var negative bool
	if n < 0 {
		negative = true
		n = -n
	}

	var buf []byte
	for n > 0 {
		buf = append(buf, byte('0'+n%10))
		n /= 10
	}

	if negative {
		buf = append(buf, '-')
	}

	// 反转
	for i, j := 0, len(buf)-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
	}

	return string(buf)
}
