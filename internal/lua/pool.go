// Package lua 提供 Lua 脚本嵌入能力。
//
// 该文件实现 LState 池，用于解决 gopher-lua 并发竞态问题。
// 每个请求从池中获取完全独立的 LState（各自拥有独立的 Global 对象），
// 彻底消除共享状态导致的 concurrent map read/write 问题。
//
// 设计要点：
//   - 每个 LState 独立创建，拥有独立的 Global 表
//   - 池化管理，避免频繁创建/销毁开销
//   - 动态伸缩，支持最大容量限制
//   - 工厂函数模式，统一 LState 初始化逻辑
//
// 作者：xfy
package lua

import (
	"sync"

	glua "github.com/yuin/gopher-lua"
)

// LStatePool 管理 LState 池。
//
// 每个请求从池中获取完全独立的 LState，彻底消除共享状态。
// 池支持动态伸缩，初始预热一定数量，运行时按需创建，
// 最大容量限制防止资源耗尽。
type LStatePool struct {
	mu      sync.Mutex
	pool    []*glua.LState
	factory func() *glua.LState
	maxSize int
	current int
}

// NewLStatePool 创建 LState 池。
//
// 参数：
//   - factory: LState 工厂函数，负责创建并初始化独立 LState
//   - initialSize: 初始池大小（预热数量）
//   - maxSize: 最大池大小（防止资源耗尽）
//
// 返回值：
//   - *LStatePool: 池实例
func NewLStatePool(factory func() *glua.LState, initialSize, maxSize int) *LStatePool {
	p := &LStatePool{
		pool:    make([]*glua.LState, 0, maxSize),
		factory: factory,
		maxSize: maxSize,
		current: 0,
	}

	// 预热：预先创建 initialSize 个 LState
	for i := 0; i < initialSize; i++ {
		L := p.factory()
		p.pool = append(p.pool, L)
		p.current++
	}

	return p
}

// Get 从池中获取一个 LState。
//
// 如果池中有可用 LState，直接返回；否则创建新的 LState（不超过 maxSize）。
//
// 返回值：
//   - *glua.LState: 可用的 LState 实例
//   - error: 池已满时返回错误
func (p *LStatePool) Get() *glua.LState {
	p.mu.Lock()
	defer p.mu.Unlock()

	// 池中有可用 LState
	if len(p.pool) > 0 {
		// 取出最后一个（避免数组移动）
		n := len(p.pool) - 1
		L := p.pool[n]
		p.pool[n] = nil // 防止内存泄漏
		p.pool = p.pool[:n]
		return L
	}

	// 池为空，检查是否可以创建新的
	if p.current >= p.maxSize {
		// 已达最大容量，等待或返回 nil
		// 这里选择返回 nil，由调用方处理
		return nil
	}

	// 创建新的 LState
	L := p.factory()
	p.current++
	return L
}

// Put 将 LState 归还池中。
//
// 归还前应确保 LState 处于干净状态（无残留数据）。
// 如果池已满（归还后超过 maxSize），直接关闭 LState。
//
// 参数：
//   - L: 要归还的 LState
func (p *LStatePool) Put(L *glua.LState) {
	if L == nil {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	// 检查池容量
	if len(p.pool) >= p.maxSize {
		// 池已满，直接关闭
		L.Close()
		p.current--
		return
	}

	// 归还到池中
	p.pool = append(p.pool, L)
}

// Close 关闭池，释放所有 LState。
//
// 关闭所有池中的 LState，并清空池。
func (p *LStatePool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()

	// 关闭所有 LState
	for _, L := range p.pool {
		if L != nil {
			L.Close()
		}
	}
	p.pool = nil
	p.current = 0
}

// Size 返回池当前大小（包括已借出的）。
func (p *LStatePool) Size() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.current
}

// Available 返回池中可用的 LState 数量。
func (p *LStatePool) Available() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.pool)
}