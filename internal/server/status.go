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
	"net"
	"strings"
	"time"

	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/config"
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
	server  *Server    // 服务器实例，用于获取状态数据
	allowed []net.IPNet // 允许访问的 IP 网络列表
	path    string      // 状态端点路径
}

// Status 状态响应结构。
//
// 包含服务器运行的各种统计信息，以 JSON 格式返回给客户端。
type Status struct {
	Version       string        `json:"version"`        // 服务器版本号
	Uptime        time.Duration `json:"uptime"`         // 服务器运行时间
	Connections   int64         `json:"connections"`    // 当前活跃连接数
	Requests      int64         `json:"requests"`       // 已处理的总请求数
	BytesSent     int64         `json:"bytes_sent"`     // 已发送的总字节数
	BytesReceived int64         `json:"bytes_received"` // 已接收的总字节数
	Cache         *CacheStats   `json:"cache,omitempty"`  // 缓存统计（可选）
	Pool          *PoolStats    `json:"pool,omitempty"`   // Goroutine 池统计（可选）
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
	}

	// 解析允许的 IP 列表
	for _, cidr := range cfg.Allow {
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
// 未授权访问返回 403 Forbidden，授权访问返回 JSON 格式状态。
//
// 参数：
//   - ctx: FastHTTP 请求上下文
func (h *StatusHandler) ServeHTTP(ctx *fasthttp.RequestCtx) {
	// 步骤1: 检查 IP 访问权限
	if !h.checkAccess(ctx) {
		ctx.Error("Forbidden: Access denied", fasthttp.StatusForbidden)
		return
	}

	// 步骤2: 收集状态数据
	status := h.collectStatus()

	// 步骤3: 返回 JSON 响应
	ctx.SetContentType("application/json; charset=utf-8")
	ctx.SetStatusCode(fasthttp.StatusOK)

	data, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		ctx.Error("Internal Server Error", fasthttp.StatusInternalServerError)
		return
	}

	ctx.Write(data) //nolint:errcheck
}

// checkAccess 检查客户端 IP 是否在允许列表中。
//
// 如果未配置允许列表，则允许所有访问。
// 检查时支持代理头部（X-Forwarded-For、X-Real-IP）。
//
// 参数：
//   - ctx: FastHTTP 请求上下文
//
// 返回值：
//   - bool: true 表示允许访问，false 表示拒绝
func (h *StatusHandler) checkAccess(ctx *fasthttp.RequestCtx) bool {
	// 如果没有配置允许列表，允许所有访问
	if len(h.allowed) == 0 {
		return true
	}

	clientIP := getClientIPForStatus(ctx)

	// 检查是否在允许列表中
	for _, network := range h.allowed {
		if network.Contains(clientIP) {
			return true
		}
	}

	return false
}

// getClientIPForStatus 从请求上下文提取客户端 IP。
//
// 按优先级依次检查：X-Forwarded-For、X-Real-IP、RemoteAddr。
// 用于状态端点的 IP 访问控制。
//
// 参数：
//   - ctx: FastHTTP 请求上下文
//
// 返回值：
//   - net.IP: 客户端 IP 地址，无法获取时返回 nil
func getClientIPForStatus(ctx *fasthttp.RequestCtx) net.IP {
	// 检查 X-Forwarded-For 头部
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

	// 检查 X-Real-IP 头部
	if xri := ctx.Request.Header.Peek("X-Real-IP"); len(xri) > 0 {
		ip := net.ParseIP(string(xri))
		if ip != nil {
			return ip
		}
	}

	// 使用 RemoteAddr
	if addr := ctx.RemoteAddr(); addr != nil {
		if tcpAddr, ok := addr.(*net.TCPAddr); ok {
			return tcpAddr.IP
		}
	}

	return nil
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
