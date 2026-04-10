# lua-nginx-module 概述

本文档为 lolly 项目提供 lua-nginx-module 的完整功能参考，帮助理解 OpenResty 核心模块的架构和实现。

## 模块简介

lua-nginx-module 是 OpenResty 的核心模块，将 Lua/LuaJIT 嵌入到 Nginx 中，通过协程实现非阻塞 I/O，让开发者可以用 Lua 编写高性能 Web 应用。

## 核心特性

| 特性 | 说明 |
|------|------|
| **非阻塞 I/O** | 通过 Lua 协程 + Nginx 事件循环实现 |
| **多阶段处理** | 支持 Nginx 11 个请求处理阶段 |
| **共享内存** | Worker 间共享数据字典 |
| **Cosocket** | 非阻塞 TCP/UDP socket API |
| **动态负载均衡** | Lua 控制上游服务器选择 |
| **SSL/TLS 扩展** | 动态证书、Session 缓存自定义 |

## 文档目录

| 文档 | 内容 |
|------|------|
| [01-directives.md](./01-directives.md) | 所有配置指令详解 |
| [02-lua-api.md](./02-lua-api.md) | ngx.* Lua API 参考 |
| [03-cosocket.md](./03-cosocket.md) | 非阻塞 Socket API |
| [04-timer-thread.md](./04-timer-thread.md) | 定时器和用户线程 |
| [05-shdict.md](./05-shdict.md) | 共享内存字典 |
| [06-ssl.md](./06-ssl.md) | SSL/TLS 功能 |
| [07-subrequest.md](./07-subrequest.md) | 内部子请求 |
| [08-balancer.md](./08-balancer.md) | 负载均衡 |
| [09-filter.md](./09-filter.md) | 过滤器链 |
| [10-code-cache.md](./10-code-cache.md) | 代码缓存与异常处理 |
| [11-architecture.md](./11-architecture.md) | 核心架构设计 |

## 源码结构

```
lua-nginx-module/
├── src/
│   ├── ngx_http_lua_module.c      # 主模块定义
│   ├── ngx_http_lua_common.h      # 核心数据结构
│   ├── ngx_http_lua_directive.c   # 指令解析
│   ├── ngx_http_lua_util.c        # 工具函数
│   ├── ngx_http_lua_ctx.c         # 请求上下文
│   ├── ngx_http_lua_cache.c       # 代码缓存
│   ├── ngx_http_lua_coroutine.c   # 协程管理
│   │
│   # Phase Handlers
│   ├── ngx_http_lua_contentby.c
│   ├── ngx_http_lua_accessby.c
│   ├── ngx_http_lua_rewriteby.c
│   ├── ngx_http_lua_headerfilterby.c
│   ├── ngx_http_lua_bodyfilterby.c
│   │
│   # Network
│   ├── ngx_http_lua_socket_tcp.c
│   ├── ngx_http_lua_socket_udp.c
│   ├── ngx_http_lua_subrequest.c
│   │
│   # Memory
│   ├── ngx_http_lua_shdict.c
│   │
│   # SSL
│   ├── ngx_http_lua_ssl_certby.c
│   ├── ngx_http_lua_ssl_session_storeby.c
│   │
│   # Timer & Thread
│   ├── ngx_http_lua_timer.c
│   ├── ngx_http_lua_uthread.c
│   ├── ngx_http_lua_semaphore.c
│   │
│   └── api/
│       └── ngx_http_lua_api.h     # 公共 API 头文件
│
├── t/                              # 测试用例 (Test::Nginx)
└── util/                           # 工具脚本
```

## 版本信息

- 版本: v0.10.29
- 发布日期: 2025-10-24
- 测试用例: 228 个文件, 10,000+ 用例