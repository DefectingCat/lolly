# NGINX TCP/UDP Stream 模块指南

## 1. Stream 模块概述

Stream 模块提供 TCP/UDP 流处理功能，包括：
- TCP 代理与负载均衡
- UDP 代理（DNS、日志收集等）
- TLS/SSL 终端
- 基于 SNI 的路由

### 版本要求

- 自 1.9.0 版本可用
- 默认不构建，需编译时添加 `--with-stream` 参数

### 配置上下文

```nginx
stream {
    # TCP/UDP 配置
    server {
        listen 12345;
        proxy_pass backend:54321;
    }
}
```

---

## 2. 基础配置示例

### TCP 代理

```nginx
stream {
    upstream backend {
        server 192.168.1.1:3306;
        server 192.168.1.2:3306;
    }

    server {
        listen 3306;
        proxy_pass backend;
        proxy_timeout 3s;
        proxy_connect_timeout 1s;
    }
}
```

### UDP 代理

```nginx
stream {
    upstream dns_servers {
        server 8.8.8.8:53;
        server 8.8.4.4:53;
    }

    server {
        listen 53 udp;
        proxy_pass dns_servers;
        proxy_timeout 20s;
    }
}
```

### Unix Socket 代理

```nginx
stream {
    server {
        listen unix:/tmp/stream.sock;
        proxy_pass unix:/tmp/backend.sock;
    }
}
```

---

## 3. 负载均衡

### 配置示例

```nginx
stream {
    upstream mysql_backend {
        hash $remote_addr consistent;  # 一致性哈希
        server mysql1:3306 weight=5;
        server mysql2:3306;
        server mysql3:3306 backup;
    }

    server {
        listen 3306;
        proxy_pass mysql_backend;
    }
}
```

### 负载均衡算法

| 算法 | 指令 | 说明 |
|------|------|------|
| 轮询 | 默认 | 加权轮询 |
| 最少连接 | `least_conn;` | 连接数最少优先 |
| 哈希 | `hash key [consistent];` | 基于键哈希 |
| 最少时间 | `least_time header \| last_byte \| last_byte inflight;` | 最小响应时间（NGINX Plus） |
| 随机 | `random [two] [least_conn];` | 随机选择 |

### server 参数

| 参数 | 说明 |
|------|------|
| `weight=N` | 权重 |
| `max_conns=N` | 最大连接数 |
| `max_fails=N` | 失败次数阈值 |
| `fail_timeout=T` | 失败统计时间 |
| `backup` | 备份服务器 |
| `down` | 标记不可用 |
| `resolve` | 解析域名 IP 变化 |

### 新负载均衡算法

#### least_time（最小响应时间）

**版本要求**：NGINX Plus

```nginx
upstream mysql_backend {
    least_time header;  # 或 last_byte, last_byte inflight
    server 192.168.1.1:3306;
    server 192.168.1.2:3306;
    zone mysql 64k;
}
```

**参数说明**：

| 参数 | 说明 |
|------|------|
| `header` | 以接收到上游第一个字节的时间为度量 |
| `last_byte` | 以接收到上游完整响应的时间为度量 |
| `last_byte inflight` | 考虑正在传输的数据 |

#### random（随机选择）

```nginx
upstream backend {
    random;                    # 纯随机
    # random two;              # 随机选两个，按权重选
    # random two least_conn;   # 随机选两个，选连接少的
    server 192.168.1.1:3306;
    server 192.168.1.2:3306;
}
```

**参数说明**：

| 参数 | 说明 |
|------|------|
| `two` | 随机选择两个上游服务器，再根据负载均衡策略选择 |
| `least_conn` | 与 `two` 配合，选择连接数较少的 |

### upstream zone 共享内存

`zone` 指令启用 upstream 配置的共享内存，多 worker 进程间共享连接状态：

```nginx
upstream backend {
    zone backend 64k;  # 64k 共享内存
    server 192.168.1.1:3306;
    server 192.168.1.2:3306;
    keepalive 32;
}
```

**说明**：
- 共享内存使所有 worker 进程共享 upstream 状态
- 必需用于 `least_conn`、`least_time`、健康检查等
- 大小根据上游服务器数量调整


