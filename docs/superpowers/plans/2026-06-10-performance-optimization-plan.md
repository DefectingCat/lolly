# 性能持续优化实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 建立完整的性能基准测试体系，收集 baseline 数据，识别 Top 10 瓶颈，实施可量化的性能优化

**Architecture:** 数据驱动优化流程：建立基准 → 采集数据 → 分析瓶颈 → 实施优化 → 回归检测。先补齐缺失的 benchmark，再跑全量基准生成 baseline，然后用 pprof 定位瓶颈，最后逐个优化验证

**Tech Stack:** Go 1.26+, testing/benchmark, pprof, benchstat, wrk/oha/h2load

---

## 文件结构映射

```
internal/benchmark/
├── micro/                    # 微基准测试
│   ├── resolver_bench_test.go    # DNS 解析器基准
│   ├── stream_bench_test.go      # Stream 代理基准
│   ├── cache_bench_test.go       # 缓存系统基准
│   ├── lua_bench_test.go         # Lua 引擎基准
│   └── variable_bench_test.go    # 变量系统基准
├── integration/              # 集成基准测试
│   ├── server_bench_test.go      # HTTP 服务器端到端
│   ├── proxy_bench_test.go       # 反向代理端到端
│   └── static_bench_test.go      # 静态文件端到端
└── system/                   # 系统压测脚本
    ├── bench.sh                  # 主压测脚本
    ├── static.lua                # wrk 静态文件压测脚本
    └── proxy.lua                 # wrk 代理压测脚本

scripts/
└── bench-suite.sh            # 一键运行全量基准

benchmarks/                   # 基准结果存储
└── v0.4.0/                   # 版本号目录
    ├── micro.txt
    ├── integration.txt
    ├── system.txt
    └── pprof/
        ├── cpu.prof
        ├── heap.prof
        ├── allocs.prof
        └── goroutine.prof
```

---

## Task 1: 建立 Benchmark 目录结构

**Files:**
- Create: `internal/benchmark/micro/`
- Create: `internal/benchmark/integration/`
- Create: `internal/benchmark/system/`
- Create: `benchmarks/`
- Modify: `.gitignore`（忽略 benchmarks/ 但保留目录）

- [ ] **Step 1: 创建目录结构**

```bash
mkdir -p internal/benchmark/micro
mkdir -p internal/benchmark/integration
mkdir -p internal/benchmark/system
mkdir -p benchmarks/v0.4.0/pprof
```

- [ ] **Step 2: 添加 .gitignore 规则**

在 `.gitignore` 末尾添加：

```
# Benchmark results
benchmarks/*/
!benchmarks/.gitkeep
```

创建 `benchmarks/.gitkeep`：

```bash
touch benchmarks/.gitkeep
```

- [ ] **Step 3: Commit**

```bash
git add internal/benchmark/ benchmarks/ .gitignore
git commit -m "chore(benchmark): establish benchmark directory structure"
```

---

## Task 2: 补充缺失的微基准 — Resolver

**Files:**
- Create: `internal/benchmark/micro/resolver_bench_test.go`

- [ ] **Step 1: 编写 resolver 基准测试**

```go
package micro

import (
	"testing"
	"time"

	"rua.plus/lolly/internal/resolver"
)

func BenchmarkResolverLookup(b *testing.B) {
	// 使用 mock resolver 避免真实网络请求
	r := resolver.NewMockResolver(map[string][]string{
		"example.com": {"93.184.216.34"},
	})

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, _ = r.Lookup("example.com")
	}
}

func BenchmarkResolverLookupWithCache(b *testing.B) {
	r := resolver.NewMockResolver(map[string][]string{
		"example.com": {"93.184.216.34"},
	})
	// 预热缓存
	_, _ = r.Lookup("example.com")

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, _ = r.Lookup("example.com")
	}
}

func BenchmarkResolverCacheSet(b *testing.B) {
	r := resolver.NewMockResolver(nil)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		r.CacheSet("host"+string(rune(b.N)), []string{"1.2.3.4"}, time.Minute)
	}
}

func BenchmarkResolverCacheGet(b *testing.B) {
	r := resolver.NewMockResolver(nil)
	r.CacheSet("example.com", []string{"1.2.3.4"}, time.Minute)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, _ = r.CacheGet("example.com")
	}
}
```

- [ ] **Step 2: 运行测试验证**

