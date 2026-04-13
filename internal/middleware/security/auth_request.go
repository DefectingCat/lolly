// Package security 提供安全相关的 HTTP 中间件。
//
// 该文件实现 auth_request 外部认证子请求中间件，支持将认证委托给
// 外部服务。根据认证服务的响应状态码决定是否允许原请求继续。
//
// 行为规则：
//   - 2xx 响应：认证通过，原请求继续处理
//   - 401/403 响应：认证失败，返回相应状态码
//   - 超时或连接失败：返回 500 内部服务器错误
//   - 其他响应：返回 500 内部服务器错误
//
// 使用示例：
//
//	cfg := &config.AuthRequestConfig{
//	    Enabled:  true,
//	    URI:      "http://auth-service:8080/verify",
//	    Method:   "GET",
//	    Timeout:  5 * time.Second,
//	    Headers: map[string]string{
//	        "X-Original-Uri": "$request_uri",
//	    },
//	}
//
//	authReq, err := security.NewAuthRequest(cfg)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// 应用为中间件
//	chain := middleware.NewChain(authReq)
//	handler := chain.Apply(finalHandler)
//
// 作者：xfy
package security

import (
	"errors"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/valyala/fasthttp"

	"rua.plus/lolly/internal/config"
	"rua.plus/lolly/internal/middleware"
	"rua.plus/lolly/internal/variable"
)

// AuthRequest 实现外部认证子请求中间件。
type AuthRequest struct {
	client *fasthttp.HostClient
	config config.AuthRequestConfig
	mu     sync.RWMutex
}

// NewAuthRequest 使用给定的配置创建一个新的 AuthRequest 中间件。
//
// 参数：
//   - cfg: 认证子请求配置
//
// 返回值：
//   - *AuthRequest: 配置完成的中间件实例
//   - error: 配置无效时返回错误
func NewAuthRequest(cfg config.AuthRequestConfig) (*AuthRequest, error) {
	if !cfg.Enabled {
		return &AuthRequest{config: cfg}, nil
	}

	if cfg.URI == "" {
		return nil, errors.New("auth_request: uri is required")
	}

	// 设置默认值
	method := cfg.Method
	if method == "" {
		method = "GET"
	}
	cfg.Method = strings.ToUpper(method)

	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	cfg.Timeout = timeout

	// 设置默认转发头
	if cfg.ForwardHeaders == nil {
		cfg.ForwardHeaders = []string{
			"Cookie",
			"Authorization",
			"X-Forwarded-For",
			"X-Real-Ip",
		}
	}

	ar := &AuthRequest{
		config: cfg,
	}

	// 如果 URI 是完整 URL（非相对路径），初始化 HTTP 客户端
	if isFullURL(cfg.URI) {
		if err := ar.initClient(); err != nil {
			return nil, err
		}
	}

	return ar, nil
}

// isFullURL 检查 URI 是否为完整 URL。
func isFullURL(uri string) bool {
	return strings.HasPrefix(uri, "http://") || strings.HasPrefix(uri, "https://")
}

// initClient 初始化用于认证子请求的 HTTP 客户端。
func (a *AuthRequest) initClient() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	// 解析目标地址
	addr, isTLS, err := parseAuthURL(a.config.URI)
	if err != nil {
		return err
	}

	// 创建独立连接池的客户端
	a.client = &fasthttp.HostClient{
		Addr:                   addr,
		IsTLS:                  isTLS,
		ReadTimeout:            a.config.Timeout,
		WriteTimeout:           a.config.Timeout,
		MaxIdleConnDuration:    90 * time.Second,
		MaxConns:               100,
		MaxConnWaitTimeout:     a.config.Timeout,
		RetryIf:                nil, // 禁用自动重试
		DisablePathNormalizing: false,
	}

	return nil
}

// parseAuthURL 解析认证服务 URL。
//
// 返回值：
//   - addr: 主机地址（如 "auth-service:8080"）
//   - isTLS: 是否使用 HTTPS
//   - error: 解析错误
func parseAuthURL(url string) (string, bool, error) {
	// 移除协议前缀
	var isTLS bool
	if strings.HasPrefix(url, "https://") {
		isTLS = true
		url = strings.TrimPrefix(url, "https://")
	} else if strings.HasPrefix(url, "http://") {
		url = strings.TrimPrefix(url, "http://")
	}

	// 提取主机部分
	host := url
	if idx := strings.Index(host, "/"); idx != -1 {
		host = host[:idx]
	}
	if idx := strings.Index(host, "?"); idx != -1 {
		host = host[:idx]
	}

	// 验证地址
	if host == "" {
		return "", false, errors.New("auth_request: invalid URL")
	}

	// 添加默认端口
	if _, _, err := net.SplitHostPort(host); err != nil {
		if isTLS {
			host = net.JoinHostPort(host, "443")
		} else {
			host = net.JoinHostPort(host, "80")
		}
	}

	return host, isTLS, nil
}

// Name 返回中间件名称。
func (a *AuthRequest) Name() string {
	return "auth_request"
}

