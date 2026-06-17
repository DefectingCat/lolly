// Package netutil 提供网络相关的通用工具函数。
//
// 该文件包含客户端 IP 提取相关的工具函数，
// 从 HTTP 请求中提取真实的客户端 IP 地址。
//
// 作者：xfy
package netutil

import (
	"net"
	"strings"
	"sync"

	"github.com/valyala/fasthttp"
)

// ExtractClientIP 从请求上下文中提取客户端 IP 地址（返回字符串）。
//
// 该函数按以下顺序提取 IP：
// 1. X-Forwarded-For 请求头的第一个 IP（最左侧）
// 2. X-Real-IP 请求头
// 3. RemoteAddr
//
// 注意：此函数不进行可信代理验证，适用于非安全场景（如日志记录）。
// 对于安全场景（如访问控制），应使用特定模块的安全实现。
//
// 参数：
//   - ctx: FastHTTP 请求上下文
//
// 返回值：
//   - string: 客户端 IP 地址字符串
func ExtractClientIP(ctx *fasthttp.RequestCtx) string {
	// 首先检查 X-Forwarded-For 请求头
	if xff := ctx.Request.Header.Peek("X-Forwarded-For"); len(xff) > 0 {
		ips := strings.Split(string(xff), ",")
		if len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}

	// 检查 X-Real-IP 请求头
	if xri := ctx.Request.Header.Peek("X-Real-IP"); len(xri) > 0 {
		return string(xri)
	}

	// 回退到 RemoteAddr
	if addr := ctx.RemoteAddr(); addr != nil {
		if tcpAddr, ok := addr.(*net.TCPAddr); ok {
			return tcpAddr.IP.String()
		}
		return addr.String()
	}

	return ""
}

// ExtractClientIPNet 从请求上下文中提取客户端 IP 地址（返回 net.IP）。
//
// 该函数与 ExtractClientIP 功能相同，但返回 net.IP 类型，
// 便于后续进行 IP 网络操作（如 CIDR 匹配）。
//
// 参数：
//   - ctx: FastHTTP 请求上下文
//
// 返回值：
//   - net.IP: 客户端 IP 地址，无法解析时返回 nil
func ExtractClientIPNet(ctx *fasthttp.RequestCtx) net.IP {
	// 首先检查 X-Forwarded-For 请求头
	if xff := ctx.Request.Header.Peek("X-Forwarded-For"); len(xff) > 0 {
		ips := strings.Split(string(xff), ",")
		if len(ips) > 0 {
			ipStr := strings.TrimSpace(ips[0])
			if ip := net.ParseIP(ipStr); ip != nil {
				return ip
			}
		}
	}

	// 检查 X-Real-IP 请求头
	if xri := ctx.Request.Header.Peek("X-Real-IP"); len(xri) > 0 {
		if ip := net.ParseIP(string(xri)); ip != nil {
			return ip
		}
	}

	// 回退到 RemoteAddr
	if addr := ctx.RemoteAddr(); addr != nil {
		if tcpAddr, ok := addr.(*net.TCPAddr); ok {
			return tcpAddr.IP
		}
	}

	return nil
}

// GetRemoteAddrIP 从 RemoteAddr 提取 IP 地址。
//
// 这是一个辅助函数，直接从连接的远程地址获取 IP，
// 不检查任何代理头。
//
// 参数：
//   - ctx: FastHTTP 请求上下文
//
// 返回值：
//   - net.IP: 客户端 IP 地址，无法获取时返回 nil
func GetRemoteAddrIP(ctx *fasthttp.RequestCtx) net.IP {
	if addr := ctx.RemoteAddr(); addr != nil {
		if tcpAddr, ok := addr.(*net.TCPAddr); ok {
			return tcpAddr.IP
		}
	}
	return nil
}

