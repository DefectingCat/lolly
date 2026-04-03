# NGINX 动态配置与服务发现指南

## 1. 动态配置概述

### 为什么需要动态配置

传统 NGINX 配置是静态的，修改配置需要重载甚至重启服务。在微服务架构和云原生环境中，这种静态模式面临挑战：

- **服务实例频繁变更**：容器化部署中 Pod 动态扩缩容
- **配置变更频繁**：路由规则、权重、限流策略需要实时调整
- **零 downtime 要求**：传统 reload 会导致连接中断
- **多环境管理**：开发、测试、生产环境配置快速切换

### 动态配置核心能力

| 能力 | 说明 | 适用场景 |
|------|------|----------|
| **动态 Upstream** | 运行时修改后端服务器列表 | 服务发现、蓝绿部署 |
| **动态 SSL** | 运行时加载/更新证书 | 多租户、自动化证书管理 |
| **动态路由** | 基于外部数据的路由决策 | A/B 测试、灰度发布 |
| **配置热重载** | 不中断服务的配置更新 | 日常配置变更 |

### 动态配置方案对比

| 方案 | 实现方式 | 优点 | 缺点 |
|------|----------|------|------|
| **DNS 服务发现** | resolver + 域名解析 | 原生支持，无需模块 | TTL 延迟，无法精细控制 |
| **dyups 模块** | 通过 HTTP API 修改 upstream | 精确控制，即时生效 | 需要第三方模块 |
| **Lua 脚本** | OpenResty + Lua | 灵活性高 | 依赖 OpenResty |
| **NJS + 外部存储** | JavaScript + etcd/Consul | 官方支持，现代化 | 需要编写脚本逻辑 |
| **nginx-unit** | 动态应用服务器 | API 驱动，语言无关 | 架构不同，迁移成本高 |

---

## 2. DNS 动态服务发现

### resolver 指令详解

NGINX 内置 DNS 解析支持，通过 `resolver` 实现基于域名的动态服务发现。

```nginx
http {
    # 配置 DNS 服务器
    resolver 10.0.0.1 10.0.0.2 valid=300s ipv6=off;
    resolver_timeout 5s;

    upstream backend {
        # 使用域名，NGINX 会按 TTL 重新解析
        server api.example.com:8080 resolve;
        server api-backup.example.com:8080 backup;
    }
}
```

### resolver 参数说明

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `address` | DNS 服务器地址，可配置多个 | - |
| `valid=time` | 覆盖 DNS 返回的 TTL | DNS TTL |
| `ipv6=on/off` | 是否解析 IPv6 地址 | on |
| `status_zone=zone` | 启用 DNS 查询统计 (Plus) | - |

### server 的 resolve 参数

```nginx
upstream backend {
    zone backend 64k;                    # 必需：共享内存区
    resolver 10.0.0.1 valid=10s;         # 可选：独立的 resolver

    server api.example.com resolve;      # 监控域名解析变化
    server 192.168.1.1;
}
```

**注意**：使用 `resolve` 需要：
1. 配置 `zone` 共享内存区
2. NGINX Plus 或 1.11.3+ 商业版本/开源版本（某些功能受限）

### 动态域名配置示例

```nginx
http {
    resolver 8.8.8.8 8.8.4.4 valid=60s;

    server {
        listen 80;
        server_name ~^(?<subdomain>.+)\.example\.com$;

        location / {
            # 动态目标地址
            proxy_pass http://$subdomain.internal;
            proxy_set_header Host $host;
        }
    }
}
```

---

## 3. 使用 etcd/Consul 进行服务发现

### 架构设计

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│   NGINX     │────▶│  NJS/Consul │────▶│   Consul    │
│             │     │   Template  │     │   /etcd     │
│             │     │             │     │             │
│  proxy_pass │◀────│  upstream   │◀────│ 服务注册中心 │
└─────────────┘     └─────────────┘     └─────────────┘
                            │
                            ▼
                     ┌─────────────┐
                     │  微服务集群  │
                     └─────────────┘
