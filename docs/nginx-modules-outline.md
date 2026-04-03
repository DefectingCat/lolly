# nginx 模块文档大纲

## 1. ngx_http_grpc_module (gRPC 代理)

### 1.1 模块概述

ngx_http_grpc_module 模块用于将请求代理到 gRPC 服务器。该模块自 nginx 1.13.10 版本开始提供，支持 HTTP/2 协议上的 gRPC 通信。

**主要特性：**
- 支持 gRPC-over-HTTP/2 协议
- 支持 gRPC 服务端流、客户端流和双向流
- 支持 SSL/TLS 加密连接
- 支持负载均衡和健康检查
- 支持请求头自定义和超时配置

### 1.2 核心指令表格

| 指令 | 语法 | 说明 | 默认值 | 上下文 |
|------|------|------|--------|--------|
| grpc_pass | `grpc_pass address;` | 设置 gRPC 服务器地址 | - | location, if in location |
| grpc_set_header | `grpc_set_header field value;` | 设置传递给 gRPC 服务器的请求头 | - | http, server, location |
| grpc_hide_header | `grpc_hide_header field;` | 隐藏 gRPC 响应中的指定头字段 | - | http, server, location |
| grpc_pass_header | `grpc_pass_header field;` | 允许传递隐藏的头字段 | - | http, server, location |
| grpc_buffer_size | `grpc_buffer_size size;` | 设置读取响应的缓冲区大小 | 4k/8k | http, server, location |
| grpc_buffering | `grpc_buffering on/off;` | 启用/禁用响应缓冲 | on | http, server, location |
| grpc_connect_timeout | `grpc_connect_timeout time;` | 设置与服务器建立连接的超时 | 60s | http, server, location |
| grpc_send_timeout | `grpc_send_timeout time;` | 设置向服务器发送请求的超时 | 60s | http, server, location |
| grpc_read_timeout | `grpc_read_timeout time;` | 设置读取服务器响应的超时 | 60s | http, server, location |
| grpc_socket_keepalive | `grpc_socket_keepalive on/off;` | 启用 TCP keepalive | off | http, server, location |
| grpc_ssl_certificate | `grpc_ssl_certificate file;` | 指定客户端 SSL 证书 | - | http, server |
| grpc_ssl_certificate_key | `grpc_ssl_certificate_key file;` | 指定客户端 SSL 证书密钥 | - | http, server |
| grpc_ssl_ciphers | `grpc_ssl_ciphers ciphers;` | 指定 SSL 加密算法 | DEFAULT | http, server |
| grpc_ssl_protocols | `grpc_ssl_protocols protocols;` | 指定 SSL 协议版本 | TLSv1 TLSv1.1 TLSv1.2 | http, server |
| grpc_ssl_verify | `grpc_ssl_verify on/off;` | 启用服务器证书验证 | off | http, server |

### 1.3 配置示例

**基本配置：**
```nginx
location / {
    grpc_pass grpc://localhost:50051;
}
```

**带 SSL 的配置：**
```nginx
location / {
    grpc_pass grpcs://grpc.example.com:443;
    grpc_ssl_certificate /etc/nginx/client.crt;
    grpc_ssl_certificate_key /etc/nginx/client.key;
    grpc_ssl_verify on;
    grpc_ssl_trusted_certificate /etc/nginx/ca.crt;
}
```

**自定义请求头：**
```nginx
location / {
    grpc_set_header Host $host;
    grpc_set_header X-Real-IP $remote_addr;
    grpc_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    grpc_set_header X-Forwarded-Proto $scheme;
    grpc_pass grpc://localhost:50051;
}
```

**超时和缓冲配置：**
```nginx
location / {
    grpc_pass grpc://localhost:50051;
    grpc_connect_timeout 5s;
    grpc_send_timeout 10s;
    grpc_read_timeout 30s;
    grpc_buffering off;
    grpc_buffer_size 16k;
}
```

**负载均衡配置：**
```nginx
upstream grpc_backend {
    server 127.0.0.1:50051 weight=5;
    server 127.0.0.1:50052;
    server 127.0.0.1:50053 backup;
    keepalive 32;
}

location / {
    grpc_pass grpc://grpc_backend;
    grpc_socket_keepalive on;
}
```

### 1.4 与普通 HTTP 代理的区别

| 特性 | gRPC 代理 | 普通 HTTP 代理 (proxy_pass) |
|------|-----------|------------------------------|
| 协议 | HTTP/2 必须 | HTTP/1.0, HTTP/1.1, HTTP/2 |
| 传输 | 二进制协议 (Protocol Buffers) | 文本协议 |
| 流支持 | 原生支持双向流 | 需要特殊配置 (chunked) |
| 连接复用 | 多路复用 (Multiplexing) | 连接池 |
| 头部传递 | 使用 grpc_set_header | 使用 proxy_set_header |
| 错误处理 | gRPC 状态码 | HTTP 状态码 |

