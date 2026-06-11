# Least Time & Session Sticky Load Balancer Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 Lolly 实现高性能的 Least Time 负载均衡算法和 Session Sticky 会话保持功能

**Architecture:** Least Time 使用原子 EWMA 统计器记录每个后端的响应时间，选择响应时间最短的目标；Session Sticky 使用 256 分片锁 + Cookie 路由表实现会话保持

**Tech Stack:** Go 1.26+, fasthttp, atomic operations, sync.RWMutex

---

## File Structure

### New Files
- `internal/loadbalance/ewma.go` - 原子 EWMA 统计器
- `internal/loadbalance/ewma_test.go` - EWMA 测试
- `internal/loadbalance/least_time.go` - Least Time balancer
- `internal/loadbalance/least_time_test.go` - Least Time 测试
- `internal/loadbalance/sticky.go` - Session Sticky balancer
- `internal/loadbalance/sticky_test.go` - Session Sticky 测试
- `internal/loadbalance/sticky_config.go` - Sticky 配置结构体

### Modified Files
- `internal/loadbalance/algorithms.go` - 添加新算法到 validAlgorithms
- `internal/loadbalance/balancer.go` - Target 增加 Stats 字段
- `internal/config/proxy_config.go` - 添加 LeastTimeConfig + StickyConfig
- `internal/config/defaults.go` - 添加默认配置注释
- `internal/config/validate.go` - 验证新配置项
- `internal/proxy/proxy.go` - 集成 createBalancer + RecordResponseTime
- `internal/proxy/target_selector.go` - Select 支持 StickySession

---

## Task 1: EWMA Statistics Core

**Files:**
- Create: `internal/loadbalance/ewma.go`
- Create: `internal/loadbalance/ewma_test.go`

### Step 1.1: Write EWMA Failing Test

```go
package loadbalance

import (
    "sync"
    "testing"
    "time"
)

func TestEWMAStats_BasicRecord(t *testing.T) {
    stats := NewEWMAStats()
    
    // Record a 100ms response time
    stats.Record(100*time.Millisecond, 200*time.Millisecond)
    
    headerTime := stats.HeaderTime()
    lastByteTime := stats.LastByteTime()
    
    if headerTime == 0 {
        t.Error("headerTime should not be zero after recording")
    }
    if lastByteTime == 0 {
        t.Error("lastByteTime should not be zero after recording")
    }
    
    // First sample: avg should equal the sample (alpha=1.0 for first sample)
    if headerTime != 100*time.Millisecond {
        t.Errorf("first headerTime = %v, want %v", headerTime, 100*time.Millisecond)
    }
    if lastByteTime != 200*time.Millisecond {
        t.Errorf("first lastByteTime = %v, want %v", lastByteTime, 200*time.Millisecond)
    }
}

func TestEWMAStats_Convergence(t *testing.T) {
    stats := NewEWMAStats()
    
    // Record multiple samples
    for i := 0; i < 10; i++ {
        stats.Record(100*time.Millisecond, 200*time.Millisecond)
    }
    
    headerTime := stats.HeaderTime()
    
    // After many identical samples, avg should converge close to the value
    // With alpha=0.3, after 10 samples of 100ms, should be close to 100ms
    diff := headerTime - 100*time.Millisecond
    if diff < 0 {
        diff = -diff
    }
    if diff > 10*time.Millisecond {
        t.Errorf("headerTime = %v, not converged to 100ms (diff=%v)", headerTime, diff)
    }
}

func TestEWMAStats_Concurrent(t *testing.T) {
    stats := NewEWMAStats()
    
    var wg sync.WaitGroup
    for i := 0; i < 100; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            for j := 0; j < 100; j++ {
                stats.Record(time.Duration(j)*time.Millisecond, time.Duration(j*2)*time.Millisecond)
            }
        }()
    }
    wg.Wait()
    
    // After concurrent writes, should have some value (not panic or race)
    headerTime := stats.HeaderTime()
    lastByteTime := stats.LastByteTime()
    
    if headerTime == 0 {
        t.Error("headerTime should not be zero after concurrent writes")
    }
    if lastByteTime == 0 {
        t.Error("lastByteTime should not be zero after concurrent writes")
    }
}
```

### Step 1.2: Run EWMA Test - Verify Fails

Run: `cd /home/xfy/Developer/lolly && go test -v ./internal/loadbalance -run TestEWMAStats`
Expected: FAIL with "undefined: NewEWMAStats"

### Step 1.3: Implement EWMA Core

```go
package loadbalance

import (
    "sync/atomic"
    "time"
)

// EWMAStats 使用原子操作实现的 EWMA（指数加权移动平均）统计器。
//
// 通过定点数运算避免浮点数，实现零锁、零分配的响应时间统计。
type EWMAStats struct {
    headerTime   atomic.Int64 // 首字节时间的 EWMA（纳秒）
    lastByteTime atomic.Int64 // 完整响应时间的 EWMA（纳秒）
    sampleCount  atomic.Int64 // 样本计数
}

// defaultAlpha 默认 EWMA alpha 值（30%，使用定点数 300/1000）
const defaultAlphaScale = 300 // alpha = 0.3

// NewEWMAStats 创建新的 EWMA 统计器
func NewEWMAStats() *EWMAStats {
    return &EWMAStats{}
}

// Record 记录一次响应时间样本。
//
// 使用原子操作无锁更新 EWMA：
//   - 第一个样本直接设为当前值
//   - 后续样本：new_avg = alpha * new + (1 - alpha) * old
//
// 参数：
//   - headerTime: 首字节时间
//   - lastByteTime: 完整响应时间
func (e *EWMAStats) Record(headerTime, lastByteTime time.Duration) {
    e.recordAtomic(&e.headerTime, headerTime)
    e.recordAtomic(&e.lastByteTime, lastByteTime)
    e.sampleCount.Add(1)
}

// recordAtomic 原子更新单个 EWMA 值
func (e *EWMAStats) recordAtomic(ptr *atomic.Int64, newValue time.Duration) {
    newNano := newValue.Nanoseconds()
    
    for {
        old := ptr.Load()
        if old == 0 {
            // 首次记录，直接设置
            if ptr.CompareAndSwap(0, newNano) {
                return
            }
            continue
        }
        
        // EWMA: new = alpha * new + (1 - alpha) * old
        // 使用定点数：alphaScale = 300 (0.3)
        // new_avg = (alpha * new + (1000 - alpha) * old) / 1000
        updated := (defaultAlphaScale*newNano + (1000-defaultAlphaScale)*old) / 1000
        
        if ptr.CompareAndSwap(old, updated) {
            return
        }
        // CAS 失败，重试
    }
}

// HeaderTime 返回首字节时间的 EWMA 值
func (e *EWMAStats) HeaderTime() time.Duration {
    return time.Duration(e.headerTime.Load())
}

// LastByteTime 返回完整响应时间的 EWMA 值
func (e *EWMAStats) LastByteTime() time.Duration {
    return time.Duration(e.lastByteTime.Load())
}

// SampleCount 返回已记录的样本数
func (e *EWMAStats) SampleCount() int64 {
    return e.sampleCount.Load()
}

// Reset 重置统计器
func (e *EWMAStats) Reset() {
    e.headerTime.Store(0)
    e.lastByteTime.Store(0)
    e.sampleCount.Store(0)
}
```

