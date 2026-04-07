<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-04-07 | Updated: 2026-04-07 -->

# errorintercept

## Purpose
HTTP 错误拦截中间件，用于应用自定义错误页面。拦截 4xx/5xx 响应并替换为预加载的错误页面内容。

## Key Files

| File | Description |
|------|-------------|
| `errorintercept.go` | 错误拦截中间件实现，与 ErrorPageManager 配合使用 |

## For AI Agents

### Working In This Directory
- 错误页面在启动时预加载，运行时不进行文件 I/O
- 只拦截 4xx 和 5xx 错误状态码
- 支持可选的响应状态码覆盖
- 与 `internal/handler.ErrorPageManager` 配合使用

### Testing Requirements
- 运行测试：`go test ./internal/middleware/errorintercept/...`
- 测试需模拟 ErrorPageManager 和错误响应

### Common Patterns
- 创建中间件：`errorintercept.New(errorPageManager)`
- 检查配置状态：`ei.manager.IsConfigured()`
- 获取管理器：`ei.GetManager()`

## Dependencies

### Internal
- `rua.plus/lolly/internal/handler` - ErrorPageManager 错误页面管理

### External
- `github.com/valyala/fasthttp` - HTTP 框架

<!-- MANUAL: -->