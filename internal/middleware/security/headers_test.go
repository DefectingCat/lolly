// Package security 提供安全头部功能的测试。
//
// 该文件测试安全头部模块的各项功能，包括：
//   - 安全头部中间件创建
//   - 各种安全头部设置（CSP、HSTS、X-Frame-Options等）
//   - 配置更新
//   - 默认和严格配置
//   - HSTS 值格式化
//
// 作者：xfy
package security

import (
	"testing"

	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/config"
)

func TestNewHeaders(t *testing.T) {
	tests := []struct {
		cfg  *config.SecurityHeaders
		name string
	}{
		{
			name: "nil config uses defaults",
			cfg:  nil,
		},
		{
			name: "custom config",
			cfg: &config.SecurityHeaders{
				XFrameOptions:         "SAMEORIGIN",
				XContentTypeOptions:   "nosniff",
				ContentSecurityPolicy: "default-src 'self'",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sh := NewHeaders(tt.cfg)
			if sh == nil {
				t.Error("Expected non-nil HeadersMiddleware")
			}
		})
	}
}

func TestHeadersName(t *testing.T) {
	sh := NewHeaders(nil)
	if sh.Name() != "security_headers" {
		t.Errorf("Expected name 'security_headers', got %s", sh.Name())
	}
}

func TestHeadersProcess(t *testing.T) {
	cfg := &config.SecurityHeaders{
		XFrameOptions:         "DENY",
		XContentTypeOptions:   "nosniff",
		ContentSecurityPolicy: "default-src 'self'",
		ReferrerPolicy:        "strict-origin-when-cross-origin",
		PermissionsPolicy:     "geolocation=()",
	}

	sh := NewHeaders(cfg)

	handlerCalled := false
	nextHandler := func(ctx *fasthttp.RequestCtx) {
		handlerCalled = true
		_, _ = ctx.WriteString("OK")
	}

	handler := sh.Process(nextHandler)
	if handler == nil {
		t.Fatal("Process() returned nil handler")
	}

	// Create request context and call handler
	ctx := &fasthttp.RequestCtx{}
	handler(ctx)

	// Check headers were set
	headers := &ctx.Response.Header

	if string(headers.Peek("X-Frame-Options")) != "DENY" {
		t.Errorf("X-Frame-Options not set correctly, got %s", headers.Peek("X-Frame-Options"))
	}

	if string(headers.Peek("X-Content-Type-Options")) != "nosniff" {
		t.Errorf("X-Content-Type-Options not set correctly, got %s", headers.Peek("X-Content-Type-Options"))
	}

	if string(headers.Peek("Content-Security-Policy")) != "default-src 'self'" {
		t.Errorf("Content-Security-Policy not set correctly, got %s", headers.Peek("Content-Security-Policy"))
	}

	if string(headers.Peek("Referrer-Policy")) != "strict-origin-when-cross-origin" {
		t.Errorf("Referrer-Policy not set correctly, got %s", headers.Peek("Referrer-Policy"))
	}

	if string(headers.Peek("Permissions-Policy")) != "geolocation=()" {
		t.Errorf("Permissions-Policy not set correctly, got %s", headers.Peek("Permissions-Policy"))
	}

	if !handlerCalled {
		t.Error("Next handler was not called")
	}
}

func TestHeadersHSTS(_ *testing.T) {
	cfg := &config.SecurityHeaders{
		XFrameOptions: "DENY",
	}

	sh := NewHeaders(cfg)

	nextHandler := func(_ *fasthttp.RequestCtx) {
	}

	handler := sh.Process(nextHandler)

	// Simulate TLS connection
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("https://example.com/")
	// Note: In actual testing, ctx.IsTLS() requires connection setup

	handler(ctx)

	// HSTS header would be set when TLS is active
	// In this test we verify the handler doesn't panic
}