---

## 4. 核心指令

### listen 指令

```nginx
server {
    listen 80;                           # TCP
    listen 53 udp;                       # UDP
    listen 443 ssl;                      # TLS
    listen 12345 udp reuseport;          # UDP + reuseport
    listen [::]:80;                      # IPv6
    listen unix:/tmp/stream.sock;        # Unix Socket
}
```

**参数说明**：

| 参数 | 说明 |
|------|------|
| `ssl` | 启用 SSL |
| `udp` | UDP 协议 |
| `reuseport` | 每个 worker 独立监听 |
| `proxy_protocol` | 启用 PROXY 协议 |
| `backlog=N` | 连接队列长度 |
| `so_keepalive` | TCP keepalive |
| `transparent` | 启用透明代理 |

### proxy_pass 指令

```nginx
server {
    listen 3306;
    proxy_pass 192.168.1.1:3306;
    proxy_pass backend;
    proxy_pass $upstream;
}
```

### 超时配置

```nginx
server {
    proxy_timeout 10m;            # 客户端/后端之间读写超时（默认 10m）
    proxy_connect_timeout 60s;    # 连接后端超时（默认 60s）
}
```

### 缓冲配置

```nginx
server {
    proxy_buffer_size 16k;        # 读写缓冲区大小（默认 16k）
}
```

### 其他核心指令

| 指令 | 语法 | 默认值 | 上下文 | 说明 |
|------|------|--------|--------|------|
| `proxy_bind` | `proxy_bind address [transparent];` | — | stream, server | 指定代理连接使用的本地地址 |
| `proxy_half_close` | `proxy_half_close on \| off;` | off | stream, server | 启用 TCP 半关闭支持 |
| `proxy_responses` | `proxy_responses number;` | — | stream, server (UDP) | UDP 每请求期望响应数 |
| `proxy_socket_keepalive` | `proxy_socket_keepalive on \| off;` | off | stream, server | 开启与上游的 TCP keepalive |

**示例配置**：

```nginx
# 透明代理
server {
    listen 3306 transparent;
    proxy_bind $remote_addr transparent;
    proxy_pass backend:3306;
}

# UDP 配置
server {
    listen 53 udp;
    proxy_pass dns_backend:53;
    proxy_responses 1;  # 每请求期望1个响应
}

# 半关闭支持（TCP 流式场景）
server {
    listen 9000;
    proxy_half_close on;
    proxy_pass backend:9000;
}
```

---

## 5. SSL/TLS 配置

### 服务端 SSL

```nginx
stream {
    server {
        listen 443 ssl;
        ssl_certificate     /path/to/cert.pem;
        ssl_certificate_key /path/to/key.pem;
        ssl_protocols       TLSv1.2 TLSv1.3;

        proxy_pass backend:8080;
    }
}
```

### 上游 SSL（连接后端使用 SSL）

```nginx
server {
    listen 3306;
    proxy_pass backend:3306;

    proxy_ssl on;
    proxy_ssl_protocols TLSv1.2 TLSv1.3;
    proxy_ssl_verify on;
    proxy_ssl_trusted_certificate /path/to/ca.pem;
    proxy_ssl_server_name on;
}
```

### SSL 配置指令

| 指令 | 说明 |
|------|------|
| `ssl_certificate` | 证书文件 |
| `ssl_certificate_key` | 私钥文件 |
| `ssl_protocols` | 启用的协议 |
| `ssl_ciphers` | 加密套件 |
| `ssl_session_cache` | 会话缓存 |
| `proxy_ssl` | 启用上游 SSL |
| `proxy_ssl_verify` | 验证上游证书 |
| `proxy_ssl_server_name` | 启用 SNI |

#### 完整上游 SSL 配置示例

```nginx
server {
    listen 3306;
    proxy_pass ssl_backend:3306;

    proxy_ssl on;
    proxy_ssl_protocols TLSv1.2 TLSv1.3;
    proxy_ssl_ciphers HIGH:!aNULL;
    proxy_ssl_certificate /path/to/client.crt;
    proxy_ssl_certificate_key /path/to/client.key;
    proxy_ssl_verify on;
    proxy_ssl_trusted_certificate /path/to/ca.crt;
    proxy_ssl_verify_depth 2;
    proxy_ssl_name backend.example.com;
    proxy_ssl_session_reuse on;
}
```

