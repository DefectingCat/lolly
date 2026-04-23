<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-04-23 | Updated: 2026-04-23 -->

# logging-monitoring

## Purpose
日志和监控示例目录，演示如何使用 Lua 实现自定义日志格式、指标收集等监控功能。

## Key Files

| File | Description |
|------|-------------|
| `README.md` | 功能说明和使用指南 |
| `log_formatter.lua` | 日志格式化器：自定义日志格式、字段提取 |
| `metrics.lua` | 指标收集：请求计数、延迟统计、Prometheus 格式 |
| `nginx.conf` | NGINX 配置示例：日志钩子集成 |

## For AI Agents

### Working In This Directory
- 日志格式支持 JSON、文本格式
- 指标可导出为 Prometheus 格式
- 支持请求级别的详细日志
- 监控数据可存储到共享字典

### Testing Requirements
- 日志和指标逻辑通过单元测试验证
- 测试日志格式、指标计算正确性

### Common Patterns
- 日志字段：`ngx.log(ngx.INFO, json.encode(log_entry))`
- 指标计数：`metrics_dict:incr("request_count")`
- Prometheus 格式：`# TYPE http_requests_total counter`

## Dependencies

### Internal
- `../../../internal/lua/` - Lua 脚本引擎
- `../../../internal/logging/` - 日志系统实现

<!-- MANUAL: -->
