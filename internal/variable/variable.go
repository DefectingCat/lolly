// Package variable 提供高性能的变量系统，支持 nginx 风格的变量展开。
//
// 该包实现了统一的变量存储和展开机制，用于：
//   - 访问日志格式模板
//   - 代理请求头设置
//   - URL 重写规则
//
// 支持的变量格式：
//   - $var: 简单变量
//   - ${var}: 带花括号的变量（用于变量后有字符的场景）
//
// 性能特性：
//   - 使用快速字符串扫描（非正则表达式）
//   - sync.Pool 复用 VariableContext
//   - 内置变量惰性求值并缓存
//
// 作者：xfy
package variable

import (
	"strconv"
	"strings"
	"sync"

	"github.com/valyala/fasthttp"
)

// Store 变量存储接口
type Store interface {
	// Get 获取变量值
	Get(name string) (string, bool)
	// Set 设置变量值（用于自定义变量）
	Set(name string, value string)
}

// BuiltinVariable 内置变量定义
type BuiltinVariable struct {
	Getter      func(ctx *fasthttp.RequestCtx) string
	Name        string
	Description string
}

// Context 变量上下文，绑定到请求
type Context struct {
	ctx                  *fasthttp.RequestCtx
	store                map[string]string
	cache                map[string]string
	serverName           string
	upstreamAddr         string
	status               int
	bodySize             int64
	duration             int64
	upstreamStatus       int
	upstreamResponseTime float64
	upstreamConnectTime  float64
	upstreamHeaderTime   float64
}

// pool 用于复用 Context
var pool = sync.Pool{
	New: func() interface{} {
		return &Context{
			store: make(map[string]string),
			cache: make(map[string]string),
		}
	},
}

// 全局自定义变量存储
var (
	globalVariables     map[string]string
	globalVariablesLock sync.RWMutex
)

// SetGlobalVariables 设置全局自定义变量。
// 在应用启动或配置重载时调用。
func SetGlobalVariables(vars map[string]string) {
	globalVariablesLock.Lock()
	defer globalVariablesLock.Unlock()
	globalVariables = make(map[string]string, len(vars))
	for k, v := range vars {
		globalVariables[k] = v
	}
}

// GetGlobalVariable 获取全局变量值。
func GetGlobalVariable(name string) (string, bool) {
	globalVariablesLock.RLock()
	defer globalVariablesLock.RUnlock()
	if globalVariables == nil {
		return "", false
	}
	v, ok := globalVariables[name]
	return v, ok
}

// GetAllGlobalVariables 获取所有全局变量的副本。
// 用于在 NewVariableContext 中批量注入。
func GetAllGlobalVariables() map[string]string {
	globalVariablesLock.RLock()
	defer globalVariablesLock.RUnlock()
	if globalVariables == nil {
		return nil
	}
	// 返回副本，避免外部修改影响全局存储
	result := make(map[string]string, len(globalVariables))
	for k, v := range globalVariables {
		result[k] = v
	}
	return result
}

// builtinVars 内置变量注册表
var builtinVars = make(map[string]*BuiltinVariable)

// RegisterBuiltin 注册内置变量
func RegisterBuiltin(v *BuiltinVariable) {
	builtinVars[v.Name] = v
}

// GetBuiltin 获取内置变量定义
func GetBuiltin(name string) *BuiltinVariable {
	return builtinVars[name]
}

// NewContext 从池中获取 Context，并注入全局变量。
func NewContext(ctx *fasthttp.RequestCtx) *Context {
	vc := pool.Get().(*Context) //nolint:errcheck // 类型断言
	vc.ctx = ctx
	vc.status = 0
	vc.bodySize = 0
	vc.duration = 0
	vc.serverName = ""
	vc.upstreamAddr = ""
	vc.upstreamStatus = 0
	vc.upstreamResponseTime = 0
	vc.upstreamConnectTime = 0
	vc.upstreamHeaderTime = 0
	// 清空缓存
	for k := range vc.cache {
		delete(vc.cache, k)
	}
	// 清空自定义变量 store，然后注入全局变量
	for k := range vc.store {
		delete(vc.store, k)
	}
	// 注入全局变量
	globals := GetAllGlobalVariables()
	for name, value := range globals {
		vc.store[name] = value
	}
	return vc
}

