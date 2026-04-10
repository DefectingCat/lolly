# Golang Lua 运行时嵌入分析

本文档分析 Go 语言嵌入 Lua 运行时的方案，为 lolly 项目实现类似 lua-nginx-module 功能提供技术参考。

---

## 一、主流 Lua 运行时方案对比

### 1.1 可选方案

| 方案 | 语言 | 性能 | Lua版本 | 特点 |
|------|------|------|---------|------|
| **gopher-lua** | 纯 Go | ~Python3 | Lua 5.1 + goto | 原生 Go 实现，goroutine/channel 集成 |
| **go-lua** | Go + C | ~原生Lua | Lua 5.1 | CGO 调用 C Lua，性能接近原生 |
| **luaJIT (CGO)** | C | 极高 | LuaJIT 2.1 | 最快，FFI 强大，但 CGO 开销 |
| **glua** (Shopify) | Go + C | 高 | Lua 5.2/5.3 | Shopify 废弃，不推荐 |

### 1.2 详细对比

#### gopher-lua

**优势**:
- **纯 Go 实现**: 无 CGO 依赖，交叉编译友好
- **原生并发**: `LChannel` 类型直接操作 Go channel
- **Context 支持**: `SetContext(ctx)` 实现超时取消
- **字节码复用**: `FunctionProto` 跨 LState 共享
- **安全沙箱**: `SkipOpenLibs` 精细控制标准库加载

**劣势**:
- 性能约 Python3 级别，低于原生 Lua/LuaJIT
- 不支持 Lua 5.2+ 特性（bit32, utf8 等）
- GC 压力较大（大量 LValue 对象）

**适用场景**:
- 需要纯 Go、交叉编译
- 性能要求中等
- 需要与 Go goroutine/channel 深度集成

#### go-lua (CGO binding)

**优势**:
- 性能接近原生 Lua（通过 CGO 直接调用 C API）
- 支持 Lua 5.1 标准库完整功能

**劣势**:
- CGO 依赖，交叉编译复杂
- Go-C 边界开销（每次调用 ~50ns）
- 协程与 goroutine 交互困难（C 栈问题）

**适用场景**:
- 性能关键场景
- 已有 C Lua 生态依赖
- 可接受 CGO 复杂度

#### LuaJIT via CGO

**优势**:
- **极致性能**: JIT 编译，接近 C 速度
- **FFI 强大**: 直接调用 C 函数无开销
- **内存高效**: 更小的内存占用

**劣势**:
- LuaJIT 2.1 开发停滞
- CGO 集成复杂
- JIT 在某些环境受限（容器、安全限制）
- Go-LuaJIT 协程映射困难

**适用场景**:
- 性能极致要求
- 已有 OpenResty/LuaJIT 生态
- 运行环境可控

### 1.3 推荐选择

**对于 lolly 项目，推荐 gopher-lua**:

1. **纯 Go**: 与项目技术栈一致，交叉编译无障碍
2. **并发集成**: 天然支持 goroutine/channel，契合 Go HTTP 服务器架构
3. **性能足够**: Python3 级性能对脚本处理场景已够用
4. **成熟稳定**: yuin/gopher-lua 维护活跃，社区成熟

---

## 二、gopher-lua 核心 API

### 2.1 LState 状态机

```go
import "github.com/yuin/gopher-lua"

// 创建 VM
L := lua.NewState(lua.Options{
    SkipOpenLibs:        true,  // 安全：禁用默认库
    IncludeGoStackTrace: true,  // Panic 时输出 Go 调用栈
})
defer L.Close()

// 执行脚本
L.DoString("print('hello')")
L.DoFile("script.lua")

// Context 控制（超时）
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
L.SetContext(ctx)
L.DoString("while true do end")  // 5秒后自动取消
```

### 2.2 栈操作

