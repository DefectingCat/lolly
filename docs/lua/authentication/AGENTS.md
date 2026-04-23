<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-04-23 | Updated: 2026-04-23 -->

# authentication

## Purpose
认证示例目录，演示如何使用 Lua 实现 Basic Auth、JWT 验证等认证功能。

## Key Files

| File | Description |
|------|-------------|
| `README.md` | 功能说明和使用指南 |
| `basic_auth.lua` | Basic Auth 认证：用户名密码验证、bcrypt 哈希 |
| `jwt_validate.lua` | JWT 验证：令牌解析、签名验证、过期检查 |
| `nginx.conf` | NGINX 配置示例：认证钩子集成 |

## For AI Agents

### Working In This Directory
- Basic Auth 使用 bcrypt 密码哈希存储
- JWT 验证支持 HS256、RS256 算法
- 认证失败返回 401 Unauthorized
- 可与外部认证服务集成

### Testing Requirements
- 认证逻辑通过单元测试验证
- 测试密码验证、令牌解析正确性

### Common Patterns
- Basic Auth：解析 Authorization 头，验证用户名密码
- JWT：解析 Bearer 令牌，验证签名和有效期
- 错误响应：`ngx.exit(401)`

## Dependencies

### Internal
- `../../../internal/lua/` - Lua 脚本引擎
- `../../../internal/middleware/security/auth.go` - 认证中间件

<!-- MANUAL: -->
