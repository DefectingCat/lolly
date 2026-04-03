# NGINX 限流与连接控制详解

## 1. ngx_http_limit_req_module (请求限流)

NGINX 的请求限流模块使用**令牌桶算法**实现，可以控制客户端的请求速率，保护后端服务器免受过载攻击。

### 1.1 limit_req_zone 定义限流区域

在 `http` 上下文中定义限流区域：

```nginx
http {
    # 基于 IP 地址的限流区域
    limit_req_zone $binary_remote_addr zone=ip_limit:10m rate=10r/s;

    # 基于服务器名称的限流区域
    limit_req_zone $server_name zone=server_limit:10m rate=100r/s;

    # 基于请求 URI 的限流区域
    limit_req_zone $uri zone=uri_limit:10m rate=5r/s;

    # 基于用户名的限流区域（需配合 auth_basic）
    limit_req_zone $remote_user zone=user_limit:10m rate=30r/m;
}
```

**指令参数说明**：

| 参数 | 说明 |
|------|------|
| `$binary_remote_addr` | 客户端二进制 IP 地址（节省内存） |
| `$remote_addr` | 客户端 IP 地址（文本格式） |
| `zone=name:size` | 共享内存区域名称和大小 |
| `rate=Nr/s` 或 `rate=Nr/m` | 限流速率（每秒/每分钟请求数） |

**内存使用估算**：
- 1MB 共享内存约可存储 16,000 个 IP 地址状态（使用 `$binary_remote_addr`）
- 1MB 共享内存约可存储 8,000 个 IP 地址状态（使用 `$remote_addr`）

### 1.2 limit_req 应用限流

在 `server` 或 `location` 上下文中应用限流：

```nginx
server {
    location /api/ {
        limit_req zone=ip_limit burst=20 nodelay;
    }
}
```

**参数说明**：

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `zone=name` | 使用的限流区域 | 必需 |
| `burst=N` | 突发请求数量（桶容量） | 0 |
| `nodelay` | 不延迟过量请求，立即处理 | - |
| `delay=N` | 开始延迟的阈值 | - |

### 1.3 limit_req_status 设置拒绝状态码

```nginx
server {
    location /api/ {
        limit_req zone=ip_limit burst=20;
        limit_req_status 429;  # 超过限流返回 429 Too Many Requests
    }
}
```

**常用状态码**：
- `503` - Service Unavailable（默认）
- `429` - Too Many Requests（推荐用于限流场景）

### 1.4 limit_req_log_level 设置日志级别

```nginx
server {
    location /api/ {
        limit_req zone=ip_limit burst=20;
        limit_req_log_level warn;  # 限流触发时使用 warn 级别记录
    }
}
```

**可选级别**：`info`, `notice`, `warn`, `error`

### 1.5 burst 和 nodelay 参数详解

**令牌桶算法原理**：

令牌桶算法包含两个核心概念：
- **速率 (rate)**：每秒向桶中放入的令牌数量
- **容量 (burst)**：桶中可容纳的最大令牌数

请求处理流程：
1. 每个请求需要消耗一个令牌
2. 如果桶中有令牌，请求立即处理
3. 如果桶中无令牌，请求被延迟或拒绝

**配置模式对比**：

**模式一：严格限流（无 burst）**
```nginx
limit_req zone=ip_limit;  # rate=1r/s
```
- 每秒只允许 1 个请求
- 多余请求立即返回 503

**模式二：允许突发（有 burst 无 nodelay）**
```nginx
limit_req zone=ip_limit burst=10;
```
- 允许突发 10 个请求
- 过量请求排队延迟处理
- 按 rate 速率逐渐释放

**模式三：立即处理突发（有 burst 有 nodelay）**
```nginx
limit_req zone=ip_limit burst=10 nodelay;
```
- 允许突发 10 个请求立即处理
- 超过 burst 的请求返回 503
- 最常用配置，用户体验最佳

**模式四：延迟阈值（使用 delay）**
```nginx
limit_req zone=ip_limit burst=20 delay=10;
```
- 前 10 个突发请求立即处理
- 第 11-20 个请求延迟处理
- 超过 20 个请求返回 503

