# NGINX HTTP/2 与 HTTP/3 (QUIC) 配置详解

## 1. HTTP/2 基础

### 1.1 HTTP/2 特性

HTTP/2 是 HTTP 协议的重大升级，主要特性包括：

| 特性 | 说明 |
|------|------|
| **多路复用** | 单一 TCP 连接上并发处理多个请求/响应 |
| **头部压缩** | HPACK 算法压缩请求头，减少传输开销 |
| **服务器推送** | 服务器主动推送资源到客户端 |
| **二进制分帧** | 二进制协议替代文本协议，更高效解析 |
| **流优先级** | 允许客户端指定请求优先级 |

### 1.2 版本要求

- NGINX 1.9.5+ 支持 HTTP/2
- 需要编译 `--with-http_v2_module` 模块
- OpenSSL 1.0.2+ 推荐（支持 ALPN）

---

## 2. ngx_http_v2_module 指令详解

### 2.1 基础启用配置

#### listen 指令中的 http2 参数

```nginx
server {
    listen 443 ssl http2;
    server_name www.example.com;

    ssl_certificate     /etc/nginx/ssl/www.example.com.crt;
    ssl_certificate_key /etc/nginx/ssl/www.example.com.key;

    location / {
        root /var/www/html;
    }
}
```

**注意**：NGINX 1.25.1+ 起，`http2` 参数从 `listen` 指令移至 `http2` 指令。

```nginx
# NGINX 1.25.1+ 推荐写法
server {
    listen 443 ssl;
    http2 on;
    # ...
}
```

### 2.2 http2 指令

| 语法 | 默认值 | 上下文 |
|------|--------|--------|
| `http2 on \| off;` | `off` | `http`, `server` |

启用或禁用 HTTP/2 协议支持。

```nginx
# 全局启用
http {
    http2 on;

    server {
        listen 443 ssl;
        # ...
    }
}

# 单个服务器启用
server {
    listen 443 ssl;
    http2 on;
    # ...
}
```

### 2.3 http2_recv_buffer_size

| 语法 | 默认值 | 上下文 |
|------|--------|--------|
| `http2_recv_buffer_size size;` | `256k` | `http`, `server` |

设置每个 worker 进程的 HTTP/2 连接接收缓冲区大小。

```nginx
# 高并发场景下增大缓冲区
http2_recv_buffer_size 512k;
```

### 2.4 http2_max_concurrent_streams

| 语法 | 默认值 | 上下文 |
|------|--------|--------|
| `http2_max_concurrent_streams number;` | `128` | `http`, `server` |

设置单个 HTTP/2 连接中最大并发流数量。

```nginx
# 增加并发流数量
http2_max_concurrent_streams 256;
```

**注意事项**：
- 值越大，内存消耗越多
- 根据服务器资源和客户端需求调整

### 2.5 http2_max_field_size

| 语法 | 默认值 | 上下文 |
|------|--------|--------|
| `http2_max_field_size size;` | `4k` | `http`, `server` |

设置压缩后的请求头字段最大大小。

```nginx
# 支持较大的 Cookie 头部
http2_max_field_size 16k;
```

### 2.6 http2_max_header_size

| 语法 | 默认值 | 上下文 |
|------|--------|--------|
| `http2_max_header_size size;` | `16k` | `http`, `server` |

设置压缩后的整个请求头最大大小。

```nginx
# 支持大量自定义头部
http2_max_header_size 32k;
```

### 2.7 http2_max_requests

| 语法 | 默认值 | 上下文 |
|------|--------|--------|
| `http2_max_requests number;` | `1000` | `http`, `server` |

设置单个 HTTP/2 连接上最大请求数量，超过后连接关闭。

```nginx
# 长连接场景下增加请求数
http2_max_requests 5000;
```

### 2.8 http2_chunk_size

| 语法 | 默认值 | 上下文 |
|------|--------|--------|
| `http2_chunk_size size;` | `8k` | `http`, `server`, `location` |

设置响应体分块大小，太小会增加开销，太大会影响优先级。

```nginx
# 大文件传输优化
http2_chunk_size 16k;
```

### 2.9 http2_body_preread_size

| 语法 | 默认值 | 上下文 |
|------|--------|--------|
| `http2_body_preread_size size;` | `64k` | `http`, `server` |

设置请求体预读缓冲区大小。

### 2.10 http2_idle_timeout

| 语法 | 默认值 | 上下文 |
|------|--------|--------|
| `http2_idle_timeout time;` | `3m` | `http`, `server` |

