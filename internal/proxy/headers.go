// Package proxy 反向代理包，为 Lolly HTTP 服务器提供反向代理功能。
//
// 该文件提供统一的 X-Forwarded 头设置逻辑。
package proxy

import (
	"strings"

	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/netutil"
)

// ForwardedHeaders 包含 X-Forwarded 系列头信息。
type ForwardedHeaders struct {
	ClientIP string // 客户端 IP
	Host     string // 原始 Host
	Proto    string // 协议 (http/https)
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

	proto := "http"
	if ctx.IsTLS() {
		proto = "https"
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
				headers.Set("X-Forwarded-For", string(existingXFF)+", "+fh.ClientIP)
			} else {
				headers.Set("X-Forwarded-For", fh.ClientIP)
			}
		} else {
			headers.Set("X-Forwarded-For", fh.ClientIP)
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