### 1.6 多区域限流配置

**分层限流策略**：

```nginx
http {
    # 全局 IP 限流：每秒 10 请求
    limit_req_zone $binary_remote_addr zone=ip_global:10m rate=10r/s;

    # API 接口限流：每分钟 30 请求
    limit_req_zone $binary_remote_addr zone=api_limit:10m rate=30r/m;

    # 登录接口限流：每分钟 5 请求（更严格）
    limit_req_zone $binary_remote_addr zone=login_limit:10m rate=5r/m;

    server {
        listen 80;
        server_name api.example.com;

        # 全局限流
        limit_req zone=ip_global burst=20 nodelay;

        location /api/v1/ {
            # 叠加 API 限流
            limit_req zone=api_limit burst=5 nodelay;
            proxy_pass http://backend;
        }

        location /api/login {
            # 叠加登录限流（最严格）
            limit_req zone=login_limit burst=3 nodelay;
            proxy_pass http://backend;
        }
    }
}
```

**多区域组合规则**：
- 多个 `limit_req` 指令按顺序执行
- 任一区域超限即触发限流
- 可实现更精细的控制策略

---

## 2. ngx_http_limit_conn_module (连接限制)

连接限制模块控制并发连接数量，不同于请求限流（控制速率），它限制的是**同时打开的连接数**。

### 2.1 limit_conn_zone 定义连接限制区域

```nginx
http {
    # 基于 IP 的连接限制
    limit_conn_zone $binary_remote_addr zone=addr:10m;

    # 基于用户的连接限制
    limit_conn_zone $remote_user zone=user:10m;

    # 基于服务器的连接限制
    limit_conn_zone $server_name zone=server:10m;
}
```

**与 limit_req_zone 的区别**：
- 不需要 `rate` 参数（只计数，不限速）
- 统计的是并发连接数，不是请求速率

### 2.2 limit_conn 应用连接限制

```nginx
server {
    location /download/ {
        limit_conn addr 10;  # 每个 IP 最多 10 个并发连接
    }
}
```

**常见应用场景**：

**限制下载并发**：
```nginx
location /download/ {
    limit_conn addr 5;           # 每 IP 最多 5 个并发下载
    limit_rate_after 10m;        # 前 10MB 不限速
    limit_rate 100k;             # 之后限速 100KB/s
}
```

**限制整体连接**：
```nginx
server {
    limit_conn server 1000;      # 整个服务器最多 1000 个并发连接

    location / {
        proxy_pass http://backend;
    }
}
```

### 2.3 limit_conn_status 设置拒绝状态码

```nginx
server {
    location /download/ {
        limit_conn addr 10;
        limit_conn_status 503;   # 超过连接限制返回 503
    }
}
```

### 2.4 limit_conn_log_level 设置日志级别

```nginx
server {
    location /download/ {
        limit_conn addr 10;
        limit_conn_log_level warn;
    }
}
```

---

## 3. ngx_stream_limit_conn_module (Stream 连接限制)

Stream 模块用于 TCP/UDP 四层代理的连接限制。

### 3.1 Stream 上下文中的连接限制

```nginx
stream {
    # 定义限流区域
    limit_conn_zone $binary_remote_addr zone=stream_addr:10m;

    server {
        listen 3306;
        proxy_pass db_backend;
        limit_conn stream_addr 5;  # 每 IP 最多 5 个并发连接
    }
}
```

### 3.2 与 HTTP 连接限制的区别

| 特性 | HTTP (ngx_http_limit_conn_module) | Stream (ngx_stream_limit_conn_module) |
|------|-----------------------------------|---------------------------------------|
| 上下文 | http, server, location | stream, server |
| 适用协议 | HTTP/HTTPS | TCP/UDP |
| 变量支持 | 完整 | 有限（主要使用 `$binary_remote_addr`） |
| 连接统计 | 基于请求 | 基于连接 |

### 3.3 Stream 综合配置示例

