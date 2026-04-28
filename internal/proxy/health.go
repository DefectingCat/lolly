// Package proxy 提供反向代理功能，支持 HTTP、WebSocket 和流式代理。
//
// 此文件实现了针对后端目标的健康检查功能，支持
// 主动健康检查（定期 HTTP 探测）和被动健康检查
// （基于观察到的失败标记目标为不健康）。
//
// 主要功能：
//   - 定期向后端发送健康检查请求
//   - 根据响应状态更新目标健康状态
//   - 支持被动健康检查（基于请求失败）
//
// 作者：xfy
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

// healthPath 默认健康检查路径。
// 当配置中未指定 path 时使用此值。
const healthPath = "/health"

// HealthChecker 对后端目标执行健康检查。
// 它支持主动（定期 HTTP 探测）和被动（基于失败的）
// 两种健康检查模式。
//
// 当启动后，检查器在后台 goroutine 中运行，定期
// 向每个目标的健康检查端点发送 HTTP GET 请求。
// 返回 2xx 状态码的目标被标记为健康；
// 超时、连接失败或非 2xx 响应将其标记为不健康。
//
// 使用示例：
//
//	targets := []*loadbalance.Target{
//	    {URL: "http://backend1:8080"},
//	    {URL: "http://backend2:8080"},
//	}
//	targets[0].Healthy.Store(true)
//	targets[1].Healthy.Store(true)
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
	stopCh           chan struct{}
	client           *fasthttp.Client
	path             string
	targets          []*loadbalance.Target
	interval         time.Duration
	timeout          time.Duration
	running          atomic.Bool
	matcher          HealthMatch                   // 健康检查匹配器
	slowStartManager *loadbalance.SlowStartManager // 慢启动管理器
	wg               sync.WaitGroup                // 等待 run goroutine 退出
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
		path = healthPath
	}

	// 创建健康检查匹配器
	var matcher HealthMatch
	if cfg.Match != nil {
		matcher = NewHealthMatch(&HealthMatchConfig{
			Status:  cfg.Match.Status,
			Body:    cfg.Match.Body,
			Headers: cfg.Match.Headers,
		})
	}
	if matcher == nil {
		matcher = DefaultHealthMatch()
	}

	// 创建慢启动管理器
	var slowStartManager *loadbalance.SlowStartManager
	if cfg.SlowStart > 0 {
		slowStartManager = loadbalance.NewSlowStartManager(cfg.SlowStart)
	}

	return &HealthChecker{
		targets:          targets,
		interval:         interval,
		timeout:          timeout,
		path:             path,
		stopCh:           make(chan struct{}),
		matcher:          matcher,
		slowStartManager: slowStartManager,
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
	if h.slowStartManager != nil {
		h.slowStartManager.Start()
	}
	h.wg.Add(1)
	go h.run()
}

// Stop 停止后台健康检查进程。
// 它向后台 goroutine 发送停止信号并等待其完成。
// Stop 是幂等的；在已停止的检查器上调用它不会产生任何效果。
// Stop 后可以再次调用 Start 重新启动检查器。
func (h *HealthChecker) Stop() {
	if !h.running.CompareAndSwap(true, false) {
		return // 已经停止，直接返回
	}
	close(h.stopCh)
	h.wg.Wait() // 等待 run goroutine 退出
	if h.slowStartManager != nil {
		h.slowStartManager.Stop()
	}
	// 重新创建 stopCh 以支持后续 Start
	h.stopCh = make(chan struct{})
}

// run 是在后台 goroutine 中运行的主要健康检查循环。
// 它对所有目标执行初始检查，然后进入循环，
// 以固定间隔检查目标，直到被停止。
func (h *HealthChecker) run() {
	defer h.wg.Done()

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
//   - matcher.Match 返回 true
//
// 目标被标记为不健康，如果满足以下条件：
//   - 连接失败
//   - 请求超时
//   - matcher.Match 返回 false
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
		h.MarkUnhealthy(target)
		return
	}

	// 提取响应头（小写 key，预分配容量）
	headers := make(map[string]string, 20)
	for key, value := range resp.Header.All() {
		headers[string(key)] = string(value)
	}

	// 使用 matcher 判断健康状态
	statusCode := resp.StatusCode()
	body := resp.Body()
	if h.matcher.Match(statusCode, body, headers) {
		h.MarkHealthy(target)
	} else {
		h.MarkUnhealthy(target)
	}
}

// MarkUnhealthy 将目标标记为不健康。
// 此方法用于被动健康检查，代理根据请求处理过程中
// 观察到的失败将目标标记为不健康。
//
// 同时调用 RecordFailure 记录软失败状态，配合 MaxFails/FailTimeout
// 实现失败计数和冷却机制。
// 同时通知 SlowStartManager 清除慢启动状态。
func (h *HealthChecker) MarkUnhealthy(target *loadbalance.Target) {
	target.Healthy.Store(false)
	target.RecordFailure()
	if h.slowStartManager != nil {
		h.slowStartManager.OnTargetUnhealthy(target)
	}
}

// MarkHealthy 将目标标记为健康。
// 此方法用于故障转移成功后，将之前失败的目标恢复为健康状态。
//
// 同时调用 RecordSuccess 重置软失败状态（failCount/failedUntil），
// 但不修改 Healthy 标志——健康检查器对 Healthy 拥有权威。
// 同时通知 SlowStartManager 开始慢启动。
func (h *HealthChecker) MarkHealthy(target *loadbalance.Target) {
	target.Healthy.Store(true)
	target.RecordSuccess()
	if h.slowStartManager != nil {
		h.slowStartManager.OnTargetHealthy(target)
	}
}

// IsRunning 如果健康检查器当前正在运行，则返回 true。
func (h *HealthChecker) IsRunning() bool {
	return h.running.Load()
}

// GetInterval 返回配置的检查间隔。
func (h *HealthChecker) GetInterval() time.Duration {
	return h.interval
}

// GetTimeout 返回配置的检查超时时间。
func (h *HealthChecker) GetTimeout() time.Duration {
	return h.timeout
}

// GetPath 返回配置的健康检查路径。
func (h *HealthChecker) GetPath() string {
	return h.path
}
