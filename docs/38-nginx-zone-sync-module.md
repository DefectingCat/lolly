# NGINX Zone Sync 模块详解

本文档详细介绍 NGINX Zone Sync 模块（集群区域同步）的配置与使用方法，包括工作原理、指令语法、配置示例和最佳实践。

---

## 1. Zone Sync 模块概述

### 什么是 Zone Sync 模块

Zone Sync 模块（`ngx_stream_zone_sync_module`）是 NGINX Plus 提供的**集群状态同步机制**，用于在多个 NGINX 节点之间同步共享内存区域的内容。

### 核心功能

| 功能 | 说明 |
|------|------|
| **多节点共享内存同步** | 在集群节点间自动同步 keyval、sticky sessions、limit_req 等共享内存数据 |
| **集群状态共享** | 实现分布式 NGINX 实例的状态一致性 |
| **动态节点发现** | 支持 DNS 动态发现或静态配置节点 |
| **SSL 加密传输** | 支持节点间通信加密 |
| **高性能** | 基于 TCP 流的高效同步协议 |

### 模块信息

| 属性 | 说明 |
|------|------|
| **模块名称** | `ngx_stream_zone_sync_module` |
| **首次版本** | 1.13.8 |
| **可用性** | NGINX Plus 商业订阅 |
| **上下文** | `stream`, `server` |

### 支持的同步内容

Zone Sync 模块可以同步以下类型的共享内存区域：

1. **HTTP Sticky Sessions**：`sticky learn` 创建的会话粘性数据
2. **HTTP Limit Request**：`limit_req` 的超额请求计数
3. **Keyval 键值对**：HTTP 和 Stream 的 `keyval_zone` 数据

---

## 2. Zone Sync 工作原理

### 2.1 同步架构

```
┌─────────────────┐         ┌─────────────────┐
│   NGINX Node A  │◄───────►│   NGINX Node B  │
│   192.168.1.10  │  TCP    │   192.168.1.11  │
│   :12345        │  Sync   │   :12345        │
└─────────────────┘         └─────────────────┘
        ▲                           ▲
        │                           │
        └───────────┬───────────────┘
                    │
                    ▼
           ┌─────────────────┐
           │   NGINX Node C  │
           │   192.168.1.12  │
           │   :12345        │
           └─────────────────┘
```

### 2.2 同步协议机制

| 机制 | 说明 |
|------|------|
| **传输层** | 基于 TCP 流连接，在 `stream` 块中配置 |
| **轮询机制** | 按 `zone_sync_interval` 间隔轮询共享内存区域更新 |
| **推送机制** | 使用缓冲区推送区域内容到对等节点 |
| **双向同步** | 节点间建立双向连接，互相发送和接收更新 |

### 2.3 节点发现方式

#### 静态配置发现

```nginx
stream {
    server {
        zone_sync;
        listen 127.0.0.1:12345;
        
        # 静态指定集群节点
        zone_sync_server 192.168.1.10:12345;
        zone_sync_server 192.168.1.11:12345;
        zone_sync_server 192.168.1.12:12345;
    }
}
```

- 需要手动维护节点列表
- 添加/删除节点需要重新加载配置

#### 动态 DNS 发现

```nginx
stream {
    resolver 192.168.1.1 valid=10s;
    
    server {
        zone_sync;
        listen 127.0.0.1:12345;
        
        # 通过 DNS 动态解析节点
        zone_sync_server cluster.example.com:12345 resolve;
    }
}
```

- 使用 `resolve` 参数启用 DNS 监控
- 自动检测 DNS 记录变化
- 无需重新加载配置即可扩缩容

### 2.4 状态传递流程

1. **变更检测**：每个节点按 `zone_sync_interval` 轮询本地共享内存
2. **变更缓冲**：将检测到的变更写入同步缓冲区（`zone_sync_buffers`）
3. **网络传输**：通过 TCP 连接将变更发送给其他节点
4. **接收解析**：接收节点解析同步消息并更新本地共享内存
5. **一致性保证**：所有节点最终达到状态一致

### 2.5 节点生命周期管理

| 操作 | 步骤 |
|------|------|
| **添加节点** | 1. 更新 DNS（动态）或配置（静态）<br>2. 启动新 NGINX 实例<br>3. 自动发现并开始同步 |
| **移除节点** | 1. 更新 DNS 或配置<br>2. 发送 `QUIT` 信号优雅关闭<br>3. 其他节点检测到连接关闭 |
| **更换节点 IP** | 1. 更新 DNS 记录<br>2. 其他节点自动检测到变化<br>3. 重新建立连接 |

---

## 3. 指令详解

