# NGINX 性能优化指南

## 1. Worker 进程配置

### worker_processes

设置工作进程数量：

```nginx
worker_processes auto;    # 自动匹配 CPU 核心数（推荐）
worker_processes 4;       # 固定 4 个进程
```

### worker_cpu_affinity

绑定 worker 到特定 CPU 核心：

```nginx
worker_processes 4;
worker_cpu_affinity 0001 0010 0100 1000;  # 4 核绑定
```

### worker_connections

每个 worker 的最大连接数：

```nginx
events {
    worker_connections 10240;  # 默认 512
}
```

### worker_rlimit_nofile

worker 进程最大打开文件数：

```nginx
worker_rlimit_nofile 100000;
```

---

## 2. 事件处理优化

### events 块配置

```nginx
events {
    worker_connections 10240;     # 每个 worker 连接数
    use epoll;                    # Linux 使用 epoll
    multi_accept on;              # 一次接受所有连接
    accept_mutex off;             # 高流量时关闭互斥锁
}
```

### 连接处理方法

| 平台 | 方法 | 说明 |
|------|------|------|
| Linux | `epoll` | 高效（推荐） |
| FreeBSD/macOS | `kqueue` | 高效 |
| Solaris | `/dev/poll` | 高效 |
| 通用 | `select/poll` | 标准（效率低） |

---

## 3. HTTP 优化

### sendfile

使用内核级文件传输：

```nginx
sendfile on;
```

### tcp_nopush / tcp_nodelay

```nginx
tcp_nopush on;     # sendfile 时发送完整数据包
tcp_nodelay on;    # 减少网络延迟
```

### keepalive 配置

```nginx
http {
    keepalive_timeout 65s;
    keepalive_requests 10000;
}
```

### 长连接到上游

```nginx
upstream backend {
    server 192.168.1.1:8080;
    keepalive 32;              # 保持 32 个空闲连接
    keepalive_timeout 60s;
    keepalive_requests 1000;
}

server {
    location / {
        proxy_pass http://backend;
        proxy_http_version 1.1;
        proxy_set_header Connection "";
    }
}
```

---

## 4. 缓冲配置

### 响应缓冲

```nginx
http {
    # 客户端缓冲
    client_body_buffer_size 16k;
    client_header_buffer_size 1k;
    client_max_body_size 10m;

    # 代理缓冲
    proxy_buffering on;
    proxy_buffer_size 4k;
    proxy_buffers 8 16k;
    proxy_busy_buffers_size 32k;
}
```

### FastCGI 缓冲

```nginx
fastcgi_buffering on;
fastcgi_buffer_size 16k;
fastcgi_buffers 16 16k;
```

---

## 5. 文件缓存

### open_file_cache

缓存打开的文件描述符：

```nginx
http {
    open_file_cache max=10000 inactive=20s;
    open_file_cache_valid 30s;
    open_file_cache_min_uses 2;
    open_file_cache_errors on;
}
```

### 静态文件缓存

```nginx
location ~* \.(jpg|jpeg|png|gif|ico|css|js|pdf)$ {
    expires 30d;
    add_header Cache-Control "public, immutable";
    open_file_cache max=1000 inactive=30s;
}
```

---

## 6. Gzip 压缩优化

```nginx
http {
    gzip on;
    gzip_comp_level 6;              # 压缩级别（1-9）
    gzip_min_length 1000;           # 最小压缩长度
    gzip_proxied any;               # 代理请求也压缩
    gzip_vary on;                   # 添加 Vary 头

    gzip_types
        text/plain
        text/css
        text/xml
        text/javascript
        application/json
        application/javascript
        application/xml;

    gzip_buffers 16 8k;
    gzip_disable "msie6";
}
```

### 预压缩文件

```nginx
location ~* \.(css|js)$ {
    gzip_static on;
    expires 30d;
}
```

---

## 7. 连接优化

### 连接复用

```nginx
upstream backend {
    server 192.168.1.1:8080;
    keepalive 64;
    keepalive_timeout 60s;
    keepalive_requests 10000;
}
```

### HTTP/2

```nginx
server {
    listen 443 ssl http2;
    # ...
}
```

### HTTP/3 (QUIC)

```nginx
server {
    listen 443 quic reuseport;
    listen 443 ssl;
    add_header Alt-Svc 'h3=":443"; ma=86400';
    # ...
}
```

---

## 8. SSL/TLS 优化

### 会话缓存

```nginx
ssl_session_cache shared:SSL:10m;
ssl_session_timeout 1h;
ssl_session_tickets on;
```

### OCSP Stapling

```nginx
ssl_stapling on;
ssl_stapling_verify on;
resolver 8.8.8.8 8.8.4.4 valid=300s;
```

### 缓冲区大小

```nginx
ssl_buffer_size 4k;    # 减少 TLS 记录大小，加快首字节
```

---

## 9. 代理优化

