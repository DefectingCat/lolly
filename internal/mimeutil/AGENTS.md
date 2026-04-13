<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-04-13 | Updated: 2026-04-13 -->

# mimeutil

## Purpose
MIME 类型检测工具包，提供文件内容类型检测功能，补充 Go 标准库缺失或错误的 MIME 类型映射。

## Key Files

| File | Description |
|------|-------------|
| `detect.go` | MIME 类型检测函数和扩展名映射表 |
| `detect_test.go` | 检测函数单元测试 |

## For AI Agents

### Working In This Directory
- `DetectContentType(filePath)` 根据扩展名返回 MIME 类型
- 使用包本地映射而非全局 `mime.AddExtensionType`，避免副作用
- 未知类型返回空字符串，调用方应使用默认值

### Testing Requirements
- 运行测试：`go test ./internal/mimeutil/...`
- 测试覆盖扩展名大小写处理和覆盖映射

### Common Patterns
```go
contentType := mimeutil.DetectContentType(filePath)
if contentType == "" {
    contentType = "application/octet-stream"
}
```

## Dependencies

### External
- `mime` - Go 标准库 MIME 类型检测
- `path/filepath` - 扩展名提取
- `strings` - 大小写处理

<!-- MANUAL: -->