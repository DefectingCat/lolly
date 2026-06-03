// Package security 提供安全头部功能的测试。
//
// 该文件测试安全头部模块的各项功能，包括：
//   - HSTS 值格式化
//
// 作者：xfy
package security

import "testing"

func TestFormatHSTSValue(t *testing.T) {
	tests := []struct {
		name              string
		expected          string
		maxAge            int
		includeSubDomains bool
		preload           bool
	}{
		{
			name:              "basic HSTS",
			maxAge:            31536000,
			includeSubDomains: true,
			preload:           false,
			expected:          "max-age=31536000; includeSubDomains",
		},
		{
			name:              "HSTS with preload",
			maxAge:            31536000,
			includeSubDomains: true,
			preload:           true,
			expected:          "max-age=31536000; includeSubDomains; preload",
		},
		{
			name:              "HSTS without subdomains",
			maxAge:            86400,
			includeSubDomains: false,
			preload:           false,
			expected:          "max-age=86400",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatHSTSValue(tt.maxAge, tt.includeSubDomains, tt.preload)
			if result != tt.expected {
				t.Errorf("formatHSTSValue() = %s, expected %s", result, tt.expected)
			}
		})
	}
}
