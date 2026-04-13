<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-04-13 | Updated: 2026-04-13 -->

# sslutil

## Purpose
SSL/TLS 工具函数包，提供证书池加载功能，用于配置 SSL 客户端验证和 CA 证书信任链。

## Key Files

| File | Description |
|------|-------------|
| `certpool.go` | 证书池加载函数，支持 PEM 格式 |

## For AI Agents

### Working In This Directory
- `LoadCertPool(certFile, _)` 加载 PEM 格式证书池（参数二为历史兼容占位）
- `LoadCACertPool(caFile)` 加载 CA 证书池的便捷函数
- 返回 `*x509.CertPool` 用于 `TLSConfig.RootCAs` 或 `ClientCAs`

### Testing Requirements
- 无独立测试文件，证书加载在 SSL 模块集成测试中覆盖

### Common Patterns
```go
// 加载 CA 证书池用于客户端验证
caPool, err := sslutil.LoadCACertPool("/path/to/ca.crt")
tlsConfig.ClientCAs = caPool
```

## Dependencies

### External
- `crypto/x509` - 证书池类型
- `os` - 文件读取

<!-- MANUAL: -->