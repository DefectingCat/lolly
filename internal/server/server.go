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
	"rua.plus/lolly/internal/logging"
	"rua.plus/lolly/internal/lua"
	"rua.plus/lolly/internal/matcher"
	"rua.plus/lolly/internal/middleware/accesslog"
	"rua.plus/lolly/internal/middleware/security"
	"rua.plus/lolly/internal/mimeutil"
	"rua.plus/lolly/internal/proxy"
	"rua.plus/lolly/internal/resolver"
	"rua.plus/lolly/internal/ssl"
	"rua.plus/lolly/internal/version"
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

// getServerName 根据配置返回服务器名称。
//
// 当 ServerTokens 为 false 时隐藏版本号，仅返回 "lolly"。
// 默认（ServerTokens 为 true 或零值）返回完整版本信息。
//
// 参数：
//   - cfg: 服务器配置对象
//
// 返回值：
//   - string: 服务器名称
func (s *Server) getServerName(cfg *config.ServerConfig) string {
	if cfg != nil && !cfg.ServerTokens {
		return "lolly"
	}
	return "lolly/" + version.Version
}

// createFastServer 创建 fasthttp.Server 实例。
//
// 根据配置创建并配置 fasthttp.Server，包含所有通用设置。
//
// 参数：
//   - serverCfg: 服务器配置对象
//   - handler: 请求处理器
//
// 返回值：
//   - *fasthttp.Server: 配置好的 fasthttp.Server 实例
func (s *Server) createFastServer(serverCfg *config.ServerConfig, handler fasthttp.RequestHandler) *fasthttp.Server {
	return &fasthttp.Server{
		Name:               s.getServerName(serverCfg),
		Handler:            handler,
		ReadTimeout:        serverCfg.ReadTimeout,
		WriteTimeout:       serverCfg.WriteTimeout,
		IdleTimeout:        serverCfg.IdleTimeout,
		MaxConnsPerIP:      serverCfg.MaxConnsPerIP,
		MaxRequestsPerConn: serverCfg.MaxRequestsPerConn,
		CloseOnShutdown:    true,
		Concurrency:        serverCfg.Concurrency,
		ReadBufferSize:     serverCfg.ReadBufferSize,
		WriteBufferSize:    serverCfg.WriteBufferSize,
		ReduceMemoryUsage:  serverCfg.ReduceMemoryUsage,
	}
}

