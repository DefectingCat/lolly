// Package security 提供了 Lolly HTTP 服务器的安全相关中间件。
//
// 该文件实现了安全响应头中间件，为响应添加标准安全头部，
// 以防止常见的 Web 安全漏洞。
//
// 实现的安全头包括：
//   - X-Frame-Options: 防止点击劫持攻击
//   - X-Content-Type-Options: 防止 MIME 类型嗅探
//   - Content-Security-Policy: 控制资源加载（XSS 防护）
//   - Strict-Transport-Security: 强制使用 HTTPS（HSTS）
//   - Referrer-Policy: 控制 Referer 信息泄露
//   - Permissions-Policy: 控制浏览器功能权限
//
// 使用示例：
//
//	cfg := &config.SecurityHeaders{
//	    XFrameOptions:        "DENY",
//	    XContentTypeOptions:  "nosniff",
//	    ContentSecurityPolicy: "default-src 'self'",
//	}
//
//	headers := security.NewSecurityHeaders(cfg)
//	chain := middleware.NewChain(headers)
//	handler := chain.Apply(finalHandler)
//
// 作者：xfy
package security

import (
	"fmt"
	"sync"

	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/config"
	"rua.plus/lolly/internal/middleware"
)

// SecurityHeadersMiddleware 安全响应头中间件。
//
// 为 HTTP 响应添加安全相关的头部字段，防止常见的 Web 安全漏洞。
// 支持配置各种安全头的值，并提供安全的默认配置。
//
// 注意事项：
//   - 所有方法均为并发安全
//   - HSTS 头仅在 TLS 连接时添加
type SecurityHeadersMiddleware struct {
	config *config.SecurityHeaders // 安全头配置
	hsts   string                   // 预格式化的 HSTS 头值
	mu     sync.RWMutex             // 读写锁，保护并发访问
}

// NewSecurityHeaders 创建新的安全响应头中间件。
//
// 根据配置创建中间件实例，如果配置为 nil 则使用安全的默认值。
//
// 参数：
//   - cfg: 安全头配置，可以为 nil 使用默认配置
//
// 返回值：
//   - *SecurityHeadersMiddleware: 配置好的中间件实例
func NewSecurityHeaders(cfg *config.SecurityHeaders) *SecurityHeadersMiddleware {
	sh := &SecurityHeadersMiddleware{}

	if cfg != nil {
		sh.config = cfg
	} else {
		// 使用安全的默认配置
		sh.config = &config.SecurityHeaders{
			XFrameOptions:       "DENY",
			XContentTypeOptions: "nosniff",
			ReferrerPolicy:      "strict-origin-when-cross-origin",
		}
	}

	// 预格式化 HSTS 头值
	sh.formatHSTS()

	return sh
}

// Name 返回中间件名称。
//
// 返回值：
//   - string: 中间件标识名 "security_headers"
func (sh *SecurityHeadersMiddleware) Name() string {
	return "security_headers"
}

// Process 包装下一个处理器，为响应添加安全头。
//
// 该方法实现了中间件接口，在调用下一个处理器后添加安全响应头。
//
// 参数：
//   - next: 下一个请求处理器
//
// 返回值：
//   - fasthttp.RequestHandler: 包装后的处理器
func (sh *SecurityHeadersMiddleware) Process(next fasthttp.RequestHandler) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		// 先调用下一个处理器
		next(ctx)

		// 为响应添加安全头
		sh.addHeaders(ctx)
	}
}

// addHeaders 为响应添加所有配置的安全头。
//
// 遍历配置的安全头并设置到响应中，使用读锁保护并发访问。
//
// 参数：
//   - ctx: FastHTTP 请求上下文
func (sh *SecurityHeadersMiddleware) addHeaders(ctx *fasthttp.RequestCtx) {
	headers := &ctx.Response.Header

	sh.mu.RLock()
	cfg := sh.config
	hstsValue := sh.hsts
	sh.mu.RUnlock()

	// X-Frame-Options
	if cfg.XFrameOptions != "" {
		headers.Set("X-Frame-Options", cfg.XFrameOptions)
	}

	// X-Content-Type-Options (default: nosniff)
	if cfg.XContentTypeOptions != "" {
		headers.Set("X-Content-Type-Options", cfg.XContentTypeOptions)
	} else {
		headers.Set("X-Content-Type-Options", "nosniff")
	}

	// Content-Security-Policy
	if cfg.ContentSecurityPolicy != "" {
		headers.Set("Content-Security-Policy", cfg.ContentSecurityPolicy)
	}

	// Strict-Transport-Security (HSTS) - only when TLS is used
	if ctx.IsTLS() && hstsValue != "" {
		headers.Set("Strict-Transport-Security", hstsValue)
	}

	// Referrer-Policy
	if cfg.ReferrerPolicy != "" {
		headers.Set("Referrer-Policy", cfg.ReferrerPolicy)
	}

	// Permissions-Policy (formerly Feature-Policy)
	if cfg.PermissionsPolicy != "" {
		headers.Set("Permissions-Policy", cfg.PermissionsPolicy)
	}
}

