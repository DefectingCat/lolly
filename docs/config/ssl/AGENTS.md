<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-04-23 | Updated: 2026-04-23 -->

# ssl

## Purpose
SSL/TLS 配置示例目录，包含 HTTPS、mTLS、OCSP Stapling、HSTS 等安全传输配置示例。

## Key Files

| File | Description |
|------|-------------|
| `basic-ssl.conf` | 基础 HTTPS 配置：证书、TLS 协议、加密套件、HTTP/2 |
| `hsts.conf` | HSTS 配置：Strict-Transport-Security、预加载 |
| `mtls.conf` | 双向 TLS 认证：客户端证书验证、CA 配置 |
| `ocsp-stapling.conf` | OCSP Stapling：在线证书状态验证、缓存配置 |

## For AI Agents

### Working In This Directory
- TLS 1.0/1.1 已弃用，推荐使用 TLS 1.2/1.3
- 加密套件推荐使用 AEAD（AES-GCM、CHACHA20-POLY1305）
- Session Tickets 禁用更安全，但会影响会话恢复性能

### Testing Requirements
- SSL 配置通过集成测试验证
- 测试证书位于 `internal/ssl/testdata/`

### Common Patterns
- 证书路径：`ssl.cert`, `ssl.key`, `ssl.cert_chain`
- 协议配置：`ssl.protocols: ["TLSv1.2", "TLSv1.3"]`
- HTTP/2 启用：`ssl.http2.enabled: true`

## Dependencies

### Internal
- `../../../internal/ssl/` - SSL 证书加载和管理
- `../../../internal/sslutil/` - SSL 工具函数

<!-- MANUAL: -->
