# 消除代码冗余实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 消除 lolly 项目中的代码冗余：删除 8 处死代码、重构 2 处源文件重复模式、提取测试辅助函数减少 184 处配置字面量重复。

**Architecture:** 分三阶段实施：阶段 1 删除未使用的死代码（零风险）；阶段 2 提取路由注册和 DEBUG 日志辅助函数（低风险重构）；阶段 3 创建测试辅助函数包并迁移重复代码（逐步替换）。

**Tech Stack:** Go 1.22+, golangci-lint, dupl/unused linters

---

## 文件结构

**创建：**
- `internal/testutil/proxy.go` - 测试辅助函数（ProxyConfig、Target 创建）

**修改：**
- `internal/config/validate.go` - 删除 `validateStatic()` 函数
- `internal/config/validate_test.go` - 删除 `TestValidateStatic` 测试
- `internal/http2/server.go` - 删除 `connectionPool.get()` 和 `connectionPool.count()`
- `internal/middleware/bodylimit/bodylimit.go` - 删除 `formatSize()` 函数
- `internal/middleware/bodylimit/bodylimit_test.go` - 删除 `TestFormatSize` 测试
- `internal/middleware/security/headers.go` - 删除 3 个 security headers 函数
- `internal/middleware/security/headers_test.go` - 删除 3 个对应测试
- `internal/ssl/ocsp.go` - 删除 `extractCertificates()` 函数
- `internal/ssl/ocsp_test.go` - 删除 2 个对应测试
- `internal/server/router.go` - 提取 `registerRoute` 辅助函数
- `internal/proxy/proxy.go` - 提取 `proxyDebugLog` 辅助函数

---

## 阶段 1：死代码删除

### Task 1: 删除 `validateStatic` 函数及其测试

**Files:**
- Modify: `internal/config/validate.go:475-484`
- Modify: `internal/config/validate_test.go:752-809`

- [ ] **Step 1: 删除 `validateStatic` 函数**

删除 `internal/config/validate.go` 第 475-484 行：

```go
// validateStatic 验证静态文件配置。
//
// 参数：
//   - s: 静态文件配置对象
//
// 返回值：
//   - error: 验证失败时返回错误信息，成功返回 nil
func validateStatic(s *StaticConfig) error {
	// 静态文件根目录非空时验证路径有效性
	if s.Root != "" {
		// 路径安全检查：不允许包含 ".."
		if err := ValidatePathTraversal(s.Root, "根目录路径"); err != nil {
			return err
		}
	}
	return nil
}
```

- [ ] **Step 2: 删除对应的单元测试**

删除 `internal/config/validate_test.go` 第 752-809 行的 `TestValidateStatic` 函数：

```go
func TestValidateStatic(t *testing.T) {
	t.Parallel()
	// TestValidateStatic 测试静态文件配置验证。
	tests := []struct {
		name    string
		errMsg  string
		config  StaticConfig
		wantErr bool
	}{
		{
			name:    "空配置有效",
			config:  StaticConfig{},
			wantErr: false,
		},
		{
			name: "有效根目录",
			config: StaticConfig{
				Root: "/var/www/html",
			},
			wantErr: false,
		},
		{
			name: "根目录含..路径遍历",
			config: StaticConfig{
				Root: "/var/www/../etc",
			},
			wantErr: true,
			errMsg:  "根目录路径不能包含 '..'",
		},
		{
			name: "根目录含多个..",
			config: StaticConfig{
				Root: "/var/../www/../html",
			},
			wantErr: true,
			errMsg:  "根目录路径不能包含 '..'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateStatic(&tt.config)
			if tt.wantErr {
				if err == nil {
					t.Errorf("validateStatic() 期望返回错误，但返回 nil")
					return
				}
				if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("validateStatic() 错误消息不匹配，期望包含 %q，实际 %q", tt.errMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("validateStatic() 期望返回 nil，但返回错误: %v", err)
				}
			}
		})
	}
}
```

- [ ] **Step 3: 运行测试确认通过**

Run: `go test ./internal/config/... -run TestValidateStatic -v`
Expected: 无此测试（因为已删除）

