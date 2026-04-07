# ACME 自动证书管理指南

本文档详细介绍如何在 Nginx 中使用 `ngx_http_acme_module` 模块实现 ACME 协议自动证书管理，包括 Let's Encrypt 的配置、自动续期、多域名和通配符证书等高级用法。

---

## 1. ACME 协议概述

### 1.1 什么是 ACME

ACME（Automatic Certificate Management Environment）协议是由 Let's Encrypt 开发的标准协议，用于自动化域名验证和 SSL/TLS 证书颁发。它使服务器能够自动获取和续期证书，无需人工干预。

### 1.2 工作原理

ACME 工作流程分为两个阶段：

```
┌─────────────┐                    ┌─────────────┐
│  ACME 客户端 │  ←──────────────→  │  CA 服务器  │
│  (Nginx)    │    1. 账户注册      │  (Let's     │
│             │    2. 订单创建      │   Encrypt)  │
│             │    3. 挑战验证      │             │
│             │    4. 证书颁发      │             │
└─────────────┘                    └─────────────┘
```

**完整流程：**

1. **账户注册**：ACME 客户端生成密钥对并向 CA 注册账户
2. **订单创建**：客户端请求为指定域名颁发证书
3. **挑战验证**：CA 要求客户端证明对域名的控制权（通过 HTTP-01、DNS-01 或 TLS-ALPN-01）
4. **证书颁发**：验证通过后，CA 签发证书并发布到 Certificate Transparency (CT) 日志

### 1.3 挑战类型对比

| 挑战类型 | 验证方式 | 通配符支持 | 端口 80 要求 | 适用场景 |
|:--------:|:--------:|:----------:|:------------:|:---------|
| HTTP-01 | 在 `/.well-known/acme-challenge/` 放置文件 | ❌ | ✅ 必须可用 | 标准 Web 服务器，最简单 |
| DNS-01 | 添加 `_acme-challenge` TXT 记录 | ✅ | ❌ 不需要 | 通配符证书、内部服务器 |
| TLS-ALPN-01 | 通过 TLS ALPN 扩展验证 | ❌ | ❌ 不需要 | 仅支持 443 端口的环境 |

---

## 2. ngx_http_acme_module 指令详解

### 2.1 指令汇总表

| 指令 | 语法 | 默认值 | 上下文 | 描述 |
|:-----|:-----|:------:|:-------|:-----|
| `acme_issuer` | `acme_issuer name { ... }` | — | http | 定义 ACME 证书颁发机构对象 |
| `uri` | `uri uri;` | — | acme_issuer | ACME 服务器目录 URL（必填） |
| `account_key` | `account_key alg[:size] \| file;` | — | acme_issuer | 账户私钥（支持 ecdsa/rsa 或文件路径） |
| `challenge` | `challenge type;` | `http-01` | acme_issuer | 挑战类型：`http-01` 或 `tls-alpn-01` |
| `contact` | `contact URL;` | — | acme_issuer | 联系邮箱（建议 `mailto:` 格式） |
| `external_account_key` | `external_account_key kid file;` | — | acme_issuer | 外部账户授权密钥（EAB） |
| `preferred_chain` | `preferred_chain name;` | — | acme_issuer | 指定首选证书链 |
| `profile` | `profile name [require];` | — | acme_issuer | 请求特定证书配置文件 |
| `ssl_trusted_certificate` | `ssl_trusted_certificate file;` | — | acme_issuer | 验证 ACME 服务器证书的 CA 证书 |
| `ssl_verify` | `ssl_verify on \| off;` | `on` | acme_issuer | 是否验证 ACME 服务器证书 |
| `state_path` | `state_path path \| off;` | `acme_<issuer>` | acme_issuer | 持久化存储路径（`off` 禁用） |
| `accept_terms_of_service` | `accept_terms_of_service;` | — | acme_issuer | 同意服务条款（部分服务器必需） |
| `acme_shared_zone` | `acme_shared_zone zone=name:size;` | `zone=ngx_acme_shared:256k` | http | 共享内存区大小 |
| `acme_certificate` | `acme_certificate issuer [identifier ...] [key=alg[:size]];` | — | server | 定义要请求的证书 |

### 2.2 嵌入式变量

在配置了 `acme_certificate` 的 `server` 块中可用：

| 变量 | 说明 | 用途 |
|:-----|:-----|:-----|
| `$acme_certificate` | SSL 证书路径 | `ssl_certificate` 指令 |
| `$acme_certificate_key` | SSL 证书私钥路径 | `ssl_certificate_key` 指令 |

