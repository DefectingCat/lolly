// Package security 提供了 Lolly HTTP 服务器的安全相关中间件。
//
// 该文件实现了基于令牌桶算法的请求速率限制中间件，
// 支持按 IP 或按键值进行请求限流和连接数限制。
//
// 主要功能：
//   - 请求速率限制：使用令牌桶算法控制请求频率
//   - 突发流量处理：允许一定程度的请求突发
//   - 多维度限流：支持按 IP、按头部键值等维度
//   - 连接数限制：控制最大并发连接数
//
// 使用示例：
//
//	cfg := &config.RateLimitConfig{
//	    RequestRate: 100,  // 每秒 100 个请求
//	    Burst:       200,  // 允许突发到 200 个请求
//	    Key:         "ip", // 按 IP 地址限流
//	}
//
//	limiter, err := security.NewRateLimiter(cfg)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// 作为中间件应用
//	chain := middleware.NewChain(limiter)
//	handler := chain.Apply(finalHandler)
//
// 作者：xfy
package security

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/config"
	"rua.plus/lolly/internal/middleware"
)

// RateLimiter 基于令牌桶算法的请求速率限制器。
//
// 实现请求限流功能，支持按 IP 或自定义键值进行限流。
// 令牌按配置的速率持续添加，每个请求消耗一个令牌。
//
// 注意事项：
//   - 所有方法均为并发安全
//   - 应定期调用 Cleanup 清理过期的桶
type RateLimiter struct {
	rate    float64                 // 每秒添加的令牌数
	burst   float64                 // 桶的最大容量
	keyFunc KeyFunc                 // 提取限流键的函数
	buckets map[string]*tokenBucket // 各键的令牌桶映射
	mu      sync.RWMutex            // 读写锁，保护并发访问
}

// tokenBucket 表示单个限流键的令牌桶。
//
// 记录当前令牌数和最后更新时间，用于令牌计算。
type tokenBucket struct {
	tokens     float64    // 当前令牌数量
	lastUpdate time.Time  // 最后更新时间
	mu         sync.Mutex // 互斥锁，保护桶内状态
}

// KeyFunc 从请求中提取限流键的函数类型。
//
// 用于确定请求属于哪个限流桶，常见的实现包括按 IP、按头部值等。
type KeyFunc func(ctx *fasthttp.RequestCtx) string

// NewRateLimiter 根据配置创建新的速率限制器。
//
// 验证配置参数的有效性，并设置相应的限流键提取函数。
//
// 参数：
//   - cfg: 限流配置，包含速率、突发量和键类型
//
// 返回值：
//   - *RateLimiter: 配置好的限流器实例
//   - error: 配置无效时返回错误（如速率小于 0）
func NewRateLimiter(cfg *config.RateLimitConfig) (middleware.Middleware, error) {
	if cfg == nil {
		return nil, errors.New("rate limit config is nil")
	}

	if cfg.RequestRate <= 0 {
		return nil, errors.New("request rate must be positive")
	}

	// 根据算法选择限流器
	algorithm := cfg.Algorithm
	if algorithm == "" {
		algorithm = "token_bucket" // 默认令牌桶
	}

	switch algorithm {
	case "token_bucket", "":
		return newTokenBucketLimiter(cfg)
	case "sliding_window":
		window := time.Duration(cfg.SlidingWindow) * time.Second
		if window <= 0 {
			window = time.Second // 默认 1 秒窗口
		}
		precise := cfg.SlidingWindowMode == "precise"
		return NewSlidingWindowLimiterWrapper(cfg, window, precise)
	default:
		return nil, fmt.Errorf("unknown algorithm: %s", algorithm)
	}
}

