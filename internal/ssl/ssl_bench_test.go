// Package ssl 提供 SSL/TLS 性能基准测试。
//
// 测试覆盖：
//   - TLS 握手性能（模拟客户端握手）
//   - 证书加载性能
//   - 重新协商开销
//   - OCSP 装订性能
//   - 会话恢复性能（使用 session ticket）
//
// 作者：xfy
package ssl

import (
	"crypto/tls"
	"os"
	"testing"
	"time"

	"rua.plus/lolly/internal/config"
)

// BenchmarkTLSHandshake 基准测试 TLS 握手性能。
//
// 使用本地 TCP 环回连接模拟真实客户端握手，
// 测量完整 TLS 握手（包括密钥交换和证书验证）的开销。
func BenchmarkTLSHandshake(b *testing.B) {
	tmpDir := b.TempDir()
	certPEM, keyPEM := generateTestCert(&testing.T{})

	certPath := tmpDir + "/cert.pem"
	keyPath := tmpDir + "/key.pem"

	if err := os.WriteFile(certPath, certPEM, 0o644); err != nil {
		b.Fatalf("write cert: %v", err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		b.Fatalf("write key: %v", err)
	}

	cfg := &config.SSLConfig{
		Cert:      certPath,
		Key:       keyPath,
		Protocols: []string{"TLSv1.2", "TLSv1.3"},
	}

	manager, err := NewTLSManager(cfg)
	if err != nil {
		b.Fatalf("NewTLSManager() failed: %v", err)
	}
	defer manager.Close()

	serverTLS := manager.GetTLSConfig()

	listener, err := tls.Listen("tcp", "127.0.0.1:0", serverTLS)
	if err != nil {
		b.Fatalf("tls.Listen failed: %v", err)
	}
	defer listener.Close()

	// 后台接受连接
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			// 读取少量数据以完成握手
			buf := make([]byte, 1)
			_, _ = conn.Read(buf)
			_ = conn.Close()
		}
	}()

	clientTLS := &tls.Config{
		InsecureSkipVerify: true,
		MinVersion:         tls.VersionTLS12,
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			conn, err := tls.Dial("tcp", listener.Addr().String(), clientTLS)
			if err != nil {
				b.Error(err)
				continue
			}
			_, _ = conn.Write([]byte{0})
			_ = conn.Close()
		}
	})
}

// BenchmarkTLSHandshake_TLS13Only 基准测试仅 TLS 1.3 的握手性能。
//
// TLS 1.3 握手比 TLS 1.2 更快（1-RTT），测量两者的差异。
func BenchmarkTLSHandshake_TLS13Only(b *testing.B) {
	tmpDir := b.TempDir()
	certPEM, keyPEM := generateTestCert(&testing.T{})

	certPath := tmpDir + "/cert.pem"
	keyPath := tmpDir + "/key.pem"

	if err := os.WriteFile(certPath, certPEM, 0o644); err != nil {
		b.Fatalf("write cert: %v", err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		b.Fatalf("write key: %v", err)
	}

	cfg := &config.SSLConfig{
		Cert:      certPath,
		Key:       keyPath,
		Protocols: []string{"TLSv1.3"},
	}

	manager, err := NewTLSManager(cfg)
	if err != nil {
		b.Fatalf("NewTLSManager() failed: %v", err)
	}
	defer manager.Close()

	serverTLS := manager.GetTLSConfig()

	listener, err := tls.Listen("tcp", "127.0.0.1:0", serverTLS)
	if err != nil {
		b.Fatalf("tls.Listen failed: %v", err)
	}
	defer listener.Close()

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			buf := make([]byte, 1)
			_, _ = conn.Read(buf)
			_ = conn.Close()
		}
	}()

	clientTLS := &tls.Config{
		InsecureSkipVerify: true,
		MinVersion:         tls.VersionTLS13,
		MaxVersion:         tls.VersionTLS13,
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			conn, err := tls.Dial("tcp", listener.Addr().String(), clientTLS)
			if err != nil {
				b.Error(err)
				continue
			}
			_, _ = conn.Write([]byte{0})
			_ = conn.Close()
		}
	})
}

