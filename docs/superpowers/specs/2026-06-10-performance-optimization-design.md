# 性能持续优化设计文档

> **版本**: v1.0  
> **日期**: 2026-06-10  
> **目标**: 极致吞吐量 + 资源效率  
> **方法**: 数据驱动优化（Benchmark → Profile → Optimize → Verify）

---

## 1. 总体架构

整个性能优化流程分为 5 个阶段，形成持续迭代闭环：

```
┌─────────────┐    ┌─────────────┐    ┌─────────────┐    ┌─────────────┐    ┌─────────────┐
│  1. 建立基准  │ → │  2. 采集数据  │ → │  3. 分析瓶颈  │ → │  4. 实施优化  │ → │  5. 回归检测  │
│  Benchmark  │    │  Baseline   │    │   Profile   │    │   Optimize   │    │   Prevent   │
└─────────────┘    └─────────────┘    └─────────────┘    └─────────────┘    └─────────────┘
       ↑                                                                            │
       └──────────────────────────────── 持续迭代 ◄─────────────────────────────────┘
```

**核心原则**:
- 每个优化必须有 benchmark 数据证明收益
- 不优化没有数据支撑的地方
- 建立可重复的性能测试环境

---

## 2. 基准测试基础设施（Benchmark Suite）

### 2.1 三层基准测试体系

#### 2.1.1 微基准（Micro Benchmark）— 单元级

针对单个函数/模块的 Go benchmark：

| 模块 | 状态 | 待补充 |
|------|------|--------|
| `loadbalance` | 已有 | Sticky、Least Time 极端场景 |
| `matcher` | 已有 | 大规模路由表（1k+ location） |
| `proxy` | 已有 | 缓存键构建、WebSocket 检测 |
| `middleware/security` | 已有 | 限流器高并发 |
| `middleware/compression` | 已有 | 大文件压缩 |
| `cache` | 部分 | 完整 CRUD、并发竞争 |
| `lua` | 部分 | 脚本执行、协程调度 |
| `resolver` | 缺失 | DNS 查询、缓存命中 |
| `variable` | 部分 | 复杂变量展开 |
| `stream` | 缺失 | TCP/UDP 转发吞吐 |

#### 2.1.2 集成基准（Integration Benchmark）— 端到端

用 `httptest` 或真实端口测试完整请求链路：

- **静态文件服务**: 小文件（1KB）、中文件（100KB）、大文件（10MB）
- **反向代理**: 直连后端、带缓存、带负载均衡
- **HTTPS/TLS**: 握手开销、TLS 1.2 vs 1.3
- **HTTP/2**: 多路复用、流控
- **HTTP/3**: QUIC 连接建立、0-RTT
- **WebSocket**: 消息转发延迟
- **Stream**: TCP/UDP 吞吐

#### 2.1.3 系统基准（System Benchmark）— 全链路

用外部压测工具测试完整服务器：

- **RPS 极限测试**: 不同并发数下的吞吐量曲线
- **延迟分布**: P50/P99/P999 延迟
- **资源占用**: CPU、内存、goroutine 数、GC 频率
- **连接数测试**: C10K、C100K 场景

### 2.2 Benchmark 目录结构

```
internal/benchmark/
├── micro/           # Go benchmark 文件
│   ├── proxy_test.go
│   ├── cache_test.go
│   ├── lua_test.go
│   └── ...
├── integration/     # 集成测试风格 benchmark
│   ├── static_bench_test.go
│   ├── proxy_bench_test.go
│   └── ...
└── system/          # 外部压测脚本 + 结果
    ├── wrk_static.sh
    ├── wrk_proxy.sh
    └── results/
```

### 2.3 基准收集工具

- **`make bench`**: 运行所有微基准
- **`make bench-stat`**: 生成基准报告
- **`scripts/bench.sh`**: 一键系统压测
- **benchstat**: 对比新旧基准数据

---

## 3. 性能数据采集与分析流程

### 3.1 Baseline 采集步骤

#### 第一步：微基准全量运行

```bash
# 运行所有微基准，保存结果
go test -bench=. -benchmem ./internal/benchmark/micro/... > benchmark-v0.4.0.txt

# 使用 benchstat 格式化
benchstat benchmark-v0.4.0.txt
```

#### 第二步：集成基准运行

```bash
# 运行集成 benchmark
go test -bench=Benchmark -benchmem ./internal/benchmark/integration/...
```

#### 第三步：系统压测（外部工具）

```bash
# 静态文件压测
wrk -t12 -c400 -d30s http://localhost:8080/

# 代理压测
wrk -t12 -c400 -d30s http://localhost:8080/api/

# HTTP/2 压测
h2load -n100000 -c100 -m10 http://localhost:8080/
```

