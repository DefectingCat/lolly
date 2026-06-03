# Skill: 更新 -g 默认配置模板

当 config struct 新增/修改字段时，同步更新 `GenerateConfigYAML` 输出模板。

## 涉及文件

| 文件 | 职责 |
|------|------|
| `internal/config/*.go` | struct 定义（改字段） |
| `internal/config/defaults.go` | `DefaultConfig()` 默认值 + `GenerateConfigYAML()` 模板 |
| `internal/config/defaults_test.go` | `TestGenerateConfigYAML*` 覆盖率测试 |

## 更新流程

### 1. 修改 struct

在对应的 `*_config.go` 文件中添加/修改字段，确保：
- 有 `yaml` tag
- 有中文注释说明字段用途和有效值

### 2. 更新 DefaultConfig()

在 `defaults.go` 的 `DefaultConfig()` 函数中为新字段设置合理默认值。

### 3. 更新 GenerateConfigYAML()

在 `defaults.go` 的 `GenerateConfigYAML()` 函数中，找到对应区块，添加/修改模板行。

**规则：**

1. **格式一致性**：使用 `buf.WriteString` 写注释行，`fmt.Fprintf` 写带默认值的行
2. **注释行前缀**：可选配置用 `#` 注释，必填/有默认值的配置直接输出
3. **注释规范**：每行末尾 `#` 注释说明用途，新字段加 `# 有效值: a, b, c` 格式的可选值说明
4. **默认值来源**：使用 `cfg.Servers[0].Xxx` 读取 `DefaultConfig()` 的值，不硬编码
5. **缩进层级**：
   - 顶层配置无缩进（`logging:`, `http3:`）
   - server 内配置 4 空格（`    listen:`）
   - server 子配置 6-8 空格（`      access:`）
   - 注释中的示例配置也保持正确缩进

### 4. struct 字段 → GenerateConfigYAML 区块对照表

| struct 文件 | GenerateConfigYAML 区块 |
|-------------|------------------------|
| `config.go` → `Config.Mode` | mode 注释块（~L254） |
| `config.go` → `Config.Mode` | servers 开头（~L263） |
| `server_config.go` → `ServerConfig` | servers 列表内（L264-288） |
| `server_config.go` → `TypesConfig` | types 注释块（~L292） |
| `server_config.go` → `LimitRateConfig` | limit_rate 注释块（~L302） |
| `monitoring_config.go` → `CacheAPIConfig` | cache_api 注释块（~L310） |
| `variable_config.go` → `LuaMiddlewareConfig` | lua 注释块（~L324） |
| `variable_config.go` → `CompressionConfig` | compression 块（~L607） |
| `variable_config.go` → `RewriteRule` | rewrite 注释块（~L598） |
| `server_config.go` → `StaticConfig` | static 块（~L349） |
| `proxy_config.go` → `ProxyConfig` | proxy 注释块（~L385） |
| `proxy_config.go` → `ProxyTimeout` | proxy.timeout（~L413） |
| `proxy_config.go` → `ProxyBufferingConfig` | proxy.buffering（~L418） |
| `proxy_config.go` → `HealthCheckConfig` | proxy.health_check（~L403） |
| `proxy_config.go` → `ProxyHeaders` | proxy.headers（~L424） |
| `cache_config.go` → `ProxyCacheConfig` | proxy.cache（~L435） |
| `cache_config.go` → `ProxyCacheValidConfig` | proxy.cache_valid（~L448） |
| `proxy_config.go` → `ProxySSLConfig` | proxy.proxy_ssl（~L454） |
| `proxy_config.go` → `NextUpstreamConfig` | proxy.next_upstream（~L464） |
| `proxy_config.go` → `BalancerByLuaConfig` | proxy.balancer_by_lua（~L467） |
| `proxy_config.go` → `RedirectRewriteConfig` | proxy.redirect_rewrite（~L472） |
| `ssl_config.go` → `SSLConfig` | ssl 注释块（~L486） |
| `ssl_config.go` → `SessionTicketsConfig` | ssl.session_tickets（~L506） |
| `ssl_config.go` → `ClientVerifyConfig` | ssl.client_verify（~L511） |
| `performance_config.go` → `HTTP2Config` | ssl.http2（~L517） |
| `security_config.go` → `SecurityConfig` | security 块（~L534） |
| `security_config.go` → `AccessConfig`/`GeoIPConfig` | security.access/geoip（~L537） |
| `security_config.go` → `RateLimitConfig` | security.rate_limit（~L555） |
| `security_config.go` → `AuthConfig` | security.auth（~L564） |
| `security_config.go` → `SecurityHeaders` | security.headers（~L573） |
| `security_config.go` → `ErrorPageConfig` | security.error_page（~L581） |
| `security_config.go` → `AuthRequestConfig` | security.auth_request（~L587） |
| `performance_config.go` → `ShutdownConfig` | shutdown 块（~L623） |
| `server_config.go` → `StreamConfig` | stream 注释块（~L631） |
| `monitoring_config.go` → `LoggingConfig` | logging 块（~L662） |
| `performance_config.go` → `PerformanceConfig` | performance 块（~L676） |
| `performance_config.go` → `HTTP3Config` | http3 块（~L693） |
| `monitoring_config.go` → `MonitoringConfig` | monitoring 块（~L703） |
| `performance_config.go` → `ResolverConfig` | resolver 块（~L723） |
| `variable_config.go` → `VariablesConfig` | variables 块（~L737） |
| `variable_config.go` → `IncludeConfig` | include 注释块（~L745） |

### 5. 运行验证

```bash
# 测试模板覆盖率（检查 struct 字段是否都在模板中出现）
go test -v -run TestGenerateConfigYAML ./internal/config/...

# 测试 app 层 generateConfig 函数
go test -v -run TestGenerateConfig ./internal/app/...

# 完整测试 + lint
make test
make lint
```

## 添加新字段的模板示例

### 简单字符串字段（可选，注释形式）
```go
buf.WriteString("    #   new_field: \"\"           # 新字段说明（有效值: a, b, c）\n")
```

### 带默认值的字段（使用 fmt.Fprintf）
```go
fmt.Fprintf(&buf, "    #   new_field: %d          # 新字段说明\n", cfg.Servers[0].NewField)
```

### 新的嵌套结构体
```go
buf.WriteString("    # new_section:                 # 新区块说明\n")
buf.WriteString("    #   enabled: false            # 是否启用\n")
buf.WriteString("    #   option: \"\"               # 选项说明\n")
buf.WriteString("\n")
```

## 常见遗漏检查清单

添加新字段后，逐项确认：

- [ ] struct 有 `yaml` tag
- [ ] `DefaultConfig()` 有默认值
- [ ] `GenerateConfigYAML()` 有注释行（可选字段）或输出行（有默认值的字段）
- [ ] 注释中包含 `有效值:` 说明（枚举类字段）
- [ ] `fmt.Fprintf` 使用 `cfg.Servers[0].Xxx` 而非硬编码数字
- [ ] `TestGenerateConfigYAMLFieldsCoverage` 通过（它会检查 struct 字段是否出现在模板中）
- [ ] `make lint` 无新增 warning
