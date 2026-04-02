# NGINX 邮件代理模块指南

## 1. 邮件代理概述

NGINX 可以作为邮件代理服务器，支持：
- **IMAP**：Internet Message Access Protocol
- **POP3**：Post Office Protocol version 3
- **SMTP**：Simple Mail Transfer Protocol

### 版本要求

默认不构建，需编译时添加 `--with-mail` 参数。

---

## 2. 基础配置示例

```nginx
worker_processes auto;

mail {
    server_name mail.example.com;

    # 认证服务器
    auth_http localhost:9000/cgi-bin/nginxauth.cgi;

    # 协议能力配置
    imap_capabilities IMAP4rev1 UIDPLUS IDLE LITERAL+ QUOTA;
    pop3_auth plain apop cram-md5;
    pop3_capabilities LAST TOP USER PIPELINING UIDL;
    smtp_auth login plain cram-md5;
    smtp_capabilities "SIZE 10485760" ENHANCEDSTATUSCODES 8BITMIME DSN;

    # IMAP 服务
    server {
        listen 143;
        protocol imap;
    }

    # POP3 服务
    server {
        listen 110;
        protocol pop3;
        proxy_pass_error_message on;
    }

    # SMTP 服务
    server {
        listen 25;
        protocol smtp;
    }

    # SMTP 提交端口
    server {
        listen 587;
        protocol smtp;
    }

    # IMAPS（SSL）
    server {
        listen 993 ssl;
        protocol imap;
        ssl_certificate     /path/to/cert.pem;
        ssl_certificate_key /path/to/key.pem;
    }

    # POP3S（SSL）
    server {
        listen 995 ssl;
        protocol pop3;
        ssl_certificate     /path/to/cert.pem;
        ssl_certificate_key /path/to/key.pem;
    }

    # SMTPS（SSL）
    server {
        listen 465 ssl;
        protocol smtp;
        ssl_certificate     /path/to/cert.pem;
        ssl_certificate_key /path/to/key.pem;
    }
}
```

---

## 3. 核心指令

### mail 上下文

```nginx
mail {
    # 邮件代理配置
}
```

### server 块

```nginx
server {
    listen 143;
    protocol imap;
}
```

### listen 指令

```nginx
server {
    listen 25;              # SMTP
    listen 110;             # POP3
    listen 143;             # IMAP
    listen 465 ssl;         # SMTPS
    listen 587;             # SMTP Submission
    listen 993 ssl;         # IMAPS
    listen 995 ssl;         # POP3S
}
```

**支持的参数**：
- `ssl`：启用 SSL
- `proxy_protocol`：启用 PROXY 协议
- `backlog=N`：连接队列长度
- `so_keepalive`：TCP keepalive

### protocol 指令

设置代理协议：

```nginx
protocol imap;
protocol pop3;
protocol smtp;
```

**自动检测**：若未设置，根据端口自动检测：

| 端口 | 协议 |
|------|------|
| 143, 993 | IMAP |
| 110, 995 | POP3 |
| 25, 587, 465 | SMTP |

### server_name 指令

```nginx
server_name mail.example.com;
```

用于：
- POP3/SMTP 问候
- SASL CRAM-MD5 盐值
- SMTP 后端的 EHLO 命令

---

## 4. 认证配置

### auth_http 指令

指定认证服务器 URL：

```nginx
auth_http http://auth.example.com/validate;
auth_http localhost:9000/cgi-bin/nginxauth.cgi;
```

### 认证服务器协议

NGINX 发送以下请求头给认证服务器：

```
GET /validate HTTP/1.0
Host: auth.example.com
Auth-Method: plain
Auth-User: user@example.com
Auth-Pass: password
Auth-Protocol: imap
Auth-Login-Attempt: 1
Client-IP: 192.168.1.100
```

认证服务器响应：

**认证成功**：
```
HTTP/1.0 200 OK
Auth-Status: OK
Auth-Server: 192.168.1.10
Auth-Port: 143
```

**认证失败**：
```
HTTP/1.0 200 OK
Auth-Status: Invalid login or password
```

### 认证方法

```nginx
# POP3 认证方法
pop3_auth plain apop cram-md5;

# SMTP 认证方法
smtp_auth login plain cram-md5;

# IMAP 认证方法（仅 plain）
# IMAP 只支持 AUTH=PLAIN
```

---

## 5. 协议能力

### IMAP 能力

```nginx
imap_capabilities IMAP4rev1 UIDPLUS IDLE LITERAL+ QUOTA;
```

### POP3 能力

```nginx
pop3_capabilities LAST TOP USER PIPELINING UIDL;
```

### SMTP 能力

```nginx
smtp_capabilities "SIZE 10485760" ENHANCEDSTATUSCODES 8BITMIME DSN;
```

---

## 6. SSL/TLS 配置

### 服务端 SSL

```nginx
server {
    listen 993 ssl;
    protocol imap;

    ssl_certificate     /path/to/cert.pem;
    ssl_certificate_key /path/to/key.pem;
    ssl_protocols       TLSv1.2 TLSv1.3;
    ssl_ciphers         HIGH:!aNULL:!MD5;
}
```

### STARTTLS

```nginx
server {
    listen 143;
    protocol imap;
    starttls on;          # 允许 STARTTLS
}

server {
    listen 587;
    protocol smtp;
    starttls on;
}
```

**starttls 选项**：
- `on`：允许 STARTTLS
- `only`：仅允许 STARTTLS 连接
- `off`：禁用 STARTTLS

