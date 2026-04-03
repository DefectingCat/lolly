<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-04-03 | Updated: 2026-04-03 -->

# security

## Purpose
安全中间件集合，提供访问控制、认证、限流和安全头部功能，保护服务器免受常见攻击。

## Key Files

| File | Description |
|------|-------------|
| `access.go` | 访问控制：IP 白名单/黑名单、地理位置限制、allow/deny 规则 |
| `auth.go` | 认证中间件：Basic Auth、Bearer Token、JWT 验证 |
| `ratelimit.go` | 限流中间件：请求限流、连接限流、令牌桶算法 |
| `headers.go` | 安全头部：X-Frame-Options、X-Content-Type-Options、CSP、HSTS |
| `*_test.go` | 各模块单元测试 |

## For AI Agents

### Working In This Directory
- 访问控制支持 CIDR 格式 IP 范围
- Basic Auth 使用 bcrypt 密码哈希
- 限流使用令牌桶算法，支持按 IP/全局限流
- 安全头部可配置启用/禁用

### Testing Requirements
- 运行测试：`go test ./internal/middleware/security/...`
- 测试 IP 匹配、密码验证、限流逻辑、头部设置

### Common Patterns
- 访问控制按顺序匹配：先检查黑名单，再检查白名单
- 限流阈值：请求速率（req/s）、并发连接数
- 认证失败返回 401，限流触发返回 429

## Dependencies

### Internal
- `../` - 中间件接口定义
- `../../config/` - 安全配置

### External
- `golang.org/x/crypto/bcrypt` - 密码哈希
- `github.com/valyala/fasthttp` - HTTP 框架

<!-- MANUAL: -->