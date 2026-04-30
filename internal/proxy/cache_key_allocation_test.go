// Package proxy 提供代理缓存键零分配验证测试。
//
// 该文件验证 buildCacheKeyHashValue 的零分配优化效果。
//
// 测试场景：
//   - buildCacheKeyHashValue: 直接哈希，目标 0 allocs/op
//   - buildCacheKeyHash: 字符串构建，对比基准
//
// 作者：xfy
package proxy

import (
	"testing"

	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/config"
	"rua.plus/lolly/internal/loadbalance"
)

// BenchmarkCacheKeyHashValue_ZeroAlloc 验证零分配路径。
//
// buildCacheKeyHashValue 直接写入哈希，不分配字符串。
func BenchmarkCacheKeyHashValue_ZeroAlloc(b *testing.B) {
	p, err := NewProxy(&config.ProxyConfig{
		Path:        "/api",
		LoadBalance: "round_robin",
		Timeout: config.ProxyTimeout{
			Connect: 5 * 1e9,
			Read:    30 * 1e9,
			Write:   30 * 1e9,
		},
	}, []*loadbalance.Target{{URL: "http://localhost:8080"}}, nil, nil)
	if err != nil {
		b.Fatalf("NewProxy() error: %v", err)
	}

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod(fasthttp.MethodGet)
	ctx.Request.SetRequestURI("/api/test?query=value&foo=bar")

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		hash := p.buildCacheKeyHashValue(ctx)
		_ = hash
	}
}

// BenchmarkCacheKeyHash_WithAlloc 对比带分配的字符串构建路径。
//
// buildCacheKeyHash 分配字符串用于 origKey 返回值。
func BenchmarkCacheKeyHash_WithAlloc(b *testing.B) {
	p, err := NewProxy(&config.ProxyConfig{
		Path:        "/api",
		LoadBalance: "round_robin",
		Timeout: config.ProxyTimeout{
			Connect: 5 * 1e9,
			Read:    30 * 1e9,
			Write:   30 * 1e9,
		},
	}, []*loadbalance.Target{{URL: "http://localhost:8080"}}, nil, nil)
	if err != nil {
		b.Fatalf("NewProxy() error: %v", err)
	}

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod(fasthttp.MethodGet)
	ctx.Request.SetRequestURI("/api/test?query=value&foo=bar")

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		hash, key := p.buildCacheKeyHash(ctx)
		_ = hash
		_ = key
	}
}

// BenchmarkCacheKeyHash_Compare 并行对比两种方法。
func BenchmarkCacheKeyHash_Compare(b *testing.B) {
	p, err := NewProxy(&config.ProxyConfig{
		Path:        "/api",
		LoadBalance: "round_robin",
	}, []*loadbalance.Target{{URL: "http://localhost:8080"}}, nil, nil)
	if err != nil {
		b.Fatalf("NewProxy() error: %v", err)
	}

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod(fasthttp.MethodGet)
	ctx.Request.SetRequestURI("/api/data?id=123&sort=desc")

	b.Run("ZeroAlloc", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			_ = p.buildCacheKeyHashValue(ctx)
		}
	})

	b.Run("WithAlloc", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			_, _ = p.buildCacheKeyHash(ctx)
		}
	})
}
