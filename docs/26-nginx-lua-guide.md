# NGINX Lua 模块深度指南

## 概述

NGINX Lua 模块（ngx_http_lua_module）是 OpenResty 平台的核心组件，它将 Lua 脚本语言嵌入 NGINX，使 NGINX 具备强大的动态脚本能力。本文档深入介绍 Lua 模块的核心概念、API 和最佳实践。

---

## 1. OpenResty 简介

### 1.1 什么是 OpenResty

OpenResty 是一个基于 NGINX 的高性能 Web 平台，由 agentzh（章亦春）创建。它在标准 NGINX 基础上集成了：

- **LuaJIT**：高性能 Lua 解释器，执行速度接近原生代码
- **ngx_lua**：NGINX Lua 嵌入模块
- **丰富的 Lua 库**：Redis、MySQL、Memcached 等客户端库
- **协程调度器**：异步非阻塞 I/O 支持

### 1.2 OpenResty 与传统 NGINX 的区别

| 特性 | 标准 NGINX | OpenResty |
|------|-----------|-----------|
| 脚本能力 | 有限（NJS） | 强大（完整 Lua 支持） |
| 性能 | 高 | 更高（LuaJIT） |
| 生态系统 | 模块扩展 | 丰富的 Lua 库 |
| 学习曲线 | 平缓 | 中等 |
| 典型应用 | 反向代理 | API 网关、WAF、边缘计算 |

### 1.3 安装 OpenResty

#### 使用包管理器安装

**Ubuntu/Debian：**
```bash
# 添加 OpenResty 仓库
wget -O - https://openresty.org/package/pubkey.gpg | sudo apt-key add -
sudo add-apt-repository -y "deb http://openresty.org/package/ubuntu $(lsb_release -sc) main"

# 安装
sudo apt-get update
sudo apt-get install -y openresty
```

**CentOS/RHEL：**
```bash
# 添加仓库
sudo yum install -y yum-utils
sudo yum-config-manager --add-repo https://openresty.org/package/centos/openresty.repo

# 安装
sudo yum install -y openresty
```

**macOS：**
```bash
brew install openresty
```

#### 从源码编译安装

```bash
# 下载源码
wget https://openresty.org/download/openresty-1.25.3.1.tar.gz
tar -xzf openresty-1.25.3.1.tar.gz
cd openresty-1.25.3.1

# 配置编译选项
./configure \
    --prefix=/usr/local/openresty \
    --with-http_ssl_module \
    --with-http_v2_module \
    --with-http_v3_module \
    --with-http_realip_module \
    --with-http_stub_status_module \
    --with-http_sub_module \
    --with-pcre-jit \
    --with-luajit

# 编译安装
make -j$(nproc)
sudo make install

# 添加到环境变量
echo 'export PATH=/usr/local/openresty/bin:$PATH' >> ~/.bashrc
source ~/.bashrc
```

#### 验证安装

```bash
# 检查版本
openresty -v
# 输出：nginx version: openresty/1.25.3.1

# 检查 LuaJIT
openresty -V 2>&1 | grep luajit
# 确认包含 --with-luajit

# 启动 OpenResty
sudo openresty

# 测试
curl http://localhost/
```

---

## 2. ngx_lua 核心指令

ngx_lua 提供了多个执行阶段指令，允许在不同请求处理阶段执行 Lua 代码。

### 2.1 执行阶段概览

```
请求处理流程：

┌─────────────────────────────────────────┐
│           init_by_lua                   │  ← NGINX 启动时
├─────────────────────────────────────────┤
│           init_worker_by_lua            │  ← 每个 worker 启动时
├─────────────────────────────────────────┤
│           ssl_certificate_by_lua        │  ← SSL 证书阶段（可选）
├─────────────────────────────────────────┤
│  set_by_lua  │  设置变量值              │
├─────────────────────────────────────────┤
│  rewrite_by_lua  │  URL 重写            │
├─────────────────────────────────────────┤
│  access_by_lua   │  访问控制            │
├─────────────────────────────────────────┤
│  content_by_lua  │  生成响应内容        │
├─────────────────────────────────────────┤
│  header_filter_by_lua  │  处理响应头    │
├─────────────────────────────────────────┤
│  body_filter_by_lua    │  处理响应体    │
├─────────────────────────────────────────┤
│  log_by_lua            │  日志阶段      │
└─────────────────────────────────────────┘
```

### 2.2 指令详解

#### init_by_lua

在 NGINX 启动时执行，用于全局初始化。

```nginx
http {
    # 加载 Lua 模块，预编译代码
    init_by_lua_block {
        require "cjson"
        require "resty.redis"
        require "resty.mysql"

        -- 预编译正则表达式
        local regex = [[\d+]]
        local m, err = ngx.re.match("hello 123", regex, "jo")

        -- 全局配置
        config = {
            redis_host = "127.0.0.1",
            redis_port = 6379,
            cache_ttl = 300
        }

        -- 打印启动信息
        ngx.log(ngx.NOTICE, "OpenResty initialized with LuaJIT")
    }
}
```

**适用场景：**
- 预加载常用模块
- 初始化全局变量/配置
- 建立数据库连接池
- 编译正则表达式

#### init_worker_by_lua

在每个 worker 进程启动时执行。

