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
├── main.go           # 程序入口
├── internal/
│   ├── config/                 # 配置解析模块
│   ├── server/                 # HTTP 服务器核心
│   ├── handler/                # 请求处理器
│   ├── middleware/             # 中间件系统（框架）
│   │   ├── security/           # 安全中间件（access、ratelimit、auth）
│   │   ├── compression/        # 压缩中间件（gzip、brotli）
│   │   ├── logging/            # 日志中间件
│   │   └── rewrite/            # URL 重写中间件
│   ├── proxy/                  # 反向代理模块
│   ├── loadbalance/            # 负载均衡模块
│   ├── ssl/                    # SSL/TLS 模块
│   ├── cache/                  # 缓存模块
│   ├── stream/                 # TCP/UDP Stream 代理
│   └── logging/                # 日志系统核心
├── pkg/
│   └── utils/                  # 公共工具函数
├── go.mod
├── go.sum
├── Makefile                    # 构建脚本
└── README.md
```

**关键文件**：

- `main.go` - 入口点，初始化和启动逻辑
- `internal/config/config.go` - 配置结构体定义和解析
- `internal/middleware/middleware.go` - 中间件框架接口定义

#### 1.2 YAML 配置解析

**配置结构体设计**：

```go
// internal/config/config.go

// Config 根配置结构
type Config struct {
    DefaultServer ServerConfig     `yaml:"server"`      // 默认服务器配置（单服务器场景）
    Servers       []ServerConfig   `yaml:"servers"`     // 多虚拟主机配置（可选）
    Logging       LoggingConfig    `yaml:"logging"`
    Performance   PerformanceConfig `yaml:"performance"`
}

// 配置字段语义说明：
// - 若只配置 `server` 字段，则作为单一服务器运行
// - 若配置 `servers` 字段，则按虚拟主机模式运行（按 Host 头匹配）
// - 若同时配置两者，`server` 作为默认 fallback 主机，`servers` 按名称匹配
// - 建议使用场景：简单部署用 `server`，多站点部署用 `servers`

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
lolly -v                 # 显示版本
```

**实现方式**：

- 使用 `flag` 标准库处理参数
- 信号处理：`SIGTERM`、`SIGINT`、`SIGHUP`

### 验证方法

```bash
# 构建测试
make build

# 版本显示
./lolly -v
```

---

## 第二阶段：HTTP 核心功能

### 目标

实现基础 HTTP 服务器、静态文件服务、请求路由、基础日志系统。

### 技术选型

**HTTP 库**：使用 [fasthttp](https://github.com/valyala/fasthttp) 替代 `net/http`。

**选择理由**：
- **高性能**：比 net/http 快 6 倍
- **零分配**：热点路径无内存分配，GC 压力最小
- **原生支持高性能场景**：无需额外优化

**路由库**：使用 [fasthttp/router](https://github.com/fasthttp/router)。

**性能对比**：

| 库 | 特点 | 性能 |
|----|------|------|
| fasthttp | 零分配，高性能 | ⭐⭐⭐⭐⭐ |
| net/http | 标准库，通用 | ⭐⭐⭐ |

**关键差异**：

| net/http | fasthttp |
|----------|----------|
| `http.Handler` 接口 | `RequestHandler` 函数 |
| `ServeMux` 内置路由 | 无，需 router 库 |
| `http.Request` 对象 | `*fasthttp.RequestCtx` |
| `http.ResponseWriter` | `ctx` 同时处理读写 |

### 任务列表

#### 2.0 中间件框架（前置依赖）

**实现**：

```go
// internal/middleware/middleware.go

import "github.com/valyala/fasthttp"

// Middleware 中间件接口
type Middleware interface {
    Name() string
    Process(next fasthttp.RequestHandler) fasthttp.RequestHandler
}

// Chain 中间件链
type Chain struct {
    middlewares []Middleware
}

