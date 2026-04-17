package matcher

import (
	"testing"

	"github.com/valyala/fasthttp"
)

func BenchmarkRadixTree_Insert(b *testing.B) {
	tree := NewRadixTree()
	handler := func(ctx *fasthttp.RequestCtx) {}

	paths := []string{
		"/", "/api", "/api/v1", "/api/v2",
		"/static", "/static/css", "/static/js",
		"/user", "/user/profile", "/user/settings",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, p := range paths {
			tree.Insert(p, handler, i)
		}
	}
}

func BenchmarkRadixTree_Find(b *testing.B) {
	tree := NewRadixTree()
	handler := func(ctx *fasthttp.RequestCtx) {}

	paths := []string{"/", "/api", "/api/v1", "/api/v2/users/123"}
	for i, p := range paths {
		tree.Insert(p, handler, i+1)
	}
	tree.MarkInitialized()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tree.FindLongestPrefix("/api/v2/users/123/details")
	}
}

func BenchmarkExactMatcher_Match(b *testing.B) {
	handler := func(ctx *fasthttp.RequestCtx) {}
	m := NewExactMatcher("/api/users", handler, 1)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Match("/api/users")
	}
}

func BenchmarkRegexMatcher_Match(b *testing.B) {
	m := MustRegexMatcher(`^/api/v[0-9]+/users/[0-9]+$`, nil, 3, false)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Match("/api/v1/users/123")
	}
}

func BenchmarkLocationEngine_Match(b *testing.B) {
	engine := NewLocationEngine()
	handler := func(ctx *fasthttp.RequestCtx) {}

	engine.AddExact("/api", handler)
	engine.AddPrefixPriority("/api/", handler)
	engine.AddRegex(`\.php$`, handler, false)
	engine.AddPrefix("/", handler)
	engine.MarkInitialized()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		engine.Match("/api/users/123")
	}
}
