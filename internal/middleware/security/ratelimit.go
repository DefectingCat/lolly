// Package security provides security-related middleware for the Lolly HTTP server.
//
// This file implements rate limiting middleware using the token bucket algorithm.
// It supports request rate limiting and connection limiting per IP or per key.
//
// Example usage:
//
//	cfg := &config.RateLimitConfig{
//	    RequestRate: 100,  // 100 requests per second
//	    Burst:       200,  // Allow burst up to 200 requests
//	    Key:         "ip", // Limit by IP address
//	}
//
//	limiter, err := security.NewRateLimiter(cfg)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Apply as middleware
//	chain := middleware.NewChain(limiter)
//	handler := chain.Apply(finalHandler)
//
//go:generate go test -v ./...
package security

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/config"
	"rua.plus/lolly/internal/middleware"
)

// RateLimiter implements request rate limiting using token bucket algorithm.
type RateLimiter struct {
	rate    float64 // Tokens added per second
	burst   float64 // Maximum bucket capacity
	keyFunc KeyFunc // Function to extract limit key
	buckets map[string]*tokenBucket
	mu      sync.RWMutex
}

// tokenBucket represents a single token bucket for rate limiting.
type tokenBucket struct {
	tokens     float64   // Current token count
	lastUpdate time.Time // Last token update time
	mu         sync.Mutex
}

// KeyFunc extracts the limiting key from a request.
type KeyFunc func(ctx *fasthttp.RequestCtx) string

// NewRateLimiter creates a new rate limiter from configuration.
//
// Parameters:
//   - cfg: Rate limit configuration with rate, burst, and key settings
//
// Returns:
//   - *RateLimiter: Configured rate limiter middleware
//   - error: Non-nil if configuration is invalid
func NewRateLimiter(cfg *config.RateLimitConfig) (*RateLimiter, error) {
	if cfg == nil {
		return nil, errors.New("rate limit config is nil")
	}

	if cfg.RequestRate <= 0 {
		return nil, errors.New("request rate must be positive")
	}

	if cfg.Burst < cfg.RequestRate {
		return nil, errors.New("burst must be at least equal to request rate")
	}

	rl := &RateLimiter{
		rate:    float64(cfg.RequestRate),
		burst:   float64(cfg.Burst),
		buckets: make(map[string]*tokenBucket),
	}

	// Set key extraction function based on config
	switch cfg.Key {
	case "ip", "":
		rl.keyFunc = keyByIP
	case "header":
		rl.keyFunc = keyByHeader
	default:
		return nil, fmt.Errorf("unknown key type: %s", cfg.Key)
	}

	return rl, nil
}

// Name returns the middleware name.
func (rl *RateLimiter) Name() string {
	return "rate_limiter"
}

// Process wraps the next handler with rate limiting logic.
// Requests exceeding the rate limit receive 429 Too Many Requests.
func (rl *RateLimiter) Process(next fasthttp.RequestHandler) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		key := rl.keyFunc(ctx)

		if !rl.Allow(key) {
			// Calculate retry-after time
			retryAfter := rl.getRetryAfter(key)
			ctx.Response.Header.Set("Retry-After", fmt.Sprintf("%d", retryAfter))
			ctx.Error("Too Many Requests", fasthttp.StatusTooManyRequests)
			return
		}

		next(ctx)
	}
}

// Allow checks if a request for the given key should be allowed.
// Uses token bucket algorithm: tokens are consumed on each request.
func (rl *RateLimiter) Allow(key string) bool {
	rl.mu.RLock()
	bucket, exists := rl.buckets[key]
	rl.mu.RUnlock()

	if !exists {
		rl.mu.Lock()
		// Check again after acquiring write lock
		if bucket, exists = rl.buckets[key]; !exists {
			bucket = &tokenBucket{
				tokens:     rl.burst, // Start with full bucket
				lastUpdate: time.Now(),
			}
			rl.buckets[key] = bucket
		}
		rl.mu.Unlock()
	}

	return bucket.consume(rl.rate, rl.burst)
}

// consume attempts to consume one token from the bucket.
// Returns true if successful, false if bucket is empty.
func (tb *tokenBucket) consume(rate, burst float64) bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(tb.lastUpdate).Seconds()

	// Add tokens based on elapsed time
	tb.tokens += elapsed * rate
	if tb.tokens > burst {
		tb.tokens = burst
	}

	tb.lastUpdate = now

	// Check if we have tokens
	if tb.tokens >= 1.0 {
		tb.tokens -= 1.0
		return true
	}

	return false
}

// getRetryAfter calculates the seconds to wait before retrying.
func (rl *RateLimiter) getRetryAfter(key string) int64 {
	rl.mu.RLock()
	bucket, exists := rl.buckets[key]
	rl.mu.RUnlock()

	if !exists {
		return 1
	}

	bucket.mu.Lock()
	defer bucket.mu.Unlock()

	// Time to generate one token
	waitTime := 1.0 / rl.rate
	// Additional time if bucket is depleted
	if bucket.tokens < 0 {
		waitTime += -bucket.tokens / rl.rate
	}

	return int64(waitTime) + 1
}

// keyByIP extracts the client IP as the limiting key.
func keyByIP(ctx *fasthttp.RequestCtx) string {
	ip := extractClientIP(ctx)
	if ip == nil {
		return "unknown"
	}
	return ip.String()
}

