# Lolly 项目需求文档

> 本文档用于指导 AI 实现一个高性能 HTTP 服务器项目。

## 一、项目定位

创建一个类似 nginx 的高性能 HTTP 服务器，项目名称为 **lolly**。

### 核心目标

- 实现与 nginx 相同的核心功能，但**不是 1:1 复刻**
- 比 nginx 更加现代、更加易用
- 充分利用 Go 语言特性（goroutine、channel、标准库等）
- 保持高性能（高并发、低内存消耗）

### 技术约束

| 约束项 | 要求 |
|--------|------|
| 语言 | 纯 Go 实现，不依赖 C 库 |
| 部署 | 静态链接，单二进制文件即可运行 |
| 配置 | YAML 格式（比 nginx 的 nginx.conf 更简单易用） |
| 兼容性 | 跨平台支持（Linux、macOS、Windows） |

---

## 二、功能需求

### 必须实现的核心功能（参考 docs/ 目录详细文档）

#### 2.1 HTTP 核心模块（优先级：最高）

参考：`docs/03-nginx-http-core.md`

- **Server/Location 配置**：虚拟主机、请求路由
- **静态文件服务**：高效 serving 静态资源
- **请求处理**：请求头、请求体、超时控制
- **keep-alive**：长连接支持
- **MIME 类型**：自动识别内容类型

#### 2.2 反向代理与负载均衡（优先级：高）

参考：`docs/04-nginx-proxy-loadbalancing.md`

- **反向代理**：proxy_pass 功能
- **负载均衡算法**：轮询、权重、最少连接、IP 哈希
- **健康检查**：主动/被动健康检测
- **WebSocket 支持**：Upgrade 协议处理
- **代理缓存**：响应缓存机制

#### 2.3 SSL/TLS 与 HTTPS（优先级：高）

参考：`docs/05-nginx-ssl-https.md`

- **HTTPS 服务**：证书配置
- **SSL 会话缓存**：减少握手开销
- **HTTP/2 支持**：现代化协议
- **自动证书管理**：可选 ACME/Let's Encrypt 集成

#### 2.4 URL 重写与请求处理（优先级：中）

参考：`docs/06-nginx-rewrite.md`

- **URL 重写**：灵活的路由规则
- **重定向**：301/302 等状态码
- **请求修改**：添加/修改请求头

#### 2.5 压缩与缓存（优先级：中）

参考：`docs/07-nginx-compression-caching.md`

- **Gzip/Brotli 压缩**：响应压缩
- **静态文件缓存**：文件描述符缓存
- **代理响应缓存**：缓存代理请求结果

#### 2.6 日志与监控（优先级：中）

参考：`docs/08-nginx-logging-monitoring.md`

- **访问日志**：可定制日志格式
- **错误日志**：分级日志
- **状态监控**：实时状态端点（类似 stub_status）
- **日志轮转**：内置支持

#### 2.7 安全与访问控制（优先级：高）

参考：`docs/09-nginx-security.md`

- **IP 访问控制**：白名单/黑名单
- **请求限制**：速率限制、连接限制
- **安全头部**：自动添加安全相关 HTTP 头
- **基础认证**：Basic Auth 支持

#### 2.8 TCP/UDP Stream 代理（优先级：低）

参考：`docs/10-nginx-stream-tcp-udp.md`

- **TCP 代理**：四层代理支持
- **UDP 代理**：UDP 流处理
- **Stream 负载均衡**：四层负载均衡

#### 2.9 邮件代理（优先级：最低，可选）

参考：`docs/11-nginx-mail-proxy.md`

- IMAP/POP3/SMTP 代理（可后期实现或不实现）

---

## 三、配置文件设计

### 设计原则

配置文件采用 YAML 格式，设计原则：

1. **简洁易用**：比 nginx.conf 更直观
2. **结构清晰**：层级明确，易于理解
3. **类型安全**：配置值有明确类型
4. **默认合理**：开箱即用，最小配置即可运行

### 配置文件示例（仅供参考）

```yaml
# lolly.yaml - 最简配置示例
server:
  listen: 8080
  name: example.com

  # 静态文件服务
  static:
    root: /var/www/html
    index: [index.html, index.htm]

  # 反向代理
  proxy:
    - path: /api
      target: http://backend:8080
      load_balance: round_robin

  # SSL/HTTPS
  ssl:
    cert: /etc/ssl/cert.pem
    key: /etc/ssl/key.pem

# 日志配置
logging:
  access: /var/log/lolly/access.log
  error: /var/log/lolly/error.log
  level: info

# 性能配置
performance:
  workers: auto  # 自动匹配 CPU 核心数
  max_connections: 10000
  keepalive_timeout: 60s
```

### 配置与 nginx 的对比

