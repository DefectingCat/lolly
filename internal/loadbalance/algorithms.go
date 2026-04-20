// Package loadbalance 提供负载均衡算法实现。
//
// 该文件包含负载均衡算法相关的辅助逻辑，包括：
//   - ValidAlgorithms 有效算法列表定义
//   - IsValidAlgorithm 算法有效性校验函数
//
// 主要用途：
//
//	用于校验和枚举系统支持的负载均衡算法类型，供配置解析和路由选择使用。
//
// 注意事项：
//   - 新增算法时必须同步更新 ValidAlgorithms 列表
//   - 空字符串被视为有效值，表示使用默认算法
//
// 作者：xfy
package loadbalance

import "slices"

// ValidAlgorithms 定义系统支持的负载均衡算法名称列表。
//
// 该列表用于配置校验，包含以下算法：
//   - round_robin: 简单轮询，按顺序均匀分配请求
//   - weighted_round_robin: 加权轮询，按权重比例分配请求
//   - least_conn: 最少连接，选择活跃连接数最少的目标
//   - ip_hash: IP 哈希，基于客户端 IP 实现会话保持
//   - consistent_hash: 一致性哈希，使用虚拟节点实现均匀分布
var ValidAlgorithms = []string{
	"round_robin",
	"weighted_round_robin",
	"least_conn",
	"ip_hash",
	"consistent_hash",
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
//   - true: 算法有效（在 ValidAlgorithms 列表中或为空字符串）
//   - false: 算法无效（不在 ValidAlgorithms 列表中且非空）
func IsValidAlgorithm(alg string) bool {
	if alg == "" {
		return true
	}
	return slices.Contains(ValidAlgorithms, alg)
}
