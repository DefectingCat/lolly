// Package proxy 提供连接池满载场景测试。
//
// 该文件测试连接池达到上限时的等待/新建行为。
//
// 测试场景：
//   - NormalPool: 正常连接池使用
//   - PoolFull: 连接池满载时的等待行为
//   - PoolReuse: 连接复用效果
//
// 作者：xfy
package proxy

import (
	"strconv"
	"testing"
	"time"

	"github.com/valyala/fasthttp"
	"github.com/valyala/fasthttp/fasthttputil"
	"rua.plus/lolly/internal/config"
	"rua.plus/lolly/internal/loadbalance"
)

// setupInmemoryBackend 创建内存后端。
func setupInmemoryBackend(body []byte) (string, func()) {
	ln := fasthttputil.NewInmemoryListener()

	srv := &fasthttp.Server{
		Handler: func(ctx *fasthttp.RequestCtx) {
			ctx.SetStatusCode(fasthttp.StatusOK)
			_, _ = ctx.Write(body)
		},
	}

	go func() {
		_ = srv.Serve(ln)
	}()

	addr := ln.Addr().String()
	cleanup := func() {
		_ = srv.Shutdown()
		_ = ln.Close()
	}

	return addr, cleanup
}

// BenchmarkConnectionPool_Normal 测试正常连接池使用。
//
// 连接池未满时的请求处理。
func BenchmarkConnectionPool_Normal(b *testing.B) {
	addr, cleanup := setupInmemoryBackend([]byte("OK"))
	defer cleanup()

	cfg := &config.ProxyConfig{
		Path:        "/api",
		LoadBalance: "round_robin",
		Timeout: config.ProxyTimeout{
			Connect: 5 * time.Second,
			Read:    30 * time.Second,
			Write:   30 * time.Second,
		},
	}

	targets := []*loadbalance.Target{{URL: "http://" + addr}}
	targets[0].Healthy.Store(true)

	p, err := NewProxy(cfg, targets, nil, nil)
	if err != nil {
		b.Fatalf("NewProxy() error: %v", err)
	}

	// 预热连接池
	for range 10 {
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.SetRequestURI("/api/test")
		ctx.Request.Header.SetMethod(fasthttp.MethodGet)
		p.ServeHTTP(ctx)
	}

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			ctx := &fasthttp.RequestCtx{}
			ctx.Request.SetRequestURI("/api/test")
			ctx.Request.Header.SetMethod(fasthttp.MethodGet)
			p.ServeHTTP(ctx)
		}
	})
}

// BenchmarkConnectionPool_HighConcurrency 测试高并发场景。
//
// 模拟连接池接近上限时的行为。
func BenchmarkConnectionPool_HighConcurrency(b *testing.B) {
	addr, cleanup := setupInmemoryBackend([]byte("response"))
	defer cleanup()

	cfg := &config.ProxyConfig{
		Path:        "/api",
		LoadBalance: "round_robin",
		Timeout: config.ProxyTimeout{
			Connect: 5 * time.Second,
			Read:    30 * time.Second,
			Write:   30 * time.Second,
		},
	}

	targets := []*loadbalance.Target{{URL: "http://" + addr}}
	targets[0].Healthy.Store(true)

	p, err := NewProxy(cfg, targets, nil, nil)
	if err != nil {
		b.Fatalf("NewProxy() error: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	// 高并发：GOMAXPROCS 个 goroutine 同时请求
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			ctx := &fasthttp.RequestCtx{}
			ctx.Request.SetRequestURI("/api/test")
			ctx.Request.Header.SetMethod(fasthttp.MethodGet)
			p.ServeHTTP(ctx)
		}
	})
}

