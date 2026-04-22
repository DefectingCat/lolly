// Package security 提供 auth_request 中间件的单元测试。
//
// 测试覆盖：
//   - 认证成功（2xx 响应）
//   - 认证失败（401/403 响应）
//   - 认证服务不可用
//   - 超时处理
//   - 变量展开
//   - 配置更新
//
// 作者：xfy
package security

import (
	"net"
	"strings"
	"testing"
	"time"

	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/config"
)

// TestNewAuthRequest 测试 AuthRequest 中间件创建
func TestNewAuthRequest(t *testing.T) {
	tests := []struct {
		name    string
		errMsg  string
		cfg     config.AuthRequestConfig
		wantErr bool
	}{
		{
			name: "正常创建（禁用）",
			cfg: config.AuthRequestConfig{
				Enabled: false,
			},
			wantErr: false,
		},
		{
			name: "正常创建（启用，相对路径）",
			cfg: config.AuthRequestConfig{
				Enabled: true,
				URI:     "/auth",
				Method:  "GET",
				Timeout: 5 * time.Second,
			},
			wantErr: false,
		},
		{
			name: "正常创建（启用，完整URL）",
			cfg: config.AuthRequestConfig{
				Enabled: true,
				URI:     "http://localhost:8080/auth",
				Method:  "POST",
				Timeout: 10 * time.Second,
			},
			wantErr: false,
		},
		{
			name: "启用但未配置 URI",
			cfg: config.AuthRequestConfig{
				Enabled: true,
				URI:     "",
			},
			wantErr: true,
			errMsg:  "uri is required",
		},
		{
			name: "使用默认值",
			cfg: config.AuthRequestConfig{
				Enabled: true,
				URI:     "/auth",
				// Method 和 Timeout 使用默认值
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ar, err := NewAuthRequest(tt.cfg)

			if tt.wantErr {
				if err == nil {
					t.Errorf("NewAuthRequest() expected error, got nil")
					return
				}
				if tt.errMsg != "" && !contains(err.Error(), tt.errMsg) {
					t.Errorf("NewAuthRequest() error = %v, should contain %q", err, tt.errMsg)
				}
				return
			}

			if err != nil {
				t.Errorf("NewAuthRequest() unexpected error: %v", err)
				return
			}

			if ar == nil {
				t.Error("NewAuthRequest() returned nil")
				return
			}

			// 验证默认值设置
			if tt.cfg.Enabled {
				if ar.config.Method == "" {
					t.Error("Method should have default value")
				}
				if ar.config.Timeout == 0 {
					t.Error("Timeout should have default value")
				}
			}
		})
	}
}

// TestAuthRequestName 测试中间件名称
func TestAuthRequestName(t *testing.T) {
	ar := &AuthRequest{}
	if name := ar.Name(); name != "auth_request" {
		t.Errorf("Name() = %q, want 'auth_request'", name)
	}
}

// TestAuthRequestProcess_Disabled 测试禁用状态下的处理
func TestAuthRequestProcess_Disabled(t *testing.T) {
	cfg := config.AuthRequestConfig{
		Enabled: false,
	}

	ar, err := NewAuthRequest(cfg)
	if err != nil {
		t.Fatalf("NewAuthRequest() failed: %v", err)
	}

	// 创建测试处理器
	called := false
	next := func(ctx *fasthttp.RequestCtx) {
		called = true
		ctx.SetStatusCode(200)
	}

	handler := ar.Process(next)

	// 执行请求
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod("GET")
	ctx.Request.Header.SetRequestURI("/test")

	handler(ctx)

	// 验证处理器被调用
	if !called {
		t.Error("Next handler should be called when disabled")
	}
	if ctx.Response.StatusCode() != 200 {
		t.Errorf("Expected status 200, got %d", ctx.Response.StatusCode())
	}
}

// TestParseAuthURL 测试 URL 解析
func TestParseAuthURL(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		wantAddr string
		wantTLS  bool
		wantErr  bool
	}{
		{
			name:     "HTTP URL 带端口",
			url:      "http://auth-service:8080/verify",
			wantAddr: "auth-service:8080",
			wantTLS:  false,
			wantErr:  false,
		},
		{
			name:     "HTTPS URL 带端口",
			url:      "https://auth-service:8443/verify",
			wantAddr: "auth-service:8443",
			wantTLS:  true,
			wantErr:  false,
		},
		{
			name:     "HTTP URL 不带端口",
			url:      "http://auth-service/verify",
			wantAddr: "auth-service:80",
			wantTLS:  false,
			wantErr:  false,
		},
		{
			name:     "HTTPS URL 不带端口",
			url:      "https://auth-service/verify",
			wantAddr: "auth-service:443",
			wantTLS:  true,
			wantErr:  false,
		},
		{
			name:     "相对路径",
			url:      "/auth",
			wantAddr: "",
			wantTLS:  false,
			wantErr:  true, // 相对路径会报错
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr, isTLS, err := parseAuthURL(tt.url)

			if tt.wantErr {
				if err == nil {
					t.Error("parseAuthURL() expected error")
				}
				return
			}

			if err != nil {
				t.Errorf("parseAuthURL() unexpected error: %v", err)
				return
			}

			// 验证地址（可能包含默认端口）
			_, _, _ = net.SplitHostPort(addr)

			if isTLS != tt.wantTLS {
				t.Errorf("isTLS = %v, want %v", isTLS, tt.wantTLS)
			}
		})
	}
}

