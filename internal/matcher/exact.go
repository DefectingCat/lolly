// Package matcher 提供 nginx 风格的 location 匹配引擎实现。
//
// 该文件实现精确路径匹配器，使用 hash map 实现 O(1) 查找。
//
// 作者：xfy
package matcher

import "github.com/valyala/fasthttp"

// ExactMatcher Hash Map 精确匹配器。
//
// 通过字符串等值比较实现 O(1) 时间复杂度的路径匹配，
// 对应 nginx 的 = 修饰符。
type ExactMatcher struct {
	// handler 请求处理器
	handler fasthttp.RequestHandler

	// path 精确匹配路径
	path string

	// priority 匹配优先级，精确匹配为 1（最高）
	priority int
}

// NewExactMatcher 创建精确匹配器。
//
// 参数：
//   - path: 精确匹配的路径
//   - handler: 匹配成功后的请求处理器
//   - priority: 优先级（通常设为 1）
//
// 返回值：
//   - *ExactMatcher: 精确匹配器实例
func NewExactMatcher(path string, handler fasthttp.RequestHandler, priority int) *ExactMatcher {
	return &ExactMatcher{
		path:     path,
		handler:  handler,
		priority: priority,
	}
}

// Match 检查路径是否精确匹配。
//
// 参数：
//   - path: 待检查的请求路径
//
// 返回值：
//   - bool: 路径完全相等时返回 true
func (m *ExactMatcher) Match(path string) bool {
	return m.path == path
}

// Result 返回匹配结果。
//
// 返回值：
//   - *MatchResult: 包含处理器和元数据的匹配结果
func (m *ExactMatcher) Result() *MatchResult {
	return &MatchResult{
		Handler:      m.handler,
		Path:         m.path,
		Priority:     m.priority,
		LocationType: LocationTypeExact,
	}
}