设置 HTTP/2 连接空闲超时时间。

```nginx
# 缩短空闲超时
http2_idle_timeout 60s;
```

---

## 3. HTTP/2 服务器推送

### 3.1 http2_push 指令

| 语法 | 默认值 | 上下文 |
|------|--------|--------|
| `http2_push uri;` | - | `http`, `server`, `location` |

预推送指定 URI 的资源到客户端。

```nginx
server {
    listen 443 ssl http2;
    server_name www.example.com;

    ssl_certificate     /etc/nginx/ssl/www.example.com.crt;
    ssl_certificate_key /etc/nginx/ssl/www.example.com.key;

    location = /index.html {
        http2_push /style.css;
        http2_push /script.js;
        http2_push /logo.png;
        alias /var/www/html/index.html;
    }
}
```

### 3.2 http2_push_preload 指令

| 语法 | 默认值 | 上下文 |
|------|--------|--------|
| `http2_push_preload on \| off;` | `off` | `http`, `server`, `location` |

根据 `Link` 响应头自动推送资源。

```nginx
location = /index.html {
    http2_push_preload on;
    add_header Link "</style.css>; rel=preload; as=style" always;
    add_header Link "</script.js>; rel=preload; as=script" always;
    alias /var/www/html/index.html;
}
```

**注意**：NGINX 1.25.0+ 已弃用 `http2_push` 和 `http2_push_preload`，推荐使用 Early Hints (103 状态码)。

---

## 4. HTTP/2 完整配置示例

### 4.1 基础配置

```nginx
http {
    # HTTP/2 全局设置
    http2_recv_buffer_size    512k;
    http2_max_concurrent_streams  256;
    http2_idle_timeout        60s;

    server {
        listen 443 ssl http2;
        server_name www.example.com;

        ssl_certificate      /etc/nginx/ssl/www.example.com.crt;
        ssl_certificate_key  /etc/nginx/ssl/www.example.com.key;
        ssl_protocols        TLSv1.2 TLSv1.3;
        ssl_ciphers          HIGH:!aNULL:!MD5;

        location / {
            root /var/www/html;
            index index.html;
        }
    }
}
```

### 4.2 高并发优化配置

```nginx
http {
    http2_recv_buffer_size      1m;
    http2_max_concurrent_streams  512;
    http2_max_field_size        16k;
    http2_max_header_size       32k;
    http2_max_requests          10000;
    http2_chunk_size            16k;
    http2_idle_timeout          180s;

    server {
        listen 443 ssl http2 reuseport;
        server_name api.example.com;

        ssl_certificate      /etc/nginx/ssl/api.example.com.crt;
        ssl_certificate_key  /etc/nginx/ssl/api.example.com.key;
        ssl_protocols        TLSv1.2 TLSv1.3;
        ssl_session_cache    shared:SSL:50m;
        ssl_session_timeout  1d;

        location / {
            proxy_pass http://backend;
            proxy_http_version 1.1;
        }
    }
}
```

---

## 5. HTTP/3 (QUIC) 基础

### 5.1 HTTP/3 特性

| 特性 | 说明 |
|------|------|
| **基于 QUIC** | 使用 QUIC 替代 TCP + TLS |
| **0-RTT 连接** | 连接建立时可立即发送数据 |
| **连接迁移** | 网络切换时保持连接（如 WiFi -> 4G）|
| **无队头阻塞** | 单流丢包不影响其他流 |
| **内置加密** | 安全性内置于协议 |

### 5.2 版本要求

- NGINX 1.25.0+ 实验性支持 HTTP/3
- 需要编译 `--with-http_v3_module` 模块
- 需要 `--with-http_quic_module` 模块（QUIC 传输）
- OpenSSL 3.5.1+ 推荐（支持 TLS 1.3 Early Data）
- 或 BoringSSL、LibreSSL、QuicTLS

### 5.3 编译配置

```bash
# 下载源码
wget https://nginx.org/download/nginx-1.25.5.tar.gz
tar -xzf nginx-1.25.5.tar.gz
cd nginx-1.25.5

# 配置编译选项
./configure \
    --with-http_ssl_module \
    --with-http_v2_module \
    --with-http_v3_module \
    --with-http_quic_module \
    --with-cc-opt="-I/path/to/openssl/include" \
    --with-ld-opt="-L/path/to/openssl/lib"

make
sudo make install
```

---

## 6. ngx_http_v3_module 指令详解

