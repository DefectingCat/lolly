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

## 2. HTTP 基础认证

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

## 3. 请求限制

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

---

## 4. 安全头部

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

## 5. 防盗链

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

## 6. SSL/TLS 安全

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

## 7. 防止常见攻击

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

## 8. 限制特定 User-Agent

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

## 9. WAF 配置（ModSecurity）

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

## 10. fail2ban 集成

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

## 11. 安全配置检查清单

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

## 12. 安全测试工具

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