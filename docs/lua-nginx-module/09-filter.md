# lua-nginx-module Filter 过滤器链

本文档详细说明 lua-nginx-module 的 header/body filter 功能。

---

## 一、核心文件

| 文件 | 说明 |
|------|------|
| `src/ngx_http_lua_headerfilterby.c` | Header filter 实现 |
| `src/ngx_http_lua_bodyfilterby.c` | Body filter 实现 |
| `src/ngx_http_lua_capturefilter.c` | Capture filter (子请求) |

---

## 二、Header Filter

### 2.1 配置

```nginx
header_filter_by_lua_block {
    ngx.header["X-Custom"] = "value"
    ngx.header["X-Request-ID"] = ngx.var.request_id
}
```

### 2.2 特点

- **同步执行**: 不能 yield
- **修改响应头**: 通过 `ngx.header` API
- **继续链**: 自动调用下一个 filter

### 2.3 内部实现

```c
// Filter chain 注册
ngx_http_next_header_filter = ngx_http_top_header_filter;
ngx_http_top_header_filter = ngx_http_lua_header_filter;
```

---

## 三、Body Filter

### 3.1 配置

```nginx
body_filter_by_lua_block {
    local chunk = ngx.arg[1]  -- 当前数据块
    local eof = ngx.arg[2]    -- EOF 标记

    -- 修改响应体
    if chunk then
        ngx.arg[1] = chunk:gsub("old", "new")
    end
}
```

### 3.2 ngx.arg API

| 索引 | 说明 |
|------|------|
| `ngx.arg[1]` | 当前数据块 (string/nil) |
| `ngx.arg[2]` | EOF 标记 (boolean) |

### 3.3 操作方式

```lua
-- 替换数据
ngx.arg[1] = "new content"

-- 丢弃当前块
ngx.arg[1] = nil

-- 设置 EOF
ngx.arg[2] = true

-- 清除 EOF
ngx.arg[2] = false
```

### 3.4 内部实现

**ngx.arg 元表**:

```c
lua_createtable(L, 0, 2);
lua_pushcfunction(L, ngx_http_lua_param_set);
lua_setfield(L, -2, "__newindex");
lua_setmetatable(L, -2);
```

**Buffer 管理**:

```c
// 输入 buffers
ctx->filter_in_bufs

// Busy buffers
ctx->filter_busy_bufs

// EOF 标记
ctx->seen_last_in_filter
```

---

## 四、Filter Chain 机制

### 4.1 架构图

```
全局变量: ngx_http_top_header_filter
         |
    [Lua Header Filter]
         | --ngx_http_next_header_filter-->
    [下一个 Filter] --> ...

全局变量: ngx_http_top_body_filter
         |
    [Lua Body Filter]
         | --ngx_http_next_body_filter-->
    [下一个 Filter] --> ...
```

### 4.2 注册流程

```c
// 在 ngx_http_lua_init() 中
ngx_http_lua_header_filter_init();
ngx_http_lua_body_filter_init();
ngx_http_lua_capture_filter_init();
```

---

## 五、使用示例

### 5.1 添加安全头

```nginx
header_filter_by_lua_block {
    ngx.header["X-Frame-Options"] = "DENY"
    ngx.header["X-Content-Type-Options"] = "nosniff"
    ngx.header["X-XSS-Protection"] = "1; mode=block"
}
```

### 5.2 响应体压缩

```nginx
body_filter_by_lua_block {
    local chunk = ngx.arg[1]
    if chunk then
        -- 简单的 gzip 压缩模拟
        ngx.arg[1] = ngx.encode_base64(chunk)
    end
}
```

### 5.3 内容替换

```nginx
body_filter_by_lua_block {
    local chunk = ngx.arg[1]
    if chunk then
        ngx.arg[1] = chunk:gsub("%{%{.(.-)%}%}", function(var)
            return ngx.var[var] or ""
        end)
    end
}
```

### 5.4 流量统计

```nginx
body_filter_by_lua_block {
    local chunk = ngx.arg[1]
    local eof = ngx.arg[2]

    if chunk then
        -- 累计响应大小
        ngx.ctx.response_size = (ngx.ctx.response_size or 0) + #chunk
    end

    if eof then
        -- 记录总大小
        ngx.log(ngx.INFO, "Response size: ", ngx.ctx.response_size)
    end
}
```

---

## 六、注意事项

1. **不能 yield**: filter 代码必须同步完成
2. **性能敏感**: 每个响应块都会执行，避免重操作
3. **chunk-by-chunk**: body filter 逐块处理，可能需要缓冲
4. **EOF 标记**: 最后一个块的 `ngx.arg[2]` 为 `true`