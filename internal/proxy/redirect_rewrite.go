// Package proxy 反向代理包，为 Lolly HTTP 服务器提供反向代理功能。
package proxy

import (
	"regexp"
	"strings"

	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/config"
	"rua.plus/lolly/internal/variable"
)

// RedirectRewrite 模式常量
const (
	redirectModeDefault = "default"
	redirectModeOff     = "off"
	redirectModeCustom  = "custom"
)

// compiledRule 预编译的改写规则
type compiledRule struct {
	pattern         *regexp.Regexp // 正则模式，nil 表示非正则匹配
	replacement     string         // 替换模板（含变量）
	exactMatch      string         // 精确匹配前缀（用于 prefix 匹配）
	caseInsensitive bool           // 正则大小写不敏感（~* 前缀）
}

// RedirectRewriter Location/Refresh 头改写器
type RedirectRewriter struct {
	proxyPath string         // 用于 default 模式（当前代理路径）
	mode      string         // "default" | "off" | "custom"（空字符串视为 default）
	rules     []compiledRule // 仅 custom 模式预编译
}

// NewRedirectRewriter 创建改写器
// proxyPath: 当前代理路径（如 "/api/"）
// 注意：mode 为空字符串时默认为 "default"
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

// Mode 返回当前模式（处理空字符串默认值）
func (r *RedirectRewriter) Mode() string {
	if r.mode == "" {
		return redirectModeDefault
	}
	return r.mode
}

// RewriteResponse 改写响应中的 Location 和 Refresh 头
// targetURL: 实际选中的上游地址（用于 default 模式）
// originalClientHost: 客户端原始 Host（在 modifyRequestHeaders 改写前保存）
// 调用位置：必须在 modifyResponseHeaders 之前
// 内部逻辑：
//   - 检查 resp.StatusCode()，仅 3xx 状态码处理 Location 头
//   - 所有状态码都处理 Refresh 头
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

// RewriteRefreshOnly 仅改写 Refresh 头（用于缓存响应路径）
// Location 头在缓存响应中不存在（缓存仅存储 2xx），故跳过
func (r *RedirectRewriter) RewriteRefreshOnly(resp *fasthttp.Response, ctx *fasthttp.RequestCtx, targetURL string, originalClientHost string) {
	refresh := resp.Header.Peek("Refresh")
	if len(refresh) > 0 {
		rewritten := r.rewriteRefresh(string(refresh), ctx, targetURL, originalClientHost)
		if rewritten != "" {
			resp.Header.Set("Refresh", rewritten)
		}
	}
}

// rewriteURL 改写单个 URL 值（Location 或 Refresh 中的 URL 部分）
// originalClientHost: 客户端原始 Host（用于 default 模式构建 replacement）
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

// rewriteDefault 动态生成 default 规则（运行时）
// 使用前缀匹配：如果 headerValue 以 targetURL 开头，替换为 replacement + 原路径后缀
// replacement 使用 originalClientHost 构建："$scheme://originalClientHost/"
// 例如：targetURL="http://backend:8000", headerValue="http://backend:8000/api/v2/users"
//
//	→ 替换为 "$scheme://originalClientHost/api/v2/users"
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

// rewriteCustom 使用预编译的 custom 规则改写 URL
// 规则按顺序匹配，第一个成功的生效
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

// rewriteRefresh 改写 Refresh 头
// 格式：`N; url=URL` 或 `N;url=URL`（无空格）或纯数字 `N`
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
