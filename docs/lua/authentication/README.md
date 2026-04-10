# Authentication 示例项目

基于 OpenResty / lua-nginx-module 的身份认证示例，演示 JWT 校验和 Basic Auth 验证两种常见场景。

## 功能特性

| 功能 | 说明 |
|------|------|
| JWT 验证 | HMAC-SHA256 签名校验，从 Header 提取 Token |
| Basic Auth | 标准 HTTP Basic Authentication 验证 |

## 文件说明

| 文件 | 用途 |
|------|------|
| `jwt_validate.lua` | JWT 签名与过期校验（HMAC-SHA256） |
| `basic_auth.lua` | Basic Auth 用户名密码验证 |
| `nginx.conf` | NGINX 配置示例（展示两种认证方式的集成） |

## 快速开始

1. 安装 OpenResty
2. 将 `nginx.conf` 复制到你的 NGINX 配置目录
3. 修改 JWT Secret 和用户凭据
4. 启动: `openresty -c /path/to/nginx.conf`

## 注意事项

- JWT 示例使用 HMAC-SHA256 算法，不使用外部依赖库（纯 Lua 实现解码+签名）
- Basic Auth 示例使用硬编码用户表，生产环境请对接数据库或 Redis
- 生产环境建议通过 HTTPS 传输认证信息
