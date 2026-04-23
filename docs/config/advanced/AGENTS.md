<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-04-23 | Updated: 2026-04-23 -->

# advanced

## Purpose
高级功能配置示例目录，包含 WebSocket、gRPC、HTTP/2、HTTP/3、Stream 代理等高级协议配置。

## Key Files

| File | Description |
|------|-------------|
| `websocket.conf` | WebSocket 代理配置：Upgrade 头处理、长连接支持 |
| `grpc.conf` | gRPC 代理配置：HTTP/2 透传、超时设置 |
| `http2.conf` | HTTP/2 配置：多路复用、服务器推送、流控制 |
| `http3.conf` | HTTP/3 (QUIC) 配置：UDP 传输、0-RTT 连接 |
| `stream-tcp.conf` | TCP Stream 代理：四层负载均衡、连接复用 |
| `stream-udp.conf` | UDP Stream 代理：DNS 负载均衡、无状态转发 |

## For AI Agents

### Working In This Directory
- HTTP/2 需要 HTTPS，HTTP/3 需要 QUIC 支持
- Stream 代理工作在四层（TCP/UDP），不解析 HTTP
- WebSocket 需要正确处理 Upgrade 和 Connection 头

### Testing Requirements
- 高级协议通过集成测试验证
- HTTP/3 测试需要 QUIC 客户端支持

### Common Patterns
- WebSocket：`proxy_set_header Upgrade $http_upgrade`
- gRPC：`grpc_pass grpc://upstream`
- Stream：`listen 12345` 在 stream 块中

## Dependencies

### Internal
- `../../../internal/http2/` - HTTP/2 实现
- `../../../internal/http3/` - HTTP/3 (QUIC) 实现
- `../../../internal/stream/` - Stream 代理实现
- `../../../internal/proxy/websocket.go` - WebSocket 代理

<!-- MANUAL: -->