```go
// 基本栈操作
L.Push(lua.LNumber(42))          // 压入数字
L.Push(lua.LString("hello"))     // 压入字符串
v := L.Get(-1)                   // 获取栈顶
L.Pop(2)                         // 弹出 2 个

// 全局变量
L.SetGlobal("myvar", lua.LNumber(100))
val := L.GetGlobal("myvar")

// Table 操作
tbl := L.NewTable()
L.SetField(tbl, "name", lua.LString("lolly"))
L.SetField(tbl, "version", lua.LString("0.2.0"))
L.SetGlobal("config", tbl)
```

### 2.3 函数注册

```go
// Go 函数签名: func(L *lua.LState) int (返回压栈结果数)
func Double(L *lua.LState) int {
    n := L.CheckInt(1)           // 获取第1个参数
    L.Push(lua.LNumber(n * 2))   // 压入结果
    return 1                      // 返回结果数
}

// 注册到全局
L.SetGlobal("double", L.NewFunction(Double))

// 批量注册到模块
mod := L.NewTable()
L.SetFuncs(mod, map[string]lua.LGFunction{
    "double": Double,
    "add":    Add,
    "sub":    Sub,
})
L.SetGlobal("mathx", mod)

// 闭包（带 upvalue）
counter := 0
L.SetGlobal("counter", L.NewClosure(func(L *lua.LState) int {
    counter++
    L.Push(lua.LNumber(counter))
    return 1
}))
```

### 2.4 调用 Lua 函数

```go
// 受保护调用（推荐）
err := L.CallByParam(lua.P{
    Fn:      L.GetGlobal("myFunc"),  // 函数
    NRet:    1,                       // 期望返回值
    Protect: true,                    // 拦截 panic
}, lua.LNumber(10), lua.LString("arg"))

if err != nil {
    // 错误处理
}
ret := L.Get(-1)  // 获取返回值
L.Pop(1)          // 清理栈
```

### 2.5 模块加载

```go
// 预加载自定义模块
L.PreloadModule("lolly", func(L *lua.LState) int {
    mod := L.NewTable()
    L.SetFuncs(mod, map[string]lua.LGFunction{
        "say":   SayHello,
        "log":   LogMessage,
        "sleep": Sleep,
    })
    L.Push(mod)
    return 1  // 返回模块表
})

// Lua 中使用
L.DoString(`
local lolly = require("lolly")
lolly.say("hello")
`)
```

---

## 三、协程支持

### 3.1 协程 API

```go
// 创建协程线程
co, cancel := L.NewThread()  // 共享全局状态

// 获取 Lua 协程函数
fn := L.GetGlobal("coroutine_func").(*lua.LFunction)

// Resume（恢复执行）
state, err, values := L.Resume(co, fn, lua.LNumber(10))
// state: lua.ResumeOK / lua.ResumeYield / lua.ResumeError
// values: yield 返回的值列表

// 检查状态
status := L.Status(co)  // "suspended" / "running" / "normal" / "dead"
```

### 3.2 Yield 模式

Lua 侧:
```lua
function async_task()
    print("start")
    coroutine.yield("waiting")  -- 挂起，返回值
    print("continue")
    return "done"
end
```

Go 侧:
```go
co, _ := L.NewThread()
fn := L.GetGlobal("async_task").(*lua.LFunction)

// 第一次 resume
st, err, vals := L.Resume(co, fn)
if st == lua.ResumeYield {
    fmt.Println("yielded:", vals[0])  // "waiting"
}

// 第二次 resume（恢复）
st, err, vals = L.Resume(co)
if st == lua.ResumeOK {
    fmt.Println("done:", vals[0])  // "done"
}
```

### 3.3 与 Go Channel 集成

gopher-lua 提供 `LChannel` 类型，让 Lua 操作 Go channel:

```go
// 创建 Go channel
ch := make(chan string, 10)

// 传递给 Lua
luaCh := lua.LChannel{Channel: ch}
L.SetGlobal("mychannel", luaCh)

// Lua 中操作
L.DoString(`
-- 发送
mychannel:send("hello")

-- 接收（阻塞）
local msg = mychannel:receive()
print(msg)
`)
```

---

