# NGINX HTTP 功能模块详解

本文档详细介绍 NGINX 常用的 HTTP 功能模块及其配置方法。

---

## 1. ngx_http_access_module (访问控制模块)

### 概述

ngx_http_access_module 模块用于限制对某些客户端地址的访问，提供简单的基于 IP 的访问控制功能。

### 核心指令

| 指令 | 语法 | 默认值 | 说明 | 上下文 |
|------|------|--------|------|--------|
| `allow` | `allow address \| CIDR \| unix: \| all;` | - | 允许指定地址访问 | http, server, location, limit_except |
| `deny` | `deny address \| CIDR \| unix: \| all;` | - | 拒绝指定地址访问 | http, server, location, limit_except |

### 配置示例

```nginx
# 拒绝单个 IP
location /admin/ {
    deny  192.168.1.1;
    allow 192.168.1.0/24;
    deny  all;
}

# 只允许内网访问管理后台
server {
    listen 80;
    server_name admin.example.com;

    location / {
        allow 10.0.0.0/8;
        allow 172.16.0.0/12;
        allow 192.168.0.0/16;
        deny  all;

        proxy_pass http://backend;
    }
}

# 拒绝特定网段，允许其他
location /api/ {
    deny  192.168.1.0/24;
    allow all;
}
```

### 应用场景

- **管理后台保护**：限制只有内网 IP 可以访问管理后台
- **API 访问控制**：限制特定 IP 才能调用敏感 API
- **防御恶意 IP**：封禁已知的攻击源 IP 地址

---

## 2. ngx_http_auth_basic_module (基础认证模块)

### 概述

ngx_http_auth_basic_module 模块允许使用 HTTP 基本认证协议验证用户名和密码，提供简单的用户名/密码访问控制。

### 核心指令

| 指令 | 语法 | 默认值 | 说明 | 上下文 |
|------|------|--------|------|--------|
| `auth_basic` | `auth_basic string \| off;` | `off` | 启用基本认证并设置提示信息 | http, server, location, limit_except |
| `auth_basic_user_file` | `auth_basic_user_file file;` | - | 指定密码文件路径 | http, server, location, limit_except |

### 配置示例

```nginx
# 基本认证配置
location /admin/ {
    auth_basic           "Administrator's Area";
    auth_basic_user_file /etc/nginx/.htpasswd;
}

# 生成密码文件
# 使用 htpasswd 工具
# htpasswd -c /etc/nginx/.htpasswd username

# 使用 openssl 生成
# printf "username:$(openssl passwd -crypt password)\n" >> /etc/nginx/.htpasswd

# 多区域不同认证
server {
    listen 80;
    server_name example.com;

    location /admin/ {
        auth_basic           "Admin Area";
        auth_basic_user_file /etc/nginx/admin.htpasswd;
        proxy_pass http://backend;
    }

    location /api/private/ {
        auth_basic           "API Access";
        auth_basic_user_file /etc/nginx/api.htpasswd;
        proxy_pass http://api_backend;
    }
}
```

### 密码文件格式

```
# /etc/nginx/.htpasswd
username1:encrypted_password1
username2:encrypted_password2
```

### 应用场景

- **开发环境保护**：为开发环境添加简单密码保护
- **内部文档访问**：限制内部文档仅供授权用户访问
- **简单后台管理**：小型项目的后台管理保护

---

## 3. ngx_http_auth_request_module (请求认证模块)

### 概述

ngx_http_auth_request_module 模块通过子请求实现基于外部服务的认证，允许向认证服务发送请求验证用户身份。

### 核心指令

| 指令 | 语法 | 默认值 | 说明 | 上下文 |
|------|------|--------|------|--------|
| `auth_request` | `auth_request uri \| off;` | `off` | 启用认证请求并指定认证 URI | http, server, location |
| `auth_request_set` | `auth_request_set $variable value;` | - | 从认证响应中设置变量 | http, server, location |

### 配置示例

```nginx
# 基础认证代理配置
location /private/ {
    auth_request /auth;
    auth_request_set $auth_status $upstream_status;

    proxy_pass http://backend;
}

# 认证服务 location
location = /auth {
    internal;
    proxy_pass http://auth-server/verify;
    proxy_pass_request_body off;
    proxy_set_header Content-Length "";
    proxy_set_header X-Original-URI $request_uri;
    proxy_set_header X-Original-Method $request_method;
}

# 完整示例：JWT 验证
server {
    listen 80;
    server_name api.example.com;

    location /api/ {
        auth_request /auth_jwt;
        auth_request_set $auth_user $upstream_http_x_user_id;
        auth_request_set $auth_roles $upstream_http_x_roles;

        proxy_set_header X-User-ID $auth_user;
        proxy_set_header X-Roles $auth_roles;
        proxy_pass http://api_backend;
    }

    location = /auth_jwt {
        internal;
        proxy_pass http://auth-service/validate;
        proxy_pass_request_body off;
        proxy_set_header Content-Length "";
        proxy_set_header Authorization $http_authorization;
        proxy_set_header X-Real-IP $remote_addr;
    }
}

# 带缓存的认证
location /protected/ {
    auth_request /auth;
    auth_request_set $auth_cache $upstream_http_x_auth_cache;

    proxy_cache_key "$cookie_session_id$request_method$host$request_uri";
    proxy_pass http://backend;
}
```

### 应用场景

- **OAuth/JWT 认证**：集成 OAuth2 或 JWT 认证服务
- **集中式认证**：统一认证中心验证用户身份
- **多系统单点登录**：实现跨系统的统一认证
- **自定义认证逻辑**：实现复杂的认证业务逻辑

---

