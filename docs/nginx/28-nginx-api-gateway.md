# NGINX API 网关配置指南

## 1. API 网关概述

### 什么是 API 网关

API 网关是微服务架构中的关键组件，作为单一入口点统一管理和暴露后端服务。NGINX 作为高性能反向代理，非常适合构建功能完善的 API 网关。

### API 网关核心功能

| 功能 | 说明 |
|------|------|
| **请求路由** | 根据路径、方法、Header 路由到不同后端服务 |
| **负载均衡** | 在多个服务实例间分发请求 |
| **认证授权** | JWT、OAuth2、API Key 验证 |
| **限流熔断** | 防止过载和服务雪崩 |
| **协议转换** | HTTP/HTTPS、WebSocket、gRPC 转换 |
| **请求/响应转换** | 修改请求头、响应体、路径重写 |
| **缓存加速** | 缓存常用 API 响应 |
| **日志监控** | 统一收集 API 调用日志和指标 |

### API 网关架构图

```
                    ┌─────────────────┐
                    │   API Gateway   │
                    │    (NGINX)      │
                    └────────┬────────┘
                             │
           ┌─────────┬───────┴───────┬─────────┐
           │         │               │         │
           ▼         ▼               ▼         ▼
    ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐
    │ User Svc │ │ Order Svc│ │PaymentSvc│ │ SearchSvc│
    └──────────┘ └──────────┘ └──────────┘ └──────────┘
```

---

## 2. API 路由设计模式

### 2.1 基于路径的路由

最常见的路由方式，根据 URL 路径前缀路由到不同服务。

```nginx
http {
    upstream user_service {
        server user-svc:8080;
    }

    upstream order_service {
        server order-svc:8081;
    }

    upstream payment_service {
        server payment-svc:8082;
    }

    server {
        listen 80;
        server_name api.example.com;

        # 用户服务路由
        location /api/v1/users/ {
            proxy_pass http://user_service/;
            proxy_set_header Host $host;
            proxy_set_header X-Real-IP $remote_addr;
        }

        # 订单服务路由
        location /api/v1/orders/ {
            proxy_pass http://order_service/;
            proxy_set_header Host $host;
        }

        # 支付服务路由
        location /api/v1/payments/ {
            proxy_pass http://payment_service/;
            proxy_set_header Host $host;
        }
    }
}
```

### 2.2 基于请求方法的路由

同一路径根据 HTTP 方法路由到不同服务。

```nginx
server {
    listen 80;
    server_name api.example.com;

    # 查询操作路由到只读服务
    location /api/v1/data/ {
        if ($request_method ~ ^(GET|HEAD)$) {
            proxy_pass http://readonly_backend;
            break;
        }

        # 写入操作路由到主服务
        if ($request_method ~ ^(POST|PUT|PATCH|DELETE)$) {
            proxy_pass http://write_backend;
            break;
        }

        return 405;  # Method Not Allowed
    }
}
```

### 2.3 基于 Header 的路由

根据请求头内容路由，常用于 A/B 测试、金丝雀发布。

```nginx
http {
    map $http_x_api_version $backend_pool {
        default      "stable_backend";
        "v2"         "beta_backend";
        "v2-beta"    "beta_backend";
    }

    upstream stable_backend {
        server api-v1:8080;
    }

    upstream beta_backend {
        server api-v2:8080;
    }

    server {
        listen 80;
        server_name api.example.com;

        location /api/ {
            proxy_pass http://$backend_pool;
            proxy_set_header Host $host;
        }
    }
}
```

### 2.4 基于 Cookie 的路由

```nginx
http {
    map $cookie_app_version $backend_node {
        default     "stable";
        "beta"      "canary";
    }

    upstream stable {
        server api-stable:8080;
    }

    upstream canary {
        server api-canary:8080;
    }

    server {
        listen 80;
        server_name api.example.com;

        location /api/ {
            proxy_pass http://$backend_node;
        }
    }
}
```

### 2.5 流量分流路由（百分比）

```nginx
http {
    # 使用 split_clients 进行百分比分流
    split_clients "${remote_addr}${http_user_agent}" $variant {
        10%     canary;      # 10% 流量到新版本
        *       stable;      # 90% 流量到稳定版
    }

    upstream stable {
        server api-v1:8080;
    }

    upstream canary {
        server api-v2:8080;
    }

    server {
        listen 80;
        server_name api.example.com;

        location /api/ {
            proxy_pass http://$variant;
            proxy_set_header X-Variant $variant;
        }
    }
}
```

### 2.6 组合路由策略

