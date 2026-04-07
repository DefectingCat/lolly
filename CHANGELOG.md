# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/),
and this project adheres to [Semantic Versioning](https://semver.org/).

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