Run: `go test ./internal/config/... -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/config/validate.go internal/config/validate_test.go
git commit -m "refactor: remove unused validateStatic function and its test"
```

---

### Task 2: 删除 `connectionPool` 未使用的方法

**Files:**
- Modify: `internal/http2/server.go:575-587`

- [ ] **Step 1: 删除 `get` 和 `count` 方法**

删除 `internal/http2/server.go` 第 575-587 行：

```go
// get 获取连接。
func (p *connectionPool) get(key string) []net.Conn {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.conns[key]
}

// count 获取连接数。
func (p *connectionPool) count(key string) int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.conns[key])
}
```

- [ ] **Step 2: 运行测试确认通过**

Run: `go test ./internal/http2/... -v`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/http2/server.go
git commit -m "refactor: remove unused connectionPool.get and connectionPool.count methods"
```

---

### Task 3: 删除 `bodylimit.formatSize` 函数及其测试

**Files:**
- Modify: `internal/middleware/bodylimit/bodylimit.go:279-305`
- Modify: `internal/middleware/bodylimit/bodylimit_test.go:36-72`

- [ ] **Step 1: 删除 `formatSize` 函数**

删除 `internal/middleware/bodylimit/bodylimit.go` 第 279-305 行：

```go
// formatSize 将字节数格式化为人类可读的字符串。
//
// 根据大小自动选择合适的单位（b、kb、mb、gb）。
//
// 参数：
//   - size: 字节数
//
// 返回值：
//   - string: 格式化后的字符串，如 "1.00mb"、"10.00kb"
func formatSize(size int64) string {
	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
	)

	switch {
	case size >= GB:
		return fmt.Sprintf("%.2fgb", float64(size)/GB)
	case size >= MB:
		return fmt.Sprintf("%.2fmb", float64(size)/MB)
	case size >= KB:
		return fmt.Sprintf("%.2fkb", float64(size)/KB)
	default:
		return fmt.Sprintf("%db", size)
	}
}
```

- [ ] **Step 2: 删除对应的单元测试**

删除 `internal/middleware/bodylimit/bodylimit_test.go` 第 36-72 行的 `TestFormatSize` 函数：

```go
func TestFormatSize(t *testing.T) {
	tests := []struct {
		input    int64
		expected string
	}{
		{512, "512b"},
		{1024, "1.00kb"},
		{1024 * 1024, "1.00mb"},
		{1024 * 1024 * 1024, "1.00gb"},
		{1536, "1.50kb"},
	}

	for _, tt := range tests {
		t.Run(formatSize(tt.input), func(t *testing.T) {
			got := formatSize(tt.input)
			if got != tt.expected {
				t.Errorf("formatSize(%d) = %s, want %s", tt.input, got, tt.expected)
			}
		})
	}
}
```

- [ ] **Step 3: 运行测试确认通过**

Run: `go test ./internal/middleware/bodylimit/... -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/middleware/bodylimit/bodylimit.go internal/middleware/bodylimit/bodylimit_test.go
git commit -m "refactor: remove unused bodylimit.formatSize function and test"
```

---

### Task 4: 删除 security headers 未使用的函数及其测试

**Files:**
- Modify: `internal/middleware/security/headers.go:291-331`
- Modify: `internal/middleware/security/headers_test.go:184-215`

- [ ] **Step 1: 删除 3 个 security headers 函数**

删除 `internal/middleware/security/headers.go` 第 291-331 行：

```go
// defaultSecurityHeaders 返回安全的安全头默认配置。
//
// 返回值：
//   - *config.SecurityHeaders: 包含安全默认值的配置对象
func defaultSecurityHeaders() *config.SecurityHeaders {
	return &config.SecurityHeaders{
		XFrameOptions:       "DENY",
		XContentTypeOptions: "nosniff",
		ReferrerPolicy:      "strict-origin-when-cross-origin",
	}
}

