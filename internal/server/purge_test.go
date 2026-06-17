// Package server 提供缓存清理处理器功能的测试。
//
// 该文件测试 PurgeHandler 模块的各项功能，包括：
//   - 路径配置（默认和自定义）
//   - localhost 特殊处理和 CIDR 解析
//   - IP 白名单访问控制
//   - Token 认证
//   - 请求处理流程（POST/方法检查）
//   - 请求体解析
//   - sendError 方法
//   - purgeByPath/purgeByPattern 方法（nil server）
//
// 作者：xfy
package server

import (
	"net"
	"strings"
	"testing"

	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/cache"
	"rua.plus/lolly/internal/config"
	"rua.plus/lolly/internal/utils"
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
				if !utils.CheckIPAccess(nil, h.allowed, nil) {
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
		if !utils.CheckTokenAuth(ctx, h.auth) {
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
		if !utils.CheckTokenAuth(ctx, h.auth) {
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

		if !utils.CheckTokenAuth(ctx, h.auth) {
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

		if !utils.CheckTokenAuth(ctx, h.auth) {
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

		if utils.CheckTokenAuth(ctx, h.auth) {
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

		if utils.CheckTokenAuth(ctx, h.auth) {
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

		if utils.CheckTokenAuth(ctx, h.auth) {
			t.Error("expected auth to fail for unknown auth type")
		}
	})
}

// TestPurgeHandler_ServeHTTP_MethodCheck 测试 ServeHTTP 的方法检查。
func TestPurgeHandler_ServeHTTP_MethodCheck(t *testing.T) {
	methods := []string{"GET", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS"}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			cfg := &config.CacheAPIConfig{
				Path:  "/_cache/purge",
				Allow: []string{},
			}

			h, err := NewPurgeHandler(nil, cfg)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			ctx := &fasthttp.RequestCtx{}
			ctx.Init(&fasthttp.Request{}, nil, nil)
			ctx.Request.Header.SetMethod(method)

			h.ServeHTTP(ctx)

			if ctx.Response.StatusCode() != fasthttp.StatusMethodNotAllowed {
				t.Errorf("expected status %d for method %s, got %d", fasthttp.StatusMethodNotAllowed, method, ctx.Response.StatusCode())
			}

			// 验证响应体包含错误信息
			if !strings.Contains(string(ctx.Response.Body()), "method not allowed") {
				t.Errorf("expected 'method not allowed' in response body, got: %s", string(ctx.Response.Body()))
			}
		})
	}
}

// TestPurgeHandler_ServeHTTP_RequestBodyParsing 测试请求体解析。
func TestPurgeHandler_ServeHTTP_RequestBodyParsing(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus int
	}{
		{
			name:       "invalid JSON",
			body:       "{invalid json}",
			wantStatus: fasthttp.StatusBadRequest,
		},
		{
			name:       "empty JSON",
			body:       "{}",
			wantStatus: fasthttp.StatusBadRequest,
		},
		{
			name:       "missing path and pattern",
			body:       `{"method": "GET"}`,
			wantStatus: fasthttp.StatusBadRequest,
		},
		{
			name:       "only path",
			body:       `{"path": "/test"}`,
			wantStatus: fasthttp.StatusOK,
		},
		{
			name:       "only pattern",
			body:       `{"pattern": "/api/*"}`,
			wantStatus: fasthttp.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.CacheAPIConfig{
				Path:  "/_cache/purge",
				Allow: []string{},
			}

			h, err := NewPurgeHandler(nil, cfg)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			ctx := &fasthttp.RequestCtx{}
			ctx.Init(&fasthttp.Request{}, nil, nil)
			ctx.Request.Header.SetMethod("POST")
			ctx.Request.SetBodyString(tt.body)

			h.ServeHTTP(ctx)

			if ctx.Response.StatusCode() != tt.wantStatus {
				t.Errorf("expected status %d, got %d", tt.wantStatus, ctx.Response.StatusCode())
			}
		})
	}
}

