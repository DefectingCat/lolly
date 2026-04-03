// Package server 提供 HTTP 服务器的核心实现，支持单服务器和虚拟主机两种运行模式。
//
// 该文件包含虚拟主机管理相关的核心逻辑，包括：
//   - 虚拟主机管理器的创建和配置
//   - 基于 Host 头的请求分发
//   - 默认主机 fallback 机制
//
// 主要用途：
//   用于支持多域名虚拟主机场景，根据请求的 Host 头分发到不同的处理器。
//
// 注意事项：
//   - 所有方法均为并发安全
//   - 未匹配的 Host 头请求由默认主机处理
//
// 作者：xfy
package server

import (
	"github.com/valyala/fasthttp"
)

// VHostManager 虚拟主机管理器。
//
// 管理多个虚拟主机，根据请求的 Host 头分发到对应的处理器。
// 支持默认主机作为未匹配请求的 fallback。
type VHostManager struct {
	// hosts 虚拟主机映射，按 server name 索引
	hosts map[string]*VirtualHost

	// defaultHost 默认主机，处理未匹配的 Host 头请求
	defaultHost *VirtualHost
}

// VirtualHost 虚拟主机。
//
// 代表一个虚拟主机配置，包含名称和对应的请求处理器。
type VirtualHost struct {
	// name 虚拟主机名称（域名）
	name string

	// handler 请求处理器
	handler fasthttp.RequestHandler
}

// NewVHostManager 创建虚拟主机管理器
func NewVHostManager() *VHostManager {
	return &VHostManager{
		hosts: make(map[string]*VirtualHost),
	}
}

// AddHost 添加虚拟主机
func (v *VHostManager) AddHost(name string, handler fasthttp.RequestHandler) {
	v.hosts[name] = &VirtualHost{
		name:    name,
		handler: handler,
	}
}

// SetDefault 设置默认主机
func (v *VHostManager) SetDefault(handler fasthttp.RequestHandler) {
	v.defaultHost = &VirtualHost{
		name:    "default",
		handler: handler,
	}
}

// Handler 返回虚拟主机选择器
func (v *VHostManager) Handler() fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		host := string(ctx.Host())
		// 去除端口号
		for i := 0; i < len(host); i++ {
			if host[i] == ':' {
				host = host[:i]
				break
			}
		}

		if vhost, ok := v.hosts[host]; ok {
			vhost.handler(ctx)
		} else if v.defaultHost != nil {
			v.defaultHost.handler(ctx)
		} else {
			ctx.Error("Host not found", fasthttp.StatusNotFound)
		}
	}
}
