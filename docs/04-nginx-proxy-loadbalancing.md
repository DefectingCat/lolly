# NGINX 反向代理与负载均衡指南

## 1. 反向代理基础

### 什么是反向代理

反向代理服务器接收客户端请求，将请求转发给后端服务器，获取响应后返回给客户端。NGINX 作为反向代理可以：

- 隐藏后端服务器真实地址
- 实现负载均衡
- 缓存响应内容
- SSL 终端加密
- 压缩响应内容
- 请求路由与重写

### 基础配置示例

```nginx
server {
    listen 80;
    server_name example.com;

    location / {
        proxy_pass http://backend.example.com:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    }
}
```

---

## 2. proxy_pass 指令详解

### 语法

`proxy_pass URL;`

URL 可以是：
- HTTP 地址：`http://backend:8080`
- HTTPS 地址：`https://backend:8443`
- Unix Socket：`unix:/tmp/backend.socket`
- upstream 组：`http://backend_group`
- 变量：`http://$backend`

### URI 传递规则

**带 URI 的 proxy_pass**：请求 URI 中匹配 location 的部分会被替换。

```nginx
location /name/ {
    proxy_pass http://127.0.0.1/remote/;
    # /name/test -> /remote/test
}

location /api/ {
    proxy_pass http://backend/v1/;
    # /api/users -> /v1/users
}
```

**不带 URI 的 proxy_pass**：请求 URI 以原始形式传递。

```nginx
location /some/path/ {
    proxy_pass http://127.0.0.1;
    # /some/path/test -> /some/path/test
}
```

**使用变量**：

```nginx
location / {
    proxy_pass http://$backend;
    # 需要配合 resolver 指令解析域名
}

resolver 10.0.0.1 valid=300s;
```

---

## 3. 请求头设置

### proxy_set_header

设置传递给后端服务器的请求头。

```nginx
proxy_set_header Host $host;                   # 传递原始 Host
proxy_set_header Host $http_host;              # 传递 Host 头（含端口）
proxy_set_header Host backend.example.com;     # 固定 Host 值

proxy_set_header X-Real-IP $remote_addr;       # 客户端真实 IP
proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;  # 代理链
proxy_set_header X-Forwarded-Proto $scheme;    # 原始协议

# 删除请求头
proxy_set_header Accept-Encoding "";           # 删除该字段
```

### 默认行为

| 头字段 | 默认值 |
|--------|--------|
| `Host` | `$proxy_host`（proxy_pass 中的地址） |
| `Connection` | `close` |

---

## 4. 负载均衡配置

### upstream 块定义

```nginx
upstream backend {
    server backend1.example.com weight=5;
    server backend2.example.com:8080;
    server 192.168.0.1:8080 max_fails=3 fail_timeout=30s;
    server backend3.example.com backup;        # 备份服务器
    server unix:/tmp/backend4;
}

server {
    location / {
        proxy_pass http://backend;
    }
}
```

### 负载均衡算法

| 算法 | 指令 | 说明 |
|------|------|------|
| **轮询** | 默认 | 请求依次分发（加权） |
| **最少连接** | `least_conn;` | 分配给活动连接最少的服务器 |
| **IP Hash** | `ip_hash;` | 同一客户端 IP 始终路由到同一服务器 |
| **Hash** | `hash key [consistent];` | 基于指定键哈希，支持一致性哈希 |
| **随机** | `random [two [method]];` | 随机选择，two 表示选两台再择优 |

### 配置示例

**轮询（默认）**：
```nginx
upstream backend {
    server srv1.example.com;
    server srv2.example.com;
    server srv3.example.com;
}
```

**加权轮询**：
```nginx
upstream backend {
    server srv1.example.com weight=5;  # 5/7 的请求
    server srv2.example.com weight=2;  # 2/7 的请求
    server srv3.example.com;           # 1/7 的请求（默认 weight=1）
}
```

**最少连接**：
```nginx
upstream backend {
    least_conn;
    server srv1.example.com;
    server srv2.example.com;
    server srv3.example.com;
}
```

**IP Hash（会话持久性）**：
```nginx
upstream backend {
    ip_hash;
    server srv1.example.com;
    server srv2.example.com;
    server srv3.example.com;
}
```

**一致性哈希**：
```nginx
upstream backend {
    hash $request_uri consistent;
    server srv1.example.com;
    server srv2.example.com;
    server srv3.example.com;
}
```

