// Package utils provides utility functions for HTTP error handling.
//
// This file implements a unified HTTP error response helper to reduce
// the scattered pattern of ctx.Error throughout the codebase.
package utils

import "github.com/valyala/fasthttp"

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
