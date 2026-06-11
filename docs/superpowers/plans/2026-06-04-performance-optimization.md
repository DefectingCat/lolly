# 性能热路径优化 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 消除 6 个已确认的热路径性能瓶颈，减少每请求堆分配和锁竞争。

**Architecture:** 针对 loadbalance filterHealthy（每请求分配）、RadixTree 堆分配、DNS LRU O(n) 操作、FileInfoCache 双重锁升级、ConsistentHash 双重锁、IsAvailable mutex 逐个进行激进优化。每项优化独立可测，不改变外部接口。

**Tech Stack:** Go 1.26+, sync.Pool, container/list, atomic operations, unsafe pointer (b2s/s2b)

---

## Task 1: loadbalance — filterHealthy 零分配优化

**Files:**
- Modify: `internal/loadbalance/balancer.go`
- Test: `internal/loadbalance/balancer_test.go`
- Benchmark: `internal/loadbalance/balancer_bench_test.go`

**问题**: `filterHealthy` 每次调用分配 2 个切片（`available` + `backups`），`filterHealthyAndExclude` 分配 3 个（加 `excludeSet` map）。`IPHash.SelectByIP` 额外分配 `fnv.New64a()` 对象。这些在每个请求的负载均衡选择中触发。

**方案**: 引入 `filterContext` 结构体持有可复用缓冲区，通过 `sync.Pool` 管理。`filterHealthy` 改为写入 `filterContext` 的预分配切片而非每次 `make`。IPHash 使用内联 FNV-64a 哈希避免 `fnv.New64a()` 分配。

- [ ] **Step 1: 定义 filterContext 和 Pool**

在 `balancer.go` 中添加：

```go
type filterContext struct {
	available []*Target
	backups   []*Target
	excludeSet map[string]bool
}

var filterContextPool = sync.Pool{
	New: func() any {
		return &filterContext{
			available:  make([]*Target, 0, 64),
			backups:    make([]*Target, 0, 64),
			excludeSet: make(map[string]bool, 8),
		}
	},
}

func acquireFilterContext() *filterContext {
	fc := filterContextPool.Get().(*filterContext)
	return fc
}

func releaseFilterContext(fc *filterContext) {
	fc.available = fc.available[:0]
	fc.backups = fc.backups[:0]
	for k := range fc.excludeSet {
		delete(fc.excludeSet, k)
	}
	filterContextPool.Put(fc)
}
```

- [ ] **Step 2: 重写 filterHealthy 为 filterInto**

```go
func filterInto(fc *filterContext, targets []*Target) []*Target {
	for _, t := range targets {
		if !t.IsAvailable() {
			continue
		}
		if t.IsBackup() {
			fc.backups = append(fc.backups, t)
		} else {
			fc.available = append(fc.available, t)
		}
	}
	if len(fc.available) > 0 {
		return fc.available
	}
	return fc.backups
}
```

- [ ] **Step 3: 重写 filterHealthyAndExclude 为 filterIntoExcluding**

```go
func filterIntoExcluding(fc *filterContext, targets []*Target, excluded []*Target) []*Target {
	if len(excluded) > 0 {
		for _, t := range excluded {
			if t != nil {
				fc.excludeSet[t.URL] = true
			}
		}
	}
	for _, t := range targets {
		if !t.IsAvailable() || fc.excludeSet[t.URL] {
			continue
		}
		if t.IsBackup() {
			fc.backups = append(fc.backups, t)
		} else {
			fc.available = append(fc.available, t)
		}
	}
	if len(fc.available) > 0 {
		return fc.available
	}
	return fc.backups
}
```

- [ ] **Step 4: 添加内联 FNV-64a 哈希函数**

避免 `fnv.New64a()` 的堆分配：

```go
func fnvHash64a(key string) uint64 {
	var h uint64 = 14695981039346656037
	for i := 0; i < len(key); i++ {
		h ^= uint64(key[i])
		h *= 1099511628211
	}
	return h
}
```

- [ ] **Step 5: 重写所有 Balancer 的 Select/SelectExcluding 使用 Pool**

RoundRobin 示例：
```go
func (r *RoundRobin) Select(targets []*Target) *Target {
	fc := acquireFilterContext()
	defer releaseFilterContext(fc)
	healthy := filterInto(fc, targets)
	if len(healthy) == 0 {
		return nil
	}
	idx := r.counter.Add(1) - 1
	return healthy[idx%uint64(len(healthy))]
}
```