```

### 方案一：NJS + Consul API

使用 NJS 模块从 Consul 获取服务列表并动态路由。

```javascript
// consul.js - NJS 脚本
function discover_backend(r) {
    // Consul HTTP API 查询服务
    var service = r.variables.arg_service || 'web';
    var consul_url = 'http://consul:8500/v1/health/service/' + service;

    r.subrequest('/_consul_query', {
        method: 'GET',
        args: 'url=' + encodeURIComponent(consul_url)
    }, function(reply) {
        if (reply.status !== 200) {
            r.return(502, 'Service discovery failed');
            return;
        }

        var services = JSON.parse(reply.body);
        if (services.length === 0) {
            r.return(503, 'No healthy instances');
            return;
        }

        // 选择第一个健康实例
        var instance = services[0].Service;
        var target = instance.Address + ':' + instance.Port;

        // 内部重写到实际地址
        r.internalRedirect('/_proxy/' + target);
    });
}

export default { discover_backend };
```

```nginx
# nginx.conf
load_module modules/ngx_http_js_module.so;

http {
    js_import /etc/nginx/consul.js;
    js_set $backend_target consul.discover_backend;

    # Consul 查询代理
    location /_consul_query {
        internal;
        proxy_pass $arg_url;
        proxy_connect_timeout 2s;
        proxy_read_timeout 2s;
    }

    # 动态代理入口
    location /api/ {
        js_content consul.discover_backend;
    }

    # 实际代理位置
    location ~ ^/_proxy/(?<addr>.+)$ {
        internal;
        proxy_pass http://$addr;
        proxy_set_header Host $host;
    }
}
```

### 方案二：confd 模板渲染

confd 监听 etcd/Consul 变更，自动渲染 NGINX 配置并 reload。

```toml
# /etc/confd/conf.d/nginx.toml
[template]
src = "nginx.tmpl"
dest = "/etc/nginx/conf.d/upstreams.conf"
keys = [
  "/services/web/*",
  "/services/api/*"
]
reload_cmd = "/usr/sbin/nginx -s reload"
```

```
# /etc/confd/templates/nginx.tmpl
{{range $service := lsdir "/services"}}
upstream {{base $service}} {
    {{$servers := getvs (printf "/services/%s/*" $service)}}
    {{range $server := $servers}}
    server {{$server}};
    {{end}}
}
{{end}}

server {
    listen 80;
    {{range $service := lsdir "/services"}}
    location /{{base $service}}/ {
        proxy_pass http://{{base $service}};
    }
    {{end}}
}
```

### 方案三：Consul Template

HashiCorp 官方工具，专用于 Consul 集成。

```hcl
# template.ctmpl
{{range service "web"}}
server {{.Address}}:{{.Port}};{{end}}
```

```bash
# 启动 consul-template
consul-template \
  -consul-addr=consul:8500 \
  -template="template.ctmpl:/etc/nginx/conf.d/web.conf:/usr/sbin/nginx -s reload"
```

---

## 4. dyups 模块使用

### 模块简介

dyups（Dynamic Upstream）是淘宝开源的 NGINX 模块，提供 HTTP API 动态管理 upstream。

**功能特性**：
- 运行时添加、删除、修改 upstream
- 无需 reload 即可更新后端服务器
- 支持查看当前 upstream 状态

### 安装编译

```bash
# 下载模块
git clone https://github.com/yzprofile/ngx_http_dyups_module.git

# 编译 NGINX 时添加模块
cd nginx-1.24.0
./configure \
    --add-module=/path/to/ngx_http_dyups_module \
    --with-http_ssl_module
make && make install
```

### 基础配置

```nginx
http {
    # 加载 dyups 模块
    dyups_shm_zone_size 10m;           # 共享内存大小

    # dyups API 接口（需要限制访问）
    server {
        listen 127.0.0.1:8081;
        server_name dyups_admin;

        location / {
            dyups_interface;            # 启用 dyups 接口
        }
    }

    # 初始 upstream 定义（可为空）
    upstream backend {
        server 127.0.0.1:8080;          # 占位服务器
    }

    server {
        listen 80;
        location / {
            proxy_pass http://backend;
        }
    }
}
```

### HTTP API 详解

#### 更新/添加 Upstream

```bash
# 更新整个 upstream（替换原有配置）
curl -X POST \
    -H "Content-Type: text/plain" \
    -d "server 192.168.1.1:8080 weight=5;
server 192.168.1.2:8080;
server 192.168.1.3:8080 backup;" \
    http://127.0.0.1:8081/upstream/backend
