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

---

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