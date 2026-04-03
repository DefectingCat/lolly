// Package security provides security-related middleware for the Lolly HTTP server.
//
// This file implements IP access control middleware, supporting CIDR-based
// allow/deny lists with IPv4 and IPv6 support.
//
// Example usage:
//
//	cfg := &config.AccessConfig{
//	    Allow: []string{"192.168.1.0/24", "10.0.0.0/8"},
//	    Deny:  []string{"192.168.2.100/32"},
//	    Default: "deny",
//	}
//
//	access, err := security.NewAccessControl(cfg)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Apply as middleware
//	chain := middleware.NewChain(access)
//	handler := chain.Apply(finalHandler)
//
//go:generate go test -v ./...
package security

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"

	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/config"
	"rua.plus/lolly/internal/middleware"
)

// Action represents the action to take for an IP.
type Action int

const (
	ActionAllow Action = iota // Allow the request
	ActionDeny                // Deny the request (403 Forbidden)
)

// AccessControl implements IP-based access control middleware.
// It checks incoming requests against configured allow/deny CIDR lists.
type AccessControl struct {
	allowList     []net.IPNet // CIDR networks to allow
	denyList      []net.IPNet // CIDR networks to deny
	defaultAction Action      // Default action when no rule matches
	mu            sync.RWMutex
}

// NewAccessControl creates a new access control middleware from configuration.
//
// Parameters:
//   - cfg: Access configuration with allow/deny lists and default action
//
// Returns:
//   - *AccessControl: Configured access control middleware
//   - error: Non-nil if CIDR parsing fails
func NewAccessControl(cfg *config.AccessConfig) (*AccessControl, error) {
	if cfg == nil {
		return nil, errors.New("access config is nil")
	}

	ac := &AccessControl{}

	// Parse allow list
	for _, cidr := range cfg.Allow {
		network, err := parseCIDR(cidr)
		if err != nil {
			return nil, fmt.Errorf("invalid allow CIDR %s: %w", cidr, err)
		}
		ac.allowList = append(ac.allowList, *network)
	}

	// Parse deny list
	for _, cidr := range cfg.Deny {
		network, err := parseCIDR(cidr)
		if err != nil {
			return nil, fmt.Errorf("invalid deny CIDR %s: %w", cidr, err)
		}
		ac.denyList = append(ac.denyList, *network)
	}

	// Set default action
	switch strings.ToLower(cfg.Default) {
	case "allow", "":
		ac.defaultAction = ActionAllow
	case "deny":
		ac.defaultAction = ActionDeny
	default:
		return nil, fmt.Errorf("invalid default action: %s", cfg.Default)
	}

	return ac, nil
}

// Name returns the middleware name.
func (ac *AccessControl) Name() string {
	return "access_control"
}

// Process wraps the next handler with access control logic.
// Requests from denied IPs receive 403 Forbidden.
func (ac *AccessControl) Process(next fasthttp.RequestHandler) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		clientIP := getClientIP(ctx)

		// Check access
		if !ac.Check(clientIP) {
			ctx.Error("Forbidden: Access denied", fasthttp.StatusForbidden)
			return
		}

		next(ctx)
	}
}

// Check checks if an IP address is allowed to access.
// Evaluation order: deny list first, then allow list, then default.
func (ac *AccessControl) Check(ip net.IP) bool {
	ac.mu.RLock()
	defer ac.mu.RUnlock()

	// Check deny list first (explicit deny takes precedence)
	for _, network := range ac.denyList {
		if network.Contains(ip) {
			return false
		}
	}

	// Check allow list
	for _, network := range ac.allowList {
		if network.Contains(ip) {
			return true
		}
	}

	// Return default action
	return ac.defaultAction == ActionAllow
}

// UpdateAllowList updates the allow list dynamically.
func (ac *AccessControl) UpdateAllowList(cidrs []string) error {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	newList := make([]net.IPNet, 0, len(cidrs))
	for _, cidr := range cidrs {
		network, err := parseCIDR(cidr)
		if err != nil {
			return fmt.Errorf("invalid CIDR %s: %w", cidr, err)
		}
		newList = append(newList, *network)
	}

	ac.allowList = newList
	return nil
}