### 6.1 http3 指令

| 语法 | 默认值 | 上下文 |
|------|--------|--------|
| `http3 on \| off;` | `off` | `http`, `server` |

启用或禁用 HTTP/3 协议支持。

```nginx
server {
    listen 443 quic reuseport;
    listen 443 ssl;
    http3 on;
    # ...
}
```

### 6.2 quic_gso 指令

| 语法 | 默认值 | 上下文 |
|------|--------|--------|
| `quic_gso on \| off;` | `off` | `http`, `server` |

启用 Generic Segmentation Offloading，提升 UDP 性能。

```nginx
quic_gso on;
```

**要求**：Linux 5.0+ 内核和网卡支持 GSO。

### 6.3 quic_host_key 指令

| 语法 | 默认值 | 上下文 |
|------|--------|--------|
| `quic_host_key file;` | - | `http`, `server` |

指定 QUIC 主机密钥文件，用于生成连接 ID 令牌。

```nginx
quic_host_key /etc/nginx/quic/host.key;
```

**生成密钥**：
```bash
openssl rand -out /etc/nginx/quic/host.key 32
```

### 6.4 quic_retry 指令

| 语法 | 默认值 | 上下文 |
|------|--------|--------|
| `quic_retry on \| off;` | `off` | `http`, `server` |

启用地址验证重试，防止连接放大攻击。

```nginx
quic_retry on;
```

### 6.5 quic_max_udp_payload_size

| 语法 | 默认值 | 上下文 |
|------|--------|--------|
| `quic_max_udp_payload_size size;` | `65527` | `http`, `server` |

设置最大 UDP 负载大小，影响 QUIC 数据包大小。

```nginx
# 标准以太网 MTU
quic_max_udp_payload_size 1200;
```

### 6.6 http3_stream_buffer_size

| 语法 | 默认值 | 上下文 |
|------|--------|--------|
| `http3_stream_buffer_size size;` | `64k` | `http`, `server` |

设置 HTTP/3 流缓冲区大小，用于控制单个流的内存使用。

```nginx
# 大文件传输场景
http3_stream_buffer_size 128k;
```

### 6.7 http3_max_concurrent_streams

| 语法 | 默认值 | 上下文 |
|------|--------|--------|
| `http3_max_concurrent_streams number;` | `128` | `http`, `server` |

设置单个 HTTP/3 连接中最大并发流数量。

```nginx
# 高并发场景
http3_max_concurrent_streams 256;
```

### 6.8 http3_max_field_size

| 语法 | 默认值 | 上下文 |
|------|--------|--------|
| `http3_max_field_size size;` | `4k` | `http`, `server` |

设置 QPACK 压缩后的请求头字段最大大小。

```nginx
# 支持较大的 Cookie 头部
http3_max_field_size 16k;
```

### 6.9 http3_max_table_size

| 语法 | 默认值 | 上下文 |
|------|--------|--------|
| `http3_max_table_size size;` | `16k` | `http`, `server` |

设置 QPACK 动态表最大大小，影响头部压缩效率。

```nginx
# 提升压缩率
http3_max_table_size 32k;
```

---

## 7. ngx_http_quic_module 指令详解

### 7.1 quic_bpf

| 语法 | 默认值 | 上下文 |
|------|--------|--------|
| `quic_bpf on \| off;` | `off` | `http`, `server` |

启用 eBPF 加速 QUIC 连接路由，提升多 worker 场景性能。

**要求**：Linux 5.6+ 内核和 eBPF 支持。

```nginx
# 高流量场景启用 eBPF 加速
quic_bpf on;
```

### 7.2 quic_cc_algorithm

| 语法 | 默认值 | 上下文 |
|------|--------|--------|
| `quic_cc_algorithm algorithm;` | `cubic` | `http`, `server` |

设置 QUIC 拥塞控制算法。

| 算法 | 说明 |
|------|------|
| `cubic` | 默认，适合大多数场景 |
| `reno` | 经典 TCP 拥塞控制 |
| `bbr` | Google BBR，高延迟网络推荐 |

```nginx
# 高延迟网络优化
quic_cc_algorithm bbr;
```

### 7.3 quic_mtu

| 语法 | 默认值 | 上下文 |
|------|--------|--------|
| `quic_mtu size;` | `—` | `http`, `server` |

设置 QUIC MTU 大小，影响数据包分片。

```nginx
# 以太网标准 MTU
quic_mtu 1200;

# 数据中心内部网络
quic_mtu 1400;
```

