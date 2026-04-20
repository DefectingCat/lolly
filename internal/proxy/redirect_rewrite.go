// Package proxy 反向代理包，为 Lolly HTTP 服务器提供反向代理功能。
//
// 该文件实现 Location 和 Refresh 响应头的改写功能。
// 当上游服务器返回重定向响应时，将 Location 头中的上游地址
// 改写为代理地址，确保客户端能正确访问。
//
// 主要功能：
//   - default 模式：自动将上游地址替换为客户端原始 Host
//   - custom 模式：使用自定义规则（正则/前缀匹配）进行改写
//   - off 模式：禁用改写
//   - Refresh 头改写：支持 "N; url=URL" 格式
//   - 变量展开：自定义规则中支持 Lolly 变量
//
// 主要用途：
//   用于处理上游服务器返回的 3xx 重定向响应，确保客户端
//   收到的是代理地址而非内部上游地址。
//
// 注意事项：
//   - 调用位置：必须在 modifyResponseHeaders 之前调用
//   - default 模式使用前缀匹配，防止部分匹配问题
//
// 作者：xfy
package proxy

import (
	"regexp"
	"strings"

	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/config"
	"rua.plus/lolly/internal/variable"
)

// redirect_rewrite 模式常量，配置 redirect_rewrite.mode 字段。
const (
	redirectModeDefault = "default" // 默认模式：自动替换上游地址为客户端 Host
	redirectModeOff     = "off"     // 关闭模式：不执行任何改写
	redirectModeCustom  = "custom"  // 自定义模式：使用预编译规则进行改写
)

// compiledRule 预编译的改写规则。
//
// 用于 custom 模式，在初始化时编译以避免运行时开销。
type compiledRule struct {
	pattern         *regexp.Regexp // 正则表达式模式，nil 表示使用前缀匹配
	replacement     string         // 替换模板，支持 Lolly 变量展开
	exactMatch      string         // 前缀匹配字符串（非正则模式下使用）
	caseInsensitive bool           // 正则匹配时是否忽略大小写（~* 前缀触发）
}

// RedirectRewriter Location 和 Refresh 响应头改写器。
//
// 根据配置的模式对上游返回的重定向响应进行改写，确保客户端
// 收到的是代理地址而非内部上游服务器地址。
type RedirectRewriter struct {
	proxyPath string         // 当前代理路径（如 "/api/"），用于 default 模式
	mode      string         // 改写模式："default" | "off" | "custom"，空字符串视为 default
	rules     []compiledRule // 预编译的改写规则，仅 custom 模式下使用
}

// NewRedirectRewriter 创建重定向改写器实例。
//
// 根据配置模式初始化改写器：
//   - nil 配置或未指定模式：默认启用 default 模式
//   - custom 模式：预编译所有改写规则（正则/前缀匹配）
//
// 参数：
//   - cfg: 重定向改写配置，nil 时使用 default 模式
//   - proxyPath: 当前代理路径（如 "/api/"）
//
// 返回值：
//   - *RedirectRewriter: 改写器实例
//   - error: 正则表达式编译失败时返回错误
func NewRedirectRewriter(cfg *config.RedirectRewriteConfig, proxyPath string) (*RedirectRewriter, error) {
	if cfg == nil {
		// 未配置时默认启用 default 模式
		return &RedirectRewriter{
			mode:      redirectModeDefault,
			proxyPath: proxyPath,
		}, nil
	}

	rw := &RedirectRewriter{
		mode:      cfg.Mode,
		proxyPath: proxyPath,
	}

	// custom 模式：预编译规则
	if cfg.Mode == redirectModeCustom {
		rules := make([]compiledRule, 0, len(cfg.Rules))
		for _, rule := range cfg.Rules {
			cr := compiledRule{
				replacement: rule.Replacement,
			}

			if strings.HasPrefix(rule.Pattern, "~") {
				// 正则模式
				var patternStr string
				if strings.HasPrefix(rule.Pattern, "~*") {
					cr.caseInsensitive = true
					patternStr = rule.Pattern[2:]
				} else {
					patternStr = rule.Pattern[1:]
				}
				re, err := regexp.Compile(patternStr)
				if err != nil {
					return nil, err
				}
				cr.pattern = re
			} else {
				// 非正则：使用前缀匹配
				cr.exactMatch = rule.Pattern
			}

			rules = append(rules, cr)
		}
		rw.rules = rules
	}

	return rw, nil
}

// Mode 返回当前改写模式。
//
// 处理空字符串默认值，空字符串视为 "default" 模式。
//
// 返回值：
//   - string: 当前模式（"default"、"off" 或 "custom"）
func (r *RedirectRewriter) Mode() string {
	if r.mode == "" {
		return redirectModeDefault
	}
	return r.mode
}