## 4. ngx_http_autoindex_module (自动索引模块)

### 概述

ngx_http_autoindex_module 模块用于自动生成目录列表页面，当请求以 `/` 结尾时展示目录内容。

### 核心指令

| 指令 | 语法 | 默认值 | 说明 | 上下文 |
|------|------|--------|------|--------|
| `autoindex` | `autoindex on \| off;` | `off` | 启用自动目录列表 | http, server, location |
| `autoindex_exact_size` | `autoindex_exact_size on \| off;` | `on` | 显示精确文件大小 | http, server, location |
| `autoindex_format` | `autoindex_format html \| xml \| json \| jsonp;` | `html` | 目录列表格式 | http, server, location |
| `autoindex_localtime` | `autoindex_localtime on \| off;` | `off` | 使用本地时间显示 | http, server, location |

### 配置示例

```nginx
# 基本目录浏览
location /files/ {
    root /data/public;
    autoindex on;
}

# 优化显示格式
location /downloads/ {
    root /data/downloads;
    autoindex on;
    autoindex_exact_size off;   # 显示 KB/MB 而非字节
    autoindex_localtime on;      # 本地时间格式
}

# JSON 格式 API
location /api/files/ {
    root /data/public;
    autoindex on;
    autoindex_format json;
}

# 文件服务器配置
server {
    listen 80;
    server_name files.example.com;

    location / {
        root /var/www/files;
        autoindex on;
        autoindex_exact_size off;
        autoindex_localtime on;

        # 美化目录页面（可选）
        add_header X-Robots-Tag "noindex, nofollow";
    }
}
```

### 应用场景

- **文件下载站**：提供文件目录浏览和下载功能
- **文档服务器**：文档资源的目录化访问
- **镜像站点**：软件镜像的目录展示
- **内部资源共享**：企业内部文件共享访问

---

## 5. ngx_http_browser_module (浏览器检测模块)

### 概述

ngx_http_browser_module 模块用于根据 User-Agent 请求头创建变量，标识是否为古老或现代浏览器。

### 核心指令

| 指令 | 语法 | 默认值 | 说明 | 上下文 |
|------|------|--------|------|--------|
| `ancient_browser` | `ancient_browser string ...;` | - | 定义古老浏览器标识 | http, server, location |
| `ancient_browser_value` | `ancient_browser_value value;` | `1` | 古老浏览器变量值 | http, server, location |
| `modern_browser` | `modern_browser browser version \| unlisted;` | - | 定义现代浏览器 | http, server, location |
| `modern_browser_value` | `modern_browser_value value;` | `0` | 现代浏览器变量值 | http, server, location |

### 配置示例

```nginx
# 定义古老浏览器
http {
    ancient_browser "MSIE 6.0";
    ancient_browser "MSIE 5.5";
    ancient_browser "MSIE 5.0";
    ancient_browser "MSIE 4.0";

    # 定义现代浏览器
    modern_browser msie 7.0;
    modern_browser opera 9.0;
    modern_browser safari 3.0;
    modern_browser firefox 3.0;
    modern_browser chrome 1.0;

    server {
        listen 80;

        location / {
            # 古老浏览器重定向
            if ($ancient_browser) {
                rewrite ^ /browser-not-supported.html last;
            }

            # 现代浏览器正常访问
            proxy_pass http://backend;
        }
    }
}

# 根据浏览器类型分配不同后端
server {
    listen 80;
    server_name app.example.com;

    location / {
        # 古老浏览器使用兼容版本
        if ($ancient_browser) {
            proxy_pass http://legacy_backend;
            break;
        }

        # 现代浏览器使用标准版本
        proxy_pass http://modern_backend;
    }
}

# 浏览器日志记录
log_format browser '$remote_addr - $remote_user [$time_local] '
                   '"$request" $status $body_bytes_sent '
                   '"$http_user_agent" ancient=$ancient_browser';

access_log /var/log/nginx/browser.log browser;
```

### 应用场景

- **浏览器兼容性提示**：检测古老浏览器提示用户升级
- **差异化服务**：为不同浏览器提供不同版本的内容
- **统计分析**：分析用户浏览器分布情况
- **功能降级**：为古老浏览器提供简化功能

---

## 6. ngx_http_charset_module (字符集模块)

### 概述

ngx_http_charset_module 模块用于将指定的字符集添加到 Content-Type 响应头，并可以在不同字符集之间转换响应内容。

### 核心指令

| 指令 | 语法 | 默认值 | 说明 | 上下文 |
|------|------|--------|------|--------|
| `charset` | `charset charset \| off;` | `off` | 设置响应字符集 | http, server, location, if in location |
| `charset_map` | `charset_map source_charset destination_charset { ... }` | - | 定义字符集映射 | http |
| `override_charset` | `override_charset on \| off;` | `off` | 覆盖源字符集 | http, server, location, if in location |
| `source_charset` | `source_charset charset;` | - | 设置源字符集 | http, server, location, if in location |

### 配置示例

```nginx
# 基本字符集设置
server {
    listen 80;
    server_name example.com;

    charset utf-8;
    source_charset utf-8;
}

# 字符集转换
location /legacy/ {
    source_charset gb2312;
    charset utf-8;
}

# 字符集映射定义
http {
    charset_map koi8-r utf-8 {
        # 从 koi8-r 到 utf-8 的字符映射
        80 E28099;   # 单引号
        95 E280A6;   # 省略号
        9A C2A0;     # 不换行空格
    }
}

# 不同路径不同字符集
server {
    listen 80;

    location /cn/ {
        charset gb2312;
        root /var/www/cn;
    }

    location /en/ {
        charset utf-8;
        root /var/www/en;
    }

    location /jp/ {
        charset shift_jis;
        root /var/www/jp;
    }
}

# 根据来源覆盖字符集
location /api/ {
    charset utf-8;
    override_charset on;  # 强制使用 utf-8
    proxy_pass http://backend;
}
```

