# API Gateway 示例项目

基于 OpenResty / lua-nginx-module 的轻量级 API 网关示例，演示如何使用 Lua 脚本实现网关核心功能。

## 功能特性

| 功能 | 说明 |
|------|------|
| 动态路由 | 基于路径、方法、Header 的规则路由 |
| API Key 认证 | 请求级 API Key 校验 |
| 限流保护 | 基于令牌桶的滑动窗口限流 |
| 上游管理 | 动态健康检查 + 故障剔除 |
| 响应包装 | 统一错误格式 |

## 文件说明

| 文件 | 用途 |
|------|------|
| `gateway.lua` | 网关主逻辑（路由、认证、限流、错误处理） |
| `upstream.lua` | 上游服务管理（健康检查、故障节点剔除、动态负载均衡） |
| `nginx.conf` | NGINX 配置示例（展示如何集成 Lua 脚本） |

## 快速开始

1. 安装 OpenResty
2. 将 `nginx.conf` 复制到 `/usr/local/openresty/nginx/conf/`
3. 按需修改 `gateway.lua` 中的路由规则和 `upstream.lua` 中的上游节点
4. 启动: `openresty -c /usr/local/openresty/nginx/conf/nginx.conf`

## 配置示例

### 添加路由规则 (gateway.lua)

```lua
-- 在 routes 表中添加新路由
["/v2/users"] = {
    method = {"GET", "POST"},
    upstream = "user_service",
    auth = true,
    rate_limit = 100,
}
```

### 添加上游节点 (upstream.lua)

```lua
-- 在 upstreams 表中添加新服务
["payment_service"] = {
    nodes = {
        { host = "10.0.0.30", port = 8083, weight = 1 },
        { host = "10.0.0.31", port = 8083, weight = 1 },
    },
    health_check_interval = 10,
}
```

## 注意事项

- 本示例用于演示目的，生产环境建议配合外部认证服务和分布式限流
- API Key 存储建议使用 Redis 等外部存储替代 Lua 共享字典
- 健康检查适用于单节点场景，多节点请使用 `lua-resty-healthcheck`
