package utils

import (
	"strconv"
	"time"
)

// GenerateETag 基于 ModTime 和 Size 生成 ETag。
// 使用 strconv.AppendInt 避免 fmt.Sprintf 分配。
// 格式: "<modtime-unix-hex>-<size-hex>"
func GenerateETag(modTime time.Time, size int64) string {
	var buf [32]byte
	b := buf[:0]
	b = append(b, '"')
	b = strconv.AppendInt(b, modTime.Unix(), 16)
	b = append(b, '-')
	b = strconv.AppendInt(b, size, 16)
	b = append(b, '"')
	return string(b)
}