// newTokenBucketLimiter 创建令牌桶限流器。
func newTokenBucketLimiter(cfg *config.RateLimitConfig) (*RateLimiter, error) {
	if cfg.Burst < cfg.RequestRate {
		return nil, errors.New("burst must be at least equal to request rate")
	}

	rl := &RateLimiter{
		rate:    float64(cfg.RequestRate),
		burst:   float64(cfg.Burst),
		buckets: make(map[string]*tokenBucket),
	}

	// 根据配置设置键提取函数
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

// SlidingWindowLimiterWrapper 滑动窗口限流器包装，实现 middleware.Middleware 接口。
type SlidingWindowLimiterWrapper struct {
	limiter *SlidingWindowLimiter
	keyFunc KeyFunc
}

// NewSlidingWindowLimiterWrapper 创建滑动窗口限流器包装。
func NewSlidingWindowLimiterWrapper(cfg *config.RateLimitConfig, window time.Duration, precise bool) (*SlidingWindowLimiterWrapper, error) {
	var keyFunc KeyFunc
	switch cfg.Key {
	case "ip", "":
		keyFunc = keyByIP
	case "header":
		keyFunc = keyByHeader
	default:
		return nil, fmt.Errorf("unknown key type: %s", cfg.Key)
	}

	return &SlidingWindowLimiterWrapper{
		limiter: NewSlidingWindowLimiter(window, cfg.RequestRate, precise),
		keyFunc: keyFunc,
	}, nil
}

// Name 返回中间件名称。
func (s *SlidingWindowLimiterWrapper) Name() string {
	return "sliding_window_limiter"
}

// Process 包装下一个处理器，添加限流逻辑。
func (s *SlidingWindowLimiterWrapper) Process(next fasthttp.RequestHandler) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		key := s.keyFunc(ctx)

		if !s.limiter.Allow(key) {
			ctx.Error("Too Many Requests", fasthttp.StatusTooManyRequests)
			return
		}

		next(ctx)
	}
}

// Name 返回中间件名称。
//
// 返回值：
//   - string: 中间件标识名 "rate_limiter"
func (rl *RateLimiter) Name() string {
	return "rate_limiter"
}

// Process 包装下一个处理器，添加限流逻辑。
//
// 超过限流阈值的请求将收到 429 Too Many Requests 响应，
// 并在响应头中设置 Retry-After 提示重试等待时间。
//
// 参数：
//   - next: 下一个请求处理器
//
// 返回值：
//   - fasthttp.RequestHandler: 包装后的处理器
func (rl *RateLimiter) Process(next fasthttp.RequestHandler) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		key := rl.keyFunc(ctx)

		if !rl.Allow(key) {
			// 计算重试等待时间
			retryAfter := rl.getRetryAfter(key)
			ctx.Response.Header.Set("Retry-After", fmt.Sprintf("%d", retryAfter))
			ctx.Error("Too Many Requests", fasthttp.StatusTooManyRequests)
			return
		}

		next(ctx)
	}
}

// Allow 检查给定键的请求是否应被允许。
//
// 使用令牌桶算法：每个请求消耗一个令牌，令牌按速率持续补充。
// 如果桶中有足够令牌则允许请求，否则拒绝。
//
// 参数：
//   - key: 限流键（如 IP 地址）
//
// 返回值：
//   - bool: true 表示允许请求，false 表示拒绝
func (rl *RateLimiter) Allow(key string) bool {
	rl.mu.RLock()
	bucket, exists := rl.buckets[key]
	rl.mu.RUnlock()

	if !exists {
		rl.mu.Lock()
		// 获取写锁后再次检查
		if bucket, exists = rl.buckets[key]; !exists {
			bucket = &tokenBucket{
				tokens:     rl.burst, // 初始满桶
				lastUpdate: time.Now(),
			}
			rl.buckets[key] = bucket
		}
		rl.mu.Unlock()
	}

	return bucket.consume(rl.rate, rl.burst)
}