// Apply 应用中间件链
func (c *Chain) Apply(final fasthttp.RequestHandler) fasthttp.RequestHandler {
    handler := final
    for i := len(c.middlewares) - 1; i >= 0; i-- {
        handler = c.middlewares[i].Process(handler)
    }
    return handler
}
```

**设计要点**：

- 定义统一的中间件接口，所有中间件实现 `RequestHandler` 函数签名
- 支持链式组合，按注册顺序逆序包装（从后往前）
- Phase 2 建立框架，后续阶段填充具体中间件实现

#### 2.1 基础 HTTP 服务器

**核心实现**：

```go
// internal/server/server.go

import "github.com/valyala/fasthttp"

// Server HTTP 服务器
type Server struct {
    config     *config.Config
    fastServer *fasthttp.Server
    handler    fasthttp.RequestHandler
    running    bool
}

// Start 启动服务器
func (s *Server) Start() error {
    s.fastServer = &fasthttp.Server{
        Handler:            s.handler,
        ReadTimeout:        s.config.Server.ReadTimeout,
        WriteTimeout:       s.config.Server.WriteTimeout,
        IdleTimeout:        s.config.Server.IdleTimeout,
        MaxConnsPerIP:      s.config.Server.MaxConnsPerIP,
        MaxRequestsPerConn: s.config.Server.MaxRequestsPerConn,
    }
    return s.fastServer.ListenAndServe(s.config.Server.Listen)
}

// Stop 快速停止服务器
func (s *Server) Stop() error {
    return s.fastServer.Shutdown()
}

// GracefulStop 优雅停止（等待请求完成）
func (s *Server) GracefulStop(timeout time.Duration) error {
    // fasthttp 的 Shutdown 本身就是优雅关闭
    return s.fastServer.Shutdown()
}
```

**实现要点**：

- 使用 `fasthttp.Server` 配置超时和连接限制
- 优雅关闭：`Shutdown()` 方法自动等待请求完成
- 配置项：`ReadTimeout`、`WriteTimeout`、`IdleTimeout`、`MaxConnsPerIP`

#### 2.2 静态文件服务

**实现**：

```go
// internal/handler/static.go

import "github.com/valyala/fasthttp"

// StaticHandler 静态文件处理器
type StaticHandler struct {
    root  string
    index []string
}

// Handle 处理静态文件请求
func (h *StaticHandler) Handle(ctx *fasthttp.RequestCtx) {
    path := string(ctx.Path())

    // 安全检查：防止目录遍历
    if strings.Contains(path, "..") {
        ctx.Error("Forbidden", fasthttp.StatusForbidden)
        return
    }

    // 拼接文件路径
    filePath := filepath.Join(h.root, path)

    // 尝试索引文件
    if info, err := os.Stat(filePath); err == nil && info.IsDir() {
        for _, idx := range h.index {
            idxPath := filepath.Join(filePath, idx)
            if fasthttp.ServeFile(ctx, idxPath) == nil {
                return
            }
        }
    }

    // 直接返回文件
    fasthttp.ServeFile(ctx, filePath)
}
```

**功能清单**：

- 文件路径安全检查（防止目录遍历）
- MIME 类型自动识别（fasthttp 内置）
- 索引文件支持（index.html、index.htm）
- Range 请求支持（fasthttp 内置）
- 文件缓存优化（可选）

#### 2.3 请求路由

**路由库**：使用 [fasthttp/router](https://github.com/fasthttp/router)，基于 radix tree 高效匹配。

**实现**：

```go
// internal/handler/router.go

import (
    "github.com/valyala/fasthttp"
    "github.com/fasthttp/router"
)

// Router 请求路由器
type Router struct {
    router *router.Router
}

// NewRouter 创建路由器
func NewRouter() *Router {
    return &Router{
        router: router.New(),
    }
}

// Register 注册路由
func (r *Router) Register(path string, handler fasthttp.RequestHandler) {
    r.router.GET(path, handler)
    r.router.POST(path, handler)
    r.router.PUT(path, handler)
    r.router.DELETE(path, handler)
}

