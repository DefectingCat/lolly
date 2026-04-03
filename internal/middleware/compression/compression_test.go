package compression

import (
	"bytes"
	"testing"

	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/config"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name string
		cfg  *config.CompressionConfig
	}{
		{
			name: "nil config uses defaults",
			cfg:  nil,
		},
		{
			name: "gzip config",
			cfg: &config.CompressionConfig{
				Type:  "gzip",
				Level: 6,
			},
		},
		{
			name: "brotli config",
			cfg: &config.CompressionConfig{
				Type:  "brotli",
				Level: 4,
			},
		},
		{
			name: "both config",
			cfg: &config.CompressionConfig{
				Type:  "both",
				Level: 6,
			},
		},
		{
			name: "custom types",
			cfg: &config.CompressionConfig{
				Type:  "gzip",
				Types: []string{"text/html", "application/json"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := New(tt.cfg)
			if err != nil {
				t.Errorf("New() error: %v", err)
			}
			if m == nil {
				t.Error("Expected non-nil middleware")
			}
		})
	}
}

func TestDefaultCompressibleTypes(t *testing.T) {
	types := defaultCompressibleTypes()
	if len(types) == 0 {
		t.Error("Expected non-empty default types")
	}

	// 检查关键类型
	expected := []string{"text/html", "text/css", "application/json"}
	for _, e := range expected {
		found := false
		for _, t := range types {
			if t == e {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected type %s in default list", e)
		}
	}
}

func TestIsCompressible(t *testing.T) {
	m, _ := New(&config.CompressionConfig{
		Types: []string{"text/html", "text/*", "application/json"},
	})

	tests := []struct {
		contentType string
		expected    bool
	}{
		{"text/html", true},
		{"text/html; charset=utf-8", true},
		{"text/css", true},
		{"text/plain", true},
		{"application/json", true},
		{"application/json; charset=utf-8", true},
		{"image/png", false},
		{"application/octet-stream", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.contentType, func(t *testing.T) {
			result := m.isCompressible(tt.contentType)
			if result != tt.expected {
				t.Errorf("isCompressible(%s) = %v, expected %v", tt.contentType, result, tt.expected)
			}
		})
	}
}

func TestCompressGzip(t *testing.T) {
	m, _ := New(&config.CompressionConfig{
		Type:  "gzip",
		Level: 6,
	})

	// 测试数据
	data := []byte("Hello, World! This is a test string that should be compressed.")

	compressed := m.compressGzip(data)
	if len(compressed) == 0 {
		t.Error("Expected compressed data")
	}

	// 压缩后应该更小（对于重复文本）
	if len(compressed) >= len(data) {
		t.Logf("Warning: compressed size %d >= original %d", len(compressed), len(data))
	}
}

func TestCompressBrotli(t *testing.T) {
	m, _ := New(&config.CompressionConfig{
		Type:  "brotli",
		Level: 4,
	})

	data := []byte("Hello, World! This is a test string that should be compressed with brotli.")

	compressed := m.compressBrotli(data)
	if len(compressed) == 0 {
		t.Error("Expected compressed data")
	}
}

func TestProcessNoCompression(t *testing.T) {
	m, _ := New(&config.CompressionConfig{
		Type:    "gzip",
		MinSize: 1000, // 大阈值
	})

	nextHandler := func(ctx *fasthttp.RequestCtx) {
		ctx.Response.Header.Set("Content-Type", "text/html")
		ctx.WriteString("Short response")
	}

	handler := m.Process(nextHandler)

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.Set("Accept-Encoding", "gzip")

	handler(ctx)

	// 响应太短，不应压缩
	encoding := ctx.Response.Header.Peek("Content-Encoding")
	if string(encoding) != "" {
		t.Errorf("Expected no Content-Encoding, got %s", encoding)
	}

	body := string(ctx.Response.Body())
	if body != "Short response" {
		t.Errorf("Expected 'Short response', got %s", body)
	}
}

func TestProcessWithGzip(t *testing.T) {
	m, _ := New(&config.CompressionConfig{
		Type:    "gzip",
		Level:   6,
		MinSize: 10, // 小阈值
		Types:   []string{"text/html"},
	})

	// 创建足够长的响应
	longResponse := bytes.Repeat([]byte("Hello World! "), 100) // 1300+ bytes

	nextHandler := func(ctx *fasthttp.RequestCtx) {
		ctx.Response.Header.Set("Content-Type", "text/html")
		ctx.Write(longResponse)
	}

	handler := m.Process(nextHandler)

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.Set("Accept-Encoding", "gzip")

	handler(ctx)

	encoding := ctx.Response.Header.Peek("Content-Encoding")
	if string(encoding) != "gzip" {
		t.Errorf("Expected Content-Encoding 'gzip', got %s", encoding)
	}

	// 响应应该被压缩（更小）
	body := ctx.Response.Body()
	if len(body) >= len(longResponse) {
		t.Errorf("Expected compressed body smaller than original, got %d >= %d", len(body), len(longResponse))
	}
}

func TestProcessWithBrotli(t *testing.T) {
	m, _ := New(&config.CompressionConfig{
		Type:    "brotli",
		Level:   4,
		MinSize: 10,
		Types:   []string{"text/html"},
	})

	longResponse := bytes.Repeat([]byte("Hello World! "), 100)

	nextHandler := func(ctx *fasthttp.RequestCtx) {
		ctx.Response.Header.Set("Content-Type", "text/html")
		ctx.Write(longResponse)
	}

	handler := m.Process(nextHandler)

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.Set("Accept-Encoding", "br")

	handler(ctx)

	encoding := ctx.Response.Header.Peek("Content-Encoding")
	if string(encoding) != "br" {
		t.Errorf("Expected Content-Encoding 'br', got %s", encoding)
	}
}

func TestProcessUnsupportedEncoding(t *testing.T) {
	m, _ := New(&config.CompressionConfig{
		Type:  "gzip",
		Level: 6,
	})

	nextHandler := func(ctx *fasthttp.RequestCtx) {
		ctx.Response.Header.Set("Content-Type", "text/html")
		ctx.WriteString("Test response")
	}

	handler := m.Process(nextHandler)

	ctx := &fasthttp.RequestCtx{}
	// 不设置 Accept-Encoding 或设置为空
	ctx.Request.Header.Set("Accept-Encoding", "")

	handler(ctx)

	// 不应压缩
	encoding := ctx.Response.Header.Peek("Content-Encoding")
	if string(encoding) != "" {
		t.Errorf("Expected no Content-Encoding, got %s", encoding)
	}
}

func TestProcessNonCompressibleType(t *testing.T) {
	m, _ := New(&config.CompressionConfig{
		Type:    "gzip",
		MinSize: 10,
	})

	longResponse := bytes.Repeat([]byte("data"), 1000)

	nextHandler := func(ctx *fasthttp.RequestCtx) {
		ctx.Response.Header.Set("Content-Type", "image/png")
		ctx.Write(longResponse)
	}

	handler := m.Process(nextHandler)

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.Set("Accept-Encoding", "gzip")

	handler(ctx)

	// 图片不应压缩
	encoding := ctx.Response.Header.Peek("Content-Encoding")
	if string(encoding) != "" {
		t.Errorf("Expected no Content-Encoding for image, got %s", encoding)
	}
}

func TestName(t *testing.T) {
	m, _ := New(nil)
	if m.Name() != "compression" {
		t.Errorf("Expected name 'compression', got %s", m.Name())
	}
}

func TestGetters(t *testing.T) {
	cfg := &config.CompressionConfig{
		Type:    "gzip",
		Level:   5,
		MinSize: 500,
		Types:   []string{"text/html"},
	}
	m, _ := New(cfg)

	if m.Level() != 5 {
		t.Errorf("Expected Level 5, got %d", m.Level())
	}
	if m.MinSize() != 500 {
		t.Errorf("Expected MinSize 500, got %d", m.MinSize())
	}
	if len(m.Types()) != 1 {
		t.Errorf("Expected 1 type, got %d", len(m.Types()))
	}
}
