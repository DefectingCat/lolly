<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-04-24 | Updated: 2026-04-24 -->

# converter

## Purpose
配置转换器模块，提供外部配置格式到 lolly YAML 配置的转换能力。

## Subdirectories

| Directory | Purpose |
|-----------|---------|
| `nginx/` | nginx 配置转换器（nginx.conf → lolly.yaml） |

## For AI Agents

### Working In This Directory
- 转换器将外部配置格式解析为 `config.Config` 结构体
- 转换过程中可能产生警告（不支持的指令），需要收集并展示给用户
- 每种外部格式有独立的子包实现

### Testing Requirements
- 运行测试：`go test ./internal/converter/...`
- 测试应覆盖各种配置场景和边界情况

### Common Patterns
- 转换器返回 `(*config.Config, []Warning, error)` 三元组
- 警告信息包含文件名、行号和描述

## Dependencies

### Internal
- `../config/` - 配置结构体定义

<!-- MANUAL: -->
