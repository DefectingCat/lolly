// Package stream 提供服务器相关函数的覆盖测试。
//
// 该文件测试以下未覆盖的方法：
//   - ListenTCP：TCP 监听
//   - Start：服务器启动
//   - getOrCreateSession：UDP 会话创建
//   - handleBackendResponse：UDP 后端响应处理
//   - serve：UDP 服务循环
//   - startCleanupTicker：UDP 过期清理 ticker
//
// 作者：xfy
package stream

import (
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListenTCP_Success(t *testing.T) {
	t.Parallel()

	s := NewServer()

	err := s.ListenTCP("127.0.0.1:0")
	require.NoError(t, err)

	s.mu.RLock()
	require.Len(t, s.listeners, 1)
	s.mu.RUnlock()

	s.mu.Lock()
	for _, ln := range s.listeners {
		_ = ln.Close()
	}
	s.mu.Unlock()
}

func TestListenTCP_InvalidAddress(t *testing.T) {
	t.Parallel()

	s := NewServer()

	err := s.ListenTCP("256.256.256.256:99999")
	assert.Error(t, err)

	s.mu.RLock()
	assert.Empty(t, s.listeners)
	s.mu.RUnlock()
}

func TestStart_NoListeners(t *testing.T) {
	t.Parallel()

	s := NewServer()

	err := s.Start()
	require.NoError(t, err)
	assert.True(t, s.running.Load())

	s.running.Store(false)
}

func TestStart_WithTCPListeners(t *testing.T) {
	s := NewServer()

	targets := []TargetSpec{
		{Addr: "127.0.0.1:0", Weight: 1},
	}
	_ = s.AddUpstream("test", targets, "round_robin", HealthCheckSpec{})

	err := s.ListenTCP("127.0.0.1:0")
	require.NoError(t, err)

	err = s.Start()
	require.NoError(t, err)
	assert.True(t, s.running.Load())

	s.running.Store(false)
	s.mu.RLock()
	for _, ln := range s.listeners {
		_ = ln.Close()
	}
	s.mu.RUnlock()
}

func TestStart_AcceptConnections(t *testing.T) {
	s := NewServer()

	backendLn, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	go func() {
		conn, acceptErr := backendLn.Accept()
		if acceptErr != nil {
			return
		}
		defer conn.Close()
		_, _ = io.Copy(conn, conn)
	}()

	targets := []TargetSpec{
		{Addr: backendLn.Addr().String(), Weight: 1},
	}
	_ = s.AddUpstream("test", targets, "round_robin", HealthCheckSpec{})
	s.upstreams["test"].targets[0].healthy.Store(true)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	proxyAddr := ln.Addr().String()

	s.mu.Lock()
	s.listeners[proxyAddr] = ln
	s.mu.Unlock()

	err = s.Start()
	require.NoError(t, err)

	clientConn, err := net.DialTimeout("tcp", proxyAddr, 2*time.Second)
	require.NoError(t, err)

	testData := []byte("hello stream proxy")
	_, err = clientConn.Write(testData)
	require.NoError(t, err)

	buf := make([]byte, len(testData))
	_ = clientConn.SetReadDeadline(time.Now().Add(3 * time.Second))
	n, err := clientConn.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, testData, buf[:n])

	_ = clientConn.Close()

	s.running.Store(false)
	s.mu.RLock()
	for _, l := range s.listeners {
		_ = l.Close()
	}
	s.mu.RUnlock()
	_ = backendLn.Close()
}

func TestNewUDPServer_DefaultTimeout(t *testing.T) {
	t.Parallel()

	udpAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	require.NoError(t, err)
	conn, err := net.ListenUDP("udp", udpAddr)
	require.NoError(t, err)
	defer conn.Close()

	upstream := &Upstream{
		targets:  []*Target{{addr: "127.0.0.1:19099"}},
		balancer: newRoundRobin(),
	}

	srv := newUDPServer(conn, upstream, 0)
	assert.Equal(t, 60*time.Second, srv.timeout)

	srv2 := newUDPServer(conn, upstream, -1*time.Second)
	assert.Equal(t, 60*time.Second, srv2.timeout)
}

func TestNewUDPServer_CustomTimeout(t *testing.T) {
	t.Parallel()

	udpAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	require.NoError(t, err)
	conn, err := net.ListenUDP("udp", udpAddr)
	require.NoError(t, err)
	defer conn.Close()

	upstream := &Upstream{
		targets:  []*Target{{addr: "127.0.0.1:19099"}},
		balancer: newRoundRobin(),
	}

	srv := newUDPServer(conn, upstream, 45*time.Second)
	assert.Equal(t, 45*time.Second, srv.timeout)
}

