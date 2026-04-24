<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-04-24 | Updated: 2026-04-24 -->

# testutil

## Purpose
端到端测试工具包，提供测试环境搭建、容器管理、证书生成、配置辅助等功能。

## Key Files

| File | Description |
|------|-------------|
| `setup.go` | 测试环境初始化：NewTestEnv、Cleanup |
| `config.go` | 配置辅助：创建测试配置、临时文件 |
| `constants.go` | 测试常量：默认端口、超时时间 |
| `container.go` | 容器管理：启动 mock 后端、清理容器 |
| `container_test.go` | 容器管理测试 |
| `certs.go` | 证书生成：自签名证书、CA 证书 |
| `ssl.go` | SSL 辅助：TLS 配置、证书验证 |
| `websocket.go` | WebSocket 辅助：WS 客户端、消息收发 |
| `concurrent.go` | 并发测试辅助：并发请求、结果收集 |

## For AI Agents

### Working In This Directory
- `NewTestEnv(t)` 创建测试环境，返回清理函数
- 测试环境自动管理临时文件和端口分配
- 容器化测试需要 Docker 运行
- 证书生成用于 TLS 测试

### Testing Requirements
- 运行测试：`go test ./internal/e2e/testutil/...`
- 部分测试需要 Docker

### Common Patterns
```go
// 创建测试环境
env := testutil.NewTestEnv(t)
defer env.Cleanup()

// 创建临时配置文件
cfgPath := env.CreateConfig(serverConfig)

// 启动容器化后端
container := env.StartContainer(containerConfig)
defer container.Terminate()

// 生成测试证书
cert, key := env.GenerateCertificate()
```

## Dependencies

### Internal
- `../../config/` - 配置结构体

### External
- `github.com/testcontainers/testcontainers-go` - 容器管理

<!-- MANUAL: -->
