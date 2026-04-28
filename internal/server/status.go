// Package server 提供了带中间件支持、虚拟主机和状态监控功能的 HTTP 服务器。
//
// 该文件实现了服务器状态监控处理器，用于暴露服务器运行状态信息，
// 支持基于 IP 的访问控制，保护状态端点不被未授权访问。
//
// 主要功能：
//   - 状态信息收集：版本、运行时间、连接数、请求数、流量统计
//   - IP 访问控制：通过 CIDR 配置允许访问的 IP 范围
//   - JSON 响应：返回结构化的状态信息
//
// 作者：xfy
package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"strings"
	"time"

	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/config"
	"rua.plus/lolly/internal/utils"
)

// StatusHandler 状态监控处理器。
//
// 提供 HTTP 状态端点，返回服务器运行状态信息。
// 支持基于 IP 的访问控制，限制状态端点的访问权限。
//
// 注意事项：
//   - 状态端点可能暴露敏感信息，建议配置 IP 白名单
//   - 所有方法均为并发安全
type StatusHandler struct {
	server  *Server
	path    string
	format  string
	allowed []net.IPNet
}

// Status 状态响应结构。
//
// 包含服务器运行的各种统计信息，以 JSON 格式返回给客户端。
type Status struct {
	Cache         *CacheStats       `json:"cache,omitempty"`
	Pool          *PoolStats        `json:"pool,omitempty"`
	SSL           *SSLStatus        `json:"ssl,omitempty"`
	Version       string            `json:"version"`
	Upstreams     []UpstreamStatus  `json:"upstreams,omitempty"`
	RateLimits    []RateLimitStatus `json:"rate_limits,omitempty"`
	Uptime        time.Duration     `json:"uptime"`
	Connections   int64             `json:"connections"`
	Requests      int64             `json:"requests"`
	BytesSent     int64             `json:"bytes_sent"`
	BytesReceived int64             `json:"bytes_received"`
}

// UpstreamStatus Upstream 统计信息。
type UpstreamStatus struct {
	Name           string         `json:"name"`            // Upstream 名称
	Targets        []TargetStatus `json:"targets"`         // 目标服务器列表
	HealthyCount   int            `json:"healthy_count"`   // 健康目标数
	UnhealthyCount int            `json:"unhealthy_count"` // 不健康目标数
	LatencyP50     float64        `json:"latency_p50_ms"`  // P50 延迟（毫秒）
	LatencyP95     float64        `json:"latency_p95_ms"`  // P95 延迟（毫秒）
	LatencyP99     float64        `json:"latency_p99_ms"`  // P99 延迟（毫秒）
}

// TargetStatus 目标服务器状态。
type TargetStatus struct {
	URL     string `json:"url"`        // 目标 URL
	Healthy bool   `json:"healthy"`    // 是否健康
	Latency int64  `json:"latency_ms"` // 延迟（毫秒）
}

// RateLimitStatus 限流统计信息。
type RateLimitStatus struct {
	ZoneName string `json:"zone_name"` // 限流区域名称
	Requests int64  `json:"requests"`  // 请求数
	Limit    int64  `json:"limit"`     // 限制值
	Rejected int64  `json:"rejected"`  // 拒绝数
}

// SSLStatus SSL 统计信息。
type SSLStatus struct {
	Handshakes    int64   `json:"handshakes"`         // 握手次数
	SessionReused int64   `json:"session_reused"`     // 会话复用次数
	ReuseRate     float64 `json:"reuse_rate_percent"` // 会话复用率（百分比）
}

// CacheStats 缓存统计信息。
type CacheStats struct {
	FileCache  FileCacheStats  `json:"file_cache"`  // 文件缓存统计
	ProxyCache ProxyCacheStats `json:"proxy_cache"` // 代理缓存统计
}

// FileCacheStats 文件缓存统计。
type FileCacheStats struct {
	Entries    int64 `json:"entries"`     // 当前缓存条目数
	MaxEntries int64 `json:"max_entries"` // 最大条目数
	Size       int64 `json:"size"`        // 当前缓存大小
	MaxSize    int64 `json:"max_size"`    // 最大缓存大小
}

// ProxyCacheStats 代理缓存统计。
type ProxyCacheStats struct {
	Entries int `json:"entries"` // 当前缓存条目数
	Pending int `json:"pending"` // 等待生成的缓存条目数
}