```nginx
http {
    # 定义映射变量
    map $http_x_env $target_backend {
        default     prod;
        "staging"   staging;
        "dev"       dev;
    }

    map $http_x_tenant_id $tenant_shard {
        default         shard1;
        ~^1[0-5]        shard1;   # 租户 10-15
        ~^1[6-9]|^2[0]  shard2;   # 租户 16-20
    }

    upstream prod {
        server api-prod-1:8080;
        server api-prod-2:8080;
    }

    upstream staging {
        server api-staging:8080;
    }

    upstream dev {
        server api-dev:8080;
    }

    upstream shard1 {
        server db-shard1:8080;
    }

    upstream shard2 {
        server db-shard2:8080;
    }

    server {
        listen 80;
        server_name api.example.com;

        # 环境路由
        location /api/ {
            proxy_pass http://$target_backend;
            proxy_set_header X-Tenant-Shard $tenant_shard;

            # 记录路由决策
            access_log /var/log/nginx/api-access.log detailed;
        }

        # 租户数据路由
        location /api/tenant-data/ {
            proxy_pass http://$tenant_shard;
        }
    }
}
```

---

## 3. 请求/响应转换

### 3.1 请求头转换

#### 添加请求头

```nginx
server {
    listen 80;
    server_name api.example.com;

    location /api/ {
        # 传递客户端信息
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header X-Forwarded-Host $host;
        proxy_set_header X-Forwarded-Port $server_port;

        # 添加网关标识
        proxy_set_header X-API-Gateway "nginx";
        proxy_set_header X-Request-ID $request_id;

        # 添加时间戳
        proxy_set_header X-Request-Time $msec;

        proxy_pass http://backend;
    }
}
```

#### 删除请求头

```nginx
server {
    location /api/ {
        # 删除敏感请求头，防止信息泄露
        proxy_set_header Authorization "";
        proxy_set_header Cookie "";
        proxy_set_header X-Internal-Token "";

        proxy_pass http://backend;
    }
}
```

#### 修改请求头

```nginx
server {
    location /api/ {
        # 重写 Host 头
        proxy_set_header Host backend.internal.com;

        # 基于条件设置
        if ($http_x_client_type = "mobile") {
            proxy_set_header X-Device-Type "mobile";
        }

        proxy_pass http://backend;
    }
}
```

### 3.2 响应头转换

#### 添加安全响应头

```nginx
server {
    listen 80;
    server_name api.example.com;

    # 添加 API 响应头
    add_header X-API-Version "v1" always;
    add_header X-RateLimit-Limit "1000" always;

    # 安全头部
    add_header X-Content-Type-Options "nosniff" always;
    add_header X-Frame-Options "DENY" always;
    add_header X-XSS-Protection "1; mode=block" always;

    # CORS 头
    add_header Access-Control-Allow-Origin "*" always;
    add_header Access-Control-Allow-Methods "GET, POST, PUT, DELETE, OPTIONS" always;
    add_header Access-Control-Allow-Headers "Authorization, Content-Type, X-Request-ID" always;

    location /api/ {
        proxy_pass http://backend;

        # 暴露额外响应头给前端
        expose_headers X-Request-ID X-RateLimit-Remaining;
    }
}
```

#### 隐藏响应头

```nginx
server {
    location /api/ {
        proxy_pass http://backend;

        # 隐藏后端服务器信息
        proxy_hide_header X-Powered-By;
        proxy_hide_header X-Runtime;
        proxy_hide_header X-Version;
        proxy_hide_header Server;
    }
}
```

### 3.3 请求体转换

#### 请求体大小限制

```nginx
http {
    # 全局请求体限制
    client_max_body_size 10m;
    client_body_buffer_size 128k;

    server {
        listen 80;
        server_name api.example.com;

        # 上传接口允许更大请求体
        location /api/v1/upload/ {
            client_max_body_size 100m;
            proxy_pass http://upload_backend;
        }

        # Webhook 接口限制较小
        location /api/v1/webhooks/ {
            client_max_body_size 1m;
            proxy_pass http://webhook_backend;
        }

        # 普通 API
        location /api/ {
            client_max_body_size 5m;
            proxy_pass http://api_backend;
        }
    }
}
```

### 3.4 响应体转换（sub_filter）

#### 修改响应内容

```nginx
server {
    listen 80;
    server_name api.example.com;

    location /api/ {
        proxy_pass http://backend;

        # 替换响应中的内部 URL 为外部 URL
        sub_filter_once off;
        sub_filter_types application/json;

        # 替换后端地址为网关地址
        sub_filter 'http://backend-internal:8080' 'https://api.example.com';

        # 替换版本标识
        sub_filter '"version": "internal"' '"version": "public"';
    }
}
```

#### JSON 字段脱敏

