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

## 12. 高级代理指令

### proxy_bind

指定连接后端时使用的源地址，用于多网卡服务器选择出口IP。

```nginx
语法: proxy_bind address [transparent];
默认: —
上下文: http, server, location
```

```nginx
proxy_bind $server_addr;              # 使用服务器IP
proxy_bind 192.168.1.1 transparent;   # 透明代理（需要root权限）
```

### proxy_intercept_errors

拦截后端错误响应，配合 error_page 自定义错误页面。

```nginx
语法: proxy_intercept_errors on | off;
默认: off
上下文: http, server, location
```

```nginx
proxy_intercept_errors on;
error_page 500 502 503 504 /50x.html;
```

### proxy_hide_header / proxy_pass_header

控制后端响应头的传递行为：

```nginx
# 隐藏后端返回的特定头
proxy_hide_header X-Powered-By;
proxy_hide_header X-Runtime;

# 传递被默认隐藏的头
proxy_pass_header X-Accel-Redirect;
proxy_pass_header X-Accel-Limit-Rate;
```

### proxy_ignore_headers

忽略后端的特定响应头（如缓存控制），允许NGINX处理这些头：

```nginx
proxy_ignore_headers Cache-Control Expires X-Accel-Redirect X-Accel-Expires;
```

### proxy_cookie_* 系列

修改后端返回的 Set-Cookie 头：

```nginx
# 修改域名
proxy_cookie_domain localhost example.com;
proxy_cookie_domain off;              # 禁用域名修改

# 修改路径
proxy_cookie_path /foo/ /bar/;
proxy_cookie_path off;                # 禁用路径修改

# 添加安全标志
proxy_cookie_flags session httponly secure samesite=strict;
proxy_cookie_flags * samesite=lax;    # 应用到所有cookie
```

### proxy_limit_rate

限制从后端读取响应的传输速率：

```nginx
proxy_limit_rate 100k;                # 100KB/s
```

### proxy_request_buffering

控制请求是否先完整缓冲再发送到后端：

```nginx
proxy_request_buffering on;           # 默认，完整缓冲
proxy_request_buffering off;          # 流式传输，支持上传进度
```

### proxy_redirect

修改后端返回的重定向头 Location 和 Refresh：

```nginx
proxy_redirect default;                                    # 使用默认替换
proxy_redirect off;                                        # 禁用替换
proxy_redirect http://localhost:8080/ http://$host/;       # 自定义替换
proxy_redirect ~^http://([^/]+)/(.+)$ http://$host/$2;      # 使用正则
```

---

## 13. SSL 客户端证书认证 (proxy_ssl_*)

用于 mTLS 双向认证场景，NGINX 作为客户端向后端提供证书：

```nginx
location / {
    proxy_pass https://backend.example.com;

    # mTLS 双向认证
    proxy_ssl_certificate /path/to/client.crt;
    proxy_ssl_certificate_key /path/to/client.key;

    # 验证后端证书
    proxy_ssl_verify on;
    proxy_ssl_trusted_certificate /path/to/ca.crt;
    proxy_ssl_verify_depth 2;

    # SSL协议和加密套件
    proxy_ssl_protocols TLSv1.2 TLSv1.3;
    proxy_ssl_ciphers HIGH:!aNULL;

    # 会话复用
    proxy_ssl_session_reuse on;

    # SNI支持
    proxy_ssl_server_name on;
    proxy_ssl_name backend.example.com;
}
```

| 指令 | 说明 | 默认值 |
|------|------|--------|
| `proxy_ssl_certificate` | 客户端证书路径 | — |
| `proxy_ssl_certificate_key` | 客户端私钥路径 | — |
| `proxy_ssl_verify` | 验证后端证书 | off |
| `proxy_ssl_trusted_certificate` | 受信任CA证书 | — |
| `proxy_ssl_verify_depth` | 验证深度 | 1 |
| `proxy_ssl_protocols` | 启用的协议 | TLSv1.2 TLSv1.3 |
| `proxy_ssl_ciphers` | 加密套件 | DEFAULT |
| `proxy_ssl_session_reuse` | 会话复用 | on |
| `proxy_ssl_name` | SNI名称 | — |

---

## 14. 高级缓存指令

### proxy_cache_methods

指定可缓存的请求方法：

```nginx
proxy_cache_methods GET HEAD POST;    # 可缓存 POST 请求
```

### proxy_cache_min_uses

设置最小访问次数才开始缓存，避免缓存低频请求：

```nginx
proxy_cache_min_uses 3;               # 第3次访问才开始缓存
```

### proxy_cache_background_update

后台异步更新过期缓存（类似 stale-while-revalidate）：

```nginx
proxy_cache_background_update on;
```

### proxy_cache_revalidate

使用 If-Modified-Since 和 If-None-Match 重新验证缓存：

```nginx
proxy_cache_revalidate on;            # 减少数据传输
```

### proxy_cache_convert_head

自动将 HEAD 请求转为 GET 以获取响应体：

```nginx
proxy_cache_convert_head on;          # 默认启用
```

### proxy_cache_purge

支持 PURGE 方法清除缓存（需编译时启用）：

```nginx
location ~ /purge(/.*) {
    proxy_cache_purge cache_zone $1;
}
```

---

## 15. FastCGI 代理

### 基础配置

```nginx
location ~ \.php$ {
    fastcgi_pass  localhost:9000;
    fastcgi_index index.php;
    fastcgi_param SCRIPT_FILENAME $document_root$fastcgi_script_name;
    include fastcgi_params;
}
```