**指令说明**：

| 指令 | 说明 |
|------|------|
| `proxy_ssl_certificate` | 客户端证书（mTLS） |
| `proxy_ssl_certificate_key` | 客户端证书私钥 |
| `proxy_ssl_ciphers` | 加密套件 |
| `proxy_ssl_verify_depth` | 证书链验证深度 |
| `proxy_ssl_name` | 验证上游证书的域名 |
| `proxy_ssl_session_reuse` | 启用 SSL 会话复用 |

---

## 6. 基于名称的虚拟服务器（SNI）

**版本要求**：1.25.5+

```nginx
stream {
    map $ssl_server_name $backend {
        app1.example.com app1_backend;
        app2.example.com app2_backend;
        default          default_backend;
    }

    upstream app1_backend {
        server 192.168.1.1:8080;
    }

    upstream app2_backend {
        server 192.168.1.2:8080;
    }

    server {
        listen 443 ssl;
        server_name app1.example.com app2.example.com;

        ssl_certificate     /path/to/cert.pem;
        ssl_certificate_key /path/to/key.pem;

        proxy_pass $backend;
    }
}
```

### SSL Preread SNI 路由（无需终止 SSL）

**版本要求**：1.25.5+

使用 `ssl_preread` 模块在不解密的情况下读取 SNI 信息进行路由：

```nginx
stream {
    ssl_preread on;  # 启用 ssl_preread
    
    map $ssl_preread_server_name $backend {
        mysql.example.com mysql_backend;
        redis.example.com redis_backend;
        default default_backend;
    }
    
    upstream mysql_backend {
        server 192.168.1.1:3306;
    }
    
    upstream redis_backend {
        server 192.168.1.2:6379;
    }
    
    upstream default_backend {
        server 192.168.1.3:8080;
    }
    
    server {
        listen 443;
        ssl_preread on;  # 在 server 上下文启用
        proxy_pass $backend;
    }
}
```

**说明**：
- `ssl_preread` 读取 ClientHello 中的 SNI 扩展
- 无需配置 SSL 证书即可实现基于域名的路由
- 适用于多服务共享端口的场景

---

## 7. PROXY 协议

### 接收 PROXY 协议

```nginx
server {
    listen 3306 proxy_protocol;
    proxy_pass backend:3306;
}
```

### 发送 PROXY 协议

```nginx
server {
    listen 3306;
    proxy_protocol on;
    proxy_pass backend:3306;
}
```

### PROXY 协议变量

| 变量 | 说明 |
|------|------|
| `$proxy_protocol_addr` | 客户端 IP |
| `$proxy_protocol_port` | 客户端端口 |
| `$proxy_protocol_server_addr` | 服务器 IP |
| `$proxy_protocol_server_port` | 服务器端口 |

---

## 8. 速率限制

```nginx
server {
    listen 3306;
    proxy_pass backend:3306;

    proxy_download_rate 1m;    # 限制从后端读取速率（字节/秒）
    proxy_upload_rate 1m;      # 限制从客户端读取速率（字节/秒）
}
```

**使用变量动态限制**：
```nginx
map $remote_addr $limit_rate {
    default     1m;
    10.0.0.1    10m;     # 特定 IP 更高速率
}

server {
    proxy_download_rate $limit_rate;
    proxy_upload_rate $limit_rate;
}
```

---

## 9. 故障转移

```nginx
server {
    listen 3306;
    proxy_pass backend:3306;

    proxy_next_upstream on;              # 连接失败时尝试下一台
    proxy_next_upstream_timeout 30s;     # 总时间限制
    proxy_next_upstream_tries 3;         # 尝试次数限制
}
```

---

## 10. 连接保持

### keepalive 连接池配置

```nginx
upstream backend {
    server 192.168.1.1:3306;
    server 192.168.1.2:3306;
    
    keepalive 32;           # 空闲连接池大小
    keepalive_requests 100; # 单连接最大请求（HTTP 适用）
    keepalive_timeout 60s;  # 空闲超时
}
```

**指令说明**：

