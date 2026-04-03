<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-04-03 | Updated: 2026-04-03 -->

# loadbalance

## Purpose
负载均衡模块，提供多种负载均衡策略和健康检查功能。

## Key Files

| File | Description |
|------|-------------|
| `balancer.go` | 负载均衡核心：Balancer 接口、Upstream、Target 管理 |
| `algorithms.go` | 负载均衡算法：RoundRobin、LeastConn、Weighted |
| `health.go` | 健康检查：HealthChecker、被动/主动检查 |
| `balancer_test.go` | 负载均衡测试 |
| `health_test.go` | 健康检查测试 |

## For AI Agents

### Working In This Directory
- `Balancer` 接口定义：Select()、AddTarget()、RemoveTarget()
- 支持 RoundRobin（轮询）、LeastConn（最少连接）、Weighted（加权）
- 健康检查支持被动（请求失败计数）和主动（定期探测）
- 不健康 Target 自动剔除，恢复后自动加入

### Testing Requirements
- 运行测试：`go test ./internal/loadbalance/...`
- 测试各算法的正确性、健康检查逻辑

### Common Patterns
- Target 权重默认为 1
- 健康检查间隔默认 10 秒
- 不健康阈值：连续 3 次失败

## Dependencies

### Internal
- `../config/` - 负载均衡配置

### External
- `github.com/valyala/fasthttp` - HTTP 客户端用于健康探测

<!-- MANUAL: -->