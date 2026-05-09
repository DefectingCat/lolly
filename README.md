# Lolly

[![Go Version](https://img.shields.io/badge/Go-1.26%2B-00ADD8?style=flat&logo=go)](https://golang.org)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

高性能 HTTP 服务器与反向代理，使用 Go 语言编写。

基于 [fasthttp](https://github.com/valyala/fasthttp) 构建，提供比标准 net/http 更高的性能。支持 HTTP/3 (QUIC)、WebSocket、虚拟主机、多种负载均衡算法、故障转移、代理缓存、Lua 脚本扩展，以及完整的安全与性能优化特性。

> **代码统计**：234 个源文件 | 242 个测试文件 | ~189,000 行 Go 代码 | 测试覆盖率 81.3%

## 特性

### 核心功能

- **静态文件服务** - 零拷贝传输（sendfile）、文件缓存、预压缩支持、try_files 配置、符号链接安全检查、ETag 和 304 Not Modified 支持
- **反向代理** - 完整的代理功能，支持请求头/响应头修改、超时控制、故障转移（next_upstream）、Location/Refresh 头改写
- **HTTP/3 (QUIC)** - 基于 quic-go，支持 0-RTT 连接
- **WebSocket** - 完整的 WebSocket 代理支持
- **虚拟主机** - 单进程支持多域名独立配置，server_name 支持通配符和正则匹配
- **多服务器模式** - 单配置文件支持多个独立 server 实例
- **Location 匹配** - nginx 风格的精确/前缀/正则匹配引擎
- **Unix Socket** - 支持 Unix socket 监听
- **配置引入** - include 指令支持配置拆分
- **TCP/UDP Stream** - 四层代理，支持 MySQL、Redis 等服务
- **Lua 脚本** - 基于 gopher-lua 的可编程扩展，支持 nginx-lua 兼容 API（ngx.var/ngx.ctx/ngx.req/ngx.resp/ngx.timer/ngx.location.capture/ngx.shared.DICT）
- **GeoIP 过滤** - 基于 MaxMind GeoIP2 的国家/地区访问控制
- **自定义错误页面** - 支持为特定状态码配置自定义错误页面
- **nginx 配置导入** - 支持将 nginx 配置文件转换为 lolly YAML 配置

### 负载均衡

| 算法                 | 说明                                                     |
| -------------------- | -------------------------------------------------------- |
| Round Robin          | 轮询，均匀分配                                           |
| Weighted Round Robin | 加权轮询，按权重分配                                     |
| Least Connections    | 最少连接，选择活跃连接最少的目标                         |
| IP Hash              | IP 哈希，同一客户端始终路由到同一目标                    |
| Consistent Hash      | 一致性哈希，支持虚拟节点（默认 150），最小化节点变更影响 |
| Random               | Power of Two Choices，随机选择两个后比较，简单高效       |

### 故障转移

支持 `next_upstream` 配置，当后端返回特定错误状态码（502/503/504）或连接失败时，自动重试下一个可用后端：

```yaml
proxy:
  - path: "/api"
    next_upstream:
      tries: 3
      http_codes: [502, 503, 504]
```

### 安全

- **访问控制** - IP/CIDR 白名单与黑名单，支持 trusted_proxies 配置
- **速率限制** - 令牌桶与滑动窗口算法，支持精确/近似模式
- **连接限制** - 单 IP 并发连接数限制
- **认证** - Basic Auth，支持 bcrypt 与 argon2id；外部 auth_request 子请求
- **安全头部** - HSTS、X-Frame-Options、CSP、Referrer-Policy
- **SSL/TLS** - OCSP Stapling、TLS 1.2/1.3、加密套件配置、Session Tickets 密钥轮换
- **请求体限制** - 可配置全局和路径级别的请求体大小限制

### 性能优化

- **Goroutine 池** - 限制并发 worker 数量，避免 goroutine 爆炸
- **文件缓存** - LRU 淘汰策略，内存上限控制
- **连接池** - 空闲连接复用，减少连接建立开销
- **零拷贝** - 大文件（≥8KB）传输使用 sendfile 系统调用
- **代理缓存** - 支持缓存后端响应，cache_lock 防止缓存击穿
- **PGO 优化** - 支持 Profile-Guided Optimization 构建

### 运维

- **热升级** - USR2 信号触发，零停机升级，支持 Unix Socket 继承
- **配置热重载** - HUP 信号触发，动态更新配置
- **日志轮转** - USR1 信号触发，重新打开日志文件
- **优雅关闭** - QUIT 信号触发，等待请求完成，支持超时配置
- **状态监控** - 内置 `/status` 端点，统计连接数、请求数、流量、上游健康状态、缓存命中率
- **pprof 端点** - 内置性能分析端点，支持 CPU/heap/goroutine/block 分析
- **缓存清理 API** - POST `/purge` 端点，支持按路径清理代理缓存

## 安装

### 构建

```bash
# 克隆仓库
git clone https://github.com/DefectingCat/lolly.git
cd lolly

# 本地构建
make build

# 生产构建（体积优化）
make build-prod

# 性能构建（最大运行时性能）
make build-perf

# PGO 构建（需先收集 profile）
make pgo-collect  # 查看收集指南
make build-pgo

# 跨平台构建
make build-all
```

构建产物位于 `bin/` 目录。

### 运行

```bash
# 使用默认配置
./bin/lolly

# 指定配置文件
./bin/lolly -c /path/to/lolly.yaml

# 生成默认配置
./bin/lolly -g -o lolly.yaml

# 导入 nginx 配置
./bin/lolly --import /etc/nginx/nginx.conf -o lolly.yaml
./bin/lolly -i nginx.conf  # 简写形式

# 显示版本
./bin/lolly -v
```

### nginx 配置导入

lolly 支持将 nginx 配置文件转换为 YAML 格式：

```bash
# 导入 nginx 配置并输出到文件
./bin/lolly --import nginx.conf -o lolly.yaml

# 导入后会显示转换警告（不支持的指令）
```

支持的 nginx 指令：

- `server` 块：listen、server_name、ssl_certificate、ssl_certificate_key
- `location` 块：proxy_pass、root、alias、index、try_files
- `upstream` 块：server（含 weight、max_fails、fail_timeout、backup、down）、least_conn、ip_hash、hash、random
- 其他：gzip、gzip_types、gzip_min_length、client_max_body_size、access_log、error_log、rewrite、return（301/302）、error_page、auth_basic

不支持的指令会在转换时显示警告，需要手动处理。

## 架构

### 请求处理流程

```
                    +------------------+
                    |     Client       |
                    +--------+---------+
                             |
         +-------------------+-------------------+
         |                   |                   |
    +----v----+        +-----v-----+        +----v----+
    | HTTP/1  |        |  HTTP/2   |        | HTTP/3  |
    | (TCP)   |        |  (TLS)    |        | (QUIC)  |
    +----+----+        +-----+-----+        +----+----+
         |                   |                   |
         +-------------------+-------------------+
                             |
                    +--------v---------+
                    |   Middleware     |
                    |     Chain        |
                    +--------+---------+
                             |
         +-------------------+-------------------+
         |                   |                   |
    +----v----+        +-----v-----+        +----v----+
    | Static  |        |   Proxy   |        | Stream  |
    | Handler |        |  Handler  |        | Handler |
    +----+----+        +-----+-----+        +----+----+
         |                   |                   |
    File Cache         Load Balancer        L4 Proxy
         |                   |                   |
    +----+----+        +-----v-----+        +----v----+
    | Disk    |        | Upstream  |        | Backend |
    | I/O     |        | Targets   |        | Servers |
    +---------+        +-----------+        +---------+
```

### 目录结构

```
internal/
├── app/          # 应用入口、信号处理、生命周期
├── config/       # 配置加载、验证、默认值
├── server/       # HTTP 服务器核心
├── handler/      # 请求处理器
├── proxy/        # 反向代理
├── loadbalance/  # 负载均衡算法
├── matcher/      # Location 匹配器
├── middleware/   # 中间件链
├── lua/          # Lua 脚本引擎
├── http2/        # HTTP/2 服务器
├── http3/        # HTTP/3 服务器
├── stream/       # TCP/UDP Stream 代理
├── ssl/          # TLS 配置
├── cache/        # 缓存系统
├── resolver/     # DNS 解析器
├── variable/     # 变量系统
├── logging/      # 日志系统
└── ...           # 其他工具模块
```

### 核心设计

#### 中间件链

请求处理流程：

```
Request → AccessLog → AccessControl → RateLimiter → Auth → AuthRequest → BodyLimit →
Rewrite → Compression → SecurityHeaders → ErrorIntercept → Handler
```

每个中间件实现 `Middleware` 接口：

```go
type Middleware interface {
    Name() string
    Process(next fasthttp.RequestHandler) fasthttp.RequestHandler
}
```

#### 负载均衡

所有负载均衡器实现 `Balancer` 接口，支持健康目标过滤和故障转移排除：

```go
type Balancer interface {
    Select(targets []*Target) *Target
    SelectExcluding(targets []*Target, excluded []*Target) *Target
}
```

#### 代理缓存

支持：

- **缓存锁**（cache_lock）- 防止缓存击穿，同一缓存键只允许一个请求访问后端
- **过期复用**（stale-while-revalidate）- 允许在后台刷新时返回过期缓存
- **后台刷新** - 异步更新缓存，不影响请求响应时间
- **错误回退**（stale-if-error）- 后端错误时返回过期缓存
- **超时回退**（stale-if-timeout）- 后端超时时返回过期缓存

## Lua 脚本

Lolly 提供基于 gopher-lua 的 Lua 脚本扩展能力，支持 nginx-lua 兼容 API。

### 执行阶段

```
请求到达 → rewrite → access → content → header_filter → body_filter → log → 响应返回
```

| 阶段          | 说明       | 可用操作                              |
| ------------- | ---------- | ------------------------------------- |
| rewrite       | URL 重写   | ngx.req.set_uri, ngx.req.set_uri_args |
| access        | 访问控制   | ngx.exit, ngx.req.get_headers         |
| content       | 内容生成   | ngx.say, ngx.print, ngx.exit          |
| header_filter | 响应头修改 | ngx.header[key] = value               |
| body_filter   | 响应体修改 | ngx.arg[1] 操作响应体                 |
| log           | 日志记录   | ngx.log, 记录请求信息                 |

### 支持的 API

```lua
-- 日志
ngx.log(ngx.ERR, "error message")
ngx.log(ngx.WARN, "warning message")
ngx.log(ngx.INFO, "info message")

-- HTTP 状态码常量
ngx.HTTP_OK = 200
ngx.HTTP_NOT_FOUND = 404
ngx.HTTP_INTERNAL_SERVER_ERROR = 500
-- 更多: HTTP_MOVED_PERMANENTLY, HTTP_BAD_REQUEST, HTTP_FORBIDDEN 等

-- 特殊常量
ngx.OK, ngx.ERROR, ngx.AGAIN, ngx.DONE, ngx.DECLINED

-- 内置变量（ngx.var）
local uri = ngx.var.uri
local method = ngx.var.request_method
local remote_addr = ngx.var.remote_addr
local host = ngx.var.host
local request_uri = ngx.var.request_uri
local scheme = ngx.var.scheme
local server_port = ngx.var.server_port
-- 任意请求头: ngx.var.http_xxx（如 ngx.var.http_user_agent）
-- 任意查询参数: ngx.var.arg_xxx（如 ngx.var.arg_token）

-- 请求操作（ngx.req）
ngx.req.get_method()          -- 获取 HTTP 方法
ngx.req.get_uri()             -- 获取 URI 路径
ngx.req.set_uri("/new_path")  -- 设置 URI
ngx.req.set_uri_args("a=1&b=2") -- 设置查询参数
ngx.req.get_uri_args()        -- 获取查询参数表
ngx.req.get_headers(max?)     -- 获取请求头表
ngx.req.set_header("X-Custom", "value")
ngx.req.clear_header("X-Custom")
ngx.req.get_body_data()       -- 获取请求体
ngx.req.read_body()           -- 确保请求体已读取

-- 响应操作（ngx.resp）
ngx.resp.get_status()         -- 获取响应状态码
ngx.resp.set_status(200)      -- 设置响应状态码
ngx.resp.get_headers(max?)    -- 获取响应头表
ngx.resp.set_header("X-Response", "value")
ngx.resp.clear_header("X-Response")

-- 快捷响应
ngx.say("Hello World")        -- 输出内容并换行
ngx.print("Hello World")      -- 输出内容不换行
ngx.flush(wait?)              -- 刷新响应缓冲区
ngx.redirect("/new", 302)     -- 重定向
ngx.exit(200)                 -- 结束请求处理

-- 请求上下文（ngx.ctx，每请求独立的 table）
ngx.ctx.user_id = 123
local id = ngx.ctx.user_id

-- 共享字典（ngx.shared.DICT）
local dict = ngx.shared.DICT
dict:set("key", "value", 3600)   -- 设置（含过期时间）
dict:add("key", "value", 3600)   -- 仅键不存在时添加
dict:replace("key", "value")     -- 仅键存在时替换
local val = dict:get("key")      -- 获取值
dict:incr("counter", 1)          -- 数值自增
dict:delete("key")               -- 删除键
dict:flush_all()                 -- 清空所有条目
dict:flush_expired(max?)         -- 清除过期条目
dict:get_keys(max?)              -- 获取所有键
dict:size()                      -- 获取条目数
dict:free_space()                -- 获取剩余容量
-- 元表语法: dict["key"] = value / val = dict["key"]

-- Socket（ngx.socket.tcp，受限）
local sock = ngx.socket.tcp()
sock:connect("127.0.0.1", 8080)
sock:settimeout(5000)            -- 设置超时（毫秒）
sock:settimeouts(1000, 2000, 3000) -- 分别设置连接/发送/读取超时
sock:send("GET / HTTP/1.0\r\n\r\n")
local response = sock:receive("*a") -- 读取全部
local line = sock:receive("*l")     -- 读取一行
local reader = sock:receiveuntil("--boundary") -- 按模式读取
sock:close()

-- 定时器（ngx.timer）
local timer_id = ngx.timer.at(5, function()
    ngx.log(ngx.INFO, "timer fired")
end)
ngx.timer.running_count() -- 获取运行中的定时器数量
-- handle:cancel()        -- 取消定时器（通过返回的 handle）

-- 子请求（ngx.location.capture）
local res = ngx.location.capture("/internal/auth", {
    method = ngx.HTTP_GET,
    body = "request body",
    args = "a=1&b=2"
})
-- res.status, res.body, res.headers

-- 动态负载均衡（ngx.balancer）
ngx.balancer.set_current_peer("backend:8080") -- 设置后端地址
ngx.balancer.set_more_tries(3)                -- 设置重试次数
local fail_type = ngx.balancer.get_last_failure() -- 获取上次失败类型
local targets = ngx.balancer.get_targets()    -- 获取所有可用目标
local client_ip = ngx.balancer.get_client_ip() -- 获取客户端 IP
```

### 示例脚本

**rewrite.lua** - URL 重写：

```lua
-- 将 /api/v1/* 重写为 /v1/*
local uri = ngx.var.uri
if string.match(uri, "^/api/v1/") then
    local new_uri = string.gsub(uri, "^/api/", "")
    ngx.req.set_uri(new_uri)
end
```

**access.lua** - 访问控制：

```lua
-- 检查请求头中的 token
local token = ngx.req.get_headers()["X-Token"]
if not token or token == "" then
    ngx.exit(401)
    return
end

-- 验证 token（示例）
if token ~= "valid-token" then
    ngx.exit(403)
    return
end
```

**content.lua** - 动态响应：

```lua
-- 生成 JSON 响应
ngx.header["Content-Type"] = "application/json"
local response = {
    status = "ok",
    timestamp = os.time(),
    request_id = ngx.var.request_id
}
ngx.say(require("cjson").encode(response))
ngx.exit(200)
```

### 安全限制

默认沙箱模式禁用以下 Lua 库：

- `os`（除 os.time）
- `io`
- `file`

可通过配置 `lua.sandbox: false` 启用完整库访问（生产环境不建议）。

### 动态负载均衡

使用 `balancer_by_lua` 实现动态后端选择：

```lua
-- 根据请求路径选择不同后端
local uri = ngx.var.uri
if string.match(uri, "^/api/") then
    ngx.balancer.set_current_peer("api-backend", 8080)
elseif string.match(uri, "^/static/") then
    ngx.balancer.set_current_peer("static-backend", 8080)
else
    ngx.balancer.set_current_peer("default-backend", 8080)
end
```

## 部署

### Docker

```bash
# 构建镜像
docker build -t lolly:latest .

# 运行容器
docker run -d \
  --name lolly \
  -p 80:80 \
  -p 443:443 \
  -p 443:443/udp \
  -v /etc/lolly:/etc/lolly:ro \
  -v /var/www:/var/www:ro \
  -v /var/log/lolly:/var/log/lolly \
  lolly:latest
```

### Docker Compose

```yaml
version: "3"
services:
  lolly:
    image: lolly:latest
    ports:
      - "80:80"
      - "443:443"
      - "443:443/udp"
    volumes:
      - ./lolly.yaml:/etc/lolly/lolly.yaml:ro
      - ./html:/var/www/html:ro
      - ./logs:/var/log/lolly
    restart: unless-stopped
```

## 监控

### 状态端点

访问 `/status`（需配置 `monitoring.status.allow` 白名单），返回服务器状态、连接池、上游健康、缓存命中等信息。

### pprof 端点

启用 `monitoring.pprof.enabled: true` 后访问：

- `/debug/pprof/` - pprof 索引
- `/debug/pprof/profile?seconds=30` - CPU profile
- `/debug/pprof/heap` - 内存分配
- `/debug/pprof/goroutine` - Goroutine 分析
- `/debug/pprof/block` - 阻塞分析

## 信号处理

| 信号            | 行为                   |
| --------------- | ---------------------- |
| SIGTERM, SIGINT | 快速停止               |
| SIGQUIT         | 优雅停止，等待请求完成 |
| SIGHUP          | 重载配置               |
| SIGUSR1         | 重新打开日志文件       |
| SIGUSR2         | 热升级                 |

## 性能

基于 fasthttp，相比标准 net/http 有显著性能提升：

- 避免不必要的内存分配（零分配设计）
- 优化的事件循环
- 高效的连接池管理
- 零拷贝传输（sendfile）
- Goroutine 池复用

### 基准测试

在 4 核 8GB 服务器上测试（使用 `internal/benchmark/tools/loadgen`）：

| 场景               | RPS         | 平均延迟 | P99 延迟 |
| ------------------ | ----------- | -------- | -------- |
| 静态文件（1KB）    | 120,000+    | 0.8ms    | 3ms      |
| 静态文件（100KB）  | 15,000+     | 6ms      | 25ms     |
| 反向代理（无缓存） | 80,000+     | 1.2ms    | 5ms      |
| 反向代理（有缓存） | 150,000+    | 0.6ms    | 2ms      |
| WebSocket 连接     | 50,000 连接 | -        | -        |

### PGO 优化

支持 Profile-Guided Optimization 构建，可获得额外 5-15% 性能提升：

```bash
# 1. 启用 pprof 端点
# 2. 运行代表性工作负载
# 3. 收集 CPU profile
curl http://localhost:8080/debug/pprof/profile?seconds=30 > default.pgo

# 4. 使用 PGO 构建
make build-pgo
```

### 生产环境推荐配置

```yaml
performance:
  goroutine_pool:
    enabled: true
    max_workers: 10000 # 根据 CPU 核数调整
    min_workers: 100
    idle_timeout: 60s
  file_cache:
    max_entries: 50000 # 根据内存调整
    max_size: 268435456 # 256MB
  transport:
    idle_conn_timeout: 90s
    max_conns_per_host: 500 # 根据后端容量调整
```

容量规划建议：

| 流量级别               | max_workers | file_cache.max_size | max_conns_per_host |
| ---------------------- | ----------- | ------------------- | ------------------ |
| 低流量（<10K RPS）     | 500         | 64MB                | 100                |
| 中流量（10K-100K RPS） | 5000        | 256MB               | 300                |
| 高流量（>100K RPS）    | 10000       | 512MB               | 500                |

## 故障排除

### 调试模式

启用详细日志：

```yaml
logging:
  error:
    level: "debug"
  access:
    format: "combined"
```

查看实时日志：

```bash
# 查看错误日志
tail -f /var/log/lolly/error.log | jq .

# 查看访问日志
tail -f /var/log/lolly/access.log
```

### 健康检查

```bash
# 检查状态端点
curl http://localhost:8080/status | jq .

# 检查后端健康
curl http://localhost:8080/status | jq '.upstreams'

# 检查缓存命中率
curl http://localhost:8080/status | jq '.cache'
```

## 开发

### 环境要求

- Go 1.26+
- make

### 命令

```bash
# 运行测试
make test

# 测试覆盖率
make test-cover

# 基准测试
make bench

# 基准测试（统计模式）
make bench-stat

# 对比基准结果
make bench-compare

# 代码检查
make check

# 格式化
make fmt

# 静态分析
make lint
```

### 项目统计

- Go 源文件：234 个
- 测试文件：242 个
- 总代码行数：~189,000 行
- 测试覆盖率：81.3%
- 中文代码注释
- 所有核心模块均有完整单元测试和性能基准测试

## 依赖

| 库                                                                       | 版本    | 用途             |
| ------------------------------------------------------------------------ | ------- | ---------------- |
| [fasthttp](https://github.com/valyala/fasthttp)                          | v1.70.0 | HTTP 服务器核心  |
| [fasthttp/router](https://github.com/fasthttp/router)                    | v1.5.4  | HTTP 路由器      |
| [quic-go](https://github.com/quic-go/quic-go)                            | v0.59.0 | QUIC/HTTP/3 实现 |
| [zerolog](https://github.com/rs/zerolog)                                 | v1.35.1 | 零分配 JSON 日志 |
| [gopher-lua](https://github.com/yuin/gopher-lua)                         | v1.1.2  | Lua 脚本引擎     |
| [klauspost/compress](https://github.com/klauspost/compress)              | v1.18.5 | Gzip/Brotli 压缩 |
| [brotli](https://github.com/andybalholm/brotli)                          | v1.2.1  | Brotli 压缩编码  |
| [geoip2-golang](https://github.com/oschwald/geoip2-golang)               | v1.13.0 | GeoIP 查询       |
| [golang-lru/v2](https://github.com/hashicorp/golang-lru)                 | v2.0.7  | LRU 缓存         |
| [uuid](https://github.com/google/uuid)                                   | v1.6.0  | UUID 生成        |
| [testcontainers-go](https://github.com/testcontainers/testcontainers-go) | v0.42.0 | 集成测试容器     |
| [testify](https://github.com/stretchr/testify)                           | v1.11.1 | 测试断言         |
| [golang.org/x/crypto](https://golang.org/x/crypto)                       | v0.50.0 | 加密工具         |
| [golang.org/x/net](https://golang.org/x/net)                             | v0.53.0 | 网络扩展         |
| [yaml.v3](https://gopkg.in/yaml.v3)                                      | v3.0.1  | YAML 配置解析    |

## 许可证

MIT License

## 作者

xfy