| 指令 | 语法 | 默认值 | 说明 |
|------|------|--------|------|
| `keepalive` | `keepalive connections;` | — | 保持到上游的空闲连接数 |
| `keepalive_requests` | `keepalive_requests number;` | 100 | 单连接最大请求数 |
| `keepalive_timeout` | `keepalive_timeout timeout;` | 60s | 空闲连接超时时间 |

---

## 11. 内置变量

| 变量 | 说明 |
|------|------|
| `$remote_addr` | 客户端 IP |
| `$remote_port` | 客户端端口 |
| `$server_addr` | 服务器 IP |
| `$server_port` | 服务器端口 |
| `$protocol` | 协议（TCP/UDP） |
| `$bytes_received` | 接收字节数 |
| `$bytes_sent` | 发送字节数 |
| `$session_time` | 会话时间（秒） |
| `$status` | 会话状态 |
| `$ssl_preread_server_name` | SNI 名称 |

### 其他 stream 指令

| 指令 | 语法 | 默认值 | 说明 |
|------|------|--------|------|
| `preread_buffer_size` | `preread_buffer_size size;` | 16k | 预读缓冲区大小 |
| `preread_timeout` | `preread_timeout timeout;` | 30s | 预读超时时间 |
| `resolver` | `resolver address ... [valid=time];` | — | DNS 解析器 |
| `resolver_timeout` | `resolver_timeout time;` | 30s | 解析超时 |
| `tcp_nodelay` | `tcp_nodelay on \| off;` | on | 启用 TCP_NODELAY |
| `variables_hash_max_size` | `variables_hash_max_size size;` | 1024 | 变量哈希表最大大小 |
| `variables_hash_bucket_size` | `variables_hash_bucket_size size;` | 64 | 变量哈希表桶大小 |

---

## 12. 应用场景示例

### MySQL 代理

```nginx
stream {
    upstream mysql_servers {
        least_conn;
        server mysql1:3306 weight=5;
        server mysql2:3306;
        server mysql3:3306 backup;
    }

    server {
        listen 3306;
        proxy_pass mysql_servers;
        proxy_timeout 600s;
        proxy_connect_timeout 2s;
    }
}
```

### Redis 代理

```nginx
stream {
    upstream redis_servers {
        server redis1:6379;
        server redis2:6379;
    }

    server {
        listen 6379;
        proxy_pass redis_servers;
        proxy_timeout 300s;
    }
}
```

### DNS 代理

```nginx
stream {
    upstream dns_servers {
        server 8.8.8.8:53;
        server 8.8.4.4:53;
    }

    server {
        listen 53 udp reuseport;
        proxy_pass dns_servers;
        proxy_timeout 20s;
        proxy_responses 1;   # 期望 1 个响应
    }
}
```

### 日志收集（Syslog）

```nginx
stream {
    upstream syslog_servers {
        server syslog1:514;
        server syslog2:514;
    }

    server {
        listen 514 udp;
        proxy_pass syslog_servers;
        proxy_timeout 10s;
    }
}
```

### WebSocket 代理（TCP 层）

```nginx
stream {
    upstream websocket_servers {
        server ws1:8080;
        server ws2:8080;
    }

    server {
        listen 8080;
        proxy_pass websocket_servers;
        proxy_timeout 3600s;  # 长连接超时
    }
}
```

---

## 13. 日志配置

```nginx
stream {
    log_format main '$remote_addr [$time_local] '
                    '$protocol $status $bytes_sent $bytes_received '
                    '$session_time "$upstream_addr"';

    access_log /var/log/nginx/stream.log main;

    server {
        listen 3306;
        proxy_pass backend:3306;
    }
}
```

---

## 14. 健康检查

### 被动检查（内置）

```nginx
upstream backend {
    server 192.168.1.1:3306 max_fails=3 fail_timeout=30s;
    server 192.168.1.2:3306 max_fails=3 fail_timeout=30s;
}
```

### 主动检查（NGINX Plus）

```nginx
upstream backend {
    zone backend 64k;

    server 192.168.1.1:3306;
    server 192.168.1.2:3306;

    health_check interval=5s passes=2 fails=3;
    health_check_timeout 5s;
}
```