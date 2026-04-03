# NGINX 安全与访问控制指南

## 1. 访问控制

### IP 访问控制

```nginx
# 允许/拒绝特定 IP
location /admin/ {
    allow 192.168.1.0/24;
    allow 10.0.0.1;
    deny all;
}

# 仅允许本地访问内部接口
location /internal/ {
    allow 127.0.0.1;
    deny all;
}
```

### 地理位置访问控制（GeoIP）

```nginx
http {
    geoip_country /usr/share/GeoIP/GeoIP.dat;

    map $geoip_country_code $allowed_country {
        default no;
        CN yes;
        US yes;
    }

    server {
        if ($allowed_country = no) {
            return 403;
        }
    }
}
```

---

## 2. 外部认证 (ngx_http_auth_request_module)

NGINX 的 `ngx_http_auth_request_module` 模块（从 NGINX 1.5.4+ 开始支持，需编译时启用）提供外部认证功能，允许通过发送子请求到认证服务来决定是否允许访问。

### 2.1 指令说明

| 指令 | 语法 | 默认值 | 上下文 |
|------|------|--------|--------|
| `auth_request` | `auth_request uri \| off;` | `off` | http, server, location |
| `auth_request_set` | `auth_request_set $variable value;` | — | http, server, location |

### 2.2 工作原理

1. **NGINX 发送子请求**：当收到客户端请求时，NGINX 向指定的认证 URI 发送子请求
2. **认证服务响应**：认证服务返回 HTTP 状态码（200 表示通过，401/403 表示拒绝）
3. **结果判定**：
   - 返回 200：NGINX 继续处理原请求
   - 返回 401/403：NGINX 拒绝访问并返回相应状态码
   - 其他状态码：视为错误，返回 500

**流程图**：
```
Client → NGINX → auth_request (子请求)
                    ↓
              认证服务
                    ↓
              200 OK? → 是 → 处理原请求 → 后端服务
                    ↓
                   否
                    ↓
              返回 401/403 → Client
```

### 2.3 基础配置示例

```nginx
server {
    listen 80;
    server_name api.example.com;

    location /protected/ {
        auth_request /auth;
        proxy_pass http://backend;
    }

    # 认证子请求 location
    location = /auth {
        internal;                    # 仅限内部子请求访问
        proxy_pass http://auth-server/verify;
        proxy_pass_request_body off; # 不传递请求体到认证服务
        proxy_set_header Content-Length "";
        proxy_set_header X-Original-URI $request_uri;
        proxy_set_header X-Client-IP $remote_addr;
    }
}
```

### 2.4 带变量提取的认证

从认证服务响应中提取自定义头部到变量：

```nginx
server {
    location /api/ {
        auth_request /auth;
        auth_request_set $auth_user $upstream_http_x_user;
        auth_request_set $auth_role $upstream_http_x_role;

        proxy_pass http://backend;
        proxy_set_header X-User $auth_user;
        proxy_set_header X-Role $auth_role;
    }

    location = /auth {
        internal;
        proxy_pass http://auth-service/verify;
        proxy_pass_request_body off;
        proxy_set_header Authorization $http_authorization;
    }
}
```

**可用变量**：
- `$upstream_http_*`：从认证服务响应中提取任意 HTTP 头部
- `$upstream_status`：认证服务返回的状态码
- `$upstream_response_time`：认证响应时间

### 2.5 JWT/OAuth2 集成示例

```nginx
server {
    listen 443 ssl;
    server_name api.example.com;

    location /api/ {
        auth_request /auth_jwt;
        auth_request_set $jwt_claims $upstream_http_x_jwt_claims;
        auth_request_set $jwt_sub $upstream_http_x_jwt_sub;

        proxy_pass http://api_backend;
        proxy_set_header X-JWT-Claims $jwt_claims;
        proxy_set_header X-User-Id $jwt_sub;

        # 自定义 401 响应
        error_page 401 = @unauthorized;
    }

    location = /auth_jwt {
        internal;
        proxy_pass http://jwt-validator/verify;
        proxy_set_header Authorization $http_authorization;
        proxy_set_header X-Original-URI $request_uri;
        proxy_pass_request_body off;
    }

    location @unauthorized {
        default_type application/json;
        return 401 '{"error":"Unauthorized","code":"INVALID_TOKEN"}';
    }
}
```

