package rewrite

import (
	"bytes"
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

func TestRewriteMiddlewareLast(t *testing.T) {
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

func TestRewriteMiddlewareRedirect(t *testing.T) {
	m, err := New([]config.RewriteRule{
		{Pattern: "^/old/(.*)$", Replacement: "/new/$1", Flag: "redirect"},
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	handlerCalled := false
	nextHandler := func(ctx *fasthttp.RequestCtx) {
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

func TestRewriteMiddlewarePermanent(t *testing.T) {
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

func TestRewriteMiddlewareBreak(t *testing.T) {
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

func TestRewriteMiddlewareChain(t *testing.T) {
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

func TestRewriteMiddlewareNoMatch(t *testing.T) {
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

func TestRewriteMiddlewareName(t *testing.T) {
	m, err := New(nil)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	if m.Name() != "rewrite" {
		t.Errorf("Expected name 'rewrite', got %s", m.Name())
	}
}

func TestRewriteMiddlewareRules(t *testing.T) {
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