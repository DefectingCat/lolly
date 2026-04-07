// Package proxy 提供反向代理性能的基准测试。
package proxy

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/valyala/fasthttp"
	"github.com/valyala/fasthttp/fasthttputil"

	"rua.plus/lolly/internal/benchmark/tools"
	"rua.plus/lolly/internal/config"
	"rua.plus/lolly/internal/loadbalance"
)

// setupMockBackend 设置一个模拟后端服务器用于基准测试。
// 返回监听器地址和清理函数。
func setupMockBackend(body []byte) (string, func()) {
	ln := fasthttputil.NewInmemoryListener()

	server := &fasthttp.Server{
		Handler: func(ctx *fasthttp.RequestCtx) {
			ctx.SetStatusCode(fasthttp.StatusOK)
			ctx.Write(body)
		},
	}

	go func() {
		server.Serve(ln)
	}()

	// 等待服务器启动
	time.Sleep(5 * time.Millisecond)

	addr := ln.Addr().String()
	cleanup := func() {
		ln.Close()
	}

	return addr, cleanup
}

// BenchmarkProxyForward 基准测试代理转发性能。
func BenchmarkProxyForward(b *testing.B) {
	testCases := []struct {
		name      string
		concurrency int
	}{
		{"concurrency1", 1},
		{"concurrency10", 10},
		{"concurrency100", 100},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			addr, cleanup := setupMockBackend([]byte("Hello, World!"))
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

			targets := []*loadbalance.Target{
				{URL: "http://" + addr},
			}
			targets[0].Healthy.Store(true)

			p, err := NewProxy(cfg, targets, nil)
			if err != nil {
				b.Fatalf("NewProxy() error: %v", err)
			}

			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					ctx := &fasthttp.RequestCtx{}
					ctx.Request.Header.SetMethod(fasthttp.MethodGet)
					ctx.Request.SetRequestURI("/api/test")
					p.ServeHTTP(ctx)
				}
			})
		})
	}
}

// BenchmarkProxyForwardSmallRequest 基准测试小请求/小响应代理转发。
func BenchmarkProxyForwardSmallRequest(b *testing.B) {
	smallBody := make([]byte, 100)
	for i := range smallBody {
		smallBody[i] = byte('a' + i%26)
	}

	addr, cleanup := setupMockBackend(smallBody)
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

	targets := []*loadbalance.Target{
		{URL: "http://" + addr},
	}
	targets[0].Healthy.Store(true)

	p, err := NewProxy(cfg, targets, nil)
	if err != nil {
		b.Fatalf("NewProxy() error: %v", err)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			ctx := &fasthttp.RequestCtx{}
			ctx.Request.Header.SetMethod(fasthttp.MethodPost)
			ctx.Request.SetRequestURI("/api/test")
			ctx.Request.SetBodyString(string(smallBody))
			p.ServeHTTP(ctx)
		}
	})
}

// BenchmarkProxyForwardLargeRequest 基准测试大请求/大响应代理转发。
func BenchmarkProxyForwardLargeRequest(b *testing.B) {
	// 1KB 请求，10KB 响应
	requestBody := make([]byte, 1024)
	for i := range requestBody {
		requestBody[i] = byte('a' + i%26)
	}
	responseBody := make([]byte, 10*1024)
	for i := range responseBody {
		responseBody[i] = byte('A' + i%26)
	}

	addr, cleanup := setupMockBackend(responseBody)
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

	targets := []*loadbalance.Target{
		{URL: "http://" + addr},
	}
	targets[0].Healthy.Store(true)

	p, err := NewProxy(cfg, targets, nil)
	if err != nil {
		b.Fatalf("NewProxy() error: %v", err)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			ctx := &fasthttp.RequestCtx{}
			ctx.Request.Header.SetMethod(fasthttp.MethodPost)
			ctx.Request.SetRequestURI("/api/test")
			ctx.Request.SetBody(requestBody)
			p.ServeHTTP(ctx)
		}
	})
}

// BenchmarkProxyForwardMultipleTargets 基准测试多目标代理转发。
func BenchmarkProxyForwardMultipleTargets(b *testing.B) {
	smallBody := []byte("OK")
	numTargets := 5
	targets := make([]*loadbalance.Target, numTargets)
	cleanups := make([]func(), numTargets)

	for i := 0; i < numTargets; i++ {
		addr, cleanup := setupMockBackend(smallBody)
		cleanups[i] = cleanup
		targets[i] = &loadbalance.Target{
			URL:    "http://" + addr,
			Weight: i + 1,
		}
		targets[i].Healthy.Store(true)
	}

	defer func() {
		for _, cleanup := range cleanups {
			cleanup()
		}
	}()

	cfg := &config.ProxyConfig{
		Path:        "/api",
		LoadBalance: "weighted_round_robin",
		Timeout: config.ProxyTimeout{
			Connect: 5 * time.Second,
			Read:    30 * time.Second,
			Write:   30 * time.Second,
		},
	}

	p, err := NewProxy(cfg, targets, nil)
	if err != nil {
		b.Fatalf("NewProxy() error: %v", err)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			ctx := &fasthttp.RequestCtx{}
			ctx.Request.Header.SetMethod(fasthttp.MethodGet)
			ctx.Request.SetRequestURI("/api/test")
			p.ServeHTTP(ctx)
		}
	})
}

