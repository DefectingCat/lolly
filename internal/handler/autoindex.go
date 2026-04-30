// Package handler 提供 HTTP 请求处理功能。
//
// 该文件实现目录列表（autoindex）功能，类似 nginx 的 autoindex 模块。
// 支持三种输出格式：HTML、JSON、XML。
//
// 作者：xfy
package handler

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/valyala/fasthttp"
)

// AutoIndexConfig 目录列表配置。
type AutoIndexConfig struct {
	Format    string // 输出格式：html、json、xml
	Localtime bool   // 使用本地时间（默认 GMT）
	ExactSize bool   // 精确大小（默认人类可读）
}

// dirEntry 目录条目信息。
type dirEntry struct {
	Name    string
	IsDir   bool
	Size    int64
	ModTime time.Time
}

// GenerateAutoIndex 生成目录列表响应。
//
// 参数：
//   - ctx: fasthttp 请求上下文
//   - dirPath: 目录路径
//   - reqPath: 请求 URI 路径
//   - config: 配置选项
//
// 返回值：
//   - bool: 是否成功生成响应
func GenerateAutoIndex(ctx *fasthttp.RequestCtx, dirPath, reqPath string, config AutoIndexConfig) bool {
	// 读取目录
	entries, err := readDirectory(dirPath)
	if err != nil {
		return false
	}

	// 排序：目录优先，然后按名称排序
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir // 目录排在前面
		}
		return entries[i].Name < entries[j].Name
	})

	// 根据格式生成响应
	switch config.Format {
	case "json":
		generateJSONIndex(ctx, reqPath, entries)
	case "xml":
		generateXMLIndex(ctx, reqPath, entries)
	default:
		generateHTMLIndex(ctx, reqPath, entries, config)
	}

	return true
}

// readDirectory 读取目录内容。
func readDirectory(dirPath string) ([]dirEntry, error) {
	dir, err := os.Open(dirPath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = dir.Close() }()

	infos, err := dir.Readdir(-1)
	if err != nil {
		return nil, err
	}

	entries := make([]dirEntry, 0, len(infos))
	for _, info := range infos {
		name := info.Name()
		// 跳过隐藏文件（以 . 开头）
		if strings.HasPrefix(name, ".") {
			continue
		}
		entries = append(entries, dirEntry{
			Name:    name,
			IsDir:   info.IsDir(),
			Size:    info.Size(),
			ModTime: info.ModTime(),
		})
	}

	return entries, nil
}

// generateHTMLIndex 生成 HTML 格式的目录列表。
func generateHTMLIndex(ctx *fasthttp.RequestCtx, reqPath string, entries []dirEntry, config AutoIndexConfig) {
	var buf bytes.Buffer

	// 确保路径以 / 结尾
	if !strings.HasSuffix(reqPath, "/") {
		reqPath += "/"
	}

	// HTML 头部
	buf.WriteString("<!DOCTYPE html>\n")
	buf.WriteString("<html>\n<head>\n")
	fmt.Fprintf(&buf, "<title>Index of %s</title>\n", html.EscapeString(reqPath))
	buf.WriteString("<style>\n")
	buf.WriteString("body { font-family: monospace; margin: 20px; }\n")
	buf.WriteString("h1 { border-bottom: 1px solid #ccc; padding-bottom: 10px; }\n")
	buf.WriteString("table { border-collapse: collapse; width: 100%; }\n")
	buf.WriteString("td, th { padding: 5px 10px; text-align: left; }\n")
	buf.WriteString("td.size { text-align: right; }\n")
	buf.WriteString("a { text-decoration: none; }\n")
	buf.WriteString("a:hover { text-decoration: underline; }\n")
	buf.WriteString("</style>\n")
	buf.WriteString("</head>\n<body>\n")
	fmt.Fprintf(&buf, "<h1>Index of %s</h1>\n", html.EscapeString(reqPath))
	buf.WriteString("<hr>\n<table>\n")
	buf.WriteString("<thead><tr><th>Name</th><th>Modified</th><th>Size</th></tr></thead>\n")
	buf.WriteString("<tbody>\n")

	// 父目录链接
	if reqPath != "/" {
		buf.WriteString("<tr><td><a href=\"../\">../</a></td><td>-</td><td>-</td></tr>\n")
	}

	// 目录条目
	for _, entry := range entries {
		name := entry.Name
		displayName := name
		href := url.PathEscape(name)

		if entry.IsDir {
			displayName += "/"
			href += "/"
		}

		// 时间格式
		var timeStr string
		if config.Localtime {
			timeStr = entry.ModTime.Local().Format("02-Jan-2006 15:04")
		} else {
			timeStr = entry.ModTime.UTC().Format("02-Jan-2006 15:04")
		}

		// 大小格式
		var sizeStr string
		if entry.IsDir {
			sizeStr = "-"
		} else if config.ExactSize {
			sizeStr = fmt.Sprintf("%d", entry.Size)
		} else {
			sizeStr = formatSize(entry.Size)
		}

		fmt.Fprintf(&buf, "<tr><td><a href=\"%s\">%s</a></td><td>%s</td><td class=\"size\">%s</td></tr>\n",
			href, html.EscapeString(displayName), timeStr, sizeStr)
	}

	buf.WriteString("</tbody>\n</table>\n<hr>\n</body>\n</html>\n")

	ctx.Response.Header.Set("Content-Security-Policy", "default-src 'self'")
	ctx.Response.Header.SetContentType("text/html; charset=utf-8")
	ctx.Response.SetBody(buf.Bytes())
}

