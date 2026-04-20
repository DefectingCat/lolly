// Package rewrite 提供 URL 重写中间件，支持正则表达式匹配和多种重写标志。
//
// 该文件包含 URL 重写相关的核心功能，包括：
//   - 正则表达式匹配和替换
//   - 多种重写标志（last、redirect、permanent、break）
//   - 变量展开支持
//   - ReDoS 安全防护
//
// 主要用途：
//
//	用于实现类似 nginx rewrite 模块的 URL 重写功能，支持灵活的 URL 变换规则。
//
// 注意事项：
//   - 重写规则按配置顺序执行，FlagLast 规则会重新从第一条规则开始匹配
//   - 最大迭代次数限制防止无限循环
//   - 正则表达式会进行安全性检查，防止 ReDoS 攻击
//
// 作者：xfy
package rewrite

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/config"
	"rua.plus/lolly/internal/variable"
)

// MaxRewriteIterations URL重写最大迭代次数，防止无限循环
const MaxRewriteIterations = 10

// Flag 重写标志类型。
type Flag int

const (
	// FlagLast 继续匹配其他规则（nginx 行为：重新从第一条规则开始匹配）。
	// 匹配到规则后会重新从第一条规则开始遍历，用于多规则链式重写。
	FlagLast Flag = iota
	// FlagRedirect 返回 302 临时重定向。
	// 客户端收到 302 响应后重新请求新 URL，不会继续匹配后续规则。
	FlagRedirect
	// FlagPermanent 返回 301 永久重定向。
	// 客户端收到 301 响应后永久重定向到新 URL，不会继续匹配后续规则。
	FlagPermanent
	// FlagBreak 停止匹配规则。
	// 修改请求路径后终止重写流程，直接进入下一个处理器。
	FlagBreak
)

// parseFlag 解析配置中的标志字符串为 Flag 枚举值。
//
// 将配置字符串转换为对应的 Flag 类型，用于控制重写后的行为。
//
// 参数：
//   - s: 标志字符串，支持 "last"、"redirect"、"permanent"、"break"
//
// 返回值：
//   - Flag: 对应的标志枚举值，无法识别时返回 FlagLast
func parseFlag(s string) Flag {
	switch strings.ToLower(s) {
	case "redirect":
		return FlagRedirect
	case "permanent":
		return FlagPermanent
	case "break":
		return FlagBreak
	default:
		return FlagLast
	}
}

// Rule 编译后的重写规则。
type Rule struct {
	// pattern 正则匹配模式，用于匹配请求路径
	pattern *regexp.Regexp
	// replacement 替换字符串，支持 $1、$2 等捕获组和变量展开
	replacement string
	// flag 执行标志，控制重写后的行为（last/redirect/permanent/break）
	flag Flag
}

// Middleware URL 重写中间件。
type Middleware struct {
	// rules 编译后的规则列表，按配置顺序执行
	rules []Rule
}

// New 创建 URL 重写中间件。
//
// 编译配置中的重写规则，验证正则表达式安全性后返回中间件实例。
//
// 参数：
//   - rules: 重写规则配置列表，包含模式、替换和标志
//
// 返回值：
//   - *Middleware: 编译后的重写中间件
//   - error: 正则表达式无效或不安全时返回错误
func New(rules []config.RewriteRule) (*Middleware, error) {
	compiled := make([]Rule, 0, len(rules))
	for _, r := range rules {
		// 验证正则表达式安全性，防止 ReDoS
		if err := validateRegexSafety(r.Pattern); err != nil {
			return nil, fmt.Errorf("unsafe regex pattern %q: %w", r.Pattern, err)
		}

		re, err := regexp.Compile(r.Pattern)
		if err != nil {
			return nil, err
		}
		compiled = append(compiled, Rule{
			pattern:     re,
			replacement: r.Replacement,
			flag:        parseFlag(r.Flag),
		})
	}
	return &Middleware{rules: compiled}, nil
}

// validateRegexSafety 验证正则表达式的安全性，防止 ReDoS 攻击。
//
// 检测可能导致灾难性回溯的危险模式，如嵌套量词。
// 限制模式最大长度 1000 字符。
//
// 参数：
//   - pattern: 待验证的正则表达式字符串
//
// 返回值：
//   - error: 检测到不安全模式时返回错误
func validateRegexSafety(pattern string) error {
	// 限制模式长度
	if len(pattern) > 1000 {
		return fmt.Errorf("pattern too long (max 1000 chars)")
	}

	// 检测危险模式：嵌套量词
	// 例如：(\w+)+, (\d+)+, (a+)+, (.+)+
	dangerousPatterns := []string{
		`(\w+)+`, `(\d+)+`, `(a+)+`, `(.+)+`,
		`(\w*)*`, `(\d*)*`, `(a*)*`, `(.*)*`,
		`(\w+)?+`, `(\d+)?+`,
	}

	for _, dangerous := range dangerousPatterns {
		if strings.Contains(pattern, dangerous) {
			return fmt.Errorf("potential catastrophic backtracking pattern detected")
		}
	}

	return nil
}

// Name 返回中间件名称。
func (m *Middleware) Name() string {
	return "rewrite"
}

// Process 应用重写规则。
//
// 对请求路径执行正则匹配和替换，根据标志控制后续行为。
// 支持迭代重写（FlagLast 会重新从第一条规则开始匹配）。
//
// 参数：
//   - next: 下一个请求处理器
//
// 返回值：
//   - fasthttp.RequestHandler: 包装后的请求处理器
func (m *Middleware) Process(next fasthttp.RequestHandler) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		path := string(ctx.Path())
		originalPath := path

		// 全局迭代计数器，用于检测循环（每次重写都计入迭代）
		iterationCount := 0
		// 规则索引，支持FlagLast后重新开始匹配
		ruleIndex := 0

		for ruleIndex < len(m.rules) {
			// 步骤1: 检查迭代次数是否超过限制（防止无限循环）
			if iterationCount >= MaxRewriteIterations {
				ctx.Error("Internal Server Error", fasthttp.StatusInternalServerError)
				return
			}

			rule := m.rules[ruleIndex]

			if rule.pattern.MatchString(path) {
				// 步骤2: 执行正则替换
				newPath := rule.pattern.ReplaceAllString(path, rule.replacement)

				// 步骤3: 对替换结果进行变量展开
				vc := variable.NewContext(ctx)
				newPath = vc.Expand(newPath)
				variable.ReleaseContext(vc)

				// 步骤4: 根据标志决定后续行为
				switch rule.flag {
				case FlagRedirect:
					// 302 临时重定向
					ctx.Redirect(newPath, fasthttp.StatusFound)
					return
				case FlagPermanent:
					// 301 永久重定向
					ctx.Redirect(newPath, fasthttp.StatusMovedPermanently)
					return
				case FlagBreak:
					// 修改路径后停止匹配，直接进入处理器
					ctx.Request.SetRequestURI(newPath)
					next(ctx)
					return
				case FlagLast:
					// 修改路径，并重新从第一条规则开始匹配（nginx兼容行为）
					path = newPath
					ctx.Request.SetRequestURI(path)
					iterationCount++ // 每次FlagLast重写都增加计数
					ruleIndex = 0    // 重新从第一条规则开始
					continue
				}
			}

			ruleIndex++
		}

		// 步骤5: 如果路径被修改过，需要重新设置
		if path != originalPath {
			ctx.Request.SetRequestURI(path)
		}

		next(ctx)
	}
}

// Rules 返回编译后的规则列表（用于调试）。
//
// 返回值：
//   - []Rule: 编译后的重写规则列表
func (m *Middleware) Rules() []Rule {
	return m.rules
}
