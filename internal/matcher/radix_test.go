package matcher

import (
	"testing"

	"github.com/valyala/fasthttp"
)

func TestRadixTree_Insert_EmptyNode(t *testing.T) {
	// Case 1: 空节点
	tree := NewRadixTree()
	handler := func(ctx *fasthttp.RequestCtx) {}

	err := tree.Insert("/api", handler, 1, "prefix")
	if err != nil {
		t.Fatalf("insert failed: %v", err)
	}

	result := tree.FindLongestPrefix("/api")
	if result == nil {
		t.Error("should find inserted path")
	}
	if result.Path != "/api" {
		t.Errorf("expected path /api, got %s", result.Path)
	}
}

func TestRadixTree_Insert_CommonPrefix(t *testing.T) {
	// Case 2: 公共前缀计算
	tree := NewRadixTree()
	handler1 := func(ctx *fasthttp.RequestCtx) { ctx.SetBodyString("1") }
	handler2 := func(ctx *fasthttp.RequestCtx) { ctx.SetBodyString("2") }

	tree.Insert("/api", handler1, 1, "prefix")
	tree.Insert("/api/users", handler2, 2, "prefix")

	result := tree.FindLongestPrefix("/api/users")
	if result == nil {
		t.Fatal("expected match")
	}
	// Lower priority number wins, so /api (priority 1) beats /api/users (priority 2)
	if result.Path != "/api" {
		t.Errorf("expected path /api (priority 1), got %s", result.Path)
	}
	if result.Priority != 1 {
		t.Errorf("expected priority 1, got %d", result.Priority)
	}
}

func TestRadixTree_Insert_NodeSplit(t *testing.T) {
	// Case 4: 节点分割
	tree := NewRadixTree()
	handler1 := func(ctx *fasthttp.RequestCtx) {}
	handler2 := func(ctx *fasthttp.RequestCtx) {}

	tree.Insert("/abc", handler1, 1, "prefix")
	tree.Insert("/abx", handler2, 2, "prefix")

	// 应该正确分割 /ab 公共前缀
	result := tree.FindLongestPrefix("/abc")
	if result == nil {
		t.Error("should find /abc after split")
	}
}

func TestRadixTree_FindLongestPrefix(t *testing.T) {
	tree := NewRadixTree()
	handler := func(ctx *fasthttp.RequestCtx) {}

	tree.Insert("/", handler, 1, "prefix")
	tree.Insert("/api", handler, 2, "prefix")
	tree.Insert("/api/v1", handler, 3, "prefix")

	// "/" has priority 1 (wins), "/api" has 2, "/api/v1" has 3
	// Lower number = higher priority
	result := tree.FindLongestPrefix("/api/v1/users")
	if result == nil {
		t.Fatal("expected match")
	}
	if result.Path != "/" {
		t.Errorf("expected / (priority 1 wins), got %s", result.Path)
	}
}

func TestRadixTree_Insert_AfterInitialized(t *testing.T) {
	tree := NewRadixTree()
	handler := func(ctx *fasthttp.RequestCtx) {}

	tree.Insert("/api", handler, 1, "prefix")
	tree.MarkInitialized()

	err := tree.Insert("/api/v2", handler, 2, "prefix")
	if err == nil {
		t.Error("should fail when inserting after initialized")
	}
}

func TestRadixTree_Insert_DuplicatePath(t *testing.T) {
	tree := NewRadixTree()
	handler := func(ctx *fasthttp.RequestCtx) {}

	tree.Insert("/api", handler, 1, "prefix")
	err := tree.Insert("/api", handler, 2, "prefix")
	if err == nil {
		t.Error("should fail on duplicate path")
	}
}

func TestRadixTree_FindLongestPrefix_NoMatch(t *testing.T) {
	tree := NewRadixTree()
	handler := func(ctx *fasthttp.RequestCtx) {}

	tree.Insert("/api", handler, 1, "prefix")

	result := tree.FindLongestPrefix("/other")
	if result != nil {
		t.Errorf("expected nil for no match, got %+v", result)
	}
}

func TestRadixTree_PriorityComparison(t *testing.T) {
	tree := NewRadixTree()
	h1 := func(ctx *fasthttp.RequestCtx) {}
	h2 := func(ctx *fasthttp.RequestCtx) {}

	tree.Insert("/api", h1, 5, "prefix")
	tree.Insert("/api/users", h2, 2, "prefix")

	// Lower priority number wins
	result := tree.FindLongestPrefix("/api/users")
	if result == nil {
		t.Fatal("expected match")
	}
	if result.Priority != 2 {
		t.Errorf("expected priority 2, got %d", result.Priority)
	}
}

func TestRadixTree_Insert_ExactMatch(t *testing.T) {
	// Case 3: path 完全匹配节点前缀，设置 handler
	tree := NewRadixTree()
	handler := func(ctx *fasthttp.RequestCtx) { ctx.SetBodyString("exact") }

	// 先插入父路径
	tree.Insert("/api", handler, 1, "prefix")

	// 再次插入相同路径（应该报错重复）
	err := tree.Insert("/api", handler, 2, "prefix")
	if err == nil {
		t.Error("should return error for duplicate path")
	}

	// 验证原 handler 未被覆盖
	result := tree.FindLongestPrefix("/api")
	if result == nil || result.Priority != 1 {
		t.Errorf("original handler should not be overwritten, got priority %d", result.Priority)
	}
}
