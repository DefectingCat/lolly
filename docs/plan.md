# Lolly 实现计划

## 概述

**目标**：创建一个类似 nginx 的高性能 HTTP 服务器，纯 Go 实现，YAML 配置，单二进制运行。

**核心原则**：
- 不是 1:1 复刻 nginx，而是更现代、更易用
- 充分利用 Go 特性（goroutine、channel、标准库）
- 功能完整性优先于极致性能
- 选择"更简单易用"的设计方案

---

## 第一阶段：项目骨架与配置系统

### 目标
搭建项目基础结构，实现配置解析和命令行工具。

### 任务列表

#### 1.1 项目目录结构
```
lolly/
├── cmd/lolly/main.go           # 程序入口
├── internal/
│   ├── config/                 # 配置解析模块
│   ├── server/                 # HTTP 服务器核心
│   ├── handler/                # 请求处理器
│   └── middleware/             # 中间件系统
├── pkg/
│   └── utils/                  # 公共工具函数
├── configs/
│   └── lolly.yaml              # 默认配置示例
├── go.mod
├── go.sum
├── Makefile                    # 构建脚本
└── README.md
```

**关键文件**：
- `cmd/lolly/main.go` - 入口点，初始化和启动逻辑
- `internal/config/config.go` - 配置结构体定义和解析

#### 1.2 YAML 配置解析

**配置结构体设计**：
```go
// internal/config/config.go

// Config 根配置结构
type Config struct {
    Server     ServerConfig     `yaml:"server"`
    Servers    []ServerConfig   `yaml:"servers"`    // 多虚拟主机
    Logging    LoggingConfig    `yaml:"logging"`
    Performance PerformanceConfig `yaml:"performance"`
}

// ServerConfig 服务器配置
type ServerConfig struct {
    Listen      string         `yaml:"listen"`       // 监听地址 ":8080"
    Name        string         `yaml:"name"`         // server_name
    Static      StaticConfig   `yaml:"static"`       // 静态文件
    Proxy       []ProxyConfig  `yaml:"proxy"`        // 反向代理规则
    SSL         SSLConfig      `yaml:"ssl"`          // SSL 配置
}

// StaticConfig 静态文件配置
type StaticConfig struct {
    Root   string   `yaml:"root"`    // 根目录
    Index  []string `yaml:"index"`   // 索引文件
}

// ProxyConfig 反向代理配置
type ProxyConfig struct {
    Path        string   `yaml:"path"`         // 路径匹配
    Target      string   `yaml:"target"`       // 目标地址
    LoadBalance string   `yaml:"load_balance"` // 负载均衡算法
}
```

**实现要点**：
- 使用 `gopkg.in/yaml.v3` 解析 YAML
- 支持配置文件路径命令行参数 `-c/--config`
- 配置验证：必填字段检查、路径有效性
- 默认配置：最小配置即可运行

#### 1.3 命令行工具

**支持的命令**：
```bash
lolly                    # 启动服务器（默认配置）
lolly -c /path/to.yaml   # 指定配置文件
lolly -t                 # 测试配置语法
lolly -v                 # 显示版本
lolly -s reload          # 重载配置（信号）
lolly -s stop            # 停止服务
lolly -s quit            # 优雅停止
```

**实现方式**：
- 使用 `flag` 标准库处理参数
- 信号处理：`SIGTERM`、`SIGINT`、`SIGHUP`

### 验证方法
```bash
# 构建测试
go build -o lolly cmd/lolly/main.go

# 配置解析测试
./lolly -t -c configs/lolly.yaml
# 输出：配置有效

# 版本显示
./lolly -v
# 输出：lolly version 0.1.0
```

### 提交信息
```
feat(config): 实现项目骨架和 YAML 配置解析

- 创建项目目录结构（cmd/internal/pkg/configs）
- 定义配置结构体（Server、Static、Proxy、Logging）
- 实现 YAML 配置文件解析
- 添加命令行参数支持（-c、-t、-v、-s）
- 添加默认配置示例文件
```

