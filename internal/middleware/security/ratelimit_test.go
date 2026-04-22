// Package security 提供速率限制功能的测试。
//
// 该文件测试速率限制模块的各项功能，包括：
//   - 速率限制器创建
//   - 令牌桶算法
//   - 令牌补充机制
//   - 计数器重置
//   - 连接数限制
//   - 统计信息获取
//
// 作者：xfy
package security

import (
	"testing"
	"time"

	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/config"
	"rua.plus/lolly/internal/testutil"
)

func TestNewRateLimiter(t *testing.T) {
	tests := []struct {
		cfg     *config.RateLimitConfig
		name    string
		wantErr bool
	}{
		{
			name:    "nil config",
			cfg:     nil,
			wantErr: true,
		},
		{
			name: "valid config",
			cfg: &config.RateLimitConfig{
				RequestRate: 100,
				Burst:       200,
			},
		},
		{
			name: "zero rate",
			cfg: &config.RateLimitConfig{
				RequestRate: 0,
				Burst:       100,
			},
			wantErr: true,
		},
		{
			name: "burst less than rate",
			cfg: &config.RateLimitConfig{
				RequestRate: 100,
				Burst:       50,
			},
			wantErr: true,
		},
		{
			name: "key by IP",
			cfg: &config.RateLimitConfig{
				RequestRate: 100,
				Burst:       200,
				Key:         "ip",
			},
		},
		{
			name: "key by header",
			cfg: &config.RateLimitConfig{
				RequestRate: 100,
				Burst:       200,
				Key:         "header",
			},
		},
		{
			name: "unknown key type",
			cfg: &config.RateLimitConfig{
				RequestRate: 100,
				Burst:       200,
				Key:         "unknown",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rl, err := NewRateLimiter(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewRateLimiter() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && rl == nil {
				t.Error("Expected non-nil RateLimiter")
			}
		})
	}
}

func TestRateLimiterAllow(t *testing.T) {
	mw, err := NewRateLimiter(&config.RateLimitConfig{
		RequestRate: 10,
		Burst:       10,
	})
	if err != nil {
		t.Fatalf("NewRateLimiter() error: %v", err)
	}

	rl, ok := mw.(*RateLimiter)
	if !ok {
		t.Fatalf("Expected *RateLimiter, got %T", mw)
	}

	// Test burst allowance
	key := "test-key"

	// Should allow burst requests
	for i := 0; i < 10; i++ {
		if !rl.Allow(key) {
			t.Errorf("Expected request %d to be allowed", i+1)
		}
	}

	// Next request should be denied (burst exhausted)
	if rl.Allow(key) {
		t.Error("Expected request to be denied after burst exhausted")
	}
}

func TestRateLimiterTokenRefill(t *testing.T) {
	mw, err := NewRateLimiter(&config.RateLimitConfig{
		RequestRate: 100, // 100 tokens per second
		Burst:       100,
	})
	if err != nil {
		t.Fatalf("NewRateLimiter() error: %v", err)
	}

	rl, ok := mw.(*RateLimiter)
	if !ok {
		t.Fatalf("Expected *RateLimiter, got %T", mw)
	}

	key := "refill-test"

	// Exhaust the burst
	for i := 0; i < 100; i++ {
		rl.Allow(key)
	}

	// Should be denied
	if rl.Allow(key) {
		t.Error("Expected request to be denied")
	}

	// Wait for token refill (10ms should give us 1 token at 100/s)
	time.Sleep(15 * time.Millisecond)

	// Should be allowed now
	if !rl.Allow(key) {
		t.Error("Expected request to be allowed after refill")
	}
}

func TestRateLimiterReset(t *testing.T) {
	mw, err := NewRateLimiter(&config.RateLimitConfig{
		RequestRate: 1,
		Burst:       1,
	})
	if err != nil {
		t.Fatalf("NewRateLimiter() error: %v", err)
	}

	rl, ok := mw.(*RateLimiter)
	if !ok {
		t.Fatalf("Expected *RateLimiter, got %T", mw)
	}

	key := "reset-test"

	// Exhaust
	rl.Allow(key)
	if rl.Allow(key) {
		t.Error("Expected denial")
	}

	// Reset
	rl.Reset(key)

	// Should be allowed again
	if !rl.Allow(key) {
		t.Error("Expected request to be allowed after reset")
	}
}

func TestRateLimiterResetAll(t *testing.T) {
	mw, err := NewRateLimiter(&config.RateLimitConfig{
		RequestRate: 1,
		Burst:       1,
	})
	if err != nil {
		t.Fatalf("NewRateLimiter() error: %v", err)
	}

	rl, ok := mw.(*RateLimiter)
	if !ok {
		t.Fatalf("Expected *RateLimiter, got %T", mw)
	}

	// Create multiple buckets
	rl.Allow("key1")
	rl.Allow("key2")

	// Reset all
	rl.ResetAll()

	stats := rl.GetStats()
	if stats.BucketCount != 0 {
		t.Errorf("Expected 0 buckets after reset, got %d", stats.BucketCount)
	}
}

func TestRateLimiterCleanup(t *testing.T) {
	mw, err := NewRateLimiter(&config.RateLimitConfig{
		RequestRate: 100,
		Burst:       100,
	})
	if err != nil {
		t.Fatalf("NewRateLimiter() error: %v", err)
	}

	rl, ok := mw.(*RateLimiter)
	if !ok {
		t.Fatalf("Expected *RateLimiter, got %T", mw)
	}

	// Create some buckets
	rl.Allow("key1")
	rl.Allow("key2")

	// Cleanup with very short max age
	rl.Cleanup(1 * time.Nanosecond)

	stats := rl.GetStats()
	if stats.BucketCount != 0 {
		t.Errorf("Expected 0 buckets after cleanup, got %d", stats.BucketCount)
	}
}

func TestRateLimiterProcess(t *testing.T) {
	mw, err := NewRateLimiter(&config.RateLimitConfig{
		RequestRate: 100,
		Burst:       100,
	})
	if err != nil {
		t.Fatalf("NewRateLimiter() error: %v", err)
	}

	nextHandler := func(ctx *fasthttp.RequestCtx) {
		_, _ = ctx.WriteString("OK")
	}

	handler := mw.Process(nextHandler)
	if handler == nil {
		t.Error("Process() returned nil handler")
	}
}

func TestRateLimiterGetStats(t *testing.T) {
	mw, err := NewRateLimiter(&config.RateLimitConfig{
		RequestRate: 100,
		Burst:       200,
	})
	if err != nil {
		t.Fatalf("NewRateLimiter() error: %v", err)
	}

	rl, ok := mw.(*RateLimiter)
	if !ok {
		t.Fatalf("Expected *RateLimiter, got %T", mw)
	}

	rl.Allow("key1")
	rl.Allow("key2")

	stats := rl.GetStats()
	if stats.BucketCount != 2 {
		t.Errorf("Expected BucketCount 2, got %d", stats.BucketCount)
	}
	if stats.Rate != 100 {
		t.Errorf("Expected Rate 100, got %f", stats.Rate)
	}
	if stats.Burst != 200 {
		t.Errorf("Expected Burst 200, got %f", stats.Burst)
	}

	// 测试优雅关闭
	rl.StopCleanup()
}

func TestRateLimiterAutoCleanup(t *testing.T) {
	// 使用自定义的创建方式，方便测试
	cfg := &config.RateLimitConfig{
		RequestRate: 100,
		Burst:       200,
		Key:         "ip",
	}

	mw, err := NewRateLimiter(cfg)
	if err != nil {
		t.Fatalf("NewRateLimiter() error: %v", err)
	}

	rl, ok := mw.(*RateLimiter)
	if !ok {
		t.Fatalf("Expected *RateLimiter, got %T", mw)
	}

	// 创建一些桶
	rl.Allow("key1")
	rl.Allow("key2")
	rl.Allow("key3")

	// 验证桶已创建
	stats := rl.GetStats()
	if stats.BucketCount != 3 {
		t.Errorf("Expected 3 buckets, got %d", stats.BucketCount)
	}

	// 手动调用 Cleanup 模拟过期清理（使用很短的过期时间）
	rl.Cleanup(1 * time.Nanosecond)

	// 验证所有桶已被清理
	stats = rl.GetStats()
	if stats.BucketCount != 0 {
		t.Errorf("Expected 0 buckets after cleanup, got %d", stats.BucketCount)
	}

	// 测试优雅关闭
	rl.StopCleanup()
}

func TestRateLimiterStopCleanup(t *testing.T) {
	mw, err := NewRateLimiter(&config.RateLimitConfig{
		RequestRate: 100,
		Burst:       200,
	})
	if err != nil {
		t.Fatalf("NewRateLimiter() error: %v", err)
	}

	rl, ok := mw.(*RateLimiter)
	if !ok {
		t.Fatalf("Expected *RateLimiter, got %T", mw)
	}

	// 验证可以正常关闭
	rl.StopCleanup()

	// 再次调用不应 panic
	rl.StopCleanup()
}

func TestNewConnLimiter(t *testing.T) {
	tests := []struct {
		name    string
		keyType string
		max     int
		perKey  bool
		wantErr bool
	}{
		{
			name:   "global limit",
			max:    100,
			perKey: false,
		},
		{
			name:    "per-key by IP",
			max:     10,
			perKey:  true,
			keyType: "ip",
		},
		{
			name:    "per-key by header",
			max:     10,
			perKey:  true,
			keyType: "header",
		},
		{
			name:    "zero max",
			max:     0,
			wantErr: true,
		},
		{
			name:    "negative max",
			max:     -1,
			wantErr: true,
		},
		{
			name:    "invalid key type",
			max:     10,
			perKey:  true,
			keyType: "invalid",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cl, err := NewConnLimiter(tt.max, tt.perKey, tt.keyType)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewConnLimiter() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && cl == nil {
				t.Error("Expected non-nil ConnLimiter")
			}
		})
	}
}