// Handler 返回路由处理器
func (r *Router) Handler() fasthttp.RequestHandler {
    return r.router.Handler
}
```

**fasthttp/router 匹配类型**：

| 类型 | 语法 | 示例 |
|------|------|------|
| Named | `{name}` | `/user/{id}` |
| Optional | `{name?}` | `/search/{q?}` |
| Regex | `{name:regex}` | `/user/{id:[0-9]+}` |
| Catch-All | `{filepath:*}` | `/files/{filepath:*}` |

**参数提取**：

```go
func handler(ctx *fasthttp.RequestCtx) {
    id := ctx.UserValue("id")  // 获取路由参数
}
```

#### 2.4 多虚拟主机支持

**实现**：

```go
// internal/server/vhost.go

import "github.com/valyala/fasthttp"

// VHostManager 虚拟主机管理器
type VHostManager struct {
    hosts       map[string]*VirtualHost  // 按 server_name 索引
    defaultHost *VirtualHost             // 默认主机
}

// VirtualHost 虚拟主机
type VirtualHost struct {
    name    string
    handler fasthttp.RequestHandler
}

// Handler 返回虚拟主机选择器
func (v *VHostManager) Handler() fasthttp.RequestHandler {
    return func(ctx *fasthttp.RequestCtx) {
        host := string(ctx.Host())
        if vhost, ok := v.hosts[host]; ok {
            vhost.handler(ctx)
        } else if v.defaultHost != nil {
            v.defaultHost.handler(ctx)
        } else {
            ctx.Error("Host not found", fasthttp.StatusNotFound)
        }
    }
}
```

**功能**：

- 按 `Host` 头选择虚拟主机
- 默认主机 fallback
- SNI 支持（SSL，Phase 4）

#### 2.5 基础日志系统（Phase 2 必需）

**原因**：调试 Phase 2-4 功能需要日志支持，将日志系统基础版本提前实现。

**选型**：使用 [zerolog](https://github.com/rs/zerolog) 作为日志库。

**选择理由**：
- **零分配**：高并发场景 GC 压力最小，性能最优
- **JSON 输出**：便于日志采集系统（ELK、Loki）解析
- **API 简洁**：链式调用风格，开发体验好
- **灵活输出**：支持 stdout/stderr/文件，开发模式可选 ConsoleWriter 美化

**性能对比**（10 条日志，禁用输出）：

| 库 | ns/op | allocs/op |
|----|-------|-----------|
| zerolog | ~40ns | **0** |
| zap (structured) | ~50ns | 0 |
| slog (Go 1.21+) | ~200ns | 5+ |
| logrus | ~2000ns | 23 |

**实现**：

```go
// internal/logging/logging.go

import "github.com/rs/zerolog"

// 全局日志实例
var log zerolog.Logger

// Init 初始化日志系统
func Init(level string, pretty bool) {
    l := parseLevel(level)
    if pretty {
        // 开发模式：带颜色和格式化（性能较差，仅开发用）
        log = zerolog.New(zerolog.ConsoleWriter{Out: os.Stdout})
    } else {
        // 生产模式：JSON 输出
        log = zerolog.New(os.Stdout).Level(l).With().Timestamp().Logger()
    }
}

// AccessLogger 访问日志（基础版）
func LogAccess(r *http.Request, status int, size int64, duration time.Duration) {
    log.Info().
        Str("method", r.Method).
        Str("path", r.URL.Path).
        Int("status", status).
        Int64("size", size).
        Dur("duration", duration).
        Msg("request")
}
```

**Phase 2 实现范围**：

- 基础请求日志：记录请求方法、路径、状态码
- 控制台输出：开发阶段便于调试（ConsoleWriter 美化）
- Phase 5 将扩展为完整日志系统（文件输出、自定义格式、访问/错误日志分离）

### 验证方法

```bash
# 启动服务器
./lolly -c lolly.yaml

