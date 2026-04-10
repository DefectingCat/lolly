# lua-nginx-module ngx.* Lua API 参考

本文档详细列出 lua-nginx-module 暴露给 Lua 的所有 API。

## API 注入点

所有 API 通过 `ngx_http_lua_inject_ngx_api` 函数注入到 `ngx` 全局表。

**源文件**: `src/ngx_http_lua_util.c:835`

### 注入函数列表

| 注入函数 | 文件 | 说明 |
|---------|------|------|
| `ngx_http_lua_inject_arg_api` | ngx_http_lua_util.c | 命令行参数 |
| `ngx_http_lua_inject_http_consts` | ngx_http_lua_consts.c | HTTP 方法/状态码常量 |
| `ngx_http_lua_inject_core_consts` | ngx_http_lua_consts.c | 核心常量 |
| `ngx_http_lua_inject_log_api` | ngx_http_lua_log.c | 日志 API |
| `ngx_http_lua_inject_output_api` | ngx_http_lua_output.c | 输出控制 |
| `ngx_http_lua_inject_control_api` | ngx_http_lua_control.c | 控制流 |
| `ngx_http_lua_inject_subrequest_api` | ngx_http_lua_subrequest.c | 子请求 |
| `ngx_http_lua_inject_req_api` | ngx_http_lua_util.c | 请求 API |
| `ngx_http_lua_inject_resp_header_api` | ngx_http_lua_headers.c | 响应头 |
| `ngx_http_lua_inject_shdict_api` | ngx_http_lua_shdict.c | 共享字典 |
| `ngx_http_lua_inject_socket_tcp_api` | ngx_http_lua_socket_tcp.c | TCP socket |
| `ngx_http_lua_inject_socket_udp_api` | ngx_http_lua_socket_udp.c | UDP socket |
| `ngx_http_lua_inject_uthread_api` | ngx_http_lua_uthread.c | 用户线程 |
| `ngx_http_lua_inject_timer_api` | ngx_http_lua_timer.c | 定时器 |
| `ngx_http_lua_inject_coroutine_api` | ngx_http_lua_coroutine.c | 协程 |

---

## 一、ngx.req.* - 请求操作 API

### 1.1 URI 操作

#### `ngx.req.set_uri(uri, jump?, binary?)`

设置请求 URI。

- **参数**:
  - `uri` (string): 新的 URI
  - `jump` (boolean, 可选): 是否执行内部跳转
  - `binary` (boolean, 可选): 是否二进制安全
- **返回值**: 无或 yield
- **适用阶段**: rewrite, server_rewrite
- **示例**:
  ```lua
  ngx.req.set_uri("/new/path", true)  -- 内部跳转
  ngx.req.set_uri("/api/v2" .. ngx.var.request_uri)  -- 重写
  ```

### 1.2 参数操作

#### `ngx.req.get_uri_args(max_args?)`

获取 URL 查询参数。

- **参数**: `max_args` (number, 可选, 默认 100)
- **返回值**: table (参数键值对，多值参数为数组)
- **示例**:
  ```lua
  local args = ngx.req.get_uri_args()
  -- URL: /test?foo=bar&foo=baz&name=john
  -- args.foo = {"bar", "baz"}
  -- args.name = "john"
  ```

#### `ngx.req.get_post_args(max_args?)`

获取 POST 表单参数。

- **参数**: `max_args` (number, 可选, 默认 100)
- **返回值**: table
- **前提**: 需要 `lua_need_request_body on` 或先调用 `ngx.req.read_body()`

#### `ngx.req.set_uri_args(args)`

设置 URL 查询参数。

- **参数**: `args` (string/number/table)
- **示例**:
  ```lua
  ngx.req.set_uri_args({foo = "bar", page = 1})
  ngx.req.set_uri_args("foo=bar&page=1")
  ```

### 1.3 请求头操作

#### `ngx.req.get_headers(max_headers?, raw?)`

获取请求头。

- **参数**:
  - `max_headers` (number, 可选)
  - `raw` (boolean, 可选): 是否保留原始大小写
- **返回值**: table
- **示例**:
  ```lua
  local headers = ngx.req.get_headers()
  local content_type = headers["Content-Type"]
  local host = headers.host or headers.Host
  ```

#### `ngx.req.set_header(header_name, header_value)`

设置请求头。

- **参数**:
  - `header_name` (string)
  - `header_value` (string/table/nil)
