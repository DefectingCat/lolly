# NGINX 压缩与缓存指南

## 1. Gzip 压缩配置

### 基础配置

```nginx
http {
    gzip on;
    gzip_min_length 1000;
    gzip_proxied any;
    gzip_comp_level 6;
    gzip_types text/plain text/css application/json application/javascript text/xml application/xml;
    gzip_vary on;
}
```

### 指令详解

| 指令 | 说明 | 默认值 |
|------|------|--------|
| `gzip` | 启用/禁用压缩 | off |
| `gzip_buffers` | 压缩缓冲区数量和大小 | 32 4k 或 16 8k |
| `gzip_comp_level` | 压缩级别（1-9） | 1 |
| `gzip_disable` | 禁用压缩的 User-Agent 正则 | - |
| `gzip_http_version` | 最小 HTTP 版本 | 1.1 |
| `gzip_min_length` | 最小压缩长度 | 20 |
| `gzip_proxied` | 代理请求压缩条件 | off |
| `gzip_types` | 压缩的 MIME 类型 | text/html |
| `gzip_vary` | 添加 Vary 头 | off |

### 压缩级别选择

| 级别 | 压缩率 | CPU 消耗 | 推荐场景 |
|------|--------|----------|----------|
| 1 | 低 | 低 | CPU 受限环境 |
| 4-6 | 中 | 中 | **推荐（平衡）** |
| 9 | 高 | 高 | 静态内容预压缩 |

### gzip_proxied 参数

| 参数 | 说明 |
|------|------|
| `off` | 禁用所有代理请求压缩 |
| `any` | 压缩所有代理请求 |
| `expired` | Expires 头表明过期的响应 |
| `no-cache` | Cache-Control: no-cache |
| `no-store` | Cache-Control: no-store |
| `private` | Cache-Control: private |
| `auth` | 包含 Authorization 头 |

### 完整配置示例

```nginx
http {
    gzip on;
    gzip_comp_level 6;
    gzip_min_length 1000;
    gzip_proxied any;
    gzip_vary on;

    gzip_types
        text/plain
        text/css
        text/xml
        text/javascript
        application/json
        application/javascript
        application/xml
        application/xml+rss
        application/x-javascript;

    gzip_disable "msie6";

    # 排除已压缩文件
    gzip_types ~ "image/(gif|jpg|jpeg|png|webp)|video/.*|application/pdf|application/zip";

    server {
        location / {
            # 继承全局配置
        }
    }
}
```

### 压缩变量

`$gzip_ratio`：压缩率（原始大小/压缩后大小）

```nginx
log_format compression '$remote_addr - $remote_user [$time_local] '
                       '"$request" $status $bytes_sent '
                       '"$http_referer" "$http_user_agent" "$gzip_ratio"';
```

---

## 2. 预压缩文件（gzip_static）

### 启用预压缩

```nginx
location / {
    gzip_static on;
    gzip_proxied any;
}
```

### 工作原理

- 请求 `/style.css`，nginx 检查 `/style.css.gz` 是否存在
- 如果存在且客户端支持 gzip，直接发送预压缩文件
- 避免实时压缩的 CPU 开销

### 生成预压缩文件

```bash
# 批量生成
find /var/www/html -type f -name "*.css" -exec gzip -k {} \;
find /var/www/html -type f -name "*.js" -exec gzip -k {} \;
```

### 配置选项

```nginx
gzip_static on;      # 发送预压缩文件
gzip_static always;  # 总是发送预压缩文件（不检查客户端支持）
gzip_static off;     # 禁用
```

---

## 3. Brotli 压缩（需模块）

### 安装模块

```bash
# 编译时添加
--add-module=/path/to/ngx_brotli
```

### 配置示例

```nginx
http {
    # Brotli 压缩
    brotli on;
    brotli_comp_level 6;
    brotli_types text/plain text/css application/json application/javascript text/xml application/xml;
    brotli_min_length 1000;

    # 同时启用 gzip 作为后备
    gzip on;
    gzip_comp_level 6;
    gzip_types text/plain text/css application/json application/javascript;
}
```

---

## 4. 代理缓存配置

### 缓存路径定义

```nginx
http {
    proxy_cache_path /data/nginx/cache
        levels=1:2              # 目录层级（a/bc/abc...）
        keys_zone=main:10m      # 共享内存区名称和大小
        max_size=1g             # 缓存最大大小
        inactive=60m            # 非活动数据保留时间
        use_temp_path=off       # 临时文件存放位置
        manager_files=100       # 缓存管理器每次处理文件数
        manager_threshold=500ms;
}
```

### 目录层级说明

`levels=1:2` 表示：
- 第一级：16 个目录（0-f）
- 第二级：256 个目录（00-ff）

缓存文件路径示例：`/data/nginx/cache/a/bc/abcdef...`

### 启用缓存

```nginx
server {
    location / {
        proxy_cache main;
        proxy_cache_key "$scheme$request_method$host$request_uri";
        proxy_cache_valid 200 302 10m;
        proxy_cache_valid 404 1m;
        proxy_pass http://backend;
    }
}
```

### 缓存键定义

```nginx
# 默认
proxy_cache_key $scheme$proxy_host$request_uri;

# 自定义
proxy_cache_key "$host$request_uri $cookie_user";
proxy_cache_key "$scheme$request_method$host$request_uri";
proxy_cache_key "$server_name$uri$is_args$args";
```

### 缓存有效期

