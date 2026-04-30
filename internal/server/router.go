package server

import (
	"strings"
	"time"

	"rua.plus/lolly/internal/config"
	"rua.plus/lolly/internal/handler"
	"rua.plus/lolly/internal/loadbalance"
	"rua.plus/lolly/internal/logging"
	"rua.plus/lolly/internal/matcher"
	"rua.plus/lolly/internal/proxy"
)

// createProxyForConfig 创建代理实例并配置健康检查。
// 返回创建的代理实例，如果创建失败则返回 nil。
func (s *Server) createProxyForConfig(proxyCfg *config.ProxyConfig) *proxy.Proxy {
	// 转换目标
	targets := make([]*loadbalance.Target, len(proxyCfg.Targets))
	for j, t := range proxyCfg.Targets {
		failTimeout := t.FailTimeout
		if t.MaxFails > 0 && failTimeout == 0 {
			failTimeout = 10 * time.Second
		}
		targets[j] = loadbalance.NewTargetFromConfig(
			t.URL, t.Weight,
			int64(t.MaxConns), int64(t.MaxFails), failTimeout,
			t.Backup, t.Down, t.ProxyURI,
		)
	}

	// 传递 Transport 配置和 Lua 引擎
	p, err := proxy.NewProxy(proxyCfg, targets, &s.config.Performance.Transport, s.luaEngine)
	if err != nil {
		logging.Error().Msg("Failed to create proxy: " + err.Error())
		return nil
	}

	// 设置 DNS 解析器（如果已配置）
	if s.resolver != nil {
		p.SetResolver(s.resolver)
		if err := p.Start(); err != nil {
			logging.Error().Err(err).Msg("Failed to start proxy")
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

	return p
}

// registerProxyRoutesWithLocationEngine 使用 LocationEngine 注册代理路由。
//
// 根据配置为 LocationEngine 注册代理路径，创建代理处理器和健康检查器。
// 支持通过 LocationType 配置不同的匹配方式。
func (s *Server) registerProxyRoutesWithLocationEngine(serverCfg *config.ServerConfig) {
	for i := range serverCfg.Proxy {
		proxyCfg := &serverCfg.Proxy[i]

		p := s.createProxyForConfig(proxyCfg)
		if p == nil {
			continue
		}

		// 根据 LocationType 注册路由
		locType := proxyCfg.LocationType
		if locType == "" {
			locType = matcher.LocationTypePrefix
		}

		switch locType {
		case matcher.LocationTypeExact:
			_ = s.locationEngine.AddExact(proxyCfg.Path, p.ServeHTTP, proxyCfg.Internal)
		case matcher.LocationTypePrefixPriority:
			_ = s.locationEngine.AddPrefixPriority(proxyCfg.Path, p.ServeHTTP, proxyCfg.Internal)
		case matcher.LocationTypeRegex, matcher.LocationTypeRegexCaseless:
			caseInsensitive := locType == matcher.LocationTypeRegexCaseless
			_ = s.locationEngine.AddRegex(proxyCfg.Path, p.ServeHTTP, caseInsensitive, proxyCfg.Internal)
		case matcher.LocationTypeNamed:
			if proxyCfg.LocationName != "" {
				_ = s.locationEngine.AddNamed(proxyCfg.LocationName, p.ServeHTTP)
			}
		case matcher.LocationTypePrefix:
			_ = s.locationEngine.AddPrefix(proxyCfg.Path, p.ServeHTTP, proxyCfg.Internal)
		default:
			_ = s.locationEngine.AddPrefix(proxyCfg.Path, p.ServeHTTP, proxyCfg.Internal)
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
		// 设置 alias（与 root 互斥）
		if static.Alias != "" {
			staticHandler.SetAlias(static.Alias)
		}
		if s.fileCache != nil {
			staticHandler.SetFileCache(s.fileCache)
			// 设置默认缓存 TTL (5s)
			staticHandler.SetCacheTTL(5 * time.Second)
		}
		if cfg.Compression.GzipStatic {
			// extensions: 源文件类型，为空使用默认值
			// GzipStaticExtensions: 预压缩文件扩展名（如 .br, .gz）
			staticHandler.SetGzipStatic(true, nil, cfg.Compression.GzipStaticExtensions)
		}

		// 设置符号链接安全检查
		staticHandler.SetSymlinkCheck(static.SymlinkCheck)

		// 设置 internal 限制
		staticHandler.SetInternal(static.Internal)

		// 设置缓存过期时间
		if static.Expires != "" {
			staticHandler.SetExpires(static.Expires)
		}

		// 设置目录列表
		if static.AutoIndex {
			staticHandler.SetAutoIndex(
				static.AutoIndex,
				static.AutoIndexFormat,
				static.AutoIndexLocaltime,
				static.AutoIndexExactSize,
			)
		}

		// 根据 LocationType 注册路由
		locType := static.LocationType
		if locType == "" {
			locType = matcher.LocationTypePrefix
		}

		switch locType {
		case matcher.LocationTypeExact:
			_ = s.locationEngine.AddExact(path, staticHandler.Handle, static.Internal)
		case matcher.LocationTypePrefixPriority:
			_ = s.locationEngine.AddPrefixPriority(path, staticHandler.Handle, static.Internal)
		case matcher.LocationTypePrefix:
			_ = s.locationEngine.AddPrefix(path, staticHandler.Handle, static.Internal)
		default:
			_ = s.locationEngine.AddPrefix(path, staticHandler.Handle, static.Internal)
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

		p := s.createProxyForConfig(proxyCfg)
		if p == nil {
			continue
		}

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
		// 设置 alias（与 root 互斥）
		if static.Alias != "" {
			staticHandler.SetAlias(static.Alias)
		}
		if s.fileCache != nil {
			staticHandler.SetFileCache(s.fileCache)
			// 设置默认缓存 TTL (5s)
			staticHandler.SetCacheTTL(5 * time.Second)
		}
		if cfg.Compression.GzipStatic {
			// extensions: 源文件类型，为空使用默认值
			// GzipStaticExtensions: 预压缩文件扩展名（如 .br, .gz）
			staticHandler.SetGzipStatic(true, nil, cfg.Compression.GzipStaticExtensions)
		}

		// 设置符号链接安全检查
		staticHandler.SetSymlinkCheck(static.SymlinkCheck)

		// 设置缓存过期时间
		if static.Expires != "" {
			staticHandler.SetExpires(static.Expires)
		}

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
