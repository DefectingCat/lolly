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
