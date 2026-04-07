<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-04-07 | Updated: 2026-04-07 -->

# bodylimit

## Purpose
HTTP 请求体大小限制中间件，防止客户端通过发送超大请求体导致服务器资源耗尽，支持全局配置和路径级别覆盖。

## Key Files

| File | Description |
|------|-------------|
| `bodylimit.go` | 请求体限制中间件实现，支持大小解析、路径级别配置 |
| `bodylimit_test.go` | 中间件单元测试 |

## For AI Agents

### Working In This Directory
- 使用 `io.LimitReader` 强制限制实际读取的字节数
- 支持路径级别配置覆盖全局配置（最长匹配优先）
- 大小字符串解析支持 b、kb、mb、gb 单位（不区分大小写）
- 超限返回 413 Request Entity Too Large

### Testing Requirements
- 运行测试：`go test ./internal/middleware/bodylimit/...`
- 测试覆盖：大小解析、路径匹配、超限处理

### Common Patterns
- 创建中间件：`bodylimit.New("10mb")`
- 添加路径配置：`bl.AddPathLimit("/upload", "100mb")`
- 获取路径限制：`bl.GetLimit(path)`
- 解析大小：`ParseSize("1kb")` → 1024
- 格式化大小：`FormatSize(1024)` → "1kb"

## Dependencies

### External
- `github.com/valyala/fasthttp` - HTTP 框架

<!-- MANUAL: -->