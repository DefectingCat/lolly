package matcher

import (
	"testing"

	"github.com/valyala/fasthttp"
)

func TestExactMatcher_Match(t *testing.T) {
	handler := func(ctx *fasthttp.RequestCtx) {}
	m := NewExactMatcher("/api", handler, 1, false)

	if !m.Match("/api") {
		t.Error("should match exact path")
	}
	if m.Match("/api/users") {
		t.Error("should not match different path")
	}
}

func TestRegexMatcher_Match(t *testing.T) {
	m := MustRegexMatcher(`\.php$`, nil, 3, false, false)

	if !m.Match("/index.php") {
		t.Error("should match .php")
	}
	if m.Match("/index.html") {
		t.Error("should not match .html")
	}
}

func TestRegexMatcher_GetCaptures(t *testing.T) {
	m := MustRegexMatcher(`^/user/(?P<id>[0-9]+)$`, nil, 3, false, false)

	captures := m.GetCaptures("/user/123")
	if captures["id"] != "123" {
		t.Errorf("expected id=123, got %s", captures["id"])
	}
}

func TestRegexMatcher_GetCaptures_NoMatch(t *testing.T) {
	m := MustRegexMatcher(`^/user/(?P<id>[0-9]+)$`, nil, 3, false, false)

	captures := m.GetCaptures("/user/abc")
	if captures != nil {
		t.Errorf("expected nil captures for non-matching path, got %v", captures)
	}
}

func TestRegexMatcher_CaseInsensitive(t *testing.T) {
	// caseInsensitive flag only affects Result().LocationType, not matching
	m := MustRegexMatcher(`\.php$`, nil, 3, true, false)

	if !m.Match("/index.php") {
		t.Error("should match .php")
	}
	// Go regexp is case-sensitive by default; flag is metadata only
	if m.Match("/index.PHP") {
		t.Error("case insensitive flag is metadata only, should not match .PHP")
	}

	result := m.Result()
	if result.LocationType != "regex_caseless" {
		t.Errorf("expected regex_caseless, got %s", result.LocationType)
	}
}

func TestRegexMatcher_Result_LocationType(t *testing.T) {
	// Case sensitive
	m := MustRegexMatcher(`\.php$`, nil, 3, false, false)
	result := m.Result()
	if result.LocationType != "regex" {
		t.Errorf("expected location type 'regex', got %s", result.LocationType)
	}

	// Case insensitive
	m2 := MustRegexMatcher(`\.php$`, nil, 3, true, false)
	result2 := m2.Result()
	if result2.LocationType != "regex_caseless" {
		t.Errorf("expected location type 'regex_caseless', got %s", result2.LocationType)
	}
}

func TestNewRegexMatcher_InvalidPattern(t *testing.T) {
	_, err := NewRegexMatcher(`[invalid`, nil, 3, false, false)
	if err == nil {
		t.Error("expected error for invalid regex pattern")
	}
}
