// Package rewrite 提供 URL 重写功能的测试。
//
// 该文件测试 URL 重写模块的各项功能，包括：
//   - 重写规则解析
//   - 正则表达式匹配
//   - 重定向和重写
//   - 规则链执行
//   - ReDoS 防护
//
// 作者：xfy
package rewrite

import (
	"bytes"
	"strings"
	"testing"

	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/config"
)

func TestParseFlag(t *testing.T) {
	tests := []struct {
		input    string
		expected Flag
	}{
		{"last", FlagLast},
		{"redirect", FlagRedirect},
		{"permanent", FlagPermanent},
		{"break", FlagBreak},
		{"LAST", FlagLast},
		{"Redirect", FlagRedirect},
		{"", FlagLast},
		{"unknown", FlagLast},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := parseFlag(tt.input)
			if result != tt.expected {
				t.Errorf("parseFlag(%s) = %v, expected %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestNew(t *testing.T) {
	tests := []struct {
		name    string
		rules   []config.RewriteRule
		wantErr bool
	}{
		{
			name:  "empty rules",
			rules: nil,
		},
		{
			name: "valid rule",
			rules: []config.RewriteRule{
				{Pattern: "^/old/(.*)$", Replacement: "/new/$1", Flag: "last"},
			},
		},
		{
			name: "invalid regex",
			rules: []config.RewriteRule{
				{Pattern: "[invalid", Replacement: "/new", Flag: "last"},
			},
			wantErr: true,
		},
		{
			name: "multiple rules",
			rules: []config.RewriteRule{
				{Pattern: "^/api/v1/(.*)$", Replacement: "/api/v2/$1", Flag: "last"},
				{Pattern: "^/old$", Replacement: "/new", Flag: "permanent"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := New(tt.rules)
			if (err != nil) != tt.wantErr {
				t.Errorf("New() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && m == nil {
				t.Error("Expected non-nil middleware")
			}
		})
	}
}

func TestMiddlewareLast(t *testing.T) {
	m, err := New([]config.RewriteRule{
		{Pattern: "^/old/(.*)$", Replacement: "/new/$1", Flag: "last"},
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	handlerCalled := false
	nextHandler := func(ctx *fasthttp.RequestCtx) {
		handlerCalled = true
		if string(ctx.Path()) != "/new/test" {
			t.Errorf("Expected path /new/test, got %s", ctx.Path())
		}
		ctx.WriteString("OK")
	}

	handler := m.Process(nextHandler)

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/old/test")

	handler(ctx)

	if !handlerCalled {
		t.Error("Handler was not called")
	}
}

func TestMiddlewareRedirect(t *testing.T) {
	m, err := New([]config.RewriteRule{
		{Pattern: "^/old/(.*)$", Replacement: "/new/$1", Flag: "redirect"},
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	handlerCalled := false
	nextHandler := func(_ *fasthttp.RequestCtx) {
		handlerCalled = true
	}

	handler := m.Process(nextHandler)

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/old/test")

	handler(ctx)

	if handlerCalled {
		t.Error("Handler should not be called for redirect")
	}

	// 检查重定向
	loc := ctx.Response.Header.Peek("Location")
	// fasthttp 会构建完整 URL，所以检查后缀
	if !bytes.HasSuffix(loc, []byte("/new/test")) {
		t.Errorf("Expected Location ending with /new/test, got %s", loc)
	}
	if ctx.Response.StatusCode() != fasthttp.StatusFound {
		t.Errorf("Expected status %d, got %d", fasthttp.StatusFound, ctx.Response.StatusCode())
	}
}

func TestMiddlewarePermanent(t *testing.T) {
	m, err := New([]config.RewriteRule{
		{Pattern: "^/old/(.*)$", Replacement: "/new/$1", Flag: "permanent"},
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	nextHandler := func(ctx *fasthttp.RequestCtx) {
		ctx.WriteString("OK")
	}

	handler := m.Process(nextHandler)

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/old/page")

	handler(ctx)

	loc := ctx.Response.Header.Peek("Location")
	// fasthttp 会构建完整 URL，所以检查后缀
	if !bytes.HasSuffix(loc, []byte("/new/page")) {
		t.Errorf("Expected Location ending with /new/page, got %s", loc)
	}
	if ctx.Response.StatusCode() != fasthttp.StatusMovedPermanently {
		t.Errorf("Expected status %d, got %d", fasthttp.StatusMovedPermanently, ctx.Response.StatusCode())
	}
}

func TestMiddlewareBreak(t *testing.T) {
	m, err := New([]config.RewriteRule{
		{Pattern: "^/api/(.*)$", Replacement: "/internal/$1", Flag: "break"},
		{Pattern: "^/internal/(.*)$", Replacement: "/final/$1", Flag: "last"},
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	handlerCalled := false
	nextHandler := func(ctx *fasthttp.RequestCtx) {
		handlerCalled = true
		// break 标志应该停止匹配，所以路径应该是 /internal/test
		if string(ctx.Path()) != "/internal/test" {
			t.Errorf("Expected path /internal/test, got %s", ctx.Path())
		}
	}

	handler := m.Process(nextHandler)

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/api/test")

	handler(ctx)

	if !handlerCalled {
		t.Error("Handler was not called")
	}
}

func TestMiddlewareChain(t *testing.T) {
	// 测试多个 last 规则链式应用
	m, err := New([]config.RewriteRule{
		{Pattern: "^/v1/(.*)$", Replacement: "/v2/$1", Flag: "last"},
		{Pattern: "^/v2/(.*)$", Replacement: "/v3/$1", Flag: "last"},
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	handlerCalled := false
	nextHandler := func(ctx *fasthttp.RequestCtx) {
		handlerCalled = true
		if string(ctx.Path()) != "/v3/resource" {
			t.Errorf("Expected path /v3/resource, got %s", ctx.Path())
		}
	}

	handler := m.Process(nextHandler)

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/v1/resource")

	handler(ctx)

	if !handlerCalled {
		t.Error("Handler was not called")
	}
}

func TestMiddlewareNoMatch(t *testing.T) {
	m, err := New([]config.RewriteRule{
		{Pattern: "^/old/(.*)$", Replacement: "/new/$1", Flag: "last"},
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	handlerCalled := false
	nextHandler := func(ctx *fasthttp.RequestCtx) {
		handlerCalled = true
		if string(ctx.Path()) != "/other/path" {
			t.Errorf("Expected path /other/path, got %s", ctx.Path())
		}
	}

	handler := m.Process(nextHandler)

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/other/path")

	handler(ctx)

	if !handlerCalled {
		t.Error("Handler was not called")
	}
}

func TestMiddlewareName(t *testing.T) {
	m, err := New(nil)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	if m.Name() != "rewrite" {
		t.Errorf("Expected name 'rewrite', got %s", m.Name())
	}
}

func TestMiddlewareRules(t *testing.T) {
	rules := []config.RewriteRule{
		{Pattern: "^/a/(.*)$", Replacement: "/b/$1", Flag: "last"},
		{Pattern: "^/c$", Replacement: "/d", Flag: "redirect"},
	}
	m, err := New(rules)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	compiled := m.Rules()
	if len(compiled) != 2 {
		t.Errorf("Expected 2 rules, got %d", len(compiled))
	}
}

func TestReDoSProtection(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		wantErr bool
	}{
		{
			name:    "safe pattern",
			pattern: "^/api/v1/(.*)$",
			wantErr: false,
		},
		{
			name:    "nested quantifier (\\w+)+",
			pattern: `(\w+)+`,
			wantErr: true,
		},
		{
			name:    "nested quantifier (.+)+",
			pattern: `(.+)+`,
			wantErr: true,
		},
		{
			name:    "nested quantifier (\\d+)+",
			pattern: `(\d+)+`,
			wantErr: true,
		},
		{
			name:    "pattern too long",
			pattern: strings.Repeat("a", 1001),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rules := []config.RewriteRule{
				{Pattern: tt.pattern, Replacement: "/new", Flag: "last"},
			}
			_, err := New(rules)
			if (err != nil) != tt.wantErr {
				t.Errorf("New() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestCrossRuleCycle 测试跨规则循环检测
// 规则 A → B → A 应该被检测为循环
func TestCrossRuleCycle(t *testing.T) {
	// 模拟规则 A: /a 重写为 /b
	// 规则 B: /b 重写为 /a
	// 这将形成 A → B → A 的循环
	m, err := New([]config.RewriteRule{
		{Pattern: "^/a$", Replacement: "/b", Flag: "last"},
		{Pattern: "^/b$", Replacement: "/a", Flag: "last"},
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	nextHandler := func(_ *fasthttp.RequestCtx) {
		t.Error("Next handler should not be called in a loop scenario")
	}

	handler := m.Process(nextHandler)

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/a")

	handler(ctx)

	// 应该返回 500 内部服务器错误
	if ctx.Response.StatusCode() != fasthttp.StatusInternalServerError {
		t.Errorf("Expected status %d for infinite loop, got %d",
			fasthttp.StatusInternalServerError, ctx.Response.StatusCode())
	}

	// 检查错误消息
	body := string(ctx.Response.Body())
	if body != "Internal Server Error" {
		t.Errorf("Expected body 'Internal Server Error', got %q", body)
	}
}

// TestFlagLastRescan 测试 FlagLast 的重新扫描语义（nginx 兼容行为）
// FlagLast 应该重新从第一条规则开始匹配
func TestFlagLastRescan(t *testing.T) {
	// 规则1: /old/* → /new/*
	// 规则2: /new/* → /final/*
	// 当请求 /old/resource 时：
	// - 规则1匹配，重写为 /new/resource，FlagLast 重新从规则1开始
	// - 规则2匹配，重写为 /final/resource
	m, err := New([]config.RewriteRule{
		{Pattern: "^/old/(.*)$", Replacement: "/new/$1", Flag: "last"},
		{Pattern: "^/new/(.*)$", Replacement: "/final/$1", Flag: "last"},
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	handlerCalled := false
	var finalPath string
	nextHandler := func(ctx *fasthttp.RequestCtx) {
		handlerCalled = true
		finalPath = string(ctx.Path())
	}

	handler := m.Process(nextHandler)

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/old/resource")

	handler(ctx)

	if !handlerCalled {
		t.Error("Handler was not called")
	}

	// 最终路径应该是 /final/resource，因为规则1重写后 FlagLast 会重新扫描
	// 然后规则2匹配
	if finalPath != "/final/resource" {
		t.Errorf("Expected final path /final/resource, got %s", finalPath)
	}
}

// TestFlagBreakNoLoop 测试 FlagBreak 不触发循环检测
func TestFlagBreakNoLoop(t *testing.T) {
	// 规则1: /a → /b，使用 break
	// 规则2: /b → /a，使用 break
	// break 应该停止匹配，不应该形成循环
	m, err := New([]config.RewriteRule{
		{Pattern: "^/a$", Replacement: "/b", Flag: "break"},
		{Pattern: "^/b$", Replacement: "/a", Flag: "break"},
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	handlerCalled := false
	var finalPath string
	nextHandler := func(ctx *fasthttp.RequestCtx) {
		handlerCalled = true
		finalPath = string(ctx.Path())
	}

	handler := m.Process(nextHandler)

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/a")

	handler(ctx)

	if !handlerCalled {
		t.Error("Handler was not called")
	}

	// 规则1匹配后 break，所以路径应该是 /b
	if finalPath != "/b" {
		t.Errorf("Expected final path /b (stop at break), got %s", finalPath)
	}
}

// TestIterationLimitExact 测试精确的迭代限制
func TestIterationLimitExact(t *testing.T) {
	// 两条规则形成循环:
	// 规则1: /a 重写为 /b
	// 规则2: /b 重写为 /a
	// 从 /a 开始，经过10次迭代应该触发 500 错误
	m, err := New([]config.RewriteRule{
		{Pattern: "^/a$", Replacement: "/b", Flag: "last"},
		{Pattern: "^/b$", Replacement: "/a", Flag: "last"},
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	nextHandler := func(_ *fasthttp.RequestCtx) {
		t.Error("Next handler should not be called when iteration limit exceeded")
	}

	handler := m.Process(nextHandler)

	// 从 /a 开始，应该触发循环检测
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/a")

	handler(ctx)

	// 应该返回 500，因为每次匹配都会触发迭代计数
	// 迭代过程: /a -> /b -> /a -> /b -> ... 直到超过10次
	if ctx.Response.StatusCode() != fasthttp.StatusInternalServerError {
		t.Errorf("Expected status %d for exceeding iteration limit, got %d",
			fasthttp.StatusInternalServerError, ctx.Response.StatusCode())
	}
}

// TestNormalRewriteNotAffected 测试正常重写不受影响
func TestNormalRewriteNotAffected(t *testing.T) {
	m, err := New([]config.RewriteRule{
		{Pattern: "^/api/v1/(.*)$", Replacement: "/api/v2/$1", Flag: "last"},
		{Pattern: "^/static/(.*)$", Replacement: "/assets/$1", Flag: "last"},
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	handlerCalled := false
	var finalPath string
	nextHandler := func(ctx *fasthttp.RequestCtx) {
		handlerCalled = true
		finalPath = string(ctx.Path())
	}

	handler := m.Process(nextHandler)

	// 测试 /api/v1/users 重写为 /api/v2/users
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/api/v1/users")

	handler(ctx)

	if !handlerCalled {
		t.Error("Handler was not called")
	}

	if finalPath != "/api/v2/users" {
		t.Errorf("Expected final path /api/v2/users, got %s", finalPath)
	}
}
