<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-04-24 | Updated: 2026-04-24 -->

# e2e

## Purpose
端到端测试模块，验证完整功能的集成测试。使用 testcontainers 启动真实后端服务进行测试。

## Key Files

| File | Description |
|------|-------------|
| `e2e_test.go` | 测试入口和通用测试辅助函数 |
| `cache_e2e_test.go` | 代理缓存端到端测试 |
| `healthcheck_e2e_test.go` | 健康检查端到端测试 |
| `http2_e2e_test.go` | HTTP/2 端到端测试 |
| `loadbalance_e2e_test.go` | 负载均衡端到端测试 |
| `proxy_e2e_test.go` | 反向代理端到端测试 |
| `ssl_e2e_test.go` | SSL/TLS 端到端测试 |
| `static_e2e_test.go` | 静态文件服务端到端测试 |
| `websocket_e2e_test.go` | WebSocket 代理端到端测试 |

## Subdirectories

| Directory | Purpose |
|-----------|---------|
| `testutil/` | 测试工具函数（容器、配置、证书） |
| `certs/` | 测试用 TLS 证书 |
| `configs/` | 测试用配置文件 |

## For AI Agents

### Working In This Directory
- 端到端测试需要 Docker 环境运行 testcontainers
- 测试启动真实 lolly 服务器和 mock 后端
- 使用 `testutil` 包提供的辅助函数创建测试环境
- 测试名称遵循 `TestE2E_<功能>_<场景>` 格式

### Testing Requirements
- 运行测试：`go test ./internal/e2e/... -tags=e2e`
- 需要 Docker 运行
- 测试可能需要较长时间（启动容器）

### Common Patterns
```go
// 创建测试环境
env := testutil.NewTestEnv(t)
defer env.Cleanup()

// 启动 lolly 服务器
env.StartServer(config)
defer env.StopServer()

// 启动 mock 后端
backend := env.StartMockBackend()
defer backend.Close()

// 发送测试请求
resp := env.Request("GET", "/api/test", nil)
```

## Dependencies

### Internal
- `./testutil/` - 测试工具函数
- `../server/` - HTTP 服务器
- `../config/` - 配置加载

### External
- `github.com/testcontainers/testcontainers-go` - 容器化测试

<!-- MANUAL: -->
