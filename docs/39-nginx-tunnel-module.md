# NGINX HTTP Tunnel 模块文档

## 1. 模块概述

### 1.1 简介

`ngx_http_tunnel_module` 是 NGINX 的商业模块，用于处理 HTTP CONNECT 请求并建立端到端的虚拟连接隧道。

**版本要求**: NGINX 1.29.3 及以上

**授权**: 仅作为 F5 NGINX 商业订阅的一部分提供，开源版本不包含此模块

### 1.2 核心用途

- **HTTP 代理隧道**: 处理 RFC 9110 定义的 CONNECT 方法，用于建立 HTTPS 代理隧道
- **TCP 穿透**: 允许客户端通过 HTTP 代理与后端 TCP 服务建立直接连接
- **动态路由**: 支持变量实现动态目标地址和绑定地址

### 1.3 配置上下文

支持以下配置块：
- `http`
- `server`
- `location`

---

## 2. 指令详解

### 2.1 核心指令

#### tunnel_pass

```nginx
tunnel_pass [address];
```

| 属性 | 说明 |
|------|------|
| **默认值** | 无（必须显式配置） |
| **上下文** | http, server, location |
| **支持变量** | 是 |

**说明**:
- 启用 CONNECT 请求处理
- 默认目标地址为 `$host:$request_port`
- `address` 可以是：域名、IP 地址、端口、UNIX 套接字路径或上游服务器组名称
- 支持变量实现动态路由

**示例**:
```nginx
tunnel_pass $host:$request_port;
tunnel_pass backend_upstream;
tunnel_pass 127.0.0.1:8443;
```

---

#### tunnel_allow_upstream

```nginx
tunnel_allow_upstream string ...;
```

| 属性 | 说明 |
|------|------|
| **默认值** | 无 |
| **上下文** | http, server, location |
| **支持变量** | 是 |

**说明**:
- 定义允许访问后端服务器的条件
- 所有参数必须非空且不等于 "0" 才允许连接
- 每次建立连接前都会评估

**示例**:
```nginx
tunnel_allow_upstream $allow_port $allow_host;
```

---

#### tunnel_bind

```nginx
tunnel_bind address | off;
```

| 属性 | 说明 |
|------|------|
| **默认值** | 无 |
| **上下文** | http, server, location |
| **支持变量** | 是 |

**说明**:
- 指定出站连接到后端服务器时使用的本地 IP 地址（可选端口）
- `off` 取消从上级配置继承的效果

**示例**:
```nginx
tunnel_bind 192.168.1.100;
tunnel_bind $local_ip:$local_port;
```

---

#### tunnel_bind_dynamic

```nginx
tunnel_bind_dynamic on | off;
```

| 属性 | 说明 |
|------|------|
| **默认值** | `off` |
| **上下文** | http, server, location |

**说明**:
- 启用后，每次连接尝试时都会执行 `tunnel_bind` 操作
- 适用于 `tunnel_bind` 中使用动态变量的场景

---

#### tunnel_socket_keepalive

```nginx
tunnel_socket_keepalive on | off;
```

| 属性 | 说明 |
|------|------|
| **默认值** | `off` |
| **上下文** | http, server, location |

**说明**:
- 配置出站连接的 TCP keepalive 行为
- `on` 开启 `SO_KEEPALIVE` 选项

---

### 2.2 超时与缓冲指令

#### tunnel_connect_timeout

```nginx
tunnel_connect_timeout time;
```

| 属性 | 说明 |
|------|------|
| **默认值** | `60s` |
| **上下文** | http, server, location |

**说明**:
- 建立后端连接的超时时间
- 通常不应超过 75 秒

---

#### tunnel_read_timeout

```nginx
tunnel_read_timeout time;
```

| 属性 | 说明 |
|------|------|
| **默认值** | `60s` |
| **上下文** | http, server, location |

**说明**:
- 客户端或后端连接上两次连续读写操作之间的超时
- 无数据传输时关闭连接

---

#### tunnel_send_timeout

```nginx
tunnel_send_timeout time;
```

| 属性 | 说明 |
|------|------|
| **默认值** | `60s` |
| **上下文** | http, server, location |

**说明**:
- 向后端服务器传输请求的超时时间
- 仅针对两次连续写操作之间

---

#### tunnel_buffer_size

```nginx
tunnel_buffer_size size;
```

| 属性 | 说明 |
|------|------|
| **默认值** | `16k` |
| **上下文** | http, server, location |

**说明**:
- 设置用于从后端服务器读取数据的缓冲区大小
- 同时也设置从客户端读取数据的缓冲区大小

---

#### tunnel_send_lowat

```nginx
tunnel_send_lowat size;
```

| 属性 | 说明 |
|------|------|
| **默认值** | `0` |
| **上下文** | http, server, location |

