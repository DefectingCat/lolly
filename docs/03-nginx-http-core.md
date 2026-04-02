# NGINX HTTP 核心模块指南

## 1. 核心配置结构

### http 上下文

```nginx
http {
    # HTTP 服务全局配置
    include       mime.types;
    default_type  application/octet-stream;

    # 日志格式定义
    log_format main '$remote_addr - $remote_user [$time_local] "$request" '
                    '$status $body_bytes_sent "$http_referer" '
                    '"$http_user_agent"';

    server {
        # 虚拟服务器配置
    }
}
```

### server 虚拟服务器

```nginx
server {
    listen       80;
    server_name  example.com www.example.com;

    location / {
        root   /var/www/html;
        index  index.html index.htm;
    }
}
```

### location 请求路由

**语法**：`location [ = | ~ | ~* | ^~ ] uri { ... }`

| 修饰符 | 说明 |
|--------|------|
| `=` | 精确匹配，必须完全匹配 URI |
| `^~` | 前缀匹配，成功后不再检查正则 |
| `~` | 正则匹配（区分大小写） |
| `~*` | 正则匹配（不区分大小写） |
| `@` | 命名 location，用于内部重定向 |

**匹配优先级**：
1. 精确匹配 (`=`)
2. 前缀匹配 (`^~`) - 不再检查正则
3. 正则匹配 (按配置文件顺序)
4. 普通前缀匹配 (最长匹配优先)

```nginx
location = /exact {
    # 精确匹配 /exact
}

location ^~ /images/ {
    # 前缀匹配，优先于正则
}

location ~ \.php$ {
    # 正则匹配 .php 文件（区分大小写）
}

location ~* \.(gif|jpg|png)$ {
    # 正则匹配图片（不区分大小写）
}

location / {
    # 普通前缀匹配，作为默认
}

location @fallback {
    # 命名 location，用于内部重定向
    proxy_pass http://backend;
}
```

---

## 2. 监听与服务器名称

### listen 指令

**语法**：`listen address[:port] [parameters];`

```nginx
server {
    listen 80;
    listen 443 ssl;
    listen [::]:80;                    # IPv6
    listen unix:/var/run/nginx.sock;   # Unix Socket
    listen 80 default_server;          # 默认服务器
    listen 443 ssl http2;              # SSL + HTTP/2
}
```

**常用参数**：

| 参数 | 说明 |
|------|------|
| `default_server` | 设置为默认服务器 |
| `ssl` | 启用 SSL |
| `http2` | 启用 HTTP/2 |
| `http3` | 启用 HTTP/3 (QUIC) |
| `quic` | 启用 QUIC 协议 |
| `reuseport` | 每个 worker 独立监听 |
| `proxy_protocol` | 启用 PROXY 协议 |
| `backlog=N` | 连接队列长度 |
| `rcvbuf=N` | 接收缓冲区大小 |
| `sndbuf=N` | 发送缓冲区大小 |

### server_name 指令

```nginx
server {
    server_name example.com;                    # 精确名称
    server_name www.example.com;                # 多个精确名称
    server_name *.example.com;                  # 通配符前缀
    server_name example.*;                      # 通配符后缀
    server_name ~^(www\.)?(.+)$;                # 正则表达式
    server_name .example.com;                   # 匹配 example.com 和 *.example.com
    server_name "";                             # 匹配空 Host 头（默认）
    server_name _;                              # catch-all（无效域名）
}
```

**匹配优先级**：
1. 精确名称
2. 最长的以 `*` 开头的通配符
3. 最长的以 `*` 结尾的通配符
4. 第一个匹配的正则表达式（按配置顺序）

---

## 3. 请求处理流程

### 服务器选择流程

1. 根据 IP 地址和端口匹配 `listen` 指令
2. 在匹配的 server 块中测试 `Host` 头是否匹配 `server_name`
3. 如果未找到匹配的服务器名称，使用该 IP:Port 的默认服务器

### Location 选择流程

1. 首先搜索最具体的字面字符串前缀位置
2. 按配置文件顺序检查正则表达式位置
3. 第一个匹配的正则表达式停止搜索并被使用
4. 如果没有正则匹配，使用之前找到的最具体前缀位置

**注意**：所有类型的 location 仅测试请求行中的 URI 部分，不包含查询参数。

---

## 4. 文件服务指令

### root 指令

设置请求的根目录，路径构造为 `root 值 + URI`。

```nginx
location /i/ {
    root /data/w3;
    # 请求 /i/top.gif -> /data/w3/i/top.gif
}
```

### alias 指令

替换指定 location 的路径。

```nginx
location /i/ {
    alias /data/w3/images/;
    # 请求 /i/top.gif -> /data/w3/images/top.gif
}
```

**root 与 alias 区别**：

```nginx
# root：URI 附加到路径后
location /images/ {
    root /data;  # /images/logo.png -> /data/images/logo.png
}

# alias：URI 替换 location 部分
location /images/ {
    alias /data/photos/;  # /images/logo.png -> /data/photos/logo.png
}
```

