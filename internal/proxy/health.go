// Package proxy provides reverse proxy functionality for the Lolly HTTP server.
//
// 此文件实现了针对后端目标的健康检查功能，支持
// 主动健康检查（定期 HTTP 探测）和被动健康检查
//（基于观察到的失败标记目标为不健康）。
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

// HealthChecker 对后端目标执行健康检查。
// 它支持主动（定期 HTTP 探测）和被动（基于失败的）
// 两种健康检查模式。
//
// 当启动后，检查器在后台 goroutine 中运行，定期
// 向每个目标的健康检查端点发送 HTTP GET 请求。
// 返回 2xx 状态码的目标被标记为健康；
// 超时、连接失败或非 2xx 响应将其标记为不健康。
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

// NewHealthChecker 使用指定的目标和配置创建一个新的 HealthChecker。
// 配置定义了检查间隔、超时和健康检查路径。
//
// 如果配置中未指定，将应用默认值：
//   - Interval: 10 秒
//   - Timeout: 5 秒
//   - Path: "/health"
//
// 返回的 HealthChecker 尚未启动；调用 Start() 开始健康检查。
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

// Start 启动后台健康检查进程。
// 它启动一个 goroutine，按照配置的间隔定期检查所有目标。
// Start 是幂等的；在已运行的检查器上调用它不会产生任何效果。
//
// 健康检查进程将持续运行，直到调用 Stop()。
func (h *HealthChecker) Start() {
	if h.running.Load() {
		return
	}

	h.running.Store(true)
	go h.run()
}

// Stop 停止后台健康检查进程。
// 它向后台 goroutine 发送停止信号并等待其完成。
// Stop 是幂等的；在已停止的检查器上调用它不会产生任何效果。
func (h *HealthChecker) Stop() {
	if !h.running.Load() {
		return
	}

	h.running.Store(false)
	close(h.stopCh)
}

// run 是在后台 goroutine 中运行的主要健康检查循环。
// 它对所有目标执行初始检查，然后进入循环，
// 以固定间隔检查目标，直到被停止。
func (h *HealthChecker) run() {
	// 执行初始健康检查
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

// checkAll 对所有配置的目标执行健康检查。
// 它使用 goroutines 并发检查每个目标以最小化延迟。
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

// checkTarget 对单个目标执行健康检查。
// 它向目标的健康检查端点发送 HTTP GET 请求
// 并根据响应更新目标的 Healthy 状态。
//
// 目标被认为健康，如果满足以下条件：
//   - HTTP 请求成功
//   - 响应状态码在 200 到 299 之间
//
// 目标被标记为不健康，如果满足以下条件：
//   - 连接失败
//   - 请求超时
//   - 响应状态码不是 2xx
func (h *HealthChecker) checkTarget(target *loadbalance.Target) {
	// 构建健康检查 URL
	url := target.URL + h.path

	// 准备请求和响应
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	req.SetRequestURI(url)
	req.Header.SetMethod(fasthttp.MethodGet)
	req.Header.Set("User-Agent", "Lolly-HealthChecker/1.0")

	// 执行带超时的健康检查
	err := h.client.DoTimeout(req, resp, h.timeout)

	if err != nil {
		// 连接失败或超时 - 标记为不健康
		loadbalance.SetHealthy(target, false)
		return
	}

	// 检查状态码 - 2xx 为健康
	statusCode := resp.StatusCode()
	if statusCode >= 200 && statusCode < 300 {
		loadbalance.SetHealthy(target, true)
	} else {
		loadbalance.SetHealthy(target, false)
	}
}

// MarkUnhealthy 将目标标记为不健康。
// 此方法用于被动健康检查，代理根据请求处理过程中
// 观察到的失败将目标标记为不健康。
//
// 在代理错误处理中的使用示例：
//
//	if err := forwardRequest(target, req, resp); err != nil {
//	    healthChecker.MarkUnhealthy(target)
//	    // 尝试其他目标或返回错误
//	}
//
// 注意：要再次将目标标记为健康，主动健康检查
// 必须成功。没有 MarkHealthy 方法 - 健康状态只能通过
// 成功的健康检查积极恢复。
func (h *HealthChecker) MarkUnhealthy(target *loadbalance.Target) {
	loadbalance.SetHealthy(target, false)
}

// IsRunning 如果健康检查器当前正在运行，则返回 true。
func (h *HealthChecker) IsRunning() bool {
	return h.running.Load()
}

// GetInterval 返回配置的检查间隔。
func (h *HealthChecker) GetInterval() time.Duration {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.interval
}

// GetTimeout 返回配置的检查超时时间。
func (h *HealthChecker) GetTimeout() time.Duration {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.timeout
}

// GetPath 返回配置的健康检查路径。
func (h *HealthChecker) GetPath() string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.path
}