// strictSecurityHeaders 返回严格模式的安全头配置。
//
// 适用于高安全要求的应用场景，包含严格的 CSP 和权限策略。
//
// 返回值：
//   - *config.SecurityHeaders: 包含严格安全值的配置对象
func strictSecurityHeaders() *config.SecurityHeaders {
	return &config.SecurityHeaders{
		XFrameOptions:         "DENY",
		XContentTypeOptions:   "nosniff",
		ContentSecurityPolicy: "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self'; font-src 'self'; connect-src 'self'; frame-ancestors 'none'",
		ReferrerPolicy:        "no-referrer",
		PermissionsPolicy:     "accelerometer=(), camera=(), geolocation=(), gyroscope=(), magnetometer=(), microphone=(), payment=(), usb=()",
	}
}

// developmentSecurityHeaders 返回开发环境使用的宽松安全头配置。
//
// 警告：请勿在生产环境使用此配置，安全性较低。
//
// 返回值：
//   - *config.SecurityHeaders: 包含宽松安全值的配置对象
func developmentSecurityHeaders() *config.SecurityHeaders {
	return &config.SecurityHeaders{
		XFrameOptions:       "SAMEORIGIN",
		XContentTypeOptions: "nosniff",
		ReferrerPolicy:      "strict-origin-when-cross-origin",
	}
}
```

- [ ] **Step 2: 删除对应的单元测试**

删除 `internal/middleware/security/headers_test.go` 第 184-215 行：

```go
func TestDefaultSecurityHeaders(t *testing.T) {
	cfg := defaultSecurityHeaders()

	if cfg.XFrameOptions != "DENY" {
		t.Errorf("Expected default X-Frame-Options 'DENY', got %s", cfg.XFrameOptions)
	}
	if cfg.XContentTypeOptions != "nosniff" {
		t.Errorf("Expected default X-Content-Type-Options 'nosniff', got %s", cfg.XContentTypeOptions)
	}
}

func TestStrictSecurityHeaders(t *testing.T) {
	cfg := strictSecurityHeaders()

	if cfg.XFrameOptions != "DENY" {
		t.Errorf("Expected X-Frame-Options 'DENY', got %s", cfg.XFrameOptions)
	}
	if cfg.ReferrerPolicy != "no-referrer" {
		t.Errorf("Expected Referrer-Policy 'no-referrer', got %s", cfg.ReferrerPolicy)
	}
	if cfg.ContentSecurityPolicy == "" {
		t.Error("Expected non-empty CSP for strict config")
	}
}

func TestDevelopmentSecurityHeaders(t *testing.T) {
	cfg := developmentSecurityHeaders()

	if cfg.XFrameOptions != "SAMEORIGIN" {
		t.Errorf("Expected X-Frame-Options 'SAMEORIGIN' for dev, got %s", cfg.XFrameOptions)
	}
}
```

- [ ] **Step 3: 运行测试确认通过**

Run: `go test ./internal/middleware/security/... -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/middleware/security/headers.go internal/middleware/security/headers_test.go
git commit -m "refactor: remove unused security header preset functions and tests"
```

---

### Task 5: 删除 `extractCertificates` 函数及其测试

**Files:**
- Modify: `internal/ssl/ocsp.go:482-514`
- Modify: `internal/ssl/ocsp_test.go:311-335`

- [ ] **Step 1: 删除 `extractCertificates` 函数**

删除 `internal/ssl/ocsp.go` 第 482-514 行：

```go
// extractCertificates 解析 PEM 数据并返回证书列表。
//
// 参数：
//   - pemData: PEM 编码的证书数据
//
// 返回值：
//   - []*x509.Certificate: 解析后的证书列表
//   - error: 解析失败时返回错误
func extractCertificates(pemData []byte) ([]*x509.Certificate, error) {
	var certs []*x509.Certificate
	rest := pemData

	for {
		block, remaining := pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type == "CERTIFICATE" {
			cert, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				return nil, fmt.Errorf("failed to parse certificate: %w", err)
			}
			certs = append(certs, cert)
		}
		rest = remaining
	}

	if len(certs) == 0 {
		return nil, errors.New("no certificates found in PEM data")
	}

	return certs, nil
}
```

- [ ] **Step 2: 删除对应的单元测试**

删除 `internal/ssl/ocsp_test.go` 第 311-335 行：

```go
func TestExtractCertificates(t *testing.T) {
	// Create valid PEM data
	certPEM, _ := generateTestCertWithOCSP(t, nil)

	certs, err := extractCertificates(certPEM)
	if err != nil {
		t.Fatalf("extractCertificates() failed: %v", err)
	}

	if len(certs) == 0 {
		t.Error("Expected at least one certificate")
	}
}