### 3.1 zone_sync

**启用共享内存区域同步**。

| 属性 | 说明 |
|------|------|
| **语法** | `zone_sync;` |
| **默认值** | — |
| **上下文** | `server` |
| **版本** | 1.13.8+ |

**配置示例**：

```nginx
stream {
    server {
        # 启用 zone 同步
        zone_sync;
        
        listen 127.0.0.1:12345;
        
        zone_sync_server 192.168.1.10:12345;
        zone_sync_server 192.168.1.11:12345;
    }
}
```

**注意事项**：
- 必须在 `stream` 块的 `server` 上下文中使用
- 需要配合 `zone_sync_server` 指定集群节点
- 每个节点必须有唯一的监听地址

---

### 3.2 zone_sync_server

**定义集群节点地址**。

| 属性 | 说明 |
|------|------|
| **语法** | `zone_sync_server address [resolve];` |
| **默认值** | — |
| **上下文** | `server` |
| **版本** | 1.13.8+ |

**参数说明**：

| 参数 | 说明 |
|------|------|
| `address` | 节点地址，支持以下格式：<br>- `IP:port`（如 `192.168.1.10:12345`）<br>- `域名:port`（如 `nginx-node1.example.com:12345`）<br>- `unix:/path/to/socket`（Unix 域套接字） |
| `resolve` | 启用 DNS 监控，域名解析变化时自动更新（需要 `resolver` 指令） |

**配置示例**：

```nginx
stream {
    # 静态配置
    server {
        zone_sync;
        listen 127.0.0.1:12345;
        
        zone_sync_server 192.168.1.10:12345;
        zone_sync_server 192.168.1.11:12345;
    }
}
```

```nginx
stream {
    resolver 192.168.1.1 valid=30s;
    
    # 动态 DNS 发现
    server {
        zone_sync;
        listen 127.0.0.1:12345;
        
        # 解析到多个 A 记录
        zone_sync_server cluster.example.com:12345 resolve;
    }
}
```

**注意事项**：
- 每个节点在配置中只能出现一次
- 集群中所有节点应该使用相同的配置
- 使用 `resolve` 时必须配置 `resolver`

---

### 3.3 zone_sync_connect_timeout

**设置与集群节点建立连接的超时时间**。

| 属性 | 说明 |
|------|------|
| **语法** | `zone_sync_connect_timeout time;` |
| **默认值** | `5s` |
| **上下文** | `stream`, `server` |
| **版本** | 1.13.8+ |

**参数说明**：

| 参数 | 说明 |
|------|------|
| `time` | 超时时间，支持单位：`ms`, `s`, `m`, `h` |

**配置示例**：

```nginx
stream {
    server {
        zone_sync;
        listen 127.0.0.1:12345;
        
        # 连接超时设置为 10 秒
        zone_sync_connect_timeout 10s;
        
        zone_sync_server 192.168.1.10:12345;
    }
}
```

**适用场景**：
- 网络延迟较高时需要增加超时
- 快速失败场景可减少超时

---

### 3.4 zone_sync_timeout

**设置连续读写操作之间的超时时间**。

| 属性 | 说明 |
|------|------|
| **语法** | `zone_sync_timeout time;` |
| **默认值** | `5s` |
| **上下文** | `stream`, `server` |
| **版本** | 1.13.8+ |

**说明**：

该指令替代了早期版本中可能的 `zone_sync_recv_timeout` 和 `zone_sync_send_timeout`，统一控制读写超时。

| 参数 | 说明 |
|------|------|
| `time` | 超时时间，两次连续读取或写入之间的最大间隔 |

**配置示例**：

```nginx
stream {
    server {
        zone_sync;
        listen 127.0.0.1:12345;
        
        # 读写超时设置为 30 秒
        zone_sync_timeout 30s;
        
        zone_sync_server 192.168.1.10:12345;
    }
}
```

---

### 3.5 zone_sync_buffers

**设置用于推送区域内容的缓冲区数量和大小**。

| 属性 | 说明 |
|------|------|
| **语法** | `zone_sync_buffers number size;` |
| **默认值** | `8 4k` 或 `8 8k`（取决于平台内存页大小） |
| **上下文** | `stream`, `server` |
| **版本** | 1.13.8+ |

**参数说明**：

| 参数 | 说明 |
|------|------|
| `number` | 缓冲区数量 |
| `size` | 每个缓冲区大小 |

**重要限制**：
- 单个缓冲区必须足够大以容纳任何共享内存区域中的**单个条目**
- 如果条目超过缓冲区大小，同步将失败

**配置示例**：

