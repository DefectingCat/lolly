<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-04-23 | Updated: 2026-04-23 -->

# caching

## Purpose
缓存示例目录，演示如何使用 Lua 实现响应缓存、缓存失效等缓存功能。

## Key Files

| File | Description |
|------|-------------|
| `README.md` | 功能说明和使用指南 |
| `cache_handler.lua` | 缓存处理器：缓存键生成、存储/读取、过期管理 |
| `nginx.conf` | NGINX 配置示例：共享字典缓存配置 |

## For AI Agents

### Working In This Directory
- 使用共享字典（shared dict）存储缓存数据
- 缓存键通常基于 URL、请求头、参数组合
- 支持缓存过期时间配置
- 缓存失效策略：TTL、主动清除

### Testing Requirements
- 缓存逻辑通过单元测试验证
- 测试缓存命中/未命中、过期清理

### Common Patterns
- 缓存键：`local key = ngx.md5(ngx.var.uri .. ngx.var.args)`
- 缓存读取：`local data = shared_dict:get(key)`
- 缓存写入：`shared_dict:set(key, data, ttl)`

## Dependencies

### Internal
- `../../../internal/lua/` - Lua 脚本引擎
- `../../../internal/cache/` - 文件缓存实现

<!-- MANUAL: -->
