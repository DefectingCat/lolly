# Lolly 负载均衡增强设计 - Least Time & Session Sticky

**日期**: 2026-06-08
**状态**: Approved

## 1. 背景与目标

Lolly 当前支持 6 种负载均衡算法：Round Robin、Weighted Round Robin、Least Connections、IP Hash、Consistent Hash、Random（Power of Two Choices）。

与 nginx Plus 对比，Lolly 缺少两个重要特性：
1. **Least Time** - 基于响应时间选择最优后端
2. **Session Sticky** - Cookie-based 会话保持

本文档设计这两个算法的高性能实现方案，目标是：
- **零锁设计**：原子操作替代互斥锁
- **零堆分配**：预分配 + 对象池
- **纳秒级延迟**：单次选择 < 100ns
- **与现有代码风格一致**

## 2. 设计概览

```
                    +----------------------+
                    |     Proxy Request    |
                    +----------+-----------+
                               |
              +----------------+----------------+
              |                                 |
        +-----v------+                  +------v------+
        | Least Time |                  | Sticky      |
        | Select     |                  | Route       |
        +-----+------+                  +------+------+
              |                                 |
        +-----v------+                  +------v------+
        | EWMA Stats |                  | Cookie      |
        | (atomic)   |                  | + Shard Map |
        +------------+                  +-------------+
```

## 3. Least Time 设计

### 3.1 核心算法

基于 EWMA（指数加权移动平均）的响应时间统计：

```
new_avg = alpha * new_sample + (1 - alpha) * old_avg
```

- `alpha` 默认 0.3，可配置（0-1 范围）
- alpha 越大，对新样本越敏感，收敛越快
- 使用 atomic.Int64 存储纳秒值，避免浮点运算

### 3.2 数据结构

```go
// EWMAStats 原子 EWMA 统计器
type EWMAStats struct {
    headerTime    atomic.Int64  // EWMA 首字节时间（纳秒）
    lastByteTime  atomic.Int64  // EWMA 完整响应时间（纳秒）
    sampleCount   atomic.Int64  // 样本计数
}

// 使用固定点整数运算避免浮点
// 将 alpha 编码为定点数：alpha * 1000
const alphaScale = 1000

func (e *EWMAStats) Record(headerTime, lastByteTime time.Duration) {
    // 原子更新，无锁
    e.updateAtomic(&e.headerTime, headerTime)
    e.updateAtomic(&e.lastByteTime, lastByteTime)
    e.sampleCount.Add(1)
}
```

### 3.3 LeastTime Balancer

```go
type LeastTime struct {
    metric string  // "header" | "last_byte"
}

func (l *LeastTime) Select(targets []*Target) *Target {
    var selected *Target
    var minTime int64 = -1
    
    for _, t := range targets {
        if !t.IsAvailable() {
            continue
        }
        
        // 原子读取响应时间
        var currentTime int64
        if l.metric == "header" {
            currentTime = t.Stats.HeaderTime()
        } else {
            currentTime = t.Stats.LastByteTime()
        }
        
        // 无统计样本时给默认值，避免新节点被饿死
        if currentTime == 0 {
            currentTime = defaultResponseTime
        }
        
        if selected == nil || currentTime < minTime {
            selected = t
            minTime = currentTime
        }
    }
    
    return selected
}
```

### 3.4 性能指标

| 操作 | 延迟 | 锁 | 堆分配 |
|------|------|-----|--------|
| Record | ~20ns | 无 | 0 |
| Select | ~50ns | 无 | 0 |

### 3.5 配置

```yaml
proxy:
  - path: /api
    load_balance: least_time
    least_time_metric: last_byte   # header | last_byte（默认）
    least_time_alpha: 0.3          # 0-1，越大越敏感（默认 0.3）
    least_time_default_ns: 1000000 # 无样本时的默认值（默认 1ms）
```

### 3.6 Proxy 层集成