对所有 6 个算法的 `Select`/`SelectExcluding` 方法应用相同模式。
IPHash 中将 `fnv.New64a()` + `h.Write()` + `h.Sum64()` 替换为 `fnvHash64a(clientIP)`。
ConsistentHash 中 `hashKeyString` 也替换为 `fnvHash64a`。

- [ ] **Step 6: 保留旧函数作为兼容别名（可选）**

保留 `filterHealthy` 和 `filterHealthyAndExclude` 函数签名但标记 `// Deprecated`，内部调用新实现，确保外部调用方不受影响。如果没有外部调用方，可直接删除。

- [ ] **Step 7: 运行现有测试验证正确性**

```bash
go test -v -count=1 ./internal/loadbalance/...
```

预期：全部 PASS，无行为变化。

- [ ] **Step 8: 运行基准测试验证性能提升**

```bash
go test -bench=BenchmarkAllBalancers -benchmem -count=5 ./internal/loadbalance/...
```

预期：allocs/op 从 2-3 降低到 0-1。

- [ ] **Step 9: 提交**

```bash
git add internal/loadbalance/balancer.go internal/loadbalance/random.go internal/loadbalance/consistent_hash.go
git commit -m "perf(loadbalance): eliminate per-request allocations in filterHealthy with sync.Pool"
```

---

## Task 2: loadbalance — IsAvailable 无锁化

**Files:**
- Modify: `internal/loadbalance/balancer.go`
- Test: `internal/loadbalance/balancer_test.go`

**问题**: `IsAvailable()` 在 `MaxFails > 0` 时获取 `failMu` mutex。这发生在 `filterHealthy`/`filterInto` 的每次目标遍历中，意味着每次 LB Select 都会对每个目标加锁一次。

**方案**: 将 `failCount` 和 `failedUntil` 改为 atomic 操作，消除 `failMu` mutex。使用 CAS 循环实现 `RecordFailure` 和冷却重置。

- [ ] **Step 1: 修改 Target 字段为 atomic**

```go
type Target struct {
	// ... 保留其他字段 ...
	failCount   atomic.Int64
	failedUntil atomic.Int64
	// 删除: failMu sync.Mutex
}
```

- [ ] **Step 2: 重写 IsAvailable 为无锁版本**

```go
func (t *Target) IsAvailable() bool {
	if !t.Healthy.Load() || t.Down {
		return false
	}
	if t.MaxConns > 0 && atomic.LoadInt64(&t.Connections) >= t.MaxConns {
		return false
	}
	if t.MaxFails > 0 {
		failCount := t.failCount.Load()
		if failCount >= t.MaxFails {
			failedUntil := t.failedUntil.Load()
			if time.Now().UnixNano() < failedUntil {
				return false
			}
			// 冷却已过期，尝试重置（允许竞争，不影响正确性）
			if failedUntil > 0 {
				t.failCount.Store(0)
				t.failedUntil.Store(0)
			}
		}
	}
	return true
}
```

- [ ] **Step 3: 重写 RecordFailure 和 RecordSuccess 为无锁版本**

```go
func (t *Target) RecordFailure() int64 {
	if t.MaxFails <= 0 {
		return 0
	}
	count := t.failCount.Add(1)
	if count >= t.MaxFails {
		timeout := t.FailTimeout
		if timeout <= 0 {
			timeout = 10 * time.Second
		}
		t.failedUntil.Store(time.Now().Add(timeout).UnixNano())
	}
	return count
}

func (t *Target) RecordSuccess() {
	if t.MaxFails <= 0 {
		return
	}
	t.failCount.Store(0)
	t.failedUntil.Store(0)
}
```

- [ ] **Step 4: 运行测试**

```bash
go test -v -count=1 -run=TestTarget ./internal/loadbalance/...
```

预期：全部 PASS。

- [ ] **Step 5: 运行完整包测试**

```bash
go test -v -count=1 ./internal/loadbalance/...
```

- [ ] **Step 6: 提交**

```bash
git add internal/loadbalance/balancer.go
git commit -m "perf(loadbalance): replace failMu mutex with atomic operations in IsAvailable"
```

---

## Task 3: matcher — RadixTree 零分配搜索

