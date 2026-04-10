# lua-nginx-module Cosocket 非阻塞 Socket API

本文档详细说明 lua-nginx-module 的非阻塞 Socket API (Cosocket)。

## 核心文件

| 文件路径 | 描述 |
|---------|------|
| `src/ngx_http_lua_socket_tcp.c` | TCP socket 实现 (~7000 行) |
| `src/ngx_http_lua_socket_tcp.h` | TCP socket 头文件 |
| `src/ngx_http_lua_socket_udp.c` | UDP socket 实现 (~1700 行) |
| `src/ngx_http_lua_socket_udp.h` | UDP socket 头文件 |

---

## 一、TCP Socket API

### 1.1 创建 Socket

#### `ngx.socket.tcp()` / `ngx.socket.stream()`

创建 TCP socket 对象。

- **返回值**: TCP socket 对象
- **示例**:
  ```lua
  local sock = ngx.socket.tcp()
  ```

### 1.2 连接

#### `sock:connect(host, port, options?)`

建立 TCP 连接。

- **参数**:
  - `host` (string): 主机名或 IP 地址
  - `port` (number): 端口号
  - `options` (table, 可选): 连接选项
- **参数格式**:
  ```lua
  -- IP + 端口
  sock:connect("127.0.0.1", 80)

  -- Unix Domain Socket
  sock:connect("unix:/path/to/socket")

  -- 带连接池选项
  sock:connect("127.0.0.1", 80, {
      pool_size = 10,    -- 连接池大小
      backlog = 5,       -- 等待队列长度
      pool = "my_pool"   -- 自定义池名称
  })
  ```
- **返回值**: 1 (成功) 或 nil, err
- **适用阶段**: rewrite, access, content

### 1.3 发送数据

#### `sock:send(data)`

发送数据。

- **参数**: `data` (string/number/boolean/table)
- **返回值**: bytes_sent (成功) 或 nil, err
- **示例**:
  ```lua
  local bytes, err = sock:send("GET / HTTP/1.1\r\n\r\n")
  local bytes, err = sock:send({ "line1", "line2" })
  ```

### 1.4 接收数据

#### `sock:receive(pattern?)`

接收数据。

- **参数**: `pattern` (string/number, 可选)
- **接收模式**:

| 模式 | 说明 |
|------|------|
| `nil` 或 `"*l"` | 读取一行 (默认) |
| `"*a"` | 读取所有数据直到 EOF |
| `number` | 读取指定字节数 |

- **返回值**: data (成功) 或 nil, err, partial
- **示例**:
  ```lua
  local line = sock:receive()       -- 读取一行
  local data = sock:receive("*a")   -- 读取全部
  local data = sock:receive(1024)   -- 读取 1024 字节
  ```

#### `sock:receiveany(max_bytes)`

接收最多指定字节数的数据。

- **参数**: `max_bytes` (number)
- **返回值**: data 或 nil, err

#### `sock:receiveuntil(pattern, options?)`

创建迭代器读取直到匹配模式。

- **参数**:
  - `pattern` (string): 结束模式
  - `options` (table): `{inclusive = true/false}`
- **返回值**: iterator 函数
- **示例**:
  ```lua
  local reader = sock:receiveuntil("\r\n\r\n")
  local data, err, partial = reader()
  local data4 = reader(4)  -- 读取 4 字节
  ```

### 1.5 超时设置

#### `sock:settimeout(ms)`

设置所有操作的超时时间。

#### `sock:settimeouts(connect_timeout, send_timeout, read_timeout)`

分别设置连接、发送、读取超时。

- **参数**: 毫秒数
- **示例**:
  ```lua
  sock:settimeout(5000)  -- 5 秒超时
  sock:settimeouts(1000, 2000, 5000)  -- 连接 1s, 发送 2s, 读取 5s
  ```

### 1.6 连接池管理

#### `sock:setkeepalive(timeout?, pool_size?)`

将连接放回连接池。

- **参数**:
  - `timeout` (number, 可选): 最大空闲时间，毫秒
  - `pool_size` (number, 可选): 池大小
- **返回值**: 1 或 nil, err
- **示例**:
  ```lua
  local ok, err = sock:setkeepalive(60000, 100)
  ```

#### `sock:getreusedtimes()`

获取连接被复用的次数。

- **返回值**: number
- **示例**:
  ```lua
  local times = sock:getreusedtimes()
  if times > 100 then
      -- 连接复用次数过多，关闭重建
      sock:close()
  end
  ```

### 1.7 关闭

#### `sock:close()`

关闭连接。

- **返回值**: 1 或 nil, err

### 1.8 SSL 握手

