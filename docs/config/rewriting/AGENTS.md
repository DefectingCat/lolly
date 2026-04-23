<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-04-23 | Updated: 2026-04-23 -->

# rewriting

## Purpose
URL 重写配置示例目录，包含 rewrite 规则、重定向等 URL 处理配置。

## Key Files

| File | Description |
|------|-------------|
| `rewrite-rules.conf` | URL 重写规则：正则匹配、变量替换、内部重定向 |
| `redirect.conf` | 重定向配置：301/302 跳转、域名迁移 |

## For AI Agents

### Working In This Directory
- rewrite 使用正则表达式匹配 URL
- last 表示重写后重新匹配 location
- break 表示重写后直接处理，不再匹配
- redirect 返回 302，permanent 返回 301

### Testing Requirements
- 重写规则通过单元测试验证
- 测试正则匹配和变量替换正确性

### Common Patterns
- 正则捕获：`rewrite ^/user/(\d+)$ /profile?id=$1 last`
- 域名迁移：`return 301 https://new.domain.com$request_uri`
- HTTP 转 HTTPS：`return 301 https://$host$request_uri`

## Dependencies

### Internal
- `../../../internal/middleware/rewrite/` - 重写中间件实现
- `../../../internal/handler/` - 请求处理器

<!-- MANUAL: -->
