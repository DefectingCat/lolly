# Lolly 代码冗余优化实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 系统性消除 Lolly 代码库中的冗余代码，包括死代码、重复实现、过度工程化和测试重复，提升可维护性和代码质量。

**Architecture:** 采用分阶段、增量式重构策略。每阶段独立可交付，确保随时可回滚。优先处理死代码（零风险、高回报），然后处理重复实现（低风险、中回报），最后处理架构级重复（中风险、长期收益）。

**Tech Stack:** Go 1.24, fasthttp, staticcheck, go vet

---

## 文件结构映射

### 删除/清理的文件
- `internal/middleware/limitrate/limitrate.go` — 死代码包主文件
- `internal/middleware/limitrate/writer.go` — 死代码包辅助文件  
- `internal/middleware/limitrate/limitrate_test.go` — 死代码包测试文件
- `internal/stream/ssl.go` — 死代码（所有字段未使用）
- `internal/stream/ssl_test.go` — 死代码测试文件
- `internal/variable/pool.go` — 死代码（所有字段未使用）
- `internal/proxy/proxy_coverage_extra_test.go` 中的 `TestExtractHostFromURL` — 被测函数即将删除

### 修改的文件（按模块分组）

**Phase 1 - 死代码清理：**
- `internal/mimeutil/detect.go:154` — 添加 defaultMIME 回退逻辑
- `internal/app/app_test.go:448` — 删除未使用的 `customSig`
- `internal/app/testutil.go:17` — 删除未使用的 `setupTestLogger`
- `internal/http3/server_test.go:138` — 删除未使用的 `generateTestCertificate`
- `internal/proxy/proxy_dns_test.go:91` — 删除未使用的方法
- `internal/server/testutil.go:15` — 删除未使用的常量
- `internal/server/upgrade_test.go:291` — 删除未使用的 `containsString`
- `internal/server/pool_bench_test.go:305` — 删除未使用的 `id` 字段
- `internal/stream/stream_test.go:24` — 删除未使用的 `generateTestCertificate`

**Phase 2 - 重复实现消除：**
- `internal/proxy/proxy.go:362,1003-1018` — 删除 `extractHostFromURL`，改用 `netutil.ParseTargetURL`
- `internal/proxy/header_modifier.go:33` — 改用 `netutil.ParseTargetURL`
- `internal/handler/static.go:628,832-836` — 删除 `generateETag` 包装，直接调用 `utils.GenerateETag`
- `internal/cache/file_cache.go:47,181` — 删除 `generateETag` 包装，直接调用 `utils.GenerateETag`
- `internal/utils/httperror.go:67-86` — 简化 `CheckIPAccess`，复用 `IPInAllowList`

**Phase 3 - 路由和服务器逻辑简化：**
- `internal/server/router.go:118-145,217-234,402-423` — 消除冗余 switch 块
- `internal/server/server.go:454-868` — 提取三种启动模式的公共函数

**Phase 4 - 负载均衡统一（可选）：**
- `internal/stream/stream.go:61-285` — 复用 `internal/loadbalance` 的算法实现

---

## 任务分解

### Phase 1: 死代码清理（P0）

---

#### Task 1.1: 删除 limitrate 死代码包

**Files:**
- Delete: `internal/middleware/limitrate/limitrate.go`
- Delete: `internal/middleware/limitrate/writer.go`
- Delete: `internal/middleware/limitrate/limitrate_test.go`

- [ ] **Step 1: 确认包未被引用**

```bash
grep -r "limitrate" --include="*.go" /home/xfy/Developer/lolly/internal/
```

Expected: 仅返回 `internal/middleware/limitrate/` 目录内的匹配，无外部引用。

- [ ] **Step 2: 删除整个目录**

```bash
rm -rf /home/xfy/Developer/lolly/internal/middleware/limitrate/
```

- [ ] **Step 3: 验证编译通过**

```bash
cd /home/xfy/Developer/lolly && go build ./...
```

Expected: 无错误，编译成功。

- [ ] **Step 4: 运行受影响包的测试**

```bash
cd /home/xfy/Developer/lolly && go test ./internal/middleware/...
```

Expected: 全部通过。

- [ ] **Step 5: Commit**

