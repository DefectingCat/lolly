# NGINX SSL/TLS 与 HTTPS 配置指南

## 1. HTTPS 基础配置

### 最简配置

```nginx
server {
    listen 443 ssl;
    server_name www.example.com;

    ssl_certificate     www.example.com.crt;
    ssl_certificate_key www.example.com.key;

    ssl_protocols       TLSv1.2 TLSv1.3;
    ssl_ciphers         HIGH:!aNULL:!MD5;
}
```

### 完整配置示例

```nginx
server {
    listen 443 ssl http2;
    server_name www.example.com;

    # 证书配置
    ssl_certificate      /etc/nginx/ssl/www.example.com.crt;
    ssl_certificate_key  /etc/nginx/ssl/www.example.com.key;

    # 协议与加密套件
    ssl_protocols        TLSv1.2 TLSv1.3;
    ssl_ciphers          ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256;
    ssl_prefer_server_ciphers on;

    # 会话缓存
    ssl_session_cache    shared:SSL:10m;
    ssl_session_timeout  10m;
    ssl_session_tickets  off;

    # OCSP Stapling
    ssl_stapling         on;
    ssl_stapling_verify  on;
    resolver             8.8.8.8 8.8.4.4 valid=300s;

    # 安全头部
    add_header Strict-Transport-Security "max-age=31536000; includeSubDomains" always;

    location / {
        root /var/www/html;
    }
}
```

---

## 2. SSL 指令详解

### 证书与密钥

| 指令 | 说明 |
|------|------|
| `ssl_certificate` | PEM 格式证书文件路径 |
| `ssl_certificate_key` | PEM 格式私钥文件路径 |
| `ssl_password_file` | 私钥密码文件（每行一个密码） |

```nginx
ssl_certificate     /path/to/cert.crt;
ssl_certificate_key /path/to/key.key;

# 多证书类型（RSA + ECDSA）
ssl_certificate     /path/to RSA.crt;
ssl_certificate     /path/to ECDSA.crt;
ssl_certificate_key /path/to RSA.key;
ssl_certificate_key /path/to ECDSA.key;
```

**注意**：
- 证书是公开实体，可设置较宽松权限
- 私钥需限制访问权限（600），但 nginx 主进程可读
- 私钥可与证书存放在同一文件中

### 协议配置

| 指令 | 说明 | 默认值 |
|------|------|--------|
| `ssl_protocols` | 启用的协议 | TLSv1.2 TLSv1.3 |
| `ssl_ciphers` | 启用的加密套件 | HIGH:!aNULL:!MD5 |
| `ssl_prefer_server_ciphers` | 服务器套件优先 | off |

```nginx
ssl_protocols TLSv1.2 TLSv1.3;

# 推荐加密套件（现代浏览器）
ssl_ciphers ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256:ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-GCM-SHA384;

ssl_prefer_server_ciphers on;
```

### 协议版本建议

| 版本 | 建议 |
|------|------|
| SSLv2 | 禁用（不安全） |
| SSLv3 | 禁用（POODLE 攻击） |
| TLSv1.0 | 禁用（旧浏览器兼容时可用） |
| TLSv1.1 | 禁用 |
| TLSv1.2 | 启用 |
| TLSv1.3 | 启用（推荐） |

---

## 3. 会话缓存

### 缓存类型

| 类型 | 说明 |
|------|------|
| `off` | 禁用会话缓存 |
| `none` | 不使用缓存，但允许会话票据 |
| `builtin` | 内置 OpenSSL 缓存（单 worker） |
| `shared` | 共享内存缓存（所有 worker） |

### 配置示例

```nginx
# 推荐：共享缓存
ssl_session_cache shared:SSL:10m;
ssl_session_timeout 10m;

# 1MB 约存储 4000 个会话
# 多 worker 共享，避免重复握手
```

### 会话票据

```nginx
ssl_session_tickets on;              # 默认 on
ssl_session_ticket_key /path/to/key; # 加密票据密钥
```

