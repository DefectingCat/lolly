# NGINX Keyval 模块详解

本文档详细介绍 NGINX Keyval 模块（动态键值存储）的配置与使用方法，包括 HTTP 和 Stream 两个子模块。

---

## 1. Keyval 模块概述

### 什么是 Keyval 模块

Keyval 模块是 NGINX 提供的**动态键值存储系统**，允许在运行时通过 API 动态管理键值对数据，无需重启 NGINX 即可更新配置逻辑。

### 模块组成

| 模块名称 | 上下文 | 商业版本 | 可用版本 |
|---------|--------|---------|---------|
| `ngx_http_keyval_module` | http | 需要 | 1.13.3+ |
| `ngx_stream_keyval_module` | stream | 需要 | 1.13.7+ |

### 核心特性

1. **动态更新**：通过 API 实时增删改查键值对
2. **内存存储**：使用共享内存，高性能访问
3. **持久化支持**：支持状态文件持久化，重启后数据不丢失
4. **过期机制**：支持键值对自动过期（TTL）
5. **集群同步**：支持多节点数据同步
6. **灵活匹配**：支持精确匹配、IP 子网匹配、前缀匹配

---

## 2. HTTP 与 Stream Keyval 模块对比

| 特性 | HTTP Keyval | Stream Keyval |
|------|-------------|---------------|
| **上下文** | `http` | `stream` |
| **模块名** | `ngx_http_keyval_module` | `ngx_stream_keyval_module` |
| **可用版本** | 1.13.3+ | 1.13.7+ |
| **键变量** | HTTP 变量（`$arg_*`, `$host`, `$uri` 等） | Stream 变量（`$ssl_server_name`, `$remote_addr` 等） |
| **使用场景** | HTTP 请求路由、限流白名单、动态配置 | TCP/UDP 四层代理路由、SSL 分流 |
| **API 管理** | `/api/{version}/http/keyvals/` | `/api/{version}/stream/keyvals/` |

### HTTP Keyval 典型应用

```nginx
http {
    # 根据请求参数动态返回值
    keyval $arg_user $user_data zone=user_db;

    server {
        location / {
            # $user_data 的值由 API 动态管理
            proxy_pass http://backend_$user_data;
        }
    }
}
```

### Stream Keyval 典型应用

```nginx
stream {
    # 根据 SSL SNI 名称路由到不同后端
    keyval $ssl_server_name $backend zone=ssl_routes;

    server {
        listen 443 ssl;
        proxy_pass $backend;
        ssl_certificate ...;
        ssl_certificate_key ...;
    }
}
```

---

## 3. HTTP Keyval 模块

### 3.1 指令详解

#### keyval_zone

定义存储键值对数据库的共享内存区域。

| 属性 | 说明 |
|------|------|
| **语法** | `keyval_zone zone=name:size [state=file] [timeout=time] [type=string\|ip\|prefix] [sync];` |
| **默认值** | — |
| **上下文** | http |

**参数说明**：

| 参数 | 说明 |
|------|------|
| `zone=name:size` | 共享内存区域名称和大小（如 `one:32k`） |
| `state=file` | 状态文件路径，JSON 格式持久化 |
| `timeout=time` | 键值对过期时间（1.15.0+） |
| `type` | 匹配类型：`string`（精确，默认）、`ip`（IP/CIDR）、`prefix`（前缀，1.17.5+） |
| `sync` | 启用集群同步（1.15.0+，需配合 `timeout`） |

**配置示例**：

```nginx
http {
    # 基本配置
    keyval_zone zone=one:32k;

    # 带持久化
    keyval_zone zone=two:64k state=/var/lib/nginx/state/two.keyval;

    # 带过期时间和同步
    keyval_zone zone=three:1m state=/var/lib/nginx/state/three.keyval timeout=1h sync;

    # IP 类型（用于 IP 黑白名单）
    keyval_zone zone=ip_list:256k type=ip;

    # 前缀类型（用于路由前缀匹配）
    keyval_zone zone=prefix_routes:128k type=prefix;
}
```