```nginx
stream {
    # 连接限制区域
    limit_conn_zone $binary_remote_addr zone=conn_limit:10m;

    # 日志格式
    log_format stream_log '$remote_addr [$time_local] $protocol '
                          '$bytes_sent $bytes_received $session_time';

    access_log /var/log/nginx/stream-access.log stream_log;

    upstream mysql_backend {
        server 192.168.1.10:3306;
        server 192.168.1.11:3306 backup;
    }

    server {
        listen 3306;
        proxy_pass mysql_backend;

        # 连接限制
        limit_conn conn_limit 10;
        limit_conn_status 503;

        # 超时设置
        proxy_connect_timeout 5s;
        proxy_timeout 300s;
    }

    server {
        listen 6379;
        proxy_pass redis_backend;

        # 更严格的连接限制
        limit_conn conn_limit 20;
    }
}
```

---

## 4. 实际应用场景

### 4.1 API 限流保护

```nginx
http {
    # API 限流区域：每秒 100 请求
    limit_req_zone $binary_remote_addr zone=api_limit:50m rate=100r/s;

    # 按 API Key 限流（更精确）
    limit_req_zone $http_x_api_key zone=api_key_limit:100m rate=1000r/s;

    server {
        listen 443 ssl;
        server_name api.example.com;

        location /v1/ {
            # 双重限流：IP + API Key
            limit_req zone=api_limit burst=200 nodelay;
            limit_req zone=api_key_limit burst=500 nodelay;

            limit_req_status 429;
            limit_req_log_level warn;

            proxy_pass http://api_backend;
        }
    }
}
```

### 4.2 登录接口防护

防止暴力破解和撞库攻击：

```nginx
http {
    # 登录接口：每分钟 5 次尝试
    limit_req_zone $binary_remote_addr zone=login_limit:10m rate=5r/m;

    # 基于用户名限流（防止针对特定用户的攻击）
    limit_req_zone $arg_username zone=user_login_limit:10m rate=10r/m;

    server {
        listen 443 ssl;
        server_name auth.example.com;

        location /login {
            # 严格限流
            limit_req zone=login_limit burst=3 nodelay;
            limit_req zone=user_login_limit burst=5 nodelay;

            limit_req_status 429;

            proxy_pass http://auth_backend;
        }

        # 密码重置同样限流
        location /forgot-password {
            limit_req zone=login_limit burst=3 nodelay;
            proxy_pass http://auth_backend;
        }
    }
}
```

### 4.3 DDoS 防护基础

多层防护策略：

```nginx
http {
    # 第一层：IP 级请求限流
    limit_req_zone $binary_remote_addr zone=ip_req:100m rate=50r/s;

    # 第二层：IP 级连接限制
    limit_conn_zone $binary_remote_addr zone=ip_conn:100m;

    # 第三层：URI 级限流（防止针对特定接口的攻击）
    limit_req_zone $binary_remote_addr$uri zone=uri_req:100m rate=10r/s;

    server {
        listen 80;
        server_name example.com;

        # 全局防护
        limit_req zone=ip_req burst=100 nodelay;
        limit_conn ip_conn 50;

        # 静态资源放宽限制
        location ~* \.(jpg|jpeg|png|gif|css|js)$ {
            limit_req off;
            limit_conn off;
            expires 30d;
        }

        # 搜索接口严格限流
        location /search {
            limit_req zone=ip_req burst=20 nodelay;
            limit_req zone=uri_req burst=5 nodelay;
            proxy_pass http://backend;
        }

        # 高危接口极严格限流
        location /admin/ {
            limit_req zone=ip_req burst=5 nodelay;
            limit_req zone=uri_req burst=2 nodelay;
            limit_conn ip_conn 5;

            # 只允许特定 IP
            allow 10.0.0.0/24;
            deny all;
        }
    }
}
```

### 4.4 动态限流（使用变量）

根据请求特征动态限流：

```nginx
http {
    # 根据请求方法设置不同限流键
    map $request_method $rate_limit_key {
        default $binary_remote_addr;
        POST    $binary_remote_addr:POST;
        GET     $binary_remote_addr:GET;
    }

    # 基于 User-Agent 的限流
    map $http_user_agent $ua_limit_key {
        default $binary_remote_addr;
        ~*bot   $binary_remote_addr:BOT;
        ~*curl  $binary_remote_addr:CURL;
    }

    # 动态限流区域
    limit_req_zone $rate_limit_key zone=method_limit:10m rate=50r/s;
    limit_req_zone $ua_limit_key zone=ua_limit:10m rate=30r/s;

    server {
        location / {
            # 应用动态限流
            limit_req zone=method_limit burst=100 nodelay;
            limit_req zone=ua_limit burst=50 nodelay;

            proxy_pass http://backend;
        }
    }
}
```