```bash
cd /home/xfy/Developer/lolly && git add -A && git commit -m "refactor: remove dead code package internal/middleware/limitrate"
```

---

#### Task 1.2: 删除 stream/ssl.go 死代码

**Files:**
- Delete: `internal/stream/ssl.go`
- Delete: `internal/stream/ssl_test.go`

- [ ] **Step 1: 确认 ssl.go 字段未被使用**

```bash
grep -r "SSLManager\|ProxySSLManager" --include="*.go" /home/xfy/Developer/lolly/internal/
```

Expected: 仅 `internal/stream/ssl.go` 自身有定义，无其他引用。

- [ ] **Step 2: 删除文件**

```bash
rm /home/xfy/Developer/lolly/internal/stream/ssl.go
rm /home/xfy/Developer/lolly/internal/stream/ssl_test.go
```

- [ ] **Step 3: 验证编译和测试**

```bash
cd /home/xfy/Developer/lolly && go build ./internal/stream/... && go test ./internal/stream/...
```

Expected: 编译和测试全部通过。

- [ ] **Step 4: Commit**

```bash
cd /home/xfy/Developer/lolly && git add -A && git commit -m "refactor: remove unused stream SSL dead code"
```

---

#### Task 1.3: 删除 variable/pool.go 死代码

**Files:**
- Delete: `internal/variable/pool.go`

- [ ] **Step 1: 确认 pool.go 变量未被使用**

```bash
grep -r "PoolStats\|gets\.\|puts\.\|newCount\.\|active\." --include="*.go" /home/xfy/Developer/lolly/internal/
```

Expected: 无引用（除 `pool.go` 自身定义外）。

- [ ] **Step 2: 删除文件**

```bash
rm /home/xfy/Developer/lolly/internal/variable/pool.go
```

- [ ] **Step 3: 验证编译和测试**

```bash
cd /home/xfy/Developer/lolly && go build ./internal/variable/... && go test ./internal/variable/...
```

Expected: 编译和测试全部通过。

- [ ] **Step 4: Commit**

```bash
cd /home/xfy/Developer/lolly && git add -A && git commit -m "refactor: remove unused variable pool statistics dead code"
```

---

#### Task 1.4: 修复 mimeutil defaultMIME 未使用问题

**Files:**
- Modify: `internal/mimeutil/detect.go:154`

- [ ] **Step 1: 阅读当前 DetectContentType 实现**

Read: `internal/mimeutil/detect.go:95-155`

当前实现：当 `mime.TypeByExtension` 返回空字符串时，直接缓存并返回空字符串，从未使用 `defaultMIME`。

- [ ] **Step 2: 在 DetectContentType 末尾添加 defaultMIME 回退**

```go
// 在 internal/mimeutil/detect.go 第 154 行（return mimeType 之前）添加：

	if mimeType == "" {
		defaultMutex.RLock()
		mimeType = defaultMIME
		defaultMutex.RUnlock()
	}

	return mimeType
```

完整修改后的第 149-158 行应为：

```go
	// 插入新条目
	entry := &mimeCacheEntry{ext: ext, mimeType: mimeType}
	entry.element = mimeLRU.PushFront(entry)
	mimeCache[ext] = entry

	if mimeType == "" {
		defaultMutex.RLock()
		mimeType = defaultMIME
		defaultMutex.RUnlock()
	}

	return mimeType
```

- [ ] **Step 3: 验证编译和测试**

```bash
cd /home/xfy/Developer/lolly && go build ./internal/mimeutil/... && go test ./internal/mimeutil/...
```

Expected: 编译和测试全部通过。

- [ ] **Step 4: Commit**

```bash
cd /home/xfy/Developer/lolly && git add -A && git commit -m "fix: use defaultMIME fallback in DetectContentType"
```

---

#### Task 1.5: 清理其他静态检查发现的死代码

**Files:**
- Modify: `internal/app/app_test.go` — 删除未使用的 `customSig`
- Modify: `internal/app/testutil.go` — 删除未使用的 `setupTestLogger`
- Modify: `internal/http3/server_test.go` — 删除未使用的 `generateTestCertificate`
- Modify: `internal/proxy/proxy_dns_test.go` — 删除未使用的方法
- Modify: `internal/server/testutil.go` — 删除未使用的 `testListenAddr`
- Modify: `internal/server/upgrade_test.go` — 删除未使用的 `containsString`
- Modify: `internal/server/pool_bench_test.go` — 删除未使用的 `id` 字段
- Modify: `internal/stream/stream_test.go` — 删除未使用的 `generateTestCertificate`

