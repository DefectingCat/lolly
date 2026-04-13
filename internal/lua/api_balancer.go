// Package lua 提供 ngx.balancer API 实现
// 本文件实现负载均衡相关的 Lua API，用于在 Lua 脚本中选择后端目标
package lua

import (
	"net/url"
	"strings"

	glua "github.com/yuin/gopher-lua"
	"rua.plus/lolly/internal/loadbalance"
)

// BalancerContext Lua Balancer 上下文
type BalancerContext struct {
	LastError error
	Selected  *loadbalance.Target
	ClientIP  string
	Targets   []*loadbalance.Target
	Retries   int
	selected  bool
}

// RegisterBalancerAPI 注册 ngx.balancer API
func RegisterBalancerAPI(L *glua.LState, bctx *BalancerContext, ngx *glua.LTable) {
	balancer := L.NewTable()

	// set_current_peer(host, port) 或 set_current_peer(url)
	L.SetField(balancer, "set_current_peer", L.NewFunction(func(L *glua.LState) int {
		nargs := L.GetTop()

		var host, port string
		if nargs >= 2 {
			// set_current_peer(host, port) 形式
			host = L.CheckString(1)
			port = L.CheckString(2)
			if !strings.HasPrefix(port, ":") {
				port = ":" + port
			}
		} else if nargs == 1 {
			// set_current_peer(url) 形式
			targetURL := L.CheckString(1)
			u, err := url.Parse(targetURL)
			if err != nil {
				L.Push(glua.LBool(false))
				L.Push(glua.LString("invalid url: " + err.Error()))
				return 2
			}
			host = u.Hostname()
			port = ":" + u.Port()
			if u.Port() == "" {
				if u.Scheme == "https" {
					port = ":443"
				} else {
					port = ":80"
				}
			}
		} else {
			L.RaiseError("set_current_peer requires 1 or 2 arguments")
			return 0
		}

		// 在 Targets 中查找匹配的目标
		targetURL := "http://" + host + port
		for _, t := range bctx.Targets {
			if t.URL == targetURL || strings.HasPrefix(t.URL, targetURL) {
				bctx.Selected = t
				bctx.selected = true
				L.Push(glua.LBool(true))
				return 1
			}
		}

		L.Push(glua.LBool(false))
		L.Push(glua.LString("target not found: " + host + port))
		return 2
	}))

	// set_more_tries(count)
	L.SetField(balancer, "set_more_tries", L.NewFunction(func(L *glua.LState) int {
		count := L.CheckInt(1)
		bctx.Retries = count
		L.Push(glua.LBool(true))
		return 1
	}))

	// get_last_failure()
	L.SetField(balancer, "get_last_failure", L.NewFunction(func(L *glua.LState) int {
		if bctx.LastError == nil {
			L.Push(glua.LNil)
			return 1
		}
		// 返回失败类型: "failed", "timeout", "next"
		failType := classifyError(bctx.LastError)
		L.Push(glua.LString(failType))
		return 1
	}))

	// get_targets() - 返回所有可用目标
	L.SetField(balancer, "get_targets", L.NewFunction(func(L *glua.LState) int {
		targetsTable := L.NewTable()
		for i, t := range bctx.Targets {
			targetTable := L.NewTable()
			L.SetField(targetTable, "url", glua.LString(t.URL))
			L.SetField(targetTable, "weight", glua.LNumber(t.Weight))
			L.SetField(targetTable, "healthy", glua.LBool(t.Healthy.Load()))
			targetsTable.RawSetInt(i+1, targetTable)
		}
		L.Push(targetsTable)
		return 1
	}))

	// get_client_ip()
	L.SetField(balancer, "get_client_ip", L.NewFunction(func(L *glua.LState) int {
		L.Push(glua.LString(bctx.ClientIP))
		return 1
	}))

	L.SetField(ngx, "balancer", balancer)
}

// IsSelected 检查是否调用了 set_current_peer
func (bctx *BalancerContext) IsSelected() bool {
	return bctx.selected
}

// classifyError 分类错误类型
func classifyError(err error) string {
	if err == nil {
		return ""
	}
	// 根据错误类型返回字符串
	errStr := err.Error()
	if strings.Contains(errStr, "timeout") {
		return "timeout"
	}
	if strings.Contains(errStr, "connection") {
		return "failed"
	}
	return "failed"
}
