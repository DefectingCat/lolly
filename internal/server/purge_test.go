// Package server 提供缓存清理处理器功能的测试。
//
// 该文件测试 PurgeHandler 模块的各项功能，包括：
//   - 路径配置（默认和自定义）
//   - localhost 特殊处理和 CIDR 解析
//   - IP 白名单访问控制
//   - Token 认证
//
// 作者：xfy
package server

import (
	"net"
	"testing"

	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/config"
)

func TestPurgeHandler_Path(t *testing.T) {
	tests := []struct {
		name     string
		cfgPath  string
		wantPath string
	}{
		{
			name:     "default path",
			cfgPath:  "",
			wantPath: "/_cache/purge",
		},
		{
			name:     "custom path",
			cfgPath:  "/api/purge",
			wantPath: "/api/purge",
		},
		{
			name:     "custom path with version prefix",
			cfgPath:  "/api/v1/cache/purge",
			wantPath: "/api/v1/cache/purge",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.CacheAPIConfig{
				Path:  tt.cfgPath,
				Allow: []string{},
			}

			h, err := NewPurgeHandler(nil, cfg)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if h.Path() != tt.wantPath {
				t.Errorf("expected path %s, got %s", tt.wantPath, h.Path())
			}
		})
	}
}

func TestPurgeHandler_NewPurgeHandler(t *testing.T) {
	t.Run("localhost special handling", func(t *testing.T) {
		cfg := &config.CacheAPIConfig{
			Path:  "/_cache/purge",
			Allow: []string{"localhost"},
		}

		h, err := NewPurgeHandler(nil, cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(h.allowed) != 2 {
			t.Fatalf("expected 2 allowed networks for localhost, got %d", len(h.allowed))
		}

		// 验证包含 127.0.0.1/32
		_, v4Net, _ := net.ParseCIDR("127.0.0.1/32")
		if v4Net == nil {
			t.Fatal("failed to parse 127.0.0.1/32")
		}
		foundV4 := false
		for _, n := range h.allowed {
			if n.String() == v4Net.String() {
				foundV4 = true
				break
			}
		}
		if !foundV4 {
			t.Error("expected 127.0.0.1/32 in allowed networks")
		}

		// 验证包含 ::1/128
		_, v6Net, _ := net.ParseCIDR("::1/128")
		if v6Net == nil {
			t.Fatal("failed to parse ::1/128")
		}
		foundV6 := false
		for _, n := range h.allowed {
			if n.String() == v6Net.String() {
				foundV6 = true
				break
			}
		}
		if !foundV6 {
			t.Error("expected ::1/128 in allowed networks")
		}
	})

	t.Run("CIDR parsing", func(t *testing.T) {
		cfg := &config.CacheAPIConfig{
			Path:  "/_cache/purge",
			Allow: []string{"10.0.0.0/8", "172.16.0.0/12"},
		}

		h, err := NewPurgeHandler(nil, cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(h.allowed) != 2 {
			t.Errorf("expected 2 allowed networks, got %d", len(h.allowed))
		}
	})

	t.Run("single IP parsed as CIDR", func(t *testing.T) {
		cfg := &config.CacheAPIConfig{
			Path:  "/_cache/purge",
			Allow: []string{"192.168.1.100"},
		}

		h, err := NewPurgeHandler(nil, cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(h.allowed) != 1 {
			t.Fatalf("expected 1 allowed network, got %d", len(h.allowed))
		}

		// 单 IP 应转换为 /32 CIDR
		if h.allowed[0].String() != "192.168.1.100/32" {
			t.Errorf("expected 192.168.1.100/32, got %s", h.allowed[0].String())
		}
	})

	t.Run("invalid IP returns error", func(t *testing.T) {
		cfg := &config.CacheAPIConfig{
			Path:  "/_cache/purge",
			Allow: []string{"not-an-ip"},
		}

		_, err := NewPurgeHandler(nil, cfg)
		if err == nil {
			t.Error("expected error for invalid IP, got nil")
		}
	})
}

func TestPurgeHandler_checkAccess(t *testing.T) {
	tests := []struct {
		name       string
		allow      []string
		clientIP   string
		wantAccess bool
	}{
		{
			name:       "no allow list - open access",
			allow:      []string{},
			clientIP:   "1.2.3.4",
			wantAccess: true,
		},
		{
			name:       "CIDR match",
			allow:      []string{"192.168.0.0/16"},
			clientIP:   "192.168.1.100",
			wantAccess: true,
		},
		{
			name:       "CIDR no match",
			allow:      []string{"10.0.0.0/8"},
			clientIP:   "192.168.1.100",
			wantAccess: false,
		},
		{
			name:       "single IP match",
			allow:      []string{"127.0.0.1"},
			clientIP:   "127.0.0.1",
			wantAccess: true,
		},
		{
			name:       "single IP no match",
			allow:      []string{"127.0.0.1"},
			clientIP:   "127.0.0.2",
			wantAccess: false,
		},
		{
			name:       "localhost allows 127.0.0.1",
			allow:      []string{"localhost"},
			clientIP:   "127.0.0.1",
			wantAccess: true,
		},
		{
			name:       "localhost allows ::1",
			allow:      []string{"localhost"},
			clientIP:   "::1",
			wantAccess: true,
		},
		{
			name:       "localhost denies other IP",
			allow:      []string{"localhost"},
			clientIP:   "10.0.0.1",
			wantAccess: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.CacheAPIConfig{
				Path:  "/_cache/purge",
				Allow: tt.allow,
			}

			h, err := NewPurgeHandler(nil, cfg)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(h.allowed) == 0 {
				// 无白名单时应允许所有访问
				if !h.checkAccess(nil) {
					t.Error("expected access to be true when no allow list configured")
				}
				return
			}

			// 直接测试 IP 是否在 allowed 列表中
			ip := net.ParseIP(tt.clientIP)
			if ip == nil {
				t.Fatalf("failed to parse client IP: %s", tt.clientIP)
			}

			found := false
			for _, network := range h.allowed {
				if network.Contains(ip) {
					found = true
					break
				}
			}

			if found != tt.wantAccess {
				t.Errorf("expected access %v, got %v", tt.wantAccess, found)
			}
		})
	}
}

func TestPurgeHandler_checkAuth(t *testing.T) {
	t.Run("no auth configured", func(t *testing.T) {
		cfg := &config.CacheAPIConfig{
			Path: "/_cache/purge",
			Auth: config.CacheAPIAuthConfig{
				Type:  "",
				Token: "",
			},
		}

		h, err := NewPurgeHandler(nil, cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		ctx := &fasthttp.RequestCtx{}
		if !h.checkAuth(ctx) {
			t.Error("expected auth to pass when no auth configured")
		}
	})

	t.Run("auth type none", func(t *testing.T) {
		cfg := &config.CacheAPIConfig{
			Path: "/_cache/purge",
			Auth: config.CacheAPIAuthConfig{
				Type:  "none",
				Token: "",
			},
		}

		h, err := NewPurgeHandler(nil, cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		ctx := &fasthttp.RequestCtx{}
		if !h.checkAuth(ctx) {
			t.Error("expected auth to pass when type is none")
		}
	})

	t.Run("token auth - correct Bearer token", func(t *testing.T) {
		cfg := &config.CacheAPIConfig{
			Path: "/_cache/purge",
			Auth: config.CacheAPIAuthConfig{
				Type:  "token",
				Token: "secret-token",
			},
		}

		h, err := NewPurgeHandler(nil, cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		ctx := &fasthttp.RequestCtx{}
		ctx.Request.Header.Set("Authorization", "Bearer secret-token")

		if !h.checkAuth(ctx) {
			t.Error("expected auth to pass with correct Bearer token")
		}
	})

	t.Run("token auth - correct direct token", func(t *testing.T) {
		cfg := &config.CacheAPIConfig{
			Path: "/_cache/purge",
			Auth: config.CacheAPIAuthConfig{
				Type:  "token",
				Token: "secret-token",
			},
		}

		h, err := NewPurgeHandler(nil, cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		ctx := &fasthttp.RequestCtx{}
		ctx.Request.Header.Set("Authorization", "secret-token")

		if !h.checkAuth(ctx) {
			t.Error("expected auth to pass with correct direct token")
		}
	})

	t.Run("token auth - wrong token", func(t *testing.T) {
		cfg := &config.CacheAPIConfig{
			Path: "/_cache/purge",
			Auth: config.CacheAPIAuthConfig{
				Type:  "token",
				Token: "secret-token",
			},
		}

		h, err := NewPurgeHandler(nil, cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		ctx := &fasthttp.RequestCtx{}
		ctx.Request.Header.Set("Authorization", "Bearer wrong-token")

		if h.checkAuth(ctx) {
			t.Error("expected auth to fail with wrong token")
		}
	})

	t.Run("token auth - missing header", func(t *testing.T) {
		cfg := &config.CacheAPIConfig{
			Path: "/_cache/purge",
			Auth: config.CacheAPIAuthConfig{
				Type:  "token",
				Token: "secret-token",
			},
		}

		h, err := NewPurgeHandler(nil, cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		ctx := &fasthttp.RequestCtx{}

		if h.checkAuth(ctx) {
			t.Error("expected auth to fail when Authorization header is missing")
		}
	})

	t.Run("token auth - unknown type", func(t *testing.T) {
		cfg := &config.CacheAPIConfig{
			Path: "/_cache/purge",
			Auth: config.CacheAPIAuthConfig{
				Type:  "basic",
				Token: "secret-token",
			},
		}

		h, err := NewPurgeHandler(nil, cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		ctx := &fasthttp.RequestCtx{}
		ctx.Request.Header.Set("Authorization", "Bearer secret-token")

		if h.checkAuth(ctx) {
			t.Error("expected auth to fail for unknown auth type")
		}
	})
}
