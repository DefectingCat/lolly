// Package server 提供 HTTP 服务器的核心实现，支持单服务器和虚拟主机两种运行模式。
//
// 该文件包含服务器相关的核心逻辑，包括：
//   - HTTP 服务器的创建和生命周期管理
//   - 中间件链的构建和应用
//   - 代理路由的注册和处理
//   - 静态文件服务的集成
//   - Goroutine 池的性能优化
//
// 主要用途：
//
//	用于启动和管理 HTTP 服务器，处理客户端请求并转发到上游服务或静态文件。
//
// 注意事项：
//   - 服务器支持优雅关闭和热升级
//   - 所有公开方法均为并发安全
//   - 使用前需确保配置已正确加载
//
// 作者：xfy
package server

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/cache"
	"rua.plus/lolly/internal/config"
	"rua.plus/lolly/internal/handler"
	"rua.plus/lolly/internal/loadbalance"
	"rua.plus/lolly/internal/logging"
	"rua.plus/lolly/internal/lua"
	"rua.plus/lolly/internal/matcher"
	"rua.plus/lolly/internal/middleware"
	"rua.plus/lolly/internal/middleware/accesslog"
	"rua.plus/lolly/internal/middleware/bodylimit"
	"rua.plus/lolly/internal/middleware/compression"
	"rua.plus/lolly/internal/middleware/errorintercept"
	"rua.plus/lolly/internal/middleware/rewrite"
	"rua.plus/lolly/internal/middleware/security"
	"rua.plus/lolly/internal/proxy"
	"rua.plus/lolly/internal/resolver"
	"rua.plus/lolly/internal/ssl"
)

// Server HTTP 服务器，封装 fasthttp.Server 并提供中间件链和生命周期管理。
//
// 该结构体是服务器的核心实体，负责：
//   - 管理配置和 fasthttp.Server 实例
//   - 构建和应用中间件链
//   - 维护健康检查器和访问日志中间件
//   - 可选的 Goroutine 池和文件缓存
//
// 注意事项：
//   - 创建后需调用 Start 方法启动服务器
//   - 关闭时建议使用 GracefulStop 实现优雅关闭
type Server struct {
	handler             fasthttp.RequestHandler
	resolver            resolver.Resolver
	tlsManager          *ssl.TLSManager
	accessLogMiddleware *accesslog.AccessLog
	luaEngine           *lua.LuaEngine
	accessControl       *security.AccessControl
	errorPageManager    *handler.ErrorPageManager
	fileCache           *cache.FileCache
	pool                *GoroutinePool
	upgradeManager      *UpgradeManager
	config              *config.Config
	fastServer          *fasthttp.Server
	fastServers         []*fasthttp.Server // 多监听器模式使用
	proxies             []*proxy.Proxy
	listeners           []net.Listener
	healthCheckers      []*proxy.HealthChecker
	locationEngine      *matcher.LocationEngine
	startTime           time.Time
	connections         atomic.Int64
	requests            atomic.Int64
	bytesSent           atomic.Int64
	bytesReceived       atomic.Int64
	running             bool
}

// New 创建 HTTP 服务器实例。
//
// 根据提供的配置创建服务器对象，但不启动服务器。
// 服务器创建后需调用 Start 方法才能开始处理请求。
//
// 参数：
//   - cfg: 服务器配置对象，包含监听地址、代理、静态文件、安全等配置
//
// 返回值：
//   - *Server: 创建的服务器实例
func New(cfg *config.Config) *Server {
	return &Server{config: cfg}
}

// trackStats 包装处理器以统计请求数和数据传输量。
//
// 在每个请求处理前后更新统计字段：requests、bytesSent、bytesReceived。
//
// 参数：
//   - handler: 原始的请求处理器
//
// 返回值：
//   - fasthttp.RequestHandler: 包装后的处理器
func (s *Server) trackStats(handler fasthttp.RequestHandler) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		s.requests.Add(1)
		s.bytesReceived.Add(int64(len(ctx.Request.Body())))
		handler(ctx)
		s.bytesSent.Add(int64(len(ctx.Response.Body())))
	}
}

// GetListeners 获取服务器监听器列表。
//
// 返回当前服务器使用的监听器，用于热升级时传递给子进程。
//
// 返回值：
//   - []net.Listener: 监听器列表
func (s *Server) GetListeners() []net.Listener {
	return s.listeners
}

// SetListeners 设置服务器监听器列表。
//
// 用于热升级时，子进程从父进程继承监听器。
//
// 参数：
//   - listeners: 要设置的监听器列表
func (s *Server) SetListeners(listeners []net.Listener) {
	s.listeners = listeners
}

// SetUpgradeManager 设置升级管理器。
//
// 用于从外部（App 层）注入升级管理器，使服务器能够在
// createListener 中检查热升级状态和继承的监听器。
//
// 参数：
//   - mgr: 升级管理器实例
func (s *Server) SetUpgradeManager(mgr *UpgradeManager) {
	s.upgradeManager = mgr
}

// GetTLSConfig 获取 TLS 配置。
//
// 返回服务器的 TLS 配置，用于 HTTP/3 等需要 TLS 的协议。
//
// 返回值：
//   - *tls.Config: TLS 配置对象
//   - error: 未配置 TLS 或配置无效时返回错误
func (s *Server) GetTLSConfig() (*tls.Config, error) {
	if s.tlsManager == nil {
		return nil, fmt.Errorf("TLS not configured")
	}
	return s.tlsManager.GetTLSConfig(), nil
}

// GetHandler 获取请求处理器。
//
// 返回服务器的请求处理器，用于 HTTP/3 等需要复用处理器的场景。
//
// 返回值：
//   - fasthttp.RequestHandler: 请求处理器
func (s *Server) GetHandler() fasthttp.RequestHandler {
	return s.handler
}