// NewStatusHandler 创建状态监控处理器。
//
// 根据配置创建处理器实例，解析允许访问的 IP 列表。
// 支持 CIDR 格式和单个 IP 格式。
//
// 参数：
//   - server: 服务器实例，用于收集状态数据
//   - cfg: 状态监控配置，包含路径和允许的 IP 列表
//
// 返回值：
//   - *StatusHandler: 配置好的状态处理器
//   - error: IP 解析失败时返回非 nil 错误
func NewStatusHandler(server *Server, cfg *config.StatusConfig) (*StatusHandler, error) {
	h := &StatusHandler{
		server: server,
		path:   cfg.Path,
		format: cfg.Format,
	}

	// 默认格式为 json
	if h.format == "" {
		h.format = "json"
	}

	// 解析允许的 IP 列表
	for _, cidr := range cfg.Allow {
		// 处理 localhost 特殊情况
		if cidr == "localhost" {
			// localhost 解析为 127.0.0.1 和 ::1
			_, v4Network, _ := net.ParseCIDR("127.0.0.1/32")
			_, v6Network, _ := net.ParseCIDR("::1/128")
			if v4Network != nil {
				h.allowed = append(h.allowed, *v4Network)
			}
			if v6Network != nil {
				h.allowed = append(h.allowed, *v6Network)
			}
			continue
		}

		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			// 尝试作为单个 IP 解析
			ip := net.ParseIP(cidr)
			if ip == nil {
				return nil, err
			}
			// 转换为 CIDR 格式
			if ip.To4() != nil {
				_, network, _ = net.ParseCIDR(cidr + "/32")
			} else {
				_, network, _ = net.ParseCIDR(cidr + "/128")
			}
		}
		if network != nil {
			h.allowed = append(h.allowed, *network)
		}
	}

	return h, nil
}

// Path 返回状态端点路径。
//
// 如果配置中未指定路径，则返回默认路径 "/_status"。
//
// 返回值：
//   - string: 状态端点的 URL 路径
func (h *StatusHandler) Path() string {
	if h.path == "" {
		return "/_status"
	}
	return h.path
}

// ServeHTTP 处理状态请求。
//
// 验证客户端 IP 权限，收集并返回服务器状态信息。
// 未授权访问返回 403 Forbidden，授权访问根据配置格式返回状态。
//
// 参数：
//   - ctx: FastHTTP 请求上下文
func (h *StatusHandler) ServeHTTP(ctx *fasthttp.RequestCtx) {
	// 步骤1: 检查 IP 访问权限
	if !utils.CheckIPAccess(ctx, h.allowed) {
		utils.SendErrorWithDetail(ctx, utils.ErrForbidden, "Access denied")
		return
	}

	// 步骤2: 收集状态数据
	status := h.collectStatus()

	// 步骤3: 根据格式返回响应
	switch h.format {
	case "prometheus":
		h.servePrometheus(ctx, status)
		return
	case "html":
		h.serveHTML(ctx, status)
		return
	case "text":
		h.serveText(ctx, status)
		return
	default:
		h.serveJSON(ctx, status)
		return
	}
}

