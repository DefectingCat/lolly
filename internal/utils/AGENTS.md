<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-04-13 | Updated: 2026-04-13 -->

# utils

## Purpose
HTTP 错误处理工具包，提供统一的错误响应助手函数，减少代码库中散落的 `ctx.Error` 调用模式。

## Key Files

| File | Description |
|------|-------------|
| `httperror.go` | HTTP 错误类型定义和发送助手函数 |

## For AI Agents

### Working In This Directory
- 使用预定义的 `HTTPError` 变量（如 `ErrNotFound`, `ErrForbidden`）而不是手动构造
- 通过 `SendError(ctx, err)` 发送标准错误响应
- 需要额外详情时使用 `SendErrorWithDetail(ctx, err, detail)`

### Testing Requirements
- 无独立测试文件，错误处理在其他模块测试中覆盖

### Common Patterns
```go
// 发送标准错误
utils.SendError(ctx, utils.ErrNotFound)

// 发送带详情的错误
utils.SendErrorWithDetail(ctx, utils.ErrBadGateway, "backend timeout")
```

## Dependencies

### External
- `github.com/valyala/fasthttp` - HTTP 状态码定义

<!-- MANUAL: -->