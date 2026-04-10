# lua-nginx-module Balancer 负载均衡

本文档详细说明 lua-nginx-module 的负载均衡功能。

---

## 一、核心文件

| 文件 | 说明 |
|------|------|
| `src/ngx_http_lua_balancer.c` | Balancer 实现 |
| `src/ngx_http_lua_balancer.h` | 头文件定义 |

---

## 二、配置指令

### `balancer_by_lua_block` / `balancer_by_lua_file`

在 upstream 块中定义负载均衡逻辑。

```nginx
upstream backend {
    server 0.0.0.1;  # 占位符

    balancer_by_lua_block {
        local balancer = require "ngx.balancer"

        -- 动态选择后端
        local host = pick_backend()
        local ok, err = balancer.set_current_peer(host, 8080)
    }
}

server {
    location / {
        proxy_pass http://backend;
    }
}
```

---

## 三、Balancer API

### 3.1 `set_current_peer(host, port)`

设置当前请求的目标服务器。

- **参数**:
  - `host` (string): IP 地址或主机名
  - `port` (number): 端口号
- **返回值**: ok 或 nil, err
- **示例**:
  ```lua
  local balancer = require "ngx.balancer"
  local ok, err = balancer.set_current_peer("192.168.1.100", 8080)
  ```

### 3.2 `set_more_tries(count)`

设置额外重试次数。

- **参数**: `count` (number)
- **示例**:
  ```lua
  balancer.set_more_tries(3)  -- 额外尝试 3 次
  ```

### 3.3 `get_last_peer_address()`

获取上次请求的 peer 地址。

- **返回值**: host, port 或 nil, err

### 3.4 `set_timeouts(connect, send, read)`

设置超时时间。

- **参数**: 毫秒数
- **示例**:
  ```lua
  balancer.set_timeouts(1000, 2000, 5000)
  ```

### 3.5 `enable_keepalive(timeout, max_requests)`

启用 keepalive。

- **参数**:
  - `timeout` (number): 空闲超时毫秒
  - `max_requests` (number): 最大请求数
- **示例**:
  ```lua
  balancer.enable_keepalive(60000, 100)
  ```

### 3.6 `bind_to_local_addr(addr)`

绑定本地出站地址。

---

## 四、内部实现

### 4.1 回调替换

Balancer 替换 upstream 的标准回调：

| 原回调 | 替换为 |
|--------|--------|
| `peer.get` | `ngx_http_lua_balancer_get_peer` |
| `peer.free` | `ngx_http_lua_balancer_free_peer` |
| `peer.notify` | `ngx_http_lua_balancer_notify_peer` |

### 4.2 Keepalive Pool

```c
typedef struct {
    ngx_queue_t          queue;       // 空闲队列
    ngx_connection_t    *connection;  // 缓存的连接
    ngx_str_t            host;
    ngx_uint_t           port;
} ngx_http_lua_balancer_ka_item_t;
```

---

## 五、使用示例

### 5.1 简单轮询

```lua
local balancer = require "ngx.balancer"

local backends = {
    {host = "192.168.1.1", port = 8080},
    {host = "192.168.1.2", port = 8080},
    {host = "192.168.1.3", port = 8080},
}

local index = 0

local function pick_backend()
    index = (index % #backends) + 1
    return backends[index]
end

-- balancer_by_lua_block 中
local backend = pick_backend()
balancer.set_current_peer(backend.host, backend.port)
```

### 5.2 一致性哈希

```lua
local balancer = require "ngx.balancer"
local resty_chash = require "resty.chash"

local ring = resty_chash:new(backends)
local server = ring:find(ngx.var.request_uri)

balancer.set_current_peer(server.host, server.port)
```

### 5.3 健康检查集成

```lua
local balancer = require "ngx.balancer"
local healthcheck = require "resty.healthcheck"

local checker = healthcheck.new({...})

local function pick_healthy_backend()
    for _, backend in ipairs(backends) do
        if checker:get_status(backend.host, backend.port) then
            return backend
        end
    end
    return nil
end
```