**Files:**
- Modify: `internal/matcher/radix.go`
- Test: `internal/matcher/radix_test.go`, `internal/matcher/integration_test.go`
- Benchmark: 新建 `internal/matcher/radix_bench_test.go`

**问题**: `searchLongest` 递归搜索中，每次遇到带 handler 的节点都分配 `&MatchResult{}`，一次查找可能分配 N 个 MatchResult 但只保留 1 个。正则匹配器 `GetCaptures` 每次分配 `map[string]string`。

**方案**: 使用 `sync.Pool` 复用 MatchResult。引入 `searchState` 避免递归中的多次分配，改为栈式迭代或就地更新最佳匹配。

- [ ] **Step 1: 添加 MatchResult Pool**

在 `radix.go` 中添加：

```go
var matchResultPool = sync.Pool{
	New: func() any {
		return &MatchResult{}
	},
}
```

- [ ] **Step 2: 重写 searchLongest 为就地更新最佳匹配**

将递归中创建 newMatch 改为直接比较节点字段，仅在最终返回时从池中获取 MatchResult：

```go
func (t *RadixTree) searchLongest(node *RadixNode, path string, bestNode *RadixNode, bestPrefixLen int) *RadixNode {
	if node == nil || path == "" {
		return bestNode
	}
	if !strings.HasPrefix(path, node.prefix) {
		return bestNode
	}
	remaining := path[len(node.prefix):]
	if node.handler != nil {
		if bestNode == nil || node.priority < bestNode.priority {
			bestNode = node
		} else if node.priority == bestNode.priority && len(node.prefix) > bestPrefixLen {
			bestNode = node
		}
	}
	for _, child := range node.children {
		bestNode = t.searchLongest(child, remaining, bestNode, bestPrefixLen)
	}
	return bestNode
}
```

- [ ] **Step 3: 修改 FindLongestPrefix 在返回时构建 MatchResult**

```go
func (t *RadixTree) FindLongestPrefix(path string) *MatchResult {
	bestNode := t.searchLongest(t.root, path, nil, 0)
	if bestNode == nil {
		return nil
	}
	result := matchResultPool.Get().(*MatchResult)
	result.Handler = bestNode.handler
	result.Path = bestNode.prefix
	result.Priority = bestNode.priority
	result.LocationType = bestNode.locationType
	result.Internal = bestNode.internal
	return result
}
```

注意：调用方使用完 MatchResult 后需调用 `PutMatchResult(result)` 归还池。

- [ ] **Step 4: 添加 ReleaseMatchResult 函数供调用方使用**

```go
func ReleaseMatchResult(r *MatchResult) {
	if r == nil {
		return
	}
	r.Handler = nil
	r.Captures = nil
	r.Path = ""
	r.LocationType = ""
	r.Internal = false
	r.Priority = 0
	matchResultPool.Put(r)
}
```

- [ ] **Step 5: 更新 LocationEngine.Match 调用 FindLongestPrefix 后释放**

在 `location.go` 中，确保所有 `FindLongestPrefix` 返回值在函数结束前调用 `ReleaseMatchResult`（需分析调用链确认所有权）。

- [ ] **Step 6: 添加基准测试文件**

创建 `internal/matcher/radix_bench_test.go`：

```go
func BenchmarkRadixTreeFindLongestPrefix(b *testing.B) {
	tree := NewRadixTree()
	paths := []string{"/", "/api", "/api/v1", "/api/v1/users", "/api/v1/users/:id", "/static", "/static/css", "/static/js", "/health", "/favicon.ico"}
	for _, p := range paths {
		tree.Insert(p, func(ctx *fasthttp.RequestCtx) {}, 0, "prefix", false)
	}
	tree.MarkInitialized()

	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		result := tree.FindLongestPrefix("/api/v1/users/123")
		ReleaseMatchResult(result)
	}
}

func BenchmarkRadixTreeFindLongestPrefixParallel(b *testing.B) {
	// 同上但用 b.RunParallel
}
```

- [ ] **Step 7: 运行所有 matcher 测试**

```bash
go test -v -count=1 ./internal/matcher/...
```

- [ ] **Step 8: 运行基准测试**

```bash
go test -bench=BenchmarkRadixTree -benchmem ./internal/matcher/...
```

预期：allocs/op 从 N（匹配路径上的 handler 节点数）降低到 1（仅池获取）。