**说明**:
- 非零值时尝试最小化发送操作（使用 `NOTE_LOWAT` 或 `SO_SNDLOWAT`）
- **注意**: 在 Linux、Solaris 和 Windows 上被忽略

---

### 2.3 上游故障转移指令

#### tunnel_next_upstream

```nginx
tunnel_next_upstream error | timeout | denied | off ...;
```

| 属性 | 说明 |
|------|------|
| **默认值** | `error timeout` |
| **上下文** | http, server, location |

**说明**:
- 指定何时将请求传递给下一台服务器
- `denied`: 被 `tunnel_allow_upstream` 拒绝
- `off`: 禁用故障转移

**重要限制**: 若已向客户端发送数据，则无法传递到下一台服务器

---

#### tunnel_next_upstream_timeout

```nginx
tunnel_next_upstream_timeout time;
```

| 属性 | 说明 |
|------|------|
| **默认值** | `0`（无限制） |
| **上下文** | http, server, location |

**说明**:
- 限制请求传递给下一台服务器的总时间
- `0` 表示关闭限制

---

#### tunnel_next_upstream_tries

```nginx
tunnel_next_upstream_tries number;
```

| 属性 | 说明 |
|------|------|
| **默认值** | `0`（无限制） |
| **上下文** | http, server, location |

**说明**:
- 限制传递给下一台服务器的尝试次数
- `0` 表示关闭限制

---

## 3. TCP 隧道配置示例

### 3.1 基础 HTTP 代理

```nginx
http {
    # 定义允许的端口
    map $request_port $allow_port {
        443            1;
        default        0;
    }

    # 定义允许的域名
    map $host $allow_host {
        hostnames;
        example.org    1;
        *.example.org  1;
        default        0;
    }

    server {
        listen 8000;
        resolver dns.example.com;

        # 权限检查
        if ($allow_port != 1) {
            return 502;
        }

        if ($allow_host != 1) {
            return 502;
        }

        # 启用隧道穿透
        tunnel_pass;
    }
}
```

**客户端使用**:
```bash
curl -x http://nginx:8000 https://example.org
```

---

### 3.2 带上游服务器的 TCP 隧道

```nginx
http {
    upstream backend_pool {
        server 10.0.0.1:8443;
        server 10.0.0.2:8443;
        server 10.0.0.3:8443;
    }

    server {
        listen 8000;

        location / {
            # 所有 CONNECT 请求转发到上游池
            tunnel_pass backend_pool;

            # 故障转移配置
            tunnel_next_upstream error timeout;
            tunnel_next_upstream_tries 3;
            tunnel_connect_timeout 30s;

            # 本地绑定地址
            tunnel_bind $server_addr;
        }
    }
}
```

---

### 3.3 动态目标路由

```nginx
http {
    map $http_x_target_host $tunnel_target {
        default         $host:$request_port;
        api.internal    10.0.0.100:8443;
        db.internal     unix:/var/run/db.sock;
    }

    server {
        listen 8000;

        location / {
            tunnel_pass $tunnel_target;
            tunnel_connect_timeout 10s;
            tunnel_read_timeout 300s;
        }
    }
}
```

---

## 4. WebSocket 隧道配置

**注意**: HTTP Tunnel 模块主要用于 TCP 隧道。WebSocket 支持取决于具体实现。

### 4.1 WebSocket 代理配置

```nginx
http {
    map $http_upgrade $connection_upgrade {
        default     upgrade;
        ''          close;
    }

    upstream websocket_backend {
        server 127.0.0.1:9000;
    }

    server {
        listen 8000;

        location /ws/ {
            # WebSocket 升级
            proxy_http_version 1.1;
            proxy_set_header Upgrade $http_upgrade;
            proxy_set_header Connection $connection_upgrade;

            # 隧道穿透（如后端支持 CONNECT）
            tunnel_pass websocket_backend;

            # 长连接超时
            tunnel_read_timeout 3600s;
            tunnel_send_timeout 3600s;
        }
    }
}
```

---

## 5. 与 stream 模块的区别

| 特性 | HTTP Tunnel Module | Stream Module |
|------|-------------------|---------------|
| **层级** | HTTP 层 (L7) | 传输层 (L4) |
| **协议** | 处理 HTTP CONNECT 请求 | 原始 TCP/UDP |
| **授权** | 商业订阅 | 开源免费 |
| **配置块** | `http`/`server`/`location` | `stream`/`server` |
| **路由能力** | 基于 HTTP 头、Host 等 L7 信息 | 仅基于 IP/端口 |
| **变量支持** | 丰富的 HTTP 变量 | 有限的 stream 变量 |
| **访问控制** | 可基于域名、端口、自定义条件 | 基于 IP 的 allow/deny |
| **典型用途** | HTTP 代理、HTTPS 穿透 | 数据库代理、TCP 负载均衡 |