### server 指令参数

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `weight=N` | 权重值 | 1 |
| `max_conns=N` | 最大并发连接数 | 0（无限制） |
| `max_fails=N` | 失败次数阈值 | 1 |
| `fail_timeout=T` | 失败统计时间及不可用持续时间 | 10s |
| `backup` | 备份服务器（主服务器不可用时使用） | - |
| `down` | 标记为永久不可用 | - |
| `resolve` | 监控域名 IP 变化（需 zone + resolver） | - |

```nginx
upstream backend {
    zone backend 64k;
    resolver 10.0.0.1;

    server backend1.example.com weight=5 max_fails=3 fail_timeout=30s;
    server backend2.example.com resolve;
    server backup1.example.com backup;
}
```

---

## 5. 健康检查

### 被动健康检查（内置）

```nginx
upstream backend {
    server srv1.example.com max_fails=3 fail_timeout=30s;
    server srv2.example.com max_fails=3 fail_timeout=30s;
}
```

**机制**：
- 在 `fail_timeout` 时间内连续失败 `max_fails` 次，服务器标记为不可用
- `fail_timeout` 时间后再次尝试

### 主动健康检查（NGINX Plus）

```nginx
upstream backend {
    zone backend 64k;

    server srv1.example.com;
    server srv2.example.com;

    health_check interval=5s fails=3 passes=2;
    health_check uri=/health;
}
```

---

## 6. 超时配置

### 主要超时指令

| 指令 | 说明 | 默认值 |
|------|------|--------|
| `proxy_connect_timeout` | 建立连接超时 | 60s |
| `proxy_send_timeout` | 传输请求超时 | 60s |
| `proxy_read_timeout` | 读取响应超时 | 60s |

```nginx
location / {
    proxy_connect_timeout 5s;
    proxy_send_timeout 10s;
    proxy_read_timeout 30s;
    proxy_pass http://backend;
}
```

---

## 7. 缓冲配置

### 响应缓冲

```nginx
proxy_buffering on;                  # 默认 on
proxy_buffer_size 4k;                # 响应头缓冲区大小
proxy_buffers 8 16k;                 # 响应体缓冲区数量和大小
proxy_busy_buffers_size 32k;         # 同时发送给客户端的缓冲区总大小
proxy_max_temp_file_size 1024m;      # 临时文件最大大小
proxy_temp_file_write_size 64k;      # 每次写入临时文件大小
```

### 禁用缓冲（实时传输）

```nginx
location /stream/ {
    proxy_buffering off;
    proxy_pass http://backend;
}
```

---

## 8. 缓存配置

### 缓存路径定义

```nginx
http {
    proxy_cache_path /data/nginx/cache
        levels=1:2                    # 目录层级（1:2 表示 16*256 个子目录）
        keys_zone=one:10m             # 共享内存区名称和大小（1MB 约 8000 个键）
        inactive=60m                  # 非活动数据保留时间
        max_size=1g                   # 缓存最大大小
        use_temp_path=off;            # 临时文件存放位置
}
```

### 启用缓存

```nginx
server {
    location / {
        proxy_cache one;              # 使用定义的缓存区
        proxy_cache_key "$host$request_uri";  # 缓存键
        proxy_cache_valid 200 302 10m;        # 200/302 响应缓存 10 分钟
        proxy_cache_valid 404 1m;             # 404 响应缓存 1 分钟
        proxy_cache_valid any 1m;             # 其他响应缓存 1 分钟
        proxy_pass http://backend;
    }
}
```

### 缓存条件控制

```nginx
# 不从缓存获取响应的条件
proxy_cache_bypass $cookie_nocache $arg_nocache;

# 不将响应保存到缓存的条件
proxy_no_cache $http_pragma $http_authorization;
```

### 使用过期缓存

```nginx
proxy_cache_use_stale error timeout updating http_500 http_502 http_503 http_504;
# 在后端错误、超时、正在更新时使用过期缓存
```

### 缓存锁

```nginx
proxy_cache_lock on;                 # 同时刻只允许一个请求填充缓存
proxy_cache_lock_timeout 5s;         # 锁超时时间
```

---

## 9. 故障转移

### proxy_next_upstream

定义在何种情况下将请求传递给下一台服务器。

```nginx
proxy_next_upstream error timeout invalid_header http_500 http_502 http_503 http_504;
proxy_next_upstream_timeout 30s;     # 限制总时间
proxy_next_upstream_tries 3;         # 限制尝试次数
```

**条件类型**：

