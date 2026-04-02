package server

import (
	"github.com/valyala/fasthttp"
)

// VHostManager 虚拟主机管理器
type VHostManager struct {
	hosts       map[string]*VirtualHost // 按 server_name 索引
	defaultHost *VirtualHost            // 默认主机
}

// VirtualHost 虚拟主机
type VirtualHost struct {
	name    string
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
