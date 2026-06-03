// Package http2 提供 HTTP/2 TLS 连接集成测试。
//
// 该文件测试 HTTP/2 服务器的 TLS 相关功能：
//   - TLS 握手成功/失败
//   - ALPN 协商 h2/http1.1
//   - HTTP/1.1 回退路径
//
// 作者：xfy
package http2

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/config"
)

// generateTestCert 生成测试用自签名证书。
func generateTestCert(t *testing.T) (tls.Certificate, *x509.CertPool) {
	t.Helper()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("Failed to generate private key: %v", err)
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Test Org"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		t.Fatalf("Failed to create certificate: %v", err)
	}

	cert := tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  privateKey,
	}

	certPool := x509.NewCertPool()
	certPool.AppendCertsFromPEM(certDER)

	return cert, certPool
}

// TestTLSHandshakeSuccess 测试 TLS 握手成功。
func TestTLSHandshakeSuccess(t *testing.T) {
	cert, _ := generateTestCert(t)

	handler := func(ctx *fasthttp.RequestCtx) {
		ctx.WriteString("Hello HTTP/2")
		ctx.SetStatusCode(fasthttp.StatusOK)
	}

	cfg := &config.HTTP2Config{
		Enabled:              true,
		MaxConcurrentStreams: 100,
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   []string{"h2", "http/1.1"},
	}

	server, err := NewServer(cfg, handler, tlsConfig)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// 创建管道连接
	serverConn, clientConn := net.Pipe()
	defer func() {
		_ = serverConn.Close()
		_ = clientConn.Close()
	}()

	// 包装服务器端连接为 TLS
	tlsServerConn := tls.Server(serverConn, tlsConfig)

	// 需要先 Add(1) 因为 handleConnection 会调用 Done()
	server.connWg.Add(1)

	// 在后台处理连接
	go func() {
		server.handleConnection(tlsServerConn)
	}()

	// 客户端 TLS 握手
	tlsClientConfig := &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{"h2"},
	}
	tlsClientConn := tls.Client(clientConn, tlsClientConfig)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := tlsClientConn.HandshakeContext(ctx); err != nil {
		t.Fatalf("TLS handshake failed: %v", err)
	}

	// 验证协商的协议
	state := tlsClientConn.ConnectionState()
	if state.NegotiatedProtocol != "h2" {
		t.Errorf("Expected negotiated protocol 'h2', got '%s'", state.NegotiatedProtocol)
	}

	// 关闭连接
	_ = tlsClientConn.Close()
	_ = tlsServerConn.Close()

	// 等待处理完成
	done := make(chan struct{})
	go func() {
		server.connWg.Wait()
		close(done)
	}()
	select {
	case <-done:
		t.Log("Connection handling completed")
	case <-time.After(2 * time.Second):
		t.Log("Timeout waiting for connection handling")
	}
}

// TestTLSHandshakeFailure 测试 TLS 握手失败。
func TestTLSHandshakeFailure(t *testing.T) {
	cert, _ := generateTestCert(t)

	handler := func(ctx *fasthttp.RequestCtx) {
		ctx.WriteString("Hello")
	}

	cfg := &config.HTTP2Config{
		Enabled: true,
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   []string{"h2"},
	}

	server, err := NewServer(cfg, handler, tlsConfig)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	serverConn, clientConn := net.Pipe()

	// 包装服务器端连接为 TLS
	tlsServerConn := tls.Server(serverConn, tlsConfig)

	// 需要先 Add(1) 因为 handleConnection 会调用 Done()
	server.connWg.Add(1)

	// 在后台处理连接
	go func() {
		server.handleConnection(tlsServerConn)
	}()

	// 客户端不进行 TLS 握手，直接发送无效数据
	_, _ = clientConn.Write([]byte("INVALID DATA NOT TLS"))

	// 等待处理完成
	time.Sleep(200 * time.Millisecond)

	// 关闭连接
	_ = clientConn.Close()
	_ = tlsServerConn.Close()

	// 等待处理完成
	done := make(chan struct{})
	go func() {
		server.connWg.Wait()
		close(done)
	}()
	select {
	case <-done:
		t.Log("Connection handling completed after handshake failure")
	case <-time.After(2 * time.Second):
		t.Log("Timeout waiting for connection handling")
	}
}

// TestALPNNegotiationH2 测试 ALPN 协商选择 h2。