### SSL 指令

| 指令 | 说明 |
|------|------|
| `ssl_certificate` | 证书文件 |
| `ssl_certificate_key` | 私钥文件 |
| `ssl_protocols` | 启用的协议 |
| `ssl_ciphers` | 加密套件 |
| `ssl_prefer_server_ciphers` | 服务器套件优先 |
| `ssl_session_cache` | 会话缓存 |
| `ssl_session_timeout` | 会话超时 |

---

## 7. 代理配置

### proxy_timeout

设置开始代理到后端之前的超时时间：

```nginx
proxy_timeout 60s;     # 默认 60s
```

### proxy_pass_error_message

向后端传递错误消息：

```nginx
proxy_pass_error_message on;
```

### xclient

SMTP XCLIENT 命令配置：

```nginx
xclient on;    # 启用 XCLIENT（默认 on）
xclient off;   # 禁用 XCLIENT
```

---

## 8. DNS 配置

### resolver 指令

配置 DNS 服务器：

```nginx
resolver 8.8.8.8 8.8.4.4 valid=300s;
resolver_timeout 30s;
```

---

## 9. 访问控制

### IP 访问控制

```nginx
server {
    listen 25;
    protocol smtp;

    allow 192.168.0.0/16;
    allow 10.0.0.0/8;
    deny all;
}
```

---

## 10. 完整配置示例

### 企业邮件代理

```nginx
worker_processes auto;

mail {
    server_name mail.example.com;

    # 认证服务器
    auth_http http://auth.example.com/mail/auth;

    # DNS
    resolver 8.8.8.8 8.8.4.4 valid=300s;

    # 协议能力
    imap_capabilities IMAP4rev1 UIDPLUS IDLE LITERAL+ QUOTA;
    pop3_capabilities LAST TOP USER PIPELINING UIDL;
    smtp_capabilities "SIZE 52428800" ENHANCEDSTATUSCODES 8BITMIME DSN;

    # 认证方法
    pop3_auth plain apop cram-md5;
    smtp_auth login plain cram-md5;

    # IMAP
    server {
        listen 143;
        protocol imap;
        starttls on;
        proxy_timeout 600s;
    }

    # IMAPS
    server {
        listen 993 ssl;
        protocol imap;
        ssl_certificate     /etc/nginx/ssl/mail.crt;
        ssl_certificate_key /etc/nginx/ssl/mail.key;
        ssl_protocols       TLSv1.2 TLSv1.3;
        proxy_timeout 600s;
    }

    # POP3
    server {
        listen 110;
        protocol pop3;
        starttls on;
        proxy_timeout 600s;
    }

    # POP3S
    server {
        listen 995 ssl;
        protocol pop3;
        ssl_certificate     /etc/nginx/ssl/mail.crt;
        ssl_certificate_key /etc/nginx/ssl/mail.key;
        ssl_protocols       TLSv1.2 TLSv1.3;
        proxy_timeout 600s;
    }

    # SMTP
    server {
        listen 25;
        protocol smtp;
        starttls on;
        xclient on;
    }

    # SMTP Submission
    server {
        listen 587;
        protocol smtp;
        starttls on;
    }

    # SMTPS
    server {
        listen 465 ssl;
        protocol smtp;
        ssl_certificate     /etc/nginx/ssl/mail.crt;
        ssl_certificate_key /etc/nginx/ssl/mail.key;
        ssl_protocols       TLSv1.2 TLSv1.3;
    }
}
```

---

## 11. 认证服务器实现示例

### Python Flask 示例

```python
from flask import Flask, request, Response

app = Flask(__name__)

@app.route('/mail/auth', methods=['GET', 'POST'])
def mail_auth():
    auth_user = request.headers.get('Auth-User', '')
    auth_pass = request.headers.get('Auth-Pass', '')
    auth_protocol = request.headers.get('Auth-Protocol', '')
    client_ip = request.headers.get('Client-IP', '')

    # 验证用户
    if validate_user(auth_user, auth_pass):
        # 返回后端服务器
        response = Response()
        response.headers['Auth-Status'] = 'OK'
        response.headers['Auth-Server'] = '192.168.1.10'
        response.headers['Auth-Port'] = get_backend_port(auth_protocol)
        return response
    else:
        response = Response()
        response.headers['Auth-Status'] = 'Invalid login or password'
        return response

def validate_user(username, password):
    # 实现用户验证逻辑
    return True

def get_backend_port(protocol):
    ports = {
        'imap': '143',
        'pop3': '110',
        'smtp': '25'
    }
    return ports.get(protocol, '143')

if __name__ == '__main__':
    app.run(port=9000)
```

---

## 12. 故障排查

### 日志配置

```nginx
mail {
    error_log /var/log/nginx/mail_error.log debug;

    server {
        listen 143;
        protocol imap;
    }
}
```

### 常见问题

**认证失败**：
- 检查 auth_http URL 是否正确
- 验证认证服务器响应格式
- 查看 auth 服务日志

**连接超时**：
- 增加 proxy_timeout 值
- 检查后端服务器状态
- 验证网络连通性

**SSL 问题**：
- 检查证书文件权限
- 验证证书链完整性
- 确认协议版本匹配

---

## 13. 禁用特定协议

编译时可禁用不需要的协议：

```bash
./configure --with-mail \
    --without-mail_pop3_module \
    --without-mail_imap_module \
    --without-mail_smtp_module
```