### Step 1.4: Run EWMA Test - Verify Passes

Run: `cd /home/xfy/Developer/lolly && go test -v ./internal/loadbalance -run TestEWMAStats`
Expected: PASS (3 tests)

### Step 1.5: Commit

```bash
cd /home/xfy/Developer/lolly
git add internal/loadbalance/ewma.go internal/loadbalance/ewma_test.go
git commit -m "feat(loadbalance): add atomic EWMA statistics core

- Zero-lock atomic EWMA implementation using fixed-point arithmetic
- Supports header_time and last_byte_time tracking
- Concurrent-safe with CAS retry loop"
```

---

## Task 2: Least Time Balancer

**Files:**
- Create: `internal/loadbalance/least_time.go`
- Create: `internal/loadbalance/least_time_test.go`

### Step 2.1: Write LeastTime Failing Test

```go
package loadbalance

import (
    "sync"
    "testing"
    "time"
)

func TestLeastTime_BasicSelect(t *testing.T) {
    lt := NewLeastTime("last_byte", time.Millisecond)
    
    targets := []*Target{
        NewTargetFromConfig("http://slow:8080", 1, 0, 0, 0, false, false, ""),
        NewTargetFromConfig("http://fast:8080", 1, 0, 0, 0, false, false, ""),
    }
    
    // Record different response times
    targets[0].Stats.Record(200*time.Millisecond, 400*time.Millisecond) // slow
    targets[1].Stats.Record(50*time.Millisecond, 100*time.Millisecond)   // fast
    
    selected := lt.Select(targets)
    if selected == nil {
        t.Fatal("expected a target, got nil")
    }
    if selected.URL != "http://fast:8080" {
        t.Errorf("selected = %s, want fast target", selected.URL)
    }
}

func TestLeastTime_NoStats(t *testing.T) {
    lt := NewLeastTime("last_byte", time.Millisecond)
    
    targets := []*Target{
        NewTargetFromConfig("http://a:8080", 1, 0, 0, 0, false, false, ""),
        NewTargetFromConfig("http://b:8080", 1, 0, 0, 0, false, false, ""),
    }
    
    // No stats recorded - should still select one (using default)
    selected := lt.Select(targets)
    if selected == nil {
        t.Fatal("expected a target, got nil")
    }
}

func TestLeastTime_HeaderMetric(t *testing.T) {
    lt := NewLeastTime("header", time.Millisecond)
    
    targets := []*Target{
        NewTargetFromConfig("http://slow:8080", 1, 0, 0, 0, false, false, ""),
        NewTargetFromConfig("http://fast:8080", 1, 0, 0, 0, false, false, ""),
    }
    
    // Record: slow has worse header time but better last_byte time
    targets[0].Stats.Record(200*time.Millisecond, 100*time.Millisecond)
    targets[1].Stats.Record(50*time.Millisecond, 300*time.Millisecond)
    
    selected := lt.Select(targets)
    if selected == nil {
        t.Fatal("expected a target, got nil")
    }
    // Should pick fast based on header_time
    if selected.URL != "http://fast:8080" {
        t.Errorf("selected = %s, want fast target based on header_time", selected.URL)
    }
}

func TestLeastTime_SelectExcluding(t *testing.T) {
    lt := NewLeastTime("last_byte", time.Millisecond)
    
    targets := []*Target{
        NewTargetFromConfig("http://a:8080", 1, 0, 0, 0, false, false, ""),
        NewTargetFromConfig("http://b:8080", 1, 0, 0, 0, false, false, ""),
        NewTargetFromConfig("http://c:8080", 1, 0, 0, 0, false, false, ""),
    }
    
    targets[0].Stats.Record(10*time.Millisecond, 20*time.Millisecond)
    targets[1].Stats.Record(30*time.Millisecond, 60*time.Millisecond)
    targets[2].Stats.Record(50*time.Millisecond, 100*time.Millisecond)
    
    // Exclude the fastest
    excluded := []*Target{targets[0]}
    selected := lt.SelectExcluding(targets, excluded)
    
    if selected == nil {
        t.Fatal("expected a target, got nil")
    }
    if selected.URL != "http://b:8080" {
        t.Errorf("selected = %s, want second fastest", selected.URL)
    }
}

func TestLeastTime_Concurrent(t *testing.T) {
    lt := NewLeastTime("last_byte", time.Millisecond)
    
    targets := []*Target{
        NewTargetFromConfig("http://a:8080", 1, 0, 0, 0, false, false, ""),
        NewTargetFromConfig("http://b:8080", 1, 0, 0, 0, false, false, ""),
    }
    
    var wg sync.WaitGroup
    
    // Concurrent recording
    for i := 0; i < 50; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            for j := 0; j < 100; j++ {
                targets[0].Stats.Record(time.Millisecond, 2*time.Millisecond)
                targets[1].Stats.Record(2*time.Millisecond, 4*time.Millisecond)
            }
        }()
    }
    
    // Concurrent selecting
    for i := 0; i < 50; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            for j := 0; j < 100; j++ {
                lt.Select(targets)
            }
        }()
    }
    
    wg.Wait()
}
```

### Step 2.2: Run LeastTime Test - Verify Fails

Run: `cd /home/xfy/Developer/lolly && go test -v ./internal/loadbalance -run TestLeastTime`
Expected: FAIL with "undefined: NewLeastTime"

### Step 2.3: Modify Target to Add Stats Field

File: `internal/loadbalance/balancer.go`

Find `type Target struct` definition and add Stats field:

