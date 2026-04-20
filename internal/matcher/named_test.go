package matcher

import (
	"testing"

	"github.com/valyala/fasthttp"
)

func TestNamedMatcher_New(t *testing.T) {
	handler := func(ctx *fasthttp.RequestCtx) {}
	m := NewNamedMatcher("error404", handler)

	if m.name != "error404" {
		t.Errorf("expected name 'error404', got %s", m.name)
	}
}

func TestNamedMatcher_Match(t *testing.T) {
	handler := func(ctx *fasthttp.RequestCtx) {}
	m := NewNamedMatcher("error404", handler)

	// Named matchers do not match by path
	if m.Match("/anything") {
		t.Error("named matcher should not match any path")
	}
	if m.Match("") {
		t.Error("named matcher should not match empty path")
	}
}

func TestNamedMatcher_Result(t *testing.T) {
	handler := func(ctx *fasthttp.RequestCtx) {}
	m := NewNamedMatcher("error404", handler)

	result := m.Result()
	if result == nil {
		t.Fatal("result should not be nil")
	}
	if result.Path != "@error404" {
		t.Errorf("expected path '@error404', got %s", result.Path)
	}
	if result.Priority != 0 {
		t.Errorf("expected priority 0, got %d", result.Priority)
	}
	if result.LocationType != LocationTypeNamed {
		t.Errorf("expected location type '%s', got %s", LocationTypeNamed, result.LocationType)
	}
	if result.Handler == nil {
		t.Error("handler should not be nil")
	}
}

func TestNamedMatcher_Name(t *testing.T) {
	handler := func(ctx *fasthttp.RequestCtx) {}

	tests := []struct {
		name     string
		expected string
	}{
		{"error404", "error404"},
		{"default", "default"},
		{"", ""},
		{"error_page", "error_page"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewNamedMatcher(tt.name, handler)
			if m.Name() != tt.expected {
				t.Errorf("expected name %q, got %q", tt.expected, m.Name())
			}
		})
	}
}

func TestNamedMatcher_UnicodeName(t *testing.T) {
	handler := func(ctx *fasthttp.RequestCtx) {}
	m := NewNamedMatcher("错误页面", handler)

	if m.Name() != "错误页面" {
		t.Errorf("expected unicode name preserved, got %s", m.Name())
	}
	result := m.Result()
	if result.Path != "@错误页面" {
		t.Errorf("expected '@错误页面', got %s", result.Path)
	}
}

func TestNamedMatcher_SpecialCharName(t *testing.T) {
	handler := func(ctx *fasthttp.RequestCtx) {}
	m := NewNamedMatcher("error-404_not.found", handler)

	if m.Name() != "error-404_not.found" {
		t.Errorf("special char name should be preserved, got %s", m.Name())
	}
}

func TestLocationEngine_AddNamed(t *testing.T) {
	engine := NewLocationEngine()
	handler := func(ctx *fasthttp.RequestCtx) {}

	err := engine.AddNamed("error404", handler)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	matcher := engine.GetNamed("error404")
	if matcher == nil {
		t.Fatal("named matcher should be retrievable")
	}
	if matcher.Name() != "error404" {
		t.Errorf("expected name 'error404', got %s", matcher.Name())
	}
}

func TestLocationEngine_AddNamed_Duplicate(t *testing.T) {
	engine := NewLocationEngine()
	handler := func(ctx *fasthttp.RequestCtx) {}

	engine.AddNamed("error404", handler)
	err := engine.AddNamed("error404", handler)
	if err == nil {
		t.Fatal("expected duplicate error")
	}
}

func TestLocationEngine_GetNamed_NonExistent(t *testing.T) {
	engine := NewLocationEngine()

	matcher := engine.GetNamed("nonexistent")
	if matcher != nil {
		t.Error("should return nil for non-existent named location")
	}
}
