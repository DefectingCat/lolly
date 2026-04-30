// Package utils 提供通用工具函数
package utils

import (
	"net"
)

// ParseIPAllowList 解析 IP/CIDR 白名单列表。
//
// 支持格式：
//   - CIDR 格式：192.168.1.0/24、::1/128
//   - 单个 IP：192.168.1.1（自动转换为 /32 或 /128）
//   - 特殊值 "localhost"：映射为 127.0.0.1/32 和 ::1/128
//
// 参数：
//   - allow: IP/CIDR 字符串列表
//
// 返回值：
//   - []net.IPNet: 解析后的网络列表
//   - error: 解析失败时返回错误
func ParseIPAllowList(allow []string) ([]net.IPNet, error) {
	if len(allow) == 0 {
		return nil, nil
	}

	result := make([]net.IPNet, 0, len(allow)+2) // +2 for localhost expansion

	for _, cidr := range allow {
		// 处理 localhost 特殊情况
		if cidr == "localhost" {
			// localhost 解析为 127.0.0.1 和 ::1
			if v4Net, err := parseCIDR("127.0.0.1/32"); err == nil {
				result = append(result, *v4Net)
			}
			if v6Net, err := parseCIDR("::1/128"); err == nil {
				result = append(result, *v6Net)
			}
			continue
		}

		// 尝试 CIDR 解析
		_, network, err := net.ParseCIDR(cidr)
		if err == nil && network != nil {
			result = append(result, *network)
			continue
		}

		// fallback: 尝试作为单个 IP 解析
		ip := net.ParseIP(cidr)
		if ip == nil {
			return nil, err // 返回原始 CIDR 解析错误
		}

		// 转换为 CIDR 格式
		var ipNet *net.IPNet
		if ip.To4() != nil {
			ipNet, _ = parseCIDR(cidr + "/32")
		} else {
			ipNet, _ = parseCIDR(cidr + "/128")
		}
		if ipNet != nil {
			result = append(result, *ipNet)
		}
	}

	return result, nil
}

// parseCIDR 是 net.ParseCIDR 的包装，返回 *net.IPNet 而不返回 net.IP
func parseCIDR(cidr string) (*net.IPNet, error) {
	_, network, err := net.ParseCIDR(cidr)
	return network, err
}

// IPInAllowList 检查 IP 是否在白名单中。
//
// 参数：
//   - ip: 要检查的 IP 地址
//   - allowList: 白名单网络列表
//
// 返回值：
//   - bool: IP 在白名单中返回 true
func IPInAllowList(ip net.IP, allowList []net.IPNet) bool {
	if len(allowList) == 0 {
		return false
	}
	for _, network := range allowList {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}