#### 第四步：pprof 数据采集

```bash
# CPU profile（30秒）
curl http://localhost:8080/debug/pprof/profile?seconds=30 > cpu.prof

# Heap profile
curl http://localhost:8080/debug/pprof/heap > heap.prof

# Allocs profile（分配热点）
curl http://localhost:8080/debug/pprof/allocs > allocs.prof

# Goroutine profile
curl http://localhost:8080/debug/pprof/goroutine > goroutine.prof
```

### 3.2 分析工具链

| 工具 | 用途 | 命令 |
|------|------|------|
| `go tool pprof` | CPU/内存分析 | `go tool pprof -http=:8081 cpu.prof` |
| `go tool trace` | 调度/延迟分析 | `go test -trace=trace.out` |
| `benchstat` | 基准对比 | `benchstat old.txt new.txt` |
| `go test -memprofile` | 分配追踪 | 集成到 benchmark |
| `perf` (Linux) | 系统级分析 | `perf record -g ./lolly` |

### 3.3 分析维度

1. **CPU 热点**: 哪些函数消耗最多 CPU？
2. **内存分配**: 每请求分配次数和大小？
3. **锁竞争**: `sync.Mutex` / `sync.RWMutex` 的争用情况？
4. **系统调用**: `syscall` / `cgo` 开销？
5. **GC 压力**: GC 频率、STW 时间？
6. **网络 I/O**: 连接建立、读写延迟？

### 3.4 瓶颈识别模板

```
性能分析报告 v0.4.0 Baseline
=============================

1. CPU 热点 Top 5
   - runtime.mallocgc (12.3%) ← 分配开销
   - runtime.scanobject (8.7%) ← GC 扫描
   - proxy.(*Proxy).ServeHTTP (7.2%)
   - matcher.(*LocationEngine).Match (5.1%)
   - compress/flate.(*compressor).write (4.8%)

2. 每请求分配 Top 5
   - time.Now(): 1 alloc/req
   - fmt.Sprintf: 0.5 alloc/req
   - ...

3. 锁竞争热点
   - cache.(*FileCache).Get: 15% 阻塞时间
   - proxy.(*Proxy).buildCacheKeyHash: 8% 阻塞时间

4. 优化优先级
   P0: [具体任务]
   P1: [具体任务]
   P2: [具体任务]
```

---

## 4. 优化实施流程

### 4.1 优化原则

- **可量化**: 每次优化必须有 benchmark 对比数据
- **最小改动**: 优先单文件/单函数改动
- **可回滚**: 保留优化前后的基准数据

### 4.2 优化分类

| 类型 | 示例 | 验证方式 |
|------|------|---------|
| 零分配 | 用 `b2s` 替代 `string([]byte)` | `-benchmem` allocs/op |
| 算法优化 | 更快的哈希、查找 | `Benchmark` ns/op |
| 并发优化 | 锁粒度细化、无锁结构 | `go test -race` + benchmark |
| 缓存优化 | 减少重复计算 | CPU profile 对比 |
| GC 优化 | 减少短生命周期对象 | `GODEBUG=gctrace=1` |

---

## 5. 回归检测机制

### 5.1 自动化检查

- **CI 集成**: 每次 PR 跑 benchmark 对比
- **阈值告警**: 性能下降 >5% 自动阻断
- **趋势追踪**: 长期性能趋势图

### 5.2 回归检测工具

```bash
# 对比两个版本
benchstat old.txt new.txt

# 示例输出
# name        old time/op    new time/op    delta
# ServeHTTP   1.20µs ± 2%    1.15µs ± 3%    -4.17%  (p=0.02 n=10+10)
```

---

## 6. 预期成果

- 完整的 benchmark 套件覆盖所有核心模块
- 可量化的 baseline 性能数据
- 识别出的 Top 10 性能瓶颈
- 每轮优化都有可验证的性能提升数据
- 自动化回归检测防止性能退化

---

## 7. 任务清单

- [ ] 建立 `internal/benchmark/` 目录结构
- [ ] 补充缺失的微基准（resolver、stream、cache、lua）
- [ ] 创建集成基准测试
- [ ] 创建系统压测脚本
- [ ] 跑第一轮全量基准 → 生成 baseline
- [ ] 采集 pprof 数据（CPU/heap/allocs/goroutine）
- [ ] 分析瓶颈 → 生成性能报告
- [ ] 制定 Top N 优化任务
- [ ] 逐个实施优化并验证
- [ ] 建立 CI 回归检测
