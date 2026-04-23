<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-04-23 | Updated: 2026-04-23 -->

# load-balancing

## Purpose
负载均衡配置示例目录，包含轮询、加权、最少连接、IP 哈希、一致性哈希等负载均衡策略配置。

## Key Files

| File | Description |
|------|-------------|
| `round-robin.conf` | 轮询负载均衡：默认策略、均匀分配 |
| `weighted.conf` | 加权轮询：按权重分配流量、服务器能力差异 |
| `least-conn.conf` | 最少连接：动态分配到连接最少的服务器 |
| `ip-hash.conf` | IP 哈希：会话保持、同一客户端固定后端 |
| `consistent-hash.conf` | 一致性哈希：分布式缓存、节点增减影响最小化 |

## For AI Agents

### Working In This Directory
- 默认策略是轮询（round-robin）
- 加权适用于服务器性能差异场景
- 最少连接适用于长连接场景
- IP 哈希适用于需要会话保持的场景
- 一致性哈希适用于分布式缓存场景

### Testing Requirements
- 负载均衡策略通过单元测试和集成测试验证
- 运行测试：`go test ./internal/loadbalance/...`

### Common Patterns
- 权重配置：`weight=N` 参数
- 健康检查：`max_fails`, `fail_timeout`
- 备用服务器：`backup` 标记

## Dependencies

### Internal
- `../../../internal/loadbalance/` - 负载均衡策略实现
- `../../../internal/resolver/` - 动态 DNS 解析

<!-- MANUAL: -->