## 四、错误处理

### 4.1 ApiError 结构

```go
type ApiError struct {
    Type       ApiErrorType  // Run/Syntax/Panic/Memory/File
    Object     LValue        // Lua 错误对象
    StackTrace string        // 调用栈
    Cause      error         // 底层错误
}
```

### 4.2 Panic Handler

```go
// 注册 panic handler（类似 lua-nginx-module）
L.SetPanic(func(L *lua.LState) {
    // 捕获 panic，记录日志
    log.Error("Lua panic: ", L.Get(-1))
    // 可以选择重建 VM 或返回错误
})

// 使用 PCall 保护调用
err := L.PCall(0, 0, nil)
if err != nil {
    apiErr := err.(*lua.ApiError)
    log.Error("Lua error: ", apiErr.StackTrace)
}
```

---

## 五、字节码缓存与复用

### 5.1 编译与复用

```go
// 编译脚本为字节码（可跨 VM 复用）
proto, err := lua.CompileString("function foo() return 42 end", "foo.lua")

// 多个 LState 共享字节码
L1 := lua.NewState()
fn1 := L1.NewFunctionFromProto(proto)

L2 := lua.NewState()
fn2 := L2.NewFunctionFromProto(proto)  // 无需重新编译
```

### 5.2 缓存设计（类似 lua-nginx-module）

```go
type CodeCache struct {
    mu     sync.RWMutex
    protos map[string]*lua.FunctionProto  // MD5(key) -> proto
}

func (c *CodeCache) GetOrCompile(src string) (*lua.FunctionProto, error) {
    key := md5Key(src)

    c.mu.RLock()
    proto, ok := c.protos[key]
    c.mu.RUnlock()

    if ok {
        return proto, nil
    }

    // 编译并缓存
    proto, err := lua.CompileString(src, key)
    if err != nil {
        return nil, err
    }

    c.mu.Lock()
    c.protos[key] = proto
    c.mu.Unlock()

    return proto, nil
}
```

---

## 六、与 lolly 项目集成设计

### 6.1 架构映射

| lua-nginx-module | lolly (Go) 实现 |
|------------------|----------------|
| `ngx_http_lua_main_conf_t` | `LuaWorker` 结构，持有 VM |
| `ngx_http_lua_ctx_t` | `LuaRequestCtx`，请求上下文 |
| `ngx_http_lua_co_ctx_t` | `LuaCoroutine`，协程状态 |
| Phase Handlers | Middleware 集成点 |
| Filter Chain | Response Filter 中间件 |

### 6.2 核心结构设计

```go
// internal/lua/engine.go

package lua

import (
    "context"
    "sync"

    glua "github.com/yuin/gopher-lua"
)

// LuaWorker - worker 级单 VM（对应 ngx_http_lua_main_conf_t）
type LuaWorker struct {
    L          *glua.LState       // 主 VM
    codeCache  *CodeCache         // 字节码缓存
    coroPool   *CoroutinePool     // 协程池
    modules    map[string]glua.LGFunction // 已加载模块
    mu         sync.RWMutex
}

// LuaRequestCtx - 请求上下文（对应 ngx_http_lua_ctx_t）
type LuaRequestCtx struct {
    Worker     *LuaWorker
    Request    *fasthttp.RequestCtx  // fasthttp 请求
    Coroutine  *LuaCoroutine         // 当前协程
    Variables  map[string]string     // ngx.var
    Output     []byte                // ngx.say 输出缓冲
    Phase      Phase                  // 当前阶段
    Ctx        context.Context       // Go context
}

// LuaCoroutine - 协程状态（对应 ngx_http_lua_co_ctx_t）
type LuaCoroutine struct {
    Thread     *glua.LState       // 协程线程
    Status     CoroutineStatus    // running/suspended/dead
    Parent     *LuaCoroutine      // 父协程
    ResumeFunc func()             // 恢复回调（类似 resume_handler）
}

// Phase - 处理阶段
type Phase int

const (
    PhaseInit Phase = iota
    PhaseAccess
    PhaseContent
    PhaseLog
    PhaseHeaderFilter
    PhaseBodyFilter
)
```

