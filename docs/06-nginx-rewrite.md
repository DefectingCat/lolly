# NGINX URL 重写与请求处理指南

## 1. rewrite 模块概述

`ngx_http_rewrite_module` 模块用于：
- 使用 PCRE 正则表达式更改请求 URI
- 返回重定向
- 有条件地选择配置

### 处理顺序

1. 按顺序执行 server 级别的 rewrite 指令
2. 根据请求 URI 搜索 location
3. 按顺序执行 location 内的 rewrite 指令
4. 如果 URI 被 rewrite 更改，重复循环（不超过 10 次）

---

## 2. rewrite 指令

### 语法

`rewrite regex replacement [flag];`

```nginx
# 基础重写
rewrite ^/old/(.*)$ /new/$1 permanent;

# 重定向
rewrite ^/download/(.*)$ /files/$1 redirect;

# 内部重写
rewrite ^/images/(.*)\.jpg$ /pics/$1.png last;
```

### Flags 参数

| Flag | 说明 |
|------|------|
| `last` | 停止当前指令集处理，开始搜索新 location |
| `break` | 停止当前指令集处理，继续在当前 location 中处理 |
| `redirect` | 返回 302 临时重定向 |
| `permanent` | 返回 301 永久重定向 |

### 示例

```nginx
server {
    # Server 级别 - 使用 last
    rewrite ^(/download/.*)/media/(.*)\..*$ $1/mp3/$2.mp3 last;
    rewrite ^(/download/.*)/audio/(.*)\..*$ $1/mp3/$2.ra  last;

    location /download/ {
        # Location 级别 - 使用 break
        rewrite ^(/download/.*)/media/(.*)\..*$ $1/mp3/$2.mp3 break;
        rewrite ^(/download/.*)/audio/(.*)\..*$ $1/mp3/$2.ra  break;
    }
}
```

### 参数处理

```nginx
# replacement 包含新参数时，旧参数自动追加
rewrite ^/users/(.*)$ /show?user=$1? last;
# ? 结尾表示不追加旧参数
```

---

## 3. return 指令

### 语法

```nginx
return code [text];
return code URL;
return URL;
```

### 状态码使用

| 状态码 | 类型 | 用途 |
|--------|------|------|
| 301 | 永久重定向 | URL 永久变更 |
| 302 | 临时重定向 | URL 临时变更 |
| 303 | 临时重定向 | POST 后重定向到 GET |
| 307 | 临时重定向 | 保持请求方法 |
| 308 | 永久重定向 | 保持请求方法 |
| 444 | 非标准 | 关闭连接（不发送响应） |

### 示例

```nginx
# 返回文本
location /status {
    return 200 "OK";
}

# 返回 JSON
location /json {
    default_type application/json;
    return 200 '{"status": "ok"}';
}

# 重定向
location /old {
    return 301 /new;
}

# 域名重定向
server {
    listen 80;
    server_name old.example.com;
    return 301 http://new.example.com$request_uri;
}

# HTTP 到 HTTPS
server {
    listen 80;
    server_name example.com;
    return 301 https://$server_name$request_uri;
}

# 拒绝请求
location /private {
    return 403;
}

# 关闭连接（不发送响应）
server {
    listen 80 default_server;
    server_name _;
    return 444;
}
```

---

## 4. if 指令

### 语法

`if (condition) { ... }`

### 条件类型

| 类型 | 示例 | 说明 |
|------|------|------|
| 变量判断 | `if ($variable)` | 空字符串或 "0" 为假 |
| 字符串相等 | `if ($a = "value")` | 等于 |
| 字符串不等 | `if ($a != "value")` | 不等于 |
| 正则匹配 | `if ($a ~ pattern)` | 区分大小写 |
| 正则匹配 | `if ($a ~* pattern)` | 不区分大小写 |
| 正则不匹配 | `if ($a !~ pattern)` | 不匹配 |
| 文件存在 | `if (-f $uri)` | 文件存在 |
| 文件不存在 | `if (!-f $uri)` | 文件不存在 |
| 目录存在 | `if (-d $uri)` | 目录存在 |
| 目录不存在 | `if (!-d $uri)` | 目录不存在 |
| 文件/目录存在 | `if (-e $uri)` | 存在 |
| 可执行 | `if (-x $uri)` | 可执行文件 |

### 示例

```nginx
# 浏览器判断
if ($http_user_agent ~ MSIE) {
    rewrite ^(.*)$ /msie/$1 break;
}

# 请求方法判断
if ($request_method = POST) {
    return 405;
}

# 文件不存在时
if (!-f $request_filename) {
    rewrite ^(.*)$ /index.php last;
}

# 防盗链
if ($invalid_referer) {
    return 403;
}

# 限制速度
if ($slow) {
    limit_rate 10k;
    break;
}
```

### 注意事项

- `if` 块会创建单独的配置上下文
- 避免在 if 中使用某些指令（可能导致意外行为）
- 推荐使用 `try_files` 或 `map` 替代复杂条件判断

---

## 5. break 指令

停止处理当前 rewrite 模块指令集。

```nginx
location /download/ {
    if ($forbidden) {
        return 403;
    }

    if ($slow) {
        limit_rate 10k;
        break;  # 停止 rewrite 模块处理，继续 location 处理
    }

    rewrite ^(.*)$ /files/$1 break;
}
```