# 静态文件测试
curl http://localhost:8080/index.html

# 路由测试
curl http://localhost:8080/static/test.txt
curl http://localhost:8080/api/health  # 应返回 404（代理未实现）
```

---

## 第三阶段：反向代理与负载均衡

### 目标

实现反向代理功能、多种负载均衡算法、健康检查。

### 任务列表

#### 3.1 反向代理核心

**实现**（基于 fasthttp）：

```go
// internal/proxy/proxy.go

import "github.com/valyala/fasthttp"

// Proxy 反向代理
type Proxy struct {
    targets    []*Target
    clients    map[string]*fasthttp.HostClient  // 每个目标一个 HostClient
    balancer   Balancer
}

// Target 后端目标
type Target struct {
    URL         string   // 目标地址，如 "http://backend1:8080"
    Weight      int
    Healthy     bool
    Connections int64    // 当前连接数（原子操作）
}

// HostClient fasthttp 客户端（连接池）
// 每个 Target 对应一个 HostClient，自动管理连接池
```

**功能清单**：

- 请求转发：修改请求头、请求体
- 响应处理：修改响应头
- 超时配置：连接超时、响应超时（fasthttp.HostClient 配置）
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
    Cert         string   `yaml:"cert"`          // 证书路径
    Key          string   `yaml:"key"`           // 私钥路径
    CertChain    string   `yaml:"cert_chain"`    // 证书链路径（可选，用于中间证书）
    Protocols    []string `yaml:"protocols"`     // TLS 版本，默认 ["TLSv1.2", "TLSv1.3"]
    Ciphers      []string `yaml:"ciphers"`       // 加密套件（仅 TLS 1.2 有效）
    OCSPStapling bool     `yaml:"ocsp_stapling"` // OCSP Stapling 支持（默认 false）
}

// TLSManager TLS 管理器
type TLSManager struct {
    configs map[string]*tls.Config  // 按 server_name
}
```

**安全默认配置**：

- **TLS 版本**：默认仅允许 TLSv1.2 和 TLSv1.3，**强制禁用 TLSv1.0/TLSv1.1**
- **加密套件默认值**（TLS 1.2，按优先级排序）：
  ```yaml
  # 默认安全加密套件，无需手动配置
  ciphers:
    - TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256 # 推荐，性能好
    - TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384 # 推荐，更安全
    - TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305 # 推荐，移动端友好
    - TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256 # ECDSA 证书专用
    - TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384 # ECDSA 证书专用
    - TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305 # ECDSA 证书专用
  ```
- **TLS 1.3**：自动使用 Go 标准库的安全套件，配置无效

**安全校验**：

- 启用 Basic Auth 时，**强制要求 SSL 配置**，否则拒绝启动
- 拒绝配置不安全的加密套件（如 RC4、DES、3DES）

**功能清单**：

- 证书加载：PEM 格式，支持证书链合并
- TLS 版本控制：TLSv1.2、TLSv1.3，**默认禁用不安全版本**
- 加密套件配置：提供安全默认值，拒绝不安全套件
- SSL 会话缓存：减少握手开销（LRU 缓存，默认 128 条）
- SNI 支持：多证书，通过 GetCertificate 回调实现
- HTTP/2 自动启用：TLS 时自动启用
- OCSP Stapling：减少客户端 CA 查询延迟和隐私风险

#### 4.2 IP 访问控制

**实现**：

```go
// internal/middleware/security/access.go

// AccessControl IP 访问控制
type AccessControl struct {
    allowList []net.IPNet
    denyList  []net.IPNet
    default   Action  // 默认动作
}

// Check 检查 IP 是否允许
func (a *AccessControl) Check(ip net.IP) bool
```

**性能优化建议**：

- 使用 CIDR 树结构（radix tree）优化大规模 ACL 匹配
- 预编译匹配规则，减少运行时开销
- 支持 IPv4 和 IPv6 双栈匹配

