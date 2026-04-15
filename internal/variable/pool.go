// Package variable 提供 sync.Pool 用于复用 Context，减少 GC 压力。
//
// 包含池统计信息、Get/Put 包装方法和统计重置功能。
//
// 作者：xfy
package variable

import (
	"sync"
	"sync/atomic"
)

// PoolStats 池统计信息。
//
// 记录 sync.Pool 的使用统计，用于监控和调试。
type PoolStats struct {
	// Gets 从池中获取对象的次数
	Gets int64
	// Puts 放回池中对象的次数
	Puts int64
	// NewCount 调用 New 函数创建对象的次数
	NewCount int64
	// Active 当前活跃对象数量（Gets - Puts）
	Active int64
}

var (
	// gets 从池中获取对象的次数
	gets atomic.Int64
	// puts 放回池中对象的次数
	puts atomic.Int64
	// newCount 调用 New 函数创建对象的次数
	newCount atomic.Int64
	// active 当前活跃对象数量（Gets - Puts）
	active atomic.Int64
)

// GetStats 获取池统计信息的副本。
//
// 返回当前统计信息的快照，线程安全。
func GetStats() PoolStats {
	return PoolStats{
		Gets:     gets.Load(),
		Puts:     puts.Load(),
		NewCount: newCount.Load(),
		Active:   active.Load(),
	}
}

// GetPool 获取底层的 sync.Pool（用于测试和调试）。
func GetPool() *sync.Pool {
	return &pool
}

// ResetStats 重置统计信息。
//
// 将所有统计计数器清零，线程安全。
func ResetStats() {
	gets.Store(0)
	puts.Store(0)
	newCount.Store(0)
	active.Store(0)
}
