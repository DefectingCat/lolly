<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-04-09 | Updated: 2026-04-09 -->

# variable

## Purpose
高性能变量系统，提供 nginx 风格的变量展开功能。用于访问日志格式模板、代理请求头设置、URL 重写规则。

## Key Files

| File | Description |
|------|-------------|
| `variable.go` | 变量系统核心：VariableContext 结构、Expand 方法、变量存储接口 |
| `builtin.go` | 内置变量定义：$remote_addr、$request_uri、$status、$time_local 等nginx 风格变量 |
| `pool.go` | sync.Pool 复用：PoolGet、PoolPut、池统计信息 |
| `ssl.go` | SSL 相关变量：$ssl_protocol、$ssl_cipher、$ssl_client_sni 等 |
| `variable_test.go` | 单元测试：变量展开、内置变量获取、自定义变量 |
| `variable_bench_test.go` | 基准测试：展开性能、池性能 |

## For AI Agents

### Working In This Directory
- 支持两种变量格式：$var 和 ${var}（用于变量后有字符）
- 使用快速字符串扫描（非正则表达式）提升性能
- sync.Pool 复用 VariableContext 减少 GC 压力
- 内置变量惰性求值并缓存结果
- 自定义变量通过 Set 方法设置

### Testing Requirements
- 运行测试：`go test ./internal/variable/...`
- 基准测试验证展开性能：`go test -bench=. ./internal/variable/...`
- 集成测试在 `../integration/` 目录

### Common Patterns
- VariableContext 绑定到单个请求
- 从池获取：PoolGet(ctx)，放回：PoolPut(vc)
- 全局变量通过 SetGlobalVariables 设置
- 上游变量：$upstream_addr、$upstream_status、$upstream_response_time

## Dependencies

### Internal
- 无内部依赖，底层模块

### External
- `github.com/valyala/fasthttp` - RequestCtx 类型

<!-- MANUAL: -->