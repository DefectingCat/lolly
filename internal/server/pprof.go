// Package server 提供 pprof 性能分析端点支持。
//
// 该文件为 fasthttp 服务器提供 pprof 端点，用于收集：
//   - CPU profile（用于 PGO 优化）
//   - 内存分配 profile
//   - Goroutine 分析
//   - 阻塞分析
//   - 锁竞争分析
//
// 主要用途：
//
//	用于在生产环境中采集性能数据，支持性能优化和问题排查。
//
// 注意事项：
//   - 仅在配置启用时生效
//   - 生产环境建议限制访问 IP
//   - CPU profile 收集需要代表性 workload
//   - 所有端点均支持流式输出，适合大数据量场景
//
// 作者：xfy
package server

import (
	"bufio"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/config"
)

// PprofHandler pprof 性能分析处理器。
//
// 封装 fasthttp 的 pprof handler，提供 IP 访问控制。
type PprofHandler struct {
	// path 端点路径前缀
	path string

	// allowedIPs 允许访问的 IP 列表
	allowedIPs []net.IP

	// allowedNets 允许访问的 CIDR 网络
	allowedNets []*net.IPNet
}

// NewPprofHandler 创建 pprof 处理器。
//
// 根据配置创建 pprof 端点处理器，包括 IP 访问控制。
//
// 参数：
//   - cfg: pprof 配置对象
//
// 返回值：
//   - *PprofHandler: 创建的处理器实例
//   - error: 创建过程中遇到的错误，如 CIDR 解析失败
func NewPprofHandler(cfg *config.PprofConfig) (*PprofHandler, error) {
	if !cfg.Enabled {
		return nil, nil
	}

	path := cfg.Path
	if path == "" {
		path = config.DefaultPprofPath
	}

	h := &PprofHandler{path: path}

	// 解析允许的 IP 列表
	for _, ipStr := range cfg.Allow {
		if ip := net.ParseIP(ipStr); ip != nil {
			h.allowedIPs = append(h.allowedIPs, ip)
			continue
		}
		// 尝试解析 CIDR
		_, net, err := net.ParseCIDR(ipStr)
		if err != nil {
			return nil, fmt.Errorf("解析 IP/CIDR 失败: %s: %w", ipStr, err)
		}
		h.allowedNets = append(h.allowedNets, net)
	}

	// 默认只允许 localhost
	if len(h.allowedIPs) == 0 && len(h.allowedNets) == 0 {
		h.allowedIPs = []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")}
	}

	return h, nil
}

// Path 返回 pprof 端点路径。
//
// 返回配置的 pprof 端点路径前缀，用于路由注册。
//
// 返回值：
//   - string: pprof 端点的路径前缀，如 "/debug/pprof"
//
// 使用示例：
//
//	path := handler.Path()
//	router.GET(path, handler.ServeHTTP)
func (h *PprofHandler) Path() string {
	return h.path
}

// ServeHTTP 处理 pprof 请求。
//
// 根据 URL 路径选择对应的 profile 处理器，
// 并检查客户端 IP 是否在允许列表中。
//
// 参数：
//   - ctx: fasthttp 请求上下文，包含请求信息和响应写入接口
//
// 注意事项：
//   - 未授权访问返回 403 Forbidden
//   - 未知的 profile 类型返回 404 Not Found
func (h *PprofHandler) ServeHTTP(ctx *fasthttp.RequestCtx) {
	// IP 访问控制
	if !h.isAllowed(ctx) {
		ctx.SetStatusCode(fasthttp.StatusForbidden)
		ctx.SetBodyString("Forbidden")
		return
	}

	// 根据路径分发
	path := string(ctx.Path())
	subPath := path[len(h.path):]

	switch subPath {
	case "", "/":
		h.handleIndex(ctx)
	case "/profile":
		h.handleCPU(ctx)
	case "/heap":
		h.handleHeap(ctx)
	case "/goroutine":
		h.handleGoroutine(ctx)
	case "/block":
		h.handleBlock(ctx)
	case "/mutex":
		h.handleMutex(ctx)
	default:
		ctx.SetStatusCode(fasthttp.StatusNotFound)
		ctx.SetBodyString("Unknown profile: " + subPath)
	}
}

// isAllowed 检查客户端 IP 是否允许访问。
//
// 根据配置的 IP 白名单和 CIDR 网络范围验证客户端 IP。
// 若未配置任何限制，则默认允许所有访问。
//
// 参数：
//   - ctx: fasthttp 请求上下文，用于获取客户端 IP
//
// 返回值：
//   - bool: true 表示允许访问，false 表示禁止访问
func (h *PprofHandler) isAllowed(ctx *fasthttp.RequestCtx) bool {
	if len(h.allowedIPs) == 0 && len(h.allowedNets) == 0 {
		return true // 无限制
	}

	ipStr := ctx.RemoteIP().String()
	clientIP := net.ParseIP(ipStr)
	if clientIP == nil {
		return false
	}

	// 检查精确 IP
	for _, ip := range h.allowedIPs {
		if ip.Equal(clientIP) {
			return true
		}
	}

	// 检查 CIDR 网络
	for _, net := range h.allowedNets {
		if net.Contains(clientIP) {
			return true
		}
	}

	return false
}