```nginx
stream {
    server {
        zone_sync;
        listen 127.0.0.1:12345;
        
        # 16 个 8k 缓冲区
        zone_sync_buffers 16 8k;
        
        zone_sync_server 192.168.1.10:12345;
    }
}
```

**调优建议**：
- 如果 keyval 条目较大（如 JSON 配置），增加 `size`
- 如果同步频繁，增加 `number`

---

### 3.6 zone_sync_interval

**设置轮询共享内存区域更新的间隔时间**。

| 属性 | 说明 |
|------|------|
| **语法** | `zone_sync_interval time;` |
| **默认值** | `1s` |
| **上下文** | `stream`, `server` |
| **版本** | 1.13.8+ |

**参数说明**：

| 参数 | 说明 |
|------|------|
| `time` | 轮询间隔，较短间隔意味着更快的同步但更高的 CPU 使用 |

**配置示例**：

```nginx
stream {
    server {
        zone_sync;
        listen 127.0.0.1:12345;
        
        # 每 500ms 轮询一次更新
        zone_sync_interval 500ms;
        
        zone_sync_server 192.168.1.10:12345;
    }
}
```

**调优建议**：
- 默认 `1s` 适用于大多数场景
- 对实时性要求高的场景可减少到 `100-500ms`
- 资源受限场景可增加到 `2-5s`

---

### 3.7 zone_sync_recv_buffer_size

**设置每个连接的接收缓冲区大小，用于解析同步消息**。

| 属性 | 说明 |
|------|------|
| **语法** | `zone_sync_recv_buffer_size size;` |
| **默认值** | `4k` 或 `8k`（与 `zone_sync_buffers` 的 `size × number` 相同） |
| **上下文** | `stream`, `server` |
| **版本** | 1.13.8+ |

**参数说明**：

| 参数 | 说明 |
|------|------|
| `size` | 接收缓冲区大小，必须大于或等于 `zone_sync_buffers` 的单个缓冲区大小 |

**配置示例**：

```nginx
stream {
    server {
        zone_sync;
        listen 127.0.0.1:12345;
        
        zone_sync_buffers 16 8k;
        # 接收缓冲区至少 8k
        zone_sync_recv_buffer_size 16k;
        
        zone_sync_server 192.168.1.10:12345;
    }
}
```

---

### 3.8 zone_sync_connect_retry_interval

**设置连接失败后重试的间隔时间**。

| 属性 | 说明 |
|------|------|
| **语法** | `zone_sync_connect_retry_interval time;` |
| **默认值** | `1s` |
| **上下文** | `stream`, `server` |
| **版本** | 1.13.8+ |

**参数说明**：

| 参数 | 说明 |
|------|------|
| `time` | 重试间隔，较短间隔可以更快恢复但可能增加网络负担 |

**配置示例**：

```nginx
stream {
    server {
        zone_sync;
        listen 127.0.0.1:12345;
        
        # 连接失败后每 5 秒重试一次
        zone_sync_connect_retry_interval 5s;
        
        zone_sync_server 192.168.1.10:12345;
    }
}
```

---

### 3.9 SSL 相关指令

Zone Sync 支持节点间 SSL 加密传输。

| 指令 | 说明 |
|------|------|
| `zone_sync_ssl on\|off` | 启用 SSL（默认 `off`） |
| `zone_sync_ssl_certificate file` | SSL 证书文件 |
| `zone_sync_ssl_certificate_key file` | SSL 密钥文件 |
| `zone_sync_ssl_protocols protocols` | SSL 协议版本（如 `TLSv1.2 TLSv1.3`） |
| `zone_sync_ssl_ciphers ciphers` | SSL 加密套件 |
| `zone_sync_ssl_server_name on\|off` | 启用 SNI（默认 `off`） |
| `zone_sync_ssl_name name` | 覆盖 SSL 验证的服务器名称 |
| `zone_sync_ssl_verify on\|off` | 启用证书验证（默认 `off`） |
| `zone_sync_ssl_verify_depth number` | 验证深度（默认 `1`） |
| `zone_sync_ssl_trusted_certificate file` | 受信任的 CA 证书 |
| `zone_sync_ssl_crl file` | 证书吊销列表 |

**SSL 配置示例**：

```nginx
stream {
    resolver 192.168.1.1 valid=30s;
    
    server {
        zone_sync;
        listen 127.0.0.1:12345;
        
        zone_sync_server cluster.example.com:12345 resolve;
        
        # 启用 SSL
        zone_sync_ssl on;
        zone_sync_ssl_certificate /etc/nginx/certs/cluster.crt;
        zone_sync_ssl_certificate_key /etc/nginx/certs/cluster.key;
        zone_sync_ssl_protocols TLSv1.2 TLSv1.3;
        zone_sync_ssl_ciphers HIGH:!aNULL:!MD5;
    }
}
```

