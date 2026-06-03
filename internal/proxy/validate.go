// Package proxy 提供反向代理的核心功能，支持请求转发、负载均衡、健康检查等特性。
//
// 包含代理验证相关的结构体，用于配置请求验证规则。
//
// 作者：xfy
package proxy

import "strings"

// containsCRLF 检查字符串是否包含回车或换行字符。
//
// 用于防止 CRLF 注入攻击。
func containsCRLF(s string) bool {
	return strings.ContainsAny(s, "\r\n")
}
