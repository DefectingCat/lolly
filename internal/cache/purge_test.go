// Package cache 提供缓存清理 API 的测试。
//
// 该文件测试 purge.go 中的各项功能，包括：
//   - PurgeAPI 创建和配置
//   - Path() 默认和自定义路径
//   - ServeHTTP 完整请求处理
//   - IP 白名单访问控制
//   - Token 认证
//   - 按路径和模式清理缓存
//   - HashPathWithMethod 哈希计算
//   - MatchPattern 通配符匹配
//
// 作者：xfy
package cache

import (
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/config"
)

func TestHashPathWithMethod(t *testing.T) {
	t.Run("GET method default", func(t *testing.T) {
		h1 := HashPathWithMethod("/api/users", "")
		h2 := HashPathWithMethod("/api/users", "GET")
		if h1 != h2 {
			t.Errorf("Expected empty method to default to GET, got %d vs %d", h1, h2)
		}
	})

	t.Run("different methods different hashes", func(t *testing.T) {
		hGet := HashPathWithMethod("/api/users", "GET")
		hPost := HashPathWithMethod("/api/users", "POST")
		if hGet == hPost {
			t.Error("Expected different hashes for different methods")
		}
	})

	t.Run("different paths different hashes", func(t *testing.T) {
		h1 := HashPathWithMethod("/api/users", "GET")
		h2 := HashPathWithMethod("/api/posts", "GET")
		if h1 == h2 {
			t.Error("Expected different hashes for different paths")
		}
	})
}

func TestMatchPattern(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		path    string
		want    bool
	}{
		// Wildcard
		{"wildcard matches all", "*", "/anything/goes", true},
		{"wildcard matches empty", "*", "/", true},

		// Prefix with trailing *
		{"prefix star match", "/api/*", "/api/users", true},
		{"prefix star match nested", "/api/*", "/api/v1/users", true},
		{"prefix star no match", "/api/*", "/other/path", false},
		{"prefix star match base", "/api/*", "/api/", true},

		// Directory prefix (pattern ends with /)
		{"dir prefix match", "/api/", "/api/users", true},
		{"dir prefix no match", "/api/", "/other/path", false},

		// Exact match
		{"exact match", "/api/users", "/api/users", true},
		{"exact no match", "/api/users", "/api/users/extra", false},

		// Middle wildcard
		{"middle wildcard", "/api/*/users", "/api/v1/users", true},
		{"middle wildcard no match prefix", "/api/*/users", "/other/v1/users", false},
		{"middle wildcard no match suffix", "/api/*/users", "/api/v1/posts", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MatchPattern(tt.pattern, tt.path)
			if result != tt.want {
				t.Errorf("MatchPattern(%s, %s) = %v, want %v", tt.pattern, tt.path, result, tt.want)
			}
		})
	}
}