- [ ] **Step 1: 运行 staticcheck 获取精确行号**

```bash
cd /home/xfy/Developer/lolly && staticcheck ./... 2>&1 | grep "U1000"
```

Expected: 输出每个死代码的精确文件路径和行号。

- [ ] **Step 2: 逐个删除死代码**

对每个 staticcheck 报告的死代码：
1. 打开文件
2. 定位到报告的函数/变量/字段
3. 删除整个未使用的声明
4. 保存文件

示例（以 `internal/server/testutil.go` 为例）：

```go
// 删除前：
const testListenAddr = "127.0.0.1:0"

// 删除后：
// （整行删除）
```

- [ ] **Step 3: 验证编译和测试**

```bash
cd /home/xfy/Developer/lolly && go build ./... && go test ./internal/app/... ./internal/http3/... ./internal/proxy/... ./internal/server/... ./internal/stream/...
```

Expected: 全部通过。

- [ ] **Step 4: Commit**

```bash
cd /home/xfy/Developer/lolly && git add -A && git commit -m "refactor: remove unused code identified by staticcheck"
```

---

### Phase 2: 重复实现消除（P1）

---

#### Task 2.1: 删除 proxy.go 中的 extractHostFromURL，统一使用 netutil

**Files:**
- Modify: `internal/proxy/proxy.go:362` — 替换调用
- Modify: `internal/proxy/proxy.go:993-1018` — 删除函数
- Modify: `internal/proxy/header_modifier.go:33` — 替换调用
- Modify: `internal/proxy/proxy_coverage_extra_test.go` — 删除测试

- [ ] **Step 1: 修改 proxy.go:362 的调用**

Read: `internal/proxy/proxy.go:360-365`

将：
```go
	tlsCfg, err := CreateTLSConfig(sslCfg, extractHostFromURL(targetURL))
```
改为：
```go
	host, _, _, err := netutil.ParseTargetURL(targetURL, false)
	if err != nil {
		return nil, fmt.Errorf("parse target URL %q: %w", targetURL, err)
	}
	tlsCfg, err := CreateTLSConfig(sslCfg, host)
```

并确保文件已导入 `rua.plus/lolly/internal/netutil`。

- [ ] **Step 2: 修改 header_modifier.go:33 的调用**

Read: `internal/proxy/header_modifier.go:30-36`

将：
```go
	targetHost := extractHostFromURL(target.URL)
```
改为：
```go
	targetHost, _, _, err := netutil.ParseTargetURL(target.URL, false)
	if err != nil {
		targetHost = target.URL
	}
```

并确保文件已导入 `rua.plus/lolly/internal/netutil`。

- [ ] **Step 3: 删除 proxy.go 中的 extractHostFromURL 函数**

删除 `internal/proxy/proxy.go` 第 993-1018 行的整个函数：

```go
// extractHostFromURL 从 URL 字符串中提取 host:port 部分。
// ...
func extractHostFromURL(urlStr string) string {
	// ...
}
```

- [ ] **Step 4: 删除 proxy_coverage_extra_test.go 中的 TestExtractHostFromURL**

Read: `internal/proxy/proxy_coverage_extra_test.go:1426-1480`

删除整个 `TestExtractHostFromURL` 函数及其相关测试用例。

- [ ] **Step 5: 验证编译和测试**

```bash
cd /home/xfy/Developer/lolly && go build ./internal/proxy/... && go test ./internal/proxy/...
```

Expected: 编译和测试全部通过。

- [ ] **Step 6: Commit**

```bash
cd /home/xfy/Developer/lolly && git add -A && git commit -m "refactor: remove extractHostFromURL, use netutil.ParseTargetURL"
```

---

#### Task 2.2: 删除 generateETag 包装函数

**Files:**
- Modify: `internal/handler/static.go:628,832-836`
- Modify: `internal/cache/file_cache.go:45-49,181`

