<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-04-23 | Updated: 2026-04-23 -->

# websocket

## Purpose
WebSocket 示例目录，演示如何使用 Lua 实现 WebSocket 服务器和客户端功能。

## Key Files

| File | Description |
|------|-------------|
| `README.md` | 功能说明和使用指南 |
| `ws_handler.lua` | WebSocket 处理器：握手、消息收发、连接管理 |
| `nginx.conf` | NGINX 配置示例：WebSocket 代理配置 |

## For AI Agents

### Working In This Directory
- WebSocket 需要正确处理 Upgrade 和 Connection 头
- 支持双向消息传递
- 需要处理连接保活（ping/pong）
- 错误处理：连接断开、协议错误

### Testing Requirements
- WebSocket 逻辑通过集成测试验证
- 测试握手、消息收发、连接关闭

### Common Patterns
- 握手验证：检查 `Upgrade: websocket` 头
- 消息循环：`while true do local data = ws:recv() end`
- 心跳：定期发送 ping 帧

## Dependencies

### Internal
- `../../../internal/lua/` - Lua 脚本引擎
- `../../../internal/proxy/websocket.go` - WebSocket 代理实现

<!-- MANUAL: -->
