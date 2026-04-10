# lua-nginx-module 代码缓存与异常处理

本文档详细说明 lua-nginx-module 的代码缓存和异常处理机制。

---

## 一、代码缓存机制

### 1.1 核心文件

| 文件 | 说明 |
|------|------|
| `src/ngx_http_lua_cache.c` | 代码缓存实现 |
| `src/ngx_http_lua_clfactory.c` | Closure factory 实现 |
| `src/ngx_http_lua_directive.c` | `lua_code_cache` 指令处理 |

### 1.2 lua_code_cache 指令

```nginx
lua_code_cache on;   # 默认，代码缓存
lua_code_cache off;  # 开发模式，每次重载
```

**影响**:
- **on**: 每个 worker 共享一个 Lua VM，代码加载一次并缓存
- **off**: 每个请求创建独立 Lua VM，代码每次重载

### 1.3 缓存键生成

**Inline Script**:
- 格式: `nhli_{MD5(src)}`
- 宏: `NGX_HTTP_LUA_INLINE_TAG`

```c
u_char *ngx_http_lua_gen_chunk_cache_key(ngx_conf_t *cf,
    const char *tag, const u_char *src, size_t src_len)
{
    // key = tag + "_" + "nhli_" + MD5(src)
}
```

**File Script**:
- 格式: `nhlf_{MD5(file_path)}`
- 宏: `NGX_HTTP_LUA_FILE_TAG`

### 1.4 Closure Factory 模式

**非 LuaJIT 环境**:

```c
#define CLFACTORY_BEGIN_CODE "return function() "
#define CLFACTORY_END_CODE   "\nend"
```

用户代码被包装为:
```lua
return function()
    <user_code>
end
```

- **缓存**: 工厂函数被缓存
- **执行**: 每次调用工厂创建新 closure，隔离 upvalue

**LuaJIT 环境**:
- 直接缓存编译后的函数
- 不需要工厂包装

### 1.5 缓存查找/存储流程

**查找**:

```c
static ngx_int_t ngx_http_lua_cache_load_code(...)
{
    // 1. 获取 Registry 中的缓存表
    lua_pushlightuserdata(L, code_cache_key);
    lua_rawget(L, LUA_REGISTRYINDEX);

    // 2. 查找缓存
    lua_getfield(L, -1, key);

    if (lua_isfunction(L, -1)) {
        // 命中
        #ifdef OPENRESTY_LUAJIT
            return NGX_OK;
        #else
            // 调用工厂生成新 closure
            lua_pcall(L, 0, 1, 0);
        #endif
    }

    return NGX_DECLINED;  // 未命中
}
```

**存储**:

```c
static ngx_int_t ngx_http_lua_cache_store_code(...)
{
    // 1. 获取缓存表
    lua_pushlightuserdata(L, code_cache_key);
    lua_rawget(L, LUA_REGISTRYINDEX);

    // 2. 存储函数
    lua_pushvalue(L, -2);  // 复制 closure
    lua_setfield(L, -2, key);  // 存入缓存表
}
```

---

## 二、异常处理机制

### 2.1 核心文件

| 文件 | 说明 |
|------|------|
| `src/ngx_http_lua_exception.c` | 异常处理实现 |
| `src/ngx_http_lua_exception.h` | 宏定义 |

### 2.2 setjmp/longjmp 机制

```c
#define NGX_LUA_EXCEPTION_TRY   if (setjmp(ngx_http_lua_exception) == 0)
#define NGX_LUA_EXCEPTION_CATCH else
#define NGX_LUA_EXCEPTION_THROW(x) longjmp(ngx_http_lua_exception, (x))
```

### 2.3 Panic Handler

```c
static int ngx_http_lua_atpanic(lua_State *L)
{
    // 1. 输出错误日志
    // 2. 通过 longjmp 恢复 nginx 执行
    NGX_LUA_EXCEPTION_THROW(1);
}
```

### 2.4 使用模式

```c
ngx_http_lua_exception_init();

NGX_LUA_EXCEPTION_TRY {
    // 执行 Lua 代码
    rc = lua_pcall(L, 0, 1, 0);

} NGX_LUA_EXCEPTION_CATCH {
    // 捕获异常
    ngx_log_error(NGX_LOG_ERR, log, 0, "Lua VM panic");
}
```

---

## 三、VM 管理

### 3.1 VM 初始化

```c
ngx_int_t ngx_http_lua_init_vm(lua_State **L, ...)
{
    // 1. 创建新 Lua 状态机
    *L = luaL_newstate();

    // 2. 加载标准库
    luaL_openlibs(*L);

    // 3. 注入 ngx.* API
    ngx_http_lua_inject_ngx_api(*L, lmcf, log);

    // 4. 设置 panic handler
    lua_atpanic(*L, ngx_http_lua_atpanic);
}
```

### 3.2 VM 状态缓存

`lua_code_cache off` 模式下的 VM 状态缓存：

```c
typedef struct {
    lua_State    *lua;        // Lua VM
    ngx_pool_t   *pool;
    // ...
} ngx_http_lua_vm_state_t;
```

---

## 四、最佳实践

### 4.1 开发环境

```nginx
lua_code_cache off;  # 代码实时生效

location /reload {
    content_by_lua_block {
        -- 无需重新加载配置
    }
}
```

### 4.2 生产环境

```nginx
lua_code_cache on;

# 使用信号重载代码
# nginx -s reload
```

### 4.3 错误处理

```lua
local ok, err = pcall(function()
    -- 可能出错的代码
end)

if not ok then
    ngx.log(ngx.ERR, "Error: ", err)
    ngx.exit(ngx.HTTP_INTERNAL_SERVER_ERROR)
end
```