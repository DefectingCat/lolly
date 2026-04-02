<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-04-02 | Updated: 2026-04-02 -->

# server

## Purpose
HTTP 服务器核心，提供服务器创建、启动、停止和虚拟主机管理功能。

## Key Files

| File | Description |
|------|-------------|
| `server.go` | 服务器核心：Server 结构体、Start()、Stop()、GracefulStop() |
| `vhost.go` | 虚拟主机管理：VHostManager、VirtualHost、按 Host 头匹配 |

## For AI Agents

### Working In This Directory
- 使用 `fasthttp.Server` 配置超时和连接限制
- `Start()` 初始化日志、创建路由、注册静态处理器、应用中间件
- 默认超时：ReadTimeout 30s、WriteTimeout 30s、IdleTimeout 120s
- `Stop()` 快速停止，`GracefulStop()` 等待请求完成
- `VHostManager` 支持多虚拟主机，按 Host 头匹配，有默认 fallback

### Testing Requirements
- 运行测试：`go test ./internal/server/...`
- 测试服务器启动/停止、虚拟主机匹配

### Common Patterns
- 服务器配置从 `config.Config` 读取
- 路由使用 `handler.NewRouter()` 创建
- 中间件通过 `middleware.NewChain().Apply()` 应用
- Host 头匹配时去除端口号

## Dependencies

### Internal
- `../config/` - 服务器配置
- `../handler/` - 路由和静态处理器
- `../logging/` - 日志初始化
- `../middleware/` - 中间件链

### External
- `github.com/valyala/fasthttp` - HTTP 服务器框架

<!-- MANUAL: -->