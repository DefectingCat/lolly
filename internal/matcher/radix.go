// Package matcher 提供 nginx 风格的 location 匹配引擎实现。
//
// 该文件实现 Radix Tree（基数树）数据结构，用于高效的前缀路径匹配。
//
// Radix Tree 是一种压缩前缀树，将共享同一前缀的路径合并到同一节点，
// 相比普通 Trie 树大幅减少内存占用。查找时使用最长前缀匹配策略。
//
// 作者：xfy
package matcher

import (
	"errors"
	"strings"

	"github.com/valyala/fasthttp"
)

// RadixNode Radix Tree 节点。
//
// 每个节点存储一个路径前缀，子节点存储剩余前缀。
// 叶子节点（isLeaf=true）包含具体的请求处理器。
type RadixNode struct {
	// children 子节点列表
	children []*RadixNode

	// handler 请求处理器（仅叶子节点有效）
	handler fasthttp.RequestHandler

	// priority 匹配优先级
	priority int

	// internal 是否为 internal location
	internal bool

	// isLeaf 是否为叶子节点（有 handler）
	isLeaf bool

	// prefix 当前节点的路径前缀
	prefix string

	// locationType 位置类型（exact/prefix/prefix_priority）
	locationType string
}

// RadixTree 前缀匹配 Radix Tree。
//
// 使用路径分割插入算法，支持最长前缀匹配查找。
// 初始化完成后可标记为只读状态，防止运行时修改。
type RadixTree struct {
	// root 根节点
	root *RadixNode

	// initialized 是否已完成初始化（标记后不可插入）
	initialized bool
}

// NewRadixTree 创建新的 Radix Tree。
//
// 返回值：
//   - *RadixTree: 空树实例，根节点已初始化
func NewRadixTree() *RadixTree {
	return &RadixTree{
		root: &RadixNode{prefix: ""},
	}
}

// Insert 插入路径到 Radix Tree。
//
// 该函数仅在启动阶段使用，初始化完成后禁止插入。
//
// 参数：
//   - path: 要插入的路径
//   - handler: 匹配成功后的请求处理器
//   - priority: 匹配优先级
//   - locationType: 位置类型标识
//   - internal: 是否为 internal location
//
// 返回值：
//   - error: 树已初始化或路径已存在时返回错误
func (t *RadixTree) Insert(path string, handler fasthttp.RequestHandler, priority int, locationType string, internal bool) error {
	if t.initialized {
		return errors.New("RadixTree already initialized")
	}
	return t.insertNode(nil, t.root, path, handler, priority, locationType, internal)
}

// insertNode 完整路径分割插入算法。
//
// 算法分为四种情况：
//   - Case 1: 空节点直接设置
//   - Case 2: 计算公共前缀长度
//   - Case 3: 路径完全匹配节点前缀，递归处理剩余部分
//   - Case 4: 需要分割节点，创建中间节点
//
// 参数：
//   - parent: 父节点（根节点时为 nil）
//   - node: 当前节点
//   - path: 待插入路径
//   - handler: 请求处理器
//   - priority: 优先级
//   - locationType: 位置类型
//   - internal: 是否为 internal location
//
// 返回值：
//   - error: 路径已存在时返回错误
func (t *RadixTree) insertNode(parent *RadixNode, node *RadixNode, path string, handler fasthttp.RequestHandler, priority int, locationType string, internal bool) error {
	// Case 1: 空节点（根节点），直接设置
	if node.prefix == "" && len(node.children) == 0 && node.handler == nil {
		if path == "" {
			node.handler = handler
			node.priority = priority
			node.isLeaf = true
			node.locationType = locationType
			node.internal = internal
			return nil
		}
		// 创建新子节点
		newNode := &RadixNode{
			prefix:       path,
			handler:      handler,
			isLeaf:       true,
			priority:     priority,
			locationType: locationType,
			internal:     internal,
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
			node.internal = internal
			return nil
		}

		// 搜索匹配剩余路径的子节点
		for _, child := range node.children {
			if strings.HasPrefix(remaining, child.prefix) {
				return t.insertNode(node, child, remaining, handler, priority, locationType, internal)
			}
		}

		// 无匹配子节点，创建新子节点
		newNode := &RadixNode{
			prefix:       remaining,
			handler:      handler,
			isLeaf:       true,
			priority:     priority,
			locationType: locationType,
			internal:     internal,
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
		internal:     internal,
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

// FindLongestPrefix 查找最长前缀匹配。
//
// 参数：
//   - path: 待匹配的请求路径
//
// 返回值：
//   - *MatchResult: 最长前缀匹配结果，无匹配时返回 nil
func (t *RadixTree) FindLongestPrefix(path string) *MatchResult {
	return t.searchLongest(t.root, path, nil)
}

// searchLongest 递归搜索最长前缀匹配。
//
// 匹配规则：
//  1. 优先级数值越小越优先
//  2. 相同优先级时，前缀越长越优先
//
// 参数：
//   - node: 当前搜索节点
//   - path: 剩余待匹配路径
//   - bestMatch: 当前最佳匹配
//
// 返回值：
//   - *MatchResult: 最佳匹配结果
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
			Internal:     node.internal,
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

// MarkInitialized 标记初始化完成。
//
// 标记后 Insert 调用将返回错误，确保运行时不可变性。
func (t *RadixTree) MarkInitialized() {
	t.initialized = true
}

// minInt 返回两个整数中的较小值。
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