```nginx
http {
    init_worker_by_lua_block {
        local delay = 5  -- 5秒间隔
        local handler

        -- 定时任务：健康检查
        handler = function(premature)
            if premature then
                return
            end

            -- 执行健康检查
            local http = require "resty.http"
            local httpc = http.new()
            local res, err = httpc:request_uri("http://127.0.0.1:8080/health", {
                method = "GET",
                timeout = 2000
            })

            if res and res.status == 200 then
                ngx.log(ngx.INFO, "Health check passed")
            else
                ngx.log(ngx.ERR, "Health check failed: ", err)
            end

            -- 重新注册定时器
            local ok, err = ngx.timer.at(delay, handler)
            if not ok then
                ngx.log(ngx.ERR, "Failed to create timer: ", err)
            end
        end

        -- 启动定时器
        ngx.timer.at(delay, handler)

        ngx.log(ngx.NOTICE, "Worker ", ngx.worker.id(), " started")
    }
}
```

**适用场景：**
- 启动后台定时任务
- worker 级别的初始化
- 定时数据采集
- 缓存预热

#### set_by_lua

设置 NGINX 变量值。

```nginx
location /api {
    set_by_lua_block $api_backend {
        local version = ngx.var.http_x_api_version

        if version == "v2" then
            return "backend_v2"
        else
            return "backend_v1"
        end
    }

    proxy_pass http://$api_backend;
}
```

**限制：**
- 不能使用阻塞 I/O
- 不能使用 `ngx.sleep`
- 不能使用 cosocket

#### rewrite_by_lua

在 rewrite 阶段执行，用于 URL 重写和重定向。

```nginx
location / {
    rewrite_by_lua_block {
        local uri = ngx.var.uri
        local args = ngx.var.args

        -- 统一处理尾部斜杠
        if uri ~= "/" and uri:sub(-1) == "/" then
            uri = uri:sub(1, -2)
        end

        -- 旧 URL 兼容处理
        if uri:match("^/old%-api/") then
            local new_uri = uri:gsub("^/old%-api", "/api/v1")
            return ngx.redirect(new_uri .. (args and "?" .. args or ""), 301)
        end

        -- 设置内部变量
        ngx.var.target_service = "user_service"
    }

    proxy_pass http://backend;
}
```

#### access_by_lua

在 access 阶段执行，用于访问控制和认证。

```nginx
location /admin {
    access_by_lua_block {
        local token = ngx.var.http_authorization

        if not token then
            ngx.header["WWW-Authenticate"] = "Bearer"
            return ngx.exit(ngx.HTTP_UNAUTHORIZED)
        end

        -- JWT 验证
        local jwt = require "resty.jwt"
        local jwt_obj = jwt:verify("secret_key", token:gsub("Bearer ", ""))

        if not jwt_obj.verified then
            ngx.log(ngx.ERR, "JWT verification failed: ", jwt_obj.reason)
            return ngx.exit(ngx.HTTP_UNAUTHORIZED)
        end

        -- 设置用户信息到变量
        ngx.var.user_id = jwt_obj.payload.sub
        ngx.var.user_role = jwt_obj.payload.role
    }

    proxy_pass http://admin_backend;
}
```

#### content_by_lua

生成响应内容的核心指令。

```nginx
location /api/status {
    content_by_lua_block {
        local cjson = require "cjson"

        local status = {
            nginx_version = ngx.var.nginx_version,
            lua_version = _VERSION,
            worker_id = ngx.worker.id(),
            worker_count = ngx.worker.count(),
            time = ngx.time(),
            connections = ngx.var.connections_active
        }

        ngx.header["Content-Type"] = "application/json"
        ngx.say(cjson.encode(status))
    }
}
```

#### header_filter_by_lua

修改响应头。

```nginx
location / {
    proxy_pass http://backend;

    header_filter_by_lua_block {
        -- 添加安全头部
        ngx.header["X-Frame-Options"] = "SAMEORIGIN"
        ngx.header["X-XSS-Protection"] = "1; mode=block"
        ngx.header["X-Content-Type-Options"] = "nosniff"

        -- 移除敏感头部
        ngx.header["Server"] = nil
        ngx.header["X-Powered-By"] = nil

        -- 根据条件设置缓存
        if ngx.var.uri:match("^/api/") then
            ngx.header["Cache-Control"] = "no-store, no-cache, must-revalidate"
        end
    }
}
```

#### body_filter_by_lua

修改响应体（基于流式处理）。

```nginx
location / {
    proxy_pass http://backend;

    body_filter_by_lua_block {
        local chunk = ngx.arg[1]
        local eof = ngx.arg[2]

        -- 累积响应体
        if ngx.ctx.body then
            ngx.ctx.body = ngx.ctx.body .. chunk
        else
            ngx.ctx.body = chunk
        end

        -- 最后一块数据处理
        if eof then
            local body = ngx.ctx.body

            -- 敏感信息脱敏
            body = body:gsub("\"phone\":\"(%d%d%d)%d%d%d%d(\d%d%d%d)\"",
                             "\"phone\":\"%1****%2\"")
            body = body:gsub("\"email\":\"([^@]+)@[^\"]+\"",
                             "\"email\":\"%1@***\"")

            ngx.arg[1] = body
        else
            -- 非最后一块，输出空并标记不完成
            ngx.arg[1] = nil
            ngx.arg[2] = false
        end
    }
}
```

