<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-04-03 | Updated: 2026-04-03 -->

# rewrite

## Purpose
URL 重写中间件，支持正则表达式匹配和替换，实现 URL 路径转换。

## Key Files

| File | Description |
|------|-------------|
| `rewrite.go` | 重写中间件：Rewrite 结构体、Rule 定义、正则匹配替换 |
| `rewrite_test.go` | 重写测试 |

## For AI Agents

### Working In This Directory
- 使用正则表达式匹配 URL 路径
- 支持捕获组替换（$1、$2 等）
- 重写后继续处理，不终止请求
- 可配置多条规则，按顺序匹配

### Testing Requirements
- 运行测试：`go test ./internal/middleware/rewrite/...`
- 测试正则匹配、捕获组替换、多规则处理

### Common Patterns
- 规则格式：Pattern（正则）→ Replacement（替换字符串）
- 重写后更新 ctx.Path()，不影响路由匹配
- 匹配失败时跳过该规则

## Dependencies

### Internal
- `../` - 中间件接口定义
- `../../config/` - 重写规则配置

<!-- MANUAL: -->