- [ ] **Step 1: 修改 handler/static.go**

Read: `internal/handler/static.go:626-630`

将：
```go
	etag := generateETag(info.ModTime(), info.Size())
```
改为：
```go
	etag := utils.GenerateETag(info.ModTime(), info.Size())
```

删除 `internal/handler/static.go` 第 832-836 行的 `generateETag` 函数。

- [ ] **Step 2: 修改 cache/file_cache.go**

Read: `internal/cache/file_cache.go:179-183`

将：
```go
	etag := generateETag(modTime, size)
```
改为：
```go
	etag := utils.GenerateETag(modTime, size)
```

删除 `internal/cache/file_cache.go` 第 45-49 行的 `generateETag` 函数。

- [ ] **Step 3: 验证编译和测试**

```bash
cd /home/xfy/Developer/lolly && go build ./internal/handler/... ./internal/cache/... && go test ./internal/handler/... ./internal/cache/...
```

Expected: 编译和测试全部通过。

- [ ] **Step 4: Commit**

```bash
cd /home/xfy/Developer/lolly && git add -A && git commit -m "refactor: remove redundant generateETag wrappers, use utils.GenerateETag directly"
```

---

#### Task 2.3: 简化 CheckIPAccess 复用 IPInAllowList

**Files:**
- Modify: `internal/utils/httperror.go:67-86`

- [ ] **Step 1: 重构 CheckIPAccess**

Read: `internal/utils/httperror.go:67-86`

将：
```go
func CheckIPAccess(ctx *fasthttp.RequestCtx, allowed []net.IPNet) bool {
	if len(allowed) == 0 {
		return true
	}

	clientIP := netutil.ExtractClientIPNet(ctx)
	if clientIP == nil {
		return false
	}

	for _, network := range allowed {
		if network.Contains(clientIP) {
			return true
		}
	}

	return false
}
```
改为：
```go
func CheckIPAccess(ctx *fasthttp.RequestCtx, allowed []net.IPNet) bool {
	if len(allowed) == 0 {
		return true
	}

	clientIP := netutil.ExtractClientIPNet(ctx)
	if clientIP == nil {
		return false
	}

	return IPInAllowList(clientIP, allowed)
}
```

- [ ] **Step 2: 验证编译和测试**

```bash
cd /home/xfy/Developer/lolly && go build ./internal/utils/... && go test ./internal/utils/...
```

Expected: 编译和测试全部通过。

- [ ] **Step 3: Commit**

```bash
cd /home/xfy/Developer/lolly && git add -A && git commit -m "refactor: simplify CheckIPAccess by reusing IPInAllowList"
```

---

### Phase 3: 路由和服务器逻辑简化（P1-P2）

---

#### Task 3.1: 简化 router.go 中的冗余 switch 块

**Files:**
- Modify: `internal/server/router.go:118-145` (`registerProxyRoutesWithLocationEngine`)
- Modify: `internal/server/router.go:217-234` (`registerStaticHandlersWithLocationEngine`)
- Modify: `internal/server/router.go:402-423` (`registerLuaRoutesWithLocationEngine`)

- [ ] **Step 1: 简化 registerProxyRoutesWithLocationEngine**

Read: `internal/server/router.go:108-148`

将第 118-145 行的 switch 块替换为：

```go
	for i := range serverCfg.Proxy {
		proxyCfg := &serverCfg.Proxy[i]
		p := s.createProxyForConfig(proxyCfg)
		if p == nil {
			continue
		}

		locType := proxyCfg.LocationType
		if locType == "" {
			locType = matcher.LocationTypePrefix
		}

		path := proxyCfg.Path
		if locType == matcher.LocationTypeNamed && proxyCfg.LocationName != "" {
			path = "@" + proxyCfg.LocationName
		}

		if err := s.registerRoute(locType, path, p.ServeHTTP, proxyCfg.Internal, "proxy"); err != nil {
			return err
		}
	}
	return nil
```

- [ ] **Step 2: 简化 registerStaticHandlersWithLocationEngine**

Read: `internal/server/router.go:208-236`

将第 217-234 行的 switch 块替换为类似逻辑（直接调用 `s.registerRoute`）。

- [ ] **Step 3: 简化 registerLuaRoutesWithLocationEngine**