func TestConnLimiterGlobal(t *testing.T) {
	cl, err := NewConnLimiter(2, false, "")
	if err != nil {
		t.Fatalf("NewConnLimiter() error: %v", err)
	}

	ctx := testutil.NewRequestCtx("GET", "/")

	// First two should succeed
	if !cl.Acquire(ctx) {
		t.Error("Expected first acquire to succeed")
	}
	if !cl.Acquire(ctx) {
		t.Error("Expected second acquire to succeed")
	}

	// Third should fail
	if cl.Acquire(ctx) {
		t.Error("Expected third acquire to fail")
	}

	// Release one
	cl.Release(ctx)

	// Should succeed now
	if !cl.Acquire(ctx) {
		t.Error("Expected acquire after release to succeed")
	}
}

func TestConnLimiterMiddleware(t *testing.T) {
	cl, err := NewConnLimiter(1, false, "")
	if err != nil {
		t.Fatalf("NewConnLimiter() error: %v", err)
	}

	middleware := cl.Middleware()
	if middleware == nil {
		t.Error("Expected non-nil middleware")
	}
	if middleware.Name() != "conn_limiter" {
		t.Errorf("Expected name 'conn_limiter', got %s", middleware.Name())
	}
}

// TestNewRateLimiter_SlidingWindow 测试滑动窗口算法
func TestNewRateLimiter_SlidingWindow(t *testing.T) {
	mw, err := NewRateLimiter(&config.RateLimitConfig{
		RequestRate: 100,
		Burst:       100,
		Algorithm:   "sliding_window",
	})
	if err != nil {
		t.Fatalf("NewRateLimiter() error: %v", err)
	}
	if mw == nil {
		t.Error("Expected non-nil middleware for sliding_window")
	}
	if mw.Name() != "sliding_window_limiter" {
		t.Errorf("Expected name 'sliding_window_limiter', got %s", mw.Name())
	}
}