---

## 4. 集群配置示例

### 4.1 最小化配置

```nginx
# nginx.conf

worker_processes auto;

events {
    worker_connections 1024;
}

http {
    # 定义需要同步的 zone
    upstream backend {
        server backend1.example.com:8080;
        sticky learn
               create=$upstream_cookie_session
               lookup=$cookie_session
               zone=session_store:1m sync;
    }

    server {
        listen 80;
        
        location / {
            proxy_pass http://backend;
        }
    }
}

stream {
    server {
        # 启用 zone 同步
        zone_sync;
        listen 127.0.0.1:12345;
        
        # 集群节点配置
        zone_sync_server node1.example.com:12345;
        zone_sync_server node2.example.com:12345;
    }
}
```

---

### 4.2 三节点集群配置

#### 节点 1（192.168.1.10）

```nginx
# /etc/nginx/nginx.conf (Node 1)

events {
    worker_connections 4096;
}

stream {
    server {
        zone_sync;
        listen 192.168.1.10:12345;
        
        zone_sync_server 192.168.1.10:12345;
        zone_sync_server 192.168.1.11:12345;
        zone_sync_server 192.168.1.12:12345;
        
        zone_sync_connect_timeout 10s;
        zone_sync_timeout 30s;
        zone_sync_interval 500ms;
    }
}
```

#### 节点 2（192.168.1.11）

```nginx
# /etc/nginx/nginx.conf (Node 2)

events {
    worker_connections 4096;
}

stream {
    server {
        zone_sync;
        listen 192.168.1.11:12345;
        
        zone_sync_server 192.168.1.10:12345;
        zone_sync_server 192.168.1.11:12345;
        zone_sync_server 192.168.1.12:12345;
        
        zone_sync_connect_timeout 10s;
        zone_sync_timeout 30s;
        zone_sync_interval 500ms;
    }
}
```

#### 节点 3（192.168.1.12）

```nginx
# /etc/nginx/nginx.conf (Node 3)

events {
    worker_connections 4096;
}

stream {
    server {
        zone_sync;
        listen 192.168.1.12:12345;
        
        zone_sync_server 192.168.1.10:12345;
        zone_sync_server 192.168.1.11:12345;
        zone_sync_server 192.168.1.12:12345;
        
        zone_sync_connect_timeout 10s;
        zone_sync_timeout 30s;
        zone_sync_interval 500ms;
    }
}
```

---

### 4.3 动态 DNS 发现配置

```nginx
# 所有节点使用相同配置

events {
    worker_connections 4096;
}

stream {
    # 配置 DNS 解析器
    resolver 192.168.1.1 valid=10s;
    
    server {
        zone_sync;
        listen 0.0.0.0:12345;
        
        # 通过 SRV 或 A 记录动态发现节点
        zone_sync_server nginx-cluster.internal:12345 resolve;
        
        zone_sync_connect_timeout 5s;
        zone_sync_connect_retry_interval 2s;
    }
}
```

**DNS 记录配置示例**：

```
# DNS A 记录
nginx-cluster.internal.  IN  A  192.168.1.10
nginx-cluster.internal.  IN  A  192.168.1.11
nginx-cluster.internal.  IN  A  192.168.1.12
```

---

### 4.4 同步 keyval 状态

```nginx
events {
    worker_connections 4096;
}

http {
    # 需要同步的 keyval zone（带 timeout 和 sync 参数）
    keyval_zone zone=user_sessions:64k state=/var/lib/nginx/state/sessions.keyval timeout=1h sync;
    keyval_zone zone=api_keys:32k state=/var/lib/nginx/state/api_keys.keyval sync;
    
    keyval $cookie_session $session_data zone=user_sessions;
    keyval $arg_api_key $api_info zone=api_keys;
    
    # API 管理接口
    server {
        listen 127.0.0.1:8080;
        
        location /api {
            api write=on;
            allow 127.0.0.1;
            deny all;
        }
    }
    
    server {
        listen 80;
        
        location / {
            if ($api_info = "") {
                return 403 "API key required";
            }
            proxy_pass http://backend;
        }
    }
}

stream {
    server {
        zone_sync;
        listen 0.0.0.0:12345;
        
        zone_sync_server node1.example.com:12345;
        zone_sync_server node2.example.com:12345;
        
        zone_sync_buffers 16 8k;
        zone_sync_interval 500ms;
    }
}
```

---

### 4.5 同步 limit_conn 状态

