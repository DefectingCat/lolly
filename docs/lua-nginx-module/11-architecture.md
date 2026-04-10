# lua-nginx-module 核心架构设计

本文档为 lolly 项目提供 lua-nginx-module 的核心架构参考。

---

## 一、核心数据结构

### 1.1 主配置结构 (ngx_http_lua_main_conf_t)

**文件**: `src/ngx_http_lua_common.h:233`

```c
struct ngx_http_lua_main_conf_s {
    lua_State                 *lua;           // Lua VM 实例
    ngx_hash_t                 shm_zones;     // 共享内存区域
    ngx_array_t               *shdict_zones;  // shdict 区域

    // Phase handlers
    ngx_http_handler_pt        init_handler;
    ngx_http_handler_pt        init_worker_handler;
    ngx_http_handler_pt        exit_worker_handler;

    // 线程缓存
    ngx_queue_t                free_lua_threads;
    ngx_queue_t                cached_lua_threads;

    // 功能开关
    unsigned                   requires_rewrite:1;
    unsigned                   requires_access:1;
    unsigned                   requires_log:1;
    // ...
};
```

### 1.2 请求上下文 (ngx_http_lua_ctx_t)

**文件**: `src/ngx_http_lua_common.h:628`

```c
struct ngx_http_lua_ctx_s {
    ngx_http_request_t        *request;       // 请求对象

    // 协程管理
    ngx_http_lua_co_ctx_t     *cur_co_ctx;    // 当前协程
    ngx_http_lua_co_ctx_t      entry_co_ctx;  // 入口协程
    ngx_queue_t                user_co_ctx;   // 用户协程列表

    // 输出缓冲
    ngx_chain_t               *out;
    ngx_chain_t               *free_bufs;
    ngx_chain_t               *busy_bufs;

    // 请求体
    ngx_chain_t               *body;
    ngx_chain_t               *filter_in_bufs;

    // 状态
    unsigned                   context;        // 当前 phase
    unsigned                   exited:1;
    unsigned                   eof:1;
    // ...
};
```

### 1.3 协程上下文 (ngx_http_lua_co_ctx_t)

**文件**: `src/ngx_http_lua_common.h:550`

```c
struct ngx_http_lua_co_ctx_s {
    lua_State                 *co;            // Lua 协程
    ngx_http_lua_co_status_e   co_status;     // 状态

    // 父子关系
    ngx_http_lua_co_ctx_t     *parent_co_ctx;
    ngx_queue_t                zombie_child_threads;

    // 子请求数据
    ngx_int_t                 *sr_statuses;
    ngx_http_headers_out_t   **sr_headers;
    ngx_str_t                 *sr_bodies;

    // 挂起状态
    unsigned                   sleep:1;
    unsigned                   sem_wait:1;
    // ...
};
```

---

## 二、Nginx 集成机制

### 2.1 Directive 注册

**文件**: `src/ngx_http_lua_module.c:114-804`

```c
static ngx_command_t ngx_http_lua_cmds[] = {
    { ngx_string("content_by_lua_block"),
      NGX_HTTP_LOC_CONF|NGX_CONF_BLOCK,
      ngx_http_lua_content_by_lua_block,
      NGX_HTTP_LOC_CONF_OFFSET,
      offsetof(ngx_http_lua_loc_conf_t, content_handler),
      NULL },
    // ... 更多指令
};
```

### 2.2 Handler 注册

**文件**: `src/ngx_http_lua_module.c:839-945`

```c
static ngx_int_t ngx_http_lua_init(ngx_conf_t *cf)
{
    // 动态注册 phase handlers
    if (lmcf->requires_rewrite) {
        h = ngx_array_push(&cmcf->phases[NGX_HTTP_REWRITE_PHASE].handlers);
        *h = ngx_http_lua_rewrite_handler;
    }

    // Filter chain 注册
    ngx_http_lua_header_filter_init();
    ngx_http_lua_body_filter_init();
}
```

### 2.3 Filter Chain

```c
// Header filter
ngx_http_next_header_filter = ngx_http_top_header_filter;
ngx_http_top_header_filter = ngx_http_lua_header_filter;

// Body filter
ngx_http_next_body_filter = ngx_http_top_body_filter;
ngx_http_top_body_filter = ngx_http_lua_body_filter;
```

---

## 三、请求处理流程

