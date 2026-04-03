# NGINX 第三方扩展模块详解

## 概述

第三方模块极大地扩展了 NGINX 的功能边界，使其能够处理各种复杂场景。本文档详细介绍常用的第三方扩展模块及其配置方法。

---

## 1. NJS (nginx JavaScript) 模块

NJS 是 NGINX 官方提供的 JavaScript 解释器，允许在 NGINX 中使用 JavaScript 编写动态逻辑。

### 核心指令

#### `js_import`

导入 JavaScript 文件。

```nginx
js_import /path/to/script.js;                    # 导入整个模块
js_import main from /path/to/script.js;          # 命名导入
```

#### `js_set`

设置 NGINX 变量为 JavaScript 函数返回值。

```nginx
js_set $auth_token validate_auth;                # 变量值由 validate_auth 函数返回
```

#### `js_content`

使用 JavaScript 生成响应内容。

```nginx
location /api/custom {
    js_content custom_response;
}
```

#### `js_body_filter`

使用 JavaScript 修改响应体。

```nginx
location / {
    js_body_filter modify_body;
    proxy_pass http://backend;
}
```

### JavaScript 语法支持

NJS 基于 ECMAScript 5.1，支持部分 ES6 特性：

| 特性 | 支持情况 | 示例 |
|------|----------|------|
| 基础类型 | 完整 | `string`, `number`, `boolean`, `null` |
| 函数 | 完整 | `function foo() {}` |
| 对象 | 完整 | `var obj = { a: 1 };` |
| 数组 | 完整 | `var arr = [1, 2, 3];` |
| let/const | 支持 | `let x = 1; const y = 2;` |
| 箭头函数 | 支持 | `(x) => x * 2` |
| 模板字符串 | 支持 | `` `Hello ${name}` `` |
| Promise | 部分 | `Promise.then()` 和 `catch()` |
| async/await | 部分 | `async function` |
| 类 (class) | 不支持 | - |
| 解构赋值 | 部分 | `const {a, b} = obj` |

### NGINX JavaScript 对象

```javascript
// r: 请求对象
function handler(r) {
    // 请求信息
    r.method;           // HTTP 方法
    r.uri;              // 请求 URI
    r.args;             // 查询参数对象
    r.headersIn;        // 请求头对象
    r.headersOut;       // 响应头对象
    r.remoteAddress;    // 客户端 IP

    // 响应操作
    r.return(200, "Hello");     // 直接返回
    r.error("message");          // 记录错误日志
    r.log("message");            // 记录日志
    r.warn("message");           // 记录警告

    // 子请求
    r.subrequest('/internal', function(reply) {
        // 处理子请求响应
        r.return(200, reply.body);
    });
}
```

### 配置示例

#### 动态路由

```javascript
// router.js
function route(r) {
    var version = r.headersIn['X-API-Version'];

    if (version === 'v2') {
        r.internalRedirect('/api/v2' + r.uri);
    } else {
        r.internalRedirect('/api/v1' + r.uri);
    }
}

export default { route };
```

```nginx
js_import main from /etc/nginx/router.js;

location /api/ {
    js_content main.route;
}

location /api/v1/ {
    proxy_pass http://backend_v1/;
}

location /api/v2/ {
    proxy_pass http://backend_v2/;
}
```

#### 自定义认证

```javascript
// auth.js
async function auth(r) {
    // 从数据库验证 token
    var reply = await r.subrequest('/auth/check', {
        method: 'POST',
        body: JSON.stringify({ token: r.headersIn['Authorization'] })
    });

    if (reply.status === 200) {
        var result = JSON.parse(reply.body);
        r.headersOut['X-User-Id'] = result.user_id;
        r.internalRedirect('/internal' + r.uri);
    } else {
        r.return(401, "Unauthorized");
    }
}

export default { auth };
```

```nginx
js_import auth from /etc/nginx/auth.js;

location /protected/ {
    js_content auth.auth;
}

location /internal/ {
    internal;                    # 只允许内部跳转
    proxy_pass http://backend/;
}
```

#### JWT 验证

