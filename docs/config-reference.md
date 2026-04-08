# Lolly 配置文件完整字段清单

> 本文档列出所有支持的配置字段及其枚举值，与代码定义完全同步。

## 顶层配置

| 字段 | 类型 | 说明 |
|------|------|------|
| `server` | ServerConfig | 单服务器模式配置 |
| `servers` | []ServerConfig | 多虚拟主机模式配置 |
| `stream` | []StreamConfig | TCP/UDP Stream 代理配置 |
| `http3` | HTTP3Config | HTTP/3 (QUIC) 配置 |
| `logging` | LoggingConfig | 日志配置 |
| `performance` | PerformanceConfig | 性能配置 |
| `monitoring` | MonitoringConfig | 监控配置 |

---

## ServerConfig 服务器配置

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `listen` | string | `:8080` | 监听地址，格式 `host:port` 或 `:port` |
| `name` | string | `localhost` | 服务器名称，虚拟主机匹配 |
| `static` | []StaticConfig | - | 静态文件服务配置 |
| `proxy` | []ProxyConfig | - | 反向代理规则列表 |
| `ssl` | SSLConfig | - | SSL/TLS 配置 |
| `security` | SecurityConfig | - | 安全配置 |
| `rewrite` | []RewriteRule | - | URL 重写规则 |
| `compression` | CompressionConfig | - | 响应压缩配置 |
| `read_timeout` | duration | `30s` | 读取超时 |
| `write_timeout` | duration | `30s` | 写入超时 |
| `idle_timeout` | duration | `120s` | 空闲超时 |
| `max_conns_per_ip` | int | `1000` | 每 IP 最大连接数 |
| `max_requests_per_conn` | int | `10000` | 每连接最大请求数 |
| `client_max_body_size` | string | `1MB` | **请求体大小限制** |

### client_max_body_size 格式

支持以下格式：
- 纯数字：表示字节数，如 `1048576`
- 带单位：`1b`, `10kb`, `5mb`, `1gb`
- 默认值：`1MB`

---

## StaticConfig 静态文件配置

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `path` | string | `/` | 匹配路径前缀 |
| `root` | string | - | 静态文件根目录 |
| `index` | []string | `["index.html", "index.htm"]` | 索引文件列表 |
| `try_files` | []string | - | **按顺序尝试查找的文件，支持 SPA** |
| `try_files_pass` | bool | `false` | **内部重定向是否触发中间件** |

### try_files 示例

```yaml
static:
  - path: "/"
    root: "/var/www/html"
    index: ["index.html"]
    try_files: ["$uri", "$uri/", "/index.html"]  # SPA 部署
    try_files_pass: false  # 内部重定向不触发中间件
```

**占位符支持：**
- `$uri` - 原始请求 URI
- `$uri/` - URI 作为目录

---

## ProxyConfig 反向代理配置

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `path` | string | **必填** | 匹配路径前缀 |
| `targets` | []ProxyTarget | **必填** | 后端目标列表 |
| `load_balance` | string | `round_robin` | 负载均衡算法 |
| `hash_key` | string | - | 一致性哈希键 |
| `virtual_nodes` | int | `150` | 一致性哈希虚拟节点数 |
| `health_check` | HealthCheckConfig | - | 健康检查配置 |
| `timeout` | ProxyTimeout | - | 超时配置 |
| `headers` | ProxyHeaders | - | 请求/响应头修改 |
| `cache` | ProxyCacheConfig | - | 代理缓存配置 |
| `client_max_body_size` | string | - | **请求体大小限制（覆盖全局）** |
| `next_upstream` | NextUpstreamConfig | - | 故障转移配置 |

### load_balance 枚举值

| 值 | 说明 |
|-----|------|
| `round_robin` | 轮询（默认） |
| `weighted_round_robin` | 加权轮询 |
| `least_conn` | 最少连接 |
| `ip_hash` | IP 哈希 |
| `consistent_hash` | 一致性哈希 |

### hash_key 枚举值

| 值 | 说明 |
|-----|------|
| `ip` | 客户端 IP |
| `uri` | 请求 URI |
| `header:X-Name` | 指定请求头的值 |

---

## SSLConfig SSL/TLS 配置

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `cert` | string | - | 证书文件路径（PEM 格式） |
| `key` | string | - | 私钥文件路径（PEM 格式） |
| `cert_chain` | string | - | 证书链文件路径 |
| `protocols` | []string | `["TLSv1.2", "TLSv1.3"]` | TLS 版本 |
| `ciphers` | []string | - | 加密套件（仅 TLS 1.2 有效） |
| `ocsp_stapling` | bool | `false` | OCSP Stapling |
| `hsts` | HSTSConfig | - | HSTS 配置 |

### protocols 枚举值

| 值 | 说明 |
|-----|------|
| `TLSv1.2` | TLS 1.2 |
| `TLSv1.3` | TLS 1.3（推荐） |

**注意**：不支持 `TLSv1.0` 和 `TLSv1.1`（已废弃）

### ciphers 安全要求

拒绝包含以下关键字的不安全套件：
- `RC4`
- `DES`
- `3DES`
- `CBC`

