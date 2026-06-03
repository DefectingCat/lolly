// Package proxy 提供反向代理的核心功能，支持请求转发、负载均衡、健康检查等特性。
//
// 包含代理工具函数，用于处理请求和响应的转换。
//
// 作者：xfy
package proxy

import (
	"bytes"

	"rua.plus/lolly/internal/utils"
)

// b2s converts byte slice to string without allocation.
// WARNING: The returned string shares memory with the original slice.
// Do not modify the slice after calling this function.
func b2s(b []byte) string {
	return utils.B2s(b)
}

// s2b converts string to byte slice without allocation.
// WARNING: The returned slice shares memory with the original string.
// Do not modify the slice contents.
func s2b(s string) []byte {
	return utils.S2b(s)
}

// isInWhitelist checks if a header key is in the whitelist.
// Uses bytes.EqualFold for case-insensitive comparison without allocation.
func isInWhitelist(key []byte, whitelist map[string]bool) bool {
	for wKey := range whitelist {
		if bytes.EqualFold(key, s2b(wKey)) {
			return true
		}
	}
	return false
}
