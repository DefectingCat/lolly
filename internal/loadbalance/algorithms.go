// Package loadbalance 负载均衡包，提供多种负载均衡算法实现。
//
// 该文件包含负载均衡相关的核心逻辑，包括：
//   - 轮询算法（Round Robin）
//   - 加权轮询算法（Weighted Round Robin）
//   - 最少连接算法（Least Connections）
//   - IP 哈希算法（IP Hash）
//   - 一致性哈希算法（Consistent Hash）
//
// 主要用途：
//
//	用于在后端服务器之间分发请求，提高服务可用性和性能。
//
// 作者：xfy
package loadbalance

// ValidAlgorithms 是支持的负载均衡算法列表。
var ValidAlgorithms = []string{
	"round_robin",
	"weighted_round_robin",
	"least_conn",
	"ip_hash",
	"consistent_hash",
}

// IsValidAlgorithm 检查算法是否有效。
func IsValidAlgorithm(alg string) bool {
	if alg == "" {
		return true
	}
	for _, a := range ValidAlgorithms {
		if a == alg {
			return true
		}
	}
	return false
}
