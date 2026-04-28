// Package server 提供 HTTP 服务器核心功能。
//
// 该文件实现缓存清理 API 处理器，支持主动清理代理缓存。
package server

import (
	"encoding/json"
	"net"
	"net/netip"

	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/cache"
	"rua.plus/lolly/internal/config"
	"rua.plus/lolly/internal/utils"
)

// PurgeHandler 缓存清理 API 处理器。
//
// 持有 Server 引用以访问所有代理实例的缓存。
// 支持 IP 白名单和 Token 认证保护。
//
// 注意事项：
//   - 仅处理 POST 请求
//   - 支持按路径和按模式两种清理方式
//   - method 参数支持指定 HTTP 方法（默认 GET）
type PurgeHandler struct {
	server  *Server
	auth    config.CacheAPIAuthConfig
	path    string
	allowed []net.IPNet
}

// NewPurgeHandler 创建缓存清理 API 处理器。
//
// 解析 IP 白名单配置，支持 CIDR 格式和单个 IP。
// localhost 特殊处理为 127.0.0.1 和 ::1。
//
// 参数：
//   - server: Server 实例，用于访问代理缓存
//   - cfg: CacheAPI 配置
//
// 返回值：
//   - *PurgeHandler: 配置好的处理器
//   - error: IP 解析失败时返回非 nil 错误
func NewPurgeHandler(server *Server, cfg *config.CacheAPIConfig) (*PurgeHandler, error) {
	h := &PurgeHandler{
		server: server,
		path:   cfg.Path,
		auth:   cfg.Auth,
	}

	// 默认路径
	if h.path == "" {
		h.path = "/_cache/purge"
	}

	// 解析允许的 IP 列表
	for _, cidr := range cfg.Allow {
		// 处理 localhost 特殊情况
		if cidr == "localhost" {
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
			ip, err := netip.ParseAddr(cidr)
			if err != nil {
				return nil, err
			}
			// 转换为 CIDR 格式
			if ip.Is4() {
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

// Path 返回 API 端点路径。
func (h *PurgeHandler) Path() string {
	return h.path
}

// ServeHTTP 处理缓存清理请求。
//
// 仅处理 POST 请求，支持精确路径和通配符模式清理。
// 返回 JSON 格式的响应。
func (h *PurgeHandler) ServeHTTP(ctx *fasthttp.RequestCtx) {
	// 仅允许 POST 方法
	if string(ctx.Method()) != "POST" {
		utils.SendJSONError(ctx, fasthttp.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// 检查 IP 访问权限
	if !utils.CheckIPAccess(ctx, h.allowed) {
		utils.SendJSONError(ctx, fasthttp.StatusForbidden, "forbidden")
		return
	}

	// 检查认证
	if !utils.CheckTokenAuth(ctx, h.auth) {
		utils.SendJSONError(ctx, fasthttp.StatusUnauthorized, "unauthorized")
		return
	}

	// 解析请求体
	var req cache.PurgeRequest
	if err := json.Unmarshal(ctx.PostBody(), &req); err != nil {
		utils.SendJSONError(ctx, fasthttp.StatusBadRequest, "invalid request body")
		return
	}

	// 执行清理
	deleted := 0
	if req.Path != "" {
		deleted = h.purgeByPath(req.Path, req.Method)
	} else if req.Pattern != "" {
		deleted = h.purgeByPattern(req.Pattern, req.Method)
	} else {
		utils.SendJSONError(ctx, fasthttp.StatusBadRequest, "missing path or pattern")
		return
	}

	// 返回响应
	ctx.SetContentType("application/json; charset=utf-8")
	ctx.SetStatusCode(fasthttp.StatusOK)
	_ = json.NewEncoder(ctx).Encode(cache.PurgeResponse{Deleted: deleted})
}

// purgeByPath 按精确路径清理缓存。
func (h *PurgeHandler) purgeByPath(path string, method string) int {
	if h.server == nil {
		return 0
	}

	hashKey := cache.HashPathWithMethod(path, method)
	deleted := 0

	for _, p := range h.server.proxies {
		if pcache := p.GetCache(); pcache != nil {
			_ = pcache.Delete(hashKey)
			deleted++
		}
	}

	return deleted
}

// purgeByPattern 按通配符模式清理缓存。
func (h *PurgeHandler) purgeByPattern(pattern string, method string) int {
	if h.server == nil {
		return 0
	}

	deleted := 0

	for _, p := range h.server.proxies {
		if pcache := p.GetCache(); pcache != nil {
			deleted += pcache.DeleteByPatternWithMethod(pattern, method)
		}
	}

	return deleted
}

// PurgeByPathForTest 测试用的导出方法。
func (h *PurgeHandler) PurgeByPathForTest(path string, method string) int {
	return h.purgeByPath(path, method)
}

// PurgeByPatternForTest 测试用的导出方法。
func (h *PurgeHandler) PurgeByPatternForTest(pattern string, method string) int {
	return h.purgeByPattern(pattern, method)
}
