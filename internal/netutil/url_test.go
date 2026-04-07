// Package netutil 提供 URL 工具功能的测试。
//
// 该文件测试网络工具模块的 URL 相关功能，包括：
//   - 目标 URL 解析
//   - 默认端口添加
//   - TLS 检测
//   - 主机提取
//
// 作者：xfy
package netutil

import "testing"

func TestParseTargetURL(t *testing.T) {
	tests := []struct {
		name           string
		targetURL      string
		addDefaultPort bool
		wantAddr       string
		wantIsTLS      bool
	}{
		// HTTP without port
		{
			name:           "http without port, add default",
			targetURL:      "http://backend.example.com",
			addDefaultPort: true,
			wantAddr:       "backend.example.com:80",
			wantIsTLS:      false,
		},
		{
			name:           "http without port, no default",
			targetURL:      "http://backend.example.com",
			addDefaultPort: false,
			wantAddr:       "backend.example.com",
			wantIsTLS:      false,
		},
		// HTTPS without port
		{
			name:           "https without port, add default",
			targetURL:      "https://api.example.com",
			addDefaultPort: true,
			wantAddr:       "api.example.com:443",
			wantIsTLS:      true,
		},
		{
			name:           "https without port, no default",
			targetURL:      "https://api.example.com",
			addDefaultPort: false,
			wantAddr:       "api.example.com",
			wantIsTLS:      true,
		},
		// HTTP with port
		{
			name:           "http with port",
			targetURL:      "http://backend:8080",
			addDefaultPort: true,
			wantAddr:       "backend:8080",
			wantIsTLS:      false,
		},
		// HTTPS with port
		{
			name:           "https with port",
			targetURL:      "https://api:8443",
			addDefaultPort: true,
			wantAddr:       "api:8443",
			wantIsTLS:      true,
		},
		// With path
		{
			name:           "http with path",
			targetURL:      "http://backend:8080/api/v1",
			addDefaultPort: false,
			wantAddr:       "backend:8080",
			wantIsTLS:      false,
		},
		{
			name:           "https with path",
			targetURL:      "https://api.example.com/v1/users",
			addDefaultPort: true,
			wantAddr:       "api.example.com:443",
			wantIsTLS:      true,
		},
		// No protocol (treat as HTTP)
		{
			name:           "no protocol",
			targetURL:      "backend:8080",
			addDefaultPort: false,
			wantAddr:       "backend:8080",
			wantIsTLS:      false,
		},
		{
			name:           "no protocol, no port, add default",
			targetURL:      "backend",
			addDefaultPort: true,
			wantAddr:       "backend:80",
			wantIsTLS:      false,
		},
		// IPv6 address
		{
			name:           "ipv6 address",
			targetURL:      "http://[::1]:8080",
			addDefaultPort: false,
			wantAddr:       "[::1]:8080",
			wantIsTLS:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotAddr, gotIsTLS := ParseTargetURL(tt.targetURL, tt.addDefaultPort)
			if gotAddr != tt.wantAddr {
				t.Errorf("ParseTargetURL() addr = %q, want %q", gotAddr, tt.wantAddr)
			}
			if gotIsTLS != tt.wantIsTLS {
				t.Errorf("ParseTargetURL() isTLS = %v, want %v", gotIsTLS, tt.wantIsTLS)
			}
		})
	}
}

func TestExtractHost(t *testing.T) {
	tests := []struct {
		name      string
		targetURL string
		want      string
	}{
		{
			name:      "http without port",
			targetURL: "http://backend.example.com",
			want:      "backend.example.com:80",
		},
		{
			name:      "https without port",
			targetURL: "https://api.example.com",
			want:      "api.example.com:443",
		},
		{
			name:      "http with port",
			targetURL: "http://backend:8080",
			want:      "backend:8080",
		},
		{
			name:      "https with path",
			targetURL: "https://api.example.com/v1/users",
			want:      "api.example.com:443",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ExtractHost(tt.targetURL); got != tt.want {
				t.Errorf("ExtractHost() = %q, want %q", got, tt.want)
			}
		})
	}
}
