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
