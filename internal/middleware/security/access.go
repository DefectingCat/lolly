// Package security 提供安全相关的 HTTP 中间件。
//
// 该文件实现 IP 访问控制中间件，支持基于 CIDR 的允许/拒绝列表，
// 兼容 IPv4 和 IPv6。
//
// 使用示例：
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
//	// 应用为中间件
//	chain := middleware.NewChain(access)
//	handler := chain.Apply(finalHandler)
//
// 作者：xfy
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

// Action 表示对 IP 的操作类型。
type Action int

const (
	// ActionAllow 允许请求通过
	ActionAllow Action = iota
	// ActionDeny 拒绝请求（返回 403 Forbidden）
	ActionDeny
)

// AccessControl 实现 IP 访问控制中间件。
//
// 根据配置的允许/拒绝 CIDR 列表检查入站请求。
// 支持动态更新访问控制列表。
type AccessControl struct {
	// allowList 允许的 CIDR 网络列表
	allowList []net.IPNet

	// denyList 拒绝的 CIDR 网络列表
	denyList []net.IPNet

	// defaultAction 默认操作，当无规则匹配时执行
	defaultAction Action

	// trustedProxies 可信代理 CIDR 列表，用于安全解析 X-Forwarded-For
	trustedProxies []net.IPNet

	// mu 保护并发访问的读写锁
	mu sync.RWMutex
}

// NewAccessControl 创建访问控制中间件。
//
// 根据配置创建访问控制中间件实例，解析 CIDR 列表并设置默认操作。
//
// 参数：
//   - cfg: 访问控制配置，包含允许/拒绝列表和默认操作
//
// 返回值：
//   - *AccessControl: 配置好的访问控制中间件
//   - error: CIDR 解析失败时返回错误
func NewAccessControl(cfg *config.AccessConfig) (*AccessControl, error) {
	if cfg == nil {
		return nil, errors.New("access config is nil")
	}

	ac := &AccessControl{}

	// 解析允许列表
	for _, cidr := range cfg.Allow {
		network, err := parseCIDR(cidr)
		if err != nil {
			return nil, fmt.Errorf("invalid allow CIDR %s: %w", cidr, err)
		}
		ac.allowList = append(ac.allowList, *network)
	}

	// 解析拒绝列表
	for _, cidr := range cfg.Deny {
		network, err := parseCIDR(cidr)
		if err != nil {
			return nil, fmt.Errorf("invalid deny CIDR %s: %w", cidr, err)
		}
		ac.denyList = append(ac.denyList, *network)
	}

	// 解析可信代理列表
	for _, cidr := range cfg.TrustedProxies {
		network, err := parseCIDR(cidr)
		if err != nil {
			return nil, fmt.Errorf("invalid trusted_proxy CIDR %s: %w", cidr, err)
		}
		ac.trustedProxies = append(ac.trustedProxies, *network)
	}

	// 设置默认操作
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

// Name 返回中间件名称。
//
// 返回值：
//   - string: 中间件标识名 "access_control"
func (ac *AccessControl) Name() string {
	return "access_control"
}

// Process 用访问控制逻辑包装下一个处理器。
//
// 被拒绝的 IP 请求将收到 403 Forbidden 响应。
//
// 参数：
//   - next: 下一个请求处理器
//
// 返回值：
//   - fasthttp.RequestHandler: 包装后的处理器
func (ac *AccessControl) Process(next fasthttp.RequestHandler) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		clientIP := ac.getClientIP(ctx)

		// 检查访问权限
		if !ac.Check(clientIP) {
			ctx.Error("Forbidden: Access denied", fasthttp.StatusForbidden)
			return
		}

		next(ctx)
	}
}

// Check 检查 IP 地址是否允许访问。
//
// 评估顺序：先检查拒绝列表，再检查允许列表，最后使用默认操作。
//
// 参数：
//   - ip: 待检查的 IP 地址
//
// 返回值：
//   - bool: true 表示允许访问，false 表示拒绝
func (ac *AccessControl) Check(ip net.IP) bool {
	ac.mu.RLock()
	defer ac.mu.RUnlock()

	// 先检查拒绝列表（显式拒绝优先）
	for _, network := range ac.denyList {
		if network.Contains(ip) {
			return false
		}
	}

	// 检查允许列表
	for _, network := range ac.allowList {
		if network.Contains(ip) {
			return true
		}
	}

	// 返回默认操作
	return ac.defaultAction == ActionAllow
}

// UpdateAllowList 动态更新允许列表。
//
// 替换当前的允许列表，使用写锁保护并发访问。
//
// 参数：
//   - cidrs: 新的 CIDR 字符串列表
//
// 返回值：
//   - error: CIDR 解析失败时返回错误
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

// UpdateDenyList 动态更新拒绝列表。
//
// 替换当前的拒绝列表，使用写锁保护并发访问。
//
// 参数：
//   - cidrs: 新的 CIDR 字符串列表
//
// 返回值：
//   - error: CIDR 解析失败时返回错误
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

// SetDefault 设置默认操作。
//
// 参数：
//   - action: 操作类型，"allow" 或 "deny"
//
// 返回值：
//   - error: 无效的操作类型时返回错误
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