// BenchmarkConnectionPool_SmallBody 测试小响应体连接池效率。
//
// 小响应体更依赖连接池复用。
func BenchmarkConnectionPool_SmallBody(b *testing.B) {
	addr, cleanup := setupInmemoryBackend([]byte("x"))
	defer cleanup()

	cfg := &config.ProxyConfig{
		Path:        "/api",
		LoadBalance: "round_robin",
		Timeout: config.ProxyTimeout{
			Connect: 5 * time.Second,
			Read:    30 * time.Second,
			Write:   30 * time.Second,
		},
	}

	targets := []*loadbalance.Target{{URL: "http://" + addr}}
	targets[0].Healthy.Store(true)

	p, err := NewProxy(cfg, targets, nil, nil)
	if err != nil {
		b.Fatalf("NewProxy() error: %v", err)
	}

	// 预热
	for range 5 {
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.SetRequestURI("/api/test")
		p.ServeHTTP(ctx)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.SetRequestURI("/api/test")
		ctx.Request.Header.SetMethod(fasthttp.MethodGet)
		p.ServeHTTP(ctx)
	}
}

// BenchmarkConnectionPool_LargeBody 测试大响应体连接池效率。
//
// 大响应体需要更多缓冲区管理。
func BenchmarkConnectionPool_LargeBody(b *testing.B) {
	largeBody := make([]byte, 50*1024) // 50KB
	for i := range largeBody {
		largeBody[i] = byte('A' + i%26)
	}

	addr, cleanup := setupInmemoryBackend(largeBody)
	defer cleanup()

	cfg := &config.ProxyConfig{
		Path:        "/api",
		LoadBalance: "round_robin",
		Timeout: config.ProxyTimeout{
			Connect: 5 * time.Second,
			Read:    30 * time.Second,
			Write:   30 * time.Second,
		},
	}

	targets := []*loadbalance.Target{{URL: "http://" + addr}}
	targets[0].Healthy.Store(true)

	p, err := NewProxy(cfg, targets, nil, nil)
	if err != nil {
		b.Fatalf("NewProxy() error: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.SetRequestURI("/api/test")
		ctx.Request.Header.SetMethod(fasthttp.MethodGet)
		p.ServeHTTP(ctx)
	}
}

// BenchmarkConnectionPool_MultiTarget 测试多目标连接池。
//
// 每个目标有独立连接池。
func BenchmarkConnectionPool_MultiTarget(b *testing.B) {
	targets := make([]*loadbalance.Target, 3)
	cleanups := make([]func(), 3)

	for i := range 3 {
		addr, cleanup := setupInmemoryBackend([]byte("backend" + strconv.Itoa(i)))
		cleanups[i] = cleanup
		targets[i] = &loadbalance.Target{
			URL:    "http://" + addr,
			Weight: 1,
		}
		targets[i].Healthy.Store(true)
	}

	defer func() {
		for _, c := range cleanups {
			c()
		}
	}()

	cfg := &config.ProxyConfig{
		Path:        "/api",
		LoadBalance: "round_robin",
		Timeout: config.ProxyTimeout{
			Connect: 5 * time.Second,
			Read:    30 * time.Second,
			Write:   30 * time.Second,
		},
	}

	p, err := NewProxy(cfg, targets, nil, nil)
	if err != nil {
		b.Fatalf("NewProxy() error: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			ctx := &fasthttp.RequestCtx{}
			ctx.Request.SetRequestURI("/api/test")
			ctx.Request.Header.SetMethod(fasthttp.MethodGet)
			p.ServeHTTP(ctx)
		}
	})
}

// BenchmarkHostClient_AcquireRelease 测试 HostClient 对象池。
//
// 验证 fasthttp.Request/Response 池化效果。
func BenchmarkHostClient_AcquireRelease(b *testing.B) {
	addr, cleanup := setupInmemoryBackend([]byte("pool test"))
	defer cleanup()

	timeout := config.ProxyTimeout{
		Connect: 5 * time.Second,
		Read:    30 * time.Second,
		Write:   30 * time.Second,
	}

	client := createHostClient("http://"+addr, timeout, nil, nil, "", nil)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		req := fasthttp.AcquireRequest()
		resp := fasthttp.AcquireResponse()

		req.SetRequestURI("http://" + addr + "/api/test")
		req.Header.SetMethod(fasthttp.MethodGet)

		_ = client.Do(req, resp)

		fasthttp.ReleaseRequest(req)
		fasthttp.ReleaseResponse(resp)
	}
}
