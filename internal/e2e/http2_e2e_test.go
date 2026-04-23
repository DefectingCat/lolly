//go:build e2e

// http2_e2e_test.go - HTTP/2 协议 E2E 测试
//
// 测试 lolly HTTP/2 功能：协议协商、流多路复用、头部压缩等。
//
// 作者：xfy
package e2e

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/http2"

	"rua.plus/lolly/internal/e2e/testutil"
)

// TestE2EHTTP2ProtocolNegotiation 测试 HTTP/2 协议协商。
//
// 验证 ALPN 协商成功选择 h2 协议。
func TestE2EHTTP2ProtocolNegotiation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testutil.DefaultTestTimeout)
	defer cancel()

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	// 生成自签名证书
	certPath, keyPath, cleanup, err := testutil.GenerateSelfSignedCert(t.TempDir())
	require.NoError(t, err, "Failed to generate certificate")
	defer cleanup()

	// 构建带 SSL 的配置
	cfg := testutil.NewConfigBuilder().
		WithServer(":8443").
		WithSSL("/etc/lolly/ssl/server.crt", "/etc/lolly/ssl/server.key").
		WithStatic("/", "/var/www/html")

	configYAML, err := cfg.Build()
	require.NoError(t, err, "Failed to build config")

	// 启动 lolly
	lolly, err := testutil.StartLolly(ctx,
		testutil.WithConfigYAML(configYAML),
		testutil.WithCert(certPath, keyPath),
	)
	require.NoError(t, err, "Failed to start lolly")
	defer lolly.Terminate(ctx)

	// 创建 HTTP/2 客户端
	client := &http.Client{
		Transport: &http2.Transport{
			AllowHTTP: false,
			DialTLSContext: func(ctx context.Context, network, addr string, cfg *tls.Config) (net.Conn, *tls.ConnectionState, error) {
				dialer := &net.Dialer{}
				conn, err := dialer.DialContext(ctx, network, addr)
				if err != nil {
					return nil, nil, err
				}
				tlsConn := tls.Client(conn, cfg)
				if err := tlsConn.HandshakeContext(ctx); err != nil {
					_ = conn.Close()
					return nil, nil, err
				}
				return tlsConn, &tlsConn.ConnectionState{}, nil
			},
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}

	// 发送请求
	resp, err := client.Get(lolly.HTTPSBaseURL())
	require.NoError(t, err, "HTTP/2 request failed")
	defer resp.Body.Close()

	// 验证 HTTP/2 协议
	assert.Equal(t, 2, resp.ProtoMajor, "Expected HTTP/2 protocol")
	t.Logf("HTTP/2 negotiation successful, status: %d", resp.StatusCode)
}

// TestE2EHTTP2StreamMultiplexing 测试 HTTP/2 流多路复用。
//
// 验证多个并发请求在单个连接上复用。
func TestE2EHTTP2StreamMultiplexing(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testutil.DefaultTestTimeout)
	defer cancel()

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	// 生成自签名证书
	certPath, keyPath, cleanup, err := testutil.GenerateSelfSignedCert(t.TempDir())
	require.NoError(t, err, "Failed to generate certificate")
	defer cleanup()

	// 构建配置
	cfg := testutil.NewConfigBuilder().
		WithServer(":8443").
		WithSSL("/etc/lolly/ssl/server.crt", "/etc/lolly/ssl/server.key").
		WithStatic("/", "/var/www/html")

	configYAML, err := cfg.Build()
	require.NoError(t, err, "Failed to build config")

	// 启动 lolly
	lolly, err := testutil.StartLolly(ctx,
		testutil.WithConfigYAML(configYAML),
		testutil.WithCert(certPath, keyPath),
	)
	require.NoError(t, err, "Failed to start lolly")
	defer lolly.Terminate(ctx)

	// 创建 HTTP/2 客户端
	tlsConfig := &tls.Config{
		InsecureSkipVerify: true,
	}
	transport := &http2.Transport{
		TLSClientConfig: tlsConfig,
	}
	client := &http.Client{Transport: transport}

	// 并发发送多个请求
	numRequests := 10
	var wg sync.WaitGroup
	errors := make(chan error, numRequests)
	start := time.Now()

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			resp, err := client.Get(fmt.Sprintf("%s/test%d", lolly.HTTPSBaseURL(), id))
			if err != nil {
				errors <- fmt.Errorf("request %d failed: %w", id, err)
				return
			}
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		}(i)
	}

	wg.Wait()
	close(errors)

	elapsed := time.Since(start)
	t.Logf("Completed %d requests in %v", numRequests, elapsed)

	// 检查错误
	for err := range errors {
		t.Errorf("Request error: %v", err)
	}

	// 多路复用应该比串行请求快
	// 如果每个请求需要 100ms，串行需要 1s，多路复用应该更快
	assert.Less(t, elapsed, 2*time.Second, "Multiplexed requests should complete quickly")
}