### 应用场景

- **多语言网站**：为不同语言设置合适的字符集
- **遗留系统兼容**：转换旧系统的非 UTF-8 编码
- **API 标准化**：统一 API 响应字符集为 UTF-8
- **字符集规范化**：确保所有响应使用正确字符集

---

## 7. ngx_http_geo_module (地理位置变量模块)

### 概述

ngx_http_geo_module 模块用于根据客户端 IP 地址创建变量，实现基于地理位置的访问控制或内容定制。

### 核心指令

| 指令 | 语法 | 默认值 | 说明 | 上下文 |
|------|------|--------|------|--------|
| `geo` | `geo [$address] $variable { ... }` | - | 定义地理位置映射 | http |

### 配置示例

```nginx
# 基本地理位置定义
http {
    geo $geo {
        default        unknown;
        127.0.0.1      local;
        192.168.1.0/24 internal;
        10.0.0.0/8     internal;
        1.2.3.4        china;
        5.6.7.8        usa;
    }

    server {
        listen 80;

        location / {
            add_header X-Geo $geo;
            return 200 "Your location: $geo";
        }
    }
}

# 使用变量作为数据源
geo $arg_ip $geo {
    default        unknown;
    192.168.0.0/16 local;
}

# 复杂地理位置配置
geo $geo_country {
    default        other;
    include        /etc/nginx/geo/countries.conf;  # 包含外部文件

    # 中国 IP 段
    1.0.1.0/24     cn;
    1.0.2.0/23     cn;
    1.0.32.0/19    cn;
    # ... 更多 IP 段

    # 美国 IP 段
    3.0.0.0/8      us;
    4.0.0.0/8      us;
    # ... 更多 IP 段

    delete         127.0.0.0/16;  # 删除特定范围
    proxy          192.168.100.1;  # 递归查询代理
    proxy_recursive on;             # 启用递归代理
}

# 应用示例：区域路由
server {
    listen 80;
    server_name example.com;

    location / {
        if ($geo_country = cn) {
            proxy_pass http://cn_backend;
            break;
        }

        if ($geo_country = us) {
            proxy_pass http://us_backend;
            break;
        }

        proxy_pass http://default_backend;
    }
}

# 访问限制
geo $allowed {
    default        0;
    192.168.0.0/16 1;
    10.0.0.0/8     1;
    172.16.0.0/12  1;
}

server {
    location /admin/ {
        if ($allowed = 0) {
            return 403;
        }
        proxy_pass http://backend;
    }
}
```

### 应用场景

- **区域路由**：根据用户地区路由到不同服务器
- **地理位置限制**：限制或允许特定地区访问
- **内容定制**：根据地区显示不同内容
- **访问统计分析**：按地理位置统计访问日志

---

## 8. ngx_http_geoip_module (GeoIP 模块)

### 概述

ngx_http_geoip_module 模块使用 MaxMind GeoIP 数据库创建变量，提供基于 IP 的地理位置信息（国家、城市、坐标等）。

### 核心指令

| 指令 | 语法 | 默认值 | 说明 | 上下文 |
|------|------|--------|------|--------|
| `geoip_country` | `geoip_country file;` | - | 指定国家数据库 | http |
| `geoip_city` | `geoip_city file;` | - | 指定城市数据库 | http |
| `geoip_org` | `geoip_org file;` | - | 指定组织数据库 | http |
| `geoip_proxy` | `geoip_proxy address \| CIDR;` | - | 定义代理服务器 | http |
| `geoip_proxy_recursive` | `geoip_proxy_recursive on \| off;` | `off` | 递归搜索代理 | http |

### 配置示例

```nginx
# 基本 GeoIP 配置
http {
    geoip_country /usr/share/GeoIP/GeoIP.dat;
    geoip_city    /usr/share/GeoIP/GeoLiteCity.dat;

    server {
        listen 80;

        location /geo {
            add_header X-Country-Code $geoip_country_code;
            add_header X-Country-Name $geoip_country_name;
            add_header X-City $geoip_city;
            add_header X-Region $geoip_region;
            add_header X-Region-Name $geoip_region_name;
            add_header X-Latitude $geoip_latitude;
            add_header X-Longitude $geoip_longitude;

            return 200 "Country: $geoip_country_name, City: $geoip_city";
        }
    }
}

# 代理环境配置
http {
    geoip_country /usr/share/GeoIP/GeoIP.dat;
    geoip_proxy 192.168.1.0/24;
    geoip_proxy_recursive on;

    server {
        location / {
            proxy_set_header X-Country $geoip_country_code;
            proxy_pass http://backend;
        }
    }
}

# 区域限制示例
server {
    listen 80;
    server_name example.com;

    location / {
        # 禁止特定国家访问
        if ($geoip_country_code = "CN") {
            return 403;
        }

        proxy_pass http://backend;
    }
}

# 多数据库配置
http {
    geoip_country /usr/share/GeoIP/GeoIP.dat;
    geoip_city    /usr/share/GeoIP/GeoLiteCity.dat;
    geoip_org     /usr/share/GeoIP/GeoIPASNum.dat;

    server {
        location /api/geo {
            default_type application/json;
            return 200 '{
                "country_code": "$geoip_country_code",
                "country_name": "$geoip_country_name",
                "city": "$geoip_city",
                "region": "$geoip_region",
                "latitude": "$geoip_latitude",
                "longitude": "$geoip_longitude",
                "org": "$geoip_org"
            }';
        }
    }
}
```

