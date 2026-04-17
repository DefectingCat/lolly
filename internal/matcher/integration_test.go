package matcher

import (
	"testing"

	"github.com/valyala/fasthttp"
)

// nginx priority: exact(=) > prefix_priority(^~) > regex(~) > prefix
func TestLocationEngine_NginxPriority(t *testing.T) {
	engine := NewLocationEngine()
	handler := func(ctx *fasthttp.RequestCtx) {}

	// 注册不同类型
	engine.AddExact("/api", handler)           // priority 1
	engine.AddPrefixPriority("/api/", handler) // priority 2 (^~)
	engine.AddRegex(`\.php$`, handler, false)  // priority 3
	engine.AddPrefix("/", handler)             // priority 4
	engine.MarkInitialized()

	// 测试精确匹配优先
	result := engine.Match("/api")
	if result.LocationType != "exact" {
		t.Errorf("expected exact, got %s", result.LocationType)
	}

	// 测试 ^~ 阻止正则
	result = engine.Match("/api/test.php")
	if result.LocationType != "prefix_priority" {
		t.Errorf("^~ should block regex, got %s", result.LocationType)
	}
}

func TestLocationEngine_RegexMatch(t *testing.T) {
	engine := NewLocationEngine()
	handler := func(ctx *fasthttp.RequestCtx) {}

	engine.AddPrefixPriority("/api/", handler)
	engine.AddRegex(`\.php$`, handler, false)
	engine.AddPrefix("/", handler)
	engine.MarkInitialized()

	// 正则匹配（^~ 不匹配 /index.php）
	result := engine.Match("/index.php")
	if result.LocationType != "regex" {
		t.Errorf("expected regex for /index.php, got %s", result.LocationType)
	}
}

func TestLocationEngine_PrefixFallback(t *testing.T) {
	engine := NewLocationEngine()
	handler := func(ctx *fasthttp.RequestCtx) {}

	engine.AddPrefix("/", handler)
	engine.MarkInitialized()

	result := engine.Match("/any/path")
	if result == nil || result.LocationType != "prefix" {
		t.Errorf("expected prefix match, got %v", result)
	}
}

func TestLocationEngine_NoMatch(t *testing.T) {
	engine := NewLocationEngine()
	engine.MarkInitialized()

	result := engine.Match("/nonexistent")
	if result != nil {
		t.Errorf("expected nil for no match, got %+v", result)
	}
}

func TestLocationEngine_RegexCaptures(t *testing.T) {
	engine := NewLocationEngine()
	handler := func(ctx *fasthttp.RequestCtx) {}

	engine.AddRegex(`^/user/(?P<id>[0-9]+)$`, handler, false)
	engine.MarkInitialized()

	result := engine.Match("/user/42")
	if result.LocationType != "regex" {
		t.Errorf("expected regex, got %s", result.LocationType)
	}
	if result.Captures == nil || result.Captures["id"] != "42" {
		t.Errorf("expected captures id=42, got %v", result.Captures)
	}
}

func TestLocationEngine_Initialized_Twice(t *testing.T) {
	engine := NewLocationEngine()
	handler := func(ctx *fasthttp.RequestCtx) {}

	engine.MarkInitialized()

	err := engine.AddExact("/api", handler)
	if err == nil {
		t.Error("should fail when adding after initialized")
	}
}

func TestLocationEngine_PathConflict(t *testing.T) {
	engine := NewLocationEngine()
	handler := func(ctx *fasthttp.RequestCtx) {}

	engine.AddExact("/api", handler)
	err := engine.AddExact("/api", handler)
	if err == nil {
		t.Error("should fail on path conflict")
	}
}
