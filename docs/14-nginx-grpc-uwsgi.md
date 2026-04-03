# Nginx gRPC、uWSGI 与 SCGI 代理模块详解

## 1. gRPC 代理模块 (ngx_http_grpc_module)

### 概述
- gRPC 是 Google 开源的高性能 RPC 框架
- 基于 HTTP/2 和 Protocol Buffers
- 支持双向流、多路复用

### 核心指令表格
| 指令 | 说明 | 默认值 |
|------|------|--------|
| grpc_pass | gRPC 服务器地址 | - |
| grpc_set_header | 设置请求头 | - |
| grpc_connect_timeout | 连接超时 | 60s |
| grpc_send_timeout | 发送超时 | 60s |
| grpc_read_timeout | 读取超时 | 60s |
| grpc_buffering | 缓冲开关 | on |
| grpc_buffer_size | 缓冲区大小 | 4k/8k |
| grpc_ssl_verify | SSL 验证 | off |
| grpc_ssl_server_name | SNI 支持 | off |

### 配置示例
```nginx
location /grpc/ {
    grpc_pass grpc://backend:50051;
    grpc_set_header X-Real-IP $remote_addr;
}
```

### 与 HTTP 代理对比
- 必须使用 HTTP/2
- 二进制协议
- 原生双向流

---

## 2. uWSGI 代理模块 (ngx_http_uwsgi_module)

### 概述
- 专为 Python/WSGI 应用设计
- 二进制协议，性能优于 FastCGI

### 核心指令表格
| 指令 | 说明 | 默认值 |
|------|------|--------|
| uwsgi_pass | uWSGI 服务器地址 | - |
| uwsgi_param | 传递参数 | - |
| uwsgi_modifier1 | 数据包修饰符1 | 0 |
| uwsgi_modifier2 | 数据包修饰符2 | 0 |
| uwsgi_connect_timeout | 连接超时 | 60s |
| uwsgi_read_timeout | 读取超时 | 60s |
| uwsgi_buffer_size | 缓冲区大小 | 4k/8k |

### Python 应用配置示例
```nginx
location / {
    include uwsgi_params;
    uwsgi_pass unix:/tmp/uwsgi.sock;
    uwsgi_param UWSGI_PYHOME /var/www/venv;
}
```

### 与 FastCGI 对比
| 特性 | uWSGI | FastCGI |
|------|-------|---------|
| 协议类型 | 二进制 | 二进制 |
| Python 支持 | 原生优化 | 需适配 |
| 性能 | 更高 | 较高 |
| 配置复杂度 | 简单 | 复杂 |
| 多语言支持 | 通用 | PHP 优先 |

---

## 3. SCGI 代理模块 (ngx_http_scgi_module)

### 概述
- 简单通用的 CGI 协议
- 纯文本协议头

### 核心指令
| 指令 | 说明 | 默认值 |
|------|------|--------|
| scgi_pass | SCGI 服务器地址 | - |
| scgi_param | 传递参数 | - |
| scgi_connect_timeout | 连接超时 | 60s |
| scgi_read_timeout | 读取超时 | 60s |
| scgi_buffer_size | 缓冲区大小 | 4k/8k |
| scgi_hide_header | 隐藏响应头 | - |
| scgi_pass_header | 透传响应头 | - |

### 配置示例
```nginx
location /scgi/ {
    include scgi_params;
    scgi_pass 127.0.0.1:4000;
    scgi_param SCRIPT_FILENAME /var/www/app.py;
}
```

### 最佳实践
- 适合简单的 CGI 应用
- 调试友好（纯文本头）
- 性能略低于 uWSGI/FastCGI

---

## 4. 三种代理对比

| 特性 | gRPC | uWSGI | SCGI | FastCGI |
|------|------|-------|------|---------|
| 协议 | HTTP/2 + Protobuf | 二进制 | 纯文本 | 二进制 |
| 语言支持 | 多语言 | Python 优先 | 通用 | PHP 优先 |
| 性能 | 最高 | 高 | 中 | 高 |
| 双向流 | ✅ | ❌ | ❌ | ❌ |
| 多路复用 | ✅ | ❌ | ❌ | ✅ |
| 配置复杂度 | 中等 | 低 | 低 | 高 |
| 适用场景 | 微服务 | Python Web | 简单 CGI | PHP Web |

---

## 5. 完整配置示例

### gRPC 微服务代理
```nginx
upstream grpc_backend {
    server grpc1:50051;
    server grpc2:50051;
}

server {
    listen 1443 ssl http2;

    ssl_certificate /etc/nginx/ssl/server.crt;
    ssl_certificate_key /etc/nginx/ssl/server.key;

    location /api/ {
        grpc_pass grpc://grpc_backend;
        grpc_set_header Host $host;
        grpc_set_header X-Real-IP $remote_addr;
        grpc_connect_timeout 30s;
        grpc_read_timeout 30s;
    }
}
```

### Django 应用 (uWSGI)
```nginx
server {
    listen 80;
    server_name django.example.com;

    location / {
        include uwsgi_params;
        uwsgi_pass unix:/run/uwsgi/app.sock;
        uwsgi_read_timeout 300s;
    }

    location /static/ {
        alias /var/www/django/static/;
    }
}
```

### SCGI 通用应用
```nginx
server {
    listen 80;
    server_name scgi.example.com;

    location / {
        include scgi_params;
        scgi_pass 127.0.0.1:4000;
        scgi_param PATH_INFO $fastcgi_script_name;
    }
}
```

---

## 6. 性能优化建议

### gRPC 优化
- 启用连接池（keepalive）
- 合理设置 HTTP/2 初始窗口大小
- 使用 SSL 会话缓存

### uWSGI 优化
- 使用 Unix Socket 而非 TCP（同机部署）
- 启用缓冲减少后端压力
- 合理设置工作进程数

### SCGI 优化
- 仅用于简单场景
- 考虑升级到 uWSGI 或 FastCGI 以获得更好性能