// BenchmarkProxyHostClient 基准测试 HostClient 性能。
func BenchmarkProxyHostClient(b *testing.B) {
	smallBody := []byte("Hello")
	addr, cleanup := setupMockBackend(smallBody)
	defer cleanup()

	timeout := config.ProxyTimeout{
		Connect: 5 * time.Second,
		Read:    30 * time.Second,
		Write:   30 * time.Second,
	}

	client := createHostClient("http://"+addr, timeout, nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := fasthttp.AcquireRequest()
		resp := fasthttp.AcquireResponse()

		req.SetRequestURI("http://" + addr + "/api/test")
		req.Header.SetMethod(fasthttp.MethodGet)

		client.Do(req, resp)

		fasthttp.ReleaseRequest(req)
		fasthttp.ReleaseResponse(resp)
	}
}

// BenchmarkProxyHostClientParallel 基准测试 HostClient 并行性能。
func BenchmarkProxyHostClientParallel(b *testing.B) {
	smallBody := []byte("Hello")
	addr, cleanup := setupMockBackend(smallBody)
	defer cleanup()

	timeout := config.ProxyTimeout{
		Connect: 5 * time.Second,
		Read:    30 * time.Second,
		Write:   30 * time.Second,
	}

	client := createHostClient("http://"+addr, timeout, nil)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			req := fasthttp.AcquireRequest()
			resp := fasthttp.AcquireResponse()

			req.SetRequestURI("http://" + addr + "/api/test")
			req.Header.SetMethod(fasthttp.MethodGet)

			client.Do(req, resp)

			fasthttp.ReleaseRequest(req)
			fasthttp.ReleaseResponse(resp)
		}
	})
}

// BenchmarkProxyWithMockBackend 基准测试使用 mock_backend 工具的代理转发。
func BenchmarkProxyWithMockBackend(b *testing.B) {
	// 使用 tools 包启动 mock 后端
	addr, cleanup := tools.SimpleMockBackend(fasthttp.StatusOK, []byte("Hello from mock backend"))
	defer cleanup()

	// 等待服务器完全启动
	time.Sleep(10 * time.Millisecond)

	cfg := &config.ProxyConfig{
		Path:        "/api",
		LoadBalance: "round_robin",
		Timeout: config.ProxyTimeout{
			Connect: 5 * time.Second,
			Read:    30 * time.Second,
			Write:   30 * time.Second,
		},
	}

	targets := []*loadbalance.Target{
		{URL: "http://" + addr},
	}
	targets[0].Healthy.Store(true)

	p, err := NewProxy(cfg, targets, nil)
	if err != nil {
		b.Fatalf("NewProxy() error: %v", err)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			ctx := &fasthttp.RequestCtx{}
			ctx.Request.Header.SetMethod(fasthttp.MethodGet)
			ctx.Request.SetRequestURI("/api/test")
			p.ServeHTTP(ctx)
		}
	})
}

// BenchmarkProxyLoadBalancerSelection 基准测试代理负载均衡器选择性能。
func BenchmarkProxyLoadBalancerSelection(b *testing.B) {
	testCases := []struct {
		name        string
		loadBalance string
		targetCount int
	}{
		{"round_robin_3", "round_robin", 3},
		{"round_robin_50", "round_robin", 50},
		{"weighted_round_robin_3", "weighted_round_robin", 3},
		{"least_conn_3", "least_conn", 3},
		{"ip_hash_3", "ip_hash", 3},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			targets := make([]*loadbalance.Target, tc.targetCount)
			for i := 0; i < tc.targetCount; i++ {
				targets[i] = &loadbalance.Target{
					URL:    fmt.Sprintf("http://backend%d:8080", i),
					Weight: i + 1,
				}
				targets[i].Healthy.Store(true)
			}

			cfg := &config.ProxyConfig{
				Path:        "/api",
				LoadBalance: tc.loadBalance,
				Timeout: config.ProxyTimeout{
					Connect: 5 * time.Second,
					Read:    30 * time.Second,
					Write:   30 * time.Second,
				},
			}

			p, err := NewProxy(cfg, targets, nil)
			if err != nil {
				b.Fatalf("NewProxy() error: %v", err)
			}

			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				counter := uint64(0)
				// 每个 goroutine 使用独立的上下文
				ctx := &fasthttp.RequestCtx{}
				ctx.Request.SetRequestURI("/api/test")
				for pb.Next() {
					idx := atomic.AddUint64(&counter, 1)
					ctx.Request.Header.Set("X-Forwarded-For", fmt.Sprintf("192.168.1.%d", idx%255))
					p.selectTarget(ctx)
				}
			})
		})
	}
}

// BenchmarkProxyHeaderProcessing 基准测试代理请求头处理性能。
func BenchmarkProxyHeaderProcessing(b *testing.B) {
	target := &loadbalance.Target{URL: "http://localhost:8080"}

	cfg := &config.ProxyConfig{
		Path:        "/api",
		LoadBalance: "round_robin",
		Timeout: config.ProxyTimeout{
			Connect: 5 * time.Second,
			Read:    30 * time.Second,
			Write:   30 * time.Second,
		},
		Headers: config.ProxyHeaders{
			SetRequest: map[string]string{
				"X-Custom-Header": "custom-value",
				"X-Another":       "another-value",
			},
			Remove: []string{"X-Remove-Me"},
		},
	}

	targets := []*loadbalance.Target{
		target,
	}
	targets[0].Healthy.Store(true)

	p, err := NewProxy(cfg, targets, nil)
	if err != nil {
		b.Fatalf("NewProxy() error: %v", err)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			ctx := &fasthttp.RequestCtx{}
			ctx.Request.Header.SetMethod(fasthttp.MethodGet)
			ctx.Request.SetRequestURI("/api/test")
			ctx.Request.Header.Set("X-Forwarded-For", "192.168.1.100")
			ctx.Request.Header.Set("X-Remove-Me", "should-be-removed")
			p.modifyRequestHeaders(ctx, target)
		}
	})
}