// NewVariableContext 是 NewContext 的别名（向后兼容）
func NewVariableContext(ctx *fasthttp.RequestCtx) *Context {
	return NewContext(ctx)
}

// ReleaseContext 释放 Context 回池中
func ReleaseContext(vc *Context) {
	if vc == nil {
		return
	}
	vc.ctx = nil
	vc.status = 0
	vc.bodySize = 0
	vc.duration = 0
	vc.serverName = ""
	vc.upstreamAddr = ""
	vc.upstreamStatus = 0
	vc.upstreamResponseTime = 0
	vc.upstreamConnectTime = 0
	vc.upstreamHeaderTime = 0
	pool.Put(vc)
}

// ReleaseVariableContext 是 ReleaseContext 的别名（向后兼容）
func ReleaseVariableContext(vc *Context) {
	ReleaseContext(vc)
}

// SetResponseInfo 设置响应信息（用于需要 status、body_bytes_sent、request_time 的场景）
func (vc *Context) SetResponseInfo(status int, bodySize int64, durationNs int64) {
	vc.status = status
	vc.bodySize = bodySize
	vc.duration = durationNs
}

// SetServerName 设置服务器名称
func (vc *Context) SetServerName(name string) {
	vc.serverName = name
}

// SetUpstreamVars 设置上游变量
func (vc *Context) SetUpstreamVars(addr string, status int, responseTime, connectTime, headerTime float64) {
	vc.upstreamAddr = addr
	vc.upstreamStatus = status
	vc.upstreamResponseTime = responseTime
	vc.upstreamConnectTime = connectTime
	vc.upstreamHeaderTime = headerTime
}

// Get 获取变量值（优先自定义变量，再查内置变量）
func (vc *Context) Get(name string) (string, bool) {
	// 1. 先查自定义变量
	if v, ok := vc.store[name]; ok {
		return v, true
	}

	// 2. 检查从 SetResponseInfo/SetServerName 设置的值
	// 优先检查 struct 字段，再检查 ctx.UserValue（兼容 SetResponseInfoInContext）
	switch name {
	case VarStatus:
		if vc.status > 0 {
			return strconv.Itoa(vc.status), true
		}
		if v := vc.ctx.UserValue(VarStatus); v != nil {
			if i, ok := v.(int); ok {
				return strconv.Itoa(i), true
			}
		}
	case VarBodyBytesSent:
		if vc.bodySize > 0 {
			return strconv.FormatInt(vc.bodySize, 10), true
		}
		if v := vc.ctx.UserValue(VarBodyBytesSent); v != nil {
			if i, ok := v.(int64); ok {
				return strconv.FormatInt(i, 10), true
			}
		}
		return "0", true
	case VarRequestTime:
		if vc.duration > 0 {
			// 转换为秒，保留 3 位小数
			seconds := float64(vc.duration) / 1e9
			return strconv.FormatFloat(seconds, 'f', 3, 64), true
		}
		if v := vc.ctx.UserValue(VarRequestTime); v != nil {
			if i, ok := v.(int64); ok {
				seconds := float64(i) / 1e9
				return strconv.FormatFloat(seconds, 'f', 3, 64), true
			}
		}
		return "0.000", true
	case VarServerName:
		if vc.serverName != "" {
			return vc.serverName, true
		}
	// 上游变量
	case VarUpstreamAddr:
		if vc.upstreamAddr != "" {
			return vc.upstreamAddr, true
		}
		return "-", true
	case VarUpstreamStatus:
		if vc.upstreamStatus > 0 {
			return strconv.Itoa(vc.upstreamStatus), true
		}
		return "-", true
	case VarUpstreamResponseTime:
		if vc.upstreamResponseTime > 0 {
			return strconv.FormatFloat(vc.upstreamResponseTime, 'f', 3, 64), true
		}
		return "-", true
	case VarUpstreamConnectTime:
		if vc.upstreamConnectTime > 0 {
			return strconv.FormatFloat(vc.upstreamConnectTime, 'f', 3, 64), true
		}
		return "-", true
	case VarUpstreamHeaderTime:
		if vc.upstreamHeaderTime > 0 {
			return strconv.FormatFloat(vc.upstreamHeaderTime, 'f', 3, 64), true
		}
		return "-", true
	}

	// 3. 查内置变量缓存
	if v, ok := vc.cache[name]; ok {
		return v, true
	}

	// 4. 求值内置变量并缓存
	if v, ok := vc.evalBuiltin(name); ok {
		vc.cache[name] = v
		return v, true
	}

	return "", false
}