---

## 3. Let's Encrypt 配置步骤

### 3.1 基础配置（HTTP-01 挑战）

**步骤 1：编译或启用模块**

确保 Nginx 包含 `ngx_http_acme_module` 模块：

```bash
nginx -V 2>&1 | grep -o 'http_acme_module'
```

**步骤 2：配置 DNS 解析器**

ACME 模块需要解析 Let's Encrypt 服务器域名：

```nginx
# nginx.conf

# 配置 DNS 解析器（根据你的网络环境调整）
resolver 8.8.8.8 8.8.4.4 valid=300s;
resolver_timeout 10s;
```

**步骤 3：定义 ACME 颁发机构**

```nginx
# Let's Encrypt 生产环境
acme_issuer letsencrypt {
    uri         https://acme-v02.api.letsencrypt.org/directory;
    contact     mailto:admin@example.com;
    state_path  /var/cache/nginx/acme-letsencrypt;
    accept_terms_of_service;
}

# Let's Encrypt 测试环境（开发调试时使用）
acme_issuer letsencrypt_staging {
    uri         https://acme-staging-v02.api.letsencrypt.org/directory;
    contact     mailto:admin@example.com;
    state_path  /var/cache/nginx/acme-staging;
    accept_terms_of_service;
}
```

**步骤 4：配置共享内存**

```nginx
# 增大共享内存以支持多个证书
acme_shared_zone zone=ngx_acme_shared:1M;
```

**步骤 5：配置 HTTPS 服务器**

```nginx
server {
    listen 443 ssl;
    server_name www.example.com example.com;

    # 启用 ACME 自动证书
    acme_certificate letsencrypt;

    # 使用 ACME 变量
    ssl_certificate       $acme_certificate;
    ssl_certificate_key   $acme_certificate_key;

    # 证书缓存优化（避免每次请求都解析）
    ssl_certificate_cache max=2;

    # SSL 优化配置
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers HIGH:!aNULL:!MD5;
    ssl_prefer_server_ciphers on;

    location / {
        root /var/www/html;
        index index.html;
    }
}
```

**步骤 6：配置 HTTP 服务器（用于挑战）**

```nginx
server {
    listen 80;
    server_name www.example.com example.com;

    # ACME HTTP-01 挑战需要访问 80 端口
    # Nginx ACME 模块会自动处理 /.well-known/acme-challenge/ 路径

    location / {
        return 301 https://$server_name$request_uri;
    }
}
```

### 3.2 首次启动和验证

```bash
# 测试配置语法
nginx -t

# 重载配置
nginx -s reload

# 查看日志确认证书申请状态
tail -f /var/log/nginx/error.log
```

### 3.3 Let's Encrypt 速率限制

| 限制类型 | 限制值 | 时间窗口 |
|:---------|:-------|:---------|
| 账户注册 | 10 个 | 每 IP 每 3 小时 |
| 新订单 | 300 个 | 每账户每 3 小时 |
| 域名证书 | 50 个 | 每域名每 7 天 |
| 相同标识符集 | 5 个 | 每 7 天 |
| 验证失败 | 5 次 | 每标识符每小时 |

**重要提示：**
- 使用 Staging 环境进行开发和测试
- 续期操作通常不会触发速率限制
- 使用 ARI（ACME Renewal Information）可免除速率限制

---

## 4. 自动续期配置

### 4.1 自动续期原理

`ngx_http_acme_module` 模块会自动处理证书续期：

1. **监控证书有效期**：模块持续监控已颁发证书的过期时间
2. **自动续期触发**：在证书过期前自动发起续期请求
3. **无缝替换**：新证书获取后自动替换，无需重启 Nginx
4. **持久化存储**：证书和密钥存储在 `state_path` 指定的目录

### 4.2 状态目录结构

```
/var/cache/nginx/acme-letsencrypt/
├── account/                    # 账户密钥和配置
│   ├── private.key            # 账户私钥
│   └── registration.json      # 账户注册信息
├── orders/                    # 订单状态
│   └── *.json
└── certs/                     # 颁发的证书
    ├── example.com/           # 按域名组织
    │   ├── cert.pem          # 证书
    │   ├── chain.pem         # 证书链
    │   ├── fullchain.pem     # 完整证书链
    │   └── privkey.pem       # 私钥
    └── www.example.com/
```

### 4.3 配置证书续期监控