- **示例**:
  ```lua
  ngx.req.set_header("X-Custom", "value")
  ngx.req.set_header("X-Multi", {"val1", "val2"})
  ngx.req.set_header("X-Remove", nil)  -- 删除
  ```

#### `ngx.req.clear_header(header_name)`

清除请求头。

### 1.4 请求体操作

#### `ngx.req.read_body()`

异步读取请求体。

- **返回值**: 无或 yield
- **适用阶段**: rewrite, access, content

#### `ngx.req.get_body_data(max?)`

获取请求体数据。

- **参数**: `max` (number, 可选)
- **返回值**: string 或 nil

#### `ngx.req.get_body_file()`

获取请求体临时文件路径。

- **返回值**: string 或 nil

#### `ngx.req.discard_body()`

丢弃请求体。

#### `ngx.req.set_body_data(data)`

设置请求体数据。

#### `ngx.req.set_body_file(file_path, auto_clean?)`

设置请求体文件。

### 1.5 其他请求信息

#### `ngx.req.http_version()`

获取 HTTP 版本。

- **返回值**: number (0.9, 1.0, 1.1, 2.0, 3.0) 或 nil

#### `ngx.req.raw_header(no_request_line?)`

获取原始请求头字符串。

- **参数**: `no_request_line` (boolean)
- **返回值**: string

#### `ngx.req.get_method()`

获取请求方法。

- **返回值**: string ("GET", "POST", etc.)

#### `ngx.req.set_method(method)`

设置请求方法。

#### `ngx.req.is_internal()`

判断是否内部请求。

- **返回值**: boolean

---

## 二、ngx.resp.* - 响应操作 API

#### `ngx.resp.get_headers(max_headers?, raw?)`

获取响应头。

- **参数**:
  - `max_headers` (number, 可选)
  - `raw` (boolean, 可选)
- **返回值**: table

---

## 三、输出控制 API

### `ngx.say(...)`

输出内容并附加换行符。

- **参数**: 可变参数 (string/number/boolean/table/nil)
- **返回值**: 1 (成功) 或 nil, err
- **适用阶段**: rewrite, access, content, precontent
- **示例**:
  ```lua
  ngx.say("Hello, ", "World!")
  ngx.say({name = "test", value = 123})  -- 输出 table
  ```

### `ngx.print(...)`

输出内容，不附加换行符。

### `ngx.flush(wait?)`

刷新输出缓冲区。

- **参数**: `wait` (boolean, 可选)
- **返回值**: 1, nil+err, 或 yield

### `ngx.eof()`

发送 EOF，结束响应。

### `ngx.send_headers()`

显式发送响应头。

### `ngx.exit(status)`

结束请求处理。

- **参数**: `status` (number) - HTTP 状态码或 ngx.ERROR 等
- **示例**:
  ```lua
  ngx.exit(ngx.HTTP_NOT_FOUND)  -- 404
  ngx.exit(ngx.HTTP_OK)  -- 正常结束
  ngx.exit(ngx.ERROR)  -- 错误
  ```

---

## 四、日志 API

### `ngx.log(level, ...)`

记录日志。

- **参数**:
  - `level`: 日志级别常量
  - `...`: 可变参数
- **示例**:
  ```lua
  ngx.log(ngx.ERR, "error message: ", err)
  ngx.log(ngx.INFO, "request from: ", ngx.var.remote_addr)
  ```

### 日志级别常量

| 常量 | 值 | 说明 |
|------|---|------|
| `ngx.STDERR` | 0 | 标准错误 |
| `ngx.EMERG` | 1 | 紧急 |
| `ngx.ALERT` | 2 | 警报 |
| `ngx.CRIT` | 3 | 严重 |
| `ngx.ERR` | 4 | 错误 |
| `ngx.WARN` | 5 | 警告 |
| `ngx.NOTICE` | 6 | 通知 |
| `ngx.INFO` | 7 | 信息 |
| `ngx.DEBUG` | 8 | 调试 |

### `print(...)`

全局函数，等效于 `ngx.log(ngx.NOTICE, ...)`

---

## 五、控制流 API

### `ngx.exec(uri, args?)`

内部重定向。

- **参数**:
  - `uri` (string): 目标 URI
  - `args` (string/table/nil): 参数
- **示例**:
  ```lua
  ngx.exec("/internal/api", {foo = "bar"})
  ```

### `ngx.redirect(uri, status?)`

HTTP 重定向。