### 可用变量

| 变量名 | 说明 |
|--------|------|
| `$geoip_country_code` | 两位国家代码 |
| `$geoip_country_code3` | 三位国家代码 |
| `$geoip_country_name` | 国家名称 |
| `$geoip_city` | 城市名称 |
| `$geoip_region` | 地区代码 |
| `$geoip_region_name` | 地区名称 |
| `$geoip_latitude` | 纬度 |
| `$geoip_longitude` | 经度 |
| `$geoip_postal_code` | 邮政编码 |
| `$geoip_org` | 组织/ISP 名称 |

### 应用场景

- **地理位置服务**：为应用提供用户地理位置信息
- **内容本地化**：根据国家展示不同语言/货币的内容
- **访问控制**：基于国家代码限制访问
- **CDN 优化**：根据地理位置选择最近的服务器

---

## 9. ngx_http_map_module (变量映射模块)

### 概述

ngx_http_map_module 模块用于创建变量，其值取决于其他变量的值，实现灵活的变量映射和转换。

### 核心指令

| 指令 | 语法 | 默认值 | 说明 | 上下文 |
|------|------|--------|------|--------|
| `map` | `map string $variable { ... }` | - | 定义变量映射 | http |
| `map_hash_max_size` | `map_hash_max_size size;` | `2048` | 哈希表最大大小 | http |
| `map_hash_bucket_size` | `map_hash_bucket_size size;` | `32\|64\|128` | 哈希桶大小 | http |

### 配置示例

```nginx
# 基本变量映射
http {
    map $http_host $backend {
        default       backend_default;
        example.com   backend_example;
        api.example.com backend_api;
        "*.test.com"  backend_test;
    }

    server {
        location / {
            proxy_pass http://$backend;
        }
    }
}

# 主机名到后端映射
map $host $backend_pool {
    default              http://default_backend;
    "~^(?<name>.+)\\.example\\.com$"  http://$name_backend;
    www.example.com      http://www_backend;
    api.example.com      http://api_backend;
}

# User-Agent 映射
map $http_user_agent $is_mobile {
    default       0;
    "~*android"   1;
    "~*iphone"    1;
    "~*ipad"      1;
    "~*mobile"    1;
}

# 应用示例
server {
    location / {
        if ($is_mobile) {
            rewrite ^ /mobile$request_uri last;
        }
        proxy_pass http://desktop_backend;
    }
}

# 复杂映射配置
map $http_upgrade $connection_upgrade {
    default          close;
    websocket        upgrade;
}

# 用于 WebSocket 代理
location /ws/ {
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection $connection_upgrade;
    proxy_pass http://ws_backend;
}

# 状态码映射
map $status $loggable {
    ~^[23]  0;      # 2xx 和 3xx 不记录
    default 1;      # 其他状态码记录
}

access_log /var/log/nginx/access.log combined if=$loggable;

# 复杂正则映射
map $request_uri $cache_key {
    default          $request_uri;
    "~^/api/(?<version>v\\d+)/"  /api/$version/generic;
}

# 多条件映射
map "$scheme:$server_port" $is_https {
    default     0;
    "https:443" 1;
    "https:8443" 1;
}
```

### 映射规则

| 源值格式 | 说明 |
|----------|------|
| `string` | 精确匹配字符串 |
| `"~regex"` | 正则匹配（区分大小写） |
| `"~*regex"` | 正则匹配（不区分大小写） |
| `"!~regex"` | 正则不匹配（区分大小写） |
| `"!~*regex"` | 正则不匹配（不区分大小写） |

### 应用场景

- **动态后端选择**：根据请求信息选择不同后端
- **设备类型检测**：根据 User-Agent 识别设备类型
- **缓存键定制**：创建自定义缓存键
- **条件日志记录**：根据条件决定是否记录日志
- **变量转换**：将一个变量的值转换为另一个值

---

## 10. ngx_http_realip_module (真实 IP 模块)

### 概述

ngx_http_realip_module 模块用于当 NGINX 位于代理或负载均衡器后方时，将客户端的真实 IP 地址替换为请求头中的地址。

### 核心指令

| 指令 | 语法 | 默认值 | 说明 | 上下文 |
|------|------|--------|------|--------|
| `set_real_ip_from` | `set_real_ip_from address \| CIDR \| unix:;` | - | 定义可信代理地址 | http, server, location |
| `real_ip_header` | `real_ip_header field \| X-Real-IP \| X-Forwarded-For \| proxy_protocol;` | `X-Real-IP` | 指定真实 IP 来源头 | http, server, location |
| `real_ip_recursive` | `real_ip_recursive on \| off;` | `off` | 递归解析 | http, server, location |

### 配置示例

