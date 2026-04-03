package security

import (
	"testing"
	"time"

	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/config"
)

func TestNewRateLimiter(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *config.RateLimitConfig
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
	rl, err := NewRateLimiter(&config.RateLimitConfig{
		RequestRate: 10,
		Burst:       10,
	})
	if err != nil {
		t.Fatalf("NewRateLimiter() error: %v", err)
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
	rl, err := NewRateLimiter(&config.RateLimitConfig{
		RequestRate: 100, // 100 tokens per second
		Burst:       100,
	})
	if err != nil {
		t.Fatalf("NewRateLimiter() error: %v", err)
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
	rl, err := NewRateLimiter(&config.RateLimitConfig{
		RequestRate: 1,
		Burst:       1,
	})
	if err != nil {
		t.Fatalf("NewRateLimiter() error: %v", err)
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
	rl, err := NewRateLimiter(&config.RateLimitConfig{
		RequestRate: 1,
		Burst:       1,
	})
	if err != nil {
		t.Fatalf("NewRateLimiter() error: %v", err)
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
	rl, err := NewRateLimiter(&config.RateLimitConfig{
		RequestRate: 100,
		Burst:       100,
	})
	if err != nil {
		t.Fatalf("NewRateLimiter() error: %v", err)
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
	rl, err := NewRateLimiter(&config.RateLimitConfig{
		RequestRate: 100,
		Burst:       100,
	})
	if err != nil {
		t.Fatalf("NewRateLimiter() error: %v", err)
	}

	nextHandler := func(ctx *fasthttp.RequestCtx) {
		ctx.WriteString("OK")
	}

	handler := rl.Process(nextHandler)
	if handler == nil {
		t.Error("Process() returned nil handler")
	}
}

func TestRateLimiterGetStats(t *testing.T) {
	rl, err := NewRateLimiter(&config.RateLimitConfig{
		RequestRate: 100,
		Burst:       200,
	})
	if err != nil {
		t.Fatalf("NewRateLimiter() error: %v", err)
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
}

func TestNewConnLimiter(t *testing.T) {
	tests := []struct {
		name    string
		max     int
		perKey  bool
		keyType string
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

	ctx := &fasthttp.RequestCtx{}

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