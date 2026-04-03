<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-04-03 | Updated: 2026-04-03 -->

# stream

## Purpose
TCP/UDP Stream 代理模块，支持四层代理和会话管理。

## Key Files

| File | Description |
|------|-------------|
| `stream.go` | Stream 代理核心：TCP/UDP 服务器、连接转发、会话管理 |
| `stream_test.go` | Stream 代理测试 |

## For AI Agents

### Working In This Directory
- TCP 代理：监听端口、转发连接、支持多上游
- UDP 代理：会话管理、超时控制、NAT 穿透
- 会话跟踪：源地址+端口作为键
- 超时配置：连接超时、空闲超时

### Testing Requirements
- 运行测试：`go test ./internal/stream/...`
- 测试 TCP/UDP 监听、连接转发、会话管理

### Common Patterns
- TCP 使用 net.Conn 进行双向数据拷贝
- UDP 使用会话映射表管理客户端连接
- 负载均衡策略与 HTTP 代理共享

## Dependencies

### Internal
- `../loadbalance/` - 目标选择
- `../config/` - Stream 配置

<!-- MANUAL: -->