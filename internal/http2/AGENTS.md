<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-04-09 | Updated: 2026-04-09 -->

# http2

## Purpose
HTTP/2 协议支持模块，基于 golang.org/x/net/http2 实现，提供 ALPN 协议协商、与 fasthttp handler 的集成、优雅关闭支持。

## Key Files

| File | Description |
|------|-------------|
| `server.go` | HTTP/2 服务器核心：Server 结构、NewServer、Start、GracefulStop |
| `adapter.go` | fasthttp 适配层：FastHTTPHandlerAdapter、零拷贝头部转换、流式请求体处理 |
| `server_test.go` | 服务器测试：创建、启动、关闭测试 |
| `adapter_test.go` | 适配器测试：头部转换、请求体处理测试 |
| `integration_test.go` | 集成测试：端到端 HTTP/2 请求处理 |

## For AI Agents

### Working In This Directory
- HTTP/2 服务器使用标准库 http.Handler 接口
- 通过适配层转换 fasthttp.RequestHandler → http.Handler
- 需要 TLS 配置进行 ALPN 协商（h2 协议标识）
- 使用 sync.Pool 复用缓冲区实现零拷贝优化
- 预估每请求 5-10µs 适配开销

### Testing Requirements
- 运行测试：`go test ./internal/http2/...`
- 测试需要 TLS 配置（部分测试）
- 集成测试验证完整请求流程

### Common Patterns
- NewServer(cfg, handler, tlsConfig) 创建服务器
- Start() 在现有 TCP 监听器上启动
- GracefulStop() 优雅关闭等待请求完成
- FastHTTPHandlerAdapter.ServeHTTP 实现标准库接口

## Dependencies

### Internal
- `../config/` - HTTP2Config 配置结构体
- `../logging/` - 日志模块

### External
- `golang.org/x/net/http2` - HTTP/2 协议实现
- `github.com/valyala/fasthttp` - fasthttp handler 类型

<!-- MANUAL: -->