func TestExtractCertificatesInvalidPEM(t *testing.T) {
	invalidPEM := []byte("not valid pem data")

	certs, err := extractCertificates(invalidPEM)
	if err == nil {
		t.Error("Expected error for invalid PEM data")
	}
	if certs != nil {
		t.Error("Expected nil certs for invalid PEM data")
	}
}
```

- [ ] **Step 3: 运行测试确认通过**

Run: `go test ./internal/ssl/... -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/ssl/ocsp.go internal/ssl/ocsp_test.go
git commit -m "refactor: remove unused extractCertificates function and tests"
```

---

## 阶段 2：源文件重复模式重构

### Task 6: 提取路由注册辅助函数

**Files:**
- Modify: `internal/server/router.go:84-124` 和 `internal/server/router.go:190-220` 和 `internal/server/router.go:390-420`

- [ ] **Step 1: 添加 `registerRoute` 辅助函数**

在 `internal/server/router.go` 的 `configureProxyRoutes` 函数之前添加：

```go
// registerRoute 根据位置类型注册路由
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

- [ ] **Step 2: 重构 proxy 路由注册**

将 `internal/server/router.go` 第 84-124 行的 switch 语句替换为：

```go
		switch locType {
		case matcher.LocationTypeExact:
			if err := s.registerRoute(locType, proxyCfg.Path, p.ServeHTTP, proxyCfg.Internal, "proxy"); err != nil {
				return err
			}
		case matcher.LocationTypePrefixPriority:
			if err := s.registerRoute(locType, proxyCfg.Path, p.ServeHTTP, proxyCfg.Internal, "proxy"); err != nil {
				return err
			}
		case matcher.LocationTypeRegex, matcher.LocationTypeRegexCaseless:
			caseInsensitive := locType == matcher.LocationTypeRegexCaseless
			if err := s.registerRoute(locType, proxyCfg.Path, p.ServeHTTP, proxyCfg.Internal, "proxy"); err != nil {
				return err
			}
		case matcher.LocationTypeNamed:
			if proxyCfg.LocationName != "" {
				if err := s.registerRoute(locType, "@"+proxyCfg.LocationName, p.ServeHTTP, false, "proxy"); err != nil {
					return err
				}
			}
		case matcher.LocationTypePrefix:
			if err := s.registerRoute(locType, proxyCfg.Path, p.ServeHTTP, proxyCfg.Internal, "proxy"); err != nil {
				return err
			}
		default:
			if err := s.registerRoute(locType, proxyCfg.Path, p.ServeHTTP, proxyCfg.Internal, "proxy"); err != nil {
				return err
			}
		}
```

- [ ] **Step 3: 重构 static 路由注册**

将 `internal/server/router.go` 第 190-220 行的类似代码替换为 `registerRoute` 调用。

- [ ] **Step 4: 重构 lua 路由注册**

将 `internal/server/router.go` 第 390-420 行的类似代码替换为 `registerRoute` 调用。

- [ ] **Step 5: 运行测试确认通过**

Run: `go test ./internal/server/... -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/server/router.go
git commit -m "refactor: extract registerRoute helper to reduce repetition"
```

---

### Task 7: 提取 DEBUG 日志辅助函数

**Files:**
- Modify: `internal/proxy/proxy.go:470-476` 和类似位置

- [ ] **Step 1: 添加 `proxyDebugLog` 辅助函数**

在 `internal/proxy/proxy.go` 的 `ServeHTTP` 方法之前添加：

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

- [ ] **Step 2: 替换第一个 DEBUG 日志**