```bash
go test -bench=. -benchmem ./internal/benchmark/micro/resolver_bench_test.go
```

Expected: 4 个 benchmark 全部运行，无编译错误

- [ ] **Step 3: Commit**

```bash
git add internal/benchmark/micro/resolver_bench_test.go
git commit -m "feat(benchmark): add resolver micro benchmarks"
```

---

## Task 3: 补充缺失的微基准 — Stream

**Files:**
- Create: `internal/benchmark/micro/stream_bench_test.go`

- [ ] **Step 1: 编写 stream 基准测试**

```go
package micro

import (
	"io"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
	"rua.plus/lolly/internal/stream"
)

func BenchmarkStreamTCPForward(b *testing.B) {
	// 创建后端 echo 服务器
	backendLn, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(b, err)
	defer backendLn.Close()

	go func() {
		for {
			conn, err := backendLn.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				_, _ = io.Copy(c, c)
			}(conn)
		}
	}()

	// 创建 stream server
	srv := stream.NewServer()
	_ = srv.AddUpstream("test", []stream.TargetSpec{
		{Addr: backendLn.Addr().String(), Weight: 1},
	}, "round_robin", stream.HealthCheckSpec{})

	// 设置 upstream 健康
	srv.SetHealthy("test", 0, true)

	_ = srv.ListenTCP("127.0.0.1:0", "test")
	_ = srv.Start()
	defer srv.Stop()

	proxyAddr := srv.GetListenerAddr("test")

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		conn, err := net.Dial("tcp", proxyAddr)
		if err != nil {
			b.Fatal(err)
		}
		_, _ = conn.Write([]byte("hello"))
		buf := make([]byte, 5)
		_, _ = io.ReadFull(conn, buf)
		conn.Close()
	}
}

func BenchmarkStreamSelectTarget(b *testing.B) {
	srv := stream.NewServer()
	_ = srv.AddUpstream("test", []stream.TargetSpec{
		{Addr: "127.0.0.1:8001", Weight: 3},
		{Addr: "127.0.0.1:8002", Weight: 2},
		{Addr: "127.0.0.1:8003", Weight: 1},
	}, "weighted_round_robin", stream.HealthCheckSpec{})

	for i := 0; i < 3; i++ {
		srv.SetHealthy("test", i, true)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, _ = srv.SelectTarget("test", nil)
	}
}
```

- [ ] **Step 2: 运行测试验证**

```bash
go test -bench=. -benchmem ./internal/benchmark/micro/stream_bench_test.go
```

Expected: 2 个 benchmark 全部运行

- [ ] **Step 3: Commit**

```bash
git add internal/benchmark/micro/stream_bench_test.go
git commit -m "feat(benchmark): add stream proxy micro benchmarks"
```

---

## Task 4: 补充缺失的微基准 — Cache

**Files:**
- Create: `internal/benchmark/micro/cache_bench_test.go`

- [ ] **Step 1: 编写 cache 基准测试**

```go
package micro

import (
	"testing"
	"time"

	"rua.plus/lolly/internal/cache"
)

func BenchmarkCacheGet(b *testing.B) {
	c := cache.New(cache.Config{MaxEntries: 10000})
	_ = c.Set("key", []byte("value"), time.Hour)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, _ = c.Get("key")
	}
}

func BenchmarkCacheSet(b *testing.B) {
	c := cache.New(cache.Config{MaxEntries: 10000})
	value := []byte("value")

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = c.Set("key"+string(rune(b.N)), value, time.Hour)
	}
}

func BenchmarkCacheGetConcurrent(b *testing.B) {
	c := cache.New(cache.Config{MaxEntries: 10000})
	for i := 0; i < 1000; i++ {
		_ = c.Set(string(rune(i)), []byte("value"), time.Hour)
	}

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			_, _ = c.Get(string(rune(i % 1000)))
			i++
		}
	})
}

func BenchmarkCacheSetConcurrent(b *testing.B) {
	c := cache.New(cache.Config{MaxEntries: 10000})
	value := []byte("value")

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			_ = c.Set(string(rune(i)), value, time.Hour)
			i++
		}
	})
}
```

- [ ] **Step 2: 运行测试验证**

```bash
go test -bench=. -benchmem ./internal/benchmark/micro/cache_bench_test.go
```

- [ ] **Step 3: Commit**

