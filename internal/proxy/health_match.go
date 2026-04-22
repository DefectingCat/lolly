// Package proxy 提供 HTTP 代理功能。
//
// 该文件实现 HealthMatch 健康检查匹配接口，支持：
//   - 默认 2xx 状态码判断
//   - 自定义状态码范围匹配
//   - 响应体正则匹配
//   - 响应头匹配
//
// 主要用途：
//
//	灵活定义后端服务器健康判断逻辑，替代硬编码的 2xx 判断。
//
// 作者：xfy
package proxy

import (
	"regexp"
	"strconv"
	"strings"
)

// HealthMatch 定义健康检查匹配接口。
//
// 用于判断健康检查响应是否表示目标健康。
type HealthMatch interface {
	// Match 判断健康检查响应是否表示目标健康。
	//
	// 参数：
	//   - status: HTTP 状态码
	//   - body: 响应体内容
	//   - headers: 响应头（key 为小写）
	//
	// 返回值：
	//   - true: 目标健康
	//   - false: 目标不健康
	Match(status int, body []byte, headers map[string]string) bool
}

// defaultHealthMatch 默认健康检查匹配器。
//
// 判断逻辑：状态码为 2xx 即健康。
type defaultHealthMatch struct{}

// Match 实现 HealthMatch 接口。
func (m *defaultHealthMatch) Match(status int, body []byte, headers map[string]string) bool {
	return status >= 200 && status < 300
}

// customHealthMatch 自定义健康检查匹配器。
//
// 支持状态码范围、响应体正则、响应头匹配。
type customHealthMatch struct {
	statusRanges  []statusRange  // 状态码范围列表
	bodyRegex     *regexp.Regexp // 响应体正则（可选）
	headerMatches []headerMatch  // 响应头匹配列表（可选）
}

// statusRange 表示状态码范围。
type statusRange struct {
	min int
	max int
}

// headerMatch 表示响应头匹配条件。
type headerMatch struct {
	key   string
	value string
}

// Match 实现 HealthMatch 接口。
func (m *customHealthMatch) Match(status int, body []byte, headers map[string]string) bool {
	// 1. 检查状态码
	if !m.matchStatus(status) {
		return false
	}

	// 2. 检查响应体正则（如果配置）
	if m.bodyRegex != nil && !m.bodyRegex.Match(body) {
		return false
	}

	// 3. 检查响应头（如果配置）
	for _, hm := range m.headerMatches {
		value, exists := headers[hm.key]
		if !exists || value != hm.value {
			return false
		}
	}

	return true
}

// matchStatus 检查状态码是否匹配任一范围。
func (m *customHealthMatch) matchStatus(status int) bool {
	for _, r := range m.statusRanges {
		if status >= r.min && status <= r.max {
			return true
		}
	}
	return false
}

// HealthMatchConfig 健康检查匹配配置。
type HealthMatchConfig struct {
	// Status 状态码范围列表，如 ["200-299", "301"]
	Status []string `yaml:"status"`

	// Body 响应体正则表达式
	Body string `yaml:"body"`

	// Headers 响应头匹配，如 {"Content-Type": "application/json"}
	Headers map[string]string `yaml:"headers"`
}

// NewHealthMatch 从配置创建健康检查匹配器。
//
// 如果配置为空或无效，返回默认匹配器（2xx 判断）。
func NewHealthMatch(cfg *HealthMatchConfig) HealthMatch {
	if cfg == nil {
		return &defaultHealthMatch{}
	}

	// 解析状态码范围
	var ranges []statusRange
	for _, s := range cfg.Status {
		r, err := parseStatusRange(s)
		if err != nil {
			continue // 忽略无效范围
		}
		ranges = append(ranges, r)
	}

	// 如果没有有效状态码范围，使用默认 2xx
	if len(ranges) == 0 {
		ranges = []statusRange{{min: 200, max: 299}}
	}

	// 解析响应体正则
	var bodyRegex *regexp.Regexp
	if cfg.Body != "" {
		bodyRegex = regexp.MustCompile(cfg.Body) // 配置加载时预编译
	}

	// 解析响应头匹配
	var headerMatches []headerMatch
	for k, v := range cfg.Headers {
		headerMatches = append(headerMatches, headerMatch{
			key:   strings.ToLower(k), // 统一小写
			value: v,
		})
	}

	return &customHealthMatch{
		statusRanges:  ranges,
		bodyRegex:     bodyRegex,
		headerMatches: headerMatches,
	}
}

// parseStatusRange 解析状态码范围字符串。
//
// 支持格式：
//   - "200" → 单个状态码
//   - "200-299" → 范围
func parseStatusRange(s string) (statusRange, error) {
	s = strings.TrimSpace(s)

	// 尝试解析范围
	if strings.Contains(s, "-") {
		parts := strings.Split(s, "-")
		if len(parts) != 2 {
			return statusRange{}, strconv.ErrSyntax
		}

		min, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
		max, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err1 != nil || err2 != nil {
			return statusRange{}, strconv.ErrSyntax
		}

		return statusRange{min: min, max: max}, nil
	}

	// 单个状态码
	code, err := strconv.Atoi(s)
	if err != nil {
		return statusRange{}, err
	}

	return statusRange{min: code, max: code}, nil
}

// DefaultHealthMatch 返回默认健康检查匹配器。
func DefaultHealthMatch() HealthMatch {
	return &defaultHealthMatch{}
}