### 7.4 quic_active_connection_id_limit

| 语法 | 默认值 | 上下文 |
|------|--------|--------|
| `quic_active_connection_id_limit number;` | `8` | `http`, `server` |

设置活跃连接 ID 数量限制，影响连接迁移能力。

```nginx
# 增强连接迁移能力
quic_active_connection_id_limit 16;
```

### 7.5 quic_stack

| 语法 | 默认值 | 上下文 |
|------|--------|--------|
| `quic_stack ngx \| boringssl;` | `ngx` | `http`, `server` |

选择 QUIC 协议栈实现。

| 值 | 说明 |
|------|------|
| `ngx` | nginx 原生实现（推荐） |
| `boringssl` | BoringSSL QUIC 实现 |

```nginx
quic_stack ngx;
```

### 7.6 quic_socket_options

| 语法 | 默认值 | 上下文 |
|------|--------|--------|
| `quic_socket_options option ...;` | `—` | `http`, `server` |

设置 QUIC socket 选项，用于优化 UDP 性能。

```nginx
# 优化 socket 缓冲区
quic_socket_options receive_buffer=1m send_buffer=1m;
```

---

## 9. 0-RTT 连接配置

### 9.1 ssl_early_data 指令

| 语法 | 默认值 | 上下文 |
|------|--------|--------|
| `ssl_early_data on \| off;` | `off` | `http`, `server` |

启用 TLS 1.3 0-RTT Early Data，允许连接建立时发送数据。

```nginx
server {
    listen 443 quic reuseport;
    listen 443 ssl;
    http3 on;

    ssl_certificate      /etc/nginx/ssl/www.example.com.crt;
    ssl_certificate_key  /etc/nginx/ssl/www.example.com.key;
    ssl_protocols        TLSv1.3;        # 必须 TLS 1.3
    ssl_early_data       on;             # 启用 0-RTT

    add_header Alt-Svc 'h3=":443"; ma=86400' always;
}
```

### 9.2 0-RTT 安全注意事项

```nginx
# 限制 0-RTT 请求（防止重放攻击）
server {
    listen 443 quic reuseport;
    listen 443 ssl;
    http3 on;

    ssl_early_data on;

    # 拒绝 0-RTT 中的非幂等请求
    if ($ssl_early_data = "1") {
        return 425;  # Too Early
    }

    location /api/ {
        # API 接口处理
        proxy_pass http://backend;
    }
}
```

---

## 10. HTTP/3 完整配置示例

### 10.1 基础配置

```nginx
http {
    server {
        listen 443 quic reuseport;
        listen 443 ssl;
        server_name www.example.com;

        ssl_certificate      /etc/nginx/ssl/www.example.com.crt;
        ssl_certificate_key  /etc/nginx/ssl/www.example.com.key;
        ssl_protocols        TLSv1.3;
        ssl_early_data       on;

        http3 on;
        quic_gso on;
        quic_retry on;

        # 告知客户端支持 HTTP/3
        add_header Alt-Svc 'h3=":443"; ma=86400' always;

        location / {
            root /var/www/html;
        }
    }
}
```

### 10.2 生产环境配置

```nginx
http {
    # HTTP/2 和 HTTP/3 共存
    http2_recv_buffer_size  512k;

    server {
        # HTTP/3 QUIC 监听
        listen 443 quic reuseport;
        # HTTP/2 HTTPS 监听
        listen 443 ssl;

        server_name www.example.com;

        # SSL 配置
        ssl_certificate      /etc/nginx/ssl/www.example.com.crt;
        ssl_certificate_key  /etc/nginx/ssl/www.example.com.key;
        ssl_protocols        TLSv1.2 TLSv1.3;
        ssl_ciphers          ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256;
        ssl_session_cache    shared:SSL:50m;
        ssl_session_timeout  1d;

        # HTTP/2 启用
        http2 on;

        # HTTP/3 启用
        http3 on;
        quic_gso on;
        quic_retry on;
        quic_host_key /etc/nginx/quic/host.key;
        quic_max_udp_payload_size 1350;

        # 0-RTT
        ssl_early_data on;

        # 协议升级提示
        add_header Alt-Svc 'h3=":443"; ma=2592000' always;

        # 安全头部
        add_header Strict-Transport-Security "max-age=31536000; includeSubDomains" always;
        add_header X-Content-Type-Options "nosniff" always;

        location / {
            root /var/www/html;
            index index.html;
        }
    }

    # HTTP 重定向到 HTTPS
    server {
        listen 80;
        server_name www.example.com;
        return 301 https://$server_name$request_uri;
    }
}
```

