// Package handler 提供 HTTP 请求处理器，包括路由、静态文件服务和零拷贝传输。
//
// 该文件包含静态文件服务相关的核心逻辑，包括：
//   - 静态文件请求处理
//   - 目录索引文件支持
//   - 文件缓存和零拷贝传输优化
//   - 预压缩文件支持
//
// 主要用途：
//
//	用于提供静态文件服务，支持缓存和零拷贝传输优化。
//
// 注意事项：
//   - 自动处理目录遍历攻击防护
//   - 支持多索引文件（如 index.html、index.htm）
//   - 支持预压缩 .gz 文件
//
// 作者：xfy
package handler

import (
	"mime"
	"os"
	"path/filepath"
	"strings"

	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/cache"
	"rua.plus/lolly/internal/middleware/compression"
)

// StaticHandler 静态文件处理器。
//
// 提供静态文件服务，支持目录索引、文件缓存和零拷贝传输。
//
// 注意事项：
//   - 自动处理目录遍历攻击防护（拒绝包含 ".." 的路径）
//   - 并发安全，可在多个 goroutine 中使用
//   - 大文件（>= 8KB）自动启用零拷贝传输
//   - alias 与 root 互斥，同时配置时 alias 优先
type StaticHandler struct {
	// root 静态文件根目录
	root string

	// alias 路径别名（与 root 互斥）
	// 例如：path: "/images/", alias: "/var/www/img/"
	// 请求 "/images/logo.png" -> 文件 "/var/www/img/logo.png"
	alias string

	// pathPrefix 路径前缀，会被剥离后拼接 root
	pathPrefix string

	// index 索引文件列表，当请求目录时依次查找
	index []string

	// useSendfile 是否启用零拷贝传输（大文件优化）
	useSendfile bool

	// fileCache 文件缓存实例（可选）
	fileCache *cache.FileCache

	// gzipStatic 预压缩文件支持（可选）
	gzipStatic *compression.GzipStatic

	// tryFiles 按顺序尝试查找的文件列表
	// 支持 $uri 和 $uri/ 占位符
	tryFiles []string

	// tryFilesPass 内部重定向是否触发中间件
	tryFilesPass bool

	// router 用于内部重定向时重新路由（当 tryFilesPass 为 true）
	router *Router
}

// NewStaticHandler 创建静态文件处理器。
//
// 初始化并返回一个新的静态文件处理器实例。
//
// 参数：
//   - root: 静态文件根目录路径
//   - pathPrefix: 路径前缀，会被剥离后拼接 root
//   - index: 索引文件列表，当请求目录时依次查找（如 ["index.html", "index.htm"]）
//   - useSendfile: 是否启用零拷贝传输（大文件优化）
//
// 返回值：
//   - *StaticHandler: 新创建的静态文件处理器
//
// 使用示例：
//
//	handler := handler.NewStaticHandler("/var/www", "/", []string{"index.html"}, true)
func NewStaticHandler(root, pathPrefix string, index []string, useSendfile bool) *StaticHandler {
	return &StaticHandler{
		root:        root,
		pathPrefix:  pathPrefix,
		index:       index,
		useSendfile: useSendfile,
	}
}

// NewStaticHandlerWithAlias 创建带 alias 的静态文件处理器。
//
// alias 与 root 的区别：
//   - root: 请求路径附加到 root 后
//     例如：root "/var/www", 请求 "/images/logo.png" -> "/var/www/images/logo.png"
//   - alias: 请求路径替换匹配部分
//     例如：alias "/var/www/img/", 请求 "/images/logo.png" -> "/var/www/img/logo.png"
//
// 参数：
//   - alias: 路径别名
//   - pathPrefix: 路径前缀，用于匹配和替换
//   - index: 索引文件列表
//   - useSendfile: 是否启用零拷贝传输
//
// 使用示例：
//
//	handler := handler.NewStaticHandlerWithAlias("/var/www/img/", "/images/", []string{"index.html"}, true)
func NewStaticHandlerWithAlias(alias, pathPrefix string, index []string, useSendfile bool) *StaticHandler {
	return &StaticHandler{
		alias:       alias,
		pathPrefix:  pathPrefix,
		index:       index,
		useSendfile: useSendfile,
	}
}

