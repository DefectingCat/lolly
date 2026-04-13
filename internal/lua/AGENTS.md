<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-04-13 | Updated: 2026-04-13 -->

# lua

## Purpose
Lua 脚本嵌入引擎，提供类似 OpenResty 的 Lua 沙箱环境，支持请求处理、定时器、共享字典等 API。

## Key Files

| File | Description |
|------|-------------|
| `engine.go` | LuaEngine 核心：协程池、代码缓存、调度器 |
| `config.go` | 引擎配置：超时、并发限制、库开关 |
| `context.go` | LuaContext：请求上下文 Lua 绑定 |
| `coroutine.go` | LuaCoroutine：协程生命周期管理 |
| `middleware.go` | 中间件集成：access_by_lua、content_by_lua |
| `middleware_config.go` | 中间件配置解析和验证 |
| `shared_dict.go` | 共享字典：线程安全的键值存储 |
| `socket_manager.go` | cosocket 管理：TCP 连接池 |
| `timer_manager.go` | 定时器管理：ngx.timer.at 实现 |
| `cache.go` | 字节码缓存：预编译脚本缓存 |
| `register.go` | API 注册：ngx 表初始化 |
| `filter_writer.go` | 响应过滤器：body_filter_by_lua |
| `api_*.go` | ngx API 实现：req、resp、ctx、var、log、timer、socket、location、shared_dict |

## For AI Agents

### Working In This Directory
- LuaEngine 是 HTTP Server 实例级单例，通过 `NewEngine(config)` 创建
- 协程通过 `engine.NewCoroutine(req)` 创建，使用后自动释放回池
- 定时器回调在专用调度器 LState 中执行，不能捕获闭包变量（使用 shared_dict 传递数据）
- API 分为安全（timer 可用）和不安全（仅请求协程可用）两类

### Testing Requirements
- 运行测试：`go test ./internal/lua/...`
- 基准测试：`go test -bench=. ./internal/lua/...`
- 测试覆盖：协程生命周期、API 限制、并发安全

### Common Patterns
```go
// 创建引擎
engine, err := lua.NewEngine(config)
engine.InitSchedulerLState() // 启用定时器

// 创建共享字典
engine.CreateSharedDict("cache", 1000)

// 中间件配置
mw := lua.NewMiddleware(mwConfig, engine)
handler = mw.Wrap(handler)
```

## Dependencies

### Internal
- `internal/config` - Lua 中间件配置结构
- `internal/logging` - 日志输出
- `internal/middleware` - 中间件接口

### External
- `github.com/yuin/gopher-lua` - Lua 解释器
- `github.com/valyala/fasthttp` - HTTP 请求上下文

<!-- MANUAL: -->