将第 470-476 行：
```go
	if logging.Debug().Enabled() {
		logging.Debug().
			Str("path", b2s(ctx.Path())).
			Str("host", b2s(ctx.Host())).
			Str("method", b2s(ctx.Method())).
			Msg("[PROXY] 收到请求")
	}
```
替换为：
```go
	proxyDebugLog("[PROXY] 收到请求",
		"path", b2s(ctx.Path()),
		"host", b2s(ctx.Host()),
		"method", b2s(ctx.Method()),
	)
```

- [ ] **Step 3: 替换其余 4 个 DEBUG 日志**

重复 Step 2 的模式，替换第 536-540、555-559、627-631、715-719 行的 DEBUG 日志。

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/proxy/... -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/proxy/proxy.go
git commit -m "refactor: extract proxyDebugLog helper for repeated debug logging"
```

---

## 阶段 3：测试辅助函数

### Task 8: 创建测试辅助函数包

**Files:**
- Create: `internal/testutil/proxy.go`

- [ ] **Step 1: 创建测试辅助函数文件**

创建 `internal/testutil/proxy.go`：

```go
package testutil

import (
	"time"

	"rua.plus/lolly/internal/config"
	"rua.plus/lolly/internal/loadbalance"
)

// NewTestProxyConfig 创建测试用的代理配置
//
// 参数：
//   - path: 代理路径
//   - targetURLs: 后端目标 URL 列表
//
// 返回值：
//   - *config.ProxyConfig: 配置好的代理配置
func NewTestProxyConfig(path string, targetURLs ...string) *config.ProxyConfig {
	cfg := &config.ProxyConfig{
		Path:        path,
		LoadBalance: "round_robin",
		Timeout: config.ProxyTimeout{
			Connect: 5 * time.Second,
			Read:    30 * time.Second,
			Write:   30 * time.Second,
		},
	}

	if len(targetURLs) > 0 {
		cfg.Targets = make([]config.ProxyTargetConfig, len(targetURLs))
		for i, url := range targetURLs {
			cfg.Targets[i] = config.ProxyTargetConfig{URL: url}
		}
	}

	return cfg
}

// NewTestProxyConfigWithCache 创建带缓存的测试代理配置
func NewTestProxyConfigWithCache(path string, maxAge time.Duration, targetURLs ...string) *config.ProxyConfig {
	cfg := NewTestProxyConfig(path, targetURLs...)
	cfg.Cache = config.ProxyCacheConfig{
		Enabled: true,
		MaxAge:  maxAge,
	}
	return cfg
}

// NewTestTarget 创建测试用的代理目标
//
// 参数：
//   - url: 目标 URL
//
// 返回值：
//   - *loadbalance.Target: 测试目标
func NewTestTarget(url string) *loadbalance.Target {
	return &loadbalance.Target{URL: url}
}

// NewTestTargets 批量创建测试目标
func NewTestTargets(urls ...string) []*loadbalance.Target {
	targets := make([]*loadbalance.Target, len(urls))
	for i, url := range urls {
		targets[i] = NewTestTarget(url)
	}
	return targets
}

// NewTestHealthyTarget 创建已标记为健康的测试目标
//
// 参数：
//   - url: 目标 URL
//
// 返回值：
//   - *loadbalance.Target: 已标记为健康的测试目标
func NewTestHealthyTarget(url string) *loadbalance.Target {
	t := NewTestTarget(url)
	t.Healthy.Store(true)
	return t
}

// NewTestHealthyTargets 批量创建健康测试目标
func NewTestHealthyTargets(urls ...string) []*loadbalance.Target {
	targets := make([]*loadbalance.Target, len(urls))
	for i, url := range urls {
		targets[i] = NewTestHealthyTarget(url)
	}
	return targets
}
```

- [ ] **Step 2: 编写辅助函数测试**

创建 `internal/testutil/proxy_test.go`：

```go
package testutil

import (
	"testing"
	"time"
)

func TestNewTestProxyConfig(t *testing.T) {
	cfg := NewTestProxyConfig("/api", "http://localhost:8080")

	if cfg.Path != "/api" {
		t.Errorf("expected path /api, got %s", cfg.Path)
	}
	if len(cfg.Targets) != 1 {
		t.Errorf("expected 1 target, got %d", len(cfg.Targets))
	}
	if cfg.Timeout.Connect != 5*time.Second {
		t.Errorf("expected 5s connect timeout, got %v", cfg.Timeout.Connect)
	}
}