// handleIndex 处理索引页面。
//
// 返回 HTML 格式的 pprof 端点索引页面，列出所有可用的 profile 类型。
//
// 参数：
//   - ctx: fasthttp 请求上下文，用于写入响应内容
func (h *PprofHandler) handleIndex(ctx *fasthttp.RequestCtx) {
	ctx.SetContentType("text/html; charset=utf-8")
	html := `<html>
<head><title>Pprof Profiles</title></head>
<body>
<h1>Pprof Profiles</h1>
<table>
<tr><td><a href="%s/profile?seconds=30">CPU Profile (30s)</a></td><td>CPU profile for 30 seconds</td></tr>
<tr><td><a href="%s/heap">Heap Profile</a></td><td>Memory allocation profile</td></tr>
<tr><td><a href="%s/goroutine">Goroutine Profile</a></td><td>Goroutine stack traces</td></tr>
<tr><td><a href="%s/block">Block Profile</a></td><td>Blocking profile</td></tr>
<tr><td><a href="%s/mutex">Mutex Profile</a></td><td>Mutex contention profile</td></tr>
</table>
<p>Usage: curl %s/profile?seconds=30 > cpu.pgo</p>
</body>
</html>`
	ctx.SetBodyString(fmt.Sprintf(html, h.path, h.path, h.path, h.path, h.path, h.path))
}

// handleCPU 处理 CPU profile 请求。
//
// 启动 CPU profile 采集，等待指定时长后停止并返回结果。
// 采集时长可通过 URL 参数 "seconds" 指定，默认 30 秒。
//
// 参数：
//   - ctx: fasthttp 请求上下文，用于获取参数和写入响应
func (h *PprofHandler) handleCPU(ctx *fasthttp.RequestCtx) {
	// 获取采集时长
	seconds := 30
	if secStr := ctx.QueryArgs().Peek("seconds"); secStr != nil {
		if sec, err := strconv.Atoi(string(secStr)); err == nil && sec > 0 {
			seconds = sec
		}
	}

	ctx.SetContentType("application/octet-stream")
	ctx.SetBodyStreamWriter(func(w *bufio.Writer) {
		// 启动 CPU profile
		if err := startCPUProfile(wrapBufioWriter(w)); err != nil {
			//nolint:errcheck
			_, _ = w.WriteString("Error starting CPU profile: " + err.Error())
			//nolint:errcheck
			_ = w.Flush()
			return
		}

		// 等待采集完成
		time.Sleep(time.Duration(seconds) * time.Second)

		// 停止 CPU profile
		stopCPUProfile()
		//nolint:errcheck
		_ = w.Flush()
	})
}

// handleHeap 处理内存 profile 请求。
//
// 执行 GC 后采集内存分配 profile 并返回结果。
//
// 参数：
//   - ctx: fasthttp 请求上下文，用于写入响应
func (h *PprofHandler) handleHeap(ctx *fasthttp.RequestCtx) {
	ctx.SetContentType("application/octet-stream")
	ctx.SetBodyStreamWriter(func(w *bufio.Writer) {
		writeHeapProfile(wrapBufioWriter(w))
		//nolint:errcheck
		_ = w.Flush()
	})
}

// handleGoroutine 处理 Goroutine profile 请求。
//
// 采集当前所有 Goroutine 的栈追踪信息并返回。
//
// 参数：
//   - ctx: fasthttp 请求上下文，用于写入响应
func (h *PprofHandler) handleGoroutine(ctx *fasthttp.RequestCtx) {
	ctx.SetContentType("application/octet-stream")
	ctx.SetBodyStreamWriter(func(w *bufio.Writer) {
		writeGoroutineProfile(wrapBufioWriter(w))
		//nolint:errcheck
		_ = w.Flush()
	})
}

// handleBlock 处理阻塞 profile 请求。
//
// 采集阻塞操作的 profile 数据并返回。
//
// 参数：
//   - ctx: fasthttp 请求上下文，用于写入响应
func (h *PprofHandler) handleBlock(ctx *fasthttp.RequestCtx) {
	ctx.SetContentType("application/octet-stream")
	ctx.SetBodyStreamWriter(func(w *bufio.Writer) {
		writeBlockProfile(wrapBufioWriter(w))
		//nolint:errcheck
		_ = w.Flush()
	})
}

// handleMutex 处理锁竞争 profile 请求。
//
// 采集互斥锁竞争的 profile 数据并返回。
//
// 参数：
//   - ctx: fasthttp 请求上下文，用于写入响应
func (h *PprofHandler) handleMutex(ctx *fasthttp.RequestCtx) {
	ctx.SetContentType("application/octet-stream")
	ctx.SetBodyStreamWriter(func(w *bufio.Writer) {
		writeMutexProfile(wrapBufioWriter(w))
		//nolint:errcheck
		_ = w.Flush()
	})
}
