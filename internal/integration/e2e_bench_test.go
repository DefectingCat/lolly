// Package integration 提供端到端集成基准测试。
//
// 该文件测试完整请求路径的吞吐量，涵盖静态文件、代理转发、
// 中间件链、Lua 脚本、HTTPS 和多路由等场景。
//
// 测试策略：
//   - 使用 fasthttputil.NewInmemoryListener 创建内存服务器
//   - 手动构建处理链模拟服务器的完整请求路径
//   - 使用 b.RunParallel 测试并发吞吐量
//   - 包含预热逻辑确保缓存命中场景测试
//
// 作者：xfy
package integration

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/valyala/fasthttp"
	"github.com/valyala/fasthttp/fasthttputil"
	"rua.plus/lolly/internal/benchmark/tools"
	"rua.plus/lolly/internal/cache"
	"rua.plus/lolly/internal/config"
	"rua.plus/lolly/internal/handler"
	"rua.plus/lolly/internal/loadbalance"
	"rua.plus/lolly/internal/lua"
	mw "rua.plus/lolly/internal/middleware"
	"rua.plus/lolly/internal/middleware/accesslog"
	"rua.plus/lolly/internal/middleware/compression"
	"rua.plus/lolly/internal/middleware/rewrite"
	"rua.plus/lolly/internal/middleware/security"
	"rua.plus/lolly/internal/proxy"
)

// generateTestCert 生成自签名测试证书（服务器认证）。
//
// 返回值：
//   - certPEM: PEM 编码的证书
//   - keyPEM: PEM 编码的私钥
func generateTestCert(b *testing.B) ([]byte, []byte) {
	b.Helper()

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		b.Fatalf("生成私钥失败: %v", err)
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Lolly Test"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost"},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		b.Fatalf("创建证书失败: %v", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})

	keyDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		b.Fatalf("编码私钥失败: %v", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: keyDER,
	})

	return certPEM, keyPEM
}

// setupTestStaticDir 创建测试静态文件目录。
//
// 返回值：
//   - dir: 临时目录路径
//   - cleanup: 清理函数
func setupTestStaticDir(b *testing.B) (string, func()) {
	b.Helper()

	dir, err := os.MkdirTemp("", "e2e_static_*")
	if err != nil {
		b.Fatalf("创建临时目录失败: %v", err)
	}

	// 创建测试文件
	testFiles := map[string][]byte{
		"index.html":    []byte("<html><body><h1>Hello from Lolly</h1></body></html>"),
		"small.css":     make([]byte, 512),   // 512B
		"medium.json":   make([]byte, 10240), // 10KB
		"assets/app.js": make([]byte, 5120),  // 5KB
	}

	for path, content := range testFiles {
		fullPath := filepath.Join(dir, path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			b.Fatalf("创建目录失败: %v", err)
		}
		if err := os.WriteFile(fullPath, content, 0o644); err != nil {
			b.Fatalf("写入文件失败: %v", err)
		}
	}

	cleanup := func() {
		_ = os.RemoveAll(dir)
	}

	return dir, cleanup
}

// setupNetworkBackend 设置真实网络后端服务器。
//
// 使用真实 TCP 监听器，确保代理转发测试走真实网络路径。
//
// 返回值：
//   - addr: 监听地址
//   - cleanup: 清理函数
func setupNetworkBackend(b *testing.B, statusCode int, body []byte) (string, func()) {
	b.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		b.Fatalf("创建监听器失败: %v", err)
	}

	srv := &fasthttp.Server{
		Handler: func(ctx *fasthttp.RequestCtx) {
			ctx.SetStatusCode(statusCode)
			_, _ = ctx.Write(body)
		},
	}

	go func() {
		_ = srv.Serve(ln)
	}()

	addr := ln.Addr().String()
	cleanup := func() {
		_ = srv.Shutdown()
		_ = ln.Close()
	}

	return addr, cleanup
}

// buildMiddlewareChainForBenchmark 构建基准测试用中间件链。
//
// 按服务器相同顺序构建：AccessLog -> Rewrite -> Compression -> SecurityHeaders。
//
// 参数：
//   - enableCompression: 是否启用压缩中间件
//   - enableSecurityHeaders: 是否启用安全头中间件
//
// 返回值：
//   - *mw.Chain: 构建完成的中间件链
func buildMiddlewareChainForBenchmark(enableCompression, enableSecurityHeaders bool) *mw.Chain {
	var middlewares []mw.Middleware

	// 1. AccessLog (始终添加)
	accessLog := accesslog.New(&config.LoggingConfig{})
	middlewares = append(middlewares, accessLog)

	// 2. Rewrite
	rewriteRules := []config.RewriteRule{
		{Pattern: "^/redirect/(.*)", Replacement: "/new/$1", Flag: "last"},
	}
	rw, _ := rewrite.New(rewriteRules)
	middlewares = append(middlewares, rw)

	// 3. Compression
	if enableCompression {
		comp, _ := compression.New(&config.CompressionConfig{
			Type:  "gzip",
			Level: 6,
			Types: []string{"text/html", "text/css", "application/json", "application/javascript"},
		})
		middlewares = append(middlewares, comp)
	}

	// 4. SecurityHeaders
	if enableSecurityHeaders {
		headers := security.NewHeadersWithHSTS(&config.SecurityHeaders{
			XFrameOptions:       "DENY",
			XContentTypeOptions: "nosniff",
		}, &config.HSTSConfig{})
		middlewares = append(middlewares, headers)
	}

	return mw.NewChain(middlewares...)
}

// createTestProxy 创建测试代理实例。
//
// 参数：
//   - backendAddr: 后端地址
//   - path: 代理路径
//
// 返回值：
//   - *proxy.Proxy: 代理实例
//   - error: 创建错误
func createTestProxy(backendAddr, path string) (*proxy.Proxy, error) {
	cfg := &config.ProxyConfig{
		Path:        path,
		LoadBalance: "round_robin",
		Timeout: config.ProxyTimeout{
			Connect: 5 * time.Second,
			Read:    30 * time.Second,
			Write:   30 * time.Second,
		},
	}

	targets := []*loadbalance.Target{
		{URL: "http://" + backendAddr},
	}
	targets[0].Healthy.Store(true)

	return proxy.NewProxy(cfg, targets, nil, nil)
}