---

## 第二阶段：HTTP 核心功能

### 目标
实现基础 HTTP 服务器、静态文件服务、请求路由。

### 任务列表

#### 2.1 基础 HTTP 服务器

**核心实现**：
```go
// internal/server/server.go

// Server HTTP 服务器
type Server struct {
    config    *config.Config
    handler   *handler.Handler
    listeners map[string]*net.Listener
    running   bool
    stopChan  chan struct{}
}

// Start 启动服务器
func (s *Server) Start() error

// Stop 停止服务器
func (s *Server) Stop() error

// GracefulStop 优雅停止（等待请求完成）
func (s *Server) GracefulStop(timeout time.Duration) error
```

**实现要点**：
- 使用 `net/http` 标准库为基础
- 支持 multiple listeners（多端口/多虚拟主机）
- 优雅关闭：`Shutdown()` 方法
- keep-alive 配置：`ReadTimeout`、`WriteTimeout`、`IdleTimeout`

#### 2.2 静态文件服务

**实现**：
```go
// internal/handler/static.go

// StaticHandler 静态文件处理器
type StaticHandler struct {
    root  string
    index []string
}

// ServeHTTP 处理静态文件请求
func (h *StaticHandler) ServeHTTP(w http.ResponseWriter, r *http.Request)
```

**功能清单**：
- 文件路径安全检查（防止目录遍历）
- MIME 类型自动识别（`mime.TypeByExtension`）
- 索引文件支持（index.html、index.htm）
- 目录列表（可选，默认禁用）
- Range 请求支持（部分下载）
- 文件缓存优化（可选）

#### 2.3 请求路由（Location 匹配）

**路由设计**：
```go
// internal/handler/router.go

// Router 请求路由器
type Router struct {
    routes []*Route
}

// Route 路由规则
type Route struct {
    Path     string      // 路径模式
    Type     MatchType   // 匹配类型：精确/前缀/正则
    Handler  http.Handler
}

// 匹配类型
type MatchType int
const (
    MatchExact  MatchType = iota  // 精确匹配 (=)
    MatchPrefix                    // 前缀匹配 (^~)
    MatchRegex                     // 正则匹配 (~)
)
```

**匹配优先级**（同 nginx）：
1. 精确匹配 `=`
2. 前缀匹配（优先）`^~`
3. 正则匹配 `~`（按顺序）
4. 最长前缀匹配

#### 2.4 多虚拟主机支持

**实现**：
```go
// internal/server/vhost.go

// VHostManager 虚拟主机管理器
type VHostManager struct {
    hosts map[string]*VirtualHost  // 按 server_name 索引
}
```

**功能**：
- 按 `Host` 头选择虚拟主机
- 默认主机fallback
- SNI 支持（SSL）

### 验证方法
```bash
# 启动服务器
./lolly -c configs/lolly.yaml

# 静态文件测试
curl http://localhost:8080/index.html

# 路由测试
curl http://localhost:8080/static/test.txt
curl http://localhost:8080/api/health  # 应返回 404（代理未实现）
```

### 提交信息
```
feat(server): 实现 HTTP 服务器核心功能

- 实现基础 HTTP 服务器（启动/停止/优雅关闭）
- 实现静态文件服务（MIME 类型、索引文件、Range 请求）
- 实现请求路由器（精确/前缀/正则匹配）
- 支持多虚拟主机配置
- 支持 keep-alive 长连接配置
```

---

## 第三阶段：反向代理与负载均衡

### 目标
实现反向代理功能、多种负载均衡算法、健康检查。

### 任务列表

#### 3.1 反向代理核心

**实现**：
```go
// internal/proxy/proxy.go

// Proxy 反向代理
type Proxy struct {
    targets    []*Target
    transport  *http.Transport
    bufferPool *bufferPool
}

// Target 后端目标
type Target struct {
    URL        *url.URL
    Weight     int
    Healthy    bool
    Connections int
}
```

