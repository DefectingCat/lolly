<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-04-09 | Updated: 2026-04-09 -->

# integration

## Purpose
集成测试目录，测试多个模块之间的端到端协作，验证组件间的正确集成。

## Key Files

| File | Description |
|------|-------------|
| `resolver_test.go` | DNS 解析器集成测试：基本解析、缓存功能、网络依赖测试 |
| `variable_test.go` | 变量系统集成测试：变量在日志、代理、重写中的端到端使用 |

## For AI Agents

### Working In This Directory
- 集成测试验证多模块协作，不同于单元测试
- 测试依赖真实网络环境（DNS 解析测试）
- 测试可能因网络原因跳过（Skip）
- 变量系统测试覆盖 nginx 风格格式展开

### Testing Requirements
- 运行测试：`go test ./internal/integration/...`
- 网络测试可能需要外部环境支持
- 测试验证端到端流程而非单个函数

### Common Patterns
- 使用真实配置结构体初始化测试
- 模拟 fasthttp.RequestCtx 进行请求测试
- Skip 处理网络不可用情况

## Dependencies

### Internal
- `../resolver/` - DNS 解析器模块
- `../variable/` - 变量系统模块
- `../config/` - 配置模块
- `../logging/` - 日志模块
- `../middleware/rewrite/` - URL 重写中间件

<!-- MANUAL: -->