// servePrometheus 以 Prometheus 格式输出指标。
func (h *StatusHandler) servePrometheus(ctx *fasthttp.RequestCtx, status *Status) {
	ctx.SetContentType("text/plain; charset=utf-8")
	ctx.SetStatusCode(fasthttp.StatusOK)

	var buf strings.Builder

	// 基础指标
	buf.WriteString("# HELP lolly_version Server version info\n")
	buf.WriteString("# TYPE lolly_version gauge\n")
	fmt.Fprintf(&buf, "lolly_version{version=\"%s\"} 1\n", status.Version)

	buf.WriteString("\n# HELP lolly_uptime_seconds Server uptime in seconds\n")
	buf.WriteString("# TYPE lolly_uptime_seconds gauge\n")
	fmt.Fprintf(&buf, "lolly_uptime_seconds %.0f\n", status.Uptime.Seconds())

	buf.WriteString("\n# HELP lolly_connections Current active connections\n")
	buf.WriteString("# TYPE lolly_connections gauge\n")
	fmt.Fprintf(&buf, "lolly_connections %d\n", status.Connections)

	buf.WriteString("\n# HELP lolly_requests_total Total requests processed\n")
	buf.WriteString("# TYPE lolly_requests_total counter\n")
	fmt.Fprintf(&buf, "lolly_requests_total %d\n", status.Requests)

	buf.WriteString("\n# HELP lolly_bytes_sent_total Total bytes sent\n")
	buf.WriteString("# TYPE lolly_bytes_sent_total counter\n")
	fmt.Fprintf(&buf, "lolly_bytes_sent_total %d\n", status.BytesSent)

	buf.WriteString("\n# HELP lolly_bytes_received_total Total bytes received\n")
	buf.WriteString("# TYPE lolly_bytes_received_total counter\n")
	fmt.Fprintf(&buf, "lolly_bytes_received_total %d\n", status.BytesReceived)

	// 缓存指标
	if status.Cache != nil {
		buf.WriteString("\n# HELP lolly_cache_entries Number of cache entries\n")
		buf.WriteString("# TYPE lolly_cache_entries gauge\n")
		fmt.Fprintf(&buf, "lolly_cache_entries{type=\"file\"} %d\n", status.Cache.FileCache.Entries)
		fmt.Fprintf(&buf, "lolly_cache_entries{type=\"proxy\"} %d\n", status.Cache.ProxyCache.Entries)

		buf.WriteString("\n# HELP lolly_cache_size_bytes Cache size in bytes\n")
		buf.WriteString("# TYPE lolly_cache_size_bytes gauge\n")
		fmt.Fprintf(&buf, "lolly_cache_size_bytes %d\n", status.Cache.FileCache.Size)

		buf.WriteString("\n# HELP lolly_cache_pending Number of pending cache requests\n")
		buf.WriteString("# TYPE lolly_cache_pending gauge\n")
		fmt.Fprintf(&buf, "lolly_cache_pending %d\n", status.Cache.ProxyCache.Pending)
	}

	// Pool 指标
	if status.Pool != nil {
		buf.WriteString("\n# HELP lolly_pool_workers Number of goroutine pool workers\n")
		buf.WriteString("# TYPE lolly_pool_workers gauge\n")
		fmt.Fprintf(&buf, "lolly_pool_workers{state=\"total\"} %d\n", status.Pool.Workers)
		fmt.Fprintf(&buf, "lolly_pool_workers{state=\"idle\"} %d\n", status.Pool.IdleWorkers)

		buf.WriteString("\n# HELP lolly_pool_queue_length Queue length\n")
		buf.WriteString("# TYPE lolly_pool_queue_length gauge\n")
		fmt.Fprintf(&buf, "lolly_pool_queue_length %d\n", status.Pool.QueueLen)
	}

	// Upstream 指标
	for _, upstream := range status.Upstreams {
		buf.WriteString("\n# HELP lolly_upstream_healthy_count Number of healthy upstream targets\n")
		buf.WriteString("# TYPE lolly_upstream_healthy_count gauge\n")
		fmt.Fprintf(&buf, "lolly_upstream_healthy_count{name=\"%s\"} %d\n", upstream.Name, upstream.HealthyCount)

		buf.WriteString("\n# HELP lolly_upstream_unhealthy_count Number of unhealthy upstream targets\n")
		buf.WriteString("# TYPE lolly_upstream_unhealthy_count gauge\n")
		fmt.Fprintf(&buf, "lolly_upstream_unhealthy_count{name=\"%s\"} %d\n", upstream.Name, upstream.UnhealthyCount)

		buf.WriteString("\n# HELP lolly_upstream_latency_ms Upstream latency in milliseconds\n")
		buf.WriteString("# TYPE lolly_upstream_latency_ms gauge\n")
		fmt.Fprintf(&buf, "lolly_upstream_latency_ms{name=\"%s\",quantile=\"0.5\"} %.2f\n", upstream.Name, upstream.LatencyP50)
		fmt.Fprintf(&buf, "lolly_upstream_latency_ms{name=\"%s\",quantile=\"0.95\"} %.2f\n", upstream.Name, upstream.LatencyP95)
		fmt.Fprintf(&buf, "lolly_upstream_latency_ms{name=\"%s\",quantile=\"0.99\"} %.2f\n", upstream.Name, upstream.LatencyP99)
	}

	// SSL 指标
	if status.SSL != nil {
		buf.WriteString("\n# HELP lolly_ssl_handshakes_total Total SSL handshakes\n")
		buf.WriteString("# TYPE lolly_ssl_handshakes_total counter\n")
		fmt.Fprintf(&buf, "lolly_ssl_handshakes_total %d\n", status.SSL.Handshakes)

		buf.WriteString("\n# HELP lolly_ssl_session_reused_total SSL sessions reused\n")
		buf.WriteString("# TYPE lolly_ssl_session_reused_total counter\n")
		fmt.Fprintf(&buf, "lolly_ssl_session_reused_total %d\n", status.SSL.SessionReused)

		buf.WriteString("\n# HELP lolly_ssl_session_reuse_rate SSL session reuse rate\n")
		buf.WriteString("# TYPE lolly_ssl_session_reuse_rate gauge\n")
		fmt.Fprintf(&buf, "lolly_ssl_session_reuse_rate %.2f\n", status.SSL.ReuseRate)
	}

	// Rate Limit 指标
	for _, rl := range status.RateLimits {
		buf.WriteString("\n# HELP lolly_rate_limit_requests Total requests in rate limit zone\n")
		buf.WriteString("# TYPE lolly_rate_limit_requests gauge\n")
		fmt.Fprintf(&buf, "lolly_rate_limit_requests{zone=\"%s\"} %d\n", rl.ZoneName, rl.Requests)

		buf.WriteString("\n# HELP lolly_rate_limit_limit Rate limit value\n")
		buf.WriteString("# TYPE lolly_rate_limit_limit gauge\n")
		fmt.Fprintf(&buf, "lolly_rate_limit_limit{zone=\"%s\"} %d\n", rl.ZoneName, rl.Limit)

		buf.WriteString("\n# HELP lolly_rate_limit_rejected_total Total rejected requests\n")
		buf.WriteString("# TYPE lolly_rate_limit_rejected_total counter\n")
		fmt.Fprintf(&buf, "lolly_rate_limit_rejected_total{zone=\"%s\"} %d\n", rl.ZoneName, rl.Rejected)
	}

	if _, err := ctx.WriteString(buf.String()); err != nil {
		log.Printf("failed to write metrics response: %v", err)
	}
}