### 6.3 Worker 级单 VM 初始化

```go
// internal/lua/worker.go

func NewLuaWorker() *LuaWorker {
    // 创建 VM（安全模式）
    L := glua.NewState(glua.Options{
        SkipOpenLibs: true,
    })

    worker := &LuaWorker{
        L:         L,
        codeCache: NewCodeCache(),
        coroPool:  NewCoroutinePool(L),
        modules:   make(map[string]glua.LGFunction),
    }

    // 加载必要标准库
    worker.loadSafeLibs()

    // 注册 lolly.* API
    worker.registerLollyAPI()

    return worker
}

func (w *LuaWorker) loadSafeLibs() {
    // 只加载安全的库（禁用 os, io 等危险库）
    w.L.CallByParam(glua.P{
        Fn:      w.L.NewFunction(glua.OpenBase),
        Protect: true,
    })
    w.L.CallByParam(glua.P{
        Fn:      w.L.NewFunction(glua.OpenTable),
        Protect: true,
    })
    w.L.CallByParam(glua.P{
        Fn:      w.L.NewFunction(glua.OpenString),
        Protect: true,
    })
    w.L.CallByParam(glua.P{
        Fn:      w.L.NewFunction(glua.OpenMath),
        Protect: true,
    })
}
```

### 6.4 lolly.* API 注册

```go
// internal/lua/api.go

func (w *LuaWorker) registerLollyAPI() {
    // 创建 lolly 模块表
    lollyMod := w.L.NewTable()

    // 注册核心 API
    w.L.SetFuncs(lollyMod, map[string]glua.LGFunction{
        // 输出
        "say":   w.apiSay,
        "print": w.apiPrint,

        // 请求
        "req":     w.apiRequest,
        "get_uri": w.apiGetURI,
        "get_arg": w.apiGetArg,
        "get_header": w.apiGetHeader,

        // 响应
        "resp":        w.apiResponse,
        "set_header":  w.apiSetHeader,
        "set_status":  w.apiSetStatus,

        // 变量
        "var":     w.apiVar,
        "set_var": w.apiSetVar,

        // 控制流
        "exit":  w.apiExit,
        "sleep": w.apiSleep,  // 异步 sleep
        "throw": w.apiThrow,

        // 日志
        "log":  w.apiLog,
        "err":  w.apiLogErr,
        "warn": w.apiLogWarn,
        "info": w.apiLogInfo,
    })

    w.L.SetGlobal("lolly", lollyMod)

    // 兼容 nginx 命名（可选）
    w.L.SetGlobal("ngx", lollyMod)
}

// apiSay - 输出内容
func (w *LuaWorker) apiSay(L *glua.LState) int {
    ctx := getRequestCtx(L)  // 从 LState 获取请求上下文
    str := L.CheckString(1)
    ctx.Output = append(ctx.Output, str...)
    return 0
}

// apiGetURI - 获取请求 URI
func (w *LuaWorker) apiGetURI(L *glua.LState) int {
    ctx := getRequestCtx(L)
    uri := string(ctx.Request.URI().Path())
    L.Push(glua.LString(uri))
    return 1
}

// apiSleep - 异步睡眠（yield 实现）
func (w *LuaWorker) apiSleep(L *glua.LState) int {
    ctx := getRequestCtx(L)
    ms := L.CheckInt(1)

    // 创建定时器，yield 当前协程
    ctx.Coroutine.ResumeFunc = func() {
        // 定时器到期后 resume
        ctx.Worker.ResumeCoroutine(ctx.Coroutine)
    }

    // 注册定时器
    go func() {
        time.Sleep(time.Duration(ms) * time.Millisecond)
        ctx.Coroutine.ResumeFunc()
    }()

    // Yield
    L.Yield(glua.LNumber(ms))
    return 0
}
```

### 6.5 请求上下文绑定