// warmupProxy 预热代理，确保连接池已建立。
//
// 发送若干预热请求，确保后续基准测试命中缓存的连接池。
//
// 参数：
//   - p: 代理实例
//   - path: 请求路径
//   - count: 预热请求数量
func warmupProxy(p *proxy.Proxy, path string, count int) {
	for i := 0; i < count; i++ {
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.SetRequestURI(path)
		ctx.Request.Header.SetMethod(fasthttp.MethodGet)
		p.ServeHTTP(ctx)
	}
}

// warmupStaticHandler 预热静态文件处理器，确保缓存已填充。
//
// 参数：
//   - h: 静态文件处理器
//   - paths: 预热路径列表
func warmupStaticHandler(h *handler.StaticHandler, paths []string) {
	for _, path := range paths {
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.SetRequestURI(path)
		h.Handle(ctx)
	}
}

// ============================================================
// BenchmarkE2EStaticFile - 静态文件完整请求路径
//
// 测试从接收请求到查找文件、缓存查找、发送响应的完整路径。
// 包含缓存未命中和缓存命中两种场景。
// ============================================================

// BenchmarkE2EStaticFile 基准测试静态文件完整请求路径（缓存未命中）。
func BenchmarkE2EStaticFile(b *testing.B) {
	dir, cleanup := setupTestStaticDir(b)
	defer cleanup()

	// 构建中间件链
	chain := buildMiddlewareChainForBenchmark(false, false)

	// 创建静态文件处理器
	staticHandler := handler.NewStaticHandler(dir, "/", []string{"index.html"}, true)

	// 创建路由器并注册静态路由
	router := handler.NewRouter()
	router.GET("/{filepath:*}", staticHandler.Handle)

	// 应用中间件
	finalHandler := chain.Apply(router.Handler())

	b.ResetTimer()
	b.ReportAllocs()

	paths := []string{"/small.css", "/medium.json", "/assets/app.js", "/index.html"}
	var counter uint64

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			idx := atomic.AddUint64(&counter, 1)
			ctx := &fasthttp.RequestCtx{}
			ctx.Request.SetRequestURI(paths[idx%uint64(len(paths))])
			ctx.Request.Header.SetMethod(fasthttp.MethodGet)
			finalHandler(ctx)
		}
	})
}

// BenchmarkE2EStaticFileCacheHit 基准测试静态文件缓存命中场景。
func BenchmarkE2EStaticFileCacheHit(b *testing.B) {
	dir, cleanup := setupTestStaticDir(b)
	defer cleanup()

	// 启用文件缓存
	fc := cache.NewFileCache(1000, 100*1024*1024, 0)
	staticHandler := handler.NewStaticHandler(dir, "/", []string{"index.html"}, true)
	staticHandler.SetFileCache(fc)
	staticHandler.SetCacheTTL(5 * time.Second)

	// 预热缓存
	warmupStaticHandler(staticHandler, []string{"/small.css", "/medium.json", "/assets/app.js", "/index.html"})

	// 构建中间件链和路由器
	chain := buildMiddlewareChainForBenchmark(false, false)
	router := handler.NewRouter()
	router.GET("/{filepath:*}", staticHandler.Handle)
	finalHandler := chain.Apply(router.Handler())

	b.ResetTimer()
	b.ReportAllocs()

	paths := []string{"/small.css", "/medium.json", "/assets/app.js", "/index.html"}
	var counter uint64

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			idx := atomic.AddUint64(&counter, 1)
			ctx := &fasthttp.RequestCtx{}
			ctx.Request.SetRequestURI(paths[idx%uint64(len(paths))])
			ctx.Request.Header.SetMethod(fasthttp.MethodGet)
			finalHandler(ctx)
		}
	})
}

// ============================================================
// BenchmarkE2EProxyForward - 代理转发完整请求路径
//
// 测试通过代理将请求转发到后端的完整路径，包括负载均衡、
// 连接池复用、请求头改写和响应转发。
// ============================================================

// BenchmarkE2EProxyForward 基准测试代理转发完整路径。
func BenchmarkE2EProxyForward(b *testing.B) {
	// 启动后端服务器
	responseBody := []byte(`{"status":"ok","message":"Hello from backend"}`)
	addr, cleanup := setupNetworkBackend(b, fasthttp.StatusOK, responseBody)
	defer cleanup()

	// 创建代理
	p, err := createTestProxy(addr, "/api")
	if err != nil {
		b.Fatalf("创建代理失败: %v", err)
	}

	// 预热连接池
	warmupProxy(p, "/api/test", 10)

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			ctx := &fasthttp.RequestCtx{}
			ctx.Request.SetRequestURI("/api/test")
			ctx.Request.Header.SetMethod(fasthttp.MethodGet)
			ctx.Request.Header.Set("X-Forwarded-For", "192.168.1.100")
			ctx.Request.Header.Set("Host", "example.com")
			p.ServeHTTP(ctx)
		}
	})
}

// BenchmarkE2EProxyForwardLargeResponse 基准测试大响应代理转发。
func BenchmarkE2EProxyForwardLargeResponse(b *testing.B) {
	// 100KB 响应体
	largeBody := make([]byte, 100*1024)
	for i := range largeBody {
		largeBody[i] = byte('A' + i%26)
	}

	addr, cleanup := setupNetworkBackend(b, fasthttp.StatusOK, largeBody)
	defer cleanup()

	p, err := createTestProxy(addr, "/api")
	if err != nil {
		b.Fatalf("创建代理失败: %v", err)
	}

	warmupProxy(p, "/api/data", 5)

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			ctx := &fasthttp.RequestCtx{}
			ctx.Request.SetRequestURI("/api/data")
			ctx.Request.Header.SetMethod(fasthttp.MethodGet)
			p.ServeHTTP(ctx)
		}
	})
}

// ============================================================
// BenchmarkE2EWithMiddleware - 带中间件链的完整路径
//
// 测试包含完整中间件链（AccessLog、Rewrite、Compression、
// SecurityHeaders）的请求处理路径。
// ============================================================