#### keyval

创建变量，其值通过键值数据库查找获得。

| 属性 | 说明 |
|------|------|
| **语法** | `keyval key $variable zone=name;` |
| **默认值** | — |
| **上下文** | http |

**参数说明**：

| 参数 | 说明 |
|------|------|
| `key` | 查找键，可使用 NGINX 变量（如 `$arg_user`, `$uri`） |
| `$variable` | 创建的变量名，存储查找到的值 |
| `zone=name` | 指定查询的共享内存区域 |

**配置示例**：

```nginx
http {
    keyval_zone zone=users:64k state=/var/lib/nginx/state/users.keyval;

    # 根据 URL 参数查找
    keyval $arg_user_id $user_role zone=users;

    # 根据 Host 查找
    keyval $host $backend_pool zone=backend_map;

    # 根据 URI 查找
    keyval $uri $cache_ttl zone=ttl_config;

    server {
        listen 80;

        location / {
            # 使用查找到的值
            proxy_pass http://$user_role;
        }
    }
}
```

### 3.2 匹配类型详解

#### string（精确匹配，默认）

```nginx
http {
    keyval_zone zone=exact:32k type=string;
    keyval $arg_key $value zone=exact;
}
```

- 查找键必须与存储键完全相同
- 适用于：用户角色映射、配置项查找

#### ip（IP/CIDR 匹配，1.17.1+）

```nginx
http {
    keyval_zone zone=ip_allow:256k type=ip;
    keyval $remote_addr $allowed zone=ip_allow;

    server {
        listen 80;

        location /api {
            if ($allowed = "") {
                return 403 "Access denied";
            }
            proxy_pass http://api_backend;
        }
    }
}
```

- 存储键可以是 IPv4/IPv6 地址或 CIDR
- 查找时匹配包含该地址的子网
- 适用于：IP 黑白名单、地理位置限流

**IP 类型数据示例**：

```json
{
    "192.168.1.0/24": "allow",
    "10.0.0.0/8": "allow",
    "192.168.1.100": "admin"
}
```

#### prefix（前缀匹配，1.17.5+）

```nginx
http {
    keyval_zone zone=routes:64k type=prefix;
    keyval $uri $handler zone=routes;

    server {
        location / {
            # /api/v1/users 匹配键 /api/v1
            proxy_pass http://$handler;
        }
    }
}
```

- 存储键必须是查找键的前缀
- 适用于：路由前缀匹配、版本控制

**前缀类型数据示例**：

```json
{
    "/api/v1": "backend_v1",
    "/api/v2": "backend_v2",
    "/admin": "admin_backend"
}
```

---

## 4. Stream Keyval 模块

### 4.1 指令详解

#### keyval_zone（Stream）

| 属性 | 说明 |
|------|------|
| **语法** | `keyval_zone zone=name:size [state=file] [timeout=time] [type=string\|ip\|prefix] [sync];` |
| **默认值** | — |
| **上下文** | stream |

与 HTTP 版本的语法完全相同，仅在 `stream` 上下文中使用。

#### keyval（Stream）

| 属性 | 说明 |
|------|------|
| **语法** | `keyval key $variable zone=name;` |
| **默认值** | — |
| **上下文** | stream |

### 4.2 配置示例

#### SSL SNI 路由

```nginx
stream {
    # 根据 SSL SNI 路由到不同后端
    keyval_zone zone=ssl_routes:64k state=/var/lib/nginx/state/ssl_routes.keyval;
    keyval $ssl_server_name $backend zone=ssl_routes;

    server {
        listen 443 ssl;
        proxy_pass $backend;

        ssl_certificate /etc/nginx/certs/default.crt;
        ssl_certificate_key /etc/nginx/certs/default.key;
    }
}
```

#### 动态 TCP 代理

```nginx
stream {
    # 根据目标端口路由
    keyval_zone zone=port_map:32k type=string;
    keyval $server_port $upstream zone=port_map;

    server {
        listen 10000-20000;
        proxy_pass $upstream;
    }
}
```

