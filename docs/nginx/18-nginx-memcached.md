## 1. 模块概述

### ngx_http_memcached_module
- 与 Memcached 缓存服务器交互
- 支持从 Memcached 读取数据
- 适合缓存动态内容

### 特性
- 通过 key 从 Memcached 读取
- 与 nginx 变量系统集成
- 支持 upstream 负载均衡

## 2. 核心指令

| 指令 | 说明 | 默认值 |
|------|------|--------|
| memcached_pass | Memcached 服务器地址 | - |
| memcached_bind | 本地绑定地址 | - |
| memcached_connect_timeout | 连接超时 | 60s |
| memcached_read_timeout | 读取超时 | 60s |
| memcached_send_timeout | 发送超时 | 60s |
| memcached_buffer_size | 缓冲区大小 | 4k/8k |
| memcached_next_upstream | 失败转发条件 | error timeout |
| memcached_gzip_flag | gzip 标志位 | - |

## 3. 关键变量

| 变量 | 说明 |
|------|------|
| $memcached_key | 缓存键（必须设置）|
| $memcached_expires | 过期时间（秒）|

## 4. 配置示例

### 基础配置
```nginx
location /cache/ {
    set $memcached_key "$uri?$args";
    memcached_pass 127.0.0.1:11211;
    
    # 缓存未命中回源
    error_page 404 502 504 = @fallback;
}

location @fallback {
    proxy_pass http://backend;
}
```

### 多服务器负载均衡
```nginx
upstream memcached_backend {
    server 192.168.1.10:11211 weight=5;
    server 192.168.1.11:11211;
    keepalive 32;
}

server {
    location /api/ {
        set $memcached_key "api:$uri:$args";
        memcached_pass memcached_backend;
        error_page 404 = @api_backend;
    }
}
```

### API 缓存示例
```nginx
location /user/ {
    # 从缓存读取用户数据
    set $memcached_key "user:$arg_id";
    memcached_pass 127.0.0.1:11211;
    memcached_connect_timeout 100ms;
    memcached_read_timeout 200ms;
    
    # 未命中时查询数据库
    error_page 404 = @database;
}

location @database {
    proxy_pass http://app_server;
    
    # 响应后写入缓存（需应用逻辑）
    # 或使用 srcache-nginx-module
}
```

## 5. 与 proxy_cache 对比

| 特性 | memcached_module | proxy_cache |
|------|------------------|-------------|
| 数据来源 | Memcached 服务 | 本地磁盘 |
| 缓存方式 | 主动读取 | 被动缓存 |
| 预填充 | 支持 | 不支持 |
| 分布式 | 原生支持 | 单机 |
| 写入支持 | 需外部工具 | 自动 |

## 6. 应用场景

| 场景 | 说明 |
|------|------|
| API 缓存 | 缓存 JSON 响应 |
| Session 存储 | 分布式 session 读取 |
| 页面片段缓存 | 头部、侧边栏组件 |
| 临时数据 | 高频读取的低频变化数据 |

## 7. 最佳实践

### 缓存键设计
```nginx
# API 请求：包含完整路径和参数
set $memcached_key "api:$request_uri";

# 用户数据：包含用户 ID
set $memcached_key "user:$cookie_user_id";

# 页面片段：包含版本号
set $memcached_key "fragment:$uri:v2";
```

### 超时设置
```nginx
# 快速失败，避免阻塞
memcached_connect_timeout 100ms;
memcached_read_timeout 200ms;
```

### 错误处理
```nginx
# 始终配置回源
error_page 404 502 504 = @fallback;

# 多服务器故障转移
memcached_next_upstream error timeout invalid_response;
```

### 连接复用
```nginx
upstream memcached_servers {
    server 10.0.0.1:11211;
    server 10.0.0.2:11211;
    keepalive 64;  # 保持连接池
}
```

## 8. 注意事项

- 模块只支持读取，不支持写入
- 写入缓存需要应用服务器或第三方模块
- 建议使用短超时（100-500ms）
- 使用 upstream + keepalive 提高性能