### index 指令

定义索引文件。

```nginx
location / {
    index index.html index.htm index.php;
}
```

### try_files 指令

按顺序检查文件是否存在。

```nginx
location / {
    try_files $uri $uri/ /index.html =404;
    # 1. 尝试 $uri（精确文件）
    # 2. 尝试 $uri/（目录）
    # 3. 尔回 /index.html（内部重定向）
    # 4. 最后返回 404
}

# 配合命名 location
location / {
    try_files $uri $uri/ @fallback;
}

location @fallback {
    proxy_pass http://backend;
}
```

### error_page 指令

定义错误页面。

```nginx
error_page 404 /404.html;
error_page 500 502 503 504 /50x.html;
error_page 404 =200 /empty.gif;  # 修改响应码为 200
```

---

## 5. 客户端请求控制

### client_max_body_size

设置请求体最大允许大小。

```nginx
client_max_body_size 10m;   # 默认 1m
client_max_body_size 0;     # 禁用检查（不限制）
```

### client_body_timeout / client_header_timeout

读取请求体/头部超时。

```nginx
client_body_timeout 60s;    # 默认 60s
client_header_timeout 60s;  # 默认 60s
```

### keepalive_timeout

设置长连接超时时间。

```nginx
keepalive_timeout 75s;      # 默认 75s
keepalive_timeout 75s 70s;  # 第二个参数设置响应头 Keep-Alive
```

### keepalive_requests

单个长连接最大请求数。

```nginx
keepalive_requests 1000;    # 默认 1000
```

---

## 6. 性能优化指令

### sendfile

启用高效的文件传输。

```nginx
sendfile on;                # 默认 off
```

### tcp_nopush / tcp_nodelay

```nginx
tcp_nopush on;              # sendfile 时优化（发送完整数据包）
tcp_nodelay on;             # 减少网络延迟（不等待完整数据包）
```

### aio

启用异步文件 I/O。

```nginx
location /video/ {
    sendfile on;
    aio threads;            # 使用线程池
    directio 8m;            # 大于 8MB 的文件使用直接 I/O
}
```

### open_file_cache

缓存打开的文件描述符。

```nginx
open_file_cache max=1000 inactive=20s;
open_file_cache_valid 30s;
open_file_cache_min_uses 2;
open_file_cache_errors on;
```

### server_tokens

隐藏版本信息。

```nginx
server_tokens off;          # 错误页和响应头不显示版本
```

---

## 7. MIME 类型配置

### types 指令

映射文件扩展名到 MIME 类型。

```nginx
types {
    text/html    html htm shtml;
    text/css     css;
    text/xml     xml;
    image/gif    gif;
    image/jpeg   jpeg jpg;
    application/javascript js;
    application/json json;
}
```

### default_type

设置默认 MIME 类型。

```nginx
default_type application/octet-stream;
```

---

## 8. 内部请求控制

### internal

指定 location 仅用于内部请求。

```nginx
location /internal/ {
    internal;               # 外部请求返回 404
    proxy_pass http://backend;
}

# 内部请求触发方式：
# 1. error_page 重定向
# 2. try_files 最后一个参数
# 3. rewrite ... last 重定向到命名 location
# 4. X-Accel-Redirect 响应头
```

### limit_rate

限制响应传输速率。

```nginx
limit_rate 100k;            # 限制为 100KB/s
limit_rate_after 1m;        # 传输 1MB 后开始限速
```

---

## 9. 常用嵌入式变量

| 变量名 | 说明 |
|--------|------|
| `$arg_name` | 请求行中的参数 name |
| `$args` | 请求行中的所有参数 |
| `$host` | 请求行中的主机名或 Host 头 |
| `$http_name` | 任意请求头字段（小写，下划线） |
| `$remote_addr` | 客户端 IP 地址 |
| `$remote_port` | 客户端端口 |
| `$remote_user` | 基础认证用户名 |
| `$request` | 完整原始请求行 |
| `$request_body` | 请求体内容 |
| `$request_filename` | 当前请求的文件路径 |
| `$request_method` | 请求方法（GET, POST 等） |
| `$request_uri` | 完整原始请求 URI（含参数） |
| `$scheme` | 请求协议（http/https） |
| `$server_addr` | 服务器 IP 地址 |
| `$server_name` | 服务器名称 |
| `$server_port` | 服务器端口 |
| `$status` | 响应状态码 |
| `$uri` | 当前请求中的 URI（规范化后） |
| `$document_root` | root 指令的值 |
| `$document_uri` | 同 $uri |
| `$query_string` | 同 $args |

### 示例

```nginx
# 获取查询参数
location /search {
    set $q $arg_q;          # ?q=value 中的 value
    proxy_pass http://backend/search?q=$q;
}

# 获取请求头
location /api {
    proxy_set_header X-Auth $http_x_auth;
    proxy_pass http://backend;
}

# 获取客户端 IP
location /ip {
    return 200 "Your IP: $remote_addr";
}
```