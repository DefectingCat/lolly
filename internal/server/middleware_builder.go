package server

import (
	"fmt"
	"time"

	"rua.plus/lolly/internal/config"
	"rua.plus/lolly/internal/lua"
	"rua.plus/lolly/internal/middleware"
	"rua.plus/lolly/internal/middleware/accesslog"
	"rua.plus/lolly/internal/middleware/bodylimit"
	"rua.plus/lolly/internal/middleware/compression"
	"rua.plus/lolly/internal/middleware/errorintercept"
	"rua.plus/lolly/internal/middleware/rewrite"
	"rua.plus/lolly/internal/middleware/security"
)

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
			return nil, fmt.Errorf("failed to create access control middleware: %w", err)
		}
		middlewares = append(middlewares, ac)
		s.accessControl = ac
	}

	// 3. Security: RateLimiter (速率限制)
	if serverCfg.Security.RateLimit.RequestRate > 0 {
		rl, err := security.NewRateLimiter(&serverCfg.Security.RateLimit)
		if err != nil {
			return nil, fmt.Errorf("failed to create rate limiter middleware: %w", err)
		}
		middlewares = append(middlewares, rl)
	}

	// 3.5 Security: ConnLimiter (连接数限制)
	if serverCfg.Security.RateLimit.ConnLimit > 0 {
		cl, err := security.NewConnLimiter(serverCfg.Security.RateLimit.ConnLimit, true, serverCfg.Security.RateLimit.Key)
		if err != nil {
			return nil, fmt.Errorf("failed to create connection limiter middleware: %w", err)
		}
		middlewares = append(middlewares, cl.Middleware())
	}

	// 4. Security: BasicAuth (认证)
	if len(serverCfg.Security.Auth.Users) > 0 {
		auth, err := security.NewBasicAuth(&serverCfg.Security.Auth)
		if err != nil {
			return nil, fmt.Errorf("failed to create auth middleware: %w", err)
		}
		middlewares = append(middlewares, auth)
	}

	// 4.3 Security: AuthRequest (外部认证子请求)
	if serverCfg.Security.AuthRequest.Enabled && serverCfg.Security.AuthRequest.URI != "" {
		authReq, err := security.NewAuthRequest(serverCfg.Security.AuthRequest)
		if err != nil {
			return nil, fmt.Errorf("failed to create auth request middleware: %w", err)
		}
		middlewares = append(middlewares, authReq)
	}

	// 4.5 BodyLimit (请求体大小限制)
	// 创建 bodylimit 中间件，使用全局配置或默认值
	bodyLimitMiddleware := bodylimit.NewWithDefault()
	if serverCfg.ClientMaxBodySize != "" {
		bl, err := bodylimit.New(serverCfg.ClientMaxBodySize)
		if err != nil {
			return nil, fmt.Errorf("failed to create body limit middleware: %w", err)
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
				return nil, fmt.Errorf("failed to add path body limit: %w", err)
			}
		}
	}
	middlewares = append(middlewares, bodyLimitMiddleware)

	// 5. Rewrite (URL 重写)
	if len(serverCfg.Rewrite) > 0 {
		rw, err := rewrite.New(serverCfg.Rewrite)
		if err != nil {
			return nil, fmt.Errorf("failed to create rewrite middleware: %w", err)
		}
		middlewares = append(middlewares, rw)
	}

	// 6. Compression (响应压缩)
	if serverCfg.Compression.Type != "" {
		comp, err := compression.New(&serverCfg.Compression)
		if err != nil {
			return nil, fmt.Errorf("failed to create compression middleware: %w", err)
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
			return nil, fmt.Errorf("failed to create Lua middleware: %w", err)
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
				return nil, fmt.Errorf("invalid phase '%s': %w", phase, err)
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
				return nil, fmt.Errorf("failed to create Lua middleware (phase=%s): %w", phase, err)
			}

			middlewares = append(middlewares, mw)
		} else {
			// 多脚本：创建 MultiPhaseLuaMiddleware
			multi := lua.NewMultiPhaseLuaMiddleware(s.luaEngine, fmt.Sprintf("lua-multi-%s", phase))
			for _, script := range scripts {
				luaPhase, err := lua.ParsePhase(phase)
				if err != nil {
					return nil, fmt.Errorf("invalid phase '%s': %w", phase, err)
				}

				timeout := script.Timeout
				if timeout == 0 {
					timeout = 30 * time.Second
				}

				err = multi.AddPhase(luaPhase, script.Path, timeout)
				if err != nil {
					return nil, fmt.Errorf("failed to add Lua phase (phase=%s): %w", phase, err)
				}
			}

			middlewares = append(middlewares, multi)
		}
	}

	return middlewares, nil
}