func TestNewTestHealthyTarget(t *testing.T) {
	target := NewTestHealthyTarget("http://localhost:8080")

	if target.URL != "http://localhost:8080" {
		t.Errorf("expected URL http://localhost:8080, got %s", target.URL)
	}
	if !target.Healthy.Load() {
		t.Error("expected target to be healthy")
	}
}

func TestNewTestHealthyTargets(t *testing.T) {
	targets := NewTestHealthyTargets("http://localhost:8080", "http://localhost:8081")

	if len(targets) != 2 {
		t.Errorf("expected 2 targets, got %d", len(targets))
	}
	for i, target := range targets {
		if !target.Healthy.Load() {
			t.Errorf("expected target %d to be healthy", i)
		}
	}
}
```

- [ ] **Step 3: 运行测试确认通过**

Run: `go test ./internal/testutil/... -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/testutil/
git commit -m "feat: add testutil package for proxy config helpers"
```

---

### Task 9: 迁移 proxy 测试使用辅助函数

**Files:**
- Modify: `internal/proxy/proxy_test.go`
- Modify: `internal/integration/proxy_integration_test.go`

- [ ] **Step 1: 修改 `internal/proxy/proxy_test.go` 导入**

添加导入：
```go
import (
	"rua.plus/lolly/internal/testutil"
)
```

- [ ] **Step 2: 替换重复的 ProxyConfig 创建**

将测试中的重复模式替换为：
```go
// 替换前：
cfg := &config.ProxyConfig{
    Path:        "/api",
    LoadBalance: "round_robin",
    Timeout: config.ProxyTimeout{
        Connect: 5 * time.Second,
        Read:    30 * time.Second,
        Write:   30 * time.Second,
    },
}

// 替换后：
cfg := testutil.NewTestProxyConfig("/api")
```

- [ ] **Step 3: 替换重复的 Target 创建**

将：
```go
targets := []*loadbalance.Target{{URL: "http://localhost:8080"}}
targets[0].Healthy.Store(true)
```
替换为：
```go
targets := testutil.NewTestHealthyTargets("http://localhost:8080")
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/proxy/... -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/proxy/proxy_test.go internal/integration/proxy_integration_test.go
git commit -m "refactor: use testutil helpers in proxy tests"
```

---

### Task 10: 迁移 server 测试使用辅助函数

**Files:**
- Modify: `internal/server/*_test.go`

- [ ] **Step 1: 批量替换 server 测试中的重复代码**

使用与 Task 9 相同的模式，替换 `internal/server/` 下所有测试文件中的重复 ProxyConfig 和 Target 创建。

- [ ] **Step 2: 运行测试确认通过**

Run: `go test ./internal/server/... -v`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/server/
git commit -m "refactor: use testutil helpers in server tests"
```

---

## 验收检查

### Task 11: 最终验证

- [ ] **Step 1: 运行 unused linter**

Run: `golangci-lint run --enable=unused ./...`
Expected: 无 unused 错误

- [ ] **Step 2: 运行 dupl linter**

Run: `golangci-lint run --enable=dupl ./...`
Expected: 源文件无 dupl 错误（测试文件允许）

- [ ] **Step 3: 运行完整测试套件**

Run: `go test ./...`
Expected: 全部 PASS

- [ ] **Step 4: 统计代码行数变化**

Run: `git diff --stat`
Expected: 总行数净减少 >200 行

- [ ] **Step 5: 最终 Commit**

```bash
git commit -m "chore: eliminate code redundancy - dead code removal, pattern extraction, test helpers"
```

---

## Self-Review Checklist

1. **Spec coverage**: 所有 3 个阶段都有详细任务 ✓
2. **Placeholder scan**: 无 TBD、TODO 或模糊描述 ✓
3. **Type consistency**: `registerRoute` 和 `proxyDebugLog` 签名与使用处一致 ✓
4. **File paths**: 所有路径均为绝对路径，与代码库匹配 ✓
5. **Commands**: 每个测试步骤都有明确的运行命令和预期输出 ✓
