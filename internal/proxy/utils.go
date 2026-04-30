package proxy

import (
	"bytes"
	"unsafe"
)

// b2s converts byte slice to string without allocation.
// WARNING: The returned string shares memory with the original slice.
// Do not modify the slice after calling this function.
func b2s(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	return unsafe.String(&b[0], len(b))
}

// s2b converts string to byte slice without allocation.
// WARNING: The returned slice shares memory with the original string.
// Do not modify the slice contents.
func s2b(s string) []byte {
	if s == "" {
		return nil
	}
	return unsafe.Slice(unsafe.StringData(s), len(s))
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