推荐套件：
```
ECDHE-ECDSA-AES256-GCM-SHA384
ECDHE-RSA-AES256-GCM-SHA384
ECDHE-ECDSA-CHACHA20-POLY1305
ECDHE-RSA-CHACHA20-POLY1305
```

---

## HSTSConfig HSTS 配置

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `max_age` | int | `31536000` | 过期时间（秒），1年 |
| `include_sub_domains` | bool | `true` | 包含子域名 |
| `preload` | bool | `false` | 加入 HSTS 预加载列表 |

---

## SecurityConfig 安全配置

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `access` | AccessConfig | - | IP 访问控制 |
| `rate_limit` | RateLimitConfig | - | 速率限制 |
| `auth` | AuthConfig | - | 认证配置 |
| `headers` | SecurityHeaders | - | 安全头部 |
| `error_page` | ErrorPageConfig | - | **自定义错误页面** |

---

## ErrorPageConfig 错误页面配置

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `pages` | map[int]string | - | 状态码到错误页面映射 |
| `default` | string | - | 默认错误页面 |
| `response_code` | int | `0` | 响应状态码覆盖 |

### 示例

```yaml
security:
  error_page:
    pages:
      404: "/var/www/errors/404.html"
      500: "/var/www/errors/500.html"
      502: "/var/www/errors/502.html"
      503: "/var/www/errors/503.html"
    default: "/var/www/errors/error.html"
    response_code: 200  # 可选：所有错误返回 200
```

---

## AccessConfig 访问控制配置

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `allow` | []string | `[]` | 允许的 IP/CIDR 列表 |
| `deny` | []string | `[]` | 拒绝的 IP/CIDR 列表 |
| `default` | string | `allow` | 默认动作 |
| `trusted_proxies` | []string | `[]` | 可信代理 CIDR |

### default 枚举值

| 值 | 说明 |
|-----|------|
| `allow` | 默认允许 |
| `deny` | 默认拒绝 |

---

## RateLimitConfig 速率限制配置

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `request_rate` | int | `0` | 每秒请求数限制（0=不限制） |
| `burst` | int | `0` | 突发上限 |
| `conn_limit` | int | `0` | 连接数限制 |
| `key` | string | `ip` | 限流 key 来源 |
| `algorithm` | string | `token_bucket` | 限流算法 |
| `sliding_window_mode` | string | `approximate` | 滑动窗口模式 |
| `sliding_window` | int | `60` | 滑动窗口大小（秒） |

### key 枚举值

| 值 | 说明 |
|-----|------|
| `ip` | 客户端 IP |
| `header` | 请求头值 |

### algorithm 枚举值

| 值 | 说明 |
|-----|------|
| `token_bucket` | 令牌桶（默认） |
| `sliding_window` | 滑动窗口 |

### sliding_window_mode 枚举值

| 值 | 说明 |
|-----|------|
| `approximate` | 近似模式（高性能） |
| `precise` | 精确模式（高精度） |

---

## AuthConfig 认证配置

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `type` | string | - | 认证类型（空=禁用） |
| `require_tls` | bool | `true` | 强制 HTTPS |
| `algorithm` | string | `bcrypt` | 密码哈希算法 |
| `users` | []User | - | 用户列表 |
| `realm` | string | `Restricted Area` | 认证域 |
| `min_password_length` | int | `8` | 密码最小长度 |

### type 枚举值

| 值 | 说明 |
|-----|------|
| ` ` (空) | 禁用认证 |
| `basic` | HTTP Basic 认证 |

### algorithm 枚举值

| 值 | 说明 |
|-----|------|
| `bcrypt` | bcrypt 算法（默认） |
| `argon2id` | Argon2id 算法 |

---

## SecurityHeaders 安全头部配置

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `x_frame_options` | string | `DENY` | 防止点击劫持 |
| `x_content_type_options` | string | `nosniff` | 防止 MIME 嗅探 |
| `content_security_policy` | string | - | CSP 策略 |
| `referrer_policy` | string | `strict-origin-when-cross-origin` | 引用策略 |
| `permissions_policy` | string | - | 权限策略 |

### x_frame_options 枚举值

| 值 | 说明 |
|-----|------|
| ` ` (空) | 禁用 |
| `DENY` | 完全禁止嵌入 |
| `SAMEORIGIN` | 仅允许同源嵌入 |

### referrer_policy 枚举值

| 值 | 说明 |
|-----|------|
| `no-referrer` | 不发送 Referer |
| `no-referrer-when-downgrade` | 降级时不发送 |
| `origin` | 仅发送源 |
| `origin-when-cross-origin` | 跨域时仅发送源 |
| `same-origin` | 同源时发送完整 |
| `strict-origin` | 严格模式仅发送源 |
| `strict-origin-when-cross-origin` | 默认值 |
| `unsafe-url` | 始终发送完整 URL |

---

## RewriteRule URL 重写规则

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `pattern` | string | **必填** | 正则匹配模式 |
| `replacement` | string | **必填** | 替换目标 |
| `flag` | string | - | 标志 |

### flag 枚举值

