// Package utils 提供通用的工具函数和辅助类型。
//
// 包含字节操作相关的工具函数，用于处理字节切片和缓冲区。
//
// 作者：xfy
package utils

import "unsafe"

// B2s converts byte slice to string without allocation.
// WARNING: The returned string shares memory with the original slice.
// Do not modify the slice after calling this function.
func B2s(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	return unsafe.String(&b[0], len(b))
}

// S2b converts string to byte slice without allocation.
// WARNING: The returned slice shares memory with the original string.
// Do not modify the slice contents.
func S2b(s string) []byte {
	if s == "" {
		return nil
	}
	return unsafe.Slice(unsafe.StringData(s), len(s))
}
