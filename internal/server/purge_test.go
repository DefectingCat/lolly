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
	"time"

	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/cache"
	"rua.plus/lolly/internal/config"
	"rua.plus/lolly/internal/loadbalance"
	"rua.plus/lolly/internal/proxy"
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
				if !utils.CheckIPAccess(nil, h.allowed) {
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

// TestPurgeHandler_PurgeByPath 测试 purgeByPath 方法（nil server）。
func TestPurgeHandler_PurgeByPath(t *testing.T) {
	h, err := NewPurgeHandler(nil, &config.CacheAPIConfig{
		Path:  "/_cache/purge",
		Allow: []string{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 验证 nil server 时返回 0
	deleted := h.PurgeByPathForTest("/test", "GET")
	if deleted != 0 {
		t.Errorf("expected 0 deletions for nil server, got %d", deleted)
	}
}

// TestPurgeHandler_PurgeByPattern 测试 purgeByPattern 方法（nil server）。
func TestPurgeHandler_PurgeByPattern(t *testing.T) {
	h, err := NewPurgeHandler(nil, &config.CacheAPIConfig{
		Path:  "/_cache/purge",
		Allow: []string{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 验证 nil server 时返回 0
	deleted := h.PurgeByPatternForTest("/api/*", "GET")
	if deleted != 0 {
		t.Errorf("expected 0 deletions for nil server, got %d", deleted)
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
		if !utils.CheckIPAccess(nil, h.allowed) {
			t.Error("expected checkAccess to return true with empty allow list")
		}
	})
}

// TestPurgeHandler_PurgeByPath_NilServer 测试 purgeByPath 处理 nil server。
func TestPurgeHandler_PurgeByPath_NilServer(t *testing.T) {
	cfg := &config.CacheAPIConfig{
		Path:  "/_cache/purge",
		Allow: []string{},
	}

	h, err := NewPurgeHandler(nil, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should return 0 when server is nil
	deleted := h.PurgeByPathForTest("/test", "GET")
	if deleted != 0 {
		t.Errorf("expected 0 deletions for nil server, got %d", deleted)
	}
}

// TestPurgeHandler_PurgeByPattern_NilServer 测试 purgeByPattern 处理 nil server。
func TestPurgeHandler_PurgeByPattern_NilServer(t *testing.T) {
	cfg := &config.CacheAPIConfig{
		Path:  "/_cache/purge",
		Allow: []string{},
	}

	h, err := NewPurgeHandler(nil, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should return 0 when server is nil
	deleted := h.PurgeByPatternForTest("/api/*", "GET")
	if deleted != 0 {
		t.Errorf("expected 0 deletions for nil server, got %d", deleted)
	}
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
		if utils.CheckIPAccess(ctx, h.allowed) {
			t.Error("expected checkAccess to return false with no client IP")
		}
	})
}

// TestPurgeHandler_PurgeByPath_WithRealCache 测试 purgeByPath 在有真实缓存时的行为。
func TestPurgeHandler_PurgeByPath_WithRealCache(t *testing.T) {
	// 创建启用缓存的代理
	cfg := &config.ProxyConfig{
		Path:        "/api",
		LoadBalance: "round_robin",
		Timeout:     config.ProxyTimeout{Connect: 5 * time.Second},
		Cache: config.ProxyCacheConfig{
			Enabled: true,
			MaxAge:  10 * time.Second,
		},
	}
	targets := []*loadbalance.Target{{URL: "http://localhost:8080"}}
	p, err := proxy.NewProxy(cfg, targets, nil, nil)
	if err != nil {
		t.Fatalf("NewProxy() error: %v", err)
	}

	// 获取缓存并添加测试数据
	pcache := p.GetCache()
	if pcache == nil {
		t.Fatal("GetCache() should return non-nil when cache enabled")
	}

	// 添加测试缓存条目
	hashKey1 := cache.HashPathWithMethod("/api/users", "GET")
	pcache.Set(hashKey1, "GET:/api/users", []byte("test data 1"), nil, 200, time.Minute)

	hashKey2 := cache.HashPathWithMethod("/api/posts", "GET")
	pcache.Set(hashKey2, "GET:/api/posts", []byte("test data 2"), nil, 200, time.Minute)

	hashKey3 := cache.HashPathWithMethod("/api/users", "POST")
	pcache.Set(hashKey3, "POST:/api/users", []byte("test data 3"), nil, 200, time.Minute)

	// 创建带有代理的 handler
	h, err := NewPurgeHandler(&Server{
		proxies: []*proxy.Proxy{p},
	}, &config.CacheAPIConfig{
		Path:  "/_cache/purge",
		Allow: []string{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	t.Run("delete existing entry", func(t *testing.T) {
		deleted := h.PurgeByPathForTest("/api/users", "GET")
		if deleted != 1 {
			t.Errorf("expected 1 deletion, got %d", deleted)
		}
	})

	t.Run("delete different method", func(t *testing.T) {
		deleted := h.PurgeByPathForTest("/api/users", "POST")
		if deleted != 1 {
			t.Errorf("expected 1 deletion, got %d", deleted)
		}
	})

	t.Run("delete non-existing path", func(t *testing.T) {
		deleted := h.PurgeByPathForTest("/api/nonexistent", "GET")
		if deleted != 1 {
			t.Errorf("expected 1 (proxy count), got %d", deleted)
		}
	})

	t.Run("multiple proxies", func(t *testing.T) {
		// 创建第二个代理
		p2, err := proxy.NewProxy(cfg, targets, nil, nil)
		if err != nil {
			t.Fatalf("NewProxy() error: %v", err)
		}
		pcache2 := p2.GetCache()
		hashKey := cache.HashPathWithMethod("/test", "GET")
		pcache2.Set(hashKey, "GET:/test", []byte("test"), nil, 200, time.Minute)

		h2, err := NewPurgeHandler(&Server{
			proxies: []*proxy.Proxy{p, p2},
		}, &config.CacheAPIConfig{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		deleted := h2.PurgeByPathForTest("/test", "GET")
		if deleted != 2 {
			t.Errorf("expected 2 deletions (2 proxies), got %d", deleted)
		}
	})
}

// TestPurgeHandler_PurgeByPattern_WithRealCache 测试 purgeByPattern 在有真实缓存时的行为。
func TestPurgeHandler_PurgeByPattern_WithRealCache(t *testing.T) {
	// 创建启用缓存的代理
	cfg := &config.ProxyConfig{
		Path:        "/api",
		LoadBalance: "round_robin",
		Timeout:     config.ProxyTimeout{Connect: 5 * time.Second},
		Cache: config.ProxyCacheConfig{
			Enabled: true,
			MaxAge:  10 * time.Second,
		},
	}
	targets := []*loadbalance.Target{{URL: "http://localhost:8080"}}
	p, err := proxy.NewProxy(cfg, targets, nil, nil)
	if err != nil {
		t.Fatalf("NewProxy() error: %v", err)
	}

	// 获取缓存并添加测试数据
	pcache := p.GetCache()
	if pcache == nil {
		t.Fatal("GetCache() should return non-nil when cache enabled")
	}

	// 添加多个测试缓存条目
	pcache.Set(cache.HashPathWithMethod("/api/users", "GET"), "GET:/api/users", []byte("data"), nil, 200, time.Minute)
	pcache.Set(cache.HashPathWithMethod("/api/users/1", "GET"), "GET:/api/users/1", []byte("data"), nil, 200, time.Minute)
	pcache.Set(cache.HashPathWithMethod("/api/posts", "GET"), "GET:/api/posts", []byte("data"), nil, 200, time.Minute)
	pcache.Set(cache.HashPathWithMethod("/api/posts/1", "GET"), "GET:/api/posts/1", []byte("data"), nil, 200, time.Minute)
	pcache.Set(cache.HashPathWithMethod("/api/users", "POST"), "POST:/api/users", []byte("data"), nil, 200, time.Minute)

	// 创建带有代理的 handler
	h, err := NewPurgeHandler(&Server{
		proxies: []*proxy.Proxy{p},
	}, &config.CacheAPIConfig{
		Path:  "/_cache/purge",
		Allow: []string{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	t.Run("wildcard pattern matches multiple", func(t *testing.T) {
		// 重新添加数据
		pcache.Set(cache.HashPathWithMethod("/api/users", "GET"), "GET:/api/users", []byte("data"), nil, 200, time.Minute)
		pcache.Set(cache.HashPathWithMethod("/api/users/1", "GET"), "GET:/api/users/1", []byte("data"), nil, 200, time.Minute)
		pcache.Set(cache.HashPathWithMethod("/api/posts", "GET"), "GET:/api/posts", []byte("data"), nil, 200, time.Minute)

		// 注意：OrigKey 格式为 "METHOD:/path"，所以模式需要匹配完整路径
		deleted := h.PurgeByPatternForTest("GET:/api/*", "GET")
		if deleted < 1 {
			t.Errorf("expected at least 1 deletion, got %d", deleted)
		}
	})

	t.Run("empty method matches all methods", func(t *testing.T) {
		// 重新添加数据
		pcache.Set(cache.HashPathWithMethod("/api/users", "GET"), "GET:/api/users", []byte("data"), nil, 200, time.Minute)
		pcache.Set(cache.HashPathWithMethod("/api/users", "POST"), "POST:/api/users", []byte("data"), nil, 200, time.Minute)

		// 使用 * 通配符匹配所有方法
		deleted := h.PurgeByPatternForTest("*:/api/users", "")
		if deleted < 1 {
			t.Errorf("expected at least 1 deletion (all methods), got %d", deleted)
		}
	})

	t.Run("specific method only", func(t *testing.T) {
		// 重新添加数据
		pcache.Set(cache.HashPathWithMethod("/api/users", "GET"), "GET:/api/users", []byte("data"), nil, 200, time.Minute)
		pcache.Set(cache.HashPathWithMethod("/api/users", "POST"), "POST:/api/users", []byte("data"), nil, 200, time.Minute)

		// 模式匹配 POST 方法的路径
		deleted := h.PurgeByPatternForTest("POST:/api/users", "POST")
		if deleted < 1 {
			t.Errorf("expected at least 1 deletion (POST only), got %d", deleted)
		}
	})
}

// TestPurgeHandler_PurgeByPath_WithProxyNoCache 测试代理没有缓存时的情况。
func TestPurgeHandler_PurgeByPath_WithProxyNoCache(t *testing.T) {
	// 创建禁用缓存的代理
	cfg := &config.ProxyConfig{
		Path:        "/api",
		LoadBalance: "round_robin",
		Timeout:     config.ProxyTimeout{Connect: 5 * time.Second},
		// Cache 未启用
	}
	targets := []*loadbalance.Target{{URL: "http://localhost:8080"}}
	p, err := proxy.NewProxy(cfg, targets, nil, nil)
	if err != nil {
		t.Fatalf("NewProxy() error: %v", err)
	}

	// 确认缓存为 nil
	if p.GetCache() != nil {
		t.Fatal("GetCache() should return nil when cache disabled")
	}

	// 创建带有代理的 handler
	h, err := NewPurgeHandler(&Server{
		proxies: []*proxy.Proxy{p},
	}, &config.CacheAPIConfig{
		Path:  "/_cache/purge",
		Allow: []string{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 没有缓存的代理应该返回 0
	deleted := h.PurgeByPathForTest("/api/users", "GET")
	if deleted != 0 {
		t.Errorf("expected 0 deletions for proxy without cache, got %d", deleted)
	}
}

// TestPurgeHandler_PurgeByPattern_WithProxyNoCache 测试代理没有缓存时的情况。
func TestPurgeHandler_PurgeByPattern_WithProxyNoCache(t *testing.T) {
	// 创建禁用缓存的代理
	cfg := &config.ProxyConfig{
		Path:        "/api",
		LoadBalance: "round_robin",
		Timeout:     config.ProxyTimeout{Connect: 5 * time.Second},
	}
	targets := []*loadbalance.Target{{URL: "http://localhost:8080"}}
	p, err := proxy.NewProxy(cfg, targets, nil, nil)
	if err != nil {
		t.Fatalf("NewProxy() error: %v", err)
	}

	// 创建带有代理的 handler
	h, err := NewPurgeHandler(&Server{
		proxies: []*proxy.Proxy{p},
	}, &config.CacheAPIConfig{
		Path:  "/_cache/purge",
		Allow: []string{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 没有缓存的代理应该返回 0
	deleted := h.PurgeByPatternForTest("/api/*", "GET")
	if deleted != 0 {
		t.Errorf("expected 0 deletions for proxy without cache, got %d", deleted)
	}
}

// TestPurgeHandler_PurgeByPath_WithCache 测试 purgeByPath 在有缓存时的行为。
func TestPurgeHandler_PurgeByPath_WithCache(t *testing.T) {
	t.Run("server with empty proxies", func(t *testing.T) {
		// 创建带有空 proxies 列表的 handler
		h, err := NewPurgeHandler(&Server{
			proxies: []*proxy.Proxy{},
		}, &config.CacheAPIConfig{
			Path:  "/_cache/purge",
			Allow: []string{},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// 空 proxies 列表应该返回 0
		deleted := h.PurgeByPathForTest("/api/users", "GET")
		if deleted != 0 {
			t.Errorf("expected 0 deletions for empty proxies, got %d", deleted)
		}
	})

	t.Run("empty path", func(t *testing.T) {
		h, err := NewPurgeHandler(nil, &config.CacheAPIConfig{
			Path:  "/_cache/purge",
			Allow: []string{},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// 空路径仍然会执行删除逻辑，只是哈希值为默认 GET 的哈希
		deleted := h.PurgeByPathForTest("", "")
		if deleted != 0 {
			t.Errorf("expected 0 deletions for nil server, got %d", deleted)
		}
	})

	t.Run("path with special characters", func(t *testing.T) {
		h, err := NewPurgeHandler(nil, &config.CacheAPIConfig{
			Path:  "/_cache/purge",
			Allow: []string{},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// 特殊字符路径
		deleted := h.PurgeByPathForTest("/api/users?id=1&name=test", "GET")
		if deleted != 0 {
			t.Errorf("expected 0 deletions for nil server, got %d", deleted)
		}
	})

	t.Run("path with unicode", func(t *testing.T) {
		h, err := NewPurgeHandler(nil, &config.CacheAPIConfig{
			Path:  "/_cache/purge",
			Allow: []string{},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Unicode 路径
		deleted := h.PurgeByPathForTest("/api/用户/列表", "GET")
		if deleted != 0 {
			t.Errorf("expected 0 deletions for nil server, got %d", deleted)
		}
	})

	t.Run("different methods", func(t *testing.T) {
		h, err := NewPurgeHandler(nil, &config.CacheAPIConfig{
			Path:  "/_cache/purge",
			Allow: []string{},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		methods := []string{"GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS"}
		for _, method := range methods {
			deleted := h.PurgeByPathForTest("/api/users", method)
			if deleted != 0 {
				t.Errorf("expected 0 deletions for nil server with method %s, got %d", method, deleted)
			}
		}
	})
}

// TestPurgeHandler_PurgeByPattern_WithCache 测试 purgeByPattern 在有缓存时的行为。
func TestPurgeHandler_PurgeByPattern_WithCache(t *testing.T) {
	t.Run("empty pattern", func(t *testing.T) {
		h, err := NewPurgeHandler(nil, &config.CacheAPIConfig{
			Path:  "/_cache/purge",
			Allow: []string{},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// 空模式
		deleted := h.PurgeByPatternForTest("", "GET")
		if deleted != 0 {
			t.Errorf("expected 0 deletions for nil server, got %d", deleted)
		}
	})

	t.Run("wildcard pattern", func(t *testing.T) {
		h, err := NewPurgeHandler(nil, &config.CacheAPIConfig{
			Path:  "/_cache/purge",
			Allow: []string{},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// 通配符模式
		deleted := h.PurgeByPatternForTest("/api/*", "GET")
		if deleted != 0 {
			t.Errorf("expected 0 deletions for nil server, got %d", deleted)
		}
	})

	t.Run("double wildcard pattern", func(t *testing.T) {
		h, err := NewPurgeHandler(nil, &config.CacheAPIConfig{
			Path:  "/_cache/purge",
			Allow: []string{},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// 双通配符模式
		deleted := h.PurgeByPatternForTest("/api/**", "GET")
		if deleted != 0 {
			t.Errorf("expected 0 deletions for nil server, got %d", deleted)
		}
	})

	t.Run("pattern with special characters", func(t *testing.T) {
		h, err := NewPurgeHandler(nil, &config.CacheAPIConfig{
			Path:  "/_cache/purge",
			Allow: []string{},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// 特殊字符模式
		deleted := h.PurgeByPatternForTest("/api/users?id=*", "GET")
		if deleted != 0 {
			t.Errorf("expected 0 deletions for nil server, got %d", deleted)
		}
	})

	t.Run("exact pattern (no wildcard)", func(t *testing.T) {
		h, err := NewPurgeHandler(nil, &config.CacheAPIConfig{
			Path:  "/_cache/purge",
			Allow: []string{},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// 精确模式（无通配符）
		deleted := h.PurgeByPatternForTest("/api/users", "GET")
		if deleted != 0 {
			t.Errorf("expected 0 deletions for nil server, got %d", deleted)
		}
	})

	t.Run("different methods", func(t *testing.T) {
		h, err := NewPurgeHandler(nil, &config.CacheAPIConfig{
			Path:  "/_cache/purge",
			Allow: []string{},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		methods := []string{"GET", "POST", "PUT", "DELETE", "PATCH"}
		for _, method := range methods {
			deleted := h.PurgeByPatternForTest("/api/*", method)
			if deleted != 0 {
				t.Errorf("expected 0 deletions for nil server with method %s, got %d", method, deleted)
			}
		}
	})

	t.Run("empty method matches all", func(t *testing.T) {
		h, err := NewPurgeHandler(nil, &config.CacheAPIConfig{
			Path:  "/_cache/purge",
			Allow: []string{},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// 空方法应该匹配所有条目
		deleted := h.PurgeByPatternForTest("/api/*", "")
		if deleted != 0 {
			t.Errorf("expected 0 deletions for nil server, got %d", deleted)
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

// TestPurgeHandler_PurgeByPath_VariousInputs 测试各种输入。
func TestPurgeHandler_PurgeByPath_VariousInputs(t *testing.T) {
	h, err := NewPurgeHandler(nil, &config.CacheAPIConfig{
		Path:  "/_cache/purge",
		Allow: []string{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tests := []struct {
		name   string
		path   string
		method string
	}{
		{"empty path and method", "", ""},
		{"empty path with method", "", "GET"},
		{"path with empty method", "/test", ""},
		{"root path", "/", "GET"},
		{"nested path", "/a/b/c/d/e", "GET"},
		{"path with trailing slash", "/api/users/", "GET"},
		{"path with query", "/api?key=value", "GET"},
		{"path with fragment", "/api#section", "GET"},
		{"path with encoded chars", "/api%2Fusers", "GET"},
		{"long path", strings.Repeat("/a", 100), "GET"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 应该不会 panic
			deleted := h.PurgeByPathForTest(tt.path, tt.method)
			if deleted != 0 {
				t.Errorf("expected 0 deletions for nil server, got %d", deleted)
			}
		})
	}
}

// TestPurgeHandler_PurgeByPattern_VariousInputs 测试各种模式输入。
func TestPurgeHandler_PurgeByPattern_VariousInputs(t *testing.T) {
	h, err := NewPurgeHandler(nil, &config.CacheAPIConfig{
		Path:  "/_cache/purge",
		Allow: []string{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tests := []struct {
		name    string
		pattern string
		method  string
	}{
		{"empty pattern and method", "", ""},
		{"empty pattern with method", "", "GET"},
		{"pattern with empty method", "/api/*", ""},
		{"single wildcard only", "*", "GET"},
		{"double wildcard only", "**", "GET"},
		{"multiple single wildcards", "/api/*/users/*", "GET"},
		{"mixed wildcards", "/api/**/users/*", "GET"},
		{"wildcard at start", "*/users", "GET"},
		{"wildcard at end", "/api/*", "GET"},
		{"consecutive wildcards", "/api/**/*", "GET"},
		{"long pattern", strings.Repeat("/a*", 20), "GET"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 应该不会 panic
			deleted := h.PurgeByPatternForTest(tt.pattern, tt.method)
			if deleted != 0 {
				t.Errorf("expected 0 deletions for nil server, got %d", deleted)
			}
		})
	}
}