```go
// internal/lua/context.go

// 请求开始时绑定上下文
func (w *LuaWorker) NewRequestCtx(req *fasthttp.RequestCtx) *LuaRequestCtx {
    ctx := &LuaRequestCtx{
        Worker:    w,
        Request:   req,
        Variables: make(map[string]string),
        Phase:     PhaseAccess,
        Ctx:       req,
    }

    // 创建请求协程
    ctx.Coroutine = w.coroPool.Acquire()
    ctx.Coroutine.Parent = nil

    // 绑定到 LState（使用 exdata 模式）
    // gopher-lua 不支持 exdata，使用全局变量
    ctx.Coroutine.Thread.SetGlobal("__lolly_req", glua.LUserData{
        Value: ctx,
        Metatable: w.L.NewTable(),  // 可设置 __index 方法
    })

    return ctx
}

// 从 LState 获取请求上下文
func getRequestCtx(L *glua.LState) *LuaRequestCtx {
    ud := L.GetGlobal("__lolly_req")
    if ud == glua.LNil {
        return nil
    }
    return ud.(*glua.LUserData).Value.(*LuaRequestCtx)
}
```

### 6.6 Yield/Resume 与 Go 异步集成

**关键设计**: Lua yield → Go channel → 恢复执行

```go
// internal/lua/coroutine.go

type CoroutinePool struct {
    L      *glua.LState
    free   chan *LuaCoroutine
    resume chan *LuaCoroutine  // 恢复队列
}

func (p *CoroutinePool) Acquire() *LuaCoroutine {
    select {
    case co := <-p.free:
        return co
    default:
        // 创建新协程
        thread, _ := p.L.NewThread()
        return &LuaCoroutine{
            Thread: thread,
            Status: StatusSuspended,
        }
    }
}

// 执行脚本（支持 yield）
func (ctx *LuaRequestCtx) RunScript(script string) error {
    // 获取或编译字节码
    proto, err := ctx.Worker.codeCache.GetOrCompile(script)
    if err != nil {
        return err
    }

    fn := ctx.Coroutine.Thread.NewFunctionFromProto(proto)

    // 开始执行
    state, err, _ := ctx.Worker.L.Resume(ctx.Coroutine.Thread, fn)

    for state == glua.ResumeYield {
        // 协程 yield，等待恢复信号
        ctx.Coroutine.Status = StatusSuspended

        // 等待 ResumeFunc 触发
        select {
        case <-ctx.Worker.coroPool.resume:
            // 恢复执行
            state, err, _ = ctx.Worker.L.Resume(ctx.Coroutine.Thread)
        case <-ctx.Ctx.Done():
            // 请求超时/取消
            return ctx.Ctx.Err()
        }
    }

    if state == glua.ResumeError {
        return err
    }

    return nil
}
```

### 6.7 中间件集成

```go
// internal/middleware/lua_middleware.go

package middleware

import (
    "rua.plus/lolly/internal/lua"
    "github.com/valyala/fasthttp"
)

type LuaMiddleware struct {
    worker *lua.LuaWorker
    config LuaConfig
}

func (m *LuaMiddleware) Process(next fasthttp.RequestHandler) fasthttp.RequestHandler {
    return func(ctx *fasthttp.RequestCtx) {
        // 创建 Lua 请求上下文
        luaCtx := m.worker.NewRequestCtx(ctx)

        // 执行 access_by_lua
        if m.config.AccessScript != "" {
            if err := luaCtx.RunScript(m.config.AccessScript); err != nil {
                ctx.Error("Lua error: "+err.Error(), 500)
                return
            }
        }

        // 执行 content_by_lua（如果有）
        if m.config.ContentScript != "" {
            luaCtx.Phase = lua.PhaseContent
            if err := luaCtx.RunScript(m.config.ContentScript); err != nil {
                ctx.Error("Lua error: "+err.Error(), 500)
                return
            }
            // 输出 Lua 内容
            ctx.Write(luaCtx.Output)
            return
        }

        // 继续下一个 handler
        next(ctx)

        // 执行 log_by_lua
        if m.config.LogScript != "" {
            luaCtx.Phase = lua.PhaseLog
            luaCtx.RunScript(m.config.LogScript)
        }
    }
}
```

