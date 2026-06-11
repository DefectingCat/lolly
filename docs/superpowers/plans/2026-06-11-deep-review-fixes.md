# 深度审查修复计划

## 目标
修复 6 个 review subagent 发现的 Critical 和 High 级别问题。

## 策略
按影响范围和修复复杂度分 4 个批次执行，每批独立提交。

## 批次 1：明确的崩溃/数据损坏（Critical，最容易验证）

### 1.1 logging.go: append 污染 fasthttp buffer
- **文件**: `internal/logging/logging.go:147`
- **问题**: `append(append(ctx.Method(), ' '), ctx.Path()...)` 可能 mutate fasthttp 内部 buffer
- **修复**: 预先分配新的 slice：`req := make([]byte, 0, len(method)+1+len(path))`
- **测试**: 验证 request 字段不修改原始 buffer

### 1.2 mimeutil/detect.go: 缓存空字符串 bug
- **文件**: `internal/mimeutil/detect.go:150`
- **问题**: 未知扩展名时先插入空字符串再替换成 defaultMIME
- **修复**: 在缓存插入前完成 defaultMIME 回退，或用 RLock 读 + Lock 写
- **测试**: 多次查找未知扩展名都应返回 `application/octet-stream`

### 1.3 server.go nil/empty config panic
- **文件**: `internal/server/server.go:270-295, 470`
- **问题**: `Start()` / `startSingleMode()` 访问 `s.config.*` 和 `Servers[0]` 无前置检查
- **修复**: `Start()` 开头加 nil check 和 `len(Servers) > 0` check；`startSingleMode` 同理
- **测试**: `New(nil).Start()` 应返回 error 而非 panic

### 1.4 app_common.go empty servers panic
- **文件**: `internal/app/app_common.go:176, 202`
- **问题**: `initHTTP3()`, `initHTTP2()` 访问 `a.cfg.Servers[0]` 未检查
- **修复**: 加 `len(a.cfg.Servers) > 0` guard
- **测试**: 空 servers 配置应跳过 HTTP2/3 初始化

### 1.5 proxy/health.go nil cfg panic
- **文件**: `internal/proxy/health.go:80`
- **问题**: `NewHealthChecker` 直接读 `cfg.Interval`
- **修复**: `if cfg == nil { cfg = &config.HealthCheckConfig{} }`
- **测试**: `NewHealthChecker(targets, nil)` 不 panic

### 1.6 resolver.go nil cfg panic
- **文件**: `internal/resolver/resolver.go:134`
- **问题**: `New(cfg)` 读 `cfg.Enabled`
- **修复**: `if cfg == nil || !cfg.Enabled { return &noopResolver{} }`
- **测试**: `New(nil)` 不 panic

### 1.7 lua/api_log.go Fatal kills server
- **文件**: `internal/lua/api_log.go:250`
- **问题**: `EMERG/ALERT/CRIT` 调用 `logger.Fatal().Msg()` 会 `os.Exit(1)`
- **修复**: 全部映射到非致命的 `Error()`（Lua 日志级别不应 kill 进程）
- **测试**: 调用 `ngx.log(ngx.EMERG, ...)` 不退出进程

---

## 批次 2：并发 / Race

### 2.1 compression pool.New race
- **文件**: `internal/middleware/compression/compression.go:75-83`
- **问题**: `compressorPool.Get()` 懒初始化 `pool.New`
- **修复**: 在 `newGzipPool`/`newBrotliPool` 构造时就设置 `pool.New`
- **测试**: `go test -race` 多次 Get 不报错

### 2.2 ssl/ocsp data race
- **文件**: `internal/ssl/ocsp.go:397-426`
- **问题**: RUnlock 后读 `resp.status/nextUpdate`
- **修复**: 整个函数体在 RLock 保护下完成，或把可变字段改为 atomic
- **测试**: `go test -race ./internal/ssl/...`

### 2.3 server lifecycle/purge race on proxies slice
- **文件**: `internal/server/lifecycle.go:222-230`, `internal/server/purge.go:127,145`
- **问题**: 无锁读 `s.proxies`
- **修复**: 用 `s.proxiesMu.RLock()` 包裹读循环
- **测试**: 并行 stats/purge 和 create proxy 时不触发 race

### 2.4 lua/api_timer active race
- **文件**: `internal/lua/api_timer.go:228-265, 303-325`
- **问题**: `executeTimer` 和 `Cancel` 都递减 active；向 closed callbackQueue 发送会 panic
- **修复**: 只在 `executeTimer` defer 中递减 active；`Cancel` 仅关闭 cancel channel；callbackQueue 发送加锁保护
- **测试**: 并发 Cancel 和触发不 panic，`active` 不会为负

