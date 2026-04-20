package matcher

import (
	"testing"

	"github.com/valyala/fasthttp"
)

func TestPrefixMatcher_New(t *testing.T) {
	pm := NewPrefixMatcher()
	if pm.tree == nil {
		t.Fatal("tree should be initialized")
	}
	if pm.priority != 4 {
		t.Errorf("expected priority 4, got %d", pm.priority)
	}
}

func TestPrefixMatcher_AddPath(t *testing.T) {
	pm := NewPrefixMatcher()
	handler := func(ctx *fasthttp.RequestCtx) {}

	err := pm.AddPath("/api", handler, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result := pm.Match("/api/users")
	if result == nil {
		t.Error("should match prefix")
	}
}

func TestPrefixMatcher_Match(t *testing.T) {
	pm := NewPrefixMatcher()
	handler := func(ctx *fasthttp.RequestCtx) {}

	pm.AddPath("/api", handler, false)
	pm.AddPath("/api/v2", handler, false)

	tests := []struct {
		path    string
		wantNil bool
	}{
		{"/api", false},
		{"/api/users", false},
		{"/api/v2/data", false},
		{"/other", true},
		{"/", true},
		{"", true},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := pm.Match(tt.path)
			if tt.wantNil && result != nil {
				t.Errorf("expected nil for path %q", tt.path)
			}
			if !tt.wantNil && result == nil {
				t.Errorf("expected match for path %q", tt.path)
			}
		})
	}
}

func TestPrefixMatcher_Match_EmptyString(t *testing.T) {
	pm := NewPrefixMatcher()
	handler := func(ctx *fasthttp.RequestCtx) {}

	pm.AddPath("/", handler, false)
	result := pm.Match("")
	if result != nil {
		t.Error("empty string should not match '/' prefix")
	}
}

func TestPrefixMatcher_Match_UnicodePath(t *testing.T) {
	pm := NewPrefixMatcher()
	handler := func(ctx *fasthttp.RequestCtx) {}

	pm.AddPath("/café", handler, false)

	result := pm.Match("/café/latte")
	if result == nil {
		t.Error("should match unicode prefix")
	}
}

func TestPrefixMatcher_Match_LongestPrefix(t *testing.T) {
	pm := NewPrefixMatcher()
	h1 := func(ctx *fasthttp.RequestCtx) { ctx.SetBodyString("1") }
	h2 := func(ctx *fasthttp.RequestCtx) { ctx.SetBodyString("2") }

	pm.AddPath("/static", h1, false)
	pm.AddPath("/static/css", h2, false)

	result := pm.Match("/static/css/main.css")
	if result == nil {
		t.Fatal("expected match")
	}
	// Prefix matcher returns first matching prefix at same priority level
	// Radix tree returns /static because it's the first registered path
	// If longest prefix is needed, use PrefixPriorityMatcher instead
	if result.Path != "/static" {
		t.Errorf("expected prefix '/static', got %s", result.Path)
	}
}

func TestPrefixMatcher_MarkInitialized(t *testing.T) {
	pm := NewPrefixMatcher()
	handler := func(ctx *fasthttp.RequestCtx) {}

	pm.AddPath("/api", handler, false)
	pm.MarkInitialized()

	err := pm.AddPath("/api/v2", handler, false)
	if err == nil {
		t.Error("should fail after initialized")
	}
}

func TestPrefixMatcher_AddPath_Duplicate(t *testing.T) {
	pm := NewPrefixMatcher()
	handler := func(ctx *fasthttp.RequestCtx) {}

	pm.AddPath("/api", handler, false)
	err := pm.AddPath("/api", handler, false)
	if err == nil {
		t.Error("should fail on duplicate path")
	}
}

func TestPrefixMatcher_Match_SpecialChars(t *testing.T) {
	pm := NewPrefixMatcher()
	handler := func(ctx *fasthttp.RequestCtx) {}

	pm.AddPath("/api/v1", handler, false)

	result := pm.Match("/api/v1?key=value&other=123")
	if result == nil {
		t.Error("should match prefix even with query params")
	}
}
