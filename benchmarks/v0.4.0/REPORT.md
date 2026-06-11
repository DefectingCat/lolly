# Lolly v0.4.0 性能分析报告

> **生成日期**: 2026-06-11  
> **测试环境**: Linux amd64, Go 1.26.4  
> **测试负载**: wrk 静态文件 + 代理 (共 ~600 并发)

---

## 1. 基准测试摘要

完整基准数据见 `benchmarks/v0.4.0/summary.txt`。

关键微基准指标：

| 模块 | 关键指标 | 数值 |
|------|---------|------|
| `cache` | FileCacheGet | ~45-53 ns/op, 1 alloc/op |
| `cache` | ProxyCacheGet | ~125 ns/op, 1 alloc/op |
| `matcher` | RadixTree 查找 | <100 ns/op |
| `proxy` | WebSocket 检测 | ~30-50 ns/op |
| `middleware/security` | RateLimiter | ~60-100 ns/op |
| `stream` | filterHealthy | ~50-200 ns/op |
| `resolver` | DNS 缓存命中 | ~100-300 ns/op |

---

## 2. CPU 热点 Top 10

数据来源：`go tool pprof -top cpu.prof`

| 排名 | 函数 | flat% | cum% | 分析 |
|------|------|-------|------|------|
| 1 | `internal/runtime/syscall/linux.Syscall6` | 61.64% | 61.64% | 系统调用开销（epoll/read/write），网络 I/O 正常开销 |
| 2 | `runtime.memmove` | 2.13% | 2.13% | 内存拷贝 |
| 3 | `zerolog/internal/json.Encoder.AppendString` | 1.14% | 1.37% | JSON 字符串编码 |
| 4 | `time.runtimeNow` | 1.00% | 1.00% | 时间获取 |
| 5 | `fasthttp.(*Server).serveConn` | 0.87% | 92.67% | 请求处理总入口 |
| 6 | `runtime.futex` | 0.71% | 0.71% | 线程同步 |
| 7 | `runtime.exitsyscall` | 0.66% | 0.73% | 系统调用返回 |
| 8 | `runtime.nanotime` | 0.66% | 0.66% | 纳秒时间 |
| 9 | `runtime.gopark` | 0.48% | 0.51% | goroutine 调度 |
| 10 | **`logging.(*Logger).LogAccess`** | **0.23%** | **16.36%** | **⚠️ 访问日志是巨大的 CPU 热点** |

**关键洞察**：
- 61.64% 的 CPU 在系统调用中，这是高并发网络 I/O 的基线开销
- **访问日志 (LogAccess) 占累计 16.36%** — 这是应用层最大的单一热点
- zerolog JSON 编码也占了一部分（1.37% cum）

---

## 3. 内存分配热点 Top 10

数据来源：`go tool pprof -top allocs.prof`

| 排名 | 函数 | flat% | cum% | 分析 |
|------|------|-------|------|------|
| 1 | **`os.statNolog`** | **74.95%** | **79.78%** | **⚠️ 绝对主导，文件 stat 调用产生大量分配** |
| 2 | `syscall.ByteSliceFromString` | 4.83% | 4.83% | 字符串转字节片 |
| 3 | `net.IP.String` | 4.80% | 4.80% | IP 地址字符串化 |
| 4 | `net.JoinHostPort` | 4.54% | 4.54% | 地址拼接 |
| 5 | `internal/bytealg.MakeNoZero` | 4.51% | 4.51% | 字节分配 |
| 6 | `sync.(*poolChain).pushHead` | 0.77% | 0.77% | sync.Pool 操作 |
| 7 | `bufio.NewReaderSize` | 0.74% | 0.74% | bufio reader 创建 |
| 8 | `net.(*Dialer).DialContext` | 0.38% | 2.39% | 连接创建 |
| 9 | `context.AfterFunc` | 0.27% | 0.68% | context 回调 |
| 10 | `zerolog.(*Event).msg` | 0% | 0.77% | 日志事件处理 |

**关键洞察**：
- **`os.statNolog` 独占 74.95% 的分配** — 这是最大的优化机会
- `net.IP.String` + `net.JoinHostPort` 合计 **9.34%** — 地址格式化反复分配
- 连接建立（DialContext）也占 2.39% cum — 连接池不够高效

---

## 4. 堆内存占用 Top 10

数据来源：`go tool pprof -top heap.prof`

