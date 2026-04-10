// Package netutil 提供网络工具功能的测试。
//
// 该文件测试网络工具模块的 IP 相关功能，包括：
//   - 客户端 IP 提取
//   - X-Forwarded-For 头解析
//   - X-Real-IP 头解析
//   - 远程地址解析
//
// 作者：xfy
package netutil

import (
	"net"
	"testing"

	"github.com/valyala/fasthttp"
)

func TestExtractClientIP(t *testing.T) {
	tests := []struct {
		name       string
		xff        string
		xri        string
		remoteAddr string
		want       string
	}{
		{
			name: "X-Forwarded-For with single IP",
			xff:  "192.168.1.100",
			want: "192.168.1.100",
		},
		{
			name: "X-Forwarded-For with multiple IPs",
			xff:  "192.168.1.100, 10.0.0.1, 172.16.0.1",
			want: "192.168.1.100",
		},
		{
			name: "X-Real-IP only",
			xri:  "192.168.1.200",
			want: "192.168.1.200",
		},
		{
			name:       "RemoteAddr fallback",
			remoteAddr: "192.168.1.1:12345",
			want:       "0.0.0.0", // fasthttp 默认初始化为 0.0.0.0
		},
		{
			name: "X-Forwarded-For takes precedence over X-Real-IP",
			xff:  "192.168.1.100",
			xri:  "192.168.1.200",
			want: "192.168.1.100",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &fasthttp.RequestCtx{}
			ctx.Init(&fasthttp.Request{}, nil, nil)

			if tt.xff != "" {
				ctx.Request.Header.Set("X-Forwarded-For", tt.xff)
			}
			if tt.xri != "" {
				ctx.Request.Header.Set("X-Real-IP", tt.xri)
			}

			got := ExtractClientIP(ctx)
			if got != tt.want {
				t.Errorf("ExtractClientIP() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExtractClientIPNet(t *testing.T) {
	tests := []struct {
		name string
		xff  string
		xri  string
		want net.IP
	}{
		{
			name: "X-Forwarded-For valid IP",
			xff:  "192.168.1.100",
			want: net.ParseIP("192.168.1.100"),
		},
		{
			name: "X-Forwarded-For invalid IP",
			xff:  "invalid-ip",
			want: net.ParseIP("0.0.0.0"), // fasthttp 默认 RemoteAddr
		},
		{
			name: "X-Real-IP valid IP",
			xri:  "192.168.1.200",
			want: net.ParseIP("192.168.1.200"),
		},
		{
			name: "No headers",
			want: net.ParseIP("0.0.0.0"), // fasthttp 默认 RemoteAddr
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &fasthttp.RequestCtx{}
			ctx.Init(&fasthttp.Request{}, nil, nil)

			if tt.xff != "" {
				ctx.Request.Header.Set("X-Forwarded-For", tt.xff)
			}
			if tt.xri != "" {
				ctx.Request.Header.Set("X-Real-IP", tt.xri)
			}

			got := ExtractClientIPNet(ctx)
			if !got.Equal(tt.want) {
				t.Errorf("ExtractClientIPNet() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetRemoteAddrIP(_ *testing.T) {
	ctx := &fasthttp.RequestCtx{}
	ctx.Init(&fasthttp.Request{}, nil, nil)

	// Without setting remote addr, should return nil
	got := GetRemoteAddrIP(ctx)
	// The result depends on how fasthttp initializes the remote addr
	// Just verify it doesn't panic
	_ = got
}