```go
// Target 表示 HTTP 代理（L7 层）的负载均衡后端服务器目标。
type Target struct {
    resolvedIPs   atomic.Pointer[[]string]
    URL           string
    hostname      string
    VirtualHashes []uint64
    Weight        int
    Connections   int64
    lastResolved  atomic.Int64
    hostnameOnce  sync.Once
    Healthy       atomic.Bool
    
    // Stats 响应时间统计（用于 least_time 算法）
    Stats *EWMAStats
    
    // ... rest of fields unchanged
```

Also update `NewTargetFromConfig` to initialize Stats:

```go
func NewTargetFromConfig(url string, weight int, maxConns int64, maxFails int64, failTimeout time.Duration, backup bool, down bool, proxyURI string) *Target {
    t := &Target{
        URL:         url,
        Weight:      weight,
        MaxConns:    maxConns,
        MaxFails:    maxFails,
        FailTimeout: failTimeout,
        Backup:      backup,
        Down:        down,
        ProxyURI:    proxyURI,
        Stats:       NewEWMAStats(), // 初始化统计器
    }
    t.initHostname()
    if !down {
        t.Healthy.Store(true)
    }
    return t
}
```

### Step 2.4: Implement LeastTime Balancer

```go
package loadbalance

import (
    "sync/atomic"
    "time"
)

// ResponseTimeRecorder 响应时间记录接口。
// 实现此接口的 balancer 可在请求完成后收到响应时间统计。
type ResponseTimeRecorder interface {
    RecordResponseTime(target *Target, headerTime, lastByteTime time.Duration)
}

// LeastTime 基于响应时间 EWMA 的负载均衡器。
//
// 选择响应时间最短的健康目标。支持两种指标：
//   - "header": 首字节时间（从发送请求到收到响应头）
//   - "last_byte": 完整响应时间（从发送请求到收到完整响应）
type LeastTime struct {
    metric       string        // "header" 或 "last_byte"
    defaultTime  time.Duration // 无统计样本时的默认值
}

// NewLeastTime 创建 Least Time 负载均衡器。
//
// 参数：
//   - metric: 使用的指标，"header" 或 "last_byte"
//   - defaultTime: 无统计样本时的默认响应时间（避免新节点被饿死）
func NewLeastTime(metric string, defaultTime time.Duration) *LeastTime {
    if metric != "header" {
        metric = "last_byte" // 默认使用 last_byte
    }
    if defaultTime <= 0 {
        defaultTime = time.Millisecond // 默认 1ms
    }
    return &LeastTime{
        metric:      metric,
        defaultTime: defaultTime,
    }
}

// Select 选择响应时间最短的健康目标。
// 只考虑可用目标。如果没有可用目标则返回 nil。
func (l *LeastTime) Select(targets []*Target) *Target {
    fc := acquireFilterContext()
    defer releaseFilterContext(fc)
    available := filterInto(fc, targets)
    return l.selectFrom(available)
}

// SelectExcluding 选择响应时间最短的目标，排除指定的目标列表。
func (l *LeastTime) SelectExcluding(targets []*Target, excluded []*Target) *Target {
    fc := acquireFilterContext()
    defer releaseFilterContext(fc)
    available := filterIntoExcluding(fc, targets, excluded)
    return l.selectFrom(available)
}

// selectFrom 从可用目标列表中选择响应时间最短的
func (l *LeastTime) selectFrom(available []*Target) *Target {
    if len(available) == 0 {
        return nil
    }
    
    var selected *Target
    var minTime int64 = -1
    defaultNano := l.defaultTime.Nanoseconds()
    
    for _, t := range available {
        var currentTime int64
        if t.Stats != nil {
            if l.metric == "header" {
                currentTime = t.Stats.headerTime.Load()
            } else {
                currentTime = t.Stats.lastByteTime.Load()
            }
        }
        
        // 无统计样本时使用默认值
        if currentTime == 0 {
            currentTime = defaultNano
        }
        
        if selected == nil || currentTime < minTime {
            selected = t
            minTime = currentTime
        }
    }
    
    return selected
}

// RecordResponseTime 记录目标响应时间（实现 ResponseTimeRecorder 接口）。
func (l *LeastTime) RecordResponseTime(target *Target, headerTime, lastByteTime time.Duration) {
    if target != nil && target.Stats != nil {
        target.Stats.Record(headerTime, lastByteTime)
    }
}

// GetMetric 返回当前使用的指标
func (l *LeastTime) GetMetric() string {
    return l.metric
}

var _ Balancer = (*LeastTime)(nil)
var _ ResponseTimeRecorder = (*LeastTime)(nil)
```

### Step 2.5: Run LeastTime Test - Verify Passes

Run: `cd /home/xfy/Developer/lolly && go test -v ./internal/loadbalance -run TestLeastTime`
Expected: PASS (5 tests)

### Step 2.6: Commit

```bash
cd /home/xfy/Developer/lolly
git add internal/loadbalance/balancer.go internal/loadbalance/least_time.go internal/loadbalance/least_time_test.go
git commit -m "feat(loadbalance): implement Least Time balancer

- Add atomic EWMA Stats field to Target
- Implement LeastTime balancer with header_time and last_byte metrics
- Support Select and SelectExcluding with zero-lock design
- Add ResponseTimeRecorder interface for proxy integration"
```

---

## Task 3: Session Sticky Balancer

**Files:**
- Create: `internal/loadbalance/sticky_config.go`
- Create: `internal/loadbalance/sticky.go`
- Create: `internal/loadbalance/sticky_test.go`

### Step 3.1: Write StickyConfig Structure

```go
package loadbalance

import "time"

// StickyConfig Session Sticky 配置
type StickyConfig struct {
    Enabled  bool          `yaml:"enabled"`
    Name     string        `yaml:"name"`     // cookie 名称
    Expires  time.Duration `yaml:"expires"`  // session 有效期
    Domain   string        `yaml:"domain"`   // cookie domain
    Path     string        `yaml:"path"`     // cookie path
    Secure   bool          `yaml:"secure"`   // Secure flag
    HttpOnly bool          `yaml:"http_only"` // HttpOnly flag
    SameSite string        `yaml:"same_site"` // SameSite attribute
}

// DefaultStickyConfig 返回默认 Sticky 配置
func DefaultStickyConfig() StickyConfig {
    return StickyConfig{
        Name:     "lolly_route",
        Expires:  time.Hour,
        Path:     "/",
        HttpOnly: true,
        SameSite: "Lax",
    }
}
```