func TestSessionKey(t *testing.T) {
	t.Parallel()

	addr1, err := net.ResolveUDPAddr("udp", "192.168.1.1:12345")
	require.NoError(t, err)

	key := sessionKey(addr1)
	assert.Equal(t, "192.168.1.1:12345", key)

	addr2, err := net.ResolveUDPAddr("udp", "[::1]:54321")
	require.NoError(t, err)
	key2 := sessionKey(addr2)
	assert.Contains(t, key2, "54321")
}

func TestGetSession_NotExist(t *testing.T) {
	t.Parallel()

	udpAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	require.NoError(t, err)
	conn, err := net.ListenUDP("udp", udpAddr)
	require.NoError(t, err)
	defer conn.Close()

	upstream := &Upstream{
		targets:  []*Target{{addr: "127.0.0.1:19100"}},
		balancer: newRoundRobin(),
	}
	srv := newUDPServer(conn, upstream, time.Minute)

	clientAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:11111")
	require.NoError(t, err)

	session := srv.getSession(clientAddr)
	assert.Nil(t, session)
}

func TestGetSession_Existing(t *testing.T) {
	t.Parallel()

	udpAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	require.NoError(t, err)
	conn, err := net.ListenUDP("udp", udpAddr)
	require.NoError(t, err)
	defer conn.Close()

	upstream := &Upstream{
		targets:  []*Target{{addr: "127.0.0.1:19101"}},
		balancer: newRoundRobin(),
	}
	srv := newUDPServer(conn, upstream, time.Minute)

	clientAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:22222")
	require.NoError(t, err)

	oldTime := time.Now().Add(-10 * time.Second)
	testSession := &udpSession{
		clientAddr: clientAddr,
		lastActive: oldTime,
		srv:        srv,
	}
	srv.sessions[sessionKey(clientAddr)] = testSession

	session := srv.getSession(clientAddr)
	require.NotNil(t, session)

	session.mu.RLock()
	newActive := session.lastActive
	session.mu.RUnlock()
	assert.True(t, newActive.After(oldTime))
}

func TestRemoveSession(t *testing.T) {
	t.Parallel()

	udpAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	require.NoError(t, err)
	conn, err := net.ListenUDP("udp", udpAddr)
	require.NoError(t, err)
	defer conn.Close()

	upstream := &Upstream{
		targets:  []*Target{{addr: "127.0.0.1:19102"}},
		balancer: newRoundRobin(),
	}
	srv := newUDPServer(conn, upstream, time.Minute)

	clientAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:33333")
	require.NoError(t, err)

	targetUDPAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	require.NoError(t, err)
	targetConn, err := net.ListenUDP("udp", targetUDPAddr)
	require.NoError(t, err)
	defer targetConn.Close()

	testSession := &udpSession{
		clientAddr: clientAddr,
		targetConn: targetConn,
		lastActive: time.Now(),
		srv:        srv,
		target:     &Target{addr: "127.0.0.1:19102"},
	}
	srv.sessions[sessionKey(clientAddr)] = testSession

	srv.removeSession(clientAddr)

	srv.mu.RLock()
	_, exists := srv.sessions[sessionKey(clientAddr)]
	srv.mu.RUnlock()
	assert.False(t, exists)
}

func TestRemoveSession_NotExist(t *testing.T) {
	t.Parallel()

	udpAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	require.NoError(t, err)
	conn, err := net.ListenUDP("udp", udpAddr)
	require.NoError(t, err)
	defer conn.Close()

	upstream := &Upstream{
		targets:  []*Target{{addr: "127.0.0.1:19103"}},
		balancer: newRoundRobin(),
	}
	srv := newUDPServer(conn, upstream, time.Minute)

	clientAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:44444")
	require.NoError(t, err)

	srv.removeSession(clientAddr)

	srv.mu.RLock()
	assert.Empty(t, srv.sessions)
	srv.mu.RUnlock()
}

