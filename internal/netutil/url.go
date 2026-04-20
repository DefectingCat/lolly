// Package netutil 提供网络相关的通用工具函数。
//
// 该文件包含 URL 解析和主机地址提取的工具函数，
// 供 proxy、middleware、server 等模块共享使用。
//
// 主要功能：
//   - ParseTargetURL: 解析目标 URL，提取主机地址和 TLS 标志
//   - ExtractHost: 简化版 URL 解析，始终添加默认端口
//
// 作者：xfy
package netutil

import "strings"

// ParseTargetURL 解析目标 URL，提取主机地址和 TLS 标志。
//
// 该函数用于统一处理代理模块中的 URL 解析逻辑，支持 http:// 和 https:// 前缀。
//
// 参数：
//   - targetURL: 目标 URL 字符串（如 "http://backend:8080/path" 或 "https://api.example.com"）
//   - addDefaultPort: 是否在没有端口时添加默认端口（:80 或 :443）
//
// 返回值：
//   - addr: 主机地址（格式 host:port）
//   - isTLS: 是否使用 TLS（HTTPS）
//
// 示例：
//
//	addr, isTLS := ParseTargetURL("https://api.example.com/api", true)
//	// addr = "api.example.com:443", isTLS = true
//
//	addr, isTLS := ParseTargetURL("http://backend:8080", false)
//	// addr = "backend:8080", isTLS = false
func ParseTargetURL(targetURL string, addDefaultPort bool) (addr string, isTLS bool) {
	addr = targetURL

	// 处理协议前缀
	if strings.HasPrefix(targetURL, "http://") {
		addr = targetURL[7:]
	} else if strings.HasPrefix(targetURL, "https://") {
		addr = targetURL[8:]
		isTLS = true
	}

	// 移除路径部分，只保留 host:port
	if idx := strings.Index(addr, "/"); idx != -1 {
		addr = addr[:idx]
	}

	// 如果需要，添加默认端口
	if addDefaultPort && !strings.Contains(addr, ":") {
		if isTLS {
			addr = addr + ":443"
		} else {
			addr = addr + ":80"
		}
	}

	return addr, isTLS
}

// ExtractHost 从 URL 提取主机地址（host:port）。
//
// 该函数是 ParseTargetURL 的简化版本，始终添加默认端口，
// 用于需要完整地址但不需要 TLS 标志的场景。
//
// 参数：
//   - targetURL: 目标 URL 字符串
//
// 返回值：
//   - string: 主机地址（格式 host:port）
//
// 示例：
//
//	host := ExtractHost("https://api.example.com/api")
//	// host = "api.example.com:443"
func ExtractHost(targetURL string) string {
	addr, _ := ParseTargetURL(targetURL, true)
	return addr
}