// buildMiddlewareChain 构建中间件链。
//
// 根据服务器配置按顺序构建中间件链，顺序为：
//
//	AccessLog -> AccessControl -> RateLimiter -> BasicAuth -> Rewrite -> Compression -> SecurityHeaders
//
// 参数：
//   - serverCfg: 单个服务器的配置对象
//
// 返回值：
//   - *middleware.Chain: 构建完成的中间件链
//   - error: 构建过程中遇到的错误，如中间件创建失败
//
// 注意事项：
//   - 各中间件按顺序依次包装请求处理器
//   - 未配置的中间件不会添加到链中
func (s *Server) buildMiddlewareChain(serverCfg *config.ServerConfig) (*middleware.Chain, error) {
	var middlewares []middleware.Middleware

	// 1. AccessLog (已集成)
	s.accessLogMiddleware = accesslog.New(&s.config.Logging)
	middlewares = append(middlewares, s.accessLogMiddleware)

	// 2. Security: AccessControl (IP 访问控制)
	if len(serverCfg.Security.Access.Allow) > 0 || len(serverCfg.Security.Access.Deny) > 0 {
		ac, err := security.NewAccessControl(&serverCfg.Security.Access)
		if err != nil {
			return nil, fmt.Errorf("创建访问控制中间件失败: %w", err)
		}
		middlewares = append(middlewares, ac)
		s.accessControl = ac
	}

	// 3. Security: RateLimiter (速率限制)
	if serverCfg.Security.RateLimit.RequestRate > 0 {
		rl, err := security.NewRateLimiter(&serverCfg.Security.RateLimit)
		if err != nil {
			return nil, fmt.Errorf("创建限流中间件失败: %w", err)
		}
		middlewares = append(middlewares, rl)
	}

	// 3.5 Security: ConnLimiter (连接数限制)
	if serverCfg.Security.RateLimit.ConnLimit > 0 {
		cl, err := security.NewConnLimiter(serverCfg.Security.RateLimit.ConnLimit, true, serverCfg.Security.RateLimit.Key)
		if err != nil {
			return nil, fmt.Errorf("创建连接限制中间件失败: %w", err)
		}
		middlewares = append(middlewares, cl.Middleware())
	}

	// 4. Security: BasicAuth (认证)
	if len(serverCfg.Security.Auth.Users) > 0 {
		auth, err := security.NewBasicAuth(&serverCfg.Security.Auth)
		if err != nil {
			return nil, fmt.Errorf("创建认证中间件失败: %w", err)
		}
		middlewares = append(middlewares, auth)
	}

	// 4.3 Security: AuthRequest (外部认证子请求)
	if serverCfg.Security.AuthRequest.Enabled && serverCfg.Security.AuthRequest.URI != "" {
		authReq, err := security.NewAuthRequest(serverCfg.Security.AuthRequest)
		if err != nil {
			return nil, fmt.Errorf("创建外部认证中间件失败: %w", err)
		}
		middlewares = append(middlewares, authReq)
	}

	// 4.5 BodyLimit (请求体大小限制)
	// 创建 bodylimit 中间件，使用全局配置或默认值
	bodyLimitMiddleware := bodylimit.NewWithDefault()
	if serverCfg.ClientMaxBodySize != "" {
		bl, err := bodylimit.New(serverCfg.ClientMaxBodySize)
		if err != nil {
			return nil, fmt.Errorf("创建请求体限制中间件失败: %w", err)
		}
		bodyLimitMiddleware = bl
	}
	// 添加路径级别的限制配置
	for i := range serverCfg.Proxy {
		if serverCfg.Proxy[i].ClientMaxBodySize != "" {
			if err := bodyLimitMiddleware.AddPathLimit(
				serverCfg.Proxy[i].Path,
				serverCfg.Proxy[i].ClientMaxBodySize,
			); err != nil {
				return nil, fmt.Errorf("添加路径请求体限制失败: %w", err)
			}
		}
	}
	middlewares = append(middlewares, bodyLimitMiddleware)

	// 5. Rewrite (URL 重写)
	if len(serverCfg.Rewrite) > 0 {
		rw, err := rewrite.New(serverCfg.Rewrite)
		if err != nil {
			return nil, fmt.Errorf("创建重写中间件失败: %w", err)
		}
		middlewares = append(middlewares, rw)
	}

	// 6. Compression (响应压缩)
	if serverCfg.Compression.Type != "" {
		comp, err := compression.New(&serverCfg.Compression)
		if err != nil {
			return nil, fmt.Errorf("创建压缩中间件失败: %w", err)
		}
		middlewares = append(middlewares, comp)
	}

	// 7. SecurityHeaders (安全头部)
	// 如果有任何安全头部配置，则启用
	if serverCfg.Security.Headers.XFrameOptions != "" ||
		serverCfg.Security.Headers.XContentTypeOptions != "" ||
		serverCfg.Security.Headers.ContentSecurityPolicy != "" ||
		serverCfg.Security.Headers.ReferrerPolicy != "" ||
		serverCfg.Security.Headers.PermissionsPolicy != "" {
		headers := security.NewHeadersWithHSTS(&serverCfg.Security.Headers, &serverCfg.SSL.HSTS)
		middlewares = append(middlewares, headers)
	}

	// 8. ErrorIntercept (错误页面拦截)
	// 如果配置了错误页面，添加错误拦截中间件
	if s.errorPageManager != nil && s.errorPageManager.IsConfigured() {
		ei := errorintercept.New(s.errorPageManager)
		middlewares = append(middlewares, ei)
	}

	// Lua 中间件（可选）
	if s.luaEngine != nil && serverCfg.Lua != nil && serverCfg.Lua.Enabled {
		luaMiddlewares, err := s.buildLuaMiddlewares(serverCfg.Lua)
		if err != nil {
			return nil, fmt.Errorf("创建 Lua 中间件失败: %w", err)
		}
		middlewares = append(middlewares, luaMiddlewares...)
	}

	return middleware.NewChain(middlewares...), nil
}

