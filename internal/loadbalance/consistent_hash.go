// Package loadbalance 提供一致性哈希负载均衡算法实现。
//
// 该文件实现基于虚拟节点的一致性哈希算法，适用于缓存代理场景。
//
// 主要用途：
//
//	用于将相同键的请求始终路由到同一后端服务器，提高缓存命中率。
//
// 算法特点：
//   - 使用虚拟节点解决数据倾斜问题
//   - 支持 FNV-64a 哈希算法
//   - 支持多种哈希键来源（IP、URI、Header）
//
// 作者：xfy
package loadbalance

import (
	"fmt"
	"hash/fnv"
	"sort"
	"sync"
)

// ConsistentHash 一致性哈希负载均衡器。
//
// 使用虚拟节点将请求均匀分布到各个目标，同时保证相同键的请求
// 始终路由到同一目标。
type ConsistentHash struct {
	circle       map[uint64]*Target
	hashKey      string
	sortedHashes []uint64
	virtualNodes int
	mu           sync.RWMutex
}

// NewConsistentHash 创建一致性哈希负载均衡器。
//
// 参数：
//   - virtualNodes: 每个目标的虚拟节点数，默认 150
//   - hashKey: 哈希键来源，支持 ip、uri、header:X-Name
func NewConsistentHash(virtualNodes int, hashKey string) *ConsistentHash {
	if virtualNodes <= 0 {
		virtualNodes = 150
	}
	return &ConsistentHash{
		virtualNodes: virtualNodes,
		circle:       make(map[uint64]*Target),
		hashKey:      hashKey,
	}
}

// Select 根据默认键选择目标。
//
// 由于一致性哈希需要具体键值，此方法返回 nil。
// 请使用 SelectByKey 方法。
func (c *ConsistentHash) Select(targets []*Target) *Target {
	return c.SelectByKey(targets, "")
}

// SelectByKey 根据指定键选择目标。
//
// 参数：
//   - targets: 可用目标列表
//   - key: 哈希键值（如客户端 IP、URI 等）
//
// 返回值：
//   - *Target: 选中的目标，如果没有健康目标则返回 nil
func (c *ConsistentHash) SelectByKey(targets []*Target, key string) *Target {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// 如果环为空，重建哈希环
	if len(c.circle) == 0 {
		c.mu.RUnlock()
		c.rebuildCircle(targets)
		c.mu.RLock()
	}

	if len(c.sortedHashes) == 0 {
		return nil
	}

	// 计算键的哈希值
	hash := c.hashKeyString(key)

	// 二分查找最近的节点
	idx := sort.Search(len(c.sortedHashes), func(i int) bool {
		return c.sortedHashes[i] >= hash
	})

	// 环形回绕
	if idx >= len(c.sortedHashes) {
		idx = 0
	}

	return c.circle[c.sortedHashes[idx]]
}

// Rebuild 重建哈希环。
//
// 当目标列表发生变化时应调用此方法。
//
// 参数：
//   - targets: 新的目标列表
func (c *ConsistentHash) Rebuild(targets []*Target) {
	c.rebuildCircle(targets)
}

// rebuildCircle 重建哈希环（内部方法，需要持有锁）。
func (c *ConsistentHash) rebuildCircle(targets []*Target) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 清空现有环
	c.circle = make(map[uint64]*Target)
	c.sortedHashes = make([]uint64, 0)

	// 为每个目标添加虚拟节点
	for _, target := range targets {
		if !target.Healthy.Load() {
			continue
		}

		// 确保目标已预计算哈希
		if len(target.VirtualHashes) == 0 {
			target.VirtualHashes = make([]uint64, c.virtualNodes)
			for i := 0; i < c.virtualNodes; i++ {
				key := fmt.Sprintf("%s#%d", target.URL, i)
				target.VirtualHashes[i] = c.hashKeyString(key)
			}
		}

		// 使用预计算的哈希值
		for _, hash := range target.VirtualHashes {
			c.circle[hash] = target
			c.sortedHashes = append(c.sortedHashes, hash)
		}
	}

	// 排序哈希值
	sort.Slice(c.sortedHashes, func(i, j int) bool {
		return c.sortedHashes[i] < c.sortedHashes[j]
	})
}

// hashKeyString 计算字符串的哈希值（使用 FNV-64a）。
func (c *ConsistentHash) hashKeyString(key string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(key))
	return h.Sum64()
}

