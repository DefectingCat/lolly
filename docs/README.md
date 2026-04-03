# NGINX 文档汇总

本目录包含 NGINX 官方文档的深度总结，涵盖 NGINX 的所有常用功能。

## 文档索引

| 序号 | 文档 | 内容概述 |
|------|------|----------|
| 01 | [概述与基础指南](./01-nginx-overview.md) | NGINX 架构、配置结构、启动停止、信号控制、命令行参数 |
| 02 | [安装与构建指南](./02-nginx-installation.md) | Linux 包安装、源码编译、配置参数、依赖库、模块编译 |
| 03 | [HTTP 核心模块](./03-nginx-http-core.md) | server/location 配置、请求路由、文件服务、客户端控制、性能优化 |
| 04 | [反向代理与负载均衡](./04-nginx-proxy-loadbalancing.md) | proxy_pass、upstream、负载均衡算法、健康检查、缓存、WebSocket |
| 05 | [SSL/TLS 与 HTTPS](./05-nginx-ssl-https.md) | HTTPS 配置、SSL 指令、会话缓存、OCSP、SNI、HTTP/2、HTTP/3 |
| 06 | [URL 重写与请求处理](./06-nginx-rewrite.md) | rewrite、return、if、map、常用重写场景、最佳实践 |
| 07 | [压缩与缓存](./07-nginx-compression-caching.md) | Gzip 压缩、代理缓存、FastCGI 缓存、静态文件缓存 |
| 08 | [日志与监控](./08-nginx-logging-monitoring.md) | 访问日志、错误日志、日志格式、条件日志、stub_status、日志分析 |
| 09 | [安全与访问控制](./09-nginx-security.md) | IP 访问控制、基础认证、请求限制、安全头部、防盗链、WAF |
| 10 | [TCP/UDP Stream 模块](./10-nginx-stream-tcp-udp.md) | TCP/UDP 代理、负载均衡、SSL、PROXY 协议、速率限制 |
| 11 | [邮件代理模块](./11-nginx-mail-proxy.md) | IMAP/POP3/SMTP 代理、认证配置、SSL/TLS、认证服务器 |
| 12 | [性能优化](./12-nginx-performance-tuning.md) | Worker 配置、事件优化、连接复用、缓冲配置、内核参数 |
| 13 | [Git Commit 规范](./13-git-commit-guide.md) | Conventional Commits 格式、类型定义、范围划分、示例库 |
| 14 | [gRPC/uWSGI/SCGI 代理](./14-nginx-grpc-uwsgi.md) | gRPC 代理、uWSGI Python 部署、SCGI 通用代理、模块对比 |
| 15 | [高级特性](./15-nginx-advanced-features.md) | 内部重定向、错误处理、请求拦截、高级路由 |
| 16 | [内部重定向](./16-nginx-internal-redirect.md) | internal 指令、X-Accel-Redirect、try_files、命名 location |
| 17 | [镜像与切片](./17-nginx-mirror-slice.md) | mirror 请求镜像、slice 大文件切片、流量复制 |
| 18 | [Memcached 集成](./18-nginx-memcached.md) | memcached_pass、缓存加速、键值设计 |
| 19 | [HTTP 功能模块详解](./19-nginx-http-modules-detail.md) | access/auth/autoindex/geo/map/realip/referer/secure_link/ssi/sub 等 16 个模块 |
| 20 | [限流与连接控制](./20-nginx-rate-limiting.md) | limit_req 请求限流、limit_conn 连接限制、令牌桶算法、DDoS 防护 |
| 21 | [HTTP/2 与 HTTP/3](./21-nginx-http2-http3.md) | HTTP/2 多路复用、HTTP/3 QUIC、配置迁移、性能对比 |
| 22 | [第三方扩展模块](./22-nginx-third-party-modules.md) | NJS/Lua/Brotli/Cache Purge/Headers More/RTMP/Sticky 模块 |
| 23 | [特殊功能模块](./23-nginx-special-modules.md) | WebDAV/图像过滤/FLV/MP4/HLS 流媒体/XSLT 转换 |
| 24 | [核心与事件模块](./24-nginx-core-events.md) | worker_processes/events/epoll/kqueue/连接数计算 |
| 25 | [内置变量速查表](./25-nginx-variables-reference.md) | HTTP/Stream/SSL/Upstream 变量完整列表（150+个） |
| 26 | [Lua 模块深度指南](./26-nginx-lua-guide.md) | OpenResty、ngx_lua 指令、共享字典、cosocket API |
| 27 | [安全深度指南](./27-nginx-security-deep-dive.md) | WAF/ModSecurity、DDoS 防护、OWASP Top 10、安全头部 |
| 28 | [API 网关配置](./28-nginx-api-gateway.md) | API 路由设计、JWT 验证、限流配额、版本控制 |
| 29 | [动态配置与服务发现](./29-nginx-dynamic-config.md) | 动态 upstream、etcd/Consul、dyups、nginx-unit |

