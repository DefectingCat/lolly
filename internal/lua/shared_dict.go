// Package lua 提供 Lua 脚本嵌入能力
package lua

import (
	"container/list"
	"sync"
	"time"
)

// SharedDict 共享内存字典
// 支持并发安全的 key-value 存储，带 LRU 汰出策略
type SharedDict struct {
	data     map[string]*sharedDictEntry
	lruList  *list.List
	name     string
	maxItems int
	mu       sync.Mutex
}

// sharedDictEntry 字典条目
type sharedDictEntry struct {
	expiredAt time.Time
	element   *list.Element
	key       string
	value     string
}

// NewSharedDict 创建共享字典
func NewSharedDict(name string, maxItems int) *SharedDict {
	return &SharedDict{
		name:     name,
		maxItems: maxItems,
		data:     make(map[string]*sharedDictEntry),
		lruList:  list.New(),
	}
}

// Get 获取值
// 返回 value, expired, err
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

// Set 设置值
// 返回 ok, err (ok=false 表示容量满且无法淘汰)
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

// Add 添加值（仅在不存在时设置）
// 返回 ok, err (ok=false 表示已存在或容量满)
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

// Incr 自增数值
// 返回 new_value, err
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
			return 0, nil // 不是数值
		}
		current = current*10 + int(c-'0')
	}

	newValue := current + increment
	entry.value = intToStr(newValue)
	d.lruList.MoveToFront(entry.element)

	return newValue, nil
}

// Delete 删除条目
func (d *SharedDict) Delete(key string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if entry, ok := d.data[key]; ok {
		d.deleteEntry(entry)
	}
	return nil
}

// FlushAll 清空所有条目
func (d *SharedDict) FlushAll() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.data = make(map[string]*sharedDictEntry)
	d.lruList = list.New()
	return nil
}

// FlushExpired 清除所有过期条目
// 返回清除的条目数
func (d *SharedDict) FlushExpired() int {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.evictExpired()
}

// Size 返回当前条目数
func (d *SharedDict) Size() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.data)
}

// FreeSlots 返回剩余容量
func (d *SharedDict) FreeSlots() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.maxItems - len(d.data)
}

// deleteEntry 删除条目（内部方法，已持有锁）
func (d *SharedDict) deleteEntry(entry *sharedDictEntry) {
	d.lruList.Remove(entry.element)
	delete(d.data, entry.key)
}

// evictExpired 淘汰过期条目（内部方法，已持有锁）
func (d *SharedDict) evictExpired() int {
	now := time.Now()
	count := 0

	// 从 LRU 链表尾部（最久未使用）开始检查
	for elem := d.lruList.Back(); elem != nil; {
		//nolint:errcheck // 类型断言检查
		key := elem.Value.(string)
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

// evictLRU 淘汰 LRU 最久未使用的条目（内部方法，已持有锁）
func (d *SharedDict) evictLRU() bool {
	if d.lruList.Len() == 0 {
		return false
	}

	elem := d.lruList.Back()
	if elem == nil {
		return false
	}

	//nolint:errcheck // 类型断言检查
	key := elem.Value.(string)
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
