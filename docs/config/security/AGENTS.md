<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-04-23 | Updated: 2026-04-23 -->

# security

## Purpose
安全配置示例目录，包含访问控制、认证、限流、连接限制、安全头部等安全防护配置。

## Key Files

| File | Description |
|------|-------------|
| `access-control.conf` | 访问控制：IP 白名单/黑名单、allow/deny 规则 |
| `basic-auth.conf` | Basic 认证：用户名密码保护、htpasswd 文件 |
| `auth-request.conf` | 子请求认证：外部认证服务集成 |
| `rate-limit.conf` | 请求限流：令牌桶、漏桶、请求速率控制 |
| `conn-limit.conf` | 连接限制：并发连接数、单 IP 连接限制 |
| `security-headers.conf` | 安全头部：X-Frame-Options、CSP、HSTS |

## For AI Agents

### Working In This Directory
- 访问控制按顺序匹配：先检查黑名单，再检查白名单
- Basic Auth 使用 bcrypt 密码哈希
- 限流使用令牌桶算法，支持按 IP/全局限流
- 安全头部建议全部启用

### Testing Requirements
- 安全配置通过单元测试验证
- 运行测试：`go test ./internal/middleware/security/...`

### Common Patterns
- 认证失败返回 401 Unauthorized
- 限流触发返回 429 Too Many Requests
- 访问拒绝返回 403 Forbidden

## Dependencies

### Internal
- `../../../internal/middleware/security/` - 安全中间件实现
- `../../../internal/config/` - 安全配置解析

<!-- MANUAL: -->
