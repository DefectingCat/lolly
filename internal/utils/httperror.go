// Package utils provides utility functions for HTTP error handling.
//
// This file implements a unified HTTP error response helper to reduce
// the scattered pattern of ctx.Error throughout the codebase.
package utils

import (
	"crypto/subtle"
	"encoding/json"
	"net"
	"strings"

	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/config"
	"rua.plus/lolly/internal/netutil"
)

// HTTPError represents an HTTP error with a message and status code.
type HTTPError struct {
	Message    string
	StatusCode int
}

// Predefined common HTTP errors.
var (
	ErrNotFound           = HTTPError{Message: "Not Found", StatusCode: fasthttp.StatusNotFound}
	ErrForbidden          = HTTPError{Message: "Forbidden", StatusCode: fasthttp.StatusForbidden}
	ErrUnauthorized       = HTTPError{Message: "Unauthorized", StatusCode: fasthttp.StatusUnauthorized}
	ErrBadGateway         = HTTPError{Message: "Bad Gateway", StatusCode: fasthttp.StatusBadGateway}
	ErrGatewayTimeout     = HTTPError{Message: "Gateway Timeout", StatusCode: fasthttp.StatusGatewayTimeout}
	ErrInternalError      = HTTPError{Message: "Internal Server Error", StatusCode: fasthttp.StatusInternalServerError}
	ErrTooManyRequests    = HTTPError{Message: "Too Many Requests", StatusCode: fasthttp.StatusTooManyRequests}
	ErrServiceUnavailable = HTTPError{Message: "Service Unavailable", StatusCode: fasthttp.StatusServiceUnavailable}
)

// SendError sends an HTTP error response to the client.
func SendError(ctx *fasthttp.RequestCtx, err HTTPError) {
	ctx.Error(err.Message, err.StatusCode)
}

// SendErrorWithDetail sends an HTTP error response with additional detail.
func SendErrorWithDetail(ctx *fasthttp.RequestCtx, err HTTPError, detail string) {
	if detail != "" {
		ctx.Error(err.Message+": "+detail, err.StatusCode)
	} else {
		SendError(ctx, err)
	}
}

// SendJSONError sends a JSON error response.
func SendJSONError(ctx *fasthttp.RequestCtx, status int, errMsg string) {
	ctx.SetContentType("application/json; charset=utf-8")
	ctx.SetStatusCode(status)
	_ = json.NewEncoder(ctx).Encode(struct {
		Error string `json:"error"`
	}{Error: errMsg})
}

// CheckIPAccess checks whether the client IP is in the allowed list.
// If allowed is empty, all access is permitted.
func CheckIPAccess(ctx *fasthttp.RequestCtx, allowed []net.IPNet) bool {
	if len(allowed) == 0 {
		return true
	}

	clientIP := netutil.ExtractClientIPNet(ctx)
	if clientIP == nil {
		return false
	}

	for _, network := range allowed {
		if network.Contains(clientIP) {
			return true
		}
	}

	return false
}

// CheckTokenAuth checks token-based authentication.
// Returns true if auth is disabled or the token matches.
func CheckTokenAuth(ctx *fasthttp.RequestCtx, auth config.CacheAPIAuthConfig) bool {
	if auth.Type == "" || auth.Type == "none" {
		return true
	}

	if auth.Type == "token" {
		authHeader := ctx.Request.Header.Peek("Authorization")
		if len(authHeader) == 0 {
			return false
		}

		authStr := string(authHeader)
		if token, ok := strings.CutPrefix(authStr, "Bearer "); ok {
			return subtle.ConstantTimeCompare([]byte(token), []byte(auth.Token)) == 1
		}

		return subtle.ConstantTimeCompare([]byte(authStr), []byte(auth.Token)) == 1
	}

	return false
}
