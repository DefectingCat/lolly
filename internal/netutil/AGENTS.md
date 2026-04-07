<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-04-07 | Updated: 2026-04-07 -->

# netutil

## Purpose
网络相关的通用工具函数包，提供客户端 IP 提取、URL 解析等功能，供 proxy、middleware、server 等模块共享使用。

## Key Files

| File | Description |
|------|-------------|
| `ip.go` | 客户端 IP 提取函数，支持 X-Forwarded-For、X-Real-IP 头 |
| `url.go` | URL 解析函数，提取主机地址和 TLS 标志 |
| `ip_test.go` | IP 提取函数单元测试 |
| `url_test.go` | URL 解析函数单元测试 |

## For AI Agents

### Working In This Directory
- IP 提取顺序：X-Forwarded-For 第一个 IP → X-Real-IP → RemoteAddr
- `ExtractClientIP` 返回字符串，适用于日志记录
- `ExtractClientIPNet` 返回 net.IP，适用于 CIDR 匹配等网络操作
- URL 解析支持 http:// 和 https:// 前缀，自动添加默认端口

### Testing Requirements
- 运行测试：`go test ./internal/netutil/...`
- 测试覆盖：各种代理头组合、URL 格式解析

### Common Patterns
- 提取客户端 IP：`ExtractClientIP(ctx)` → "192.168.1.1"
- 提取 net.IP：`ExtractClientIPNet(ctx)` → net.IP 对象
- 解析 URL：`ParseTargetURL("https://api.example.com", true)` → ("api.example.com:443", true)
- 提取主机：`ExtractHost("http://backend:8080/path")` → "backend:8080"

## Dependencies

### External
- `github.com/valyala/fasthttp` - HTTP 请求上下文

<!-- MANUAL: -->