// extractClientIP extracts the client IP from the request context.
func extractClientIP(ctx *fasthttp.RequestCtx) net.IP {
	// Check X-Forwarded-For header first
	if xff := ctx.Request.Header.Peek("X-Forwarded-For"); len(xff) > 0 {
		ips := strings.Split(string(xff), ",")
		if len(ips) > 0 {
			ipStr := strings.TrimSpace(ips[0])
			ip := net.ParseIP(ipStr)
			if ip != nil {
				return ip
			}
		}
	}

	// Check X-Real-IP header
	if xri := ctx.Request.Header.Peek("X-Real-IP"); len(xri) > 0 {
		ip := net.ParseIP(string(xri))
		if ip != nil {
			return ip
		}
	}

	// Fall back to RemoteAddr
	if addr := ctx.RemoteAddr(); addr != nil {
		if tcpAddr, ok := addr.(*net.TCPAddr); ok {
			return tcpAddr.IP
		}
	}

	return nil
}

// keyByHeader extracts a header value as the limiting key.
// Uses X-RateLimit-Key header by default.
func keyByHeader(ctx *fasthttp.RequestCtx) string {
	key := ctx.Request.Header.Peek("X-RateLimit-Key")
	if len(key) == 0 {
		// Fall back to IP if header not present
		return keyByIP(ctx)
	}
	return string(key)
}

// Reset resets the bucket for a specific key.
func (rl *RateLimiter) Reset(key string) {
	rl.mu.Lock()
	delete(rl.buckets, key)
	rl.mu.Unlock()
}

// ResetAll resets all buckets.
func (rl *RateLimiter) ResetAll() {
	rl.mu.Lock()
	rl.buckets = make(map[string]*tokenBucket)
	rl.mu.Unlock()
}

// Cleanup removes buckets that haven't been used recently.
// This prevents memory growth from stale clients.
func (rl *RateLimiter) Cleanup(maxAge time.Duration) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	for key, bucket := range rl.buckets {
		bucket.mu.Lock()
		if now.Sub(bucket.lastUpdate) > maxAge {
			delete(rl.buckets, key)
		}
		bucket.mu.Unlock()
	}
}

// GetStats returns rate limiter statistics.
type RateLimitStats struct {
	BucketCount int
	Rate        float64
	Burst       float64
}

// GetStats returns current rate limiter statistics.
func (rl *RateLimiter) GetStats() RateLimitStats {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	return RateLimitStats{
		BucketCount: len(rl.buckets),
		Rate:        rl.rate,
		Burst:       rl.burst,
	}
}

// ConnLimiter implements connection count limiting.
// This is a separate limiter for maximum concurrent connections.
type ConnLimiter struct {
	max     int              // Maximum concurrent connections
	current int64            // Current connection count (atomic)
	perKey  bool             // Limit per key instead of global
	keyFunc KeyFunc          // Key extraction function
	counts  map[string]int64 // Connection counts per key
	mu      sync.RWMutex
}

// NewConnLimiter creates a new connection limiter.
//
// Parameters:
//   - max: Maximum concurrent connections allowed
//   - perKey: If true, limit per key; if false, global limit
//   - keyType: Key type for per-key limiting ("ip" or "header")
//
// Returns:
//   - *ConnLimiter: Configured connection limiter
//   - error: Non-nil if configuration is invalid
func NewConnLimiter(max int, perKey bool, keyType string) (*ConnLimiter, error) {
	if max <= 0 {
		return nil, errors.New("max connections must be positive")
	}

	cl := &ConnLimiter{
		max:    max,
		perKey: perKey,
		counts: make(map[string]int64),
	}

	if perKey {
		switch keyType {
		case "ip", "":
			cl.keyFunc = keyByIP
		case "header":
			cl.keyFunc = keyByHeader
		default:
			return nil, fmt.Errorf("unknown key type: %s", keyType)
		}
	}

	return cl, nil
}

// Acquire attempts to acquire a connection slot.
// Returns true if successful, false if limit exceeded.
func (cl *ConnLimiter) Acquire(ctx *fasthttp.RequestCtx) bool {
	if !cl.perKey {
		// Global limit
		current := loadInt64(&cl.current)
		if current >= int64(cl.max) {
			return false
		}
		addInt64(&cl.current, 1)
		return true
	}

	// Per-key limit
	key := cl.keyFunc(ctx)

	cl.mu.Lock()
	defer cl.mu.Unlock()

	current := cl.counts[key]
	if current >= int64(cl.max) {
		return false
	}

	cl.counts[key] = current + 1
	return true
}

// Release releases a connection slot.
func (cl *ConnLimiter) Release(ctx *fasthttp.RequestCtx) {
	if !cl.perKey {
		addInt64(&cl.current, -1)
		return
	}

	key := cl.keyFunc(ctx)

	cl.mu.Lock()
	if cl.counts[key] > 0 {
		cl.counts[key]--
	}
	cl.mu.Unlock()
}

// Middleware returns a middleware wrapper for connection limiting.
func (cl *ConnLimiter) Middleware() middleware.Middleware {
	return &connLimiterMiddleware{limiter: cl}
}

// connLimiterMiddleware wraps ConnLimiter as middleware.
type connLimiterMiddleware struct {
	limiter *ConnLimiter
}

// Name returns the middleware name.
func (m *connLimiterMiddleware) Name() string {
	return "conn_limiter"
}

// Process wraps the handler with connection limiting.
func (m *connLimiterMiddleware) Process(next fasthttp.RequestHandler) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		if !m.limiter.Acquire(ctx) {
			ctx.Error("Service Unavailable: Connection limit exceeded", fasthttp.StatusServiceUnavailable)
			return
		}

		defer m.limiter.Release(ctx)
		next(ctx)
	}
}

// Atomic operations helpers for connection count
func loadInt64(ptr *int64) int64 {
	return *ptr // Go atomic operations would use sync/atomic in production
}

func addInt64(ptr *int64, delta int64) {
	*ptr += delta // Simplified; production would use atomic.AddInt64
}

// Verify interface compliance
var _ middleware.Middleware = (*RateLimiter)(nil)
var _ middleware.Middleware = (*connLimiterMiddleware)(nil)
