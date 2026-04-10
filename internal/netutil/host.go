// Package netutil 提供网络相关的工具函数。
//
// 该文件包含主机名处理相关的工具函数。
//
// 作者：xfy
package netutil

import (
	"strings"
)

// StripPort 从 Host 头中移除端口号。
//
// 支持 IPv4 和 IPv6 格式：
//   - example.com:8080 -> example.com
//   - [::1]:8080 -> [::1]
//   - [2001:db8::1]:443 -> [2001:db8::1]
//   - example.com -> example.com
//
// 参数：
//   - host: 主机名（可能包含端口）
//
// 返回值：
//   - string: 移除端口后的主机名
func StripPort(host string) string {
	if len(host) == 0 {
		return host
	}

	// IPv6 格式：以 '[' 开头，找 ']:' 作为分隔点
	if host[0] == '[' {
		for i := 0; i < len(host)-1; i++ {
			if host[i] == ']' && host[i+1] == ':' {
				return host[:i+1]
			}
		}
		return host
	}

	// IPv4 或域名格式：找第一个 ':' 作为分隔点
	for i := 0; i < len(host); i++ {
		if host[i] == ':' {
			return host[:i]
		}
	}

	return host
}

// HasPort 检查主机名是否包含端口号。
//
// 参数：
//   - host: 主机名
//
// 返回值：
//   - bool: true 表示包含端口
func HasPort(host string) bool {
	if len(host) == 0 {
		return false
	}

	// IPv6 格式
	if host[0] == '[' {
		return strings.Contains(host, "]:")
	}

	// IPv4 或域名格式
	return strings.Contains(host, ":")
}