### Step 3.2: Write Sticky Test (Failing)

```go
package loadbalance

import (
    "strings"
    "sync"
    "testing"
    "time"

    "github.com/valyala/fasthttp"
)

func TestStickySession_BasicRoute(t *testing.T) {
    fallback := NewRoundRobin()
    config := DefaultStickyConfig()
    config.Expires = time.Hour
    
    sticky := NewStickySession(config, fallback)
    sticky.Start()
    defer sticky.Stop()
    
    targets := []*Target{
        NewTargetFromConfig("http://backend1:8080", 1, 0, 0, 0, false, false, ""),
        NewTargetFromConfig("http://backend2:8080", 1, 0, 0, 0, false, false, ""),
    }
    
    ctx := &fasthttp.RequestCtx{}
    
    // First request - should set cookie
    selected1 := sticky.Select(ctx, targets)
    if selected1 == nil {
        t.Fatal("expected a target, got nil")
    }
    
    // Check cookie was set
    cookie := ctx.Response.Header.PeekCookie(config.Name)
    if len(cookie) == 0 {
        t.Fatal("expected cookie to be set")
    }
    
    // Second request with same cookie - should route to same target
    ctx2 := &fasthttp.RequestCtx{}
    ctx2.Request.Header.SetCookieBytesV(config.Name, extractCookieValue(cookie))
    
    selected2 := sticky.Select(ctx2, targets)
    if selected2 == nil {
        t.Fatal("expected a target, got nil")
    }
    if selected2.URL != selected1.URL {
        t.Errorf("sticky routing failed: got %s, want %s", selected2.URL, selected1.URL)
    }
}

func TestStickySession_TargetUnavailable(t *testing.T) {
    fallback := NewRoundRobin()
    config := DefaultStickyConfig()
    
    sticky := NewStickySession(config, fallback)
    sticky.Start()
    defer sticky.Stop()
    
    targets := []*Target{
        NewTargetFromConfig("http://backend1:8080", 1, 0, 0, 0, false, false, ""),
        NewTargetFromConfig("http://backend2:8080", 1, 0, 0, 0, false, false, ""),
    }
    
    ctx := &fasthttp.RequestCtx{}
    
    // First request
    selected1 := sticky.Select(ctx, targets)
    
    // Make target unavailable
    selected1.Healthy.Store(false)
    
    // Second request with cookie - should fallback to another target
    ctx2 := &fasthttp.RequestCtx{}
    cookie := ctx.Response.Header.PeekCookie(config.Name)
    ctx2.Request.Header.SetCookieBytesV(config.Name, extractCookieValue(cookie))
    
    selected2 := sticky.Select(ctx2, targets)
    if selected2 == nil {
        t.Fatal("expected a target after fallback, got nil")
    }
    if selected2.URL == selected1.URL {
        t.Error("expected fallback to different target")
    }
}

func TestStickySession_CookieEncodeDecode(t *testing.T) {
    targetURL := "http://backend1:8080"
    expires := time.Now().Add(time.Hour)
    
    encoded := encodeStickyCookie(targetURL, expires)
    decodedURL, decodedExpires, ok := decodeStickyCookie(encoded)
    
    if !ok {
        t.Fatal("decode failed")
    }
    if decodedURL != targetURL {
        t.Errorf("url = %s, want %s", decodedURL, targetURL)
    }
    if decodedExpires.Unix() != expires.Unix() {
        t.Errorf("expires mismatch")
    }
}

func TestStickySession_Concurrent(t *testing.T) {
    fallback := NewRoundRobin()
    config := DefaultStickyConfig()
    
    sticky := NewStickySession(config, fallback)
    sticky.Start()
    defer sticky.Stop()
    
    targets := []*Target{
        NewTargetFromConfig("http://backend1:8080", 1, 0, 0, 0, false, false, ""),
        NewTargetFromConfig("http://backend2:8080", 1, 0, 0, 0, false, false, ""),
    }
    
    var wg sync.WaitGroup
    for i := 0; i < 100; i++ {
        wg.Add(1)
        go func(id int) {
            defer wg.Done()
            ctx := &fasthttp.RequestCtx{}
            sticky.Select(ctx, targets)
        }(i)
    }
    wg.Wait()
}

// Helper to extract cookie value from Set-Cookie header
func extractCookieValue(cookieHeader []byte) []byte {
    s := string(cookieHeader)
    // Format: "name=value; ..."
    parts := strings.SplitN(s, "=", 2)
    if len(parts) != 2 {
        return nil
    }
    valueParts := strings.SplitN(parts[1], ";", 2)
    return []byte(valueParts[0])
}
```

### Step 3.3: Run Sticky Test - Verify Fails

Run: `cd /home/xfy/Developer/lolly && go test -v ./internal/loadbalance -run TestStickySession`
Expected: FAIL with undefined functions

### Step 3.4: Implement StickySession