**密钥轮转**：
```nginx
ssl_session_ticket_key /path/to/old.key;
ssl_session_ticket_key /path/to/current.key;
ssl_session_ticket_key /path/to/new.key;
```

---

## 4. OCSP Stapling

减少 SSL 握手时间，提升性能。

```nginx
ssl_stapling on;
ssl_stapling_verify on;
ssl_stapling_file /path/to/ocsp.der;          # 可选：手动指定 OCSP 响应
ssl_stapling_responder http://ocsp.example.com;  # 可选：覆盖响应者 URL
resolver 8.8.8.8 8.8.4.4 valid=300s;
resolver_timeout 5s;
```

---

## 5. 客户端证书验证

### 基础配置

```nginx
ssl_client_certificate /path/to/ca.crt;
ssl_verify_client on;                # on | off | optional | optional_no_ca
ssl_verify_depth 2;
```

| 参数 | 说明 |
|------|------|
| `on` | 必须提供有效证书 |
| `off` | 不验证客户端证书 |
| `optional` | 可选验证，失败仍允许访问 |
| `optional_no_ca` | 可选证书，不验证有效性 |

### 错误处理

```nginx
# 验证错误返回 495
error_page 495 /cert_error.html;

# 未提供证书返回 496
error_page 496 /no_cert.html;

# HTTP 请求发送到 HTTPS 端口返回 497
error_page 497 /redirect.html;
```

---

## 6. HTTPS 服务器优化

### CPU 资源优化

SSL 握手消耗 CPU 资源，建议：

```nginx
worker_processes auto;               # 与 CPU 核心数相同
keepalive_timeout 70;                # 延长连接，减少握手
ssl_session_cache shared:SSL:10m;    # 会话缓存
```

### DH 参数

增强 DHE 加密套件安全性：

```nginx
ssl_dhparam /path/to/dhparam.pem;

# 生成 DH 参数
openssl dhparam -out dhparam.pem 2048
```

### ECDH 曲线

```nginx
ssl_ecdh_curve auto;                 # 默认 auto
ssl_ecdh_curve prime256v1:secp384r1; # 指定曲线
```

---

## 7. SSL 证书链

### 证书链问题

浏览器报错证书可信度问题，可能缺少中间证书。

### 解决方案

```bash
# 合并证书链
cat www.example.com.crt bundle.crt > www.example.com.chained.crt
```

**注意顺序**：服务器证书必须排在中间证书之前。

### 验证证书链

```bash
openssl s_client -connect www.example.com:443
```

---

## 8. 单服务器 HTTP/HTTPS

```nginx
server {
    listen 80;
    listen 443 ssl;
    server_name www.example.com;

    ssl_certificate     www.example.com.crt;
    ssl_certificate_key www.example.com.key;
}
```

### HTTP 重定向到 HTTPS

```nginx
server {
    listen 80;
    server_name www.example.com;
    return 301 https://$server_name$request_uri;
}

server {
    listen 443 ssl;
    server_name www.example.com;
    # ...
}
```

---

## 9. 多域名 HTTPS（SNI）

### 问题

在单个 IP 上配置多个 HTTPS 服务器时，SSL 握手发生在 HTTP 请求之前，默认返回默认服务器证书。

### 解决方案

**方案一：SNI（Server Name Indication）**

```nginx
server {
    listen 443 ssl;
    server_name www.example.com;
    ssl_certificate www.example.com.crt;
    ssl_certificate_key www.example.com.key;
}

server {
    listen 443 ssl;
    server_name www.example.org;
    ssl_certificate www.example.org.crt;
    ssl_certificate_key www.example.org.key;
}
```

**要求**：
- OpenSSL 0.9.8f+（启用 `--enable-tlsext`）
- 0.9.8j+ 默认启用
- 大多数现代浏览器支持

**验证 SNI 支持**：
```bash
nginx -V | grep "TLS SNI support enabled"
```

**方案二：多名称证书**

使用 SubjectAltName 或通配符证书：

```nginx
# 通配符证书（仅匹配一级子域名）
ssl_certificate *.example.com.crt;

# SubjectAltName 证书（多个域名）
# 包含 example.com 和 example.org
```

