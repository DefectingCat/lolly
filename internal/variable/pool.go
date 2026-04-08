// pool.go - VariableContext 池管理
//
// 提供 sync.Pool 复用 VariableContext，减少 GC 压力。
//
// 作者：xfy
package variable

import (
	"sync"

	"github.com/valyala/fasthttp"
)

// PoolStats 池统计信息
type PoolStats struct {
	Gets     int64 // Get 次数
	Puts     int64 // Put 次数
	NewCount int64 // New 创建次数
	Active   int64 // 当前活跃数量 (Gets - Puts)
}

var (
	// stats 池统计
	stats PoolStats
	// statsMu 保护统计信息
	statsMu sync.RWMutex
)

// GetStats 获取池统计信息
func GetStats() PoolStats {
	statsMu.RLock()
	s := stats
	statsMu.RUnlock()
	return s
}

// GetPool 获取底层的 sync.Pool（用于测试和调试）
func GetPool() *sync.Pool {
	return &pool
}

// PoolGet 从池中获取 VariableContext（包装方法，用于统计）
func PoolGet(ctx *fasthttp.RequestCtx) *VariableContext {
	vc := pool.Get().(*VariableContext)

	// 初始化
	vc.ctx = ctx
	vc.status = 0
	vc.bodySize = 0
	vc.duration = 0
	vc.serverName = ""

	// 清空缓存和自定义变量
	for k := range vc.cache {
		delete(vc.cache, k)
	}
	for k := range vc.store {
		delete(vc.store, k)
	}

	// 更新统计
	statsMu.Lock()
	stats.Gets++
	stats.Active = stats.Gets - stats.Puts
	statsMu.Unlock()

	return vc
}

// PoolPut 将 VariableContext 放回池中（包装方法，用于统计）
func PoolPut(vc *VariableContext) {
	if vc == nil {
		return
	}

	// 清理引用
	vc.ctx = nil
	vc.status = 0
	vc.bodySize = 0
	vc.duration = 0
	vc.serverName = ""

	pool.Put(vc)

	// 更新统计
	statsMu.Lock()
	stats.Puts++
	stats.Active = stats.Gets - stats.Puts
	statsMu.Unlock()
}

// ResetStats 重置统计信息
func ResetStats() {
	statsMu.Lock()
	stats = PoolStats{}
	statsMu.Unlock()
}
