// Package security 提供安全头部功能的测试。
//
// 该文件测试安全头部模块的各项功能，包括：
//   - HSTS 值格式化
//   - nil 配置保护
//
// 作者：xfy
package security

import (
	"testing"

	"github.com/valyala/fasthttp"
)

// TestAddHeadersNilConfig 验证配置在运行时被置为 nil 也不会 panic。
func TestAddHeadersNilConfig(t *testing.T) {
	sh := NewHeadersWithHSTS(nil, nil)
	// 故意将内部配置置为 nil，模拟异常状态
	sh.config = nil

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod("GET")
	ctx.Request.Header.SetRequestURI("/")

	// 不应 panic
	sh.addHeaders(ctx)

	if string(ctx.Response.Header.Peek("X-Frame-Options")) != "DENY" {
		t.Errorf("expected default X-Frame-Options=DENY, got %q", ctx.Response.Header.Peek("X-Frame-Options"))
	}
	if string(ctx.Response.Header.Peek("X-Content-Type-Options")) != "nosniff" {
		t.Errorf("expected default X-Content-Type-Options=nosniff, got %q", ctx.Response.Header.Peek("X-Content-Type-Options"))
	}
}

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