---

## 11. HTTP/1.1 vs HTTP/2 vs HTTP/3 对比

### 11.1 特性对比表

| 特性 | HTTP/1.1 | HTTP/2 | HTTP/3 |
|------|----------|--------|--------|
| **传输层** | TCP | TCP | QUIC/UDP |
| **多路复用** | 否（串行或连接池）| 是 | 是 |
| **头部压缩** | 否 | HPACK | QPACK |
| **服务器推送** | 否 | 是（已弃用）| 否 |
| **队头阻塞** | 有 | TCP 层 | 无 |
| **连接建立** | 1-2 RTT | 1-2 RTT + TLS | 0-1 RTT |
| **连接迁移** | 否 | 否 | 是 |
| **加密要求** | 可选 | TLS 推荐 | 强制 |
| **握手延迟** | 高 | 中 | 低 |
| **NAT 友好** | 是 | 是 | 需特殊处理 |

### 11.2 性能对比

| 场景 | HTTP/1.1 | HTTP/2 | HTTP/3 |
|------|----------|--------|--------|
| **首屏加载** | 基准 | 20-50% 提升 | 30-60% 提升 |
| **高延迟网络** | 基准 | 30-40% 提升 | 50-80% 提升 |
| **丢包网络** | 基准 | 轻微下降 | 显著提升 |
| **移动网络切换** | 需重连 | 需重连 | 无缝迁移 |

### 11.3 适用场景

| 协议 | 推荐场景 |
|------|----------|
| **HTTP/1.1** | 兼容旧客户端、简单代理、健康检查 |
| **HTTP/2** | 现代 Web 应用、API 服务、服务器推送（遗留）|
| **HTTP/3** | 移动应用、实时通信、高延迟网络、视频流 |

---

## 12. 迁移指南和兼容性

### 12.1 渐进式迁移策略

```nginx
http {
    # 同时支持 HTTP/2 和 HTTP/3
    server {
        listen 443 quic reuseport;
        listen 443 ssl;

        server_name www.example.com;

        ssl_certificate     /etc/nginx/ssl/www.example.com.crt;
        ssl_certificate_key /etc/nginx/ssl/www.example.com.key;
        ssl_protocols       TLSv1.2 TLSv1.3;

        # HTTP/2
        http2 on;

        # HTTP/3（实验性）
        http3 on;

        # 告知客户端支持 HTTP/3
        add_header Alt-Svc 'h3=":443"; ma=86400' always;

        location / {
            root /var/www/html;
        }
    }
}
```

### 12.2 Alt-Svc 头部详解

```nginx
# 基础声明
add_header Alt-Svc 'h3=":443"; ma=86400' always;

# 多协议声明
add_header Alt-Svc 'h3=":443"; h3-29=":443"; ma=86400' always;

# 指定不同端口
add_header Alt-Svc 'h3=":8443"; ma=3600' always;
```

**参数说明**：
- `h3`：HTTP/3 协议
- `h3-29`：HTTP/3 草案版本（兼容旧客户端）
- `ma`：最大有效期（秒）

### 12.3 浏览器兼容性

| 浏览器 | HTTP/2 | HTTP/3 |
|--------|--------|--------|
| Chrome 49+ | 支持 | 87+ 实验性，后续稳定 |
| Firefox 36+ | 支持 | 88+ 实验性，后续稳定 |
| Safari 11+ | 支持 | 14+ 实验性，后续稳定 |
| Edge 79+ | 支持 | 87+ 实验性，后续稳定 |

### 12.4 回退策略

```nginx
map $http_user_agent $supports_http3 {
    default 0;
    "~*Chrome/8[0-9]" 1;
    "~*Firefox/8[0-9]" 1;
    "~*Safari/1[4-9]" 1;
}

server {
    listen 443 ssl http2;

    location / {
        # 根据客户端能力调整
        if ($supports_http3) {
            add_header Alt-Svc 'h3=":443"; ma=86400' always;
        }
        proxy_pass http://backend;
    }
}
```

---

## 13. 性能优化建议

### 13.1 HTTP/2 优化

