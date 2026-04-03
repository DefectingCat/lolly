<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-04-03 | Updated: 2026-04-03 -->

# ssl

## Purpose
SSL/TLS 模块，提供证书加载、OCSP Stapling 和 TLS 配置管理功能。

## Key Files

| File | Description |
|------|-------------|
| `ssl.go` | SSL/TLS 核心：证书加载、TLS 配置、SNI 支持、会话缓存 |
| `ocsp.go` | OCSP Stapling：在线证书状态查询、缓存、自动刷新 |
| `ssl_test.go` | SSL/TLS 测试 |
| `ocsp_test.go` | OCSP 测试 |

## For AI Agents

### Working In This Directory
- 支持多证书加载，按 SNI 匹配
- OCSP Stapling 提升 TLS 握手性能
- TLS 配置强制使用安全参数（TLS 1.2+、强密码套件）
- 会话缓存减少握手开销

### Testing Requirements
- 运行测试：`go test ./internal/ssl/...`
- 测试证书加载、OCSP 查询、TLS 配置

### Common Patterns
- 证书格式支持 PEM
- OCSP 响应缓存 1 小时
- 会话票据自动轮换

## Dependencies

### Internal
- `../config/` - SSL 配置

### External
- `crypto/tls` - Go 标准库 TLS 实现
- `golang.org/x/crypto/ocsp` - OCSP 协议支持

<!-- MANUAL: -->