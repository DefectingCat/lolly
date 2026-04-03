package security

import (
	"net"
	"testing"

	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/config"
)

func TestNewAccessControl(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *config.AccessConfig
		wantErr bool
	}{
		{
			name:    "nil config",
			cfg:     nil,
			wantErr: true,
		},
		{
			name: "empty config",
			cfg:  &config.AccessConfig{},
		},
		{
			name: "valid allow list",
			cfg: &config.AccessConfig{
				Allow: []string{"192.168.1.0/24", "10.0.0.1"},
			},
		},
		{
			name: "valid deny list",
			cfg: &config.AccessConfig{
				Deny: []string{"192.168.2.100/32"},
			},
		},
		{
			name: "invalid CIDR",
			cfg: &config.AccessConfig{
				Allow: []string{"invalid"},
			},
			wantErr: true,
		},
		{
			name: "default allow",
			cfg: &config.AccessConfig{
				Default: "allow",
			},
		},
		{
			name: "default deny",
			cfg: &config.AccessConfig{
				Default: "deny",
			},
		},
		{
			name: "invalid default",
			cfg: &config.AccessConfig{
				Default: "invalid",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ac, err := NewAccessControl(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewAccessControl() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && ac == nil {
				t.Error("Expected non-nil AccessControl")
			}
		})
	}
}

func TestAccessControlCheck(t *testing.T) {
	tests := []struct {
		name     string
		cfg      *config.AccessConfig
		ip       string
		expected bool
	}{
		{
			name: "default allow",
			cfg: &config.AccessConfig{
				Default: "allow",
			},
			ip:       "192.168.1.100",
			expected: true,
		},
		{
			name: "default deny",
			cfg: &config.AccessConfig{
				Default: "deny",
			},
			ip:       "192.168.1.100",
			expected: false,
		},
		{
			name: "explicit allow",
			cfg: &config.AccessConfig{
				Allow:   []string{"192.168.1.0/24"},
				Default: "deny",
			},
			ip:       "192.168.1.100",
			expected: true,
		},
		{
			name: "not in allow list",
			cfg: &config.AccessConfig{
				Allow:   []string{"192.168.1.0/24"},
				Default: "deny",
			},
			ip:       "192.168.2.100",
			expected: false,
		},
		{
			name: "explicit deny",
			cfg: &config.AccessConfig{
				Deny:    []string{"192.168.2.100"},
				Default: "allow",
			},
			ip:       "192.168.2.100",
			expected: false,
		},
		{
			name: "deny takes precedence",
			cfg: &config.AccessConfig{
				Allow:   []string{"192.168.0.0/16"},
				Deny:    []string{"192.168.2.100"},
				Default: "deny",
			},
			ip:       "192.168.2.100",
			expected: false,
		},
		{
			name: "single IP allow",
			cfg: &config.AccessConfig{
				Allow:   []string{"10.0.0.1"},
				Default: "deny",
			},
			ip:       "10.0.0.1",
			expected: true,
		},
		{
			name: "IPv6 allow",
			cfg: &config.AccessConfig{
				Allow:   []string{"2001:db8::/32"},
				Default: "deny",
			},
			ip:       "2001:db8::1",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ac, err := NewAccessControl(tt.cfg)
			if err != nil {
				t.Fatalf("NewAccessControl() error: %v", err)
			}

			ip := net.ParseIP(tt.ip)
			if ip == nil {
				t.Fatalf("Invalid IP: %s", tt.ip)
			}

			result := ac.Check(ip)
			if result != tt.expected {
				t.Errorf("Check(%s) = %v, expected %v", tt.ip, result, tt.expected)
			}
		})
	}
}

func TestAccessControlProcess(t *testing.T) {
	ac, err := NewAccessControl(&config.AccessConfig{
		Allow:   []string{"127.0.0.1"},
		Default: "deny",
	})
	if err != nil {
		t.Fatalf("NewAccessControl() error: %v", err)
	}

	// Create a simple handler
	nextHandler := func(ctx *fasthttp.RequestCtx) {
		_, _ = ctx.WriteString("OK")
	}

	handler := ac.Process(nextHandler)

	// Verify the handler is created correctly
	if handler == nil {
		t.Error("Process() returned nil handler")
	}
}