### 2.6 多级认证组合

结合 `auth_request` 与 `auth_basic`：

```nginx
server {
    location /admin/ {
        # 先进行基础认证
        auth_basic "Admin Access";
        auth_basic_user_file /etc/nginx/.htpasswd;

        # 再进行外部权限校验
        auth_request /auth_admin;

        proxy_pass http://admin_backend;
    }

    location = /auth_admin {
        internal;
        proxy_pass http://permission-service/check-admin;
        proxy_set_header X-User $remote_user;
    }
}
```

### 2.7 错误处理

```nginx
server {
    location /api/ {
        auth_request /auth;

        # 认证失败重定向到登录页
        error_page 401 = @login;

        proxy_pass http://backend;
    }

    location @login {
        return 302 https://auth.example.com/login?redirect=$request_uri;
    }

    # 认证服务不可用时
    error_page 500 = @auth_error;

    location @auth_error {
        return 503 '{"error":"Authentication service unavailable"}';
    }
}
```

### 2.8 应用场景

| 场景 | 实现方式 |
|------|----------|
| **OAuth2/OIDC 集成** | 认证服务验证 access_token，返回用户信息 |
| **JWT Token 验证** | 验证 JWT 签名和过期时间，提取 claims |
| **统一认证网关** | 集中处理多个服务的认证逻辑 |
| **权限分级验证** | 根据路径或资源进行细粒度权限检查 |
| **多因素认证** | 组合多种认证方式（密码 + 短信/邮件） |
| **API Key 验证** | 验证请求中的 API Key 有效性 |

### 2.9 最佳实践

**1. 认证服务高可用**
```nginx
upstream auth_backend {
    server 192.168.1.10:8080;
    server 192.168.1.11:8080 backup;
    keepalive 32;
}

location = /auth {
    internal;
    proxy_pass http://auth_backend/verify;
    proxy_connect_timeout 5s;
    proxy_send_timeout 5s;
    proxy_read_timeout 5s;
}
```

**2. 缓存认证结果**（减少重复验证）
```nginx
location /api/ {
    auth_request /auth;

    # 启用缓存（需配合 proxy_cache）
    proxy_cache auth_cache;
    proxy_cache_valid 200 1m;

    proxy_pass http://backend;
}
```

**3. 调试认证流程**
```nginx
# 记录认证请求日志
log_format auth_log '$remote_addr - $time_local '
                    'auth_status=$auth_request_status '
                    'user=$auth_user';

access_log /var/log/nginx/auth.log auth_log;
```

---

## 3. HTTP 基础认证

### 配置认证

```nginx
location /admin/ {
    auth_basic "Admin Area";
    auth_basic_user_file /etc/nginx/.htpasswd;
}
```

### 创建密码文件

```bash
# 使用 htpasswd 创建
htpasswd -c /etc/nginx/.htpasswd user1
htpasswd /etc/nginx/.htpasswd user2

# 使用 openssl
echo "user1:$(openssl passwd -apr1 password1)" > /etc/nginx/.htpasswd
```

### 密码文件格式

```
# 注释
user1:encrypted_password1
user2:encrypted_password2:comment
```

**支持的密码类型**：
- `crypt()` 加密
- MD5 哈希（apr1）
- `{scheme}data` 语法（RFC 2307）

### 组合访问控制

```nginx
location /admin/ {
    auth_basic "Admin Area";
    auth_basic_user_file /etc/nginx/.htpasswd;

    # 认证通过后，还需 IP 白名单
    satisfy any;  # any 或 all

    allow 192.168.1.0/24;
    deny all;
}
```

---

## 4. 请求限制

### 请求速率限制

```nginx
http {
    # 定义限制区域
    limit_req_zone $binary_remote_addr zone=req_limit:10m rate=10r/s;
    limit_req_zone $server_name zone=server_limit:10m rate=100r/s;

    server {
        location /api/ {
            # 应用限制
            limit_req zone=req_limit burst=20 nodelay;
            limit_req zone=server_limit burst=50;
        }
    }
}
```

**参数说明**：
| 参数 | 说明 |
|------|------|
| `zone=name:size` | 共享内存区域 |
| `rate=Nr/s` | 请求速率（每秒 N 个请求） |
| `burst=N` | 突发请求数量 |
| `nodelay` | 不延迟过量请求 |
| `delay=N` | 开始延迟的阈值 |