```nginx
server {
    location /api/users/ {
        proxy_pass http://user_backend;

        # 脱敏手机号（示例：隐藏中间4位）
        sub_filter_once off;
        sub_filter_types application/json;
        sub_filter '([0-9]{3})[0-9]{4}([0-9]{4})' '$1****$2';
    }
}
```

### 3.5 URL 重写与转换

#### 路径重写

```nginx
server {
    listen 80;
    server_name api.example.com;

    # 旧版本路径兼容
    location /api/v0/ {
        rewrite ^/api/v0/(.*)$ /api/v1/$1 permanent;
    }

    # 内部路径映射
    location /api/public/ {
        rewrite ^/api/public/(.*)$ /internal/api/$1 break;
        proxy_pass http://backend;
    }

    # 带参数重写
    location /api/search/ {
        rewrite ^/api/search/(.*)$ /search?q=$1 break;
        proxy_pass http://search_backend;
    }
}
```

#### 查询参数处理

```nginx
server {
    location /api/ {
        # 添加默认参数
        if ($arg_version = "") {
            set $args $args&version=v1;
        }

        # 移除敏感参数
        if ($arg_api_key) {
            set $args '';
            # 或者保留其他参数
            set $args $arg_foo=$arg_foo;
        }

        proxy_pass http://backend;
    }
}
```

---

## 4. JWT 验证

### 4.1 通过 auth_request 进行 JWT 验证

使用外部认证服务验证 JWT Token。

```nginx
http {
    upstream api_backend {
        server api:8080;
    }

    upstream auth_service {
        server auth:8080;
    }

    server {
        listen 80;
        server_name api.example.com;

        location /api/ {
            # JWT 验证
            auth_request /auth_verify;

            # 从认证响应中提取用户信息
            auth_request_set $auth_user $upstream_http_x_user_id;
            auth_request_set $auth_role $upstream_http_x_user_role;
            auth_request_set $auth_tenant $upstream_http_x_tenant_id;

            # 传递给后端
            proxy_set_header X-User-ID $auth_user;
            proxy_set_header X-User-Role $auth_role;
            proxy_set_header X-Tenant-ID $auth_tenant;

            proxy_pass http://api_backend;
        }

        # JWT 验证子请求
        location = /auth_verify {
            internal;
            proxy_pass http://auth_service/verify;
            proxy_pass_request_body off;
            proxy_set_header Content-Length "";

            # 传递 Authorization 头
            proxy_set_header Authorization $http_authorization;
            proxy_set_header X-Original-URI $request_uri;
            proxy_set_header X-Client-IP $remote_addr;

            proxy_connect_timeout 3s;
            proxy_read_timeout 3s;
        }

        # 认证失败处理
        error_page 401 = @unauthorized;
        location @unauthorized {
            default_type application/json;
            return 401 '{"error":"Unauthorized","code":"INVALID_TOKEN"}';
        }

        error_page 403 = @forbidden;
        location @forbidden {
            default_type application/json;
            return 403 '{"error":"Forbidden","code":"INSUFFICIENT_PERMISSIONS"}';
        }
    }
}
```

### 4.2 使用 Lua 进行 JWT 验证（OpenResty）

```nginx
http {
    lua_package_path "/usr/local/lib/lua/?.lua;;";

    server {
        listen 80;
        server_name api.example.com;

        location /api/ {
            access_by_lua_block {
                local jwt = require "resty.jwt"
                local validators = require "resty.jwt-validators"

                -- 获取 token
                local auth_header = ngx.var.http_authorization
                if not auth_header then
                    ngx.status = 401
                    ngx.say('{"error":"Missing Authorization header"}')
                    ngx.exit(ngx.HTTP_UNAUTHORIZED)
                end

                local token = string.gsub(auth_header, "Bearer ", "")

                -- 验证 JWT
                local jwt_obj = jwt:verify(
                    ngx.var.jwt_secret,  -- JWT 密钥
                    token,
                    {
                        iss = "https://auth.example.com",
                        validators = {
                            exp = validators.opt_is_not_expired(),
                            iat = validators.opt_is_not_before_now(),
                        }
                    }
                )

                if not jwt_obj.verified then
                    ngx.status = 401
                    ngx.say('{"error":"Invalid token","details":"' .. jwt_obj.reason .. '"}')
                    ngx.exit(ngx.HTTP_UNAUTHORIZED)
                end

                -- 设置变量供后续使用
                ngx.var.jwt_sub = jwt_obj.payload.sub
                ngx.var.jwt_role = jwt_obj.payload.role or "user"
            }

            proxy_set_header X-User-ID $jwt_sub;
            proxy_set_header X-User-Role $jwt_role;
            proxy_pass http://api_backend;
        }
    }
}
```