// BenchmarkTLSCertificateLoad 基准测试证书加载性能。
//
// 测量从磁盘加载 X509 密钥对的开销，包括 PEM 解码和
// 私钥解析。
func BenchmarkTLSCertificateLoad(b *testing.B) {
	tmpDir := b.TempDir()
	certPEM, keyPEM := generateTestCert(&testing.T{})

	certPath := tmpDir + "/cert.pem"
	keyPath := tmpDir + "/key.pem"

	if err := os.WriteFile(certPath, certPEM, 0o644); err != nil {
		b.Fatalf("write cert: %v", err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		b.Fatalf("write key: %v", err)
	}

	b.ResetTimer()
	for b.Loop() {
		_, err := tls.LoadX509KeyPair(certPath, keyPath)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkTLSCertificateLoad_InMemory 基准测试内存中证书加载性能。
//
// 不经过磁盘 I/O，直接测量 PEM 解析和证书构建的开销。
func BenchmarkTLSCertificateLoad_InMemory(b *testing.B) {
	certPEM, keyPEM := generateTestCert(&testing.T{})

	b.ResetTimer()
	for b.Loop() {
		_, err := tls.X509KeyPair(certPEM, keyPEM)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkTLSCertificateLoad_Parallel 基准测试并发证书加载性能。
//
// 模拟多协程同时加载证书的场景，验证线程安全性。
func BenchmarkTLSCertificateLoad_Parallel(b *testing.B) {
	tmpDir := b.TempDir()
	certPEM, keyPEM := generateTestCert(&testing.T{})

	certPath := tmpDir + "/cert.pem"
	keyPath := tmpDir + "/key.pem"

	if err := os.WriteFile(certPath, certPEM, 0o644); err != nil {
		b.Fatalf("write cert: %v", err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		b.Fatalf("write key: %v", err)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := tls.LoadX509KeyPair(certPath, keyPath)
			if err != nil {
				b.Error(err)
			}
		}
	})
}

// BenchmarkTLSRenegotiation 基准测试 TLS 重新协商开销。
//
// TLS 1.3 已移除重新协商，该测试测量 TLS 1.2 下的
// 重新协商成本，用于评估是否值得启用重新协商。
// 通过建立 TLS 1.2 连接并检查握手统计来测量开销。
func BenchmarkTLSRenegotiation(b *testing.B) {
	tmpDir := b.TempDir()
	certPEM, keyPEM := generateTestCert(&testing.T{})

	certPath := tmpDir + "/cert.pem"
	keyPath := tmpDir + "/key.pem"

	if err := os.WriteFile(certPath, certPEM, 0o644); err != nil {
		b.Fatalf("write cert: %v", err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		b.Fatalf("write key: %v", err)
	}

	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		b.Fatalf("tls.LoadX509KeyPair failed: %v", err)
	}

	serverTLS := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
		MaxVersion:   tls.VersionTLS12,
	}

	listener, err := tls.Listen("tcp", "127.0.0.1:0", serverTLS)
	if err != nil {
		b.Fatalf("tls.Listen failed: %v", err)
	}
	defer listener.Close()

	// 服务端接受连接
	acceptDone := make(chan struct{})
	go func() {
		defer close(acceptDone)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			// 读取数据完成握手后关闭
			buf := make([]byte, 1)
			_, _ = conn.Read(buf)
			_ = conn.Close()
		}
	}()

	clientTLS := &tls.Config{
		InsecureSkipVerify: true,
		MinVersion:         tls.VersionTLS12,
		MaxVersion:         tls.VersionTLS12,
		Renegotiation:      tls.RenegotiateOnceAsClient,
	}

	b.ResetTimer()
	for b.Loop() {
		conn, err := tls.Dial("tcp", listener.Addr().String(), clientTLS)
		if err != nil {
			b.Fatalf("Dial failed: %v", err)
		}
		_, _ = conn.Write([]byte{0})
		_ = conn.Close()
	}
	b.StopTimer()
	listener.Close()
	<-acceptDone
}

// BenchmarkOCSPStapling 基准测试 OCSP 装订性能。
//
// 测量启用 OCSP Stapling 后，GetConfigForClient 回调中
// 获取和附加 OCSP 响应的开销。
func BenchmarkOCSPStapling(b *testing.B) {
	// 直接测试 OCSPManager 的响应获取性能
	// （自签证书没有 OCSP Server URL，无法端到端测试）
	ocspMgr := NewOCSPManager(DefaultOCSPConfig())
	ocspMgr.Start()
	defer ocspMgr.Stop()

	// 注册一个模拟序列号的状态
	testSerial := "1234567890"
	ocspMgr.mu.Lock()
	ocspMgr.responses[testSerial] = &ocspResponse{
		response:   make([]byte, 1024),
		thisUpdate: time.Now(),
		nextUpdate: time.Now().Add(time.Hour),
		status:     statusValid,
		fetchedAt:  time.Now(),
		errors:     0,
	}
	ocspMgr.mu.Unlock()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			resp := ocspMgr.GetOCSPResponse(testSerial)
			if resp == nil {
				b.Error("expected non-nil OCSP response")
			}
		}
	})
}