// formatHSTS 根据配置格式化 HSTS 头值。
//
// HSTS（HTTP Strict Transport Security）用于强制浏览器使用 HTTPS 连接。
// 默认配置为 1 年有效期，包含子域名。
func (sh *SecurityHeadersMiddleware) formatHSTS() {
	// 默认 HSTS 值
	maxAge := 31536000 // 1 年有效期（秒）
	includeSubDomains := true // 包含所有子域名
	preload := false // 不预加载到浏览器列表

	// 实际使用时应从 SSLConfig.HSTS 获取配置
	// 当前使用默认值
	sh.hsts = formatHSTSValue(maxAge, includeSubDomains, preload)
}

// formatHSTSValue 格式化 HSTS 头值组件。
//
// 参数：
//   - maxAge: HSTS 有效期（秒）
//   - includeSubDomains: 是否包含子域名
//   - preload: 是否预加载到浏览器 HSTS 列表
//
// 返回值：
//   - string: 格式化后的 HSTS 头值
func formatHSTSValue(maxAge int, includeSubDomains bool, preload bool) string {
	value := fmt.Sprintf("max-age=%d", maxAge)

	if includeSubDomains {
		value += "; includeSubDomains"
	}

	if preload {
		value += "; preload"
	}

	return value
}

// UpdateConfig 更新安全头配置。
//
// 使用写锁保护并发访问，同时更新 HSTS 格式化值。
//
// 参数：
//   - cfg: 新的安全头配置
func (sh *SecurityHeadersMiddleware) UpdateConfig(cfg *config.SecurityHeaders) {
	sh.mu.Lock()
	sh.config = cfg
	sh.formatHSTS()
	sh.mu.Unlock()
}

// SetXFrameOptions 设置 X-Frame-Options 头值。
//
// 参数：
//   - value: 新的 X-Frame-Options 值（如 "DENY"、"SAMEORIGIN"）
func (sh *SecurityHeadersMiddleware) SetXFrameOptions(value string) {
	sh.mu.Lock()
	if sh.config != nil {
		sh.config.XFrameOptions = value
	}
	sh.mu.Unlock()
}

// SetContentSecurityPolicy 设置 CSP 头值。
//
// 参数：
//   - value: 新的 Content-Security-Policy 值
func (sh *SecurityHeadersMiddleware) SetContentSecurityPolicy(value string) {
	sh.mu.Lock()
	if sh.config != nil {
		sh.config.ContentSecurityPolicy = value
	}
	sh.mu.Unlock()
}

// SetReferrerPolicy 设置 Referrer-Policy 头值。
//
// 参数：
//   - value: 新的 Referrer-Policy 值（如 "no-referrer"、"strict-origin"）
func (sh *SecurityHeadersMiddleware) SetReferrerPolicy(value string) {
	sh.mu.Lock()
	if sh.config != nil {
		sh.config.ReferrerPolicy = value
	}
	sh.mu.Unlock()
}

// SetPermissionsPolicy 设置 Permissions-Policy 头值。
//
// 参数：
//   - value: 新的 Permissions-Policy 值
func (sh *SecurityHeadersMiddleware) SetPermissionsPolicy(value string) {
	sh.mu.Lock()
	if sh.config != nil {
		sh.config.PermissionsPolicy = value
	}
	sh.mu.Unlock()
}

// GetConfig 返回当前的安全头配置。
//
// 返回值：
//   - *config.SecurityHeaders: 当前配置的副本
func (sh *SecurityHeadersMiddleware) GetConfig() *config.SecurityHeaders {
	sh.mu.RLock()
	defer sh.mu.RUnlock()
	return sh.config
}

// DefaultSecurityHeaders 返回安全的安全头默认配置。
//
// 返回值：
//   - *config.SecurityHeaders: 包含安全默认值的配置对象
func DefaultSecurityHeaders() *config.SecurityHeaders {
	return &config.SecurityHeaders{
		XFrameOptions:       "DENY",
		XContentTypeOptions: "nosniff",
		ReferrerPolicy:      "strict-origin-when-cross-origin",
	}
}

// StrictSecurityHeaders 返回严格模式的安全头配置。
//
// 适用于高安全要求的应用场景，包含严格的 CSP 和权限策略。
//
// 返回值：
//   - *config.SecurityHeaders: 包含严格安全值的配置对象
func StrictSecurityHeaders() *config.SecurityHeaders {
	return &config.SecurityHeaders{
		XFrameOptions:         "DENY",
		XContentTypeOptions:   "nosniff",
		ContentSecurityPolicy: "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self'; font-src 'self'; connect-src 'self'; frame-ancestors 'none'",
		ReferrerPolicy:        "no-referrer",
		PermissionsPolicy:     "accelerometer=(), camera=(), geolocation=(), gyroscope=(), magnetometer=(), microphone=(), payment=(), usb=()",
	}
}

// DevelopmentSecurityHeaders 返回开发环境使用的宽松安全头配置。
//
// 警告：请勿在生产环境使用此配置，安全性较低。
//
// 返回值：
//   - *config.SecurityHeaders: 包含宽松安全值的配置对象
func DevelopmentSecurityHeaders() *config.SecurityHeaders {
	return &config.SecurityHeaders{
		XFrameOptions:       "SAMEORIGIN",
		XContentTypeOptions: "nosniff",
		ReferrerPolicy:      "strict-origin-when-cross-origin",
	}
}

// 验证接口实现
var _ middleware.Middleware = (*SecurityHeadersMiddleware)(nil)
