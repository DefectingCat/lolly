package lua

import (
	"net"
	"testing"
)

func TestIsRestrictedIP(t *testing.T) {
	tests := []struct {
		name    string
		ip      net.IP
		blocked bool
	}{
		// IPv4 回环
		{"IPv4 loopback", net.ParseIP("127.0.0.1"), true},
		{"IPv4 loopback alt", net.ParseIP("127.0.1.1"), true},
		// IPv4 私有
		{"IPv4 private 10.x", net.ParseIP("10.0.0.1"), true},
		{"IPv4 private 172.16.x", net.ParseIP("172.16.0.1"), true},
		{"IPv4 private 192.168.x", net.ParseIP("192.168.0.1"), true},
		// IPv4 链路本地
		{"IPv4 link-local", net.ParseIP("169.254.1.1"), true},
		// IPv4 未指定
		{"IPv4 unspecified", net.ParseIP("0.0.0.0"), true},
		// IPv4 公网
		{"IPv4 public", net.ParseIP("8.8.8.8"), false},
		{"IPv4 public 2", net.ParseIP("1.1.1.1"), false},
		// IPv6 回环
		{"IPv6 loopback", net.ParseIP("::1"), true},
		// IPv6 链路本地
		{"IPv6 link-local", net.ParseIP("fe80::1"), true},
		// IPv6 链路本地多播（应被拦截）
		{"IPv6 link-local multicast", net.ParseIP("ff02::1"), true},
		// IPv6 公网
		{"IPv6 public", net.ParseIP("2001:4860:4860::8888"), false},
		// IPv4-mapped IPv6
		{"IPv4-mapped IPv6 loopback", net.ParseIP("::ffff:127.0.0.1"), true},
		{"IPv4-mapped IPv6 private", net.ParseIP("::ffff:10.0.0.1"), true},
		// nil IP
		{"nil IP", net.IP{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isRestrictedIP(tt.ip)
			if result != tt.blocked {
				t.Errorf("isRestrictedIP(%v) = %v, want %v", tt.ip, result, tt.blocked)
			}
		})
	}
}