### 4.3 使用 NJS（NGINX JavaScript）进行 JWT 验证

```nginx
load_module modules/ngx_http_js_module.so;

http {
    js_import /etc/nginx/jwt_verify.js;

    server {
        listen 80;
        server_name api.example.com;

        location /api/ {
            # 调用 NJS 验证
            js_set $jwt_payload jwt_verify.verify;

            # 验证失败时拒绝
            if ($jwt_payload = "") {
                return 401 '{"error":"Unauthorized"}';
            }

            proxy_set_header X-JWT-Payload $jwt_payload;
            proxy_pass http://api_backend;
        }
    }
}
```

**jwt_verify.js**:

```javascript
function verify(r) {
    var auth = r.headersIn['Authorization'];
    if (!auth || !auth.startsWith('Bearer ')) {
        return '';
    }

    var token = auth.substring(7);

    try {
        // 简单 JWT 解析（base64 解码 payload）
        var parts = token.split('.');
        if (parts.length !== 3) {
            return '';
        }

        // 解码 payload
        var payload = parts[1];
        // base64url 解码
        var decoded = Buffer.from(payload, 'base64url').toString();
        var claims = JSON.parse(decoded);

        // 验证过期时间
        if (claims.exp && claims.exp < Math.floor(Date.now() / 1000)) {
            return '';
        }

        return JSON.stringify(claims);
    } catch (e) {
        return '';
    }
}

export default {verify};
```

### 4.4 JWT 验证配置参考表

| 验证方式 | 优点 | 缺点 | 适用场景 |
|----------|------|------|----------|
| **auth_request** | 灵活、业务逻辑外置、支持复杂验证 | 额外网络请求、延迟增加 | 需要与认证中心实时交互 |
| **Lua (OpenResty)** | 高性能、功能丰富、社区成熟 | 需要 OpenResty 或 lua-nginx-module | 复杂验证逻辑、本地处理 |
| **NJS** | NGINX 官方支持、无需额外模块 | 功能相对简单、性能略低 | 简单验证、官方生态优先 |

---

## 5. 限流与配额管理

### 5.1 基础限流配置

```nginx
http {
    # 按 IP 限流区域
    limit_req_zone $binary_remote_addr zone=ip_limit:10m rate=10r/s;

    # 按用户限流区域
    limit_req_zone $http_x_user_id zone=user_limit:10m rate=100r/m;

    # 全局 API 限流
    limit_req_zone $server_name zone=api_global:10m rate=1000r/s;

    server {
        listen 80;
        server_name api.example.com;

        # 全局限流
        limit_req zone=api_global burst=200 nodelay;

        location /api/ {
            # IP 级限流
            limit_req zone=ip_limit burst=20 nodelay;

            proxy_pass http://api_backend;
        }

        location /api/v1/users/ {
            # 用户级限流
            limit_req zone=user_limit burst=10 nodelay;

            proxy_pass http://user_backend;
        }
    }
}
```

### 5.2 分层限流策略

```nginx
http {
    # 不同层级的限流区域
    limit_req_zone $binary_remote_addr zone=ip:10m rate=30r/s;
    limit_req_zone $http_x_api_key zone=api_key:10m rate=100r/s;
    limit_req_zone $http_x_user_id zone=user:10m rate=60r/m;
    limit_req_zone $server_name zone=global:10m rate=5000r/s;

    server {
        listen 80;
        server_name api.example.com;

        # 全局保护
        limit_req zone=global burst=1000 nodelay;

        # 健康检查端点不限流
        location /health {
            limit_req off;
            access_log off;
            return 200 '{"status":"ok"}';
        }

        # 公开 API - 仅 IP 限流
        location /api/public/ {
            limit_req zone=ip burst=50 nodelay;
            proxy_pass http://public_backend;
        }

        # 认证 API - IP + API Key 双重限流
        location /api/v1/ {
            limit_req zone=ip burst=30 nodelay;
            limit_req zone=api_key burst=100 nodelay;
            proxy_pass http://api_backend;
        }

        # 用户级 API - 三层限流
        location /api/v1/user/ {
            limit_req zone=ip burst=20 nodelay;
            limit_req zone=api_key burst=50 nodelay;
            limit_req zone=user burst=10 nodelay;
            proxy_pass http://user_backend;
        }
    }
}
```

### 5.3 配额管理（基于连接数）

```nginx
http {
    # 按 API Key 限制并发连接
    limit_conn_zone $http_x_api_key zone=conn_by_key:10m;

    # 按用户限制并发连接
    limit_conn_zone $http_x_user_id zone=conn_by_user:10m;

    server {
        listen 80;
        server_name api.example.com;

        location /api/v1/stream/ {
            # 每个 API Key 最多 10 个并发连接
            limit_conn conn_by_key 10;

            # 每个用户最多 5 个并发连接
            limit_conn conn_by_user 5;

            proxy_pass http://streaming_backend;
            proxy_buffering off;
        }
    }
}
```