```nginx
# 基本真实 IP 配置
http {
    set_real_ip_from 192.168.1.0/24;
    set_real_ip_from 10.0.0.0/8;
    set_real_ip_from 172.16.0.0/12;
    real_ip_header X-Forwarded-For;
    real_ip_recursive on;
}

# CDN/云服务配置
http {
    # Cloudflare
    set_real_ip_from 103.21.244.0/22;
    set_real_ip_from 103.22.200.0/22;
    set_real_ip_from 103.31.4.0/22;
    # ... 更多 Cloudflare IP

    # 阿里云 SLB
    set_real_ip_from 100.64.0.0/10;

    real_ip_header X-Forwarded-For;
    real_ip_recursive on;

    server {
        listen 80;

        location / {
            # 现在 $remote_addr 是真实客户端 IP
            add_header X-Real-Client-IP $remote_addr;
            proxy_pass http://backend;
        }
    }
}

# 多级代理配置
server {
    listen 80;

    set_real_ip_from 192.168.0.0/16;
    set_real_ip_from unix:;      # 允许 Unix socket
    real_ip_header X-Forwarded-For;
    real_ip_recursive on;         # 递归获取最左侧非代理 IP

    location / {
        # X-Forwarded-For: client, proxy1, proxy2
        # recursive on 会选择 client 作为 remote_addr
        proxy_pass http://backend;
    }
}

# 不同 location 不同配置
server {
    location /api/ {
        set_real_ip_from 10.0.0.0/8;
        real_ip_header X-Real-IP;
        proxy_pass http://api_backend;
    }

    location /app/ {
        set_real_ip_from 192.168.1.0/24;
        real_ip_header X-Forwarded-For;
        proxy_pass http://app_backend;
    }
}
```

### 应用场景

- **反向代理环境**：获取客户端真实 IP 用于日志和访问控制
- **CDN 部署**：从 CDN 请求头中提取源客户端 IP
- **负载均衡器后方**：在负载均衡架构中保持真实 IP 信息
- **安全防护**：基于真实 IP 进行访问限制和防护

---

## 11. ngx_http_referer_module (Referer 防盗链模块)

### 概述

ngx_http_referer_module 模块用于基于 Referer 请求头字段过滤请求，实现简单的防盗链功能。

### 核心指令

| 指令 | 语法 | 默认值 | 说明 | 上下文 |
|------|------|--------|------|--------|
| `valid_referers` | `valid_referers none \| blocked \| server_names \| string ...;` | - | 定义有效的 Referer | http, server, location |
| `referer_hash_max_size` | `referer_hash_max_size size;` | `2048` | 哈希表最大大小 | server, location |
| `referer_hash_bucket_size` | `referer_hash_bucket_size size;` | `64` | 哈希桶大小 | server, location |

### 配置示例

```nginx
# 基本防盗链配置
location /images/ {
    valid_referers none blocked server_names
                   *.example.com example.com
                   *.example.cn;

    if ($invalid_referer) {
        return 403;
    }

    root /var/www/images;
}

# 图片防盗链（返回替代图片）
location ~* \\.(gif|jpg|jpeg|png|bmp|swf|flv)$ {
    valid_referers none blocked *.example.com example.com
                   *.google.com *.baidu.com;

    if ($invalid_referer) {
        # 返回防盗链提示图片
        rewrite ^/ /images/forbidden.png break;
    }

    root /var/www/static;
}

# 视频防盗链
location /videos/ {
    valid_referers none blocked server_names
                   *.example.com;

    if ($invalid_referer) {
        return 403 "Forbidden: Hotlinking is not allowed";
    }

    # 限制下载速度
    limit_rate 500k;

    root /var/www/videos;
}

# 复杂防盗链配置
server {
    listen 80;
    server_name file.example.com;

    location /downloads/ {
        valid_referers none blocked;
        valid_referers server_names;
        valid_referers *.example.com;
        valid_referers *.example.cn;
        valid_referers ~\\.example\\.com$;  # 正则匹配

        if ($invalid_referer) {
            # 重定向到登录页
            rewrite ^ http://example.com/login?ref=$request_uri redirect;
        }

        root /var/www/downloads;
    }
}

# 允许空 Referer（直接访问）
location /public/ {
    valid_referers none server_names;  # none 允许直接访问
    root /var/www/public;
}
```

### valid_referers 参数说明

| 参数 | 说明 |
|------|------|
| `none` | 允许 Referer 头缺失的请求（直接访问） |
| `blocked` | 允许 Referer 存在但被防火墙或代理删除的请求 |
| `server_names` | 允许 server_name 中配置的服务器名称 |
| `string` | 具体的 URL 或域名 |
| `*.example.com` | 匹配指定域名的所有子域名 |
| `~regex` | 正则表达式匹配 |

### 应用场景

- **图片防盗链**：防止其他网站直接引用图片资源
- **视频防盗链**：保护视频资源不被非法盗链
- **下载保护**：限制资源下载来源
- **流量控制**：防止带宽被外部网站消耗

---

## 12. ngx_http_secure_link_module (安全链接模块)

### 概述

ngx_http_secure_link_module 模块用于检查请求链接的真实性，保护资源不被未授权访问，通过 MD5 哈希和过期时间验证链接有效性。

### 核心指令

| 指令 | 语法 | 默认值 | 说明 | 上下文 |
|------|------|--------|------|--------|
| `secure_link` | `secure_link expression;` | - | 定义安全链接 MD5 值 | http, server, location |
| `secure_link_md5` | `secure_link_md5 expression;` | - | 定义 MD5 计算表达式 | http, server, location |
| `secure_link_secret` | `secure_link_secret word;` | - | 定义密钥（简化版） | location |

### 配置示例

