package matcher

import (
	"testing"

	"github.com/valyala/fasthttp"
)

func TestLocationEngine_New(t *testing.T) {
	engine := NewLocationEngine()
	if engine.exactMatchers == nil {
		t.Error("exactMatchers should be initialized")
	}
	if engine.prefixPriorityTree == nil {
		t.Error("prefixPriorityTree should be initialized")
	}
	if engine.prefixTree == nil {
		t.Error("prefixTree should be initialized")
	}
	if engine.regexMatchers == nil {
		t.Error("regexMatchers should be initialized")
	}
	if engine.namedMatchers == nil {
		t.Error("namedMatchers should be initialized")
	}
	if engine.registeredPaths == nil {
		t.Error("registeredPaths should be initialized")
	}
}

func TestLocationEngine_AddExact(t *testing.T) {
	engine := NewLocationEngine()
	handler := func(ctx *fasthttp.RequestCtx) {}

	err := engine.AddExact("/api", handler)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result := engine.Match("/api")
	if result == nil {
		t.Fatal("expected match")
	}
	if result.Path != "/api" {
		t.Errorf("expected path '/api', got %s", result.Path)
	}
	if result.LocationType != LocationTypeExact {
		t.Errorf("expected location type '%s', got %s", LocationTypeExact, result.LocationType)
	}
}

func TestLocationEngine_AddExact_AfterInitialized(t *testing.T) {
	engine := NewLocationEngine()
	handler := func(ctx *fasthttp.RequestCtx) {}

	engine.MarkInitialized()
	err := engine.AddExact("/api", handler)
	if err == nil {
		t.Error("expected error after initialized")
	}
}

func TestLocationEngine_AddExact_PathConflict(t *testing.T) {
	engine := NewLocationEngine()
	handler := func(ctx *fasthttp.RequestCtx) {}

	engine.AddExact("/api", handler)
	err := engine.AddExact("/api", handler)
	if err == nil {
		t.Error("expected conflict error")
	}
}

func TestLocationEngine_AddPrefixPriority(t *testing.T) {
	engine := NewLocationEngine()
	handler := func(ctx *fasthttp.RequestCtx) {}

	err := engine.AddPrefixPriority("/static", handler)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result := engine.Match("/static/css/style.css")
	if result == nil {
		t.Fatal("expected match")
	}
	if result.Path != "/static" {
		t.Errorf("expected path '/static', got %s", result.Path)
	}
	if result.LocationType != LocationTypePrefixPriority {
		t.Errorf("expected location type '%s', got %s", LocationTypePrefixPriority, result.LocationType)
	}
}

func TestLocationEngine_AddPrefixPriority_AfterInitialized(t *testing.T) {
	engine := NewLocationEngine()
	handler := func(ctx *fasthttp.RequestCtx) {}

	engine.MarkInitialized()
	err := engine.AddPrefixPriority("/static", handler)
	if err == nil {
		t.Error("expected error after initialized")
	}
}

func TestLocationEngine_AddPrefix(t *testing.T) {
	engine := NewLocationEngine()
	handler := func(ctx *fasthttp.RequestCtx) {}

	err := engine.AddPrefix("/api", handler)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result := engine.Match("/api/users")
	if result == nil {
		t.Fatal("expected match")
	}
	if result.LocationType != LocationTypePrefix {
		t.Errorf("expected location type '%s', got %s", LocationTypePrefix, result.LocationType)
	}
}

func TestLocationEngine_AddPrefix_AfterInitialized(t *testing.T) {
	engine := NewLocationEngine()
	handler := func(ctx *fasthttp.RequestCtx) {}

	engine.MarkInitialized()
	err := engine.AddPrefix("/api", handler)
	if err == nil {
		t.Error("expected error after initialized")
	}
}

