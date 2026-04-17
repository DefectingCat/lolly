package matcher

import "github.com/valyala/fasthttp"

// LocationType 常量定义
const (
	LocationTypeExact          = "exact"
	LocationTypePrefix         = "prefix"
	LocationTypePrefixPriority = "prefix_priority"
	LocationTypeRegex          = "regex"
	LocationTypeRegexCaseless  = "regex_caseless"
	LocationTypeNamed          = "named"
)

// MatchResult 匹配结果
type MatchResult struct {
	Captures     map[string]string // 正则捕获组
	Handler      fasthttp.RequestHandler
	LocationType string
	Path         string
	Priority     int
}

// Matcher 接口
type Matcher interface {
	Match(path string) bool
	Result() *MatchResult
}