- **参数**:
  - `uri` (string): 目标 URL
  - `status` (number): HTTP 状态码 (默认 302)
- **可选状态**: 301, 302, 303, 307, 308
- **示例**:
  ```lua
  ngx.redirect("https://example.com/new")
  ngx.redirect("/login", ngx.HTTP_MOVED_TEMPORARILY)
  ```

### `ngx.on_abort(callback)`

注册客户端断开连接回调。

---

## 六、子请求 API

### `ngx.location.capture(uri, options?)`

发起单个子请求。

- **参数**:
  - `uri` (string): 内部 URI
  - `options` (table, 可选):
    - `method`: HTTP 方法常量
    - `args`: 参数字符串/table
    - `body`: 请求体
    - `headers`: 请求头 table
    - `vars`: 变量 table
    - `share_all_vars`: boolean
    - `copy_all_vars`: boolean
    - `always_forward_body`: boolean
    - `ctx`: Lua table
- **返回值**: table {status, header, body, truncated}
- **适用阶段**: rewrite, access, content
- **示例**:
  ```lua
  local res = ngx.location.capture("/api/users", {
      method = ngx.HTTP_POST,
      body = '{"name":"john"}',
      headers = {["Content-Type"] = "application/json"}
  })
  ngx.say("Status: ", res.status)
  ngx.say("Body: ", res.body)
  ```

### `ngx.location.capture_multi(requests)`

并发发起多个子请求。

- **参数**: `requests` (array of {uri, options?})
- **返回值**: array of response tables
- **示例**:
  ```lua
  local res1, res2 = ngx.location.capture_multi({
      {"/api/users"},
      {"/api/products", {method = ngx.HTTP_GET}}
  })
  ```

---

## 七、变量访问 API

### `ngx.var.VARIABLE_NAME`

读写 nginx 变量。

- **读取**: `local value = ngx.var.host`
- **写入**: `ngx.var.my_var = "value"`
- **删除**: `ngx.var.my_var = nil`

**内部实现**: 通过 FFI 函数 `ngx_http_lua_ffi_var_get` 和 `ngx_http_lua_ffi_var_set`

---

## 八、请求上下文 API

### `ngx.ctx`

当前请求的 Lua 表，可在请求各阶段共享数据。

- **特点**:
  - 每个请求独立
  - 子请求有独立的 ctx（除非显式传递）
  - 请求结束时自动清理
- **示例**:
  ```lua
  -- access_by_lua
  ngx.ctx.user_id = 123

  -- content_by_lua (同一请求)
  ngx.say("User: ", ngx.ctx.user_id)
  ```

---

## 九、睡眠 API

### `ngx.sleep(seconds)`

非阻塞睡眠。

- **参数**: `seconds` (number) - 睡眠秒数，支持小数
- **返回值**: yield
- **示例**:
  ```lua
  ngx.sleep(0.1)  -- 睡眠 100ms
  ngx.sleep(1)    -- 睡眠 1s
  ```

---

## 十、常量定义

### 核心常量

| 常量 | 值 | 说明 |
|------|---|------|
| `ngx.OK` | 0 | 成功 |
| `ngx.ERROR` | -1 | 错误 |
| `ngx.AGAIN` | -2 | 再次 |
| `ngx.DONE` | -4 | 完成 |
| `ngx.DECLINED` | -5 | 拒绝 |
| `ngx.null` | lightuserdata | NULL 值 |

### HTTP 状态码常量

| 常量 | 值 |
|------|---|
| `ngx.HTTP_OK` | 200 |
| `ngx.HTTP_CREATED` | 201 |
| `ngx.HTTP_NO_CONTENT` | 204 |
| `ngx.HTTP_BAD_REQUEST` | 400 |
| `ngx.HTTP_UNAUTHORIZED` | 401 |
| `ngx.HTTP_FORBIDDEN` | 403 |
| `ngx.HTTP_NOT_FOUND` | 404 |
| `ngx.HTTP_INTERNAL_SERVER_ERROR` | 500 |
| `ngx.HTTP_SERVICE_UNAVAILABLE` | 503 |

### HTTP 方法常量

| 常量 | 值 |
|------|---|
| `ngx.HTTP_GET` | 2 |
| `ngx.HTTP_POST` | 8 |
| `ngx.HTTP_PUT` | 16 |
| `ngx.HTTP_DELETE` | 32 |
| `ngx.HTTP_HEAD` | 4 |
| `ngx.HTTP_PATCH` | 2048 |