```

#### 删除 Upstream

```bash
# 删除指定 upstream
curl -X DELETE http://127.0.0.1:8081/upstream/backend
```

#### 查询 Upstream 状态

```bash
# 获取所有 upstream 列表
curl http://127.0.0.1:8081/list

# 获取特定 upstream 详情
curl http://127.0.0.1:8081/detail
```

**返回示例**：
```json
{
  "backend": [
    {
      "server": "192.168.1.1:8080",
      "weight": 5,
      "max_fails": 1,
      "fail_timeout": "10s"
    },
    {
      "server": "192.168.1.2:8080",
      "weight": 1,
      "backup": true
    }
  ]
}
```

### 完整动态配置示例

```nginx
http {
    dyups_shm_zone_size 20m;

    # 管理接口（严格限制访问）
    server {
        listen 127.0.0.1:8081;

        # 只允许本地访问
        allow 127.0.0.1;
        deny all;

        location / {
            dyups_interface;
        }
    }

    # 对外服务
    server {
        listen 80;
        server_name api.example.com;

        location / {
            proxy_pass http://dynamic_backend;
            proxy_set_header Host $host;
            proxy_set_header X-Real-IP $remote_addr;

            # 连接超时设置
            proxy_connect_timeout 5s;
            proxy_send_timeout 10s;
            proxy_read_timeout 30s;
        }
    }
}
```

```bash
# 服务注册脚本
#!/bin/bash

UPSTREAM_NAME="dynamic_backend"
ADMIN_URL="http://127.0.0.1:8081"

# 从服务发现获取实例列表
SERVERS=$(curl -s http://consul:8500/v1/health/service/web | \
    jq -r '.[].Service | "server \(.Address):\(.Port);"')

# 更新 NGINX upstream
curl -X POST \
    -H "Content-Type: text/plain" \
    -d "$SERVERS" \
    "$ADMIN_URL/upstream/$UPSTREAM_NAME"

echo "Upstream updated: $UPSTREAM_NAME"
```

---

## 5. nginx-unit 简介

### 什么是 nginx-unit

nginx-unit 是 NGINX 推出的动态应用服务器，专为微服务架构设计，支持通过 API 动态配置应用部署。

**核心特性**：

| 特性 | 说明 |
|------|------|
| **动态配置** | 通过 REST API 实时配置，无需重启 |
| **多语言支持** | Go、Python、PHP、Perl、Ruby、Node.js、Java |
| **语言无关路由** | 统一的路由配置，与应用语言解耦 |
| **零停机更新** | 平滑的应用版本切换 |
| **静态文件服务** | 内置高效的静态资源服务 |

### 架构对比

```
传统 NGINX:                    nginx-unit:
┌──────────┐                   ┌─────────────────┐
│  nginx   │──▶  php-fpm       │     unit        │
│          │                   │  ┌───────────┐  │
│  proxy   │──▶  uwsgi         │  │  router   │  │
│          │                   │  └─────┬─────┘  │
│  static  │◀──  files         │  ┌─────┴─────┐  │
└──────────┘                   │  │  lang     │  │
                               │  │ modules   │  │
                               │  │ ┌─┬─┬─┐   │  │
                               │  │ │ │ │ │   │  │
                               │  │ └─┴─┴─┘   │  │
                               │  └───────────┘  │
                               └─────────────────┘
```

### 安装与启动

```bash
# macOS
brew install nginx-unit

# Ubuntu/Debian
curl -X PUT --data-binary @unit.deb http://nginx.org/...
sudo dpkg -i unit.deb

# 启动服务
sudo unitd --log /var/log/unit.log
```

### 核心 API

#### 配置监听器

```bash
# 创建 HTTP 监听器
curl -X PUT http://localhost:8000/config/listeners/127.0.0.1:80 \
    -d '{"pass": "routes"}'
```

#### 配置应用

```bash
# 配置 PHP 应用
curl -X PUT http://localhost:8000/config/applications/php_app \
    -d '{
        "type": "php",
        "root": "/var/www/php-app",
        "script": "index.php",
        "processes": {
            "max": 20,
            "spare": 5
        }
    }'