**功能清单**：
- 请求转发：修改请求头、请求体
- 响应处理：修改响应头
- 超时配置：连接超时、响应超时
- WebSocket 支持：Upgrade 协议检测和转发
- 错误处理：后端不可用时的响应

#### 3.2 负载均衡算法

**实现**：
```go
// internal/loadbalance/balancer.go

// Balancer 负载均衡器接口
type Balancer interface {
    Select(targets []*Target) *Target
}

// RoundRobin 轮询算法
type RoundRobin struct {
    current uint64
}

// WeightedRoundRobin 权重轮询
type WeightedRoundRobin struct {
    weights []int
    current int
}

// LeastConnections 最少连接
type LeastConnections struct{}

// IPHash IP 哈希
type IPHash struct{}
```

**算法实现**：
| 算法 | 说明 |
|------|------|
| round_robin | 简单轮询 |
| weighted_round_robin | 按权重轮询 |
| least_conn | 选择连接数最少的目标 |
| ip_hash | 按客户端 IP 哈希固定目标 |

#### 3.3 健康检查

**实现**：
```go
// internal/proxy/health.go

// HealthChecker 健康检查器
type HealthChecker struct {
    interval   time.Duration
    timeout    time.Duration
    path       string  // 健康检查路径
    targets    []*Target
}

// Check 执行健康检查
func (h *HealthChecker) Check()

// Start 后台定期检查
func (h *HealthChecker) Start()
```

**类型**：
- **被动检查**：请求失败时标记不健康
- **主动检查**：定期发送探测请求

**配置示例**：
```yaml
proxy:
  - path: /api
    targets:
      - url: http://backend1:8080
        weight: 3
      - url: http://backend2:8080
        weight: 1
    load_balance: weighted_round_robin
    health_check:
      interval: 10s
      path: /health
      timeout: 5s
```

#### 3.4 代理缓存（可选）

**实现**：
```go
// internal/cache/proxy_cache.go

// ProxyCache 代理响应缓存
type ProxyCache struct {
    storage CacheStorage
    rules   []CacheRule
}
```

### 验证方法
```bash
# 启动后端服务（用于测试）
# backend1: python3 -m http.server 8001
# backend2: python3 -m http.server 8002

# 配置代理
# lolly.yaml:
#   proxy:
#     - path: /api
#       targets: [http://localhost:8001, http://localhost:8002]

# 测试代理
curl http://localhost:8080/api/test

# 测试负载均衡（多次请求）
for i in {1..10}; do curl http://localhost:8080/api/test; done
```

### 提交信息
```
feat(proxy): 实现反向代理和负载均衡功能

- 实现反向代理核心（请求转发、响应处理）
- 实现负载均衡算法（轮询、权重、最少连接、IP哈希）
- 实现被动健康检查（请求失败标记）
- 实现主动健康检查（定期探测）
- 支持 WebSocket 协议升级代理
```

---

## 第四阶段：安全与 SSL/TLS

### 目标
实现 HTTPS 支持、访问控制、请求限制。

### 任务列表

#### 4.1 SSL/TLS 支持

**实现**：
```go
// internal/ssl/ssl.go

// SSLConfig SSL 配置
type SSLConfig struct {
    Cert       string   `yaml:"cert"`       // 证书路径
    Key        string   `yaml:"key"`        // 私钥路径
    Protocols  []string `yaml:"protocols"`  // TLS 版本
    Ciphers    []string `yaml:"ciphers"`    // 加密套件
}

// TLSManager TLS 管理器
type TLSManager struct {
    configs map[string*tls.Config  // 按 server_name
}
```

**功能清单**：
- 证书加载：PEM 格式
- TLS 版本控制：TLSv1.2、TLSv1.3
- 加密套件配置
- SSL 会话缓存（减少握手开销）
- SNI 支持（多证书）
- HTTP/2 自动启用（TLS 时）

#### 4.2 IP 访问控制

**实现**：
```go
// internal/security/access.go

// AccessControl IP 访问控制
type AccessControl struct {
    allowList []net.IPNet
    denyList  []net.IPNet
    default   Action  // 默认动作
}

// Check 检查 IP 是否允许
func (a *AccessControl) Check(ip net.IP) bool
```

