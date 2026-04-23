<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-04-23 | Updated: 2026-04-23 -->

# api-gateway

## Purpose
API 网关示例项目，演示如何使用 Lua 脚本实现动态路由、认证、限流、健康检查等网关核心功能。

## Key Files

| File | Description |
|------|-------------|
| `README.md` | 项目说明：功能特性、快速开始、配置示例 |
| `gateway.lua` | 网关主逻辑：路由、认证、限流、错误处理 |
| `upstream.lua` | 上游服务管理：健康检查、故障剔除、动态负载均衡 |
| `nginx.conf` | NGINX 配置示例：Lua 脚本集成方式 |

## For AI Agents

### Working In This Directory
- 基于 OpenResty / lua-nginx-module 风格
- 路由规则在 `gateway.lua` 的 `routes` 表中定义
- 上游节点在 `upstream.lua` 的 `upstreams` 表中配置
- 生产环境建议使用 Redis 替代共享字典存储

### Testing Requirements
- Lua 脚本通过集成测试验证
- 测试框架：`internal/lua/` 模块测试

### Common Patterns
- 路由匹配：路径 + 方法 + Header 组合
- 限流算法：令牌桶滑动窗口
- 健康检查：被动探测 + 故障剔除

## Dependencies

### Internal
- `../../../internal/lua/` - Lua 脚本引擎实现
- `../../../internal/loadbalance/` - 负载均衡策略

### External
- OpenResty / lua-nginx-module - Lua 运行环境

<!-- MANUAL: -->