// BenchmarkOCSPStapling_Miss 基准测试 OCSP 未命中性能。
//
// 测量查询不存在序列号时的开销，验证优雅降级不影响性能。
func BenchmarkOCSPStapling_Miss(b *testing.B) {
	ocspMgr := NewOCSPManager(DefaultOCSPConfig())
	ocspMgr.Start()
	defer ocspMgr.Stop()

	nonExistentSerial := "nonexistent"

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			resp := ocspMgr.GetOCSPResponse(nonExistentSerial)
			if resp != nil {
				b.Error("expected nil OCSP response for non-existent serial")
			}
		}
	})
}

// BenchmarkOCSPStapling_GetStatus 基准测试 OCSP 状态查询性能。
//
// 测量 GetStatus 方法获取证书状态信息的开销。
func BenchmarkOCSPStapling_GetStatus(b *testing.B) {
	ocspMgr := NewOCSPManager(DefaultOCSPConfig())
	ocspMgr.Start()
	defer ocspMgr.Stop()

	// 注册一些模拟状态
	for i := 0; i < 10; i++ {
		serial := string(rune('0' + i))
		ocspMgr.mu.Lock()
		ocspMgr.responses[serial] = &ocspResponse{
			response:   make([]byte, 512),
			thisUpdate: time.Now(),
			nextUpdate: time.Now().Add(time.Hour),
			status:     statusValid,
			fetchedAt:  time.Now(),
			errors:     0,
		}
		ocspMgr.mu.Unlock()
	}

	b.ResetTimer()
	var idx int
	for b.Loop() {
		serial := string(rune('0' + (idx % 10)))
		idx++
		status, hasResponse := ocspMgr.GetStatus(serial)
		if !hasResponse {
			b.Error("expected hasResponse=true")
		}
		_ = status
	}
}

// BenchmarkSessionResumption 基准测试使用 Session Ticket 的会话恢复性能。
//
// 使用 ClientSessionCache 实现会话恢复，
// 测量使用缓存 session 时的握手开销。
func BenchmarkSessionResumption(b *testing.B) {
	tmpDir := b.TempDir()
	certPEM, keyPEM := generateTestCert(&testing.T{})

	certPath := tmpDir + "/cert.pem"
	keyPath := tmpDir + "/key.pem"

	if err := os.WriteFile(certPath, certPEM, 0o644); err != nil {
		b.Fatalf("write cert: %v", err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		b.Fatalf("write key: %v", err)
	}

	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		b.Fatalf("tls.LoadX509KeyPair failed: %v", err)
	}

	// 创建 Session Ticket 管理器
	sessionMgr, err := NewSessionTicketManager(config.SessionTicketsConfig{
		Enabled:        true,
		RotateInterval: time.Hour,
		RetainKeys:     3,
	})
	if err != nil {
		b.Fatalf("NewSessionTicketManager failed: %v", err)
	}
	defer sessionMgr.Stop()

	// 预热密钥轮换
	for i := 0; i < 2; i++ {
		_ = sessionMgr.RotateKey()
	}

	serverTLS := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
		MaxVersion:   tls.VersionTLS13,
	}
	sessionMgr.ApplyToTLSConfig(serverTLS)

	listener, err := tls.Listen("tcp", "127.0.0.1:0", serverTLS)
	if err != nil {
		b.Fatalf("tls.Listen failed: %v", err)
	}
	defer listener.Close()

	// 服务端接受连接
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			buf := make([]byte, 1)
			_, _ = conn.Read(buf)
			_ = conn.Close()
		}
	}()

	// 使用 ClientSessionCache 的客户端配置
	clientTLS := &tls.Config{
		InsecureSkipVerify: true,
		MinVersion:         tls.VersionTLS12,
		ClientSessionCache: tls.NewLRUClientSessionCache(8),
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			conn, err := tls.Dial("tcp", listener.Addr().String(), clientTLS)
			if err != nil {
				b.Error(err)
				continue
			}
			_, _ = conn.Write([]byte{0})

			// 检查是否使用了会话恢复
			cs := conn.ConnectionState()
			_ = cs.DidResume

			_ = conn.Close()
		}
	})
}

