// Package proxy 提供 WebSocket 代理模块的性能基准测试。
//
// 该文件测试 WebSocket 代理模块各项操作的性能，包括：
//   - 握手升级请求构建性能
//   - 不同帧大小的数据转发性能
//   - Ping/Pong 心跳往返延迟
//   - 并发连接吞吐量
//
// 注意：WebSocket 代理在 TCP 层进行透明转发，不解析 WebSocket 帧协议。
// 帧编码/解码开销由底层 TCP 数据传输的编解码开销代表。
//
// 作者：xfy
package proxy

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/valyala/fasthttp"
)

// BenchmarkWebSocketHandshake 基准测试 WebSocket 握手升级请求构建性能。
//
// 测试 buildWebSocketUpgradeRequest() 函数的开销，包括：
//   - 路径和查询参数处理
//   - 请求头复制（Upgrade、Connection、Sec-WebSocket-*）
//   - X-Forwarded 代理头注入
//
// 模拟真实客户端请求，包含常见的 WebSocket 握手头。
func BenchmarkWebSocketHandshake(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.SetRequestURI("/ws?token=abc123&channel=default")
		ctx.Request.Header.SetHost("client.example.com")
		ctx.Request.Header.Set("Upgrade", "websocket")
		ctx.Request.Header.Set("Connection", "Upgrade")
		ctx.Request.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
		ctx.Request.Header.Set("Sec-WebSocket-Version", "13")
		ctx.Request.Header.Set("Sec-WebSocket-Protocol", "chat, superchat")
		ctx.Request.Header.Set("Sec-WebSocket-Extensions", "permessage-deflate")
		ctx.Request.Header.Set("Origin", "https://example.com")

		result := buildWebSocketUpgradeRequest(ctx, "backend.example.com:8080")

		// 验证握手请求包含关键头
		if !strings.Contains(result, "Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==") {
			b.Fatal("handshake missing Sec-WebSocket-Key")
		}
		if !strings.Contains(result, "Upgrade: websocket") {
			b.Fatal("handshake missing Upgrade header")
		}
	}
}

// BenchmarkWebSocketFrameSmall 基准测试小帧（<126 bytes）发送接收性能。
//
// 测试 copyData() 函数处理小数据块的开销。小帧对应 WebSocket
// 协议中 length < 126 的场景，帧头仅需 2 bytes（无 extended payload）。
// 这是最常见的 WebSocket 帧大小（聊天消息、心跳、状态更新等）。
func BenchmarkWebSocketFrameSmall(b *testing.B) {
	smallData := make([]byte, 100) // 100 bytes < 126
	for i := range smallData {
		smallData[i] = byte(i % 256)
	}

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			src1, src2 := net.Pipe()
			dst1, dst2 := net.Pipe()

			bridge := &WebSocketBridge{}

			// 启动数据复制
			done := make(chan struct{})
			go func() {
				_ = bridge.copyData(dst1, src1, "bench-small")
				close(done)
			}()

			// 发送数据
			go func() {
				_, _ = src2.Write(smallData)
				_ = src2.Close()
			}()

			// 读取数据
			buf := make([]byte, len(smallData))
			n, err := dst2.Read(buf)
			if err != nil {
				b.Fatalf("read error: %v", err)
			}
			if n != len(smallData) {
				b.Fatalf("expected %d bytes, got %d", len(smallData), n)
			}

			// 清理
			_ = dst1.Close()
			_ = dst2.Close()
			<-done
		}
	})
}

// BenchmarkWebSocketFrameMedium 基准测试中等帧（126-65535 bytes）发送接收性能。
//
// 测试 copyData() 函数处理中等数据块的开销。中等帧对应 WebSocket
// 协议中 126 <= length < 65536 的场景，帧头需要 4 bytes
// （2 bytes 基础头 + 2 bytes extended payload length）。
// 典型场景：JSON  payloads、小型文件传输、表单数据。
func BenchmarkWebSocketFrameMedium(b *testing.B) {
	mediumData := make([]byte, 32*1024) // 32KB，典型中等帧大小
	for i := range mediumData {
		mediumData[i] = byte(i % 256)
	}

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			src1, src2 := net.Pipe()
			dst1, dst2 := net.Pipe()

			bridge := &WebSocketBridge{}

			done := make(chan struct{})
			go func() {
				_ = bridge.copyData(dst1, src1, "bench-medium")
				close(done)
			}()

			go func() {
				_, _ = src2.Write(mediumData)
				_ = src2.Close()
			}()

			// 循环读取完整数据（可能分多次 Read）
			received := 0
			buf := make([]byte, len(mediumData))
			for received < len(mediumData) {
				n, err := dst2.Read(buf[received:])
				if err != nil {
					b.Fatalf("read error at offset %d: %v", received, err)
				}
				received += n
			}
			if received != len(mediumData) {
				b.Fatalf("expected %d bytes, got %d", len(mediumData), received)
			}

			_ = dst1.Close()
			_ = dst2.Close()
			<-done
		}
	})
}

