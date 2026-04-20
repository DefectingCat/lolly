package matcher

import (
	"testing"

	"github.com/valyala/fasthttp"
)

func TestPrefixPriorityMatcher_New(t *testing.T) {
	ppm := NewPrefixPriorityMatcher()
	if ppm.tree == nil {
		t.Fatal("tree should be initialized")
	}
	if ppm.priority != 2 {
		t.Errorf("expected priority 2, got %d", ppm.priority)
	}
}

func TestPrefixPriorityMatcher_AddPath(t *testing.T) {
	ppm := NewPrefixPriorityMatcher()
	handler := func(ctx *fasthttp.RequestCtx) {}

	err := ppm.AddPath("/static", handler, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result := ppm.Match("/static/css/style.css")
	if result == nil {
		t.Error("should match prefix priority")
	}
}

func TestPrefixPriorityMatcher_Match(t *testing.T) {
	ppm := NewPrefixPriorityMatcher()
	handler := func(ctx *fasthttp.RequestCtx) {}

	ppm.AddPath("/static", handler, false)
	ppm.AddPath("/static/images", handler, false)

	tests := []struct {
		path    string
		wantNil bool
	}{
		{"/static", false},
		{"/static/css/main.css", false},
		{"/static/images/logo.png", false},
		{"/dynamic", true},
		{"/", true},
		{"", true},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := ppm.Match(tt.path)
			if tt.wantNil && result != nil {
				t.Errorf("expected nil for path %q", tt.path)
			}
			if !tt.wantNil && result == nil {
				t.Errorf("expected match for path %q", tt.path)
			}
		})
	}
}

func TestPrefixPriorityMatcher_Priority(t *testing.T) {
	ppm := NewPrefixPriorityMatcher()
	h1 := func(ctx *fasthttp.RequestCtx) {}
	h2 := func(ctx *fasthttp.RequestCtx) {}

	ppm.AddPath("/api/v1", h1, false)
	ppm.AddPath("/api/v2", h2, false)

	result := ppm.Match("/api/v2/data")
	if result == nil {
		t.Fatal("expected match")
	}
	// All entries have priority 2, longest matching prefix wins
	if result.Path != "/api/v2" {
		t.Errorf("expected '/api/v2', got %s", result.Path)
	}
	if result.Priority != 2 {
		t.Errorf("expected priority 2, got %d", result.Priority)
	}
}

func TestPrefixPriorityMatcher_Match_EmptyString(t *testing.T) {
	ppm := NewPrefixPriorityMatcher()
	handler := func(ctx *fasthttp.RequestCtx) {}

	ppm.AddPath("/", handler, false)
	result := ppm.Match("")
	if result != nil {
		t.Error("empty string should not match '/' prefix")
	}
}

func TestPrefixPriorityMatcher_Match_UnicodePath(t *testing.T) {
	ppm := NewPrefixPriorityMatcher()
	handler := func(ctx *fasthttp.RequestCtx) {}

	ppm.AddPath("/文档", handler, false)

	result := ppm.Match("/文档/报告")
	if result == nil {
		t.Error("should match unicode prefix")
	}
}

func TestPrefixPriorityMatcher_MarkInitialized(t *testing.T) {
	ppm := NewPrefixPriorityMatcher()
	handler := func(ctx *fasthttp.RequestCtx) {}

	ppm.AddPath("/static", handler, false)
	ppm.MarkInitialized()

	err := ppm.AddPath("/static/v2", handler, false)
	if err == nil {
		t.Error("should fail after initialized")
	}
}

func TestPrefixPriorityMatcher_AddPath_Duplicate(t *testing.T) {
	ppm := NewPrefixPriorityMatcher()
	handler := func(ctx *fasthttp.RequestCtx) {}

	ppm.AddPath("/static", handler, false)
	err := ppm.AddPath("/static", handler, false)
	if err == nil {
		t.Error("should fail on duplicate path")
	}
}

func TestPrefixPriorityMatcher_Result_LocationType(t *testing.T) {
	ppm := NewPrefixPriorityMatcher()
	handler := func(ctx *fasthttp.RequestCtx) {}

	ppm.AddPath("/static", handler, false)

	result := ppm.Match("/static/file.txt")
	if result == nil {
		t.Fatal("expected match")
	}
	if result.LocationType != "prefix_priority" {
		t.Errorf("expected location type 'prefix_priority', got %s", result.LocationType)
	}
}