| 排名 | 函数 | inuse% | 分析 |
|------|------|--------|------|
| 1 | `bufio.NewReaderSize` | 27.31% | bufio reader 长期占用 |
| 2 | `bufio.NewWriterSize` | 27.29% | bufio writer 长期占用 |
| 3 | `fasthttp.(*RequestHeader).parseFirstLine` | 18.14% | 请求头解析 |
| 4 | `runtime.mallocgc` | 9.09% | 一般分配 |
| 5 | `golang.org/x/net/http2/hpack.init` | 4.56% | HTTP/2 hpack 初始化 |
| 6 | `fasthttp.(*Server).acquireCtx` | 4.54% | 请求上下文 |
| 7 | `zerolog.init` | 4.54% | zerolog 事件池 |
| 8 | `sync.(*poolChain).pushHead` | 4.53% | sync.Pool 链 |

**关键洞察**：
- `bufio.Reader/Writer` 合计 **54.6%** 堆内存 — 可以优化池化复用
- fasthttp 的请求上下文和头解析占 22.68%

---

## 5. 优化建议与优先级

### P0 — 高优先级（预期收益大）

#### 1. 访问日志 CPU 优化 (cum 16.36%)

**问题**: `logging.(*Logger).LogAccess` 累计占 16.36% CPU，包括 JSON 编码和时间获取。

**方向**:
- 添加访问日志采样配置（如只记录 1% 请求）
- 使用对象池复用日志事件
- 简化默认访问日志格式（避免复杂 JSON）
- 异步批量写入访问日志

**预期收益**: CPU 降低 5-15%

#### 2. 静态文件 stat 调用优化 (allocs 74.95%)

**问题**: `os.statNolog` 独占 74.95% 分配，说明静态文件 handler 频繁调用 stat 检查文件存在性。

**方向**:
- 增强 `FileInfoCache` 命中率和 TTL
- 对不存在的路径做负缓存（negative cache）
- 避免在代理路径上调用 stat
- 优化 `try_files` 实现，减少重复 stat

**预期收益**: 分配降低 50-70%

### P1 — 中优先级

#### 3. 网络地址字符串化优化 (allocs 9.34%)

**问题**: `net.IP.String` 和 `net.JoinHostPort` 合计 9.34% 分配。

**方向**:
- 在访问日志等热路径缓存 IP 字符串
- 使用预格式化的 host:port 缓存
- 避免重复解析 X-Forwarded-For

**预期收益**: 分配降低 5-10%

#### 4. bufio Reader/Writer 池化 (heap 54.6%)

**问题**: `bufio.NewReaderSize/NewWriterSize` 占 54.6% 堆内存。

**方向**:
- 复用 fasthttp HostClient 的 reader/writer
- 在代理路径中池化 bufio 对象

**预期收益**: 内存占用降低 20-40%

### P2 — 低优先级

#### 5. 连接池优化

**问题**: `net.Dialer.DialContext` 占 2.39% cum。

**方向**:
- 增加 fasthttp HostClient 连接池大小
- 优化空闲连接超时

#### 6. 系统调用优化

**问题**: syscall 占 61.64% CPU，这是基线。

**方向**:
- 考虑 io_uring（Linux）降低 syscall 开销
- sendfile 优化（已部分实现）

---

## 6. 建议的下一步实施顺序

1. **Task A**: 访问日志采样 + 异步化 — 最大 CPU 收益
2. **Task B**: 静态文件 stat 负缓存 + FileInfoCache 优化 — 最大分配收益
3. **Task C**: IP/Host:Port 字符串缓存 — 降低分配
4. **Task D**: bufio 池化 — 降低内存占用
5. **Task E**: 重新跑 benchmark 验证收益

---

## 7. 原始数据文件

- `benchmarks/v0.4.0/pprof/cpu.prof` — CPU profile
- `benchmarks/v0.4.0/pprof/allocs.prof` — 分配 profile
- `benchmarks/v0.4.0/pprof/heap.prof` — 堆内存 profile
- `benchmarks/v0.4.0/pprof/goroutine.prof` — Goroutine profile
- `benchmarks/v0.4.0/cpu-top.txt` — CPU top 函数
- `benchmarks/v0.4.0/allocs-top.txt` — 分配 top 函数
- `benchmarks/v0.4.0/heap-top.txt` — 堆内存 top 函数
- `benchmarks/v0.4.0/summary.txt` — 基准测试汇总