**配置示例**：

```yaml
security:
  access:
    allow: [192.168.1.0/24, 10.0.0.0/8, "2001:db8::/32"] # 支持 IPv6
    deny: [192.168.2.100/32]
    default: deny
    # 可选：使用高性能匹配模式
    optimize: true # 启用 CIDR 树优化（适用于 >100 条规则）
```

#### 4.3 请求限制

**实现**：

```go
// internal/middleware/security/ratelimit.go

// RateLimiter 速率限制器（令牌桶算法）
type RateLimiter struct {
    rate     int           // 令牌生成速率（请求/秒）
    burst    int           // 桶容量（突发流量上限）
    buckets  map[string]*TokenBucket
    mu       sync.RWMutex
}

// TokenBucket 令牌桶
type TokenBucket struct {
    tokens     float64
    lastUpdate time.Time
}

// SlidingWindowLimiter 滑动窗口限流器（可选，解决边界突发问题）
type SlidingWindowLimiter struct {
    window    time.Duration
    limit     int
    requests  map[string][]time.Time
}

// ConnLimiter 连接数限制器
type ConnLimiter struct {
    max      int
    current  int
    mu       sync.Mutex
}
```

**算法选择**：
| 算法 | 适用场景 | 特点 |
|------|----------|------|
| 令牌桶 (Token Bucket) | API 请求限流 | 允许突发流量，推荐默认使用 |
| 滑动窗口 (Sliding Window) | 精确限流 | 解决固定窗口边界问题，无突发 |

**功能**：

- 请求速率限制（`limit_req`）
- 连接数限制（`limit_conn`）
- 按 IP 或按 key 限制
- 超限响应：429 Too Many Requests
- 支持 `Retry-After` 响应头告知等待时间

#### 4.4 基础认证

**实现**：

```go
// internal/middleware/security/auth.go

// BasicAuth 基础认证
type BasicAuth struct {
    users     map[string]string  // username -> hashed_password
    algorithm HashAlgorithm      // 哈希算法：bcrypt（默认）或 argon2id
    realm     string
    requireTLS bool              // 强制 HTTPS（默认 true）
}

// HashAlgorithm 哈希算法类型
type HashAlgorithm int
const (
    HashBcrypt HashAlgorithm = iota  // bcrypt（默认，推荐）
    HashArgon2id                       // Argon2id（更安全，计算密集）
)

// Authenticate 验证认证信息
func (b *BasicAuth) Authenticate(r *http.Request) bool
```

**安全要求**：

- **强制 HTTPS**：启用 Basic Auth 时必须配置 SSL，否则拒绝启动
- **安全哈希**：默认使用 bcrypt（成本因子 12），可选 Argon2id
- **弃用 apr1**：不再支持不安全的 MD5-based apr1 哈希
- **密码强度**：配置验证，拒绝弱密码

**配置示例**：

```yaml
security:
  auth:
    type: basic
    require_tls: true # 强制 HTTPS（默认 true）
    algorithm: bcrypt # bcrypt（默认）或 argon2id
    users:
      - name: admin
        password: $2b$12$... # bcrypt 哈希（推荐）
      - name: api_user
        password: $argon2id$... # Argon2id 哈希（可选）
    realm: "Restricted Area"
    min_password_length: 12 # 密码最小长度
```

#### 4.5 安全头部

**实现**：