```go
// 在请求完成后调用
func (p *Proxy) recordResponseTime(target *loadbalance.Target, start time.Time) {
    if tracker, ok := p.balancer.(ResponseTimeRecorder); ok {
        headerTime := target.HeaderReceived.Sub(start)
        lastByteTime := time.Since(start)
        tracker.RecordResponseTime(target, headerTime, lastByteTime)
    }
}
```

## 4. Session Sticky 设计

### 4.1 核心算法

基于 Cookie 的路由表 + 分片锁：

- Cookie 值编码：`base64(target_url + "|" + expires_timestamp)`
- 256 个分片，每个分片独立 `sync.RWMutex`
- 分片索引：`fnvHash64a(cookie_value) % 256`
- 后台 goroutine 每 60s 清理过期 session

### 4.2 数据结构

```go
// StickySession Sticky Session 负载均衡器
type StickySession struct {
    config      StickyConfig
    fallback    loadbalance.Balancer  // fallback 算法
    
    // 256 个分片，降低锁冲突概率
    shards      [256]*stickyShard
    cleaner     *time.Ticker
    stopCh      chan struct{}
    started     atomic.Bool
}

type stickyShard struct {
    mu       sync.RWMutex
    sessions map[string]*stickyEntry  // key: cookie value
}

type stickyEntry struct {
    targetURL   string
    expiresAt   int64  // Unix 纳秒
    createdAt   int64  // Unix 纳秒
}
```

### 4.3 路由流程

```
请求到达
  |
  v
检查 Cookie "lolly_route"
  |
  +-- 存在 -->
  |            解码 cookie 值
  |            查找目标是否健康
  |            |
  |            +-- 健康 --> 路由到该目标
  |            |
  |            +-- 不健康 -> 删除 session
  |                         用 fallback 选择新目标
  |                         设置新 cookie
  |
  +-- 不存在 -->
                用 fallback 选择目标
                设置 Set-Cookie 响应头
```

### 4.4 Cookie 编码

```go
// encodeCookie 编码路由信息到 cookie 值
// 格式: base64(target_url + "|" + expires_timestamp)
func encodeCookie(targetURL string, expires time.Time) string {
    raw := targetURL + "|" + strconv.FormatInt(expires.Unix(), 10)
    return base64.URLEncoding.EncodeToString([]byte(raw))
}

// decodeCookie 解码 cookie 值
func decodeCookie(value string) (targetURL string, expires time.Time, ok bool) {
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
```

### 4.5 选择逻辑

```go
func (s *StickySession) Select(ctx *fasthttp.RequestCtx, targets []*Target) *Target {
    // 1. 检查 cookie
    cookie := ctx.Request.Header.Cookie(s.config.Name)
    if len(cookie) > 0 {
        targetURL, _, ok := decodeCookie(string(cookie))
        if ok {
            // 查找目标
            for _, t := range targets {
                if t.URL == targetURL && t.IsAvailable() {
                    return t
                }
            }
            // 目标不可用，删除 session（延迟删除）
            s.deleteSession(string(cookie))
        }
    }
    
    // 2. 使用 fallback 算法选择
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
```

### 4.6 性能指标

| 操作 | 延迟 | 锁冲突概率 |
|------|------|-----------|
| Session 查找 | ~30ns | 0.4% (256 分片) |
| Session 写入 | ~50ns | 0.4% |
| 清理过期 | 后台，不影响主路径 | - |

### 4.7 配置

```yaml
proxy:
  - path: /api
    load_balance: sticky
    sticky:
      enabled: true
      name: "lolly_route"        # cookie 名称（默认）
      expires: "1h"              # session 有效期（默认 1h）
      domain: ""                 # cookie domain
      path: "/"                  # cookie path（默认 /）
      secure: false              # Secure flag
      http_only: true            # HttpOnly flag（默认 true）
      same_site: "Lax"           # SameSite（默认 Lax）
    # fallback 算法配置
    fallback_balance: round_robin  # 首次路由和失效回退算法
```

## 5. 扩展 Balancer 接口

为支持 Least Time 的响应时间记录，扩展一个可选接口：