func TestHeadersUpdate(t *testing.T) {
	sh := NewHeaders(nil)

	// Update X-Frame-Options
	sh.SetXFrameOptions("SAMEORIGIN")
	cfg := sh.GetConfig()
	if cfg.XFrameOptions != "SAMEORIGIN" {
		t.Errorf("Expected X-Frame-Options 'SAMEORIGIN', got %s", cfg.XFrameOptions)
	}

	// Update CSP
	sh.SetContentSecurityPolicy("default-src 'unsafe-inline'")
	cfg = sh.GetConfig()
	if cfg.ContentSecurityPolicy != "default-src 'unsafe-inline'" {
		t.Errorf("Expected CSP update, got %s", cfg.ContentSecurityPolicy)
	}

	// Update Referrer-Policy
	sh.SetReferrerPolicy("no-referrer")
	cfg = sh.GetConfig()
	if cfg.ReferrerPolicy != "no-referrer" {
		t.Errorf("Expected Referrer-Policy 'no-referrer', got %s", cfg.ReferrerPolicy)
	}

	// Update Permissions-Policy
	sh.SetPermissionsPolicy("camera=()")
	cfg = sh.GetConfig()
	if cfg.PermissionsPolicy != "camera=()" {
		t.Errorf("Expected Permissions-Policy 'camera=()', got %s", cfg.PermissionsPolicy)
	}
}

func TestUpdateConfig(t *testing.T) {
	sh := NewHeaders(nil)

	newCfg := &config.SecurityHeaders{
		XFrameOptions:  "DENY",
		ReferrerPolicy: "no-referrer",
	}

	sh.UpdateConfig(newCfg)

	cfg := sh.GetConfig()
	if cfg.XFrameOptions != "DENY" {
		t.Errorf("Expected X-Frame-Options 'DENY', got %s", cfg.XFrameOptions)
	}
	if cfg.ReferrerPolicy != "no-referrer" {
		t.Errorf("Expected Referrer-Policy 'no-referrer', got %s", cfg.ReferrerPolicy)
	}
}

func TestDefaultSecurityHeaders(t *testing.T) {
	cfg := defaultSecurityHeaders()

	if cfg.XFrameOptions != "DENY" {
		t.Errorf("Expected default X-Frame-Options 'DENY', got %s", cfg.XFrameOptions)
	}
	if cfg.XContentTypeOptions != "nosniff" {
		t.Errorf("Expected default X-Content-Type-Options 'nosniff', got %s", cfg.XContentTypeOptions)
	}
}

func TestStrictSecurityHeaders(t *testing.T) {
	cfg := strictSecurityHeaders()

	if cfg.XFrameOptions != "DENY" {
		t.Errorf("Expected X-Frame-Options 'DENY', got %s", cfg.XFrameOptions)
	}
	if cfg.ReferrerPolicy != "no-referrer" {
		t.Errorf("Expected Referrer-Policy 'no-referrer', got %s", cfg.ReferrerPolicy)
	}
	if cfg.ContentSecurityPolicy == "" {
		t.Error("Expected non-empty CSP for strict config")
	}
}

func TestDevelopmentSecurityHeaders(t *testing.T) {
	cfg := developmentSecurityHeaders()

	if cfg.XFrameOptions != "SAMEORIGIN" {
		t.Errorf("Expected X-Frame-Options 'SAMEORIGIN' for dev, got %s", cfg.XFrameOptions)
	}
}

func TestFormatHSTSValue(t *testing.T) {
	tests := []struct {
		name              string
		expected          string
		maxAge            int
		includeSubDomains bool
		preload           bool
	}{
		{
			name:              "basic HSTS",
			maxAge:            31536000,
			includeSubDomains: true,
			preload:           false,
			expected:          "max-age=31536000; includeSubDomains",
		},
		{
			name:              "HSTS with preload",
			maxAge:            31536000,
			includeSubDomains: true,
			preload:           true,
			expected:          "max-age=31536000; includeSubDomains; preload",
		},
		{
			name:              "HSTS without subdomains",
			maxAge:            86400,
			includeSubDomains: false,
			preload:           false,
			expected:          "max-age=86400",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatHSTSValue(tt.maxAge, tt.includeSubDomains, tt.preload)
			if result != tt.expected {
				t.Errorf("formatHSTSValue() = %s, expected %s", result, tt.expected)
			}
		})
	}
}

// TestNewHeadersWithHSTS 测试带 HSTS 配置的创建
func TestNewHeadersWithHSTS(t *testing.T) {
	cfg := &config.SecurityHeaders{
		XFrameOptions: "DENY",
	}

	hstsCfg := &config.HSTSConfig{
		MaxAge:            86400,
		IncludeSubDomains: true,
		Preload:           false,
	}

	sh := NewHeadersWithHSTS(cfg, hstsCfg)
	if sh == nil {
		t.Error("NewHeadersWithHSTS() should not return nil")
	}
	if sh.hsts != "max-age=86400; includeSubDomains" {
		t.Errorf("HSTS = %q, want 'max-age=86400; includeSubDomains'", sh.hsts)
	}
}

