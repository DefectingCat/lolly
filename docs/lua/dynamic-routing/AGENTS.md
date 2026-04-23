<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-04-23 | Updated: 2026-04-23 -->

# dynamic-routing

## Purpose
动态路由示例目录，演示如何使用 Lua 实现基于路径、方法、Header 的动态路由。

## Key Files

| File | Description |
|------|-------------|
| `README.md` | 功能说明和使用指南 |
| `router.lua` | 路由器实现：路由规则匹配、上游选择 |
| `nginx.conf` | NGINX 配置示例：动态路由集成 |

## For AI Agents

### Working In This Directory
- 路由规则支持正则表达式匹配
- 支持按路径、方法、Header 组合匹配
- 动态上游选择支持负载均衡
- 路由规则可热更新

### Testing Requirements
- 路由逻辑通过单元测试验证
- 测试路由匹配、上游选择正确性

### Common Patterns
- 路由表：`local routes = { ["/api/*"] = "api_upstream" }`
- 匹配逻辑：遍历路由表，正则匹配请求路径
- 上游设置：`ngx.var.upstream = matched_upstream`

## Dependencies

### Internal
- `../../../internal/lua/` - Lua 脚本引擎
- `../../../internal/loadbalance/` - 负载均衡实现

<!-- MANUAL: -->