// TestNewRateLimiter_SlidingWindowDefault 测试滑动窗口默认配置
func TestNewRateLimiter_SlidingWindowDefault(t *testing.T) {
	mw, err := NewRateLimiter(&config.RateLimitConfig{
		RequestRate:       100,
		Burst:             100,
		Algorithm:         "sliding_window",
		SlidingWindow:     0,  // 使用默认值
		SlidingWindowMode: "", // 使用默认值
	})
	if err != nil {
		t.Fatalf("NewRateLimiter() error: %v", err)
	}
	if mw == nil {
		t.Error("Expected non-nil middleware")
	}
}

// TestNewRateLimiter_SlidingWindowPrecise 测试滑动窗口精确模式
func TestNewRateLimiter_SlidingWindowPrecise(t *testing.T) {
	mw, err := NewRateLimiter(&config.RateLimitConfig{
		RequestRate:       100,
		Burst:             100,
		Algorithm:         "sliding_window",
		SlidingWindow:     1,
		SlidingWindowMode: "precise",
	})
	if err != nil {
		t.Fatalf("NewRateLimiter() error: %v", err)
	}
	if mw == nil {
		t.Error("Expected non-nil middleware")
	}
}

// TestNewRateLimiter_UnknownAlgorithm 测试未知算法
func TestNewRateLimiter_UnknownAlgorithm(t *testing.T) {
	_, err := NewRateLimiter(&config.RateLimitConfig{
		RequestRate: 100,
		Burst:       100,
		Algorithm:   "unknown_algo",
	})
	if err == nil {
		t.Error("NewRateLimiter() should return error for unknown algorithm")
	}
}