#### log_by_lua

日志阶段处理。

```nginx
http {
    log_by_lua_block {
        local uri = ngx.var.uri
        local status = ngx.var.status
        local request_time = ngx.var.request_time

        -- 慢请求记录
        if tonumber(request_time) > 1.0 then
            ngx.log(ngx.WARN, "Slow request: ", uri,
                    " status=", status,
                    " time=", request_time)
        end

        -- 统计信息发送到监控
        if status >= 500 then
            local statsd = require "resty.statsd"
            statsd.increment("nginx.error." .. status)
        end
    }
}
```

### 2.3 指令上下文支持

| 指令 | http | server | location | upstream |
|------|------|--------|----------|----------|
| `init_by_lua` | ✓ | ✗ | ✗ | ✗ |
| `init_worker_by_lua` | ✓ | ✗ | ✗ | ✗ |
| `set_by_lua` | ✗ | ✓ | ✓ | ✗ |
| `rewrite_by_lua` | ✓ | ✓ | ✓ | ✗ |
| `access_by_lua` | ✓ | ✓ | ✓ | ✗ |
| `content_by_lua` | ✗ | ✗ | ✓ | ✗ |
| `header_filter_by_lua` | ✓ | ✓ | ✓ | ✗ |
| `body_filter_by_lua` | ✓ | ✓ | ✓ | ✗ |
| `log_by_lua` | ✓ | ✓ | ✓ | ✗ |
| `balancer_by_lua` | ✗ | ✗ | ✗ | ✓ |

---

## 3. Lua 共享字典（ngx.shared.DICT）

共享字典是在所有 worker 进程间共享的内存缓存。

### 3.1 定义共享字典

```nginx
http {
    # 语法：lua_shared_dict <name> <size>
    lua_shared_dict cache 10m;           # 通用缓存
    lua_shared_dict sessions 5m;         # 会话存储
    lua_shared_dict rate_limit 1m;       # 限流计数
    lua_shared_dict locks 1m;            # 锁存储
}
```

**内存估算：**
- 每个 key-value 对约占用 60-80 字节（小数据）
- 1MB 可存储约 12,000-16,000 个简单键值对

### 3.2 基本操作

```lua
-- 获取字典实例
local cache = ngx.shared.cache

-- 存储数据（支持过期时间）
cache:set("key", "value", 300)         -- 300秒后过期
cache:set("key", "value", 0)          -- 永不过期

-- 带过期时间的存储
cache:set("session:123", user_data, 1800)  -- 30分钟会话

-- 获取数据
local value, flags = cache:get("key")
if value then
    ngx.say("Value: ", value)
else
    ngx.say("Cache miss")
end

-- 删除数据
cache:delete("key")

-- 原子递增（用于计数器）
local new_val, err = cache:incr("counter", 1, 0)  -- 从0开始，每次+1

-- 批量获取
cache:set("user:1", "Alice")
cache:set("user:2", "Bob")
cache:set("user:3", "Charlie")

local keys = {"user:1", "user:2", "user:3"}
local values = cache:get(keys)
```

### 3.3 高级操作

```lua
local cache = ngx.shared.cache

-- 安全添加（仅当 key 不存在时设置）
local success, err, forcible = cache:add("key", "value", 300)
if not success then
    ngx.log(ngx.ERR, "Failed to add: ", err)
end

-- 安全替换（仅当 key 存在时设置）
local success = cache:replace("key", "new_value")

-- 原子操作：如果不存在则设置
local ok = cache:add("lock:process", "1", 10)
if ok then
    -- 获取到锁
    -- 执行操作
    cache:delete("lock:process")
end

-- 获取过期时间
local ttl, err = cache:ttl("key")
if ttl then
    ngx.say("TTL: ", ttl, " seconds")
end

-- 获取信息
local info = cache:get_keys(0)  -- 0 表示获取所有 keys
ngx.say("Keys count: ", #info)

-- 清空字典（慎用）
cache:flush_all()

-- 过期数据清理
cache:flush_expired(100)  -- 清理最多100个过期条目
```

### 3.4 应用场景

#### 分布式限流

```nginx
http {
    lua_shared_dict rate_limit 10m;

    server {
        location /api {
            access_by_lua_block {
                local limit = ngx.shared.rate_limit
                local key = "rate:" .. ngx.var.binary_remote_addr

                -- 令牌桶算法实现
                local now = ngx.time()
                local rate = 10  -- 每秒10个请求
                local burst = 20  -- 突发容量

                local last = limit:get(key .. ":last")
                local tokens = limit:get(key .. ":tokens")

                if not last then
                    tokens = burst
                else
                    local elapsed = now - last
                    tokens = math.min(burst, tokens + elapsed * rate)
                end

                if tokens < 1 then
                    return ngx.exit(ngx.HTTP_TOO_MANY_REQUESTS)
                end

                tokens = tokens - 1
                limit:set(key .. ":tokens", tokens)
                limit:set(key .. ":last", now)
            }

            proxy_pass http://api_backend;
        }
    }
}
```

#### 会话存储