func TestCleanupExpiredSessions_RemovesExpired(t *testing.T) {
	t.Parallel()

	udpAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	require.NoError(t, err)
	conn, err := net.ListenUDP("udp", udpAddr)
	require.NoError(t, err)
	defer conn.Close()

	upstream := &Upstream{
		targets:  []*Target{{addr: "127.0.0.1:19104"}},
		balancer: newRoundRobin(),
	}
	srv := newUDPServer(conn, upstream, 100*time.Millisecond)

	expiredAddr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:55555")
	expiredSession := &udpSession{
		clientAddr: expiredAddr,
		lastActive: time.Now().Add(-1 * time.Hour),
		srv:        srv,
	}
	srv.sessions[sessionKey(expiredAddr)] = expiredSession

	activeAddr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:55556")
	activeSession := &udpSession{
		clientAddr: activeAddr,
		lastActive: time.Now(),
		srv:        srv,
	}
	srv.sessions[sessionKey(activeAddr)] = activeSession

	srv.cleanupExpiredSessions()

	srv.mu.RLock()
	assert.Len(t, srv.sessions, 1)
	_, hasActive := srv.sessions[sessionKey(activeAddr)]
	srv.mu.RUnlock()
	assert.True(t, hasActive)
}

func TestCleanupExpiredSessions_AllExpired(t *testing.T) {
	t.Parallel()

	udpAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	require.NoError(t, err)
	conn, err := net.ListenUDP("udp", udpAddr)
	require.NoError(t, err)
	defer conn.Close()

	upstream := &Upstream{
		targets:  []*Target{{addr: "127.0.0.1:19105"}},
		balancer: newRoundRobin(),
	}
	srv := newUDPServer(conn, upstream, 1*time.Millisecond)

	for i := range 5 {
		addr, _ := net.ResolveUDPAddr("udp", fmt.Sprintf("127.0.0.1:%d", 60000+i))
		sess := &udpSession{
			clientAddr: addr,
			lastActive: time.Now().Add(-1 * time.Hour),
			srv:        srv,
		}
		srv.sessions[sessionKey(addr)] = sess
	}

	srv.cleanupExpiredSessions()

	srv.mu.RLock()
	assert.Empty(t, srv.sessions)
	srv.mu.RUnlock()
}

func TestGetOrCreateSession_NoHealthyTargets(t *testing.T) {
	t.Parallel()

	udpAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	require.NoError(t, err)
	conn, err := net.ListenUDP("udp", udpAddr)
	require.NoError(t, err)
	defer conn.Close()

	upstream := &Upstream{
		targets:  []*Target{{addr: "127.0.0.1:19106"}},
		balancer: newRoundRobin(),
	}
	srv := newUDPServer(conn, upstream, time.Minute)

	clientAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:51111")
	require.NoError(t, err)

	session, err := srv.getOrCreateSession(clientAddr)
	assert.Nil(t, session)
	assert.Error(t, err)
}

func TestGetOrCreateSession_ExistingSession(t *testing.T) {
	t.Parallel()

	udpAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	require.NoError(t, err)
	conn, err := net.ListenUDP("udp", udpAddr)
	require.NoError(t, err)
	defer conn.Close()

	upstream := &Upstream{
		targets:  []*Target{{addr: "127.0.0.1:19107"}},
		balancer: newRoundRobin(),
	}
	upstream.targets[0].healthy.Store(true)
	srv := newUDPServer(conn, upstream, time.Minute)

	clientAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:52222")
	require.NoError(t, err)

	existingSession := &udpSession{
		clientAddr: clientAddr,
		lastActive: time.Now(),
		srv:        srv,
	}
	srv.sessions[sessionKey(clientAddr)] = existingSession

	session, err := srv.getOrCreateSession(clientAddr)
	require.NoError(t, err)
	assert.Equal(t, existingSession, session)
}

func TestGetOrCreateSession_NewSession(t *testing.T) {
	backendAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	require.NoError(t, err)
	backendConn, err := net.ListenUDP("udp", backendAddr)
	require.NoError(t, err)
	defer backendConn.Close()

	proxyAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	require.NoError(t, err)
	proxyConn, err := net.ListenUDP("udp", proxyAddr)
	require.NoError(t, err)
	defer proxyConn.Close()

	upstream := &Upstream{
		targets:  []*Target{{addr: backendConn.LocalAddr().String()}},
		balancer: newRoundRobin(),
	}
	upstream.targets[0].healthy.Store(true)
	srv := newUDPServer(proxyConn, upstream, time.Minute)

	clientAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:53333")
	require.NoError(t, err)

	session, err := srv.getOrCreateSession(clientAddr)
	require.NoError(t, err)
	require.NotNil(t, session)
	assert.Equal(t, clientAddr.String(), session.clientAddr.String())
	assert.NotNil(t, session.targetConn)

	srv.mu.RLock()
	stored, exists := srv.sessions[sessionKey(clientAddr)]
	srv.mu.RUnlock()
	assert.True(t, exists)
	assert.Equal(t, session, stored)

	srv.removeSession(clientAddr)
	srv.wg.Wait()
}