// TestSlidingWindowLimiterWrapper_Process 测试滑动窗口包装器的 Process 方法
func TestSlidingWindowLimiterWrapper_Process(t *testing.T) {
	mw, err := NewRateLimiter(&config.RateLimitConfig{
		RequestRate: 100,
		Burst:       100,
		Algorithm:   "sliding_window",
	})
	if err != nil {
		t.Fatalf("NewRateLimiter() error: %v", err)
	}

	called := false
	nextHandler := func(ctx *fasthttp.RequestCtx) {
		called = true
	}

	handler := mw.Process(nextHandler)
	if handler == nil {
		t.Fatal("Process() returned nil handler")
	}

	ctx := testutil.NewRequestCtx("GET", "/test")
	handler(ctx)

	if !called {
		t.Error("Next handler should be called when rate limit allows")
	}
}

// TestSlidingWindowLimiterWrapper_ProcessDenied 测试滑动窗口拒绝请求
func TestSlidingWindowLimiterWrapper_ProcessDenied(t *testing.T) {
	mw, err := NewRateLimiter(&config.RateLimitConfig{
		RequestRate:       1,
		Burst:             1,
		Algorithm:         "sliding_window",
		SlidingWindowMode: "precise",
	})
	if err != nil {
		t.Fatalf("NewRateLimiter() error: %v", err)
	}

	callCount := 0
	nextHandler := func(ctx *fasthttp.RequestCtx) {
		callCount++
	}

	handler := mw.Process(nextHandler)
	ctx := testutil.NewRequestCtx("GET", "/test")

	// 第一个请求应该被允许
	handler(ctx)
	// 第二个请求应该被拒绝
	handler(ctx)

	if callCount != 1 {
		t.Errorf("Expected next handler to be called once, got %d", callCount)
	}
}

// TestRateLimiter_GetRetryAfter 测试 getRetryAfter 方法
func TestRateLimiter_GetRetryAfter(t *testing.T) {
	mw, err := NewRateLimiter(&config.RateLimitConfig{
		RequestRate: 10,
		Burst:       10,
	})
	if err != nil {
		t.Fatalf("NewRateLimiter() error: %v", err)
	}

	rl, ok := mw.(*RateLimiter)
	if !ok {
		t.Fatalf("Expected *RateLimiter, got %T", mw)
	}
	defer rl.StopCleanup()

	// 测试不存在的键
	retryAfter := rl.getRetryAfter("nonexistent")
	if retryAfter != 1 {
		t.Errorf("getRetryAfter(nonexistent) = %d, want 1", retryAfter)
	}

	// 创建一个桶并消耗令牌
	key := "test-key"
	for i := 0; i < 10; i++ {
		rl.Allow(key)
	}

	// 获取重试时间
	retryAfter = rl.getRetryAfter(key)
	if retryAfter < 1 {
		t.Errorf("getRetryAfter() = %d, want at least 1", retryAfter)
	}
}