```nginx
http {
    lua_shared_dict sessions 20m;

    server {
        location /login {
            content_by_lua_block {
                local cjson = require "cjson"
                local sessions = ngx.shared.sessions

                -- 验证用户名密码
                local username = ngx.var.arg_username
                local password = ngx.var.arg_password

                if not authenticate(username, password) then
                    return ngx.exit(ngx.HTTP_UNAUTHORIZED)
                end

                -- 创建会话
                local session_id = ngx.md5(ngx.time() .. ngx.var.remote_addr)
                local session_data = {
                    username = username,
                    login_time = ngx.time(),
                    ip = ngx.var.remote_addr
                }

                sessions:set("session:" .. session_id,
                            cjson.encode(session_data), 3600)

                -- 设置 Cookie
                ngx.header["Set-Cookie"] = "session=" .. session_id ..
                                           "; Path=/; HttpOnly; Secure"
                ngx.say("Login successful")
            }
        }

        location /profile {
            access_by_lua_block {
                local cjson = require "cjson"
                local sessions = ngx.shared.sessions

                -- 获取 session
                local cookie = ngx.var.cookie_session
                if not cookie then
                    return ngx.exit(ngx.HTTP_UNAUTHORIZED)
                end

                local data = sessions:get("session:" .. cookie)
                if not data then
                    return ngx.exit(ngx.HTTP_UNAUTHORIZED)
                end

                -- 解析会话数据
                local session = cjson.decode(data)
                ngx.var.user_name = session.username
            }

            proxy_pass http://backend;
        }
    }
}
```

#### 缓存穿透防护

```lua
-- 缓存穿透防护（防止缓存击穿和雪崩）
local function get_with_lock(cache, key, ttl, fetch_func)
    -- 1. 尝试从缓存获取
    local value = cache:get(key)
    if value then
        return value
    end

    local lock_key = "lock:" .. key
    local lock_ttl = 10  -- 锁超时时间

    -- 2. 尝试获取锁
    local ok = cache:add(lock_key, "1", lock_ttl)
    if not ok then
        -- 3. 未获取到锁，等待后重试
        ngx.sleep(0.1)
        return cache:get(key)
    end

    -- 4. 获取到锁，从数据源加载
    local ok2, result = pcall(fetch_func)
    if ok2 and result then
        cache:set(key, result, ttl)
    end

    -- 5. 释放锁
    cache:delete(lock_key)

    return result
end

-- 使用示例
local value = get_with_lock(ngx.shared.cache, "user:123", 300, function()
    -- 从数据库获取
    return fetch_from_db("user", 123)
end)
```

---

## 4. Cosocket API（非阻塞网络 I/O）

Cosocket 是 OpenResty 提供的非阻塞网络 I/O 接口，支持 TCP、UDP 和 Unix Domain Socket。

### 4.1 TCP Cosocket

```lua
-- 创建 TCP socket
local sock = ngx.socket.tcp()

-- 设置超时
sock:settimeout(5000)  -- 5秒超时

-- 连接服务器
local ok, err = sock:connect("127.0.0.1", 6379)
if not ok then
    ngx.log(ngx.ERR, "Failed to connect: ", err)
    return
end

-- 发送数据
local bytes, err = sock:send("PING\r\n")
if not bytes then
    ngx.log(ngx.ERR, "Failed to send: ", err)
    return
end

-- 接收数据
local line, err = sock:receive("*l")  -- 接收一行
if not line then
    ngx.log(ngx.ERR, "Failed to receive: ", err)
    return
end

ngx.say("Response: ", line)

-- 关闭连接
sock:close()
```

### 4.2 高级用法

```lua
local sock = ngx.socket.tcp()

-- 连接池复用
sock:setkeepalive(60000, 100)  -- 60秒超时，最多100个连接

-- 指定模式接收
local data, err = sock:receive(1024)      -- 接收最多1024字节
local data, err = sock:receive("*a")      -- 接收所有数据
local data, err = sock:receiveuntil("\r\n") -- 接收直到指定分隔符

-- 批量发送
local ok, err = sock:send({
    "GET / HTTP/1.1\r\n",
    "Host: example.com\r\n",
    "Connection: close\r\n",
    "\r\n"
})
```

### 4.3 UDP Cosocket

```lua
local sock = ngx.socket.udp()

-- 设置超时
sock:settimeout(2000)

-- 设置目标
local ok, err = sock:setpeername("127.0.0.1", 53)
if not ok then
    ngx.log(ngx.ERR, "Failed to set peer: ", err)
    return
end

-- 发送 DNS 查询
local query = build_dns_query("example.com")
local ok, err = sock:send(query)

-- 接收响应
local data, err = sock:receive(512)
if data then
    local result = parse_dns_response(data)
    ngx.say("IP: ", result)
end

sock:close()
```

### 4.4 异步 HTTP 请求

使用 `resty.http` 库进行异步 HTTP 请求：

```bash
# 安装 lua-resty-http
luarocks install lua-resty-http
```

```lua
local http = require "resty.http"

-- 简单 GET 请求
local httpc = http.new()
local res, err = httpc:request_uri("http://api.example.com/data", {
    method = "GET",
    headers = {
        ["Accept"] = "application/json"
    }
})

if res then
    ngx.status = res.status
    ngx.say(res.body)
else
    ngx.status = 502
    ngx.say("Request failed: ", err)
end
```

#### 高级 HTTP 请求

