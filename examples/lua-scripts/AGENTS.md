<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-04-13 | Updated: 2026-04-13 -->

# lua-scripts

## Purpose
Lua 脚本示例集合，展示 lolly Lua 沙箱的各种用法。

## Key Files

| File | Description |
|------|-------------|
| `auth.lua` | Access 阶段认证检查示例（token 验证） |
| `content.lua` | Content 阶段内容生成示例 |
| `log.lua` | Log 阶段日志记录示例 |
| `timer.lua` | 定时器使用示例（ngx.timer.at） |
| `shared_dict.lua` | 共享字典使用示例 |
| `subrequest.lua` | 子请求示例（ngx.location.capture） |

## For AI Agents

### Working In This Directory
- 示例脚本演示典型用法，可作为配置参考
- 脚本使用 OpenResty 兼容的 `ngx.*` API
- 定时器回调不能捕获闭包变量，需使用 `ngx.shared.DICT` 传递数据

### Testing Requirements
- 示例脚本通过集成测试验证功能

### Common Patterns
```lua
-- Access 阶段认证
local auth_header = ngx.req.get_headers()["Authorization"]
if not auth_header then
    ngx.exit(401)
end

-- 定时器（使用 shared_dict 传递数据）
ngx.shared.timer_data:set("key", ngx.var.request_id)
ngx.timer.at(5, function()
    ngx.log(ngx.INFO, ngx.shared.timer_data:get("key"))
end)
```

## Dependencies

### Internal
- `../../internal/lua` - Lua 引擎实现

<!-- MANUAL: -->