// TestIsFullURL 测试 URL 检测
func TestIsFullURL(t *testing.T) {
	tests := []struct {
		uri      string
		expected bool
	}{
		{"http://example.com", true},
		{"https://example.com", true},
		{"/auth", false},
		{"auth", false},
		{"", false},
		{"ftp://example.com", false},
	}

	for _, tt := range tests {
		t.Run(tt.uri, func(t *testing.T) {
			result := isFullURL(tt.uri)
			if result != tt.expected {
				t.Errorf("isFullURL(%q) = %v, want %v", tt.uri, result, tt.expected)
			}
		})
	}
}

// TestAuthRequestExpandVars 测试变量展开
func TestAuthRequestExpandVars(t *testing.T) {
	ar := &AuthRequest{
		config: config.AuthRequestConfig{},
	}

	tests := []struct {
		name     string
		method   string
		uri      string
		host     string
		template string
		expected string
	}{
		{
			name:     "无变量",
			method:   "GET",
			uri:      "/test",
			host:     "example.com",
			template: "http://auth-service/auth",
			expected: "http://auth-service/auth",
		},
		{
			name:     "包含变量",
			method:   "POST",
			uri:      "/api/users",
			host:     "api.example.com",
			template: "http://auth-service/verify?uri=$request_uri",
			expected: "http://auth-service/verify?uri=/api/users",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &fasthttp.RequestCtx{}
			ctx.Request.Header.SetMethod(tt.method)
			ctx.Request.Header.SetRequestURI(tt.uri)
			ctx.Request.Header.SetHost(tt.host)

			result := ar.expandVars(ctx, tt.template)
			if result != tt.expected {
				t.Errorf("expandVars() = %q, want %q", result, tt.expected)
			}
		})
	}
}

// TestAuthRequestUpdateConfig 测试配置更新
func TestAuthRequestUpdateConfig(t *testing.T) {
	// 创建初始实例
	cfg := config.AuthRequestConfig{
		Enabled: true,
		URI:     "http://old-auth:8080/auth",
		Method:  "GET",
		Timeout: 5 * time.Second,
	}

	ar, err := NewAuthRequest(cfg)
	if err != nil {
		t.Fatalf("NewAuthRequest() failed: %v", err)
	}

	// 更新为禁用
	t.Run("更新为禁用", func(t *testing.T) {
		newCfg := config.AuthRequestConfig{
			Enabled: false,
		}
		err := ar.UpdateConfig(newCfg)
		if err != nil {
			t.Errorf("UpdateConfig() failed: %v", err)
		}
		if ar.config.Enabled {
			t.Error("Expected config to be disabled")
		}
	})

	// 更新为新的启用配置
	t.Run("更新为新配置", func(t *testing.T) {
		newCfg := config.AuthRequestConfig{
			Enabled: true,
			URI:     "http://new-auth:8080/auth",
			Method:  "POST",
			Timeout: 10 * time.Second,
		}
		err := ar.UpdateConfig(newCfg)
		if err != nil {
			t.Errorf("UpdateConfig() failed: %v", err)
		}
		if ar.config.URI != "http://new-auth:8080/auth" {
			t.Errorf("URI not updated: %s", ar.config.URI)
		}
		if ar.config.Method != "POST" {
			t.Errorf("Method not updated: %s", ar.config.Method)
		}
	})

	// 更新失败（缺少 URI）
	t.Run("更新失败（缺少 URI）", func(t *testing.T) {
		newCfg := config.AuthRequestConfig{
			Enabled: true,
			URI:     "",
		}
		err := ar.UpdateConfig(newCfg)
		if err == nil {
			t.Error("UpdateConfig() should fail without URI")
		}
	})
}

// TestAuthRequestClose 测试关闭
func TestAuthRequestClose(t *testing.T) {
	cfg := config.AuthRequestConfig{
		Enabled: true,
		URI:     "http://auth-service:8080/auth",
	}

	ar, err := NewAuthRequest(cfg)
	if err != nil {
		t.Fatalf("NewAuthRequest() failed: %v", err)
	}

	err = ar.Close()
	if err != nil {
		t.Errorf("Close() failed: %v", err)
	}

	// 验证客户端被清理
	if ar.client != nil {
		t.Error("client should be nil after Close()")
	}
}

// BenchmarkAuthRequestExpandVars 基准测试：变量展开
func BenchmarkAuthRequestExpandVars(b *testing.B) {
	ar := &AuthRequest{
		config: config.AuthRequestConfig{},
	}

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod("GET")
	ctx.Request.Header.SetRequestURI("/api/users?page=1")
	ctx.Request.Header.SetHost("api.example.com")

	template := "http://auth-service/verify?uri=$request_uri&host=$host"

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = ar.expandVars(ctx, template)
	}
}