```nginx
# 完整版安全链接配置
location /s/ {
    # 从请求参数获取 MD5 和过期时间
    secure_link $arg_md5,$arg_expires;
    secure_link_md5 "$secure_link_expires$uri$remote_addr secret_key";

    # $secure_link 变量值：
    # - 空字符串：链接无效
    # - "0"：链接已过期
    # - "1"：链接有效

    if ($secure_link = "") {
        return 403;
    }

    if ($secure_link = "0") {
        return 410;  # Gone，链接过期
    }

    # 链接有效，提供文件
    root /var/www/secure;
}

# 下载链接生成示例（Python）
# import hashlib
# import time
#
# secret = "secret_key"
# uri = "/s/file.pdf"
# expires = str(int(time.time()) + 3600)  # 1小时后过期
#
# md5_hash = hashlib.md5(f"{expires}{uri}127.0.0.1 {secret}".encode()).hexdigest()
# url = f"http://example.com{uri}?md5={md5_hash}&expires={expires}"

# 简化版安全链接
location /p/ {
    secure_link_secret my_secret_password;

    if ($secure_link = "") {
        return 403;
    }

    root /var/www/protected;
}

# 简化版链接格式：/p/md5_hash/filename
# MD5 生成：echo -n "filename secret" | md5sum

# 带 IP 限制的安全链接
location /download/ {
    secure_link $arg_md5,$arg_expires;
    secure_link_md5 "$secure_link_expires$uri$remote_addr my_secret";

    if ($secure_link = "") {
        return 403 "Invalid link";
    }

    if ($secure_link = "0") {
        return 410 "Link expired";
    }

    # 记录下载日志
    access_log /var/log/nginx/secure_downloads.log;

    # 限速下载
    limit_rate 1m;

    alias /var/www/downloads/;
}

# 不同路径不同密钥
location /premium/ {
    secure_link $arg_key,$arg_time;
    secure_link_md5 "$secure_link_expires$uri$remote_addr premium_secret";

    if ($secure_link != "1") {
        return 403;
    }

    root /var/www/premium;
}
```

### 应用场景

- **临时下载链接**：生成限时有效的下载链接
- **付费内容保护**：保护付费资源不被直接访问
- **会员专享资源**：为会员生成专属访问链接
- **防盗链升级**：比 referer 更安全的资源保护方案

---

## 13. ngx_http_split_clients_module (A/B 测试模块)

### 概述

ngx_http_split_clients_module 模块用于基于客户端 IP 地址或变量创建变量，实现 A/B 测试或按比例分流。

### 核心指令

| 指令 | 语法 | 默认值 | 说明 | 上下文 |
|------|------|--------|------|--------|
| `split_clients` | `split_clients string $variable { ... }` | - | 定义分流规则 | http |

### 配置示例

```nginx
# 基本 A/B 测试配置
http {
    # 基于 remote_addr 分流
    split_clients "${remote_addr}AAA" $variant {
        50%               variant_a;
        40%               variant_b;
        *                 variant_c;  # 剩余 10%
    }

    server {
        listen 80;

        location / {
            add_header X-Variant $variant;

            if ($variant = "variant_a") {
                proxy_pass http://backend_a;
                break;
            }

            if ($variant = "variant_b") {
                proxy_pass http://backend_b;
                break;
            }

            proxy_pass http://backend_c;
        }
    }
}

# 灰度发布配置
http {
    split_clients "${http_cookie}AAA" $canary {
        10%               canary;
        *                 stable;
    }

    server {
        location / {
            if ($canary = "canary") {
                proxy_pass http://canary_backend;
                break;
            }

            proxy_pass http://stable_backend;
        }
    }
}

# 多版本测试
http {
    split_clients "${remote_addr}${http_user_agent}AAA" $version {
        33.33%            v1;
        33.33%            v2;
        *                 v3;
    }

    server {
        location / {
            proxy_set_header X-Version $version;
            proxy_pass http://backend_$version;
        }
    }
}

# 与 cookie 结合实现粘性分流
http {
    # 优先检查 cookie，没有则新建
    split_clients "${remote_addr}AAA" $ab_group {
        50%               A;
        *                 B;
    }

    map $cookie_ab_group $sticky_group {
        default  $cookie_ab_group;
        ""       $ab_group;
    }

    server {
        location / {
            add_header Set-Cookie "ab_group=$sticky_group; Path=/; Max-Age=2592000" always;

            if ($sticky_group = "A") {
                proxy_pass http://backend_a;
                break;
            }

            proxy_pass http://backend_b;
        }
    }
}
```

### 分流算法说明

| 参数 | 说明 |
|------|------|
| `string` | 用于计算哈希的字符串，通常包含 `$remote_addr` |
| `percentage%` | 流量百分比（总和不能超过 100%） |
| `*` | 匹配剩余所有流量 |

### 应用场景

- **A/B 测试**：将流量分配到不同版本进行效果对比
- **灰度发布**：逐步将流量切换到新版本
- **蓝绿部署**：平滑切换生产环境
- **流量分担**：按比例分散到不同后端

---

## 14. ngx_http_ssi_module (SSI 模块)

### 概述

ngx_http_ssi_module 模块用于处理 SSI（Server Side Includes）命令，在服务器端将多个文件内容合并到响应中。

### 核心指令

| 指令 | 语法 | 默认值 | 说明 | 上下文 |
|------|------|--------|------|--------|
| `ssi` | `ssi on \| off;` | `off` | 启用 SSI 处理 | http, server, location, if in location |
| `ssi_last_modified` | `ssi_last_modified on \| off;` | `off` | 保留原始 Last-Modified | http, server, location |
| `ssi_min_file_chunk` | `ssi_min_file_chunk size;` | `1k` | 最小文件块大小 | http, server, location |
| `ssi_silent_errors` | `ssi_silent_errors on \| off;` | `off` | 静默处理错误 | http, server, location |
| `ssi_types` | `ssi_types mime-type ...;` | `text/html` | 处理 MIME 类型 | http, server, location |
| `ssi_value_length` | `ssi_value_length length;` | `256` | SSI 命令值最大长度 | http, server, location |

### 配置示例

