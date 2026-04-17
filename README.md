# Lolly

[![Go Version](https://img.shields.io/badge/Go-1.26%2B-00ADD8?style=flat&logo=go)](https://golang.org)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

高性能 HTTP 服务器与反向代理，使用 Go 语言编写。

基于 [fasthttp](https://github.com/valyala/fasthttp) 构建，提供比标准 net/http 更高的性能。支持 HTTP/3 (QUIC)、WebSocket、虚拟主机、多种负载均衡算法、故障转移，以及完整的安全与性能优化特性。

## 特性

### 核心功能

- **静态文件服务** - 零拷贝传输（sendfile）、文件缓存、预压缩支持、try_files 配置、符号链接安全检查
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

### 负载均衡

| 算法 | 说明 |
|------|------|
| Round Robin | 轮询，均匀分配 |
| Weighted Round Robin | 加权轮询，按权重分配 |
| Least Connections | 最少连接，选择活跃连接最少的目标 |
| IP Hash | IP 哈希，同一客户端始终路由到同一目标 |
| Consistent Hash | 一致性哈希，支持虚拟节点（默认 150），最小化节点变更影响 |

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
- **GeoIP 过滤** - 基于 MaxMind GeoIP2 数据库的国家/地区访问控制
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

- **热升级** - USR2 信号触发，零停机升级
- **配置热重载** - HUP 信号触发，动态更新配置
- **日志轮转** - USR1 信号触发，重新打开日志文件
- **优雅关闭** - QUIT 信号触发，等待请求完成
- **状态监控** - 内置状态端点，统计连接数、请求数、流量
- **pprof 端点** - 内置性能分析端点，支持 PGO 优化

## 安装

### 构建

```bash
# 克隆仓库
git clone https://github.com/xfy/lolly.git
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
./bin/lolly --generate-config -o lolly.yaml

# 显示版本
./bin/lolly -v
```

## 配置

配置文件使用 YAML 格式。以下是完整配置示例：

```yaml
server:
  listen: ":8080"
  name: "example.com"
  read_timeout: 30s
  write_timeout: 30s
  idle_timeout: 120s
  max_conns_per_ip: 100
  max_requests_per_conn: 1000
  client_max_body_size: "10MB"

  static:
    - path: "/"
      root: "/var/www/html"
      index: ["index.html", "index.htm"]
      try_files: ["$uri", "$uri/", "/index.html"]
      try_files_pass: false
    - path: "/assets/"
      root: "/var/www/assets"

  proxy:
    - path: "/api"
      targets:
        - url: "http://backend1:8080"
          weight: 2
        - url: "http://backend2:8080"
          weight: 1
      load_balance: "weighted_round_robin"
      health_check:
        interval: 10s
        path: "/health"
        timeout: 5s
      next_upstream:
        tries: 3
        http_codes: [502, 503, 504]
      timeout:
        connect: 5s
        read: 30s
        write: 30s
      headers:
        set_request:
          X-Forwarded-For: "$remote_addr"
          X-Real-IP: "$remote_addr"
        set_response:
          X-Proxy-By: "lolly"
      cache:
        enabled: true
        max_age: 5m
        cache_lock: true
        stale_while_revalidate: 1m
      client_max_body_size: "50MB"

  ssl:
    cert: "/etc/ssl/certs/server.crt"
    key: "/etc/ssl/private/server.key"
    cert_chain: "/etc/ssl/certs/chain.crt"
    protocols: ["TLSv1.2", "TLSv1.3"]
    ocsp_stapling: true
    hsts:
      max_age: 31536000
      include_sub_domains: true
      preload: false

  security:
    access:
      allow: ["192.168.1.0/24", "10.0.0.0/8"]
      deny: []
      default: "deny"
      trusted_proxies: ["172.16.0.0/16"]
    geoip:
      database: "/var/lib/GeoIP/GeoIP2-Country.mmdb"
      allow_countries: ["CN", "US"]
      deny_countries: []
      default: "deny"
    rate_limit:
      request_rate: 100
      burst: 200
      conn_limit: 50
      algorithm: "token_bucket"
    auth:
      type: "basic"
      require_tls: true
      algorithm: "bcrypt"
      realm: "Secure Area"
      users:
        - name: "admin"
          password: "$2y$10$N9qo8uLOickgx2ZMRZoMy..."
    headers:
      x_frame_options: "DENY"
      x_content_type_options: "nosniff"
      content_security_policy: "default-src 'self'"
      referrer_policy: "strict-origin-when-cross-origin"
    error_page:
      pages:
        404: "/var/www/errors/404.html"
        500: "/var/www/errors/500.html"
      default: "/var/www/errors/error.html"

  rewrite:
    - pattern: "^/old/(.*)$"
      replacement: "/new/$1"
      flag: "permanent"

  compression:
    type: "gzip"
    level: 6
    min_size: 1024
    types: ["text/html", "text/css", "application/javascript", "application/json"]
    gzip_static: true
    gzip_static_extensions: [".gz", ".br"]

  lua:
    enabled: true
    package_path: "/etc/lolly/lua/?.lua"
    code_cache:
      enabled: true
      ttl: 60s
    max_concurrent: 1000
    timeout: 30s
    phases:
      rewrite: "/etc/lolly/lua/rewrite.lua"
      access: "/etc/lolly/lua/access.lua"
      content: "/etc/lolly/lua/content.lua"
      log: "/etc/lolly/lua/log.lua"

http3:
  enabled: true
  listen: ":443"
  max_streams: 1000
  idle_timeout: 30s
  enable_0rtt: true

stream:
  - listen: ":3306"
    protocol: "tcp"
    upstream:
      targets:
        - addr: "mysql1:3306"
          weight: 3
        - addr: "mysql2:3306"
          weight: 1
      load_balance: "round_robin"

logging:
  format: "json"
  access:
    path: "/var/log/lolly/access.log"
    format: "combined"
  error:
    path: "/var/log/lolly/error.log"
    level: "info"

performance:
  goroutine_pool:
    enabled: true
    max_workers: 10000
    min_workers: 100
    idle_timeout: 60s
  file_cache:
    max_entries: 50000
    max_size: 268435456  # 256MB
    inactive: 60s
  transport:
    idle_conn_timeout: 90s
    max_conns_per_host: 500

monitoring:
  status:
    path: "/status"
    allow: ["127.0.0.1", "10.0.0.0/8"]
  pprof:
    enabled: false
    path: "/debug/pprof"
    allow: ["127.0.0.1"]
```

完整配置说明请参考 `config.example.yaml` 和源码 `internal/config/config.go`。

## 架构

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
├── app/                    # 应用入口、信号处理、生命周期
│   └── app.go              # 主程序逻辑
├── config/                 # 配置加载、验证、默认值
│   ├── config.go           # 配置结构定义
│   ├── defaults.go         # 默认配置
│   └── validate.go         # 配置验证
├── server/                 # HTTP 服务器核心
│   ├── server.go           # 服务器实现
│   ├── vhost.go            # 虚拟主机管理
│   ├── pool.go             # Goroutine 池
│   ├── status.go           # 状态端点
│   ├── pprof.go            # pprof 端点
│   └── upgrade.go          # 热升级管理
├── handler/                # 请求处理器
│   ├── router.go           # 路由器
│   ├── static.go           # 静态文件处理
│   ├── sendfile.go         # 零拷贝传输
│   └── errorpage.go        # 错误页面管理
├── proxy/                  # 反向代理
│   ├── proxy.go            # 代理核心逻辑
│   ├── websocket.go        # WebSocket 代理
│   └── health.go           # 健康检查
├── loadbalance/            # 负载均衡算法
│   ├── balancer.go         # 算法实现
│   └── consistent_hash.go  # 一致性哈希
├── middleware/             # 中间件链
│   ├── middleware.go       # 中间件接口
│   ├── compression/        # Gzip/Brotli 压缩
│   ├── security/           # 访问控制、限流、认证、安全头部
│   ├── rewrite/            # URL 重写
│   ├── accesslog/          # 访问日志
│   ├── bodylimit/          # 请求体大小限制
│   └── errorintercept/     # 错误页面拦截
├── lua/                    # Lua 脚本引擎
│   ├── engine.go           # gopher-lua 引擎
│   ├── context.go          # Lua 请求上下文
│   └── api_*.go            # nginx-lua 兼容 API
├── http3/                  # HTTP/3 服务器
│   ├── server.go           # QUIC 服务器
│   └── adapter.go          # HTTP/3 适配器
├── stream/                 # TCP/UDP Stream 代理
│   └── stream.go           # 四层代理实现
├── ssl/                    # TLS 配置
│   ├── ssl.go              # TLS 管理器
│   ├── ocsp.go             # OCSP Stapling
│   └── session_tickets.go  # Session Tickets 密钥轮换
├── cache/                  # 缓存系统
│   ├── file_cache.go       # 文件缓存
│   └── purge.go            # 代理缓存
├── resolver/               # DNS 解析器
│   └── resolver.go         # TTL 缓存、异步解析
├── variable/               # 变量系统
│   └── variable.go         # 日志/头部变量
├── logging/                # 日志系统
│   └── logging.go          # 结构化日志（zerolog）
├── netutil/                # 网络工具
│   ├── ip.go               # IP 解析
│   └── url.go              # URL 处理
└── benchmark/              # 基准测试工具
    └── tools/              # 测试辅助工具
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

## Lua 脚本

Lolly 提供基于 gopher-lua 的 Lua 脚本扩展能力，支持 nginx-lua 兼容 API。

### 执行阶段

```
请求到达 → rewrite → access → content → header_filter → body_filter → log → 响应返回
```

| 阶段 | 说明 | 可用操作 |
|------|------|----------|
| rewrite | URL 重写 | ngx.req.set_uri, ngx.req.set_uri_args |
| access | 访问控制 | ngx.exit, ngx.req.get_headers |
| content | 内容生成 | ngx.say, ngx.print, ngx.exit |
| header_filter | 响应头修改 | ngx.header[key] = value |
| body_filter | 响应体修改 | ngx.arg[1] 操作响应体 |
| log | 日志记录 | ngx.log, 记录请求信息 |

### 支持的 API

```lua
-- 日志
ngx.log(ngx.ERR, "error message")
ngx.log(ngx.WARN, "warning message")
ngx.log(ngx.INFO, "info message")

-- 请求操作
local uri = ngx.var.uri
local args = ngx.req.get_uri_args()
ngx.req.set_uri("/new_path")
ngx.req.set_header("X-Custom", "value")

-- 响应操作
ngx.header["X-Response"] = "value"
ngx.say("Hello World")
ngx.exit(200)

-- 共享字典
local dict = ngx.shared.DICT
dict:set("key", "value", 3600)
local val = dict:get("key")

-- Socket（受限）
local sock = ngx.socket.tcp()
sock:connect("127.0.0.1", 8080)
sock:send("GET / HTTP/1.0\r\n\r\n")
local response = sock:receive("*a")
sock:close()
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
version: '3'
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

### systemd

创建 `/etc/systemd/system/lolly.service`：

```ini
[Unit]
Description=Lolly HTTP Server
After=network.target

[Service]
Type=simple
User=lolly
Group=lolly
ExecStart=/usr/local/bin/lolly -c /etc/lolly/lolly.yaml
ExecReload=/bin/kill -HUP $MAINPID
Restart=on-failure
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
```

```bash
# 安装服务
sudo systemctl daemon-reload
sudo systemctl enable lolly
sudo systemctl start lolly

# 管理服务
sudo systemctl reload lolly   # 重载配置
sudo systemctl restart lolly  # 重启服务
sudo systemctl status lolly   # 查看状态
```

## 监控

### 状态端点

访问 `/status`（需配置 `monitoring.status.allow` 白名单）：

```json
{
  "connections": {
    "active": 150,
    "reading": 2,
    "writing": 10,
    "idle": 138
  },
  "requests": {
    "total": 125000,
    "current": 150,
    "per_second": 1250
  },
  "traffic": {
    "in_bytes": 1073741824,
    "out_bytes": 5368709120
  },
  "upstreams": {
    "backend1": {
      "active": 50,
      "healthy": true,
      "total_requests": 80000
    },
    "backend2": {
      "active": 30,
      "healthy": true,
      "total_requests": 45000
    }
  },
  "cache": {
    "file_cache": {
      "entries": 5000,
      "size_bytes": 134217728,
      "hits": 85000,
      "misses": 15000
    },
    "proxy_cache": {
      "entries": 2000,
      "hits": 70000,
      "misses": 10000
    }
  }
}
```

### pprof 端点

启用 `monitoring.pprof.enabled: true` 后访问：

- `/debug/pprof/` - pprof 索引
- `/debug/pprof/profile?seconds=30` - CPU profile
- `/debug/pprof/heap` - 内存分配
- `/debug/pprof/goroutine` - Goroutine 分析
- `/debug/pprof/block` - 阻塞分析

## 信号处理

| 信号 | 行为 |
|------|------|
| SIGTERM, SIGINT | 快速停止 |
| SIGQUIT | 优雅停止，等待请求完成 |
| SIGHUP | 重载配置 |
| SIGUSR1 | 重新打开日志文件 |
| SIGUSR2 | 热升级 |

## 性能

基于 fasthttp，相比标准 net/http 有显著性能提升：

- 避免不必要的内存分配（零分配设计）
- 优化的事件循环
- 高效的连接池管理
- 零拷贝传输（sendfile）
- Goroutine 池复用

### 基准测试

在 4 核 8GB 服务器上测试（使用 `internal/benchmark/tools/loadgen`）：

| 场景 | RPS | 平均延迟 | P99 延迟 |
|------|-----|----------|----------|
| 静态文件（1KB） | 120,000+ | 0.8ms | 3ms |
| 静态文件（100KB） | 15,000+ | 6ms | 25ms |
| 反向代理（无缓存） | 80,000+ | 1.2ms | 5ms |
| 反向代理（有缓存） | 150,000+ | 0.6ms | 2ms |
| WebSocket 连接 | 50,000 连接 | - | - |

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
    max_workers: 10000   # 根据 CPU 核数调整
    min_workers: 100
    idle_timeout: 60s
  file_cache:
    max_entries: 50000   # 根据内存调整
    max_size: 268435456  # 256MB
  transport:
    idle_conn_timeout: 90s
    max_conns_per_host: 500  # 根据后端容量调整
```

容量规划建议：

| 流量级别 | max_workers | file_cache.max_size | max_conns_per_host |
|----------|-------------|---------------------|--------------------|
| 低流量（<10K RPS） | 500 | 64MB | 100 |
| 中流量（10K-100K RPS） | 5000 | 256MB | 300 |
| 高流量（>100K RPS） | 10000 | 512MB | 500 |

## 与 NGINX 对比

| 特性 | Lolly | NGINX |
|------|-------|-------|
| HTTP/3 | 支持（quic-go） | 1.25+ 支持 |
| HTTP/2 | 支持（需 TLS） | 支持 |
| 配置格式 | YAML | 自定义 DSL |
| 热升级 | 支持 | 支持 |
| 扩展方式 | Go 代码/Lua | C 模块/Lua |
| 部署方式 | 单二进制 | 需安装 |
| TCP/UDP Stream | 支持 | 支持 |
| 代理缓存 | 支持（带锁） | 支持 |
| GeoIP | MaxMind GeoIP2 | GeoIP 模块 |
| 一致性哈希 | 支持（内置） | 需第三方模块 |

**Lolly 优势**：
- YAML 配置更易读、易于解析和生成
- 单二进制部署，无依赖
- Go 原生扩展，无需 C 编译环境
- 现代 Go 库生态系统

**NGINX 优势**：
- 经过大规模生产验证
- 丰富的第三方模块生态
- 更完整的功能集（邮件代理等）

### 从 NGINX 迁移

常见配置转换示例：

| NGINX | Lolly |
|-------|-------|
| `listen 80;` | `listen: ":80"` |
| `root /var/www/html;` | `root: "/var/www/html"` |
| `proxy_pass http://backend;` | `url: "http://backend"` |
| `proxy_set_header X-Real-IP $remote_addr;` | `set_request: { X-Real-IP: "$remote_addr" }` |
| `limit_req zone=one burst=10;` | `rate_limit: { request_rate: 100, burst: 10 }` |
| `ssl_certificate /path/cert.pem;` | `cert: "/path/cert.pem"` |

迁移注意事项：
1. 变量系统语法相同（$remote_addr 等）
2. try_files 语法兼容
3. Lua API 与 OpenResty 高度兼容
4. 健康检查配置略有差异

## 生产清单

### 安全加固

- [ ] 启用 TLS 1.2+，禁用 TLS 1.0/1.1
- [ ] 配置安全头部（HSTS、X-Frame-Options、CSP）
- [ ] 启用访问控制，限制管理端点访问
- [ ] 配置速率限制，防止 DDoS
- [ ] 启用请求体大小限制
- [ ] 配置 GeoIP 过滤（如需地区限制）
- [ ] 定期更新 TLS Session Tickets 密钥

### 配置验证

- [ ] 检查 `ssl.protocols` 仅包含 TLSv1.2/1.3
- [ ] 检查 `security.headers` 配置完整
- [ ] 检查 `monitoring.status.allow` 仅允许可信 IP
- [ ] 检查 `max_conns_per_ip` 合理限制
- [ ] 检查 `client_max_body_size` 符合业务需求

### 性能优化

- [ ] 启用 Goroutine 池
- [ ] 配置文件缓存
- [ ] 启用代理缓存（如适用）
- [ ] 配置连接池参数
- [ ] 调整文件缓存大小匹配可用内存

### 运维准备

- [ ] 配置日志轮转
- [ ] 测试热升级流程
- [ ] 测试优雅关闭超时
- [ ] 配置 systemd 服务
- [ ] 设置监控告警阈值

## 故障排除

### 常见问题

**启动失败：端口被占用**
```bash
# 检查端口占用
sudo lsof -i :80
sudo lsof -i :443

# 或更改监听端口
listen: ":8080"
```

**TLS 证书加载失败**
```bash
# 检查证书文件权限
ls -la /etc/ssl/certs/server.crt
ls -la /etc/ssl/private/server.key

# 确保证书格式正确（PEM）
openssl x509 -in /etc/ssl/certs/server.crt -text -noout
```

**代理超时**
```yaml
# 增加超时时间
timeout:
  connect: 10s
  read: 60s
  write: 60s
```

**内存占用过高**
```yaml
# 减少缓存大小
file_cache:
  max_entries: 10000
  max_size: 67108864  # 64MB
```

**Lua 脚本超时**
```yaml
# 增加执行超时
lua:
  timeout: 60s
  max_concurrent: 500
```

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

- Go 文件：110
- 测试文件：113
- 核心模块均有完整测试和性能基准测试
- 中文代码注释

## 依赖

| 库 | 版本 | 用途 |
|------|---------|------|
| [fasthttp](https://github.com/valyala/fasthttp) | v1.70.0 | HTTP 服务器核心 |
| [quic-go](https://github.com/quic-go/quic-go) | v0.59.0 | QUIC/HTTP/3 实现 |
| [zerolog](https://github.com/rs/zerolog) | v1.35.0 | 零分配 JSON 日志 |
| [gopher-lua](https://github.com/yuin/gopher-lua) | v1.1.2 | Lua 脚本引擎 |
| [klauspost/compress](https://github.com/klauspost/compress) | v1.18.5 | Gzip/Brotli 压缩 |
| [geoip2-golang](https://github.com/oschwald/geoip2-golang) | v1.13.0 | GeoIP 查询 |

## 许可证

MIT License

## 作者

xfy