```lua
local http = require "resty.http"

local httpc = http.new()

-- 配置超时
httpc:set_timeout(5000)

-- 建立连接（连接复用）
local ok, err = httpc:connect("api.example.com", 443)
if not ok then
    return ngx.exit(ngx.HTTP_BAD_GATEWAY)
end

-- SSL 握手
local session, err = httpc:ssl_handshake(false, "api.example.com", false)

-- 发送请求
local res, err = httpc:request({
    method = "POST",
    path = "/v1/users",
    headers = {
        ["Content-Type"] = "application/json",
        ["Authorization"] = "Bearer " .. token
    },
    body = [[{"name":"John","email":"john@example.com"}]]
})

if not res then
    ngx.log(ngx.ERR, "Request failed: ", err)
    return ngx.exit(ngx.HTTP_BAD_GATEWAY)
end

-- 流式读取响应
local reader = res.body_reader
repeat
    local chunk, err = reader(8192)
    if err then
        ngx.log(ngx.ERR, "Read error: ", err)
        break
    end
    if chunk then
        ngx.print(chunk)
    end
until not chunk

-- 保持连接复用
local ok, err = httpc:set_keepalive(60000, 100)
```

---

## 5. 与 Redis/MySQL 集成

### 5.1 Redis 集成

```bash
# 安装 lua-resty-redis（OpenResty 已内置）
```

#### 基础操作

```lua
local redis = require "resty.redis"

-- 创建连接
local red = redis:new()
red:set_timeout(1000)  -- 1秒超时

-- 连接
local ok, err = red:connect("127.0.0.1", 6379)
if not ok then
    ngx.log(ngx.ERR, "Failed to connect: ", err)
    return ngx.exit(ngx.HTTP_INTERNAL_SERVER_ERROR)
end

-- 基本操作
red:set("key", "value")
red:set("key", "value", 300)  -- 带过期时间

local res, err = red:get("key")
if res == ngx.null then
    ngx.say("Key not found")
else
    ngx.say("Value: ", res)
end

-- 列表操作
red:lpush("queue", "task1")
red:lpush("queue", "task2")
local task = red:rpop("queue")

-- 哈希操作
red:hset("user:1001", "name", "Alice")
red:hset("user:1001", "age", "30")
local user = red:hgetall("user:1001")

-- 事务
red:multi()
red:incr("counter")
red:lpush("log", "new entry")
local res, err = red:exec()

-- 连接池
local ok, err = red:set_keepalive(60000, 100)
```

#### Redis 连接池封装

```lua
-- redis_pool.lua
local redis = require "resty.redis"
local _M = {}

function _M.get_connection()
    local red = redis:new()
    red:set_timeout(1000)

    -- 使用 Unix socket（性能更好）
    local ok, err = red:connect("unix:/var/run/redis/redis.sock")
    if not ok then
        -- 回退到 TCP
        ok, err = red:connect("127.0.0.1", 6379)
        if not ok then
            return nil, err
        end
    end

    -- 认证（如有密码）
    -- local res, err = red:auth("password")

    return red, nil
end

function _M.return_connection(red)
    if not red then
        return
    end
    local ok, err = red:set_keepalive(60000, 100)
    if not ok then
        red:close()
    end
end

return _M
```

#### 缓存模式封装

```lua
-- cache.lua
local redis_pool = require "redis_pool"
local cjson = require "cjson"
local _M = {}

function _M.get(key, ttl, fetch_func)
    local red, err = redis_pool.get_connection()
    if not red then
        -- Redis 不可用，直接获取数据
        return fetch_func()
    end

    -- 尝试从 Redis 获取
    local data, err = red:get("cache:" .. key)
    if data and data ~= ngx.null then
        redis_pool.return_connection(red)
        return cjson.decode(data)
    end

    -- 从数据源获取
    local result = fetch_func()
    if result then
        -- 异步写入 Redis
        red:set("cache:" .. key, cjson.encode(result), "EX", ttl)
    end

    redis_pool.return_connection(red)
    return result
end

return _M
```

### 5.2 MySQL 集成

```bash
# 安装 lua-resty-mysql（OpenResty 已内置）
```

#### 基础操作

```lua
local mysql = require "resty.mysql"

-- 创建连接
local db, err = mysql:new()
if not db then
    ngx.log(ngx.ERR, "Failed to create mysql: ", err)
    return
end

db:set_timeout(1000)

-- 连接数据库
local ok, err, errcode, sqlstate = db:connect({
    host = "127.0.0.1",
    port = 3306,
    database = "test",
    user = "root",
    password = "password",
    charset = "utf8mb4",
    max_packet_size = 1024 * 1024,
    pool = "mysqlpool"  -- 连接池名称
})

if not ok then
    ngx.log(ngx.ERR, "Failed to connect: ", err)
    return
end

-- 查询
local res, err, errcode, sqlstate = db:query("SELECT * FROM users WHERE id = 1")
if not res then
    ngx.log(ngx.ERR, "Query failed: ", err)
    return
end

-- 处理结果
for i, row in ipairs(res) do
    ngx.say("User: ", row.name, ", Email: ", row.email)
end

-- 插入/更新
local res, err = db:query([[
    INSERT INTO users (name, email)
    VALUES ('John', 'john@example.com')
]])

if res then
    ngx.say("Inserted ID: ", res.insert_id)
end

-- 事务
local res, err = db:query("START TRANSACTION")
local res1, err1 = db:query("UPDATE accounts SET balance = balance - 100 WHERE id = 1")
local res2, err2 = db:query("UPDATE accounts SET balance = balance + 100 WHERE id = 2")

if res1 and res2 then
    db:query("COMMIT")
else
    db:query("ROLLBACK")
end

-- 放回连接池
local ok, err = db:set_keepalive(60000, 100)
```

