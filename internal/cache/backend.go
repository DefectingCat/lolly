// Package cache 提供文件缓存和代理缓存功能。
//
// 该文件定义 CacheBackend 接口，统一内存缓存和磁盘缓存的访问方式。
// 支持分层缓存架构（L1 内存 + L2 磁盘），热点数据常驻内存，冷数据持久化到磁盘。
//
// 主要用途：
//
//	作为缓存后端的抽象层，使 ProxyCache（内存）和 DiskCache（磁盘）可互换使用。
//
// 设计原则：
//   - 接口精简为核心 CRUD 操作
//   - Set 方法无返回值，与现有 ProxyCache.Set 签名一致
//   - Get 返回 stale 标志，支持过期缓存复用
//
// 作者：xfy
package cache

import "time"

// CacheBackend 缓存后端接口。
//
// 统一内存缓存和磁盘缓存的访问方式，支持分层缓存架构。
// 实现包括 ProxyCache（内存）、DiskCache（磁盘）和 TieredCache（分层）。
type CacheBackend interface {
	// Get 获取缓存条目。
	//
	// 参数：
	//   - hashKey: 缓存键的哈希值
	//   - origKey: 原始缓存键（用于双重验证，防止哈希碰撞）
	//
	// 返回值：
	//   - *ProxyCacheEntry: 缓存条目，包含响应数据、头部、状态码等
	//   - bool: 是否存在
	//   - bool: 是否过期（stale），true 表示可使用过期缓存
	Get(hashKey uint64, origKey string) (entry *ProxyCacheEntry, exists bool, stale bool)

	// Set 设置缓存条目。
	//
	// 无返回值，与现有 ProxyCache.Set 签名一致。
	// 写入操作为异步或同步取决于具体实现。
	//
	// 参数：
	//   - hashKey: 缓存键的哈希值
	//   - origKey: 原始缓存键
	//   - data: 响应体数据
	//   - headers: 响应头
	//   - status: HTTP 状态码
	//   - maxAge: 缓存有效期
	Set(hashKey uint64, origKey string, data []byte, headers map[string]string, status int, maxAge time.Duration)

	// Delete 删除缓存条目。
	//
	// 参数：
	//   - hashKey: 缓存键的哈希值
	//
	// 返回值：
	//   - error: 删除失败时返回错误，成功返回 nil
	Delete(hashKey uint64) error

	// CacheStats 返回缓存统计信息。
	//
	// 注意：此方法名与 ProxyCache.Stats() 不同，以保持向后兼容。
	// ProxyCache 同时实现 Stats() 返回 ProxyCacheStats 和 CacheStats() 返回 CacheStats。
	//
	// 返回值：
	//   - CacheStats: 缓存统计数据
	CacheStats() CacheStats
}

// CacheStats 缓存统计信息。
//
// 包含缓存的基本统计数据，用于监控和运维。
type CacheStats struct {
	// Entries 当前缓存条目数量
	Entries int64

	// Size 缓存总大小（字节）
	Size int64

	// HitCount 缓存命中次数
	HitCount int64

	// MissCount 缓存未命中次数
	MissCount int64

	// Evictions 缓存淘汰次数
	Evictions int64
}

// 确保 ProxyCache 实现 CacheBackend 接口
// ProxyCache 已有 Get, Set, Delete, Stats 方法，但签名略有不同：
// - Delete() 无返回值，需要添加包装方法
// - Stats() 返回 ProxyCacheStats，需要适配
