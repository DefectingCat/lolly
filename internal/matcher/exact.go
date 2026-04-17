package matcher

import (
	"github.com/valyala/fasthttp"
)

// ExactMatcher Hash Map 精确匹配
type ExactMatcher struct {
	path     string
	handler  fasthttp.RequestHandler
	priority int
}

// NewExactMatcher 创建精确匹配器
func NewExactMatcher(path string, handler fasthttp.RequestHandler, priority int) *ExactMatcher {
	return &ExactMatcher{
		path:     path,
		handler:  handler,
		priority: priority,
	}
}

// Match 检查路径是否精确匹配
func (m *ExactMatcher) Match(path string) bool {
	return m.path == path
}

// Result 返回匹配结果
func (m *ExactMatcher) Result() *MatchResult {
	return &MatchResult{
		Handler:      m.handler,
		Path:         m.path,
		Priority:     m.priority,
		LocationType: "exact",
	}
}
