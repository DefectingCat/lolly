// Package proxy 反向代理包，为 Lolly HTTP 服务器提供反向代理功能。
//
// 该文件提供统一的 X-Forwarded 系列请求头设置逻辑，包括：
//   - X-Forwarded-For: 客户端 IP 地址链
//   - X-Real-IP: 客户端真实 IP 地址
//   - X-Forwarded-Host: 原始请求 Host
//   - X-Forwarded-Proto: 原始请求协议（http/https）
//
// 主要用途：
//
//	用于在代理转发时保留客户端原始请求信息，使后端服务能够获取
//	客户端的真实 IP、Host 和协议。
//
// 注意事项：
//   - 所有函数均为非并发安全（无状态函数）
//   - X-Forwarded-For 支持追加模式和覆盖模式
//
// 作者：xfy
package proxy

import (
	"strings"

	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/netutil"
)

// 协议常量，用于标识请求使用的传输层协议。
const (
	protoHTTP  = "http"  // 明文 HTTP 协议
	protoHTTPS = "https" // TLS 加密的 HTTPS 协议
)

// ForwardedHeaders 包含 X-Forwarded 系列头信息。
//
// 用于在代理转发时保留客户端原始请求信息。
type ForwardedHeaders struct {
	ClientIP string // 客户端 IP 地址，从连接信息或 X-Real-IP 头提取
	Host     string // 原始请求 Host 头，表示客户端访问的主机名
	Proto    string // 原始请求协议，http 或 https
}

// ExtractForwardedHeaders 从请求上下文中提取 X-Forwarded 头信息。
//
// 参数：
//   - ctx: FastHTTP 请求上下文
//
// 返回值：
//   - ForwardedHeaders: 提取的头信息
func ExtractForwardedHeaders(ctx *fasthttp.RequestCtx) ForwardedHeaders {
	clientIP := netutil.ExtractClientIP(ctx)
	host := string(ctx.Host())

	proto := protoHTTP
	if ctx.IsTLS() {
		proto = protoHTTPS
	}

	return ForwardedHeaders{
		ClientIP: clientIP,
		Host:     host,
		Proto:    proto,
	}
}

// SetForwardedHeaders 设置 X-Forwarded 系列请求头。
//
// 参数：
//   - headers: 目标请求头
//   - fh: ForwardedHeaders 结构体
//   - appendXFF: 是否追加到已有的 X-Forwarded-For 头
func SetForwardedHeaders(headers *fasthttp.RequestHeader, fh ForwardedHeaders, appendXFF bool) {
	// 设置 X-Real-IP
	if fh.ClientIP != "" {
		headers.Set("X-Real-IP", fh.ClientIP)
	}

	// 设置 X-Forwarded-For
	if fh.ClientIP != "" {
		if appendXFF {
			existingXFF := headers.Peek("X-Forwarded-For")
			if len(existingXFF) > 0 {
				// SAFETY: Ephemeral — xffBuf is written to header immediately and not reused.
				var xffBuf []byte
				xffBuf = append(xffBuf, existingXFF...)
				xffBuf = append(xffBuf, ", "...)
				xffBuf = append(xffBuf, fh.ClientIP...)
				headers.SetBytesKV([]byte("X-Forwarded-For"), xffBuf)
			} else {
				headers.SetBytesKV([]byte("X-Forwarded-For"), []byte(fh.ClientIP))
			}
		} else {
			headers.SetBytesKV([]byte("X-Forwarded-For"), []byte(fh.ClientIP))
		}
	}

	// 设置 X-Forwarded-Host
	if fh.Host != "" {
		headers.Set("X-Forwarded-Host", fh.Host)
	}

	// 设置 X-Forwarded-Proto
	if fh.Proto != "" {
		headers.Set("X-Forwarded-Proto", fh.Proto)
	}
}

// WriteForwardedHeaders 将 X-Forwarded 头写入到 strings.Builder。
// 用于 WebSocket 升级请求构建。
//
// 参数：
//   - builder: strings.Builder 实例
//   - fh: ForwardedHeaders 结构体
func WriteForwardedHeaders(builder *strings.Builder, fh ForwardedHeaders) {
	if fh.ClientIP != "" {
		builder.WriteString("X-Forwarded-For: ")
		builder.WriteString(fh.ClientIP)
		builder.WriteString("\r\n")
		builder.WriteString("X-Real-IP: ")
		builder.WriteString(fh.ClientIP)
		builder.WriteString("\r\n")
	}

	if fh.Host != "" {
		builder.WriteString("X-Forwarded-Host: ")
		builder.WriteString(fh.Host)
		builder.WriteString("\r\n")
	}

	if fh.Proto != "" {
		builder.WriteString("X-Forwarded-Proto: ")
		builder.WriteString(fh.Proto)
		builder.WriteString("\r\n")
	}
}
