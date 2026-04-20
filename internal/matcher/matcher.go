// Package matcher 提供 nginx 风格的 location 匹配引擎实现。
//
// 该包实现了多种 location 匹配策略，包括：
//   - 精确匹配（exact）：完全匹配请求路径
//   - 前缀匹配（prefix）：基于 Radix Tree 的最长前缀匹配
//   - 前缀优先匹配（prefix_priority）：^~ 类型，跳过正则匹配
//   - 正则匹配（regex/regex_caseless）：支持命名捕获组
//   - 命名匹配（named）：@name 形式的内部跳转目标
//
// 主要用途：
//
//	用于反向代理模块根据请求路径选择对应的后端处理器，
//	优先级顺序与 nginx 一致：精确 > 前缀优先(^~) > 正则(~,~*) > 普通前缀
//
// 注意事项：
//   - Radix Tree 在初始化完成后不可修改（MarkInitialized 后 Insert 返回错误）
//   - 正则匹配器按注册顺序执行，先匹配到的优先
//   - 所有匹配器均为非并发安全，应在启动阶段完成配置
//
// 作者：xfy
package matcher

import "github.com/valyala/fasthttp"

// LocationType 常量定义，表示不同 location 匹配类型。
const (
	LocationTypeExact          = "exact"           // 精确匹配 =
	LocationTypePrefix         = "prefix"          // 普通前缀匹配
	LocationTypePrefixPriority = "prefix_priority" // 前缀优先匹配 ^~
	LocationTypeRegex          = "regex"           // 正则匹配 ~
	LocationTypeRegexCaseless  = "regex_caseless"  // 大小写不敏感正则匹配 ~*
	LocationTypeNamed          = "named"           // 命名匹配 @name
)

// MatchResult 匹配结果。
//
// 包含匹配成功后的处理器、捕获组和位置类型信息。
type MatchResult struct {
	// Captures 正则表达式的命名捕获组
	Captures map[string]string

	// Handler 匹配到的请求处理器
	Handler fasthttp.RequestHandler

	// Internal 是否为 internal location
	Internal bool

	// LocationType 匹配类型（exact/prefix/regex 等）
	LocationType string

	// Path 匹配的路径模式
	Path string

	// Priority 匹配优先级（数值越小优先级越高）
	Priority int
}

// Matcher 匹配器接口。
//
// 所有具体匹配器（ExactMatcher、RegexMatcher 等）均需实现此接口。
type Matcher interface {
	// Match 检查给定路径是否匹配
	Match(path string) bool

	// Result 返回匹配结果（包含 Handler 和元数据）
	Result() *MatchResult
}