// ExtractClientIPWithTrustedProxies 从请求上下文安全提取客户端 IP。
// 仅当 remoteAddr 属于 trustedProxies 时，才解析 X-Forwarded-For 头部，
// 并取右侧（最接近服务器）第一个非可信 IP 作为真实客户端 IP。
func ExtractClientIPWithTrustedProxies(ctx *fasthttp.RequestCtx, trustedProxies []net.IPNet) net.IP {
	remoteIP := GetRemoteAddrIP(ctx)
	if remoteIP == nil || len(trustedProxies) == 0 {
		return remoteIP
	}

	trusted := false
	for i := range trustedProxies {
		if trustedProxies[i].Contains(remoteIP) {
			trusted = true
			break
		}
	}
	if !trusted {
		return remoteIP
	}

	if xff := ctx.Request.Header.Peek("X-Forwarded-For"); len(xff) > 0 {
		ips := strings.Split(string(xff), ",")
		for i := len(ips) - 1; i >= 0; i-- {
			ipStr := strings.TrimSpace(ips[i])
			if ip := net.ParseIP(ipStr); ip != nil {
				isTrusted := false
				for j := range trustedProxies {
					if trustedProxies[j].Contains(ip) {
						isTrusted = true
						break
					}
				}
				if !isTrusted {
					return ip
				}
			}
		}
	}

	return remoteIP
}

// remoteAddrCache 缓存 RemoteAddr 字符串化结果，避免重复的 net.TCPAddr.String() 分配。
type remoteAddrCache struct {
	mu      sync.RWMutex
	entries map[string]string
	maxSize int
}

var globalRemoteAddrCache = &remoteAddrCache{
	entries: make(map[string]string, 1024),
	maxSize: 1024,
}

// FormatRemoteAddr 使用缓存格式化 RemoteAddr，避免重复的地址字符串分配。
// 优先使用 ctx.RemoteIP() 获取 IP，对 IPv4 直接零分配格式化，IPv6 回退到 addr.String()。
func FormatRemoteAddr(ctx *fasthttp.RequestCtx) string {
	ip := ctx.RemoteIP()
	if ip == nil {
		addr := ctx.RemoteAddr()
		if addr == nil {
			return "-"
		}
		return addr.String()
	}

	// 优先尝试 IPv4 快速路径（零分配）
	if ipv4 := ip.To4(); ipv4 != nil {
		return formatIPv4(ipv4)
	}

	// IPv6：尝试缓存
	ipStr := ip.String()
	globalRemoteAddrCache.mu.RLock()
	if cached, ok := globalRemoteAddrCache.entries[ipStr]; ok {
		globalRemoteAddrCache.mu.RUnlock()
		return cached
	}
	globalRemoteAddrCache.mu.RUnlock()

	// 未命中缓存，回退到 addr.String()
	addr := ctx.RemoteAddr()
	if addr == nil {
		return "-"
	}
	result := addr.String()

	globalRemoteAddrCache.mu.Lock()
	if len(globalRemoteAddrCache.entries) < globalRemoteAddrCache.maxSize {
		globalRemoteAddrCache.entries[ipStr] = result
	}
	globalRemoteAddrCache.mu.Unlock()
	return result
}

// formatIPv4 将 4 字节 IPv4 地址格式化为字符串（零分配）。
func formatIPv4(ip net.IP) string {
	var buf [15]byte
	n := 0
	for i := 0; i < 4; i++ {
		if i > 0 {
			buf[n] = '.'
			n++
		}
		n += writeUint8(buf[n:], ip[i])
	}
	return string(buf[:n])
}

// writeUint8 将 uint8 写入 buf，返回写入的字节数。
func writeUint8(buf []byte, v byte) int {
	if v >= 100 {
		buf[0] = byte('0' + v/100)
		buf[1] = byte('0' + (v/10)%10)
		buf[2] = byte('0' + v%10)
		return 3
	}
	if v >= 10 {
		buf[0] = byte('0' + v/10)
		buf[1] = byte('0' + v%10)
		return 2
	}
	buf[0] = byte('0' + v)
	return 1
}