```nginx
http {
    # 1. 调整 worker 进程数
    worker_processes auto;

    # 2. 优化连接参数
    http2_max_concurrent_streams  256;
    http2_recv_buffer_size        512k;
    http2_idle_timeout            180s;

    server {
        listen 443 ssl http2;

        # 3. 启用 TLS 会话复用
        ssl_session_cache    shared:SSL:50m;
        ssl_session_timeout  1d;
        ssl_session_tickets  on;

        # 4. 长连接优化
        keepalive_timeout    65;
        keepalive_requests   1000;

        location / {
            # 5. 资源文件缓存
            location ~* \.(css|js|png|jpg|jpeg|gif|ico|svg)$ {
                expires 30d;
                add_header Cache-Control "public, immutable";
            }

            root /var/www/html;
        }
    }
}
```

### 13.2 HTTP/3 优化

```nginx
http {
    server {
        listen 443 quic reuseport;
        listen 443 ssl;

        http3 on;

        # 1. 启用 GSO 提升 UDP 性能
        quic_gso on;

        # 2. 启用地址验证（防止攻击）
        quic_retry on;

        # 3. 优化 UDP 包大小
        quic_max_udp_payload_size 1350;

        # 4. 使用 0-RTT
        ssl_early_data on;

        # 5. 会话缓存
        ssl_session_cache    shared:SSL:50m;
        ssl_session_timeout  1d;

        location / {
            proxy_pass http://backend;
        }
    }
}
```

### 13.3 内核参数优化

```bash
# /etc/sysctl.conf

# UDP 缓冲区优化（HTTP/3）
net.core.rmem_max = 16777216
net.core.wmem_max = 16777216
net.ipv4.udp_rmem = 4096 87380 16777216
net.ipv4.udp_wmem = 4096 65536 16777216

# TCP 缓冲区优化（HTTP/2）
net.ipv4.tcp_rmem = 4096 87380 16777216
net.ipv4.tcp_wmem = 4096 65536 16777216
net.ipv4.tcp_congestion_control = bbr

# 连接跟踪
net.netfilter.nf_conntrack_udp_timeout = 60
net.netfilter.nf_conntrack_udp_timeout_stream = 120

# 应用配置
sysctl -p
```

### 13.4 监控指标

```nginx
# 在日志中记录协议版本
log_format main '$remote_addr - $remote_user [$time_local] "$request" '
                '$status $body_bytes_sent "$http_referer" '
                '"$http_user_agent" $server_protocol '
                'ssl=$ssl_protocol http2=$http2 http3=$http3';

server {
    access_log /var/log/nginx/access.log main;
}
```

### 13.5 调试检查

```bash
# 检查 HTTP/2 支持
curl -I --http2 https://www.example.com

# 检查 HTTP/3 支持（需要支持 HTTP/3 的 curl）
curl -I --http3 https://www.example.com

# 使用 quiche 客户端测试
# https://github.com/cloudflare/quiche

# 查看 NGINX HTTP/3 统计
curl https://www.example.com/nginx_stats

# 检查 Alt-Svc 头部
curl -I https://www.example.com | grep -i alt-svc
```

---

## 14. 常见问题排查

### 14.1 HTTP/2 问题

| 问题 | 原因 | 解决 |
|------|------|------|
| 浏览器降级 HTTP/1.1 | ALPN 未启用 | OpenSSL 1.0.2+ |
| 大量 STREAM_CLOSED 错误 | 客户端提前关闭 | 正常行为，无需处理 |
| 内存占用高 | 流数量过多 | 减少 http2_max_concurrent_streams |

### 14.2 HTTP/3 问题

| 问题 | 原因 | 解决 |
|------|------|------|
| 无法建立连接 | 防火墙阻断 UDP | 开放 UDP 443 |
| 连接不稳定 | MTU 设置不当 | 调整 quic_max_udp_payload_size |
| 0-RTT 失败 | 会话票据无效 | 检查 ssl_session_ticket_key |
| NAT 超时 | UDP 连接被清理 | 缩短 http3_idle_timeout |

---

## 15. 参考链接

- [NGINX HTTP/2 文档](https://nginx.org/en/docs/http/ngx_http_v2_module.html)
- [NGINX HTTP/3 文档](https://nginx.org/en/docs/http/ngx_http_v3_module.html)
- [HTTP/2 RFC 7540](https://tools.ietf.org/html/rfc7540)
- [HTTP/3 RFC 9114](https://tools.ietf.org/html/rfc9114)
- [QUIC RFC 9000](https://tools.ietf.org/html/rfc9000)
- [Cloudflare HTTP/3 实践](https://developers.cloudflare.com/http3/)
