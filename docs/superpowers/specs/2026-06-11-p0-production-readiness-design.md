# P0: 生产可用性特性设计

日期: 2026-06-11
状态: Approved

## 概述

5 个独立特性，使 lolly 达到生产可部署状态。所有特性互不依赖，可完全并行开发。

---

## 1. CORS 中间件

### 配置

在 `security.cors` 下新增 `CORSConfig` 结构体，server 级别配置：

```yaml
security:
  cors:
    enabled: true
    allowed_origins: ["https://example.com", "https://api.example.com"]
    allowed_methods: ["GET", "POST", "PUT", "DELETE", "OPTIONS"]
    allowed_headers: ["Content-Type", "Authorization", "X-Request-ID"]
    expose_headers: ["X-Total-Count"]
    allow_credentials: true
    max_age: 3600
```

字段说明：
- `enabled`: 启用开关（默认 false）
- `allowed_origins`: 允许的源列表，支持 `"*"` 通配（与 `allow_credentials: true` 互斥，遵循 CORS 规范）
- `allowed_methods`: 允许的 HTTP 方法
- `allowed_headers`: 允许的请求头
- `expose_headers`: 允许前端读取的响应头
- `allow_credentials`: 是否允许发送 Cookie
- `max_age`: preflight 缓存时间（秒），默认 0

### 文件

| 文件 | 操作 |
|------|------|
| `internal/middleware/cors/cors.go` | 新建：CORS 中间件实现 |
| `internal/middleware/cors/cors_test.go` | 新建：单元测试 |
| `internal/config/security_config.go` | 修改：添加 `CORSConfig` 结构体 |
| `internal/config/defaults.go` | 修改：CORS 默认值
| `internal/config/validate.go` | 修改：CORS 验证（origins+credentials 互斥检查）
| `internal/server/middleware_builder.go` | 修改：注册 CORS 中间件（步骤 7.5，SecurityHeaders 之后、ErrorIntercept 之前）

### 行为

1. 非 CORS 请求（无 `Origin` 头）：直接 pass-through
2. Preflight (OPTIONS)：返回 204 + CORS 头，不进入后续 handler
3. 实际请求：调用 `next(ctx)` 后添加 CORS 响应头
4. Origin 不匹配：不加 CORS 头，浏览器阻止跨域

---

## 2. Request-ID 传播

### 配置

无需额外配置，默认启用。

### 文件

| 文件 | 操作 |
|------|------|
| `internal/middleware/requestid/requestid.go` | 新建：Request-ID 中间件 |
| `internal/middleware/requestid/requestid_test.go` | 新建：单元测试 |
| `internal/proxy/headers.go` | 修改：`SetForwardedHeaders` 添加 X-Request-ID 传播 |
| `internal/server/middleware_builder.go` | 修改：注册为第一个中间件（AccessLog 之前）

### 行为

1. 检查入站 `X-Request-ID` 请求头
2. 有值 → 复用（信任下游），存入 `ctx.SetUserValue("request_id", id)`
3. 无值 → 生成 UUID v4，存入 `ctx.SetUserValue`
4. 始终在响应中设置 `X-Request-ID` 头
5. 代理转发时，`SetForwardedHeaders` 自动传播 `X-Request-ID`（从 `ctx.UserValue("request_id")` 读取）

### 与现有代码的关系

`internal/variable/builtin.go` 已有 `$request_id` 变量，会从 `ctx.UserValue("request_id")` 读取。中间件在请求早期设置此值后，变量系统、access log、proxy header forwarding 都能正确使用。

---

## 3. /healthz + /readyz 端点

### 配置

在 `monitoring` 下新增：

```yaml
monitoring:
  healthz:
    enabled: true
    path: "/healthz"
  readyz:
    enabled: true
    path: "/readyz"
```

字段说明：
- `enabled`: 默认 true（开箱即用）
- `path`: 可自定义路径

### 文件

