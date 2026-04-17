package matcher

import (
	"regexp"

	"github.com/valyala/fasthttp"
)

// RegexMatcher 正则匹配 + 捕获组
type RegexMatcher struct {
	pattern         *regexp.Regexp
	handler         fasthttp.RequestHandler
	priority        int
	caseInsensitive bool
	captures        map[string]string
}

// NewRegexMatcher 创建正则匹配器
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

// MustRegexMatcher 创建正则匹配器，失败时 panic
func MustRegexMatcher(pattern string, handler fasthttp.RequestHandler, priority int, caseInsensitive bool) *RegexMatcher {
	m, err := NewRegexMatcher(pattern, handler, priority, caseInsensitive)
	if err != nil {
		panic(err)
	}
	return m
}

// Match 检查路径是否正则匹配
func (m *RegexMatcher) Match(path string) bool {
	return m.pattern.MatchString(path)
}

// Result 返回匹配结果
func (m *RegexMatcher) Result() *MatchResult {
	locType := "regex"
	if m.caseInsensitive {
		locType = "regex_caseless"
	}
	return &MatchResult{
		Handler:      m.handler,
		Path:         m.pattern.String(),
		Priority:     m.priority,
		LocationType: locType,
		Captures:     m.captures,
	}
}

// GetCaptures 获取正则捕获组
func (m *RegexMatcher) GetCaptures(path string) map[string]string {
	matches := m.pattern.FindStringSubmatch(path)
	if matches == nil {
		return nil
	}

	result := make(map[string]string)
	names := m.pattern.SubexpNames()
	for i, name := range names {
		if i == 0 {
			continue
		}
		if name != "" && i < len(matches) {
			result[name] = matches[i]
		}
	}
	return result
}
