//go:build !linux

// Package handler 提供 HTTP 请求处理器，包括路由、静态文件服务和零拷贝传输。
//
// 该文件包含非 Linux 平台的 sendfile 实现（使用 fallback 方式）。
//
// 作者：xfy
package handler

import (
	"io"
	"net"
	"os"
	"syscall"

	"github.com/valyala/fasthttp"
)

const (
	// MinSendfileSize 使用 sendfile 的最小文件大小（8KB）。
	// 小于该值的文件使用普通 io.Copy，避免系统调用开销。
	MinSendfileSize = 8 * 1024
)

// SendFile 零拷贝文件传输。
//
// 大文件使用系统调用直接从文件传输到 socket，避免用户空间拷贝，
// 从而减少 CPU 和内存开销，提升传输性能。
//
// 参数：
//   - ctx: fasthttp 请求上下文，用于获取底层连接
//   - file: 要传输的文件对象
//   - offset: 文件起始偏移量（字节）
//   - length: 传输长度（字节），-1 表示传输到文件末尾
//
// 返回值：
//   - error: 传输过程中的错误
//
// 注意事项：
//   - 小于 8KB 的文件使用普通 io.Copy
//   - 非 Linux 平台（macOS、Windows）使用 fallback 方式
//
// 使用示例：
//
//	file, _ := os.Open("large_file.bin")
//	defer file.Close()
//	info, _ := file.Stat()
//	err := SendFile(ctx, file, 0, info.Size())
func SendFile(ctx *fasthttp.RequestCtx, file *os.File, offset, length int64) error {
	// 小文件使用普通 io.Copy
	if length < MinSendfileSize {
		return copyFile(ctx, file, offset, length)
	}

	// 尝试获取 socket 文件描述符
	conn := getNetConn(ctx)
	if conn == nil {
		return copyFile(ctx, file, offset, length)
	}

	// 非 Linux 平台使用 fallback
	err := platformSendfile(conn, file, offset, length)
	if err != nil {
		// sendfile 失败，fallback 到 io.Copy
		return copyFile(ctx, file, offset, length)
	}

	return nil
}

// getNetConn 从 fasthttp.RequestCtx 获取底层 net.Conn。
//
// 参数：
//   - ctx: fasthttp 请求上下文
//
// 返回值：
//   - net.Conn: 底层网络连接，如果无法获取则返回 nil
func getNetConn(ctx *fasthttp.RequestCtx) net.Conn {
	// fasthttp 内部使用 net.Conn，通过接口获取
	return ctx.Conn()
}

// copyFile 普通文件拷贝（fallback）。
//
// 使用 io.Copy 进行文件传输，适用于不支持 sendfile 的平台或小文件。
//
// 参数：
//   - ctx: fasthttp 请求上下文，作为写入目标
//   - file: 源文件对象
//   - offset: 文件起始偏移量
//   - length: 传输长度，0 表示拷贝到文件末尾
//
// 返回值：
//   - error: 拷贝过程中的错误
func copyFile(ctx *fasthttp.RequestCtx, file *os.File, offset, length int64) error {
	if offset > 0 {
		if _, err := file.Seek(offset, io.SeekStart); err != nil {
			return err
		}
	}

	// 使用 io.CopyN 或 io.Copy
	if length > 0 {
		_, err := io.CopyN(ctx, file, length)
		return err
	}

	_, err := io.Copy(ctx, file)
	return err
}

// platformSendfile 非 Linux 平台的 sendfile 实现。
//
// macOS 和 Windows 不支持 sendfile 系统调用，返回 ENOTSUP 触发 fallback。
//
// 参数：
//   - conn: 目标网络连接
//   - file: 源文件对象
//   - offset: 文件起始偏移量
//   - length: 传输长度
//
// 返回值：
//   - error: 始终返回 ENOTSUP，表示不支持
func platformSendfile(conn net.Conn, file *os.File, offset, length int64) error {
	// macOS sendfile 签名复杂，简化使用 fallback
	// Windows TransmitFile 需要特殊 API
	return syscall.ENOTSUP
}