```go
// internal/middleware/security/headers.go

// SecurityHeaders 安全头部配置
type SecurityHeaders struct {
    XFrameOptions        string `yaml:"x_frame_options"`         // DENY/SAMEORIGIN/ALLOW-FROM
    XContentTypeOptions  string `yaml:"x_content_type_options"`  // nosniff（默认）
    ContentSecurityPolicy string `yaml:"content_security_policy"` // CSP 策略
    HSTS                 HSTSConfig `yaml:"hsts"`               // HSTS 配置
    ReferrerPolicy       string `yaml:"referrer_policy"`        // 推荐值
    PermissionsPolicy    string `yaml:"permissions_policy"`     // 权限策略
}

// HSTSConfig HSTS 配置
type HSTSConfig struct {
    MaxAge            int  `yaml:"max_age"`             // 过期时间（秒），默认 31536000（1年）
    IncludeSubDomains bool `yaml:"include_sub_domains"` // 包含子域名，默认 true
    Preload           bool `yaml:"preload"`             // HSTS 预加载列表，默认 false
}
```

**默认安全头部**：
| 头部 | 默认值 | 说明 |
|------|--------|------|
| X-Frame-Options | DENY | 防止点击劫持，可配置为 SAMEORIGIN |
| X-Content-Type-Options | nosniff | 防止 MIME 类型嗅探 |
| Content-Security-Policy | 可配置 | **关键**：防止 XSS 攻击 |
| Strict-Transport-Security | max-age=31536000; includeSubDomains | HSTS，强制 HTTPS |
| Referrer-Policy | strict-origin-when-cross-origin | 控制引用信息泄露 |
| Permissions-Policy | 可配置 | 控制浏览器功能权限 |

**注意**：`X-XSS-Protection` 已被现代浏览器弃用，不再默认添加，重点依赖 CSP 防护。

**配置示例**：

```yaml
security:
  headers:
    x_frame_options: SAMEORIGIN # 或 DENY（默认）
    content_security_policy: "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'"
    hsts:
      max_age: 31536000 # 1年
      include_sub_domains: true # 包含子域名
      preload: false # 不加入预加载列表（需用户显式启用）
    referrer_policy: strict-origin-when-cross-origin
    permissions_policy: "geolocation=(), microphone=(), camera=()"
```

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
    flag: permanent # 301
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
  type: gzip # gzip/brotli/both
  level: 6 # 1-9
  min_size: 1024 # 最小 1KB 才压缩
  types: [text/html, text/css, application/json]
```

#### 5.3 缓存系统

**静态文件缓存**：

```go
// internal/cache/file_cache.go

// FileCache 文件描述符缓存
type FileCache struct {
    maxEntries int
    maxSize    int64         // 内存上限（新增）
    inactive   time.Duration
    entries    map[string]*FileEntry
    lruList    *list.List    // LRU 淘汰链表（新增）
}

// FileEntry 缓存条目
type FileEntry struct {
    fd         *os.File
    size       int64
    modTime    time.Time
    lastAccess time.Time
}
```

**代理响应缓存**：

```go
// internal/cache/proxy_cache.go

// ProxyCache 代理缓存
type ProxyCache struct {
    storage    Storage         // 内存/磁盘存储
    rules      []CacheRule
    maxAge     time.Duration
    cacheLock  bool            // 缓存锁开关（默认 true）
    lock       *sync.RWMutex   // 缓存锁，防止击穿
    pending    map[string]*chan struct{} // 正在生成的缓存项
}

// CacheRule 缓存规则
type CacheRule struct {
    Path      string
    Methods   []string
    Statuses  []int           // 可缓存的响应状态码
    MaxAge    time.Duration
}
```

**缓存锁机制（防击穿）**：

- 当多个请求同时请求同一个未缓存的资源时，只让一个请求去后端获取
- 其他请求等待第一个请求完成后从缓存读取
- 防止缓存击穿导致后端压力骤增

**配置示例**：

```yaml
cache:
  file:
    max_entries: 10000
    max_size: 256MB # 内存上限
    inactive: 20s
    lru_eviction: true # 启用 LRU 淘汰

  proxy:
    enabled: true
    storage: memory # memory/disk
    max_size: 1GB
    cache_lock: true # 防止缓存击穿
    stale_while_revalidate: 60s # 过期缓存复用
    rules:
      - path: /api/cacheable
        methods: [GET]
        statuses: [200, 301, 302]
        max_age: 10m
