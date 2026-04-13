<!-- Generated: 2026-04-02 | Updated: 2026-04-13 -->

# lolly

## Purpose
高性能 HTTP 服务器，类似 nginx 的纯 Go 实现。使用 YAML 配置，单二进制运行，支持静态文件服务、反向代理、负载均衡、SSL/TLS、安全控制等功能。

## Key Files

| File | Description |
|------|-------------|
| `main.go` | 程序入口，CLI 参数解析和启动逻辑 |
| `go.mod` | Go 模块定义，依赖 fasthttp、zerolog、yaml.v3 |
| `go.sum` | 依赖版本锁定 |
| `Makefile` | 构建脚本，支持多平台编译、测试、覆盖率 |
| `lolly.yaml` | 默认配置文件示例 |
| `config.example.yaml` | 完整配置文件示例（所有字段枚举） |
| `.gitignore` | Git 忽略规则 |

## Subdirectories

| Directory | Purpose |
|-----------|---------|
| `internal/` | 核心业务代码（不可被外部导入） |
| `bin/` | 编译输出目录 |
| `docs/` | 项目文档和实现计划 |
| `examples/` | Lua 脚本示例 |
| `html/` | 静态 HTML 文件（测试/示例） |
| `scripts/` | 构建/测试辅助脚本（回归检测） |
| `.github/` | CI/CD 工作流定义 |

## For AI Agents

### Working In This Directory
- 这是 Go 项目，使用 `go mod` 管理依赖
- 使用 `make build` 构建二进制文件到 `bin/` 目录
- 使用 `make test` 运行测试，`make test-cover` 生成覆盖率报告
- 配置文件使用 YAML 格式，结构定义在 `internal/config/config.go`
- HTTP 库使用 fasthttp（比 net/http 快 6 倍），路由使用 fasthttp/router
- 日志库使用 zerolog（零分配，JSON 输出）

### Testing Requirements
- 运行测试前确保依赖已下载：`go mod download`
- 测试覆盖率目标 >80%
- 使用 `make check` 运行完整检查（fmt + lint + test）

### Common Patterns
- 配置结构体使用 `yaml` 标签，通过 `gopkg.in/yaml.v3` 解析
- 中间件使用 `fasthttp.RequestHandler` 函数签名
- 版本信息通过 `-ldflags` 在编译时注入
- 信号处理：SIGTERM/SIGINT 快速停止，SIGQUIT 优雅停止

## Dependencies

### External
- `github.com/valyala/fasthttp` - 高性能 HTTP 服务器
- `github.com/fasthttp/router` - 基于 radix tree 的路由器
- `github.com/rs/zerolog` - 零分配 JSON 日志库
- `gopkg.in/yaml.v3` - YAML 解析库

<!-- MANUAL: -->