Read: `internal/server/router.go:393-425`

将第 402-423 行的 switch 块替换为类似逻辑（直接调用 `s.registerRoute`）。

- [ ] **Step 4: 验证编译和测试**

```bash
cd /home/xfy/Developer/lolly && go build ./internal/server/... && go test ./internal/server/...
```

Expected: 编译和测试全部通过。

- [ ] **Step 5: Commit**

```bash
cd /home/xfy/Developer/lolly && git add -A && git commit -m "refactor: eliminate redundant switch blocks in router.go LocationEngine functions"
```

---

#### Task 3.2: 提取 server.go 三种启动模式的公共函数

**Files:**
- Modify: `internal/server/server.go:454-868`

**新增辅助函数（添加到 server.go 末尾，在 SetResolver 之前）：**

- [ ] **Step 1: 提取 `registerMonitoringEndpoints` 函数**

在 `internal/server/server.go` 中新增：

```go
// registerMonitoringEndpoints 注册状态监控、性能分析和缓存清理端点。
// isDefault 为 true 时注册所有端点，否则跳过（用于多服务器模式）。
func (s *Server) registerMonitoringEndpoints(router *handler.Router, serverCfg *config.ServerConfig, isDefault bool) {
	// 状态监控端点
	if isDefault && s.config.Monitoring.Status.Enabled {
		statusHandler, err := NewStatusHandler(s, &s.config.Monitoring.Status)
		if err != nil {
			logging.Error().Msg("Failed to create status handler: " + err.Error())
		} else {
			router.GET(statusHandler.Path(), statusHandler.ServeHTTP)
		}
	}

	// pprof 性能分析端点
	if isDefault && s.config.Monitoring.Pprof.Enabled {
		pprofHandler, err := NewPprofHandler(&s.config.Monitoring.Pprof)
		if err != nil {
			logging.Error().Msg("Failed to create pprof handler: " + err.Error())
		} else {
			router.GET(pprofHandler.Path(), pprofHandler.ServeHTTP)
			router.GET(pprofHandler.Path()+"/{profile:*}", pprofHandler.ServeHTTP)
		}
	}

	// 缓存清理 API
	if isDefault && serverCfg.CacheAPI != nil && serverCfg.CacheAPI.Enabled {
		purgeHandler, err := NewPurgeHandler(s, serverCfg.CacheAPI)
		if err != nil {
			logging.Error().Msg("Failed to create cache purge handler: " + err.Error())
		} else {
			router.POST(purgeHandler.Path(), purgeHandler.ServeHTTP)
		}
	}
}
```

- [ ] **Step 2: 提取 `wrapHandler` 函数**

```go
// wrapHandler 应用中间件链、连接池包装和统计追踪。
func (s *Server) wrapHandler(base fasthttp.RequestHandler, serverCfg *config.ServerConfig) (fasthttp.RequestHandler, error) {
	chain, err := s.buildMiddlewareChain(serverCfg)
	if err != nil {
		return nil, err
	}

	handler := chain.Apply(base)
	if s.pool != nil {
		handler = s.pool.WrapHandler(handler)
	}
	handler = s.trackStats(handler)
	return handler, nil
}
```

- [ ] **Step 3: 提取 `startServer` 函数**

```go
// startServer 创建监听器并启动 fasthttp.Server，支持可选 TLS。
func (s *Server) startServer(serverCfg *config.ServerConfig, fastSrv *fasthttp.Server) error {
	ln, err := s.createListener(serverCfg)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}
	s.listeners = append(s.listeners, ln)

	// 检查 SSL/TLS
	if serverCfg.SSL.Cert != "" && serverCfg.SSL.Key != "" {
		tlsManager, err := ssl.NewTLSManager(&serverCfg.SSL)
		if err != nil {
			return fmt.Errorf("failed to create TLS manager: %w", err)
		}
		fastSrv.TLSConfig = tlsManager.GetTLSConfig()
		return fastSrv.ServeTLS(ln, "", "")
	}

	return fastSrv.Serve(ln)
}
```

- [ ] **Step 4: 重构 startSingleMode 使用新函数**

将 `startSingleMode` 中的监控注册、中间件链构建、fasthttp.Server 创建和启动逻辑替换为对新辅助函数的调用。

