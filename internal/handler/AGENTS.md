<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-04-02 | Updated: 2026-04-02 -->

# handler

## Purpose
HTTP 请求处理器，包含路由管理和静态文件服务功能。

## Key Files

| File | Description |
|------|-------------|
| `router.go` | 路由器封装：基于 fasthttp/router，支持 GET/POST/PUT/DELETE/HEAD |
| `static.go` | 静态文件处理器：文件路径安全检查、索引文件支持 |

## For AI Agents

### Working In This Directory
- 使用 `fasthttp/router` 进行路由匹配，基于 radix tree 高效查找
- 路径参数语法：`{name}` 命名参数，`{name?}` 可选参数，`{filepath:*}` 捕获所有
- 静态文件处理器包含目录遍历安全检查（拒绝 ".."）
- 索引文件默认为 index.html、index.htm，可配置

### Testing Requirements
- 运行测试：`go test ./internal/handler/...`
- 测试路由匹配、路径安全检查、索引文件逻辑

### Common Patterns
- 使用 `github.com/valyala/fasthttp` 的 `RequestCtx` 处理请求
- `fasthttp.ServeFile()` 用于静态文件响应
- 路由参数通过 `ctx.UserValue("name")` 获取

## Dependencies

### External
- `github.com/fasthttp/router` - 路由器
- `github.com/valyala/fasthttp` - HTTP 框架

<!-- MANUAL: -->