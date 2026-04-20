// Package matcher 提供 nginx 风格的 location 匹配引擎实现。
//
// 该文件实现前缀匹配器，基于 Radix Tree 实现最长前缀匹配。
//
// 作者：xfy
package matcher

import "github.com/valyala/fasthttp"

// PrefixMatcher 普通前缀匹配器（封装 RadixTree）。
//
// 使用 Radix Tree 数据结构存储前缀路径，
// 查找时返回最长匹配前缀，对应 nginx 无修饰符前缀匹配。
type PrefixMatcher struct {
	// tree 基数树，存储前缀路径
	tree *RadixTree

	// priority 匹配优先级，普通前缀为 4（最低）
	priority int
}

// NewPrefixMatcher 创建前缀匹配器。
//
// 返回值：
//   - *PrefixMatcher: 前缀匹配器实例，内部已初始化 Radix Tree
func NewPrefixMatcher() *PrefixMatcher {
	return &PrefixMatcher{
		tree:     NewRadixTree(),
		priority: 4, // 普通前缀优先级
	}
}

// AddPath 添加路径到前缀匹配器。
//
// 参数：
//   - path: 前缀路径
//   - handler: 匹配成功后的请求处理器
//   - internal: 是否为 internal location
//
// 返回值：
//   - error: 路径重复或树已初始化时返回错误
func (pm *PrefixMatcher) AddPath(path string, handler fasthttp.RequestHandler, internal bool) error {
	return pm.tree.Insert(path, handler, pm.priority, "prefix", internal)
}

// Match 前缀匹配，返回最长前缀匹配结果。
//
// 参数：
//   - path: 待匹配的请求路径
//
// 返回值：
//   - *MatchResult: 最长前缀匹配结果，无匹配时返回 nil
func (pm *PrefixMatcher) Match(path string) *MatchResult {
	return pm.tree.FindLongestPrefix(path)
}

// MarkInitialized 标记初始化完成。
//
// 调用后不能再添加新路径，确保运行时线程安全。
func (pm *PrefixMatcher) MarkInitialized() {
	pm.tree.MarkInitialized()
}
