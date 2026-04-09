<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-04-09 | Updated: 2026-04-09 -->

# resolver

## Purpose
DNS 解析器模块，提供带缓存的 DNS 解析功能，支持动态解析后端服务域名、TTL 缓存、后台刷新。

## Key Files

| File | Description |
|------|-------------|
| `resolver.go` | DNS 解析器核心：Resolver 接口、DNSResolver 实现、LookupHost 方法 |
| `cache.go` | DNS 缓存管理：缓存条目结构、TTL 过期、缓存命中统计 |
| `stats.go` | 统计信息：ResolverStats 结构、缓存命中率、解析延迟追踪 |
| `resolver_test.go` | 单元测试：解析功能、缓存行为、错误处理测试 |

## For AI Agents

### Working In This Directory
- Resolver 接口定义：LookupHost、LookupHostWithCache、Refresh、Start、Stop
- 使用 sync.Map 实现并发安全缓存
- 后台刷新需要调用 Start() 启动
- 停止使用时应调用 Stop() 释放资源
- 用于代理模块动态解析 upstream 域名

### Testing Requirements
- 运行测试：`go test ./internal/resolver/...`
- 测试覆盖缓存逻辑和错误处理
- 集成测试在 `../integration/` 目录

### Common Patterns
- 接口设计支持多种解析器实现
- 缓存条目包含 TTL 和过期时间
- 统计信息使用 atomic 计数器

## Dependencies

### Internal
- `../config/` - ResolverConfig 配置结构体

### External
- `net` - Go 标准库 DNS 解析
- `sync` - 并发安全
- `context` - 上下文超时控制

<!-- MANUAL: -->