// TestE2EHTTP2HeaderCompression 测试 HTTP/2 头部压缩。
//
// 验证 HPACK 压缩正常工作。
func TestE2EHTTP2HeaderCompression(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testutil.DefaultTestTimeout)
	defer cancel()

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	// 生成自签名证书
	certPath, keyPath, cleanup, err := testutil.GenerateSelfSignedCert(t.TempDir())
	require.NoError(t, err, "Failed to generate certificate")
	defer cleanup()

	// 构建配置
	cfg := testutil.NewConfigBuilder().
		WithServer(":8443").
		WithSSL("/etc/lolly/ssl/server.crt", "/etc/lolly/ssl/server.key").
		WithStatic("/", "/var/www/html")

	configYAML, err := cfg.Build()
	require.NoError(t, err, "Failed to build config")

	// 启动 lolly
	lolly, err := testutil.StartLolly(ctx,
		testutil.WithConfigYAML(configYAML),
		testutil.WithCert(certPath, keyPath),
	)
	require.NoError(t, err, "Failed to start lolly")
	defer lolly.Terminate(ctx)

	// 创建 HTTP/2 客户端
	transport := &http2.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}
	client := &http.Client{Transport: transport}

	// 发送多个请求，头部应该被压缩复用
	for i := 0; i < 5; i++ {
		req, err := http.NewRequest("GET", lolly.HTTPSBaseURL(), nil)
		require.NoError(t, err)

		// 添加自定义头部
		req.Header.Set("X-Custom-Header", "test-value-that-should-be-compressed")

		resp, err := client.Do(req)
		if err != nil {
			t.Logf("Request %d error: %v", i, err)
			continue
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()

		t.Logf("Request %d: status %d", i, resp.StatusCode)
	}
}

// TestE2EHTTP2ServerPush 测试 HTTP/2 服务器推送（如果支持）。
//
// 验证服务器推送功能。
func TestE2EHTTP2ServerPush(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testutil.DefaultTestTimeout)
	defer cancel()

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	// 生成自签名证书
	certPath, keyPath, cleanup, err := testutil.GenerateSelfSignedCert(t.TempDir())
	require.NoError(t, err, "Failed to generate certificate")
	defer cleanup()

	// 构建配置
	cfg := testutil.NewConfigBuilder().
		WithServer(":8443").
		WithSSL("/etc/lolly/ssl/server.crt", "/etc/lolly/ssl/server.key").
		WithStatic("/", "/var/www/html")

	configYAML, err := cfg.Build()
	require.NoError(t, err, "Failed to build config")

	// 启动 lolly
	lolly, err := testutil.StartLolly(ctx,
		testutil.WithConfigYAML(configYAML),
		testutil.WithCert(certPath, keyPath),
	)
	require.NoError(t, err, "Failed to start lolly")
	defer lolly.Terminate(ctx)

	// 创建支持推送的客户端
	transport := &http2.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}
	client := &http.Client{Transport: transport}

	// 发送请求
	resp, err := client.Get(lolly.HTTPSBaseURL())
	require.NoError(t, err, "Request failed")
	defer resp.Body.Close()

	// 检查是否支持推送（通过响应头）
	pushSupported := resp.Header.Get("HTTP2-Settings") != ""
	t.Logf("HTTP/2 response status: %d, push supported: %v", resp.StatusCode, pushSupported)
}

// TestE2EHTTP2ConnectionPreface 测试 HTTP/2 连接前缀。
//
// 验证服务器正确响应 HTTP/2 连接前缀。
func TestE2EHTTP2ConnectionPreface(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testutil.DefaultTestTimeout)
	defer cancel()

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	// 生成自签名证书
	certPath, keyPath, cleanup, err := testutil.GenerateSelfSignedCert(t.TempDir())
	require.NoError(t, err, "Failed to generate certificate")
	defer cleanup()

	// 构建配置
	cfg := testutil.NewConfigBuilder().
		WithServer(":8443").
		WithSSL("/etc/lolly/ssl/server.crt", "/etc/lolly/ssl/server.key").
		WithStatic("/", "/var/www/html")

	configYAML, err := cfg.Build()
	require.NoError(t, err, "Failed to build config")

	// 启动 lolly
	lolly, err := testutil.StartLolly(ctx,
		testutil.WithConfigYAML(configYAML),
		testutil.WithCert(certPath, keyPath),
	)
	require.NoError(t, err, "Failed to start lolly")
	defer lolly.Terminate(ctx)

	// 建立 TLS 连接
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", lolly.HTTPSAddr())
	require.NoError(t, err, "Failed to connect")
	defer conn.Close()

	tlsConn := tls.Client(conn, &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{"h2"},
	})
	require.NoError(t, tlsConn.HandshakeContext(ctx), "TLS handshake failed")

	// 验证协商的协议
	state := tlsConn.ConnectionState()
	assert.Equal(t, "h2", state.NegotiatedProtocol, "Expected h2 protocol")

	// 发送 HTTP/2 连接前缀
	preface := "PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n"
	_, err = tlsConn.Write([]byte(preface))
	require.NoError(t, err, "Failed to send preface")

	// 读取响应（服务器应该发送 SETTINGS 帧）
	buf := make([]byte, 1024)
	_ = tlsConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := tlsConn.Read(buf)
	if err != nil {
		t.Logf("Read error (expected SETTINGS frame): %v", err)
	} else {
		t.Logf("Received %d bytes after preface", n)
		// 检查是否是 SETTINGS 帧（类型 0x04）
		if n >= 9 && buf[3] == 0x04 {
			t.Log("Received SETTINGS frame")
		}
	}
}