```
nginx 请求
    ↓
ngx_http_lua_content_handler
    ↓
ngx_http_lua_content_run
    ↓
ngx_http_lua_run_thread  ←───┐
    ↓                        │
lua_pcall / lua_resume      │
    ↓                        │
用户 Lua 代码                │
    │                        │
    ├── lua_yield ───────────┘ (NGX_AGAIN)
    │       ↓
    │   事件注册
    │       ↓
    │   resume_handler
    │       ↓
    │   恢复执行
    │
    └── return
            ↓
        NGX_OK / NGX_ERROR
```

---

## 四、协程驱动架构

### 4.1 Yield/Resume 机制

```c
// Yield
lua_yield(L, nresults);    // 挂起 Lua 协程
ctx->resume_handler = ...; // 设置恢复函数
return NGX_AGAIN;          // 让出 nginx 控制权

// Resume
resume_handler(r);         // 事件触发时调用
lua_resume(L, nargs);      // 恢复 Lua 协程
```

### 4.2 非阻塞 I/O 模式

```
ngx.sleep(1)
    │
    ├── 添加 nginx 定时器
    ├── lua_yield
    └── return NGX_AGAIN
            ↓
    nginx 处理其他请求
            ↓
    定时器到期
            ↓
    ngx_http_lua_sleep_handler
            ↓
    lua_resume
            ↓
    用户代码继续
```

---

## 五、内存管理策略

### 5.1 双重内存管理

| 场景 | 使用 |
|------|------|
| 请求临时数据 | `ngx_palloc(r->pool)` / `ngx_pfree()` |
| 长期对象 | `lua_newuserdata()` (Lua GC) |

### 5.2 请求对象获取

```c
#ifdef OPENRESTY_LUAJIT
    // 使用 exdata (更快)
    r = lua_getexdata(L);
#else
    // 全局变量
    lua_getglobal(L, "__ngx_req");
    r = lua_touserdata(L, -1);
#endif
```

---

## 六、设计模式总结

| 模式 | 说明 |
|------|------|
| **Phase-based** | Nginx 11 个 phase，每个可注册 Lua handler |
| **协程驱动** | 非阻塞 I/O 通过 yield/resume 与 nginx 事件循环协作 |
| **VM 共享** | per-worker 单 VM，协程实现请求隔离 |
| **Closure Factory** | 非 LuaJIT 环境隔离 upvalue |
| **Filter Chain** | 插入式 header/body 过滤 |

---

## 七、关键设计决策

### 7.1 为什么用协程？

- **同步代码风格**: 开发者写同步代码，框架处理异步
- **非阻塞**: yield 时 nginx 可以处理其他请求
- **简单错误处理**: 使用标准 Lua 错误机制

### 7.2 为什么单 VM？

- **内存效率**: 代码只加载一次
- **共享状态**: 全局变量和缓存自然共享
- **性能**: 无 VM 切换开销

### 7.3 为什么用 FFI？

- **性能**: 绕过 Lua C API 开销
- **简洁**: 直接调用 C 函数
- **灵活**: 可动态加载

---

## 八、lolly 实现参考

### 8.1 核心模块实现顺序

1. **VM 管理**: Lua 状态机创建、初始化、销毁
2. **Context 管理**: 请求上下文、协程上下文
3. **Phase Handlers**: 各阶段的 handler 注册和执行
4. **Yield/Resume**: 协程挂起和恢复机制
5. **API 注入**: ngx.* 命名空间构建

### 8.2 需要实现的 C API

```c
// VM 管理
lua_State *create_vm();
void destroy_vm(lua_State *L);

// 协程管理
int run_thread(lua_State *L, lua_State *co);
int yield_thread(lua_State *co, int nresults);
int resume_thread(lua_State *co, int nargs);

// 请求对象
ngx_http_request_t *get_request(lua_State *L);
void set_request(lua_State *L, ngx_http_request_t *r);

// Phase 执行
int run_phase_handler(ngx_http_request_t *r, int phase);
```

### 8.3 数据结构映射

| lua-nginx-module | lolly 参考 |
|------------------|-----------|
| `ngx_http_lua_main_conf_t` | 全局配置，VM 管理 |
| `ngx_http_lua_loc_conf_t` | Location 配置，handler 存储 |
| `ngx_http_lua_ctx_t` | 请求级上下文 |
| `ngx_http_lua_co_ctx_t` | 协程状态管理 |