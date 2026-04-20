// Package matcher 提供 nginx 风格的 location 匹配引擎实现。
//
// 该文件实现正则表达式匹配器，支持命名捕获组提取。
//
// 作者：xfy
package matcher

import (
	"regexp"

	"github.com/valyala/fasthttp"
)

// RegexMatcher 正则表达式匹配器。
//
// 使用 Go 标准库 regexp 编译正则模式，
// 支持命名捕获组提取，对应 nginx 的 ~ 和 ~* 修饰符。
type RegexMatcher struct {
	// pattern 编译后的正则表达式
	pattern *regexp.Regexp

	// handler 匹配成功后的请求处理器
	handler fasthttp.RequestHandler

	// captures 最后一次匹配提取的命名捕获组
	captures map[string]string

	// priority 匹配优先级，正则匹配为 3
	priority int

	// caseInsensitive 是否大小写不敏感（~* 模式）
	caseInsensitive bool
}

// NewRegexMatcher 创建正则匹配器。
//
// 参数：
//   - pattern: 正则表达式模式字符串
//   - handler: 匹配成功后的请求处理器
//   - priority: 优先级（通常设为 3）
//   - caseInsensitive: 是否大小写不敏感
//
// 返回值：
//   - *RegexMatcher: 正则匹配器实例
//   - error: 正则表达式编译失败时返回错误
func NewRegexMatcher(pattern string, handler fasthttp.RequestHandler, priority int, caseInsensitive bool) (*RegexMatcher, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}

	return &RegexMatcher{
		pattern:         re,
		handler:         handler,
		priority:        priority,
		caseInsensitive: caseInsensitive,
		captures:        make(map[string]string),
	}, nil
}

// MustRegexMatcher 创建正则匹配器，编译失败时 panic。
//
// 适用于启动时配置加载场景，配置错误直接终止程序。
//
// 参数：
//   - pattern: 正则表达式模式字符串
//   - handler: 匹配成功后的请求处理器
//   - priority: 优先级
//   - caseInsensitive: 是否大小写不敏感
//
// 返回值：
//   - *RegexMatcher: 正则匹配器实例
func MustRegexMatcher(pattern string, handler fasthttp.RequestHandler, priority int, caseInsensitive bool) *RegexMatcher {
	m, err := NewRegexMatcher(pattern, handler, priority, caseInsensitive)
	if err != nil {
		panic(err)
	}
	return m
}

// Match 检查路径是否匹配正则表达式。
//
// 参数：
//   - path: 待匹配的请求路径
//
// 返回值：
//   - bool: 正则匹配成功时返回 true
func (m *RegexMatcher) Match(path string) bool {
	return m.pattern.MatchString(path)
}

// Result 返回匹配结果。
//
// 返回值：
//   - *MatchResult: 包含处理器和匹配元数据的结果
func (m *RegexMatcher) Result() *MatchResult {
	locType := LocationTypeRegex
	if m.caseInsensitive {
		locType = LocationTypeRegexCaseless
	}
	return &MatchResult{
		Handler:      m.handler,
		Path:         m.pattern.String(),
		Priority:     m.priority,
		LocationType: locType,
		Captures:     m.captures,
	}
}

// GetCaptures 获取正则表达式在当前路径上的命名捕获组。
//
// 该方法在每次匹配后调用，提取捕获组数据供后续使用。
//
// 参数：
//   - path: 当前请求路径
//
// 返回值：
//   - map[string]string: 命名捕获组映射，无捕获时返回 nil
func (m *RegexMatcher) GetCaptures(path string) map[string]string {
	matches := m.pattern.FindStringSubmatch(path)
	if matches == nil {
		return nil
	}

	result := make(map[string]string)
	names := m.pattern.SubexpNames()
	for i, name := range names {
		if i == 0 {
			continue // 跳过全匹配（索引 0）
		}
		if name != "" && i < len(matches) {
			result[name] = matches[i]
		}
	}
	return result
}