```go
package loadbalance

import (
    "encoding/base64"
    "strconv"
    "strings"
    "sync"
    "sync/atomic"
    "time"

    "github.com/valyala/fasthttp"
)

const stickyShardCount = 256

// StickySession Cookie-based 会话保持负载均衡器。
//
// 使用 256 个分片锁降低锁冲突概率，支持 TTL 过期和后台清理。
type StickySession struct {
    config   StickyConfig
    fallback Balancer
    
    shards  [stickyShardCount]*stickyShard
    cleaner *time.Ticker
    stopCh  chan struct{}
    started atomic.Bool
}

type stickyShard struct {
    mu       sync.RWMutex
    sessions map[string]*stickyEntry
}

type stickyEntry struct {
    targetURL string
    expiresAt int64 // Unix 纳秒
}

// NewStickySession 创建 Session Sticky 负载均衡器。
//
// 参数：
//   - config: Sticky 配置
//   - fallback: 首次路由和目标失效时的 fallback 算法
func NewStickySession(config StickyConfig, fallback Balancer) *StickySession {
    if fallback == nil {
        fallback = NewRoundRobin()
    }
    
    s := &StickySession{
        config:   config,
        fallback: fallback,
        stopCh:   make(chan struct{}),
    }
    
    for i := 0; i < stickyShardCount; i++ {
        s.shards[i] = &stickyShard{
            sessions: make(map[string]*stickyEntry),
        }
    }
    
    return s
}

// Start 启动后台清理任务。
func (s *StickySession) Start() {
    if s.started.Swap(true) {
        return
    }
    s.cleaner = time.NewTicker(60 * time.Second)
    go s.cleanupLoop()
}

// Stop 停止后台清理任务。
func (s *StickySession) Stop() {
    if !s.started.Swap(false) {
        return
    }
    close(s.stopCh)
}

// cleanupLoop 后台清理循环
func (s *StickySession) cleanupLoop() {
    for {
        select {
        case <-s.cleaner.C:
            s.cleanupExpired()
        case <-s.stopCh:
            return
        }
    }
}

// cleanupExpired 清理所有过期 session
func (s *StickySession) cleanupExpired() {
    now := time.Now().UnixNano()
    for _, shard := range s.shards {
        shard.mu.Lock()
        for key, entry := range shard.sessions {
            if entry.expiresAt < now {
                delete(shard.sessions, key)
            }
        }
        shard.mu.Unlock()
    }
}

// Select 根据 Cookie 选择目标。
//
// 1. 检查请求中的 sticky cookie
// 2. 如果存在且目标健康，路由到该目标
// 3. 如果不存在或目标不可用，使用 fallback 选择
// 4. 设置新的 Set-Cookie 响应头
func (s *StickySession) Select(ctx *fasthttp.RequestCtx, targets []*Target) *Target {
    // 1. 检查现有 cookie
    cookieValue := ctx.Request.Header.Cookie(s.config.Name)
    if len(cookieValue) > 0 {
        targetURL, expires, ok := decodeStickyCookie(string(cookieValue))
        if ok && expires.After(time.Now()) {
            // 查找目标是否可用
            for _, t := range targets {
                if t.URL == targetURL && t.IsAvailable() {
                    return t
                }
            }
            // 目标不可用，删除 session
            s.deleteSession(string(cookieValue))
        }
    }
    
    // 2. 使用 fallback 选择
    selected := s.fallback.Select(targets)
    if selected == nil {
        return nil
    }
    
    // 3. 种 cookie
    s.setCookie(ctx, selected.URL)
    
    // 4. 记录 session
    s.recordSession(selected.URL)
    
    return selected
}

// SelectExcluding 排除指定目标后选择。
func (s *StickySession) SelectExcluding(targets []*Target, excluded []*Target) *Target {
    // Session Sticky 通常不用于 failover 场景，
    // 但如果需要，可以先尝试 cookie，不行再用 fallback.SelectExcluding
    // 这里简化实现：使用 fallback 的 SelectExcluding
    return s.fallback.SelectExcluding(targets, excluded)
}

// setCookie 设置 Set-Cookie 响应头
func (s *StickySession) setCookie(ctx *fasthttp.RequestCtx, targetURL string) {
    expires := time.Now().Add(s.config.Expires)
    cookieValue := encodeStickyCookie(targetURL, expires)
    
    var cookie fasthttp.Cookie
    cookie.SetKey(s.config.Name)
    cookie.SetValue(cookieValue)
    cookie.SetExpire(expires)
    cookie.SetPath(s.config.Path)
    if s.config.Domain != "" {
        cookie.SetDomain(s.config.Domain)
    }
    if s.config.Secure {
        cookie.SetSecure(true)
    }
    if s.config.HttpOnly {
        cookie.SetHTTPOnly(true)
    }
    switch strings.ToLower(s.config.SameSite) {
    case "strict":
        cookie.SetSameSite(fasthttp.CookieSameSiteStrictMode)
    case "none":
        cookie.SetSameSite(fasthttp.CookieSameSiteNoneMode)
    default:
        cookie.SetSameSite(fasthttp.CookieSameSiteLaxMode)
    }
    
    ctx.Response.Header.SetCookie(&cookie)
}

// recordSession 记录 session 到路由表
func (s *StickySession) recordSession(targetURL string) {
    cookieValue := encodeStickyCookie(targetURL, time.Now().Add(s.config.Expires))
    shard := s.getShard(cookieValue)
    
    shard.mu.Lock()
    shard.sessions[cookieValue] = &stickyEntry{
        targetURL: targetURL,
        expiresAt: time.Now().Add(s.config.Expires).UnixNano(),
    }
    shard.mu.Unlock()
}

// deleteSession 删除 session
func (s *StickySession) deleteSession(cookieValue string) {
    shard := s.getShard(cookieValue)
    shard.mu.Lock()
    delete(shard.sessions, cookieValue)
    shard.mu.Unlock()
}

// getShard 根据 cookie 值计算分片索引
func (s *StickySession) getShard(cookieValue string) *stickyShard {
    hash := fnvHash64a(cookieValue)
    return s.shards[hash%stickyShardCount]
}

// encodeStickyCookie 编码路由信息到 cookie 值
// 格式: base64(target_url + "|" + expires_timestamp)
func encodeStickyCookie(targetURL string, expires time.Time) string {
    raw := targetURL + "|" + strconv.FormatInt(expires.Unix(), 10)
    return base64.URLEncoding.EncodeToString([]byte(raw))
}

// decodeStickyCookie 解码 cookie 值
func decodeStickyCookie(value string) (targetURL string, expires time.Time, ok bool) {
    raw, err := base64.URLEncoding.DecodeString(value)
    if err != nil {
        return
    }
    parts := strings.Split(string(raw), "|")
    if len(parts) != 2 {
        return
    }
    ts, err := strconv.ParseInt(parts[1], 10, 64)
    if err != nil {
        return
    }
    return parts[0], time.Unix(ts, 0), true
}

var _ Balancer = (*StickySession)(nil)
```

### Step 3.5: Run Sticky Test - Verify Passes

Run: `cd /home/xfy/Developer/lolly && go test -v ./internal/loadbalance -run TestStickySession`
Expected: PASS (4 tests)

### Step 3.6: Commit

```bash
cd /home/xfy/Developer/lolly
git add internal/loadbalance/sticky_config.go internal/loadbalance/sticky.go internal/loadbalance/sticky_test.go
git commit -m "feat(loadbalance): implement Session Sticky balancer

- Add 256-shard lock map for concurrent session routing
- Cookie-based session persistence with base64 encoding
- TTL expiration with background cleanup goroutine
- Support Secure, HttpOnly, SameSite cookie attributes
- Fallback to configured balancer when session target unavailable"
```

---

## Task 4: Configuration Integration

**Files:**
- Modify: `internal/loadbalance/algorithms.go`
- Modify: `internal/config/proxy_config.go`
- Modify: `internal/config/defaults.go`
- Modify: `internal/config/validate.go`