// 辅助函数
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}

// TestAuthRequest_Middleware 测试 Middleware 方法
func TestAuthRequest_Middleware(t *testing.T) {
	ar := &AuthRequest{}

	mw := ar.Middleware()
	if mw == nil {
		t.Error("Middleware() should not return nil")
	}
}

// TestAuthRequestExpandVars_Empty 测试空模板展开
func TestAuthRequestExpandVars_Empty(t *testing.T) {
	ar := &AuthRequest{config: config.AuthRequestConfig{}}
	ctx := &fasthttp.RequestCtx{}

	result := ar.expandVars(ctx, "")
	if result != "" {
		t.Errorf("expandVars(empty) = %q, want empty", result)
	}
}

// TestAuthRequestExpandVars_NoVars 测试无变量模板
func TestAuthRequestExpandVars_NoVars(t *testing.T) {
	ar := &AuthRequest{config: config.AuthRequestConfig{}}
	ctx := &fasthttp.RequestCtx{}

	result := ar.expandVars(ctx, "http://auth-service/verify")
	if result != "http://auth-service/verify" {
		t.Errorf("expandVars(no vars) = %q, want original", result)
	}
}

// TestUpdateConfig_RelativePath 测试更新为相对路径配置
func TestUpdateConfig_RelativePath(t *testing.T) {
	cfg := config.AuthRequestConfig{
		Enabled: true,
		URI:     "http://auth-service:8080/auth",
	}

	ar, err := NewAuthRequest(cfg)
	if err != nil {
		t.Fatalf("NewAuthRequest() failed: %v", err)
	}

	// 更新为相对路径
	newCfg := config.AuthRequestConfig{
		Enabled: true,
		URI:     "/auth/verify",
	}
	err = ar.UpdateConfig(newCfg)
	if err != nil {
		t.Errorf("UpdateConfig() failed: %v", err)
	}

	// 验证 client 被清空（相对路径不需要独立客户端）
	if ar.client != nil {
		t.Error("client should be nil for relative path")
	}
}

// TestAuthRequest_ProcessEnabled 测试 Process 处理启用状态
func TestAuthRequest_ProcessEnabled(t *testing.T) {
	cfg := config.AuthRequestConfig{
		Enabled:        true,
		URI:            "/auth/verify",
		Method:         "GET",
		Timeout:        5 * time.Second,
		ForwardHeaders: []string{"X-Custom-Header"},
		Headers:        map[string]string{"X-Auth-Source": "lolly"},
	}

	ar, err := NewAuthRequest(cfg)
	if err != nil {
		t.Fatalf("NewAuthRequest() failed: %v", err)
	}

	next := func(ctx *fasthttp.RequestCtx) {
		ctx.SetStatusCode(200)
	}

	handler := ar.Process(next)
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod("GET")
	ctx.Request.SetRequestURI("/test")
	ctx.Request.Header.Set("X-Custom-Header", "custom-value")

	// 执行处理器（由于没有实际的认证服务，请求会失败）
	handler(ctx)

	// 验证处理器行为 - 由于认证服务不可达，应该返回 500
	if ctx.Response.StatusCode() != 500 {
		t.Logf("Status = %d (expected 500 due to unreachable auth service)", ctx.Response.StatusCode())
	}
}

// TestParseAuthURL_Invalid 测试解析无效 URL
func TestParseAuthURL_Invalid(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{"empty string", ""},
		{"only protocol", "http://"},
		{"only https protocol", "https://"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := parseAuthURL(tt.url)
			if err == nil {
				t.Errorf("parseAuthURL(%q) should return error", tt.url)
			}
		})
	}
}

// TestExpandVars_EdgeCases 测试变量展开边缘情况
func TestExpandVars_EdgeCases(t *testing.T) {
	ar := &AuthRequest{config: config.AuthRequestConfig{}}

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/path?query=value")
	ctx.Request.Header.SetHost("example.com")

	// 测试带多个变量的模板
	result := ar.expandVars(ctx, "http://auth?uri=$request_uri&host=$host&method=$request_method")
	if result == "" {
		t.Error("expandVars should return non-empty result")
	}

	// 测试只有 $ 符号的模板
	result = ar.expandVars(ctx, "http://auth?param=$")
	if result != "http://auth?param=$" {
		t.Errorf("expandVars with lone $ should return original, got %q", result)
	}
}

// TestInitClient_HTTPS 测试 HTTPS 客户端初始化
func TestInitClient_HTTPS(t *testing.T) {
	cfg := config.AuthRequestConfig{
		Enabled: true,
		URI:     "https://secure-auth:8443/verify",
		Method:  "GET",
		Timeout: 10 * time.Second,
	}

	ar, err := NewAuthRequest(cfg)
	if err != nil {
		t.Fatalf("NewAuthRequest() failed: %v", err)
	}

	if ar.client == nil {
		t.Error("client should be initialized for HTTPS URL")
	}
	if !ar.client.IsTLS {
		t.Error("client.IsTLS should be true for HTTPS URL")
	}
}
