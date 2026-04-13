<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-04-13 | Updated: 2026-04-13 -->

# config

## Purpose
nginx 配置示例目录，展示 lolly 需要兼容的功能特性，用于功能对照和迁移参考。

## Key Files

| File | Description |
|------|-------------|
| `README.md` | 目录结构和功能对照表（关键参考文件） |

## Subdirectories

| Directory | Purpose |
|-----------|---------|
| `basic/` | 基础配置：静态服务器、反向代理、虚拟主机 |
| `ssl/` | SSL/TLS 配置：HTTPS、mTLS、OCSP、HSTS |
| `load-balancing/` | 负载均衡：轮询、加权、最少连接、IP 哈希、一致性哈希 |
| `advanced/` | 高级功能：WebSocket、gRPC、HTTP/2、HTTP/3、Stream |
| `security/` | 安全配置：限流、连接限制、访问控制、认证、安全头 |
| `caching/` | 缓存配置：代理缓存、Gzip、Brotli |
| `rewriting/` | URL 重写：rewrite 规则、重定向 |
| `lua/` | Lua 配置：基础 Lua、access_by_lua、content_by_lua、balancer_by_lua |

## For AI Agents

### Working In This Directory
- `README.md` 包含 nginx 指令与 lolly 配置对照表（关键参考）
- 每个子目录包含 nginx 配置文件和对应的 lolly YAML 注释
- 修改功能时应先查阅对应目录了解 nginx 兼容需求

### Testing Requirements
- 配置示例通过集成测试验证功能兼容性

### Common Patterns
- 配置对照格式：nginx 指令 ↔ lolly YAML 配置项
- 功能覆盖：负载均衡、SSL、安全、代理、缓存、重写、Lua

## Dependencies

### Internal
- `../../internal/config` - lolly 配置解析实现

<!-- MANUAL: -->