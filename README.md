# Lolly

[![Go Version](https://img.shields.io/badge/Go-1.26%2B-00ADD8?style=flat&logo=go)](https://golang.org)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

高性能 HTTP 服务器与反向代理，使用 Go 语言编写。

基于 [fasthttp](https://github.com/valyala/fasthttp) 构建，提供比标准 net/http 更高的性能。支持 HTTP/3 (QUIC)、WebSocket、虚拟主机、多种负载均衡算法、故障转移，以及完整的安全与性能优化特性。

## 特性

### 核心功能

- **静态文件服务** - 零拷贝传输（sendfile）、文件缓存、预压缩支持、try_files 配置
- **反向代理** - 完整的代理功能，支持请求头/响应头修改、超时控制、故障转移（next_upstream）
- **HTTP/3 (QUIC)** - 基于 quic-go，支持 0-RTT 连接
- **WebSocket** - 完整的 WebSocket 代理支持
- **虚拟主机** - 单进程支持多域名独立配置
- **TCP/UDP Stream** - 四层代理，支持 MySQL、Redis 等服务
- **自定义错误页面** - 支持为特定状态码配置自定义错误页面

### 负载均衡

| 算法 | 说明 |
|------|------|
| Round Robin | 轮询，均匀分配 |
| Weighted Round Robin | 加权轮询，按权重分配 |
| Least Connections | 最少连接，选择活跃连接最少的目标 |
| IP Hash | IP 哈希，同一客户端始终路由到同一目标 |
| Consistent Hash | 一致性哈希，支持虚拟节点，最小化节点变更影响 |

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

- **访问控制** - IP/CIDR 白名单与黑名单
- **速率限制** - 令牌桶与滑动窗口算法
- **连接限制** - 单 IP 并发连接数限制
- **认证** - Basic Auth，支持 bcrypt 与 argon2id
- **安全头部** - HSTS、X-Frame-Options、CSP、Referrer-Policy
- **SSL/TLS** - OCSP Stapling、TLS 1.2/1.3、加密套件配置
- **请求体限制** - 可配置全局和路径级别的请求体大小限制

### 性能优化

- **Goroutine 池** - 限制并发 worker 数量，避免 goroutine 爆炸
- **文件缓存** - LRU 淘汰策略，内存上限控制
- **连接池** - 空闲连接复用，减少连接建立开销
- **零拷贝** - 大文件传输使用 sendfile 系统调用
- **代理缓存** - 支持缓存后端响应，防止缓存击穿
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
    max_idle_conns_per_host: 100
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

完整配置说明请参考源码 `internal/config/config.go`。

## 架构

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
├── http3/                  # HTTP/3 服务器
│   ├── server.go           # QUIC 服务器
│   └── adapter.go          # HTTP/3 适配器
├── stream/                 # TCP/UDP Stream 代理
│   └── stream.go           # 四层代理实现
├── ssl/                    # TLS 配置
│   ├── ssl.go              # TLS 管理器
│   └── ocsp.go             # OCSP Stapling
├── cache/                  # 缓存系统
│   └── file_cache.go       # 文件缓存、代理缓存
├── logging/                # 日志系统
│   └── logging.go          # 结构化日志
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
Request → AccessLog → AccessControl → RateLimiter → Auth → BodyLimit → Rewrite → Compression → SecurityHeaders → ErrorIntercept → Handler
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
- 缓存锁（防止缓存击穿）
- 过期缓存复用（stale-while-revalidate）
- 后台刷新

## 信号处理

| 信号 | 行为 |
|------|------|
| SIGTERM, SIGINT | 快速停止 |
| SIGQUIT | 优雅停止，等待请求完成 |
| SIGHUP | 重载配置 |
| SIGUSR1 | 重新打开日志文件 |
| SIGUSR2 | 热升级 |

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

- Go 文件：85+
- 提交次数：75+
- 测试覆盖：各核心模块均有完整测试

## 性能

基于 fasthttp，相比标准 net/http 有显著性能提升：

- 避免不必要的内存分配
- 优化的事件循环
- 高效的连接池管理
- 零拷贝传输
- Goroutine 池复用

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

建议生产环境配置：

```yaml
performance:
  goroutine_pool:
    enabled: true
    max_workers: 10000
    min_workers: 100
    idle_timeout: 60s
  file_cache:
    max_entries: 50000
    max_size: 268435456  # 256MB
  transport:
    max_idle_conns_per_host: 100
    idle_conn_timeout: 90s
```

## 与 NGINX 对比

| 特性 | Lolly | NGINX |
|------|-------|-------|
| HTTP/3 | 支持 | 1.25+ 支持 |
| 配置格式 | YAML | 自定义格式 |
| 热升级 | 支持 | 支持 |
| 扩展方式 | Go 代码 | C 模块/Lua |
| 部署 | 单二进制 | 需安装 |
| 内存占用 | 较低 | 较低 |
| 故障转移 | 支持 | 支持 |
| 代理缓存 | 支持 | 支持 |

## 依赖

- [fasthttp](https://github.com/valyala/fasthttp) - 高性能 HTTP 服务器
- [quic-go](https://github.com/quic-go/quic-go) - QUIC/HTTP/3 实现
- [zerolog](https://github.com/rs/zerolog) - 高性能日志库
- [klauspost/compress](https://github.com/klauspost/compress) - 压缩算法
- [fasthttp/router](https://github.com/fasthttp/router) - 高性能路由器

## 许可证

MIT License

## 作者

xfy