### FastCGI 指令完整列表

| 指令 | 语法 | 默认值 | 上下文 |
|------|------|--------|--------|
| `fastcgi_pass` | fastcgi_pass address; | — | location |
| `fastcgi_index` | fastcgi_index name; | — | http, server, location |
| `fastcgi_param` | fastcgi_param parameter value [if_not_empty]; | — | http, server, location |
| `fastcgi_split_path_info` | fastcgi_split_path_info regex; | — | location |
| `fastcgi_buffer_size` | fastcgi_buffer_size size; | 4k/8k | http, server, location |
| `fastcgi_buffers` | fastcgi_buffers number size; | 8 4k/8k | http, server, location |
| `fastcgi_busy_buffers_size` | fastcgi_busy_buffers_size size; | 8k/16k | http, server, location |
| `fastcgi_temp_file_write_size` | fastcgi_temp_file_write_size size; | 8k/16k | http, server, location |
| `fastcgi_temp_path` | fastcgi_temp_path path [level1 [level2 [level3]]]; | — | http, server, location |
| `fastcgi_cache` | fastcgi_cache zone; | — | http, server, location |
| `fastcgi_cache_key` | fastcgi_cache_key string; | — | http, server, location |
| `fastcgi_cache_valid` | fastcgi_cache_valid [code...] time; | — | http, server, location |
| `fastcgi_cache_methods` | fastcgi_cache_methods method...; | GET HEAD | http, server, location |
| `fastcgi_cache_min_uses` | fastcgi_cache_min_uses number; | 1 | http, server, location |
| `fastcgi_cache_bypass` | fastcgi_cache_bypass string...; | — | http, server, location |
| `fastcgi_no_cache` | fastcgi_no_cache string...; | — | http, server, location |
| `fastcgi_cache_use_stale` | fastcgi_cache_use_stale condition...; | — | http, server, location |
| `fastcgi_cache_background_update` | fastcgi_cache_background_update on/off; | off | http, server, location |
| `fastcgi_cache_revalidate` | fastcgi_cache_revalidate on/off; | off | http, server, location |
| `fastcgi_cache_lock` | fastcgi_cache_lock on/off; | off | http, server, location |
| `fastcgi_cache_lock_timeout` | fastcgi_cache_lock_timeout time; | 5s | http, server, location |
| `fastcgi_cache_convert_head` | fastcgi_cache_convert_head on/off; | on | http, server, location |
| `fastcgi_connect_timeout` | fastcgi_connect_timeout time; | 60s | http, server, location |
| `fastcgi_send_timeout` | fastcgi_send_timeout time; | 60s | http, server, location |
| `fastcgi_read_timeout` | fastcgi_read_timeout time; | 60s | http, server, location |
| `fastcgi_send_lowat` | fastcgi_send_lowat size; | 0 | http, server, location |
| `fastcgi_request_buffering` | fastcgi_request_buffering on/off; | on | http, server, location |
| `fastcgi_intercept_errors` | fastcgi_intercept_errors on/off; | off | http, server, location |
| `fastcgi_hide_header` | fastcgi_hide_header field; | — | http, server, location |
| `fastcgi_pass_header` | fastcgi_pass_header field; | — | http, server, location |
| `fastcgi_ignore_headers` | fastcgi_ignore_headers field...; | — | http, server, location |
| `fastcgi_limit_rate` | fastcgi_limit_rate rate; | 0 | http, server, location |

### FastCGI 缓存完整配置示例

```nginx
http {
    # 缓存路径定义
    fastcgi_cache_path /var/cache/nginx/php
        levels=1:2
        keys_zone=php:10m
        max_size=100m
        inactive=60m
        use_temp_path=off;

    server {
        listen 80;
        server_name example.com;

        location ~ \.php$ {
            # 启用缓存
            fastcgi_cache php;
            fastcgi_cache_key "$scheme$request_method$host$request_uri";

            # 缓存有效期
            fastcgi_cache_valid 200 302 1h;
            fastcgi_cache_valid 404 1m;
            fastcgi_cache_valid any 5m;

            # 使用过期缓存
            fastcgi_cache_use_stale error timeout updating http_500 http_503;

            # 后台更新
            fastcgi_cache_background_update on;

            # 重新验证
            fastcgi_cache_revalidate on;

            # 缓存锁
            fastcgi_cache_lock on;
            fastcgi_cache_lock_timeout 5s;

            # 绕过缓存条件
            fastcgi_cache_bypass $cookie_nocache $arg_nocache;
            fastcgi_no_cache $http_pragma $http_authorization;

            # FastCGI 后端
            fastcgi_pass unix:/run/php/php-fpm.sock;
            fastcgi_index index.php;
            fastcgi_param SCRIPT_FILENAME $document_root$fastcgi_script_name;

            # 超时设置
            fastcgi_connect_timeout 5s;
            fastcgi_send_timeout 60s;
            fastcgi_read_timeout 60s;

            # 缓冲配置
            fastcgi_buffer_size 16k;
            fastcgi_buffers 8 16k;
            fastcgi_busy_buffers_size 32k;

            # 错误处理
            fastcgi_intercept_errors on;

            include fastcgi_params;
        }
    }
}
```

---

## 16. 内置变量

| 变量 | 说明 |
|------|------|
| `$proxy_host` | proxy_pass 中的服务器名称和端口 |
| `$proxy_port` | proxy_pass 中的端口 |
| `$proxy_add_x_forwarded_for` | X-Forwarded-For 头 + 客户端 IP |

---

## 17. 综合配置示例

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