### 5.4 限流响应自定义

```nginx
http {
    limit_req_zone $binary_remote_addr zone=limit:10m rate=10r/s;

    server {
        listen 80;
        server_name api.example.com;

        location /api/ {
            limit_req zone=limit burst=20 nodelay;

            # 自定义限流响应
            limit_req_status 429;  # Too Many Requests

            proxy_pass http://api_backend;
        }

        # 自定义 429 响应
        error_page 429 @rate_limited;
        location @rate_limited {
            default_type application/json;
            add_header Retry-After 60 always;
            return 429 '{"error":"Rate limit exceeded","retry_after":60,"limit":10}';
        }
    }
}
```

### 5.5 基于路径的差异化限流

```nginx
http {
    # 不同限流区域
    limit_req_zone $binary_remote_addr zone=light:10m rate=100r/m;
    limit_req_zone $binary_remote_addr zone=medium:10m rate=10r/s;
    limit_req_zone $binary_remote_addr zone=heavy:10m rate=1r/s;

    map $uri $rate_limit_zone {
        default          "";
        ~*^/api/health    "none";
        ~*^/api/search    "heavy";
        ~*^/api/reports   "heavy";
        ~*^/api/export    "heavy";
        ~*^/api/webhooks  "light";
    }

    server {
        listen 80;
        server_name api.example.com;

        location /api/ {
            # 根据路径应用不同限流
            if ($rate_limit_zone = "none") {
                limit_req off;
            }

            if ($rate_limit_zone = "light") {
                limit_req zone=light burst=5 nodelay;
            }

            if ($rate_limit_zone = "heavy") {
                limit_req zone=heavy burst=3 nodelay;
            }

            proxy_pass http://api_backend;
        }
    }
}
```

### 5.6 限流指标暴露

```nginx
http {
    limit_req_zone $binary_remote_addr zone=api:10m rate=100r/s;

    server {
        listen 80;
        server_name api.example.com;

        location /api/ {
            limit_req zone=api burst=200 nodelay;

            # 添加限流相关响应头
            add_header X-RateLimit-Limit 100 always;
            add_header X-RateLimit-Window 1s always;

            proxy_pass http://api_backend;
        }

        # 限流状态端点
        location /api/status/ratelimit {
            limit_req off;
            stub_status on;
            access_log off;
        }
    }
}
```

---

## 6. API 版本控制策略

### 6.1 URL 路径版本控制

```nginx
http {
    upstream api_v1 {
        server api-v1:8080;
    }

    upstream api_v2 {
        server api-v2:8080;
    }

    server {
        listen 80;
        server_name api.example.com;

        # v1 路由
        location /api/v1/ {
            proxy_pass http://api_v1/;
            proxy_set_header X-API-Version "v1";
        }

        # v2 路由
        location /api/v2/ {
            proxy_pass http://api_v2/;
            proxy_set_header X-API-Version "v2";
        }

        # 默认版本（向后兼容）
        location /api/ {
            rewrite ^/api/(.*)$ /api/v2/$1 break;
            proxy_pass http://api_v2;
        }
    }
}
```

### 6.2 Header 版本控制

```nginx
http {
    upstream api_v1 {
        server api-v1:8080;
    }

    upstream api_v2 {
        server api-v2:8080;
    }

    # 根据 Accept-Version 头路由
    map $http_accept_version $api_version {
        default     "v2";
        "1"         "v1";
        "2"         "v2";
        "v1"        "v1";
        "v2"        "v2";
    }

    server {
        listen 80;
        server_name api.example.com;

        location /api/ {
            # 版本不存在时返回 400
            if ($api_version = "") {
                return 400 '{"error":"Unsupported API version"}';
            }

            proxy_pass http://api_$api_version;
            proxy_set_header X-API-Version $api_version;
        }
    }
}
```

### 6.3 内容协商版本控制

```nginx
http {
    upstream api_v1 {
        server api-v1:8080;
    }

    upstream api_v2 {
        server api-v2:8080;
    }

    server {
        listen 80;
        server_name api.example.com;

        location /api/users {
            # 检查 Accept 头中的版本媒体类型
            if ($http_accept ~ "application/vnd\.api\.v2\+json") {
                proxy_pass http://api_v2;
                break;
            }

            if ($http_accept ~ "application/vnd\.api\.v1\+json") {
                proxy_pass http://api_v1;
                break;
            }

            # 默认版本
            proxy_pass http://api_v2;
        }
    }
}
```

