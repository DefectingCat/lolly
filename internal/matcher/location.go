// Package matcher 提供 nginx 风格的 location 匹配引擎实现。
//
// 该文件实现统一的 LocationEngine，整合所有匹配策略，
// 按照 nginx 优先级顺序执行匹配。
//
// 匹配优先级从高到低：
//  1. 精确匹配（=）
//  2. 前缀优先匹配（^~）
//  3. 正则匹配（~, ~*）
//  4. 普通前缀匹配
//
// 作者：xfy
package matcher

import (
	"errors"
	"fmt"
	"regexp"

	"github.com/valyala/fasthttp"
)

// LocationEngine 统一匹配引擎。
//
// 整合所有 location 匹配策略，按照 nginx 优先级顺序执行：
//   - 精确匹配：O(1) hash map 查找
//   - 前缀优先：Radix Tree 最长前缀匹配
//   - 正则匹配：按注册顺序逐个尝试
//   - 普通前缀：Radix Tree 最长前缀匹配
//
// 注意事项：
//   - 调用 MarkInitialized 后不可再添加 location
//   - 正则匹配器按注册顺序执行，配置时应将高频模式放在前面
type LocationEngine struct {
	// prefixPriorityTree ^~ 类型前缀优先匹配树（优先级 2）
	prefixPriorityTree *RadixTree

	// prefixTree 普通前缀匹配树（优先级 4）
	prefixTree *RadixTree

	// exactMatchers 精确匹配映射
	exactMatchers map[string]*ExactMatcher

	// namedMatchers 命名 location 映射
	namedMatchers map[string]*NamedMatcher

	// registeredPaths 已注册路径（用于冲突检测）
	registeredPaths map[string]string

	// regexMatchers 正则匹配器列表（按注册顺序）
	regexMatchers []*RegexMatcher

	// initialized 是否已完成初始化
	initialized bool
}

// NewLocationEngine 创建新的匹配引擎实例。
//
// 返回值：
//   - *LocationEngine: 初始化的引擎实例
func NewLocationEngine() *LocationEngine {
	return &LocationEngine{
		exactMatchers:      make(map[string]*ExactMatcher),
		prefixPriorityTree: NewRadixTree(),
		prefixTree:         NewRadixTree(),
		regexMatchers:      []*RegexMatcher{},
		namedMatchers:      make(map[string]*NamedMatcher),
		registeredPaths:    make(map[string]string),
	}
}

// AddExact 添加精确匹配 location。
//
// 参数：
//   - path: 精确匹配路径
//   - handler: 请求处理器
//   - internal: 是否为 internal location
//
// 返回值：
//   - error: 引擎已初始化或路径冲突时返回错误
func (e *LocationEngine) AddExact(path string, handler fasthttp.RequestHandler, internal bool) error {
	if e.initialized {
		return errors.New("LocationEngine already initialized")
	}

	if err := e.checkConflict(path, "exact"); err != nil {
		return err
	}

	matcher := NewExactMatcher(path, handler, 1, internal)
	e.exactMatchers[path] = matcher
	return nil
}

// AddPrefixPriority 添加 ^~ 前缀优先匹配 location。
//
// 参数：
//   - path: 前缀优先路径
//   - handler: 请求处理器
//   - internal: 是否为 internal location
//
// 返回值：
//   - error: 引擎已初始化或路径冲突时返回错误
func (e *LocationEngine) AddPrefixPriority(path string, handler fasthttp.RequestHandler, internal bool) error {
	if e.initialized {
		return errors.New("LocationEngine already initialized")
	}

	if err := e.checkConflict(path, "prefix_priority"); err != nil {
		return err
	}

	return e.prefixPriorityTree.Insert(path, handler, 2, "prefix_priority", internal)
}

// AddRegex 添加正则匹配 location。
//
// 参数：
//   - pattern: 正则表达式模式
//   - handler: 请求处理器
//   - caseInsensitive: 是否大小写不敏感（~* 模式）
//   - internal: 是否为 internal location
//
// 返回值：
//   - error: 引擎已初始化或正则表达式无效时返回错误
func (e *LocationEngine) AddRegex(pattern string, handler fasthttp.RequestHandler, caseInsensitive bool, internal bool) error {
	if e.initialized {
		return errors.New("LocationEngine already initialized")
	}

	matcher, err := NewRegexMatcher(pattern, handler, 3, caseInsensitive, internal)
	if err != nil {
		return fmt.Errorf("invalid regex pattern: %w", err)
	}

	e.regexMatchers = append(e.regexMatchers, matcher)
	return nil
}

// AddPrefix 添加普通前缀匹配 location。
//
// 参数：
//   - path: 前缀路径
//   - handler: 请求处理器
//   - internal: 是否为 internal location
//
// 返回值：
//   - error: 引擎已初始化或路径冲突时返回错误
func (e *LocationEngine) AddPrefix(path string, handler fasthttp.RequestHandler, internal bool) error {
	if e.initialized {
		return errors.New("LocationEngine already initialized")
	}

	if err := e.checkConflict(path, "prefix"); err != nil {
		return err
	}

	return e.prefixTree.Insert(path, handler, 4, "prefix", internal)
}