```javascript
// jwt.js
function verify_jwt(r) {
    var jwt = r.headersIn['Authorization'];
    if (!jwt) {
        return null;
    }

    jwt = jwt.replace('Bearer ', '');

    var parts = jwt.split('.');
    if (parts.length !== 3) {
        return null;
    }

    try {
        var header = JSON.parse(Buffer.from(parts[0], 'base64').toString());
        var payload = JSON.parse(Buffer.from(parts[1], 'base64').toString());

        // 验证过期时间
        if (payload.exp && payload.exp < Math.floor(Date.now() / 1000)) {
            return null;
        }

        return payload;
    } catch (e) {
        return null;
    }
}

function get_user_id(r) {
    var payload = verify_jwt(r);
    return payload ? payload.sub : '';
}

export default { get_user_id };
```

```nginx
js_import jwt from /etc/nginx/jwt.js;
js_set $user_id jwt.get_user_id;

server {
    location /api/ {
        if ($user_id = "") {
            return 401 "Invalid or expired token";
        }

        proxy_set_header X-User-Id $user_id;
        proxy_pass http://backend;
    }
}
```

### 安装

```bash
# 从源码编译
./configure \
    --add-dynamic-module=/path/to/njs/nginx \
    --with-compat

make modules

# 加载模块
# nginx.conf
load_module modules/ngx_http_js_module.so;
```

---

## 2. ngx_http_lua_module (OpenResty)

ngx_http_lua_module 是 OpenResty 平台的核心组件，允许在 NGINX 中嵌入 Lua 脚本。

### OpenResty 平台介绍

OpenResty 是一个基于 NGINX 的全功能 Web 平台，集成了：
- LuaJIT（高性能 Lua 解释器）
- 大量精心设计的 Lua 库
- 完整的协程调度器
- 异步非阻塞 I/O 能力

### Lua 指令概述

| 指令 | 上下文 | 说明 |
|------|--------|------|
| `init_by_lua` | http | NGINX 启动时执行 |
| `init_worker_by_lua` | http | 每个 worker 启动时执行 |
| `set_by_lua` | server/location/if | 设置变量值 |
| `content_by_lua` | location/location if | 生成响应内容 |
| `access_by_lua` | http/server/location | 访问控制阶段执行 |
| `rewrite_by_lua` | http/server/location | 重写阶段执行 |
| `header_filter_by_lua` | http/server/location | 处理响应头 |
| `body_filter_by_lua` | http/server/location | 处理响应体 |
| `log_by_lua` | http/server/location | 日志阶段执行 |
| `balancer_by_lua` | upstream | 自定义负载均衡 |

### 配置示例

```nginx
http {
    # 全局初始化
    init_by_lua_block {
        require "cjson"
        redis = require "resty.redis"
    }

    server {
        location /api {
            content_by_lua_block {
                local cjson = require "cjson"

                local data = {
                    message = "Hello from Lua",
                    time = ngx.time()
                }

                ngx.header['Content-Type'] = 'application/json'
                ngx.say(cjson.encode(data))
            }
        }

        location /auth {
            access_by_lua_block {
                local token = ngx.var.http_authorization
                if not token then
                    ngx.exit(ngx.HTTP_UNAUTHORIZED)
                end
                -- 验证 token 逻辑
            }
        }
    }
}
```

### 应用场景

| 场景 | 说明 |
|------|------|
| **API 网关** | 路由、限流、认证、日志 |
| **WAF** | Web 应用防火墙，自定义安全规则 |
| **动态负载均衡** | 基于实时指标的流量调度 |
| **边缘计算** | 在边缘节点执行业务逻辑 |
| **实时数据处理** | 日志实时分析、指标采集 |

---

## 3. ngx_http_brotli_module (Brotli 压缩)

Brotli 是 Google 开发的新一代压缩算法，相比 Gzip 提供更好的压缩比。

### 核心指令

#### `brotli`

启用或禁用 Brotli 压缩。

```nginx
brotli on;                       # 启用动态压缩
brotli_static on;                # 启用静态 .br 文件支持
```

#### `brotli_comp_level`

设置压缩级别（0-11）。

```nginx
brotli_comp_level 6;             # 默认值，平衡压缩比和速度
```

**级别说明**：

| 级别 | 压缩比 | 速度 | 适用场景 |
|------|--------|------|----------|
| 1 | 低 | 最快 | 高流量低延迟场景 |
| 6 | 中 | 中等 | 通用场景（推荐） |
| 11 | 最高 | 最慢 | 预压缩静态资源 |