```

#### 5.4 日志系统

**扩展 Phase 2 的 zerolog 实现**，增加文件输出和访问/错误日志分离。

**实现**：

```go
// internal/logging/logging.go

import (
    "io"
    "github.com/rs/zerolog"
)

// Logger 日志管理器
type Logger struct {
    accessLog zerolog.Logger  // 访问日志
    errorLog  zerolog.Logger  // 错误日志
}

// New 创建日志管理器
func New(cfg *LoggingConfig) *Logger {
    // 访问日志：stdout 或文件
    accessOut := getOutput(cfg.Access.Path)
    accessLog := zerolog.New(accessOut).With().Timestamp().Logger()

    // 错误日志：stderr 或文件
    errorOut := getOutput(cfg.Error.Path)
    errorLevel := parseLevel(cfg.Error.Level)
    errorLog := zerolog.New(errorOut).Level(errorLevel).With().Timestamp().Logger()

    return &Logger{accessLog: accessLog, errorLog: errorLog}
}

// LogAccess 记录访问日志（nginx 格式变量）
func (l *Logger) LogAccess(r *http.Request, status int, size int64, duration time.Duration) {
    l.accessLog.Info().
        Str("remote_addr", r.RemoteAddr).
        Str("request", fmt.Sprintf("%s %s", r.Method, r.URL.Path)).
        Int("status", status).
        Int64("body_bytes_sent", size).
        Dur("request_time", duration).
        Msg("")
}

// getOutput 获取输出目标（stdout/stderr/文件）
func getOutput(path string) io.Writer {
    if path == "" {
        return os.Stdout
    }
    f, _ := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
    return f
}
```

**日志格式变量**（支持 nginx 风格配置）：
- `$remote_addr` - 客户端 IP
- `$request` - 请求行（方法 + 路径）
- `$status` - 响应状态码
- `$body_bytes_sent` - 响应体大小
- `$request_time` - 请求耗时

**配置示例**：

```yaml
logging:
  access:
    path: /var/log/lolly/access.log  # 留空则输出到 stdout
    format: json                     # json 或 text（Phase 2 ConsoleWriter）
  error:
    path: /var/log/lolly/error.log   # 留空则输出到 stderr
    level: info                      # debug/info/warn/error
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
    path: /_status # 状态端点路径
    allow: [127.0.0.1] # 仅允许本地访问
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

##### 6.3.1 连接复用

```go
// http.Transport 连接池配置
transport := &http.Transport{
    MaxIdleConns:        100,              // 最大空闲连接数
    MaxIdleConnsPerHost: 32,               // 每主机最大空闲连接
    IdleConnTimeout:     90 * time.Second, // 空闲连接超时
    MaxConnsPerHost:     0,                // 每主机最大连接数（0=无限制）
}
```

##### 6.3.2 缓冲池

```go
// 使用 sync.Pool 实现分级缓冲池
var bufferPool = sync.Pool{
    New: func() interface{} {
        return make([]byte, 32*1024) // 32KB 缓冲区
    },
}
```

##### 6.3.3 零拷贝（sendfile）

```go
// internal/handler/sendfile.go

// SendFile 零拷贝文件传输（仅 Linux）
// 大文件（>= 8KB）使用 sendfile 系统调用
func SendFile(w http.ResponseWriter, f *os.File, offset, length int64) error {
    // Linux: syscall.Sendfile
    // macOS: syscall.Sendfile（不同签名）
    // Windows: syscall.TransmitFile
    // 其他平台: 降级为 io.Copy
}

// 跨平台兼容方案
// - Linux: sendfile(out_fd, in_fd, offset, count)
// - macOS: sendfile(in_fd, out_fd, offset, &len, sf_hdtr, flags)
// - Windows: TransmitFile(socket, handle, bytes_to_write, ...
// - Fallback: io.CopyBuffer
```