```nginx
# 可选：配置日志监控续期情况
error_log /var/log/nginx/acme.log info;

acme_issuer letsencrypt {
    uri         https://acme-v02.api.letsencrypt.org/directory;
    contact     mailto:admin@example.com;
    state_path  /var/cache/nginx/acme-letsencrypt;
    accept_terms_of_service;

    # 可选：指定首选证书链
    preferred_chain "ISRG Root X1";
}
```

### 4.4 备份和恢复

**备份脚本：**

```bash
#!/bin/bash
# backup-acme.sh

BACKUP_DIR="/backup/nginx-acme/$(date +%Y%m%d)"
mkdir -p "$BACKUP_DIR"

# 备份 ACME 状态目录
cp -r /var/cache/nginx/acme-letsencrypt "$BACKUP_DIR/"

# 备份 Nginx 配置
cp -r /etc/nginx "$BACKUP_DIR/nginx-config"

echo "ACME backup completed: $BACKUP_DIR"
```

**恢复脚本：**

```bash
#!/bin/bash
# restore-acme.sh

BACKUP_DIR="$1"

if [ -z "$BACKUP_DIR" ]; then
    echo "Usage: $0 <backup-directory>"
    exit 1
fi

# 恢复 ACME 状态
systemctl stop nginx
cp -r "$BACKUP_DIR/acme-letsencrypt" /var/cache/nginx/
chown -R nginx:nginx /var/cache/nginx/acme-letsencrypt
systemctl start nginx

echo "ACME restore completed from: $BACKUP_DIR"
```

### 4.5 监控和告警

**检查证书过期时间脚本：**

```bash
#!/bin/bash
# check-cert-expiry.sh

CERT_DIR="/var/cache/nginx/acme-letsencrypt/certs"
WARNING_DAYS=7

for cert_path in $CERT_DIR/*/cert.pem; do
    if [ -f "$cert_path" ]; then
        domain=$(basename $(dirname "$cert_path"))
        expiry=$(openssl x509 -enddate -noout -in "$cert_path" | cut -d= -f2)
        expiry_epoch=$(date -d "$expiry" +%s)
        now_epoch=$(date +%s)
        days_left=$(( ($expiry_epoch - $now_epoch) / 86400 ))

        if [ $days_left -lt $WARNING_DAYS ]; then
            echo "WARNING: Certificate for $domain expires in $days_left days"
        else
            echo "OK: Certificate for $domain expires in $days_left days"
        fi
    fi
done
```

---

## 5. 多域名证书管理

### 5.1 多域名证书（SAN 证书）

单个证书可以包含多个域名（Subject Alternative Names）：

```nginx
acme_issuer letsencrypt {
    uri         https://acme-v02.api.letsencrypt.org/directory;
    contact     mailto:admin@example.com;
    state_path  /var/cache/nginx/acme-letsencrypt;
    accept_terms_of_service;
}

server {
    listen 443 ssl;
    # 主域名和多个别名
    server_name example.com www.example.com api.example.com;

    # 为所有 server_name 申请单个证书
    acme_certificate letsencrypt;

    ssl_certificate       $acme_certificate;
    ssl_certificate_key   $acme_certificate_key;
    ssl_certificate_cache max=2;

    location / {
        proxy_pass http://backend;
    }
}
```

### 5.2 独立域名证书

为不同域名申请独立证书：

```nginx
acme_issuer letsencrypt {
    uri         https://acme-v02.api.letsencrypt.org/directory;
    contact     mailto:admin@example.com;
    state_path  /var/cache/nginx/acme-letsencrypt;
    accept_terms_of_service;
}

# 主站点
server {
    listen 443 ssl;
    server_name example.com www.example.com;

    acme_certificate letsencrypt example.com www.example.com;

    ssl_certificate       $acme_certificate;
    ssl_certificate_key   $acme_certificate_key;
    ssl_certificate_cache max=2;

    location / {
        root /var/www/example;
    }
}

# API 站点
server {
    listen 443 ssl;
    server_name api.example.com;

    acme_certificate letsencrypt api.example.com;

    ssl_certificate       $acme_certificate;
    ssl_certificate_key   $acme_certificate_key;
    ssl_certificate_cache max=2;

    location / {
        proxy_pass http://api-backend;
    }
}

# 博客站点
server {
    listen 443 ssl;
    server_name blog.example.com;

    acme_certificate letsencrypt blog.example.com;

    ssl_certificate       $acme_certificate;
    ssl_certificate_key   $acme_certificate_key;
    ssl_certificate_cache max=2;

    location / {
        proxy_pass http://blog-backend;
    }
}
```

### 5.3 多 ACME 账户配置

不同域名使用不同的 ACME 账户：

