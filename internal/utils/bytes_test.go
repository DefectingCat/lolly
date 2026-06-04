// Package utils 提供字节操作工具函数的测试。
//
// 该文件测试 B2s 和 S2b 函数，包括：
//   - 空值处理
//   - 正常值转换
//   - 内存共享验证
//
// 作者：xfy
package utils

import (
	"testing"
	"unsafe"

	"github.com/stretchr/testify/assert"
)

func TestB2s(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
		want  string
	}{
		{"nil_slice", nil, ""},
		{"empty_slice", []byte{}, ""},
		{"single_byte", []byte("a"), "a"},
		{"ascii_string", []byte("hello world"), "hello world"},
		{"utf8_string", []byte("你好世界"), "你好世界"},
		{"special_chars", []byte("!@#$%^&*()"), "!@#$%^&*()"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := B2s(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestB2s_ZeroAlloc(t *testing.T) {
	original := []byte("test")
	s := B2s(original)
	ptr := unsafe.StringData(s)
	slicePtr := unsafe.SliceData(original)
	assert.Equal(t, slicePtr, ptr, "B2s result should share memory with original slice")
}

func TestS2b(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []byte
	}{
		{"empty_string", "", nil},
		{"single_char", "a", []byte("a")},
		{"ascii_string", "hello world", []byte("hello world")},
		{"utf8_string", "你好世界", []byte("你好世界")},
		{"special_chars", "!@#$%^&*()", []byte("!@#$%^&*()")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := S2b(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestS2b_ZeroAlloc(t *testing.T) {
	original := "test"
	b := S2b(original)
	strPtr := unsafe.StringData(original)
	slicePtr := unsafe.SliceData(b)
	assert.Equal(t, strPtr, slicePtr, "S2b result should share memory with original string")
}

func TestB2s_S2b_RoundTrip(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{"empty", ""},
		{"ascii", "hello"},
		{"utf8", "你好"},
		{"binary", "\x00\x01\xff"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := S2b(tt.value)
			s := B2s(b)
			assert.Equal(t, tt.value, s)
		})
	}
}