---

## 七、实现路线图

### 7.1 阶段一：基础嵌入（Week 1-2）

| 任务 | 文件 | 说明 |
|------|------|------|
| 添加 gopher-lua 依赖 | `go.mod` | `github.com/yuin/gopher-lua` |
| 创建 LuaWorker | `internal/lua/worker.go` | 单 VM 管理 |
| 代码缓存 | `internal/lua/cache.go` | 字节码缓存 |
| 基础 API 注入 | `internal/lua/api.go` | say/print/get_uri 等 |

### 7.2 阶段二：请求集成（Week 3-4）

| 任务 | 文件 | 说明 |
|------|------|------|
| LuaRequestCtx | `internal/lua/context.go` | 请求上下文绑定 |
| LuaMiddleware | `internal/middleware/lua_middleware.go` | 中间件集成 |
| 配置支持 | `internal/config/lua.go` | Lua 指令解析 |

### 7.3 阶段三：异步支持（Week 5-6）

| 任务 | 文件 | 说明 |
|------|------|------|
| CoroutinePool | `internal/lua/coroutine.go` | 协程池 |
| Yield/Resume | `internal/lua/coroutine.go` | 异步 sleep 等 |
| LChannel 集成 | `internal/lua/channel.go` | Go channel 操作 |

### 7.4 阶段四：高级功能（Week 7-10）

| 任务 | 说明 |
|------|------|
| 共享内存 (shdict) | Go sync.Map + Lua Table API |
| Cosocket | 非阻塞 TCP socket |
| 子请求 | 内部 location capture |
| Timer | ngx.timer.at 实现 |
| Balancer | 动态负载均衡 |

---

## 八、性能考量

### 8.1 性能优化点

| 优化 | 方法 |
|------|------|
| **字节码缓存** | 预编译脚本，跨请求复用 |
| **协程池** | 预创建协程，避免频繁创建销毁 |
| **减少 GC** | 使用 LTable 而非大量小对象 |
| **并行执行** | 多 worker 独立 VM，无锁竞争 |

### 8.2 性能基准

```go
// 建议基准测试
func BenchmarkLuaSimple(b *testing.B) {
    L := lua.NewState()
    defer L.Close()
    L.DoString("function test() return 1 + 1 end")

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        L.CallByParam(lua.P{Fn: L.GetGlobal("test")})
    }
}

func BenchmarkLuaWithCtx(b *testing.B) {
    // 带请求上下文的基准
}
```

---

## 九、安全考虑

### 9.1 安全沙箱

```go
// 禁用危险库
L := lua.NewState(lua.Options{SkipOpenLibs: true})

// 只加载安全库
safeLibs := []glua.LGFunction{
    glua.OpenBase,   // 基础
    glua.OpenTable,  // 表操作
    glua.OpenString, // 字符串
    glua.OpenMath,   // 数学
    // 禁用: OpenOS, OpenIO, OpenPackage (部分)
}
```

### 9.2 资源限制

```go
// Context 超时
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
L.SetContext(ctx)

// 内存限制（通过 Options）
L := lua.NewState(lua.Options{
    RegistryMaxSize: 1024 * 1024,  // 限制注册表大小
})

// CPU 限制（通过 goroutine 监控）
go func() {
    time.Sleep(5 * time.Second)
    cancel()  // 强制取消
}()
```

---

## 十、参考资源

- [gopher-lua GitHub](https://github.com/yuin/gopher-lua) - 主仓库
- [gopher-lua API 文档](https://pkg.go.dev/github.com/yuin/gopher-lua) - GoDoc
- [Lua 5.1 手册](https://www.lua.org/manual/5.1/) - 语言参考
- [lua-nginx-module 文档](../lua-nginx-module/) - 架构参考（已生成）