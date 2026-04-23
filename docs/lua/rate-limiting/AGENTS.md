<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-04-23 | Updated: 2026-04-23 -->

# rate-limiting

## Purpose
限流示例目录，演示如何使用 Lua 实现请求限流、令牌桶算法等限流功能。

## Key Files

| File | Description |
|------|-------------|
| `README.md` | 功能说明和使用指南 |
| `access.lua` | 限流实现：令牌桶、滑动窗口、请求计数 |
| `nginx.conf` | NGINX 配置示例：限流钩子集成 |

## For AI Agents

### Working In This Directory
- 令牌桶算法：固定速率生成令牌，请求消耗令牌
- 滑动窗口：统计时间窗口内请求数
- 支持按 IP、全局限流
- 限流触发返回 429 Too Many Requests

### Testing Requirements
- 限流逻辑通过单元测试验证
- 测试令牌桶、滑动窗口算法正确性

### Common Patterns
- 令牌桶：`local tokens = min(max_tokens, tokens + rate * elapsed)`
- 限流检查：`if tokens < 1 then ngx.exit(429) end`
- 滑动窗口：使用共享字典存储请求时间戳

## Dependencies

### Internal
- `../../../internal/lua/` - Lua 脚本引擎
- `../../../internal/middleware/security/ratelimit.go` - 限流中间件

<!-- MANUAL: -->