### 5.1 stream 模块示例对比

```nginx
# stream 模块 (L4 层)
stream {
    server {
        listen 8443;
        proxy_pass backend_pool;
    }
}

# tunnel 模块 (L7 层，处理 CONNECT)
http {
    server {
        listen 8000;
        location / {
            tunnel_pass backend_pool;
        }
    }
}
```

---

## 6. 与 Lolly 项目的关系和建议

### 6.1 Lolly 项目概述

Lolly 是一个 Go 语言实现的高性能网络代理/隧道项目，专注于：
- 轻量级部署
- 简洁的配置
- 高性能转发

### 6.2 功能对比

| 特性 | NGINX Tunnel | Lolly |
|------|-------------|-------|
| **CONNECT 支持** | 原生支持 | 需确认实现 |
| **配置复杂度** | 高（多指令组合） | 低（简洁配置） |
| **动态路由** | 支持变量 | 需确认 |
| **故障转移** | 完善的上游故障转移 | 需确认 |
| **性能** | 高（C 语言优化） | 高（Go 并发模型） |
| **可观测性** | 标准日志 | 可定制 |
| **扩展性** | 模块扩展 | 代码扩展 |

### 6.3 对 Lolly 的建议

#### 6.3.1 参考设计

1. **CONNECT 方法处理**: 参考 `tunnel_pass` 的默认行为 `$host:$request_port`
2. **条件访问控制**: 实现类似 `tunnel_allow_upstream` 的灵活条件评估
3. **超时分层**: 区分连接、读取、发送超时，默认值参考 NGINX

#### 6.3.2 差异化优势

1. **简化配置**: Lolly 可提供更简洁的单行配置实现常见场景
2. **原生可观测性**: 内置 pprof、指标导出（已支持 pprof 端点）
3. **动态重载**: Go 的热重载比 NGINX 更友好

#### 6.3.3 建议新增功能

```yaml
# 建议的 Lolly 配置格式
tunnel:
  enable: true
  default_target: "$host:$port"  # 类似 tunnel_pass
  allowed_ports: [443, 8443]     # 类似 map $allow_port
  allowed_hosts: ["*.example.com"]
  timeouts:
    connect: 60s
    read: 300s
    send: 60s
  bind_address: "0.0.0.0"        # 类似 tunnel_bind
  keepalive: true                # 类似 tunnel_socket_keepalive
```

#### 6.3.4 实现优先级

| 优先级 | 功能 | 参考 NGINX 指令 |
|--------|------|---------------|
| P0 | 基础 CONNECT 处理 | `tunnel_pass` |
| P0 | 访问控制（端口/域名） | `tunnel_allow_upstream` |
| P1 | 连接超时 | `tunnel_connect_timeout` |
| P1 | 读写超时 | `tunnel_read_timeout` / `tunnel_send_timeout` |
| P2 | 本地地址绑定 | `tunnel_bind` |
| P2 | TCP Keepalive | `tunnel_socket_keepalive` |
| P3 | 上游故障转移 | `tunnel_next_upstream` |

---

## 附录

### A. 完整配置模板

```nginx
http {
    # 1. 访问控制映射
    map $request_port $tunnel_allow_port {
        443            1;
        8443           1;
        default        0;
    }

    map $host $tunnel_allow_host {
        hostnames;
        example.com    1;
        *.example.com  1;
        default        0;
    }

    # 2. 代理服务器
    server {
        listen 8000;
        server_name proxy.example.com;

        # DNS 解析
        resolver 8.8.8.8 8.8.4.4 valid=30s;

        # 访问控制
        if ($tunnel_allow_port != 1) {
            return 403;
        }

        if ($tunnel_allow_host != 1) {
            return 403;
        }

        # 隧道配置
        tunnel_pass $host:$request_port;

        # 超时配置
        tunnel_connect_timeout 30s;
        tunnel_read_timeout 300s;
        tunnel_send_timeout 60s;

        # 缓冲配置
        tunnel_buffer_size 32k;

        # Keepalive
        tunnel_socket_keepalive on;

        # 日志
        access_log /var/log/nginx/tunnel_access.log;
        error_log /var/log/nginx/tunnel_error.log;
    }
}
```

### B. 调试命令

```bash
# 测试 CONNECT 请求
curl -v -x http://nginx:8000 https://example.org

# 查看连接状态
nginx -T | grep tunnel

# 监控日志
tail -f /var/log/nginx/tunnel_access.log
tail -f /var/log/nginx/tunnel_error.log
```

### C. 参考资料

- [NGINX 官方文档](https://nginx.org/en/docs/http/ngx_http_tunnel_module.html)
- [RFC 9110 - CONNECT 方法](https://datatracker.ietf.org/doc/html/rfc9110#section-9.3.6)
- [F5 NGINX 商业订阅](https://www.f5.com/products/nginx)
