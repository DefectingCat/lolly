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

---

## 15. 访问控制模块

### ngx_stream_access_module

基于 IP 地址的访问控制，允许或拒绝特定客户端连接。

**指令**：

| 指令 | 语法 | 默认值 | 上下文 |
|------|------|--------|--------|
| `allow` | `allow address \| CIDR \| unix: \| all;` | — | stream, server |
| `deny` | `deny address \| CIDR \| unix: \| all;` | — | stream, server |

**配置示例**：

```nginx
stream {
    # 数据库访问控制
    server {
        listen 3306;
        
        # 允许内网访问
        allow 10.0.0.0/8;
        allow 192.168.0.0/16;
        allow 172.16.0.0/12;
        
        # 拒绝其他所有
        deny all;
        
        proxy_pass mysql_backend;
    }

    # Redis 访问控制
    server {
        listen 6379;
        
        # 仅允许特定 IP
        allow 192.168.1.100;
        allow 192.168.1.101;
        deny all;
        
        proxy_pass redis_backend;
    }

    # 管理端口（仅本地）
    server {
        listen 9000;
        
        allow 127.0.0.1;
        deny all;
        
        proxy_pass admin_backend;
    }
}
```

**规则匹配顺序**：
- 按配置顺序依次检查
- 首个匹配的规则决定结果
- 未匹配任何规则时默认允许

---

## 16. 连接限制模块

### ngx_stream_limit_conn_module

限制并发连接数，防止资源耗尽。

**指令**：

| 指令 | 语法 | 默认值 | 上下文 |
|------|------|--------|--------|
| `limit_conn_zone` | `limit_conn_zone key zone=name:size;` | — | stream |
| `limit_conn` | `limit_conn zone number;` | — | stream, server |
| `limit_conn_log_level` | `limit_conn_log_level info \| notice \| warn \| error;` | error | stream, server |

**配置示例**：

```nginx
stream {
    # 按客户端 IP 限制连接数
    limit_conn_zone $binary_remote_addr zone=addr:10m;
    
    # 按上游服务器限制连接数
    limit_conn_zone $server_addr zone=server:10m;

    # MySQL 代理 - 每 IP 最多 10 个连接
    server {
        listen 3306;
        limit_conn addr 10;
        proxy_pass mysql_backend;
    }

    # Redis 代理 - 每 IP 最多 5 个连接
    server {
        listen 6379;
        limit_conn addr 5;
        limit_conn_log_level warn;
        proxy_pass redis_backend;
    }

    # 全局连接限制
    server {
        listen 8080;
        limit_conn addr 50;      # 每 IP 最多 50
        limit_conn server 1000;  # 服务总连接上限
        proxy_pass backend;
    }
}
```

**内存计算**：
- 1MB 共享内存可存储约 16,000 个 32 字节 key（$binary_remote_addr）
- 或约 8,000 个 IPv6 地址（16 字节）

---

## 17. 地理位置模块

### ngx_stream_geo_module

根据客户端 IP 地址创建变量值，用于地理路由或访问控制。

**指令**：

| 指令 | 语法 | 默认值 | 上下文 |
|------|------|--------|--------|
| `geo` | `geo [$address] $variable { ... }` | — | stream |

**配置示例**：

```nginx
stream {
    # 基础地理映射
    geo $remote_addr $region {
        default        other;
        10.0.0.0/8     internal;
        192.168.0.0/16 internal;
        172.16.0.0/12  internal;
        
        # 中国大陆 IP 段（示例）
        1.0.1.0/24     china;
        1.0.2.0/23     china;
        # ... 更多 IP 段
    }

    # 使用变量进行路由
    map $region $backend_pool {
        internal  internal_backend;
        china     china_backend;
        other     global_backend;
    }

    upstream internal_backend {
        server 192.168.1.1:3306;
    }

    upstream china_backend {
        server 10.0.1.1:3306;
    }

    upstream global_backend {
        server 10.0.2.1:3306;
    }

    server {
        listen 3306;
        proxy_pass $backend_pool;
    }
}
```

**高级用法**：

```nginx
stream {
    # 使用变量作为地址源
    geo $realip_remote_addr $region {
        default other;
        # ... 配置
    }

    # 带 CIDR 包含
    geo $country {
        default    XX;
        include    /etc/nginx/geo/countries.conf;
    }

    # countries.conf 内容示例：
    # 1.0.1.0/24     CN;
    # 1.0.2.0/23     CN;
    # 1.1.1.0/24     AU;
}

    # 使用 GeoIP 数据库（需 ngx_stream_geoip_module）
    geoip_country /usr/share/GeoIP/GeoIP.dat;
    
    map $geoip_country_code $backend {
        default global_backend;
        CN      china_backend;
        US      us_backend;
        EU      eu_backend;
    }
}
```

---

## 18. 真实 IP 模块

### ngx_stream_realip_module

处理 PROXY 协议头，获取客户端真实 IP 地址。

**指令**：