// SetAlias 设置路径别名。
//
// alias 与 root 互斥，设置 alias 会清空 root。
//
// 参数：
//   - alias: 路径别名
func (h *StaticHandler) SetAlias(alias string) {
	h.alias = alias
	if alias != "" {
		h.root = ""
	}
}

// SetRoot 设置静态文件根目录。
//
// root 与 alias 互斥，设置 root 会清空 alias。
//
// 参数：
//   - root: 静态文件根目录
func (h *StaticHandler) SetRoot(root string) {
	h.root = root
	if root != "" {
		h.alias = ""
	}
}

// GetAlias 获取路径别名。
func (h *StaticHandler) GetAlias() string {
	return h.alias
}

// GetRoot 获取静态文件根目录。
func (h *StaticHandler) GetRoot() string {
	return h.root
}

// SetFileCache 设置文件缓存。
//
// 为静态文件处理器启用文件缓存功能。
// 缓存可以显著提升小文件的访问性能。
//
// 参数：
//   - fc: 文件缓存实例
//
// 注意事项：
//   - 仅对小于 1MB 的文件启用缓存
//   - 缓存会自动检测文件修改并更新
func (h *StaticHandler) SetFileCache(fc *cache.FileCache) {
	h.fileCache = fc
}

// SetGzipStatic 设置预压缩文件支持。
//
// 启用后，对于匹配扩展名的请求，优先发送 .gz 预压缩文件。
//
// 参数：
//   - enabled: 是否启用预压缩支持
//   - extensions: 需要支持预压缩的文件扩展名列表（如 [".html", ".css", ".js"]）
//
// 使用示例：
//
//	handler.SetGzipStatic(true, []string{".html", ".css", ".js"})
func (h *StaticHandler) SetGzipStatic(enabled bool, extensions []string) {
	if enabled {
		h.gzipStatic = compression.NewGzipStatic(true, h.root, extensions)
	}
}

// SetTryFiles 设置 try_files 配置。
//
// 配置按顺序尝试查找的文件列表，支持 $uri 和 $uri/ 占位符。
// 用于 SPA 部署，当请求的文件不存在时可以回退到指定文件。
//
// 参数：
//   - tryFiles: 按顺序尝试的文件列表，如 ["$uri", "$uri/", "/index.html"]
//   - tryFilesPass: 内部重定向是否触发中间件，默认为 false
//   - router: 当 tryFilesPass 为 true 时使用的路由器
//
// 使用示例：
//
//	handler.SetTryFiles([]string{"$uri", "$uri/", "/index.html"}, false, nil)
func (h *StaticHandler) SetTryFiles(tryFiles []string, tryFilesPass bool, router *Router) {
	h.tryFiles = tryFiles
	h.tryFilesPass = tryFilesPass
	h.router = router
}

// Handle 处理静态文件请求。
//
// 根据请求路径查找并返回对应的静态文件。
// 支持目录索引文件、try_files、缓存查找和零拷贝传输。
//
// 参数：
//   - ctx: fasthttp 请求上下文
//
// 处理流程：
//  1. 安全检查：防止目录遍历攻击
//  2. 如果配置了 try_files，按顺序尝试查找文件
//  3. 检查文件/目录是否存在
//  4. 如果是目录，尝试查找索引文件
//  5. 尝试发送预压缩文件
//  6. 尝试从缓存获取
//  7. 大文件使用零拷贝传输
//  8. 读取文件并存入缓存
func (h *StaticHandler) Handle(ctx *fasthttp.RequestCtx) {
	reqPath := string(ctx.Path())

	// 安全检查：防止目录遍历
	if strings.Contains(reqPath, "..") {
		ctx.Error("Forbidden", fasthttp.StatusForbidden)
		return
	}

	// 如果配置了 try_files，按顺序尝试
	if len(h.tryFiles) > 0 {
		h.handleTryFiles(ctx, reqPath)
		return
	}

	// 标准处理流程
	h.handleStandard(ctx, reqPath)
}