// TestKeyByIP 测试 keyByIP 函数
func TestKeyByIP(t *testing.T) {
	ctx := testutil.NewRequestCtx("GET", "/test")

	key := keyByIP(ctx)
	if key == "" {
		t.Error("keyByIP() should return non-empty string")
	}
	if key == "unknown" {
		t.Error("keyByIP() should return IP address, not 'unknown'")
	}
}

// TestKeyByHeader 测试 keyByHeader 函数
func TestKeyByHeader(t *testing.T) {
	// 测试有头部的情况
	ctx := testutil.NewRequestCtx("GET", "/test")
	ctx.Request.Header.Set("X-RateLimit-Key", "custom-key")

	key := keyByHeader(ctx)
	if key != "custom-key" {
		t.Errorf("keyByHeader() = %q, want 'custom-key'", key)
	}

	// 测试没有头部的情况（应该回退到 IP）
	ctx2 := testutil.NewRequestCtx("GET", "/test")
	key2 := keyByHeader(ctx2)
	if key2 == "" {
		t.Error("keyByHeader() should fallback to IP when header not set")
	}
}

// TestConnLimiter_PerKey 测试按键限制
func TestConnLimiter_PerKey(t *testing.T) {
	cl, err := NewConnLimiter(2, true, "ip")
	if err != nil {
		t.Fatalf("NewConnLimiter() error: %v", err)
	}

	ctx := testutil.NewRequestCtx("GET", "/")

	// 同一个键的前两个应该成功
	if !cl.Acquire(ctx) {
		t.Error("Expected first acquire to succeed")
	}
	if !cl.Acquire(ctx) {
		t.Error("Expected second acquire to succeed")
	}

	// 第三个应该失败
	if cl.Acquire(ctx) {
		t.Error("Expected third acquire to fail")
	}

	// 释放一个
	cl.Release(ctx)

	// 现在应该成功
	if !cl.Acquire(ctx) {
		t.Error("Expected acquire after release to succeed")
	}
}

// TestConnLimiter_ReleaseUnderflow 测试 Release 下溢保护
func TestConnLimiter_ReleaseUnderflow(t *testing.T) {
	cl, err := NewConnLimiter(2, true, "ip")
	if err != nil {
		t.Fatalf("NewConnLimiter() error: %v", err)
	}

	ctx := testutil.NewRequestCtx("GET", "/")

	// 在没有 Acquire 的情况下 Release（测试下溢保护）
	cl.Release(ctx) // 不应该 panic

	// 验证计数不会变成负数
	cl.Acquire(ctx)
	cl.Acquire(ctx)
	if cl.Acquire(ctx) {
		t.Error("Expected third acquire to fail")
	}
}

// TestConnLimiterMiddleware_Process 测试连接限制中间件 Process
func TestConnLimiterMiddleware_Process(t *testing.T) {
	cl, err := NewConnLimiter(1, false, "")
	if err != nil {
		t.Fatalf("NewConnLimiter() error: %v", err)
	}

	mw := cl.Middleware()

	called := false
	nextHandler := func(ctx *fasthttp.RequestCtx) {
		called = true
		ctx.SetStatusCode(200)
	}

	handler := mw.Process(nextHandler)
	ctx := testutil.NewRequestCtx("GET", "/test")

	// 第一个请求应该成功
	handler(ctx)
	if !called {
		t.Error("Next handler should be called")
	}
	if ctx.Response.StatusCode() != 200 {
		t.Errorf("Status = %d, want 200", ctx.Response.StatusCode())
	}
}

