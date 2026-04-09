// Package cache 提供文件缓存和代理缓存功能，支持 LRU 淘汰和缓存锁防击穿。
//
// 该文件实现了缓存清理 API，用于主动清理代理缓存。
//
// 主要功能：
//   - 精确路径清理：删除指定路径的缓存条目
//   - 通配符模式清理：按模式批量删除缓存条目
//   - IP 白名单访问控制
//   - Token 认证支持
//
// 作者：xfy
package cache

import (
	"encoding/json"
	"hash/fnv"
	"net"
	"strings"

	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/config"
)

// PurgeAPI 缓存清理 API 处理器。
//
// 提供 HTTP API 用于主动清理代理缓存，支持精确路径和通配符模式清理。
//
// 注意事项：
//   - 所有方法均为并发安全
//   - 支持 IP 白名单和 Token 认证
//   - 仅处理 POST 请求
type PurgeAPI struct {
	// cache 代理缓存实例
	cache *ProxyCache

	// allowed 允许访问的 IP 网络列表
	allowed []net.IPNet

	// auth 认证配置
	auth config.CacheAPIAuthConfig

	// path API 端点路径
	path string
}

// PurgeRequest 清理请求结构。
type PurgeRequest struct {
	// Path 精确路径
	Path string `json:"path,omitempty"`

	// Pattern 通配符模式（支持 * 通配符）
	Pattern string `json:"pattern,omitempty"`
}

// PurgeResponse 清理响应结构。
type PurgeResponse struct {
	// Deleted 被删除的缓存条目数
	Deleted int `json:"deleted"`
}

// PurgeErrorResponse 错误响应结构。
type PurgeErrorResponse struct {
	// Error 错误信息
	Error string `json:"error"`
}

// NewPurgeAPI 创建缓存清理 API 处理器。
//
// 参数：
//   - cache: 代理缓存实例
//   - cfg: 缓存 API 配置
//
// 返回值：
//   - *PurgeAPI: 配置好的处理器
//   - error: IP 解析失败时返回非 nil 错误
func NewPurgeAPI(cache *ProxyCache, cfg *config.CacheAPIConfig) (*PurgeAPI, error) {
	p := &PurgeAPI{
		cache: cache,
		auth:  cfg.Auth,
		path:  cfg.Path,
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
			p.allowed = append(p.allowed, *network)
		}
	}

	return p, nil
}

// Path 返回 API 端点路径。
func (p *PurgeAPI) Path() string {
	if p.path == "" {
		return "/_cache/purge"
	}
	return p.path
}