- [ ] **Step 9: 提交**

```bash
git add internal/matcher/radix.go internal/matcher/radix_bench_test.go
git commit -m "perf(matcher): eliminate heap allocations in RadixTree search with sync.Pool"
```

---

## Task 4: resolver — LRU 从 O(n) 切换到 O(1)

**Files:**
- Modify: `internal/resolver/resolver.go`, `internal/resolver/cache.go`
- Test: `internal/resolver/resolver_test.go`, `internal/resolver/mock_dns_test.go`
- Benchmark: `internal/resolver/resolver_bench_test.go`

**问题**: DNS 缓存的 LRU 使用 `[]string` 切片实现 `moveToFrontLocked`，每次操作 O(n) 线性扫描 + 切片重组。`storeCache` 持有写锁执行整个 O(n) 操作，阻塞所有并发读。

**方案**: 将 LRU 从 `[]string` 切片替换为 `container/list` + `map[string]*list.Element`（与 FileCache 和 FileInfoCache 的模式一致）。moveToFront 和 eviction 都变为 O(1)。

- [ ] **Step 1: 修改 DNSResolver 结构体**

```go
type DNSResolver struct {
	config       *config.ResolverConfig
	stopCh       chan struct{}
	refreshHosts map[string]struct{}
	cache        map[string]*DNSCacheEntry
	lruList      *list.List                    // 替代 lruOrder []string
	lruIndex     map[string]*list.Element      // 新增：host -> list.Element
	hits         atomic.Int64
	misses       atomic.Int64
	errors       atomic.Int64
	latencyNs    atomic.Int64
	count        atomic.Int64
	mu           sync.RWMutex
	serverIdx    atomic.Uint32
	started      atomic.Bool
}
```

- [ ] **Step 2: 重写 storeCache**

```go
func (r *DNSResolver) storeCache(host string, entry *DNSCacheEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if elem, ok := r.lruIndex[host]; ok {
		r.cache[host] = entry
		r.lruList.MoveToFront(elem)
		return
	}

	if r.config.CacheSize > 0 && len(r.cache) >= r.config.CacheSize {
		r.evictLRULocked()
	}

	r.cache[host] = entry
	elem := r.lruList.PushFront(host)
	r.lruIndex[host] = elem
}
```

- [ ] **Step 3: 重写 evictLRULocked**

```go
func (r *DNSResolver) evictLRULocked() {
	oldest := r.lruList.Back()
	if oldest == nil {
		return
	}
	host := oldest.Value.(string)
	delete(r.cache, host)
	delete(r.lruIndex, host)
	r.lruList.Remove(oldest)
}
```

- [ ] **Step 4: 删除 moveToFrontLocked**（不再需要，由 `lruList.MoveToFront` 替代）

- [ ] **Step 5: 更新 New() 构造函数**

```go
return &DNSResolver{
	config:       &configCopy,
	stopCh:       make(chan struct{}),
	refreshHosts: make(map[string]struct{}),
	cache:        make(map[string]*DNSCacheEntry),
	lruList:      list.New(),
	lruIndex:     make(map[string]*list.Element),
}
```

- [ ] **Step 6: 更新 DeleteCacheEntry**

```go
func (r *DNSResolver) DeleteCacheEntry(host string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.cache, host)
	if elem, ok := r.lruIndex[host]; ok {
		r.lruList.Remove(elem)
		delete(r.lruIndex, host)
	}
	delete(r.refreshHosts, host)
}
```

- [ ] **Step 7: 更新 ClearCache**

```go
func (r *DNSResolver) ClearCache() {
	r.mu.Lock()
	r.cache = make(map[string]*DNSCacheEntry)
	r.lruList = list.New()
	r.lruIndex = make(map[string]*list.Element)
	r.refreshHosts = make(map[string]struct{})
	r.mu.Unlock()
}
```

- [ ] **Step 8: 添加 import "container/list"**

- [ ] **Step 9: 运行所有 resolver 测试**

```bash
go test -v -count=1 ./internal/resolver/...
```

- [ ] **Step 10: 运行基准测试验证**

```bash
go test -bench=BenchmarkDNS -benchmem -count=5 ./internal/resolver/...
```

预期：`BenchmarkDNSResolverCacheWriteLock` 和 `BenchmarkDNSResolverMixedWorkload` 显著提速。

