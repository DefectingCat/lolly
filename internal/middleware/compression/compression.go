// Package compression 提供 HTTP 响应压缩中间件，支持 gzip 和 brotli 算法。
//
// 该文件包含压缩相关的核心逻辑，包括：
//   - gzip 压缩（兼容性好，所有浏览器支持）
//   - brotli 压缩（压缩率更高，适合现代浏览器）
//   - MIME 类型过滤
//   - 最小压缩大小控制
//
// 主要用途：
//
//	用于压缩 HTTP 响应内容，减少传输数据量，提升页面加载速度。
//
// 注意事项：
//   - 使用缓冲池复用压缩对象，减少内存分配
//   - 小于 MinSize 的响应不压缩
//
// 作者：xfy
package compression

import (
	"bufio"
	"bytes"
	"strings"
	"sync"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/gzip"
	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/config"
)

// streamingThreshold 流式压缩阈值。
// 响应体超过此大小时使用 SetBodyStreamWriter 流式压缩，
// 消除 compressed buffer 分配，降低内存峰值。
const streamingThreshold = 64 * 1024 // 64KB

// Algorithm 压缩算法类型。
type Algorithm int

const (
	// AlgorithmGzip 使用 gzip 压缩。
	AlgorithmGzip Algorithm = iota
	// AlgorithmBrotli 使用 brotli 压缩。
	AlgorithmBrotli

	compressionGZIP = "gzip"
)

// Middleware 响应压缩中间件。
type Middleware struct {
	// gzipPool gzip.Writer 缓冲池
	gzipPool sync.Pool
	// brotliPool brotli.Writer 缓冲池
	brotliPool sync.Pool
	// types 可压缩的 MIME 类型列表
	types []string

	// level 压缩级别（1-9）
	level int
	// minSize 最小压缩大小（字节）
	minSize int
	// algorithm 压缩算法
	algorithm Algorithm
}

// New 创建压缩中间件。
//
// 参数：
//   - cfg: 压缩配置，包含算法类型、压缩级别、最小压缩大小等
//
// 返回值：
//   - *Middleware: 压缩中间件实例
//   - error: 配置无效时返回错误
func New(cfg *config.CompressionConfig) (*Middleware, error) {
	if cfg == nil {
		cfg = &config.CompressionConfig{
			Type:    "gzip",
			Level:   6,
			MinSize: 1024,
			Types:   defaultCompressibleTypes(),
		}
	}

	// 设置默认值
	if cfg.Level == 0 {
		cfg.Level = 6
	}
	if cfg.MinSize == 0 {
		cfg.MinSize = 1024
	}
	if len(cfg.Types) == 0 {
		cfg.Types = defaultCompressibleTypes()
	}

	// 解析算法类型
	var algo Algorithm
	switch strings.ToLower(cfg.Type) {
	case "brotli":
		algo = AlgorithmBrotli
	case compressionGZIP:
		algo = AlgorithmGzip
	case "both":
		// both 模式优先使用 brotli（如果客户端支持）
		algo = AlgorithmBrotli
	default:
		algo = AlgorithmGzip
	}

	m := &Middleware{
		types:     cfg.Types,
		level:     cfg.Level,
		minSize:   cfg.MinSize,
		algorithm: algo,
	}

	// 初始化缓冲池
	m.gzipPool = sync.Pool{
		New: func() any {
			w, err := gzip.NewWriterLevel(nil, cfg.Level)
			if err != nil {
				// 使用默认压缩级别作为回退
				w, _ = gzip.NewWriterLevel(nil, gzip.DefaultCompression)
			}
			return w
		},
	}

	// 初始化 brotli 缓冲池
	m.brotliPool = sync.Pool{
		New: func() any {
			return brotli.NewWriterOptions(nil, brotli.WriterOptions{
				Quality: cfg.Level,
			})
		},
	}

	return m, nil
}