// UpdateDenyList updates the deny list dynamically.
func (ac *AccessControl) UpdateDenyList(cidrs []string) error {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	newList := make([]net.IPNet, 0, len(cidrs))
	for _, cidr := range cidrs {
		network, err := parseCIDR(cidr)
		if err != nil {
			return fmt.Errorf("invalid CIDR %s: %w", cidr, err)
		}
		newList = append(newList, *network)
	}

	ac.denyList = newList
	return nil
}

// SetDefault sets the default action.
func (ac *AccessControl) SetDefault(action string) error {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	switch strings.ToLower(action) {
	case "allow":
		ac.defaultAction = ActionAllow
	case "deny":
		ac.defaultAction = ActionDeny
	default:
		return fmt.Errorf("invalid action: %s", action)
	}

	return nil
}

// parseCIDR parses a CIDR string, supporting both IPv4 and IPv6.
// Handles both full CIDR notation (192.168.1.0/24) and single IPs (192.168.1.1).
func parseCIDR(cidr string) (*net.IPNet, error) {
	// Handle single IP (no /prefix)
	if !strings.Contains(cidr, "/") {
		ip := net.ParseIP(cidr)
		if ip == nil {
			return nil, fmt.Errorf("invalid IP address: %s", cidr)
		}

		// Convert to CIDR with full mask
		if ip.To4() != nil {
			cidr = cidr + "/32"
		} else {
			cidr = cidr + "/128"
		}
	}

	// Parse CIDR
	ip, network, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, err
	}

	// Ensure IP is in canonical form
	network.IP = ip

	return network, nil
}

// getClientIP extracts the client IP from the request context.
// Checks X-Forwarded-For and X-Real-IP headers first, then falls back to RemoteAddr.
func getClientIP(ctx *fasthttp.RequestCtx) net.IP {
	// Check X-Forwarded-For header first
	if xff := ctx.Request.Header.Peek("X-Forwarded-For"); len(xff) > 0 {
		ips := strings.Split(string(xff), ",")
		if len(ips) > 0 {
			ipStr := strings.TrimSpace(ips[0])
			ip := net.ParseIP(ipStr)
			if ip != nil {
				return ip
			}
		}
	}

	// Check X-Real-IP header
	if xri := ctx.Request.Header.Peek("X-Real-IP"); len(xri) > 0 {
		ip := net.ParseIP(string(xri))
		if ip != nil {
			return ip
		}
	}

	// Fall back to RemoteAddr
	if addr := ctx.RemoteAddr(); addr != nil {
		if tcpAddr, ok := addr.(*net.TCPAddr); ok {
			return tcpAddr.IP
		}
		// Parse from string representation
		ipStr := addr.String()
		if idx := strings.LastIndex(ipStr, ":"); idx != -1 {
			ipStr = ipStr[:idx]
		}
		// Remove brackets from IPv6
		ipStr = strings.TrimPrefix(strings.TrimSuffix(ipStr, "]"), "[")
		return net.ParseIP(ipStr)
	}

	return nil
}

// GetStats returns access control statistics.
type AccessStats struct {
	AllowCount int
	DenyCount  int
	Default    string
}

// GetStats returns current access control statistics.
func (ac *AccessControl) GetStats() AccessStats {
	ac.mu.RLock()
	defer ac.mu.RUnlock()

	return AccessStats{
		AllowCount: len(ac.allowList),
		DenyCount:  len(ac.denyList),
		Default:    actionToString(ac.defaultAction),
	}
}

// actionToString converts an Action to its string representation.
func actionToString(action Action) string {
	switch action {
	case ActionAllow:
		return "allow"
	case ActionDeny:
		return "deny"
	default:
		return "unknown"
	}
}

// Verify interface compliance
var _ middleware.Middleware = (*AccessControl)(nil)
