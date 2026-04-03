<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-04-03 | Updated: 2026-04-03 -->

# proxy

## Purpose
反向代理模块，提供 HTTP 和 WebSocket 代理功能，支持连接池和超时控制。

## Key Files

| File | Description |
|------|-------------|
| `proxy.go` | HTTP 反向代理核心：Proxy 结构体、Forward()、请求/响应处理 |
| `websocket.go` | WebSocket 代理：握手升级、双向数据转发、连接管理 |
| `health.go` | 健康检查集成：与 loadbalance 模块协作 |
| `proxy_test.go` | HTTP 代理测试 |
| `health_test.go` | 健康检查测试 |

## For AI Agents

### Working In This Directory
- HTTP 代理使用 fasthttp 的 HostClient 进行转发
- WebSocket 代理支持双向数据流，处理 Upgrade 请求
- 连接池配置：MaxConns、MaxIdleConnDuration
- 超时配置：ReadTimeout、WriteTimeout、IdleTimeout

### Testing Requirements
- 运行测试：`go test ./internal/proxy/...`
- 测试请求转发、WebSocket 升级、超时处理

### Common Patterns
- 代理请求时复制原始头部，添加 X-Forwarded-For
- WebSocket 使用 goroutine 双向转发
- 错误响应返回 502（Bad Gateway）或 504（Gateway Timeout）

## Dependencies

### Internal
- `../loadbalance/` - 目标选择和健康检查
- `../config/` - 代理配置

### External
- `github.com/valyala/fasthttp` - HTTP 客户端/服务器

<!-- MANUAL: -->