// serveJSON 以 JSON 格式输出状态。
func (h *StatusHandler) serveJSON(ctx *fasthttp.RequestCtx, status *Status) {
	ctx.SetContentType("application/json; charset=utf-8")
	ctx.SetStatusCode(fasthttp.StatusOK)

	data, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		utils.SendError(ctx, utils.ErrInternalError)
		return
	}

	if _, err := ctx.Write(data); err != nil {
		log.Printf("failed to write status response: %v", err)
	}
}

// serveText 以纯文本格式输出状态。
func (h *StatusHandler) serveText(ctx *fasthttp.RequestCtx, status *Status) {
	ctx.SetContentType("text/plain; charset=utf-8")
	ctx.SetStatusCode(fasthttp.StatusOK)

	var buf strings.Builder

	buf.WriteString("Lolly Status\n")
	buf.WriteString("============\n\n")
	fmt.Fprintf(&buf, "Version:       %s\n", status.Version)
	fmt.Fprintf(&buf, "Uptime:        %s\n", status.Uptime.Round(time.Second))
	fmt.Fprintf(&buf, "Connections:   %d\n", status.Connections)
	fmt.Fprintf(&buf, "Requests:      %d\n", status.Requests)
	fmt.Fprintf(&buf, "Bytes Sent:    %d\n", status.BytesSent)
	fmt.Fprintf(&buf, "Bytes Received: %d\n", status.BytesReceived)

	if status.Cache != nil {
		buf.WriteString("\nCache:\n")
		fmt.Fprintf(&buf, "  File Entries: %d / %d\n", status.Cache.FileCache.Entries, status.Cache.FileCache.MaxEntries)
		fmt.Fprintf(&buf, "  File Size:    %d / %d bytes\n", status.Cache.FileCache.Size, status.Cache.FileCache.MaxSize)
		fmt.Fprintf(&buf, "  Proxy Entries: %d\n", status.Cache.ProxyCache.Entries)
		fmt.Fprintf(&buf, "  Proxy Pending: %d\n", status.Cache.ProxyCache.Pending)
	}

	if status.Pool != nil {
		buf.WriteString("\nGoroutine Pool:\n")
		fmt.Fprintf(&buf, "  Workers:      %d (idle: %d)\n", status.Pool.Workers, status.Pool.IdleWorkers)
		fmt.Fprintf(&buf, "  Queue:        %d / %d\n", status.Pool.QueueLen, status.Pool.QueueCap)
	}

	if len(status.Upstreams) > 0 {
		buf.WriteString("\nUpstreams:\n")
		for _, upstream := range status.Upstreams {
			fmt.Fprintf(&buf, "  %s: %d healthy, %d unhealthy\n", upstream.Name, upstream.HealthyCount, upstream.UnhealthyCount)
			fmt.Fprintf(&buf, "    Latency: P50=%.2fms, P95=%.2fms, P99=%.2fms\n", upstream.LatencyP50, upstream.LatencyP95, upstream.LatencyP99)
		}
	}

	if status.SSL != nil {
		buf.WriteString("\nSSL:\n")
		fmt.Fprintf(&buf, "  Handshakes:    %d\n", status.SSL.Handshakes)
		fmt.Fprintf(&buf, "  Session Reused: %d\n", status.SSL.SessionReused)
		fmt.Fprintf(&buf, "  Reuse Rate:    %.2f%%\n", status.SSL.ReuseRate)
	}

	if len(status.RateLimits) > 0 {
		buf.WriteString("\nRate Limits:\n")
		for _, rl := range status.RateLimits {
			fmt.Fprintf(&buf, "  %s: %d requests, limit=%d, rejected=%d\n", rl.ZoneName, rl.Requests, rl.Limit, rl.Rejected)
		}
	}

	if _, err := ctx.WriteString(buf.String()); err != nil {
		log.Printf("failed to write text response: %v", err)
	}
}