func TestLocationEngine_AddRegex(t *testing.T) {
	engine := NewLocationEngine()
	handler := func(ctx *fasthttp.RequestCtx) {}

	err := engine.AddRegex(`\.php$`, handler, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result := engine.Match("/index.php")
	if result == nil {
		t.Fatal("expected match")
	}
	if result.LocationType != LocationTypeRegex {
		t.Errorf("expected location type '%s', got %s", LocationTypeRegex, result.LocationType)
	}
}

func TestLocationEngine_AddRegex_CaseInsensitive(t *testing.T) {
	engine := NewLocationEngine()
	handler := func(ctx *fasthttp.RequestCtx) {}

	err := engine.AddRegex(`(?i)\.php$`, handler, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result := engine.Match("/index.PHP")
	if result == nil {
		t.Fatal("expected match for case insensitive")
	}
	if result.LocationType != LocationTypeRegexCaseless {
		t.Errorf("expected location type '%s', got %s", LocationTypeRegexCaseless, result.LocationType)
	}
}

func TestLocationEngine_AddRegex_InvalidPattern(t *testing.T) {
	engine := NewLocationEngine()
	handler := func(ctx *fasthttp.RequestCtx) {}

	err := engine.AddRegex(`[invalid`, handler, false)
	if err == nil {
		t.Error("expected error for invalid regex")
	}
}

func TestLocationEngine_AddRegex_Captures(t *testing.T) {
	engine := NewLocationEngine()
	handler := func(ctx *fasthttp.RequestCtx) {}

	err := engine.AddRegex(`^/user/(?P<id>[0-9]+)$`, handler, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result := engine.Match("/user/123")
	if result == nil {
		t.Fatal("expected match")
	}
	if result.Captures == nil {
		t.Fatal("expected captures")
	}
	if result.Captures["id"] != "123" {
		t.Errorf("expected id=123, got %s", result.Captures["id"])
	}
}

func TestLocationEngine_Match_PriorityOrder(t *testing.T) {
	engine := NewLocationEngine()
	hExact := func(ctx *fasthttp.RequestCtx) {}
	hPrefixPriority := func(ctx *fasthttp.RequestCtx) {}
	hRegex := func(ctx *fasthttp.RequestCtx) {}
	hPrefix := func(ctx *fasthttp.RequestCtx) {}

	// All match "/api/path"
	engine.AddExact("/api/path", hExact)
	engine.AddPrefixPriority("/api", hPrefixPriority)
	engine.AddRegex(`^/api/`, hRegex, false)
	engine.AddPrefix("/api/path", hPrefix)

	// Exact should win (priority 1)
	result := engine.Match("/api/path")
	if result == nil {
		t.Fatal("expected match")
	}
	if result.LocationType != LocationTypeExact {
		t.Errorf("expected exact match to win, got %s", result.LocationType)
	}
}

func TestLocationEngine_Match_PrefixPriorityBeatsRegex(t *testing.T) {
	engine := NewLocationEngine()
	hPrefixPriority := func(ctx *fasthttp.RequestCtx) {}
	hRegex := func(ctx *fasthttp.RequestCtx) {}

	// No exact match for this path
	engine.AddPrefixPriority("/static", hPrefixPriority)
	engine.AddRegex(`\.css$`, hRegex, false)

	// ^~ prefix priority should beat regex
	result := engine.Match("/static/style.css")
	if result == nil {
		t.Fatal("expected match")
	}
	if result.LocationType != LocationTypePrefixPriority {
		t.Errorf("expected prefix_priority to win over regex, got %s", result.LocationType)
	}
}

func TestLocationEngine_Match_RegexBeatsPrefix(t *testing.T) {
	engine := NewLocationEngine()
	hRegex := func(ctx *fasthttp.RequestCtx) {}
	hPrefix := func(ctx *fasthttp.RequestCtx) {}

	engine.AddRegex(`\.php$`, hRegex, false)
	engine.AddPrefix("/", hPrefix)

	// Regex should win over plain prefix
	result := engine.Match("/index.php")
	if result == nil {
		t.Fatal("expected match")
	}
	if result.LocationType != LocationTypeRegex {
		t.Errorf("expected regex to win over prefix, got %s", result.LocationType)
	}
}

func TestLocationEngine_Match_FallbackToPrefix(t *testing.T) {
	engine := NewLocationEngine()
	hPrefix := func(ctx *fasthttp.RequestCtx) {}

	engine.AddPrefix("/api", hPrefix)

	result := engine.Match("/api/users")
	if result == nil {
		t.Fatal("expected prefix match")
	}
	if result.LocationType != LocationTypePrefix {
		t.Errorf("expected prefix match, got %s", result.LocationType)
	}
}

func TestLocationEngine_Match_NoMatch(t *testing.T) {
	engine := NewLocationEngine()
	hPrefix := func(ctx *fasthttp.RequestCtx) {}

	engine.AddPrefix("/api", hPrefix)

	result := engine.Match("/other")
	if result != nil {
		t.Errorf("expected no match, got %+v", result)
	}
}

func TestLocationEngine_Match_EmptyString(t *testing.T) {
	engine := NewLocationEngine()
	hPrefix := func(ctx *fasthttp.RequestCtx) {}

	engine.AddPrefix("/api", hPrefix)

	result := engine.Match("")
	if result != nil {
		t.Errorf("expected no match for empty string, got %+v", result)
	}
}

func TestLocationEngine_Match_UnicodePath(t *testing.T) {
	engine := NewLocationEngine()
	handler := func(ctx *fasthttp.RequestCtx) {}

	engine.AddPrefixPriority("/文档", handler)

	result := engine.Match("/文档/报告")
	if result == nil {
		t.Fatal("expected unicode prefix match")
	}
	if result.Path != "/文档" {
		t.Errorf("expected '/文档', got %s", result.Path)
	}
}

func TestLocationEngine_MarkInitialized(t *testing.T) {
	engine := NewLocationEngine()
	handler := func(ctx *fasthttp.RequestCtx) {}

	engine.AddPrefix("/api", handler)
	engine.MarkInitialized()

	// All add methods should fail after initialized
	if engine.AddExact("/exact", handler) == nil {
		t.Error("AddExact should fail after initialized")
	}
	if engine.AddPrefixPriority("/pp", handler) == nil {
		t.Error("AddPrefixPriority should fail after initialized")
	}
	if engine.AddPrefix("/pre", handler) == nil {
		t.Error("AddPrefix should fail after initialized")
	}
	if engine.AddRegex(`test`, handler, false) == nil {
		t.Error("AddRegex should fail after initialized")
	}
	if engine.AddNamed("test", handler) == nil {
		t.Error("AddNamed should fail after initialized")
	}
}

func TestParseRegexPattern(t *testing.T) {
	tests := []struct {
		input        string
		wantPattern  string
		wantCaseless bool
		wantIsRegex  bool
	}{
		{"", "", false, false},
		{"/api", "/api", false, false},
		{"~\\.php$", "\\.php$", true, true},
		{"^~", "", false, true},
		{"^~/static", "/static", false, true},
		{"~*.php$", "*.php$", true, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			pattern, caseless, isRegex := ParseRegexPattern(tt.input)
			if pattern != tt.wantPattern {
				t.Errorf("pattern: expected %q, got %q", tt.wantPattern, pattern)
			}
			if caseless != tt.wantCaseless {
				t.Errorf("caseless: expected %v, got %v", tt.wantCaseless, caseless)
			}
			if isRegex != tt.wantIsRegex {
				t.Errorf("isRegex: expected %v, got %v", tt.wantIsRegex, isRegex)
			}
		})
	}
}

func TestMustCompileRegex(t *testing.T) {
	re := MustCompileRegex(`^/api`)
	if re == nil {
		t.Error("expected compiled regex")
	}

	re = MustCompileRegex(`[invalid`)
	if re != nil {
		t.Error("expected nil for invalid regex")
	}
}
