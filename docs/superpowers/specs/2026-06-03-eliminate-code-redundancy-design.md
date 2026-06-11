# 消除代码冗余设计文档

> **日期：** 2026-06-03  
> **目标：** 消除 lolly 项目中的代码冗余，提升可维护性和代码质量  
> **范围：** 死代码删除、重复模式重构、测试辅助函数提取

---

## 1. 问题分析

通过对代码库的静态分析（`golangci-lint` + `dupl` + `unused`），发现以下冗余代码：

### 1.1 死代码（Dead Code）

| 文件 | 函数/方法 | 行号 | 说明 |
|------|----------|------|------|
| `internal/config/validate.go` | `validateStatic()` | 475 | `validateStatics()` 已内联相同逻辑，仅被测试调用 |
| `internal/http2/server.go` | `connectionPool.get()` | 576 | 无任何引用 |
| `internal/http2/server.go` | `connectionPool.count()` | 583 | 无任何引用 |
| `internal/middleware/bodylimit/bodylimit.go` | `formatSize()` | 288 | 业务代码未使用，仅被测试调用；`autoindex.go` 有同名函数 |
| `internal/middleware/security/headers.go` | `defaultSecurityHeaders()` | 295 | 仅被测试调用，业务代码未使用 |
| `internal/middleware/security/headers.go` | `strictSecurityHeaders()` | 309 | 仅被测试调用，业务代码未使用 |
| `internal/middleware/security/headers.go` | `developmentSecurityHeaders()` | 325 | 仅被测试调用，业务代码未使用 |
| `internal/ssl/ocsp.go` | `extractCertificates()` | 490 | 仅被测试调用，业务代码未使用 |

**排除项**（经确认实际被使用）：
- `setupTestLogger()` - 在 `app_test.go` 中被调用 47 次
- `canonicalHeaderKey()` - 在 `server_test.go` 中被调用

### 1.2 源文件重复模式

**路由注册错误处理（`internal/server/router.go`）**

19 次重复模式（proxy、static、lua 三种 handler）：
```go
if err := s.locationEngine.AddXXX(path, handler, internal); err != nil {
    if err := s.handleRegistrationError("type", path, err); err != nil {
        return err
    }
}
```

**DEBUG 日志条件检查（`internal/proxy/proxy.go`）**

5 次重复模式：
```go
if logging.Debug().Enabled() {
    logging.Debug().Str("key", value).Msg("[PROXY] message")
}
```

### 1.3 测试文件重复代码

| 模式 | 出现次数 | 位置 |
|------|---------|------|
| `config.ProxyConfig{...}` | 184 | 各测试文件 |
| `config.ProxyTimeout{Connect: 5 * time.Second}` | 85 | 各测试文件 |
| `targets := []*loadbalance.Target{{URL: "http://..."}}` | 123 | 各测试文件 |
| `targets[0].Healthy.Store(true)` | 41 | 各测试文件 |

---

## 2. 设计方案

### 2.1 阶段 1：死代码删除

**策略**：直接删除未使用的函数，同时清理仅被测试调用的函数的测试代码。

**处理清单**：
1. `validateStatic()` - 删除函数，将测试迁移到测试 `validateStatics()`
2. `connectionPool.get()` / `connectionPool.count()` - 直接删除
3. `formatSize()` (bodylimit) - 删除函数，删除测试；`autoindex.go` 的同名函数保留
4. `defaultSecurityHeaders()` / `strictSecurityHeaders()` / `developmentSecurityHeaders()` - 删除函数，删除测试
5. `extractCertificates()` - 删除函数，删除测试

### 2.2 阶段 2：重复模式重构

**2.2.1 路由注册辅助函数**

在 `internal/server/router.go` 中提取辅助函数：

```go
// registerRoute 注册路由并处理错误
func (s *Server) registerRoute(
    locType string,
    path string,
    handler fasthttp.RequestHandler,
    internal bool,
    source string,
) error {
    var err error
    switch locType {
    case matcher.LocationTypeExact:
        err = s.locationEngine.AddExact(path, handler, internal)
    case matcher.LocationTypePrefixPriority:
        err = s.locationEngine.AddPrefixPriority(path, handler, internal)
    case matcher.LocationTypeRegex:
        err = s.locationEngine.AddRegex(path, handler, false, internal)
    case matcher.LocationTypeRegexCaseless:
        err = s.locationEngine.AddRegex(path, handler, true, internal)
    case matcher.LocationTypeNamed:
        err = s.locationEngine.AddNamed(path, handler)
    default:
        err = s.locationEngine.AddPrefix(path, handler, internal)
    }
    if err != nil {
        return s.handleRegistrationError(source, path, err)
    }
    return nil
}
```

**2.2.2 DEBUG 日志辅助函数**

在 `internal/proxy/proxy.go` 中提取辅助函数：

```go
// proxyDebugLog 在 DEBUG 级别记录代理日志
func proxyDebugLog(msg string, kv ...interface{}) {
    if !logging.Debug().Enabled() {
        return
    }
    event := logging.Debug()
    for i := 0; i < len(kv)-1; i += 2 {
        key, ok := kv[i].(string)
        if !ok {
            continue
        }
        switch v := kv[i+1].(type) {
        case string:
            event = event.Str(key, v)
        case int:
            event = event.Int(key, v)
        case bool:
            event = event.Bool(key, v)
        }
    }
    event.Msg(msg)
}
```

### 2.3 阶段 3：测试辅助函数

在 `internal/testutil/` 包中创建辅助函数：

```go
package testutil

import (
    "rua.plus/lolly/internal/config"
    "rua.plus/lolly/internal/loadbalance"
)

// NewTestProxyConfig 创建测试用的代理配置
func NewTestProxyConfig(path string, targets []string) *config.ProxyConfig {
    cfg := &config.ProxyConfig{
        Path:        path,
        LoadBalance: "round_robin",
        Timeout: config.ProxyTimeout{
            Connect: 5 * time.Second,
            Read:    30 * time.Second,
            Write:   30 * time.Second,
        },
    }
    // ...
    return cfg
}

// NewTestTarget 创建测试用的代理目标
func NewTestTarget(url string) *loadbalance.Target {
    return &loadbalance.Target{URL: url}
}

// NewTestHealthyTarget 创建已标记为健康的测试目标
func NewTestHealthyTarget(url string) *loadbalance.Target {
    t := NewTestTarget(url)
    t.Healthy.Store(true)
    return t
}
```

**迁移策略**：
1. 先创建辅助函数
2. 逐步替换测试文件中的重复代码
3. 每次替换后运行测试确保通过

---

## 3. 风险评估

| 风险 | 可能性 | 影响 | 缓解措施 |
|------|--------|------|---------|
| 删除的函数实际上被间接使用 | 低 | 高 | 通过 `grep` 确认无引用后再删除 |
| 重构引入新 bug | 中 | 中 | 每次变更后运行完整测试套件 |
| 测试辅助函数改变测试语义 | 低 | 中 | 保持默认配置与原始代码一致 |

---

## 4. 验收标准

- [ ] `golangci-lint run --enable=unused ./...` 无 unused 错误
- [ ] `golangci-lint run --enable=dupl ./...` 源文件无 dupl 错误
- [ ] `go test ./...` 全部通过
- [ ] 代码总行数减少 >200 行
- [ ] 测试文件中的 `ProxyConfig{` 字面量减少 >50%

---

## 5. 实施顺序

1. **阶段 1（死代码）** - 低风险，快速见效
2. **阶段 2（源文件重构）** - 中等风险，改善可维护性
3. **阶段 3（测试辅助函数）** - 低风险，最大减负