# 配置 Python (ASGI) 应用
curl -X PUT http://localhost:8000/config/applications/python_app \
    -d '{
        "type": "python",
        "path": "/var/www/python-app",
        "module": "wsgi",
        "callable": "app"
    }'
```

#### 配置路由

```bash
# 配置路由规则
curl -X PUT http://localhost:8000/config/routes \
    -d '[
        {
            "match": {"uri": "/api/*"},
            "action": {"pass": "applications/python_app"}
        },
        {
            "match": {"uri": "*.php"},
            "action": {"pass": "applications/php_app"}
        },
        {
            "action": {"share": "/var/www/static"}
        }
    ]'
```

### 完整配置示例

```json
{
    "listeners": {
        "*:80": {
            "pass": "routes/main"
        },
        "*:443": {
            "pass": "routes/main",
            "tls": {
                "certificate": "bundle"
            }
        }
    },

    "routes": {
        "main": [
            {
                "match": {"host": "api.example.com"},
                "action": {"pass": "applications/api"}
            },
            {
                "match": {"uri": "/admin/*"},
                "action": {"pass": "applications/admin"}
            },
            {
                "match": {"uri": ["*.jpg", "*.png", "*.css", "*.js"]},
                "action": {"share": "/var/www/static"}
            },
            {
                "action": {"pass": "applications/frontend"}
            }
        ]
    },

    "applications": {
        "api": {
            "type": "python",
            "path": "/var/www/api",
            "module": "app",
            "callable": "application",
            "processes": 10
        },
        "admin": {
            "type": "php",
            "root": "/var/www/admin",
            "script": "index.php",
            "processes": {"max": 10, "spare": 3}
        },
        "frontend": {
            "type": "node",
            "working_directory": "/var/www/frontend",
            "executable": "server.js",
            "processes": 4
        }
    },

    "settings": {
        "http": {
            "header_read_timeout": 30,
            "body_read_timeout": 30,
            "send_timeout": 30,
            "idle_timeout": 180
        }
    }
}
```

### NGINX 与 nginx-unit 集成

```nginx
# NGINX 作为反向代理，unit 运行动态应用
upstream unit_backend {
    server 127.0.0.1:8000;
    keepalive 32;
}

server {
    listen 80;
    server_name app.example.com;

    location / {
        proxy_pass http://unit_backend;
        proxy_http_version 1.1;
        proxy_set_header Connection "";
        proxy_set_header Host $host;
    }
}
```

---

## 6. 动态 SSL 证书加载

### SSL 证书管理挑战

- 多租户 SaaS：每个租户需要独立证书
- 自动化证书续期：Let's Encrypt 等需要动态更新
- 证书数量大：数万证书无法全部预加载

### 方案一：变量驱动 SSL 证书

```nginx
http {
    # 证书映射
    map $ssl_server_name $ssl_cert {
        default          /etc/nginx/certs/default.crt;
        app1.example.com /etc/nginx/certs/app1.crt;
        app2.example.com /etc/nginx/certs/app2.crt;
    }

    map $ssl_server_name $ssl_key {
        default          /etc/nginx/certs/default.key;
        app1.example.com /etc/nginx/certs/app1.key;
        app2.example.com /etc/nginx/certs/app2.key;
    }

    server {
        listen 443 ssl;
        server_name app1.example.com app2.example.com;

        ssl_certificate     $ssl_cert;
        ssl_certificate_key $ssl_key;

        location / {
            proxy_pass http://backend;
        }
    }
}
```

**限制**：标准 NGINX 不支持变量形式的 `ssl_certificate`。

### 方案二：OpenResty Lua 方案

```lua
-- ssl_certificate.lua
local ssl = require "ngx.ssl"
local cert_cache = require "resty.lrucache"

local function load_certificate(domain)
    -- 从 Redis/Consul 获取证书
    local cert_data = get_cert_from_storage(domain)

    if not cert_data then
        return nil, "certificate not found"
    end

    local ok, err = ssl.clear_certs()
    if not ok then
        return nil, "failed to clear certs: " .. err
    end

    local ok, err = ssl.set_der_cert(cert_data.cert)
    if not ok then
        return nil, "failed to set cert: " .. err
    end

    local ok, err = ssl.set_der_priv_key(cert_data.key)
    if not ok then
        return nil, "failed to set key: " .. err
    end

    return true
end

