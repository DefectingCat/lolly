package matcher

import "github.com/valyala/fasthttp"

// PrefixPriorityMatcher ^~ 类型前缀优先匹配器（封装 RadixTree）
type PrefixPriorityMatcher struct {
	tree     *RadixTree
	priority int
}

// NewPrefixPriorityMatcher 创建前缀优先匹配器
func NewPrefixPriorityMatcher() *PrefixPriorityMatcher {
	return &PrefixPriorityMatcher{
		tree:     NewRadixTree(),
		priority: 2, // ^~ 类型优先级更高
	}
}

// AddPath 添加路径
func (ppm *PrefixPriorityMatcher) AddPath(path string, handler fasthttp.RequestHandler) error {
	return ppm.tree.Insert(path, handler, ppm.priority)
}

// Match 前缀优先匹配，返回最长前缀匹配结果
func (ppm *PrefixPriorityMatcher) Match(path string) *MatchResult {
	return ppm.tree.FindLongestPrefix(path)
}

// MarkInitialized 标记初始化完成
func (ppm *PrefixPriorityMatcher) MarkInitialized() {
	ppm.tree.MarkInitialized()
}
