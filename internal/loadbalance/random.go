package loadbalance

import (
	"math/rand/v2"
	"sync/atomic"
)

// Random 实现随机负载均衡（Power of Two Choices 算法）。
//
// 随机选择两个候选目标，从中选择连接数较少的那个。
// 相比纯随机，Power of Two Choices 能更好地均衡负载，
// 同时保持 O(1) 的选择复杂度。
//
// 当只有一个候选目标时直接返回；当 MaxConns 限制生效时
// 自动跳过已满的目标。
type Random struct{}

// NewRandom 创建一个新的随机负载均衡器。
func NewRandom() *Random {
	return &Random{}
}

// Select 使用 Power of Two Choices 算法选择目标。
// 随机选择两个候选，返回连接数较少的那个。
// 只考虑可用目标。如果没有可用目标则返回 nil。
func (r *Random) Select(targets []*Target) *Target {
	available := filterHealthy(targets)
	if len(available) == 0 {
		return nil
	}

	if len(available) == 1 {
		return available[0]
	}

	// Power of Two Choices
	i := rand.IntN(len(available))
	j := rand.IntN(len(available) - 1)
	if j >= i {
		j++
	}

	a, b := available[i], available[j]
	if atomic.LoadInt64(&a.Connections) <= atomic.LoadInt64(&b.Connections) {
		return a
	}
	return b
}

// SelectExcluding 使用 Power of Two Choices 算法选择目标，排除指定的目标列表。
func (r *Random) SelectExcluding(targets []*Target, excluded []*Target) *Target {
	available := filterHealthyAndExclude(targets, excluded)
	if len(available) == 0 {
		return nil
	}

	if len(available) == 1 {
		return available[0]
	}

	i := rand.IntN(len(available))
	j := rand.IntN(len(available) - 1)
	if j >= i {
		j++
	}

	a, b := available[i], available[j]
	if atomic.LoadInt64(&a.Connections) <= atomic.LoadInt64(&b.Connections) {
		return a
	}
	return b
}