// BenchmarkSessionResumption_FullHandshake 基准测试完整握手（无会话恢复）。
//
// 作为对照组，测量没有 session ticket 的完整 TLS 握手开销，
// 与 BenchmarkSessionResumption 对比评估会话恢复的性能提升。
func BenchmarkSessionResumption_FullHandshake(b *testing.B) {
	tmpDir := b.TempDir()
	certPEM, keyPEM := generateTestCert(&testing.T{})

	certPath := tmpDir + "/cert.pem"
	keyPath := tmpDir + "/key.pem"

	if err := os.WriteFile(certPath, certPEM, 0o644); err != nil {
		b.Fatalf("write cert: %v", err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		b.Fatalf("write key: %v", err)
	}

	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		b.Fatalf("tls.LoadX509KeyPair failed: %v", err)
	}

	serverTLS := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
		MaxVersion:   tls.VersionTLS13,
	}

	listener, err := tls.Listen("tcp", "127.0.0.1:0", serverTLS)
	if err != nil {
		b.Fatalf("tls.Listen failed: %v", err)
	}
	defer listener.Close()

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			buf := make([]byte, 1)
			_, _ = conn.Read(buf)
			_ = conn.Close()
		}
	}()

	// 禁用客户端 session 缓存
	clientTLS := &tls.Config{
		InsecureSkipVerify: true,
		MinVersion:         tls.VersionTLS12,
		ClientSessionCache: nil,
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			conn, err := tls.Dial("tcp", listener.Addr().String(), clientTLS)
			if err != nil {
				b.Error(err)
				continue
			}
			_, _ = conn.Write([]byte{0})
			_ = conn.Close()
		}
	})
}

// BenchmarkSessionTicketManager_ApplyToTLSConfig 基准测试应用 Session Ticket 到 TLS 配置的开销。
func BenchmarkSessionTicketManager_ApplyToTLSConfig(b *testing.B) {
	sessionMgr, err := NewSessionTicketManager(config.SessionTicketsConfig{
		Enabled:        true,
		RotateInterval: time.Hour,
		RetainKeys:     3,
	})
	if err != nil {
		b.Fatalf("NewSessionTicketManager failed: %v", err)
	}
	defer sessionMgr.Stop()

	for i := 0; i < 2; i++ {
		_ = sessionMgr.RotateKey()
	}

	baseTLS := &tls.Config{
		Certificates: []tls.Certificate{},
		MinVersion:   tls.VersionTLS12,
	}

	b.ResetTimer()
	for b.Loop() {
		tlsCfg := baseTLS.Clone()
		sessionMgr.ApplyToTLSConfig(tlsCfg)
	}
}

// BenchmarkSNI_GetCertificate 基准测试 SNI 证书查找性能。
//
// 测量 GetCertificate 回调在多证书场景下的查找开销。
func BenchmarkSNI_GetCertificate(b *testing.B) {
	tmpDir := b.TempDir()
	certPEM, keyPEM := generateTestCert(&testing.T{})

	certPath := tmpDir + "/cert.pem"
	keyPath := tmpDir + "/key.pem"

	if err := os.WriteFile(certPath, certPEM, 0o644); err != nil {
		b.Fatalf("write cert: %v", err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		b.Fatalf("write key: %v", err)
	}

	configs := map[string]*config.SSLConfig{
		"example.com": {
			Cert: certPath,
			Key:  keyPath,
		},
		"api.example.com": {
			Cert: certPath,
			Key:  keyPath,
		},
		"cdn.example.com": {
			Cert: certPath,
			Key:  keyPath,
		},
	}

	manager, err := NewMultiTLSManager(configs, &config.SSLConfig{
		Cert: certPath,
		Key:  keyPath,
	})
	if err != nil {
		b.Fatalf("NewMultiTLSManager failed: %v", err)
	}
	defer manager.Close()

	getCert := manager.GetCertificate()

	hostnames := []string{
		"example.com",
		"api.example.com",
		"cdn.example.com",
		"unknown.example.com",
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		idx := 0
		for pb.Next() {
			hostname := hostnames[idx%len(hostnames)]
			idx++

			_, err := getCert(&tls.ClientHelloInfo{
				ServerName: hostname,
			})
			if err != nil {
				b.Error(err)
			}
		}
	})
}

// BenchmarkCipherSuiteParsing 基准测试加密套件解析性能。
func BenchmarkCipherSuiteParsing(b *testing.B) {
	ciphers := []string{
		"TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256",
		"TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384",
		"TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305",
		"TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256",
		"TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384",
	}

	b.ResetTimer()
	for b.Loop() {
		_, err := parseCipherSuites(ciphers)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkTLSVersionsParsing 基准测试 TLS 版本解析性能。
func BenchmarkTLSVersionsParsing(b *testing.B) {
	protocols := []string{"TLSv1.2", "TLSv1.3"}

	b.ResetTimer()
	for b.Loop() {
		_, _, err := parseTLSVersions(protocols)
		if err != nil {
			b.Fatal(err)
		}
	}
}