func TestHandleBackendResponse_Timeout(t *testing.T) {
	proxyAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	require.NoError(t, err)
	proxyConn, err := net.ListenUDP("udp", proxyAddr)
	require.NoError(t, err)
	defer proxyConn.Close()

	upstream := &Upstream{
		targets:  []*Target{{addr: "127.0.0.1:19108"}},
		balancer: newRoundRobin(),
	}
	upstream.targets[0].healthy.Store(true)
	srv := newUDPServer(proxyConn, upstream, 50*time.Millisecond)

	backendAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	require.NoError(t, err)
	backendConn, err := net.ListenUDP("udp", backendAddr)
	require.NoError(t, err)
	defer backendConn.Close()

	clientAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:54444")
	require.NoError(t, err)

	session := &udpSession{
		clientAddr: clientAddr,
		targetConn: backendConn,
		target:     upstream.targets[0],
		lastActive: time.Now().Add(-1 * time.Hour),
		srv:        srv,
	}

	srv.mu.Lock()
	srv.sessions[sessionKey(clientAddr)] = session
	srv.mu.Unlock()

	srv.wg.Add(1)
	go session.handleBackendResponse()

	done := make(chan struct{})
	go func() {
		srv.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		srv.mu.RLock()
		_, exists := srv.sessions[sessionKey(clientAddr)]
		srv.mu.RUnlock()
		assert.False(t, exists)
	case <-time.After(3 * time.Second):
		t.Fatal("handleBackendResponse did not finish in time")
	}
}

func TestServe_ReceivesAndForwards(t *testing.T) {
	backendAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	require.NoError(t, err)
	backendConn, err := net.ListenUDP("udp", backendAddr)
	require.NoError(t, err)
	defer backendConn.Close()

	go func() {
		buf := make([]byte, 65535)
		for {
			n, addr, readErr := backendConn.ReadFromUDP(buf)
			if readErr != nil {
				return
			}
			_, _ = backendConn.WriteToUDP(buf[:n], addr)
		}
	}()

	proxyAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	require.NoError(t, err)
	proxyConn, err := net.ListenUDP("udp", proxyAddr)
	require.NoError(t, err)
	defer proxyConn.Close()

	upstream := &Upstream{
		targets:  []*Target{{addr: backendConn.LocalAddr().String()}},
		balancer: newRoundRobin(),
	}
	upstream.targets[0].healthy.Store(true)
	srv := newUDPServer(proxyConn, upstream, 10*time.Second)

	srv.wg.Add(1)
	go func() {
		defer srv.wg.Done()
		srv.serve()
	}()
	srv.wg.Add(1)
	go func() {
		defer srv.wg.Done()
		srv.startCleanupTicker()
	}()

	clientConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	require.NoError(t, err)
	defer clientConn.Close()

	testData := []byte("test udp stream")
	proxyTarget := &net.UDPAddr{
		IP:   net.ParseIP("127.0.0.1"),
		Port: proxyConn.LocalAddr().(*net.UDPAddr).Port,
	}
	_, err = clientConn.WriteToUDP(testData, proxyTarget)
	require.NoError(t, err)

	buf := make([]byte, 65535)
	_ = clientConn.SetReadDeadline(time.Now().Add(3 * time.Second))
	n, _, err := clientConn.ReadFromUDP(buf)
	require.NoError(t, err)
	assert.Equal(t, testData, buf[:n])

	close(srv.stopCh)
	srv.running.Store(false)
	srv.wg.Wait()
}

func TestStartCleanupTicker_StopsOnSignal(t *testing.T) {
	t.Parallel()

	udpAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	require.NoError(t, err)
	conn, err := net.ListenUDP("udp", udpAddr)
	require.NoError(t, err)
	defer conn.Close()

	upstream := &Upstream{
		targets:  []*Target{{addr: "127.0.0.1:19110"}},
		balancer: newRoundRobin(),
	}
	srv := newUDPServer(conn, upstream, time.Minute)

	done := make(chan struct{})
	go func() {
		srv.startCleanupTicker()
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)
	close(srv.stopCh)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("startCleanupTicker did not stop after signal")
	}
}