```nginx
# 基本 SSI 配置
server {
    listen 80;

    location / {
        ssi on;
        root /var/www/html;
    }
}

# 多类型 SSI 处理
server {
    ssi on;
    ssi_types text/html application/xhtml+xml;
    ssi_silent_errors on;

    location / {
        root /var/www/html;
    }
}

# 大型页面优化
server {
    ssi on;
    ssi_min_file_chunk 10k;  # 小于 10k 的文件不单独处理

    location / {
        root /var/www/html;
    }
}

# 局部禁用 SSI
server {
    ssi on;

    location / {
        root /var/www/html;
    }

    location /no-ssi/ {
        ssi off;
        root /var/www/static;
    }
}
```

### SSI 命令示例

```html
<!--# 包含其他文件 -->
<!--#include file="header.html" -->
<!--#include virtual="/header.html" -->

<!--# 包含远程内容 -->
<!--#include virtual="/remote/content" wait="yes" -->

<!--# 设置变量 -->
<!--#set var="name" value="value" -->
<!--#set var="docroot" value="$DOCUMENT_ROOT" -->

<!--# 条件判断 -->
<!--#if expr="$name = /test/" -->
    <p>匹配成功</p>
<!--#elif expr="$name = /other/" -->
    <p>其他匹配</p>
<!--#else -->
    <p>默认内容</p>
<!--#endif -->

<!--# 循环 -->
<!--#config timefmt="%A" -->

<!--# 显示文件信息 -->
<!--#flastmod file="file.html" -->
<!--#fsize file="file.html" -->

<!--# 调用 CGI -->
<!--#exec cmd="date" -->
<!--#exec cgi="/cgi-bin/script.cgi" -->

<!--# 完整示例：页面模板 -->
<!DOCTYPE html>
<html>
<head>
    <!--#set var="title" value="页面标题" -->
    <title><!--#echo var="title" --></title>
    <!--#include virtual="/common/head.html" -->
</head>
<body>
    <!--#include virtual="/common/header.html" -->

    <main>
        <h1><!--#echo var="title" --></h1>
        <p>页面内容</p>
    </main>

    <!--#include virtual="/common/footer.html" -->
</body>
</html>
```

### SSI 变量

| 变量名 | 说明 |
|--------|------|
| `DOCUMENT_NAME` | 当前文件名 |
| `DOCUMENT_URI` | 当前 URI |
| `QUERY_STRING_UNESCAPED` | 未转义的查询字符串 |
| `DATE_LOCAL` | 本地时间 |
| `DATE_GMT` | GMT 时间 |
| `LAST_MODIFIED` | 最后修改时间 |

### 应用场景

- **页面模板化**：将公共部分（头尾）抽离为单独文件
- **动态内容嵌入**：在静态页面中嵌入动态内容
- **多语言支持**：根据条件加载不同语言内容
- **内容组合**：将多个内容源组合成一个页面

---

## 15. ngx_http_sub_module (文本替换模块)

### 概述

ngx_http_sub_module 模块用于替换响应中的指定字符串，可以在输出时动态修改内容。

### 核心指令

| 指令 | 语法 | 默认值 | 说明 | 上下文 |
|------|------|--------|------|--------|
| `sub_filter` | `sub_filter string replacement;` | - | 定义替换规则 | http, server, location |
| `sub_filter_last_modified` | `sub_filter_last_modified on \| off;` | `off` | 保留 Last-Modified | http, server, location |
| `sub_filter_once` | `sub_filter_once on \| off;` | `on` | 只替换第一次出现 | http, server, location |
| `sub_filter_types` | `sub_filter_types mime-type ...;` | `text/html` | 处理的 MIME 类型 | http, server, location |

### 配置示例

```nginx
# 基本文本替换
location / {
    sub_filter 'http://old-domain.com' 'https://new-domain.com';
    proxy_pass http://backend;
}

# 多替换规则
location / {
    sub_filter 'Welcome' '欢迎';
    sub_filter 'Login' '登录';
    sub_filter 'Logout' '退出';
    sub_filter_once off;  # 替换所有出现
    proxy_pass http://backend;
}

# 变量替换
location / {
    sub_filter 'SERVER_TIME' $time_iso8601;
    sub_filter 'REMOTE_ADDR' $remote_addr;
    sub_filter_once off;
    proxy_pass http://backend;
}

# HTML 注入
location / {
    sub_filter '</head>' '<script src="/analytics.js"></script></head>';
    sub_filter '</body>' '<footer>Copyright 2024</footer></body>';
    proxy_pass http://backend;
}

# 链接替换为绝对路径
location / {
    sub_filter 'href="/' 'href="https://cdn.example.com/';
    sub_filter 'src="/' 'src="https://cdn.example.com/';
    sub_filter_once off;
    sub_filter_types text/html text/css;
    proxy_pass http://backend;
}

# 开发环境提示
location / {
    sub_filter '<body>' '<body><div style="background:yellow;padding:10px;">开发环境</div>';
    proxy_pass http://backend;
}

# 完整示例：CDN 替换
server {
    listen 80;
    server_name cdn.example.com;

    location / {
        proxy_pass http://origin_backend;

        # 替换资源链接到 CDN
        sub_filter 'href="/static/' 'href="https://cdn.example.com/static/';
        sub_filter 'src="/static/' 'src="https://cdn.example.com/static/';
        sub_filter_once off;
        sub_filter_types text/html application/javascript text/css;

        # 保留缓存头
        sub_filter_last_modified on;
    }
}
```

### 配置参数说明

| 参数 | 说明 |
|------|------|
| `sub_filter_once on` | 只替换每个响应中的第一个匹配项 |
| `sub_filter_once off` | 替换所有匹配项 |
| `sub_filter_last_modified on` | 保留原始 Last-Modified 头（用于缓存） |
| `sub_filter_types` | 指定处理的 MIME 类型 |

### 应用场景