// BenchmarkE2EWithMiddleware 基准测试带完整中间件链的请求路径。
func BenchmarkE2EWithMiddleware(b *testing.B) {
	// 启动后端
	responseBody := []byte(`{"status":"ok","data":"middleware test"}`)
	addr, cleanup := setupNetworkBackend(b, fasthttp.StatusOK, responseBody)
	defer cleanup()

	// 创建代理
	p, err := createTestProxy(addr, "/api")
	if err != nil {
		b.Fatalf("创建代理失败: %v", err)
	}
	warmupProxy(p, "/api/test", 5)

	// 构建完整中间件链
	chain := buildMiddlewareChainForBenchmark(true, true)

	// 创建路由器
	router := handler.NewRouter()
	router.GET("/api/{path:*}", p.ServeHTTP)

	// 应用中间件链
	finalHandler := chain.Apply(router.Handler())

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			ctx := &fasthttp.RequestCtx{}
			ctx.Request.SetRequestURI("/api/test")
			ctx.Request.Header.SetMethod(fasthttp.MethodGet)
			ctx.Request.Header.Set("Host", "example.com")
			ctx.Request.Header.Set("Accept-Encoding", "gzip, deflate")
			finalHandler(ctx)
		}
	})
}

// BenchmarkE2EWithMiddlewareNoCompression 基准测试不带压缩的中间件链。
func BenchmarkE2EWithMiddlewareNoCompression(b *testing.B) {
	responseBody := []byte(`{"status":"ok"}`)
	addr, cleanup := setupNetworkBackend(b, fasthttp.StatusOK, responseBody)
	defer cleanup()

	p, err := createTestProxy(addr, "/api")
	if err != nil {
		b.Fatalf("创建代理失败: %v", err)
	}
	warmupProxy(p, "/api/test", 5)

	// 只启用安全头，不启用压缩
	chain := buildMiddlewareChainForBenchmark(false, true)

	router := handler.NewRouter()
	router.GET("/api/{path:*}", p.ServeHTTP)
	finalHandler := chain.Apply(router.Handler())

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			ctx := &fasthttp.RequestCtx{}
			ctx.Request.SetRequestURI("/api/test")
			ctx.Request.Header.SetMethod(fasthttp.MethodGet)
			ctx.Request.Header.Set("Host", "example.com")
			finalHandler(ctx)
		}
	})
}

// ============================================================
// BenchmarkE2ELuaScript - 带 Lua 脚本执行的完整路径
//
// 测试包含 Lua 中间件的请求处理路径，包括 Lua 引擎初始化、
// 脚本执行和结果处理。
// ============================================================

// BenchmarkE2ELuaScript 基准测试带 Lua 脚本执行的完整路径。
func BenchmarkE2ELuaScript(b *testing.B) {
	// 启动后端
	responseBody := []byte(`{"status":"ok","lua":"executed"}`)
	addr, cleanup := setupNetworkBackend(b, fasthttp.StatusOK, responseBody)
	defer cleanup()

	p, err := createTestProxy(addr, "/api")
	if err != nil {
		b.Fatalf("创建代理失败: %v", err)
	}
	warmupProxy(p, "/api/test", 5)

	// 创建 Lua 引擎
	engine, err := lua.NewEngine(lua.DefaultConfig())
	if err != nil {
		b.Fatalf("创建 Lua 引擎失败: %v", err)
	}
	defer engine.Close()

	// 创建简单的 Lua 脚本
	tmpDir := b.TempDir()
	scriptPath := filepath.Join(tmpDir, "access.lua")
	scriptContent := `-- access phase: add custom header
ngx.header["X-Lua-Processed"] = "true"`
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0o644); err != nil {
		b.Fatalf("写入 Lua 脚本失败: %v", err)
	}

	// 创建 Lua 中间件
	luaMW, err := lua.NewLuaMiddleware(engine, lua.LuaMiddlewareConfig{
		ScriptPath: scriptPath,
		Phase:      lua.PhaseAccess,
	})
	if err != nil {
		b.Fatalf("创建 Lua 中间件失败: %v", err)
	}

	// 使用 MultiPhaseLuaMiddleware 组合多个 Lua 阶段
	multiLua := lua.NewMultiPhaseLuaMiddleware(engine, "e2e-lua")

	// 添加 content phase 脚本
	contentScript := filepath.Join(tmpDir, "content.lua")
	if err := os.WriteFile(contentScript, []byte(`-- content phase: noop`), 0o644); err != nil {
		b.Fatalf("写入 Lua 脚本失败: %v", err)
	}
	if err := multiLua.AddPhase(lua.PhaseContent, contentScript, 10*time.Second); err != nil {
		b.Fatalf("添加 Lua 阶段失败: %v", err)
	}

	// 组合中间件链：AccessLog -> Lua -> Proxy
	var middlewares []mw.Middleware
	middlewares = append(middlewares, accesslog.New(&config.LoggingConfig{}))
	middlewares = append(middlewares, luaMW)

	chain := mw.NewChain(middlewares...)

	router := handler.NewRouter()
	router.GET("/api/{path:*}", p.ServeHTTP)

	wrappedByLua := luaMW.Process(router.Handler())
	finalHandler := chain.Apply(wrappedByLua)

	// 预热 Lua 引擎（字节码缓存）
	for i := 0; i < 5; i++ {
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.SetRequestURI("/api/test")
		ctx.Request.Header.SetMethod(fasthttp.MethodGet)
		finalHandler(ctx)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.SetRequestURI("/api/test")
		ctx.Request.Header.SetMethod(fasthttp.MethodGet)
		ctx.Request.Header.Set("Host", "example.com")
		finalHandler(ctx)
	}
}