// buildLuaMiddlewares 根据 Lua 配置创建中间件。
//
// 根据 Scripts 配置创建 LuaMiddleware 或 MultiPhaseLuaMiddleware。
// 支持单脚本和多阶段脚本配置。
//
// 参数：
//   - luaCfg: Lua 配置对象
//
// 返回值：
//   - []middleware.Middleware: 创建的中间件列表
//   - error: 创建过程中遇到的错误
func (s *Server) buildLuaMiddlewares(luaCfg *config.LuaMiddlewareConfig) ([]middleware.Middleware, error) {
	if s.luaEngine == nil {
		return nil, nil
	}

	// 按阶段分组脚本
	phaseScripts := make(map[string][]config.LuaScriptConfig)
	for _, script := range luaCfg.Scripts {
		// 默认启用
		enabled := script.Enabled
		if !enabled && script.Timeout == 0 && script.Path != "" {
			enabled = true // 零值时默认启用
		}
		if enabled {
			phaseScripts[script.Phase] = append(phaseScripts[script.Phase], script)
		}
	}

	var middlewares []middleware.Middleware

	// 为每个阶段创建中间件
	for phase, scripts := range phaseScripts {
		if len(scripts) == 0 {
			continue
		}

		// 单脚本：直接创建 LuaMiddleware
		if len(scripts) == 1 {
			script := scripts[0]
			luaPhase, err := lua.ParsePhase(phase)
			if err != nil {
				return nil, fmt.Errorf("无效的阶段 '%s': %w", phase, err)
			}

			timeout := script.Timeout
			if timeout == 0 {
				timeout = 30 * time.Second
			}

			cfg := lua.LuaMiddlewareConfig{
				ScriptPath: script.Path,
				Phase:      luaPhase,
				Timeout:    timeout,
				Name:       fmt.Sprintf("lua-%s", phase),
			}

			mw, err := lua.NewLuaMiddleware(s.luaEngine, cfg)
			if err != nil {
				return nil, fmt.Errorf("创建 Lua 中间件失败 (phase=%s): %w", phase, err)
			}

			middlewares = append(middlewares, mw)
		} else {
			// 多脚本：创建 MultiPhaseLuaMiddleware
			multi := lua.NewMultiPhaseLuaMiddleware(s.luaEngine, fmt.Sprintf("lua-multi-%s", phase))
			for _, script := range scripts {
				luaPhase, err := lua.ParsePhase(phase)
				if err != nil {
					return nil, fmt.Errorf("无效的阶段 '%s': %w", phase, err)
				}

				timeout := script.Timeout
				if timeout == 0 {
					timeout = 30 * time.Second
				}

				err = multi.AddPhase(luaPhase, script.Path, timeout)
				if err != nil {
					return nil, fmt.Errorf("添加 Lua 阶段失败 (phase=%s): %w", phase, err)
				}
			}

			middlewares = append(middlewares, multi)
		}
	}

	return middlewares, nil
}

// Start 启动 HTTP 服务器。
//
// 初始化日志系统、性能优化组件（Goroutine池、文件缓存），
// 根据配置选择单服务器模式或虚拟主机模式启动。
//
// 返回值：
//   - error: 启动过程中遇到的错误，如监听地址绑定失败
//
// 注意事项：
//   - 该方法会阻塞运行，直到服务器停止
//   - 调用前需确保配置已正确加载
//   - Goroutine池和文件缓存根据配置自动启用
func (s *Server) Start() error {
	logging.Init(s.config.Logging.Error.Level, s.config.Logging.Format)

	// 记录启动时间
	s.startTime = time.Now()

	// 初始化 GoroutinePool
	s.pool = initGoroutinePool(&s.config.Performance)

	// 初始化文件缓存
	s.fileCache = initFileCache(&s.config.Performance)

	// 初始化错误页面管理器
	var err error
	if len(s.config.Servers) > 0 {
		s.errorPageManager, err = initErrorPageManager(&s.config.Servers[0].Security.ErrorPage)
		if err != nil {
			return err
		}

		// 初始化 Lua 引擎
		s.luaEngine, err = initLuaEngine(s.config.Servers[0].Lua)
		if err != nil {
			return err
		}
	}

	// 根据模式选择启动方式
	mode := s.config.GetMode()
	switch mode {
	case config.ServerModeSingle:
		return s.startSingleMode()
	case config.ServerModeVHost:
		return s.startVHostMode()
	case config.ServerModeMultiServer:
		return s.startMultiServerMode()
	case config.ServerModeAuto:
		// auto 模式下 GetMode() 会自动推断，此处为防御性处理
		return s.startSingleMode()
	default:
		// 默认使用单服务器模式
		return s.startSingleMode()
	}
}

// createListener 根据配置创建监听器。
//
// 支持两种监听器格式：
//   - "unix:/path/to/socket" -> Unix domain socket
//   - ":8080" / "127.0.0.1:8080" -> TCP
//
// Unix socket 模式下会自动处理：
//   - 热升级时继承的监听器复用
//   - 旧 socket 文件清理
//   - socket 文件权限设置
//
// 参数：
//   - cfg: 服务器配置
//
// 返回值：
//   - net.Listener: 创建的监听器
//   - error: 创建失败时返回错误
func (s *Server) createListener(cfg *config.ServerConfig) (net.Listener, error) {
	listenAddr := cfg.Listen

	if strings.HasPrefix(listenAddr, "unix:") {
		// Unix Socket 模式
		socketPath := listenAddr[5:]

		// 1. 检查继承的监听器（热升级场景）
		if s.upgradeManager != nil && s.upgradeManager.IsChild() {
			inherited, _ := s.upgradeManager.GetInheritedListeners()
			for _, ln := range inherited {
				if ln.Addr().Network() == "unix" && ln.Addr().String() == socketPath {
					return ln, nil
				}
			}
		}

		// 2. 清理旧 socket 文件
		if _, err := os.Stat(socketPath); err == nil {
			os.Remove(socketPath)
		}

		// 3. 创建 Unix socket listener
		listener, err := net.Listen("unix", socketPath)
		if err != nil {
			return nil, fmt.Errorf("create unix socket failed: %w", err)
		}

		// 4. 设置 socket 文件权限
		mode := 0o666
		if cfg.UnixSocket.Mode > 0 {
			mode = cfg.UnixSocket.Mode
		}
		if err := os.Chmod(socketPath, os.FileMode(mode)); err != nil {
			logging.Warn().Err(err).Msg("设置 socket 文件权限失败")
		}

		// 5. 设置文件所有权（需要 root 权限）
		if cfg.UnixSocket.User != "" || cfg.UnixSocket.Group != "" {
			// 简化处理：仅记录警告，实际实现需要 syscall.Chown
			logging.Warn().Msg("Unix socket 用户/组配置需要 root 权限，已跳过")
		}

		return listener, nil
	}

	// TCP 模式
	return net.Listen("tcp", listenAddr)
}