#### `brotli_types`

指定压缩的 MIME 类型。

```nginx
brotli_types text/plain text/css application/json application/javascript text/xml application/xml;
```

### 完整配置示例

```nginx
# 动态压缩
location / {
    brotli on;
    brotli_comp_level 6;
    brotli_types text/plain text/css application/json
                 application/javascript text/xml application/xml
                 application/rss+xml text/javascript
                 application/vnd.ms-fontobject application/x-font-ttf
                 font/opentype image/svg+xml image/x-icon;

    proxy_pass http://backend;
}

# 静态预压缩文件
location ~ \.(js|css|html|json)$ {
    brotli_static on;            # 优先查找 .br 文件
    gzip_static on;              # 回退到 .gz 文件
    try_files $uri $uri/ =404;
}
```

### Brotli vs Gzip 对比

| 特性 | Brotli | Gzip |
|------|--------|------|
| 压缩比 | 更高（约 20-30%） | 较低 |
| 压缩速度 | 较慢 | 较快 |
| 解压速度 | 快 | 快 |
| 浏览器支持 | 现代浏览器 | 几乎所有浏览器 |
| 字典支持 | 有（静态字典） | 无 |
| 最佳用途 | 静态资源预压缩 | 动态内容压缩 |

### 预压缩静态资源

```bash
# 使用 brotli 命令行工具压缩
brotli -q 11 -o file.js.br file.js

# 批量压缩
find . -type f \( -name "*.js" -o -name "*.css" \) -exec brotli -q 11 -o {}.br {} \;
```

---

## 4. ngx_cache_purge_module (缓存清除)

该模块允许通过 HTTP 请求清除 NGINX 代理缓存中的特定内容。

### 核心指令

#### `proxy_cache_purge`

配置缓存清除的访问控制。

```nginx
proxy_cache_purge on;            # 启用清除功能
proxy_cache_purge $purge_method; # 基于变量控制
```

#### `fastcgi_cache_purge`

FastCGI 缓存清除。

```nginx
fastcgi_cache_purge on;
```

### 配置示例

```nginx
http {
    proxy_cache_path /var/cache/nginx levels=1:2
                     keys_zone=my_cache:10m
                     max_size=1g inactive=60m;

    map $request_method $purge_method {
        default 0;
        PURGE   1;
    }

    server {
        listen 80;
        server_name example.com;

        location / {
            proxy_cache my_cache;
            proxy_cache_key "$scheme$host$request_uri";
            proxy_cache_purge $purge_method;
            proxy_pass http://backend;

            # 可选：限制 PURGE 来源
            if ($purge_method = 1) {
                allow 127.0.0.1;
                allow 192.168.1.0/24;
                deny all;
            }
        }
    }
}
```

### 使用方式

```bash
# 清除单个 URL
curl -X PURGE http://example.com/path/to/resource

# 清除通配符（需配置 proxy_cache_purge 支持）
curl -X PURGE http://example.com/*

# 使用 API Key 认证
curl -X PURGE -H "X-Purge-Key: secret123" http://example.com/api/data
```

### 安全考虑

```nginx
# 仅允许特定 IP 执行 PURGE
location / {
    proxy_cache_purge $purge_method;

    if ($purge_method = 1) {
        # 检查来源 IP
        set $allowed 0;
        if ($remote_addr ~ "^192\.168\.") {
            set $allowed 1;
        }

        # 或检查 API Key
        if ($http_x_purge_key != "secret_key") {
            set $allowed 0;
        }

        if ($allowed = 0) {
            return 403 "Purge not allowed";
        }
    }
}
```

---

## 5. ngx_headers_more_module (增强头部)

提供更灵活的 HTTP 头部操作能力。

### 核心指令

#### `more_set_headers`

设置响应头，支持条件表达式。

```nginx
# 基础用法
more_set_headers "Server: MyServer";
more_set_headers "X-Frame-Options: DENY";

# 状态码限定
more_set_headers "X-Debug: on" always;
more_set_headers -s 200 302 "Cache-Control: public";

# 多行
more_set_headers "X-Powered-By: NGINX
X-Custom: Value";
```

#### `more_clear_headers`

清除响应头。

```nginx
# 清除单个
more_clear_headers Server;

# 清除多个
more_clear_headers Server X-Powered-By X-AspNet-Version;

# 使用通配符
clear_headers "X-*";
```