```nginx
# 主账户
acme_issuer letsencrypt_main {
    uri         https://acme-v02.api.letsencrypt.org/directory;
    contact     mailto:admin@example.com;
    state_path  /var/cache/nginx/acme-main;
    accept_terms_of_service;
    account_key ecdsa:384;
}

# 客户项目账户
acme_issuer letsencrypt_client {
    uri         https://acme-v02.api.letsencrypt.org/directory;
    contact     mailto:client-projects@example.com;
    state_path  /var/cache/nginx/acme-client;
    accept_terms_of_service;
    account_key ecdsa:256;
}

server {
    listen 443 ssl;
    server_name project1.example.com;

    acme_certificate letsencrypt_client project1.example.com;

    ssl_certificate       $acme_certificate;
    ssl_certificate_key   $acme_certificate_key;
}
```

---

## 6. 通配符证书配置（DNS 验证）

### 6.1 DNS-01 挑战说明

通配符证书（如 `*.example.com`）必须使用 DNS-01 挑战类型验证。这要求：

1. 能够自动修改 DNS 记录（通过 DNS 提供商 API）
2. 在 `_acme-challenge.example.com` 添加 TXT 记录
3. 等待 DNS 传播后验证

**注意：** `ngx_http_acme_module` 本身不直接支持 DNS-01 挑战（需要外部 DNS 管理工具配合），可以使用 `certbot` 等工具获取通配符证书后由 Nginx 使用。

### 6.2 使用 Certbot 获取通配符证书

```bash
# 安装 certbot 和 DNS 插件（以 Cloudflare 为例）
# Ubuntu/Debian
sudo apt install certbot python3-certbot-dns-cloudflare

# CentOS/RHEL
sudo yum install certbot python3-certbot-dns-cloudflare
```

**配置 DNS API 凭证：**

```bash
# 创建 Cloudflare 凭证文件
sudo mkdir -p /etc/letsencrypt
sudo cat > /etc/letsencrypt/dnscloudflare.ini << 'EOF'
dns_cloudflare_api_token = your-api-token-here
EOF
sudo chmod 600 /etc/letsencrypt/dnscloudflare.ini
```

**申请通配符证书：**

```bash
sudo certbot certonly \
    --dns-cloudflare \
    --dns-cloudflare-credentials /etc/letsencrypt/dnscloudflare.ini \
    -d "example.com" \
    -d "*.example.com" \
    --preferred-challenges dns-01
```

**自动续期配置：**

```bash
# 测试续期
certbot renew --dry-run

# 添加定时任务
echo "0 3 * * * root certbot renew --quiet --deploy-hook 'nginx -s reload'" | sudo tee -a /etc/crontab
```

### 6.3 Nginx 使用通配符证书

```nginx
server {
    listen 443 ssl;
    # 使用通配符匹配所有子域名
    server_name *.example.com;

    # 使用 certbot 获取的证书
    ssl_certificate /etc/letsencrypt/live/example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/example.com/privkey.pem;

    ssl_certificate_cache max=2;

    # 子域名路由
    location / {
        # 提取子域名
        set $subdomain "";
        if ($host ~* ^([^.]+)\.example\.com$) {
            set $subdomain $1;
        }

        # 根据子域名代理到不同后端
        proxy_pass http://$subdomain-backend;
    }
}

# 主域名单独配置
server {
    listen 443 ssl;
    server_name example.com;

    ssl_certificate /etc/letsencrypt/live/example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/example.com/privkey.pem;

    location / {
        root /var/www/main;
    }
}
```

### 6.4 主流 DNS 提供商 Certbot 插件

| 提供商 | 安装命令 | 配置方式 |
|:-------|:---------|:---------|
| Cloudflare | `python3-certbot-dns-cloudflare` | API Token |
| Route53 | `python3-certbot-dns-route53` | AWS IAM 凭证 |
| Alibaba Cloud | `certbot-dns-aliyun` | Access Key |
| Tencent Cloud | `certbot-dns-tencentcloud` | Secret ID/Key |
| GoDaddy | `certbot-dns-godaddy` | API Key/Secret |

---

## 7. 与 Certbot 方案对比

### 7.1 方案对比表