// startSingleMode 单服务器模式启动。
//
// 在单服务器模式下，创建单一路由器，注册代理路由和静态文件服务，
// 应用中间件链后启动 fasthttp 服务器。
//
// 返回值：
//   - error: 启动过程中遇到的错误
//
// 注意事项：
//   - 静态文件服务作为 fallback 处理非代理路径的请求
//   - 使用零拷贝传输优化大文件传输
func (s *Server) startSingleMode() error {
	// 使用 Servers[0] 配置（迁移后 Server 字段为空）
	serverCfg := &s.config.Servers[0]

	// 创建 LocationEngine
	s.locationEngine = matcher.NewLocationEngine()

	// 注册状态监控端点（如果配置）
	if s.config.Monitoring.Status.Path != "" || len(s.config.Monitoring.Status.Allow) > 0 {
		statusHandler, err := NewStatusHandler(s, &s.config.Monitoring.Status)
		if err != nil {
			logging.Error().Msg("创建状态处理器失败: " + err.Error())
		} else {
			_ = s.locationEngine.AddExact(statusHandler.Path(), statusHandler.ServeHTTP)
		}
	}

	// 注册 pprof 性能分析端点（如果配置）
	if s.config.Monitoring.Pprof.Enabled {
		pprofHandler, err := NewPprofHandler(&s.config.Monitoring.Pprof)
		if err != nil {
			logging.Error().Msg("创建 pprof 处理器失败: " + err.Error())
		} else {
			_ = s.locationEngine.AddExact(pprofHandler.Path(), pprofHandler.ServeHTTP)
			_ = s.locationEngine.AddPrefixPriority(pprofHandler.Path()+"/", pprofHandler.ServeHTTP)
		}
	}

	// 注册缓存清理 API（如果配置）
	if serverCfg.CacheAPI != nil && serverCfg.CacheAPI.Enabled {
		purgeHandler, err := NewPurgeHandler(s, serverCfg.CacheAPI)
		if err != nil {
			logging.Error().Msg("创建缓存清理处理器失败: " + err.Error())
		} else {
			_ = s.locationEngine.AddExact(purgeHandler.Path(), purgeHandler.ServeHTTP)
		}
	}

	// 注册代理路由
	s.registerProxyRoutesWithLocationEngine(serverCfg)

	// 静态文件服务
	s.registerStaticHandlersWithLocationEngine(serverCfg)

	// 标记 LocationEngine 初始化完成
	s.locationEngine.MarkInitialized()

	// 构建中间件链
	chain, err := s.buildMiddlewareChain(serverCfg)
	if err != nil {
		return err
	}

	// 创建主请求处理器，使用 LocationEngine 匹配路由
	locationEngine := s.locationEngine
	baseHandler := func(ctx *fasthttp.RequestCtx) {
		path := string(ctx.Path())
		result := locationEngine.Match(path)
		if result != nil && result.Handler != nil {
			result.Handler(ctx)
			return
		}
		// 无匹配，返回 404
		ctx.SetStatusCode(404)
		ctx.SetBodyString("Not Found")
	}

	// 应用中间件
	handler := chain.Apply(baseHandler)
	if s.pool != nil {
		handler = s.pool.WrapHandler(handler)
	}
	// 包装统计追踪
	handler = s.trackStats(handler)
	s.handler = handler

	s.fastServer = &fasthttp.Server{
		Name:               "lolly",
		Handler:            s.handler,
		ReadTimeout:        serverCfg.ReadTimeout,
		WriteTimeout:       serverCfg.WriteTimeout,
		IdleTimeout:        serverCfg.IdleTimeout,
		MaxConnsPerIP:      serverCfg.MaxConnsPerIP,
		MaxRequestsPerConn: serverCfg.MaxRequestsPerConn,
		CloseOnShutdown:    true,
		// 高并发优化配置
		Concurrency:       serverCfg.Concurrency,
		ReadBufferSize:    serverCfg.ReadBufferSize,
		WriteBufferSize:   serverCfg.WriteBufferSize,
		ReduceMemoryUsage: serverCfg.ReduceMemoryUsage,
	}

	s.running = true

	// 创建监听器并保存，用于热升级
	ln, err := s.createListener(serverCfg)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}
	s.listeners = []net.Listener{ln}

	// 检查是否配置了 SSL/TLS
	if serverCfg.SSL.Cert != "" && serverCfg.SSL.Key != "" {
		var err error
		s.tlsManager, err = ssl.NewTLSManager(&serverCfg.SSL)
		if err != nil {
			return fmt.Errorf("创建 TLS 管理器失败: %w", err)
		}
		s.fastServer.TLSConfig = s.tlsManager.GetTLSConfig()
		return s.fastServer.ServeTLS(ln, "", "")
	}

	return s.fastServer.Serve(ln)
}