-- 在 ssl_certificate_by_lua_block 中调用
local domain = ssl.server_name()
load_certificate(domain)
```

```nginx
server {
    listen 443 ssl;

    ssl_certificate_by_lua_file /etc/nginx/ssl_certificate.lua;

    # 占位证书（首次连接需要）
    ssl_certificate /etc/nginx/certs/default.crt;
    ssl_certificate_key /etc/nginx/certs/default.key;
}
```

### 方案三：NGINX Plus 动态证书

NGINX Plus 支持 `ssl_certificate` 和 `ssl_certificate_key` 使用变量：

```nginx
# NGINX Plus 配置
server {
    listen 443 ssl;
    server_name ~^(?<domain>.+)$;

    ssl_certificate     /etc/nginx/certs/$domain.crt;
    ssl_certificate_key /etc/nginx/certs/$domain.key;

    # 证书数据存储在共享内存
    ssl_session_cache shared:SSL:10m;
}
```

### 方案四：密钥管理存储（KMS）集成

```nginx
# 使用 NJS 从外部 KMS 获取证书
js_import /etc/nginx/ssl_manager.js;

server {
    listen 443 ssl;

    ssl_certificate /tmp/dynamic.crt;
    ssl_certificate_key /tmp/dynamic.key;

    # 定期更新证书
    location /_ssl_update {
        internal;
        js_content ssl_manager.update_cert;
    }
}
```

---

## 7. 配置热重载策略

### reload 机制详解

```
进程变化流程:

时间线 ──────────────────────────────────────────────▶

Master    ├─────────┬─────────┬─────────┬──────────┤
              │         │         │          │
Worker-1  ├─────────┴────┬────┴────────┤         │
                        graceful stop
Worker-2               ├───────────────┴─────────┤
                              new worker
```

### 优雅重载配置

```bash
#!/bin/bash
# safe-reload.sh - 安全重载脚本

NGINX_BIN="/usr/local/nginx/sbin/nginx"
CONFIG_FILE="/etc/nginx/nginx.conf"

# 1. 测试配置有效性
echo "Testing configuration..."
$NGINX_BIN -t -c $CONFIG_FILE
if [ $? -ne 0 ]; then
    echo "Configuration test failed! Aborting reload."
    exit 1
fi

# 2. 优雅重载
echo "Reloading NGINX..."
$NGINX_BIN -s reload

# 3. 验证重载成功
sleep 1
NEW_PID=$(cat /var/run/nginx.pid)
echo "New master PID: $NEW_PID"

# 4. 检查 worker 进程
WORKER_COUNT=$(ps aux | grep "nginx: worker" | grep -v grep | wc -l)
echo "Active workers: $WORKER_COUNT"
```

### 配置版本管理

```nginx
http {
    # 在响应头中暴露配置版本
    add_header X-Config-Version "v2.3.1" always;

    # 或者使用变量（NJS 设置）
    js_set $config_version get_config_version;
    add_header X-Config-Version $config_version always;
}
```

```javascript
// config_version.js
var configVersion = "2.3.1";
var configTimestamp = Date.now();

function get_config_version(r) {
    return configVersion + "-" + configTimestamp;
}

export default { get_config_version };
```

### 金丝雀重载策略

```bash
#!/bin/bash
# canary-reload.sh - 金丝雀重载

# 步骤1：启动测试实例
echo "Starting canary instance..."
nginx -c /etc/nginx/nginx.conf \
      -p /var/run/nginx-canary \
      -g "pid /var/run/nginx-canary.pid;"

# 步骤2：健康检查
sleep 2
if ! curl -f http://localhost:8080/health; then
    echo "Canary health check failed!"
    kill $(cat /var/run/nginx-canary.pid)
    exit 1
fi

# 步骤3：切换流量（通过负载均衡器或 DNS）
echo "Switching traffic to canary..."

# 步骤4：观察一段时间后正式重载主实例
sleep 60
nginx -s reload

# 步骤5：停止金丝雀实例
kill $(cat /var/run/nginx-canary.pid)
```

### 自动回滚机制

```python
# reload_monitor.py
import subprocess
import time
import requests