#### 客户端 IP 限流

```nginx
stream {
    # IP 黑名单
    keyval_zone zone=blocked_ips:256k type=ip;
    keyval $remote_addr $is_blocked zone=blocked_ips;

    server {
        listen 3306;

        # 通过 njs 或 map 实现阻断
        # 注意：stream 需要配合其他模块实现拒绝逻辑
        proxy_pass mysql_backend;
    }
}
```

---

## 5. API 管理接口

### 5.1 HTTP Keyvals API

#### 端点概览

| 端点 | 方法 | 功能 |
|------|------|------|
| `/api/{version}/http/keyvals/` | GET | 列出所有 HTTP keyval zones |
| `/api/{version}/http/keyvals/{zone}` | GET | 查询指定 zone 的键值对 |
| `/api/{version}/http/keyvals/{zone}` | POST | 添加键值对 |
| `/api/{version}/http/keyvals/{zone}` | PATCH | 修改或删除单个键 |
| `/api/{version}/http/keyvals/{zone}` | DELETE | 清空整个 zone |

> **注意**：API 版本当前为 `9`

#### 查询键值对

```bash
# 获取所有键值对
curl http://localhost:8080/api/9/http/keyvals/one

# 获取特定键
curl http://localhost:8080/api/9/http/keyvals/one?key=user1
```

**响应示例**：

```json
{
    "user1": "backend_a",
    "user2": "backend_b",
    "user3": {
        "value": "backend_c",
        "expire": 1699123456789
    }
}
```

#### 添加键值对（POST）

```bash
# 添加单个键值对
curl -X POST http://localhost:8080/api/9/http/keyvals/one \
    -H "Content-Type: application/json" \
    -d '{"user4": "backend_d"}'

# 添加带过期时间的键值对（毫秒）
curl -X POST http://localhost:8080/api/9/http/keyvals/one \
    -H "Content-Type: application/json" \
    -d '{
        "user5": {
            "value": "backend_e",
            "expire": 3600000
        }
    }'
```

**状态码**：
- `201` - 创建成功
- `409` - 键已存在
- `400` - 格式错误
- `413` - 请求体过大（超过 `client_body_buffer_size`）

#### 修改/删除键值对（PATCH）

```bash
# 修改键值
curl -X PATCH http://localhost:8080/api/9/http/keyvals/one \
    -H "Content-Type: application/json" \
    -d '{"user1": {"value": "new_backend", "expire": 7200000}}'

# 删除键（设置为 null）
curl -X PATCH http://localhost:8080/api/9/http/keyvals/one \
    -H "Content-Type: application/json" \
    -d '{"user1": null}'
```

**注意**：PATCH 一次只能更新一个键

#### 清空 Zone（DELETE）

```bash
# 删除 zone 中所有键值对
curl -X DELETE http://localhost:8080/api/9/http/keyvals/one
```

### 5.2 Stream Keyvals API

Stream keyval 使用相同的 API 格式，只是端点路径不同：

| 端点 | 方法 | 功能 |
|------|------|------|
| `/api/{version}/stream/keyvals/` | GET | 列出所有 Stream keyval zones |
| `/api/{version}/stream/keyvals/{zone}` | GET | 查询指定 zone |
| `/api/{version}/stream/keyvals/{zone}` | POST | 添加键值对 |
| `/api/{version}/stream/keyvals/{zone}` | PATCH | 修改或删除键 |
| `/api/{version}/stream/keyvals/{zone}` | DELETE | 清空 zone |

**使用示例**：

```bash
# 添加 SSL 路由
curl -X POST http://localhost:8080/api/9/stream/keyvals/ssl_routes \
    -H "Content-Type: application/json" \
    -d '{"api.example.com": "192.168.1.10:8443"}'

# 查询
curl http://localhost:8080/api/9/stream/keyvals/ssl_routes
```

### 5.3 完整 API 配置示例

