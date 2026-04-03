<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-04-03 | Updated: 2026-04-03 -->

# http3

## Purpose
HTTP/3 (QUIC) 协议支持模块，提供基于 quic-go 的 HTTP/3 服务器实现，与现有 fasthttp handler 集成。

## Key Files

| File | Description |
|------|-------------|
| `server.go` | HTTP/3 服务器核心实现（启动、停止、优雅关闭、统计） |
| `adapter.go` | fasthttp.RequestHandler 与 http.Handler 适配层 |

## For AI Agents

### Working In This Directory
- HTTP/3 需要 TLS 配置，必须与 `internal/ssl/` 模块配合使用
- 使用 quic-go 库实现 QUIC 协议
- 通过 Adapter 将 fasthttp handler 转换为标准库 http.Handler
- 配置结构体定义在 `internal/config/config.go` 的 `HTTP3Config`

### Testing Requirements
- 测试需要模拟 QUIC 连接
- 运行测试：`go test ./internal/http3/...`

### Common Patterns
- 使用 `sync.Pool` 复用 RequestCtx 对象
- 使用 `quic.ListenEarly` 创建 0-RTT 支持的监听器
- Alt-Svc 头用于告知客户端可使用 HTTP/3

## Dependencies

### Internal
- `rua.plus/lolly/internal/config` - HTTP3Config 配置结构
- `rua.plus/lolly/internal/logging` - 日志输出

### External
- `github.com/quic-go/quic-go` - QUIC 协议实现
- `github.com/quic-go/quic-go/http3` - HTTP/3 服务器
- `github.com/valyala/fasthttp` - HTTP 处理器接口

<!-- MANUAL: -->