// BenchmarkE2EMultiLuaPhase 基准测试多 Lua 阶段执行路径。
func BenchmarkE2EMultiLuaPhase(b *testing.B) {
	responseBody := []byte(`{"status":"ok","multi_lua":"executed"}`)
	addr, cleanup := setupNetworkBackend(b, fasthttp.StatusOK, responseBody)
	defer cleanup()

	p, err := createTestProxy(addr, "/api")
	if err != nil {
		b.Fatalf("创建代理失败: %v", err)
	}
	warmupProxy(p, "/api/test", 5)

	// 创建 Lua 引擎
	engine, err := lua.NewEngine(lua.DefaultConfig())
	if err != nil {
		b.Fatalf("创建 Lua 引擎失败: %v", err)
	}
	defer engine.Close()

	tmpDir := b.TempDir()

	// 创建多阶段 Lua 中间件
	multiLua := lua.NewMultiPhaseLuaMiddleware(engine, "multi-phase")

	// 添加 rewrite phase
	rewriteScript := filepath.Join(tmpDir, "rewrite.lua")
	if err := os.WriteFile(rewriteScript, []byte(`-- rewrite phase: modify path`), 0o644); err != nil {
		b.Fatalf("写入 Lua 脚本失败: %v", err)
	}
	if err := multiLua.AddPhase(lua.PhaseRewrite, rewriteScript, 10*time.Second); err != nil {
		b.Fatalf("添加 Lua 阶段失败: %v", err)
	}

	// 添加 access phase
	accessScript := filepath.Join(tmpDir, "access2.lua")
	if err := os.WriteFile(accessScript, []byte(`-- access phase: add header`), 0o644); err != nil {
		b.Fatalf("写入 Lua 脚本失败: %v", err)
	}
	if err := multiLua.AddPhase(lua.PhaseAccess, accessScript, 10*time.Second); err != nil {
		b.Fatalf("添加 Lua 阶段失败: %v", err)
	}

	// 构建中间件链
	router := handler.NewRouter()
	router.GET("/api/{path:*}", p.ServeHTTP)
	wrappedByLua := multiLua.Process(router.Handler())

	baseChain := buildMiddlewareChainForBenchmark(false, false)
	finalHandler := baseChain.Apply(wrappedByLua)

	// 预热
	for i := 0; i < 5; i++ {
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.SetRequestURI("/api/test")
		ctx.Request.Header.SetMethod(fasthttp.MethodGet)
		finalHandler(ctx)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.SetRequestURI("/api/test")
		ctx.Request.Header.SetMethod(fasthttp.MethodGet)
		ctx.Request.Header.Set("Host", "example.com")
		finalHandler(ctx)
	}
}

// ============================================================
// BenchmarkE2EHTTPS - HTTPS 完整请求路径
//
// 测试 TLS 握手和 HTTPS 请求处理的完整路径。
// 使用内存监听器模拟 HTTPS 连接。
// ============================================================

// BenchmarkE2EHTTPS 基准测试 HTTPS 完整请求路径。
func BenchmarkE2EHTTPS(b *testing.B) {
	// 生成测试证书
	certPEM, keyPEM := generateTestCert(b)

	// 写入临时文件
	tmpDir := b.TempDir()
	certPath := filepath.Join(tmpDir, "server.crt")
	keyPath := filepath.Join(tmpDir, "server.key")
	if err := os.WriteFile(certPath, certPEM, 0o644); err != nil {
		b.Fatalf("写入证书文件失败: %v", err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		b.Fatalf("写入密钥文件失败: %v", err)
	}

	// 加载 TLS 证书
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		b.Fatalf("加载 TLS 证书失败: %v", err)
	}

	// 启动后端
	responseBody := []byte(`{"status":"ok","tls":"verified"}`)
	addr, cleanup := setupNetworkBackend(b, fasthttp.StatusOK, responseBody)
	defer cleanup()

	// 创建代理
	p, err := createTestProxy(addr, "/api")
	if err != nil {
		b.Fatalf("创建代理失败: %v", err)
	}
	warmupProxy(p, "/api/test", 5)

	// 创建路由器
	router := handler.NewRouter()
	router.GET("/api/{path:*}", p.ServeHTTP)

	// 构建中间件链
	chain := buildMiddlewareChainForBenchmark(false, true)
	finalHandler := chain.Apply(router.Handler())

	// 创建 TLS 监听器
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		b.Fatalf("创建监听器失败: %v", err)
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}
	tlsLn := tls.NewListener(ln, tlsConfig)

	tlsSrv := &fasthttp.Server{
		Name:    "lolly",
		Handler: finalHandler,
	}
	go func() {
		_ = tlsSrv.Serve(tlsLn)
	}()

	tlsAddr := ln.Addr().String()

	// 创建 TLS 客户端
	client := &fasthttp.HostClient{
		Addr:      tlsAddr,
		IsTLS:     true,
		TLSConfig: &tls.Config{InsecureSkipVerify: true},
		MaxConns:  1000,
	}

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		req := fasthttp.AcquireRequest()
		resp := fasthttp.AcquireResponse()
		defer fasthttp.ReleaseRequest(req)
		defer fasthttp.ReleaseResponse(resp)

		for pb.Next() {
			req.SetRequestURI("https://" + tlsAddr + "/api/test")
			req.Header.SetMethod(fasthttp.MethodGet)
			req.Header.Set("Host", "example.com")
			_ = client.Do(req, resp)
			resp.Reset()
		}
	})
}

// BenchmarkE2ETLSHandshake 基准测试 TLS 握手开销。
func BenchmarkE2ETLSHandshake(b *testing.B) {
	certPEM, keyPEM := generateTestCert(b)

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		b.Fatalf("加载 TLS 证书失败: %v", err)
	}

	responseBody := []byte("ok")
	_, cleanup := setupNetworkBackend(b, fasthttp.StatusOK, responseBody)
	defer cleanup()

	// 创建 TLS 监听器
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		b.Fatalf("创建监听器失败: %v", err)
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}
	tlsLn := tls.NewListener(ln, tlsConfig)

	srv := &fasthttp.Server{
		Handler: func(ctx *fasthttp.RequestCtx) {
			ctx.SetStatusCode(fasthttp.StatusOK)
			_, _ = ctx.Write(responseBody)
		},
	}
	go func() {
		_ = srv.Serve(tlsLn)
	}()

	tlsAddr := ln.Addr().String()

	b.ResetTimer()
	b.ReportAllocs()

	// 每次迭代都创建新连接以模拟完整握手
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			// 创建新客户端（新连接 = 新 TLS 握手）
			client := &fasthttp.HostClient{
				Addr:      tlsAddr,
				IsTLS:     true,
				TLSConfig: &tls.Config{InsecureSkipVerify: true},
				MaxConns:  1,
			}

			req := fasthttp.AcquireRequest()
			resp := fasthttp.AcquireResponse()

			req.SetRequestURI("https://" + tlsAddr + "/")
			req.Header.SetMethod(fasthttp.MethodGet)
			_ = client.Do(req, resp)

			fasthttp.ReleaseRequest(req)
			fasthttp.ReleaseResponse(resp)
			client.CloseIdleConnections()
		}
	})
}