### 6.4 版本弃用与 Sunset

```nginx
http {
    server {
        listen 80;
        server_name api.example.com;

        location /api/v1/ {
            # 添加弃用警告头
            add_header Deprecation "true" always;
            add_header Sunset "Sun, 31 Dec 2024 23:59:59 GMT" always;
            add_header Link '</api/v2/>; rel="successor-version"' always;

            proxy_pass http://api_v1;
        }
    }
}
```

---

## 7. OpenAPI/Swagger 集成

### 7.1 Swagger UI 托管

```nginx
http {
    server {
        listen 80;
        server_name api.example.com;

        # Swagger UI 静态文件
        location /docs/ {
            alias /var/www/swagger-ui/;
            try_files $uri $uri/ /docs/index.html;
        }

        # OpenAPI 规范文件
        location /api-docs/ {
            alias /etc/nginx/api-docs/;
            default_type application/json;

            # CORS
            add_header Access-Control-Allow-Origin "*" always;
            add_header Access-Control-Allow-Methods "GET, OPTIONS" always;
        }

        # 特定服务文档
        location /api-docs/users.yaml {
            alias /etc/nginx/api-docs/users.yaml;
            default_type text/yaml;
        }

        location /api-docs/orders.yaml {
            alias /etc/nginx/api-docs/orders.yaml;
            default_type text/yaml;
        }
    }
}
```

### 7.2 多服务 API 文档聚合

```nginx
http {
    server {
        listen 80;
        server_name api.example.com;

        # 聚合文档入口
        location /api-docs {
            default_type application/json;
            return 200 '{
                "openapi": "3.0.0",
                "info": {
                    "title": "API Gateway",
                    "version": "1.0.0"
                },
                "servers": [
                    {"url": "https://api.example.com"}
                ],
                "paths": {
                    "/api/v1/users": {"$ref": "/api-docs/users.yaml#/paths/~1users"},
                    "/api/v1/orders": {"$ref": "/api-docs/orders.yaml#/paths/~1orders"}
                }
            }';
        }

        # 代理到各服务文档
        location /api-docs/users/ {
            proxy_pass http://user_service/docs/;
        }

        location /api-docs/orders/ {
            proxy_pass http://order_service/docs/;
        }
    }
}
```

### 7.3 API 文档访问控制

```nginx
http {
    server {
        listen 80;
        server_name api.example.com;

        # 公开文档
        location /docs/public/ {
            alias /var/www/docs/public/;
        }

        # 内部文档（需认证）
        location /docs/internal/ {
            auth_basic "Internal API Docs";
            auth_basic_user_file /etc/nginx/.htpasswd-docs;

            alias /var/www/docs/internal/;
        }

        # API 定义文件保护
        location ~ ^/api-docs/.*\.(yaml|json)$ {
            # 只允许特定 referer
            valid_referers server_names *.example.com;

            if ($invalid_referer) {
                return 403;
            }

            alias /etc/nginx/api-docs/;
        }
    }
}
```

---

## 8. 完整 API 网关配置示例

### 8.1 生产级 API 网关配置