```bash
git add internal/benchmark/micro/cache_bench_test.go
git commit -m "feat(benchmark): add cache micro benchmarks"
```

---

## Task 5: 补充缺失的微基准 — Lua

**Files:**
- Create: `internal/benchmark/micro/lua_bench_test.go`

- [ ] **Step 1: 编写 Lua 基准测试**

```go
package micro

import (
	"testing"

	"rua.plus/lolly/internal/lua"
)

func BenchmarkLuaSimpleScript(b *testing.B) {
	engine := lua.NewEngine()
	defer engine.Close()

	script := `
		local a = 1 + 2
		return a
	`

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = engine.ExecuteString(script)
	}
}

func BenchmarkLuaNginxAPI(b *testing.B) {
	engine := lua.NewEngine()
	defer engine.Close()

	script := `
		ngx.var.request_uri = "/test"
		return ngx.var.request_uri
	`

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = engine.ExecuteString(script)
	}
}

func BenchmarkLuaJSONEncode(b *testing.B) {
	engine := lua.NewEngine()
	defer engine.Close()

	script := `
		local json = require("cjson")
		local t = {name = "test", value = 123}
		return json.encode(t)
	`

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = engine.ExecuteString(script)
	}
}
```

- [ ] **Step 2: 运行测试验证**

```bash
go test -bench=. -benchmem ./internal/benchmark/micro/lua_bench_test.go
```

- [ ] **Step 3: Commit**

```bash
git add internal/benchmark/micro/lua_bench_test.go
git commit -m "feat(benchmark): add lua engine micro benchmarks"
```

---

## Task 6: 创建集成基准测试 — Server

**Files:**
- Create: `internal/benchmark/integration/server_bench_test.go`

- [ ] **Step 1: 编写服务器集成基准**

```go
package integration

import (
	"fmt"
	"testing"

	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/config"
	"rua.plus/lolly/internal/server"
)

func BenchmarkServerStaticRequest(b *testing.B) {
	cfg := &config.Config{
		Servers: []config.ServerConfig{{
			Listen: "127.0.0.1:0",
			Static: []config.StaticConfig{{
				Path: "/",
				Root: "./testdata",
			}},
		}},
	}

	srv := server.New(cfg)
	go srv.Start()
	defer srv.Stop()

	// 等待服务器启动
	addr := srv.GetAddr()

	client := &fasthttp.Client{}
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	req.SetRequestURI("http://" + addr + "/")
	req.Header.SetMethod("GET")

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = client.Do(req, resp)
	}
}

func BenchmarkServerProxyRequest(b *testing.B) {
	// 启动后端服务器
	backend := &fasthttp.Server{
		Handler: func(ctx *fasthttp.RequestCtx) {
			ctx.SetBodyString("ok")
		},
	}
	go backend.ListenAndServe("127.0.0.1:18081")

	cfg := &config.Config{
		Servers: []config.ServerConfig{{
			Listen: "127.0.0.1:0",
			Proxy: []config.ProxyConfig{{
				Path: "/api",
				Targets: []config.ProxyTarget{{
					URL: "http://127.0.0.1:18081",
				}},
			}},
		}},
	}

	srv := server.New(cfg)
	go srv.Start()
	defer srv.Stop()

	addr := srv.GetAddr()

	client := &fasthttp.Client{}
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	req.SetRequestURI("http://" + addr + "/api/test")
	req.Header.SetMethod("GET")

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = client.Do(req, resp)
	}
}
```

- [ ] **Step 2: 运行测试验证**

```bash
go test -bench=. -benchmem ./internal/benchmark/integration/server_bench_test.go
```

- [ ] **Step 3: Commit**

```bash
git add internal/benchmark/integration/server_bench_test.go
git commit -m "feat(benchmark): add server integration benchmarks"
```

---

## Task 7: 创建系统压测脚本

**Files:**
- Create: `internal/benchmark/system/bench.sh`
- Create: `internal/benchmark/system/static.lua`
- Create: `internal/benchmark/system/proxy.lua`

- [ ] **Step 1: 编写 wrk 压测脚本 — 静态文件**

`internal/benchmark/system/static.lua`:

```lua
-- wrk static file benchmark script
wrk.method = "GET"
wrk.headers["Accept"] = "text/html"

-- 随机访问不同路径增加真实感
math.randomseed(os.time())

request = function()
    local paths = {"/", "/index.html", "/about.html", "/contact.html"}
    local path = paths[math.random(#paths)]
    return wrk.format(nil, path)
end

response = function(status, headers, body)
    if status ~= 200 then
        print("Error: " .. status)
    end
end
```

- [ ] **Step 2: 编写 wrk 压测脚本 — 代理**

`internal/benchmark/system/proxy.lua`:

```lua
-- wrk proxy benchmark script
wrk.method = "GET"
wrk.headers["Accept"] = "application/json"

request = function()
    local paths = {"/api/users", "/api/posts", "/api/comments"}
    local path = paths[math.random(#paths)]
    return wrk.format(nil, path)
end
```

- [ ] **Step 3: 编写主压测脚本**

`internal/benchmark/system/bench.sh`:

```bash
#!/bin/bash
set -e

# Lolly System Benchmark Suite
# Usage: ./bench.sh [lolly_addr] [duration]

ADDR=${1:-"http://127.0.0.1:8080"}
DURATION=${2:-"30s"}
CONNECTIONS=${3:-400}
THREADS=${4:-12}

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RESULTS_DIR="${SCRIPT_DIR}/../../../benchmarks/$(date +%Y%m%d-%H%M%S)"
mkdir -p "$RESULTS_DIR"

echo "=== Lolly System Benchmark ==="
echo "Target: $ADDR"
echo "Duration: $DURATION"
echo "Connections: $CONNECTIONS"
echo "Threads: $THREADS"
echo "Results: $RESULTS_DIR"
echo ""

# Check tools
check_tool() {
    if ! command -v "$1" &> /dev/null; then
        echo "Warning: $1 not found, skipping related tests"
        return 1
    fi
    return 0
}

# 1. Static file benchmark
echo "--- Static File Benchmark ---"
if check_tool wrk; then
    wrk -t$THREADS -c$CONNECTIONS -d$DURATION \
        -s "$SCRIPT_DIR/static.lua" \
        "$ADDR" > "$RESULTS_DIR/static.txt"
    echo "Static: $(grep 'Requests/sec' "$RESULTS_DIR/static.txt" || echo 'N/A')"
fi

# 2. Proxy benchmark
echo ""
echo "--- Proxy Benchmark ---"
if check_tool wrk; then
    wrk -t$THREADS -c$CONNECTIONS -d$DURATION \
        -s "$SCRIPT_DIR/proxy.lua" \
        "$ADDR/api" > "$RESULTS_DIR/proxy.txt"
    echo "Proxy: $(grep 'Requests/sec' "$RESULTS_DIR/proxy.txt" || echo 'N/A')"
fi

# 3. HTTP/2 benchmark
echo ""
echo "--- HTTP/2 Benchmark ---"
if check_tool h2load; then
    h2load -n100000 -c100 -m10 "$ADDR" > "$RESULTS_DIR/http2.txt" 2>&1 || true
    echo "HTTP/2: $(grep 'finished' "$RESULTS_DIR/http2.txt" || echo 'N/A')"
fi

# 4. Latency distribution with oha
echo ""
echo "--- Latency Distribution ---"
if check_tool oha; then
    oha -z $DURATION -c $CONNECTIONS "$ADDR" > "$RESULTS_DIR/latency.txt"
    echo "Latency: $(grep 'Success rate' "$RESULTS_DIR/latency.txt" || echo 'N/A')"
fi

echo ""
echo "=== Results saved to $RESULTS_DIR ==="
```

- [ ] **Step 4: 添加执行权限**

```bash
chmod +x internal/benchmark/system/bench.sh
```

- [ ] **Step 5: Commit**

```bash
git add internal/benchmark/system/
git commit -m "feat(benchmark): add system benchmark scripts"
```

---

## Task 8: 创建一键全量基准脚本

**Files:**
- Create: `scripts/bench-suite.sh`
- Modify: `Makefile`

- [ ] **Step 1: 编写一键基准脚本**

`scripts/bench-suite.sh`:

