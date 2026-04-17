package matcher

import "github.com/valyala/fasthttp"

// NamedMatcher @命名 location
type NamedMatcher struct {
	name    string
	handler fasthttp.RequestHandler
}

// NewNamedMatcher 创建命名匹配器
func NewNamedMatcher(name string, handler fasthttp.RequestHandler) *NamedMatcher {
	return &NamedMatcher{
		name:    name,
		handler: handler,
	}
}

// Match 检查命名是否匹配（命名 location 不使用 path 匹配）
func (m *NamedMatcher) Match(path string) bool {
	return false
}

// Result 返回匹配结果
func (m *NamedMatcher) Result() *MatchResult {
	return &MatchResult{
		Handler:      m.handler,
		Path:         "@" + m.name,
		Priority:     0,
		LocationType: "named",
	}
}

// Name 返回命名 location 的名称
func (m *NamedMatcher) Name() string {
	return m.name
}