| 特性 | ngx_http_acme_module | Certbot |
|:-----|:--------------------:|:-------:|
| **集成度** | 内置于 Nginx，无需外部工具 | 独立程序，需单独安装 |
| **配置复杂度** | 纯 Nginx 配置 | 需要额外配置和定时任务 |
| **HTTP-01 支持** | ✅ 原生支持 | ✅ 支持 |
| **DNS-01 支持** | ❌ 不支持 | ✅ 支持 |
| **通配符证书** | ❌ 不支持 | ✅ 支持 |
| **TLS-ALPN-01** | ✅ 支持 | ❌ 不支持 |
| **自动续期** | ✅ 自动，无需外部任务 | ✅ 需配置 cron/systemd timer |
| **证书热重载** | ✅ 无缝更新 | ⚠️ 需 reload/restart Nginx |
| **多 Web 服务器** | ❌ 仅 Nginx | ✅ Apache, Nginx 等 |
| **外部账户绑定** | ✅ 支持 EAB | ✅ 支持 |

### 7.2 选择建议

**选择 ngx_http_acme_module：**
- 纯 Nginx 环境，追求配置简洁
- 使用 HTTP-01 或 TLS-ALPN-01 挑战
- 不需要通配符证书
- 希望证书续期完全自动化

**选择 Certbot：**
- 需要通配符证书
- 使用 DNS-01 挑战
- 多 Web 服务器环境
- 需要与外部系统集成

### 7.3 混合方案

结合两者优势：使用 `ngx_http_acme_module` 处理常规证书，使用 `certbot` 处理通配符证书：

```nginx
# 常规域名使用内置 ACME
server {
    listen 443 ssl;
    server_name www.example.com api.example.com;

    acme_certificate letsencrypt;
    ssl_certificate $acme_certificate;
    ssl_certificate_key $acme_certificate_key;
}

# 通配符使用 certbot 证书
server {
    listen 443 ssl;
    server_name *.example.com;

    ssl_certificate /etc/letsencrypt/live/example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/example.com/privkey.pem;
}
```

---

## 8. 完整配置示例

### 8.1 单站点基础配置

```nginx
# /etc/nginx/nginx.conf

user nginx;
worker_processes auto;
error_log /var/log/nginx/error.log warn;
pid /var/run/nginx.pid;

events {
    worker_connections 1024;
}

http {
    include /etc/nginx/mime.types;
    default_type application/octet-stream;

    log_format main '$remote_addr - $remote_user [$time_local] "$request" '
                    '$status $body_bytes_sent "$http_referer" '
                    '"$http_user_agent" "$http_x_forwarded_for"';

    access_log /var/log/nginx/access.log main;

    sendfile on;
    tcp_nopush on;
    tcp_nodelay on;
    keepalive_timeout 65;
    types_hash_max_size 2048;

    # DNS 解析器配置
    resolver 8.8.8.8 8.8.4.4 valid=300s;
    resolver_timeout 10s;

    # ACME 共享内存
    acme_shared_zone zone=ngx_acme_shared:1M;

    # Let's Encrypt 配置
    acme_issuer letsencrypt {
        uri         https://acme-v02.api.letsencrypt.org/directory;
        contact     mailto:admin@example.com;
        state_path  /var/cache/nginx/acme-letsencrypt;
        accept_terms_of_service;

        # 使用 ECDSA 账户密钥（更高效）
        account_key ecdsa:384;

        # 启用证书验证
        ssl_verify on;
    }

    # HTTP 服务器 - 处理 ACME 挑战和重定向
    server {
        listen 80;
        server_name example.com www.example.com;

        # ACME 挑战自动处理
        # 其他请求重定向到 HTTPS
        location / {
            return 301 https://$server_name$request_uri;
        }
    }

    # HTTPS 服务器
    server {
        listen 443 ssl http2;
        server_name example.com www.example.com;

        # 启用 ACME 自动证书
        acme_certificate letsencrypt;

        # 使用 ACME 变量
        ssl_certificate       $acme_certificate;
        ssl_certificate_key   $acme_certificate_key;
        ssl_certificate_cache max=2;

        # SSL 安全配置
        ssl_protocols TLSv1.2 TLSv1.3;
        ssl_ciphers ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256:ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-GCM-SHA384;
        ssl_prefer_server_ciphers on;
        ssl_session_cache shared:SSL:10m;
        ssl_session_timeout 1d;
        ssl_session_tickets off;

        # OCSP Stapling
        ssl_stapling on;
        ssl_stapling_verify on;
        ssl_trusted_certificate $acme_certificate;

        # 安全响应头
        add_header Strict-Transport-Security "max-age=63072000; includeSubDomains; preload" always;
        add_header X-Frame-Options "SAMEORIGIN" always;
        add_header X-Content-Type-Options "nosniff" always;
        add_header X-XSS-Protection "1; mode=block" always;
        add_header Referrer-Policy "strict-origin-when-cross-origin" always;

        root /var/www/example;
        index index.html index.htm;

        location / {
            try_files $uri $uri/ =404;
        }

        # 静态文件缓存
        location ~* \.(js|css|png|jpg|jpeg|gif|ico|svg|woff|woff2|ttf|eot)$ {
            expires 6M;
            access_log off;
        }
    }
}
```