// handleTryFiles 处理 try_files 逻辑。
//
// 按顺序尝试查找文件，支持 $uri 和 $uri/ 占位符。
//
// 占位符说明：
//   - $uri: 请求路径对应的文件
//   - $uri/: 请求路径对应的目录下的索引文件
//
// 参数：
//   - ctx: fasthttp 请求上下文
//   - reqPath: 原始请求路径
func (h *StaticHandler) handleTryFiles(ctx *fasthttp.RequestCtx, reqPath string) {
	// 获取相对路径（剥离路径前缀）
	relPath := reqPath
	if h.pathPrefix != "" && h.pathPrefix != "/" {
		relPath = strings.TrimPrefix(reqPath, h.pathPrefix)
		if !strings.HasPrefix(relPath, "/") {
			relPath = "/" + relPath
		}
	}

	for _, tryFile := range h.tryFiles {
		// 解析占位符
		targetPath := h.resolveTryFilePath(tryFile, relPath)

		// 构建完整文件路径（支持 alias 和 root）
		var filePath string
		if h.alias != "" {
			filePath = filepath.Join(h.alias, targetPath)
		} else {
			filePath = filepath.Join(h.root, targetPath)
		}

		// 检查文件/目录是否存在
		info, err := os.Stat(filePath)
		if err != nil {
			continue // 不存在，尝试下一个
		}

		if info.IsDir() {
			// 如果是目录，尝试查找索引文件
			for _, idx := range h.index {
				idxPath := filepath.Join(filePath, idx)
				if idxInfo, err := os.Stat(idxPath); err == nil && !idxInfo.IsDir() {
					h.serveFile(ctx, idxPath, idxInfo)
					return
				}
			}
			continue // 目录中没有索引文件，尝试下一个
		}

		// 找到文件，检查是否是内部重定向
		if tryFile != "$uri" && !strings.HasPrefix(tryFile, "$uri") {
			// 这是内部重定向（fallback 文件）
			h.handleInternalRedirect(ctx, targetPath)
			return
		}

		// 直接服务文件
		h.serveFile(ctx, filePath, info)
		return
	}

	// 所有 try_files 都未找到
	ctx.Error("Not Found", fasthttp.StatusNotFound)
}

// resolveTryFilePath 解析 try_files 中的占位符。
//
// 参数：
//   - tryFile: try_files 配置项
//   - relPath: 相对请求路径
//
// 返回值：
//   - string: 解析后的文件路径
func (h *StaticHandler) resolveTryFilePath(tryFile, relPath string) string {
	switch {
	case tryFile == "$uri":
		return relPath
	case tryFile == "$uri/":
		return relPath + "/"
	case strings.HasPrefix(tryFile, "/"):
		// 绝对路径，直接返回（去掉开头的 /）
		return tryFile[1:]
	default:
		// 其他情况直接返回
		return tryFile
	}
}

// handleInternalRedirect 处理内部重定向。
//
// 当 try_files 的回退文件与原始请求不同时触发。
// 根据 tryFilesPass 配置决定是否重新进入中间件链。
//
// 参数：
//   - ctx: fasthttp 请求上下文
//   - targetPath: 重定向目标路径（相对于 root 或 alias）
func (h *StaticHandler) handleInternalRedirect(ctx *fasthttp.RequestCtx, targetPath string) {
	if h.tryFilesPass && h.router != nil {
		// tryFilesPass 为 true，重新进入中间件链
		// 修改请求路径后重新路由
		newPath := h.pathPrefix + targetPath
		if !strings.HasPrefix(newPath, "/") {
			newPath = "/" + newPath
		}
		ctx.Request.SetRequestURI(newPath)
		h.router.Handler()(ctx)
		return
	}

	// tryFilesPass 为 false（默认），直接服务文件，不触发中间件
	var filePath string
	if h.alias != "" {
		filePath = filepath.Join(h.alias, targetPath)
	} else {
		filePath = filepath.Join(h.root, targetPath)
	}
	info, err := os.Stat(filePath)
	if err != nil {
		ctx.Error("Not Found", fasthttp.StatusNotFound)
		return
	}
	if info.IsDir() {
		ctx.Error("Forbidden", fasthttp.StatusForbidden)
		return
	}
	h.serveFile(ctx, filePath, info)
}

