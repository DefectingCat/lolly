package matcher

import "github.com/valyala/fasthttp"

// MatchResult 匹配结果
type MatchResult struct {
	Handler      fasthttp.RequestHandler
	Path         string
	Priority     int
	LocationType string

	// 正则捕获组
	Captures map[string]string
}

// Matcher 接口
type Matcher interface {
	Match(path string) bool
	Result() *MatchResult
}
