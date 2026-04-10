# lua-nginx-module Subrequest 内部请求

本文档详细说明 lua-nginx-module 的子请求功能。

---

## 一、核心文件

| 文件 | 说明 |
|------|------|
| `src/ngx_http_lua_subrequest.c` | 子请求实现 |
| `src/ngx_http_lua_capturefilter.c` | 响应捕获过滤器 |

---

## 二、API 概述

### `ngx.location.capture(uri, options?)`

发起单个子请求。

### `ngx.location.capture_multi(requests)`

并发发起多个子请求。

**关键发现**: `capture` 实际上是 `capture_multi` 的薄包装。

---

## 三、支持的 HTTP 方法

| 常量 | 方法 |
|------|------|
| `ngx.HTTP_GET` | GET |
| `ngx.HTTP_POST` | POST |
| `ngx.HTTP_PUT` | PUT |
| `ngx.HTTP_DELETE` | DELETE |
| `ngx.HTTP_HEAD` | HEAD |
| `ngx.HTTP_PATCH` | PATCH |
| `ngx.HTTP_OPTIONS` | OPTIONS |

---

## 四、选项参数

```lua
local res = ngx.location.capture(uri, {
    method = ngx.HTTP_POST,      -- HTTP 方法
    args = "foo=bar",            -- 参数字符串或 table
    body = '{"data":1}',         -- 请求体
    headers = {                  -- 请求头
        ["Content-Type"] = "application/json"
    },
    vars = {                     -- 变量
        upstream = "backend"
    },
    share_all_vars = false,      -- 共享所有变量
    copy_all_vars = false,       -- 复制所有变量
    always_forward_body = false, -- 始终转发请求体
    ctx = {}                     -- 传递上下文
})
```

---

## 五、响应结构

```lua
local res = ngx.location.capture("/api")
-- res = {
--     status = 200,           -- HTTP 状态码
--     header = {...},         -- 响应头 table
--     body = "...",           -- 响应体
--     truncated = false       -- 是否被截断
-- }
```

---

## 六、使用示例

### 6.1 基本用法

```lua
local res = ngx.location.capture("/internal/users")
if res.status == 200 then
    local users = cjson.decode(res.body)
    ngx.say("Users: ", #users)
end
```

### 6.2 POST 请求

```lua
local res = ngx.location.capture("/api/create", {
    method = ngx.HTTP_POST,
    body = '{"name":"john"}',
    headers = {
        ["Content-Type"] = "application/json"
    }
})
```

### 6.3 并发请求

```lua
local res1, res2, res3 = ngx.location.capture_multi({
    {"/api/users"},
    {"/api/products", {method = ngx.HTTP_GET}},
    {"/api/orders", {args = "status=pending"}}
})

ngx.say("Users: ", res1.status)
ngx.say("Products: ", res2.status)
ngx.say("Orders: ", res3.status)
```

---

## 七、内部实现

### 7.1 数据存储结构

```c
struct ngx_http_lua_co_ctx_s {
    ngx_int_t               *sr_statuses;  // 子请求状态码数组
    ngx_http_headers_out_t **sr_headers;   // 子请求响应头数组
    ngx_str_t               *sr_bodies;    // 子请求响应体数组
    uint8_t                 *sr_flags;     // 子请求标志位数组

    unsigned                 nsubreqs;     // 子请求总数
    unsigned                 pending_subreqs; // 待处理子请求数
};
```

### 7.2 响应捕获过滤器

```c
// 头部过滤器
static ngx_int_t ngx_http_lua_capture_header_filter(ngx_http_request_t *r)
{
    if (ctx && ctx->capture) {
        r->filter_need_in_memory = 1;  // 强制内存缓冲
        return NGX_OK;  // 拦截，不发送到客户端
    }
}

// 响应体过滤器
static ngx_int_t ngx_http_lua_capture_body_filter(ngx_http_request_t *r, ngx_chain_t *in)
{
    // 将响应体复制到 ctx->body 链表
}
```

### 7.3 响应组装

```c
static void ngx_http_lua_handle_subreq_responses(...)
{
    for (index = 0; index < coctx->nsubreqs; index++) {
        lua_createtable(co, 0, 4);  // 创建响应表

        // status
        lua_pushinteger(co, coctx->sr_statuses[index]);
        lua_setfield(co, -2, "status");

        // truncated
        if (coctx->sr_flags[index] & NGX_HTTP_LUA_SUBREQ_TRUNCATED) {
            lua_pushboolean(co, 1);
            lua_setfield(co, -2, "truncated");
        }

        // body
        lua_pushlstring(co, body_str->data, body_str->len);
        lua_setfield(co, -2, "body");

        // header
        // ...
    }
}
```

---

## 八、请求体传递规则

| 条件 | 行为 |
|------|------|
| 指定了 `body` 选项 | 使用自定义请求体 |
| GET/DELETE 等方法 | 不转发原请求体 |
| 其他情况 | 深拷贝原请求体 |

---

## 九、注意事项

1. **子请求必须是内部 location**（以 `/` 开头或使用 `internal` 指令）
2. **嵌套深度有限制**（Nginx 默认 50）
3. **不能跨 server 块**
4. **异步操作会挂起父请求**