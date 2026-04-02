// Package proxy provides reverse proxy functionality for the Lolly HTTP server.
//
// This package implements a high-performance reverse proxy using fasthttp.HostClient
// for connection pooling and automatic keep-alive management. It supports load balancing,
// WebSocket forwarding, custom headers, and comprehensive timeout configurations.
//
// Example usage:
//
//	targets := []*loadbalance.Target{
//	    {URL: "http://backend1:8080", Weight: 1, Healthy: true},
//	    {URL: "http://backend2:8080", Weight: 2, Healthy: true},
//	}
//
//	proxyConfig := &config.ProxyConfig{
//	    Path:        "/api",
//	    LoadBalance: "weighted_round_robin",
//	    Timeout: config.ProxyTimeout{
//	        Connect: 5 * time.Second,
//	        Read:    30 * time.Second,
//	        Write:   30 * time.Second,
//	    },
//	}
//
//	p, err := proxy.NewProxy(proxyConfig, targets)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Use p.ServeHTTP as fasthttp request handler
//
//go:generate go test -v ./...
package proxy

import (
	"errors"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/config"
	"rua.plus/lolly/internal/loadbalance"
)

// Proxy represents a reverse proxy instance that forwards HTTP requests to backend targets.
// It manages connection pools for each target and provides load balancing capabilities.
type Proxy struct {
	targets  []*loadbalance.Target
	clients  map[string]*fasthttp.HostClient // key: target URL
	balancer loadbalance.Balancer
	config   *config.ProxyConfig
	mu       sync.RWMutex
}

// NewProxy creates a new reverse proxy instance with the given configuration and targets.
// It initializes the load balancer based on the config and creates HostClients for each target.
//
// Parameters:
//   - cfg: Proxy configuration including timeouts, headers, and load balancing strategy
//   - targets: List of backend targets to proxy requests to
//
// Returns:
//   - *Proxy: Configured proxy instance ready to serve requests
//   - error: Non-nil if initialization fails (invalid config, no healthy targets, etc.)
func NewProxy(cfg *config.ProxyConfig, targets []*loadbalance.Target) (*Proxy, error) {
	if cfg == nil {
		return nil, errors.New("proxy config is nil")
	}

	if len(targets) == 0 {
		return nil, errors.New("no proxy targets provided")
	}

	// Create balancer based on configuration
	balancer, err := createBalancer(cfg.LoadBalance)
	if err != nil {
		return nil, err
	}

	p := &Proxy{
		targets:  targets,
		clients:  make(map[string]*fasthttp.HostClient),
		balancer: balancer,
		config:   cfg,
	}

	// Initialize HostClient for each target
	for _, target := range targets {
		if target.URL == "" {
			continue
		}

		client := createHostClient(target.URL, cfg.Timeout)
		p.clients[target.URL] = client
	}

	return p, nil
}

// createBalancer creates a load balancer based on the configured algorithm.
func createBalancer(algorithm string) (loadbalance.Balancer, error) {
	switch algorithm {
	case "round_robin", "":
		return loadbalance.NewRoundRobin(), nil
	case "weighted_round_robin":
		return loadbalance.NewWeightedRoundRobin(), nil
	case "least_conn":
		return loadbalance.NewLeastConnections(), nil
	case "ip_hash":
		return loadbalance.NewIPHash(), nil
	default:
		return nil, errors.New("unsupported load balance algorithm: " + algorithm)
	}
}

// createHostClient creates a fasthttp.HostClient for a target URL.
func createHostClient(targetURL string, timeout config.ProxyTimeout) *fasthttp.HostClient {
	// Parse host and scheme from target URL
	addr := targetURL
	isTLS := false

	if strings.HasPrefix(targetURL, "http://") {
		addr = targetURL[7:]
	} else if strings.HasPrefix(targetURL, "https://") {
		addr = targetURL[8:]
		isTLS = true
	}

	// Remove path if present, keep only host:port
	if idx := strings.Index(addr, "/"); idx != -1 {
		addr = addr[:idx]
	}

	client := &fasthttp.HostClient{
		Addr:                   addr,
		IsTLS:                  isTLS,
		ReadTimeout:            timeout.Read,
		WriteTimeout:           timeout.Write,
		MaxIdleConnDuration:    60 * time.Second,
		MaxConns:               100,
		MaxConnWaitTimeout:     timeout.Connect,
		RetryIf:                nil, // Disable automatic retries
		DisablePathNormalizing: false,
		SecureErrorLogMessage:  false,
	}

	return client
}