// ============================================================
// BenchmarkE2EMultipleRoutes - 多路由匹配性能
//
// 测试路由器在多条路由规则下的匹配性能，
// 模拟真实服务器有多条代理和静态文件路径的场景。
// ============================================================

// BenchmarkE2EMultipleRoutes 基准测试多路由匹配性能。
func BenchmarkE2EMultipleRoutes(b *testing.B) {
	// 启动多个后端
	addr1, cleanup1 := setupNetworkBackend(b, fasthttp.StatusOK, []byte(`{"service":"api-v1"}`))
	defer cleanup1()
	addr2, cleanup2 := setupNetworkBackend(b, fasthttp.StatusOK, []byte(`{"service":"api-v2"}`))
	defer cleanup2()
	addr3, cleanup3 := setupNetworkBackend(b, fasthttp.StatusOK, []byte(`{"service":"admin"}`))
	defer cleanup3()

	// 创建多个代理
	p1, _ := createTestProxy(addr1, "/api/v1")
	p2, _ := createTestProxy(addr2, "/api/v2")
	p3, _ := createTestProxy(addr3, "/admin")

	warmupProxy(p1, "/api/v1/test", 3)
	warmupProxy(p2, "/api/v2/test", 3)
	warmupProxy(p3, "/admin/test", 3)

	// 创建静态文件目录
	staticDir, staticCleanup := setupTestStaticDir(b)
	defer staticCleanup()

	staticHandler := handler.NewStaticHandler(staticDir, "/static/", []string{"index.html"}, true)
	warmupStaticHandler(staticHandler, []string{"/static/small.css", "/static/medium.json"})

	// 构建路由器，注册多条路由
	router := handler.NewRouter()
	router.GET("/api/v1/{path:*}", p1.ServeHTTP)
	router.GET("/api/v2/{path:*}", p2.ServeHTTP)
	router.POST("/api/v2/{path:*}", p2.ServeHTTP)
	router.GET("/admin/{path:*}", p3.ServeHTTP)
	router.GET("/static/{filepath:*}", staticHandler.Handle)
	router.GET("/health", func(ctx *fasthttp.RequestCtx) {
		ctx.SetBodyString(`{"status":"healthy"}`)
	})
	router.GET("/", func(ctx *fasthttp.RequestCtx) {
		ctx.SetBodyString(`<html><body>Welcome</body></html>`)
	})

	// 应用中间件链
	chain := buildMiddlewareChainForBenchmark(false, false)
	finalHandler := chain.Apply(router.Handler())

	// 预热路由匹配
	testPaths := []string{"/api/v1/test", "/api/v2/data", "/admin/dashboard", "/static/small.css", "/health", "/"}
	for _, path := range testPaths {
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.SetRequestURI(path)
		ctx.Request.Header.SetMethod(fasthttp.MethodGet)
		finalHandler(ctx)
	}

	b.ResetTimer()
	b.ReportAllocs()

	// 使用原子计数器轮询不同路径
	var counter uint64
	paths := []string{
		"/api/v1/users",
		"/api/v2/items",
		"/api/v2/items/123",
		"/admin/settings",
		"/static/small.css",
		"/static/medium.json",
		"/static/assets/app.js",
		"/health",
		"/",
	}

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			idx := atomic.AddUint64(&counter, 1)
			ctx := &fasthttp.RequestCtx{}
			ctx.Request.SetRequestURI(paths[idx%uint64(len(paths))])
			ctx.Request.Header.SetMethod(fasthttp.MethodGet)
			ctx.Request.Header.Set("Host", "example.com")
			finalHandler(ctx)
		}
	})
}

// BenchmarkE2EMultipleRoutesWithMiddleware 基准测试多路由+完整中间件链。
func BenchmarkE2EMultipleRoutesWithMiddleware(b *testing.B) {
	addr1, cleanup1 := setupNetworkBackend(b, fasthttp.StatusOK, []byte(`{"service":"api"}`))
	defer cleanup1()
	addr2, cleanup2 := setupNetworkBackend(b, fasthttp.StatusOK, []byte(`{"service":"graphql"}`))
	defer cleanup2()

	p1, _ := createTestProxy(addr1, "/api")
	p2, _ := createTestProxy(addr2, "/graphql")

	warmupProxy(p1, "/api/test", 3)
	warmupProxy(p2, "/graphql/query", 3)

	staticDir, staticCleanup := setupTestStaticDir(b)
	defer staticCleanup()

	fc := cache.NewFileCache(500, 50*1024*1024, 0)
	staticHandler := handler.NewStaticHandler(staticDir, "/", []string{"index.html"}, true)
	staticHandler.SetFileCache(fc)
	staticHandler.SetCacheTTL(5 * time.Second)
	warmupStaticHandler(staticHandler, []string{"/small.css", "/index.html"})

	router := handler.NewRouter()
	router.GET("/api/{path:*}", p1.ServeHTTP)
	router.POST("/api/{path:*}", p1.ServeHTTP)
	router.GET("/graphql/{path:*}", p2.ServeHTTP)
	router.POST("/graphql/{path:*}", p2.ServeHTTP)
	router.GET("/{filepath:*}", staticHandler.Handle)

	// 完整中间件链
	chain := buildMiddlewareChainForBenchmark(true, true)
	finalHandler := chain.Apply(router.Handler())

	// 预热
	paths := []string{"/api/test", "/graphql/query", "/small.css", "/index.html"}
	for _, path := range paths {
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.SetRequestURI(path)
		ctx.Request.Header.SetMethod(fasthttp.MethodGet)
		ctx.Request.Header.Set("Accept-Encoding", "gzip")
		finalHandler(ctx)
	}

	b.ResetTimer()
	b.ReportAllocs()

	var counter uint64
	testPaths := []string{
		"/api/users",
		"/api/users/42",
		"/graphql/query",
		"/small.css",
		"/medium.json",
		"/index.html",
		"/assets/app.js",
	}

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			idx := atomic.AddUint64(&counter, 1)
			ctx := &fasthttp.RequestCtx{}
			ctx.Request.SetRequestURI(testPaths[idx%uint64(len(testPaths))])
			ctx.Request.Header.SetMethod(fasthttp.MethodGet)
			ctx.Request.Header.Set("Host", "example.com")
			ctx.Request.Header.Set("Accept-Encoding", "gzip, deflate")
			finalHandler(ctx)
		}
	})
}