// startVHostMode 虚拟主机模式启动。
//
// 在虚拟主机模式下，为每个配置的服务器创建独立的路由器和中间件链，
// 通过虚拟主机管理器根据 Host 头分发请求。
//
// 返回值：
//   - error: 启动过程中遇到的错误
//
// 注意事项：
//   - 每个虚拟主机有独立的中间件配置
//   - 未匹配的 Host 头请求由默认主机处理
func (s *Server) startVHostMode() error {
	vhostMgr := NewVHostManager()

	for i := range s.config.Servers {
		router := handler.NewRouter()
		s.registerProxyRoutes(router, &s.config.Servers[i])

		// 静态文件
		s.registerStaticHandlers(router, &s.config.Servers[i])

		// 为每个虚拟主机构建独立的中间件链
		chain, err := s.buildMiddlewareChain(&s.config.Servers[i])
		if err != nil {
			return err
		}

		handler := chain.Apply(router.Handler())
		if s.pool != nil {
			handler = s.pool.WrapHandler(handler)
		}

		// 注册 server_names 数组中的所有主机名
		names := s.config.Servers[i].ServerNames
		if len(names) == 0 {
			// 如果未配置 server_names，使用 Name 字段
			names = []string{s.config.Servers[i].Name}
		}
		for _, name := range names {
			if err := vhostMgr.AddHost(name, handler); err != nil {
				return fmt.Errorf("add host %s: %w", name, err)
			}
		}
	}

	// 默认主机
	if s.config.GetDefaultServerFromList() != nil {
		router := handler.NewRouter()

		// 注册状态监控端点（如果启用）
		if s.config.Monitoring.Status.Enabled {
			statusHandler, err := NewStatusHandler(s, &s.config.Monitoring.Status)
			if err != nil {
				logging.Error().Msg("创建状态处理器失败: " + err.Error())
			} else {
				router.GET(statusHandler.Path(), statusHandler.ServeHTTP)
			}
		}

		// 注册 pprof 性能分析端点（如果配置）
		if s.config.Monitoring.Pprof.Enabled {
			pprofHandler, err := NewPprofHandler(&s.config.Monitoring.Pprof)
			if err != nil {
				logging.Error().Msg("创建 pprof 处理器失败: " + err.Error())
			} else {
				router.GET(pprofHandler.Path(), pprofHandler.ServeHTTP)
				router.GET(pprofHandler.Path()+"/{profile:*}", pprofHandler.ServeHTTP)
			}
		}

		// 注册缓存清理 API（如果配置）
		defaultSrv := s.config.GetDefaultServerFromList()
		if defaultSrv != nil && defaultSrv.CacheAPI != nil && defaultSrv.CacheAPI.Enabled {
			purgeHandler, err := NewPurgeHandler(s, defaultSrv.CacheAPI)
			if err != nil {
				logging.Error().Msg("创建缓存清理处理器失败: " + err.Error())
			} else {
				router.POST(purgeHandler.Path(), purgeHandler.ServeHTTP)
			}
		}

		s.registerProxyRoutes(router, defaultSrv)

		// 静态文件
		s.registerStaticHandlers(router, defaultSrv)

		chain, err := s.buildMiddlewareChain(defaultSrv)
		if err != nil {
			return err
		}

		handler := chain.Apply(router.Handler())
		if s.pool != nil {
			handler = s.pool.WrapHandler(handler)
		}
		vhostMgr.SetDefault(handler)
	}

	s.handler = vhostMgr.Handler()
	// 包装统计追踪
	s.handler = s.trackStats(s.handler)

	// 使用 Servers[0] 配置（迁移后 Server 字段为空）
	serverCfg := &s.config.Servers[0]

	s.fastServer = &fasthttp.Server{
		Name:               "lolly",
		Handler:            s.handler,
		ReadTimeout:        serverCfg.ReadTimeout,
		WriteTimeout:       serverCfg.WriteTimeout,
		IdleTimeout:        serverCfg.IdleTimeout,
		MaxConnsPerIP:      serverCfg.MaxConnsPerIP,
		MaxRequestsPerConn: serverCfg.MaxRequestsPerConn,
		CloseOnShutdown:    true,
		// 高并发优化配置
		Concurrency:       serverCfg.Concurrency,
		ReadBufferSize:    serverCfg.ReadBufferSize,
		WriteBufferSize:   serverCfg.WriteBufferSize,
		ReduceMemoryUsage: serverCfg.ReduceMemoryUsage,
	}

	s.running = true

	// 创建监听器并保存，用于热升级
	ln, err := s.createListener(serverCfg)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}
	s.listeners = []net.Listener{ln}

	// 检查是否配置了 SSL/TLS
	if serverCfg.SSL.Cert != "" && serverCfg.SSL.Key != "" {
		var err error
		s.tlsManager, err = ssl.NewTLSManager(&serverCfg.SSL)
		if err != nil {
			return fmt.Errorf("创建 TLS 管理器失败: %w", err)
		}
		s.fastServer.TLSConfig = s.tlsManager.GetTLSConfig()
		return s.fastServer.ServeTLS(ln, "", "")
	}

	return s.fastServer.Serve(ln)
}