```nginx
http {
    # 定义 keyval zones
    keyval_zone zone=users:64k state=/var/lib/nginx/state/users.keyval timeout=1h;
    keyval_zone zone=routes:32k state=/var/lib/nginx/state/routes.keyval;
    keyval_zone zone=ip_blacklist:256k type=ip;

    # 使用 keyval
    keyval $arg_user $user_role zone=users;
    keyval $uri $backend zone=routes;
    keyval $remote_addr $blocked zone=ip_blacklist;

    # API 服务配置
    server {
        listen 8080;
        server_name localhost;

        location /api {
            # 启用 API 写权限
            api write=on;

            # 安全限制
            allow 127.0.0.1;
            allow 10.0.0.0/8;
            deny all;
        }
    }

    # 业务服务
    server {
        listen 80;
        server_name app.example.com;

        location / {
            # IP 黑名单检查
            if ($blocked = "blocked") {
                return 403 "IP blocked";
            }

            # 动态路由
            proxy_pass http://$backend;
        }
    }
}

stream {
    keyval_zone zone=ssl_routes:64k state=/var/lib/nginx/state/ssl_routes.keyval;
    keyval $ssl_server_name $backend zone=ssl_routes;

    server {
        listen 443 ssl;
        proxy_pass $backend;

        ssl_certificate /etc/nginx/certs/default.crt;
        ssl_certificate_key /etc/nginx/certs/default.key;
    }
}
```

---

## 6. 使用场景详解

### 6.1 动态 IP 黑名单

```nginx
http {
    # IP 类型 zone，支持 CIDR
    keyval_zone zone=blacklist:1m type=ip state=/var/lib/nginx/state/blacklist.keyval;
    keyval $remote_addr $blocked zone=blacklist;

    server {
        listen 80;

        location / {
            # 检查是否在黑名单
            if ($blocked = "blocked") {
                return 403 "Access denied";
            }

            proxy_pass http://backend;
        }
    }
}
```

**API 操作**：

```bash
# 封禁单个 IP
curl -X POST http://localhost:8080/api/9/http/keyvals/blacklist \
    -d '{"192.168.1.100": "blocked"}'

# 封禁 IP 段
curl -X POST http://localhost:8080/api/9/http/keyvals/blacklist \
    -d '{"10.0.0.0/8": "blocked"}'

# 解封
curl -X PATCH http://localhost:8080/api/9/http/keyvals/blacklist \
    -d '{"192.168.1.100": null}'
```

### 6.2 限流白名单

```nginx
http {
    # 限流白名单（这些 IP 不限流）
    keyval_zone zone=rate_whitelist:256k type=ip;
    keyval $remote_addr $rate_whitelisted zone=rate_whitelist;

    # 定义限流区域
    limit_req_zone $binary_remote_addr zone=api_limit:10m rate=10r/s;

    server {
        listen 80;

        location /api {
            # 白名单跳过限流
            if ($rate_whitelisted = "whitelisted") {
                proxy_pass http://api_backend;
                break;
            }

            limit_req zone=api_limit burst=20 nodelay;
            proxy_pass http://api_backend;
        }
    }
}
```

### 6.3 动态路由映射

```nginx
http {
    keyval_zone zone=routes:64k type=prefix state=/var/lib/nginx/state/routes.keyval;
    keyval $uri $backend zone=routes;

    upstream backend_a {
        server 192.168.1.10:8080;
    }

    upstream backend_b {
        server 192.168.1.11:8080;
    }

    server {
        listen 80;

        location / {
            # 默认后端
            if ($backend = "") {
                set $backend "backend_a";
            }

            proxy_pass http://$backend;
        }
    }
}
```

**API 操作**：

```bash
# 设置路由规则
curl -X POST http://localhost:8080/api/9/http/keyvals/routes \
    -d '{
        "/api/v1": "backend_a",
        "/api/v2": "backend_b",
        "/admin": "backend_a"
    }'
```

### 6.4 A/B 测试动态分配

