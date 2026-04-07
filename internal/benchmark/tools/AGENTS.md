<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-04-07 | Updated: 2026-04-07 -->

# tools

## Purpose
基准测试工具集，提供 Mock 后端服务器、负载生成器和测试数据生成功能，支持各种测试场景（延迟、错误、随机响应）。

## Key Files

| File | Description |
|------|-------------|
| `mock_backend.go` | Mock 后端实现，支持多种响应模式（固定、延迟、错误、随机） |
| `loadgen.go` | 负载生成器，收集 QPS、延迟分布（P50/P90/P99）统计 |
| `testdata.go` | 测试数据生成，支持多种预定义大小（1KB~10MB） |

## For AI Agents

### Working In This Directory
- Mock 后端使用 `fasthttputil.InmemoryListener` 进行零网络开销测试
- `BackendMode` 支持四种模式：ModeFixed、ModeDelay、ModeError、ModeRandomResponse
- 负载生成器支持并发执行和百分位延迟统计
- 测试数据使用随机字节填充，适合压缩测试

### Testing Requirements
- 工具本身无独立测试，作为其他模块基准测试的基础设施
- 使用示例见 `internal/proxy/proxy_bench_test.go`

### Common Patterns
- 创建简单后端：`SimpleMockBackend(statusCode, body)`
- 创建延迟后端：`DelayedMockBackend(delay, body)`
- 创建错误后端：`ErrorMockBackend(errorRate, body)`
- 创建加权目标：`CreateWeightedTestTargets(n)`
- 运行负载测试：`loadGen.Run(n, concurrency)`

## Dependencies

### External
- `github.com/valyala/fasthttp` - HTTP 框架
- `github.com/valyala/fasthttp/fasthttputil` - 内存监听器

<!-- MANUAL: -->