<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-04-03 | Updated: 2026-04-03 -->

# cache

## Purpose
文件缓存模块，提供静态文件的内存缓存和过期管理功能。

## Key Files

| File | Description |
|------|-------------|
| `file_cache.go` | 文件缓存核心：FileCache 结构体、Get/Set/Delete、TTL 管理、LRU 淘汰 |
| `cache_test.go` | 缓存单元测试 |

## For AI Agents

### Working In This Directory
- 缓存项包含文件内容、MIME 类型、最后修改时间
- TTL 默认 5 分钟，可配置
- LRU 淘汰策略，最大缓存大小可配置
- 支持缓存失效和手动清除

### Testing Requirements
- 运行测试：`go test ./internal/cache/...`
- 测试缓存存取、过期淘汰、并发安全

### Common Patterns
- 使用 `sync.RWMutex` 保证并发安全
- 缓存键为文件路径的哈希值
- 缓存命中时直接返回内容，避免磁盘 I/O

## Dependencies

### Internal
- `../config/` - 缓存配置参数

<!-- MANUAL: -->