重构后的 `startSingleMode` 核心逻辑：

```go
func (s *Server) startSingleMode() error {
	serverCfg := &s.config.Servers[0]
	s.applyTypesConfig(serverCfg)

	s.locationEngine = matcher.NewLocationEngine()
	s.registerMonitoringEndpointsWithLocationEngine(serverCfg)

	if err := s.registerProxyRoutesWithLocationEngine(serverCfg); err != nil {
		return err
	}
	// ... Lua 和静态文件注册

	s.locationEngine.MarkInitialized()

	baseHandler := func(ctx *fasthttp.RequestCtx) {
		// LocationEngine 匹配逻辑
	}

	handler, err := s.wrapHandler(baseHandler, serverCfg)
	if err != nil {
		return err
	}
	s.handler = handler

	s.fastServer = s.createFastServer(serverCfg, s.handler)
	s.running.Store(true)

	return s.startServer(serverCfg, s.fastServer)
}
```

- [ ] **Step 5: 重构 startVHostMode 使用新函数**

类似地，将 `startVHostMode` 中的重复逻辑替换为对新辅助函数的调用。

- [ ] **Step 6: 重构 startMultiServerMode 使用新函数**

类似地，将 `startMultiServerMode` 中的重复逻辑替换为对新辅助函数的调用。

- [ ] **Step 7: 验证编译和测试**

```bash
cd /home/xfy/Developer/lolly && go build ./internal/server/... && go test ./internal/server/...
```

Expected: 编译和测试全部通过。

- [ ] **Step 8: Commit**

```bash
cd /home/xfy/Developer/lolly && git add -A && git commit -m "refactor: extract common functions from server startup modes"
```

---

### Phase 4: 负载均衡统一（P3 - 可选/长期）

---

#### Task 4.1: 分析 Stream 和 HTTP 负载均衡的差异

**Files:**
- Read: `internal/stream/stream.go:61-285`
- Read: `internal/loadbalance/balancer.go:101-273`

- [ ] **Step 1: 对比两种实现的差异**

重点关注：
- Stream 版本使用 `sync.Pool` 优化，HTTP 版本没有
- HTTP 版本有 `SelectExcluding` 方法，Stream 版本没有
- 两者 Target 类型不同（Stream 用 `string`，HTTP 用 `*Target`）

- [ ] **Step 2: 决策是否统一**

如果差异较小，建议：
1. 在 `internal/loadbalance` 中定义接口
2. Stream 复用 HTTP 的实现，只保留 `sync.Pool` 优化作为可选项

如果差异较大，建议：
1. 保持现状
2. 在文档中注明重复，待架构演进时统一

---

## 验证清单

每阶段完成后运行：

```bash
# 1. 编译检查
cd /home/xfy/Developer/lolly && go build ./...

# 2. 静态分析
cd /home/xfy/Developer/lolly && staticcheck ./...

# 3. 单元测试
cd /home/xfy/Developer/lolly && go test ./internal/...

# 4. 完整测试套件
cd /home/xfy/Developer/lolly && make test
```

Expected: 
- `go build ./...` — 无错误
- `staticcheck ./...` — 无新的警告
- `go test ./internal/...` — 全部通过
- `make test` — 全部通过

---

## 回滚策略

每个 Task 完成后立即 commit。如需回滚：

```bash
# 回滚单个 Task
git revert <commit-hash>

# 回滚整个 Phase
git revert <phase-first-commit>..<phase-last-commit>
```

---

## 风险评估

| 任务 | 风险等级 | 影响范围 | 缓解措施 |
|------|----------|----------|----------|
| Task 1.1-1.5 | 极低 | 仅删除死代码 | 编译和测试验证 |
| Task 2.1-2.3 | 低 | 替换函数调用 | 全量测试 |
| Task 3.1 | 低 | router.go 内部重构 | server 包测试 |
| Task 3.2 | 中 | server.go 核心逻辑 | 完整回归测试 |
| Task 4.1 | 中 | 架构变更 | 延后到单独迭代 |

---

*Plan generated: 2026-06-03*
*Estimated effort: 4-6 hours for Phases 1-3, 2-4 hours for Phase 4*
