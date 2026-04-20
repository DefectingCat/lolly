// Package matcher 提供 nginx 风格的 location 匹配引擎实现。
//
// 该文件实现前缀优先匹配器（^~ 类型），比普通前缀匹配优先级更高。
//
// 作者：xfy
package matcher

import "github.com/valyala/fasthttp"

// PrefixPriorityMatcher 前缀优先匹配器（^~ 类型）。
//
// 基于 Radix Tree 实现，优先级为 2（仅次于精确匹配）。
// 对应 nginx 的 ^~ 修饰符：匹配成功后跳过正则匹配阶段。
type PrefixPriorityMatcher struct {
	// tree 基数树，存储前缀优先路径
	tree *RadixTree

	// priority 匹配优先级，^~ 类型为 2
	priority int
}

// NewPrefixPriorityMatcher 创建前缀优先匹配器。
//
// 返回值：
//   - *PrefixPriorityMatcher: 前缀优先匹配器实例
func NewPrefixPriorityMatcher() *PrefixPriorityMatcher {
	return &PrefixPriorityMatcher{
		tree:     NewRadixTree(),
		priority: 2, // ^~ 类型优先级更高
	}
}

// AddPath 添加路径到前缀优先匹配器。
//
// 参数：
//   - path: 前缀优先路径
//   - handler: 匹配成功后的请求处理器
//
// 返回值：
//   - error: 路径重复或树已初始化时返回错误
func (ppm *PrefixPriorityMatcher) AddPath(path string, handler fasthttp.RequestHandler) error {
	return ppm.tree.Insert(path, handler, ppm.priority, "prefix_priority")
}

// Match 前缀优先匹配，返回最长前缀匹配结果。
//
// 参数：
//   - path: 待匹配的请求路径
//
// 返回值：
//   - *MatchResult: 最长前缀匹配结果，无匹配时返回 nil
func (ppm *PrefixPriorityMatcher) Match(path string) *MatchResult {
	return ppm.tree.FindLongestPrefix(path)
}

// MarkInitialized 标记初始化完成。
//
// 调用后不能再添加新路径。
func (ppm *PrefixPriorityMatcher) MarkInitialized() {
	ppm.tree.MarkInitialized()
}