#### MySQL 连接池封装

```lua
-- mysql_pool.lua
local mysql = require "resty.mysql"
local cjson = require "cjson"

local _M = {
    config = {
        host = "127.0.0.1",
        port = 3306,
        database = "app",
        user = "app_user",
        password = "app_pass",
        charset = "utf8mb4",
        max_packet_size = 1024 * 1024,
        pool_size = 50
    }
}

function _M.query(sql)
    local db, err = mysql:new()
    if not db then
        return nil, err
    end

    db:set_timeout(3000)

    local ok, err = db:connect({
        host = _M.config.host,
        port = _M.config.port,
        database = _M.config.database,
        user = _M.config.user,
        password = _M.config.password,
        charset = _M.config.charset,
        max_packet_size = _M.config.max_packet_size,
        pool = "mysqlpool"
    })

    if not ok then
        return nil, err
    end

    local res, err = db:query(sql)
    db:set_keepalive(60000, _M.config.pool_size)

    if not res then
        return nil, err
    end

    return res, nil
end

-- 参数化查询（防 SQL 注入）
function _M.escape(str)
    return ngx.quote_sql_str(str)
end

return _M
```

### 5.3 综合示例：用户信息查询

```nginx
location /api/user {
    content_by_lua_block {
        local cjson = require "cjson"
        local user_id = ngx.var.arg_id

        if not user_id or not user_id:match("^%d+$") then
            ngx.status = 400
            ngx.say("Invalid user ID")
            return
        end

        -- 1. 尝试从 Redis 获取
        local redis = require "resty.redis"
        local red = redis:new()
        red:set_timeout(1000)

        local ok, err = red:connect("127.0.0.1", 6379)
        if ok then
            local data = red:get("user:" .. user_id)
            if data and data ~= ngx.null then
                ngx.header["Content-Type"] = "application/json"
                ngx.header["X-Cache"] = "HIT"
                ngx.say(data)
                red:set_keepalive(60000, 100)
                return
            end
            red:set_keepalive(60000, 100)
        end

        -- 2. 从 MySQL 查询
        local mysql = require "resty.mysql"
        local db = mysql:new()
        db:set_timeout(2000)

        local ok, err = db:connect({
            host = "127.0.0.1",
            port = 3306,
            database = "users",
            user = "readonly",
            password = "readonly",
            charset = "utf8mb4"
        })

        if not ok then
            ngx.status = 500
            ngx.say("Database error")
            return
        end

        local sql = "SELECT id, name, email, created_at FROM users WHERE id = " .. user_id
        local res, err = db:query(sql)
        db:set_keepalive(60000, 50)

        if not res or #res == 0 then
            ngx.status = 404
            ngx.say("User not found")
            return
        end

        local user = res[1]
        local response = cjson.encode(user)

        -- 3. 异步写入 Redis（不阻塞响应）
        local ok, err = red:connect("127.0.0.1", 6379)
        if ok then
            red:set("user:" .. user_id, response, "EX", 300)  -- 缓存5分钟
            red:set_keepalive(60000, 100)
        end

        ngx.header["Content-Type"] = "application/json"
        ngx.header["X-Cache"] = "MISS"
        ngx.say(response)
    }
}
```

---

## 6. 性能优化技巧

### 6.1 LuaJIT 优化

```lua
-- 1. 使用局部变量缓存全局变量
local ngx = ngx
local cjson = require "cjson"
local http = require "resty.http"

-- 2. 避免在循环中创建函数
local function process_item(item)
    return item * 2
end

for i = 1, 1000 do
    process_item(i)  -- 比内联函数更高效
end

-- 3. 使用 table.new 预分配（OpenResty 扩展）
local new_tab = require "table.new"
local t = new_tab(100, 0)  -- 预分配100个数组元素

-- 4. 字符串拼接使用 table.concat
local parts = {}
for i = 1, 100 do
    parts[i] = "item" .. i
end
local result = table.concat(parts, ",")

-- 5. 使用 ngx.re 而不是 Lua 正则
-- 高效
local m, err = ngx.re.match(str, [[\d+]], "jo")
-- 低效
local m = str:match("%d+")

-- 6. 避免使用 pairs/ipairs 进行数值索引遍历
-- 高效
for i = 1, #arr do
    local v = arr[i]
end
-- 低效
for i, v in ipairs(arr) do
end
```

### 6.2 连接池优化

```lua
-- 合理设置连接池大小
local pool_size = 100
local keepalive_timeout = 60000  -- 60秒

-- Redis
red:set_keepalive(keepalive_timeout, pool_size)

-- MySQL
db:set_keepalive(keepalive_timeout, pool_size)

-- HTTP
httpc:set_keepalive(keepalive_timeout, pool_size)
```

### 6.3 缓存策略