// applyTypesConfig 应用 MIME 类型配置。
//
// 根据配置设置自定义 MIME 类型映射和默认类型。
//
// 参数：
//   - cfg: 服务器配置对象
func (s *Server) applyTypesConfig(cfg *config.ServerConfig) {
	if cfg == nil {
		return
	}
	if len(cfg.Types.Map) > 0 {
		mimeutil.AddTypes(cfg.Types.Map)
	}
	if cfg.Types.DefaultType != "" {
		mimeutil.SetDefaultType(cfg.Types.DefaultType)
	}
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
			_ = os.Remove(socketPath)
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
			logging.Warn().Err(err).Msg("Failed to set socket file permissions")
		}

		// 5. 设置文件所有权（需要 root 权限）
		if cfg.UnixSocket.User != "" || cfg.UnixSocket.Group != "" {
			// 简化处理：仅记录警告，实际实现需要 syscall.Chown
			logging.Warn().Msg("Unix socket user/group config requires root privileges, skipped")
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

	// 应用 MIME 类型配置
	s.applyTypesConfig(serverCfg)

	// 创建 LocationEngine
	s.locationEngine = matcher.NewLocationEngine()

	// 注册状态监控端点（如果配置）
	if s.config.Monitoring.Status.Path != "" || len(s.config.Monitoring.Status.Allow) > 0 {
		statusHandler, err := NewStatusHandler(s, &s.config.Monitoring.Status)
		if err != nil {
			logging.Error().Msg("Failed to create status handler: " + err.Error())
		} else {
			_ = s.locationEngine.AddExact(statusHandler.Path(), statusHandler.ServeHTTP, false)
		}
	}

	// 注册 pprof 性能分析端点（如果配置）
	if s.config.Monitoring.Pprof.Enabled {
		pprofHandler, err := NewPprofHandler(&s.config.Monitoring.Pprof)
		if err != nil {
			logging.Error().Msg("Failed to create pprof handler: " + err.Error())
		} else {
			_ = s.locationEngine.AddExact(pprofHandler.Path(), pprofHandler.ServeHTTP, false)
			_ = s.locationEngine.AddPrefixPriority(pprofHandler.Path()+"/", pprofHandler.ServeHTTP, false)
		}
	}

	// 注册缓存清理 API（如果配置）
	if serverCfg.CacheAPI != nil && serverCfg.CacheAPI.Enabled {
		purgeHandler, err := NewPurgeHandler(s, serverCfg.CacheAPI)
		if err != nil {
			logging.Error().Msg("Failed to create cache purge handler: " + err.Error())
		} else {
			_ = s.locationEngine.AddExact(purgeHandler.Path(), purgeHandler.ServeHTTP, false)
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

	s.fastServer = s.createFastServer(serverCfg, s.handler)

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
			return fmt.Errorf("failed to create TLS manager: %w", err)
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
				logging.Error().Msg("Failed to create status handler: " + err.Error())
			} else {
				router.GET(statusHandler.Path(), statusHandler.ServeHTTP)
			}
		}

		// 注册 pprof 性能分析端点（如果配置）
		if s.config.Monitoring.Pprof.Enabled {
			pprofHandler, err := NewPprofHandler(&s.config.Monitoring.Pprof)
			if err != nil {
				logging.Error().Msg("Failed to create pprof handler: " + err.Error())
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
				logging.Error().Msg("Failed to create cache purge handler: " + err.Error())
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

	s.fastServer = s.createFastServer(serverCfg, s.handler)

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
			return fmt.Errorf("failed to create TLS manager: %w", err)
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
		logging.Warn().Msg("multi_server mode not implemented for graceful upgrade, falling back to vhost mode")
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
				errCh <- fmt.Errorf("failed to listen on %s: %w", serverCfg.Listen, err)
				return
			}
			s.listeners[idx] = ln

			// 创建路由器
			router := handler.NewRouter()

			// 注册缓存清理 API（仅第一个服务器）
			if idx == 0 && serverCfg.CacheAPI != nil && serverCfg.CacheAPI.Enabled {
				purgeHandler, purgeErr := NewPurgeHandler(s, serverCfg.CacheAPI)
				if purgeErr != nil {
					errCh <- fmt.Errorf("failed to create cache purge handler (server[%d]): %w", idx, purgeErr)
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
				errCh <- fmt.Errorf("failed to build middleware chain (server[%d]): %w", idx, err)
				return
			}

			// 应用中间件
			h := chain.Apply(router.Handler())
			if s.pool != nil {
				h = s.pool.WrapHandler(h)
			}
			h = s.trackStats(h)

			// 创建 fasthttp.Server
			fastSrv := s.createFastServer(serverCfg, h)

			// 检查 SSL 配置
			if serverCfg.SSL.Cert != "" && serverCfg.SSL.Key != "" {
				tlsManager, err := ssl.NewTLSManager(&serverCfg.SSL)
				if err != nil {
					errCh <- fmt.Errorf("failed to create TLS manager (server[%d]): %w", idx, err)
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
				logging.Error().Err(serveErr).Msgf("Server [%d] error while listening on %s", i, l.Addr())
			}
		}(fastSrv, ln, idx)
	}

	// 等待服务器停止（阻塞）
	wg.Wait()
	return nil
}

// SetResolver 设置 DNS 解析器。
func (s *Server) SetResolver(r resolver.Resolver) {
	s.resolver = r
}

// GetResolver 返回 DNS 解析器。
func (s *Server) GetResolver() resolver.Resolver {
	return s.resolver
}
