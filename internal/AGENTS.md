<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-04-02 | Updated: 2026-04-02 -->

# internal

## Purpose
核心业务代码目录，包含服务器、配置、处理器、中间件、日志等模块。Go 的 internal 包机制确保这些代码不可被外部项目导入。

## Subdirectories

| Directory | Purpose |
|-----------|---------|
| `app/` | 应用程序入口和运行逻辑（启动、信号处理、版本信息） |
| `config/` | 配置解析、验证和默认值生成 |
| `handler/` | HTTP 请求处理器（路由、静态文件） |
| `logging/` | 日志系统（zerolog 初始化、访问日志） |
| `middleware/` | 中间件框架（接口定义、链式组合） |
| `server/` | HTTP 服务器核心和虚拟主机管理 |

## For AI Agents

### Working In This Directory
- 所有包使用 `rua.plus/lolly/internal/{package}` 导入路径
- 各子包有独立职责，遵循 Go 包设计原则
- 添加新功能时应参考 `docs/plan.md` 确定所属模块
- 测试文件与源文件同目录，使用 `_test.go` 后缀

### Testing Requirements
- 每个包应有对应的测试文件
- 运行测试：`go test ./internal/...`
- 测试覆盖率目标 >80%

### Common Patterns
- 使用 fasthttp 的 `RequestHandler` 函数签名处理请求
- 配置结构体使用 `yaml` 标签
- 中间件通过 `Chain.Apply()` 逆序包装
- 服务器通过 `fasthttp.Server` 配置超时和连接限制

## Dependencies

### External
- `github.com/valyala/fasthttp` - HTTP 服务器框架
- `github.com/fasthttp/router` - 路由器
- `github.com/rs/zerolog` - 日志库
- `gopkg.in/yaml.v3` - YAML 解析

<!-- MANUAL: -->