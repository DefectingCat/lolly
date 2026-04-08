# 配置参考文档

## 目录

- [变量系统](#变量系统)
- [DNS 解析器](#dns-解析器)
- [访问日志格式](#访问日志格式)

---

## 变量系统

Lolly 支持 nginx 风格的变量系统，可用于访问日志格式、代理请求头和 URL 重写规则。

### 内置变量

| 变量名 | 说明 | 示例值 |
|--------|------|--------|
| `$host` | 请求的主机名（Host 头） | `example.com` |
| `$remote_addr` | 客户端 IP 地址 | `192.168.1.1` |
| `$remote_port` | 客户端端口 | `54321` |
| `$request_uri` | 原始请求 URI（包含查询参数） | `/api/users?page=1` |
| `$uri` | 解码后的 URI 路径 | `/api/users` |
| `$args` | 查询参数字符串 | `page=1&limit=10` |
| `$request_method` | HTTP 请求方法 | `GET`, `POST` |
| `$scheme` | 协议 | `http`, `https` |
| `$server_name` | 服务器名称 | `localhost` |
| `$server_port` | 服务器端口 | `8080` |
| `$status` | HTTP 响应状态码 | `200`, `404` |
| `$body_bytes_sent` | 发送的响应体字节数 | `1024` |
| `$request_time` | 请求处理时间（秒） | `0.050` |
| `$time_local` | 本地时间 | `08/Apr/2026:11:04:58 +0800` |
| `$time_iso8601` | ISO8601 格式时间 | `2026-04-08T11:04:58+08:00` |
| `$request_id` | 唯一请求标识符 | `uuid` |

### 动态 HTTP 头变量

以 `$http_` 开头的变量用于获取 HTTP 请求头值：

- `$http_user_agent` - User-Agent 头
- `$http_referer` - Referer 头
- `$http_x_forwarded_for` - X-Forwarded-For 头
- 其他任意请求头：`$http_header_name`

### 变量格式

支持两种格式：

1. **简单格式**: `$var`
   ```
   $host $uri
   ```

2. **花括号格式**: `${var}`
   ```
   ${host}:8080
   ${scheme}://${host}${uri}
   ```

### 在代理请求头中使用变量

```yaml
proxy:
  - path: /api
    targets:
      - url: http://backend:8080
    headers:
      set_request:
        X-Real-IP: "$remote_addr"
        X-Forwarded-Host: "$host"
        X-Request-ID: "$request_id"
```

### 在访问日志中使用变量

```yaml
logging:
  access:
    format: '$remote_addr - $remote_user [$time_local] "$request_method $uri $scheme" $status $body_bytes_sent'
```

### 自定义变量

```yaml
variables:
  set:
    app_name: "lolly"
    version: "1.0.0"
  request_id: true  # 自动生成唯一请求 ID
```

---

## DNS 解析器

Lolly 内置 DNS 解析器，支持动态解析后端服务域名。

### 配置选项

```yaml
resolver:
  enabled: true              # 是否启用
  addresses:                 # DNS 服务器地址列表
    - "8.8.8.8:53"
    - "8.8.4.4:53"
  valid: 30s                 # 缓存有效期（TTL）
  timeout: 5s                # DNS 查询超时
  ipv4: true                 # 查询 IPv4 地址
  ipv6: false                # 查询 IPv6 地址
  cache_size: 1024           # 缓存最大条目数
```

### 功能特性

- **DNS 缓存**: 按 TTL 缓存解析结果，减少 DNS 查询延迟
- **后台刷新**: 自动在 TTL/2 时刷新缓存，避免过期
- **故障转移**: 解析失败时使用缓存 IP 继续服务
- **健康检查**: 首次解析失败标记目标不健康

### 使用场景

当后端目标使用域名时，DNS 解析器自动生效：

```yaml
proxy:
  - path: /api
    targets:
      - url: http://backend.example.com:8080  # 使用域名
        weight: 1
```

### 监控指标

通过状态端点获取 DNS 解析统计：

- `CacheHits` - 缓存命中次数
- `CacheMisses` - 缓存未命中次数
- `CacheEntries` - 当前缓存条目数
- `ResolveErrors` - 解析错误次数
- `AverageLatency` - 平均解析延迟

---

## 访问日志格式

### nginx 兼容格式

Lolly 默认提供 nginx 兼容的访问日志格式：

```yaml
logging:
  access:
    format: '$remote_addr - $remote_user [$time] "$request" $status $body_bytes_sent "$http_referer" "$http_user_agent"'
```

示例输出：
```
192.168.1.1 - - [08/Apr/2026:11:04:58 +0800] "GET /api/users HTTP/1.1" 200 1024 "-" "Mozilla/5.0"
```

### JSON 格式

设置格式为 `json` 输出结构化日志：

```yaml
logging:
  access:
    format: 'json'
```

示例输出：
```json
{
  "remote_addr": "192.168.1.1",
  "request": "GET /api/users HTTP/1.1",
  "status": 200,
  "body_bytes_sent": 1024,
  "http_user_agent": "Mozilla/5.0"
}
```

### 自定义格式

使用变量创建自定义格式：

```yaml
logging:
  access:
    format: '$remote_addr $request_method $uri $status $request_time'
```

---

## 完整配置示例

```yaml
server:
  listen: ":8080"
  name: "localhost"

  proxy:
    - path: /api
      targets:
        - url: http://backend.example.com:8080
      headers:
        set_request:
          X-Real-IP: "$remote_addr"
          X-Forwarded-Host: "$host"
          X-Request-ID: "$request_id"

resolver:
  enabled: true
  addresses:
    - "8.8.8.8:53"
  valid: 30s
  timeout: 5s

variables:
  set:
    app_name: "lolly"
  request_id: true

logging:
  access:
    format: '$remote_addr - $remote_user [$time_local] "$request_method $uri $scheme" $status $body_bytes_sent'
    path: "/var/log/lolly/access.log"
  error:
    level: "info"
    path: "/var/log/lolly/error.log"
```
