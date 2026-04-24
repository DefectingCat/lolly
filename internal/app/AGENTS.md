<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-04-02 | Updated: 2026-04-24 -->

# app

## Purpose
应用程序入口包，提供启动逻辑、信号处理、版本信息显示、配置生成和 nginx 配置导入功能。

## Key Files

| File | Description |
|------|-------------|
| `app.go` | 核心逻辑：Run 入口、startServer、printVersion、generateConfig |
| `import.go` | nginx 配置导入：ImportNginxConfig、转换警告处理 |
| `app_test.go` | 单元测试：版本显示、配置生成、服务器启动测试 |

## For AI Agents

### Working In This Directory
- 版本信息变量通过 `-ldflags` 在编译时注入，不要硬编码修改
- `Run()` 是程序入口函数，返回退出码
- 信号处理：SIGTERM/SIGINT 快速停止，SIGQUIT 优雅停止（等待 30s）
- `generateConfig()` 生成默认 YAML 配置，调用 `config.DefaultConfig()`
- `ImportNginxConfig()` 将 nginx 配置转换为 lolly YAML 格式

### Testing Requirements
- 测试文件 `app_test.go` 包含启动逻辑测试
- 运行测试：`go test ./internal/app/...`
- 测试覆盖率目标 >80%

### Common Patterns
- 使用 `os.Signal` 和 `signal.Notify` 处理系统信号
- 服务器启动在 goroutine 中执行，通过 channel 等待错误或信号
- `shutdownTimeout` 定义优雅停止超时时间

## Dependencies

### Internal
- `../config/` - 配置加载和验证
- `../server/` - HTTP 服务器创建和启动
- `../converter/nginx/` - nginx 配置转换器

<!-- MANUAL: -->