### 连接数限制

```nginx
http {
    limit_conn_zone $binary_remote_addr zone=conn_limit:10m;

    server {
        location /download/ {
            limit_conn conn_limit 10;  # 每个 IP 最多 10 个连接
            limit_conn_status 503;     # 超限返回 503
            limit_conn_log_level warn; # 日志级别
        }
    }
}
```

### 综合限制示例

```nginx
http {
    # 请求速率限制
    limit_req_zone $binary_remote_addr zone=req_limit:10m rate=10r/s;

    # 连接数限制
    limit_conn_zone $binary_remote_addr zone=conn_limit:10m;

    server {
        # 全局限制
        limit_req zone=req_limit burst=20 nodelay;
        limit_conn conn_limit 20;

        location /api/ {
            # API 接口更严格
            limit_req zone=req_limit burst=5 nodelay;
            limit_conn conn_limit 5;
        }

        location /static/ {
            # 静态资源不限制
            limit_req off;
            limit_conn off;
        }
    }
}
```

### Dry Run 模式（限流测试）

NGINX 支持限流 dry run 模式，用于测试限流配置而不实际拒绝请求：

```nginx
server {
    location /api/ {
        limit_req zone=req_limit burst=20 nodelay;
        limit_req_dry_run on;       # 请求不被拒绝，但记录日志

        limit_conn conn_limit 10;
        limit_conn_dry_run on;      # 连接不被拒绝，但记录日志

        proxy_pass http://backend;
    }
}
```

**Dry Run 作用**：
- 限流判断正常执行，但不返回 503/429 错误
- 记录 `limiting requests/connections, dry run` 到错误日志
- 用于评估限流阈值是否设置合理

> **注意**：详细限流配置请参考 [20-nginx-rate-limiting.md](./20-nginx-rate-limiting.md)

---

## 5. 安全头部

### 基础安全头部

```nginx
server {
    # 隐藏版本号
    server_tokens off;

    # HSTS（强制 HTTPS）
    add_header Strict-Transport-Security "max-age=31536000; includeSubDomains; preload" always;

    # 防止 MIME 类型嗅探
    add_header X-Content-Type-Options "nosniff" always;

    # XSS 保护
    add_header X-XSS-Protection "1; mode=block" always;

    # 点击劫持保护
    add_header X-Frame-Options "SAMEORIGIN" always;

    # CSP
    add_header Content-Security-Policy "default-src 'self'; script-src 'self' 'unsafe-inline' 'unsafe-eval'; style-src 'self' 'unsafe-inline';" always;

    # Referrer 策略
    add_header Referrer-Policy "strict-origin-when-cross-origin" always;

    # 权限策略
    add_header Permissions-Policy "geolocation=(), microphone=(), camera=()" always;
}
```

### X-Frame-Options 选项

| 值 | 说明 |
|-----|------|
| `DENY` | 完全禁止 iframe 嵌入 |
| `SAMEORIGIN` | 仅允许同源 iframe |
| `ALLOW-FROM uri` | 允许指定来源（已废弃） |

---

## 6. 防盗链

### 基础防盗链

```nginx
location ~* \.(jpg|jpeg|png|gif|webp|flv|mp4|swf)$ {
    valid_referers none blocked server_names *.example.com example.* ~\.google\.;

    if ($invalid_referer) {
        return 403;
        # 或返回替代图片
        # rewrite ^/.*$ /hotlink.png break;
    }
}
```

### valid_referers 参数

| 参数 | 说明 |
|------|------|
| `none` | 缺少 Referer 头 |
| `blocked` | Referer 被 firewall 等删除 |
| `server_names` | server_name 中的域名 |
| `*.domain` | 通配符域名 |
| `~regex` | 正则表达式 |

---

## 7. SSL/TLS 安全

### 协议配置

```nginx
server {
    listen 443 ssl;
    server_name example.com;

    # 仅启用安全协议
    ssl_protocols TLSv1.2 TLSv1.3;

    # 安全加密套件
    ssl_ciphers ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256:ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-GCM-SHA384;
    ssl_prefer_server_ciphers on;

    # DH 参数
    ssl_dhparam /etc/nginx/ssl/dhparam.pem;

    # 会话配置
    ssl_session_cache shared:SSL:10m;
    ssl_session_timeout 10m;
    ssl_session_tickets off;

    # OCSP Stapling
    ssl_stapling on;
    ssl_stapling_verify on;
}
```

