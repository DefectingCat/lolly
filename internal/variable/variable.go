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
	"maps"
	"strconv"
	"strings"
	"sync"

	"github.com/valyala/fasthttp"
)

// BuiltinVariable 内置变量定义
type BuiltinVariable struct {
	Getter      func(ctx *fasthttp.RequestCtx) string
	GetterBytes func(ctx *fasthttp.RequestCtx) []byte // 零拷贝 getter，用于 EphemeralGet
	Name        string
	Description string
}

// Context 变量上下文，绑定到请求
type Context struct {
	ctx                  *fasthttp.RequestCtx
	store                map[string]string
	cache                map[string]string // string 缓存（用于 PersistentGet）
	bytesCache           map[string][]byte // []byte 缓存（用于 EphemeralGet）
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
	New: func() any {
		return &Context{
			store:      make(map[string]string),
			cache:      make(map[string]string),
			bytesCache: make(map[string][]byte),
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
	maps.Copy(globalVariables, vars)
}

// GetGlobalVariable 获取全局变量值。
//
// 线程安全地查询全局自定义变量存储。
// 使用读锁保护，可在多个 goroutine 中并发调用。
//
// 参数：
//   - name: 变量名称
//
// 返回值：
//   - string: 变量值，不存在时返回空字符串
//   - bool: 变量是否存在，true 表示存在，false 表示不存在
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
//
// 线程安全地返回全局自定义变量存储的完整快照。
// 返回的是副本，外部修改不会影响全局存储。
//
// 返回值：
//   - map[string]string: 所有全局变量的键值对副本，未初始化时返回 nil
func GetAllGlobalVariables() map[string]string {
	globalVariablesLock.RLock()
	defer globalVariablesLock.RUnlock()
	if globalVariables == nil {
		return nil
	}
	// 返回副本，避免外部修改影响全局存储
	result := make(map[string]string, len(globalVariables))
	maps.Copy(result, globalVariables)
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

// NewContext 从池中获取 Context。
// 全局变量通过 Get() 惰性加载。
func NewContext(ctx *fasthttp.RequestCtx) *Context {
	vc, ok := pool.Get().(*Context)
	if !ok {
		// 池中类型不正确时返回新 Context
		return &Context{ctx: ctx}
	}
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
	// 清空内置变量缓存
	for k := range vc.cache {
		delete(vc.cache, k)
	}
	// 清空内置变量 bytes 缓存
	for k := range vc.bytesCache {
		delete(vc.bytesCache, k)
	}
	// 清空自定义变量 store
	for k := range vc.store {
		delete(vc.store, k)
	}
	return vc
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

// Get 获取变量值（优先自定义变量，再查全局变量，最后查内置变量）
func (vc *Context) Get(name string) (string, bool) {
	// 1. 先查自定义变量
	if v, ok := vc.store[name]; ok {
		return v, true
	}

	// 2. 惰性加载全局变量（首次访问时查找，避免每请求复制）
	if v, ok := GetGlobalVariable(name); ok {
		return v, true
	}

	// 3. 检查从 SetResponseInfo/SetServerName 设置的值
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

// EphemeralGet 获取请求作用域内的变量值（返回 []byte）。
//
// 警告：返回的 []byte 在请求结束后失效。
// 安全用法：仅用于即时消费场景（如写入日志、响应头）。
// 如需持久化存储，请使用 PersistentGet()。
//
// 该方法通过 BuiltinVariable 中注册的 GetterBytes 函数
// 提供零拷贝访问变量值。
func (vc *Context) EphemeralGet(name string) []byte {
	// 1. 先查自定义变量（需要转换为 []byte）
	if v, ok := vc.store[name]; ok {
		return []byte(v) // 注意：这里分配了，因为 store 是 string
	}

	// 2. 查全局变量（需要转换为 []byte）
	if v, ok := GetGlobalVariable(name); ok {
		return []byte(v) // 注意：这里分配了，因为全局变量是 string
	}

	// 3. 检查 bytesCache 缓存
	if v, ok := vc.bytesCache[name]; ok {
		return v
	}

	// 4. 检查从 SetResponseInfo/SetServerName 设置的值
	// SAFETY: 这些值来自 struct 字段，在请求期间有效
	switch name {
	case VarStatus:
		if vc.status > 0 {
			b := []byte(strconv.Itoa(vc.status))
			vc.bytesCache[name] = b
			return b
		}
		if v := vc.ctx.UserValue(VarStatus); v != nil {
			if i, ok := v.(int); ok {
				b := []byte(strconv.Itoa(i))
				vc.bytesCache[name] = b
				return b
			}
		}
	case VarBodyBytesSent:
		if vc.bodySize > 0 {
			b := []byte(strconv.FormatInt(vc.bodySize, 10))
			vc.bytesCache[name] = b
			return b
		}
		if v := vc.ctx.UserValue(VarBodyBytesSent); v != nil {
			if i, ok := v.(int64); ok {
				b := []byte(strconv.FormatInt(i, 10))
				vc.bytesCache[name] = b
				return b
			}
		}
		b := []byte("0")
		vc.bytesCache[name] = b
		return b
	case VarRequestTime:
		if vc.duration > 0 {
			seconds := float64(vc.duration) / 1e9
			b := []byte(strconv.FormatFloat(seconds, 'f', 3, 64))
			vc.bytesCache[name] = b
			return b
		}
		if v := vc.ctx.UserValue(VarRequestTime); v != nil {
			if i, ok := v.(int64); ok {
				seconds := float64(i) / 1e9
				b := []byte(strconv.FormatFloat(seconds, 'f', 3, 64))
				vc.bytesCache[name] = b
				return b
			}
		}
		b := []byte("0.000")
		vc.bytesCache[name] = b
		return b
	case VarServerName:
		if vc.serverName != "" {
			return []byte(vc.serverName)
		}
	// 上游变量
	case VarUpstreamAddr:
		if vc.upstreamAddr != "" {
			return []byte(vc.upstreamAddr)
		}
		return []byte("-")
	case VarUpstreamStatus:
		if vc.upstreamStatus > 0 {
			b := []byte(strconv.Itoa(vc.upstreamStatus))
			vc.bytesCache[name] = b
			return b
		}
		return []byte("-")
	case VarUpstreamResponseTime:
		if vc.upstreamResponseTime > 0 {
			b := []byte(strconv.FormatFloat(vc.upstreamResponseTime, 'f', 3, 64))
			vc.bytesCache[name] = b
			return b
		}
		return []byte("-")
	case VarUpstreamConnectTime:
		if vc.upstreamConnectTime > 0 {
			b := []byte(strconv.FormatFloat(vc.upstreamConnectTime, 'f', 3, 64))
			vc.bytesCache[name] = b
			return b
		}
		return []byte("-")
	case VarUpstreamHeaderTime:
		if vc.upstreamHeaderTime > 0 {
			b := []byte(strconv.FormatFloat(vc.upstreamHeaderTime, 'f', 3, 64))
			vc.bytesCache[name] = b
			return b
		}
		return []byte("-")
	}

	// 5. 使用 GetterBytes 求值内置变量（零拷贝）
	if b, ok := vc.evalBuiltinBytes(name); ok {
		vc.bytesCache[name] = b
		return b
	}

	// 6. 如果只有 Getter，调用并转换为 []byte
	if v, ok := vc.evalBuiltin(name); ok {
		b := []byte(v)
		vc.bytesCache[name] = b
		return b
	}

	return nil
}

// PersistentGet 获取持久化字符串变量值。
//
// 当需要跨请求存储变量值时使用此方法
// （如保存到数据库、缓存或长期存活的结构体中）。
func (vc *Context) PersistentGet(name string) string {
	// 直接调用 Get，它返回 string
	v, _ := vc.Get(name)
	return v
}

// evalBuiltinBytes 求值内置变量，返回 []byte（零拷贝）
func (vc *Context) evalBuiltinBytes(name string) ([]byte, bool) {
	builtin := builtinVars[name]
	if builtin == nil || builtin.GetterBytes == nil {
		return nil, false
	}
	return builtin.GetterBytes(vc.ctx), true
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