| 值 | 说明 |
|-----|------|
| ` ` (空) | 继续匹配后续规则 |
| `last` | 停止匹配后续规则 |
| `break` | 停止匹配但继续处理 |
| `redirect` | 302 临时重定向 |
| `permanent` | 301 永久重定向 |

---

## CompressionConfig 压缩配置

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `type` | string | `gzip` | 压缩类型 |
| `level` | int | `6` | 压缩级别（0-9） |
| `min_size` | int | `1024` | 最小压缩大小（字节） |
| `types` | []string | 见下方 | 可压缩的 MIME 类型 |
| `gzip_static` | bool | `false` | 启用预压缩文件支持 |
| `gzip_static_extensions` | []string | `[".br", ".gz"]` | 预压缩文件扩展名 |

### type 枚举值

| 值 | 说明 |
|-----|------|
| ` ` (空) | 禁用压缩 |
| `gzip` | 仅 gzip |
| `brotli` | 仅 brotli |
| `both` | gzip + brotli |

### 默认可压缩 MIME 类型

```yaml
types:
  - "text/html"
  - "text/css"
  - "text/javascript"
  - "application/json"
  - "application/javascript"
```

---

## NextUpstreamConfig 故障转移配置

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `tries` | int | `1` | 最大尝试次数 |
| `http_codes` | []int | - | 触发重试的 HTTP 状态码 |

**注意**：`tries=1` 表示禁用故障转移

### 示例

```yaml
next_upstream:
  tries: 3
  http_codes: [502, 503, 504]
```

---

## StreamConfig TCP/UDP Stream 代理

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `listen` | string | **必填** | 监听地址 |
| `protocol` | string | **必填** | 协议类型 |
| `upstream` | StreamUpstream | - | 上游配置 |

### protocol 枚举值

| 值 | 说明 |
|-----|------|
| `tcp` | TCP 协议 |
| `udp` | UDP 协议 |

---

## HTTP3Config HTTP/3 配置

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `enabled` | bool | `false` | 是否启用 |
| `listen` | string | `:443` | UDP 监听地址 |
| `max_streams` | int | `100` | 最大并发流 |
| `idle_timeout` | duration | `60s` | 空闲超时 |
| `enable_0rtt` | bool | `false` | 启用 0-RTT |

---

## LoggingConfig 日志配置

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `format` | string | `text` | 全局日志格式 |
| `access` | AccessLogConfig | - | 访问日志配置 |
| `error` | ErrorLogConfig | - | 错误日志配置 |

### format 枚举值

| 值 | 说明 |
|-----|------|
| `text` | 文本格式（默认） |
| `json` | JSON 格式 |

### access.format 支持变量

| 变量 | 说明 |
|------|------|
| `$remote_addr` | 客户端 IP |
| `$remote_user` | 认证用户名 |
| `$request` | 请求行 |
| `$status` | 响应状态码 |
| `$body_bytes_sent` | 响应体大小 |
| `$request_time` | 请求处理时间 |
| `$http_referer` | Referer 头 |
| `$http_user_agent` | User-Agent 头 |
| `$time` | 时间戳（RFC3339） |

### error.level 枚举值

| 值 | 说明 |
|-----|------|
| `debug` | 调试级别 |
| `info` | 信息级别（默认） |
| `warn` | 警告级别 |
| `error` | 错误级别 |

---

## PerformanceConfig 性能配置

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `goroutine_pool` | GoroutinePoolConfig | - | Goroutine 池配置 |
| `file_cache` | FileCacheConfig | - | 文件缓存配置 |
| `transport` | TransportConfig | - | HTTP Transport 配置 |

### GoroutinePoolConfig

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `enabled` | bool | `false` | 是否启用 |
| `max_workers` | int | `1000` | 最大 worker 数 |
| `min_workers` | int | `10` | 最小 worker 数（预热） |
| `idle_timeout` | duration | `60s` | 空闲超时 |

### FileCacheConfig

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `max_entries` | int64 | `10000` | 最大缓存条目数 |
| `max_size` | int64 | `268435456` (256MB) | 内存上限 |
| `inactive` | duration | `20s` | 未访问淘汰时间 |

### TransportConfig

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `max_idle_conns` | int | `100` | 最大空闲连接数 |
| `max_idle_conns_per_host` | int | `32` | 每主机空闲连接 |
| `idle_conn_timeout` | duration | `90s` | 空闲超时 |
| `max_conns_per_host` | int | `0` | 每主机最大连接（0=不限制） |

---

## MonitoringConfig 监控配置

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `status` | StatusConfig | - | 状态端点配置 |
| `pprof` | PprofConfig | - | pprof 配置 |

### StatusConfig

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `path` | string | `/_status` | 端点路径 |
| `allow` | []string | `["127.0.0.1"]` | 允许访问的 IP |

### PprofConfig

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `enabled` | bool | `false` | 是否启用 |
| `path` | string | `/debug/pprof` | 端点路径前缀 |
| `allow` | []string | `["127.0.0.1"]` | 允许访问的 IP |

---

## 变更记录

- 2026-04-07: 首次创建，发现 `--generate-config` 缺少 `client_max_body_size`、`try_files`、`try_files_pass`、`error_page` 字段