| nginx 配置 | lolly 配置（预期） |
|------------|-------------------|
| `worker_processes auto;` | `performance.workers: auto` |
| `listen 80;` | `server.listen: 80` |
| `root /var/www;` | `server.static.root: /var/www` |
| `proxy_pass http://backend;` | `server.proxy.target: http://backend` |
| `ssl_certificate cert.pem;` | `server.ssl.cert: cert.pem` |

---

## 四、架构设计要求

### 4.1 Go 特性利用

充分利用 Go 语言特性：

- **Goroutine**：每个连接一个 goroutine，天然高并发
- **Channel**：进程内通信、信号处理
- **标准库**：`net/http`、`net`、`io`、`sync` 等
- **Context**：请求超时控制、取消传播
- **接口**：模块化设计，接口抽象

### 4.2 模块化设计

```
lolly/
├── cmd/
│   └── lolly/
│       └── main.go          # 入口
├── internal/
│   ├── config/              # 配置解析
│   ├── server/              # HTTP 服务器核心
│   ├── proxy/               # 反向代理
│   ├── loadbalance/         # 负载均衡算法
│   ├── ssl/                 # SSL/TLS 支持
│   ├── rewrite/             # URL 重写
│   ├── cache/               # 缓存模块
│   ├── compression/         # 压缩模块
│   ├── logging/             # 日志模块
│   ├── security/            # 安全模块
│   ├── stream/              # TCP/UDP 代理
│   └── middleware/          # 中间件系统
├── pkg/
│   └── utils/               # 公共工具
└── configs/
    └── lolly.yaml           # 默认配置
```

### 4.3 性能要求

- 单机支持数万并发连接
- 低内存消耗（相比 nginx 同等功能）
- 静态文件高效传输（利用 `sendfile` 等优化）
- 连接复用、缓冲池

---

## 五、开发规范

### 5.1 提交规范

每完成一个功能点提交一次，提交格式遵循最佳实践：

```
<type>(<scope>): <subject>

<body>

<footer>
```

**type 类型**：
- `feat`: 新功能
- `fix`: 修复 bug
- `docs`: 文档变更
- `refactor`: 重构
- `test`: 测试相关
- `chore`: 构建/工具相关

**示例**：
```
feat(server): 实现静态文件服务功能

- 支持配置根目录和索引文件
- 实现 MIME 类型自动识别
- 支持 keep-alive 长连接

Closes #1
```

### 5.2 代码注释规范

严格遵循 `docs/comments.md` 规范：

- **中文注释**：所有注释使用中文
- **文件头注释**：每个文件必须有文件头注释
- **函数注释**：说明用途、参数、返回值、注意事项
- **说明"为什么"**：注释解释原因，而非仅描述代码
- **TODO/FIXME**：使用标准格式，标注负责人和时间

### 5.3 实现顺序建议

建议按以下顺序逐步实现：

1. **第一阶段**：项目骨架 + 配置解析
   - 项目目录结构
   - YAML 配置解析
   - 基本命令行参数

2. **第二阶段**：HTTP 核心功能
   - 基础 HTTP 服务器
   - Server/Location 配置
   - 静态文件服务

3. **第三阶段**：代理与负载均衡
   - 反向代理
   - 负载均衡算法
   - 健康检查

4. **第四阶段**：安全与 SSL
   - SSL/TLS 支持
   - IP 访问控制
   - 请求限制

5. **第五阶段**：增强功能
   - URL 重写
   - 压缩
   - 缓存
   - 日志系统

6. **第六阶段**：高级功能
   - TCP/UDP Stream
   - 性能优化
   - 监控端点

---

## 六、参考资料

| 资料位置 | 内容 |
|----------|------|
| `docs/01-nginx-overview.md` | nginx 架构、信号控制、命令行参数 |
| `docs/02-nginx-installation.md` | 安装构建（参考模块结构） |
| `docs/03-nginx-http-core.md` | HTTP 核心模块详细功能 |
| `docs/04-nginx-proxy-loadbalancing.md` | 反向代理、负载均衡详细功能 |
| `docs/05-nginx-ssl-https.md` | SSL/HTTPS 配置详细功能 |
| `docs/06-nginx-rewrite.md` | URL 重写详细功能 |
| `docs/07-nginx-compression-caching.md` | 压缩、缓存详细功能 |
| `docs/08-nginx-logging-monitoring.md` | 日志、监控详细功能 |
| `docs/09-nginx-security.md` | 安全功能详细说明 |
| `docs/10-nginx-stream-tcp-udp.md` | TCP/UDP 代理功能 |
| `docs/11-nginx-mail-proxy.md` | 邮件代理（可选参考） |
| `docs/12-nginx-performance-tuning.md` | 性能优化参考 |
| `docs/comments.md` | Go 代码注释规范（必须遵循） |

---

## 七、其他说明

- 本项目是学习/实验性质的高性能服务器实现
- 优先保证代码质量和可维护性
- 功能完整性优先于极致性能优化
- 遇到设计决策问题时，选择"更简单易用"的方案