// Set 设置自定义变量
func (vc *Context) Set(name string, value string) {
	vc.store[name] = value
}

// evalBuiltin 求值内置变量
func (vc *Context) evalBuiltin(name string) (string, bool) {
	builtin := builtinVars[name]
	if builtin == nil || builtin.Getter == nil {
		return "", false
	}
	return builtin.Getter(vc.ctx), true
}

// expandCore 是变量展开的核心实现
// lookup: 变量查找函数，返回变量值和是否找到
// keepOriginal: 当变量未找到时，是否保持原样（true=保持原样，false=替换为空字符串）
func expandCore(template string, lookup func(name string) (value string, found bool), keepOriginal bool) string {
	if template == "" {
		return ""
	}

	// 快速路径：没有变量
	hasVar := false
	for i := 0; i < len(template); i++ {
		if template[i] == '$' {
			hasVar = true
			break
		}
	}
	if !hasVar {
		return template
	}

	var result strings.Builder
	result.Grow(len(template) * 2)

	i := 0
	for i < len(template) {
		if template[i] != '$' {
			result.WriteByte(template[i])
			i++
			continue
		}

		// 到达末尾，保留 $
		if i+1 >= len(template) {
			result.WriteByte('$')
			i++
			continue
		}

		// ${var} 格式
		if template[i+1] == '{' {
			// 查找匹配的 }
			end := strings.IndexByte(template[i+2:], '}')
			if end == -1 {
				result.WriteByte('$')
				i++
				continue
			}
			// end 是相对 i+2 的偏移量
			varName := template[i+2 : i+2+end]
			if varName == "" {
				// 空变量名，保持 ${}
				result.WriteString("${}")
				i += 2 + end + 1
				continue
			}
			// 获取变量值
			if v, ok := lookup(varName); ok {
				result.WriteString(v)
			} else if keepOriginal {
				// 未定义变量，保持原样
				result.WriteString("${")
				result.WriteString(varName)
				result.WriteByte('}')
			}
			// i+2 是变量名开始，+end 是 } 的位置，+1 跳过 }
			i += 2 + end + 1
			continue
		}

		// $var 格式（变量名由字母、数字、下划线组成）
		j := i + 1
		for j < len(template) {
			c := template[j]
			if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' {
				j++
			} else {
				break
			}
		}

		if j == i+1 {
			// 变量名长度为0，保留 $
			result.WriteByte('$')
			i++
			continue
		}

		varName := template[i+1 : j]
		if v, ok := lookup(varName); ok {
			result.WriteString(v)
		} else if keepOriginal {
			// 未定义变量，保持原样
			result.WriteByte('$')
			result.WriteString(varName)
		}
		i = j // 跳过变量名
	}

	return result.String()
}

// Expand 展开模板字符串中的变量
// 支持 $var 和 ${var} 两种格式
// 对于未定义的变量，保持原样不变
func (vc *Context) Expand(template string) string {
	return expandCore(template, func(name string) (string, bool) {
		return vc.Get(name)
	}, true)
}

// ExpandString 展开字符串（静态函数，用于简单场景）
// 需要提供变量值查找函数
func ExpandString(template string, lookup func(string) string) string {
	return expandCore(template, func(name string) (string, bool) {
		v := lookup(name)
		return v, v != ""
	}, true)
}