```nginx
events {
    worker_connections 4096;
}

http {
    # 限制连接数 zone（需要同步）
    limit_conn_zone $binary_remote_addr zone=addr_limit:10m;
    
    # 注意：limit_conn_zone 不直接支持 sync 参数
    # 需要通过 keyval 间接实现分布式限流
    
    server {
        listen 80;
        
        location /downloads {
            limit_conn addr_limit 10;
            proxy_pass http://backend;
        }
    }
}

stream {
    server {
        zone_sync;
        listen 0.0.0.0:12345;
        
        zone_sync_server node1.example.com:12345;
        zone_sync_server node2.example.com:12345;
    }
}
```

**注意**：`limit_conn_zone` 和 `limit_req_zone` 的同步需要 NGINX Plus 特定版本支持。

---

### 4.6 同步 sticky sessions

```nginx
events {
    worker_connections 4096;
}

http {
    upstream api_backend {
        server 192.168.1.10:8080;
        server 192.168.1.11:8080;
        
        # sticky learn 会话粘性，带 sync 参数
        sticky learn
               create=$upstream_cookie_route
               lookup=$cookie_route
               zone=sticky_sessions:2m sync;
    }
    
    server {
        listen 80;
        
        location /api {
            proxy_pass http://api_backend;
        }
    }
}

stream {
    server {
        zone_sync;
        listen 0.0.0.0:12345;
        
        zone_sync_server node1.example.com:12345;
        zone_sync_server node2.example.com:12345;
        
        zone_sync_interval 200ms;  # 更快的会话同步
    }
}
```

---

## 5. 与 Keyval 模块配合使用

### 5.1 Keyval Zone 同步配置

```nginx
events {
    worker_connections 4096;
}

http {
    # ========== 需要同步的 keyval zones ==========
    
    # 用户会话（带过期时间）
    keyval_zone zone=sessions:2m state=/var/lib/nginx/state/sessions.keyval timeout=30m sync;
    keyval $cookie_session $session_info zone=sessions;
    
    # 动态路由配置
    keyval_zone zone=routes:512k state=/var/lib/nginx/state/routes.keyval sync;
    keyval $uri $backend_pool zone=routes;
    
    # API 限流黑白名单
    keyval_zone zone=rate_whitelist:1m type=ip sync;
    keyval $remote_addr $rate_exempt zone=rate_whitelist;
    
    # ========== 后端定义 ==========
    upstream backend_pool_a {
        server 192.168.1.10:8080;
        server 192.168.1.11:8080;
    }
    
    upstream backend_pool_b {
        server 192.168.1.20:8080;
        server 192.168.1.21:8080;
    }
    
    # ========== API 管理服务器 ==========
    server {
        listen 127.0.0.1:8080;
        
        location /api {
            api write=on;
            allow 127.0.0.1;
            deny all;
        }
    }
    
    # ========== 主业务服务器 ==========
    server {
        listen 80;
        server_name app.example.com;
        
        location / {
            # 白名单跳过限流
            if ($rate_exempt = "exempt") {
                proxy_pass http://backend_pool_a;
                break;
            }
            
            # 动态路由
            if ($backend_pool = "") {
                set $backend_pool "backend_pool_a";
            }
            
            proxy_pass http://$backend_pool;
        }
    }
}

stream {
    # Zone Sync 配置
    server {
        zone_sync;
        listen 0.0.0.0:12345;
        
        # 集群节点
        zone_sync_server 192.168.1.10:12345;
        zone_sync_server 192.168.1.11:12345;
        
        # 同步参数调优
        zone_sync_buffers 32 16k;
        zone_sync_interval 500ms;
        zone_sync_timeout 30s;
    }
}
```

---

### 5.2 通过 API 管理同步数据

```bash
#!/bin/bash

# API 端点
API_BASE="http://127.0.0.1:8080/api/9"

# 添加会话
curl -X POST "${API_BASE}/http/keyvals/sessions" \
    -H "Content-Type: application/json" \
    -d '{
        "user123": {
            "value": "{\"user_id\": 123, \"role\": \"admin\"}",
            "expire": 1800000
        }
    }'

# 添加路由规则
curl -X POST "${API_BASE}/http/keyvals/routes" \
    -H "Content-Type: application/json" \
    -d '{
        "/api/v1": "backend_pool_a",
        "/api/v2": "backend_pool_b",
        "/admin": "backend_pool_a"
    }'

# 查询会话数据
curl "${API_BASE}/http/keyvals/sessions?key=user123"

# 查询所有路由
curl "${API_BASE}/http/keyvals/routes"

# 删除键
curl -X PATCH "${API_BASE}/http/keyvals/sessions" \
    -H "Content-Type: application/json" \
    -d '{"user123": null}'

# 清空 zone
curl -X DELETE "${API_BASE}/http/keyvals/routes"
```

