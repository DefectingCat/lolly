# WebSocket 示例

基于 Lua 沙箱的 WebSocket 连接处理示例，展示如何在 `lolly` 项目中实现 WebSocket 通信。

## 功能

- 连接验证（Token 校验）
- 消息路由与处理框架
- 心跳保活
- 安全沙箱隔离

## 文件说明

| 文件 | 用途 |
|------|------|
| `nginx.conf` | Nginx + Lua 配置示例 |
| `ws_handler.lua` | WebSocket 消息处理逻辑 |

## 使用方式

1. 将 `nginx.conf` 中的配置集成到你的 Nginx 配置中
2. 根据实际情况修改 `ws_handler.lua` 中的验证逻辑和消息处理
3. 确保 `content_by_lua_file` 指向正确的脚本路径