// ============================================================
// BenchmarkE2EProxyWithCache - 带代理缓存的完整路径
//
// 测试代理缓存命中/未命中场景下的吞吐量。
// ============================================================

// BenchmarkE2EProxyWithCache 基准测试代理缓存未命中场景。
func BenchmarkE2EProxyWithCache(b *testing.B) {
	responseBody := []byte(`{"cached":true,"data":"proxy cache test"}`)
	addr, cleanup := setupNetworkBackend(b, fasthttp.StatusOK, responseBody)
	defer cleanup()

	cfg := &config.ProxyConfig{
		Path:        "/cached",
		LoadBalance: "round_robin",
		Timeout: config.ProxyTimeout{
			Connect: 5 * time.Second,
			Read:    30 * time.Second,
			Write:   30 * time.Second,
		},
		Cache: config.ProxyCacheConfig{
			Enabled: true,
			MaxAge:  5 * time.Minute,
		},
	}

	targets := []*loadbalance.Target{{URL: "http://" + addr}}
	targets[0].Healthy.Store(true)

	p, err := proxy.NewProxy(cfg, targets, nil, nil)
	if err != nil {
		b.Fatalf("创建代理失败: %v", err)
	}

	warmupProxy(p, "/cached/test", 5)

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			ctx := &fasthttp.RequestCtx{}
			ctx.Request.SetRequestURI("/cached/item")
			ctx.Request.Header.SetMethod(fasthttp.MethodGet)
			p.ServeHTTP(ctx)
		}
	})
}

// BenchmarkE2EProxyCacheHit 基准测试代理缓存命中场景。
func BenchmarkE2EProxyCacheHit(b *testing.B) {
	responseBody := []byte(`{"cached":true,"data":"cache hit test"}`)
	addr, cleanup := setupNetworkBackend(b, fasthttp.StatusOK, responseBody)
	defer cleanup()

	cfg := &config.ProxyConfig{
		Path:        "/cached",
		LoadBalance: "round_robin",
		Timeout: config.ProxyTimeout{
			Connect: 5 * time.Second,
			Read:    30 * time.Second,
			Write:   30 * time.Second,
		},
		Cache: config.ProxyCacheConfig{
			Enabled: true,
			MaxAge:  5 * time.Minute,
		},
	}

	targets := []*loadbalance.Target{{URL: "http://" + addr}}
	targets[0].Healthy.Store(true)

	p, err := proxy.NewProxy(cfg, targets, nil, nil)
	if err != nil {
		b.Fatalf("创建代理失败: %v", err)
	}

	// 预热使缓存命中
	warmupProxy(p, "/cached/item", 10)

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			ctx := &fasthttp.RequestCtx{}
			ctx.Request.SetRequestURI("/cached/item")
			ctx.Request.Header.SetMethod(fasthttp.MethodGet)
			p.ServeHTTP(ctx)
		}
	})
}

// ============================================================
// BenchmarkE2EInmemoryServer - 纯内存服务器完整路径
//
// 使用 fasthttputil.NewInmemoryListener 创建完全在内存中运行的
// 服务器，消除网络开销，测试纯处理逻辑的吞吐量。
// ============================================================

// BenchmarkE2EInmemoryServer 基准测试纯内存服务器的完整请求路径。
func BenchmarkE2EInmemoryServer(b *testing.B) {
	// 启动内存后端
	backendLn := fasthttputil.NewInmemoryListener()
	backendSrv := &fasthttp.Server{
		Handler: func(ctx *fasthttp.RequestCtx) {
			ctx.SetStatusCode(fasthttp.StatusOK)
			_, _ = ctx.Write([]byte(`{"status":"ok","backend":"inmemory"}`))
		},
	}
	go func() {
		_ = backendSrv.Serve(backendLn)
	}()

	// 后端地址（使用内存监听器的地址）
	backendAddr := backendLn.Addr().String()

	// 创建代理（通过内存监听器传递连接）
	p, err := createTestProxy(backendAddr, "/api")
	if err != nil {
		b.Fatalf("创建代理失败: %v", err)
	}

	// 创建静态文件目录
	staticDir, staticCleanup := setupTestStaticDir(b)
	defer staticCleanup()
	staticHandler := handler.NewStaticHandler(staticDir, "/static/", []string{"index.html"}, true)

	// 构建路由器
	router := handler.NewRouter()
	router.GET("/api/{path:*}", p.ServeHTTP)
	router.GET("/static/{filepath:*}", staticHandler.Handle)
	router.GET("/health", func(ctx *fasthttp.RequestCtx) {
		ctx.SetBodyString(`{"status":"healthy"}`)
	})

	// 中间件链
	chain := buildMiddlewareChainForBenchmark(false, false)
	finalHandler := chain.Apply(router.Handler())

	// 创建内存服务器
	serverLn := fasthttputil.NewInmemoryListener()
	fastSrv := &fasthttp.Server{
		Name:    "lolly",
		Handler: finalHandler,
	}
	go func() {
		_ = fastSrv.Serve(serverLn)
	}()

	// 创建内存客户端
	client := &fasthttp.HostClient{
		Dial: func(addr string) (net.Conn, error) {
			return serverLn.Dial()
		},
		MaxConns: 1000,
	}

	// 预热
	warmupPaths := []string{"/api/test", "/static/small.css", "/health"}
	for _, path := range warmupPaths {
		req := fasthttp.AcquireRequest()
		resp := fasthttp.AcquireResponse()
		req.SetRequestURI("http://localhost" + path)
		req.Header.SetMethod(fasthttp.MethodGet)
		_ = client.Do(req, resp)
		fasthttp.ReleaseRequest(req)
		fasthttp.ReleaseResponse(resp)
	}

	b.ResetTimer()
	b.ReportAllocs()

	var counter uint64
	paths := []string{"/api/test", "/static/small.css", "/static/medium.json", "/health", "/api/data"}

	b.RunParallel(func(pb *testing.PB) {
		req := fasthttp.AcquireRequest()
		resp := fasthttp.AcquireResponse()
		defer fasthttp.ReleaseRequest(req)
		defer fasthttp.ReleaseResponse(resp)

		for pb.Next() {
			idx := atomic.AddUint64(&counter, 1)
			req.SetRequestURI("http://localhost" + paths[idx%uint64(len(paths))])
			req.Header.SetMethod(fasthttp.MethodGet)
			req.Header.Set("Host", "example.com")
			_ = client.Do(req, resp)
			resp.ResetBody()
		}
	})
}