// AddNamed 添加命名 location。
//
// 命名 location 用于内部跳转（如 error_page），不直接匹配请求路径。
//
// 参数：
//   - name: location 名称（不含 @ 前缀）
//   - handler: 请求处理器
//
// 返回值：
//   - error: 引擎已初始化或名称重复时返回错误
func (e *LocationEngine) AddNamed(name string, handler fasthttp.RequestHandler) error {
	if e.initialized {
		return errors.New("LocationEngine already initialized")
	}

	if existing, ok := e.namedMatchers[name]; ok {
		return fmt.Errorf("named location '@%s' already registered", existing.name)
	}

	matcher := NewNamedMatcher(name, handler)
	e.namedMatchers[name] = matcher
	return nil
}

// Match 统一匹配入口。
//
// 按照 nginx 优先级顺序执行匹配：
//  1. 精确匹配（=）- O(1)
//  2. 前缀优先匹配（^~）- O(log n)
//  3. 正则匹配（~, ~*）- 按顺序
//  4. 普通前缀匹配 - O(log n)
//
// 参数：
//   - path: 请求路径
//
// 返回值：
//   - *MatchResult: 匹配结果，无匹配时返回 nil
func (e *LocationEngine) Match(path string) *MatchResult {
	// 1. 精确匹配 (=) - O(1)
	if m, ok := e.exactMatchers[path]; ok {
		return m.Result()
	}

	// 2. 前缀优先匹配 (^~) - O(log n)
	prefixPriorityResult := e.prefixPriorityTree.FindLongestPrefix(path)
	if prefixPriorityResult != nil && prefixPriorityResult.Handler != nil {
		return prefixPriorityResult
	}

	// 3. 正则匹配 (~, ~*) - 按顺序
	for _, m := range e.regexMatchers {
		if m.Match(path) {
			result := m.Result()
			result.Captures = m.GetCaptures(path)
			return result
		}
	}

	// 4. 前缀匹配（无修饰符）- O(log n)
	return e.prefixTree.FindLongestPrefix(path)
}

// GetNamed 获取命名 location。
//
// 参数：
//   - name: location 名称
//
// 返回值：
//   - *NamedMatcher: 命名匹配器，不存在时返回 nil
func (e *LocationEngine) GetNamed(name string) *NamedMatcher {
	return e.namedMatchers[name]
}

// MarkInitialized 标记初始化完成。
//
// 调用后所有 Add 方法均返回错误，确保运行时安全。
func (e *LocationEngine) MarkInitialized() {
	e.initialized = true
	e.prefixPriorityTree.MarkInitialized()
	e.prefixTree.MarkInitialized()
}

// checkConflict 检查路径冲突。
//
// 参数：
//   - path: 待注册的路径
//   - locationType: location 类型
//
// 返回值：
//   - error: 路径已存在时返回冲突错误
func (e *LocationEngine) checkConflict(path, locationType string) error {
	if existing, ok := e.registeredPaths[path]; ok {
		return fmt.Errorf("path conflict: '%s' already registered as '%s', trying to register as '%s'",
			path, existing, locationType)
	}
	e.registeredPaths[path] = locationType
	return nil
}

// ParseRegexPattern 解析 nginx 风格的正则模式。
//
// 支持以下前缀：
//   - ~:  大小写敏感正则（case-sensitive regex）
//   - ~*: 大小写不敏感正则（case-insensitive regex）
//   - ^~: 前缀优先匹配（非正则）
//
// 该函数用于配置验证层，检测用户配置的模式格式是否正确。
// 运行时匹配器直接使用 LocationType 枚举进行匹配。
func ParseRegexPattern(pattern string) (cleanPattern string, caseInsensitive bool, isRegex bool) {
	if len(pattern) == 0 {
		return pattern, false, false
	}

	// Handle ~* (case-insensitive regex) - must check first (2-char prefix)
	if len(pattern) >= 2 && pattern[0] == '~' && pattern[1] == '*' {
		cleanPattern = pattern[2:]
		return cleanPattern, true, true // case-insensitive, is regex
	}

	// Handle ~ (case-sensitive regex)
	if pattern[0] == '~' {
		cleanPattern = pattern[1:]
		return cleanPattern, false, true // case-sensitive, is regex
	}

	// Handle ^~ (prefix priority, NOT regex)
	if len(pattern) >= 2 && pattern[0] == '^' && pattern[1] == '~' {
		cleanPattern = pattern[2:]
		return cleanPattern, false, false // NOT a regex
	}

	// Default: exact/prefix match
	return pattern, false, false
}

// MustCompileRegex 编译正则表达式，失败返回 nil。
//
// 参数：
//   - pattern: 正则表达式模式
//
// 返回值：
//   - *regexp.Regexp: 编译后的正则表达式，失败时返回 nil
func MustCompileRegex(pattern string) *regexp.Regexp {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil
	}
	return re
}
