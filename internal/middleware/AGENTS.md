<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-04-02 | Updated: 2026-04-02 -->

# middleware

## Purpose
中间件框架，定义统一的中间件接口和链式组合机制。

## Key Files

| File | Description |
|------|-------------|
| `middleware.go` | 中间件接口和链：Middleware 接口、Chain 结构体、Apply 方法 |

## For AI Agents

### Working In This Directory
- `Middleware` 接口定义：`Name() string` 和 `Process(next) RequestHandler`
- `Chain` 用于组合多个中间件，按注册顺序逆序包装
- `Apply()` 从最后一个中间件开始包装，确保执行顺序正确
- 后续阶段将添加具体中间件实现（安全、压缩、重写等）

### Testing Requirements
- 运行测试：`go test ./internal/middleware/...`
- 测试链式组合和执行顺序

### Common Patterns
- 中间件使用 `fasthttp.RequestHandler` 函数签名
- 包装模式：`func(ctx) { preProcess(); next(ctx); postProcess() }`
- 空 Chain 的 `Apply()` 直接返回原始 handler

## Dependencies

### External
- `github.com/valyala/fasthttp` - HTTP 框架

<!-- MANUAL: -->