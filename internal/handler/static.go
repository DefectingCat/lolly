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
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/cache"
	"rua.plus/lolly/internal/middleware/compression"
	"rua.plus/lolly/internal/mimeutil"
	"rua.plus/lolly/internal/utils"
)

const httpTimeFormat = "Mon, 02 Jan 2006 15:04:05 GMT"

// Expires directive constants
const (
	expiresOff = "off"
	expiresMax = "max"
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
	// 指针类型字段（按大小排列）
	fileCache     *cache.FileCache
	fileInfoCache *FileInfoCache // FileInfo 缓存，减少 os.Stat 调用
	gzipStatic    *compression.GzipStatic
	router        *Router
	// 字符串字段
	root       string
	alias      string
	pathPrefix string
	expires    string // 缓存过期时间（nginx 兼容格式）
	// 切片字段
	index    []string
	tryFiles []string
	// AutoIndex 配置
	autoIndex          bool
	autoIndexFormat    string
	autoIndexLocaltime bool
	autoIndexExactSize bool
	// 基本类型字段
	pathPrefixLen int           // 预计算的路径前缀长度，用于零分配路径剥离
	cacheTTL      time.Duration // 缓存新鲜度 TTL（默认 5s，0 表示每次验证 ModTime）
	useSendfile   bool
	tryFilesPass  bool
	symlinkCheck  bool
	internal      bool
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
	// 规范化 root 路径，确保 TrimPrefix 能正确工作
	// filepath.Clean 会去掉 ./ 并规范化路径分隔符
	cleanRoot := filepath.Clean(root)
	if !strings.HasSuffix(cleanRoot, string(filepath.Separator)) {
		cleanRoot += string(filepath.Separator)
	}

	// 预计算前缀长度，用于零分配路径剥离
	prefixLen := len(pathPrefix)
	if pathPrefix == "/" {
		prefixLen = 0 // 根路径无需剥离
	}

	return &StaticHandler{
		root:          cleanRoot,
		pathPrefix:    pathPrefix,
		pathPrefixLen: prefixLen,
		index:         index,
		useSendfile:   useSendfile,
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
	// 预计算前缀长度
	prefixLen := len(pathPrefix)
	if pathPrefix == "/" {
		prefixLen = 0
	}

	return &StaticHandler{
		alias:         alias,
		pathPrefix:    pathPrefix,
		pathPrefixLen: prefixLen,
		index:         index,
		useSendfile:   useSendfile,
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
	// 规范化 root 路径
	cleanRoot := filepath.Clean(root)
	if !strings.HasSuffix(cleanRoot, string(filepath.Separator)) {
		cleanRoot += string(filepath.Separator)
	}
	h.root = cleanRoot
	if cleanRoot != "" {
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

// SetFileInfoCache 设置 FileInfo 缓存。
//
// 为静态文件处理器启用 FileInfo 缓存功能。
// 缓存可以减少 os.Stat 调用，提升性能。
//
// 参数：
//   - fic: FileInfo 缓存实例
func (h *StaticHandler) SetFileInfoCache(fic *FileInfoCache) {
	h.fileInfoCache = fic
}

// SetGzipStatic 设置预压缩文件支持。
//
// 启用后，对于匹配扩展名的请求，优先发送预压缩文件。
//
// 参数：
//   - enabled: 是否启用预压缩支持
//   - extensions: 支持预压缩的源文件扩展名列表（如 [".html", ".css", ".js"]），为空使用默认值
//   - precompressedExtensions: 预压缩文件扩展名列表（如 [".br", ".gz"]），为空使用默认值
//
// 使用示例：
//
//	handler.SetGzipStatic(true, nil, []string{".gz", ".br"})
func (h *StaticHandler) SetGzipStatic(enabled bool, extensions, precompressedExtensions []string) {
	if enabled {
		h.gzipStatic = compression.NewGzipStatic(true, h.root, extensions, precompressedExtensions)
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

// SetSymlinkCheck 设置符号链接安全检查。
//
// 启用后，服务文件前会验证符号链接指向的文件是否在允许的根目录范围内。
// 防止通过符号链接访问敏感文件（如 /etc/passwd）。
//
// 参数：
//   - enabled: 是否启用符号链接安全检查
func (h *StaticHandler) SetSymlinkCheck(enabled bool) {
	h.symlinkCheck = enabled
}

// SetInternal 设置内部访问限制。
//
// 启用后，仅允许内部重定向访问该静态位置。
// 外部直接请求将返回 404 Not Found。
//
// 参数：
//   - enabled: 是否启用内部访问限制
func (h *StaticHandler) SetInternal(enabled bool) {
	h.internal = enabled
}

// SetExpires 设置缓存过期时间。
//
// 支持 nginx 兼容格式：30d, 1h, 1m, max, epoch, off
// 设置后会在响应中添加 Cache-Control 和 Expires 头。
//
// 参数：
//   - expires: 过期时间字符串
func (h *StaticHandler) SetExpires(expires string) {
	h.expires = expires
}

// SetAutoIndex 设置目录列表功能。
//
// 启用后，当请求目录且没有索引文件时，生成目录列表页面。
//
// 参数：
//   - enabled: 是否启用
//   - format: 输出格式（html/json/xml）
//   - localtime: 使用本地时间
//   - exactSize: 显示精确大小
func (h *StaticHandler) SetAutoIndex(enabled bool, format string, localtime, exactSize bool) {
	h.autoIndex = enabled
	h.autoIndexFormat = format
	h.autoIndexLocaltime = localtime
	h.autoIndexExactSize = exactSize
}

// SetCacheTTL 设置缓存新鲜度 TTL。
//
// TTL 控制缓存条目的新鲜度验证间隔。
// 在 TTL 窗口内，缓存命中时跳过 ModTime 验证以减少 os.Stat 调用。
//
// 参数：
//   - ttl: TTL 时间间隔
//
// TTL 值说明：
//   - ttl > 0: TTL 内跳过 ModTime 验证，过期后验证
//   - ttl = 0: 每次请求验证 ModTime（向后兼容）
//
// 默认 TTL 为 5 秒。
func (h *StaticHandler) SetCacheTTL(ttl time.Duration) {
	h.cacheTTL = ttl
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

	// 检查 internal 限制
	if h.internal && !utils.IsInternalRedirect(ctx) {
		utils.SendError(ctx, utils.ErrNotFound)
		return
	}

	// 安全检查：防止目录遍历
	if strings.Contains(reqPath, "..") {
		utils.SendError(ctx, utils.ErrForbidden)
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
	// 零分配路径剥离：使用切片替代 strings.TrimPrefix
	relPath := reqPath
	if h.pathPrefixLen > 0 {
		relPath = reqPath[h.pathPrefixLen:]
		if len(relPath) > 0 && relPath[0] != '/' {
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
					h.serveFile(ctx, idxPath, idxInfo, false)
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
		h.serveFile(ctx, filePath, info, false)
		return
	}

	// 所有 try_files 都未找到
	utils.SendError(ctx, utils.ErrNotFound)
}

// resolveTryFilePath 解析 try_files 中的占位符。
//
// 支持的占位符：
//   - $uri: 请求路径
//   - $uri/: 请求路径加斜杠
//   - $uri.<ext>: 请求路径加扩展名（如 $uri.html）
//
// nginx 兼容性说明：
//   - $uri 变量语义与 nginx try_files 一致
//   - 附加安全验证在 validateStatics 时执行
//
// 参数：
//   - tryFile: try_files 配置项（已在 validateStatics 时验证）
//   - relPath: 相对请求路径
//
// 返回值：
//   - string: 解析后的文件路径，根路径边界返回空字符串触发回退
func (h *StaticHandler) resolveTryFilePath(tryFile, relPath string) string {
	switch {
	// ====== 保留：现有逻辑 ======
	case tryFile == "$uri":
		return relPath
	case tryFile == "$uri/":
		return relPath + "/"

	// ====== 新增：动态后缀支持 ======
	case strings.HasPrefix(tryFile, "$uri."):
		// 提取后缀部分（包含点，如 ".html"）
		suffix := tryFile[4:] // "$uri" 是4个字符，后面是 ".html" 等后缀
		// 根路径边界处理：返回空字符串让 try_files 继续下一个条目
		// 避免 "/.html" 这样的隐藏文件名
		if relPath == "/" {
			return "" // 触发回退到下一个 try_files 条目
		}
		return relPath + suffix

	// ====== 保留：现有逻辑 ======
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
		utils.SendError(ctx, utils.ErrNotFound)
		return
	}
	if info.IsDir() {
		utils.SendError(ctx, utils.ErrForbidden)
		return
	}
	h.serveFile(ctx, filePath, info, false)
}

// handleStandard 标准静态文件处理流程。
//
// 参数：
//   - ctx: fasthttp 请求上下文
//   - reqPath: 请求路径
func (h *StaticHandler) handleStandard(ctx *fasthttp.RequestCtx, reqPath string) {
	// 计算文件路径
	var filePath string

	// 零分配路径剥离：使用切片替代 strings.TrimPrefix
	relPath := reqPath
	if h.pathPrefixLen > 0 {
		relPath = reqPath[h.pathPrefixLen:]
		if len(relPath) > 0 && relPath[0] != '/' {
			relPath = "/" + relPath
		}
	}

	if h.alias != "" {
		// alias 模式：将匹配的路径前缀替换为 alias
		filePath = filepath.Join(h.alias, relPath)
	} else {
		// root 模式：将请求路径附加到 root
		filePath = filepath.Join(h.root, relPath)
	}

	// 检查文件/目录是否存在
	// 先查 FileInfo 缓存（TTL 内信任缓存，不验证 ModTime）
	var info os.FileInfo
	var err error

	if h.fileInfoCache != nil {
		if cachedInfo, ok := h.fileInfoCache.Get(filePath); ok {
			info = cachedInfo
		}
	}

	if info == nil {
		// 缓存未命中，调用 os.Stat
		info, err = os.Stat(filePath)
		if err != nil {
			utils.SendError(ctx, utils.ErrNotFound)
			return
		}
		if h.fileInfoCache != nil {
			h.fileInfoCache.Set(filePath, info)
		}
	}

	// 符号链接安全检查
	if h.symlinkCheck {
		if err := h.validateSymlink(filePath); err != nil {
			utils.SendError(ctx, utils.ErrForbidden)
			return
		}
	}

	// 如果是目录，尝试索引文件
	if info.IsDir() {
		for _, idx := range h.index {
			idxPath := filepath.Join(filePath, idx)
			if idxInfo, err := os.Stat(idxPath); err == nil && !idxInfo.IsDir() {
				h.serveFile(ctx, idxPath, idxInfo, true)
				return
			}
		}
		// 尝试 autoindex
		if h.autoIndex {
			config := AutoIndexConfig{
				Format:    h.autoIndexFormat,
				Localtime: h.autoIndexLocaltime,
				ExactSize: h.autoIndexExactSize,
			}
			if GenerateAutoIndex(ctx, filePath, reqPath, config) {
				return
			}
		}
		utils.SendError(ctx, utils.ErrForbidden)
		return
	}

	// Phase 2: 缓存查找 + TTL 验证	// 在 serveFile 调用前检查缓存，减少 os.ReadFile 调用
	// 注意: CachedAt 迁移已在 FileCache.Get() 内部完成，确保并发安全
	etag := generateETag(info.ModTime(), info.Size())
	if isNotModified(ctx, etag, info.ModTime()) {
		ctx.Response.SetStatusCode(fasthttp.StatusNotModified)
		ctx.Response.Header.Set("ETag", etag)
		ctx.Response.Header.Set("Last-Modified", info.ModTime().UTC().Format(httpTimeFormat))
		ctx.Response.SkipBody = true
		return
	}
	if h.fileCache != nil {
		if entry, ok := h.fileCache.Get(filePath); ok {
			// TTL 验证（cacheTTL > 0 时启用）
			if h.cacheTTL > 0 && time.Since(entry.CachedAt) < h.cacheTTL {
				// TTL 内直接返回（无需验证 ModTime）
				ctx.Response.SetBody(entry.Data)
				ctx.Response.Header.SetContentType(mimeutil.DetectContentType(filePath))
				ctx.Response.Header.Set("ETag", entry.ETag)
				ctx.Response.Header.Set("Last-Modified", info.ModTime().UTC().Format(httpTimeFormat))
				return
			}

			// TTL 过期或未启用 TTL，验证文件新鲜度
			if entry.ModTime.Equal(info.ModTime()) {
				// 文件未修改，刷新 TTL 并返回
				if h.cacheTTL > 0 {
					h.fileCache.RefreshCachedAt(filePath)
				}
				ctx.Response.SetBody(entry.Data)
				ctx.Response.Header.SetContentType(mimeutil.DetectContentType(filePath))
				ctx.Response.Header.Set("ETag", entry.ETag)
				ctx.Response.Header.Set("Last-Modified", info.ModTime().UTC().Format(httpTimeFormat))
				return
			}

			// 文件已修改，删除缓存继续处理
			h.fileCache.Delete(filePath)
		}
	}

	// Phase 3: 缓存未命中，调用 serveFile 处理
	h.serveFile(ctx, filePath, info, true)
}

// serveFile 提供文件服务，支持缓存和零拷贝传输。
//
// 内部方法，负责实际的文件发送逻辑。
//
// 参数：
//   - ctx: fasthttp 请求上下文
//   - filePath: 文件绝对路径
//   - info: 文件信息（用于判断文件大小和修改时间）
func (h *StaticHandler) serveFile(ctx *fasthttp.RequestCtx, filePath string, info os.FileInfo, skipCacheLookup bool) {
	// 生成 ETag 并检查条件请求（在预压缩检查之前）
	etag := generateETag(info.ModTime(), info.Size())
	if isNotModified(ctx, etag, info.ModTime()) {
		ctx.Response.SetStatusCode(fasthttp.StatusNotModified)
		ctx.Response.Header.Set("ETag", etag)
		ctx.Response.Header.Set("Last-Modified", info.ModTime().UTC().Format(httpTimeFormat))
		h.setCacheHeaders(ctx)
		ctx.Response.SkipBody = true
		return
	}

	// 尝试发送预压缩文件
	if h.gzipStatic != nil {
		relPath := strings.TrimPrefix(filePath, h.root)
		if h.gzipStatic.ServeFile(ctx, relPath) {
			// 预压缩文件已发送，补充验证头
			ctx.Response.Header.Set("ETag", etag)
			ctx.Response.Header.Set("Last-Modified", info.ModTime().UTC().Format(httpTimeFormat))
			h.setCacheHeaders(ctx)
			return
		}
	}

	// 尝试从缓存获取
	if !skipCacheLookup && h.fileCache != nil {
		if entry, ok := h.fileCache.Get(filePath); ok {
			// 检查文件是否被修改
			if entry.ModTime.Equal(info.ModTime()) {
				// 缓存命中且文件未修改
				ctx.Response.SetBody(entry.Data)
				ctx.Response.Header.SetContentType(mimeutil.DetectContentType(filePath))
				ctx.Response.Header.Set("ETag", etag)
				ctx.Response.Header.Set("Last-Modified", info.ModTime().UTC().Format(httpTimeFormat))
				h.setCacheHeaders(ctx)
				return
			}
			// 文件已修改，删除旧缓存
			h.fileCache.Delete(filePath)
		}
	}

	// 大文件使用零拷贝传输
	// 使用 fasthttp 的 SetBodyStream，它会：
	// 1. 先写 HTTP 头到 bufio.Writer
	// 2. Flush HTTP 头到 socket（关键步骤）
	// 3. copyZeroAlloc → ReadFrom → sendfile
	// 这样保证 HTTP 头先发送，避免顺序错乱导致的 "200 0" malformed response
	if h.useSendfile && info.Size() >= MinSendfileSize {
		ctx.Response.Header.SetContentType(mimeutil.DetectContentType(filePath))
		ctx.Response.Header.Set("ETag", etag)
		ctx.Response.Header.Set("Last-Modified", info.ModTime().UTC().Format(httpTimeFormat))
		h.setCacheHeaders(ctx)

		file, err := os.Open(filePath)
		if err == nil {
			// SetBodyStream 会在 handler 返回后由 fasthttp 统一处理
			// HTTP 头写入、Flush 和 sendfile 的顺序
			ctx.Response.SetBodyStream(file, int(info.Size()))
			return
		}
	}

	// 读取文件内容
	data, err := os.ReadFile(filePath)
	if err != nil {
		utils.SendError(ctx, utils.ErrInternalError)
		return
	}

	// 存入缓存（仅对小文件缓存）
	if h.fileCache != nil && info.Size() < 1024*1024 { // < 1MB
		_ = h.fileCache.Set(filePath, data, info.Size(), info.ModTime())
	}

	ctx.Response.SetBody(data)
	ctx.Response.Header.SetContentType(mimeutil.DetectContentType(filePath))
	ctx.Response.Header.Set("ETag", etag)
	ctx.Response.Header.Set("Last-Modified", info.ModTime().UTC().Format(httpTimeFormat))
	h.setCacheHeaders(ctx)
}

// setCacheHeaders 设置缓存控制响应头。
func (h *StaticHandler) setCacheHeaders(ctx *fasthttp.RequestCtx) {
	if h.expires == "" || h.expires == expiresOff {
		return
	}

	if h.expires == "epoch" {
		ctx.Response.Header.Set("Cache-Control", "no-cache, no-store, must-revalidate")
		ctx.Response.Header.Set("Expires", "Thu, 01 Jan 1970 00:00:00 GMT")
		return
	}

	if h.expires == expiresMax {
		ctx.Response.Header.Set("Cache-Control", "public, max-age=315360000, immutable")
		ctx.Response.Header.Set("Expires", time.Now().Add(315360000*time.Second).UTC().Format(httpTimeFormat))
		return
	}

	maxAge := parseExpires(h.expires)
	if maxAge > 0 {
		ctx.Response.Header.Set("Cache-Control", fmt.Sprintf("public, max-age=%d", maxAge))
		ctx.Response.Header.Set("Expires", time.Now().Add(time.Duration(maxAge)*time.Second).UTC().Format(httpTimeFormat))
	}
}

// parseExpires 解析 nginx 兼容的过期时间格式。
// 支持格式：30d, 1h, 1m, 1s, 30d1h 等
// 返回秒数。
func parseExpires(expires string) int64 {
	if expires == "" || expires == expiresOff {
		return 0
	}
	if expires == expiresMax {
		return 315360000
	}
	if expires == "epoch" {
		return -1
	}

	var total int64
	var num int64
	for _, ch := range expires {
		switch {
		case ch >= '0' && ch <= '9':
			num = num*10 + int64(ch-'0')
		case ch == 'd':
			total += num * 86400
			num = 0
		case ch == 'h':
			total += num * 3600
			num = 0
		case ch == 'm':
			total += num * 60
			num = 0
		case ch == 's':
			total += num
			num = 0
		}
	}
	return total
}

// validateSymlink 验证符号链接是否安全。
//
// 检查文件是否是符号链接，如果是则验证链接指向的文件
// 是否在允许的根目录（root 或 alias）范围内。
// 防止通过符号链接访问敏感文件（如 /etc/passwd）。
//
// 参数：
//   - filePath: 要验证的文件路径
//
// 返回值：
//   - error: 如果符号链接不安全或解析失败，返回错误
func (h *StaticHandler) validateSymlink(filePath string) error {
	// 获取文件信息（不跟随符号链接）
	info, err := os.Lstat(filePath)
	if err != nil {
		return err
	}

	// 如果不是符号链接，直接返回成功
	if info.Mode()&os.ModeSymlink == 0 {
		return nil
	}

	// 获取符号链接指向的实际路径
	realPath, err := filepath.EvalSymlinks(filePath)
	if err != nil {
		return err
	}

	// 获取允许的基础路径
	basePath := h.root
	if h.alias != "" {
		basePath = h.alias
	}

	// 如果没有配置根目录，拒绝符号链接
	if basePath == "" {
		return os.ErrPermission
	}

	// 解析基础路径为绝对路径
	absBase, err := filepath.Abs(basePath)
	if err != nil {
		return err
	}

	// 解析目标路径为绝对路径
	absTarget, err := filepath.Abs(realPath)
	if err != nil {
		return err
	}

	// 确保目标路径在基础路径范围内
	if !strings.HasPrefix(absTarget, absBase+string(filepath.Separator)) && absTarget != absBase {
		return os.ErrPermission
	}

	return nil
}

// generateETag 基于 ModTime 和 Size 生成 ETag。
// 使用 strconv.AppendInt 避免 fmt.Sprintf 分配。
func generateETag(modTime time.Time, size int64) string {
	var buf [32]byte
	b := buf[:0]
	b = append(b, '"')
	b = strconv.AppendInt(b, modTime.Unix(), 16)
	b = append(b, '-')
	b = strconv.AppendInt(b, size, 16)
	b = append(b, '"')
	return string(b)
}

// isNotModified 检查条件请求是否匹配（返回 true 表示应返回 304）。
func isNotModified(ctx *fasthttp.RequestCtx, etag string, modTime time.Time) bool {
	if match := ctx.Request.Header.Peek("If-None-Match"); len(match) > 0 {
		// RFC 9110: If-None-Match = #entity-tag，逗号分隔
		for tag := range strings.SplitSeq(string(match), ",") {
			if strings.TrimSpace(tag) == etag {
				return true
			}
		}
	}
	if since := ctx.Request.Header.Peek("If-Modified-Since"); len(since) > 0 {
		if t, err := fasthttp.ParseHTTPDate(since); err == nil {
			return !modTime.After(t)
		}
	}
	return false
}