func TestNewPurgeAPI(t *testing.T) {
	t.Run("nil cache", func(t *testing.T) {
		cfg := &config.CacheAPIConfig{
			Path:  "/_cache/purge",
			Allow: []string{},
		}
		api, err := NewPurgeAPI(nil, cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if api == nil {
			t.Fatal("expected non-nil PurgeAPI")
		}
		if api.cache != nil {
			t.Error("expected nil cache")
		}
	})

	t.Run("with cache", func(t *testing.T) {
		pc := NewProxyCache(nil, false, 0, 0, 0)
		cfg := &config.CacheAPIConfig{
			Path:  "/custom/purge",
			Allow: []string{"127.0.0.1"},
			Auth: config.CacheAPIAuthConfig{
				Type:  "token",
				Token: "test-token",
			},
		}
		api, err := NewPurgeAPI(pc, cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if api.cache != pc {
			t.Error("expected cache to match")
		}
		if api.path != "/custom/purge" {
			t.Errorf("expected path /custom/purge, got %s", api.path)
		}
		if api.auth.Token != "test-token" {
			t.Errorf("expected token test-token, got %s", api.auth.Token)
		}
	})

	t.Run("CIDR parsing", func(t *testing.T) {
		cfg := &config.CacheAPIConfig{
			Allow: []string{"10.0.0.0/8", "172.16.0.0/12"},
		}
		api, err := NewPurgeAPI(nil, cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(api.allowed) != 2 {
			t.Errorf("expected 2 allowed networks, got %d", len(api.allowed))
		}
	})

	t.Run("single IP converted to CIDR", func(t *testing.T) {
		cfg := &config.CacheAPIConfig{
			Allow: []string{"192.168.1.100"},
		}
		api, err := NewPurgeAPI(nil, cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(api.allowed) != 1 {
			t.Fatalf("expected 1 allowed network, got %d", len(api.allowed))
		}
		if api.allowed[0].String() != "192.168.1.100/32" {
			t.Errorf("expected 192.168.1.100/32, got %s", api.allowed[0].String())
		}
	})

	t.Run("single IPv6 converted to CIDR", func(t *testing.T) {
		cfg := &config.CacheAPIConfig{
			Allow: []string{"::1"},
		}
		api, err := NewPurgeAPI(nil, cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(api.allowed) != 1 {
			t.Fatalf("expected 1 allowed network, got %d", len(api.allowed))
		}
		if api.allowed[0].String() != "::1/128" {
			t.Errorf("expected ::1/128, got %s", api.allowed[0].String())
		}
	})

	t.Run("invalid IP returns error", func(t *testing.T) {
		cfg := &config.CacheAPIConfig{
			Allow: []string{"not-an-ip"},
		}
		_, err := NewPurgeAPI(nil, cfg)
		if err == nil {
			t.Error("expected error for invalid IP, got nil")
		}
	})

	t.Run("mixed valid and CIDR", func(t *testing.T) {
		cfg := &config.CacheAPIConfig{
			Allow: []string{"10.0.0.0/8", "192.168.1.1"},
		}
		api, err := NewPurgeAPI(nil, cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(api.allowed) != 2 {
			t.Errorf("expected 2 allowed networks, got %d", len(api.allowed))
		}
	})
}

func TestPurgeAPI_Path(t *testing.T) {
	tests := []struct {
		name     string
		cfgPath  string
		wantPath string
	}{
		{"default path", "", "/_cache/purge"},
		{"custom path", "/api/purge", "/api/purge"},
		{"custom path with version", "/api/v1/cache/purge", "/api/v1/cache/purge"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.CacheAPIConfig{
				Path:  tt.cfgPath,
				Allow: []string{},
			}
			api, err := NewPurgeAPI(nil, cfg)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if api.Path() != tt.wantPath {
				t.Errorf("expected path %s, got %s", tt.wantPath, api.Path())
			}
		})
	}
}

func TestPurgeAPI_ServeHTTP_MethodNotAllowed(t *testing.T) {
	cfg := &config.CacheAPIConfig{
		Allow: []string{},
	}
	api, err := NewPurgeAPI(nil, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tests := []string{"GET", "PUT", "DELETE", "PATCH", "OPTIONS"}
	for _, method := range tests {
		t.Run(method, func(t *testing.T) {
			ctx := &fasthttp.RequestCtx{}
			ctx.Request.Header.SetMethod(method)
			api.ServeHTTP(ctx)

			if ctx.Response.StatusCode() != fasthttp.StatusMethodNotAllowed {
				t.Errorf("expected status 405, got %d", ctx.Response.StatusCode())
			}
		})
	}
}

func TestPurgeAPI_ServeHTTP_AccessForbidden(t *testing.T) {
	cfg := &config.CacheAPIConfig{
		Allow: []string{"10.0.0.0/8"},
	}
	api, err := NewPurgeAPI(nil, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod("POST")
	ctx.Request.SetBodyString(`{"path": "/test"}`)
	// Set RemoteAddr to 192.168.1.1 (not in 10.0.0.0/8)
	ctx.SetRemoteAddr(&net.TCPAddr{IP: net.ParseIP("192.168.1.1"), Port: 12345})

	api.ServeHTTP(ctx)

	if ctx.Response.StatusCode() != fasthttp.StatusForbidden {
		t.Errorf("expected status 403, got %d", ctx.Response.StatusCode())
	}
}

func TestPurgeAPI_ServeHTTP_Unauthorized(t *testing.T) {
	pc := NewProxyCache(nil, false, 0, 0, 0)
	cfg := &config.CacheAPIConfig{
		Allow: []string{"127.0.0.1"},
		Auth: config.CacheAPIAuthConfig{
			Type:  "token",
			Token: "secret",
		},
	}
	api, err := NewPurgeAPI(pc, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod("POST")
	ctx.Request.SetBodyString(`{"path": "/test"}`)
	ctx.SetRemoteAddr(&net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345})
	// No Authorization header

	api.ServeHTTP(ctx)

	if ctx.Response.StatusCode() != fasthttp.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", ctx.Response.StatusCode())
	}
}

func TestPurgeAPI_ServeHTTP_BadRequest(t *testing.T) {
	pc := NewProxyCache(nil, false, 0, 0, 0)
	cfg := &config.CacheAPIConfig{
		Allow: []string{},
	}
	api, err := NewPurgeAPI(pc, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	t.Run("invalid JSON", func(t *testing.T) {
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.Header.SetMethod("POST")
		ctx.Request.SetBodyString(`{invalid json}`)

		api.ServeHTTP(ctx)

		if ctx.Response.StatusCode() != fasthttp.StatusBadRequest {
			t.Errorf("expected status 400, got %d", ctx.Response.StatusCode())
		}
	})

	t.Run("missing path and pattern", func(t *testing.T) {
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.Header.SetMethod("POST")
		ctx.Request.SetBodyString(`{}`)

		api.ServeHTTP(ctx)

		if ctx.Response.StatusCode() != fasthttp.StatusBadRequest {
			t.Errorf("expected status 400, got %d", ctx.Response.StatusCode())
		}
	})
}

func TestPurgeAPI_ServeHTTP_PurgeByPath(t *testing.T) {
	pc := NewProxyCache(nil, false, 0, 0, 0)
	key := "GET:/api/test"
	pc.Set(hashKey(key), key, []byte("data"), nil, 200, 10*60*time.Second)

	cfg := &config.CacheAPIConfig{
		Allow: []string{},
	}
	api, err := NewPurgeAPI(pc, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod("POST")
	ctx.Request.SetBodyString(`{"path": "/api/test"}`)

	api.ServeHTTP(ctx)

	if ctx.Response.StatusCode() != fasthttp.StatusOK {
		t.Fatalf("expected status 200, got %d", ctx.Response.StatusCode())
	}

	var resp PurgeResponse
	if err := json.Unmarshal(ctx.Response.Body(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp.Deleted != 1 {
		t.Errorf("expected 1 deleted, got %d", resp.Deleted)
	}

	// Verify cache is gone
	_, ok, _ := pc.Get(hashKey(key), key)
	if ok {
		t.Error("expected cache entry to be purged")
	}
}

func TestPurgeAPI_ServeHTTP_PurgeByPath_NotFound(t *testing.T) {
	pc := NewProxyCache(nil, false, 0, 0, 0)
	cfg := &config.CacheAPIConfig{
		Allow: []string{},
	}
	api, err := NewPurgeAPI(pc, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod("POST")
	ctx.Request.SetBodyString(`{"path": "/nonexistent"}`)

	api.ServeHTTP(ctx)

	if ctx.Response.StatusCode() != fasthttp.StatusOK {
		t.Fatalf("expected status 200, got %d", ctx.Response.StatusCode())
	}

	var resp PurgeResponse
	if err := json.Unmarshal(ctx.Response.Body(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp.Deleted != 0 {
		t.Errorf("expected 0 deleted, got %d", resp.Deleted)
	}
}

func TestPurgeAPI_ServeHTTP_PurgeByPattern(t *testing.T) {
	pc := NewProxyCache(nil, false, 0, 0, 0)
	pc.Set(hashKey("GET:/api/users"), "GET:/api/users", []byte("users"), nil, 200, 10*60*time.Second)
	pc.Set(hashKey("GET:/api/posts"), "GET:/api/posts", []byte("posts"), nil, 200, 10*60*time.Second)
	pc.Set(hashKey("GET:/static/css"), "GET:/static/css", []byte("css"), nil, 200, 10*60*time.Second)

	cfg := &config.CacheAPIConfig{
		Allow: []string{},
	}
	api, err := NewPurgeAPI(pc, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod("POST")
	ctx.Request.SetBodyString(`{"pattern": "GET:/api/*"}`)

	api.ServeHTTP(ctx)

	if ctx.Response.StatusCode() != fasthttp.StatusOK {
		t.Fatalf("expected status 200, got %d", ctx.Response.StatusCode())
	}

	var resp PurgeResponse
	if err := json.Unmarshal(ctx.Response.Body(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp.Deleted != 2 {
		t.Errorf("expected 2 deleted (api/users and api/posts), got %d", resp.Deleted)
	}

	// Verify /static/css is still there
	_, ok, _ := pc.Get(hashKey("GET:/static/css"), "GET:/static/css")
	if !ok {
		t.Error("expected /static/css to still exist")
	}
}

func TestPurgeAPI_ServeHTTP_PurgeByPattern_Wildcard(t *testing.T) {
	pc := NewProxyCache(nil, false, 0, 0, 0)
	pc.Set(hashKey("GET:/a"), "GET:/a", []byte("a"), nil, 200, 10*60*time.Second)
	pc.Set(hashKey("GET:/b"), "GET:/b", []byte("b"), nil, 200, 10*60*time.Second)

	cfg := &config.CacheAPIConfig{
		Allow: []string{},
	}
	api, err := NewPurgeAPI(pc, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod("POST")
	ctx.Request.SetBodyString(`{"pattern": "*"}`)

	api.ServeHTTP(ctx)

	var resp PurgeResponse
	if err := json.Unmarshal(ctx.Response.Body(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp.Deleted != 2 {
		t.Errorf("expected 2 deleted, got %d", resp.Deleted)
	}
}

func TestPurgeAPI_ServeHTTP_PurgeByPattern_DirPrefix(t *testing.T) {
	pc := NewProxyCache(nil, false, 0, 0, 0)
	pc.Set(hashKey("GET:/api/v1/users"), "GET:/api/v1/users", []byte("u"), nil, 200, 10*60*time.Second)
	pc.Set(hashKey("GET:/api/v2/posts"), "GET:/api/v2/posts", []byte("p"), nil, 200, 10*60*time.Second)
	pc.Set(hashKey("GET:/other"), "GET:/other", []byte("o"), nil, 200, 10*60*time.Second)

	cfg := &config.CacheAPIConfig{
		Allow: []string{},
	}
	api, err := NewPurgeAPI(pc, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod("POST")
	ctx.Request.SetBodyString(`{"pattern": "GET:/api/"}`)

	api.ServeHTTP(ctx)

	var resp PurgeResponse
	if err := json.Unmarshal(ctx.Response.Body(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp.Deleted != 2 {
		t.Errorf("expected 2 deleted, got %d", resp.Deleted)
	}
}

func TestPurgeAPI_ServeHTTP_ContentType(t *testing.T) {
	pc := NewProxyCache(nil, false, 0, 0, 0)
	cfg := &config.CacheAPIConfig{
		Allow: []string{},
	}
	api, err := NewPurgeAPI(pc, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	t.Run("success response content type", func(t *testing.T) {
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.Header.SetMethod("POST")
		ctx.Request.SetBodyString(`{"path": "/test"}`)
		api.ServeHTTP(ctx)

		ct := string(ctx.Response.Header.Peek("Content-Type"))
		if ct != "application/json; charset=utf-8" {
			t.Errorf("expected content-type application/json; charset=utf-8, got %s", ct)
		}
	})

	t.Run("error response content type", func(t *testing.T) {
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.Header.SetMethod("GET")
		api.ServeHTTP(ctx)

		ct := string(ctx.Response.Header.Peek("Content-Type"))
		if ct != "application/json; charset=utf-8" {
			t.Errorf("expected content-type application/json; charset=utf-8, got %s", ct)
		}
	})
}

func TestPurgeAPI_ServeHTTP_AccessAllowed(t *testing.T) {
	pc := NewProxyCache(nil, false, 0, 0, 0)
	cfg := &config.CacheAPIConfig{
		Allow: []string{"10.0.0.0/8"},
	}
	api, err := NewPurgeAPI(pc, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod("POST")
	ctx.Request.SetBodyString(`{"path": "/api/test"}`)
	ctx.SetRemoteAddr(&net.TCPAddr{IP: net.ParseIP("10.1.2.3"), Port: 12345})

	api.ServeHTTP(ctx)

	// Should succeed (access allowed, no auth required)
	if ctx.Response.StatusCode() != fasthttp.StatusOK {
		t.Errorf("expected status 200, got %d", ctx.Response.StatusCode())
	}
}

func TestPurgeAPI_ServeHTTP_TokenAuth(t *testing.T) {
	pc := NewProxyCache(nil, false, 0, 0, 0)
	cfg := &config.CacheAPIConfig{
		Allow: []string{},
		Auth: config.CacheAPIAuthConfig{
			Type:  "token",
			Token: "my-secret",
		},
	}
	api, err := NewPurgeAPI(pc, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	t.Run("Bearer token", func(t *testing.T) {
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.Header.SetMethod("POST")
		ctx.Request.SetBodyString(`{"path": "/test"}`)
		ctx.Request.Header.Set("Authorization", "Bearer my-secret")

		api.ServeHTTP(ctx)

		if ctx.Response.StatusCode() != fasthttp.StatusOK {
			t.Errorf("expected status 200, got %d", ctx.Response.StatusCode())
		}
	})

	t.Run("Direct token", func(t *testing.T) {
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.Header.SetMethod("POST")
		ctx.Request.SetBodyString(`{"path": "/test"}`)
		ctx.Request.Header.Set("Authorization", "my-secret")

		api.ServeHTTP(ctx)

		if ctx.Response.StatusCode() != fasthttp.StatusOK {
			t.Errorf("expected status 200, got %d", ctx.Response.StatusCode())
		}
	})
}

func TestPurgeAPI_ServeHTTP_AuthTypeNone(t *testing.T) {
	pc := NewProxyCache(nil, false, 0, 0, 0)
	cfg := &config.CacheAPIConfig{
		Allow: []string{},
		Auth: config.CacheAPIAuthConfig{
			Type: "none",
		},
	}
	api, err := NewPurgeAPI(pc, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod("POST")
	ctx.Request.SetBodyString(`{"path": "/test"}`)

	api.ServeHTTP(ctx)

	if ctx.Response.StatusCode() != fasthttp.StatusOK {
		t.Errorf("expected status 200, got %d", ctx.Response.StatusCode())
	}
}

func TestPurgeAPI_PurgeByPath_NilCache(t *testing.T) {
	cfg := &config.CacheAPIConfig{
		Allow: []string{},
	}
	api, err := NewPurgeAPI(nil, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Directly test purgeByPath with nil cache
	result := api.purgeByPath("/test")
	if result != 0 {
		t.Errorf("expected 0 deleted with nil cache, got %d", result)
	}
}

func TestPurgeAPI_PurgeByPattern_NilCache(t *testing.T) {
	cfg := &config.CacheAPIConfig{
		Allow: []string{},
	}
	api, err := NewPurgeAPI(nil, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Directly test purgeByPattern with nil cache
	result := api.purgeByPattern("*")
	if result != 0 {
		t.Errorf("expected 0 deleted with nil cache, got %d", result)
	}
}

func TestPurgeAPI_ErrorResponse(t *testing.T) {
	cfg := &config.CacheAPIConfig{
		Allow: []string{},
	}
	api, err := NewPurgeAPI(nil, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	t.Run("method not allowed", func(t *testing.T) {
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.Header.SetMethod("DELETE")
		api.ServeHTTP(ctx)

		if ctx.Response.StatusCode() != fasthttp.StatusMethodNotAllowed {
			t.Fatalf("expected status 405, got %d", ctx.Response.StatusCode())
		}

		var errResp PurgeErrorResponse
		if err := json.Unmarshal(ctx.Response.Body(), &errResp); err != nil {
			t.Fatalf("failed to parse error response: %v", err)
		}
		if errResp.Error != "method not allowed" {
			t.Errorf("expected error 'method not allowed', got %s", errResp.Error)
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		cfg2 := &config.CacheAPIConfig{
			Allow: []string{"10.0.0.0/8"},
		}
		api2, err := NewPurgeAPI(nil, cfg2)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		ctx := &fasthttp.RequestCtx{}
		ctx.Request.Header.SetMethod("POST")
		ctx.SetRemoteAddr(&net.TCPAddr{IP: net.ParseIP("192.168.1.1"), Port: 12345})
		api2.ServeHTTP(ctx)

		if ctx.Response.StatusCode() != fasthttp.StatusForbidden {
			t.Fatalf("expected status 403, got %d", ctx.Response.StatusCode())
		}

		var errResp PurgeErrorResponse
		if err := json.Unmarshal(ctx.Response.Body(), &errResp); err != nil {
			t.Fatalf("failed to parse error response: %v", err)
		}
		if errResp.Error != "forbidden" {
			t.Errorf("expected error 'forbidden', got %s", errResp.Error)
		}
	})
}

func TestPurgeAPI_PurgeByPath_WrongMethod(t *testing.T) {
	// Test that hashPath only uses GET, so purging a POST-cached entry won't work
	pc := NewProxyCache(nil, false, 0, 0, 0)
	// Set a cache entry with GET:/api/test key
	pc.Set(hashKey("GET:/api/test"), "GET:/api/test", []byte("data"), nil, 200, 10*60*time.Second)

	cfg := &config.CacheAPIConfig{
		Allow: []string{},
	}
	api, err := NewPurgeAPI(pc, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod("POST")
	ctx.Request.SetBodyString(`{"path": "/api/test"}`)

	api.ServeHTTP(ctx)

	var resp PurgeResponse
	if err := json.Unmarshal(ctx.Response.Body(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp.Deleted != 1 {
		t.Errorf("expected 1 deleted, got %d", resp.Deleted)
	}
}

func TestMatchPattern_ComplexPatterns(t *testing.T) {
	t.Run("multiple wildcards not supported", func(t *testing.T) {
		// Pattern with multiple * in the middle is not supported
		result := MatchPattern("/api/*/users/*", "/api/v1/users/123")
		if result {
			t.Error("expected complex pattern with multiple wildcards to return false")
		}
	})

	t.Run("pattern without wildcard exact match", func(t *testing.T) {
		result := MatchPattern("/api/users", "/api/users")
		if !result {
			t.Error("expected exact match")
		}
	})

	t.Run("pattern without wildcard no match", func(t *testing.T) {
		result := MatchPattern("/api/users", "/api/users/123")
		if result {
			t.Error("expected no match for non-exact")
		}
	})
}
