package netutil

import "testing"

func TestStripPort(t *testing.T) {
	tests := []struct {
		name     string
		host     string
		expected string
	}{
		// IPv4 格式
		{"IPv4 with port", "example.com:8080", "example.com"},
		{"IPv4 with port 443", "example.com:443", "example.com"},
		{"IPv4 no port", "example.com", "example.com"},
		{"IPv4 with port and path", "example.com:8080", "example.com"},

		// IPv6 格式
		{"IPv6 localhost with port", "[::1]:443", "[::1]"},
		{"IPv6 full with port", "[2001:db8::1]:8443", "[2001:db8::1]"},
		{"IPv6 no port", "[::1]", "[::1]"},
		{"IPv6 full no port", "[2001:db8::1]", "[2001:db8::1]"},

		// 边界情况
		{"empty string", "", ""},
		{"just port", ":8080", ""},
		{"IPv6 with empty brackets", "[]", "[]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := StripPort(tt.host)
			if result != tt.expected {
				t.Errorf("StripPort(%q) = %q, want %q", tt.host, result, tt.expected)
			}
		})
	}
}

func TestHasPort(t *testing.T) {
	tests := []struct {
		name     string
		host     string
		expected bool
	}{
		{"IPv4 with port", "example.com:8080", true},
		{"IPv4 no port", "example.com", false},
		{"IPv6 with port", "[::1]:443", true},
		{"IPv6 no port", "[::1]", false},
		{"empty string", "", false},
		{"just port", ":8080", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HasPort(tt.host)
			if result != tt.expected {
				t.Errorf("HasPort(%q) = %v, want %v", tt.host, result, tt.expected)
			}
		})
	}
}