// TestNewHeadersWithHSTS_NilConfig 测试 nil HSTS 配置
func TestNewHeadersWithHSTS_NilConfig(t *testing.T) {
	sh := NewHeadersWithHSTS(nil, nil)
	if sh == nil {
		t.Error("NewHeadersWithHSTS() should not return nil")
	}
	// 应该使用默认配置
	if sh.config == nil {
		t.Error("Should use default config when nil is passed")
	}
}

// TestNewHeadersWithHSTS_ZeroMaxAge 测试 MaxAge 为 0 时使用默认值
func TestNewHeadersWithHSTS_ZeroMaxAge(t *testing.T) {
	cfg := &config.SecurityHeaders{
		XFrameOptions: "DENY",
	}

	hstsCfg := &config.HSTSConfig{
		MaxAge:            0, // 应该使用默认值 31536000
		IncludeSubDomains: true,
		Preload:           false,
	}

	sh := NewHeadersWithHSTS(cfg, hstsCfg)
	if sh.hsts != "max-age=31536000; includeSubDomains" {
		t.Errorf("HSTS with zero maxAge should use default: %q", sh.hsts)
	}
}

// TestHeadersProcess_NilConfig 测试 Process 处理 nil config
func TestHeadersProcess_NilConfig(t *testing.T) {
	sh := NewHeadersWithHSTS(nil, nil)

	nextCalled := false
	nextHandler := func(ctx *fasthttp.RequestCtx) {
		nextCalled = true
	}

	handler := sh.Process(nextHandler)
	ctx := &fasthttp.RequestCtx{}
	handler(ctx)

	if !nextCalled {
		t.Error("Next handler should be called")
	}

	// 验证默认安全头被设置
	if string(ctx.Response.Header.Peek("X-Content-Type-Options")) != "nosniff" {
		t.Error("Default X-Content-Type-Options should be set")
	}
}

// TestHeadersProcess_TLS 测试 TLS 情况下 HSTS 头设置
func TestHeadersProcess_TLS(t *testing.T) {
	cfg := &config.SecurityHeaders{
		XFrameOptions: "DENY",
	}

	hstsCfg := &config.HSTSConfig{
		MaxAge:            31536000,
		IncludeSubDomains: true,
		Preload:           true,
	}

	sh := NewHeadersWithHSTS(cfg, hstsCfg)

	nextHandler := func(ctx *fasthttp.RequestCtx) {}

	handler := sh.Process(nextHandler)
	ctx := &fasthttp.RequestCtx{}
	// 注意：在测试环境中无法真正模拟 TLS 连接
	// 这个测试验证 handler 不会 panic
	handler(ctx)
}

// TestAddHeaders_AllHeaders 测试所有安全头设置
func TestAddHeaders_AllHeaders(t *testing.T) {
	cfg := &config.SecurityHeaders{
		XFrameOptions:         "SAMEORIGIN",
		XContentTypeOptions:   "nosniff",
		ContentSecurityPolicy: "default-src 'self'",
		ReferrerPolicy:        "strict-origin",
		PermissionsPolicy:     "camera=(), microphone=()",
	}

	sh := NewHeaders(cfg)

	nextHandler := func(ctx *fasthttp.RequestCtx) {}
	handler := sh.Process(nextHandler)

	ctx := &fasthttp.RequestCtx{}
	handler(ctx)

	headers := &ctx.Response.Header

	if string(headers.Peek("X-Frame-Options")) != "SAMEORIGIN" {
		t.Error("X-Frame-Options not set correctly")
	}
	if string(headers.Peek("X-Content-Type-Options")) != "nosniff" {
		t.Error("X-Content-Type-Options not set correctly")
	}
	if string(headers.Peek("Content-Security-Policy")) != "default-src 'self'" {
		t.Error("Content-Security-Policy not set correctly")
	}
	if string(headers.Peek("Referrer-Policy")) != "strict-origin" {
		t.Error("Referrer-Policy not set correctly")
	}
	if string(headers.Peek("Permissions-Policy")) != "camera=(), microphone=()" {
		t.Error("Permissions-Policy not set correctly")
	}
}
