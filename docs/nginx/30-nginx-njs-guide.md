# Nginx njs JavaScript 模块指南

## 目录

1. [njs 概述与特性](#1-njs-概述与特性)
2. [安装与启用](#2-安装与启用)
3. [核心指令](#3-核心指令)
4. [njs 语法基础](#4-njs-语法基础)
5. [常见应用场景与配置示例](#5-常见应用场景与配置示例)
6. [njs vs Lua 对比](#6-njs-vs-lua-对比)
7. [性能优化建议](#7-性能优化建议)

---

## 1. njs 概述与特性

### 什么是 njs

njs（nginx JavaScript）是 nginx 的一个模块，它使用 JavaScript 语言扩展 nginx 的服务器功能。njs 提供了一个嵌入式的 JavaScript 引擎，以及一个独立的命令行工具用于开发和调试。

### 主要特性

| 特性 | 说明 |
|------|------|
| **ECMAScript 5.1+ 兼容** | 遵循 ES5.1 严格模式，支持部分 ES6+ 特性 |
| **双引擎支持** | 支持原生 njs 引擎和 QuickJS 引擎 (v0.8.6+) |
| **异步支持** | 完整的 Promise 和 async/await 支持 (v0.7.0+) |
| **Fetch API** | 内置 HTTP 客户端功能 (v0.7.0+) |
| **Crypto API** | WebCrypto 和 Node.js 风格的加密支持 |
| **共享字典** | 跨 worker 进程的内存键值存储 (v0.8.0+) |
| **HTTP 和 Stream** | 同时支持 HTTP 和 TCP/UDP Stream 模块 |

### 适用场景

- **访问控制**: 复杂的安全检查逻辑
- **请求/响应处理**: 动态修改头部、响应体
- **内容生成**: 灵活的内容处理程序
- **API 网关**: JWT 验证、动态路由
- **数据处理**: 过滤和转换响应体

---

## 2. 安装与启用

### 从源码编译安装

#### 编译参数

```bash
# 下载 nginx 和 njs 源码
cd /usr/local/src
wget http://nginx.org/download/nginx-1.25.3.tar.gz
tar -xzf nginx-1.25.3.tar.gz

git clone https://github.com/nginx/njs.git
cd nginx-1.25.3

# 编译安装（动态模块方式推荐）
./configure \
    --prefix=/etc/nginx \
    --sbin-path=/usr/sbin/nginx \
    --modules-path=/usr/lib/nginx/modules \
    --with-compat \
    --add-dynamic-module=../njs/nginx

make && make install
```

#### 启用模块

在 nginx.conf 顶部添加：

```nginx
# 动态加载 njs 模块
load_module modules/ngx_http_js_module.so;

# 如果使用 Stream 模块
load_module modules/ngx_stream_js_module.so;

user nginx;
worker_processes auto;
...
```

### 使用包管理器安装

#### CentOS/RHEL

```bash
# 添加 nginx 官方仓库
sudo tee /etc/yum.repos.d/nginx.repo << 'EOF'
[nginx-stable]
name=nginx stable repo
baseurl=http://nginx.org/packages/centos/$releasever/$basearch/
gpgcheck=1
enabled=1
gpgkey=https://nginx.org/keys/nginx_signing.key
EOF

# 安装 nginx 和 njs 模块
sudo yum install nginx nginx-module-njs
```

#### Ubuntu/Debian

```bash
# 添加 nginx 官方仓库
sudo apt-get update
sudo apt-get install curl gnupg2 ca-certificates lsb-release

curl -fsSL https://nginx.org/keys/nginx_signing.key | sudo apt-key add -
sudo tee /etc/apt/sources.list.d/nginx.list << 'EOF'
deb http://nginx.org/packages/ubuntu `lsb_release -cs` nginx
deb-src http://nginx.org/packages/ubuntu `lsb_release -cs` nginx
EOF

# 安装 nginx 和 njs 模块
sudo apt-get update
sudo apt-get install nginx nginx-module-njs
```

#### Alpine Linux

```bash
apk add nginx nginx-mod-http-njs
```

### 验证安装

```bash
# 检查模块是否加载
nginx -V 2>&1 | grep njs

# 使用 njs CLI 工具验证
njs -v

# 测试 JavaScript 语法
njs -c "console.log('Hello from njs');"
```

---

## 3. 核心指令

### 指令汇总表

| 指令 | 语法 | 上下文 | 说明 |
|------|------|--------|------|
| `js_import` | `js_import module.js \| export_name from module.js;` | http, server, location | 导入 njs 模块 |
| `js_set` | `js_set $variable module.function [nocache];` | http, server, location | 设置变量处理器 |
| `js_content` | `js_content module.function;` | location, if, limit_except | 设置内容处理器 |
| `js_body_filter` | `js_body_filter module.function [buffer_type=string \| buffer];` | location, if, limit_except | 响应体过滤器 |
| `js_header_filter` | `js_header_filter module.function;` | location, if, limit_except | 响应头过滤器 |
| `js_var` | `js_var $variable [value];` | http, server, location | 声明可写变量 |
| `js_engine` | `js_engine njs \| qjs;` | http, server, location | 设置 JavaScript 引擎 |
| `js_path` | `js_path path;` | http, server, location | 设置模块搜索路径 |
| `js_shared_dict_zone` | `js_shared_dict_zone zone=name:size [timeout=time] [type=string\|number] [evict];` | http | 共享内存字典 |
| `js_periodic` | `js_periodic module.function [interval=time] [jitter=number] [worker_affinity=mask];` | location | 周期性任务 |
| `js_preload_object` | `js_preload_object name.json \| name from file.json;` | http, server, location | 预加载配置对象 |

### 指令详细说明

#### js_import

导入 JavaScript 模块文件。

```nginx
# 基本用法
js_import /etc/nginx/njs/http.js;

# 使用别名
js_import main from /etc/nginx/njs/http.js;

# 导入特定导出
js_import {hello, api} from /etc/nginx/njs/utils.js;
```

#### js_set

使用 JavaScript 函数设置 nginx 变量。

```nginx
# 基本用法
js_set $foo http.foo;

# 不缓存模式（每次引用都执行）
js_set $dynamic http.dynamic_handler nocache;
```

#### js_content

将 JavaScript 函数设置为 location 的内容处理器。

```nginx
location /api {
    js_content api.handleRequest;
}
```

#### js_body_filter

设置响应体过滤器函数，用于修改响应内容。

```nginx
location / {
    proxy_pass http://backend;
    js_body_filter http.modifyBody;
}
```

#### js_engine

选择 JavaScript 引擎（njs 或 QuickJS）。

```nginx
http {
    # 使用 QuickJS 引擎（ES2023 支持）
    js_engine qjs;
    
    # 或使用原生 njs 引擎
    js_engine njs;
}
```

---

## 4. njs 语法基础

### ECMAScript 兼容性

njs 遵循 **ECMAScript 5.1 (严格模式)**，并支持部分 ES6+ 扩展。

### 支持的 ES6+ 特性

| 特性 | 版本 | 示例 |
|------|------|------|
| `let` / `const` | 0.6.0+ | `const x = 10; let y = 20;` |
| 箭头函数 | 0.3.1+ | `(a, b) => a + b` |
| 模板字符串 | 0.3.2+ | `` `Hello ${name}` `` |
| async/await | 0.7.0+ | `async function() { await ... }` |
| Promise | 0.3.8+ | `Promise.all()`, `.then()` |
| ES6 模块 | 0.3.0+ | `export default {...}` |
| 可选链操作符 | 0.9.6+ | `obj?.property` |
| 逻辑赋值 | 0.9.6+ | `a ||= b`, `a &&= b` |

### 不支持的特性

| 特性 | 说明 |
|------|------|
| 类（Classes） | 不支持 `class` 关键字 |
| 解构赋值 | `const {a, b} = obj` 不支持 |
| 展开运算符 | `...` 展开语法不支持 |
| 默认参数 | `function(a=1)` 不支持 |
| 生成器函数 | `function*`, `yield` 不支持 |
| Proxy/Reflect | 不支持 |
| Map/Set | 不支持原生 Map/Set |

### 请求对象（r）API

HTTP 处理函数接收请求对象 `r`，包含以下属性和方法：

#### 属性

| 属性 | 类型 | 说明 |
|------|------|------|
| `r.method` | string | HTTP 方法 (GET, POST 等) |
| `r.uri` | string | 请求 URI |
| `r.httpVersion` | string | HTTP 版本 |
| `r.remoteAddress` | string | 客户端 IP 地址 |
| `r.headersIn` | object | 请求头（只读） |
| `r.headersOut` | object | 响应头（可写） |
| `r.args` | object | URL 查询参数 |
| `r.variables` | object | nginx 变量 |
| `r.requestText` | string | 请求体文本 |
| `r.requestBuffer` | Buffer | 请求体 Buffer |
| `r.status` | number | 响应状态码 |

#### 方法

| 方法 | 说明 |
|------|------|
| `r.return(status[, body])` | 返回响应 |
| `r.send(data)` | 发送响应体片段 |
| `r.sendHeader()` | 发送响应头 |
| `r.finish()` | 完成响应 |
| `r.log(msg)` | 记录信息日志 |
| `r.error(msg)` | 记录错误日志 |
| `r.subrequest(uri[, opts[, cb]])` | 发起子请求 |
| `r.internalRedirect(uri)` | 内部重定向 |

### ngx 全局对象

| 属性/方法 | 说明 |
|-----------|------|
| `ngx.fetch(url[, opts])` | Fetch API 请求 |
| `ngx.log(level, msg)` | 写入错误日志 |
| `ngx.version` | nginx 版本 |
| `ngx.worker_id` | Worker 进程 ID |
| `ngx.shared.<zone>` | 共享字典访问 |

### 内置模块

```javascript
// 文件系统模块 (v0.8.9+)
import fs from 'fs';
const data = fs.readFileSync('/path/to/file');

// 加密模块 (v0.7.0+)
import crypto from 'crypto';
const hash = crypto.createHash('sha256');

// Buffer 模块
import { Buffer } from 'buffer';
const buf = Buffer.from('hello');

// 查询字符串
import qs from 'querystring';
const obj = qs.parse('a=1&b=2');
```

---

## 5. 常见应用场景与配置示例

### 示例 1: 动态响应生成

创建一个简单的 "Hello World" 端点。

**JavaScript 文件 (`/etc/nginx/njs/hello.js`):**

```javascript
function hello(r) {
    r.return(200, "Hello world!\n");
}

function personalizedHello(r) {
    const name = r.args.name || "Guest";
    r.return(200, `Hello, ${name}!\n`);
}

function jsonResponse(r) {
    const data = {
        method: r.method,
        uri: r.uri,
        headers: r.headersIn,
        remoteAddress: r.remoteAddress
    };
    r.headersOut['Content-Type'] = 'application/json';
    r.return(200, JSON.stringify(data, null, 2));
}

export default { hello, personalizedHello, jsonResponse };
```

**nginx 配置:**

```nginx
load_module modules/ngx_http_js_module.so;

events {}

http {
    js_import /etc/nginx/njs/hello.js;
    
    server {
        listen 80;
        server_name example.com;
        
        # 简单的 Hello World
        location /hello {
            js_content hello.hello;
        }
        
        # 个性化问候
        location /greet {
            js_content hello.personalizedHello;
        }
        
        # JSON 响应
        location /info {
            js_content hello.jsonResponse;
        }
    }
}
```

### 示例 2: 请求头处理

修改请求头和响应头。

**JavaScript 文件 (`/etc/nginx/njs/headers.js`):**

```javascript
// 添加自定义响应头
function addCustomHeaders(r) {
    r.headersOut['X-Powered-By'] = 'njs';
    r.headersOut['X-Request-ID'] = r.variables.request_id;
    r.headersOut['X-Processed-At'] = new Date().toISOString();
    
    // 继续处理到上游
    r.internalRedirect('@proxy');
}

// 响应头过滤器
function modifyResponseHeaders(r) {
    // 移除敏感头
    delete r.headersOut['Server'];
    delete r.headersOut['X-Powered-By'];
    
    // 添加安全头
    r.headersOut['X-Frame-Options'] = 'SAMEORIGIN';
    r.headersOut['X-Content-Type-Options'] = 'nosniff';
    r.headersOut['Referrer-Policy'] = 'strict-origin-when-cross-origin';
}

// 基于请求头的路由
function routeByHeader(r) {
    const apiVersion = r.headersIn['X-API-Version'];
    
    if (apiVersion === 'v2') {
        r.internalRedirect('@api_v2');
    } else if (apiVersion === 'v1') {
        r.internalRedirect('@api_v1');
    } else {
        r.return(400, JSON.stringify({ error: 'Invalid API version' }));
    }
}

// 设置变量用于日志
function getClientInfo(r) {
    const userAgent = r.headersIn['User-Agent'] || 'unknown';
    const device = userAgent.match(/Mobile|Android|iPhone/i) ? 'mobile' : 'desktop';
    return device;
}

export default { 
    addCustomHeaders, 
    modifyResponseHeaders, 
    routeByHeader,
    getClientInfo 
};
```

**nginx 配置:**

```nginx
http {
    js_import /etc/nginx/njs/headers.js;
    
    # 使用 js_set 设置变量
    js_set $device_type headers.getClientInfo;
    
    log_format custom '$remote_addr - $device_type - "$request" ' 
                      '$status $body_bytes_sent';
    
    server {
        listen 80;
        
        # 添加自定义头并代理
        location /api {
            js_content headers.addCustomHeaders;
        }
        
        location @proxy {
            proxy_pass http://backend;
            js_header_filter headers.modifyResponseHeaders;
        }
        
        # 基于 API 版本路由
        location / {
            js_content headers.routeByHeader;
        }
        
        location @api_v1 {
            proxy_pass http://api-v1-backend;
        }
        
        location @api_v2 {
            proxy_pass http://api-v2-backend;
        }
    }
}
```

### 示例 3: JWT 验证（简化版）

实现简单的 JWT token 验证。

**JavaScript 文件 (`/etc/nginx/njs/jwt.js`):**

```javascript
import crypto from 'crypto';

// Base64URL 解码
function base64UrlDecode(str) {
    // 添加标准 Base64 填充
    const padding = '='.repeat((4 - str.length % 4) % 4);
    const base64 = str.replace(/-/g, '+').replace(/_/g, '/') + padding;
    return Buffer.from(base64, 'base64').toString('utf8');
}

// 简单的 JWT 验证（仅验证签名格式，生产环境请使用完整实现）
function verifyJwt(r) {
    const authHeader = r.headersIn['Authorization'];
    
    if (!authHeader || !authHeader.startsWith('Bearer ')) {
        r.return(401, JSON.stringify({ error: 'Missing or invalid Authorization header' }));
        return;
    }
    
    const token = authHeader.substring(7);
    const parts = token.split('.');
    
    if (parts.length !== 3) {
        r.return(401, JSON.stringify({ error: 'Invalid JWT format' }));
        return;
    }
    
    try {
        const payload = JSON.parse(base64UrlDecode(parts[1]));
        
        // 检查过期时间
        if (payload.exp && payload.exp < Date.now() / 1000) {
            r.return(401, JSON.stringify({ error: 'Token expired' }));
            return;
        }
        
        // 在变量中存储用户信息
        r.variables.jwt_sub = payload.sub || '';
        r.variables.jwt_role = payload.role || 'user';
        
        // 继续处理
        r.internalRedirect('@protected');
        
    } catch (e) {
        r.return(401, JSON.stringify({ error: 'Invalid token payload' }));
    }
}

// 检查权限
function checkRole(r, requiredRole) {
    const userRole = r.variables.jwt_role || '';
    return userRole === requiredRole;
}

export default { verifyJwt, checkRole };
```

**nginx 配置:**

```nginx
http {
    js_import /etc/nginx/njs/jwt.js;
    
    # 声明变量
    js_var $jwt_sub;
    js_var $jwt_role;
    
    server {
        listen 80;
        
        # 公开端点
        location /public {
            proxy_pass http://backend;
        }
        
        # 需要 JWT 验证的端点
        location /protected {
            js_content jwt.verifyJwt;
        }
        
        location @protected {
            proxy_pass http://backend;
            proxy_set_header X-User-ID $jwt_sub;
            proxy_set_header X-User-Role $jwt_role;
        }
        
        # 管理员端点
        location /admin {
            js_content jwt.verifyJwt;
        }
        
        location @admin {
            # 检查角色
            if ($jwt_role != 'admin') {
                return 403 "Forbidden";
            }
            proxy_pass http://admin-backend;
        }
    }
}
```

### 示例 4: 动态路由

根据请求参数动态选择上游。

**JavaScript 文件 (`/etc/nginx/njs/router.js`):**

```javascript
// 基于地理位置的路由
function routeByGeo(r) {
    const country = r.variables.geoip_country_code || 'US';
    
    const regionMap = {
        'CN': '@asia_backend',
        'JP': '@asia_backend',
        'KR': '@asia_backend',
        'DE': '@eu_backend',
        'FR': '@eu_backend',
        'UK': '@eu_backend',
        'US': '@us_backend',
        'CA': '@us_backend'
    };
    
    const target = regionMap[country] || '@default_backend';
    r.internalRedirect(target);
}

// 基于请求体的路由（用于 webhooks）
function routeByPayload(r) {
    try {
        const body = JSON.parse(r.requestText || '{}');
        const eventType = body.event || 'unknown';
        
        const routeMap = {
            'payment.success': '@payment_success',
            'payment.failed': '@payment_failed',
            'user.created': '@user_created',
            'user.deleted': '@user_deleted'
        };
        
        const target = routeMap[eventType] || '@default_webhook';
        r.internalRedirect(target);
        
    } catch (e) {
        r.return(400, JSON.stringify({ error: 'Invalid JSON' }));
    }
}

// 基于负载的路由（选择最空闲的上游）
function routeByLoad(r) {
    const upstreams = ['backend1', 'backend2', 'backend3'];
    let selected = upstreams[0];
    let minConnections = Number.MAX_SAFE_INTEGER;
    
    for (const upstream of upstreams) {
        // 使用共享字典存储连接数
        const connections = ngx.shared.load_stats.get(upstream) || 0;
        if (connections < minConnections) {
            minConnections = connections;
            selected = upstream;
        }
    }
    
    // 增加计数
    ngx.shared.load_stats.incr(selected, 1, 0, 60);
    
    r.variables.target_upstream = selected;
    r.internalRedirect('@dynamic_proxy');
}

// A/B 测试路由
function routeABTest(r) {
    const cookie = r.headersIn['Cookie'] || '';
    const variantMatch = cookie.match(/ab_variant=(\w+)/);
    let variant = variantMatch ? variantMatch[1] : null;
    
    if (!variant) {
        // 50/50 分流
        variant = Math.random() < 0.5 ? 'a' : 'b';
        // 设置 cookie
        r.headersOut['Set-Cookie'] = `ab_variant=${variant}; Path=/; Max-Age=86400`;
    }
    
    if (variant === 'a') {
        r.internalRedirect('@variant_a');
    } else {
        r.internalRedirect('@variant_b');
    }
}

export default { 
    routeByGeo, 
    routeByPayload, 
    routeByLoad, 
    routeABTest 
};
```

**nginx 配置:**

```nginx
http {
    js_import /etc/nginx/njs/router.js;
    
    # 配置共享字典
    js_shared_dict_zone zone=load_stats:1M type=number;
    
    # GeoIP 模块（可选）
    geoip_country /usr/share/GeoIP/GeoIP.dat;
    
    upstream backend1 {
        server 10.0.1.10:8080;
    }
    
    upstream backend2 {
        server 10.0.1.11:8080;
    }
    
    upstream backend3 {
        server 10.0.1.12:8080;
    }
    
    server {
        listen 80;
        
        # 地理位置路由
        location / {
            js_content router.routeByGeo;
        }
        
        # Webhook 路由
        location /webhooks {
            js_content router.routeByPayload;
        }
        
        # A/B 测试
        location /experiment {
            js_content router.routeABTest;
        }
        
        # 后端定义
        location @asia_backend {
            proxy_pass http://asia-cluster;
        }
        
        location @eu_backend {
            proxy_pass http://eu-cluster;
        }
        
        location @us_backend {
            proxy_pass http://us-cluster;
        }
        
        location @default_backend {
            proxy_pass http://default-cluster;
        }
    }
}
```

### 示例 5: 响应体修改

使用 body filter 修改响应内容。

**JavaScript 文件 (`/etc/nginx/njs/body_filter.js`):**

```javascript
// 简单的文本替换过滤器
function replaceText(r, data, flags) {
    if (data.length > 0) {
        // 替换敏感信息
        let modified = data.toString().replace(
            /\b\d{4}[\s-]?\d{4}[\s-]?\d{4}[\s-]?\d{4}\b/g,
            '****-****-****-****'
        );
        
        // 替换邮箱
        modified = modified.replace(
            /\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Z|a-z]{2,}\b/g,
            '***@***.***'
        );
        
        r.sendBuffer(modified, flags);
    } else {
        r.sendBuffer(data, flags);
    }
}

// 添加内容到响应体
function appendContent(r, data, flags) {
    if (flags.last) {
        // 在最后添加内容
        const append = '\n<!-- Processed by njs -->';
        r.sendBuffer(data + append, flags);
    } else {
        r.sendBuffer(data, flags);
    }
}

// JSON 数据修改
function modifyJson(r, data, flags) {
    if (data.length > 0) {
        try {
            const obj = JSON.parse(data.toString());
            
            // 添加服务器信息
            obj._meta = {
                processed_by: 'nginx-njs',
                timestamp: Date.now()
            };
            
            // 移除敏感字段
            delete obj.password;
            delete obj.secret_key;
            delete obj.internal_notes;
            
            r.sendBuffer(JSON.stringify(obj), flags);
        } catch (e) {
            // JSON 解析失败，原样返回
            r.sendBuffer(data, flags);
        }
    } else {
        r.sendBuffer(data, flags);
    }
}

// HTML 注入（添加分析脚本）
function injectAnalytics(r, data, flags) {
    if (data.length > 0) {
        let html = data.toString();
        
        if (flags.last && html.includes('</body>')) {
            const analytics = `
<script>
(function() {
    console.log('Page loaded: ' + location.pathname);
    // 发送分析数据...
})();
</script>
`;
            html = html.replace('</body>', analytics + '</body>');
        }
        
        r.sendBuffer(html, flags);
    } else {
        r.sendBuffer(data, flags);
    }
}

export default { 
    replaceText, 
    appendContent, 
    modifyJson, 
    injectAnalytics 
};
```

**nginx 配置:**

```nginx
http {
    js_import /etc/nginx/njs/body_filter.js;
    
    server {
        listen 80;
        
        # 敏感信息脱敏
        location /api/users {
            proxy_pass http://backend;
            js_body_filter body_filter.replaceText;
        }
        
        # JSON API 修改
        location /api/data {
            proxy_pass http://backend;
            js_body_filter body_filter.modifyJson;
        }
        
        # HTML 页面注入
        location / {
            proxy_pass http://backend;
            js_body_filter body_filter.injectAnalytics;
        }
    }
}
```

### 示例 6: 使用 Fetch API 的请求聚合

将多个后端请求合并为一个响应。

**JavaScript 文件 (`/etc/nginx/njs/aggregate.js`):**

```javascript
async function aggregateUserData(r) {
    const userId = r.args.userId;
    
    if (!userId) {
        r.return(400, JSON.stringify({ error: 'Missing userId' }));
        return;
    }
    
    try {
        // 并行发起多个请求
        const [profileRes, ordersRes, preferencesRes] = await Promise.all([
            ngx.fetch(`http://user-service/users/${userId}`),
            ngx.fetch(`http://order-service/orders?userId=${userId}`),
            ngx.fetch(`http://preference-service/preferences/${userId}`)
        ]);
        
        // 解析所有响应
        const [profile, orders, preferences] = await Promise.all([
            profileRes.json(),
            ordersRes.json(),
            preferencesRes.json()
        ]);
        
        // 合并数据
        const result = {
            user: profile,
            orders: orders,
            preferences: preferences,
            aggregatedAt: new Date().toISOString()
        };
        
        r.headersOut['Content-Type'] = 'application/json';
        r.return(200, JSON.stringify(result, null, 2));
        
    } catch (e) {
        r.return(500, JSON.stringify({ error: 'Failed to aggregate data', message: e.message }));
    }
}

// 带缓存的请求
async function cachedFetch(r) {
    const cacheKey = 'api:' + r.uri;
    const cached = ngx.shared.api_cache.get(cacheKey);
    
    if (cached) {
        r.headersOut['X-Cache'] = 'HIT';
        r.return(200, cached);
        return;
    }
    
    try {
        const response = await ngx.fetch('http://backend' + r.uri);
        const body = await response.text();
        
        // 缓存 60 秒
        ngx.shared.api_cache.set(cacheKey, body, 60000);
        
        r.headersOut['X-Cache'] = 'MISS';
        r.return(response.status, body);
        
    } catch (e) {
        r.return(502, JSON.stringify({ error: 'Backend unavailable' }));
    }
}

export default { aggregateUserData, cachedFetch };
```

**nginx 配置:**

```nginx
http {
    js_import /etc/nginx/njs/aggregate.js;
    
    # 配置共享字典用于缓存
    js_shared_dict_zone zone=api_cache:10M type=string evict;
    
    server {
        listen 80;
        
        # 数据聚合端点
        location /api/aggregate {
            js_content aggregate.aggregateUserData;
        }
        
        # 带缓存的代理
        location /api/ {
            js_content aggregate.cachedFetch;
        }
    }
}
```

---

## 6. njs vs Lua 对比

| 特性 | njs (JavaScript) | Lua (ngx_lua) |
|------|------------------|---------------|
| **语言流行度** | 广泛，前后端通用 | 游戏/嵌入式领域为主 |
| **学习曲线** | 低（开发者熟悉） | 中等（需学习新语言） |
| **异步支持** | 原生 Promise/async-await | 协程 (coroutine) |
| **模块生态** | Node.js 部分兼容 | LuaRocks 生态 |
| **JSON 处理** | 原生支持 | 需 cjson 库 |
| **正则表达式** | 原生支持 | 模式匹配（不同语法） |
| **调试工具** | njs CLI 工具 | 有限 |
| **内存管理** | 自动垃圾回收 | 自动垃圾回收 |
| **执行引擎** | njs/QuickJS | LuaJIT/PUC-Rio |
| **HTTP 客户端** | 内置 Fetch API | 需 cosocket |
| **共享内存** | js_shared_dict_zone | ngx.shared.DICT |
| **子请求** | r.subrequest() | ngx.location.capture |

### 选择建议

| 场景 | 推荐方案 |
|------|----------|
| 团队熟悉 JavaScript | njs |
| 需要复杂异步逻辑 | njs（async/await 更清晰） |
| 已有 Lua 代码库 | 继续使用 Lua |
| 极致性能要求 | LuaJIT（可能更快） |
| 前后端代码共享 | njs |
| OpenResty 生态依赖 | Lua |

---

## 7. 性能优化建议

### 1. 使用 QuickJS 引擎

QuickJS 引擎在某些场景下性能更好，支持 ES2023。

```nginx
http {
    js_engine qjs;
}
```

### 2. 合理配置 Context 重用

```nginx
http {
    # 调整 QuickJS context 池大小（默认 128）
    js_context_reuse 256;
}
```

### 3. 使用缓存

```javascript
// 在 njs 中缓存计算结果
const cache = {};

function cachedOperation(r) {
    const key = r.args.key;
    
    if (cache[key]) {
        return cache[key];
    }
    
    const result = expensiveComputation(key);
    cache[key] = result;
    return result;
}
```

### 4. 使用共享字典

```nginx
# 配置足够大的共享内存
js_shared_dict_zone zone=my_cache:100M type=string timeout=300s evict;
```

### 5. 避免同步阻塞

```javascript
// 使用异步操作
async function good(r) {
    const result = await ngx.fetch('http://backend');
    r.return(200, await result.text());
}

// 避免同步文件操作
import fs from 'fs';
// 仅使用同步方法，因为 njs 在 nginx 中不支持异步文件操作
```

### 6. 过滤器性能注意

```javascript
// js_body_filter 和 js_header_filter 只支持同步操作
function filter(r, data, flags) {
    // 只能使用同步操作
    r.sendBuffer(data.toLowerCase(), flags);
    
    // 以下操作不支持：
    // await r.subrequest(...)
    // setTimeout(...)
}
```

### 7. 预加载配置对象

```nginx
# 预加载配置到内存
js_preload_object config.json;
js_preload_object api_keys from /etc/nginx/secrets/keys.json;
```

```javascript
// 在 JavaScript 中访问
function handler(r) {
    const config = ngx.conf.config;
    const keys = ngx.conf.api_keys;
}
```

### 8. 调整 Fetch API 缓冲区

```nginx
http {
    # 增加 Fetch API 缓冲区
    js_fetch_buffer_size 32k;
    js_fetch_max_response_buffer_size 10m;
    js_fetch_timeout 30s;
    
    # 启用连接池
    js_fetch_keepalive 32;
    js_fetch_keepalive_timeout 60s;
}
```

### 9. 监控和调试

```javascript
// 使用定时器监控性能
function monitoredHandler(r) {
    const start = Date.now();
    
    // 处理逻辑...
    
    const duration = Date.now() - start;
    r.headersOut['X-Response-Time'] = duration + 'ms';
    ngx.log(ngx.INFO, `Request processed in ${duration}ms`);
}

// 内存统计（仅限 CLI）
console.log(njs.memoryStats);
```

### 10. 代码组织最佳实践

```javascript
// 模块化组织代码
// utils.js
function logRequest(r) {
    ngx.log(ngx.INFO, `${r.method} ${r.uri}`);
}

function sanitizeInput(input) {
    return input.replace(/[<>]/g, '');
}

export default { logRequest, sanitizeInput };

// handlers.js
import utils from 'utils.js';

function handleApiRequest(r) {
    utils.logRequest(r);
    // ...
}

export default { handleApiRequest };
```

---

## 附录：快速参考

### 常用代码片段

```javascript
// 1. 读取请求体
const body = r.requestText;
const json = JSON.parse(body);

// 2. 设置响应头
r.headersOut['X-Custom'] = 'value';
r.headersOut['Content-Type'] = 'application/json';

// 3. 获取查询参数
const param = r.args.name;
const allParams = r.args; // 对象

// 4. 获取请求头
const auth = r.headersIn['Authorization'];

// 5. 子请求
const reply = await r.subrequest('/internal/api');
r.return(200, reply.responseText);

// 6. 外部 HTTP 请求
const response = await ngx.fetch('https://api.example.com/data');
const data = await response.json();

// 7. 日志记录
r.log('Info message');
r.warn('Warning message');
r.error('Error message');
ngx.log(ngx.INFO, 'Nginx log');

// 8. 共享字典操作
const dict = ngx.shared.myDict;
dict.set('key', 'value', 60000); // 60秒 TTL
const val = dict.get('key');
dict.incr('counter', 1, 0);

// 9. Base64 编码/解码
const encoded = btoa('hello');
const decoded = atob(encoded);

// 10. 哈希计算
import crypto from 'crypto';
const hash = crypto.createHash('sha256').update('data').digest('hex');
```

### 版本要求

| 功能 | 最低版本 |
|------|----------|
| 基本 HTTP 模块 | 0.4.0 |
| Stream 模块 | 0.4.4 |
| Fetch API | 0.7.0 |
| WebCrypto | 0.7.0 |
| async/await | 0.7.0 |
| 共享字典 | 0.8.0 |
| QuickJS 引擎 | 0.8.6 |
| fs 模块 | 0.8.9 |
| Fetch keepalive | 0.9.2 |

### 官方资源

- [njs 官方文档](https://nginx.org/en/docs/njs/)
- [ngx_http_js_module](https://nginx.org/en/docs/http/ngx_http_js_module.html)
- [GitHub 仓库](https://github.com/nginx/njs)
- [示例代码](https://github.com/nginx/njs-examples/)