// TestConnLimiterMiddleware_ProcessLimitExceeded 测试连接限制超出
func TestConnLimiterMiddleware_ProcessLimitExceeded(t *testing.T) {
	cl, err := NewConnLimiter(1, false, "")
	if err != nil {
		t.Fatalf("NewConnLimiter() error: %v", err)
	}

	mw := cl.Middleware()

	callCount := 0
	nextHandler := func(ctx *fasthttp.RequestCtx) {
		callCount++
		ctx.SetStatusCode(200)
	}

	handler := mw.Process(nextHandler)

	// 用尽连接限制
	ctx1 := testutil.NewRequestCtx("GET", "/test1")
	cl.Acquire(ctx1) // 手动占用一个槽位

	// 现在应该无法获取新的连接
	ctx2 := testutil.NewRequestCtx("GET", "/test2")
	handler(ctx2)

	if callCount != 0 {
		t.Error("Next handler should NOT be called when limit exceeded")
	}
	if ctx2.Response.StatusCode() != 503 {
		t.Errorf("Status = %d, want 503", ctx2.Response.StatusCode())
	}
}

// TestNewSlidingWindowLimiterWrapper_Error 测试滑动窗口包装器错误情况
func TestNewSlidingWindowLimiterWrapper_Error(t *testing.T) {
	_, err := NewSlidingWindowLimiterWrapper(&config.RateLimitConfig{
		RequestRate: 100,
		Key:         "invalid_key_type",
	}, time.Second, false)
	if err == nil {
		t.Error("NewSlidingWindowLimiterWrapper should return error for invalid key type")
	}
}

// TestRateLimiter_Name 测试 RateLimiter Name 方法
func TestRateLimiter_Name(t *testing.T) {
	mw, err := NewRateLimiter(&config.RateLimitConfig{
		RequestRate: 10,
		Burst:       10,
	})
	if err != nil {
		t.Fatalf("NewRateLimiter() error: %v", err)
	}

	rl, ok := mw.(*RateLimiter)
	if !ok {
		t.Fatalf("Expected *RateLimiter, got %T", mw)
	}
	defer rl.StopCleanup()

	if rl.Name() != "rate_limiter" {
		t.Errorf("Name() = %q, want 'rate_limiter'", rl.Name())
	}
}

// TestRateLimiter_ProcessDenied 测试限流拒绝
func TestRateLimiter_ProcessDenied(t *testing.T) {
	mw, err := NewRateLimiter(&config.RateLimitConfig{
		RequestRate: 1,
		Burst:       1,
	})
	if err != nil {
		t.Fatalf("NewRateLimiter() error: %v", err)
	}

	rl, ok := mw.(*RateLimiter)
	if !ok {
		t.Fatalf("Expected *RateLimiter, got %T", mw)
	}
	defer rl.StopCleanup()

	callCount := 0
	nextHandler := func(ctx *fasthttp.RequestCtx) {
		callCount++
	}

	handler := rl.Process(nextHandler)

	// 第一个请求应该成功
	ctx1 := testutil.NewRequestCtx("GET", "/test")
	handler(ctx1)

	// 第二个请求应该被限流（使用不同的 context）
	ctx2 := testutil.NewRequestCtx("GET", "/test")
	handler(ctx2)

	if callCount != 1 {
		t.Errorf("Expected next handler to be called once, got %d", callCount)
	}

	// 检查第二个请求的状态码
	if ctx2.Response.StatusCode() != 429 {
		t.Errorf("Status = %d, want 429", ctx2.Response.StatusCode())
	}
}

// TestKeyByIP_Unknown 测试无法获取 IP 的情况
func TestKeyByIP_Unknown(t *testing.T) {
	// 创建一个没有设置远程地址的上下文
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/test")

	key := keyByIP(ctx)
	// netutil.ExtractClientIPNet 会返回默认值 0.0.0.0 而不是 nil
	// 所以这里验证返回值不是空的即可
	if key == "" {
		t.Error("keyByIP() should return non-empty string")
	}
}
