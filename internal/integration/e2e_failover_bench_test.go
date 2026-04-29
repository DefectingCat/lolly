// Package integration 提供后端故障切换 E2E 基准测试。
//
// 该文件测试负载均衡器剔除/恢复后端的开销。
//
// 测试场景：
//   - 健康后端正常选择
//   - 后端标记不健康后的剔除开销
//   - 后端重新标记健康后的恢复开销
//
// 作者：xfy
package integration

import (
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/config"
	"rua.plus/lolly/internal/loadbalance"
	"rua.plus/lolly/internal/proxy"
)

// setupFailoverBackends 创建多后端用于故障切换测试。
//
// 参数：
//   - count: 后端数量
//   - healthyCount: 初始健康后端数量
//
// 返回值：
//   - targets: 目标列表
//   - cleanups: 清理函数列表
func setupFailoverBackends(b *testing.B, count, healthyCount int) ([]*loadbalance.Target, []func()) {
	b.Helper()

	targets := make([]*loadbalance.Target, count)
	cleanups := make([]func(), count)

	for i := 0; i < count; i++ {
		addr, cleanup := setupNetworkBackend(b, fasthttp.StatusOK, []byte(`{"backend":`+strconv.Itoa(i)+`}`))
		cleanups[i] = cleanup
		targets[i] = &loadbalance.Target{
			URL:    "http://" + addr,
			Weight: 1,
		}
		// 设置健康状态
		if i < healthyCount {
			targets[i].Healthy.Store(true)
		} else {
			targets[i].Healthy.Store(false)
		}
	}

	return targets, cleanups
}

// BenchmarkE2EFailover_NormalSelect 测试健康后端正常选择。
//
// 所有后端健康时的负载均衡选择开销。
func BenchmarkE2EFailover_NormalSelect(b *testing.B) {
	targets, cleanups := setupFailoverBackends(b, 5, 5)
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

	p, err := proxy.NewProxy(cfg, targets, nil, nil)
	if err != nil {
		b.Fatalf("NewProxy() error: %v", err)
	}

	warmupProxy(p, "/api/test", 10)

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

// BenchmarkE2EFailover_OneUnhealthy 测试一个后端不健康。
//
// 4/5 后端健康时的选择开销。
func BenchmarkE2EFailover_OneUnhealthy(b *testing.B) {
	targets, cleanups := setupFailoverBackends(b, 5, 4)
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

	p, err := proxy.NewProxy(cfg, targets, nil, nil)
	if err != nil {
		b.Fatalf("NewProxy() error: %v", err)
	}

	warmupProxy(p, "/api/test", 10)

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

// BenchmarkE2EFailover_MostUnhealthy 测试多数后端不健康。
//
// 1/5 后端健康时的选择开销（剔除开销增大）。
func BenchmarkE2EFailover_MostUnhealthy(b *testing.B) {
	targets, cleanups := setupFailoverBackends(b, 5, 1)
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

	p, err := proxy.NewProxy(cfg, targets, nil, nil)
	if err != nil {
		b.Fatalf("NewProxy() error: %v", err)
	}

	warmupProxy(p, "/api/test", 10)

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

// BenchmarkE2EFailover_DynamicToggle 测试动态健康状态切换。
//
// 模拟后端健康状态在测试中变化。
func BenchmarkE2EFailover_DynamicToggle(b *testing.B) {
	targets, cleanups := setupFailoverBackends(b, 3, 3)
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

	p, err := proxy.NewProxy(cfg, targets, nil, nil)
	if err != nil {
		b.Fatalf("NewProxy() error: %v", err)
	}

	warmupProxy(p, "/api/test", 10)

	var toggleCounter atomic.Uint64

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			// 每 100 次请求切换一个后端的健康状态
			count := toggleCounter.Add(1)
			if count % 100 == 0 {
				targetIdx := int(count / 100) % len(targets)
				current := targets[targetIdx].Healthy.Load()
				targets[targetIdx].Healthy.Store(!current)
			}

			ctx := &fasthttp.RequestCtx{}
			ctx.Request.SetRequestURI("/api/test")
			ctx.Request.Header.SetMethod(fasthttp.MethodGet)
			p.ServeHTTP(ctx)
		}
	})
}

// BenchmarkE2EFailover_AllUnhealthy 测试所有后端不健康。
//
// 无可用后端时的选择开销（应该返回错误）。
func BenchmarkE2EFailover_AllUnhealthy(b *testing.B) {
	targets, cleanups := setupFailoverBackends(b, 3, 0)
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

	p, err := proxy.NewProxy(cfg, targets, nil, nil)
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

// BenchmarkE2EFailover_SelectOnly 测试纯选择开销（无实际请求）。
//
// 验证负载均衡器选择逻辑的分配。
func BenchmarkE2EFailover_SelectOnly(b *testing.B) {
	targets := make([]*loadbalance.Target, 5)
	for i := 0; i < 5; i++ {
		targets[i] = &loadbalance.Target{
			URL:    "http://backend" + strconv.Itoa(i) + ":8080",
			Weight: 1,
		}
		targets[i].Healthy.Store(true)
	}

	rr := loadbalance.NewRoundRobin()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = rr.Select(targets)
	}
}