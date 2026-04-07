<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-04-07 | Updated: 2026-04-07 -->

# benchmark

## Purpose
基准测试基础设施目录，提供负载生成、Mock 后端和测试数据生成工具，用于验证服务器性能和回归检测。

## Subdirectories

| Directory | Purpose |
|-----------|---------|
| `tools/` | 基准测试工具集（Mock 后端、负载生成器、测试数据生成） |

## For AI Agents

### Working In This Directory
- 基准测试使用 Go 的 testing.B 框架
- Mock 后端使用 fasthttputil.InmemoryListener 进行内存通信
- 负载生成器支持并发请求和延迟统计

### Testing Requirements
- 运行基准测试：`go test -bench=. ./internal/benchmark/...`
- 基准测试文件使用 `_bench_test.go` 后缀
- 使用 `make benchmark` 运行完整基准测试套件

### Common Patterns
- 使用 `fasthttputil.NewInmemoryListener` 避免网络开销
- 统计收集：QPS、P50/P90/P99 延迟、错误率
- 回归检测通过 `scripts/check_regression.py` 自动执行

## Dependencies

### Internal
- `rua.plus/lolly/internal/proxy` - 代理模块基准测试
- `rua.plus/lolly/internal/cache` - 缓存模块基准测试
- `rua.plus/lolly/internal/loadbalance` - 负载均衡基准测试

### External
- `github.com/valyala/fasthttp` - HTTP 客户端/服务器
- `github.com/valyala/fasthttp/fasthttputil` - 内存测试工具

<!-- MANUAL: -->