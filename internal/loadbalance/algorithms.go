// Package loadbalance 提供负载均衡算法实现。
//
// 该文件包含负载均衡算法相关的辅助逻辑，包括：
//   - IsValidAlgorithm 算法有效性校验函数
//
// 主要用途：
//
//	用于校验和枚举系统支持的负载均衡算法类型，供配置解析和路由选择使用。
//
// 注意事项：
//   - 新增算法时必须同步更新 validAlgorithms 列表
//   - 空字符串被视为有效值，表示使用默认算法
//
// 作者：xfy
package loadbalance

import "slices"

// validAlgorithms 定义系统支持的负载均衡算法名称列表。
var validAlgorithms = []string{
	"round_robin",
	"weighted_round_robin",
	"least_conn",
	"ip_hash",
	"consistent_hash",
	"random",
}

// IsValidAlgorithm 检查给定的算法名称是否有效。
//
// 该函数用于配置解析阶段校验算法参数的合法性。
// 空字符串被视为有效值，表示由调用方选择默认算法。
//
// 参数：
//   - alg: 算法名称字符串，如 "round_robin"、"least_conn" 等
//
// 返回值：
//   - true: 算法有效（在 validAlgorithms 列表中或为空字符串）
//   - false: 算法无效（不在 validAlgorithms 列表中且非空）
func IsValidAlgorithm(alg string) bool {
	if alg == "" {
		return true
	}
	return slices.Contains(validAlgorithms, alg)
}