- [ ] **Step 11: 提交**

```bash
git add internal/resolver/resolver.go internal/resolver/cache.go
git commit -m "perf(resolver): replace slice-based LRU with container/list for O(1) operations"
```

---

## Task 5: handler — FileInfoCache 近似 LRU 消除读锁升级

**Files:**
- Modify: `internal/handler/fileinfo_cache.go`
- Test: `internal/handler/static_test.go`（间接，通过现有测试验证）
- Benchmark: `internal/handler/static_bench_test.go`

**问题**: `FileInfoCache.Get()` 在每次缓存命中时需要 **两次锁获取**：先 RLock 检查存在性和 TTL，然后释放 RLock，再 Lock 做 `MoveToFront` LRU 更新。每次命中都有 RLock→Lock 升级。

**方案**: 采用近似 LRU 策略——Get 路径跳过 `MoveToFront`，仅 RLock 快速路径返回。仅在 Set 路径（写操作）时更新 LRU 位置。这与 FileCache 的近似 LRU 策略一致。

- [ ] **Step 1: 重写 Get 为纯 RLock 快速路径**

```go
func (c *FileInfoCache) Get(filePath string) (os.FileInfo, bool) {
	c.mu.RLock()
	entry, ok := c.entries[filePath]
	if !ok {
		c.mu.RUnlock()
		return nil, false
	}
	if time.Since(entry.cachedAt) > fileInfoCacheTTL {
		c.mu.RUnlock()
		// 过期删除仍需写锁
		c.mu.Lock()
		if e, ok := c.entries[filePath]; ok && time.Since(e.cachedAt) > fileInfoCacheTTL {
			c.lruList.Remove(e.element)
			delete(c.entries, filePath)
		}
		c.mu.Unlock()
		return nil, false
	}
	info := entry.info
	c.mu.RUnlock()
	return info, true
}
```

- [ ] **Step 2: 在 Set 中添加 LRU 位置更新**

```go
func (c *FileInfoCache) Set(filePath string, info os.FileInfo) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if entry, ok := c.entries[filePath]; ok {
		entry.info = info
		entry.cachedAt = time.Now()
		c.lruList.MoveToFront(entry.element)
		return
	}
	// ... 淘汰和插入逻辑不变 ...
}
```

- [ ] **Step 3: 添加 FileInfoCache 专项基准测试**

在 `internal/handler/static_bench_test.go` 中添加：

```go
func BenchmarkFileInfoCacheGetHit(b *testing.B) {
	cache := NewFileInfoCache()
	info, _ := os.Stat("testdata/style.css")
	cache.Set("/style.css", info)

	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		cache.Get("/style.css")
	}
}

func BenchmarkFileInfoCacheGetHitParallel(b *testing.B) {
	cache := NewFileInfoCache()
	info, _ := os.Stat("testdata/style.css")
	cache.Set("/style.css", info)

	b.ResetTimer()
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			cache.Get("/style.css")
		}
	})
}
```

注意：需确认 `NewFileInfoCache` 是否已导出，若未导出则在包内测试。

- [ ] **Step 4: 运行所有 handler 测试**

```bash
go test -v -count=1 ./internal/handler/...
```

- [ ] **Step 5: 运行基准测试**

```bash
go test -bench=BenchmarkFileInfoCache -benchmem ./internal/handler/...
```

预期：Get hit 路径从 2 次锁操作降到 1 次 RLock，并行吞吐显著提升。

- [ ] **Step 6: 提交**

```bash
git add internal/handler/fileinfo_cache.go internal/handler/static_bench_test.go
git commit -m "perf(handler): eliminate read-lock upgrade in FileInfoCache.Get with approximate LRU"
```

---

## Task 6: loadbalance — ConsistentHash 消除双重锁

**Files:**
- Modify: `internal/loadbalance/consistent_hash.go`
- Test: `internal/loadbalance/balancer_test.go`

**问题**: `SelectByKey` 和 `SelectExcludingByKey` 在发现 `circle` 为空时执行 `RLock → RUnlock → rebuildCircle(Lock) → RLock`，即释放读锁、获取写锁重建、再获取读锁。在冷启动高并发时，多个 goroutine 可能同时触发 rebuild。