#### `more_set_input_headers`

设置请求头（传递给后端）。

```nginx
more_set_input_headers "X-Real-IP: $remote_addr";
```

### 配置示例

#### 隐藏 Server 信息

```nginx
server {
    # 清除默认 Server 头
    more_clear_headers Server;

    # 可选：设置自定义值
    more_set_headers "Server: Secure Server";

    location / {
        proxy_pass http://backend;
    }
}
```

#### 添加安全头部

```nginx
server {
    # 点击劫持防护
    more_set_headers "X-Frame-Options: SAMEORIGIN";

    # XSS 防护
    more_set_headers "X-XSS-Protection: 1; mode=block";

    # MIME 类型嗅探防护
    more_set_headers "X-Content-Type-Options: nosniff";

    # 内容安全策略
    more_set_headers "Content-Security-Policy: default-src 'self'";

    # 引用策略
    more_set_headers "Referrer-Policy: strict-origin-when-cross-origin";

    # HSTS（HTTPS 严格传输安全）
    more_set_headers "Strict-Transport-Security: max-age=31536000; includeSubDomains";

    location / {
        proxy_pass http://backend;
    }
}
```

#### 条件头部设置

```nginx
# 仅在特定路径添加头部
location /api/ {
    more_set_headers "X-API-Version: 2.0";
    proxy_pass http://api_backend;
}

# 基于状态码
error_page 500 502 503 504 /50x.html;
location = /50x.html {
    more_set_headers "X-Error-Source: upstream";
    internal;
}
```

---

## 6. nginx-rtmp-module (流媒体)

为 NGINX 添加 RTMP/HLS/DASH 流媒体服务能力。

### 核心配置块

```nginx
rtmp {
    server {
        listen 1935;             # RTMP 默认端口
        chunk_size 4096;

        # 推流应用
        application live {
            live on;

            # HLS 输出
            hls on;
            hls_path /var/www/hls;
            hls_fragment 3s;
            hls_playlist_length 60s;

            # DASH 输出
            dash on;
            dash_path /var/www/dash;
            dash_fragment 3s;
        }

        # 录制应用
        application record {
            live on;
            record all;
            record_path /var/recordings;
            record_unique on;
            record_suffix .flv;
        }
    }
}
```

### 关键指令

| 指令 | 说明 |
|------|------|
| `live on` | 启用直播模式 |
| `hls on` | 启用 HLS 输出 |
| `hls_path` | HLS 文件存储路径 |
| `hls_fragment` | HLS 切片时长 |
| `dash on` | 启用 DASH 输出 |
| `push rtmp://...` | 推流到其他服务器 |
| `pull rtmp://...` | 从其他服务器拉流 |

### 完整直播配置示例

```nginx
rtmp {
    server {
        listen 1935;
        listen [::]:1935 ipv6only=on;

        application live {
            live on;

            # 推流认证
            on_publish http://localhost:8080/auth;

            # HLS 配置
            hls on;
            hls_path /var/www/stream/hls;
            hls_fragment 3s;
            hls_playlist_length 60s;
            hls_nested on;              # 为每个流创建子目录
            hls_cleanup on;             # 自动清理旧切片

            # 多码率 HLS
            hls_variant _low BANDWIDTH=500000;
            hls_variant _mid BANDWIDTH=1500000;
            hls_variant _high BANDWIDTH=3000000;

            # DASH 配置
            dash on;
            dash_path /var/www/stream/dash;
            dash_fragment 3s;
            dash_nested on;

            # 录制配置
            record all;
            record_path /var/recordings;
            record_unique on;
            record_suffix .flv;
            record_max_size 500M;
            record_max_frames 2;

            # 推流到其他平台
            push rtmp://youtube.com/live2/stream_key;
            push rtmp://facebook.com/rtmp/stream_key;
        }
    }
}

http {
    server {
        listen 80;
        server_name stream.example.com;

        # HLS 播放
        location /hls {
            types {
                application/vnd.apple.mpegurl m3u8;
                video/mp2t ts;
            }
            root /var/www/stream;
            add_header Cache-Control no-cache;
            add_header Access-Control-Allow-Origin *;
        }

        # DASH 播放
        location /dash {
            types {
                application/dash+xml mpd;
            }
            root /var/www/stream;
            add_header Cache-Control no-cache;
            add_header Access-Control-Allow-Origin *;
        }

        # 推流认证接口
        location /auth {
            proxy_pass http://auth_backend/verify;
            proxy_set_header Content-Type application/x-www-form-urlencoded;
        }
    }
}
```

