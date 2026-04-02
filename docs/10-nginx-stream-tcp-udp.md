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

```nginx
upstream backend {
    server 192.168.1.1:3306;
    keepalive 32;              # 保持 32 个空闲连接
    keepalive_timeout 60s;     # 空闲连接超时
    keepalive_requests 1000;   # 单个连接最大请求数
}
```

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