// BenchmarkE2EInmemoryServerParallel 基准测试内存服务器并发吞吐量。
//
// 使用 tools 包的 MockBackend 工具模拟后端。
func BenchmarkE2EInmemoryServerParallel(b *testing.B) {
	// 使用 tools 包的 mock 后端
	addr, cleanup := tools.SimpleMockBackend(fasthttp.StatusOK, []byte(`{"mock":"tools"}`))
	defer cleanup()

	time.Sleep(10 * time.Millisecond) // 等待后端启动

	p, err := createTestProxy(addr, "/api")
	if err != nil {
		b.Fatalf("创建代理失败: %v", err)
	}
	warmupProxy(p, "/api/test", 10)

	// 中间件链
	chain := buildMiddlewareChainForBenchmark(true, true)

	router := handler.NewRouter()
	router.GET("/api/{path:*}", p.ServeHTTP)
	router.GET("/health", func(ctx *fasthttp.RequestCtx) {
		ctx.SetBodyString("ok")
	})

	finalHandler := chain.Apply(router.Handler())

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			ctx := &fasthttp.RequestCtx{}
			ctx.Request.SetRequestURI("/api/test")
			ctx.Request.Header.SetMethod(fasthttp.MethodGet)
			ctx.Request.Header.Set("Host", "example.com")
			ctx.Request.Header.Set("Accept-Encoding", "gzip")
			finalHandler(ctx)
		}
	})
}

// ============================================================
// BenchmarkE2EStaticWithCompression - 静态文件+压缩完整路径
//
// 测试静态文件服务配合响应压缩的完整处理路径。
// ============================================================

// BenchmarkE2EStaticWithCompression 基准测试静态文件+压缩完整路径。
func BenchmarkE2EStaticWithCompression(b *testing.B) {
	staticDir, staticCleanup := setupTestStaticDir(b)
	defer staticCleanup()

	// 创建可压缩的内容
	jsonContent := make([]byte, 20*1024) // 20KB JSON
	template := `{"key":"value","data":"repeat"}`
	for i := range jsonContent {
		jsonContent[i] = template[i%len(template)]
	}
	jsonPath := filepath.Join(staticDir, "compressible.json")
	if err := os.WriteFile(jsonPath, jsonContent, 0o644); err != nil {
		b.Fatalf("写入压缩测试文件失败: %v", err)
	}

	staticHandler := handler.NewStaticHandler(staticDir, "/", []string{"index.html"}, true)

	// 创建压缩中间件
	comp, err := compression.New(&config.CompressionConfig{
		Type:  "gzip",
		Level: 6,
		Types: []string{"application/json", "text/html", "text/css"},
	})
	if err != nil {
		b.Fatalf("创建压缩中间件失败: %v", err)
	}

	// 预热缓存
	warmupStaticHandler(staticHandler, []string{"/compressible.json"})

	chain := mw.NewChain(accesslog.New(&config.LoggingConfig{}), comp)
	router := handler.NewRouter()
	router.GET("/{filepath:*}", staticHandler.Handle)
	finalHandler := chain.Apply(router.Handler())

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			ctx := &fasthttp.RequestCtx{}
			ctx.Request.SetRequestURI("/compressible.json")
			ctx.Request.Header.SetMethod(fasthttp.MethodGet)
			ctx.Request.Header.Set("Accept-Encoding", "gzip, deflate, br")
			ctx.Request.Header.Set("Host", "example.com")
			finalHandler(ctx)
		}
	})
}

// ============================================================
// BenchmarkE2ERewriteMiddleware - URL重写中间件完整路径
//
// 测试 URL 重写中间件的完整处理路径。
// ============================================================

// BenchmarkE2ERewriteMiddleware 基准测试 URL 重写中间件完整路径。
func BenchmarkE2ERewriteMiddleware(b *testing.B) {
	responseBody := []byte(`{"status":"ok","rewritten":true}`)
	addr, cleanup := setupNetworkBackend(b, fasthttp.StatusOK, responseBody)
	defer cleanup()

	p, err := createTestProxy(addr, "/api")
	if err != nil {
		b.Fatalf("创建代理失败: %v", err)
	}
	warmupProxy(p, "/api/test", 5)

	// 创建 Rewrite 中间件
	rw, err := rewrite.New([]config.RewriteRule{
		{Pattern: "^/old-api/(.*)", Replacement: "/api/$1", Flag: "last"},
		{Pattern: "^/v1/(.*)", Replacement: "/api/$1", Flag: "last"},
		{Pattern: "^/legacy/(.*)", Replacement: "/api/v1/$1", Flag: "redirect"},
	})
	if err != nil {
		b.Fatalf("创建 rewrite 中间件失败: %v", err)
	}

	chain := mw.NewChain(accesslog.New(&config.LoggingConfig{}), rw)

	router := handler.NewRouter()
	router.GET("/api/{path:*}", p.ServeHTTP)
	router.GET("/api/v1/{path:*}", p.ServeHTTP)
	finalHandler := chain.Apply(router.Handler())

	b.ResetTimer()
	b.ReportAllocs()

	paths := []string{
		"/old-api/users",
		"/v1/items",
		"/api/direct",
		"/old-api/settings",
		"/v1/data/123",
	}
	var counter uint64

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			idx := atomic.AddUint64(&counter, 1)
			ctx := &fasthttp.RequestCtx{}
			ctx.Request.SetRequestURI(paths[idx%uint64(len(paths))])
			ctx.Request.Header.SetMethod(fasthttp.MethodGet)
			ctx.Request.Header.Set("Host", "example.com")
			finalHandler(ctx)
		}
	})
}