// PrecomputeHashes 预计算目标的虚拟节点哈希值。
//
// 此方法应在目标初始化时调用，避免在 SelectExcludingByKey 中重复计算哈希值。
// 预计算的哈希值存储在 Target.VirtualHashes 中，用于故障转移场景。
//
// 参数：
//   - targets: 需要预计算哈希的目标列表
//   - virtualNodes: 每个目标的虚拟节点数
func (c *ConsistentHash) PrecomputeHashes(targets []*Target, virtualNodes int) {
	if virtualNodes <= 0 {
		virtualNodes = 150
	}

	for _, target := range targets {
		// 如果已经预计算过且数量匹配，跳过
		if len(target.VirtualHashes) == virtualNodes {
			continue
		}

		// 预计算该目标的所有虚拟节点哈希
		target.VirtualHashes = make([]uint64, virtualNodes)
		for i := 0; i < virtualNodes; i++ {
			key := fmt.Sprintf("%s#%d", target.URL, i)
			target.VirtualHashes[i] = c.hashKeyString(key)
		}
	}
}

// GetHashKey 返回哈希键配置。
func (c *ConsistentHash) GetHashKey() string {
	return c.hashKey
}

// GetVirtualNodes 返回虚拟节点数。
func (c *ConsistentHash) GetVirtualNodes() int {
	return c.virtualNodes
}

// ConsistentHashStats 返回一致性哈希统计信息。
type ConsistentHashStats struct {
	// VirtualNodes 每个目标的虚拟节点数量
	VirtualNodes int

	// CircleSize 哈希环中的节点总数
	CircleSize int

	// SortedHashes 排序后的哈希值数量
	SortedHashes int
}

// GetStats 返回统计信息。
func (c *ConsistentHash) GetStats() ConsistentHashStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return ConsistentHashStats{
		VirtualNodes: c.virtualNodes,
		CircleSize:   len(c.circle),
		SortedHashes: len(c.sortedHashes),
	}
}

// SelectExcluding 根据指定键选择目标，排除指定的目标列表。
//
// 参数：
//   - targets: 可用目标列表
//   - excluded: 需要排除的目标列表
//
// 返回值：
//   - *Target: 选中的目标，如果没有可用目标则返回 nil
func (c *ConsistentHash) SelectExcluding(targets []*Target, excluded []*Target) *Target {
	return c.SelectExcludingByKey(targets, excluded, "")
}

// SelectExcludingByKey 根据指定键选择目标，排除指定的目标列表。
//
// 参数：
//   - targets: 可用目标列表
//   - excluded: 需要排除的目标列表
//   - key: 哈希键值（如客户端 IP、URI 等）
//
// 返回值：
//   - *Target: 选中的目标，如果没有可用目标则返回 nil
func (c *ConsistentHash) SelectExcludingByKey(targets []*Target, excluded []*Target, key string) *Target {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// 构建排除集合
	excludeSet := make(map[string]bool, len(excluded))
	for _, t := range excluded {
		if t != nil {
			excludeSet[t.URL] = true
		}
	}

	// 如果没有排除的目标，使用正常选择
	if len(excludeSet) == 0 {
		return c.SelectByKey(targets, key)
	}

	// 使用预计算的虚拟节点哈希构建哈希环
	// 避免在每次调用时重新计算哈希值
	circle := make(map[uint64]*Target)
	sortedHashes := make([]uint64, 0, len(targets)*c.virtualNodes)

	for _, target := range targets {
		if !target.Healthy.Load() || excludeSet[target.URL] {
			continue
		}

		// 确保目标已预计算哈希
		if len(target.VirtualHashes) == 0 {
			// 回退到动态计算（不应该发生，但保持安全）
			c.mu.RUnlock()
			c.PrecomputeHashes([]*Target{target}, c.virtualNodes)
			c.mu.RLock()
		}

		// 使用预计算的哈希值
		for _, hash := range target.VirtualHashes {
			circle[hash] = target
			sortedHashes = append(sortedHashes, hash)
		}
	}

	if len(sortedHashes) == 0 {
		return nil
	}

	// 排序哈希值（仅在需要时）
	// 使用 sort.Slice 进行排序
	sort.Slice(sortedHashes, func(i, j int) bool {
		return sortedHashes[i] < sortedHashes[j]
	})

	// 计算键的哈希值
	hash := c.hashKeyString(key)

	// 二分查找最近的节点
	idx := sort.Search(len(sortedHashes), func(i int) bool {
		return sortedHashes[i] >= hash
	})

	// 环形回绕
	if idx >= len(sortedHashes) {
		idx = 0
	}

	return circle[sortedHashes[idx]]
}

// 验证接口实现
var _ Balancer = (*ConsistentHash)(nil)
