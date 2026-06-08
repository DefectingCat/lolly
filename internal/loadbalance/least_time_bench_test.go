package loadbalance

import (
	"testing"
	"time"

	"github.com/valyala/fasthttp"
)

func BenchmarkLeastTime_Select(b *testing.B) {
	lt := NewLeastTime("last_byte", time.Millisecond)
	targets := []*Target{
		NewTargetFromConfig("http://a:8080", 1, 0, 0, 0, false, false, ""),
		NewTargetFromConfig("http://b:8080", 1, 0, 0, 0, false, false, ""),
		NewTargetFromConfig("http://c:8080", 1, 0, 0, 0, false, false, ""),
	}

	// Pre-populate stats
	for _, t := range targets {
		t.Stats.Record(10*time.Millisecond, 20*time.Millisecond)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lt.Select(targets)
	}
}

func BenchmarkLeastTime_Record(b *testing.B) {
	stats := NewEWMAStats()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		stats.Record(10*time.Millisecond, 20*time.Millisecond)
	}
}

func BenchmarkLeastTime_Concurrent(b *testing.B) {
	lt := NewLeastTime("last_byte", time.Millisecond)
	targets := []*Target{
		NewTargetFromConfig("http://a:8080", 1, 0, 0, 0, false, false, ""),
		NewTargetFromConfig("http://b:8080", 1, 0, 0, 0, false, false, ""),
	}

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			lt.Select(targets)
		}
	})
}

func BenchmarkStickySession_Select(b *testing.B) {
	fallback := NewRoundRobin()
	config := DefaultStickyConfig()

	sticky := NewStickySession(config, fallback)
	sticky.Start()
	defer sticky.Stop()

	targets := []*Target{
		NewTargetFromConfig("http://backend1:8080", 1, 0, 0, 0, false, false, ""),
		NewTargetFromConfig("http://backend2:8080", 1, 0, 0, 0, false, false, ""),
	}

	// Pre-populate a cookie
	ctx := &fasthttp.RequestCtx{}
	sticky.Select(ctx, targets)
	cookie := ctx.Response.Header.PeekCookie(config.Name)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.Header.SetCookie(config.Name, string(extractCookieValue(cookie)))
		sticky.Select(ctx, targets)
	}
}

func BenchmarkStickySession_SelectNew(b *testing.B) {
	fallback := NewRoundRobin()
	config := DefaultStickyConfig()

	sticky := NewStickySession(config, fallback)
	sticky.Start()
	defer sticky.Stop()

	targets := []*Target{
		NewTargetFromConfig("http://backend1:8080", 1, 0, 0, 0, false, false, ""),
		NewTargetFromConfig("http://backend2:8080", 1, 0, 0, 0, false, false, ""),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx := &fasthttp.RequestCtx{}
		sticky.Select(ctx, targets)
	}
}