**配置示例**：
```yaml
security:
  access:
    allow: [192.168.1.0/24, 10.0.0.0/8]
    deny: [192.168.2.100]
    default: deny
```

#### 4.3 请求限制

**实现**：
```go
// internal/security/ratelimit.go

// RateLimiter 速率限制器
type RateLimiter struct {
    requests int
    per      time.Duration
    buckets  map[string]*Bucket
    mu       sync.RWMutex
}

// ConnLimiter 连接数限制器
type ConnLimiter struct {
    max      int
    current  int
    mu       sync.Mutex
}
```

**功能**：
- 请求速率限制（`limit_req`）
- 连接数限制（`limit_conn`）
- 按 IP 或按 key 限制
- 超限响应：429 Too Many Requests

#### 4.4 基础认证

**实现**：
```go
// internal/security/auth.go

// BasicAuth 基础认证
type BasicAuth struct {
    users map[string]string  // username -> hashed_password
}

// Authenticate 验证认证信息
func (b *BasicAuth) Authenticate(r *http.Request) bool
```

**配置示例**：
```yaml
security:
  auth:
    type: basic
    users:
      - name: admin
        password: $apr1$...  # htpasswd 格式
    realm: "Restricted Area"
```

#### 4.5 安全头部

**自动添加的安全头**：
- `X-Frame-Options: DENY`
- `X-Content-Type-Options: nosniff`
- `X-XSS-Protection: 1; mode=block`
- `Strict-Transport-Security: max-age=31536000`

### 验证方法
```bash
# HTTPS 测试
curl -k https://localhost:8443/

# IP 访问控制测试
curl --interface 192.168.1.100 http://localhost:8080/  # 应允许
curl --interface 192.168.2.100 http://localhost:8080/  # 应拒绝

# 速率限制测试
for i in {1..20}; do curl http://localhost:8080/; done  # 部分应返回 429

# 基础认证测试
curl -u admin:password http://localhost:8080/protected/
```

### 提交信息
```
feat(security): 实现 SSL/TLS 和安全访问控制

- 实现 SSL/TLS 支持（证书加载、TLS版本、加密套件）
- 实现 SSL 会话缓存
- 实现 SNI 多证书支持
- 实现 IP 访问控制（白名单/黑名单）
- 实现请求速率限制
- 实现连接数限制
- 实现基础认证（Basic Auth）
- 添加安全响应头部
```

---

## 第五阶段：增强功能

### 目标
实现 URL 重写、压缩、缓存、日志系统。

### 任务列表

#### 5.1 URL 重写

**实现**：
```go
// internal/rewrite/rewrite.go

// RewriteRule 重写规则
type RewriteRule struct {
    Pattern     string  // 匹配模式
    Replacement string  // 替换目标
    Flag        RewriteFlag  // last/redirect/break
}

type RewriteFlag int
const (
    FlagLast     RewriteFlag = iota  // 继续匹配其他规则
    FlagRedirect                      // 302 重定向
    FlagPermanent                     // 301 重定向
    FlagBreak                         // 停止匹配
)
```

**配置示例**：
```yaml
rewrite:
  - pattern: "^/old/(.*)$"
    replacement: "/new/$1"
    flag: permanent  # 301
  - pattern: "^/api/v1/(.*)$"
    replacement: "/api/v2/$1"
    flag: last
```

#### 5.2 Gzip/Brotli 压缩

**实现**：
```go
// internal/compression/compression.go

// CompressionHandler 压缩中间件
type CompressionHandler struct {
    types    []string  // 压缩的 MIME 类型
    level    int       // 压缩级别
    minSize  int       // 最小压缩大小
}
```

**配置示例**：
```yaml
compression:
  type: gzip  # gzip/brotli/both
  level: 6    # 1-9
  min_size: 1024  # 最小 1KB 才压缩
  types: [text/html, text/css, application/json]
```

