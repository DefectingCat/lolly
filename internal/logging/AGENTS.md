<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-04-02 | Updated: 2026-04-02 -->

# logging

## Purpose
日志模块，使用 zerolog 提供零分配的 JSON 日志输出和访问日志记录。

## Key Files

| File | Description |
|------|-------------|
| `logging.go` | 日志初始化和访问日志记录：Init()、LogAccess()、parseLevel() |

## For AI Agents

### Working In This Directory
- 使用 zerolog 的链式 API 记录日志
- `Init(level, pretty)` 初始化：pretty=true 使用 ConsoleWriter（开发模式）
- `LogAccess()` 记录请求日志：method、path、status、size、duration、remote_addr
- 日志级别：debug、info、warn、error（默认 info）

### Testing Requirements
- 运行测试：`go test ./internal/logging/...`
- 测试日志级别解析和格式化输出

### Common Patterns
- 全局日志实例 `log`，通过 `Init()` 初始化
- zerolog 的零分配设计适合高并发场景
- 生产模式输出 JSON 格式，便于日志采集系统解析

## Dependencies

### External
- `github.com/rs/zerolog` - 零分配 JSON 日志库
- `github.com/valyala/fasthttp` - 获取请求信息

<!-- MANUAL: -->