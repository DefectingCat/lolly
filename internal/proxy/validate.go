package proxy

import "strings"

// containsCRLF 检查字符串是否包含回车或换行字符。
//
// 用于防止 CRLF 注入攻击。
func containsCRLF(s string) bool {
	return strings.ContainsAny(s, "\r\n")
}
