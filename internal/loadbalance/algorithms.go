// Package loadbalance 负载均衡包为 Lolly HTTP 服务器提供负载均衡算法。
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