// serveHTML 以 HTML 格式输出状态。
func (h *StatusHandler) serveHTML(ctx *fasthttp.RequestCtx, status *Status) {
	ctx.SetContentType("text/html; charset=utf-8")
	ctx.SetStatusCode(fasthttp.StatusOK)

	var buf strings.Builder

	buf.WriteString("<!DOCTYPE html>\n")
	buf.WriteString("<html lang=\"en\">\n")
	buf.WriteString("<head>\n")
	buf.WriteString("<meta charset=\"UTF-8\">\n")
	buf.WriteString("<title>Lolly Status</title>\n")
	buf.WriteString("<style>\n")
	buf.WriteString("body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; margin: 2em; }\n")
	buf.WriteString("h1 { color: #333; }\n")
	buf.WriteString("table { border-collapse: collapse; width: 100%; max-width: 600px; }\n")
	buf.WriteString("th, td { border: 1px solid #ddd; padding: 8px; text-align: left; }\n")
	buf.WriteString("th { background-color: #f5f5f5; }\n")
	buf.WriteString(".section { margin-top: 1.5em; }\n")
	buf.WriteString(".healthy { color: green; }\n")
	buf.WriteString(".unhealthy { color: red; }\n")
	buf.WriteString("</style>\n")
	buf.WriteString("</head>\n")
	buf.WriteString("<body>\n")

	buf.WriteString("<h1>Lolly Status</h1>\n")

	buf.WriteString("<table>\n")
	fmt.Fprintf(&buf, "<tr><th>Version</th><td>%s</td></tr>\n", status.Version)
	fmt.Fprintf(&buf, "<tr><th>Uptime</th><td>%s</td></tr>\n", status.Uptime.Round(time.Second))
	fmt.Fprintf(&buf, "<tr><th>Connections</th><td>%d</td></tr>\n", status.Connections)
	fmt.Fprintf(&buf, "<tr><th>Requests</th><td>%d</td></tr>\n", status.Requests)
	fmt.Fprintf(&buf, "<tr><th>Bytes Sent</th><td>%d</td></tr>\n", status.BytesSent)
	fmt.Fprintf(&buf, "<tr><th>Bytes Received</th><td>%d</td></tr>\n", status.BytesReceived)
	buf.WriteString("</table>\n")

	if status.Cache != nil {
		buf.WriteString("<div class=\"section\">\n")
		buf.WriteString("<h2>Cache</h2>\n")
		buf.WriteString("<table>\n")
		buf.WriteString("<tr><th>Type</th><th>Entries</th><th>Max</th><th>Size</th></tr>\n")
		fmt.Fprintf(&buf, "<tr><td>File</td><td>%d</td><td>%d</td><td>%d bytes</td></tr>\n",
			status.Cache.FileCache.Entries, status.Cache.FileCache.MaxEntries, status.Cache.FileCache.Size)
		fmt.Fprintf(&buf, "<tr><td>Proxy</td><td>%d</td><td>-</td><td>-</td></tr>\n", status.Cache.ProxyCache.Entries)
		buf.WriteString("</table>\n")
		buf.WriteString("</div>\n")
	}

	if status.Pool != nil {
		buf.WriteString("<div class=\"section\">\n")
		buf.WriteString("<h2>Goroutine Pool</h2>\n")
		buf.WriteString("<table>\n")
		buf.WriteString("<tr><th>Workers</th><th>Idle</th><th>Queue</th></tr>\n")
		fmt.Fprintf(&buf, "<tr><td>%d</td><td>%d</td><td>%d / %d</td></tr>\n",
			status.Pool.Workers, status.Pool.IdleWorkers, status.Pool.QueueLen, status.Pool.QueueCap)
		buf.WriteString("</table>\n")
		buf.WriteString("</div>\n")
	}

	if len(status.Upstreams) > 0 {
		buf.WriteString("<div class=\"section\">\n")
		buf.WriteString("<h2>Upstreams</h2>\n")
		buf.WriteString("<table>\n")
		buf.WriteString("<tr><th>Name</th><th>Healthy</th><th>Unhealthy</th><th>P50</th><th>P95</th><th>P99</th></tr>\n")
		for _, upstream := range status.Upstreams {
			fmt.Fprintf(&buf, "<tr><td>%s</td><td class=\"healthy\">%d</td><td class=\"unhealthy\">%d</td><td>%.2fms</td><td>%.2fms</td><td>%.2fms</td></tr>\n",
				upstream.Name, upstream.HealthyCount, upstream.UnhealthyCount,
				upstream.LatencyP50, upstream.LatencyP95, upstream.LatencyP99)
		}
		buf.WriteString("</table>\n")
		buf.WriteString("</div>\n")
	}

	if status.SSL != nil {
		buf.WriteString("<div class=\"section\">\n")
		buf.WriteString("<h2>SSL</h2>\n")
		buf.WriteString("<table>\n")
		buf.WriteString("<tr><th>Handshakes</th><th>Session Reused</th><th>Reuse Rate</th></tr>\n")
		fmt.Fprintf(&buf, "<tr><td>%d</td><td>%d</td><td>%.2f%%</td></tr>\n",
			status.SSL.Handshakes, status.SSL.SessionReused, status.SSL.ReuseRate)
		buf.WriteString("</table>\n")
		buf.WriteString("</div>\n")
	}

	if len(status.RateLimits) > 0 {
		buf.WriteString("<div class=\"section\">\n")
		buf.WriteString("<h2>Rate Limits</h2>\n")
		buf.WriteString("<table>\n")
		buf.WriteString("<tr><th>Zone</th><th>Requests</th><th>Limit</th><th>Rejected</th></tr>\n")
		for _, rl := range status.RateLimits {
			fmt.Fprintf(&buf, "<tr><td>%s</td><td>%d</td><td>%d</td><td>%d</td></tr>\n",
				rl.ZoneName, rl.Requests, rl.Limit, rl.Rejected)
		}
		buf.WriteString("</table>\n")
		buf.WriteString("</div>\n")
	}

	buf.WriteString("</body>\n")
	buf.WriteString("</html>\n")

	if _, err := ctx.WriteString(buf.String()); err != nil {
		log.Printf("failed to write html response: %v", err)
	}
}

