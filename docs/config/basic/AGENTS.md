<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-04-23 | Updated: 2026-04-23 -->

# basic

## Purpose
基础配置示例目录，包含静态文件服务器、反向代理、虚拟主机等核心功能的 nginx 配置示例。

## Key Files

| File | Description |
|------|-------------|
| `static-server.conf` | 静态文件服务器配置：sendfile、try_files、缓存策略 |
| `reverse-proxy.conf` | 反向代理配置：proxy_pass、超时设置、缓冲配置 |
| `virtual-host.conf` | 虚拟主机配置：多域名、server_name 匹配 |

## For AI Agents

### Working In This Directory
- 每个配置文件包含 Lolly YAML 对照注释
- 修改基础功能时应参考这些配置了解 nginx 兼容需求
- 配置文件使用 nginx 指令风格，注释说明 Lolly 对应项

### Testing Requirements
- 配置示例通过集成测试验证功能兼容性
- 运行测试：`go test ./internal/integration/...`

### Common Patterns
- 配置对照格式：nginx 指令 ↔ Lolly YAML 配置项
- 注释使用 `# Lolly 对应:` 标记映射关系

## Dependencies

### Internal
- `../../../internal/config/` - Lolly 配置解析实现
- `../../../internal/handler/` - 静态文件处理器

<!-- MANUAL: -->