// BenchmarkE2EAccessControl 基准测试 IP 访问控制完整路径。
func BenchmarkE2EAccessControl(b *testing.B) {
	responseBody := []byte(`{"status":"ok"}`)
	addr, cleanup := setupNetworkBackend(b, fasthttp.StatusOK, responseBody)
	defer cleanup()

	p, err := createTestProxy(addr, "/api")
	if err != nil {
		b.Fatalf("创建代理失败: %v", err)
	}
	warmupProxy(p, "/api/test", 5)

	// 创建访问控制中间件（允许特定 IP 段）
	ac, err := security.NewAccessControl(&config.AccessConfig{
		Allow:   []string{"192.168.1.0/24", "10.0.0.0/8"},
		Deny:    []string{"192.168.1.100"},
		Default: "deny",
	})
	if err != nil {
		b.Fatalf("创建访问控制中间件失败: %v", err)
	}

	chain := mw.NewChain(accesslog.New(&config.LoggingConfig{}), ac)

	router := handler.NewRouter()
	router.GET("/api/{path:*}", p.ServeHTTP)
	finalHandler := chain.Apply(router.Handler())

	b.ResetTimer()
	b.ReportAllocs()

	// 混合允许和拒绝的 IP
	allowedIPs := []string{"192.168.1.50", "192.168.1.200", "10.0.0.1", "10.1.2.3"}
	deniedIPs := []string{"192.168.1.100", "172.16.0.1", "8.8.8.8"}
	allIPs := append(allowedIPs, deniedIPs...)
	var counter uint64

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			idx := atomic.AddUint64(&counter, 1)
			clientIP := allIPs[idx%uint64(len(allIPs))]
			ctx := &fasthttp.RequestCtx{}
			ctx.Request.SetRequestURI("/api/test")
			ctx.Request.Header.SetMethod(fasthttp.MethodGet)
			ctx.Request.Header.Set("X-Forwarded-For", clientIP)
			ctx.Request.Header.Set("X-Real-IP", clientIP)
			ctx.SetRemoteAddr(&net.TCPAddr{
				IP:   net.ParseIP(clientIP),
				Port: 12345,
			})
			finalHandler(ctx)
		}
	})
}

// BenchmarkE2ERateLimiter 基准测试速率限制完整路径。
func BenchmarkE2ERateLimiter(b *testing.B) {
	responseBody := []byte(`{"status":"ok"}`)
	addr, cleanup := setupNetworkBackend(b, fasthttp.StatusOK, responseBody)
	defer cleanup()

	p, err := createTestProxy(addr, "/api")
	if err != nil {
		b.Fatalf("创建代理失败: %v", err)
	}
	warmupProxy(p, "/api/test", 5)

	// 创建限流中间件
	rl, err := security.NewRateLimiter(&config.RateLimitConfig{
		RequestRate: 10000,
		Burst:       20000,
		Algorithm:   "token_bucket",
	})
	if err != nil {
		b.Fatalf("创建限流中间件失败: %v", err)
	}

	chain := mw.NewChain(accesslog.New(&config.LoggingConfig{}), rl)

	router := handler.NewRouter()
	router.GET("/api/{path:*}", p.ServeHTTP)
	finalHandler := chain.Apply(router.Handler())

	b.ResetTimer()
	b.ReportAllocs()

	var counter uint64

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			idx := atomic.AddUint64(&counter, 1)
			ctx := &fasthttp.RequestCtx{}
			ctx.Request.SetRequestURI("/api/test")
			ctx.Request.Header.SetMethod(fasthttp.MethodGet)
			ctx.Request.Header.Set("X-Forwarded-For", "192.168.1."+strconv.Itoa(int(idx%255)))
			ctx.SetRemoteAddr(&net.TCPAddr{
				IP:   net.ParseIP("192.168.1." + strconv.Itoa(int(idx%255))),
				Port: 12345,
			})
			finalHandler(ctx)
		}
	})
}

// BenchmarkE2EBasicAuth 基准测试 Basic 认证完整路径。
func BenchmarkE2EBasicAuth(b *testing.B) {
	responseBody := []byte(`{"status":"ok","authenticated":true}`)
	addr, cleanup := setupNetworkBackend(b, fasthttp.StatusOK, responseBody)
	defer cleanup()

	p, err := createTestProxy(addr, "/api")
	if err != nil {
		b.Fatalf("创建代理失败: %v", err)
	}
	warmupProxy(p, "/api/test", 5)

	// 创建 Basic Auth 中间件（使用 bcrypt 哈希）
	bcryptPassword, _ := security.HashPassword("testpass", security.HashBcrypt)
	auth, err := security.NewBasicAuth(&config.AuthConfig{
		RequireTLS: false,
		Users: []config.User{
			{Name: "admin", Password: bcryptPassword},
			{Name: "user", Password: bcryptPassword},
		},
	})
	if err != nil {
		b.Fatalf("创建认证中间件失败: %v", err)
	}

	chain := mw.NewChain(accesslog.New(&config.LoggingConfig{}), auth)

	router := handler.NewRouter()
	router.GET("/api/{path:*}", p.ServeHTTP)
	finalHandler := chain.Apply(router.Handler())

	b.ResetTimer()
	b.ReportAllocs()

	// 预热认证（缓存 bcrypt 结果）
	for i := 0; i < 3; i++ {
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.SetRequestURI("/api/test")
		ctx.Request.Header.SetMethod(fasthttp.MethodGet)
		ctx.Request.Header.Set("Authorization", "Basic YWRtaW46dGVzdHBhc3M=") // admin:testpass
		finalHandler(ctx)
	}

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			ctx := &fasthttp.RequestCtx{}
			ctx.Request.SetRequestURI("/api/test")
			ctx.Request.Header.SetMethod(fasthttp.MethodGet)
			ctx.Request.Header.Set("Authorization", "Basic YWRtaW46dGVzdHBhc3M=")
			finalHandler(ctx)
		}
	})
}