// ServeHTTP handles the incoming HTTP request by forwarding it to a selected backend target.
// It implements the fasthttp request handler interface.
//
// The method:
// 1. Selects a target using load balancing
// 2. Prepares the request (modifies headers)
// 3. Forwards the request to the backend
// 4. Copies the response back to the client
//
// If no healthy targets are available, returns 502 Bad Gateway.
// If the backend request fails, returns appropriate error response.
func (p *Proxy) ServeHTTP(ctx *fasthttp.RequestCtx) {
	// Select target using load balancer
	target := p.selectTarget(ctx)
	if target == nil {
		ctx.Error("Bad Gateway: no healthy upstream", fasthttp.StatusBadGateway)
		return
	}

	// Get the client for selected target
	client := p.getClient(target.URL)
	if client == nil {
		ctx.Error("Bad Gateway: upstream client unavailable", fasthttp.StatusBadGateway)
		return
	}

	// Increment connection count for least_connections tracking
	loadbalance.IncrementConnections(target)
	defer loadbalance.DecrementConnections(target)

	// Check if this is a WebSocket upgrade request
	if isWebSocketRequest(ctx) {
		p.handleWebSocket(ctx, target, client)
		return
	}

	// Prepare request
	req := &ctx.Request

	// Modify request headers
	p.modifyRequestHeaders(ctx, target)

	// Perform the proxy request
	err := client.Do(req, &ctx.Response)
	if err != nil {
		// Handle different error types
		if errors.Is(err, fasthttp.ErrTimeout) {
			ctx.Error("Gateway Timeout", fasthttp.StatusGatewayTimeout)
		} else if errors.Is(err, fasthttp.ErrConnectionClosed) {
			ctx.Error("Bad Gateway: upstream connection closed", fasthttp.StatusBadGateway)
		} else {
			ctx.Error("Bad Gateway", fasthttp.StatusBadGateway)
		}
		return
	}

	// Modify response headers
	p.modifyResponseHeaders(ctx)
}

// selectTarget selects a backend target using the configured load balancer.
// It extracts the client IP from the request for IP hash balancing.
// Returns nil if no healthy targets are available.
func (p *Proxy) selectTarget(ctx *fasthttp.RequestCtx) *loadbalance.Target {
	p.mu.RLock()
	balancer := p.balancer
	targets := p.targets
	p.mu.RUnlock()

	if len(targets) == 0 {
		return nil
	}

	// For IPHash balancer, extract client IP
	if ipHash, ok := balancer.(*loadbalance.IPHash); ok {
		clientIP := getClientIP(ctx)
		return ipHash.SelectByIP(targets, clientIP)
	}

	return balancer.Select(targets)
}

// getClientIP extracts the client IP address from the request context.
func getClientIP(ctx *fasthttp.RequestCtx) string {
	// Check X-Forwarded-For header first
	if xff := ctx.Request.Header.Peek("X-Forwarded-For"); len(xff) > 0 {
		ips := strings.Split(string(xff), ",")
		if len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}

	// Check X-Real-IP header
	if xri := ctx.Request.Header.Peek("X-Real-IP"); len(xri) > 0 {
		return string(xri)
	}

	// Fall back to RemoteAddr
	if addr := ctx.RemoteAddr(); addr != nil {
		if tcpAddr, ok := addr.(*net.TCPAddr); ok {
			return tcpAddr.IP.String()
		}
		return addr.String()
	}

	return ""
}