### 8.2 多站点生产配置

```nginx
# /etc/nginx/nginx.conf

user nginx;
worker_processes auto;
error_log /var/log/nginx/error.log warn;
pid /var/run/nginx.pid;

events {
    worker_connections 2048;
    use epoll;
    multi_accept on;
}

http {
    include /etc/nginx/mime.types;
    default_type application/octet-stream;

    log_format main '$remote_addr - $remote_user [$time_local] "$request" '
                    '$status $body_bytes_sent "$http_referer" '
                    '"$http_user_agent" "$http_x_forwarded_for" '
                    'rt=$request_time uct="$upstream_connect_time" '
                    'uht="$upstream_header_time" urt="$upstream_response_time"';

    access_log /var/log/nginx/access.log main;

    sendfile on;
    tcp_nopush on;
    tcp_nodelay on;
    keepalive_timeout 65;
    types_hash_max_size 2048;
    server_tokens off;

    # Gzip 压缩
    gzip on;
    gzip_vary on;
    gzip_proxied any;
    gzip_comp_level 6;
    gzip_types text/plain text/css text/xml application/json application/javascript application/rss+xml application/atom+xml image/svg+xml;

    # DNS 配置
    resolver 8.8.8.8 1.1.1.1 valid=300s;
    resolver_timeout 10s;

    # ACME 配置
    acme_shared_zone zone=ngx_acme_shared:2M;

    # Let's Encrypt 生产环境
    acme_issuer letsencrypt {
        uri         https://acme-v02.api.letsencrypt.org/directory;
        contact     mailto:ssl@example.com;
        state_path  /var/cache/nginx/acme-letsencrypt;
        accept_terms_of_service;
        account_key ecdsa:384;
        preferred_chain "ISRG Root X1";
    }

    # Let's Encrypt 测试环境
    acme_issuer letsencrypt_staging {
        uri         https://acme-staging-v02.api.letsencrypt.org/directory;
        contact     mailto:ssl@example.com;
        state_path  /var/cache/nginx/acme-staging;
        accept_terms_of_service;
    }

    # SSL 优化配置（共享）
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256:ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-GCM-SHA384;
    ssl_prefer_server_ciphers on;
    ssl_session_cache shared:SSL:50m;
    ssl_session_timeout 1d;
    ssl_session_tickets off;
    ssl_stapling on;
    ssl_stapling_verify on;

    # 通用安全响应头
    add_header X-Frame-Options "SAMEORIGIN" always;
    add_header X-Content-Type-Options "nosniff" always;
    add_header X-XSS-Protection "1; mode=block" always;
    add_header Referrer-Policy "strict-origin-when-cross-origin" always;

    # 站点配置目录
    include /etc/nginx/conf.d/*.conf;
}
```

```nginx
# /etc/nginx/conf.d/01-http-default.conf

# HTTP 默认服务器 - 处理所有 80 端口请求
server {
    listen 80 default_server;
    listen [::]:80 default_server;
    server_name _;

    # ACME 挑战处理
    location /.well-known/acme-challenge/ {
        # 由 ngx_http_acme_module 自动处理
        root /var/cache/nginx/acme-challenges;
    }

    # 所有其他请求重定向到 HTTPS
    location / {
        return 301 https://$host$request_uri;
    }
}
```

```nginx
# /etc/nginx/conf.d/10-example.com.conf

# 主站点
server {
    listen 443 ssl http2;
    listen [::]:443 ssl http2;
    server_name example.com www.example.com;

    acme_certificate letsencrypt;

    ssl_certificate       $acme_certificate;
    ssl_certificate_key   $acme_certificate_key;
    ssl_certificate_cache max=2;

    add_header Strict-Transport-Security "max-age=63072000; includeSubDomains; preload" always;

    root /var/www/example;
    index index.html index.php;

    location / {
        try_files $uri $uri/ /index.php?$query_string;
    }

    # PHP 处理
    location ~ \.php$ {
        fastcgi_pass unix:/var/run/php/php8.1-fpm.sock;
        fastcgi_index index.php;
        fastcgi_param SCRIPT_FILENAME $document_root$fastcgi_script_name;
        include fastcgi_params;
    }

    location ~ /\.ht {
        deny all;
    }
}
```