| 文件 | 操作 |
|------|------|
| `internal/server/healthz.go` | 新建：healthz/readyz handler |
| `internal/server/healthz_test.go` | 新建：单元测试 |
| `internal/config/monitoring_config.go` | 修改：添加 `HealthzConfig`、`ReadyzConfig` |
| `internal/config/defaults.go` | 修改：默认值
| `internal/server/server.go` | 修改：注册端点（三种模式都需要） |

### 行为

**healthz（存活探针）**：
- GET → 200 `{"status":"ok"}`
- 无任何依赖检查，只要进程活着就返回 200

**readyz（就绪探针）**：
- GET → 200 `{"status":"ready"}` 或 503 `{"status":"not ready","reasons":["no healthy upstreams"]}`
- 检查条件：至少有一个 server 已启动 + 至少有一个 upstream 目标可用（如果有配置 proxy 的话）
- 无 proxy 配置的纯静态文件服务器永远返回 200

### 注册位置

与 status/pprof 同级：
- Single 模式：`locationEngine.AddExact`
- VHost/Multi 模式：`router.GET`

默认启用，不需要 IP 白名单（K8s 探针来自 kubelet）。

---

## 4. 环境变量插值

### 语法

仅 `${ENV_VAR}` 花括号语法，与 lolly 自身 `$variable` 系统无歧义。

缺失环境变量时保留原样（`${MISSING_VAR}` 不展开）。

### 文件

| 文件 | 操作 |
|------|------|
| `internal/config/env.go` | 新建：`ExpandEnv(data []byte) []byte` 函数 |
| `internal/config/env_test.go` | 新建：单元测试 |
| `internal/config/config.go` | 修改：`Load()` 和 `processIncludes()` 中调用 `ExpandEnv` |

### 实现

正则 `\$\{([^}]+)\}` 匹配：
- 匹配到 → `os.Getenv(key)`
- 环境变量存在 → 替换为值
- 环境变量不存在 → 保留 `${key}` 原样

调用位置：
1. `Load()`: `os.ReadFile` 之后、`yaml.Unmarshal` 之前
2. `processIncludes()`: 每个被 include 文件 `os.ReadFile` 之后、`yaml.Unmarshal` 之前

### 示例

```yaml
servers:
  - listen: "${LISTEN_ADDR}:8080"
    ssl:
      cert: "${SSL_CERT_PATH}"
      key: "${SSL_KEY_PATH}"
    security:
      auth:
        users:
          - name: admin
            password: "${ADMIN_PASSWORD_HASH}"
```

---

## 5. CI/CD 流水线

### 配置

`.github/workflows/ci.yml` 单文件，push to master + PR 触发。

### Jobs

| Job | 依赖 | 步骤 |
|-----|------|------|
| `lint` | 无 | gofumpt 检查 → golangci-lint |
| `test` | 无 | `go test -race ./internal/...` |
| `build` | lint + test | 多平台静态构建（linux/amd64, linux/arm64, darwin/amd64, darwin/arm64） |
| `docker` | build（仅 push/tag） | docker build + push |

### 文件

| 文件 | 操作 |
|------|------|
| `.github/workflows/ci.yml` | 新建：GitHub Actions CI 流水线 |

### 约束

- Go 1.26
- `CGO_ENABLED=0`
- E2E 测试需要 Docker service（testcontainers）
- 使用 `make fmt`、`make lint`、`make test` 命令

---

## 依赖关系

```
CORS ──────┐
Request-ID ┤── 全部独立，可并行
healthz ───┤
env interp ┤
CI/CD ─────┘
```

## 提交策略

每个特性一个独立 commit：
1. `feat(middleware/cors): add CORS middleware with server-level config`
2. `feat(middleware/requestid): add request ID generation and propagation`
3. `feat(server): add /healthz and /readyz endpoints for k8s probes`
4. `feat(config): add ${ENV_VAR} interpolation in YAML config`
5. `ci: add GitHub Actions CI pipeline`