| 指令 | 语法 | 默认值 | 上下文 |
|------|------|--------|--------|
| `set_real_ip_from` | `set_real_ip_from address \| CIDR;` | — | stream, server |
| `real_ip_header` | `real_ip_header field;` | proxy_protocol | stream, server |

**配置示例**：

```nginx
stream {
    server {
        listen 3306 proxy_protocol;  # 接收 PROXY 协议
        
        # 信任的代理服务器地址
        set_real_ip_from 10.0.0.0/8;
        set_real_ip_from 192.168.0.0/16;
        set_real_ip_from 172.16.0.0/12;
        
        proxy_pass mysql_backend;
    }
}
```

**可用变量**：

| 变量 | 说明 |
|------|------|
| `$realip_remote_addr` | 原始客户端地址（PROXY 协议中的地址）|
| `$realip_remote_port` | 原始客户端端口 |
| `$proxy_protocol_addr` | PROXY 协议中的客户端地址 |
| `$proxy_protocol_port` | PROXY 协议中的客户端端口 |
| `$proxy_protocol_server_addr` | PROXY 协议中的目标服务器地址 |
| `$proxy_protocol_server_port` | PROXY 协议中的目标服务器端口 |

**典型场景**：

```nginx
stream {
    # 场景：负载均衡器 → NGINX → 后端
    server {
        listen 3306 proxy_protocol;
        
        # 负载均衡器的 IP
        set_real_ip_from 10.0.0.1;
        set_real_ip_from 10.0.0.2;
        
        # 使用真实 IP 进行限流
        limit_conn_zone $realip_remote_addr zone=conn_limit:10m;
        limit_conn conn_limit 10;
        
        # 日志记录真实 IP
        log_format main '$realip_remote_addr [$time_local] '
                        '$protocol $status $bytes_sent $bytes_received';
        access_log /var/log/nginx/stream.log main;
        
        proxy_pass mysql_backend;
    }
}
```

---

## 19. 高级日志配置

### 日志格式详解

```nginx
stream {
    # JSON 格式日志
    log_format json_combined escape=json
        '{'
            '"time_local":"$time_local",'
            '"remote_addr":"$remote_addr",'
            '"server_addr":"$server_addr",'
            '"server_port":"$server_port",'
            '"protocol":"$protocol",'
            '"status":"$status",'
            '"bytes_sent":"$bytes_sent",'
            '"bytes_received":"$bytes_received",'
            '"session_time":"$session_time",'
            '"upstream_addr":"$upstream_addr",'
            '"upstream_bytes_sent":"$upstream_bytes_sent",'
            '"upstream_bytes_received":"$upstream_bytes_received",'
            '"upstream_connect_time":"$upstream_connect_time"'
        '}';

    # 详细格式日志
    log_format detailed '$remote_addr - [$time_local] '
                        '$protocol/$status '
                        'sent:$bytes_sent recv:$bytes_received '
                        'time:$session_time '
                        'upstream:$upstream_addr '
                        'upstream_time:$upstream_connect_time';

    # 条件日志（仅记录错误）
    map $status $loggable {
        ~^[23]  0;
        default 1;
    }

    server {
        listen 3306;
        access_log /var/log/nginx/stream.json json_combined;
        access_log /var/log/nginx/stream_errors.log detailed if=$loggable;
        proxy_pass backend;
    }
}
```

### 日志缓冲与压缩

```nginx
stream {
    server {
        listen 3306;
        
        # 缓冲写入（提升性能）
        access_log /var/log/nginx/stream.log main buffer=32k flush=5s;
        
        # gzip 压缩日志
        access_log /var/log/nginx/stream.log.gz main gzip buffer=32k;
        
        proxy_pass backend;
    }
}
```

### open_log_file_cache

缓存日志文件描述符，减少文件打开操作：

```nginx
stream {
    open_log_file_cache max=1000 inactive=20s valid=1m min_uses=2;
    
    server {
        listen 3306;
        access_log /var/log/nginx/stream.log main;
        proxy_pass backend;
    }
}
```

**参数说明**：

| 参数 | 说明 |
|------|------|
| `max` | 缓存的最大文件描述符数 |
| `inactive` | 非活动文件保留时间 |
| `valid` | 检查文件是否有效的时间间隔 |
| `min_uses` | 最小使用次数才缓存 |

---

## 20. Stream 模块源码架构分析

基于 nginx 1.31.0 源码（`lib/nginx/src/stream/`，24 个 .c 文件）。

### 20.1 模块文件结构

```
src/stream/
├── ngx_stream.c              # 模块入口（252 行）
├── ngx_stream_core_module.c  # 核心配置（1524 行）
├── ngx_stream_handler.c      # 连接处理（390 行）
├── ngx_stream_proxy_module.c # 反向代理（2870 行）
├── ngx_stream_upstream.c     # upstream 管理
├── ngx_stream_upstream_round_robin.c  # 负载均衡
├── ngx_stream_ssl_module.c   # SSL/TLS 支持
├── ngx_stream_ssl_preread_module.c    # SSL preread（SNI 路由）
├── ngx_stream_access_module.c # IP 访问控制
├── ngx_stream_log_module.c   # 访问日志
├── ngx_stream_variables.c    # 变量支持
├── ngx_stream_return_module.c # 返回响应
├── ngx_stream_map_module.c   # 变量映射
├── ngx_stream_split_clients_module.c  # A/B 测试
└── modules/                  # 其他子模块
```

