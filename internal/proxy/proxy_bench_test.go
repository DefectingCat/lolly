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
	"rua.plus/lolly/internal/variable"
)

// setupMockBackend 设置一个模拟后端服务器用于基准测试。
// 返回监听器地址和清理函数。
func setupMockBackend(body []byte) (string, func()) {
	ln := fasthttputil.NewInmemoryListener()

	server := &fasthttp.Server{
		Handler: func(ctx *fasthttp.RequestCtx) {
			ctx.SetStatusCode(fasthttp.StatusOK)
			_, _ = ctx.Write(body)
		},
	}

	go func() {
		_ = server.Serve(ln)
	}()

	// 等待服务器启动
	time.Sleep(5 * time.Millisecond)

	addr := ln.Addr().String()
	cleanup := func() {
		_ = ln.Close()
	}

	return addr, cleanup
}

// BenchmarkProxyForward 基准测试代理转发性能。
func BenchmarkProxyForward(b *testing.B) {
	testCases := []struct {
		name        string
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

			p, err := NewProxy(cfg, targets, nil, nil)
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

	p, err := NewProxy(cfg, targets, nil, nil)
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

	p, err := NewProxy(cfg, targets, nil, nil)
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

	p, err := NewProxy(cfg, targets, nil, nil)
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

	client := createHostClient("http://"+addr, timeout, nil, nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := fasthttp.AcquireRequest()
		resp := fasthttp.AcquireResponse()

		req.SetRequestURI("http://" + addr + "/api/test")
		req.Header.SetMethod(fasthttp.MethodGet)

		_ = client.Do(req, resp)

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

	client := createHostClient("http://"+addr, timeout, nil, nil)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			req := fasthttp.AcquireRequest()
			resp := fasthttp.AcquireResponse()

			req.SetRequestURI("http://" + addr + "/api/test")
			req.Header.SetMethod(fasthttp.MethodGet)

			_ = client.Do(req, resp)

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

	p, err := NewProxy(cfg, targets, nil, nil)
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

			p, err := NewProxy(cfg, targets, nil, nil)
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

	p, err := NewProxy(cfg, targets, nil, nil)
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

// BenchmarkBuildCacheKeyHash 基准测试缓存键哈希计算性能。
func BenchmarkBuildCacheKeyHash(b *testing.B) {
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod(fasthttp.MethodGet)
	ctx.Request.SetRequestURI("/api/test?query=1")

	p, err := NewProxy(&config.ProxyConfig{
		Path:        "/api",
		LoadBalance: "round_robin",
		Timeout: config.ProxyTimeout{
			Connect: 5 * time.Second,
			Read:    30 * time.Second,
			Write:   30 * time.Second,
		},
	}, []*loadbalance.Target{{URL: "http://localhost:8080"}}, nil, nil)
	if err != nil {
		b.Fatalf("NewProxy() error: %v", err)
	}

	b.Run("buildCacheKeyHash_with_string", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			hashKey, _ := p.buildCacheKeyHash(ctx)
			_ = hashKey
		}
	})

	b.Run("buildCacheKeyHashValue_direct", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			hashKey := p.buildCacheKeyHashValue(ctx)
			_ = hashKey
		}
	})
}

// BenchmarkProxyObjectPoolGetRelease 基准测试 proxy 中对象池的获取/释放效果。
// 验证 UpstreamTiming 和变量上下文的池复用性能，对比有无池化的差异。
func BenchmarkProxyObjectPoolGetRelease(b *testing.B) {
	addr, cleanup := setupMockBackend([]byte("OK"))
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

	_, err := NewProxy(cfg, targets, nil, nil)
	if err != nil {
		b.Fatalf("NewProxy() error: %v", err)
	}

	// 本测试聚焦上游计时器和变量上下文池化效果，Proxy 仅用于初始化验证

	b.Run("UpstreamTiming_Pooled", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			timing := NewUpstreamTiming()
			timing.MarkConnectStart()
			time.Sleep(time.Microsecond)
			timing.MarkConnectEnd()
			timing.MarkHeaderReceived()
			timing.MarkResponseEnd()
			_ = timing.GetConnectTime()
			_ = timing.GetHeaderTime()
			_ = timing.GetResponseTime()
		}
	})

	b.Run("VariableContext_Pooled", func(b *testing.B) {
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.Header.SetMethod(fasthttp.MethodGet)
		ctx.Request.SetRequestURI("/api/test")
		ctx.Request.Header.Set("X-Forwarded-For", "192.168.1.1")

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			vc := variable.NewContext(ctx)
			vc.Set("key", "value")
			_ = vc.Expand("$key")
			variable.ReleaseContext(vc)
		}
	})
}

// BenchmarkProxyResponsePoolParallel 基准测试并行场景下的响应池获取/释放性能。
// 验证 fasthttp.Request/Response 对象池在并发代理请求中的表现。
func BenchmarkProxyResponsePoolParallel(b *testing.B) {
	addr, cleanup := setupMockBackend([]byte("parallel response"))
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
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			ctx := &fasthttp.RequestCtx{}
			ctx.Request.Header.SetMethod(fasthttp.MethodGet)
			ctx.Request.SetRequestURI("/api/test")
			p.ServeHTTP(ctx)
		}
	})
}

// BenchmarkProxyZeroAllocPath 基准测试零分配路径性能。
// 验证 buildCacheKeyHashValue 的零分配优化效果，
// 对比旧的字符串构建方式与直接哈希写入的差异。
func BenchmarkProxyZeroAllocPath(b *testing.B) {
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod(fasthttp.MethodGet)
	ctx.Request.SetRequestURI("/api/test?query=value&foo=bar")

	p, err := NewProxy(&config.ProxyConfig{
		Path:        "/api",
		LoadBalance: "round_robin",
		Timeout: config.ProxyTimeout{
			Connect: 5 * time.Second,
			Read:    30 * time.Second,
			Write:   30 * time.Second,
		},
	}, []*loadbalance.Target{{URL: "http://localhost:8080"}}, nil, nil)
	if err != nil {
		b.Fatalf("NewProxy() error: %v", err)
	}

	b.Run("ZeroAlloc_buildCacheKeyHashValue", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			hash := p.buildCacheKeyHashValue(ctx)
			_ = hash
		}
	})

	b.Run("WithAlloc_buildCacheKeyHash", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			hash, key := p.buildCacheKeyHash(ctx)
			_ = hash
			_ = key
		}
	})

	b.Run("ForwardedHeaders_ExtractSet", func(b *testing.B) {
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.Header.SetMethod(fasthttp.MethodGet)
		ctx.Request.SetRequestURI("/api/test")
		ctx.Request.Header.Set("X-Forwarded-For", "10.0.0.1")
		ctx.Request.Header.Set("X-Real-IP", "10.0.0.2")
		ctx.Request.Header.Set("Forwarded", "for=10.0.0.3")
		ctx.Request.Header.Set("X-Forwarded-Proto", "https")
		ctx.Request.Header.Set("X-Forwarded-Host", "example.com")

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			fh := ExtractForwardedHeaders(ctx)
			_ = fh
		}
	})
}
