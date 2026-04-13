<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-04-02 | Updated: 2026-04-13 -->

# docs

## Purpose
项目文档目录，包含实现计划、nginx 功能参考文档、代码规范和开发指南。

## Key Files

| File | Description |
|------|-------------|
| `plan.md` | 实现计划，定义 6 个开发阶段和任务列表（关键文件，高频访问） |
| `README.md` | 项目概述文档 |
| `prompts.md` | 开发提示和 AI 交互指南 |
| `comments.md` | 代码注释规范（必须遵循） |
| `13-git-commit-guide.md` | Git 提交规范指南 |

## Subdirectories

| Directory | Purpose |
|-----------|---------|
| `config/` | nginx 配置示例和功能对照表 |
| `lua/` | Lua 功能文档和 API 参考 |
| `lua-nginx-module/` | lua-nginx-module 文档 |

### nginx 功能参考文档

| File | Description |
|------|-------------|
| `01-nginx-overview.md` | nginx 概述和架构介绍 |
| `02-nginx-installation.md` | nginx 安装指南 |
| `03-nginx-http-core.md` | HTTP 核心功能参考 |
| `04-nginx-proxy-loadbalancing.md` | 反向代理和负载均衡参考 |
| `05-nginx-ssl-https.md` | SSL/HTTPS 配置参考 |
| `06-nginx-rewrite.md` | URL 重写规则参考 |
| `07-nginx-compression-caching.md` | 压缩和缓存配置参考 |
| `08-nginx-logging-monitoring.md` | 日志和监控配置参考 |
| `09-nginx-security.md` | 安全控制参考 |
| `10-nginx-stream-tcp-udp.md` | TCP/UDP Stream 代理参考 |
| `11-nginx-mail-proxy.md` | 邮件代理参考 |
| `12-nginx-performance-tuning.md` | 性能调优参考 |
| `14-nginx-grpc-uwsgi.md` | gRPC/uWSGI/SCGI 代理参考 |
| `15-nginx-advanced-features.md` | 高级特性参考 |
| `16-nginx-internal-redirect.md` | 内部重定向参考 |
| `17-nginx-mirror-slice.md` | 镜像与切片参考 |
| `18-nginx-memcached.md` | Memcached 集成参考 |
| `19-nginx-http-modules-detail.md` | HTTP 功能模块详解 |
| `20-nginx-rate-limiting.md` | 限流与连接控制参考 |
| `21-nginx-http2-http3.md` | HTTP/2 与 HTTP/3 参考 |
| `22-nginx-third-party-modules.md` | 第三方扩展模块参考 |
| `23-nginx-special-modules.md` | 特殊功能模块参考 |
| `24-nginx-core-events.md` | 核心与事件模块参考 |

## For AI Agents

### Working In This Directory
- `plan.md` 是开发的核心参考，定义了各阶段的任务和验证方法
- 修改代码前应先查阅对应的 nginx 参考文档了解功能需求
- 代码注释必须遵循 `comments.md` 规范
- Git 提交格式遵循 `13-git-commit-guide.md`

### Testing Requirements
- 文档无测试要求，但修改代码后需按 `plan.md` 中的验证方法测试

### Common Patterns
- 参考文档采用 nginx 配置对比方式说明功能
- plan.md 使用阶段划分（Phase 1-6）组织任务
- 每个阶段有明确的任务列表和验证方法

## Dependencies

### Internal
- `../internal/` - 实现代码目录，文档描述的功能在此实现

<!-- MANUAL: -->