// generateJSONIndex 生成 JSON 格式的目录列表。
func generateJSONIndex(ctx *fasthttp.RequestCtx, _ string, entries []dirEntry) {
	type jsonEntry struct {
		Name  string `json:"name"`
		Type  string `json:"type"`
		Mtime string `json:"mtime"`
		Size  int64  `json:"size,omitempty"`
	}

	jsonEntries := make([]jsonEntry, 0, len(entries))
	for _, entry := range entries {
		e := jsonEntry{
			Name:  entry.Name,
			Mtime: entry.ModTime.UTC().Format(time.RFC1123),
		}
		if entry.IsDir {
			e.Type = "directory"
		} else {
			e.Type = "file"
			e.Size = entry.Size
		}
		jsonEntries = append(jsonEntries, e)
	}

	data, err := json.MarshalIndent(jsonEntries, "", "  ")
	if err != nil {
		ctx.Response.SetStatusCode(fasthttp.StatusInternalServerError)
		return
	}

	ctx.Response.Header.SetContentType("application/json")
	ctx.Response.SetBody(data)
}

// generateXMLIndex 生成 XML 格式的目录列表。
func generateXMLIndex(ctx *fasthttp.RequestCtx, reqPath string, entries []dirEntry) {
	type xmlEntry struct {
		XMLName xml.Name `xml:"element"`
		Name    string   `xml:"name,attr"`
		Type    string   `xml:"type,attr"`
		Mtime   string   `xml:"mtime,attr"`
		Size    int64    `xml:"size,attr,omitempty"`
	}

	type xmlList struct {
		XMLName  xml.Name   `xml:"list"`
		Path     string     `xml:"path,attr"`
		Elements []xmlEntry `xml:",any"`
	}

	xmlEntries := make([]xmlEntry, 0, len(entries))
	for _, entry := range entries {
		e := xmlEntry{
			Name:  entry.Name,
			Mtime: entry.ModTime.UTC().Format(time.RFC3339),
		}
		if entry.IsDir {
			e.Type = "directory"
		} else {
			e.Type = "file"
			e.Size = entry.Size
		}
		xmlEntries = append(xmlEntries, e)
	}

	list := xmlList{
		Path:     reqPath,
		Elements: xmlEntries,
	}

	data, err := xml.MarshalIndent(list, "", "  ")
	if err != nil {
		ctx.Response.SetStatusCode(fasthttp.StatusInternalServerError)
		return
	}

	ctx.Response.Header.SetContentType("text/xml; charset=utf-8")
	ctx.Response.SetBody([]byte(xml.Header + string(data)))
}

// formatSize 格式化文件大小为人类可读格式。
func formatSize(size int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)

	switch {
	case size >= GB:
		return fmt.Sprintf("%.1fG", float64(size)/float64(GB))
	case size >= MB:
		return fmt.Sprintf("%.1fM", float64(size)/float64(MB))
	case size >= KB:
		return fmt.Sprintf("%.1fK", float64(size)/float64(KB))
	default:
		return fmt.Sprintf("%d", size)
	}
}
