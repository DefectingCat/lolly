<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-04-23 | Updated: 2026-04-23 -->

# lua

## Purpose
Lua 配置示例目录，包含 Lua 脚本集成、各阶段钩子、共享字典等 Lua 功能配置。

## Key Files

| File | Description |
|------|-------------|
| `basic-lua.conf` | 基础 Lua 配置：脚本加载、基本语法 |
| `access-by-lua.conf` | access 阶段 Lua：访问控制、认证检查 |
| `content-by-lua.conf` | content 阶段 Lua：动态内容生成 |
| `balancer-by-lua.conf` | 负载均衡 Lua：动态上游选择 |
| `shared-dict.conf` | 共享字典：进程间数据共享、缓存 |

## For AI Agents

### Working In This Directory
- Lua 钩子按 nginx 处理阶段执行：rewrite → access → content
- 共享字典用于进程间数据共享，适合缓存场景
- Lua 代码在沙箱中执行，有安全限制
- 参考 OpenResty 风格的 ngx API

### Testing Requirements
- Lua 脚本通过集成测试验证
- 运行测试：`go test ./internal/lua/...`

### Common Patterns
- 访问控制：`access_by_lua_block { ... }`
- 动态内容：`content_by_lua_block { ngx.say(...) }`
- 共享字典：`lua_shared_dict cache 10m`

## Dependencies

### Internal
- `../../../internal/lua/` - Lua 脚本引擎实现
- `../../../docs/lua-nginx-module/` - lua-nginx-module 文档参考

### External
- `github.com/yuin/gopher-lua` - Go Lua 解释器

<!-- MANUAL: -->