// Process 实现中间件处理逻辑。
// 向认证服务发送子请求，根据响应决定是否允许原请求继续。
func (a *AuthRequest) Process(next fasthttp.RequestHandler) fasthttp.RequestHandler {
	if !a.config.Enabled {
		return next
	}

	return func(ctx *fasthttp.RequestCtx) {
		// 执行认证子请求
		allowed, statusCode, err := a.doAuthRequest(ctx)
		if err != nil {
			// 认证服务不可用或超时，返回 500
			ctx.Error("Internal Server Error", fasthttp.StatusInternalServerError)
			return
		}

		if !allowed {
			// 认证失败，返回认证服务的状态码
			ctx.Error("Unauthorized", statusCode)
			return
		}

		// 认证通过，继续处理原请求
		next(ctx)
	}
}

// doAuthRequest 执行认证子请求。
//
// 返回值：
//   - allowed: 是否允许请求继续
//   - statusCode: 认证服务的响应状态码
//   - error: 请求过程中的错误
func (a *AuthRequest) doAuthRequest(ctx *fasthttp.RequestCtx) (bool, int, error) {
	// 创建认证子请求
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	// 设置请求方法
	req.Header.SetMethod(a.config.Method)

	// 构建请求 URI（支持变量展开）
	uri := a.expandVars(ctx, a.config.URI)

	// 如果是相对路径，转换为完整 URL
	if !isFullURL(uri) {
		// 从原请求构建完整 URL
		scheme := "http"
		if ctx.IsTLS() {
			scheme = "https"
		}
		host := string(ctx.Host())
		if host == "" {
			host = "localhost"
		}
		uri = scheme + "://" + host + uri
	}
	req.SetRequestURI(uri)

	// 转发原请求的头
	for _, headerName := range a.config.ForwardHeaders {
		if value := ctx.Request.Header.Peek(headerName); len(value) > 0 {
			req.Header.Set(headerName, string(value))
		}
	}

	// 设置自定义头（支持变量展开）
	for key, value := range a.config.Headers {
		expanded := a.expandVars(ctx, value)
		req.Header.Set(key, expanded)
	}

	// 发送认证请求
	a.mu.RLock()
	client := a.client
	a.mu.RUnlock()

	if client != nil {
		// 使用独立连接池
		err := client.Do(req, resp)
		if err != nil {
			return false, 0, err
		}
	} else {
		// 使用默认客户端（相对路径情况）
		err := fasthttp.Do(req, resp)
		if err != nil {
			return false, 0, err
		}
	}

	// 根据响应状态码判断认证结果
	statusCode := resp.StatusCode()
	switch {
	case statusCode >= 200 && statusCode < 300:
		// 2xx：认证通过
		return true, statusCode, nil
	case statusCode == 401 || statusCode == 403:
		// 401/403：认证失败
		return false, statusCode, nil
	default:
		// 其他状态码：视为认证服务错误
		return false, 500, nil
	}
}

// expandVars 展开字符串中的变量。
func (a *AuthRequest) expandVars(ctx *fasthttp.RequestCtx, template string) string {
	if template == "" {
		return ""
	}

	// 快速检查：如果没有变量则直接返回
	if !strings.Contains(template, "$") {
		return template
	}

	// 创建变量上下文
	vc := variable.NewContext(ctx)
	defer variable.ReleaseContext(vc)

	return vc.Expand(template)
}

// UpdateConfig 动态更新配置。
// 用于配置热重载场景。
func (a *AuthRequest) UpdateConfig(cfg config.AuthRequestConfig) error {
	if !cfg.Enabled {
		a.mu.Lock()
		a.config = cfg
		a.client = nil
		a.mu.Unlock()
		return nil
	}

	if cfg.URI == "" {
		return errors.New("auth_request: uri is required")
	}

	// 设置默认值
	method := cfg.Method
	if method == "" {
		method = "GET"
	}
	cfg.Method = strings.ToUpper(method)

	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	cfg.Timeout = timeout

	if cfg.ForwardHeaders == nil {
		cfg.ForwardHeaders = []string{
			"Cookie",
			"Authorization",
			"X-Forwarded-For",
			"X-Real-Ip",
		}
	}

	a.mu.Lock()
	a.config = cfg

	// 重新初始化客户端
	if isFullURL(cfg.URI) {
		if err := a.initClientUnlocked(); err != nil {
			a.mu.Unlock()
			return err
		}
	} else {
		a.client = nil
	}

	a.mu.Unlock()
	return nil
}

// initClientUnlocked 在无锁状态下初始化客户端。
// 调用者必须持有写锁。
func (a *AuthRequest) initClientUnlocked() error {
	addr, isTLS, err := parseAuthURL(a.config.URI)
	if err != nil {
		return err
	}

	a.client = &fasthttp.HostClient{
		Addr:                   addr,
		IsTLS:                  isTLS,
		ReadTimeout:            a.config.Timeout,
		WriteTimeout:           a.config.Timeout,
		MaxIdleConnDuration:    90 * time.Second,
		MaxConns:               100,
		MaxConnWaitTimeout:     a.config.Timeout,
		RetryIf:                nil,
		DisablePathNormalizing: false,
	}

	return nil
}

// Close 关闭中间件并释放资源。
func (a *AuthRequest) Close() error {
	a.mu.Lock()
	a.client = nil
	a.mu.Unlock()
	return nil
}

// Middleware 返回中间件接口实现。
// 用于兼容中间件链。
func (a *AuthRequest) Middleware() middleware.Middleware {
	return a
}

// Ensure security implements Middleware interface
var _ middleware.Middleware = (*AuthRequest)(nil)
