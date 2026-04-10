# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/),
and this project adheres to [Semantic Versioning](https://semver.org/).

## [0.2.0] - 2026-04-10

### Added

#### 核心基础设施

- 变量系统模块，支持内置变量、SSL 客户端证书变量、上游变量，自动注入请求上下文
- DNS 解析器模块，支持自定义 DNS 服务器配置，集成到代理请求处理
- HTTP/2 协议支持，集成到服务器和应用层

#### SSL/TLS 增强

- Session Tickets 支持，含密钥轮换和内存/文件存储后端
- mTLS 客户端证书验证，支持可选/强制验证模式
- TCP/UDP Stream SSL/TLS 支持，完整证书配置

#### 中间件与处理

- auth_request 外部认证中间件，支持子请求验证流程
- static handler alias 指令，支持路径别名映射
- 代理响应临时文件处理，保护内存避免大响应OOM
- rate limiting 后台自动清理和优雅关闭

#### 可观测性

- Prometheus 格式状态输出支持
- 缓存清理 API 端点
- 分层性能回归检测策略与基准测试套件

#### 配置与构建

- Resolver/SSL 默认值完善及 YAML 配置示例输出
- 负载均衡算法配置验证
- golangci-lint 静态检查配置
- 所有构建命令启用静态链接支持
- goimports 替代 go fmt 格式化代码

### Changed

- resolver/variable/ssl 等核心类型重命名，移除冗余前缀
- HeadersMiddleware 重命名移除冗余前缀
- HTTP/2 使用 textproto.CanonicalMIMEHeaderKey 替代手动实现
- 配置废弃字段标记与移除

### Performance

- 一致性哈希虚拟节点哈希值预计算
- 代理缓存使用 uint64 哈希键优化性能

### Documentation

- 新增 variable/resolver 等 AGENTS.md 模块文档
- nginx 健康检查详解与高级模块文档
- 配置字段完整参考文档

### Tests

- 变量系统单元测试与基准测试
- SSL Session Tickets/Stream SSL 测试
- try_files、错误页面、pprof 单元测试

---

## [0.1.0] - 2026-04-07

### Added

#### 核心功能

- 反向代理与负载均衡模块，支持轮询/加权/最少连接/一致性哈希算法
- 故障转移 (next_upstream) 支持，自动切换备用上游
- 文件缓存模块，支持缓存元数据管理
- SSL/TLS 支持，强制 TLS 1.2+，支持证书配置
- HTTP/3 (QUIC) 支持，含 0-RTT 与性能配置验证
- 静态文件服务，支持多静态目录、路径前缀匹配、try_files 配置

#### 中间件系统

- URL 重写中间件，含 ReDoS 保护与循环检测
- gzip/deflate/Brotli 响应压缩中间件
- 请求体大小限制中间件
- 自定义错误页面支持
- 访问日志中间件，支持 nginx combined 格式
- 安全中间件（访问控制、可信代理配置）

#### 可观测性

- pprof 性能分析端点
- 访问/错误日志分离，支持全局格式配置
- 服务器状态 API

#### 配置与构建

- 配置加载模块，支持 YAML/CLI 参数
- 配置验证功能，多项验证函数
- Makefile 构建脚本，含基准测试基础设施
- 程序信号处理（优雅关闭、热升级）

### Changed

- 统一错误处理风格，空白标识符忽略明确不关心的返回值
- 抽取网络工具函数到 netutil 包，移除冗余代码
- 优化字符串构建方式，使用 fmt.Fprintf 替代冗余写法
- 增强 FlagLast 语义与循环检测

### Fixed

- 配置与代码实现不一致问题修复
- Phase 8 问题修复与功能完善

### Documentation

- 添加项目 README 文档
- 核心模块 GoDoc 文档注释
- nginx 模块翻译文档 (Lua/安全/API网关/动态配置/ACME 指南)
- 模块上下文文档 (AGENTS.md)
- 开发计划文档

### Tests

- handler/logging/middleware/server/proxy/cache/loadbalance/security 等模块单元测试
- 核心模块基准测试与回归检测

---

## Initial Development - 2026-04-02 to 2026-04-03

### Project Initialization

- 项目初始化，添加 nginx 文档作为参考
- Makefile 构建脚本与程序入口
- 配置加载模块与 CLI 参数解析
- 基础 HTTP 服务器核心功能

### Core Modules (Phase 1-4)

- 应用逻辑抽取到 internal/app 包
- 信号处理与配置结构完善
- 反向代理与负载均衡实现
- SSL/TLS 与安全中间件
- 日志模块增强
- 文件缓存实现
- URL 重写与压缩中间件

### Performance & Integration (Phase 5-7)

- 访问日志中间件
- 性能优化与热升级
- 访问控制与可信代理配置
- Phase 6-7 功能完善与测试覆盖

### HTTP/3 & Advanced Features (Phase 8-9)

- HTTP/3 (QUIC) 支持
- 配置验证增强
- Brotli 压缩支持
- pprof 性能分析端点
- 故障转移支持
- 自定义错误页面
- 请求体大小限制
- try_files 配置支持