### 2.5 lua/api_socket_tcp ConnectAsync race
- **文件**: `internal/lua/api_socket_tcp.go:196-205, 640-642`
- **问题**: goroutine 设置 `s.currentOp = nil` 后，caller 访问 op.ID
- **修复**: `Connect` 直接返回 `*SocketOperation` 给 caller，不要通过字段传递
- **测试**: 快速完成连接的 localhost 不触发 nil panic

---

## 批次 3：资源泄漏 / 功能损坏

### 3.1 proxy connection count leak (2 places)
- **文件**: `internal/proxy/proxy.go:889-896, 898-919`
- **问题**: X-Accel-Redirect 和重试路径漏了 `DecrementConnections`
- **修复**: 两个 return/continue 前都调用 `loadbalance.DecrementConnections(target)`
- **测试**: 内部 redirect 和 retry 后连接计数归零

### 3.2 server/pool.go deadlock
- **文件**: `internal/server/pool.go:178-184`
- **问题**: queue full 时启动 worker 后立即 blocking send，可能死锁
- **修复**: 用非阻塞 send，或在启动 worker 时直接把 task 传给 worker
- **测试**: 持续 Submit 满载任务不阻塞

### 3.3 handler/sendfile_linux.go FD closed before use
- **文件**: `internal/handler/sendfile_linux.go:151-169`
- **问题**: `getSocketFd` 返回 FD 后 `defer file.Close()` 已执行
- **修复**: 移除 defer，由 `linuxSendfile` 负责关闭；或重构 syscall 实现
- **测试**: Linux sendfile 实际生效

### 3.4 proxy/websocket.go bufio data loss
- **文件**: `internal/proxy/websocket.go:335-349, 415-427`
- **问题**: `bufio.Reader` 缓冲的 frame 数据在返回后被丢弃
- **修复**: 桥接前 drain `resp.Body`，或复用同一个 `bufio.Reader`
- **测试**: WebSocket 连接不丢首帧

### 3.5 stream/stream.go UDP shutdown deadlock
- **文件**: `internal/stream/stream.go:966-1001`
- **问题**: UDP serve 在 `Stop()` 后不退出，`wg.Wait()` 死锁
- **修复**: 非 timeout 错误时检查 `stopCh`；`Stop()` 设置 `udpSrv.running.Store(false)`
- **测试**: UDP server Stop 能正常完成

### 3.6 stream/stream.go upstream name mismatch
- **文件**: `internal/stream/stream.go:596-608`
- **问题**: `AddUpstream` 用 name，`handleConnection` 用 listener addr 查表
- **修复**: 统一 key，或建立 listener→upstream 映射
- **测试**: TCP stream proxy 转发到后端

### 3.7 ratelimit token bucket cleanup leak
- **文件**: `internal/middleware/security/ratelimit.go:132-162, 476-512`
- **问题**: cleanup goroutine 不停止；`StopCleanup` double-close panic
- **修复**: `sync.Once` 保护 close；reload 时调用 StopCleanup
- **测试**: 多次创建/销毁 limiter 不泄漏 goroutine，不 panic

---

## 批次 4：High severity

### 4.1 middleware/security/headers.go nil cfg
- 在 `UpdateConfig` 或 `addHeaders` 中做 nil guard

### 4.2 middleware/security/sliding_window.go div by zero
- 构造函数中校验 `window > 0`

### 4.3 handler/static.go data race
- setter 和 Handle 并发访问字段；加 `sync.RWMutex`

### 4.4 proxy/proxy_dns.go HostClient.Addr race
- DNS 更新时不能并发修改 live client 的 Addr

### 4.5 loadbalance/slow_start.go Start/Stop bug
- `stopCh` 重建、`findTarget` 未初始化

### 4.6 resolver restart broken
- `stopCh` 关闭后不可重用；`Start()` 重新创建

### 4.7 variable/variable.go nil map panic
- fallback Context 中初始化所有 map

### 4.8 variable/builtin.go request_id/time_local
- request_id 只生成一次并存入 UserValue；time_local 用 `-0700`

### 4.9 logging.go file handle leak
- `New()` 中保存 `*os.File` 以便 `Close()` 关闭

---

## 验证
每个批次完成后：
1. `go test -race` 针对修改的包
2. `make lint`
3. `go test ./internal/...`
4. `make build`

全部完成后做一次全量验证。
