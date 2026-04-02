<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-04-02 | Updated: 2026-04-02 -->

# config

## Purpose
配置模块，提供 YAML 配置文件的解析、验证、默认值生成和序列化功能。是整个项目的核心依赖模块（高频访问）。

## Key Files

| File | Description |
|------|-------------|
| `config.go` | 配置结构体定义：Config、ServerConfig、ProxyConfig、SSLConfig 等 |
| `defaults.go` | 默认配置生成：DefaultConfig()、GenerateConfigYAML()（高频访问） |
| `validate.go` | 配置验证：validateServer、validateSSL、validateSecurity 等 |
| `config_test.go` | 配置解析测试 |
| `defaults_test.go` | 默认配置测试 |
| `validate_test.go` | 验证逻辑测试 |

## For AI Agents

### Working In This Directory
- 所有配置结构体使用 `yaml` 标签，支持 YAML 序列化/反序列化
- `Load()` 加载配置文件，自动调用验证
- `DefaultConfig()` 返回安全默认值（TLS 1.2+，安全头部等）
- `GenerateConfigYAML()` 生成带注释的配置模板
- `defaults.go` 是高频访问文件，修改需谨慎

### Testing Requirements
- 测试覆盖完整：结构体解析、默认值、验证逻辑
- 运行测试：`go test ./internal/config/...`
- 验证测试使用 table-driven 方式

### Common Patterns
- 使用 `gopkg.in/yaml.v3` 解析 YAML
- 验证函数按配置类型分组：validateServer、validateSSL、validateSecurity
- 时间配置使用 `time.Duration`，YAML 中用秒数或带单位字符串（如 "10s"）
- 安全配置强制要求：Basic Auth 启用时必须配置 SSL

## Dependencies

### External
- `gopkg.in/yaml.v3` - YAML 解析库

<!-- MANUAL: -->