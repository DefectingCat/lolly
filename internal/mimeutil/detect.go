// Package mimeutil 提供 MIME 类型检测工具函数。
//
// 该文件实现了基于文件扩展名的 MIME 类型检测，
// 补充 Go 标准库 mime 包中缺失或错误的映射。
//
// 主要功能：
//   - 本地 MIME 映射：避免 mime.AddExtensionType 的全局副作用
//   - 自动回退：未覆盖的扩展名回退到标准库
//   - 大小写处理：自动将扩展名转为小写再查找
//   - LRU 缓存：缓存检测结果，减少重复计算
//
// 注意事项：
//   - 使用包本地映射而非全局修改，确保多线程安全
//   - 部分扩展名（如 .otf、.webm）Go 标准库返回错误类型，已在此纠正
//
// 作者：xfy
package mimeutil

import (
	"container/list"
	"mime"
	"path/filepath"
	"strings"
	"sync"
)

const mimeCacheSize = 64 // 常见扩展名约 50 个

// mimeCacheEntry MIME 缓存条目
type mimeCacheEntry struct {
	ext      string
	mimeType string
	element  *list.Element
}

// mimeOverrides 补充 Go 标准库缺失或错误的 MIME 类型映射。
// 使用包本地映射而非 mime.AddExtensionType，避免全局副作用。
//
// 注意: 部分扩展名 Go 返回错误类型而非缺失:
//   - .otf: Go 映射到 OpenDocument 公式模板，应为字体格式
//   - .webm: Go 返回 audio/webm，但 webm 可包含视频
var (
	mimeOverrides = map[string]string{
		".eot":         "application/vnd.ms-fontobject", // 缺失
		".otf":         "font/otf",                      // Go 返回错误类型
		".webmanifest": "application/manifest+json",     // 缺失
		".map":         "application/json",              // 缺失
		".webm":        "video/webm",                    // Go 返回 audio/webm
		// 注意: Go 1.26.2+ 已正确支持 .mjs, .avif, .woff, .woff2
	}
	mimeMutex sync.RWMutex

	// MIME 检测结果缓存（O(1) LRU）
	mimeCache   = make(map[string]*mimeCacheEntry, mimeCacheSize)
	mimeLRU     = list.New()
	mimeCacheMu sync.Mutex

	defaultMIME  = "application/octet-stream"
	defaultMutex sync.RWMutex
)

// AddTypes 添加自定义 MIME 类型映射（线程安全）。
//
// 参数:
//   - types: 扩展名到 MIME 类型的映射，扩展名会自动转为小写
func AddTypes(types map[string]string) {
	mimeMutex.Lock()
	for ext, mimeType := range types {
		mimeOverrides[strings.ToLower(ext)] = mimeType
	}
	mimeMutex.Unlock()

	// 清除缓存中受影响的条目
	mimeCacheMu.Lock()
	for ext := range types {
		ext = strings.ToLower(ext)
		if entry, ok := mimeCache[ext]; ok {
			mimeLRU.Remove(entry.element)
			delete(mimeCache, ext)
		}
	}
	mimeCacheMu.Unlock()
}

// SetDefaultType 设置默认 MIME 类型（线程安全）。
//
// 参数:
//   - defaultType: 默认 MIME 类型
func SetDefaultType(defaultType string) {
	defaultMutex.Lock()
	defer defaultMutex.Unlock()
	defaultMIME = defaultType
}

// GetDefaultType 获取默认 MIME 类型（线程安全）。
//
// 返回值:
//   - string: 当前默认 MIME 类型
func GetDefaultType() string {
	defaultMutex.RLock()
	defer defaultMutex.RUnlock()
	return defaultMIME
}

// DetectContentType 检测文件的 MIME 类型。
//
// 优先使用包本地映射，回退到 Go 标准库 mime.TypeByExtension。
// 自动处理扩展名大小写问题。
// 使用 LRU 缓存减少重复计算。
//
// 参数:
//   - filePath: 文件路径
//
// 返回值:
//   - string: MIME 类型，未知类型返回空字符串
func DetectContentType(filePath string) string {
	ext := strings.ToLower(filepath.Ext(filePath))

	// 先查缓存
	mimeCacheMu.Lock()
	if entry, ok := mimeCache[ext]; ok {
		// 命中，移动到 LRU 头部
		mimeLRU.MoveToFront(entry.element)
		mimeType := entry.mimeType
		mimeCacheMu.Unlock()
		return mimeType
	}
	mimeCacheMu.Unlock()

	// 未命中，计算
	mimeMutex.RLock()
	mimeType, ok := mimeOverrides[ext]
	mimeMutex.RUnlock()

	if !ok {
		mimeType = mime.TypeByExtension(ext)
	}

	// 写入缓存
	mimeCacheMu.Lock()
	defer mimeCacheMu.Unlock()

	// 双重检查（可能其他 goroutine 已写入）
	if entry, ok := mimeCache[ext]; ok {
		mimeLRU.MoveToFront(entry.element)
		return entry.mimeType
	}

	// 淘汰最久未用的
	if mimeLRU.Len() >= mimeCacheSize {
		if oldest := mimeLRU.Back(); oldest != nil {
			if entry, ok := oldest.Value.(*mimeCacheEntry); ok {
				delete(mimeCache, entry.ext)
			}
			mimeLRU.Remove(oldest)
		}
	}

	// 插入新条目
	entry := &mimeCacheEntry{ext: ext, mimeType: mimeType}
	entry.element = mimeLRU.PushFront(entry)
	mimeCache[ext] = entry

	return mimeType
}