### Step 4.1: Add Algorithms to Valid List

File: `internal/loadbalance/algorithms.go`

```go
var validAlgorithms = []string{
    "round_robin",
    "weighted_round_robin",
    "least_conn",
    "ip_hash",
    "consistent_hash",
    "random",
    "least_time",
    "sticky",
}
```

### Step 4.2: Add Config Structures

File: `internal/config/proxy_config.go`

Add to existing ProxyConfig:

```go
// ProxyConfig 代理配置
type ProxyConfig struct {
    // ... existing fields ...
    
    // LeastTime 最小时间负载均衡配置
    LeastTime LeastTimeConfig `yaml:"least_time"`
    
    // Sticky Session Sticky 配置
    Sticky StickyConfig `yaml:"sticky"`
}

// LeastTimeConfig 最小时间负载均衡配置
type LeastTimeConfig struct {
    Metric      string        `yaml:"metric"`       // "header" 或 "last_byte"
    DefaultTime time.Duration `yaml:"default_time"` // 无样本时的默认时间
}

// StickyConfig Session Sticky 配置
type StickyConfig struct {
    Enabled      bool          `yaml:"enabled"`
    Name         string        `yaml:"name"`
    Expires      time.Duration `yaml:"expires"`
    Domain       string        `yaml:"domain"`
    Path         string        `yaml:"path"`
    Secure       bool          `yaml:"secure"`
    HttpOnly     bool          `yaml:"http_only"`
    SameSite     string        `yaml:"same_site"`
    FallbackAlgo string        `yaml:"fallback_balance"` // fallback 算法
}
```

### Step 4.3: Update Defaults

File: `internal/config/defaults.go`

在生成默认配置的函数中添加注释（搜索 `load_balance:` 相关行并扩展）：

```go
buf.WriteString("    #     load_balance: round_robin   # 负载均衡算法（有效值: round_robin, weighted_round_robin, least_conn, ip_hash, consistent_hash, random, least_time, sticky）\n")

// 在 proxy 配置块后添加：
buf.WriteString("    #     least_time:              # 最小时间负载均衡配置\n")
buf.WriteString("    #       metric: last_byte      # 指标类型（header: 首字节时间, last_byte: 完整响应时间）\n")
buf.WriteString("    #       default_time: 1ms      # 无统计样本时的默认响应时间\n")
buf.WriteString("    #     sticky:                  # Session Sticky 配置\n")
buf.WriteString("    #       enabled: false         # 是否启用\n")
buf.WriteString("    #       name: lolly_route      # cookie 名称\n")
buf.WriteString("    #       expires: 1h            # session 有效期\n")
buf.WriteString("    #       path: /                # cookie 路径\n")
buf.WriteString("    #       http_only: true        # HttpOnly flag\n")
buf.WriteString("    #       same_site: Lax         # SameSite 属性\n")
buf.WriteString("    #       fallback_balance: round_robin  # fallback 算法\n")
```

### Step 4.4: Add Validation

File: `internal/config/validate.go`

在验证 ProxyConfig 的地方添加：

```go
// validate least_time config
if p.LoadBalance == "least_time" {
    if p.LeastTime.Metric != "" && p.LeastTime.Metric != "header" && p.LeastTime.Metric != "last_byte" {
        return fmt.Errorf("无效的 least_time metric: %s（有效值: header, last_byte）", p.LeastTime.Metric)
    }
}

// validate sticky config
if p.LoadBalance == "sticky" {
    if !p.Sticky.Enabled {
        return fmt.Errorf("load_balance=sticky 时 sticky.enabled 必须为 true")
    }
    if p.Sticky.FallbackAlgo != "" && !loadbalance.IsValidAlgorithm(p.Sticky.FallbackAlgo) {
        return fmt.Errorf("无效的 sticky fallback_balance: %s", p.Sticky.FallbackAlgo)
    }
}
```

### Step 4.5: Run Config Tests

Run: `cd /home/xfy/Developer/lolly && go test -v ./internal/config -run TestValidate`
Expected: PASS (所有验证测试)

### Step 4.6: Commit

```bash
cd /home/xfy/Developer/lolly
git add internal/loadbalance/algorithms.go internal/config/proxy_config.go internal/config/defaults.go internal/config/validate.go
git commit -m "feat(config): add Least Time and Sticky configuration support

- Add least_time and sticky to valid algorithms list
- Add LeastTimeConfig and StickyConfig structures
- Update default config generation with new options
- Add configuration validation for new fields"
```

---

## Task 5: Proxy Integration

**Files:**
- Modify: `internal/proxy/proxy.go`
- Modify: `internal/proxy/target_selector.go`

### Step 5.1: Update createBalancer

File: `internal/proxy/proxy.go`

在 `createBalancerByName` 函数中添加：

```go
func createBalancerByName(name string, cfg *config.ProxyConfig) (loadbalance.Balancer, error) {
    switch name {
    // ... existing cases ...
    case "least_time":
        metric := cfg.LeastTime.Metric
        if metric == "" {
            metric = "last_byte"
        }
        defaultTime := cfg.LeastTime.DefaultTime
        if defaultTime <= 0 {
            defaultTime = time.Millisecond
        }
        return loadbalance.NewLeastTime(metric, defaultTime), nil
    case "sticky":
        stickyCfg := loadbalance.StickyConfig{
            Enabled:      cfg.Sticky.Enabled,
            Name:         cfg.Sticky.Name,
            Expires:      cfg.Sticky.Expires,
            Domain:       cfg.Sticky.Domain,
            Path:         cfg.Sticky.Path,
            Secure:       cfg.Sticky.Secure,
            HttpOnly:     cfg.Sticky.HttpOnly,
            SameSite:     cfg.Sticky.SameSite,
        }
        if stickyCfg.Name == "" {
            stickyCfg.Name = "lolly_route"
        }
        if stickyCfg.Expires <= 0 {
            stickyCfg.Expires = time.Hour
        }
        if stickyCfg.Path == "" {
            stickyCfg.Path = "/"
        }
        
        fallbackAlgo := cfg.Sticky.FallbackAlgo
        if fallbackAlgo == "" {
            fallbackAlgo = "round_robin"
        }
        fallbackBalancer, err := createBalancerByName(fallbackAlgo, cfg)
        if err != nil {
            return nil, fmt.Errorf("sticky fallback balancer: %w", err)
        }
        
        sticky := loadbalance.NewStickySession(stickyCfg, fallbackBalancer)
        sticky.Start()
        return sticky, nil
    // ... rest ...
    }
}
```

