//go:build linux

// Package handler 提供 HTTP 请求处理器，包括路由、静态文件服务和零拷贝传输。
//
// 该文件包含 Linux 平台完整的 sendfile 实现（零拷贝 + 公共函数）。
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

	// Linux 平台使用 sendfile 系统调用
	err := linuxSendfile(conn, file.Fd(), offset, length)
	if err != nil {
		// sendfile 失败，fallback 到 io.Copy
		return copyFile(ctx, file, offset, length)
	}

	return nil
}

// getNetConn 从 fasthttp.RequestCtx 获取底层 net.Conn。
func getNetConn(ctx *fasthttp.RequestCtx) net.Conn {
	return ctx.Conn()
}

// copyFile 普通文件拷贝（fallback）。
func copyFile(ctx *fasthttp.RequestCtx, file *os.File, offset, length int64) error {
	if offset > 0 {
		if _, err := file.Seek(offset, io.SeekStart); err != nil {
			return err
		}
	}

	if length > 0 {
		_, err := io.CopyN(ctx, file, length)
		return err
	}

	_, err := io.Copy(ctx, file)
	return err
}

// linuxSendfile Linux sendfile 系统调用。
//
// 使用 Linux 特有的 sendfile 系统调用实现零拷贝传输。
func linuxSendfile(conn net.Conn, fileFd uintptr, _, length int64) error {
	socketFd, err := getSocketFd(conn)
	if err != nil {
		return err
	}

	// Linux sendfile: sendfile(out_fd, in_fd, offset, count)
	var sent int64
	remain := length

	for remain > 0 {
		n, err := syscall.Sendfile(int(socketFd), int(fileFd), nil, int(remain))
		if err != nil {
			return err
		}
		if n == 0 {
			break // EOF
		}
		sent += int64(n)
		remain -= int64(n)
	}

	return nil
}

// getSocketFd 获取 socket 文件描述符。
//
// 从网络连接中提取底层的文件描述符，用于 sendfile 系统调用。
func getSocketFd(conn net.Conn) (uintptr, error) {
	switch c := conn.(type) {
	case *net.TCPConn:
		file, err := c.File()
		if err != nil {
			return 0, err
		}
		defer func() { _ = file.Close() }()
		return file.Fd(), nil
	case *net.UnixConn:
		file, err := c.File()
		if err != nil {
			return 0, err
		}
		defer func() { _ = file.Close() }()
		return file.Fd(), nil
	default:
		return 0, syscall.ENOTSUP
	}
}