// startMultiServerMode 多服务器模式启动。
//
// 为每个配置的服务器创建独立的 fasthttp.Server 实例，
// 每个实例监听各自的地址并运行在独立的 goroutine 中。
//
// 返回值：
//   - error: 启动过程中遇到的第一个错误（或全部成功时返回 nil）
//
// 注意事项：
//   - 每个服务器有独立的中间件配置
//   - 热升级场景下回退到虚拟主机模式
//   - 使用 goroutine 并行启动多个服务器
func (s *Server) startMultiServerMode() error {
	// 热升级检测：multi_server 热升级未实现，回退到 vhost 模式
	if os.Getenv("GRACEFUL_UPGRADE") == "1" {
		logging.Warn().Msg("热升级模式下 multi_server 模式未实现，回退到虚拟主机模式")
		return s.startVHostMode()
	}

	s.fastServers = make([]*fasthttp.Server, len(s.config.Servers))
	s.listeners = make([]net.Listener, len(s.config.Servers))

	var wg sync.WaitGroup
	errCh := make(chan error, len(s.config.Servers))

	// 并行创建监听器和 fasthttp.Server
	for i := range s.config.Servers {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			serverCfg := &s.config.Servers[idx]

			// 创建监听器
			ln, err := s.createListener(serverCfg)
			if err != nil {
				errCh <- fmt.Errorf("监听地址 %s 失败: %w", serverCfg.Listen, err)
				return
			}
			s.listeners[idx] = ln

			// 创建路由器
			router := handler.NewRouter()

			// 注册缓存清理 API（仅第一个服务器）
			if idx == 0 && serverCfg.CacheAPI != nil && serverCfg.CacheAPI.Enabled {
				purgeHandler, purgeErr := NewPurgeHandler(s, serverCfg.CacheAPI)
				if purgeErr != nil {
					errCh <- fmt.Errorf("创建缓存清理处理器失败 (server[%d]): %w", idx, purgeErr)
					return
				}
				router.POST(purgeHandler.Path(), purgeHandler.ServeHTTP)
			}

			s.registerProxyRoutes(router, serverCfg)

			// 静态文件服务
			s.registerStaticHandlers(router, serverCfg)

			// 构建独立的中间件链
			chain, err := s.buildMiddlewareChain(serverCfg)
			if err != nil {
				errCh <- fmt.Errorf("构建中间件链失败 (server[%d]): %w", idx, err)
				return
			}

			// 应用中间件
			h := chain.Apply(router.Handler())
			if s.pool != nil {
				h = s.pool.WrapHandler(h)
			}
			h = s.trackStats(h)

			// 创建 fasthttp.Server
			fastSrv := &fasthttp.Server{
				Name:               "lolly",
				Handler:            h,
				ReadTimeout:        serverCfg.ReadTimeout,
				WriteTimeout:       serverCfg.WriteTimeout,
				IdleTimeout:        serverCfg.IdleTimeout,
				MaxConnsPerIP:      serverCfg.MaxConnsPerIP,
				MaxRequestsPerConn: serverCfg.MaxRequestsPerConn,
				CloseOnShutdown:    true,
			}

			// 检查 SSL 配置
			if serverCfg.SSL.Cert != "" && serverCfg.SSL.Key != "" {
				tlsManager, err := ssl.NewTLSManager(&serverCfg.SSL)
				if err != nil {
					errCh <- fmt.Errorf("创建 TLS 管理器失败 (server[%d]): %w", idx, err)
					return
				}
				fastSrv.TLSConfig = tlsManager.GetTLSConfig()
			}

			s.fastServers[idx] = fastSrv
		}(i)
	}

	// 等待所有 goroutine 完成
	wg.Wait()
	close(errCh)

	// 检查是否有错误
	var firstErr error
	for err := range errCh {
		if firstErr == nil {
			firstErr = err
		}
	}

	// 如果有错误，清理已创建的监听器
	if firstErr != nil {
		for _, ln := range s.listeners {
			if ln != nil {
				_ = ln.Close()
			}
		}
		return firstErr
	}

	s.running = true

	// 启动所有服务器
	for idx, fastSrv := range s.fastServers {
		ln := s.listeners[idx]
		if fastSrv == nil || ln == nil {
			continue
		}

		wg.Add(1)
		go func(f *fasthttp.Server, l net.Listener, i int) {
			defer wg.Done()
			var serveErr error
			if f.TLSConfig != nil {
				serveErr = f.ServeTLS(l, "", "")
			} else {
				serveErr = f.Serve(l)
			}
			if serveErr != nil {
				logging.Error().Err(serveErr).Msgf("服务器 [%d] 监听 %s 时发生错误", i, l.Addr())
			}
		}(fastSrv, ln, idx)
	}

	// 等待服务器停止（阻塞）
	wg.Wait()
	return nil
}

// registerProxyRoutesWithLocationEngine 使用 LocationEngine 注册代理路由。
//
// 根据配置为 LocationEngine 注册代理路径，创建代理处理器和健康检查器。
// 支持通过 LocationType 配置不同的匹配方式。
func (s *Server) registerProxyRoutesWithLocationEngine(serverCfg *config.ServerConfig) {
	for i := range serverCfg.Proxy {
		proxyCfg := &serverCfg.Proxy[i]

		// 转换目标
		targets := make([]*loadbalance.Target, len(proxyCfg.Targets))
		for j, t := range proxyCfg.Targets {
			targets[j] = loadbalance.NewTargetFromConfig(t.URL, t.Weight)
		}

		// 传递 Transport 配置和 Lua 引擎
		p, err := proxy.NewProxy(proxyCfg, targets, &s.config.Performance.Transport, s.luaEngine)
		if err != nil {
			logging.Error().Msg("创建代理失败: " + err.Error())
			continue
		}

		// 设置 DNS 解析器（如果已配置）
		if s.resolver != nil {
			p.SetResolver(s.resolver)
			if err := p.Start(); err != nil {
				logging.Error().Err(err).Msg("启动代理失败")
			}
		}

		// 启动健康检查
		if proxyCfg.HealthCheck.Interval > 0 {
			hc := proxy.NewHealthChecker(targets, &proxyCfg.HealthCheck)
			hc.Start()
			s.healthCheckers = append(s.healthCheckers, hc)
			// 设置被动健康检查
			p.SetHealthChecker(hc)
		}

		// 保存代理实例用于缓存统计
		s.proxies = append(s.proxies, p)

		// 根据 LocationType 注册路由
		locType := proxyCfg.LocationType
		if locType == "" {
			locType = matcher.LocationTypePrefix
		}

		switch locType {
		case matcher.LocationTypeExact:
			_ = s.locationEngine.AddExact(proxyCfg.Path, p.ServeHTTP)
		case matcher.LocationTypePrefixPriority:
			_ = s.locationEngine.AddPrefixPriority(proxyCfg.Path, p.ServeHTTP)
		case matcher.LocationTypeRegex, matcher.LocationTypeRegexCaseless:
			caseInsensitive := locType == matcher.LocationTypeRegexCaseless
			_ = s.locationEngine.AddRegex(proxyCfg.Path, p.ServeHTTP, caseInsensitive)
		case matcher.LocationTypeNamed:
			if proxyCfg.LocationName != "" {
				_ = s.locationEngine.AddNamed(proxyCfg.LocationName, p.ServeHTTP)
			}
		case matcher.LocationTypePrefix:
			_ = s.locationEngine.AddPrefix(proxyCfg.Path, p.ServeHTTP)
		default:
			_ = s.locationEngine.AddPrefix(proxyCfg.Path, p.ServeHTTP)
		}
	}
}

