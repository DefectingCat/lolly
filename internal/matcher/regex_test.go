package matcher

import (
	"testing"

	"github.com/valyala/fasthttp"
)

func TestRegexMatcher_New(t *testing.T) {
	handler := func(ctx *fasthttp.RequestCtx) {}
	m, err := NewRegexMatcher(`^/api/`, handler, 3, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.pattern == nil {
		t.Fatal("pattern should be compiled")
	}
	if m.priority != 3 {
		t.Errorf("expected priority 3, got %d", m.priority)
	}
	if m.caseInsensitive {
		t.Error("expected caseSensitive")
	}
}

func TestRegexMatcher_New_InvalidPattern(t *testing.T) {
	_, err := NewRegexMatcher(`[invalid`, nil, 3, false)
	if err == nil {
		t.Fatal("expected error for invalid regex")
	}
}

func TestRegexMatcher_Match_Paths(t *testing.T) {
	handler := func(ctx *fasthttp.RequestCtx) {}
	m := MustRegexMatcher(`^/api/`, handler, 3, false)

	tests := []struct {
		path  string
		match bool
	}{
		{"/api/users", true},
		{"/api/v1/data", true},
		{"/other", false},
		{"/API/users", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := m.Match(tt.path)
			if result != tt.match {
				t.Errorf("path %q: expected %v, got %v", tt.path, tt.match, result)
			}
		})
	}
}

func TestRegexMatcher_Match_CaseInsensitive(t *testing.T) {
	handler := func(ctx *fasthttp.RequestCtx) {}
	m := MustRegexMatcher(`(?i)^/api/`, handler, 3, true)

	if !m.Match("/api/users") {
		t.Error("should match lowercase")
	}
	if !m.Match("/API/users") {
		t.Error("should match uppercase with (?i) flag")
	}
	if !m.Match("/Api/Users") {
		t.Error("should match mixed case with (?i) flag")
	}
}

func TestRegexMatcher_Result(t *testing.T) {
	handler := func(ctx *fasthttp.RequestCtx) {}
	m := MustRegexMatcher(`\.php$`, handler, 3, false)

	result := m.Result()
	if result == nil {
		t.Fatal("result should not be nil")
	}
	if result.Path != `\.php$` {
		t.Errorf("expected path '\\.php$', got %s", result.Path)
	}
	if result.Priority != 3 {
		t.Errorf("expected priority 3, got %d", result.Priority)
	}
	if result.LocationType != LocationTypeRegex {
		t.Errorf("expected location type '%s', got %s", LocationTypeRegex, result.LocationType)
	}
}

func TestRegexMatcher_Result_Caseless(t *testing.T) {
	handler := func(ctx *fasthttp.RequestCtx) {}
	m := MustRegexMatcher(`\.php$`, handler, 3, true)

	result := m.Result()
	if result.LocationType != LocationTypeRegexCaseless {
		t.Errorf("expected location type '%s', got %s", LocationTypeRegexCaseless, result.LocationType)
	}
}

func TestRegexMatcher_GetCaptures_NamedGroups(t *testing.T) {
	handler := func(ctx *fasthttp.RequestCtx) {}
	m := MustRegexMatcher(`^/user/(?P<id>[0-9]+)/post/(?P<post>[a-z]+)$`, handler, 3, false)

	captures := m.GetCaptures("/user/42/post/hello")
	if captures == nil {
		t.Fatal("expected captures")
	}
	if captures["id"] != "42" {
		t.Errorf("expected id=42, got %s", captures["id"])
	}
	if captures["post"] != "hello" {
		t.Errorf("expected post=hello, got %s", captures["post"])
	}
}

func TestRegexMatcher_GetCaptures_NoMatchPath(t *testing.T) {
	handler := func(ctx *fasthttp.RequestCtx) {}
	m := MustRegexMatcher(`^/user/(?P<id>[0-9]+)$`, handler, 3, false)

	captures := m.GetCaptures("/user/abc")
	if captures != nil {
		t.Errorf("expected nil captures, got %v", captures)
	}
}

func TestRegexMatcher_GetCaptures_NoNamedGroups(t *testing.T) {
	handler := func(ctx *fasthttp.RequestCtx) {}
	m := MustRegexMatcher(`^/user/[0-9]+$`, handler, 3, false)

	// No named groups, should return empty map
	captures := m.GetCaptures("/user/123")
	if captures == nil {
		t.Fatal("expected empty map, not nil")
	}
	if len(captures) != 0 {
		t.Errorf("expected empty captures, got %v", captures)
	}
}

func TestRegexMatcher_Match_UnicodePath(t *testing.T) {
	handler := func(ctx *fasthttp.RequestCtx) {}
	m := MustRegexMatcher(`^/文档/`, handler, 3, false)

	if !m.Match("/文档/报告") {
		t.Error("should match unicode path")
	}
	if m.Match("/文档") {
		t.Error("should not match partial path without trailing slash")
	}
}

func TestRegexMatcher_Match_SpecialChars(t *testing.T) {
	handler := func(ctx *fasthttp.RequestCtx) {}
	m := MustRegexMatcher(`^/path\?query=`, handler, 3, false)

	if !m.Match("/path?query=test") {
		t.Error("should match path with query string")
	}
}

func TestRegexMatcher_Match_EmptyPath(t *testing.T) {
	handler := func(ctx *fasthttp.RequestCtx) {}
	m := MustRegexMatcher(`^$`, handler, 3, false)

	if !m.Match("") {
		t.Error("should match empty string with ^$ pattern")
	}
}

func TestMustRegexMatcher_Panic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for invalid regex")
		}
	}()
	MustRegexMatcher(`[invalid`, nil, 3, false)
}
