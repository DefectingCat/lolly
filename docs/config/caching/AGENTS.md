<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-04-23 | Updated: 2026-04-23 -->

# caching

## Purpose
缓存配置示例目录，包含代理缓存、Gzip 压缩、Brotli 压缩等性能优化配置。

## Key Files

| File | Description |
|------|-------------|
| `proxy-cache.conf` | 代理缓存：响应缓存、缓存键、过期策略 |
| `gzip.conf` | Gzip 压缩：文本压缩、压缩级别、MIME 类型 |
| `brotli.conf` | Brotli 压缩：更高压缩率、现代浏览器支持 |

## For AI Agents

### Working In This Directory
- 代理缓存适用于后端响应不频繁变化的场景
- Gzip 是通用压缩方案，所有浏览器支持
- Brotli 压缩率更高，但需要浏览器支持
- 压缩级别建议 4-6，平衡 CPU 和压缩率

### Testing Requirements
- 缓存配置通过集成测试验证
- 压缩测试验证 Content-Encoding 头

### Common Patterns
- 缓存键：`$scheme$host$request_uri`
- 缓存控制：`X-Cache-Status` 头
- 压缩条件：`gzip_types text/plain text/css application/json`

## Dependencies

### Internal
- `../../../internal/cache/` - 文件缓存实现
- `../../../internal/middleware/compression/` - 压缩中间件

<!-- MANUAL: -->