---

## 6. set 指令

设置变量值。

```nginx
set $variable value;

# 示例
set $mobile false;

if ($http_user_agent ~ "Mobile") {
    set $mobile true;
}

location /api {
    if ($mobile = true) {
        proxy_pass http://mobile_backend;
    }
    proxy_pass http://desktop_backend;
}
```

---

## 7. map 指令（替代复杂 if）

### 语法

`map source $variable { ... }`

### 示例

```nginx
http {
    # 浏览器映射
    map $http_user_agent $mobile {
        default         false;
        "~*Mobile"      true;
        "~*Android"     true;
        "~*iPhone"      true;
    }

    # 环境映射
    map $host $backend {
        default         http://backend.prod;
        dev.example.com http://backend.dev;
        test.example.com http://backend.test;
    }

    # 日志级别映射
    map $status $loggable {
        ~^[23]  0;  # 2xx/3xx 不记录
        default 1;
    }

    server {
        location / {
            if ($mobile = true) {
                proxy_pass http://mobile_backend;
            }
            proxy_pass $backend;
        }

        access_log /var/log/nginx/access.log combined if=$loggable;
    }
}
```

---

## 8. 常见重写场景

### 域名重定向

```nginx
# 旧域名到新域名
server {
    listen 80;
    server_name old.example.com;
    return 301 http://new.example.com$request_uri;
}

# 多域名统一
server {
    listen 80;
    server_name example.com www.example.com;
    return 301 https://example.com$request_uri;
}

# www 到非 www
server {
    listen 80;
    server_name www.example.com;
    return 301 http://example.com$request_uri;
}

# 非 www 到 www
server {
    listen 80;
    server_name example.com;
    return 301 http://www.example.com$request_uri;
}
```

### HTTP 到 HTTPS

```nginx
server {
    listen 80;
    server_name example.com;
    return 301 https://$server_name$request_uri;
}

server {
    listen 443 ssl;
    server_name example.com;
    # ...
}
```

### URL 结构变更

```nginx
# 产品页面重写
rewrite ^/products/([0-9]+)$ /product?id=$1 last;

# 分类页面
rewrite ^/category/([a-z]+)$ /cat?name=$1 last;

# API 版本迁移
rewrite ^/api/v1/(.*)$ /api/v2/$1 permanent;
```

### 静态文件处理

```nginx
location / {
    try_files $uri $uri/ @fallback;
}

location @fallback {
    rewrite ^(.*)$ /index.php last;
}

# 图片路径重写
rewrite ^/images/(.*)\.(gif|jpg|png)$ /static/$1.$2 last;
```

### PHP 应用重写

```nginx
location / {
    try_files $uri $uri/ /index.php?$query_string;
}

location ~ \.php$ {
    fastcgi_pass localhost:9000;
    fastcgi_param SCRIPT_FILENAME $document_root$fastcgi_script_name;
    include fastcgi_params;
}
```

### WordPress 重写

```nginx
location / {
    try_files $uri $uri/ /index.php?$args;
}

# WordPress 固定链接支持
rewrite /wp-admin$ /wp-admin/ permanent;
```

---

## 9. 防盗链配置

```nginx
location ~* \.(gif|jpg|png|swf|flv)$ {
    valid_referers none blocked server_names *.example.com example.*;

    if ($invalid_referer) {
        return 403;
        # 或返回替代图片
        # return 301 http://example.com/nolink.png;
    }
}
```

---

## 10. rewrite_log 指令

启用重写日志记录。

```nginx
rewrite_log on;   # 将 rewrite 处理结果记录到 error_log（notice 级别）
```

---

## 11. 内部实现说明

rewrite 指令在配置阶段编译为内部指令，请求处理时由虚拟栈机器解释执行。

**配置示例编译结果**：

```nginx
location /download/ {
    if ($forbidden) {
        return 403;
    }
    rewrite ^/(.*)$ /files/$1 break;
}
```

编译为内部指令：
```
variable $forbidden
check against zero
    return 403
    end of code
match of regular expression
copy $1
copy "/files/"
end of regular expression
end of code
```

---

## 12. 最佳实践

### 优先使用 return 而非 rewrite

```nginx
# 推荐
return 301 /new;

# 不推荐
rewrite ^(.*)$ /new permanent;
```

### 使用 try_files 替代复杂的 if 文件检查

```nginx
# 推荐
location / {
    try_files $uri $uri/ @fallback;
}

# 不推荐
location / {
    if (!-e $request_filename) {
        rewrite ^(.*)$ /index.php last;
    }
}
```

### 使用 map 替代重复的 if 判断

```nginx
# 推荐
http {
    map $http_user_agent $is_mobile {
        default      false;
        "~*Mobile"   true;
    }
}

# 不推荐
location / {
    if ($http_user_agent ~ Mobile) {
        set $is_mobile true;
    }
}
```

### Location 级别使用 break

```nginx
location /download/ {
    rewrite ^(.*)$ /files/$1 break;
    # 使用 break 而非 last，避免 10 次循环错误
}
```

### 注意循环限制

rewrite 循环不超过 10 次，超过会返回 500 错误。

```nginx
# 错误示例（无限循环）
rewrite ^/test$ /test last;
```