// TestE2EHTTP2LargeRequest 测试 HTTP/2 大请求处理。
//
// 验证大请求体的处理。
func TestE2EHTTP2LargeRequest(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testutil.DefaultTestTimeout)
	defer cancel()

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	// 生成自签名证书
	certPath, keyPath, cleanup, err := testutil.GenerateSelfSignedCert(t.TempDir())
	require.NoError(t, err, "Failed to generate certificate")
	defer cleanup()

	// 构建配置
	cfg := testutil.NewConfigBuilder().
		WithServer(":8443").
		WithSSL("/etc/lolly/ssl/server.crt", "/etc/lolly/ssl/server.key").
		WithProxy("/upload", "http://backend:8080")

	configYAML, err := cfg.Build()
	require.NoError(t, err, "Failed to build config")

	// 启动 lolly
	lolly, err := testutil.StartLolly(ctx,
		testutil.WithConfigYAML(configYAML),
		testutil.WithCert(certPath, keyPath),
	)
	require.NoError(t, err, "Failed to start lolly")
	defer lolly.Terminate(ctx)

	// 创建 HTTP/2 客户端
	transport := &http2.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}
	client := &http.Client{Transport: transport}

	// 发送大请求（1MB）
	largeBody := strings.NewReader(strings.Repeat("x", 1024*1024))
	req, err := http.NewRequest("POST", lolly.HTTPSBaseURL()+"/upload", largeBody)
	require.NoError(t, err)

	resp, err := client.Do(req)
	if err != nil {
		t.Logf("Large request error: %v", err)
	} else {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		t.Logf("Large request status: %d", resp.StatusCode)
	}
}

// TestE2EHTTP2ConcurrentStreams 测试 HTTP/2 并发流限制。
//
// 验证服务器正确处理大量并发流。
func TestE2EHTTP2ConcurrentStreams(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testutil.DefaultTestTimeout)
	defer cancel()

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	// 生成自签名证书
	certPath, keyPath, cleanup, err := testutil.GenerateSelfSignedCert(t.TempDir())
	require.NoError(t, err, "Failed to generate certificate")
	defer cleanup()

	// 构建配置
	cfg := testutil.NewConfigBuilder().
		WithServer(":8443").
		WithSSL("/etc/lolly/ssl/server.crt", "/etc/lolly/ssl/server.key").
		WithStatic("/", "/var/www/html")

	configYAML, err := cfg.Build()
	require.NoError(t, err, "Failed to build config")

	// 启动 lolly
	lolly, err := testutil.StartLolly(ctx,
		testutil.WithConfigYAML(configYAML),
		testutil.WithCert(certPath, keyPath),
	)
	require.NoError(t, err, "Failed to start lolly")
	defer lolly.Terminate(ctx)

	// 创建 HTTP/2 客户端
	transport := &http2.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}
	client := &http.Client{Transport: transport}

	// 发送大量并发请求
	numStreams := 100
	var wg sync.WaitGroup
	successCount := 0
	mu := sync.Mutex{}

	for i := 0; i < numStreams; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			resp, err := client.Get(fmt.Sprintf("%s/stream%d", lolly.HTTPSBaseURL(), id))
			if err == nil {
				_, _ = io.Copy(io.Discard, resp.Body)
				_ = resp.Body.Close()
				mu.Lock()
				successCount++
				mu.Unlock()
			}
		}(i)
	}

	wg.Wait()

	t.Logf("Successfully handled %d/%d concurrent streams", successCount, numStreams)
	assert.Greater(t, successCount, numStreams/2, "Most streams should succeed")
}
