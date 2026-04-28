package lua

import "net"

// isRestrictedIP 检查 IP 地址是否属于受限范围（私有、回环、链路本地等）。
//
// 用于防止 Lua Cosocket 的 SSRF 攻击。
func isRestrictedIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsUnspecified()
}