// consume 尝试从桶中消耗一个令牌。
//
// 根据时间流逝补充令牌，然后检查是否有足够令牌消耗。
//
// 参数：
//   - rate: 令牌补充速率（每秒）
//   - burst: 桶的最大容量
//
// 返回值：
//   - bool: true 表示成功消耗令牌，false 表示桶空
func (tb *tokenBucket) consume(rate, burst float64) bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(tb.lastUpdate).Seconds()

	// 根据时间流逝补充令牌
	tb.tokens += elapsed * rate
	if tb.tokens > burst {
		tb.tokens = burst // 不超过桶容量
	}

	tb.lastUpdate = now

	// 检查是否有足够令牌
	if tb.tokens >= 1.0 {
		tb.tokens -= 1.0
		return true
	}

	return false
}

// getRetryAfter 计算重试前需等待的秒数。
//
// 根据令牌桶当前状态计算需要等待的时间，
// 包括生成一个令牌的时间和补偿欠缺令牌的时间。
//
// 参数：
//   - key: 限流键
//
// 返回值：
//   - int64: 建议等待的秒数
func (rl *RateLimiter) getRetryAfter(key string) int64 {
	rl.mu.RLock()
	bucket, exists := rl.buckets[key]
	rl.mu.RUnlock()

	if !exists {
		return 1
	}

	bucket.mu.Lock()
	defer bucket.mu.Unlock()

	// 生成一个令牌的时间
	waitTime := 1.0 / rl.rate
	// 如果桶欠缺令牌，需额外等待时间
	if bucket.tokens < 0 {
		waitTime += -bucket.tokens / rl.rate
	}

	return int64(waitTime) + 1
}

// keyByIP 提取客户端 IP 作为限流键。
//
// 从请求上下文获取客户端的真实 IP 地址。
//
// 参数：
//   - ctx: FastHTTP 请求上下文
//
// 返回值：
//   - string: IP 地址字符串，无法获取时返回 "unknown"
func keyByIP(ctx *fasthttp.RequestCtx) string {
	ip := extractClientIP(ctx)
	if ip == nil {
		return "unknown"
	}
	return ip.String()
}

// extractClientIP 从请求上下文提取客户端 IP。
//
// 按优先级依次检查：X-Forwarded-For、X-Real-IP、RemoteAddr。
//
// 参数：
//   - ctx: FastHTTP 请求上下文
//
// 返回值：
//   - net.IP: 客户端 IP 地址，无法获取时返回 nil
func extractClientIP(ctx *fasthttp.RequestCtx) net.IP {
	// 优先检查 X-Forwarded-For 头部
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

	// 检查 X-Real-IP 头部
	if xri := ctx.Request.Header.Peek("X-Real-IP"); len(xri) > 0 {
		ip := net.ParseIP(string(xri))
		if ip != nil {
			return ip
		}
	}

	// 回退到 RemoteAddr
	if addr := ctx.RemoteAddr(); addr != nil {
		if tcpAddr, ok := addr.(*net.TCPAddr); ok {
			return tcpAddr.IP
		}
	}

	return nil
}

// keyByHeader 提取头部值作为限流键。
//
// 默认使用 X-RateLimit-Key 头部，如果不存在则回退到 IP。
//
// 参数：
//   - ctx: FastHTTP 请求上下文
//
// 返回值：
//   - string: 头部值或 IP 地址字符串
func keyByHeader(ctx *fasthttp.RequestCtx) string {
	key := ctx.Request.Header.Peek("X-RateLimit-Key")
	if len(key) == 0 {
		// 头部不存在时回退到 IP
		return keyByIP(ctx)
	}
	return string(key)
}

// Reset 重置指定键的令牌桶。
//
// 删除该键的桶记录，下次请求时将重新创建满载的桶。
//
// 参数：
//   - key: 要重置的限流键
func (rl *RateLimiter) Reset(key string) {
	rl.mu.Lock()
	delete(rl.buckets, key)
	rl.mu.Unlock()
}

// ResetAll 重置所有令牌桶。
//
// 清空所有桶记录，所有客户端将重新开始计数。
func (rl *RateLimiter) ResetAll() {
	rl.mu.Lock()
	rl.buckets = make(map[string]*tokenBucket)
	rl.mu.Unlock()
}