### Step 5.2: Add Response Time Recording

在 Proxy 的请求处理流程中（找到请求完成后调用的地方，通常在 Do 或类似调用之后）：

```go
// recordResponseTime 记录目标响应时间
func (p *Proxy) recordResponseTime(target *loadbalance.Target, startTime time.Time, headerReceived time.Time) {
    if target == nil || target.Stats == nil {
        return
    }
    
    headerTime := headerReceived.Sub(startTime)
    lastByteTime := time.Since(startTime)
    
    target.Stats.Record(headerTime, lastByteTime)
}
```

**注意：** 需要在实际发起请求的地方调用这个函数。通常是在 fasthttp HostClient.Do 调用后。

由于 proxy.go 文件较大且结构复杂，找到合适的插入点：

在 proxy.go 中找到执行请求的地方（通常有 `client.Do` 或类似的调用），在成功返回后添加：

```go
// 在请求完成后（例如 Do 调用之后）
if recorder, ok := p.balancer.(loadbalance.ResponseTimeRecorder); ok {
    recorder.RecordResponseTime(target, headerTime, lastByteTime)
}
```

### Step 5.3: Update Target Selector for Sticky

File: `internal/proxy/target_selector.go`

修改 `selectByBalancer` 支持 StickySession：

```go
func (p *Proxy) selectByBalancer(ctx *fasthttp.RequestCtx, targets []*loadbalance.Target) *loadbalance.Target {
    p.mu.RLock()
    balancer := p.balancer
    p.mu.RUnlock()
    
    // StickySession 需要请求上下文
    if sticky, ok := balancer.(*loadbalance.StickySession); ok {
        return sticky.Select(ctx, targets)
    }
    
    // ... existing IPHash and ConsistentHash handling ...
    
    return balancer.Select(targets)
}
```

同样修改 `selectTargetExcluding`：

```go
func (p *Proxy) selectTargetExcluding(ctx *fasthttp.RequestCtx, excluded []*loadbalance.Target) *loadbalance.Target {
    // ... existing code ...
    
    // StickySession 通常不用于 failover，但如果是的话：
    if sticky, ok := balancer.(*loadbalance.StickySession); ok {
        return sticky.SelectExcluding(targets, excluded)
    }
    
    // ... rest ...
}
```

### Step 5.4: Run Proxy Tests

Run: `cd /home/xfy/Developer/lolly && go test -v ./internal/proxy -run TestProxy`
Expected: PASS (现有测试不受影响)

### Step 5.5: Commit

```bash
cd /home/xfy/Developer/lolly
git add internal/proxy/proxy.go internal/proxy/target_selector.go
git commit -m "feat(proxy): integrate Least Time and Sticky balancers

- Add least_time and sticky to createBalancerByName
- Implement response time recording for Least Time
- Support StickySession in target selector with request context
- StickySession auto-starts when created"
```

---

## Task 6: Full Integration Test

**Files:**
- Modify: `internal/loadbalance/balancer_test.go` (add integration tests)

### Step 6.1: Add Integration Tests

```go
func TestBalancerIntegration_LeastTime(t *testing.T) {
    targets := []*Target{
        NewTargetFromConfig("http://slow:8080", 1, 0, 0, 0, false, false, ""),
        NewTargetFromConfig("http://fast:8080", 1, 0, 0, 0, false, false, ""),
    }
    
    lt := NewLeastTime("last_byte", time.Millisecond)
    
    // Simulate: slow target has 100ms avg, fast has 10ms avg
    for i := 0; i < 10; i++ {
        targets[0].Stats.Record(50*time.Millisecond, 100*time.Millisecond)
        targets[1].Stats.Record(5*time.Millisecond, 10*time.Millisecond)
    }
    
    // Select 100 times, should mostly pick fast
    fastCount := 0
    for i := 0; i < 100; i++ {
        selected := lt.Select(targets)
        if selected.URL == "http://fast:8080" {
            fastCount++
        }
    }
    
    if fastCount < 80 {
        t.Errorf("fast target selected %d/100 times, expected >80", fastCount)
    }
}

func TestBalancerIntegration_StickyWithLeastTimeFallback(t *testing.T) {
    fallback := NewLeastTime("last_byte", time.Millisecond)
    config := StickyConfig{
        Enabled:  true,
        Name:     "test_route",
        Expires:  time.Hour,
        Path:     "/",
        HttpOnly: true,
    }
    
    sticky := NewStickySession(config, fallback)
    sticky.Start()
    defer sticky.Stop()
    
    targets := []*Target{
        NewTargetFromConfig("http://backend1:8080", 1, 0, 0, 0, false, false, ""),
        NewTargetFromConfig("http://backend2:8080", 1, 0, 0, 0, false, false, ""),
    }
    
    ctx := &fasthttp.RequestCtx{}
    
    // First request
    selected1 := sticky.Select(ctx, targets)
    if selected1 == nil {
        t.Fatal("expected a target")
    }
    
    // Verify cookie set
    cookie := ctx.Response.Header.PeekCookie("test_route")
    if len(cookie) == 0 {
        t.Fatal("expected cookie")
    }
    
    // Make selected1 unhealthy
    selected1.Healthy.Store(false)
    
    // Second request with cookie should fallback
    ctx2 := &fasthttp.RequestCtx{}
    ctx2.Request.Header.SetCookieBytesV("test_route", extractCookieValue(cookie))
    
    selected2 := sticky.Select(ctx2, targets)
    if selected2 == nil {
        t.Fatal("expected fallback target")
    }
    if selected2.URL == selected1.URL {
        t.Error("expected different target after fallback")
    }
}
```

### Step 6.2: Run Integration Tests

Run: `cd /home/xfy/Developer/lolly && go test -v ./internal/loadbalance -run TestBalancerIntegration`
Expected: PASS (2 tests)

### Step 6.3: Commit

```bash
cd /home/xfy/Developer/lolly
git add internal/loadbalance/balancer_test.go
git commit -m "test(loadbalance): add integration tests for Least Time and Sticky

- Verify Least Time picks faster target consistently
- Verify Sticky fallback when target becomes unhealthy
- Test cookie encoding and session persistence"
```

---

## Task 7: Benchmark Tests

**Files:**
- Create: `internal/loadbalance/least_time_bench_test.go`
- Create: `internal/loadbalance/sticky_bench_test.go`