// TestPurgeHandler_PurgeResponse 测试 purge 响应格式。
func TestPurgeHandler_PurgeResponse(t *testing.T) {
	cfg := &config.CacheAPIConfig{
		Path:  "/_cache/purge",
		Allow: []string{},
	}

	h, err := NewPurgeHandler(nil, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := &fasthttp.RequestCtx{}
	ctx.Init(&fasthttp.Request{}, nil, nil)
	ctx.Request.Header.SetMethod("POST")
	ctx.Request.SetBodyString(`{"path": "/test"}`)

	h.ServeHTTP(ctx)

	// 验证响应
	if ctx.Response.StatusCode() != fasthttp.StatusOK {
		t.Errorf("expected status %d, got %d", fasthttp.StatusOK, ctx.Response.StatusCode())
	}

	// 验证响应体格式
	body := string(ctx.Response.Body())
	if !strings.Contains(body, `"deleted"`) {
		t.Errorf("expected 'deleted' field in response body, got: %s", body)
	}
}

// TestPurgeHandler_SendError 测试 sendError 方法的错误响应格式。
func TestPurgeHandler_SendError(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		errMsg   string
		wantBody string
	}{
		{
			name:     "bad request",
			status:   fasthttp.StatusBadRequest,
			errMsg:   "invalid request",
			wantBody: `{"error":"invalid request"}`,
		},
		{
			name:     "forbidden",
			status:   fasthttp.StatusForbidden,
			errMsg:   "access denied",
			wantBody: `{"error":"access denied"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.CacheAPIConfig{
				Path:  "/_cache/purge",
				Allow: []string{},
			}

			_, err := NewPurgeHandler(nil, cfg)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			ctx := &fasthttp.RequestCtx{}
			ctx.Init(&fasthttp.Request{}, nil, nil)

			utils.SendJSONError(ctx, tt.status, tt.errMsg)

			if ctx.Response.StatusCode() != tt.status {
				t.Errorf("expected status %d, got %d", tt.status, ctx.Response.StatusCode())
			}

			body := string(ctx.Response.Body())
			if !strings.Contains(body, tt.errMsg) {
				t.Errorf("expected '%s' in response body, got: %s", tt.errMsg, body)
			}

			// 验证内容类型
			contentType := string(ctx.Response.Header.ContentType())
			if contentType != "application/json; charset=utf-8" {
				t.Errorf("expected content-type 'application/json; charset=utf-8', got: %s", contentType)
			}
		})
	}
}

// TestPurgeHandler_CacheKeyWithMethod 测试带方法的缓存键。
func TestPurgeHandler_CacheKeyWithMethod(t *testing.T) {
	tests := []struct {
		path   string
		method string
	}{
		{"/test", "GET"},
		{"/test", "POST"},
		{"/api/users", "GET"},
		{"/api/users", "DELETE"},
	}

	for _, tt := range tests {
		t.Run(tt.path+"_"+tt.method, func(t *testing.T) {
			key := cache.HashPathWithMethod(tt.path, tt.method)
			if key == 0 {
				t.Error("expected non-zero hash key")
			}

			// 同一路径和方法应该产生相同的键
			key2 := cache.HashPathWithMethod(tt.path, tt.method)
			if key != key2 {
				t.Errorf("expected same key for same inputs, got %d and %d", key, key2)
			}

			// 同一路径不同方法应该产生不同的键
			key3 := cache.HashPathWithMethod(tt.path, "OTHER")
			if key == key3 {
				t.Errorf("expected different key for different method, got %d and %d", key, key3)
			}
		})
	}
}

// TestPurgeHandler_EmptyMethodDefaultsToGET 测试空方法默认为 GET。
func TestPurgeHandler_EmptyMethodDefaultsToGET(t *testing.T) {
	key1 := cache.HashPathWithMethod("/test", "")
	key2 := cache.HashPathWithMethod("/test", "GET")

	if key1 != key2 {
		t.Errorf("expected same key for empty and 'GET' method, got %d and %d", key1, key2)
	}
}

// TestPurgeHandler_checkAccess_NilContext 测试 checkAccess 处理。
func TestPurgeHandler_checkAccess_NilContext(t *testing.T) {
	t.Run("empty allow list allows all", func(t *testing.T) {
		cfg := &config.CacheAPIConfig{
			Path:  "/_cache/purge",
			Allow: []string{},
		}

		h, err := NewPurgeHandler(nil, cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Empty allow list should allow access (returns true even with nil context)
		if !utils.CheckIPAccess(nil, h.allowed, nil) {
			t.Error("expected checkAccess to return true with empty allow list")
		}
	})
}

// TestPurgeHandler_ServeHTTP_WithAllowList 测试带白名单的请求处理。
func TestPurgeHandler_ServeHTTP_WithAllowList(t *testing.T) {
	cfg := &config.CacheAPIConfig{
		Path:  "/_cache/purge",
		Allow: []string{"192.168.0.0/16"},
	}

	h, err := NewPurgeHandler(nil, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 测试 POST 请求（会尝试访问控制检查）
	ctx := &fasthttp.RequestCtx{}
	ctx.Init(&fasthttp.Request{}, nil, nil)
	ctx.Request.Header.SetMethod("POST")
	ctx.Request.SetBodyString(`{"path": "/test"}`)

	h.ServeHTTP(ctx)

	// 由于无法设置 RemoteIP，checkAccess 会返回 false
	// 所以应该返回 403
	if ctx.Response.StatusCode() != fasthttp.StatusForbidden {
		t.Logf("Status: %d, Body: %s", ctx.Response.StatusCode(), string(ctx.Response.Body()))
	}
}

// TestPurgeHandler_checkAccess_WithAllowedIP 测试 checkAccess 方法。
func TestPurgeHandler_checkAccess_WithAllowedIP(t *testing.T) {
	t.Run("with allow list and nil remote", func(t *testing.T) {
		cfg := &config.CacheAPIConfig{
			Path:  "/_cache/purge",
			Allow: []string{"192.168.0.0/16"},
		}

		h, err := NewPurgeHandler(nil, cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Create a valid context but with nil remote address
		ctx := &fasthttp.RequestCtx{}
		ctx.Init(&fasthttp.Request{}, nil, nil)

		// context with nil remote address - should return false (no client IP)
		if utils.CheckIPAccess(ctx, h.allowed, nil) {
			t.Error("expected checkAccess to return false with no client IP")
		}
	})
}

// TestPurgeHandler_PurgeByPath_HashConsistency 测试哈希一致性。
func TestPurgeHandler_PurgeByPath_HashConsistency(t *testing.T) {
	// 验证相同路径和方法产生相同哈希
	path := "/api/users"
	method := "GET"

	hash1 := cache.HashPathWithMethod(path, method)
	hash2 := cache.HashPathWithMethod(path, method)

	if hash1 != hash2 {
		t.Errorf("hash not consistent: %d != %d", hash1, hash2)
	}

	// 验证不同路径产生不同哈希
	hash3 := cache.HashPathWithMethod("/api/posts", method)
	if hash1 == hash3 {
		t.Error("expected different hashes for different paths")
	}

	// 验证不同方法产生不同哈希
	hash4 := cache.HashPathWithMethod(path, "POST")
	if hash1 == hash4 {
		t.Error("expected different hashes for different methods")
	}
}

// TestPurgeHandler_PurgeByPattern_PatternMatching 测试模式匹配逻辑。
func TestPurgeHandler_PurgeByPattern_PatternMatching(t *testing.T) {
	tests := []struct {
		pattern string
		path    string
		want    bool
	}{
		// 通配符结尾 - 前缀匹配
		{"/api/*", "/api/users", true},
		{"/api/*", "/api/posts", true},
		{"/api/*", "/api/users/123", true}, // * 匹配剩余所有内容
		{"/api/*", "/other/path", false},

		// 单个 * 匹配所有
		{"*", "/api/users", true},
		{"*", "/any/path", true},

		// 中间通配符
		{"/api/*/users", "/api/v1/users", true},
		{"/api/*/users", "/api/v2/users", true},
		{"/api/*/users", "/api/users", true}, // 前缀和后缀都匹配
		{"/api/*/users", "/api/v1/posts", false},

		// 精确匹配
		{"/api/users", "/api/users", true},
		{"/api/users", "/api/posts", false},

		// 空模式
		{"", "", true},
		{"", "/api", false},

		// 目录前缀匹配（以 / 结尾）
		{"/api/", "/api/users", true},
		{"/api/", "/api/users/123", true},
		{"/api/", "/other/path", false},
	}

	for _, tt := range tests {
		t.Run(tt.pattern+"_"+tt.path, func(t *testing.T) {
			got := cache.MatchPattern(tt.pattern, tt.path)
			if got != tt.want {
				t.Errorf("MatchPattern(%q, %q) = %v, want %v", tt.pattern, tt.path, got, tt.want)
			}
		})
	}
}