// TestALPNHTTP11Fallback 测试 ALPN 协商回退到 HTTP/1.1。
func TestALPNHTTP11Fallback(t *testing.T) {
	cert, _ := generateTestCert(t)

	handler := func(ctx *fasthttp.RequestCtx) {
		ctx.WriteString("HTTP/1.1 response")
		ctx.SetStatusCode(fasthttp.StatusOK)
	}

	cfg := &config.HTTP2Config{
		Enabled: true,
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   []string{"h2", "http/1.1"},
	}

	server, err := NewServer(cfg, handler, tlsConfig)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	serverConn, clientConn := net.Pipe()
	defer func() {
		_ = serverConn.Close()
		_ = clientConn.Close()
	}()

	tlsServerConn := tls.Server(serverConn, tlsConfig)

	// 需要先 Add(1) 因为 handleConnection 会调用 Done()
	server.connWg.Add(1)

	go func() {
		server.handleConnection(tlsServerConn)
	}()

	// 客户端只支持 HTTP/1.1
	tlsClientConfig := &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{"http/1.1"},
	}
	tlsClientConn := tls.Client(clientConn, tlsClientConfig)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := tlsClientConn.HandshakeContext(ctx); err != nil {
		t.Fatalf("TLS handshake failed: %v", err)
	}

	// 验证协商的协议是 http/1.1
	state := tlsClientConn.ConnectionState()
	if state.NegotiatedProtocol != "http/1.1" {
		t.Errorf("Expected negotiated protocol 'http/1.1', got '%s'", state.NegotiatedProtocol)
	}

	_ = tlsClientConn.Close()
	_ = tlsServerConn.Close()
}

// TestTLSListenerWrapper 测试 TLS 监听器包装。


// TestTLSListenerExistingProtos 测试已有 NextProtos 的情况。


// TestServeHTTP1Fallback 测试 HTTP/1.1 回退。
func TestServeHTTP1Fallback(t *testing.T) {
	handler := func(ctx *fasthttp.RequestCtx) {
		ctx.WriteString("HTTP/1.1 response")
		ctx.SetStatusCode(fasthttp.StatusOK)
		ctx.Response.Header.Set("X-Test", "value")
	}

	cfg := &config.HTTP2Config{
		Enabled: true,
	}

	server, err := NewServer(cfg, handler, nil)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	serverConn, clientConn := net.Pipe()
	defer func() {
		_ = serverConn.Close()
		_ = clientConn.Close()
	}()

	var wg sync.WaitGroup
	wg.Go(func() {
		server.serveHTTP1(serverConn)
	})

	// 发送 HTTP/1.1 请求
	request := "GET /test HTTP/1.1\r\nHost: localhost\r\n\r\n"
	_, _ = clientConn.Write([]byte(request))

	// 读取响应
	buf := make([]byte, 1024)
	_ = clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := clientConn.Read(buf)
	if err != nil {
		t.Fatalf("Failed to read response: %v", err)
	}

	response := string(buf[:n])
	if response == "" {
		t.Error("Expected non-empty response")
	}

	// 关闭连接
	_ = clientConn.Close()
	wg.Wait()
}

// TestConnectionPoolOperations 测试连接池操作。
func TestConnectionPoolOperations(t *testing.T) {
	pool := newConnectionPool()

	// 创建模拟连接
	conn1 := &mockTestConn{remoteAddr: &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345}}
	conn2 := &mockTestConn{remoteAddr: &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12346}}

	// 添加连接
	pool.add("client1", conn1)
	pool.add("client1", conn2)

	// 验证连接数
	if count := len(pool.conns["client1"]); count != 2 {
		t.Errorf("Expected 2 connections, got %d", count)
	}

	// 获取连接
	conns := pool.conns["client1"]
	if len(conns) != 2 {
		t.Errorf("Expected 2 connections, got %d", len(conns))
	}

	// 移除连接
	pool.remove("client1", conn1)
	if count := len(pool.conns["client1"]); count != 1 {
		t.Errorf("Expected 1 connection after removal, got %d", count)
	}

	// 关闭所有连接
	pool.closeAll()
	if count := len(pool.conns["client1"]); count != 0 {
		t.Errorf("Expected 0 connections after closeAll, got %d", count)
	}
}

// mockTestConn 是用于测试的模拟连接。
type mockTestConn struct {
	remoteAddr net.Addr
}

func (m *mockTestConn) Read(_ []byte) (n int, err error)  { return 0, nil }
func (m *mockTestConn) Write(_ []byte) (n int, err error) { return 0, nil }
func (m *mockTestConn) Close() error                      { return nil }
func (m *mockTestConn) LocalAddr() net.Addr               { return m.remoteAddr }
func (m *mockTestConn) RemoteAddr() net.Addr              { return m.remoteAddr }
func (m *mockTestConn) SetDeadline(_ time.Time) error     { return nil }
func (m *mockTestConn) SetReadDeadline(_ time.Time) error { return nil }
func (m *mockTestConn) SetWriteDeadline(_ time.Time) error {
	return nil
}