### 1.5 应用场景

1. **微服务架构网关**：作为 gRPC 服务的统一入口
2. **SSL/TLS 终止**：对外提供 HTTPS，内部使用明文 gRPC
3. **负载均衡**：在多个 gRPC 服务实例间分发请求
4. **A/B 测试**：基于路由规则将流量导向不同版本的服务
5. **流量监控**：收集 gRPC 调用指标和日志

---

## 2. ngx_http_uwsgi_module (uWSGI 代理)

### 2.1 模块概述

ngx_http_uwsgi_module 模块用于将请求代理到 uwsgi 协议服务器，主要用于 Python WSGI 应用程序（如 Django、Flask）的部署。

**主要特性：**
- 专为 Python WSGI 应用设计
- 使用 uwsgi 协议（比 HTTP/FastCGI 更高效）
- 支持 Unix 域套接字和 TCP 套接字
- 支持参数传递和环境变量设置
- 支持缓冲、缓存和超时配置

### 2.2 核心指令表格

| 指令 | 语法 | 说明 | 默认值 | 上下文 |
|------|------|------|--------|--------|
| uwsgi_pass | `uwsgi_pass address;` | 设置 uwsgi 服务器地址 | - | location, if in location |
| uwsgi_param | `uwsgi_param parameter value [if_not_empty];` | 设置传递给 uWSGI 的参数 | - | http, server, location |
| uwsgi_modifier1 | `uwsgi_modifier1 number;` | 设置 uWSGI 数据包修饰符1 | 0 | http, server, location |
| uwsgi_modifier2 | `uwsgi_modifier2 number;` | 设置 uWSGI 数据包修饰符2 | 0 | http, server, location |
| uwsgi_bind | `uwsgi_bind address [transparent];` | 绑定到特定地址 | - | http, server, location |
| uwsgi_buffering | `uwsgi_buffering on/off;` | 启用/禁用响应缓冲 | on | http, server, location |
| uwsgi_buffer_size | `uwsgi_buffer_size size;` | 设置响应缓冲区大小 | 4k/8k | http, server, location |
| uwsgi_buffers | `uwsgi_buffers number size;` | 设置缓冲区数量和大小 | 8 4k/8k | http, server, location |
| uwsgi_busy_buffers_size | `uwsgi_busy_buffers_size size;` | 设置忙缓冲区大小 | 8k/16k | http, server, location |
| uwsgi_cache | `uwsgi_cache zone;` | 启用响应缓存 | - | http, server, location |
| uwsgi_cache_key | `uwsgi_cache_key string;` | 设置缓存键 | - | http, server, location |
| uwsgi_cache_valid | `uwsgi_cache_valid time;` | 设置缓存有效期 | - | http, server, location |
| uwsgi_connect_timeout | `uwsgi_connect_timeout time;` | 连接超时 | 60s | http, server, location |
| uwsgi_send_timeout | `uwsgi_send_timeout time;` | 发送超时 | 60s | http, server, location |
| uwsgi_read_timeout | `uwsgi_read_timeout time;` | 读取超时 | 60s | http, server, location |
| uwsgi_hide_header | `uwsgi_hide_header field;` | 隐藏响应头 | - | http, server, location |
| uwsgi_pass_header | `uwsgi_pass_header field;` | 允许传递隐藏头 | - | http, server, location |
| uwsgi_ignore_client_abort | `uwsgi_ignore_client_abort on/off;` | 忽略客户端中止 | off | http, server, location |
| uwsgi_intercept_errors | `uwsgi_intercept_errors on/off;` | 拦截错误码 | off | http, server, location |

### 2.3 配置示例

**基本 Django/Flask 配置：**
```nginx
location / {
    include uwsgi_params;
    uwsgi_pass unix:/run/uwsgi/app.sock;
}
```

**TCP 套接字配置：**
```nginx
location / {
    include uwsgi_params;
    uwsgi_pass 127.0.0.1:3031;
}
```

