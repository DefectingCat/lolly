// Package cache 提供文件缓存功能测试。
package cache

import (
	"testing"
	"time"
)

func TestFileCacheLastAccessUpdatedOnHit(t *testing.T) {
	c := NewFileCache(1, 1<<20, 10*time.Second)
	c.Set("/x", []byte("data"), 4, time.Now(), "text/plain")

	entry := c.entries["/x"]
	before := entry.LastAccess
	// 将访问时间设为过去，确保超过 accessUpdateInterval 阈值但不超过 inactive
	entry.LastAccess = before.Add(-7 * time.Second)

	c.Get("/x")
	after := c.entries["/x"].LastAccess

	if !after.After(before) {
		t.Fatal("LastAccess was not updated on cache hit")
	}
}
