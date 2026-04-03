// Package security provides security-related middleware for the Lolly HTTP server.
//
// This file implements security headers middleware, adding standard security
// headers to responses to protect against common web vulnerabilities.
//
// Headers implemented:
//   - X-Frame-Options: Prevent clickjacking
//   - X-Content-Type-Options: Prevent MIME sniffing
//   - Content-Security-Policy: Control resource loading (XSS protection)
//   - Strict-Transport-Security: Enforce HTTPS (HSTS)
//   - Referrer-Policy: Control referrer information
//   - Permissions-Policy: Control browser features
//
// Example usage:
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
//go:generate go test -v ./...
package security

import (
	"fmt"
	"sync"

	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/config"
	"rua.plus/lolly/internal/middleware"
)

// SecurityHeadersMiddleware adds security-related headers to responses.
type SecurityHeadersMiddleware struct {
	config *config.SecurityHeaders
	hsts   string // Pre-formatted HSTS header value
	mu     sync.RWMutex
}

// NewSecurityHeaders creates a new security headers middleware.
//
// Parameters:
//   - cfg: Security headers configuration (can be nil for defaults)
//
// Returns:
//   - *SecurityHeadersMiddleware: Configured middleware with default safe values
func NewSecurityHeaders(cfg *config.SecurityHeaders) *SecurityHeadersMiddleware {
	sh := &SecurityHeadersMiddleware{}

	if cfg != nil {
		sh.config = cfg
	} else {
		// Use secure defaults
		sh.config = &config.SecurityHeaders{
			XFrameOptions:       "DENY",
			XContentTypeOptions: "nosniff",
			ReferrerPolicy:      "strict-origin-when-cross-origin",
		}
	}

	// Pre-format HSTS header if configured
	sh.formatHSTS()

	return sh
}

// Name returns the middleware name.
func (sh *SecurityHeadersMiddleware) Name() string {
	return "security_headers"
}

// Process wraps the next handler, adding security headers to the response.
func (sh *SecurityHeadersMiddleware) Process(next fasthttp.RequestHandler) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		// Call next handler first
		next(ctx)

		// Add security headers to response
		sh.addHeaders(ctx)
	}
}

// addHeaders adds all configured security headers to the response.
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

// formatHSTS formats the HSTS header value from configuration.
func (sh *SecurityHeadersMiddleware) formatHSTS() {
	// Default HSTS values
	maxAge := 31536000 // 1 year
	includeSubDomains := true
	preload := false

	// These would come from SSLConfig.HSTS in real usage
	// For now, use defaults
	sh.hsts = formatHSTSValue(maxAge, includeSubDomains, preload)
}

// formatHSTSValue formats HSTS header value components.
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

// UpdateConfig updates the security headers configuration.
func (sh *SecurityHeadersMiddleware) UpdateConfig(cfg *config.SecurityHeaders) {
	sh.mu.Lock()
	sh.config = cfg
	sh.formatHSTS()
	sh.mu.Unlock()
}

// SetXFrameOptions sets the X-Frame-Options header value.
func (sh *SecurityHeadersMiddleware) SetXFrameOptions(value string) {
	sh.mu.Lock()
	if sh.config != nil {
		sh.config.XFrameOptions = value
	}
	sh.mu.Unlock()
}

// SetContentSecurityPolicy sets the CSP header value.
func (sh *SecurityHeadersMiddleware) SetContentSecurityPolicy(value string) {
	sh.mu.Lock()
	if sh.config != nil {
		sh.config.ContentSecurityPolicy = value
	}
	sh.mu.Unlock()
}

// SetReferrerPolicy sets the Referrer-Policy header value.
func (sh *SecurityHeadersMiddleware) SetReferrerPolicy(value string) {
	sh.mu.Lock()
	if sh.config != nil {
		sh.config.ReferrerPolicy = value
	}
	sh.mu.Unlock()
}

// SetPermissionsPolicy sets the Permissions-Policy header value.
func (sh *SecurityHeadersMiddleware) SetPermissionsPolicy(value string) {
	sh.mu.Lock()
	if sh.config != nil {
		sh.config.PermissionsPolicy = value
	}
	sh.mu.Unlock()
}

// GetConfig returns the current configuration.
func (sh *SecurityHeadersMiddleware) GetConfig() *config.SecurityHeaders {
	sh.mu.RLock()
	defer sh.mu.RUnlock()
	return sh.config
}

// DefaultSecurityHeaders returns a SecurityHeaders config with safe defaults.
func DefaultSecurityHeaders() *config.SecurityHeaders {
	return &config.SecurityHeaders{
		XFrameOptions:       "DENY",
		XContentTypeOptions: "nosniff",
		ReferrerPolicy:      "strict-origin-when-cross-origin",
	}
}

// StrictSecurityHeaders returns a SecurityHeaders config with strict values.
// Suitable for high-security applications.
func StrictSecurityHeaders() *config.SecurityHeaders {
	return &config.SecurityHeaders{
		XFrameOptions:         "DENY",
		XContentTypeOptions:   "nosniff",
		ContentSecurityPolicy: "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self'; font-src 'self'; connect-src 'self'; frame-ancestors 'none'",
		ReferrerPolicy:        "no-referrer",
		PermissionsPolicy:     "accelerometer=(), camera=(), geolocation=(), gyroscope=(), magnetometer=(), microphone=(), payment=(), usb=()",
	}
}

// DevelopmentSecurityHeaders returns relaxed security headers for development.
// WARNING: Do not use in production.
func DevelopmentSecurityHeaders() *config.SecurityHeaders {
	return &config.SecurityHeaders{
		XFrameOptions:       "SAMEORIGIN",
		XContentTypeOptions: "nosniff",
		ReferrerPolicy:      "strict-origin-when-cross-origin",
	}
}

// Verify interface compliance
var _ middleware.Middleware = (*SecurityHeadersMiddleware)(nil)