---

### 5.3 集群状态监控

```bash
# 查询 Zone Sync 状态
curl http://127.0.0.1:8080/api/9/stream/zone_sync/

# 示例响应
{
    "connections": {
        "active": 2,
        "idle": 0,
        "closed": 5
    },
    "messages": {
        "sent": 12345,
        "received": 12300
    },
    "bytes": {
        "sent": 1048576,
        "received": 1024000
    },
    "zones": {
        "sessions": {
            "entries": 150,
            "size": 32768
        },
        "routes": {
            "entries": 25,
            "size": 8192
        }
    }
}
```

---

## 6. 性能调优与最佳实践

### 6.1 缓冲区大小调优

```nginx
stream {
    server {
        zone_sync;
        listen 0.0.0.0:12345;
        
        # 场景 1：小型键值对（<1KB）
        zone_sync_buffers 8 4k;
        
        # 场景 2：中型配置（1-4KB）
        zone_sync_buffers 16 8k;
        
        # 场景 3：大型 JSON 配置（>4KB）
        zone_sync_buffers 32 16k;
        
        zone_sync_server node1.example.com:12345;
    }
}
```

---

### 6.2 同步间隔调优

```nginx
# 高实时性场景（会话同步）
zone_sync_interval 100ms;

# 一般场景（路由配置）
zone_sync_interval 500ms;

# 低频率场景（配置同步）
zone_sync_interval 2s;
```

---

### 6.3 网络调优

```nginx
stream {
    server {
        zone_sync;
        listen 0.0.0.0:12345;
        
        # 高延迟网络（跨数据中心）
        zone_sync_connect_timeout 15s;
        zone_sync_timeout 60s;
        zone_sync_connect_retry_interval 5s;
        
        # 低延迟网络（同数据中心）
        zone_sync_connect_timeout 3s;
        zone_sync_timeout 10s;
        zone_sync_connect_retry_interval 1s;
        
        zone_sync_server node1.example.com:12345;
    }
}
```

---

### 6.4 内存规划

```nginx
http {
    # 估算内存需求
    # 每个键值对：key(50B) + value(100B) + overhead(50B) ≈ 200B
    # 10,000 键值对 ≈ 2MB
    
    keyval_zone zone=large_db:4m sync;
    
    # 建议预留 50% 余量
    keyval_zone zone=with_margin:6m sync;
}
```

---

### 6.5 安全建议

```nginx
stream {
    server {
        zone_sync;
        # 只监听内网地址
        listen 192.168.1.10:12345;
        
        # 启用 SSL 加密
        zone_sync_ssl on;
        zone_sync_ssl_certificate /etc/nginx/certs/cluster.crt;
        zone_sync_ssl_certificate_key /etc/nginx/certs/cluster.key;
        zone_sync_ssl_protocols TLSv1.3;
        zone_sync_ssl_ciphers ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-GCM-SHA384;
        
        zone_sync_server node1.internal:12345;
    }
}
```

---

## 7. 故障排查

### 7.1 常见问题

| 问题 | 可能原因 | 解决方案 |
|------|----------|----------|
| 连接失败 | 防火墙/网络不通 | 检查端口连通性，确认防火墙规则 |
| 同步延迟 | 缓冲区不足 | 增加 `zone_sync_buffers` 大小 |
| 节点无法发现 | DNS 解析问题 | 检查 `resolver` 配置，验证 DNS 记录 |
| 数据不一致 | 网络分区 | 检查网络连接，增加超时时间 |
| 内存增长 | 无过期时间 | 配置 `timeout` 参数 |

---

### 7.2 调试命令

```bash
# 测试配置
nginx -t

# 重载配置
nginx -s reload

# 检查进程状态
ps aux | grep nginx

# 查看网络连接
netstat -tlnp | grep nginx
ss -tlnp | grep nginx

# 监控日志
tail -f /var/log/nginx/error.log

# 查看 Zone Sync API 状态
curl http://localhost:8080/api/9/stream/zone_sync/
```

---

## 8. 与 Lolly 项目的关系和建议

### 8.1 项目对比