// defaultCompressibleTypes 返回默认可压缩的 MIME 类型。
func defaultCompressibleTypes() []string {
	return []string{
		"text/html",
		"text/css",
		"text/javascript",
		"text/plain",
		"text/xml",
		"application/json",
		"application/javascript",
		"application/xml",
		"application/xhtml+xml",
	}
}

// Name 返回中间件名称。
func (m *Middleware) Name() string {
	return "compression"
}

// Process 应用压缩中间件。
//
// 参数：
//   - next: 下一个请求处理器
//
// 返回值：
//   - fasthttp.RequestHandler: 包装后的请求处理器
func (m *Middleware) Process(next fasthttp.RequestHandler) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		// 检查客户端是否支持压缩（零拷贝使用 []byte）
		acceptEncoding := ctx.Request.Header.Peek("Accept-Encoding")

		// 根据算法和客户端支持选择压缩方式
		var useGzip, useBrotli bool
		switch m.algorithm {
		case AlgorithmGzip:
			useGzip = bytes.Contains(acceptEncoding, []byte("gzip"))
		case AlgorithmBrotli:
			// brotli 或 both 模式
			if bytes.Contains(acceptEncoding, []byte("br")) {
				useBrotli = true
			} else if bytes.Contains(acceptEncoding, []byte("gzip")) {
				useGzip = true
			}
		}

		// 如果不需要压缩，直接执行
		if !useGzip && !useBrotli {
			next(ctx)
			return
		}

		// 执行处理器
		next(ctx)

		// 检查是否已有 Content-Encoding（由 gzip_static 或上游处理器设置）
		// 已编码的响应不应再次压缩，避免双重编码导致数据损坏
		if len(ctx.Response.Header.Peek("Content-Encoding")) > 0 {
			return
		}

		// 获取响应体
		body := ctx.Response.Body()
		bodyLen := len(body)

		// 检查是否满足压缩条件
		if bodyLen < m.minSize {
			return // 不压缩
		}

		// 检查 MIME 类型（零拷贝使用 []byte）
		contentType := ctx.Response.Header.ContentType()
		if !m.isCompressible(contentType) {
			return // 不压缩此类型
		}

		// 执行压缩
		var encoding string
		if useBrotli {
			encoding = "br"
		} else if useGzip {
			encoding = compressionGZIP
		}

		if bodyLen > streamingThreshold {
			// 大响应：流式压缩，消除 compressed buffer 分配
			if useBrotli {
				m.streamBrotli(ctx, encoding)
			} else if useGzip {
				m.streamGzip(ctx, encoding)
			}
		} else {
			// 小响应：缓冲压缩
			var compressed []byte

			if useBrotli {
				compressed = m.compressBrotli(body)
			} else if useGzip {
				compressed = m.compressGzip(body)
			}

			if len(compressed) > 0 && len(compressed) < bodyLen {
				ctx.Response.SetBody(compressed)
				ctx.Response.Header.Set("Content-Encoding", encoding)
				ctx.Response.Header.Del("Content-Length")
			}
		}
	}
}

// isCompressible 检查 MIME 类型是否可压缩。
//
// 参数：
//   - contentType: 内容类型（MIME 类型）[]
//
// 返回值：
//   - bool: 是否可压缩
func (m *Middleware) isCompressible(contentType []byte) bool {
	// 移除 charset 等参数
	ct := contentType
	if idx := bytes.IndexByte(ct, ';'); idx >= 0 {
		ct = ct[:idx]
	}
	ct = bytes.TrimSpace(ct)

	for _, t := range m.types {
		if bytes.Equal(bytes.ToLower([]byte(t)), ct) {
			return true
		}
		// 支持通配符匹配
		if base, found := strings.CutSuffix(t, "/*"); found {
			if bytes.HasPrefix(ct, []byte(base)) {
				return true
			}
		}
	}
	return false
}

