# Rate Limiting with Lua

基于 `ngx.shared.DICT` 的自定义速率限制示例，弥补 NGINX 原生 `limit_req` 模块在动态策略上的不足。

## 功能

- 基于客户端 IP 的请求速率限制
- 基于 API Key 的请求速率限制
- 自定义限流阈值与时间窗口
- 返回标准的 `429 Too Many Requests` 响应

## 文件说明

| 文件 | 用途 |
|------|------|
| `nginx.conf` | NGINX 配置示例，定义共享内存区域和 Lua 挂载点 |
| `access.lua` | Lua 速率限制实现脚本 |

## 快速开始

1. 在 `nginx.conf` 中配置 `lua_shared_dict` 共享内存区域
2. 在对应的 `location` 块中使用 `access_by_lua_file` 引入脚本
3. 通过请求头 `X-API-Key` 或客户端 IP 进行限流

## 配置示例

```nginx
http {
    # 定义共享内存：名称 大小
    lua_shared_dict rate_limit 10m;

    server {
        location /api/ {
            access_by_lua_file /path/to/access.lua;
            proxy_pass http://backend;
        }
    }
}
```

## 限流策略

| 标识方式 | 优先级 | 默认阈值 | 时间窗口 |
|----------|--------|----------|----------|
| API Key (`X-API-Key`) | 高 | 100 req | 60 秒 |
| 客户端 IP | 低 | 20 req | 60 秒 |

当同时存在 API Key 和 IP 时，优先使用 API Key 进行限流。
