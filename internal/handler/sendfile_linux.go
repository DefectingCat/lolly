//go:build linux

// Package handler 提供 HTTP 请求处理器，包括路由、静态文件服务和零拷贝传输。
//
// 该文件包含 Linux 平台完整的 sendfile 实现（零拷贝）。
//
// 作者：xfy
package handler

import (
	"net"
	"os"
	"syscall"
	"time"

	"github.com/valyala/fasthttp"
)

const (
	// sendfileMaxRetries sendfile 系统调用最大重试次数。
	// 用于处理 EAGAIN/EWOULDBLOCK 等临时性错误。
	sendfileMaxRetries = 100

	// sendfileRetryDelay sendfile 重试间隔等待时间。
	// 短暂等待以允许 socket 缓冲区恢复。
	sendfileRetryDelay = 1 * time.Millisecond
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
		// EPIPE/ECONNRESET 表示客户端已断开，不应 fallback
		// 因为 HTTP 头可能已发送，fallback 会造成响应混乱
		if err == syscall.EPIPE || err == syscall.ECONNRESET {
			return err // 直接返回错误，不 fallback
		}
		// 其他错误尝试 fallback 到 io.Copy
		return copyFile(ctx, file, offset, length)
	}

	return nil
}

// linuxSendfile Linux sendfile 系统调用。
//
// 使用 Linux 特有的 sendfile 系统调用实现零拷贝传输。
// 正确处理临时错误（EAGAIN、EINTR）和连接断开（EPIPE、ECONNRESET）。
//
// 参数：
//   - conn: 目标网络连接（必须是 TCPConn 或 UnixConn）
//   - fileFd: 源文件的文件描述符
//   - offset: 文件起始偏移量（未使用，由内核自动处理）
//   - length: 传输长度（字节）
//
// 返回值：
//   - error: 传输过程中的错误，nil 表示成功
func linuxSendfile(conn net.Conn, fileFd uintptr, _, length int64) error {
	socketFd, err := getSocketFd(conn)
	if err != nil {
		return err
	}

	// Linux sendfile: sendfile(out_fd, in_fd, offset, count)
	var sent int64
	remain := length
	retries := 0

	for remain > 0 {
		n, err := syscall.Sendfile(int(socketFd), int(fileFd), nil, int(remain))
		if err != nil {
			// 处理临时错误：socket 缓冲区满，等待后重试
			if err == syscall.EAGAIN || err == syscall.EWOULDBLOCK {
				retries++
				if retries > sendfileMaxRetries {
					// 超过最大重试次数，返回错误
					return err
				}
				// socket 缓冲区满，短暂等待后重试
				time.Sleep(sendfileRetryDelay)
				continue
			}

			// 被信号中断，重试
			if err == syscall.EINTR {
				retries++
				if retries > sendfileMaxRetries {
					return err
				}
				continue
			}

			// 客户端断开连接，返回错误让 fasthttp 知道请求未完成
			// 注意：不要返回 nil，否则 fasthttp 会发送 200 + 空 body
			if err == syscall.EPIPE || err == syscall.ECONNRESET {
				return err // 返回错误，让 fasthttp 处理连接断开
			}

			// 其他错误直接返回
			return err
		}

		if n == 0 {
			break // EOF 或连接关闭
		}

		// 成功发送数据，重置重试计数
		retries = 0
		sent += int64(n)
		remain -= int64(n)
	}

	return nil
}

// getSocketFd 获取 socket 文件描述符。
//
// 从网络连接中提取底层的文件描述符，用于 sendfile 系统调用。
// 支持 TCPConn 和 UnixConn 两种连接类型。
//
// 参数：
//   - conn: 网络连接对象
//
// 返回值：
//   - uintptr: socket 文件描述符，失败时返回 0
//   - error: 获取失败时的错误，不支持的连接类型返回 ENOTSUP
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