def reload_nginx():
    """执行 NGINX 重载并监控状态"""

    # 记录重载前状态
    before_metrics = collect_metrics()

    # 执行重载
    result = subprocess.run(['nginx', '-s', 'reload'], capture_output=True)

    if result.returncode != 0:
        print("Reload failed:", result.stderr)
        return False

    # 监控窗口期
    time.sleep(5)

    # 检查关键指标
    after_metrics = collect_metrics()

    if after_metrics['error_rate'] > before_metrics['error_rate'] * 2:
        print("Error rate increased! Rolling back...")
        rollback()
        return False

    if after_metrics['5xx_count'] > 100:
        print("5xx errors detected! Rolling back...")
        rollback()
        return False

    return True

def rollback():
    """回滚到上一个配置版本"""
    subprocess.run(['cp', '/etc/nginx/nginx.conf.backup',
                   '/etc/nginx/nginx.conf'])
    subprocess.run(['nginx', '-s', 'reload'])

def collect_metrics():
    """收集性能指标"""
    # 实现指标收集逻辑
    pass
```

---

## 8. 完整动态配置示例

### 微服务网关配置

```nginx
# /etc/nginx/nginx.conf
user nginx;
worker_processes auto;
error_log /var/log/nginx/error.log warn;
pid /var/run/nginx.pid;

load_module modules/ngx_http_js_module.so;

events {
    worker_connections 4096;
    use epoll;
    multi_accept on;
}