// RewriteResponse 改写响应中的 Location 和 Refresh 头。
//
// 处理逻辑：
//   - 仅对 3xx 状态码处理 Location 头（重定向响应）
//   - 所有状态码都处理 Refresh 头
//
// 调用位置：必须在 modifyResponseHeaders 之前调用。
//
// 参数：
//   - resp: 上游响应
//   - ctx: FastHTTP 请求上下文，用于变量展开
//   - targetURL: 实际选中的上游地址（用于 default 模式）
//   - originalClientHost: 客户端原始 Host（在 modifyRequestHeaders 改写前保存）
func (r *RedirectRewriter) RewriteResponse(resp *fasthttp.Response, ctx *fasthttp.RequestCtx, targetURL string, originalClientHost string) {
	statusCode := resp.StatusCode()

	// 仅 3xx 状态码处理 Location 头
	if statusCode >= 300 && statusCode < 400 {
		location := resp.Header.Peek("Location")
		if len(location) > 0 {
			rewritten := r.rewriteURL(string(location), ctx, targetURL, originalClientHost)
			if rewritten != "" {
				resp.Header.Set("Location", rewritten)
			}
		}
	}

	// 所有状态码都处理 Refresh 头
	refresh := resp.Header.Peek("Refresh")
	if len(refresh) > 0 {
		rewritten := r.rewriteRefresh(string(refresh), ctx, targetURL, originalClientHost)
		if rewritten != "" {
			resp.Header.Set("Refresh", rewritten)
		}
	}
}

// RewriteRefreshOnly 仅改写 Refresh 头（用于缓存响应路径）。
//
// 缓存响应中仅存储 2xx 状态码，不存在 Location 头，
// 故跳过 Location 处理，仅处理 Refresh 头。
//
// 参数：
//   - resp: 缓存响应
//   - ctx: FastHTTP 请求上下文
//   - targetURL: 上游地址
//   - originalClientHost: 客户端原始 Host
func (r *RedirectRewriter) RewriteRefreshOnly(resp *fasthttp.Response, ctx *fasthttp.RequestCtx, targetURL string, originalClientHost string) {
	refresh := resp.Header.Peek("Refresh")
	if len(refresh) > 0 {
		rewritten := r.rewriteRefresh(string(refresh), ctx, targetURL, originalClientHost)
		if rewritten != "" {
			resp.Header.Set("Refresh", rewritten)
		}
	}
}

// rewriteURL 改写单个 URL 值（Location 或 Refresh 头中的 URL 部分）。
//
// 根据当前模式选择对应的改写逻辑：
//   - off: 原样返回
//   - custom: 使用预编译规则改写
//   - default: 动态替换上游地址为客户端 Host
//
// 参数：
//   - headerValue: 头中的 URL 值
//   - ctx: FastHTTP 请求上下文
//   - targetURL: 上游目标地址
//   - originalClientHost: 客户端原始 Host
//
// 返回值：
//   - string: 改写后的 URL，未匹配时返回原始值
func (r *RedirectRewriter) rewriteURL(headerValue string, ctx *fasthttp.RequestCtx, targetURL string, originalClientHost string) string {
	if headerValue == "" {
		return ""
	}

	switch r.Mode() {
	case redirectModeOff:
		return headerValue

	case redirectModeCustom:
		return r.rewriteCustom(headerValue, ctx)

	case redirectModeDefault, "":
		return r.rewriteDefault(headerValue, ctx, targetURL, originalClientHost)

	default:
		return headerValue
	}
}

// rewriteDefault 使用 default 模式动态改写 URL。
//
// 使用前缀匹配：如果 headerValue 以 targetURL 开头，且后面紧跟
// "/"、"?"、"#" 或字符串结束，则将 targetURL 部分替换为
// "$scheme://originalClientHost"。
//
// 防止部分匹配问题：例如 "https://www.google.com" 不会
// 错误匹配 "https://www.google.com.hk"。
//
// 参数：
//   - headerValue: 原始 URL 值
//   - ctx: FastHTTP 请求上下文（用于判断 TLS 协议）
//   - targetURL: 上游目标地址
//   - originalClientHost: 客户端原始 Host
//
// 返回值：
//   - string: 改写后的 URL，未匹配时返回原始值
func (r *RedirectRewriter) rewriteDefault(headerValue string, ctx *fasthttp.RequestCtx, targetURL string, originalClientHost string) string {
	if targetURL == "" {
		return headerValue
	}

	// 精确前缀匹配：headerValue 以 targetURL 开头，且后面是 / ? # 或结束
	// 防止 "https://www.google.com" 匹配 "https://www.google.com.hk"（后者后面是 .hk）
	if strings.HasPrefix(headerValue, targetURL) {
		remaining := headerValue[len(targetURL):]
		// 检查剩余部分是否以合法分隔符开头
		if len(remaining) == 0 || remaining[0] == '/' || remaining[0] == '?' || remaining[0] == '#' {
			// 使用客户端原始 host 构建 replacement
			scheme := protoHTTP
			if ctx.IsTLS() {
				scheme = protoHTTPS
			}
			replacement := scheme + "://" + originalClientHost
			return replacement + remaining
		}
	}

	return headerValue
}