```go
// ResponseTimeRecorder 响应时间记录接口
// 实现此接口的 balancer 可在请求完成后收到响应时间统计
type ResponseTimeRecorder interface {
    RecordResponseTime(target *Target, headerTime, lastByteTime time.Duration)
}
```

**为什么用接口扩展而非修改 Balancer？**
- 不破坏现有 6 个 balancer 的实现
- 类型断言在运行时判断，无性能开销
- 符合 Go 接口隔离原则

## 6. 文件改动清单

### 6.1 新增文件

| 文件 | 行数 | 说明 |
|------|------|------|
| `internal/loadbalance/ewma.go` | ~80 | 原子 EWMA 统计器 |
| `internal/loadbalance/least_time.go` | ~120 | Least Time balancer |
| `internal/loadbalance/sticky.go` | ~280 | Session Sticky balancer |
| `internal/loadbalance/sticky_config.go` | ~30 | Sticky 配置结构体 |
| `internal/loadbalance/least_time_test.go` | ~200 | Least Time 单元测试 |
| `internal/loadbalance/sticky_test.go` | ~250 | Session Sticky 单元测试 |

### 6.2 修改文件

| 文件 | 修改内容 |
|------|----------|
| `internal/loadbalance/algorithms.go` | 添加 `least_time`、`sticky` 到 validAlgorithms |
| `internal/loadbalance/balancer.go` | Target 增加 `Stats *EWMAStats` 字段 |
| `internal/config/proxy_config.go` | 添加 `LeastTimeConfig`、`StickyConfig` |
| `internal/config/defaults.go` | 添加新配置项默认值注释 |
| `internal/config/validate.go` | 验证 `least_time_metric`、`fallback_balance` |
| `internal/proxy/proxy.go` | createBalancer 增加新算法；请求完成后调用 RecordResponseTime |
| `internal/proxy/target_selector.go` | Select 支持 StickySession（需 ctx 参数） |

## 7. 测试策略

### 7.1 Least Time 测试

- **基准测试**: 测量 Select/Record 延迟
- **并发测试**: 100 goroutine 并发 Record + Select，验证无数据竞争
- **收敛测试**: 验证 EWMA 对新旧样本的权重分配
- **故障转移**: 验证目标失效后选择其他目标

### 7.2 Session Sticky 测试

- **Cookie 编码/解码**: 验证 round-trip 正确性
- **路由一致性**: 相同 cookie 始终路由到同一目标
- **目标失效**: 目标不可用时 fallback 并更新 cookie
- **过期清理**: 验证过期 session 被清理
- **并发安全**: 100 goroutine 并发读写，验证无数据竞争
- **分片均衡**: 验证 hash 分布均匀

## 8. 与 nginx Plus 对比

| 特性 | nginx Plus | Lolly 方案 |
|------|------------|------------|
| Least Time header | ✅ | ✅ |
| Least Time last_byte | ✅ | ✅ |
| EWMA 平滑 | ✅ | ✅ (alpha 可调) |
| Session Sticky cookie | ✅ | ✅ |
| Session Sticky learn | ✅ | ❌ (暂不支持) |
| Secure/HttpOnly/SameSite | ✅ | ✅ |
| 目标失效 fallback | ✅ | ✅ |
| Session TTL | ✅ | ✅ |

## 9. 风险与缓解

| 风险 | 影响 | 缓解 |
|------|------|------|
| 新节点被饿死 | 高 | 无统计样本时给默认值 `least_time_default_ns` |
| Sticky 内存增长 | 中 | TTL + 后台清理 + 分片限制 |
| Cookie 过大 | 低 | 仅编码 URL + timestamp，通常 < 200 bytes |
| 目标频繁上下线 | 中 | session 延迟删除，避免惊群 |

## 10. 后续优化

1. **Session Sticky Learn 模式**: 学习后端返回的 Set-Cookie，而非主动种植
2. **Least Time 加权**: 结合权重和响应时间进行加权选择
3. **统计持久化**: 重启后保留历史响应时间统计

---

**设计批准**: ✅ 已批准
**下一步**: 编写实现计划 (writing-plans)