func TestParseCIDR(t *testing.T) {
	tests := []struct {
		name    string
		cidr    string
		wantErr bool
	}{
		{
			name: "valid IPv4 CIDR",
			cidr: "192.168.1.0/24",
		},
		{
			name: "valid IPv4 single",
			cidr: "192.168.1.1",
		},
		{
			name: "valid IPv6 CIDR",
			cidr: "2001:db8::/32",
		},
		{
			name: "valid IPv6 single",
			cidr: "2001:db8::1",
		},
		{
			name:    "invalid IP",
			cidr:    "invalid",
			wantErr: true,
		},
		{
			name:    "invalid CIDR",
			cidr:    "192.168.1.0/33",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			network, err := parseCIDR(tt.cidr)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseCIDR() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && network == nil {
				t.Error("Expected non-nil network")
			}
		})
	}
}

func TestUpdateAllowList(t *testing.T) {
	ac, err := NewAccessControl(&config.AccessConfig{
		Default: "deny",
	})
	if err != nil {
		t.Fatalf("NewAccessControl() error: %v", err)
	}

	// Update allow list
	err = ac.UpdateAllowList([]string{"10.0.0.0/8"})
	if err != nil {
		t.Errorf("UpdateAllowList() error: %v", err)
	}

	// Check that IP is now allowed
	ip := net.ParseIP("10.0.0.1")
	if !ac.Check(ip) {
		t.Error("Expected IP to be allowed after update")
	}

	// Test invalid update
	err = ac.UpdateAllowList([]string{"invalid"})
	if err == nil {
		t.Error("Expected error for invalid CIDR")
	}
}

func TestUpdateDenyList(t *testing.T) {
	ac, err := NewAccessControl(&config.AccessConfig{
		Allow:   []string{"0.0.0.0/0"},
		Default: "allow",
	})
	if err != nil {
		t.Fatalf("NewAccessControl() error: %v", err)
	}

	// Update deny list
	err = ac.UpdateDenyList([]string{"192.168.2.0/24"})
	if err != nil {
		t.Errorf("UpdateDenyList() error: %v", err)
	}

	// Check that IP is now denied
	ip := net.ParseIP("192.168.2.1")
	if ac.Check(ip) {
		t.Error("Expected IP to be denied after update")
	}
}

func TestSetDefault(t *testing.T) {
	ac, err := NewAccessControl(&config.AccessConfig{
		Default: "allow",
	})
	if err != nil {
		t.Fatalf("NewAccessControl() error: %v", err)
	}

	// Change to deny
	err = ac.SetDefault("deny")
	if err != nil {
		t.Errorf("SetDefault() error: %v", err)
	}

	stats := ac.GetStats()
	if stats.Default != "deny" {
		t.Errorf("Expected default 'deny', got %s", stats.Default)
	}

	// Test invalid action
	err = ac.SetDefault("invalid")
	if err == nil {
		t.Error("Expected error for invalid action")
	}
}

func TestGetStats(t *testing.T) {
	ac, err := NewAccessControl(&config.AccessConfig{
		Allow:   []string{"192.168.1.0/24", "10.0.0.0/8"},
		Deny:    []string{"192.168.2.100"},
		Default: "deny",
	})
	if err != nil {
		t.Fatalf("NewAccessControl() error: %v", err)
	}

	stats := ac.GetStats()
	if stats.AllowCount != 2 {
		t.Errorf("Expected AllowCount 2, got %d", stats.AllowCount)
	}
	if stats.DenyCount != 1 {
		t.Errorf("Expected DenyCount 1, got %d", stats.DenyCount)
	}
	if stats.Default != "deny" {
		t.Errorf("Expected Default 'deny', got %s", stats.Default)
	}
}
