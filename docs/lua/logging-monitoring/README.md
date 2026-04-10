# Logging & Monitoring

Lua 示例项目，演示如何在 Nginx 中使用 Lua 进行结构化日志输出和请求指标收集。

## 文件说明

| 文件 | 说明 |
|------|------|
| `nginx.conf` | Nginx 配置示例，集成 Lua 日志和指标模块 |
| `log_formatter.lua` | 自定义 JSON 结构化日志格式化 |
| `metrics.lua` | 请求耗时统计与指标收集 |

## 使用方法

1. 将 `nginx.conf` 中的路径调整为实际部署路径
2. 在 `nginx.conf` 的 `http` 块中引入 Lua 模块
3. 重启 Nginx 即可生效

## 日志输出格式

```json
{
  "time": "2026-04-10T12:00:00Z",
  "remote_addr": "127.0.0.1",
  "method": "GET",
  "uri": "/api/users",
  "status": 200,
  "request_time": 0.035,
  "upstream_time": 0.028,
  "body_bytes_sent": 1024
}
```