| 策略 | 适用场景 | 实现方式 |
|------|---------|---------|
| **本地缓存** | 热点数据、配置信息 | `ngx.shared.DICT` |
| **Redis 缓存** | 分布式缓存、会话 | `resty.redis` |
| **多级缓存** | 高并发读取 | L1（本地）+ L2（Redis） |
| **缓存预热** | 系统启动时 | `init_worker_by_lua` |
| **缓存穿透防护** | 防止缓存击穿 | 互斥锁 + 空值缓存 |

### 6.4 Worker 间通信

```lua
-- 使用共享字典实现 worker 间通信
http {
    lua_shared_dict ipc 1m;

    init_worker_by_lua_block {
        local ipc = ngx.shared.ipc

        -- 订阅消息
        local check_message
        check_message = function(premature)
            if premature then return end

            -- 获取消息
            local msg = ipc:get("broadcast")
            if msg then
                -- 处理消息
                ngx.log(ngx.INFO, "Worker ", ngx.worker.id(),
                       " received: ", msg)
                ipc:delete("broadcast")
            end

            ngx.timer.at(0.1, check_message)
        end

        ngx.timer.at(0.1, check_message)
    }
}
```

### 6.5 内存管理

```lua
-- 1. 及时释放大对象
local large_data = fetch_large_data()
-- 处理数据
process(large_data)
large_data = nil  -- 显式释放引用

-- 2. 使用弱引用表（缓存场景）
local weak_cache = setmetatable({}, {
    __mode = "v"
})

-- 3. 避免闭包捕获大对象
local function create_handler(config)
    -- 只捕获需要的字段
    local timeout = config.timeout
    return function()
        -- 使用 timeout，不持有整个 config
    end
end

-- 4. 控制字符串创建
-- 使用 string.sub 而不是正则提取
local prefix = str:sub(1, 10)
```

---

## 7. 完整配置示例

### 7.1 API 网关配置

```nginx
user openresty;
worker_processes auto;
error_log /var/log/openresty/error.log warn;
pid /run/openresty.pid;

events {
    worker_connections 4096;
    use epoll;
    multi_accept on;
}

http {
    include /usr/local/openresty/nginx/conf/mime.types;
    default_type application/octet-stream;

    # Lua 共享字典
    lua_shared_dict cache 50m;
    lua_shared_dict rate_limit 10m;
    lua_shared_dict sessions 20m;
    lua_shared_dict locks 5m;

    # Lua 库路径
    lua_package_path "/usr/local/openresty/site/lualib/?.lua;;";
    lua_package_cpath "/usr/local/openresty/site/lualib/?.so;;";

    # 全局初始化
    init_by_lua_block {
        require "cjson"
        require "resty.redis"
        require "resty.mysql"

        -- 全局配置
        CONFIG = {
            redis = { host = "127.0.0.1", port = 6379 },
            mysql = {
                host = "127.0.0.1", port = 3306,
                database = "api_db",
                user = "api_user", password = "api_pass"
            },
            jwt_secret = "your-secret-key",
            rate_limit = { rps = 100, burst = 200 }
        }
    }

    # Worker 初始化
    init_worker_by_lua_block {
        -- 定期清理过期缓存
        local flush_expired
        flush_expired = function(premature)
            if premature then return end
            ngx.shared.cache:flush_expired(100)
            ngx.timer.at(30, flush_expired)
        end
        ngx.timer.at(30, flush_expired)
    }

    # 日志格式
    log_format api_log '$remote_addr - $remote_user [$time_local] '
                       '"$request" $status $body_bytes_sent '
                       '"$http_referer" "$http_user_agent" '
                       'rt=$request_time uct="$upstream_connect_time" '
                       'uht="$upstream_header_time" urt="$upstream_response_time"'
                       ' cache=$upstream_http_x_cache';

    access_log /var/log/openresty/access.log api_log;

    # 性能优化
    sendfile on;
    tcp_nopush on;
    tcp_nodelay on;
    keepalive_timeout 65;
    types_hash_max_size 2048;

    # 限流区域
    limit_req_zone $binary_remote_addr zone=ip:10m rate=10r/s;

    # 上游服务器
    upstream api_backend {
        server 127.0.0.1:8080 weight=5;
        server 127.0.0.1:8081 weight=5;
        keepalive 100;
    }

    # 主服务器
    server {
        listen 80;
        server_name api.example.com;
        return 301 https://$server_name$request_uri;
    }

    server {
        listen 443 ssl http2;
        server_name api.example.com;

        # SSL 配置
        ssl_certificate /etc/ssl/certs/api.crt;
        ssl_certificate_key /etc/ssl/private/api.key;
        ssl_protocols TLSv1.2 TLSv1.3;
        ssl_ciphers HIGH:!aNULL:!MD5;
        ssl_prefer_server_ciphers on;

        # 安全头部
        add_header X-Frame-Options "SAMEORIGIN" always;
        add_header X-XSS-Protection "1; mode=block" always;
        add_header X-Content-Type-Options "nosniff" always;
        add_header Referrer-Policy "strict-origin-when-cross-origin" always;

        # 全局访问控制
        location / {
            # IP 白名单检查
            access_by_lua_block {
                local whitelist = { ["10.0.0.0/24"] = true }
                -- 实现 IP 检查逻辑
            }

            proxy_pass http://api_backend;
            proxy_http_version 1.1;
            proxy_set_header Connection "";
            proxy_set_header Host $host;
            proxy_set_header X-Real-IP $remote_addr;
            proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        }

        # 健康检查
        location /health {
            access_log off;
            content_by_lua_block {
                local cjson = require "cjson"
                local health = {
                    status = "healthy",
                    time = ngx.time(),
                    worker = ngx.worker.id()
                }
                ngx.header["Content-Type"] = "application/json"
                ngx.say(cjson.encode(health))
            }
        }

        # API 路由
        location /api/v1/ {
            # 限流
            limit_req zone=ip burst=20 nodelay;

            access_by_lua_block {
                -- JWT 认证
                local token = ngx.var.http_authorization
                if not token then
                    return ngx.exit(ngx.HTTP_UNAUTHORIZED)
                end

                -- 验证 JWT（简化示例）
                -- 实际使用 resty.jwt 库
            }

            header_filter_by_lua_block {
                ngx.header["X-API-Version"] = "v1"
            }

            proxy_pass http://api_backend;
        }

        # 登录接口
        location /api/auth/login {
            limit_req zone=ip burst=5 nodelay;

            content_by_lua_block {
                local cjson = require "cjson"

                ngx.req.read_body()
                local data = ngx.req.get_body_data()
                if not data then
                    ngx.status = 400
                    ngx.say(cjson.encode({ error = "No body" }))
                    return
                end

                local args = cjson.decode(data)
                -- 验证用户名密码
                -- 生成 JWT

                ngx.header["Content-Type"] = "application/json"
                ngx.say(cjson.encode({
                    token = "jwt_token_here",
                    expires_in = 3600
                }))
            }
        }

        # 静态资源
        location /static/ {
            alias /var/www/static/;
            expires 30d;
            add_header Cache-Control "public, immutable";
        }
    }
}
```