// compressGzip 使用 gzip 压缩数据。
//
// 参数：
//   - data: 待压缩的原始数据
//
// 返回值：
//   - []byte: 压缩后的数据
func (m *Middleware) compressGzip(data []byte) []byte {
	w, ok := m.gzipPool.Get().(*gzip.Writer)
	if !ok {
		return data // fallback to uncompressed
	}
	defer m.gzipPool.Put(w)

	var buf bytes.Buffer
	w.Reset(&buf)
	if _, err := w.Write(data); err != nil { //nolint:staticcheck // intentionally empty branch
		// 忽略写入错误，缓冲到 bytes.Buffer 时不太可能失败
	}
	_ = w.Close()

	return buf.Bytes()
}

// compressBrotli 使用 brotli 压缩数据。
//
// 参数：
//   - data: 待压缩的原始数据
//
// 返回值：
//   - []byte: 压缩后的数据
func (m *Middleware) compressBrotli(data []byte) []byte {
	w, ok := m.brotliPool.Get().(*brotli.Writer)
	if !ok {
		return data // fallback to uncompressed
	}
	defer m.brotliPool.Put(w)

	var buf bytes.Buffer
	w.Reset(&buf)
	if _, err := w.Write(data); err != nil { //nolint:staticcheck // intentionally empty branch
		// 忽略写入错误，缓冲到 bytes.Buffer 时不太可能失败
	}
	_ = w.Close()

	return buf.Bytes()
}

// Types 返回可压缩的 MIME 类型列表。
//
// 返回值：
//   - []string: 可压缩的 MIME 类型列表
func (m *Middleware) Types() []string {
	return m.types
}

// Level 返回压缩级别。
//
// 返回值：
//   - int: 压缩级别（1-9）
func (m *Middleware) Level() int {
	return m.level
}

// MinSize 返回最小压缩大小。
//
// 返回值：
//   - int: 最小压缩大小（字节）
func (m *Middleware) MinSize() int {
	return m.minSize
}

// streamGzip 使用 gzip 流式压缩。
//
// 通过 SetBodyStreamWriter 将压缩数据直接写入响应流，
// 消除 compressed buffer 分配，降低内存峰值。
//
// 参数：
//   - ctx: fasthttp 请求上下文
//   - encoding: Content-Encoding 值（"gzip"）
func (m *Middleware) streamGzip(ctx *fasthttp.RequestCtx, encoding string) {
	ctx.Response.Header.Set("Content-Encoding", encoding)
	ctx.Response.Header.Del("Content-Length") // 使用 chunked encoding

	body := ctx.Response.Body()
	ctx.SetBodyStreamWriter(func(w *bufio.Writer) {
		writer, ok := m.gzipPool.Get().(*gzip.Writer)
		if !ok {
			// pool 获取失败，直接写原始 body
			_, _ = w.Write(body)
			_ = w.Flush()
			return
		}
		defer m.gzipPool.Put(writer)

		writer.Reset(w)
		_, _ = writer.Write(body)
		_ = writer.Close()
		_ = w.Flush()
	})
}

// streamBrotli 使用 brotli 流式压缩。
//
// 通过 SetBodyStreamWriter 将压缩数据直接写入响应流，
// 消除 compressed buffer 分配，降低内存峰值。
//
// 参数：
//   - ctx: fasthttp 请求上下文
//   - encoding: Content-Encoding 值（"br"）
func (m *Middleware) streamBrotli(ctx *fasthttp.RequestCtx, encoding string) {
	ctx.Response.Header.Set("Content-Encoding", encoding)
	ctx.Response.Header.Del("Content-Length") // 使用 chunked encoding

	body := ctx.Response.Body()
	ctx.SetBodyStreamWriter(func(w *bufio.Writer) {
		writer, ok := m.brotliPool.Get().(*brotli.Writer)
		if !ok {
			// pool 获取失败，直接写原始 body
			_, _ = w.Write(body)
			_ = w.Flush()
			return
		}
		defer m.brotliPool.Put(writer)

		writer.Reset(w)
		_, _ = writer.Write(body)
		_ = writer.Close()
		_ = w.Flush()
	})
}