##### 6.3.4 Goroutine 池（可选）

```go
// internal/server/pool.go

// GoroutinePool Goroutine 池配置
type GoroutinePool struct {
    maxWorkers   int           // 最大 worker 数
    minWorkers   int           // 最小 worker 数（预热）
    idleTimeout  time.Duration // 空闲超时
    taskQueue    chan Task     // 任务队列
}

// 配置示例
performance:
  goroutine_pool:
    enabled: true          // 启用池化（高 QPS 场景推荐）
    max_workers: 10000     // 最大并发数
    min_workers: 100       // 预热 worker 数
    idle_timeout: 60s      // 空闲超时
```

##### 6.3.5 对象池

```go
// 使用 sync.Pool 复用对象
var requestPool = sync.Pool{
    New: func() interface{} {
        return new(Request)
    },
}
```

##### 6.3.6 代理缓存锁（防击穿）

```go
// internal/cache/proxy_cache.go

// ProxyCache 代理缓存（增加缓存锁）
type ProxyCache struct {
    storage   Storage
    rules     []CacheRule
    maxAge    time.Duration
    lock      *sync.RWMutex     // 缓存锁，防止缓存击穿
    pending   map[string]*chan struct{} // 正在生成的缓存项
}

// 缓存锁机制：
// 1. 请求到达时检查是否有 pending 请求
// 2. 有则等待 pending 完成
// 3. 无则创建 pending，生成缓存后广播
```

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

---

## 文件依赖关系图

```
Phase 1:
  cmd/lolly/main.go → internal/config/config.go
  cmd/lolly/main.go → internal/middleware/middleware.go

Phase 2:
  internal/server/server.go → internal/config/config.go
  internal/server/server.go → internal/handler/router.go
  internal/server/server.go → internal/logging/logging.go
  internal/handler/router.go → internal/handler/static.go
  internal/handler/router.go → internal/middleware/middleware.go

Phase 3:
  internal/handler/router.go → internal/proxy/proxy.go
  internal/proxy/proxy.go → internal/loadbalance/balancer.go
  internal/proxy/proxy.go → internal/proxy/health.go

Phase 4:
  internal/server/server.go → internal/ssl/ssl.go
  internal/middleware/middleware.go → internal/middleware/security/access.go
  internal/middleware/middleware.go → internal/middleware/security/ratelimit.go
  internal/middleware/middleware.go → internal/middleware/security/auth.go
  internal/middleware/middleware.go → internal/middleware/security/headers.go

Phase 5:
  internal/middleware/middleware.go → internal/middleware/rewrite/rewrite.go
  internal/middleware/middleware.go → internal/middleware/compression/compression.go
  internal/proxy/proxy.go → internal/cache/proxy_cache.go
  internal/server/server.go → internal/logging/logging.go（扩展）

Phase 6:
  cmd/lolly/main.go → internal/stream/stream.go
  internal/server/server.go → internal/server/upgrade.go
  internal/server/server.go → internal/server/pool.go（可选）
```

---

## 总体进度追踪

| 阶段    | 状态   | 主要功能                  |
| ------- | ------ | ------------------------- |
| Phase 1 | ✅ 完成 | 项目骨架、配置系统        |
| Phase 2 | ✅ 完成 | HTTP 核心、静态文件、路由 |
| Phase 3 | ⏳ 待开始 | 反向代理、负载均衡        |
| Phase 4 | ⏳ 待开始 | SSL/TLS、安全控制         |
| Phase 5 | ⏳ 待开始 | 重写、压缩、缓存、日志    |
| Phase 6 | ⏳ 待开始 | Stream、性能优化          |

**Phase 2 技术选型变更**：
- HTTP 库：使用 [fasthttp](https://github.com/valyala/fasthttp) 替代 `net/http`（性能提升 6 倍）
- 日志库：使用 [zerolog](https://github.com/rs/zerolog)（零分配，~40ns/op）

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