// registerStaticHandlersWithLocationEngine 使用 LocationEngine 注册静态文件处理器。
func (s *Server) registerStaticHandlersWithLocationEngine(cfg *config.ServerConfig) {
	for _, static := range cfg.Static {
		path := static.Path
		if path == "" {
			path = "/"
		}

		staticHandler := handler.NewStaticHandler(
			static.Root,
			path,
			static.Index,
			true, // useSendfile
		)
		if s.fileCache != nil {
			staticHandler.SetFileCache(s.fileCache)
			// 设置默认缓存 TTL (5s)
			staticHandler.SetCacheTTL(5 * time.Second)
		}
		if cfg.Compression.GzipStatic {
			staticHandler.SetGzipStatic(true, cfg.Compression.GzipStaticExtensions)
		}

		// 设置符号链接安全检查
		staticHandler.SetSymlinkCheck(static.SymlinkCheck)

		// 根据 LocationType 注册路由
		locType := static.LocationType
		if locType == "" {
			locType = matcher.LocationTypePrefix
		}

		switch locType {
		case matcher.LocationTypeExact:
			_ = s.locationEngine.AddExact(path, staticHandler.Handle)
		case matcher.LocationTypePrefixPriority:
			_ = s.locationEngine.AddPrefixPriority(path, staticHandler.Handle)
		case matcher.LocationTypePrefix:
			_ = s.locationEngine.AddPrefix(path, staticHandler.Handle)
		default:
			_ = s.locationEngine.AddPrefix(path, staticHandler.Handle)
		}
	}
}

// registerProxyRoutes 注册代理路由。
//
// 根据配置为路由器注册代理路径，创建代理处理器和健康检查器。
// 支持 GET、POST、PUT、DELETE、HEAD 等 HTTP 方法。
//
// 参数：
//   - router: 路由器实例，用于注册路由规则
//   - serverCfg: 服务器配置，包含代理目标、负载均衡、健康检查等设置
//
// 注意事项：
//   - 代理目标初始状态默认为健康
//   - 健康检查根据配置自动启动
func (s *Server) registerProxyRoutes(router *handler.Router, serverCfg *config.ServerConfig) {
	for i := range serverCfg.Proxy {
		proxyCfg := &serverCfg.Proxy[i]

		// 转换目标
		targets := make([]*loadbalance.Target, len(proxyCfg.Targets))
		for j, t := range proxyCfg.Targets {
			targets[j] = loadbalance.NewTargetFromConfig(t.URL, t.Weight)
		}

		// 传递 Transport 配置和 Lua 引擎
		p, err := proxy.NewProxy(proxyCfg, targets, &s.config.Performance.Transport, s.luaEngine)
		if err != nil {
			logging.Error().Msg("创建代理失败: " + err.Error())
			continue
		}

		// 设置 DNS 解析器（如果已配置）
		if s.resolver != nil {
			p.SetResolver(s.resolver)
			if err := p.Start(); err != nil {
				logging.Error().Err(err).Msg("启动代理失败")
			}
		}

		// 启动健康检查
		if proxyCfg.HealthCheck.Interval > 0 {
			hc := proxy.NewHealthChecker(targets, &proxyCfg.HealthCheck)
			hc.Start()
			s.healthCheckers = append(s.healthCheckers, hc)
			// 设置被动健康检查
			p.SetHealthChecker(hc)
		}

		// 保存代理实例用于缓存统计
		s.proxies = append(s.proxies, p)

		// 使用前缀匹配（通配符）注册代理路由
		// path: / 匹配所有子路径如 /sorry/index
		// path: /api/ 匹配 /api/* 所有子路径
		routePath := proxyCfg.Path
		// 确保通配符路由格式正确
		if !strings.HasSuffix(routePath, "/") && routePath != "/" {
			routePath += "/"
		}
		wildcardPath := routePath + "{path:*}"
		router.GET(wildcardPath, p.ServeHTTP)
		router.POST(wildcardPath, p.ServeHTTP)
		router.PUT(wildcardPath, p.ServeHTTP)
		router.DELETE(wildcardPath, p.ServeHTTP)
		router.HEAD(wildcardPath, p.ServeHTTP)
	}
}

// shutdownServers 并行关闭多个 fasthttp.Server 实例。
//
// 使用 goroutine 并行关闭所有服务器，收集所有错误并返回聚合错误。
// 部分服务器关闭失败不会影响其他服务器的关闭。
//
// 参数：
//   - ctx: 关闭上下文，用于控制超时和取消
//   - servers: 要关闭的 fasthttp.Server 实例列表
//
// 返回值：
//   - error: 聚合错误，无错误或全部成功时返回 nil
func shutdownServers(ctx context.Context, servers []*fasthttp.Server) error {
	if len(servers) == 0 {
		return nil
	}

	var (
		mu   sync.Mutex
		errs []error
		wg   sync.WaitGroup
	)

	for _, srv := range servers {
		if srv == nil {
			continue
		}
		wg.Add(1)
		go func(s *fasthttp.Server) {
			defer wg.Done()
			if err := s.Shutdown(); err != nil {
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
			}
		}(srv)
	}

	// 等待所有关闭完成或上下文取消
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		if len(errs) == 0 {
			return nil
		}
		if len(errs) == 1 {
			return errs[0]
		}
		msgs := make([]string, len(errs))
		for i, e := range errs {
			msgs[i] = e.Error()
		}
		return fmt.Errorf("关闭服务器时发生 %d 个错误: %s", len(errs), strings.Join(msgs, "; "))
	case <-ctx.Done():
		return ctx.Err()
	}
}