```nginx
# 按响应码设置
proxy_cache_valid 200 302 10m;
proxy_cache_valid 301 1h;
proxy_cache_valid 404 1m;
proxy_cache_valid any 1m;

# 使用响应头控制
# X-Accel-Expires: 有效期（秒）
# Expires: HTTP 过期时间
# Cache-Control: 缓存控制
```

### 条件缓存

```nginx
# 不从缓存获取
proxy_cache_bypass $cookie_nocache $arg_nocache;

# 不保存到缓存
proxy_no_cache $http_pragma $http_authorization;

# 示例：登录用户不缓存
map $cookie_user $skip_cache {
    default 0;
    ""      1;  # 未登录用户不缓存
}

location / {
    proxy_cache main;
    proxy_cache_bypass $skip_cache;
    proxy_no_cache $skip_cache;
}
```

### 使用过期缓存

```nginx
proxy_cache_use_stale error timeout updating http_500 http_502 http_503 http_504;
```

| 条件 | 说明 |
|------|------|
| `error` | 与后端通信出错 |
| `timeout` | 超时 |
| `invalid_header` | 无效响应头 |
| `updating` | 缓存正在更新 |
| `http_XXX` | 后端返回指定状态码 |

### 缓存锁

```nginx
proxy_cache_lock on;              # 同时只有一个请求填充缓存
proxy_cache_lock_timeout 5s;      # 锁超时时间
proxy_cache_lock_age 5s;          # 允许新请求时间
```

### 后台更新

```nginx
proxy_cache_background_update on;  # 后台更新过期缓存
```

---

## 5. FastCGI 缓存

### 配置示例

```nginx
http {
    fastcgi_cache_path /var/cache/nginx/fastcgi
        levels=1:2
        keys_zone=fastcgi:10m
        inactive=60m
        max_size=100m;

    server {
        location ~ \.php$ {
            fastcgi_cache fastcgi;
            fastcgi_cache_key "$request_method$host$request_uri";
            fastcgi_cache_valid 200 302 10m;
            fastcgi_cache_valid 404 1m;
            fastcgi_cache_methods GET HEAD;

            fastcgi_pass localhost:9000;
            fastcgi_param SCRIPT_FILENAME $document_root$fastcgi_script_name;
            include fastcgi_params;
        }
    }
}
```

---

## 6. 缓存清除（商业版）

```nginx
map $request_method $purge_method {
    PURGE 1;
    default 0;
}

server {
    location / {
        proxy_cache main;
        proxy_cache_key $uri;
        proxy_cache_purge $purge_method;
    }
}

# 清除缓存
# curl -X PURGE http://example.com/path/to/file
```

---

## 7. 静态文件缓存

### 浏览器缓存

```nginx
location ~* \.(jpg|jpeg|png|gif|ico|css|js|pdf|txt|woff)$ {
    expires 30d;
    add_header Cache-Control "public, immutable";
    access_log off;
}

location ~* \.(html|htm)$ {
    expires 1h;
    add_header Cache-Control "public, must-revalidate";
}

# 禁用缓存
location /api/ {
    add_header Cache-Control "no-cache, no-store, must-revalidate";
    add_header Pragma "no-cache";
    expires 0;
}
```

### expires 指令

```nginx
expires 30d;      # 30 天
expires 1h;       # 1 小时
expires epoch;    # 1970-01-01（不缓存）
expires max;      # 最大值（2037 年）
expires off;      # 不修改（默认）
```

### try_files + 缓存

```nginx
location / {
    try_files $uri @backend;
}

location @backend {
    proxy_cache main;
    proxy_cache_valid 200 10m;
    proxy_pass http://backend;
}
```

---

## 8. 文件描述符缓存

```nginx
open_file_cache max=1000 inactive=20s;
open_file_cache_valid 30s;
open_file_cache_min_uses 2;
open_file_cache_errors on;
```

| 参数 | 说明 |
|------|------|
| `max` | 缓存最大文件数 |
| `inactive` | 非活动文件移除时间 |
| `valid` | 检查文件是否存在的间隔 |
| `min_uses` | 保持打开的最少使用次数 |

---

## 9. 缓存状态监控

### stub_status 模块

```nginx
location /nginx_status {
    stub_status;
    allow 127.0.0.1;
    deny all;
}
```

输出示例：
```
Active connections: 10
server accepts handled requests
 100 100 200
Reading: 0 Writing: 1 Waiting: 9
```

### 缓存命中率计算

```nginx
log_format cache_status '$remote_addr - [$time_local] '
                        '"$request" $status '
                        'cache: $upstream_cache_status';

# 状态值：HIT, MISS, BYPASS, EXPIRED, STALE, UPDATING, REVALIDATED
```

---

## 10. 缓存最佳实践

### 缓存策略建议

| 内容类型 | 缓存时间 | 策略 |
|----------|----------|------|
| 静态资源（图片、CSS、JS） | 30 天 + immutable | 浏览器缓存 |
| HTML 页面 | 1 小时 | 协商缓存 |
| API 响应 | 按需 | 代理缓存 |
| 用户特定内容 | 不缓存 | 动态生成 |

### 避免缓存问题

```nginx
# 避免缓存POST请求
proxy_cache_methods GET HEAD;

# 避免缓存带查询参数的请求
proxy_cache_key "$host$request_uri";

# 避免缓存大文件
proxy_max_temp_file_size 0;

# 动态内容禁用缓存
location /api/ {
    proxy_cache off;
    add_header Cache-Control "no-store";
}
```

### 缓存预热

```bash
# 预热缓存脚本
#!/bin/bash
urls=(
    "https://example.com/page1"
    "https://example.com/page2"
)

for url in "${urls[@]}"; do
    curl -s "$url" > /dev/null &
done
wait
```