### 超时配置

```nginx
proxy_connect_timeout 5s;
proxy_send_timeout 30s;
proxy_read_timeout 30s;
```

### 缓冲优化

```nginx
proxy_buffering on;
proxy_buffer_size 8k;
proxy_buffers 8 32k;
proxy_busy_buffers_size 64k;
proxy_max_temp_file_size 1024m;
```

### 连接复用

```nginx
location / {
    proxy_pass http://backend;
    proxy_http_version 1.1;
    proxy_set_header Connection "";
}
```

---

## 10. 内核参数优化

### /etc/sysctl.conf

```bash
# 网络优化
net.core.somaxconn = 65535
net.core.netdev_max_backlog = 65535
net.ipv4.tcp_max_syn_backlog = 65535
net.ipv4.tcp_tw_reuse = 1
net.ipv4.tcp_fin_timeout = 30
net.ipv4.ip_local_port_range = 1024 65535

# 连接跟踪
net.netfilter.nf_conntrack_max = 1000000

# 文件描述符
fs.file-max = 1000000

# 应用配置
sysctl -p
```

### /etc/security/limits.conf

```bash
# 增加文件描述符限制
* soft nofile 100000
* hard nofile 100000
```

---

## 11. 监控与调优

### stub_status

```nginx
location /nginx_status {
    stub_status;
    allow 127.0.0.1;
    deny all;
}
```

### 日志分析

```nginx
log_format perf '$remote_addr [$time_local] "$request" '
                '$status $body_bytes_sent $request_time '
                '$upstream_connect_time $upstream_header_time $upstream_response_time';
```

### 性能指标

| 指标 | 说明 | 优化方向 |
|------|------|----------|
| QPS | 每秒请求数 | 增加 worker，优化配置 |
| 延迟 | 请求响应时间 | 减少缓冲，启用缓存 |
| 连接数 | 并发连接 | 增加 worker_connections |
| CPU | CPU 使用率 | 减少压缩级别，禁用日志 |
| 内存 | 内存使用 | 调整缓冲区大小 |

---

## 12. 负载测试

### 使用 ab (Apache Bench)

```bash
ab -n 10000 -c 100 http://example.com/
```

### 使用 wrk

```bash
wrk -t 12 -c 400 -d 30s http://example.com/
```

### 使用 hey

```bash
hey -n 10000 -c 100 http://example.com/
```

---

## 13. 配置示例

### 高性能 Web 服务器

```nginx
user nginx;
worker_processes auto;
worker_rlimit_nofile 100000;

events {
    worker_connections 4096;
    use epoll;
    multi_accept on;
}

http {
    sendfile on;
    tcp_nopush on;
    tcp_nodelay on;
    keepalive_timeout 65;
    keepalive_requests 10000;

    open_file_cache max=10000 inactive=20s;
    open_file_cache_valid 30s;

    gzip on;
    gzip_comp_level 6;
    gzip_min_length 1000;
    gzip_proxied any;
    gzip_vary on;
    gzip_types text/plain text/css application/json application/javascript text/xml;

    server {
        listen 80 backlog=65535;
        server_name example.com;

        location / {
            root /var/www/html;
            try_files $uri $uri/ =404;
        }

        location ~* \.(jpg|jpeg|png|gif|ico|css|js)$ {
            expires 30d;
            add_header Cache-Control "public, immutable";
        }
    }
}
```

### 高性能代理服务器

```nginx
user nginx;
worker_processes auto;
worker_rlimit_nofile 100000;

events {
    worker_connections 10000;
    use epoll;
    multi_accept on;
}

http {
    sendfile on;
    tcp_nopush on;
    tcp_nodelay on;
    keepalive_timeout 65;

    upstream backend {
        server 192.168.1.1:8080;
        server 192.168.1.2:8080;
        keepalive 64;
    }

    server {
        listen 80 backlog=65535;

        location / {
            proxy_pass http://backend;
            proxy_http_version 1.1;
            proxy_set_header Connection "";

            proxy_buffering on;
            proxy_buffer_size 8k;
            proxy_buffers 8 32k;

            proxy_cache main;
            proxy_cache_key $uri;
            proxy_cache_valid 200 10m;
        }
    }
}
```

---

## 14. 故障排查

### 高 CPU 使用率

1. 检查日志级别（避免 debug）
2. 减少 gzip 压缩级别
3. 检查正则表达式复杂度
4. 使用 `strace` 分析

### 高内存使用

1. 减小缓冲区大小
2. 限制连接数
3. 检查内存泄漏

### 连接超时

1. 增加超时时间
2. 检查后端服务器
3. 查看系统日志

### 性能分析工具

```bash
# 查看连接状态
ss -tn

# 查看 nginx 进程
ps aux | grep nginx

# 系统负载
top -p $(pgrep nginx | head -1)

# 网络统计
netstat -an | grep :80 | wc -l
```