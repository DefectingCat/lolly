<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-04-23 | Updated: 2026-04-23 -->

# middleware

## Purpose
中间件示例目录，演示 Lua 中间件的概念和用法。

## Key Files

| File | Description |
|------|-------------|
| `README.md` | 中间件概念说明和使用指南 |

## For AI Agents

### Working In This Directory
- 中间件按处理阶段执行：rewrite → access → content → log
- 中间件可链式组合
- 支持请求修改和响应拦截
- 参考 OpenResty 风格的中间件模式

### Testing Requirements
- 中间件逻辑通过集成测试验证
- 测试中间件链执行顺序

### Common Patterns
- 链式调用：按注册顺序执行
- 请求修改：`ngx.req.set_header()`
- 响应拦截：`ngx.exit()` 提前返回

## Dependencies

### Internal
- `../../../internal/lua/` - Lua 脚本引擎
- `../../../internal/middleware/` - 中间件框架实现

<!-- MANUAL: -->