```nginx
http {
    keyval_zone zone=ab_test:32k state=/var/lib/nginx/state/ab_test.keyval;
    keyval $cookie_userid $ab_group zone=ab_test;

    upstream version_a {
        server 192.168.1.10:8080;
    }

    upstream version_b {
        server 192.168.1.11:8080;
    }

    server {
        listen 80;

        location / {
            # 未分配或指定为 a 的用户
            if ($ab_group = "a") {
                proxy_pass http://version_a;
                break;
            }

            # 指定为 b 的用户
            if ($ab_group = "b") {
                proxy_pass http://version_b;
                break;
            }

            # 新用户默认走版本 A
            proxy_pass http://version_a;
        }
    }
}
```

### 6.5 SSL 证书动态路由（Stream）

```nginx
stream {
    # 域名到后端的映射
    keyval_zone zone=ssl_routes:64k state=/var/lib/nginx/state/ssl_routes.keyval;
    keyval $ssl_server_name $backend zone=ssl_routes;

    server {
        listen 443 ssl;

        # 默认证书
        ssl_certificate /etc/nginx/certs/default.crt;
        ssl_certificate_key /etc/nginx/certs/default.key;

        proxy_pass $backend;
    }
}

http {
    # API 管理 Stream keyval
    server {
        listen 8080;

        location /api {
            api write=on;
        }
    }
}
```

---

## 7. 与 Lolly 项目的关系和建议

### 7.1 项目对比

