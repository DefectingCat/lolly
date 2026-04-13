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