// StopWithTimeout 快速停止服务器（支持自定义超时）。
//
// 立即停止服务器，不等待正在处理的请求完成。
// 停止所有健康检查器和访问日志中间件。
//
// 参数：
//   - timeout: 快速关闭的最大等待时间
//
// 返回值：
//   - error: 停止过程中遇到的错误
//
// 注意事项：
//   - 对于生产环境，建议使用 GracefulStop 实现优雅关闭
//   - timeout <= 0 时会使用默认 5s 超时
func (s *Server) StopWithTimeout(timeout time.Duration) error {
	// 防御性检查：如果 timeout <= 0，使用默认值
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	s.running = false

	// 停止 Goroutine 池
	if s.pool != nil {
		s.pool.Stop()
	}

	// 停止健康检查器
	for _, hc := range s.healthCheckers {
		hc.Stop()
	}

	// 关闭访问日志
	if s.accessLogMiddleware != nil {
		_ = s.accessLogMiddleware.Close()
	}

	// 关闭 TLS 管理器
	if s.tlsManager != nil {
		s.tlsManager.Close()
	}

	// 关闭 AccessControl (释放 GeoIP 资源)
	if s.accessControl != nil {
		if err := s.accessControl.Close(); err != nil {
			logging.Warn().Err(err).Msg("关闭 AccessControl 失败")
		}
	}

	// 关闭 Lua 引擎
	if s.luaEngine != nil {
		s.luaEngine.Close()
		logging.Info().Msg("Lua 引擎已关闭")
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// 多服务器模式：并行关闭所有 fasthttp.Server
	if len(s.fastServers) > 0 {
		return shutdownServers(ctx, s.fastServers)
	}

	// 单服务器模式：关闭单个 fasthttp.Server
	if s.fastServer != nil {
		done := make(chan struct{})
		go func() {
			_ = s.fastServer.Shutdown()
			close(done)
		}()

		select {
		case <-done:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

// GracefulStop 优雅停止服务器。
//
// 等待正在处理的请求完成后再停止服务器，确保连接正常关闭。
// 如果超时时间到达仍有请求未完成，将返回超时错误。
//
// 参数：
//   - timeout: 优雅关闭的最大等待时间
//
// 返回值：
//   - error: 停止过程中遇到的错误，超时返回 context.DeadlineExceeded
//
// 注意事项：
//   - 推荐在生产环境使用此方法关闭服务器
//   - 超时后会强制关闭，可能导致部分请求中断
func (s *Server) GracefulStop(timeout time.Duration) error {
	s.running = false

	// 停止 Goroutine 池
	if s.pool != nil {
		s.pool.Stop()
	}

	// 停止健康检查器
	for _, hc := range s.healthCheckers {
		hc.Stop()
	}

	// 关闭访问日志
	if s.accessLogMiddleware != nil {
		_ = s.accessLogMiddleware.Close()
	}

	// 关闭 TLS 管理器
	if s.tlsManager != nil {
		s.tlsManager.Close()
	}

	// 关闭 AccessControl (释放 GeoIP 资源)
	if s.accessControl != nil {
		if err := s.accessControl.Close(); err != nil {
			logging.Warn().Err(err).Msg("关闭 AccessControl 失败")
		}
	}

	// 关闭 Lua 引擎
	if s.luaEngine != nil {
		s.luaEngine.Close()
		logging.Info().Msg("Lua 引擎已关闭")
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// 多服务器模式：并行关闭所有 fasthttp.Server
	if len(s.fastServers) > 0 {
		return shutdownServers(ctx, s.fastServers)
	}

	// 单服务器模式：关闭单个 fasthttp.Server
	if s.fastServer != nil {
		done := make(chan struct{})
		go func() {
			_ = s.fastServer.Shutdown()
			close(done)
		}()

		select {
		case <-done:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

// getProxyCacheStats 收集所有代理缓存的统计信息。
func (s *Server) getProxyCacheStats() ProxyCacheStats {
	var total ProxyCacheStats
	for _, p := range s.proxies {
		if stats := p.GetCacheStats(); stats != nil {
			total.Entries += stats.Entries
			total.Pending += stats.Pending
		}
	}
	return total
}

// registerStaticHandlers 注册静态文件处理器。
//
// 为路由器注册静态文件服务，支持多个静态目录、文件缓存和预压缩文件。
//
// 参数：
//   - router: 路由器实例，用于注册路由规则
//   - cfg: 服务器配置，包含静态文件和压缩设置
func (s *Server) registerStaticHandlers(router *handler.Router, cfg *config.ServerConfig) {
	for _, static := range cfg.Static {
		path := static.Path
		if path == "" {
			path = "/"
		}

		staticHandler := handler.NewStaticHandler(
			static.Root,
			path,
			static.Index,
			true, // useSendfile
		)
		if s.fileCache != nil {
			staticHandler.SetFileCache(s.fileCache)
			// 设置默认缓存 TTL (5s)
			staticHandler.SetCacheTTL(5 * time.Second)
		}
		if cfg.Compression.GzipStatic {
			staticHandler.SetGzipStatic(true, cfg.Compression.GzipStaticExtensions)
		}

		// 设置符号链接安全检查
		staticHandler.SetSymlinkCheck(static.SymlinkCheck)

		// 设置 try_files 配置
		if len(static.TryFiles) > 0 {
			// 注意：tryFilesPass 需要路由器支持，当前实现传入 nil
			// 如果 tryFilesPass 为 true，需要额外处理
			staticHandler.SetTryFiles(static.TryFiles, static.TryFilesPass, router)
		}

		// 注册路由：确保路径以 / 结尾
		routePath := path
		if !strings.HasSuffix(routePath, "/") {
			routePath += "/"
		}
		router.GET(routePath+"{filepath:*}", staticHandler.Handle)
		router.HEAD(routePath+"{filepath:*}", staticHandler.Handle)
	}
}

// SetResolver 设置 DNS 解析器。
func (s *Server) SetResolver(r resolver.Resolver) {
	s.resolver = r
}

// GetResolver 返回 DNS 解析器。
func (s *Server) GetResolver() resolver.Resolver {
	return s.resolver
}