```nginx
# /etc/nginx/conf.d/20-api.example.com.conf

# API 站点
server {
    listen 443 ssl http2;
    listen [::]:443 ssl http2;
    server_name api.example.com;

    acme_certificate letsencrypt api.example.com;

    ssl_certificate       $acme_certificate;
    ssl_certificate_key   $acme_certificate_key;
    ssl_certificate_cache max=2;

    add_header Strict-Transport-Security "max-age=63072000" always;

    # API 限流
    limit_req_zone $binary_remote_addr zone=api:10m rate=10r/s;
    limit_req zone=api burst=20 nodelay;

    location / {
        proxy_pass http://api_backend;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_cache_bypass $http_upgrade;
    }
}

upstream api_backend {
    server 10.0.1.10:8080;
    server 10.0.1.11:8080;
    keepalive 32;
}
```

### 8.3 测试环境配置

```nginx
# 测试/开发环境使用 Staging

acme_issuer letsencrypt_staging {
    uri         https://acme-staging-v02.api.letsencrypt.org/directory;
    contact     mailto:dev@example.com;
    state_path  /var/cache/nginx/acme-staging;
    accept_terms_of_service;
}

server {
    listen 443 ssl;
    server_name staging.example.com;

    # 开发环境使用 Staging
    acme_certificate letsencrypt_staging staging.example.com;

    ssl_certificate       $acme_certificate;
    ssl_certificate_key   $acme_certificate_key;

    # 注意：Staging 证书不会被浏览器信任
    # 仅用于测试续期流程

    location / {
        root /var/www/staging;
    }
}
```

---

## 9. 故障排查指南

### 9.1 常见问题及解决方案

#### 问题 1：证书申请失败（验证失败）

**症状：**
```
[error] acme: challenge failed for example.com
```

**排查步骤：**

1. **检查 DNS 解析**
   ```bash
   nslookup example.com
   dig example.com A
   ```

2. **检查 80 端口可访问性**
   ```bash
   # 从外部测试
   curl -I http://example.com/

   # 检查防火墙
   sudo iptables -L -n | grep 80
   ```

3. **检查 Nginx 错误日志**
   ```bash
   sudo tail -f /var/log/nginx/error.log
   ```

4. **验证挑战响应**
   ```bash
   # 手动测试挑战 URL
   curl http://example.com/.well-known/acme-challenge/test
   ```

**解决方案：**
- 确保域名 DNS 解析正确且已生效
- 确保防火墙允许 80 端口入站连接
- 确保 `server_name` 包含申请证书的域名

#### 问题 2：DNS 解析失败

**症状：**
```
[error] could not resolve acme-v02.api.letsencrypt.org
```

**解决方案：**

```nginx
# 检查 resolver 配置
http {
    # 使用可靠的 DNS 服务器
    resolver 8.8.8.8 8.8.4.4 1.1.1.1 valid=300s;
    resolver_timeout 30s;
    ...
}
```

**测试 DNS 解析：**
```bash
# 测试系统 DNS
nslookup acme-v02.api.letsencrypt.org

# 测试 Nginx 配置语法
nginx -t
```

#### 问题 3：速率限制错误

**症状：**
```
[error] acme: 429 Too Many Requests
```

**排查：**
```bash
# 检查最近申请记录
grep "acme" /var/log/nginx/error.log | tail -20
```

**解决方案：**
- 切换到 Staging 环境进行测试
- 等待当前速率限制窗口重置
- 检查是否有重复申请配置
- 使用现有证书而不是重新申请

#### 问题 4：证书不自动续期

**症状：**
证书过期但未自动续期

**排查：**
```bash
# 检查证书状态
openssl x509 -in /var/cache/nginx/acme-letsencrypt/certs/example.com/cert.pem -noout -dates

# 检查 Nginx 错误日志
grep -i "acme\|certificate" /var/log/nginx/error.log
```

**解决方案：**

```nginx
# 确保配置正确
acme_issuer letsencrypt {
    uri         https://acme-v02.api.letsencrypt.org/directory;
    contact     mailto:admin@example.com;
    state_path  /var/cache/nginx/acme-letsencrypt;  # 确保有写入权限
    accept_terms_of_service;
}
```

**检查权限：**
```bash
# 确保 Nginx 用户可以写入 state_path
sudo chown -R nginx:nginx /var/cache/nginx/acme-letsencrypt
sudo chmod 700 /var/cache/nginx/acme-letsencrypt
```

#### 问题 5：$acme_certificate 变量为空