### 4.5 白名单配置

使用 `geo` 模块实现白名单：

```nginx
http {
    # 定义白名单
    geo $limit_key {
        default $binary_remote_addr;

        # 白名单 IP 返回空值（不限流）
        10.0.0.0/24 "";
        192.168.1.0/24 "";
        127.0.0.1 "";
    }

    # 使用白名单的限流区域
    limit_req_zone $limit_key zone=white_limit:10m rate=10r/s;
    limit_conn_zone $limit_key zone=white_conn:10m;

    server {
        location / {
            # 白名单 IP 不会触发限流
            limit_req zone=white_limit burst=20 nodelay;
            limit_conn white_conn 10;

            proxy_pass http://backend;
        }
    }
}
```

**高级白名单配置**（使用 map）：

```nginx
http {
    geo $white_ip {
        default 0;
        10.0.0.0/24 1;
        192.168.1.0/24 1;
        127.0.0.1 1;
    }

    map $white_ip $limit_key {
        0 $binary_remote_addr;      # 非白名单：使用 IP
        1 "";                        # 白名单：空值（不限流）
    }

    limit_req_zone $limit_key zone=api:10m rate=10r/s;

    server {
        location /api/ {
            limit_req zone=api burst=20 nodelay;
            proxy_pass http://backend;
        }
    }
}
```

---

## 5. 性能影响和最佳实践

### 5.1 性能影响分析

**内存使用**：

| 配置项 | 内存估算 |
|--------|----------|
| `limit_req_zone` 10MB | 约 160,000 个 IP 状态 |
| `limit_conn_zone` 10MB | 约 160,000 个连接状态 |
| 每个状态项 | 约 64-128 字节 |

**CPU 开销**：
- 限流检查：O(1) 时间复杂度
- 对高并发场景影响极小（通常 < 1% CPU）

**延迟影响**：
- `nodelay` 模式：无额外延迟
- 排队模式：可能增加请求延迟

### 5.2 配置最佳实践

**1. 合理设置 zone 大小**
```nginx
# 根据预期并发数计算
# 10,000 并发 IP × 64 字节 ≈ 640KB
# 留 2-3 倍余量：建议 2-5MB
limit_req_zone $binary_remote_addr zone=api:5m rate=100r/s;
```

**2. 使用二进制格式变量**
```nginx
# 推荐：节省 50% 内存
limit_req_zone $binary_remote_addr zone=good:10m rate=10r/s;

# 不推荐：浪费内存
limit_req_zone $remote_addr zone=bad:10m rate=10r/s;
```

**3. 分层限流策略**
```nginx
http {
    # 全局宽松限流
    limit_req_zone $binary_remote_addr zone=global:50m rate=100r/s;

    # API 严格限流
    limit_req_zone $binary_remote_addr zone=api:20m rate=10r/s;

    server {
        # 全局限流
        limit_req zone=global burst=200 nodelay;

        location /api/ {
            # 叠加 API 限流
            limit_req zone=api burst=20 nodelay;
        }
    }
}
```

**4. 静态资源排除**
```nginx
location ~* \.(css|js|png|jpg|jpeg|gif|ico|woff|woff2)$ {
    limit_req off;
    limit_conn off;
    expires 30d;
}
```

**5. 合理设置 burst**
```nginx
# 推荐：burst = rate × 2（允许 2 秒突发）
limit_req zone=api rate=10r/s burst=20 nodelay;

# 高流量 API：burst = rate × 5
limit_req zone=high_api rate=100r/s burst=500 nodelay;
```

### 5.3 监控与调试

**查看限流状态**：
```bash
# 查看限流触发日志
tail -f /var/log/nginx/error.log | grep "limiting requests"

# 统计限流次数
awk '/limiting requests/ {print $1}' /var/log/nginx/error.log | sort | uniq -c
```