**方案**: 使用 `sync.Once` 或 `atomic.Bool` 保证 rebuild 只执行一次。在首次 Select 前完成 rebuild，后续调用直接 RLock 读取。同时将 `hashKeyString` 替换为内联 `fnvHash64a`（Task 1 中已定义）。

- [ ] **Step 1: 添加 rebuildOnce 字段**

```go
type ConsistentHash struct {
	circle       map[uint64]*Target
	hashKey      string
	sortedHashes []uint64
	virtualNodes int
	mu           sync.RWMutex
	rebuilt      atomic.Bool
}
```

- [ ] **Step 2: 重写 SelectByKey 使用 ensureRebuilt**

```go
func (c *ConsistentHash) ensureRebuilt(targets []*Target) {
	if c.rebuilt.Load() {
		return
	}
	c.rebuildCircle(targets)
}

func (c *ConsistentHash) SelectByKey(targets []*Target, key string) *Target {
	c.ensureRebuilt(targets)

	c.mu.RLock()
	defer c.mu.RUnlock()

	if len(c.sortedHashes) == 0 {
		return nil
	}

	hash := fnvHash64a(key)
	idx := sort.Search(len(c.sortedHashes), func(i int) bool {
		return c.sortedHashes[i] >= hash
	})
	if idx >= len(c.sortedHashes) {
		idx = 0
	}
	return c.circle[c.sortedHashes[idx]]
}
```

- [ ] **Step 3: 更新 Rebuild 方法重置 rebuilt 标志**

```go
func (c *ConsistentHash) Rebuild(targets []*Target) {
	c.rebuilt.Store(false)
	c.rebuildCircle(targets)
}
```

- [ ] **Step 4: 更新 rebuildCircle 设置 rebuilt 标志**

```go
func (c *ConsistentHash) rebuildCircle(targets []*Target) {
	c.mu.Lock()
	defer c.mu.Unlock()
	// ... 现有逻辑不变 ...
	c.rebuilt.Store(true)
}
```

- [ ] **Step 5: 同样更新 SelectExcludingByKey**

移除内部的 `RLock → RUnlock → rebuildCircle → RLock` 模式，改为先 `ensureRebuilt` 再 `RLock`。

- [ ] **Step 6: 将 hashKeyString 替换为 fnvHash64a**

```go
// 删除 hashKeyString 方法
// 在 PrecomputeHashes 中将 c.hashKeyString(key) 替换为 fnvHash64a(key)
```

- [ ] **Step 7: 运行测试**

```bash
go test -v -count=1 ./internal/loadbalance/...
```

- [ ] **Step 8: 运行基准测试**

```bash
go test -bench=BenchmarkConsistentHash -benchmem ./internal/loadbalance/...
```

- [ ] **Step 9: 提交**

```bash
git add internal/loadbalance/consistent_hash.go
git commit -m "perf(loadbalance): eliminate double-lock in ConsistentHash with atomic rebuild guard"
```

---

## Task 7: 全局验证与基准对比

**Files:**
- 无新文件修改

- [ ] **Step 1: 运行完整测试套件**

```bash
make test
```

- [ ] **Step 2: 运行集成测试**

```bash
make test-integration
```

- [ ] **Step 3: 运行代码格式化和静态检查**

```bash
make fmt && make lint
```

- [ ] **Step 4: 保存基准对比结果**

```bash
make bench-stat
mv benchmark-current.txt bench-after-optimization.txt
```

如有优化前的基准数据，运行 `benchstat bench-before.txt bench-after-optimization.txt` 对比。

- [ ] **Step 5: 最终提交（如有 lint 修复）**

```bash
git add -A
git commit -m "chore: lint fixes after performance optimization"
```

---

## 依赖关系

```
Task 1 (filterHealthy Pool) ──→ Task 6 (ConsistentHash，复用 fnvHash64a)
Task 2 (IsAvailable atomic) ──→ 无依赖（可并行）
Task 3 (RadixTree Pool)     ──→ 无依赖（可并行）
Task 4 (Resolver LRU)        ──→ 无依赖（可并行）
Task 5 (FileInfoCache)       ──→ 无依赖（可并行）
Task 7 (全局验证)            ──→ 依赖 Task 1-6 全部完成
```

**推荐并行执行**: Task 1+2 可同一批（同一文件），Task 3/4/5 可并行，Task 6 在 Task 1 后执行。