// getClient returns the HostClient for a given target URL.
func (p *Proxy) getClient(targetURL string) *fasthttp.HostClient {
	p.mu.RLock()
	client := p.clients[targetURL]
	p.mu.RUnlock()
	return client
}

// modifyRequestHeaders modifies the request headers before forwarding to backend.
// It adds standard proxy headers and applies custom header configurations.
func (p *Proxy) modifyRequestHeaders(ctx *fasthttp.RequestCtx, target *loadbalance.Target) {
	headers := &ctx.Request.Header

	// Add X-Real-IP header
	clientIP := getClientIP(ctx)
	if clientIP != "" {
		headers.Set("X-Real-IP", clientIP)
	}

	// Add/Append X-Forwarded-For header
	existingXFF := headers.Peek("X-Forwarded-For")
	if len(existingXFF) > 0 {
		headers.Set("X-Forwarded-For", string(existingXFF)+", "+clientIP)
	} else {
		headers.Set("X-Forwarded-For", clientIP)
	}

	// Add X-Forwarded-Host header
	host := string(ctx.Host())
	if host != "" {
		headers.Set("X-Forwarded-Host", host)
	}

	// Add X-Forwarded-Proto header
	proto := "http"
	if ctx.IsTLS() {
		proto = "https"
	}
	headers.Set("X-Forwarded-Proto", proto)

	// Set custom request headers from config
	if p.config.Headers.SetRequest != nil {
		for key, value := range p.config.Headers.SetRequest {
			headers.Set(key, value)
		}
	}

	// Remove configured headers
	if len(p.config.Headers.Remove) > 0 {
		for _, key := range p.config.Headers.Remove {
			headers.Del(key)
		}
	}
}

// modifyResponseHeaders modifies the response headers before sending to client.
func (p *Proxy) modifyResponseHeaders(ctx *fasthttp.RequestCtx) {
	// Set custom response headers from config
	if p.config.Headers.SetResponse != nil {
		for key, value := range p.config.Headers.SetResponse {
			ctx.Response.Header.Set(key, value)
		}
	}
}

// isWebSocketRequest checks if the request is a WebSocket upgrade request.
func isWebSocketRequest(ctx *fasthttp.RequestCtx) bool {
	// Check Connection header
	connection := ctx.Request.Header.Peek("Connection")
	if !strings.EqualFold(string(connection), "upgrade") {
		// Also check for "Upgrade" substring (e.g., "keep-alive, Upgrade")
		if !strings.Contains(strings.ToLower(string(connection)), "upgrade") {
			return false
		}
	}

	// Check Upgrade header
	upgrade := ctx.Request.Header.Peek("Upgrade")
	return strings.EqualFold(string(upgrade), "websocket")
}

// handleWebSocket handles WebSocket upgrade requests.
// For now, it returns 501 Not Implemented as WebSocket proxying
// requires special handling beyond HTTP.
func (p *Proxy) handleWebSocket(ctx *fasthttp.RequestCtx, target *loadbalance.Target, client *fasthttp.HostClient) {
	// WebSocket proxying requires raw TCP connection handling
	// which is beyond the scope of basic HTTP proxying
	// This can be implemented later using a TCP bridge
	ctx.Error("WebSocket proxying not implemented", fasthttp.StatusNotImplemented)
}

// UpdateTargets updates the proxy targets and reinitializes clients.
// This is useful for dynamic configuration updates.
func (p *Proxy) UpdateTargets(targets []*loadbalance.Target) error {
	if len(targets) == 0 {
		return errors.New("no targets provided")
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	// Clear old clients
	p.clients = make(map[string]*fasthttp.HostClient)

	// Initialize new clients
	for _, target := range targets {
		if target.URL == "" {
			continue
		}

		client := createHostClient(target.URL, p.config.Timeout)
		p.clients[target.URL] = client
	}

	p.targets = targets
	return nil
}

// GetTargets returns the current list of targets.
func (p *Proxy) GetTargets() []*loadbalance.Target {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.targets
}

// GetConfig returns the proxy configuration.
func (p *Proxy) GetConfig() *config.ProxyConfig {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.config
}