---

## 模块分类索引

### 核心模块
- [核心与事件模块](./24-nginx-core-events.md) - ngx_core_module, ngx_events_module
- [HTTP 核心模块](./03-nginx-http-core.md) - ngx_http_core_module

### 代理与负载均衡
- [反向代理与负载均衡](./04-nginx-proxy-loadbalancing.md) - ngx_http_proxy_module, ngx_http_upstream_module
- [gRPC/uWSGI/SCGI 代理](./14-nginx-grpc-uwsgi.md) - gRPC, Python, CGI 代理
- [TCP/UDP Stream 模块](./10-nginx-stream-tcp-udp.md) - ngx_stream_* 模块
- [邮件代理模块](./11-nginx-mail-proxy.md) - ngx_mail_* 模块

### 安全与访问控制
- [安全与访问控制](./09-nginx-security.md) - 综合安全配置
- [HTTP 功能模块详解](./19-nginx-http-modules-detail.md) - access/auth_basic/auth_request/referer/secure_link
- [限流与连接控制](./20-nginx-rate-limiting.md) - limit_req, limit_conn

### 性能与优化
- [性能优化](./12-nginx-performance-tuning.md) - Worker/事件/缓冲/内核参数
- [压缩与缓存](./07-nginx-compression-caching.md) - Gzip, proxy_cache, fastcgi_cache
- [HTTP/2 与 HTTP/3](./21-nginx-http2-http3.md) - HTTP/2, HTTP/3 QUIC

### 内容处理
- [URL 重写与请求处理](./06-nginx-rewrite.md) - rewrite, return, map
- [镜像与切片](./17-nginx-mirror-slice.md) - mirror, slice
- [特殊功能模块](./23-nginx-special-modules.md) - WebDAV, 图像过滤, 流媒体

### 扩展与第三方
- [第三方扩展模块](./22-nginx-third-party-modules.md) - NJS, Lua, Brotli, RTMP 等
- [Lua 模块深度指南](./26-nginx-lua-guide.md) - OpenResty、ngx_lua、cosocket

### 安全深度
- [安全与访问控制](./09-nginx-security.md) - 综合安全配置
- [安全深度指南](./27-nginx-security-deep-dive.md) - WAF、DDoS、OWASP

### API 与动态配置
- [API 网关配置](./28-nginx-api-gateway.md) - API 路由、JWT、限流配额
- [动态配置与服务发现](./29-nginx-dynamic-config.md) - 动态 upstream、etcd/Consul

### 参考手册
- [内置变量速查表](./25-nginx-variables-reference.md) - 150+ 个变量完整列表

## 快速参考

### 核心配置结构

```nginx
# 全局配置
worker_processes auto;
error_log /var/log/nginx/error.log;

events {
    worker_connections 10240;
}

http {
    # HTTP 全局配置
    include       mime.types;
    default_type  application/octet-stream;

    # 上游服务器
    upstream backend {
        server 192.168.1.1:8080;
        server 192.168.1.2:8080;
    }

    server {
        listen 80;
        server_name example.com;

        location / {
            proxy_pass http://backend;
        }
    }
}

stream {
    # TCP/UDP 代理
    server {
        listen 3306;
        proxy_pass mysql:3306;
    }
}

mail {
    # 邮件代理
    server {
        listen 25;
        protocol smtp;
    }
}
```

### 常用命令

```bash
# 测试配置
nginx -t

# 重载配置
nginx -s reload

# 优雅停止
nginx -s quit

# 查看版本和编译参数
nginx -V
```

### 性能优化要点

1. **Worker 进程**：`worker_processes auto`
2. **连接数**：`worker_connections 10240`
3. **文件传输**：`sendfile on`
4. **长连接**：`keepalive_timeout 65`
5. **压缩**：`gzip on; gzip_comp_level 6`
6. **缓存**：`open_file_cache` + `proxy_cache`
7. **SSL 优化**：`ssl_session_cache shared:SSL:10m`

### 安全配置要点

1. **隐藏版本**：`server_tokens off`
2. **安全协议**：`ssl_protocols TLSv1.2 TLSv1.3`
3. **安全头部**：HSTS、X-Frame-Options、CSP
4. **请求限制**：`limit_req` + `limit_conn`
5. **访问控制**：IP 白名单 + 基础认证

---

## 官方资源

- **官方网站**：https://nginx.org/
- **官方文档**：https://nginx.org/en/docs/
- **模块参考**：https://nginx.org/en/docs/ngx_core_module.html
- **FAQ**：https://nginx.org/en/docs/faq.html
- **Wiki**：https://wiki.nginx.org/

---

## 版本说明

- 文档基于 NGINX 官方文档整理
- 涵盖 NGINX 开源版主要功能
- 部分高级功能需要 NGINX Plus 商业版
- 建议使用 NGINX 1.24+ 版本以获得最新特性