package integration

import (
	"testing"

	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/matcher"
)

func TestRegexConfigCaseSensitive(t *testing.T) {
	// 测试 ~ 修饰符（case-sensitive）
	// 创建 regex matcher，验证只匹配小写
	m, err := matcher.NewRegexMatcher(`\.php$`, nil, 3, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if !m.Match("/test.php") {
		t.Error("~ modifier should match lowercase .php")
	}
	if m.Match("/test.PHP") {
		t.Error("~ modifier should NOT match uppercase .PHP")
	}
}

func TestRegexConfigCaseInsensitive(t *testing.T) {
	// 测试 ~* 修饰符（case-insensitive）
	m, err := matcher.NewRegexMatcher(`(?i)\.php$`, nil, 3, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if !m.Match("/test.php") {
		t.Error("~* modifier should match lowercase .php")
	}
	if !m.Match("/test.PHP") {
		t.Error("~* modifier should match uppercase .PHP")
	}
}

func TestPrefixPriorityNotRegex(t *testing.T) {
	// 测试 ^~ 修饰符（非正则）
	// 需要提供非 nil handler，否则 RadixTree 不会存储该节点
	dummyHandler := func(ctx *fasthttp.RequestCtx) {}

	engine := matcher.NewLocationEngine()
	err := engine.AddPrefixPriority("/images", dummyHandler, false)
	if err != nil {
		t.Fatal(err)
	}

	result := engine.Match("/images/logo.png")
	if result == nil {
		t.Error("^~ should match prefix")
	}
	if result.LocationType == matcher.LocationTypeRegex || result.LocationType == matcher.LocationTypeRegexCaseless {
		t.Error("^~ should NOT be treated as regex")
	}
	if result.LocationType != matcher.LocationTypePrefixPriority {
		t.Errorf("^~ should have LocationTypePrefixPriority, got: %s", result.LocationType)
	}
}