[Lolly](https://github.com/xfy/lolly) 是一个使用 Go 语言编写的高性能 HTTP 服务器与反向代理。与 NGINX Zone Sync 相比：

| 特性 | NGINX Zone Sync | Lolly |
|------|-----------------|-------|
| **集群同步** | 内置支持，商业功能 | 需自行实现 |
| **状态存储** | 共享内存 | 内存/外部存储 |
| **节点发现** | 静态/DNS | 需自行实现 |
| **传输协议** | TCP（专有协议） | 可自定义 |
| **许可证** | 商业订阅 | 开源免费 |

---

### 8.2 Go 实现分布式状态同步建议

#### 方案 1：基于 Gossip 协议

```go
// internal/cluster/gossip.go
package cluster

import (
    "sync"
    "time"
    
    "github.com/hashicorp/memberlist"
)

type StateSync struct {
    list  *memberlist.Memberlist
    store sync.Map
    config *Config
}

type Config struct {
    NodeName     string
    BindAddr     string
    BindPort     int
    JoinNodes    []string
    SyncInterval time.Duration
}

func NewStateSync(cfg *Config) (*StateSync, error) {
    s := &StateSync{config: cfg}
    
    mlConfig := memberlist.DefaultLANConfig()
    mlConfig.Name = cfg.NodeName
    mlConfig.BindAddr = cfg.BindAddr
    mlConfig.BindPort = cfg.BindPort
    mlConfig.Delegate = s
    
    list, err := memberlist.Create(mlConfig)
    if err != nil {
        return nil, err
    }
    
    s.list = list
    
    if len(cfg.JoinNodes) > 0 {
        _, err = list.Join(cfg.JoinNodes)
        if err != nil {
            return nil, err
        }
    }
    
    return s, nil
}

// 实现 memberlist.Delegate 接口
func (s *StateSync) NodeMeta(limit int) []byte {
    return []byte{}
}

func (s *StateSync) NotifyMsg(buf []byte) {
    // 处理接收到的同步消息
}

func (s *StateSync) GetBroadcasts(overhead, limit int) [][]byte {
    // 返回待广播的变更
    return nil
}

func (s *StateSync) LocalState(join bool) []byte {
    // 返回本地状态
    return nil
}

func (s *StateSync) MergeRemoteState(buf []byte, join bool) {
    // 合并远程状态
}

// 状态操作接口
func (s *StateSync) Set(key, value string) error {
    s.store.Store(key, value)
    return s.broadcastUpdate(key, value)
}

func (s *StateSync) Get(key string) (string, bool) {
    val, ok := s.store.Load(key)
    if !ok {
        return "", false
    }
    return val.(string), true
}

func (s *StateSync) Delete(key string) error {
    s.store.Delete(key)
    return s.broadcastDelete(key)
}
```

---

#### 方案 2：基于 Redis Pub/Sub

```go
// internal/cluster/redis_sync.go
package cluster

import (
    "context"
    "encoding/json"
    "time"
    
    "github.com/redis/go-redis/v9"
)

type RedisSync struct {
    client  *redis.Client
    channel string
    nodeID  string
    store   sync.Map
}

type SyncMessage struct {
    Type      string      `json:"type"` // "set", "delete", "full"
    Key       string      `json:"key,omitempty"`
    Value     interface{} `json:"value,omitempty"`
    NodeID    string      `json:"node_id"`
    Timestamp int64       `json:"timestamp"`
}

func NewRedisSync(addr, channel, nodeID string) (*RedisSync, error) {
    client := redis.NewClient(&redis.Options{
        Addr: addr,
    })
    
    s := &RedisSync{
        client:  client,
        channel: channel,
        nodeID:  nodeID,
    }
    
    // 启动订阅协程
    go s.subscribe(context.Background())
    
    return s, nil
}

func (s *RedisSync) subscribe(ctx context.Context) {
    pubsub := s.client.Subscribe(ctx, s.channel)
    defer pubsub.Close()
    
    ch := pubsub.Channel()
    for msg := range ch {
        if err := s.handleMessage(msg.Payload); err != nil {
            // 处理错误
        }
    }
}

func (s *RedisSync) handleMessage(payload string) error {
    var msg SyncMessage
    if err := json.Unmarshal([]byte(payload), &msg); err != nil {
        return err
    }
    
    // 忽略自己发送的消息
    if msg.NodeID == s.nodeID {
        return nil
    }
    
    switch msg.Type {
    case "set":
        s.store.Store(msg.Key, msg.Value)
    case "delete":
        s.store.Delete(msg.Key)
    }
    
    return nil
}

func (s *RedisSync) Set(ctx context.Context, key string, value interface{}) error {
    msg := SyncMessage{
        Type:      "set",
        Key:       key,
        Value:     value,
        NodeID:    s.nodeID,
        Timestamp: time.Now().UnixNano(),
    }
    
    data, _ := json.Marshal(msg)
    s.store.Store(key, value)
    
    return s.client.Publish(ctx, s.channel, data).Err()
}

func (s *RedisSync) Delete(ctx context.Context, key string) error {
    msg := SyncMessage{
        Type:      "delete",
        Key:       key,
        NodeID:    s.nodeID,
        Timestamp: time.Now().UnixNano(),
    }
    
    data, _ := json.Marshal(msg)
    s.store.Delete(key)
    
    return s.client.Publish(ctx, s.channel, data).Err()
}
```

---

#### 方案 3：基于 etcd 的强一致性同步

```go
// internal/cluster/etcd_sync.go
package cluster

import (
    "context"
    "time"
    
    clientv3 "go.etcd.io/etcd/client/v3"
)

type EtcdSync struct {
    client *clientv3.Client
    prefix string
    nodeID string
    ctx    context.Context
}

func NewEtcdSync(endpoints []string, prefix, nodeID string) (*EtcdSync, error) {
    client, err := clientv3.New(clientv3.Config{
        Endpoints:   endpoints,
        DialTimeout: 5 * time.Second,
    })
    if err != nil {
        return nil, err
    }
    
    return &EtcdSync{
        client: client,
        prefix: prefix,
        nodeID: nodeID,
        ctx:    context.Background(),
    }, nil
}

func (s *EtcdSync) Set(key string, value string, ttl time.Duration) error {
    ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
    defer cancel()
    
    if ttl > 0 {
        lease, err := s.client.Grant(ctx, int64(ttl.Seconds()))
        if err != nil {
            return err
        }
        _, err = s.client.Put(ctx, s.prefix+key, value,
            clientv3.WithLease(lease.ID))
        return err
    }
    
    _, err := s.client.Put(ctx, s.prefix+key, value)
    return err
}

func (s *EtcdSync) Get(key string) (string, error) {
    ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
    defer cancel()
    
    resp, err := s.client.Get(ctx, s.prefix+key)
    if err != nil {
        return "", err
    }
    
    if len(resp.Kvs) == 0 {
        return "", nil
    }
    
    return string(resp.Kvs[0].Value), nil
}

func (s *EtcdSync) Watch(ctx context.Context, key string) <-chan string {
    ch := make(chan string)
    
    go func() {
        defer close(ch)
        
        rch := s.client.Watch(ctx, s.prefix+key)
        for wresp := range rch {
            for _, ev := range wresp.Events {
                ch <- string(ev.Kv.Value)
            }
        }
    }()
    
    return ch
}
```

---

### 8.3 Lolly 实现建议

参考 NGINX Zone Sync 模块，建议 Lolly 可以考虑以下实现：

1. **模块化设计**
   - 抽象 `StateSync` 接口
   - 支持多种后端（内存、Redis、etcd、Gossip）
   - 插件式注册

2. **配置示例**

```yaml
# config.yaml
cluster:
  enabled: true
  node_id: "node-1"
  bind_addr: "0.0.0.0"
  bind_port: 9000
  join_nodes:
    - "192.168.1.10:9000"
    - "192.168.1.11:9000"
  sync_interval: 500ms
  backend: "gossip"  # 或 "redis", "etcd"

# Redis 后端配置
redis:
  addr: "localhost:6379"
  channel: "lolly:sync"

# etcd 后端配置
etcd:
  endpoints:
    - "localhost:2379"
  prefix: "/lolly/state/"
```

3. **API 设计**

```go
// 定义同步状态接口
type StateSync interface {
    Set(key string, value interface{}, ttl time.Duration) error
    Get(key string) (interface{}, bool)
    Delete(key string) error
    Watch(key string) <-chan StateChange
    Close() error
}

// 状态变更事件
type StateChange struct {
    Key       string
    Value     interface{}
    Deleted   bool
    Timestamp time.Time
}
```

4. **与 NGINX Plus API 兼容**

```go
// 提供与 NGINX Plus 兼容的 REST API
// POST /api/lolly/keyvals/{zone}
// GET  /api/lolly/keyvals/{zone}
// PATCH /api/lolly/keyvals/{zone}
// DELETE /api/lolly/keyvals/{zone}
```

---

## 9. 参考链接

- [NGINX Zone Sync 模块官方文档](https://nginx.org/en/docs/stream/ngx_stream_zone_sync_module.html)
- [NGINX Keyval 模块官方文档](https://nginx.org/en/docs/stream/ngx_stream_keyval_module.html)
- [NGINX Plus API 文档](https://nginx.org/en/docs/http/ngx_http_api_module.html)
- [Lolly 项目 GitHub](https://github.com/xfy/lolly)
- [memberlist (HashiCorp)](https://github.com/hashicorp/memberlist)
- [etcd 官方文档](https://etcd.io/docs/)