```bash
#!/bin/bash
set -e

# Run complete benchmark suite and save results

VERSION=$(git describe --tags --always --dirty 2>/dev/null || echo "dev")
RESULTS_DIR="benchmarks/$VERSION"
mkdir -p "$RESULTS_DIR/pprof"

echo "=== Lolly Benchmark Suite v$VERSION ==="
echo "Results: $RESULTS_DIR"
echo ""

# 1. Micro benchmarks
echo "--- Running Micro Benchmarks ---"
go test -bench=. -benchmem \
    ./internal/benchmark/micro/... \
    > "$RESULTS_DIR/micro.txt" 2>&1 || true

echo "Micro benchmarks done"

# 2. Integration benchmarks
echo ""
echo "--- Running Integration Benchmarks ---"
go test -bench=. -benchmem \
    ./internal/benchmark/integration/... \
    > "$RESULTS_DIR/integration.txt" 2>&1 || true

echo "Integration benchmarks done"

# 3. Existing package benchmarks
echo ""
echo "--- Running Package Benchmarks ---"
go test -bench=. -benchmem \
    ./internal/loadbalance/... \
    ./internal/matcher/... \
    ./internal/proxy/... \
    ./internal/middleware/... \
    > "$RESULTS_DIR/packages.txt" 2>&1 || true

echo "Package benchmarks done"

# 4. Summary
echo ""
echo "=== Results Summary ==="
echo "Micro:        $RESULTS_DIR/micro.txt"
echo "Integration:  $RESULTS_DIR/integration.txt"
echo "Packages:     $RESULTS_DIR/packages.txt"

if command -v benchstat &> /dev/null; then
    echo ""
    echo "--- Top Results ---"
    grep -h "Benchmark" "$RESULTS_DIR"/*.txt | head -20
fi

echo ""
echo "All results saved to $RESULTS_DIR"
```

- [ ] **Step 2: 添加 Makefile 目标**

在 `Makefile` 中添加：

```makefile
.PHONY: bench bench-stat bench-suite

# Run all benchmarks
bench:
	go test -bench=. -benchmem ./internal/benchmark/micro/... ./internal/benchmark/integration/...

# Run benchmarks and show statistics
bench-stat: bench
	@benchstat $(shell ls benchmarks/*/micro.txt 2>/dev/null | tail -1)

# Run complete benchmark suite
bench-suite:
	@bash scripts/bench-suite.sh

# Run system benchmarks (requires running server)
bench-system:
	@bash internal/benchmark/system/bench.sh
```

- [ ] **Step 3: 添加执行权限**

```bash
chmod +x scripts/bench-suite.sh
```

- [ ] **Step 4: 运行测试**

```bash
make bench-suite
```

Expected: 脚本运行成功，结果保存到 `benchmarks/dev/` 目录

- [ ] **Step 5: Commit**

```bash
git add scripts/bench-suite.sh Makefile
git commit -m "feat(benchmark): add one-click benchmark suite"
```

---

## Task 9: 运行第一轮全量基准 → 生成 Baseline

**Files:**
- Create: `benchmarks/v0.4.0/*.txt`

- [ ] **Step 1: 运行微基准**

```bash
go test -bench=. -benchmem \
    ./internal/benchmark/micro/... \
    > benchmarks/v0.4.0/micro.txt
```

- [ ] **Step 2: 运行已有包的基准**

```bash
go test -bench=. -benchmem \
    ./internal/loadbalance/... \
    ./internal/matcher/... \
    ./internal/proxy/... \
    ./internal/middleware/... \
    ./internal/server/... \
    ./internal/cache/... \
    ./internal/stream/... \
    ./internal/resolver/... \
    ./internal/variable/... \
    ./internal/lua/... \
    > benchmarks/v0.4.0/packages.txt
```

- [ ] **Step 3: 格式化基准结果**

```bash
# 如果安装了 benchstat
benchstat benchmarks/v0.4.0/micro.txt
benchstat benchmarks/v0.4.0/packages.txt
```

- [ ] **Step 4: Commit baseline**

```bash
git add benchmarks/v0.4.0/
git commit -m "chore(benchmark): add v0.4.0 baseline performance data"
```

---

## Task 10: 采集 pprof 数据

**Files:**
- Create: `benchmarks/v0.4.0/pprof/*.prof`

**前置条件**: 需要启动一个配置了 pprof 的 lolly 服务器

- [ ] **Step 1: 启动带 pprof 的测试服务器**

创建临时测试配置 `benchmark-pprof.yaml`:

```yaml
servers:
  - listen: ":8080"
    static:
      - path: "/"
        root: "./testdata"
    proxy:
      - path: "/api"
        targets:
          - url: "http://127.0.0.1:18081"

monitoring:
  pprof:
    enabled: true
    path: "/debug/pprof"
    allow:
      - "127.0.0.1"
```

启动后端 mock 服务器（可以用 Python/Node 快速启动一个 echo 服务）

启动 lolly:

```bash
./bin/lolly -c benchmark-pprof.yaml &
LOLLY_PID=$!
```

- [ ] **Step 2: 采集 CPU profile**

```bash
curl -s "http://localhost:8080/debug/pprof/profile?seconds=30" \
    > benchmarks/v0.4.0/pprof/cpu.prof
```

- [ ] **Step 3: 采集 Heap profile**

```bash
curl -s "http://localhost:8080/debug/pprof/heap" \
    > benchmarks/v0.4.0/pprof/heap.prof
```

- [ ] **Step 4: 采集 Allocs profile**

```bash
curl -s "http://localhost:8080/debug/pprof/allocs" \
    > benchmarks/v0.4.0/pprof/allocs.prof
```

- [ ] **Step 5: 采集 Goroutine profile**

```bash
curl -s "http://localhost:8080/debug/pprof/goroutine" \
    > benchmarks/v0.4.0/pprof/goroutine.prof
```

- [ ] **Step 6: 停止测试服务器**

```bash
kill $LOLLY_PID
rm benchmark-pprof.yaml
```

- [ ] **Step 7: Commit pprof 数据**

```bash
git add benchmarks/v0.4.0/pprof/
git commit -m "chore(benchmark): add v0.4.0 pprof profiles"
```

---

## Task 11: 分析瓶颈 → 生成性能报告

**Files:**
- Create: `benchmarks/v0.4.0/REPORT.md`

- [ ] **Step 1: 分析 CPU profile**

```bash
go tool pprof -top benchmarks/v0.4.0/pprof/cpu.prof > benchmarks/v0.4.0/cpu-top.txt
```

查看 Top 20 CPU 消耗函数：

```bash
go tool pprof -top -n 20 benchmarks/v0.4.0/pprof/cpu.prof
```

- [ ] **Step 2: 分析 Heap profile**

```bash
go tool pprof -top benchmarks/v0.4.0/pprof/heap.prof > benchmarks/v0.4.0/heap-top.txt
```

- [ ] **Step 3: 分析 Allocs profile**

```bash
go tool pprof -top benchmarks/v0.4.0/pprof/allocs.prof > benchmarks/v0.4.0/allocs-top.txt
```

- [ ] **Step 4: 汇总生成报告**

`benchmarks/v0.4.0/REPORT.md`:

```markdown
# Lolly v0.4.0 性能分析报告

> 生成日期: $(date)

## 1. 基准测试摘要

### 微基准
[粘贴 micro.txt 关键结果]

### 包基准
[粘贴 packages.txt 关键结果]

## 2. CPU 热点 Top 10

[粘贴 cpu-top.txt 结果]

## 3. 内存分配热点 Top 10

[粘贴 allocs-top.txt 结果]

## 4. 内存占用 Top 10

[粘贴 heap-top.txt 结果]

## 5. 优化建议

### P0 (高优先级)
- [ ] [根据分析结果填写]

### P1 (中优先级)
- [ ] [根据分析结果填写]

### P2 (低优先级)
- [ ] [根据分析结果填写]
```

- [ ] **Step 5: Commit 报告**

```bash
git add benchmarks/v0.4.0/REPORT.md benchmarks/v0.4.0/*-top.txt
git commit -m "docs(benchmark): add v0.4.0 performance analysis report"
```

---

## Task 12: 实施优化（基于报告）

> **注意**: 此 Task 的内容将在 Task 11 完成后根据实际瓶颈数据制定。以下为占位模板，实际实施时需替换为具体分析结果。

### Task 12.1: 优化 [瓶颈1]

**Files:**
- Modify: `internal/[package]/[file].go:[line-range]`

- [ ] **Step 1: 编写优化前 benchmark**

```bash
# 已有 baseline，无需重复
```

- [ ] **Step 2: 实施优化**

[根据实际瓶颈实施具体优化]

- [ ] **Step 3: 验证优化效果**

```bash
go test -bench=[BenchmarkName] -benchmem ./internal/[package]/...
benchstat benchmarks/v0.4.0/old.txt benchmarks/v0.4.0/new.txt
```