#### 5.3 缓存系统

**静态文件缓存**：
```go
// internal/cache/file_cache.go

// FileCache 文件描述符缓存
type FileCache struct {
    maxEntries int
    inactive   time.Duration
    entries    map[string]*FileEntry
}
```

**代理响应缓存**：
```go
// internal/cache/proxy_cache.go

// ProxyCache 代理缓存
type ProxyCache struct {
    storage   Storage  // 内存/磁盘存储
    rules     []CacheRule
    maxAge    time.Duration
}
```

#### 5.4 日志系统

**实现**：
```go
// internal/logging/logging.go

// Logger 日志管理器
type Logger struct {
    accessLog *AccessLogger
    errorLog  *ErrorLogger
    level     LogLevel
}

// AccessLogger 访问日志
type AccessLogger struct {
    format string  // 日志格式
    output io.Writer
}

// LogFormat 日志格式变量
// $remote_addr - 客户端 IP
// $request - 请求行
// $status - 响应状态码
// $body_bytes_sent - 响应体大小
// $request_time - 请求耗时
```

**配置示例**：
```yaml
logging:
  access:
    path: /var/log/lolly/access.log
    format: "$remote_addr - $request - $status - $body_bytes_sent"
  error:
    path: /var/log/lolly/error.log
    level: info  # debug/info/warn/error
```

#### 5.5 状态监控端点

**实现**：
```go
// internal/server/status.go

// StatusHandler 状态监控处理器
type StatusHandler struct {
    server *Server
}

// 返回数据
type Status struct {
    Connections   int
    Requests      int64
    BytesSent     int64
    BytesReceived int64
    Uptime        time.Duration
}
```

**配置示例**：
```yaml
monitoring:
  status:
    path: /_status  # 状态端点路径
    allow: [127.0.0.1]  # 仅允许本地访问
```

### 验证方法
```bash
# 重写测试
curl http://localhost:8080/old/page  # 应重定向到 /new/page

# 压缩测试
curl -H "Accept-Encoding: gzip" -I http://localhost:8080/index.html
# 应返回 Content-Encoding: gzip

# 缓存测试
curl -I http://localhost:8080/static/test.txt
# 应返回缓存相关头部

# 日志测试
cat /var/log/lolly/access.log

# 状态监控测试
curl http://localhost:8080/_status
```

### 提交信息
```
feat(enhance): 实现重写、压缩、缓存和日志功能

- 实现 URL 重写规则（正则匹配、301/302重定向）
- 实现 Gzip 响应压缩
- 实现静态文件缓存（文件描述符缓存）
- 实现代理响应缓存
- 实现访问日志（可定制格式）
- 实现分级错误日志
- 实现状态监控端点
```

---

## 第六阶段：高级功能

### 目标
实现 TCP/UDP Stream 代理、性能优化、优雅升级。

### 任务列表

#### 6.1 TCP/UDP Stream 代理

**实现**：
```go
// internal/stream/stream.go

// StreamServer TCP/UDP 代理服务器
type StreamServer struct {
    listeners map[string]*net.Listener
    upstreams map[string]*StreamUpstream
}

// StreamUpstream Stream 上游
type StreamUpstream struct {
    targets  []*StreamTarget
    balancer Balancer
}

// StreamTarget Stream 目标
type StreamTarget struct {
    addr      string
    healthy   bool
}
```

**配置示例**：
```yaml
stream:
  - listen: 3306
    protocol: tcp
    upstream:
      targets: [mysql1:3306, mysql2:3306]
      load_balance: round_robin
  - listen: 53
    protocol: udp
    upstream:
      targets: [dns1:53, dns2:53]
```

#### 6.2 优雅升级（热升级）

**实现**：
```go
// internal/server/upgrade.go

// GracefulUpgrade 优雅升级
func GracefulUpgrade(newBinary string) error

// 逻辑：
// 1. 启动新进程，继承监听 socket
// 2. 新进程开始接受新连接
// 3. 旧进程停止接受新连接，完成现有请求后退出
```

