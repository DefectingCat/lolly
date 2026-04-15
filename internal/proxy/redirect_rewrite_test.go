package proxy

import (
	"testing"

	"rua.plus/lolly/internal/config"
	"rua.plus/lolly/internal/testutil"

	"github.com/valyala/fasthttp"
)

// TestRedirectRewrite_ExactMatch 测试精确匹配改写
func TestRedirectRewrite_ExactMatch(t *testing.T) {
	cfg := &config.RedirectRewriteConfig{
		Mode: "custom",
		Rules: []config.RedirectRewriteRule{
			{Pattern: "http://localhost:8000/", Replacement: "/"},
		},
	}
	rw, err := NewRedirectRewriter(cfg, "/")
	if err != nil {
		t.Fatalf("NewRedirectRewriter() error: %v", err)
	}

	ctx := testutil.NewRequestCtx("GET", "/api/test")

	resp := &fasthttp.Response{}
	resp.Header.Set("Location", "http://localhost:8000/api/users")
	resp.SetStatusCode(301)

	rw.RewriteResponse(resp, ctx, "", "frontend:8080")

	got := string(resp.Header.Peek("Location"))
	want := "/api/users"
	if got != want {
		t.Errorf("Location = %q, want %q", got, want)
	}
}

// TestRedirectRewrite_DefaultMode 测试 default 模式前缀匹配
func TestRedirectRewrite_DefaultMode(t *testing.T) {
	cfg := &config.RedirectRewriteConfig{
		Mode: "default",
	}
	rw, err := NewRedirectRewriter(cfg, "/api/")
	if err != nil {
		t.Fatalf("NewRedirectRewriter() error: %v", err)
	}

	ctx := testutil.NewRequestCtx("GET", "/api/test")

	resp := &fasthttp.Response{}
	resp.Header.Set("Location", "http://backend1:8000/api/v2/users")
	resp.SetStatusCode(301)

	// targetURL = http://backend1:8000, originalClientHost = frontend:8080
	rw.RewriteResponse(resp, ctx, "http://backend1:8000", "frontend:8080")

	got := string(resp.Header.Peek("Location"))
	// default 模式：targetURL 前缀替换为 http://originalClientHost
	want := "http://frontend:8080/api/v2/users"
	if got != want {
		t.Errorf("Location = %q, want %q", got, want)
	}
}

// TestRedirectRewrite_DefaultMode_ExternalService 测试代理外部服务
// 验证 Location 指向外部服务时不应改写
func TestRedirectRewrite_DefaultMode_ExternalService(t *testing.T) {
	cfg := &config.RedirectRewriteConfig{
		Mode: "default",
	}
	rw, err := NewRedirectRewriter(cfg, "/")
	if err != nil {
		t.Fatalf("NewRedirectRewriter() error: %v", err)
	}

	ctx := testutil.NewRequestCtx("GET", "/")

	resp := &fasthttp.Response{}
	// Google 返回的 Location 指向 google.com.hk（不以 targetURL 开头）
	resp.Header.Set("Location", "https://www.google.com.hk/search")
	resp.SetStatusCode(302)

	// targetURL = https://www.google.com, originalClientHost = localhost:8081
	rw.RewriteResponse(resp, ctx, "https://www.google.com", "localhost:8081")

	got := string(resp.Header.Peek("Location"))
	// 不匹配 targetURL 前缀，应该原样返回
	want := "https://www.google.com.hk/search"
	if got != want {
		t.Errorf("Location = %q, want %q", got, want)
	}
}

// TestRedirectRewrite_DefaultMode_MatchingExternal 测试匹配外部服务
// 验证 Location 以 targetURL 开头时正确改写
func TestRedirectRewrite_DefaultMode_MatchingExternal(t *testing.T) {
	cfg := &config.RedirectRewriteConfig{
		Mode: "default",
	}
	rw, err := NewRedirectRewriter(cfg, "/")
	if err != nil {
		t.Fatalf("NewRedirectRewriter() error: %v", err)
	}

	ctx := testutil.NewRequestCtx("GET", "/")

	resp := &fasthttp.Response{}
	// Location 以 targetURL 开头，应该被改写
	resp.Header.Set("Location", "https://www.google.com/search?q=test")
	resp.SetStatusCode(302)

	rw.RewriteResponse(resp, ctx, "https://www.google.com", "localhost:8081")

	got := string(resp.Header.Peek("Location"))
	// targetURL 前缀替换为 http://originalClientHost
	want := "http://localhost:8081/search?q=test"
	if got != want {
		t.Errorf("Location = %q, want %q", got, want)
	}
}