// Cleanup 清理长时间未使用的令牌桶。
//
// 删除超过 maxAge 时间未更新的桶，防止内存无限增长。
// 建议定期调用此方法（如每分钟一次）。
//
// 参数：
//   - maxAge: 未使用桶的最大保留时间
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

// RateLimitStats 速率限制器统计信息。
type RateLimitStats struct {
	BucketCount int     // 当前活跃的桶数量
	Rate        float64 // 令牌补充速率（每秒）
	Burst       float64 // 桶的最大容量
}

// GetStats 返回当前速率限制器的统计信息。
//
// 返回值：
//   - RateLimitStats: 包含桶数量、速率和容量的统计对象
func (rl *RateLimiter) GetStats() RateLimitStats {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	return RateLimitStats{
		BucketCount: len(rl.buckets),
		Rate:        rl.rate,
		Burst:       rl.burst,
	}
}

// ConnLimiter 连接数限制器。
//
// 控制最大并发连接数，支持全局限制或按键值限制。
// 与 RateLimiter 不同，此限制器控制并发而非速率。
//
// 注意事项：
//   - 使用后必须调用 Release 释放连接槽
//   - 所有方法均为并发安全
type ConnLimiter struct {
	max     int              // 最大并发连接数
	current int64            // 当前连接数（原子操作）
	perKey  bool             // 是否按键限制，false 为全局限制
	keyFunc KeyFunc          // 键提取函数
	counts  map[string]int64 // 各键的连接数计数
	mu      sync.RWMutex     // 读写锁
}

// NewConnLimiter 创建新的连接数限制器。
//
// 参数：
//   - max: 最大并发连接数
//   - perKey: true 为按键限制，false 为全局限制
//   - keyType: 按键限制时的键类型（"ip" 或 "header"）
//
// 返回值：
//   - *ConnLimiter: 配置好的连接限制器
//   - error: 配置无效时返回错误
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

// Acquire 尝试获取一个连接槽。
//
// 如果当前连接数已达上限则返回 false。
// 成功获取后必须调用 Release 释放。
//
// 参数：
//   - ctx: FastHTTP 请求上下文
//
// 返回值：
//   - bool: true 表示成功获取，false 表示已达上限
func (cl *ConnLimiter) Acquire(ctx *fasthttp.RequestCtx) bool {
	if !cl.perKey {
		// 全局限制
		current := loadInt64(&cl.current)
		if current >= int64(cl.max) {
			return false
		}
		addInt64(&cl.current, 1)
		return true
	}

	// 按键限制
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

// Release 释放一个连接槽。
//
// 必须在连接结束时调用，否则连接数将持续增长。
//
// 参数：
//   - ctx: FastHTTP 请求上下文
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

// Middleware 返回连接限制的中间件包装。
//
// 返回值：
//   - middleware.Middleware: 可用于中间件链的限制器
func (cl *ConnLimiter) Middleware() middleware.Middleware {
	return &connLimiterMiddleware{limiter: cl}
}

// connLimiterMiddleware 连接限制器的中间件包装。
type connLimiterMiddleware struct {
	limiter *ConnLimiter // 连接限制器实例
}

// Name 返回中间件名称。
//
// 返回值：
//   - string: 中间件标识名 "conn_limiter"
func (m *connLimiterMiddleware) Name() string {
	return "conn_limiter"
}

// Process 包装处理器，添加连接限制逻辑。
//
// 获取连接槽后执行处理器，完成后自动释放。
// 超过连接限制时返回 503 Service Unavailable。
//
// 参数：
//   - next: 下一个请求处理器
//
// 返回值：
//   - fasthttp.RequestHandler: 包装后的处理器
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

// 连接数原子操作辅助函数
func loadInt64(ptr *int64) int64 {
	return atomic.LoadInt64(ptr)
}

func addInt64(ptr *int64, delta int64) {
	atomic.AddInt64(ptr, delta)
}

// 验证接口实现
// 验证接口实现
var _ middleware.Middleware = (*RateLimiter)(nil)
var _ middleware.Middleware = (*connLimiterMiddleware)(nil)
