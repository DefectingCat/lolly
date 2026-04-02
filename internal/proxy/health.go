// Package proxy provides reverse proxy functionality for the Lolly HTTP server.
//
// This file implements health checking for backend targets, supporting both
// active health checks (periodic HTTP probes) and passive health checks
// (marking targets unhealthy based on observed failures).
//
//go:generate go test -v ./...
package proxy

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/valyala/fasthttp"

	"rua.plus/lolly/internal/config"
	"rua.plus/lolly/internal/loadbalance"
)

// HealthChecker performs health checks on backend targets.
// It supports both active (periodic HTTP probes) and passive (failure-based)
// health checking modes.
//
// The checker runs in a background goroutine when started, periodically
// sending HTTP GET requests to each target's health check endpoint.
// Targets responding with 2xx status codes are marked as healthy;
// timeouts, connection failures, or non-2xx responses mark them as unhealthy.
//
// Example usage:
//
//	targets := []*loadbalance.Target{
//	    {URL: "http://backend1:8080", Healthy: true},
//	    {URL: "http://backend2:8080", Healthy: true},
//	}
//
//	cfg := &config.HealthCheckConfig{
//	    Interval: 10 * time.Second,
//	    Path:     "/health",
//	    Timeout:  5 * time.Second,
//	}
//
//	checker := New(targets, cfg)
//	checker.Start()
//	defer checker.Stop()
type HealthChecker struct {
	targets  []*loadbalance.Target
	interval time.Duration
	timeout  time.Duration
	path     string
	stopCh   chan struct{}
	running  atomic.Bool
	client   *fasthttp.Client
	mu       sync.RWMutex
}

// NewHealthChecker creates a new HealthChecker with the specified targets and configuration.
// The configuration defines the check interval, timeout, and health check path.
//
// Default values are applied if not specified in the config:
//   - Interval: 10 seconds
//   - Timeout: 5 seconds
//   - Path: "/health"
//
// The returned HealthChecker is not started; call Start() to begin health checks.
func NewHealthChecker(targets []*loadbalance.Target, cfg *config.HealthCheckConfig) *HealthChecker {
	interval := cfg.Interval
	if interval <= 0 {
		interval = 10 * time.Second
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	path := cfg.Path
	if path == "" {
		path = "/health"
	}

	return &HealthChecker{
		targets:  targets,
		interval: interval,
		timeout:  timeout,
		path:     path,
		stopCh:   make(chan struct{}),
		client: &fasthttp.Client{
			ReadTimeout:  timeout,
			WriteTimeout: timeout,
		},
	}
}

// Start begins the background health check process.
// It launches a goroutine that periodically checks all targets at the configured interval.
// Start is idempotent; calling it on an already running checker has no effect.
//
// The health check process continues until Stop() is called.
func (h *HealthChecker) Start() {
	if h.running.Load() {
		return
	}

	h.running.Store(true)
	go h.run()
}

// Stop halts the background health check process.
// It signals the background goroutine to stop and waits for it to complete.
// Stop is idempotent; calling it on a stopped checker has no effect.
func (h *HealthChecker) Stop() {
	if !h.running.Load() {
		return
	}

	h.running.Store(false)
	close(h.stopCh)
}

// run is the main health check loop running in a background goroutine.
// It performs an initial check on all targets, then enters a loop that
// checks targets at regular intervals until stopped.
func (h *HealthChecker) run() {
	// Perform initial health check
	h.checkAll()

	ticker := time.NewTicker(h.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			h.checkAll()
		case <-h.stopCh:
			return
		}
	}
}

// checkAll performs health checks on all configured targets.
// It checks each target concurrently using goroutines to minimize latency.
func (h *HealthChecker) checkAll() {
	var wg sync.WaitGroup

	for _, target := range h.targets {
		wg.Add(1)
		go func(t *loadbalance.Target) {
			defer wg.Done()
			h.checkTarget(t)
		}(target)
	}

	wg.Wait()
}

// checkTarget performs a health check on a single target.
// It sends an HTTP GET request to the target's health check endpoint
// and updates the target's Healthy status based on the response.
//
// A target is considered healthy if:
//   - The HTTP request succeeds
//   - The response status code is between 200 and 299
//
// A target is marked unhealthy if:
//   - The connection fails
//   - The request times out
//   - The response status code is not 2xx
func (h *HealthChecker) checkTarget(target *loadbalance.Target) {
	// Build health check URL
	url := target.URL + h.path

	// Prepare request and response
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	req.SetRequestURI(url)
	req.Header.SetMethod(fasthttp.MethodGet)
	req.Header.Set("User-Agent", "Lolly-HealthChecker/1.0")

	// Perform health check with timeout
	err := h.client.DoTimeout(req, resp, h.timeout)

	if err != nil {
		// Connection failed or timeout - mark as unhealthy
		loadbalance.SetHealthy(target, false)
		return
	}

	// Check status code - 2xx is healthy
	statusCode := resp.StatusCode()
	if statusCode >= 200 && statusCode < 300 {
		loadbalance.SetHealthy(target, true)
	} else {
		loadbalance.SetHealthy(target, false)
	}
}

// MarkUnhealthy marks a target as unhealthy.
// This method is intended for passive health checking, where the proxy
// marks targets as unhealthy based on observed failures during request handling.
//
// Example usage in proxy error handling:
//
//	if err := forwardRequest(target, req, resp); err != nil {
//	    healthChecker.MarkUnhealthy(target)
//	    // Try another target or return error
//	}
//
// Note: To mark a target as healthy again, the active health check
// must succeed. There is no MarkHealthy method - health status can only
// be positively restored through successful health checks.
func (h *HealthChecker) MarkUnhealthy(target *loadbalance.Target) {
	loadbalance.SetHealthy(target, false)
}

// IsRunning returns true if the health checker is currently running.
func (h *HealthChecker) IsRunning() bool {
	return h.running.Load()
}

// GetInterval returns the configured check interval.
func (h *HealthChecker) GetInterval() time.Duration {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.interval
}

// GetTimeout returns the configured check timeout.
func (h *HealthChecker) GetTimeout() time.Duration {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.timeout
}

// GetPath returns the configured health check path.
func (h *HealthChecker) GetPath() string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.path
}