### 推流与播放

```bash
# OBS / FFmpeg 推流
ffmpeg -re -i input.mp4 -c copy -f flv rtmp://server/live/stream_name

# 播放地址
# HLS: http://server/hls/stream_name.m3u8
# DASH: http://server/dash/stream_name.mpd
```

---

## 7. ngx_http_fancyindex_module (美化目录)

提供更美观、功能更强大的目录列表页面。

### 核心指令

#### `fancyindex`

启用美化目录。

```nginx
fancyindex on;
```

#### `fancyindex_css_href`

自定义 CSS 样式表。

```nginx
fancyindex_css_href /fancyindex.css;
```

#### `fancyindex_exact_size`

显示精确文件大小。

```nginx
fancyindex_exact_size off;       # 显示人性化大小（1K, 234M, 2G）
```

#### `fancyindex_header` / `fancyindex_footer`

自定义页眉页脚。

```nginx
fancyindex_header /header.html;
fancyindex_footer /footer.html;
```

### 配置示例

```nginx
server {
    listen 80;
    server_name files.example.com;
    root /var/www/files;

    location / {
        fancyindex on;
        fancyindex_exact_size off;
        fancyindex_localtime on;        # 使用本地时间
        fancyindex_show_path off;       # 隐藏路径显示
        fancyindex_name_length 50;      # 文件名最大显示长度

        # 自定义样式
        fancyindex_css_href "/style/fancyindex.css";
        fancyindex_header "/style/header.html";
        fancyindex_footer "/style/footer.html";

        # 忽略隐藏文件
        fancyindex_ignore ".." ".*";

        # 默认排序
        fancyindex_default_sort name_desc;
    }

    # 静态资源不显示目录列表
    location ~* \.(jpg|jpeg|png|gif|ico|css|js)$ {
        fancyindex off;
    }
}
```

### 主题配置

创建自定义主题：

```html
<!-- header.html -->
<!DOCTYPE html>
<html>
<head>
    <title>文件列表</title>
    <link rel="stylesheet" href="/style/custom.css">
</head>
<body>
    <div class="container">
        <h1>文件下载中心</h1>

<!-- footer.html -->
    </div>
    <footer>
        <p>&copy; 2024 Example Corp</p>
    </footer>
</body>
</html>
```

---

## 8. Sticky Module (会话保持)

基于 Cookie 的会话保持模块，确保同一客户端的请求始终路由到同一后端服务器。

### 核心指令

#### `sticky`

启用基于 Cookie 的会话保持。

```nginx
upstream backend {
    server backend1.example.com;
    server backend2.example.com;
    server backend3.example.com;

    sticky cookie srv_id expires=1h domain=.example.com path=/;
}
```

**参数说明**：

| 参数 | 说明 |
|------|------|
| `cookie name` | Cookie 名称 |
| `expires=time` | Cookie 过期时间 |
| `domain=name` | Cookie 域名 |
| `path=path` | Cookie 路径 |
| `secure` | 仅 HTTPS 传输 |
| `httponly` | 禁止 JavaScript 访问 |

### sticky vs ip_hash 对比

| 特性 | sticky (cookie) | ip_hash |
|------|-----------------|---------|
| 会话保持机制 | 基于 Cookie | 基于客户端 IP |
| 支持 NAT/代理 | 是（无视 IP 变化） | 否（同一 NAT 下用户共享会话） |
| 移动端支持 | 优秀 | 较差（IP 频繁变化） |
| 首次请求 | 轮询分配 | 哈希分配 |
| Cookie 依赖 | 需要 | 不需要 |

### 配置示例

```nginx
upstream backend {
    server 10.0.0.1:8080 weight=5;
    server 10.0.0.2:8080 weight=5;
    server 10.0.0.3:8080 backup;

    # 基础配置
    sticky cookie srv_id expires=1h domain=.example.com path=/;

    # 完整配置
    # sticky cookie srv_id
    #        expires=1h
    #        domain=.example.com
    #        path=/
    #        secure
    #        httponly;
}

server {
    listen 80;
    server_name app.example.com;

    location / {
        proxy_pass http://backend;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }
}
```

