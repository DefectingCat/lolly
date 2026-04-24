<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-04-24 | Updated: 2026-04-24 -->

# nginx

## Purpose
nginx 配置转换器，将 nginx.conf 格式转换为 lolly YAML 配置。支持 server、location、upstream、proxy_pass 等核心指令。

## Key Files

| File | Description |
|------|-------------|
| `parser.go` | nginx 配置解析器：词法分析、语法树构建 |
| `converter.go` | 配置转换器：nginx AST → lolly Config |
| `parser_test.go` | 解析器测试：各种 nginx 配置格式 |
| `converter_test.go` | 转换器测试：指令映射、警告生成 |

## For AI Agents

### Working In This Directory
- 解析器将 nginx 配置文本解析为 `NginxConfig` AST
- 转换器遍历 AST 生成 `config.Config` 结构体
- 不支持的指令会生成 `Warning` 而非错误，允许部分转换
- location 匹配类型映射：`=` → `exact`，`^~` → `prefix_priority`，`~` → `regex`，`~*` → `regex_caseless`

### Testing Requirements
- 运行测试：`go test ./internal/converter/nginx/...`
- 测试覆盖：完整配置、部分配置、错误处理

### Common Patterns
```go
// 解析 nginx 配置
parser := nginx.NewParser(content)
cfg, err := parser.Parse()

// 转换为 lolly 配置
result, err := nginx.Convert(cfg)
// result.Config - 转换后的配置
// result.Warnings - 转换警告
```

### 支持的 nginx 指令

| 指令 | 转换说明 |
|------|----------|
| `server` | 转换为 `ServerConfig` |
| `listen` | 转换为 `listen` 字段 |
| `server_name` | 转换为 `name`/`server_names` |
| `location` | 转换为 `ProxyConfig` 或 `StaticConfig` |
| `proxy_pass` | 转换为代理目标 |
| `root`/`alias` | 转换为静态文件根目录 |
| `upstream` | 转换为负载均衡目标列表 |
| `gzip` | 转换为压缩配置 |
| `ssl_certificate` | 转换为 SSL 证书配置 |
| `rewrite` | 转换为 URL 重写规则 |
| `return 301/302` | 转换为重定向规则 |

### 不支持的指令（生成警告）

- `if`、`map`、`set` - 条件逻辑不支持
- `limit_req`、`limit_conn` - 使用 `rate_limit` 配置替代
- `add_header` - 使用 `security.headers` 配置替代
- `auth_request` - 使用 `security.auth_request` 配置替代

## Dependencies

### Internal
- `../../config/` - 配置结构体定义

<!-- MANUAL: -->