**自定义参数：**
```nginx
location / {
    uwsgi_param SCRIPT_NAME /myapp;
    uwsgi_param PATH_INFO $1;
    uwsgi_param QUERY_STRING $query_string;
    uwsgi_param REQUEST_METHOD $request_method;
    uwsgi_param CONTENT_TYPE $content_type;
    uwsgi_param CONTENT_LENGTH $content_length;
    uwsgi_param REQUEST_URI $request_uri;
    uwsgi_param DOCUMENT_ROOT $document_root;
    uwsgi_param SERVER_PROTOCOL $server_protocol;
    uwsgi_param HTTPS $https if_not_empty;
    uwsgi_param REMOTE_ADDR $remote_addr;
    uwsgi_param REMOTE_PORT $remote_port;
    uwsgi_param SERVER_PORT $server_port;
    uwsgi_param SERVER_NAME $server_name;
    uwsgi_pass unix:/run/uwsgi/app.sock;
}
```

**带缓存的配置：**
```nginx
http {
    uwsgi_cache_path /var/cache/nginx/uwsgi levels=1:2 keys_zone=uwsgi:10m max_size=1g;

    server {
        location / {
            uwsgi_cache uwsgi;
            uwsgi_cache_key $host$request_uri;
            uwsgi_cache_valid 200 10m;
            uwsgi_cache_valid 404 1m;
            include uwsgi_params;
            uwsgi_pass unix:/run/uwsgi/app.sock;
        }
    }
}
```

**负载均衡配置：**
```nginx
upstream uwsgi_backend {
    server 127.0.0.1:3031 weight=5;
    server 127.0.0.1:3032;
    server 127.0.0.1:3033 backup;
    keepalive 32;
}

location / {
    include uwsgi_params;
    uwsgi_pass uwsgi_backend;
}
```

### 2.4 与 FastCGI 的对比

| 特性 | uWSGI | FastCGI |
|------|-------|---------|
| 设计目标 | Python WSGI 专用 | 通用语言接口 |
| 协议开销 | 更低（二进制协议） | 较高（文本协议）|
| 性能 | 更高 | 较低 |
| 配置复杂度 | 简单 | 较复杂 |
| 语言支持 | 主要为 Python | PHP、Perl、Ruby 等 |
| 功能扩展 | 丰富的插件系统 | 有限 |
| 进程管理 | 内置多种模式 | 依赖外部管理器 |

### 2.5 应用场景

1. **Django 应用部署**：高性能 Python Web 框架部署
2. **Flask 应用部署**：轻量级 Python Web 框架部署
3. **Python API 服务**：RESTful API 后端服务
4. **机器学习模型服务**：部署 ML 模型推理服务
5. **数据科学应用**：Jupyter、Streamlit 等应用托管

---

## 3. ngx_http_scgi_module (SCGI 代理)

### 3.1 模块概述

ngx_http_scgi_module 模块用于将请求代理到 SCGI（Simple Common Gateway Interface）服务器。SCGI 是一种简化版的 CGI 协议，旨在提供比传统 CGI 更好的性能。

**主要特性：**
- 简化的 CGI 协议实现
- 比传统 CGI 更快的性能（保持持久连接）
- 支持 Unix 域套接字和 TCP 套接字
- 支持参数传递和环境变量设置
- 轻量级，适合小型项目

### 3.2 核心指令表格

| 指令 | 语法 | 说明 | 默认值 | 上下文 |
|------|------|------|--------|--------|
| scgi_pass | `scgi_pass address;` | 设置 SCGI 服务器地址 | - | location, if in location |
| scgi_param | `scgi_param parameter value [if_not_empty];` | 设置传递给 SCGI 的参数 | - | http, server, location |
| scgi_bind | `scgi_bind address [transparent];` | 绑定到特定地址 | - | http, server, location |
| scgi_buffering | `scgi_buffering on/off;` | 启用/禁用响应缓冲 | on | http, server, location |
| scgi_buffer_size | `scgi_buffer_size size;` | 设置响应缓冲区大小 | 4k/8k | http, server, location |
| scgi_buffers | `scgi_buffers number size;` | 设置缓冲区数量和大小 | 8 4k/8k | http, server, location |
| scgi_busy_buffers_size | `scgi_busy_buffers_size size;` | 设置忙缓冲区大小 | 8k/16k | http, server, location |
| scgi_cache | `scgi_cache zone;` | 启用响应缓存 | - | http, server, location |
| scgi_cache_key | `scgi_cache_key string;` | 设置缓存键 | - | http, server, location |
| scgi_cache_valid | `scgi_cache_valid time;` | 设置缓存有效期 | - | http, server, location |
| scgi_connect_timeout | `scgi_connect_timeout time;` | 连接超时 | 60s | http, server, location |
| scgi_send_timeout | `scgi_send_timeout time;` | 发送超时 | 60s | http, server, location |
| scgi_read_timeout | `scgi_read_timeout time;` | 读取超时 | 60s | http, server, location |
| scgi_hide_header | `scgi_hide_header field;` | 隐藏响应头 | - | http, server, location |
| scgi_pass_header | `scgi_pass_header field;` | 允许传递隐藏头 | - | http, server, location |
| scgi_ignore_client_abort | `scgi_ignore_client_abort on/off;` | 忽略客户端中止 | off | http, server, location |
| scgi_intercept_errors | `scgi_intercept_errors on/off;` | 拦截错误码 | off | http, server, location |
| scgi_pass_request_headers | `scgi_pass_request_headers on/off;` | 传递请求头 | on | http, server, location |
| scgi_pass_request_body | `scgi_pass_request_body on/off;` | 传递请求体 | on | http, server, location |