### 20.2 处理阶段（7 Phase）

```c
// src/stream/ngx_stream_core_module.h
typedef enum {
    NGX_STREAM_POST_ACCEPT_PHASE = 0,   // 接受后处理（proxy_protocol）
    NGX_STREAM_PREACCESS_PHASE,         // 预访问检查
    NGX_STREAM_ACCESS_PHASE,            // 访问控制（allow/deny）
    NGX_STREAM_SSL_PHASE,               // SSL 握手
    NGX_STREAM_PREREAD_PHASE,           // 协议识别预读（SNI 检测）
    NGX_STREAM_CONTENT_PHASE,           // 内容处理（调用 handler）
    NGX_STREAM_LOG_PHASE                // 日志记录
} ngx_stream_phases;
```

### 20.3 TCP 代理处理流程

```
client 连接
  |
  v
ngx_stream_init_connection()
  |
  +-- 创建 ngx_stream_session_t
  +-- 执行 phases (accept, access, ssl, preread, content)
  |
  v
ngx_stream_proxy_handler()
  |
  +-- ngx_stream_proxy_connect() 连接上游服务器
  +-- ngx_stream_proxy_process() 双向数据转发
  |
  v
双向数据流:
  client <---> ngx_stream_proxy_process <---> upstream
```

### 20.4 UDP 代理处理流程

```
client 数据
  |
  v
ngx_stream_core_udp_handler()
  |
  +-- 创建临时 UDP session
  +-- ngx_stream_proxy_init_upstream() 获取上游地址
  +-- sendto() 直接发送数据
  |
  v
通过 udp_connection_pool 复用连接
```

### 20.5 SSL Preread 模块（SNI 路由）

```c
// src/stream/ngx_stream_ssl_preread_module.c
// 在 PREREAD_PHASE 阶段解析 TLS ClientHello
// 提取 SNI（Server Name Indication）用于路由决策

typedef struct {
    ngx_uint_t                  server_name_len;
    u_char                      server_name[256];
} ngx_stream_ssl_preread_ctx_t;

// 根据 $ssl_preread_server_name 变量选择 upstream 组
```

### 20.6 关键源码路径

| 功能 | 文件路径 | 行数 |
|------|----------|------|
| Stream 入口 | `src/stream/ngx_stream.c` | 252 |
| 核心配置 | `src/stream/ngx_stream_core_module.c` | 1524 |
| 连接处理 | `src/stream/ngx_stream_handler.c` | 390 |
| 反向代理 | `src/stream/ngx_stream_proxy_module.c` | 2870 |
| SSL 模块 | `src/stream/ngx_stream_ssl_module.c` | - |
| SSL Preread | `src/stream/ngx_stream_ssl_preread_module.c` | - |
| 变量系统 | `src/stream/ngx_stream_variables.c` | - |
| 负载均衡 | `src/stream/ngx_stream_upstream_round_robin.c` | - |

---

## 21. Mail 模块架构概述

基于 nginx 1.31.0 源码（`lib/nginx/src/mail/`，14 个 .c 文件）。

### 21.1 协议支持

| 协议 | 端口 | SSL 端口 |
|------|------|----------|
| IMAP | 143 | 993 |
| POP3 | 110 | 995 |
| SMTP | 25/587 | 465 |

### 21.2 模块文件结构

```
src/mail/
├── ngx_mail.c                # 模块入口（486 行）
├── ngx_mail_core_module.c    # 核心配置（534 行）
├── ngx_mail_handler.c        # 连接处理（750 行）
├── ngx_mail_imap_module.c    # IMAP 模块
├── ngx_mail_imap_handler.c   # IMAP 协议处理
├── ngx_mail_pop3_module.c    # POP3 模块
├── ngx_mail_pop3_handler.c   # POP3 协议处理
├── ngx_mail_smtp_module.c    # SMTP 模块
├── ngx_mail_smtp_handler.c   # SMTP 协议处理
├── ngx_mail_proxy_module.c   # 邮件代理
├── ngx_mail_auth.c           # 认证核心
└── ngx_mail_variables.c      # 变量支持
```

### 21.3 认证机制支持

- PLAIN、LOGIN（基础认证）
- CRAM-MD5（挑战-响应认证）
- EXTERNAL（外部认证）
- APOP（POP3 专用）

### 21.4 代理流程

```
client 连接
  |
  v
ngx_mail_init_connection()
  |
  +-- 根据端口识别协议（IMAP/POP3/SMTP）
  +-- ngx_mail_auth() 执行认证
  |
  v
ngx_mail_proxy_init() 连接上游邮件服务器
  |
  v
协议转发: client <-> proxy <-> upstream
```

---

*源码分析基于 nginx 1.31.0*
*源码目录：`lib/nginx/src/stream/` 和 `lib/nginx/src/mail/`*