[Lolly](https://github.com/xfy/lolly) 是一个使用 Go 语言编写的高性能 HTTP 服务器与反向代理。与 NGINX Keyval 模块相比：

| 特性 | NGINX Keyval | Lolly |
|------|--------------|-------|
| **动态配置** | 通过 API 实时更新 | 通过配置热重载（HUP 信号） |
| **键值存储** | 共享内存，API 管理 | 配置文件/YAML |
| **持久化** | state 文件自动持久化 | 配置文件持久化 |
| **匹配类型** | string、ip、prefix | 需自定义实现 |
| **商业限制** | NGINX Plus 专有功能 | 开源免费 |
| **集群同步** | 内置支持 | 需自行实现 |

### 7.2 Lolly 的动态配置建议

虽然 Lolly 当前不支持类似 Keyval 的动态键值存储，但可以通过以下方式实现类似功能：

#### 方案 1：配置热重载

Lolly 已支持 SIGHUP 信号触发配置重载：

```go
// 在配置中定义映射表
proxy:
  - path: "/api"
    dynamic_routes:
      - key: "user1"
        target: "http://backend1:8080"
      - key: "user2"
        target: "http://backend2:8080"
```

修改配置后执行：

```bash
kill -HUP <lolly_pid>
```

#### 方案 2：自定义中间件实现

可以在 Lolly 中实现类似 Keyval 的动态路由中间件：

```go
// internal/middleware/keyval/keyval.go
package keyval

import (
    "sync"
    "time"
)

type KeyvalStore struct {
    mu     sync.RWMutex
    data   map[string]Entry
    file   string
}

type Entry struct {
    Value   string
    Expires time.Time
}

func (s *KeyvalStore) Get(key string) (string, bool) {
    s.mu.RLock()
    defer s.mu.RUnlock()

    entry, ok := s.data[key]
    if !ok {
        return "", false
    }

    if !entry.Expires.IsZero() && time.Now().After(entry.Expires) {
        return "", false
    }

    return entry.Value, true
}

func (s *KeyvalStore) Set(key, value string, ttl time.Duration) {
    s.mu.Lock()
    defer s.mu.Unlock()

    entry := Entry{Value: value}
    if ttl > 0 {
        entry.Expires = time.Now().Add(ttl)
    }

    s.data[key] = entry
}
```

#### 方案 3：外部存储集成

建议 Lolly 集成外部键值存储：

- **Redis**：支持过期、持久化、集群
- **etcd**：支持监听变更、服务发现
- **Consul**：服务发现与健康检查

```go
// Redis 集成示例
import "github.com/redis/go-redis/v9"

type RedisKeyval struct {
    client *redis.Client
}

func (r *RedisKeyval) Get(ctx context.Context, key string) (string, error) {
    return r.client.Get(ctx, key).Result()
}

func (r *RedisKeyval) Set(ctx context.Context, key, value string, ttl time.Duration) error {
    return r.client.Set(ctx, key, value, ttl).Err()
}
```

### 7.3 Lolly 潜在增强功能

参考 NGINX Keyval 模块，建议 Lolly 可考虑实现：

1. **内置动态键值 API**
   - RESTful API 管理键值对
   - 支持内存存储与持久化
   - TTL 过期机制

2. **多种匹配类型**
   - 精确匹配
   - IP/CIDR 匹配（用于黑白名单）
   - 前缀匹配（用于路由）

3. **集群同步**
   - 基于 Raft 或 gossip 协议
   - 配置变更广播

4. **NGINX Plus 兼容 API**
   - 兼容 `/api/{version}/http/keyvals/` 接口
   - 便于迁移现有 NGINX Plus 用户

---

## 8. 最佳实践

### 8.1 内存大小规划

```nginx
# 估算每个键值对大小
# key: 平均 50 字节
# value: 平均 100 字节
# overhead: 约 50 字节
# 总计: 约 200 字节/键值对

# 10万键值对约需 20MB
keyval_zone zone=large:20m;

# 设置合理超时，避免内存无限增长
keyval_zone zone=with_ttl:32k timeout=1h;
```

### 8.2 持久化策略

```nginx
# 重要数据启用持久化
keyval_zone zone=critical:64k state=/var/lib/nginx/state/critical.keyval;

# 临时数据不持久化
keyval_zone zone=temp:32k timeout=5m;
```

### 8.3 安全建议

```nginx
http {
    # API 严格访问控制
    server {
        listen 8080;

        location /api {
            api write=on;

            # 只允许特定 IP
            allow 127.0.0.1;
            allow 10.0.0.0/8;
            deny all;

            # 可选：添加基础认证
            auth_basic "API Access";
            auth_basic_user_file /etc/nginx/api.htpasswd;
        }
    }
}
```

### 8.4 监控与日志

```nginx
http {
    # 记录 keyval 相关请求
    log_format keyval '$remote_addr - $time_local '
                      'keyval_zone=$keyval_zone key=$key val=$value';

    server {
        location / {
            # 添加自定义头便于调试
            add_header X-Keyval-Value $user_role;

            proxy_pass http://backend;
        }
    }
}
```

### 8.5 集群部署注意事项

启用 `sync` 时的限制：

```nginx
http {
    # sync 需要 timeout 参数
    keyval_zone zone=shared:64k timeout=1h sync;

    # 注意：
    # 1. DELETE 操作只在目标节点立即生效
    # 2. 其他节点需等待 timeout 过期
    # 3. PATCH 删除（设为 null）同样受此限制
}
```

---

## 9. 完整配置示例

### 综合应用场景

```nginx
# nginx.conf

user nginx;
worker_processes auto;

events {
    worker_connections 4096;
}

http {
    # ========== Keyval Zones ==========

    # 用户角色映射（持久化）
    keyval_zone zone=user_roles:64k state=/var/lib/nginx/state/user_roles.keyval;
    keyval $arg_user $user_role zone=user_roles;

    # IP 黑名单（IP 类型，支持 CIDR）
    keyval_zone zone=ip_blacklist:1m type=ip state=/var/lib/nginx/state/blacklist.keyval;
    keyval $remote_addr $blocked zone=ip_blacklist;

    # 限流白名单
    keyval_zone zone=rate_whitelist:512k type=ip;
    keyval $remote_addr $whitelisted zone=rate_whitelist;

    # 路由映射（前缀匹配）
    keyval_zone zone=routes:128k type=prefix state=/var/lib/nginx/state/routes.keyval;
    keyval $uri $backend zone=routes;

    # 临时会话（5分钟过期）
    keyval_zone zone=sessions:32k timeout=5m;
    keyval $cookie_session $session_data zone=sessions;

    # ========== Rate Limit ==========
    limit_req_zone $binary_remote_addr zone=general:10m rate=100r/s;

    # ========== Upstreams ==========
    upstream backend_admin {
        server 192.168.1.10:8080;
    }

    upstream backend_api {
        server 192.168.1.11:8080;
        server 192.168.1.12:8080;
    }

    upstream backend_default {
        server 192.168.1.20:8080;
    }

    # ========== API Server ==========
    server {
        listen 127.0.0.1:8080;

        location /api {
            api write=on;
            allow 127.0.0.1;
            allow 10.0.0.0/8;
            deny all;
        }
    }

    # ========== Main Server ==========
    server {
        listen 80;
        server_name app.example.com;

        # 安全头
        add_header X-Frame-Options "SAMEORIGIN" always;
        add_header X-Content-Type-Options "nosniff" always;

        location / {
            # 1. 检查 IP 黑名单
            if ($blocked = "blocked") {
                return 403 "Access denied";
            }

            # 2. 限流（白名单除外）
            if ($whitelisted = "") {
                limit_req zone=general burst=200 nodelay;
            }

            # 3. 根据路由映射选择后端
            if ($backend = "") {
                set $backend "default";
            }

            # 4. 根据用户角色选择后端
            if ($user_role = "admin") {
                set $backend "admin";
            }

            # 5. 代理到对应后端
            proxy_pass http://backend_$backend;

            # 传递 keyval 信息到后端
            proxy_set_header X-User-Role $user_role;
            proxy_set_header X-Session-Data $session_data;
        }
    }
}

stream {
    # SSL 证书动态路由
    keyval_zone zone=ssl_routes:64k state=/var/lib/nginx/state/ssl_routes.keyval;
    keyval $ssl_server_name $backend zone=ssl_routes;

    server {
        listen 443 ssl;

        ssl_certificate /etc/nginx/certs/default.crt;
        ssl_certificate_key /etc/nginx/certs/default.key;

        proxy_pass $backend;
        proxy_ssl on;
    }
}
```

---

## 10. 常见问题

### Q1: Keyval 数据在重启后会丢失吗？

**A**: 如果配置了 `state` 参数，数据会持久化到 JSON 文件，重启后自动加载。未配置 `state` 的数据会丢失。

### Q2: 如何备份 Keyval 数据？

**A**: 直接备份 state 文件即可：

```bash
cp /var/lib/nginx/state/*.keyval /backup/
```

### Q3: 集群同步有什么限制？

**A**: 启用 `sync` 后：
- 添加操作会同步到所有节点
- 删除操作只在目标节点立即生效，其他节点需等待 `timeout` 过期

### Q4: 可以手动编辑 state 文件吗？

**A**: 不建议。state 文件由 NGINX 自动管理，手动编辑可能导致数据损坏。

### Q5: 与 map 模块有什么区别？

**A**: 
- `map` 是静态配置，重启生效
- `keyval` 是动态存储，API 实时更新

### Q6: Stream 模块可以使用哪些变量作为 key？

**A**: Stream 上下文的可用变量包括：
- `$remote_addr` - 客户端地址
- `$remote_port` - 客户端端口
- `$server_addr` - 服务器地址
- `$server_port` - 服务器端口
- `$ssl_server_name` - SSL SNI 名称
- `$ssl_session_id` - SSL 会话 ID

---

## 参考链接

- [NGINX HTTP Keyval 模块官方文档](https://nginx.org/en/docs/http/ngx_http_keyval_module.html)
- [NGINX Stream Keyval 模块官方文档](https://nginx.org/en/docs/stream/ngx_stream_keyval_module.html)
- [NGINX HTTP API 模块官方文档](https://nginx.org/en/docs/http/ngx_http_api_module.html)
- [Lolly 项目 GitHub](https://github.com/xfy/lolly)