#### `sock:sslhandshake(session?, server_name?, verify?, ...)`

执行 SSL 握手。

- **参数**:
  - `session` (userdata, 可选): SSL 会话对象
  - `server_name` (string, 可选): SNI 主机名
  - `verify` (boolean, 可选): 是否验证证书
- **返回值**: session 或 nil, err
- **示例**:
  ```lua
  local session, err = sock:sslhandshake(nil, "example.com", true)
  ```

### 1.9 Socket 选项

#### `sock:setoption(option, value)`

设置 socket 选项 (FFI 接口)。

| 选项 | 说明 |
|------|------|
| `ngx.HTTP_LUA_SOCKOPT_KEEPALIVE` | SO_KEEPALIVE |
| `ngx.HTTP_LUA_SOCKOPT_TCP_NODELAY` | TCP_NODELAY |
| `ngx.HTTP_LUA_SOCKOPT_SNDBUF` | SO_SNDBUF |
| `ngx.HTTP_LUA_SOCKOPT_RCVBUF` | SO_RCVBUF |
| `ngx.HTTP_LUA_SOCKOPT_REUSEADDR` | SO_REUSEADDR |

---

## 二、UDP Socket API

### 2.1 创建 Socket

#### `ngx.socket.udp()`

创建 UDP socket 对象。

### 2.2 设置对端

#### `sock:setpeername(host, port)`

设置目标地址。

- **参数**:
  - `host` (string): 主机名或 IP
  - `port` (number): 端口号
- **示例**:
  ```lua
  local sock = ngx.socket.udp()
  sock:setpeername("127.0.0.1", 53)
  ```

### 2.3 发送

#### `sock:send(data)`

发送数据报。

- **参数**: `data` (string)
- **返回值**: 1 或 nil, err

### 2.4 接收

#### `sock:receive(size?)`

接收数据报。

- **参数**: `size` (number, 可选) - 最大 65536 字节
- **返回值**: data 或 nil, err

### 2.5 绑定本地地址

#### `sock:bind(address)`

绑定本地地址。

---

## 三、内部实现机制

### 3.1 非阻塞原理

Cosocket 通过以下机制实现非阻塞：

1. **协程挂起**: 调用 `lua_yield()` 挂起 Lua 协程
2. **事件注册**: 向 Nginx 事件循环注册读写事件
3. **恢复执行**: 事件触发时通过 `resume_handler` 恢复协程

### 3.2 连接池实现

```
连接池结构:
┌─────────────────────────────────────┐
│  pool = "host:port"                  │
│  ┌─────────┐ ┌─────────┐ ┌─────────┐│
│  │conn 1   │→│conn 2   │→│conn N   ││
│  └─────────┘ └─────────┘ └─────────┘│
│  free_queue                          │
└─────────────────────────────────────┘
```

**关键数据结构**:

```c
typedef struct {
    ngx_queue_t          queue;       // 空闲队列链接
    ngx_connection_t    *connection;  // 缓存的连接
    ngx_str_t            host;
    ngx_uint_t           port;
    // ...
} ngx_http_lua_socket_pool_item_t;
```

### 3.3 请求 Socket

#### `ngx.req.socket(raw?)`

获取请求的 raw socket。

- **参数**: `raw` (boolean) - 是否原始模式
- **返回值**: cosocket 对象
- **示例**:
  ```lua
  local sock, err = ngx.req.socket()
  sock:receive()  -- 接收客户端数据
  sock:send(data) -- 发送响应
  ```

---

## 四、最佳实践

### 4.1 连接池使用

```lua
local function query_redis()
    local sock = ngx.socket.tcp()
    sock:settimeout(1000)

    -- 使用连接池
    local ok, err = sock:connect("127.0.0.1", 6379, {pool = "redis"})
    if not ok then
        return nil, err
    end

    local bytes, err = sock:send("PING\r\n")
    local data, err = sock:receive()

    -- 放回连接池而非关闭
    sock:setkeepalive(60000, 100)

    return data
end
```

### 4.2 错误处理

```lua
local function safe_request()
    local sock = ngx.socket.tcp()
    local ok, err = sock:connect("127.0.0.1", 80)
    if not ok then
        ngx.log(ngx.ERR, "connect failed: ", err)
        return nil, err
    end

    local bytes, err = sock:send("GET / HTTP/1.0\r\n\r\n")
    if not bytes then
        sock:close()
        return nil, err
    end

    local reader = sock:receiveuntil("\r\n\r\n")
    local headers, err = reader()
    if not headers then
        sock:close()
        return nil, err
    end

    sock:setkeepalive()
    return headers
end
```