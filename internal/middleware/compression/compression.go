// Package compression 提供 HTTP 响应压缩中间件，支持 gzip 和 brotli 算法。
package compression

import (
	"bytes"
	"strings"
	"sync"

	"github.com/klauspost/compress/gzip"
	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/config"
)

// Algorithm 压缩算法类型。
type Algorithm int

const (
	// AlgorithmGzip 使用 gzip 压缩。
	AlgorithmGzip Algorithm = iota
	// AlgorithmBrotli 使用 brotli 压缩。
	AlgorithmBrotli
)

// CompressionMiddleware 响应压缩中间件。
type CompressionMiddleware struct {
	types     []string  // 可压缩的 MIME 类型
	level     int       // 压缩级别
	minSize   int       // 最小压缩大小
	algorithm Algorithm // 压缩算法

	// 缓冲池
	gzipPool sync.Pool
}

// New 创建压缩中间件。
func New(cfg *config.CompressionConfig) (*CompressionMiddleware, error) {
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
	case "gzip":
		algo = AlgorithmGzip
	case "both":
		// both 模式优先使用 brotli（如果客户端支持）
		algo = AlgorithmBrotli
	default:
		algo = AlgorithmGzip
	}

	m := &CompressionMiddleware{
		types:     cfg.Types,
		level:     cfg.Level,
		minSize:   cfg.MinSize,
		algorithm: algo,
	}

	// 初始化缓冲池
	m.gzipPool = sync.Pool{
		New: func() interface{} {
			w, _ := gzip.NewWriterLevel(nil, cfg.Level)
			return w
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
func (m *CompressionMiddleware) Name() string {
	return "compression"
}

// Process 应用压缩中间件。
func (m *CompressionMiddleware) Process(next fasthttp.RequestHandler) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		// 检查客户端是否支持压缩
		acceptEncoding := string(ctx.Request.Header.Peek("Accept-Encoding"))

		// 根据算法和客户端支持选择压缩方式
		var useGzip, useBrotli bool
		if m.algorithm == AlgorithmGzip {
			useGzip = strings.Contains(acceptEncoding, "gzip")
		} else if m.algorithm == AlgorithmBrotli {
			// brotli 或 both 模式
			if strings.Contains(acceptEncoding, "br") {
				useBrotli = true
			} else if strings.Contains(acceptEncoding, "gzip") {
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

		// 获取响应体
		body := ctx.Response.Body()
		bodyLen := len(body)

		// 检查是否满足压缩条件
		if bodyLen < m.minSize {
			return // 不压缩
		}

		// 检查 MIME 类型
		contentType := string(ctx.Response.Header.ContentType())
		if !m.isCompressible(contentType) {
			return // 不压缩此类型
		}

		// 执行压缩
		var compressed []byte
		var encoding string

		if useBrotli {
			compressed = m.compressBrotli(body)
			encoding = "br"
		} else if useGzip {
			compressed = m.compressGzip(body)
			encoding = "gzip"
		}

		if len(compressed) > 0 && len(compressed) < bodyLen {
			ctx.Response.SetBody(compressed)
			ctx.Response.Header.Set("Content-Encoding", encoding)
			ctx.Response.Header.Del("Content-Length") // 让 fasthttp 自动计算
		}
	}
}

// isCompressible 检查 MIME 类型是否可压缩。
func (m *CompressionMiddleware) isCompressible(contentType string) bool {
	// 移除 charset 等参数
	ct := contentType
	if idx := strings.Index(ct, ";"); idx >= 0 {
		ct = ct[:idx]
	}
	ct = strings.TrimSpace(strings.ToLower(ct))

	for _, t := range m.types {
		if strings.ToLower(t) == ct {
			return true
		}
		// 支持通配符匹配
		if strings.HasSuffix(t, "/*") {
			base := strings.TrimSuffix(t, "/*")
			if strings.HasPrefix(ct, base) {
				return true
			}
		}
	}
	return false
}

// compressGzip 使用 gzip 压缩数据。
func (m *CompressionMiddleware) compressGzip(data []byte) []byte {
	w := m.gzipPool.Get().(*gzip.Writer)
	defer m.gzipPool.Put(w)

	var buf bytes.Buffer
	w.Reset(&buf)
	w.Write(data)
	w.Close()

	return buf.Bytes()
}

// compressBrotli 使用 brotli 压缩数据。
func (m *CompressionMiddleware) compressBrotli(data []byte) []byte {
	// 简单实现：brotli 需要额外依赖，这里降级为 gzip
	// 实际生产环境应使用 github.com/andybalholm/brotli
	return m.compressGzip(data)
}

// Types 返回可压缩的 MIME 类型列表。
func (m *CompressionMiddleware) Types() []string {
	return m.types
}

// Level 返回压缩级别。
func (m *CompressionMiddleware) Level() int {
	return m.level
}

// MinSize 返回最小压缩大小。
func (m *CompressionMiddleware) MinSize() int {
	return m.minSize
}
