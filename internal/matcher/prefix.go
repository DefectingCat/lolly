package matcher

import "github.com/valyala/fasthttp"

// PrefixMatcher 普通前缀匹配器（封装 RadixTree）
type PrefixMatcher struct {
	tree     *RadixTree
	priority int
}

// NewPrefixMatcher 创建前缀匹配器
func NewPrefixMatcher() *PrefixMatcher {
	return &PrefixMatcher{
		tree:     NewRadixTree(),
		priority: 4, // 普通前缀优先级
	}
}

// AddPath 添加路径
func (pm *PrefixMatcher) AddPath(path string, handler fasthttp.RequestHandler) error {
	return pm.tree.Insert(path, handler, pm.priority, "prefix")
}

// Match 前缀匹配，返回最长前缀匹配结果
func (pm *PrefixMatcher) Match(path string) *MatchResult {
	return pm.tree.FindLongestPrefix(path)
}

// MarkInitialized 标记初始化完成
func (pm *PrefixMatcher) MarkInitialized() {
	pm.tree.MarkInitialized()
}