// collectStatus 收集服务器状态数据。
//
// 从服务器实例读取各项统计指标，构建状态响应对象。
//
// 返回值：
//   - *Status: 包含服务器运行状态的结构体
func (h *StatusHandler) collectStatus() *Status {
	status := &Status{
		Version:       "1.0.0",
		Uptime:        time.Since(h.server.startTime),
		Connections:   h.server.connections.Load(),
		Requests:      h.server.requests.Load(),
		BytesSent:     h.server.bytesSent.Load(),
		BytesReceived: h.server.bytesReceived.Load(),
	}

	// 收集缓存统计
	if h.server.fileCache != nil {
		stats := h.server.fileCache.Stats()
		status.Cache = &CacheStats{
			FileCache: FileCacheStats{
				Entries:    stats.Entries,
				MaxEntries: stats.MaxEntries,
				Size:       stats.Size,
				MaxSize:    stats.MaxSize,
			},
		}

		// 收集代理缓存统计
		proxyCacheStats := h.server.getProxyCacheStats()
		if proxyCacheStats.Entries > 0 || proxyCacheStats.Pending > 0 {
			status.Cache.ProxyCache = proxyCacheStats
		}
	}

	// 收集 Goroutine 池统计
	if h.server.pool != nil {
		poolStats := h.server.pool.Stats()
		status.Pool = &PoolStats{
			Workers:     poolStats.Workers,
			IdleWorkers: poolStats.IdleWorkers,
			MaxWorkers:  poolStats.MaxWorkers,
			MinWorkers:  poolStats.MinWorkers,
			QueueLen:    poolStats.QueueLen,
			QueueCap:    poolStats.QueueCap,
		}
	}

	return status
}