**症状：**
```
[emerg] BIO_new_file("$acme_certificate") failed
```

**解决方案：**

```nginx
# 确保 acme_certificate 指令在 server 块中
server {
    listen 443 ssl;
    server_name example.com;

    # 必须先声明 acme_certificate
    acme_certificate letsencrypt;

    # 然后才能使用变量
    ssl_certificate       $acme_certificate;
    ssl_certificate_key   $acme_certificate_key;
}
```

### 9.2 调试配置

**启用详细日志：**

```nginx
# 开发调试时使用
error_log /var/log/nginx/acme-debug.log debug;

acme_issuer letsencrypt_staging {
    uri         https://acme-staging-v02.api.letsencrypt.org/directory;
    contact     mailto:debug@example.com;
    state_path  /var/cache/nginx/acme-staging;
    accept_terms_of_service;

    # 禁用服务器证书验证（仅测试时使用）
    # ssl_verify off;
}
```

### 9.3 诊断脚本

```bash
#!/bin/bash
# diagnose-acme.sh - ACME 诊断脚本

echo "=== Nginx ACME 诊断 ==="
echo

# 检查 Nginx 版本和模块
echo "1. Nginx 版本和模块："
nginx -V 2>&1 | grep -E "(nginx version|http_acme_module)"
echo

# 检查配置语法
echo "2. 配置语法检查："
nginx -t
echo

# 检查 DNS 解析
echo "3. DNS 解析测试："
nslookup acme-v02.api.letsencrypt.org 2>/dev/null || echo "DNS 解析失败"
echo

# 检查证书目录
echo "4. ACME 状态目录："
for dir in /var/cache/nginx/acme-*; do
    if [ -d "$dir" ]; then
        echo "  目录: $dir"
        echo "  大小: $(du -sh "$dir" 2>/dev/null | cut -f1)"
        echo "  权限: $(stat -c %a "$dir" 2>/dev/null || stat -f %A "$dir" 2>/dev/null)"
        echo "  所有者: $(stat -c %U:%G "$dir" 2>/dev/null || stat -f %Su:%Sg "$dir" 2>/dev/null)"
    fi
done
echo

# 检查现有证书
echo "5. 现有证书："
for cert in /var/cache/nginx/acme-*/certs/*/cert.pem; do
    if [ -f "$cert" ]; then
        domain=$(basename $(dirname "$cert"))
        echo "  域名: $domain"
        openssl x509 -in "$cert" -noout -dates -subject 2>/dev/null | sed 's/^/    /'
        echo
    fi
done

# 检查端口监听
echo "6. 端口监听："
ss -tlnp | grep -E ":80|:443" | sed 's/^/  /'
echo

# 检查防火墙
echo "7. 防火墙状态："
if command -v iptables &> /dev/null; then
    iptables -L -n | grep -E "80|443" | sed 's/^/  /'
else
    echo "  iptables 不可用"
fi
echo

# 检查错误日志
echo "8. 最近的 ACME 相关错误："
grep -i "acme" /var/log/nginx/error.log 2>/dev/null | tail -10 | sed 's/^/  /'
echo

echo "=== 诊断完成 ==="
```

### 9.4 获取帮助

**官方资源：**
- Nginx ACME 模块文档：https://nginx.org/en/docs/http/ngx_http_acme_module.html
- Let's Encrypt 文档：https://letsencrypt.org/docs/
- Let's Encrypt 社区：https://community.letsencrypt.org/

**测试工具：**
- SSL Labs 测试：https://www.ssllabs.com/ssltest/
- Let's Debug：https://letsdebug.net/（诊断域名验证问题）

---

## 附录：快速参考

### 配置检查清单

- [ ] DNS 解析器已配置（`resolver`）
- [ ] `acme_shared_zone` 已定义
- [ ] `acme_issuer` 块配置正确
- [ ] `acme_certificate` 指令在 `server` 块中
- [ ] `ssl_certificate` 使用 `$acme_certificate` 变量
- [ ] 80 端口服务器配置正确
- [ ] state_path 目录可写
- [ ] 防火墙允许 80/443 端口
- [ ] 域名 DNS 已生效

### 常用命令

```bash
# 测试配置
nginx -t

# 重载配置
nginx -s reload

# 查看证书信息
openssl x509 -in cert.pem -noout -text

# 查看证书过期时间
openssl x509 -in cert.pem -noout -dates

# 手动清理状态（谨慎使用）
rm -rf /var/cache/nginx/acme-letsencrypt/certs/example.com
```

---

*文档版本：1.0*  
*最后更新：2025年4月*