### 备用方案（无 Cookie 时）

```nginx
upstream backend {
    zone backend 64k;

    server backend1.example.com;
    server backend2.example.com;

    # 尝试 sticky，失败时使用 ip_hash
    sticky cookie srv_id expires=1h;
}

# 或使用 route 指令结合其他逻辑
upstream backend {
    server backend1.example.com route=a;
    server backend2.example.com route=b;

    sticky route $route_cookie;
}
```

---

## 9. 安装方法

### 动态模块安装

动态模块可以在不重新编译 NGINX 的情况下添加功能。

#### 步骤

```bash
# 1. 获取与 NGINX 版本匹配的模块源码
cd /usr/local/src
git clone https://github.com/nginx/njs.git
git clone https://github.com/google/ngx_brotli.git

# 2. 下载相同版本的 NGINX 源码
wget http://nginx.org/download/nginx-1.24.0.tar.gz
tar -xzf nginx-1.24.0.tar.gz
cd nginx-1.24.0

# 3. 配置动态模块
./configure \
    --with-compat \
    --add-dynamic-module=../njs/nginx \
    --add-dynamic-module=../ngx_brotli

# 4. 编译模块
make modules

# 5. 安装模块
mkdir -p /usr/local/nginx/modules
cp objs/ngx_http_js_module.so /usr/local/nginx/modules/
cp objs/ngx_http_brotli_filter_module.so /usr/local/nginx/modules/
cp objs/ngx_http_brotli_static_module.so /usr/local/nginx/modules/
```

#### 加载模块

```nginx
# nginx.conf
# 在顶级上下文加载
load_module modules/ngx_http_js_module.so;
load_module modules/ngx_http_brotli_filter_module.so;
load_module modules/ngx_http_brotli_static_module.so;

user nginx;
worker_processes auto;
...
```

### 编译安装（静态链接）

将模块静态编译进 NGINX 可执行文件。

```bash
# 1. 准备源码
cd /usr/local/src
wget http://nginx.org/download/nginx-1.24.0.tar.gz
tar -xzf nginx-1.24.0.tar.gz

# 2. 下载第三方模块
git clone https://github.com/openresty/headers-more-nginx-module.git
git clone https://github.com/FRiCKLE/ngx_cache_purge.git

# 3. 配置编译
cd nginx-1.24.0
./configure \
    --prefix=/usr/local/nginx \
    --user=nginx \
    --group=nginx \
    --with-http_ssl_module \
    --with-http_v2_module \
    --with-http_realip_module \
    --with-http_gzip_static_module \
    --with-http_stub_status_module \
    --with-pcre \
    --with-threads \
    --add-module=../headers-more-nginx-module \
    --add-module=../ngx_cache_purge

# 4. 编译安装
make
make install
```

### 包管理器安装

#### Ubuntu/Debian

```bash
# NJS 模块
apt install nginx-module-njs

# OpenResty（包含 Lua 模块）
apt install openresty
```

#### CentOS/RHEL

```bash
# 使用官方仓库
yum install nginx-module-njs
yum install nginx-module-image-filter

# 启用模块
echo 'load_module /usr/lib64/nginx/modules/ngx_http_js_module.so;' > /etc/nginx/conf.d/njs.conf
```

### 动态模块 vs 编译安装对比

| 特性 | 动态模块 | 静态编译 |
|------|----------|----------|
| 灵活性 | 高（可随时加载/卸载） | 低（需重新编译） |
| 维护成本 | 低 | 高 |
| 性能 | 略低（模块加载开销） | 略高 |
| 部署便捷性 | 高 | 低 |
| 版本兼容性 | 需版本匹配 | 无兼容性问题 |
| 适用场景 | 生产环境、频繁更新 | 性能敏感、定制化需求 |

### 最佳实践

1. **优先使用官方包**：通过包管理器安装的模块通常经过充分测试
2. **版本匹配**：动态模块必须与 NGINX 版本完全匹配
3. **最小化模块**：仅加载必要的模块，减少攻击面
4. **测试环境先行**：新模块先在测试环境验证
5. **文档记录**：记录所有加载的第三方模块及其版本
