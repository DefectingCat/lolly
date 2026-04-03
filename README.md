# Lolly

高性能 HTTP 服务器与反向代理，使用 Go 语言编写。

基于 [fasthttp](https://github.com/valyala/fasthttp) 构建，提供比标准 net/http 更高的性能。支持 HTTP/3、WebSocket、虚拟主机、多种负载均衡算法，以及完整的安全与性能优化特性。

## 特性

### 核心功能

- **静态文件服务** - 零拷贝传输（sendfile）、文件缓存、预压缩支持
- **反向代理** - 完整的代理功能，支持请求头/响应头修改、超时控制
- **HTTP/3 (QUIC)** - 基于 quic-go，支持 0-RTT 连接
- **WebSocket** - 完整的 WebSocket 代理支持
- **虚拟主机** - 单进程支持多域名独立配置
- **TCP/UDP Stream** - 四层代理，支持 MySQL、Redis 等服务

### 负载均衡

| 算法 | 说明 |
|------|------|
| Round Robin | 轮询，均匀分配 |
| Weighted Round Robin | 加权轮询，按权重分配 |
| Least Connections | 最少连接，选择活跃连接最少的目标 |
| IP Hash | IP 哈希，同一客户端始终路由到同一目标 |
| Consistent Hash | 一致性哈希，支持虚拟节点，最小化节点变更影响 |

### 安全

- **访问控制** - IP/CIDR 白名单与黑名单
- **速率限制** - 令牌桶与滑动窗口算法
- **认证** - Basic Auth，支持 bcrypt 与 argon2id
- **安全头部** - HSTS、X-Frame-Options、CSP、Referrer-Policy
- **SSL/TLS** - OCSP Stapling、TLS 1.2/1.3、加密套件配置

### 性能优化

- **Goroutine 池** - 限制并发 worker 数量，避免 goroutine 爆炸
- **文件缓存** - LRU 淘汰策略，内存上限控制
- **连接池** - 空闲连接复用，减少连接建立开销
- **零拷贝** - 大文件传输使用 sendfile 系统调用

### 运维

- **热升级** - USR2 信号触发，零停机升级
- **配置热重载** - HUP 信号触发，动态更新配置
- **日志轮转** - USR1 信号触发，重新打开日志文件
- **优雅关闭** - QUIT 信号触发，等待请求完成
- **状态监控** - 内置状态端点，统计连接数、请求数、流量

## 安装

### 构建

```bash
# 克隆仓库
git clone https://github.com/xfy/lolly.git
cd lolly

# 本地构建
make build

# 生产构建（优化体积）
make build-prod

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

配置文件使用 YAML 格式。以下是基本示例：

```yaml
server:
  listen: ":8080"
  static:
    root: "/var/www/html"
    index: ["index.html", "index.htm"]
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
  ssl:
    cert: "/etc/ssl/certs/server.crt"
    key: "/etc/ssl/private/server.key"
  compression:
    type: "gzip"
    level: 6
    min_size: 1024
  security:
    rate_limit:
      request_rate: 100
      burst: 200

http3:
  enabled: true
  listen: ":443"
  max_streams: 100

logging:
  format: "json"
  access:
    path: "/var/log/lolly/access.log"
  error:
    path: "/var/log/lolly/error.log"
    level: "info"

performance:
  goroutine_pool:
    enabled: true
    max_workers: 1000
  file_cache:
    max_entries: 10000
    max_size: 100MB
```

完整配置说明请参考源码 `internal/config/config.go`。

## 架构

```
internal/
  app/          # 应用入口、信号处理、生命周期
  config/       # 配置加载、验证、默认值
  server/       # HTTP 服务器、虚拟主机、Goroutine 池
  handler/      # 路由器、静态文件处理器
  proxy/        # 反向代理、WebSocket、健康检查
  loadbalance/  # 负载均衡算法
  middleware/   # 中间件链
    compression/  # Gzip/Brotli 压缩
    security/     # 访问控制、限流、认证、安全头部
    rewrite/      # URL 重写
    accesslog/    # 访问日志
  http3/        # HTTP/3 服务器
  stream/       # TCP/UDP Stream 代理
  ssl/          # TLS 配置、OCSP Stapling
  cache/        # 文件缓存、代理缓存
  logging/      # 日志系统
```

## 信号处理

| 信号 | 行为 |
|------|------|
| SIGTERM, SIGINT | 快速停止 |
| SIGQUIT | 优雅停止，等待请求完成 |
| SIGHUP | 重载配置 |
| SIGUSR1 | 重新打开日志文件 |
| SIGUSR2 | 热升级 |

## 测试

```bash
# 运行测试
make test

# 测试覆盖率
make test-cover

# 基准测试
make bench

# 代码检查
make check
```

## 性能

基于 fasthttp，相比标准 net/http 有显著性能提升：

- 避免不必要的内存分配
- 优化的事件循环
- 高效的连接池管理
- 零拷贝传输

建议生产环境配置：

```yaml
performance:
  goroutine_pool:
    enabled: true
    max_workers: 10000
  file_cache:
    max_entries: 50000
    max_size: 256MB
  transport:
    max_idle_conns: 1000
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

## 依赖

- [fasthttp](https://github.com/valyala/fasthttp) - 高性能 HTTP 服务器
- [quic-go](https://github.com/quic-go/quic-go) - QUIC/HTTP/3 实现
- [zerolog](https://github.com/rs/zerolog) - 高性能日志库
- [klauspost/compress](https://github.com/klauspost/compress) - 压缩算法

## 许可证

MIT License

## 作者

xfy