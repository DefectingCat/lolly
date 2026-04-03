<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-04-03 | Updated: 2026-04-03 -->

# accesslog

## Purpose
访问日志中间件，记录每个请求的详细信息，包括方法、路径、状态码、响应时间和客户端地址。

## Key Files

| File | Description |
|------|-------------|
| `accesslog.go` | 访问日志中间件：AccessLog 结构体、Process() 方法、日志格式化 |
| `accesslog_test.go` | 访问日志测试 |

## For AI Agents

### Working In This Directory
- 记录请求信息：method、path、status、size、duration、remote_addr
- 支持自定义日志格式
- 响应时间单位为毫秒
- 日志级别为 info

### Testing Requirements
- 运行测试：`go test ./internal/middleware/accesslog/...`
- 测试日志格式化、字段提取

### Common Patterns
- 使用 zerolog 的链式 API
- duration 通过 ctx.Time() 获取
- 可配置排除路径（如健康检查）

## Dependencies

### Internal
- `../` - 中间件接口定义
- `../../logging/` - 日志实例

### External
- `github.com/rs/zerolog` - 日志库
- `github.com/valyala/fasthttp` - HTTP 框架

<!-- MANUAL: -->