**信号处理**：
- `SIGUSR2`：触发升级
- `SIGWINCH`：优雅关闭 worker

#### 6.3 性能优化

**优化点**：
- 连接复用：`http.Transport` 配置
- 缓冲池：减少内存分配
- 零拷贝：`io.Copy` 使用 `sendfile`（Linux）
- Goroutine 池：控制并发数（可选）
- 对象池：`sync.Pool` 复用对象

#### 6.4 信号处理完善

**完整信号支持**：
| 信号 | 行为 |
|------|------|
| `SIGTERM/SIGINT` | 快速停止 |
| `SIGQUIT` | 优雅停止 |
| `SIGHUP` | 重载配置 |
| `SIGUSR1` | 重新打开日志 |
| `SIGUSR2` | 热升级 |

### 验证方法
```bash
# TCP Stream 测试
# 启动 MySQL 后端
mysql -h localhost -P 3306  # 应通过 lolly 代理连接

# 热升级测试
kill -USR2 <pid>  # 触发升级
ps aux | grep lolly  # 应有两个进程

# 性能测试
# 使用 wrk 或 ab 进行压力测试
wrk -t4 -c1000 -d30s http://localhost:8080/
```

### 提交信息
```
feat(advanced): 实现 Stream 代理和高级功能

- 实现 TCP Stream 代理和负载均衡
- 实现 UDP Stream 代理
- 实现优雅升级（热升级）
- 完善信号处理（SIGHUP、SIGUSR1、SIGUSR2）
- 实现性能优化（连接复用、缓冲池）
```

---

## 文件依赖关系图

```
Phase 1:
  cmd/lolly/main.go → internal/config/config.go

Phase 2:
  internal/server/server.go → internal/config/config.go
  internal/server/server.go → internal/handler/router.go
  internal/handler/router.go → internal/handler/static.go

Phase 3:
  internal/handler/router.go → internal/proxy/proxy.go
  internal/proxy/proxy.go → internal/loadbalance/balancer.go
  internal/proxy/proxy.go → internal/proxy/health.go

Phase 4:
  internal/server/server.go → internal/ssl/ssl.go
  internal/handler/router.go → internal/security/access.go
  internal/handler/router.go → internal/security/ratelimit.go
  internal/handler/router.go → internal/security/auth.go

Phase 5:
  internal/handler/router.go → internal/rewrite/rewrite.go
  internal/server/server.go → internal/compression/compression.go
  internal/proxy/proxy.go → internal/cache/proxy_cache.go
  internal/server/server.go → internal/logging/logging.go

Phase 6:
  cmd/lolly/main.go → internal/stream/stream.go
  internal/server/server.go → internal/server/upgrade.go
```

---

## 总体进度追踪

| 阶段 | 状态 | 主要功能 |
|------|------|----------|
| Phase 1 | 待开始 | 项目骨架、配置系统 |
| Phase 2 | 待开始 | HTTP 核心、静态文件、路由 |
| Phase 3 | 待开始 | 反向代理、负载均衡 |
| Phase 4 | 待开始 | SSL/TLS、安全控制 |
| Phase 5 | 待开始 | 重写、压缩、缓存、日志 |
| Phase 6 | 待开始 | Stream、性能优化 |

---

## 参考文档

详细功能参考 `docs/` 目录：
- HTTP 核心：`docs/03-nginx-http-core.md`
- 代理负载均衡：`docs/04-nginx-proxy-loadbalancing.md`
- SSL/HTTPS：`docs/05-nginx-ssl-https.md`
- URL 重写：`docs/06-nginx-rewrite.md`
- 压缩缓存：`docs/07-nginx-compression-caching.md`
- 日志监控：`docs/08-nginx-logging-monitoring.md`
- 安全控制：`docs/09-nginx-security.md`
- Stream 代理：`docs/10-nginx-stream-tcp-udp.md`
- 性能优化：`docs/12-nginx-performance-tuning.md`

**代码注释规范**：`docs/comments.md`（必须遵循）