// parseCIDR 解析 CIDR 字符串，支持 IPv4 和 IPv6。
//
// 支持完整的 CIDR 表示法（如 192.168.1.0/24）和单个 IP（如 192.168.1.1）。
// 单个 IP 会自动转换为 /32（IPv4）或 /128（IPv6）的 CIDR。
//
// 参数：
//   - cidr: CIDR 字符串或单个 IP 地址
//
// 返回值：
//   - *net.IPNet: 解析后的 IP 网络对象
//   - error: 解析失败时返回错误
func parseCIDR(cidr string) (*net.IPNet, error) {
	// 处理单个 IP（没有 /前缀）
	if !strings.Contains(cidr, "/") {
		ip := net.ParseIP(cidr)
		if ip == nil {
			return nil, fmt.Errorf("invalid IP address: %s", cidr)
		}

		// 转换为完整掩码的 CIDR
		if ip.To4() != nil {
			cidr = cidr + "/32"
		} else {
			cidr = cidr + "/128"
		}
	}

	// 解析 CIDR
	ip, network, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, err
	}

	// 确保 IP 为规范形式
	network.IP = ip

	return network, nil
}

// getClientIP 从请求上下文安全提取客户端 IP。
//
// 仅当请求来自可信代理时，才解析 X-Forwarded-For 头部。
// 使用右侧（最接近客户端）的非可信 IP 作为真实客户端 IP。
//
// 参数：
//   - ctx: FastHTTP 请求上下文
//
// 返回值：
//   - net.IP: 客户端 IP 地址，无法获取时返回 nil
func (ac *AccessControl) getClientIP(ctx *fasthttp.RequestCtx) net.IP {
	remoteIP := getRemoteAddrIP(ctx)

	// 仅当配置了可信代理且请求来自可信代理时，才解析 X-Forwarded-For
	if len(ac.trustedProxies) > 0 && remoteIP != nil {
		isTrusted := false
		for _, network := range ac.trustedProxies {
			if network.Contains(remoteIP) {
				isTrusted = true
				break
			}
		}

		if isTrusted {
			// 使用右侧（最接近客户端）的非可信 IP
			if xff := ctx.Request.Header.Peek("X-Forwarded-For"); len(xff) > 0 {
				ips := strings.Split(string(xff), ",")
				for i := len(ips) - 1; i >= 0; i-- {
					ipStr := strings.TrimSpace(ips[i])
					if ip := net.ParseIP(ipStr); ip != nil {
						// 检查该 IP 是否在可信代理列表中
						trusted := false
						for _, network := range ac.trustedProxies {
							if network.Contains(ip) {
								trusted = true
								break
							}
						}
						if !trusted {
							return ip
						}
					}
				}
			}
		}
	}

	// 检查 X-Real-IP 头部（仅来自可信代理时）
	if len(ac.trustedProxies) > 0 && remoteIP != nil {
		isTrusted := false
		for _, network := range ac.trustedProxies {
			if network.Contains(remoteIP) {
				isTrusted = true
				break
			}
		}

		if isTrusted {
			if xri := ctx.Request.Header.Peek("X-Real-IP"); len(xri) > 0 {
				if ip := net.ParseIP(string(xri)); ip != nil {
					return ip
				}
			}
		}
	}

	return remoteIP
}

// getRemoteAddrIP 从 RemoteAddr 提取 IP。
//
// 参数：
//   - ctx: FastHTTP 请求上下文
//
// 返回值：
//   - net.IP: 客户端 IP 地址，无法获取时返回 nil
func getRemoteAddrIP(ctx *fasthttp.RequestCtx) net.IP {
	if addr := ctx.RemoteAddr(); addr != nil {
		if tcpAddr, ok := addr.(*net.TCPAddr); ok {
			return tcpAddr.IP
		}
		// 从字符串表示解析
		ipStr := addr.String()
		if idx := strings.LastIndex(ipStr, ":"); idx != -1 {
			ipStr = ipStr[:idx]
		}
		// 移除 IPv6 的方括号
		ipStr = strings.TrimPrefix(strings.TrimSuffix(ipStr, "]"), "[")
		return net.ParseIP(ipStr)
	}
	return nil
}

// AccessStats 访问控制统计信息结构。
type AccessStats struct {
	AllowCount int    // 允许列表中的规则数量
	DenyCount  int    // 拒绝列表中的规则数量
	Default    string // 默认操作（"allow" 或 "deny"）
}

// GetStats 返回当前访问控制的统计信息。
//
// 返回值：
//   - AccessStats: 包含规则数量和默认操作的统计对象
func (ac *AccessControl) GetStats() AccessStats {
	ac.mu.RLock()
	defer ac.mu.RUnlock()

	return AccessStats{
		AllowCount: len(ac.allowList),
		DenyCount:  len(ac.denyList),
		Default:    actionToString(ac.defaultAction),
	}
}

// actionToString 将 Action 转换为其字符串表示。
//
// 参数：
//   - action: 操作类型
//
// 返回值：
//   - string: 操作类型的字符串表示
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

// 验证接口实现
var _ middleware.Middleware = (*AccessControl)(nil)