| 条件 | 说明 |
|------|------|
| `error` | 与后端建立连接出错 |
| `timeout` | 连接、传输或读取超时 |
| `invalid_header` | 后端返回空或无效响应头 |
| `http_XXX` | 后端返回指定状态码 |
| `non_idempotent` | 非幂等请求（POST、LOCK）也进行重试 |

---

## 10. SSL/HTTPS 代理

### 代理到 HTTPS 后端

```nginx
location / {
    proxy_pass https://backend.example.com;
    proxy_ssl_verify on;                         # 验证后端证书
    proxy_ssl_trusted_certificate /path/to/ca.crt;
    proxy_ssl_verify_depth 2;
    proxy_ssl_server_name on;                    # 启用 SNI
}
```

### 代理 SSL 配置

| 指令 | 说明 | 默认值 |
|------|------|--------|
| `proxy_ssl` | 启用 HTTPS 代理 | off |
| `proxy_ssl_protocols` | 启用的协议 | TLSv1.2 TLSv1.3 |
| `proxy_ssl_ciphers` | 加密套件 | DEFAULT |
| `proxy_ssl_verify` | 验证后端证书 | off |
| `proxy_ssl_verify_depth` | 验证深度 | 1 |
| `proxy_ssl_server_name` | 启用 SNI | off |
| `proxy_ssl_certificate` | 客户端证书 | - |
| `proxy_ssl_certificate_key` | 客户端密钥 | - |

---

## 11. WebSocket 代理

### 基础配置

```nginx
location /chat/ {
    proxy_pass http://backend;
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "upgrade";
}
```

### 动态处理

```nginx
http {
    map $http_upgrade $connection_upgrade {
        default upgrade;
        ''      close;
    }

    server {
        location /chat/ {
            proxy_pass http://backend;
            proxy_http_version 1.1;
            proxy_set_header Upgrade $http_upgrade;
            proxy_set_header Connection $connection_upgrade;
            proxy_read_timeout 3600s;  # 增加超时时间
        }
    }
}
```

---

## 12. FastCGI 代理

### 基础配置

```nginx
location ~ \.php$ {
    fastcgi_pass  localhost:9000;
    fastcgi_index index.php;
    fastcgi_param SCRIPT_FILENAME $document_root$fastcgi_script_name;
    include fastcgi_params;
}
```

### 常用 FastCGI 指令

| 指令 | 说明 |
|------|------|
| `fastcgi_pass` | FastCGI 服务器地址 |
| `fastcgi_index` | URI 以斜杠结尾时追加的文件名 |
| `fastcgi_param` | 传递参数给 FastCGI 服务器 |
| `fastcgi_split_path_info` | 分离 SCRIPT_NAME 和 PATH_INFO |
| `fastcgi_connect_timeout` | 连接超时 |
| `fastcgi_read_timeout` | 读取超时 |
| `fastcgi_buffer_size` | 响应头缓冲区大小 |
| `fastcgi_buffers` | 响应体缓冲区 |

---

## 13. 内置变量

| 变量 | 说明 |
|------|------|
| `$proxy_host` | proxy_pass 中的服务器名称和端口 |
| `$proxy_port` | proxy_pass 中的端口 |
| `$proxy_add_x_forwarded_for` | X-Forwarded-For 头 + 客户端 IP |

---

## 14. 综合配置示例

```nginx
http {
    upstream backend {
        zone backend 64k;
        least_conn;

        server backend1.example.com weight=5 max_fails=3 fail_timeout=30s;
        server backend2.example.com resolve;
        server backup.example.com backup;

        keepalive 32;
    }

    proxy_cache_path /data/nginx/cache levels=1:2 keys_zone=main:10m inactive=60m max_size=1g;

    server {
        listen 80;
        server_name example.com;

        location / {
            proxy_pass http://backend;
            proxy_http_version 1.1;
            proxy_set_header Connection "";

            proxy_set_header Host $host;
            proxy_set_header X-Real-IP $remote_addr;
            proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;

            proxy_connect_timeout 5s;
            proxy_read_timeout 30s;

            proxy_cache main;
            proxy_cache_key "$host$request_uri";
            proxy_cache_valid 200 10m;

            proxy_next_upstream error timeout http_502 http_503;
        }

        location /api/ {
            proxy_pass http://backend;
            proxy_set_header Host $host;
            proxy_buffering off;
        }

        location /ws/ {
            proxy_pass http://backend;
            proxy_http_version 1.1;
            proxy_set_header Upgrade $http_upgrade;
            proxy_set_header Connection $connection_upgrade;
            proxy_read_timeout 3600s;
        }
    }
}
```