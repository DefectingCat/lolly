<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-04-03 | Updated: 2026-04-03 -->

# compression

## Purpose
响应压缩中间件，支持 Gzip 和 Deflate 压缩，自动根据 Accept-Encoding 头选择压缩方式。

## Key Files

| File | Description |
|------|-------------|
| `compression.go` | 压缩中间件：Compression 结构体、Process() 方法、压缩级别配置 |
| `compression_test.go` | 压缩测试 |

## For AI Agents

### Working In This Directory
- 支持 Gzip 和 Deflate 压缩
- 自动检测 Accept-Encoding 头选择压缩方式
- 压缩级别可配置（1-9，默认 6）
- 小于 1KB 的响应不压缩

### Testing Requirements
- 运行测试：`go test ./internal/middleware/compression/...`
- 测试压缩检测、响应处理、级别配置

### Common Patterns
- 使用 fasthttp 的内置压缩支持
- Content-Type 过滤：仅压缩 text/* 和 application/json
- Vary: Accept-Encoding 头自动添加

## Dependencies

### Internal
- `../` - 中间件接口定义
- `../../config/` - 压缩配置

### External
- `github.com/valyala/fasthttp` - HTTP 框架（内置压缩）

<!-- MANUAL: -->