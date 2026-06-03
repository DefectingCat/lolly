// Package lua 提供 Lua 脚本扩展功能，支持 IP 黑白名单、请求处理等。
//
// 包含 IP 守卫相关的逻辑，用于处理 IP 黑白名单功能。
//
// 作者：xfy
package lua

import "net"

// isRestrictedIP 检查 IP 地址是否属于受限范围（私有、回环、链路本地等）。
//
// 用于防止 Lua Cosocket 的 SSRF 攻击。
func isRestrictedIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsUnspecified()
}
