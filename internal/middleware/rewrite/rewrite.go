// Package rewrite 提供 URL 重写中间件，支持正则表达式匹配和多种重写标志。
package rewrite

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/config"
)

// Flag 重写标志类型。
type Flag int

const (
	// FlagLast 继续匹配其他规则。
	FlagLast Flag = iota
	// FlagRedirect 返回 302 临时重定向。
	FlagRedirect
	// FlagPermanent 返回 301 永久重定向。
	FlagPermanent
	// FlagBreak 停止匹配规则。
	FlagBreak
)

// parseFlag 解析配置中的标志字符串。
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
	// pattern 正则匹配模式
	pattern *regexp.Regexp
	// replacement 替换字符串，支持 $1、$2 等捕获组
	replacement string
	// flag 执行标志，控制重写后行为
	flag Flag
}

// RewriteMiddleware URL 重写中间件。
type RewriteMiddleware struct {
	// rules 编译后的规则列表，按配置顺序执行
	rules []Rule
}

// New 创建重写中间件。
func New(rules []config.RewriteRule) (*RewriteMiddleware, error) {
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
	return &RewriteMiddleware{rules: compiled}, nil
}

// validateRegexSafety 验证正则表达式的安全性，防止 ReDoS 攻击。
//
// 检测可能导致灾难性回溯的危险模式，如嵌套量词。
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
func (m *RewriteMiddleware) Name() string {
	return "rewrite"
}

// Process 应用重写规则。
func (m *RewriteMiddleware) Process(next fasthttp.RequestHandler) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		path := string(ctx.Path())
		originalPath := path

		for _, rule := range m.rules {
			if rule.pattern.MatchString(path) {
				// 执行正则替换
				newPath := rule.pattern.ReplaceAllString(path, rule.replacement)

				switch rule.flag {
				case FlagRedirect:
					ctx.Redirect(newPath, fasthttp.StatusFound)
					return
				case FlagPermanent:
					ctx.Redirect(newPath, fasthttp.StatusMovedPermanently)
					return
				case FlagBreak:
					// 修改路径后停止匹配
					ctx.Request.SetRequestURI(newPath)
					next(ctx)
					return
				case FlagLast:
					// 修改路径，继续匹配其他规则
					path = newPath
					ctx.Request.SetRequestURI(path)
				}
			}
		}

		// 如果路径被修改过，需要重新设置
		if path != originalPath {
			ctx.Request.SetRequestURI(path)
		}

		next(ctx)
	}
}

// Rules 返回编译后的规则列表（用于调试）。
func (m *RewriteMiddleware) Rules() []Rule {
	return m.rules
}