### 生成 DH 参数

```bash
openssl dhparam -out /etc/nginx/ssl/dhparam.pem 2048
```

---

## 8. 防止常见攻击

### SQL 注入防护

```nginx
# 阻止可疑请求
if ($request_uri ~* "(union|select|insert|drop|delete|update|cast|script|alert)") {
    return 403;
}

# 或使用 ModSecurity 模块
```

### 防止目录遍历

```nginx
location ~* \.(git|svn|htpasswd|htaccess|env) {
    deny all;
    return 404;
}

# 禁止访问隐藏文件
location ~ /\. {
    deny all;
    return 404;
}
```

### 限制请求方法

```nginx
location / {
    if ($request_method !~ ^(GET|HEAD|POST)$ ) {
        return 405;
    }
}
```

### 限制请求体大小

```nginx
http {
    client_max_body_size 10m;
    client_body_buffer_size 128k;
}
```

### 超时设置

```nginx
http {
    client_body_timeout 10s;
    client_header_timeout 10s;
    send_timeout 10s;
}
```

---

## 9. 限制特定 User-Agent

```nginx
# 阻止恶意爬虫
if ($http_user_agent ~* (bot|crawl|spider|scraper)) {
    return 403;
}

# 阻止特定工具
if ($http_user_agent ~* (wget|curl|python-requests|scrapy)) {
    return 403;
}

# 使用 map 更优雅
map $http_user_agent $block_ua {
    default 0;
    ~*bot 1;
    ~*crawl 1;
    ~*spider 1;
}

if ($block_ua) {
    return 403;
}
```

---

## 10. WAF 配置（ModSecurity）

### 安装 ModSecurity

```bash
# Ubuntu/Debian
apt install libmodsecurity3 modsecurity-crs

# 加载模块
load_module modules/ngx_http_modsecurity_module.so;
```

### 配置示例

```nginx
modsecurity on;
modsecurity_rules_file /etc/nginx/modsecurity.conf;

# 使用 OWASP Core Rule Set
modsecurity_rules '
    Include /usr/share/modsecurity-crs/*.conf
    Include /usr/share/modsecurity-crs/rules/*.conf
';
```

---

## 11. fail2ban 集成

### 创建 filter

```ini
# /etc/fail2ban/filter.d/nginx-limit-req.conf
[Definition]
failregex = ^<HOST> -.*"(GET|POST|HEAD).*" (403|404|444|503).*$
ignoreregex =
```

### 创建 jail

```ini
# /etc/fail2ban/jail.local
[nginx-limit-req]
enabled = true
filter = nginx-limit-req
port = http,https
logpath = /var/log/nginx/*error.log
findtime = 60
bantime = 3600
maxretry = 10
```

---

## 12. 安全配置检查清单

### 基础安全

- [ ] 隐藏版本号 (`server_tokens off`)
- [ ] 禁用 SSLv2/SSLv3/TLSv1.0/TLSv1.1
- [ ] 配置 HSTS
- [ ] 设置安全头部
- [ ] 禁用自动索引 (`autoindex off`)
- [ ] 禁止访问隐藏文件

### 访问控制

- [ ] 管理后台 IP 白名单
- [ ] 内部接口认证保护
- [ ] 敏感目录访问限制

### 请求限制

- [ ] 请求速率限制
- [ ] 连接数限制
- [ ] 请求体大小限制
- [ ] 请求超时设置

### SSL/TLS

- [ ] 使用 TLSv1.2/TLSv1.3
- [ ] 配置安全加密套件
- [ ] 启用 OCSP Stapling
- [ ] 配置 DH 参数
- [ ] 禁用会话票据（或轮转密钥）

### 日志监控

- [ ] 记录访问日志
- [ ] 监控异常请求
- [ ] 配置 fail2ban

---

## 13. 安全测试工具

### 在线测试

- SSL Labs: https://www.ssllabs.com/ssltest/
- Security Headers: https://securityheaders.com/
- Observatory: https://observatory.mozilla.org/

### 命令行工具

```bash
# SSL 测试
testssl.sh example.com

# 头部检查
curl -I https://example.com

# 漏洞扫描
nikto -h https://example.com
nmap --script http-vuln* -p 443 example.com
```