**方案三：共享证书**

```nginx
http {
    ssl_certificate     shared.crt;
    ssl_certificate_key shared.key;

    server {
        listen 443 ssl;
        server_name www.example.com;
    }

    server {
        listen 443 ssl;
        server_name www.example.org;
    }
}
```

---

## 10. HTTP/2 配置

### 启用 HTTP/2

```nginx
server {
    listen 443 ssl http2;
    server_name www.example.com;

    ssl_certificate     www.example.com.crt;
    ssl_certificate_key www.example.com.key;
}
```

### HTTP/2 推送

```nginx
http2_push /style.css;
http2_push /image.png;

# 条件推送
http2_push_preload on;
# 配合 Link 响应头：Link: </style.css>; rel=preload; as=style
```

---

## 11. HTTP/3 (QUIC) 配置

### 版本要求

NGINX 1.25.0+，需编译 `--with-http_v3_module`。

### SSL 库要求

推荐 OpenSSL 3.5.1+，支持 early data。可选 BoringSSL、LibreSSL、QuicTLS。

### 配置示例

```nginx
server {
    listen 443 quic reuseport;
    listen 443 ssl;
    server_name www.example.com;

    ssl_certificate     www.example.com.crt;
    ssl_certificate_key www.example.com.key;

    ssl_protocols TLSv1.3;            # HTTP/3 需要 TLSv1.3

    quic_retry on;                    # 地址验证
    quic_gso on;                      # Generic Segmentation Offloading
    quic_host_key /path/to/key;       # Token 密钥

    ssl_early_data on;                # 0-RTT
    add_header Alt-Svc 'h3=":443"; ma=86400';  # 声明 HTTP/3 支持
}
```

---

## 12. 安全头部配置

```nginx
# HSTS（强制 HTTPS）
add_header Strict-Transport-Security "max-age=31536000; includeSubDomains; preload" always;

# 防止 MIME 类型嗅探
add_header X-Content-Type-Options "nosniff" always;

# XSS 保护
add_header X-XSS-Protection "1; mode=block" always;

# 禁止 iframe 嵌入
add_header X-Frame-Options "SAMEORIGIN" always;

# CSP
add_header Content-Security-Policy "default-src 'self'; script-src 'self' 'unsafe-inline'" always;
```

---

## 13. 内置变量

| 变量 | 说明 |
|------|------|
| `$ssl_protocol` | 建立的 SSL 协议版本 |
| `$ssl_cipher` | 当前连接使用的加密套件 |
| `$ssl_ciphers` | 客户端支持的加密套件列表 |
| `$ssl_client_cert` | 客户端证书（PEM 格式） |
| `$ssl_client_fingerprint` | 客户端证书 SHA1 指纹 |
| `$ssl_client_s_dn` | 客户端证书 Subject DN |
| `$ssl_client_i_dn` | 客户端证书 Issuer DN |
| `$ssl_client_serial` | 客户端证书序列号 |
| `$ssl_client_verify` | 验证结果（SUCCESS/FAILED/NONE） |
| `$ssl_server_name` | SNI 请求的服务器名称 |
| `$ssl_session_id` | 会话 ID |
| `$ssl_session_reused` | 会话是否复用（r 或 .） |
| `$ssl_early_data` | 是否使用 TLS 1.3 early data |

---

## 14. 配置检查与测试

### 检查配置

```bash
nginx -t
```

### 测试 SSL 配置

```bash
# 检查证书
openssl s_client -connect www.example.com:443 -servername www.example.com

# 检查协议支持
openssl s_client -connect www.example.com:443 -tls1_2
openssl s_client -connect www.example.com:443 -tls1_3

# 检查加密套件
nmap --script ssl-enum-ciphers -p 443 www.example.com

# 使用 testssl.sh 工具
testssl.sh www.example.com
```

### 在线测试工具

- SSL Labs: https://www.ssllabs.com/ssltest/
- SSL Checker: https://www.sslshopper.com/ssl-checker.html