Expected: 性能提升 > 5%

- [ ] **Step 4: Commit**

```bash
git add internal/[package]/
git commit -m "perf([package]): optimize [description]"
```

### Task 12.2-12.N: 重复优化流程

对每个识别的瓶颈重复上述流程。

---

## Task 13: 建立性能回归检测

**Files:**
- Create: `.github/workflows/benchmark.yml` (如果恢复 CI)
- Create: `scripts/bench-compare.sh`
- Modify: `Makefile`

- [ ] **Step 1: 创建基准对比脚本**

`scripts/bench-compare.sh`:

```bash
#!/bin/bash
set -e

# Compare current benchmark against baseline
# Usage: ./bench-compare.sh [baseline_version]

BASELINE=${1:-"v0.4.0"}
BASELINE_FILE="benchmarks/$BASELINE/packages.txt"
CURRENT_FILE="benchmarks/current.txt"

if [ ! -f "$BASELINE_FILE" ]; then
    echo "Baseline not found: $BASELINE_FILE"
    exit 1
fi

echo "Comparing against baseline: $BASELINE"

# Run current benchmarks
go test -bench=. -benchmem \
    ./internal/loadbalance/... \
    ./internal/matcher/... \
    ./internal/proxy/... \
    ./internal/middleware/... \
    > "$CURRENT_FILE"

# Compare
if command -v benchstat &> /dev/null; then
    benchstat "$BASELINE_FILE" "$CURRENT_FILE"
else
    echo "benchstat not found, install with: go install golang.org/x/perf/cmd/benchstat@latest"
    exit 1
fi
```

- [ ] **Step 2: 添加 Makefile 目标**

```makefile
.PHONY: bench-compare

# Compare current performance against baseline
bench-compare:
	@bash scripts/bench-compare.sh
```

- [ ] **Step 3: 添加执行权限**

```bash
chmod +x scripts/bench-compare.sh
```

- [ ] **Step 4: 测试回归检测**

```bash
make bench-compare
```

Expected: 显示当前性能与 baseline 的对比，无显著退化

- [ ] **Step 5: Commit**

```bash
git add scripts/bench-compare.sh Makefile
git commit -m "feat(benchmark): add performance regression detection"
```

---

## Task 14: 最终验证

- [ ] **Step 1: 全量测试通过**

```bash
make test
```

Expected: 全部 PASS

- [ ] **Step 2: Race 检测通过**

```bash
go test -race ./internal/...
```

Expected: 零 race

- [ ] **Step 3: Lint 通过**

```bash
make lint
```

Expected: 零 issues

- [ ] **Step 4: 构建验证**

```bash
make build
```

Expected: 构建成功

- [ ] **Step 5: 最终 Commit**

```bash
git log --oneline -20
```

确认所有 benchmark 相关 commit 都在。

---

## 附录：常用命令速查

```bash
# 运行所有微基准
go test -bench=. -benchmem ./internal/benchmark/micro/...

# 运行单个基准
go test -bench=BenchmarkCacheGet -benchmem ./internal/benchmark/micro/...

# 对比两个基准结果
benchstat old.txt new.txt

# 查看 CPU profile
go tool pprof -http=:8081 benchmarks/v0.4.0/pprof/cpu.prof

# 查看内存分配
go tool pprof -http=:8081 benchmarks/v0.4.0/pprof/allocs.prof

# 生成火焰图
go tool pprof -png benchmarks/v0.4.0/pprof/cpu.prof > cpu-flamegraph.png

# 系统压测
make bench-system

# 性能回归检测
make bench-compare
```

---

## Spec Coverage Check

| Spec Section | Task |
|-------------|------|
| 建立 benchmark 目录结构 | Task 1 |
| 补充 resolver 微基准 | Task 2 |
| 补充 stream 微基准 | Task 3 |
| 补充 cache 微基准 | Task 4 |
| 补充 lua 微基准 | Task 5 |
| 集成基准测试 | Task 6 |
| 系统压测脚本 | Task 7 |
| 一键基准脚本 | Task 8 |
| 生成 baseline | Task 9 |
| 采集 pprof | Task 10 |
| 分析报告 | Task 11 |
| 实施优化 | Task 12 |
| 回归检测 | Task 13 |
| 最终验证 | Task 14 |