// TestRedirectRewrite_OffMode 测试 off 模式不改写
func TestRedirectRewrite_OffMode(t *testing.T) {
	cfg := &config.RedirectRewriteConfig{
		Mode: "off",
	}
	rw, err := NewRedirectRewriter(cfg, "/")
	if err != nil {
		t.Fatalf("NewRedirectRewriter() error: %v", err)
	}

	ctx := testutil.NewRequestCtx("GET", "/")

	resp := &fasthttp.Response{}
	resp.Header.Set("Location", "http://backend:8000/path")
	resp.SetStatusCode(301)

	rw.RewriteResponse(resp, ctx, "http://backend:8000", "frontend:8080")

	got := string(resp.Header.Peek("Location"))
	want := "http://backend:8000/path"
	if got != want {
		t.Errorf("Location = %q, want %q", got, want)
	}
}

// TestRedirectRewrite_RelativeURL 测试相对 URL 不改写
func TestRedirectRewrite_RelativeURL(t *testing.T) {
	cfg := &config.RedirectRewriteConfig{
		Mode: "default",
	}
	rw, err := NewRedirectRewriter(cfg, "/")
	if err != nil {
		t.Fatalf("NewRedirectRewriter() error: %v", err)
	}

	ctx := testutil.NewRequestCtx("GET", "/")

	resp := &fasthttp.Response{}
	resp.Header.Set("Location", "/new-path")
	resp.SetStatusCode(302)

	rw.RewriteResponse(resp, ctx, "http://backend:8000", "frontend:8080")

	got := string(resp.Header.Peek("Location"))
	want := "/new-path"
	if got != want {
		t.Errorf("Location = %q, want %q", got, want)
	}
}

// TestRedirectRewrite_EmptyLocation 测试空 Location 不改写
func TestRedirectRewrite_EmptyLocation(t *testing.T) {
	cfg := &config.RedirectRewriteConfig{
		Mode: "default",
	}
	rw, err := NewRedirectRewriter(cfg, "/")
	if err != nil {
		t.Fatalf("NewRedirectRewriter() error: %v", err)
	}

	ctx := testutil.NewRequestCtx("GET", "/")

	resp := &fasthttp.Response{}
	resp.Header.Set("Location", "")
	resp.SetStatusCode(302)

	rw.RewriteResponse(resp, ctx, "http://backend:8000", "frontend:8080")

	got := resp.Header.Peek("Location")
	if len(got) != 0 {
		t.Errorf("Location should be empty, got %q", got)
	}
}

// TestRedirectRewrite_NonRedirectStatus 测试非 3xx 状态码不改写 Location
func TestRedirectRewrite_NonRedirectStatus(t *testing.T) {
	cfg := &config.RedirectRewriteConfig{
		Mode: "default",
	}
	rw, err := NewRedirectRewriter(cfg, "/")
	if err != nil {
		t.Fatalf("NewRedirectRewriter() error: %v", err)
	}

	ctx := testutil.NewRequestCtx("GET", "/")

	resp := &fasthttp.Response{}
	resp.Header.Set("Location", "http://backend:8000/path")
	resp.SetStatusCode(200) // 非 3xx

	rw.RewriteResponse(resp, ctx, "http://backend:8000", "frontend:8080")

	got := string(resp.Header.Peek("Location"))
	want := "http://backend:8000/path"
	if got != want {
		t.Errorf("Location = %q, want %q", got, want)
	}
}

// TestRedirectRewrite_RefreshHeader 测试 Refresh 头改写
func TestRedirectRewrite_RefreshHeader(t *testing.T) {
	cfg := &config.RedirectRewriteConfig{
		Mode: "custom",
		Rules: []config.RedirectRewriteRule{
			{Pattern: "http://backend:8000/", Replacement: "http://frontend/"},
		},
	}
	rw, err := NewRedirectRewriter(cfg, "/")
	if err != nil {
		t.Fatalf("NewRedirectRewriter() error: %v", err)
	}

	ctx := testutil.NewRequestCtx("GET", "/")

	resp := &fasthttp.Response{}
	resp.Header.Set("Refresh", "5; url=http://backend:8000/api/")
	resp.SetStatusCode(200)

	rw.RewriteResponse(resp, ctx, "", "frontend:8080")

	got := string(resp.Header.Peek("Refresh"))
	want := "5; url=http://frontend/api/"
	if got != want {
		t.Errorf("Refresh = %q, want %q", got, want)
	}
}

// TestRedirectRewrite_NilConfig 测试 nil 配置默认启用 default 模式
func TestRedirectRewrite_NilConfig(t *testing.T) {
	rw, err := NewRedirectRewriter(nil, "/api/")
	if err != nil {
		t.Fatalf("NewRedirectRewriter() error: %v", err)
	}

	if rw.Mode() != "default" {
		t.Errorf("Mode() = %q, want %q", rw.Mode(), "default")
	}
}

// TestRedirectRewrite_EmptyMode 测试空 Mode 默认为 default
func TestRedirectRewrite_EmptyMode(t *testing.T) {
	cfg := &config.RedirectRewriteConfig{
		Mode: "",
	}
	rw, err := NewRedirectRewriter(cfg, "/")
	if err != nil {
		t.Fatalf("NewRedirectRewriter() error: %v", err)
	}

	if rw.Mode() != "default" {
		t.Errorf("Mode() = %q, want %q", rw.Mode(), "default")
	}
}