http {
    include /etc/nginx/mime.types;
    default_type application/octet-stream;

    # 日志格式
    log_format main '$remote_addr - $remote_user [$time_local] "$request" '
                    '$status $body_bytes_sent "$http_referer" '
                    '"$http_user_agent" "$http_x_forwarded_for" '
                    'upstream=$upstream_addr '
                    'config=$config_version';

    access_log /var/log/nginx/access.log main;

    # 动态配置模块
    js_import /etc/nginx/dynamic_config.js;
    js_set $config_version dynamic_config.get_version;
    js_set $backend dynamic_config.resolve_backend;

    # 上游状态共享内存
    upstream_zone upstreams 64m;

    # === 动态 Upstream 管理 ===
    # 使用 dyups 或 Consul 模板生成
    include /etc/nginx/conf.d/upstreams/*.conf;

    # === 服务发现 API ===
    server {
        listen 127.0.0.1:8081;
        location / {
            # dyups 管理接口
            dyups_interface;

            # 或自定义 NJS 接口
            # js_content dynamic_config.admin_api;
        }
    }

    # === 主网关服务 ===
    server {
        listen 80;
        listen 443 ssl http2;
        server_name gateway.example.com;

        # 动态 SSL（NGINX Plus）
        # ssl_certificate /etc/nginx/certs/$ssl_server_name.crt;
        # ssl_certificate_key /etc/nginx/certs/$ssl_server_name.key;

        ssl_certificate /etc/nginx/certs/default.crt;
        ssl_certificate_key /etc/nginx/certs/default.key;

        # 安全头
        add_header X-Frame-Options "SAMEORIGIN" always;
        add_header X-Content-Type-Options "nosniff" always;
        add_header X-Config-Version $config_version always;

        # 限流
        limit_req_zone $binary_remote_addr zone=api:10m rate=100r/s;

        # 健康检查
        location /health {
            access_log off;
            return 200 "healthy\n";
        }

        # API 路由（动态后端解析）
        location /api/ {
            limit_req zone=api burst=200 nodelay;

            proxy_pass http://$backend;
            proxy_set_header Host $host;
            proxy_set_header X-Real-IP $remote_addr;
            proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;

            proxy_connect_timeout 5s;
            proxy_send_timeout 30s;
            proxy_read_timeout 30s;

            proxy_next_upstream error timeout http_502 http_503;
        }

        # 静态内容
        location / {
            root /var/www/static;
            try_files $uri $uri/ /index.html;
            expires 1h;
        }
    }
}
```

```javascript
// /etc/nginx/dynamic_config.js - NJS 动态配置脚本
var version = "2.0.0";
var backends = {
    "api": "api_upstream",
    "user": "user_service_upstream",
    "order": "order_service_upstream"
};

function get_version(r) {
    return version;
}

function resolve_backend(r) {
    var service = r.variables.arg_service || "api";
    var upstream = backends[service];

    if (!upstream) {
        r.error("Unknown service: " + service);
        return "fallback_upstream";
    }

    return upstream;
}

// 从 Consul/ETCD 刷新配置
function refresh_from_consul(r) {
    r.subrequest('/_consul/services', {
        method: 'GET'
    }, function(reply) {
        if (reply.status == 200) {
            var services = JSON.parse(reply.body);
            // 更新后端映射
            // 实际实现需要原子更新
            version = Date.now().toString();
            r.return(200, "Config refreshed to " + version);
        } else {
            r.return(502, "Failed to fetch from Consul");
        }
    });
}

export default {
    get_version,
    resolve_backend,
    refresh_from_consul
};
```

### 动态配置管理脚本

```bash
#!/bin/bash
# /usr/local/bin/nginx-dynamic-manager

CONFIG_DIR="/etc/nginx/conf.d/upstreams"
ADMIN_URL="http://127.0.0.1:8081"
CONSUL_URL="http://consul:8500"

# 从 Consul 同步 upstream
sync_from_consul() {
    local service=$1

    # 查询健康实例
    local instances=$(curl -s "$CONSUL_URL/v1/health/service/$service" | \
        jq -r '.[] | select(.Checks[].Status == "passing") |
        "server \(.Service.Address):\(.Service.Port) weight=1;"')

    if [ -z "$instances" ]; then
        echo "No healthy instances for $service"
        return 1
    fi

    # 更新 dyups
    curl -X POST \
        -H "Content-Type: text/plain" \
        -d "$instances" \
        "$ADMIN_URL/upstream/${service}_upstream"

    echo "Updated $service upstream with:"
    echo "$instances"
}

# 批量更新所有服务
sync_all() {
    local services=$(curl -s "$CONSUL_URL/v1/catalog/services" | jq -r 'keys[]')

    for service in $services; do
        sync_from_consul $service
    done
}

# 查看当前 upstream 状态
status() {
    curl -s "$ADMIN_URL/detail" | jq .
}

# 主入口
case "$1" in
    sync)
        if [ -n "$2" ]; then
            sync_from_consul "$2"
        else
            sync_all
        fi
        ;;
    status)
        status
        ;;
    *)
        echo "Usage: $0 {sync [service]|status}"
        exit 1
        ;;
esac
```

### 配置自动同步服务

```systemd
# /etc/systemd/system/nginx-consul-sync.service
[Unit]
Description=NGINX Consul Sync
After=network.target nginx.service

[Service]
Type=simple
ExecStart=/usr/local/bin/consul-template \
    -consul-addr=consul:8500 \
    -template="/etc/consul-templates/nginx-upstreams.ctmpl:$CONFIG_DIR/upstreams.conf:/usr/local/bin/nginx-dynamic-manager sync"
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
```

```bash
# /etc/consul-templates/nginx-upstreams.ctmpl
{{range service "api"}}
server {{.Address}}:{{.Port}} weight={{.Weights.Passing}} max_fails=3 fail_timeout=30s;
{{end}}
```

---

## 9. 监控与运维

### 动态配置监控指标

| 指标 | 采集方式 | 告警阈值 |
|------|----------|----------|
| upstream 变更次数 | access_log 分析 | 突变检测 |
| 服务发现延迟 | 自定义 metrics | > 5s |
| 证书过期时间 | 定时检查 | < 7 天 |
| reload 失败次数 | 脚本监控 | > 0 |

### 关键日志字段

```nginx
log_format dynamic '$remote_addr [$time_local] '
                   'svc=$service '
                   'upstream=$upstream_addr '
                   'ups_resp_time=$upstream_response_time '
                   'cfg_ver=$config_version '
                   'discover_latency=$discover_time';
```

---

## 总结

NGINX 动态配置能力从简单的 DNS 解析到完整的 API 驱动配置，为现代云原生架构提供了灵活的解决方案：

| 场景 | 推荐方案 | 复杂度 |
|------|----------|--------|
| 简单服务发现 | DNS resolver | 低 |
| 动态 upstream 管理 | dyups 模块 | 中 |
| 多语言微服务 | nginx-unit | 中 |
| 复杂路由逻辑 | OpenResty/NJS | 高 |
| 企业级服务网格 | Consul + NGINX Plus | 高 |

选择方案时需权衡功能需求、运维复杂度和团队技术栈，从简单方案开始逐步演进。