**添加限流监控日志**：
```nginx
log_format limit_log '$remote_addr - $time_local '
                     'req_zone=$limit_req_zone conn_zone=$limit_conn_zone '
                     'status=$status';

access_log /var/log/nginx/limit.log limit_log;
```

**NGINX Plus 状态监控**（商业版）：
```nginx
server {
    location /api/ {
        limit_req zone=api burst=20 nodelay;

        # 暴露限流统计
        location /api/status {
            api /status/http/limit_reqs;
        }
    }
}
```

### 5.4 常见问题排查

**问题一：限流不生效**
```nginx
# 检查 zone 是否正确定义
# 确保在 http 上下文定义 zone
http {
    limit_req_zone $binary_remote_addr zone=api:10m rate=10r/s;  # ✓ 正确
}

# 错误示例：在 server 中定义
server {
    limit_req_zone ...  # ✗ 错误，必须在 http 上下文
}
```

**问题二：内存溢出**
```nginx
# 错误：zone 太小
limit_req_zone $binary_remote_addr zone=small:1m rate=100r/s;
# 1MB 只能存储约 16,000 个状态
# 高并发时会被覆盖，限流失效

# 正确：根据流量预估
limit_req_zone $binary_remote_addr zone=proper:50m rate=100r/s;
```

**问题三：误杀正常用户**
```nginx
# 错误配置：burst 过小，无 nodelay
limit_req zone=api burst=2;  # 请求会排队，用户体验差

# 正确配置：合理的 burst + nodelay
limit_req zone=api burst=50 nodelay;  # 允许突发，立即处理
```

### 5.5 综合配置模板

```nginx
user nginx;
worker_processes auto;

http {
    # ========== 限流区域定义 ==========

    # 全局 IP 限流：每秒 50 请求
    limit_req_zone $binary_remote_addr zone=ip_global:50m rate=50r/s;

    # API 限流：每秒 10 请求
    limit_req_zone $binary_remote_addr zone=api_limit:20m rate=10r/s;

    # 登录限流：每分钟 10 请求
    limit_req_zone $binary_remote_addr zone=login_limit:10m rate=10r/m;

    # 连接限制：每 IP 最多 50 连接
    limit_conn_zone $binary_remote_addr zone=conn_limit:50m;

    # ========== 白名单 ==========

    geo $white_ip {
        default 0;
        10.0.0.0/24 1;
        192.168.0.0/16 1;
        127.0.0.1 1;
    }

    map $white_ip $limit_key {
        0 $binary_remote_addr;
        1 "";
    }

    # ========== 服务器配置 ==========

    server {
        listen 80;
        server_name example.com;

        # 全局限流（白名单除外）
        limit_req zone=ip_global burst=100 nodelay;
        limit_conn conn_limit 50;

        limit_req_status 429;
        limit_conn_status 503;
        limit_req_log_level warn;

        # 静态资源不限流
        location ~* \.(css|js|png|jpg|jpeg|gif|ico|woff|woff2|ttf)$ {
            limit_req off;
            limit_conn off;
            expires 30d;
            root /var/www/static;
        }

        # API 接口
        location /api/ {
            limit_req zone=api_limit burst=20 nodelay;
            proxy_pass http://api_backend;
        }

        # 登录接口
        location /login {
            limit_req zone=login_limit burst=5 nodelay;
            proxy_pass http://auth_backend;
        }

        # 默认路由
        location / {
            proxy_pass http://backend;
        }
    }
}

# ========== Stream 连接限制 ==========

stream {
    limit_conn_zone $binary_remote_addr zone=stream_conn:10m;

    server {
        listen 3306;
        proxy_pass mysql_backend;
        limit_conn stream_conn 10;
    }
}
```

---

## 6. 参考文档

- [NGINX Limiting Requests](http://nginx.org/en/docs/http/ngx_http_limit_req_module.html)
- [NGINX Limiting Connections](http://nginx.org/en/docs/http/ngx_http_limit_conn_module.html)
- [NGINX Stream Limit Connections](http://nginx.org/en/docs/stream/ngx_stream_limit_conn_module.html)