```nginx
# /etc/nginx/nginx.conf

user nginx;
worker_processes auto;
worker_rlimit_nofile 65535;

error_log /var/log/nginx/error.log warn;
pid /var/run/nginx.pid;

events {
    worker_connections 4096;
    use epoll;
    multi_accept on;
}

http {
    include /etc/nginx/mime.types;
    default_type application/json;

    # 日志格式
    log_format api_log '$remote_addr - $remote_user [$time_local] '
                       '"$request" $status $body_bytes_sent '
                       '"$http_referer" "$http_user_agent" '
                       'rt=$request_time uct="$upstream_connect_time" '
                       'uht="$upstream_header_time" urt="$upstream_response_time" '
                       'req_id="$request_id" api_key="$http_x_api_key" '
                       'user_id="$jwt_sub" tenant="$jwt_tenant"';

    access_log /var/log/nginx/access.log api_log;

    # 性能优化
    sendfile on;
    tcp_nopush on;
    tcp_nodelay on;
    keepalive_timeout 65;
    types_hash_max_size 2048;

    # 客户端限制
    client_max_body_size 10m;
    client_body_buffer_size 128k;
    client_header_buffer_size 4k;
    large_client_header_buffers 4 8k;

    # 限流区域
    limit_req_zone $binary_remote_addr zone=ip:10m rate=30r/s;
    limit_req_zone $http_x_api_key zone=api_key:10m rate=100r/s;
    limit_req_zone $server_name zone=global:10m rate=10000r/s;

    # 连接限制
    limit_conn_zone $binary_remote_addr zone=addr:10m;
    limit_conn_zone $http_x_api_key zone=api_conn:10m;

    # 上游服务定义
    upstream user_service {
        zone user_service 64k;
        least_conn;
        server user-svc-1:8080 weight=5;
        server user-svc-2:8080 weight=5;
        server user-svc-3:8080 backup;
        keepalive 32;
    }

    upstream order_service {
        zone order_service 64k;
        least_conn;
        server order-svc-1:8080;
        server order-svc-2:8080;
        keepalive 32;
    }

    upstream payment_service {
        zone payment_service 64k;
        server payment-svc-1:8080 max_fails=3 fail_timeout=30s;
        server payment-svc-2:8080 max_fails=3 fail_timeout=30s;
        keepalive 16;
    }

    upstream auth_service {
        zone auth_service 64k;
        server auth-svc:8080;
        keepalive 16;
    }

    # 变量映射
    map $http_x_api_version $api_version {
        default "v1";
        "v1"    "v1";
        "v2"    "v2";
    }

    map $uri $rate_limit_tier {
        default          "medium";
        ~*^/api/health    "none";
        ~*^/api/metrics   "none";
        ~*^/api/export    "strict";
        ~*^/api/search    "strict";
        ~*^/api/webhooks  "relaxed";
    }

    # API Gateway 服务器
    server {
        listen 80;
        listen [::]:80;
        server_name api.example.com;

        # 返回 444 关闭非指定域名访问
        location / {
            return 444;
        }

        # 健康检查（无认证、无限流）
        location = /health {
            access_log off;
            limit_req off;
            return 200 '{"status":"healthy","gateway":"nginx"}';
        }

        # Prometheus 指标端点
        location = /metrics {
            access_log off;
            limit_req off;
            stub_status on;
        }

        # API 文档
        location /docs/ {
            alias /var/www/api-docs/;
            try_files $uri $uri/ =404;

            # 缓存文档
            expires 1h;
            add_header Cache-Control "public, immutable";
        }

        # 主 API 入口
        location /api/ {
            # 请求 ID 生成
            add_header X-Request-ID $request_id always;

            # 全局限流
            limit_req zone=global burst=2000 nodelay;

            # 分层限流
            limit_req zone=ip burst=50 nodelay;
            limit_req zone=api_key burst=100 nodelay;

            # 连接限制
            limit_conn addr 50;
            limit_conn api_conn 100;

            # JWT 验证（可选，根据路径）
            location ~ ^/api/(v1|v2)/(users|orders|payments)/ {
                auth_request /auth/verify;
                auth_request_set $jwt_sub $upstream_http_x_user_id;
                auth_request_set $jwt_role $upstream_http_x_user_role;
                auth_request_set $jwt_tenant $upstream_http_x_tenant_id;

                # 认证失败处理
                error_page 401 = @auth_error;
                error_page 403 = @forbidden_error;

                # 子路径路由
                location ~ ^/api/(v1|v2)/users/ {
                    rewrite ^/api/(v1|v2)/(.*)$ /$2 break;
                    proxy_pass http://user_service;
                }

                location ~ ^/api/(v1|v2)/orders/ {
                    rewrite ^/api/(v1|v2)/(.*)$ /$2 break;
                    proxy_pass http://order_service;
                }

                location ~ ^/api/(v1|v2)/payments/ {
                    rewrite ^/api/(v1|v2)/(.*)$ /$2 break;
                    proxy_pass http://payment_service;
                }
            }

            # 公开 API（无需认证）
            location ~ ^/api/(v1|v2)/public/ {
                rewrite ^/api/(v1|v2)/(.*)$ /$2 break;
                proxy_pass http://public_service;
            }
        }

        # 认证子请求
        location = /auth/verify {
            internal;
            proxy_pass http://auth_service/verify;
            proxy_pass_request_body off;
            proxy_set_header Content-Length "";
            proxy_set_header Authorization $http_authorization;
            proxy_set_header X-Original-URI $request_uri;
            proxy_set_header X-Original-Method $request_method;
            proxy_connect_timeout 3s;
            proxy_read_timeout 3s;
        }

        # WebSocket 支持
        location /ws/ {
            proxy_pass http://websocket_backend;
            proxy_http_version 1.1;
            proxy_set_header Upgrade $http_upgrade;
            proxy_set_header Connection "upgrade";
            proxy_set_header Host $host;
            proxy_set_header X-Real-IP $remote_addr;
            proxy_read_timeout 86400s;
            proxy_send_timeout 86400s;
        }

        # 错误处理
        error_page 500 502 503 504 @api_error;
        location @api_error {
            internal;
            default_type application/json;
            add_header X-Error-Source "gateway" always;
            return 500 '{"error":"Internal Server Error","code":"GATEWAY_ERROR"}';
        }

        location @auth_error {
            internal;
            default_type application/json;
            return 401 '{"error":"Unauthorized","code":"AUTH_REQUIRED"}';
        }

        location @forbidden_error {
            internal;
            default_type application/json;
            return 403 '{"error":"Forbidden","code":"ACCESS_DENIED"}';
        }

        location @rate_limited {
            internal;
            default_type application/json;
            add_header Retry-After 60 always;
            return 429 '{"error":"Rate limit exceeded","code":"RATE_LIMITED","retry_after":60}';
        }
    }

    # HTTPS 服务器
    server {
        listen 443 ssl http2;
        listen [::]:443 ssl http2;
        server_name api.example.com;

        # SSL 配置
        ssl_certificate /etc/nginx/ssl/api.example.com.crt;
        ssl_certificate_key /etc/nginx/ssl/api.example.com.key;
        ssl_protocols TLSv1.2 TLSv1.3;
        ssl_ciphers ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256:ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-GCM-SHA384;
        ssl_prefer_server_ciphers off;
        ssl_session_cache shared:SSL:10m;
        ssl_session_timeout 1d;
        ssl_session_tickets off;

        # HSTS
        add_header Strict-Transport-Security "max-age=63072000; includeSubDomains; preload" always;

        # 安全头部
        add_header X-Frame-Options "DENY" always;
        add_header X-Content-Type-Options "nosniff" always;
        add_header X-XSS-Protection "1; mode=block" always;
        add_header Referrer-Policy "strict-origin-when-cross-origin" always;

        # 重用 HTTP 配置
        include /etc/nginx/api-locations.conf;
    }
}
```