// BenchmarkWebSocketFrameLarge 基准测试大帧（>65535 bytes）发送接收性能。
//
// 测试 copyData() 函数处理大数据块的开销。大帧对应 WebSocket
// 协议中 length >= 65536 的场景，帧头需要 10 bytes
// （2 bytes 基础头 + 8 bytes extended payload length）。
// 典型场景：文件传输、视频帧、大数据集推送。
//
// 使用 32KB 内部缓冲区，大帧需要多次 Read/Write 循环。
func BenchmarkWebSocketFrameLarge(b *testing.B) {
	largeData := make([]byte, 128*1024) // 128KB > 65535
	for i := range largeData {
		largeData[i] = byte(i % 256)
	}

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			src1, src2 := net.Pipe()
			dst1, dst2 := net.Pipe()

			bridge := &WebSocketBridge{}

			done := make(chan struct{})
			go func() {
				_ = bridge.copyData(dst1, src1, "bench-large")
				close(done)
			}()

			go func() {
				_, _ = src2.Write(largeData)
				_ = src2.Close()
			}()

			// 循环读取完整数据
			received := 0
			buf := make([]byte, len(largeData))
			for received < len(largeData) {
				n, err := dst2.Read(buf[received:])
				if err != nil {
					b.Fatalf("read error at offset %d: %v", received, err)
				}
				received += n
			}
			if received != len(largeData) {
				b.Fatalf("expected %d bytes, got %d", len(largeData), received)
			}

			_ = dst1.Close()
			_ = dst2.Close()
			<-done
		}
	})
}

// BenchmarkWebSocketPingPong 基准测试 Ping/Pong 心跳往返性能。
//
// WebSocket 代理在 TCP 层透明转发，Ping/Pong 控制帧由底层
// TCP 连接直接传递。本测试模拟 Ping/Pong 的往返延迟：
//   - 发送小数据包（Ping 帧，通常 0-125 bytes payload）
//   - 等待对端接收并返回（Pong 帧）
//   - 测量完整往返时间
//
// 使用 16-byte 模拟 Ping payload，这是最常见的心跳大小。
func BenchmarkWebSocketPingPong(b *testing.B) {
	pingPayload := []byte("ping-1234567890") // 16 bytes，模拟 Ping 帧

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			client1, client2 := net.Pipe()
			target1, target2 := net.Pipe()

			// 启动双向桥接
			bridge := NewWebSocketBridge(client1, target1)
			bridgeDone := make(chan struct{})
			go func() {
				_ = bridge.Bridge()
				close(bridgeDone)
			}()

			// 客户端发送 Ping
			go func() {
				_, _ = client2.Write(pingPayload)
			}()

			// 后端读取 Ping 并回传 Pong
			buf := make([]byte, len(pingPayload))
			n, err := target2.Read(buf)
			if err != nil {
				b.Fatalf("target read error: %v", err)
			}
			if n != len(pingPayload) {
				b.Fatalf("expected %d bytes, got %d", len(pingPayload), n)
			}

			// 后端回传 Pong
			go func() {
				_, _ = target2.Write(buf[:n])
			}()

			// 客户端读取 Pong
			pongBuf := make([]byte, len(pingPayload))
			n, err = client2.Read(pongBuf)
			if err != nil {
				b.Fatalf("client read error: %v", err)
			}
			if n != len(pingPayload) {
				b.Fatalf("expected %d bytes Pong, got %d", len(pingPayload), n)
			}

			// 清理
			_ = client2.Close()
			_ = target2.Close()
			<-bridgeDone
		}
	})
}

// BenchmarkWebSocketConcurrent 基准测试并发连接吞吐量。
//
// 模拟多个 WebSocket 连接同时进行双向数据转发的场景。
// 测试 Bridge() 函数在高并发下的性能表现，包括：
//   - 多个 goroutine 同时读写
//   - sync.Mutex 保护关闭状态的开销
//   - 双向数据转发的总吞吐量
//
// 每个连接发送 4KB 数据，模拟典型的 WebSocket 消息大小。
func BenchmarkWebSocketConcurrent(b *testing.B) {
	messageSize := 4 * 1024 // 4KB 典型消息大小
	message := make([]byte, messageSize)
	for i := range message {
		message[i] = byte(i % 256)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		client1, client2 := net.Pipe()
		target1, target2 := net.Pipe()

		bridge := NewWebSocketBridge(client1, target1)

		// 启动桥接
		bridgeDone := make(chan struct{})
		go func() {
			_ = bridge.Bridge()
			close(bridgeDone)
		}()

		var wg sync.WaitGroup
		wg.Add(2)

		// 客户端 -> 后端
		go func() {
			defer wg.Done()
			_, _ = client2.Write(message)
		}()

		// 后端读取客户端数据
		go func() {
			defer wg.Done()
			buf := make([]byte, messageSize)
			received := 0
			for received < messageSize {
				n, err := target2.Read(buf[received:])
				if err != nil {
					break
				}
				received += n
			}
		}()

		wg.Wait()

		// 清理
		_ = client2.Close()
		_ = target2.Close()
		<-bridgeDone
	}
}

// BenchmarkWebSocketWriteUpgradeResponse 基准测试升级响应写入性能。
//
// 测试 writeUpgradeResponse() 函数将 HTTP 101 响应写回客户端的开销。
// 包含响应行构建、头格式化和单次 Write 调用。
func BenchmarkWebSocketWriteUpgradeResponse(b *testing.B) {
	resp := &http.Response{
		ProtoMajor: 1,
		ProtoMinor: 1,
		Status:     "101 Switching Protocols",
		StatusCode: 101,
		Header: http.Header{
			"Upgrade":              []string{"websocket"},
			"Connection":           []string{"Upgrade"},
			"Sec-WebSocket-Accept": []string{"s3pPLMBiTxaQ9kYGzzhZRbK+xOo="},
		},
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		conn1, conn2 := net.Pipe()

		done := make(chan error, 1)
		go func() {
			done <- writeUpgradeResponse(conn1, resp)
			_ = conn1.Close()
		}()

		// 读取响应
		buf := make([]byte, 1024)
		n, err := conn2.Read(buf)
		if err != nil {
			b.Fatalf("read error: %v", err)
		}

		response := string(buf[:n])
		if !strings.Contains(response, "101 Switching Protocols") {
			b.Fatal("response missing status line")
		}

		_ = conn2.Close()
		<-done
	}
}