// ServeHTTP 处理缓存清理请求。
//
// 仅处理 POST 请求，支持精确路径和通配符模式清理。
// 返回 JSON 格式的响应。
func (p *PurgeAPI) ServeHTTP(ctx *fasthttp.RequestCtx) {
	// 仅允许 POST 方法
	if string(ctx.Method()) != "POST" {
		p.sendError(ctx, fasthttp.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// 检查 IP 访问权限
	if !p.checkAccess(ctx) {
		p.sendError(ctx, fasthttp.StatusForbidden, "forbidden")
		return
	}

	// 检查认证
	if !p.checkAuth(ctx) {
		p.sendError(ctx, fasthttp.StatusUnauthorized, "unauthorized")
		return
	}

	// 解析请求体
	var req PurgeRequest
	if err := json.Unmarshal(ctx.PostBody(), &req); err != nil {
		p.sendError(ctx, fasthttp.StatusBadRequest, "invalid request body")
		return
	}

	// 执行清理
	deleted := 0
	if req.Path != "" {
		deleted = p.purgeByPath(req.Path)
	} else if req.Pattern != "" {
		deleted = p.purgeByPattern(req.Pattern)
	} else {
		p.sendError(ctx, fasthttp.StatusBadRequest, "missing path or pattern")
		return
	}

	// 返回响应
	ctx.SetContentType("application/json; charset=utf-8")
	ctx.SetStatusCode(fasthttp.StatusOK)
	json.NewEncoder(ctx).Encode(PurgeResponse{Deleted: deleted}) //nolint:errcheck
}

// checkAccess 检查客户端 IP 是否在允许列表中。
func (p *PurgeAPI) checkAccess(ctx *fasthttp.RequestCtx) bool {
	// 如果没有配置允许列表，允许所有访问
	if len(p.allowed) == 0 {
		return true
	}

	clientIP := p.getClientIP(ctx)
	if clientIP == nil {
		return false
	}

	// 检查是否在允许列表中
	for _, network := range p.allowed {
		if network.Contains(clientIP) {
			return true
		}
	}

	return false
}

// checkAuth 检查认证。
func (p *PurgeAPI) checkAuth(ctx *fasthttp.RequestCtx) bool {
	// 无需认证
	if p.auth.Type == "" || p.auth.Type == "none" {
		return true
	}

	// Token 认证
	if p.auth.Type == "token" {
		// 从 Authorization header 获取 token
		authHeader := ctx.Request.Header.Peek("Authorization")
		if len(authHeader) == 0 {
			return false
		}

		// 支持 Bearer token 格式
		authStr := string(authHeader)
		if token, ok := strings.CutPrefix(authStr, "Bearer "); ok {
			return token == p.auth.Token
		}

		// 也支持直接传递 token
		return authStr == p.auth.Token
	}

	return false
}

// getClientIP 从请求上下文提取客户端 IP。
func (p *PurgeAPI) getClientIP(ctx *fasthttp.RequestCtx) net.IP {
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

// purgeByPath 按精确路径清理缓存。
func (p *PurgeAPI) purgeByPath(path string) int {
	if p.cache == nil {
		return 0
	}

	// 计算缓存键的哈希值
	hashKey := hashPath(path)

	// 尝试删除
	p.cache.mu.Lock()
	defer p.cache.mu.Unlock()

	if _, ok := p.cache.entries[hashKey]; ok {
		delete(p.cache.entries, hashKey)
		return 1
	}

	return 0
}

// purgeByPattern 按通配符模式清理缓存。
func (p *PurgeAPI) purgeByPattern(pattern string) int {
	if p.cache == nil {
		return 0
	}

	p.cache.mu.Lock()
	defer p.cache.mu.Unlock()

	deleted := 0
	for hashKey, entry := range p.cache.entries {
		if matchPattern(pattern, entry.OrigKey) {
			delete(p.cache.entries, hashKey)
			deleted++
		}
	}

	return deleted
}

// hashPath 使用 FNV-64a 计算路径的哈希值。
// 与代理层 buildCacheKeyHash 使用相同的算法，确保一致性。
// 注意：代理层的 key 格式为 "METHOD:URI"，purge 时默认使用 GET 方法。
func hashPath(path string) uint64 {
	// 默认使用 GET 方法，与代理层 key 格式一致
	key := "GET:" + path
	h := fnv.New64a()
	h.Write([]byte(key))
	return h.Sum64()
}

// matchPattern 检查路径是否匹配通配符模式。
// 仅支持 * 通配符，匹配任意字符。
func matchPattern(pattern, path string) bool {
	// 特殊情况：* 匹配所有
	if pattern == "*" {
		return true
	}

	// 检查是否有通配符
	if !strings.Contains(pattern, "*") {
		return path == pattern
	}

	// 简单的前缀匹配：/api/users/* 匹配 /api/users/123
	if prefix, ok := strings.CutSuffix(pattern, "*"); ok {
		return strings.HasPrefix(path, prefix)
	}

	// 中间通配符：/api/*/users 匹配 /api/v1/users
	parts := strings.Split(pattern, "*")
	if len(parts) == 2 {
		return strings.HasPrefix(path, parts[0]) && strings.HasSuffix(path, parts[1])
	}

	// 复杂模式不支持，返回 false
	return false
}

// sendError 发送错误响应。
func (p *PurgeAPI) sendError(ctx *fasthttp.RequestCtx, status int, errMsg string) {
	ctx.SetContentType("application/json; charset=utf-8")
	ctx.SetStatusCode(status)
	json.NewEncoder(ctx).Encode(PurgeErrorResponse{Error: errMsg}) //nolint:errcheck
}
