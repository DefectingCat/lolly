package matcher

import (
	"errors"
	"strings"

	"github.com/valyala/fasthttp"
)

// RadixNode Radix Tree 节点
type RadixNode struct {
	children     []*RadixNode
	handler      fasthttp.RequestHandler
	priority     int
	isLeaf       bool
	prefix       string
	locationType string // exact/prefix/prefix_priority
}

// RadixTree 前缀匹配 Radix Tree
type RadixTree struct {
	root        *RadixNode
	initialized bool
}

// NewRadixTree 创建新 Radix Tree
func NewRadixTree() *RadixTree {
	return &RadixTree{
		root: &RadixNode{prefix: ""},
	}
}

// Insert 插入路径到 Radix Tree（startup-only）
func (t *RadixTree) Insert(path string, handler fasthttp.RequestHandler, priority int, locationType string) error {
	if t.initialized {
		return errors.New("RadixTree already initialized")
	}
	return t.insertNode(nil, t.root, path, handler, priority, locationType)
}

// insertNode 完整路径分割插入算法
func (t *RadixTree) insertNode(parent *RadixNode, node *RadixNode, path string, handler fasthttp.RequestHandler, priority int, locationType string) error {
	// Case 1: 空节点（根节点），直接设置
	if node.prefix == "" && len(node.children) == 0 && node.handler == nil {
		if path == "" {
			node.handler = handler
			node.priority = priority
			node.isLeaf = true
			node.locationType = locationType
			return nil
		}
		// 创建新子节点
		newNode := &RadixNode{
			prefix:       path,
			handler:      handler,
			isLeaf:       true,
			priority:     priority,
			locationType: locationType,
		}
		node.children = append(node.children, newNode)
		return nil
	}

	// Case 2: 计算公共前缀长度
	commonLen := 0
	maxLen := minInt(len(node.prefix), len(path))
	for commonLen < maxLen && node.prefix[commonLen] == path[commonLen] {
		commonLen++
	}

	// Case 3: path 完全匹配节点前缀
	if commonLen == len(node.prefix) {
		remaining := path[commonLen:]

		if remaining == "" {
			// 路径完全匹配，设置 handler
			if node.handler != nil {
				return errors.New("path already exists")
			}
			node.handler = handler
			node.priority = priority
			node.isLeaf = true
			node.locationType = locationType
			return nil
		}

		// 搜索匹配剩余路径的子节点
		for _, child := range node.children {
			if strings.HasPrefix(remaining, child.prefix) {
				return t.insertNode(node, child, remaining, handler, priority, locationType)
			}
		}

		// 无匹配子节点，创建新子节点
		newNode := &RadixNode{
			prefix:       remaining,
			handler:      handler,
			isLeaf:       true,
			priority:     priority,
			locationType: locationType,
		}
		node.children = append(node.children, newNode)
		return nil
	}

	// Case 4: 需要分割节点（公共前缀 < 节点前缀）
	// 创建中间节点保存公共前缀
	splitNode := &RadixNode{
		prefix:   node.prefix[:commonLen],
		children: []*RadixNode{},
	}

	// 修改原节点为公共前缀之后的部分
	node.prefix = node.prefix[commonLen:]

	// 创建新节点保存剩余路径
	newNode := &RadixNode{
		prefix:       path[commonLen:],
		handler:      handler,
		isLeaf:       true,
		priority:     priority,
		locationType: locationType,
	}

	// 将原节点和新节点作为 splitNode 的子节点
	splitNode.children = append(splitNode.children, node)
	splitNode.children = append(splitNode.children, newNode)

	// 替换父节点的子节点引用
	if parent == nil {
		t.root = splitNode
	} else {
		for i, child := range parent.children {
			if child == node {
				parent.children[i] = splitNode
				break
			}
		}
	}

	return nil
}

// FindLongestPrefix 查找最长前缀匹配
func (t *RadixTree) FindLongestPrefix(path string) *MatchResult {
	return t.searchLongest(t.root, path, nil)
}

// searchLongest 递归搜索最长前缀匹配
func (t *RadixTree) searchLongest(node *RadixNode, path string, bestMatch *MatchResult) *MatchResult {
	if node == nil || path == "" {
		return bestMatch
	}

	// 检查是否匹配节点前缀
	if !strings.HasPrefix(path, node.prefix) {
		return bestMatch
	}

	remaining := path[len(node.prefix):]

	// 如果节点有 handler，更新最佳匹配
	if node.handler != nil {
		newMatch := &MatchResult{
			Handler:      node.handler,
			Path:         node.prefix,
			Priority:     node.priority,
			LocationType: node.locationType,
		}

		// nil-safe 优先级比较 + 长度比较
		if bestMatch == nil {
			bestMatch = newMatch
		} else if node.priority < bestMatch.Priority {
			bestMatch = newMatch
		} else if node.priority == bestMatch.Priority && len(node.prefix) > len(bestMatch.Path) {
			bestMatch = newMatch
		}
	}

	// 继续搜索子节点
	for _, child := range node.children {
		childMatch := t.searchLongest(child, remaining, bestMatch)
		if childMatch != nil {
			// nil-safe 比较
			if bestMatch == nil {
				bestMatch = childMatch
			} else if childMatch.Priority < bestMatch.Priority {
				bestMatch = childMatch
			} else if childMatch.Priority == bestMatch.Priority && len(childMatch.Path) > len(bestMatch.Path) {
				bestMatch = childMatch
			}
		}
	}

	return bestMatch
}

// MarkInitialized 标记初始化完成
func (t *RadixTree) MarkInitialized() {
	t.initialized = true
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
