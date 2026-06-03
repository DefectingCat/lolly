// Package matcher 提供 nginx 风格的 location 匹配引擎实现。
//
// 该文件实现命名 location 匹配器，用于内部跳转。
//
// 作者：xfy
package matcher

import "github.com/valyala/fasthttp"

// NamedMatcher 命名 location 匹配器。
//
// 对应 nginx 的 @name 语法，用于内部跳转（如 error_page、try_files）。
// 命名 location 不直接匹配请求路径，而是通过名称引用。
type NamedMatcher struct {
	// handler 请求处理器
	handler fasthttp.RequestHandler

	// name location 名称（不含 @ 前缀）
	name string
}

// NewNamedMatcher 创建命名匹配器。
//
// 参数：
//   - name: location 名称
//   - handler: 关联的请求处理器
//
// 返回值：
//   - *NamedMatcher: 命名匹配器实例
func NewNamedMatcher(name string, handler fasthttp.RequestHandler) *NamedMatcher {
	return &NamedMatcher{
		name:    name,
		handler: handler,
	}
}