// handleStandard 标准静态文件处理流程。
//
// 参数：
//   - ctx: fasthttp 请求上下文
//   - reqPath: 请求路径
func (h *StaticHandler) handleStandard(ctx *fasthttp.RequestCtx, reqPath string) {
	// 计算文件路径
	var filePath string

	if h.alias != "" {
		// alias 模式：将匹配的路径前缀替换为 alias
		// 例如：path: "/images/", alias: "/var/www/img/"
		// 请求 "/images/logo.png" -> 文件 "/var/www/img/logo.png"
		relPath := reqPath
		if h.pathPrefix != "" && h.pathPrefix != "/" {
			relPath = strings.TrimPrefix(reqPath, h.pathPrefix)
			if !strings.HasPrefix(relPath, "/") {
				relPath = "/" + relPath
			}
		}
		// 使用 alias 替换匹配部分
		filePath = filepath.Join(h.alias, relPath)
	} else {
		// root 模式：将请求路径附加到 root
		// 剥离路径前缀
		relPath := reqPath
		if h.pathPrefix != "" && h.pathPrefix != "/" {
			relPath = strings.TrimPrefix(reqPath, h.pathPrefix)
			if !strings.HasPrefix(relPath, "/") {
				relPath = "/" + relPath
			}
		}

		// 拼接文件路径
		filePath = filepath.Join(h.root, relPath)
	}

	// 检查文件/目录是否存在
	info, err := os.Stat(filePath)
	if err != nil {
		ctx.Error("Not Found", fasthttp.StatusNotFound)
		return
	}

	// 如果是目录，尝试索引文件
	if info.IsDir() {
		for _, idx := range h.index {
			idxPath := filepath.Join(filePath, idx)
			if idxInfo, err := os.Stat(idxPath); err == nil && !idxInfo.IsDir() {
				h.serveFile(ctx, idxPath, idxInfo)
				return
			}
		}
		ctx.Error("Forbidden", fasthttp.StatusForbidden)
		return
	}

	// 直接返回文件
	h.serveFile(ctx, filePath, info)
}

// serveFile 提供文件服务，支持缓存和零拷贝传输。
//
// 内部方法，负责实际的文件发送逻辑。
//
// 参数：
//   - ctx: fasthttp 请求上下文
//   - filePath: 文件绝对路径
//   - info: 文件信息（用于判断文件大小和修改时间）
func (h *StaticHandler) serveFile(ctx *fasthttp.RequestCtx, filePath string, info os.FileInfo) {
	// 尝试发送预压缩文件
	if h.gzipStatic != nil {
		relPath := strings.TrimPrefix(filePath, h.root)
		if h.gzipStatic.ServeFile(ctx, relPath) {
			return // 预压缩文件已发送
		}
	}

	// 尝试从缓存获取
	if h.fileCache != nil {
		if entry, ok := h.fileCache.Get(filePath); ok {
			// 检查文件是否被修改
			if entry.ModTime.Equal(info.ModTime()) {
				// 缓存命中且文件未修改
				ctx.Response.SetBody(entry.Data)
				ctx.Response.Header.SetContentType(mime.TypeByExtension(filepath.Ext(filePath)))
				return
			}
			// 文件已修改，删除旧缓存
			h.fileCache.Delete(filePath)
		}
	}

	// 大文件使用零拷贝传输
	if h.useSendfile && info.Size() >= MinSendfileSize {
		file, err := os.Open(filePath)
		if err == nil {
			defer func() { _ = file.Close() }()
			if err := SendFile(ctx, file, 0, info.Size()); err == nil {
				return
			}
			// sendfile 失败，fallback 到 ServeFile
		}
	}

	// 读取文件内容
	data, err := os.ReadFile(filePath)
	if err != nil {
		ctx.Error("Internal Server Error", fasthttp.StatusInternalServerError)
		return
	}

	// 存入缓存（仅对小文件缓存）
	if h.fileCache != nil && info.Size() < 1024*1024 { // < 1MB
		_ = h.fileCache.Set(filePath, data, info.Size(), info.ModTime())
	}

	ctx.Response.SetBody(data)
	ctx.Response.Header.SetContentType(mime.TypeByExtension(filepath.Ext(filePath)))
}