### 3.3 配置示例

**基本配置：**
```nginx
location / {
    include scgi_params;
    scgi_pass localhost:4000;
}
```

**Unix 域套接字配置：**
```nginx
location / {
    include scgi_params;
    scgi_pass unix:/tmp/scgi.sock;
}
```

**自定义参数：**
```nginx
location / {
    scgi_param SCRIPT_NAME /myapp;
    scgi_param PATH_INFO $1;
    scgi_param QUERY_STRING $query_string;
    scgi_param REQUEST_METHOD $request_method;
    scgi_param CONTENT_TYPE $content_type;
    scgi_param CONTENT_LENGTH $content_length;
    scgi_param REQUEST_URI $request_uri;
    scgi_param DOCUMENT_ROOT $document_root;
    scgi_param SERVER_PROTOCOL $server_protocol;
    scgi_param REMOTE_ADDR $remote_addr;
    scgi_param REMOTE_PORT $remote_port;
    scgi_param SERVER_PORT $server_port;
    scgi_param SERVER_NAME $server_name;
    scgi_pass localhost:4000;
}
```

**带缓存的配置：**
```nginx
http {
    scgi_cache_path /var/cache/nginx/scgi levels=1:2 keys_zone=scgi:10m max_size=1g;

    server {
        location / {
            scgi_cache scgi;
            scgi_cache_key $host$request_uri;
            scgi_cache_valid 200 5m;
            include scgi_params;
            scgi_pass localhost:4000;
        }
    }
}
```

### 3.4 与 FastCGI/uWSGI 的对比

| 特性 | SCGI | FastCGI | uWSGI |
|------|------|---------|-------|
| 协议复杂度 | 简单（纯文本）| 中等 | 复杂（二进制）|
| 性能 | 中等 | 中等 | 最高 |
| 资源占用 | 低 | 中等 | 中等 |
| 连接方式 | 持久连接 | 持久连接 | 持久连接 |
| 功能丰富度 | 基础功能 | 较丰富 | 最丰富 |
| 语言支持 | 通用 | PHP 为主 | Python 为主 |
| 实现难度 | 简单 | 中等 | 复杂 |
| 适用场景 | 小型项目 | 中型项目 | 大型生产环境 |

**SCGI vs FastCGI：**
- **SCGI**：协议更简单，实现更容易，适合轻量级应用
- **FastCGI**：更成熟，生态更好，PHP 应用首选

**SCGI vs uWSGI：**
- **SCGI**：通用协议，不绑定特定语言
- **uWSGI**：Python 专用，功能最丰富

### 3.5 应用场景

1. **小型 CGI 应用**：简单脚本语言的 Web 接口
2. **自定义语言运行环境**：为特定语言实现 SCGI 接口
3. **嵌入式系统**：资源受限环境下的轻量级方案
4. **原型开发**：快速验证 Web 应用概念
5. **学习目的**：理解 CGI 协议的简化实现

---

## 4. 三个模块对比总结

| 维度 | ngx_http_grpc_module | ngx_http_uwsgi_module | ngx_http_scgi_module |
|------|---------------------|----------------------|---------------------|
| **协议类型** | HTTP/2 (gRPC) | uwsgi (二进制) | SCGI (文本) |
| **主要语言** | 多语言 | Python | 多语言 |
| **性能** | 高（HTTP/2 多路复用）| 很高 | 中等 |
| **典型应用** | 微服务通信 | Django/Flask | CGI 脚本 |
| **配置复杂度** | 中等 | 简单 | 简单 |
| **功能特性** | 流、SSL、LB | 缓冲、缓存 | 基础代理 |
| **生态成熟度** | 快速增长 | 成熟（Python）| 小众 |

## 5. 选择建议

- **微服务/API 网关**：选择 **grpc_module**
- **Python Web 应用**：选择 **uwsgi_module**
- **简单 CGI/轻量级**：选择 **scgi_module** 或 **fastcgi_module**

---

*文档生成时间：2026-04-03*