// rewriteCustom 使用预编译的 custom 规则改写 URL。
//
// 规则按定义顺序匹配，第一个成功的规则生效：
//   - 正则匹配（~ 或 ~* 前缀）：支持大小写不敏感
//   - 前缀匹配（无特殊前缀）：使用 HasPrefix 精确前缀匹配
// 替换模板支持 Lolly 变量展开。
//
// 参数：
//   - headerValue: 原始 URL 值
//   - ctx: FastHTTP 请求上下文
//
// 返回值：
//   - string: 改写后的 URL，未匹配时返回原始值
func (r *RedirectRewriter) rewriteCustom(headerValue string, ctx *fasthttp.RequestCtx) string {
	vc := variable.NewContext(ctx)
	defer variable.ReleaseContext(vc)

	for _, rule := range r.rules {
		if rule.pattern != nil {
			// 正则匹配
			if rule.caseInsensitive {
				// 大小写不敏感：先将 headerValue 转为小写匹配，但替换时保留原始值
				lowerValue := strings.ToLower(headerValue)
				loc := rule.pattern.FindStringIndex(lowerValue)
				if loc != nil {
					expanded := vc.Expand(rule.replacement)
					result := headerValue[:loc[0]] + expanded + headerValue[loc[1]:]
					return result
				}
			} else {
				loc := rule.pattern.FindStringIndex(headerValue)
				if loc != nil {
					expanded := vc.Expand(rule.replacement)
					result := headerValue[:loc[0]] + expanded + headerValue[loc[1]:]
					return result
				}
			}
		} else if rule.exactMatch != "" {
			// 前缀匹配
			if strings.HasPrefix(headerValue, rule.exactMatch) {
				expanded := vc.Expand(rule.replacement)
				suffix := strings.TrimPrefix(headerValue, rule.exactMatch)
				return expanded + suffix
			}
		}
	}

	return headerValue
}

// rewriteRefresh 改写 Refresh 响应头。
//
// Refresh 头格式：`N; url=URL` 或 `N;url=URL`（无空格）或纯数字 `N`。
// 该方法提取 URL 部分进行改写，保持延迟值不变。
//
// 参数：
//   - value: 原始 Refresh 头值
//   - ctx: FastHTTP 请求上下文
//   - targetURL: 上游目标地址
//   - originalClientHost: 客户端原始 Host
//
// 返回值：
//   - string: 改写后的 Refresh 头值，URL 未变化时返回原始值
func (r *RedirectRewriter) rewriteRefresh(value string, ctx *fasthttp.RequestCtx, targetURL string, originalClientHost string) string {
	delay, url, valid := parseRefreshHeader(value)
	if !valid || url == "" {
		// 无法解析或无 URL，原样返回
		return value
	}

	rewrittenURL := r.rewriteURL(url, ctx, targetURL, originalClientHost)
	if rewrittenURL == url {
		// URL 未变化，原样返回
		return value
	}

	return delay + "; url=" + rewrittenURL
}

// parseRefreshHeader 解析 Refresh 头格式
// 格式：`N; url=URL` 或 `N;url=URL`（无空格）或纯数字 `N`
// 返回：delay(N), url(URL), 是否有效
// 边缘处理：忽略引号、忽略多余参数
func parseRefreshHeader(value string) (delay string, url string, valid bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", "", false
	}

	// 查找 url= 部分
	urlIdx := strings.Index(strings.ToLower(value), "url=")
	if urlIdx == -1 {
		// 纯数字格式，有效但无 URL
		return value, "", true
	}

	// 提取 delay 部分（url= 之前的部分）
	delay = strings.TrimSpace(value[:urlIdx])
	// 去除 delay 末尾的分号
	delay = strings.TrimSuffix(delay, ";")
	delay = strings.TrimSpace(delay)
	if delay == "" {
		return "", "", false
	}

	// 提取 URL 部分（url= 之后）
	url = strings.TrimSpace(value[urlIdx+4:])
	if url == "" {
		return delay, "", true
	}

	// 去除 URL 两端的引号（如果有）
	if len(url) >= 2 {
		if (url[0] == '"' && url[len(url)-1] == '"') ||
			(url[0] == '\'' && url[len(url)-1] == '\'') {
			url = url[1 : len(url)-1]
		}
	}

	return delay, url, true
}
