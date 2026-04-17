package matcher

import (
	"errors"
	"fmt"
	"regexp"

	"github.com/valyala/fasthttp"
)

// LocationEngine 统一匹配引擎
type LocationEngine struct {
	prefixPriorityTree *RadixTree // ^~ 类型（优先级 2）
	prefixTree         *RadixTree // 普通前缀（优先级 4）
	exactMatchers      map[string]*ExactMatcher
	namedMatchers      map[string]*NamedMatcher
	registeredPaths    map[string]string
	regexMatchers      []*RegexMatcher
	initialized        bool
}

// NewLocationEngine 创建新引擎
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

// AddExact 添加精确匹配 location
func (e *LocationEngine) AddExact(path string, handler fasthttp.RequestHandler) error {
	if e.initialized {
		return errors.New("LocationEngine already initialized")
	}

	if err := e.checkConflict(path, "exact"); err != nil {
		return err
	}

	matcher := NewExactMatcher(path, handler, 1)
	e.exactMatchers[path] = matcher
	return nil
}

// AddPrefixPriority 添加 ^~ 前缀优先匹配 location
func (e *LocationEngine) AddPrefixPriority(path string, handler fasthttp.RequestHandler) error {
	if e.initialized {
		return errors.New("LocationEngine already initialized")
	}

	if err := e.checkConflict(path, "prefix_priority"); err != nil {
		return err
	}

	return e.prefixPriorityTree.Insert(path, handler, 2, "prefix_priority")
}

// AddRegex 添加正则匹配 location
func (e *LocationEngine) AddRegex(pattern string, handler fasthttp.RequestHandler, caseInsensitive bool) error {
	if e.initialized {
		return errors.New("LocationEngine already initialized")
	}

	matcher, err := NewRegexMatcher(pattern, handler, 3, caseInsensitive)
	if err != nil {
		return fmt.Errorf("invalid regex pattern: %w", err)
	}

	e.regexMatchers = append(e.regexMatchers, matcher)
	return nil
}

// AddPrefix 添加普通前缀匹配 location
func (e *LocationEngine) AddPrefix(path string, handler fasthttp.RequestHandler) error {
	if e.initialized {
		return errors.New("LocationEngine already initialized")
	}

	if err := e.checkConflict(path, "prefix"); err != nil {
		return err
	}

	return e.prefixTree.Insert(path, handler, 4, "prefix")
}

// AddNamed 添加命名 location
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

// Match 统一匹配入口
// nginx 优先级：精确匹配 → 前缀优先(^~) → 正则 → 普通前缀
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

// GetNamed 获取命名 location
func (e *LocationEngine) GetNamed(name string) *NamedMatcher {
	return e.namedMatchers[name]
}

// MarkInitialized 标记初始化完成
func (e *LocationEngine) MarkInitialized() {
	e.initialized = true
	e.prefixPriorityTree.MarkInitialized()
	e.prefixTree.MarkInitialized()
}

// checkConflict 检查路径冲突
func (e *LocationEngine) checkConflict(path, locationType string) error {
	if existing, ok := e.registeredPaths[path]; ok {
		return fmt.Errorf("path conflict: '%s' already registered as '%s', trying to register as '%s'",
			path, existing, locationType)
	}
	e.registeredPaths[path] = locationType
	return nil
}

// ParseRegexPattern 解析 nginx 风格的正则模式（支持 ^~ ~ ~* 前缀）
func ParseRegexPattern(pattern string) (cleanPattern string, caseInsensitive bool, isRegex bool) {
	if len(pattern) == 0 {
		return pattern, false, false
	}

	switch pattern[0] {
	case '~':
		cleanPattern = pattern[1:]
		caseInsensitive = true
		return cleanPattern, caseInsensitive, true
	case '^':
		if len(pattern) > 1 && pattern[1] == '~' {
			cleanPattern = pattern[2:]
			caseInsensitive = false
			return cleanPattern, caseInsensitive, true
		}
	}

	return pattern, false, false
}

// MustCompileRegex 编译正则表达式，失败返回原始字符串
func MustCompileRegex(pattern string) *regexp.Regexp {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil
	}
	return re
}
