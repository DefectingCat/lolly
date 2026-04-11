# Lua Middleware 使用指南

## 概述

LuaMiddleware 提供了将 Lua 脚本嵌入 HTTP 请求处理流程的能力，支持在不同执行阶段运行自定义逻辑。

## 快速开始

### 创建 Lua 引擎

```go
import "rua.plus/lolly/internal/lua"

// 创建 Lua 引擎
engine, err := lua.NewEngine(lua.DefaultConfig())
if err != nil {
    log.Fatal(err)
}
defer engine.Close()
```

### 创建单阶段中间件

```go
config := lua.LuaMiddlewareConfig{
    ScriptPath: "/path/to/script.lua",
    Phase:      lua.PhaseContent,  // 内容生成阶段
    Timeout:    30 * time.Second,
    Name:       "my-lua-middleware",
}

middleware, err := lua.NewLuaMiddleware(engine, config)
if err != nil {
    log.Fatal(err)
}
```

### 创建多阶段中间件

```go
multi := lua.NewMultiPhaseLuaMiddleware(engine, "multi-phase")

// 添加不同阶段的脚本
multi.AddPhase(lua.PhaseRewrite, "/scripts/rewrite.lua", 10*time.Second)
multi.AddPhase(lua.PhaseAccess, "/scripts/access.lua", 10*time.Second)
multi.AddPhase(lua.PhaseContent, "/scripts/content.lua", 10*time.Second)
multi.AddPhase(lua.PhaseLog, "/scripts/log.lua", 10*time.Second)
```

## 执行阶段

阶段按以下顺序执行（请求处理流程）：

```
rewrite → access → content → header_filter → body_filter → log
```

| 阶段 | 常量 | 用途 |
|------|------|------|
| Rewrite | `PhaseRewrite` | URL 重写、请求修改 |
| Access | `PhaseAccess` | 访问控制、认证授权 |
| Content | `PhaseContent` | 内容生成（默认阶段） |
| Header Filter | `PhaseHeaderFilter` | 响应头过滤 |
| Body Filter | `PhaseBodyFilter` | 响应体过滤 |
| Log | `PhaseLog` | 日志记录 |

## 可用的 ngx API

在 Lua 脚本中可使用以下 nginx 风格 API：

### ngx.req - 请求操作

```lua
-- 获取请求方法
local method = ngx.req.get_method()

-- 获取请求头
local headers = ngx.req.get_headers()
local content_type = headers["Content-Type"]

-- 设置请求头
ngx.req.set_header("X-Custom", "value")

-- 获取请求体
local body = ngx.req.get_body_data()

-- 设置 URI
ngx.req.set_uri("/new/path")
```

### ngx.resp - 响应操作

```lua
-- 获取/设置状态码
local status = ngx.resp.get_status()
ngx.resp.set_status(404)

-- 设置响应头
ngx.resp.set_header("X-Response-Time", "100ms")
```

### ngx.var - 变量操作

```lua
-- 获取/设置变量
local uri = ngx.var.uri
ngx.var.custom_var = "value"
```

### ngx.ctx - 请求上下文

```lua
-- 在阶段间传递数据
ngx.ctx.user_id = "123"
ngx.ctx.auth_time = ngx.now()
```

### ngx.say/print/flush - 输出

```lua
-- 输出内容到响应体
ngx.say("Hello from Lua!")
ngx.print("No newline")
ngx.flush()  -- 刷新缓冲
```

### ngx.exit - 终止请求

```lua
-- 终止请求处理，不再执行后续处理器
ngx.exit(200)  -- 成功
ngx.exit(403)  -- 禁止访问
ngx.exit(ngx.HTTP_NOT_FOUND)  -- 404
```

### ngx.redirect - 重定向

```lua
-- HTTP 重定向
ngx.redirect("/new-location", 301)
ngx.redirect("https://example.com", 302)
```

## 配置文件格式

在 YAML 配置文件中添加 Lua 中间件配置：

```yaml
server:
  lua:
    enabled: true
    global_settings:
      max_concurrent_coroutines: 1000
      coroutine_timeout: 30s
      code_cache_size: 1000
      enable_file_watch: true
      max_execution_time: 30s
    scripts:
      - path: "/scripts/auth.lua"
        phase: "access"
        timeout: 10s
        enabled: true
      - path: "/scripts/transform.lua"
        phase: "content"
        timeout: 30s
        enabled: true
```

## 错误处理

### 脚本执行错误

当 Lua 脚本执行出错时，中间件会返回 500 错误：

```lua
-- 这会导致 500 错误
error("something went wrong")
```

### ngx.exit 终止

`ngx.exit()` 通过抛出特殊错误终止执行，这是正常行为：

```lua
ngx.say("Processing...")
ngx.exit(200)  -- 正常终止，返回 200
-- 此后的代码不会执行
ngx.say("Never reached")
```

### 启用/禁用控制

```go
// 动态启用/禁用
middleware.SetEnabled(false)  // 禁用中间件
middleware.SetEnabled(true)   // 启用中间件

// 检查状态
if middleware.IsEnabled() {
    // 中间件已启用
}
```

## 性能考虑

### 单请求开销

基准测试显示单请求 Lua 开销约 **0.1ms**，远低于 1ms 阈值：

```
BenchmarkLuaMiddlewareOverhead-8    10000    99.885µs
```

### 最佳实践

1. **字节码缓存**：脚本编译后缓存，避免重复编译
2. **协程复用**：请求级协程从引擎池获取
3. **避免阻塞**：使用 `ngx.sleep()` 时注意超时
4. **限制脚本大小**：大脚本增加编译时间

## 示例脚本

### 访问控制（access phase）

```lua
-- auth.lua
local token = ngx.req.get_headers()["Authorization"]
if not token then
    ngx.exit(401)
    return
end

-- 验证 token
if token ~= "valid-token" then
    ngx.exit(403)
    return
end

-- 记录认证信息
ngx.ctx.user = "authenticated"
```

### 响应头注入（header_filter phase）

```lua
-- headers.lua
ngx.resp.set_header("X-Server", "lolly")
ngx.resp.set_header("X-Request-Id", ngx.var.request_id)
```

### 日志记录（log phase）

```lua
-- log.lua
local log_data = {
    uri = ngx.var.uri,
    method = ngx.req.get_method(),
    status = ngx.resp.get_status(),
    duration = ngx.now() - ngx.ctx.start_time
}

-- 写入日志文件或发送到日志服务
ngx.log(ngx.INFO, "request completed: " .. ngx.json.encode(log_data))
```

## 安全限制

默认配置下，以下 Lua 库被禁用：

- **os** - 操作系统访问
- **io** - 文件 I/O
- **load/loadfile** - 动态代码加载

可通过配置启用（谨慎使用）：

```yaml
lua:
  global_settings:
    enable_os_lib: false   # 安全
    enable_io_lib: false   # 安全
    enable_load_lib: false # 安全
```

沙箱限制：

- 协程创建被拦截（防止无限协程）
- 全局表只读（防止污染全局环境）
- 危险函数移除（debug, coroutine.create 等）