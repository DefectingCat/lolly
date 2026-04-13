<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-04-02 | Updated: 2026-04-13 -->

# internal

## Purpose
核心业务代码目录，包含服务器、配置、处理器、中间件、日志等模块。Go 的 internal 包机制确保这些代码不可被外部项目导入。

## Subdirectories

| Directory | Purpose |
|-----------|---------|
| `app/` | 应用程序入口和运行逻辑（启动、信号处理、版本信息） |
| `benchmark/` | 基准测试基础设施（Mock 后端、负载生成器、回归检测） |
| `cache/` | 文件缓存模块（缓存存储、过期管理） |
| `config/` | 配置解析、验证和默认值生成 |
| `handler/` | HTTP 请求处理器（路由、静态文件、Sendfile） |
| `http2/` | HTTP/2 协议支持（ALPN 协商、fasthttp 适配） |
| `http3/` | HTTP/3 (QUIC) 协议支持（fasthttp 适配、0-RTT） |
| `integration/` | 集成测试（多模块端到端协作验证） |
| `loadbalance/` | 负载均衡策略（轮询、最少连接、健康检查） |
| `logging/` | 日志系统（zerolog 初始化、访问日志） |
| `lua/` | Lua 脚本引擎（OpenResty 风格沙箱、ngx API） |
| `middleware/` | 中间件框架（接口定义、链式组合） |
| `mimeutil/` | MIME 类型检测（扩展名映射、类型推断） |
| `netutil/` | 网络工具函数（客户端 IP 提取、URL 解析） |
| `proxy/` | 反向代理模块（HTTP/WebSocket 代理） |
| `resolver/` | DNS 解析器（缓存、后台刷新、域名动态解析） |
| `server/` | HTTP 服务器核心、虚拟主机、热升级、状态监控 |
| `ssl/` | SSL/TLS 管理（证书加载、OCSP Stapling） |
| `sslutil/` | SSL 工具函数（证书池加载、CA 信任链） |
| `stream/` | TCP/UDP Stream 代理模块 |
| `utils/` | HTTP 错误处理（统一错误响应助手） |
| `variable/` | 变量系统（nginx 风格变量展开、日志格式模板） |

## For AI Agents

### Working In This Directory
- 所有包使用 `rua.plus/lolly/internal/{package}` 导入路径
- 各子包有独立职责，遵循 Go 包设计原则
- 添加新功能时应参考 `docs/plan.md` 确定所属模块
- 测试文件与源文件同目录，使用 `_test.go` 后缀

### Testing Requirements
- 每个包应有对应的测试文件
- 运行测试：`go test ./internal/...`
- 测试覆盖率目标 >80%

### Common Patterns
- 使用 fasthttp 的 `RequestHandler` 函数签名处理请求
- 配置结构体使用 `yaml` 标签
- 中间件通过 `Chain.Apply()` 逆序包装
- 服务器通过 `fasthttp.Server` 配置超时和连接限制

## Dependencies

### External
- `github.com/valyala/fasthttp` - HTTP 服务器框架
- `github.com/fasthttp/router` - 路由器
- `github.com/rs/zerolog` - 日志库
- `gopkg.in/yaml.v3` - YAML 解析

<!-- MANUAL: -->