- **域名迁移**：批量替换旧域名为新域名
- **CDN 集成**：将资源链接替换为 CDN 地址
- **内容本地化**：替换页面中的特定文本
- **环境标识**：添加开发/测试环境标识
- **分析代码注入**：动态注入统计代码

---

## 16. ngx_http_userid_module (用户 ID 模块)

### 概述

ngx_http_userid_module 模块用于设置 cookie 以标识客户端，实现用户跟踪和会话管理。

### 核心指令

| 指令 | 语法 | 默认值 | 说明 | 上下文 |
|------|------|--------|------|--------|
| `userid` | `userid on \| v1 \| log \| off;` | `off` | 启用用户 ID | http, server, location |
| `userid_domain` | `userid_domain name \| none;` | `none` | cookie 域 | http, server, location |
| `userid_expires` | `userid_expires time \| max \| off;` | `off` | cookie 过期时间 | http, server, location |
| `userid_flags` | `userid_flags off \| flag ...;` | `none` | cookie 标志 | http, server, location |
| `userid_mark` | `userid_mark letter \| digit \| = \| off;` | `off` | 标记字符 | http, server, location |
| `userid_name` | `userid_name name;` | `UID` | cookie 名称 | http, server, location |
| `userid_p3p` | `userid_p3p string \| none;` | `none` | P3P 头 | http, server, location |
| `userid_path` | `userid_path path;` | `/` | cookie 路径 | http, server, location |
| `userid_service` | `userid_service number;` | `IP 地址最后一段` | 服务标识 | http, server, location |

### 配置示例

```nginx
# 基本用户 ID 配置
server {
    listen 80;

    userid on;
    userid_name uid;
    userid_domain example.com;
    userid_path /;
    userid_expires 365d;
}

# 完整用户跟踪配置
server {
    listen 80;
    server_name track.example.com;

    userid on;
    userid_name _uid;
    userid_domain .example.com;   # 跨子域
    userid_path /;
    userid_expires max;            # 浏览器会话
    userid_flags httponly secure;  # 安全标志

    location / {
        proxy_pass http://backend;
        proxy_set_header X-User-ID $uid_got$uid_set;
    }
}

# 仅日志记录模式
server {
    userid log;  # 不设置 cookie，只记录已有 ID

    log_format uid '$remote_addr - $uid_got [$time_local] '
                   '"$request" $status';
    access_log /var/log/nginx/uid.log uid;
}

# 多站点统一 ID
http {
    userid_name _ga;
    userid_domain .example.com;
    userid_path /;
    userid_expires 2y;

    server {
        server_name www.example.com;
        userid on;
    }

    server {
        server_name blog.example.com;
        userid on;
    }

    server {
        server_name shop.example.com;
        userid on;
    }
}

# 与日志结合
server {
    userid on;
    userid_name session_id;

    log_format tracking '$remote_addr $uid_got $request_time '
                        '"$request" $status';

    access_log /var/log/nginx/tracking.log tracking;
}
```

### Cookie 版本

| 版本 | 说明 |
|------|------|
| `v1` | 使用 Base64 编码，默认版本 |
| `on` | 同 v1 |
| `log` | 不设置新 cookie，仅记录现有 ID |
| `off` | 禁用 |

### 可用变量

| 变量名 | 说明 |
|--------|------|
| `$uid_got` | 客户端发送的 cookie 值 |
| `$uid_set` | 设置的 cookie 值 |
| `$uid_reset` | 重置 cookie 的标志 |

### 应用场景

- **用户行为跟踪**：追踪用户跨页面访问行为
- **A/B 测试**：识别用户所属的测试组
- **会话管理**：配合应用实现会话跟踪
- **访问统计**：分析用户访问模式和频次
- **广告追踪**：记录用户来源和转化

---

## 附录：模块编译与加载

### 静态编译

```bash
# 配置时启用模块
./configure \
    --with-http_ssl_module \
    --with-http_realip_module \
    --with-http_geoip_module \
    --with-http_sub_module \
    --with-http_addition_module \
    --with-http_auth_request_module \
    --with-http_secure_link_module \
    --with-http_slice_module \
    --with-http_stub_status_module

make && make install
```

### 动态模块加载

```nginx
# nginx.conf
load_module modules/ngx_http_geoip_module.so;

http {
    # 使用模块
}
```

### 检查已编译模块

```bash
# 查看编译参数
nginx -V

# 查看已加载模块
nginx -V 2>&1 | tr ' ' '\n' | grep module
```

---

## 模块组合使用示例

```nginx
# 综合应用示例：安全且可追踪的文件下载

http {
    # 真实 IP 获取
    set_real_ip_from 10.0.0.0/8;
    real_ip_header X-Forwarded-For;

    # 用户 ID 跟踪
    userid on;
    userid_name download_id;
    userid_expires 1d;

    # 地理位置
    geoip_country /usr/share/GeoIP/GeoIP.dat;

    server {
        listen 80;
        server_name download.example.com;

        # 访问控制
        location /premium/ {
            # 基础认证
            auth_basic "Premium Downloads";
            auth_basic_user_file /etc/nginx/premium.htpasswd;

            # 防盗链
            valid_referers none blocked *.example.com;
            if ($invalid_referer) {
                return 403;
            }

            # 安全链接验证
            secure_link $arg_md5,$arg_expires;
            secure_link_md5 "$secure_link_expires$uri$remote_addr secret";

            if ($secure_link = "") {
                return 403;
            }

            if ($secure_link = "0") {
                return 410;
            }

            # 日志记录
            access_log /var/log/nginx/premium_downloads.log;

            alias /var/www/premium/;
        }
    }
}
```