### 7.2 WAF（Web 应用防火墙）配置

```nginx
http {
    lua_shared_dict waf_rules 10m;
    lua_shared_dict waf_block 50m;

    init_by_lua_block {
        -- 加载 WAF 规则
        local cjson = require "cjson"
        local rules = {
            {
                id = "1001",
                name = "SQL Injection",
                pattern = [[(?:union|select|insert|update|delete|drop|create)\s+]],
                severity = "high",
                action = "block"
            },
            {
                id = "1002",
                name = "XSS Attack",
                pattern = [[<script[^>]*>[\s\S]*?</script>]],
                severity = "high",
                action = "block"
            },
            {
                id = "1003",
                name = "Path Traversal",
                pattern = [[\.\./\.\.]],
                severity = "medium",
                action = "block"
            }
        }

        -- 存储到共享字典
        local waf_rules = ngx.shared.waf_rules
        for _, rule in ipairs(rules) do
            waf_rules:set(rule.id, cjson.encode(rule))
        end
    }

    server {
        listen 80;
        server_name protected.example.com;

        location / {
            access_by_lua_block {
                local cjson = require "cjson"
                local waf_rules = ngx.shared.waf_rules
                local waf_block = ngx.shared.waf_block

                local ip = ngx.var.remote_addr

                -- 检查 IP 是否被封锁
                if waf_block:get("block:" .. ip) then
                    return ngx.exit(ngx.HTTP_FORBIDDEN)
                end

                -- 获取所有规则
                local rules = waf_rules:get_keys(0)
                local matched = false
                local matched_rule = nil

                -- 检查请求
                local check_string = ngx.var.request_uri .. " " ..
                                    (ngx.var.http_user_agent or "") .. " " ..
                                    (ngx.var.http_cookie or "")

                for _, rule_id in ipairs(rules) do
                    local rule_data = waf_rules:get(rule_id)
                    if rule_data then
                        local rule = cjson.decode(rule_data)
                        local m, err = ngx.re.match(check_string, rule.pattern, "ijo")
                        if m then
                            matched = true
                            matched_rule = rule
                            break
                        end
                    end
                end

                if matched then
                    -- 记录攻击
                    ngx.log(ngx.ERR, "WAF blocked request from ", ip,
                           " Rule: ", matched_rule.id, " - ", matched_rule.name)

                    -- 增加计数
                    local count, err = waf_block:incr("count:" .. ip, 1, 0)
                    if count and count > 100 then
                        -- 封锁 IP
                        waf_block:set("block:" .. ip, "1", 3600)
                        ngx.log(ngx.ERR, "IP blocked: ", ip)
                    end

                    return ngx.exit(ngx.HTTP_FORBIDDEN)
                end
            }

            proxy_pass http://backend;
        }
    }
}
```

---

## 8. 参考文档

- [OpenResty 官方文档](https://openresty.org/en/)
- [LuaJIT 文档](http://luajit.org/)
- [lua-nginx-module](https://github.com/openresty/lua-nginx-module)
- [lua-resty-core](https://github.com/openresty/lua-resty-core)
- [lua-resty-redis](https://github.com/openresty/lua-resty-redis)
- [lua-resty-mysql](https://github.com/openresty/lua-resty-mysql)
- [lua-resty-http](https://github.com/ledgetech/lua-resty-http)
- [lua-resty-jwt](https://github.com/cdbattags/lua-resty-jwt)
- [Lua 5.1 参考手册](https://www.lua.org/manual/5.1/)