### 8.2 配置结构建议

```
/etc/nginx/
├── nginx.conf              # 主配置
├── conf.d/
│   ├── 00-upstreams.conf   # 上游服务定义
│   ├── 10-limits.conf      # 限流配置
│   ├── 20-maps.conf        # 变量映射
│   └── 30-ssl.conf         # SSL 通用配置
├── sites-enabled/
│   └── api-gateway.conf    # API 网关服务器配置
├── api-docs/               # API 文档
│   ├── openapi.yaml
│   ├── users.yaml
│   └── orders.yaml
└── ssl/
    ├── api.example.com.crt
    └── api.example.com.key
```

### 8.3 常用指令速查表

| 类别 | 指令 | 说明 |
|------|------|------|
| **路由** | `proxy_pass` | 代理到后端 |
| | `rewrite` | URL 重写 |
| | `map` | 变量映射 |
| **请求头** | `proxy_set_header` | 设置代理请求头 |
| | `proxy_hide_header` | 隐藏响应头 |
| | `add_header` | 添加响应头 |
| **认证** | `auth_request` | 外部认证 |
| | `auth_request_set` | 提取认证变量 |
| **限流** | `limit_req_zone` | 定义限流区域 |
| | `limit_req` | 应用限流 |
| | `limit_conn_zone` | 定义连接限制区域 |
| | `limit_conn` | 应用连接限制 |
| **转换** | `sub_filter` | 响应内容替换 |
| | `client_max_body_size` | 请求体大小限制 |

---

## 9. 最佳实践

### 9.1 安全最佳实践

1. **始终使用 HTTPS**：生产环境强制 TLSv1.2+
2. **隐藏后端信息**：移除 Server、X-Powered-By 等响应头
3. **启用 HSTS**：防止降级攻击
4. **实施认证**：敏感接口必须认证
5. **输入验证**：限制请求体大小和方法

### 9.2 性能最佳实践

1. **启用 keepalive**：减少连接建立开销
2. **合理配置缓冲区**：平衡内存使用和响应速度
3. **使用缓存**：对读多写少的 API 启用缓存
4. **连接池**：配置上游 keepalive 连接
5. **启用 gzip**：压缩 JSON 响应

### 9.3 可观测性最佳实践

1. **结构化日志**：包含请求 ID、用户 ID、耗时等
2. **健康检查端点**：便于负载均衡器探测
3. **指标暴露**：使用 stub_status 或 nginx-module-vts
4. **分布式追踪**：传递 Trace ID 到后端
5. **告警配置**：对错误率和延迟设置告警

### 9.4 部署检查清单

- [ ] 配置文件语法验证 (`nginx -t`)
- [ ] 限流阈值合理性验证
- [ ] SSL 证书有效性检查
- [ ] 后端服务连通性测试
- [ ] 认证流程端到端测试
- [ ] 错误响应格式验证
- [ ] 性能基准测试
- [ ] 安全扫描（端口、Header 等）
