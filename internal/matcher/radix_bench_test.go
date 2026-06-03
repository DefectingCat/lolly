package matcher

import (
	"testing"

	"github.com/valyala/fasthttp"
)

func BenchmarkRadixTreeFindLongestPrefix(b *testing.B) {
	tree := NewRadixTree()
	paths := []string{"/", "/api", "/api/v1", "/api/v1/users", "/static", "/static/css", "/static/js", "/health", "/favicon.ico"}
	dummyHandler := func(ctx *fasthttp.RequestCtx) {}
	for _, p := range paths {
		tree.Insert(p, dummyHandler, 0, LocationTypePrefix, false)
	}
	tree.MarkInitialized()

	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		result := tree.FindLongestPrefix("/api/v1/users")
		ReleaseMatchResult(result)
	}
}

func BenchmarkRadixTreeFindLongestPrefixParallel(b *testing.B) {
	tree := NewRadixTree()
	paths := []string{"/", "/api", "/api/v1", "/api/v1/users", "/static", "/static/css", "/static/js", "/health", "/favicon.ico"}
	dummyHandler := func(ctx *fasthttp.RequestCtx) {}
	for _, p := range paths {
		tree.Insert(p, dummyHandler, 0, LocationTypePrefix, false)
	}
	tree.MarkInitialized()

	b.ResetTimer()
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			result := tree.FindLongestPrefix("/api/v1/users")
			ReleaseMatchResult(result)
		}
	})
}
