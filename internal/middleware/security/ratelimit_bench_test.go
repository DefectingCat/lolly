// Package security 提供限流中间件的基准测试。
//
// 该文件测试令牌桶限流器的性能，包括单客户端和多客户端场景。
//
// 作者：xfy
package security

import (
	"net"
	"testing"

	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/config"
)

// setupRateLimitRequestCtx 创建用于基准测试的 fasthttp.RequestCtx。
func setupRateLimitRequestCtx(ip string) *fasthttp.RequestCtx {
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod(fasthttp.MethodGet)
	ctx.Request.SetRequestURI("/api/test")
	ctx.Init(&fasthttp.Request{}, &net.TCPAddr{
		IP:   net.ParseIP(ip),
		Port: 12345,
	}, nil)
	return ctx
}

// BenchmarkRateLimiterAllow 测试单客户端 Allow 性能。
func BenchmarkRateLimiterAllow(b *testing.B) {
	cfg := &config.RateLimitConfig{
		RequestRate: 1000,
		Burst:       2000,
		Key:         "ip",
	}
	rl, _ := NewRateLimiter(cfg)
	defer rl.(*RateLimiter).StopCleanup()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rl.(*RateLimiter).Allow("192.168.1.100")
	}
}

// BenchmarkRateLimiterAllowParallel_10Clients 测试 10 客户端并发 Allow 性能。
func BenchmarkRateLimiterAllowParallel_10Clients(b *testing.B) {
	cfg := &config.RateLimitConfig{
		RequestRate: 10000,
		Burst:       20000,
		Key:         "ip",
	}
	rl, _ := NewRateLimiter(cfg)
	defer rl.(*RateLimiter).StopCleanup()

	clients := make([]string, 10)
	for i := range clients {
		clients[i] = "192.168.1." + string(rune('0'+i))
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			rl.(*RateLimiter).Allow(clients[i%10])
			i++
		}
	})
}

// BenchmarkRateLimiterAllowParallel_100Clients 测试 100 客户端并发 Allow 性能。
func BenchmarkRateLimiterAllowParallel_100Clients(b *testing.B) {
	cfg := &config.RateLimitConfig{
		RequestRate: 100000,
		Burst:       200000,
		Key:         "ip",
	}
	rl, _ := NewRateLimiter(cfg)
	defer rl.(*RateLimiter).StopCleanup()

	clients := make([]string, 100)
	for i := range clients {
		clients[i] = "10.0.0." + string(rune(i))
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			rl.(*RateLimiter).Allow(clients[i%100])
			i++
		}
	})
}

// BenchmarkRateLimiterAllowParallel_1000Clients 测试 1000 客户端并发 Allow 性能。
func BenchmarkRateLimiterAllowParallel_1000Clients(b *testing.B) {
	cfg := &config.RateLimitConfig{
		RequestRate: 1000000,
		Burst:       2000000,
		Key:         "ip",
	}
	rl, _ := NewRateLimiter(cfg)
	defer rl.(*RateLimiter).StopCleanup()

	clients := make([]string, 1000)
	for i := range clients {
		clients[i] = "172.16.0." + string(rune(i%256))
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			rl.(*RateLimiter).Allow(clients[i%1000])
			i++
		}
	})
}

// BenchmarkRateLimiterCleanup_1000Buckets 测试清理 1000 个过期桶的性能。
func BenchmarkRateLimiterCleanup_1000Buckets(b *testing.B) {
	cfg := &config.RateLimitConfig{
		RequestRate: 100,
		Burst:       200,
		Key:         "ip",
	}
	mw, _ := NewRateLimiter(cfg)
	rl := mw.(*RateLimiter)
	defer rl.StopCleanup()

	// 预创建 1000 个桶
	for i := 0; i < 1000; i++ {
		rl.Allow("192.168.0." + string(rune(i)))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rl.Cleanup(0) // 清理所有桶
		// 重新创建桶以保持测试一致性
		for j := 0; j < 1000; j++ {
			rl.Allow("192.168.0." + string(rune(j)))
		}
	}
}

// BenchmarkKeyByIP 测试 IP 提取性能。
func BenchmarkKeyByIP(b *testing.B) {
	ctx := setupRateLimitRequestCtx("192.168.1.100")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		keyByIP(ctx)
	}
}

// BenchmarkRateLimiterMiddleware 组件级测试：测量限流中间件本身的开销。
func BenchmarkRateLimiterMiddleware(b *testing.B) {
	cfg := &config.RateLimitConfig{
		RequestRate: 100000,
		Burst:       200000,
		Key:         "ip",
	}
	rl, _ := NewRateLimiter(cfg)
	defer rl.(*RateLimiter).StopCleanup()

	mockHandler := func(ctx *fasthttp.RequestCtx) {
		ctx.SetStatusCode(fasthttp.StatusOK)
		ctx.SetBodyString("OK")
	}

	handler := rl.Process(mockHandler)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx := setupRateLimitRequestCtx("192.168.1.100")
		handler(ctx)
	}
}

// BenchmarkRateLimiterMiddlewareParallel 测试并发场景下的中间件性能。
func BenchmarkRateLimiterMiddlewareParallel(b *testing.B) {
	cfg := &config.RateLimitConfig{
		RequestRate: 1000000,
		Burst:       2000000,
		Key:         "ip",
	}
	rl, _ := NewRateLimiter(cfg)
	defer rl.(*RateLimiter).StopCleanup()

	mockHandler := func(ctx *fasthttp.RequestCtx) {
		ctx.SetStatusCode(fasthttp.StatusOK)
		ctx.SetBodyString("OK")
	}

	handler := rl.Process(mockHandler)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			ip := "192.168.1." + string(rune('0'+i%10))
			ctx := setupRateLimitRequestCtx(ip)
			handler(ctx)
			i++
		}
	})
}

// BenchmarkRateLimiterStats 测试获取统计信息的性能。
func BenchmarkRateLimiterStats(b *testing.B) {
	cfg := &config.RateLimitConfig{
		RequestRate: 1000,
		Burst:       2000,
		Key:         "ip",
	}
	mw, _ := NewRateLimiter(cfg)
	rl := mw.(*RateLimiter)
	defer rl.StopCleanup()

	// 预创建一些桶
	for i := 0; i < 100; i++ {
		rl.Allow("192.168.0." + string(rune(i)))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rl.GetStats()
	}
}
