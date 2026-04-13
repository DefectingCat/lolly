<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-04-13 | Updated: 2026-04-13 -->

# lua

## Purpose
Lua 功能文档目录，包含 API 参考、使用指南和最佳实践示例。

## Key Files

| File | Description |
|------|-------------|
| `API.md` | Lua Engine API 参考（定时器限制、安全/不安全 API 区分） |

## Subdirectories

| Directory | Purpose |
|-----------|---------|
| `api-gateway/` | API 网关使用示例 |
| `authentication/` | 认证功能示例 |
| `caching/` | 缓存使用示例 |
| `dynamic-routing/` | 动态路由示例 |
| `logging-monitoring/` | 日志和监控示例 |
| `middleware/` | 中间件使用示例 |
| `rate-limiting/` | 限流功能示例 |
| `websocket/` | WebSocket 功能示例 |

## For AI Agents

### Working In This Directory
- `API.md` 是关键参考，说明定时器回调限制和 API 可用性
- 定时器回调不能捕获闭包变量，必须使用 `ngx.shared.DICT` 传递数据
- Request-scoped API（ngx.req、ngx.resp、ngx.var、ngx.ctx）在定时器回调中不可用

### Testing Requirements
- Lua 功能通过 `../../internal/lua` 测试验证

### Common Patterns
- 安全 API（timer 可用）：`ngx.shared.DICT.*`, `ngx.log`, `ngx.timer.*`
- 不安全 API（仅请求协程）：`ngx.req.*`, `ngx.resp.*`, `ngx.var.*`, `ngx.ctx.*`

## Dependencies

### Internal
- `../../internal/lua` - Lua 引擎实现
- `../../examples/lua-scripts` - 脚本示例

<!-- MANUAL: -->