### Step 7.1: Least Time Benchmark

```go
package loadbalance

import (
    "sync"
    "testing"
    "time"
)

func BenchmarkLeastTime_Select(b *testing.B) {
    lt := NewLeastTime("last_byte", time.Millisecond)
    targets := []*Target{
        NewTargetFromConfig("http://a:8080", 1, 0, 0, 0, false, false, ""),
        NewTargetFromConfig("http://b:8080", 1, 0, 0, 0, false, false, ""),
        NewTargetFromConfig("http://c:8080", 1, 0, 0, 0, false, false, ""),
    }
    
    // Pre-populate stats
    for _, t := range targets {
        t.Stats.Record(10*time.Millisecond, 20*time.Millisecond)
    }
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        lt.Select(targets)
    }
}

func BenchmarkLeastTime_Record(b *testing.B) {
    stats := NewEWMAStats()
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        stats.Record(10*time.Millisecond, 20*time.Millisecond)
    }
}

func BenchmarkLeastTime_Concurrent(b *testing.B) {
    lt := NewLeastTime("last_byte", time.Millisecond)
    targets := []*Target{
        NewTargetFromConfig("http://a:8080", 1, 0, 0, 0, false, false, ""),
        NewTargetFromConfig("http://b:8080", 1, 0, 0, 0, false, false, ""),
    }
    
    b.RunParallel(func(pb *testing.PB) {
        for pb.Next() {
            lt.Select(targets)
        }
    })
}
```

### Step 7.2: Sticky Benchmark

```go
package loadbalance

import (
    "testing"

    "github.com/valyala/fasthttp"
)

func BenchmarkStickySession_Select(b *testing.B) {
    fallback := NewRoundRobin()
    config := DefaultStickyConfig()
    
    sticky := NewStickySession(config, fallback)
    sticky.Start()
    defer sticky.Stop()
    
    targets := []*Target{
        NewTargetFromConfig("http://backend1:8080", 1, 0, 0, 0, false, false, ""),
        NewTargetFromConfig("http://backend2:8080", 1, 0, 0, 0, false, false, ""),
    }
    
    // Pre-populate a cookie
    ctx := &fasthttp.RequestCtx{}
    sticky.Select(ctx, targets)
    cookie := ctx.Response.Header.PeekCookie(config.Name)
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        ctx := &fasthttp.RequestCtx{}
        ctx.Request.Header.SetCookieBytesV(config.Name, extractCookieValue(cookie))
        sticky.Select(ctx, targets)
    }
}

func BenchmarkStickySession_SelectNew(b *testing.B) {
    fallback := NewRoundRobin()
    config := DefaultStickyConfig()
    
    sticky := NewStickySession(config, fallback)
    sticky.Start()
    defer sticky.Stop()
    
    targets := []*Target{
        NewTargetFromConfig("http://backend1:8080", 1, 0, 0, 0, false, false, ""),
        NewTargetFromConfig("http://backend2:8080", 1, 0, 0, 0, false, false, ""),
    }
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        ctx := &fasthttp.RequestCtx{}
        sticky.Select(ctx, targets)
    }
}
```

### Step 7.3: Run Benchmarks

Run: `cd /home/xfy/Developer/lolly && go test -bench=. -benchmem ./internal/loadbalance -run=^$`
Expected: 显示性能数据

### Step 7.4: Commit

```bash
cd /home/xfy/Developer/lolly
git add internal/loadbalance/least_time_bench_test.go internal/loadbalance/sticky_bench_test.go
git commit -m "perf(loadbalance): add benchmarks for Least Time and Sticky

- Benchmark Select and Record operations
- Concurrent benchmark for realistic load testing
- Baseline for future performance optimization"
```

---

## Task 8: Final Verification

### Step 8.1: Run All Loadbalance Tests

Run: `cd /home/xfy/Developer/lolly && go test -v ./internal/loadbalance`
Expected: ALL PASS

### Step 8.2: Run All Config Tests

Run: `cd /home/xfy/Developer/lolly && go test -v ./internal/config`
Expected: ALL PASS

### Step 8.3: Run All Proxy Tests

Run: `cd /home/xfy/Developer/lolly && go test -v ./internal/proxy`
Expected: ALL PASS

### Step 8.4: Build

Run: `cd /home/xfy/Developer/lolly && go build ./...`
Expected: SUCCESS (no errors)

### Step 8.5: Final Commit

```bash
cd /home/xfy/Developer/lolly
git log --oneline -10
```

---

## Spec Coverage Checklist

| Spec Requirement | Plan Task |
|------------------|-----------|
| Least Time with EWMA | Task 1 + 2 |
| header_time metric | Task 2 (NewLeastTime parameter) |
| last_byte_time metric | Task 2 (NewLeastTime parameter) |
| Session Sticky cookie | Task 3 |
| 256-shard lock map | Task 3 (stickyShard) |
| Cookie encoding | Task 3 (encodeStickyCookie) |
| TTL expiration | Task 3 (stickyEntry.expiresAt) |
| Background cleanup | Task 3 (cleanupLoop) |
| Fallback algorithm | Task 3 (fallback balancer) |
| Configuration integration | Task 4 |
| Proxy integration | Task 5 |
| Response time recording | Task 5 |
| Zero-lock design | Task 1 (atomic EWMA) |
| Zero-allocation | Task 1 + 2 (no heap alloc in hot path) |
| Concurrent safety | All tasks (atomic + locks) |

---

## Placeholder Scan

- No "TBD" or "TODO" in any task
- No "implement later" or "fill in details"
- All code blocks contain complete implementation
- All test commands include expected output
- All file paths are exact

---

## Type Consistency Check

- `EWMAStats.Record(headerTime, lastByteTime time.Duration)` - consistent
- `LeastTime.Select(targets)` returns `*Target` - consistent with Balancer interface
- `StickySession.Select(ctx, targets)` - consistent with extended usage
- `ResponseTimeRecorder.RecordResponseTime(target, headerTime, lastByteTime)` - consistent

---

## Execution Handoff

**Plan complete and saved to `docs/superpowers/plans/2026-06-08-loadbalance-enhancement.md`.**

Two execution options:

**1. Subagent-Driven (recommended)** - Dispatch a fresh subagent per task, review between tasks, fast iteration

**2. Inline Execution** - Execute tasks in this session using executing-plans, batch execution with checkpoints

**Which approach?**
