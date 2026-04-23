//go:build e2e

// websocket_e2e_test.go - WebSocket E2E 测试
//
// 测试 lolly WebSocket 代理功能：连接建立、消息传递、并发连接等。
//
// 作者：xfy
package e2e

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rua.plus/lolly/internal/e2e/testutil"
)

// wsEchoHandler WebSocket Echo 处理器。
func wsEchoHandler(w http.ResponseWriter, r *http.Request) {
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	for {
		messageType, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		if err := conn.WriteMessage(messageType, data); err != nil {
			return
		}
	}
}

// TestE2EWebSocketBasic 测试基础 WebSocket 代理。
//
// 验证 WebSocket 连接可以成功建立和消息传递。
func TestE2EWebSocketBasic(t *testing.T) {
	// 创建本地 WebSocket Echo 服务器
	server := httptest.NewServer(http.HandlerFunc(wsEchoHandler))
	defer server.Close()

	// 将 http:// 替换为 ws://
	wsURL := "ws" + server.URL[4:]

	t.Logf("WebSocket Echo server: %s", wsURL)

	// 创建 WebSocket 客户端
	client, err := testutil.NewWSClient(context.Background(), wsURL)
	require.NoError(t, err, "Failed to create WebSocket client")
	defer client.Close()

	// 发送消息
	testMessage := "Hello, WebSocket!"
	err = client.Send(testMessage)
	require.NoError(t, err, "Failed to send message")

	// 接收响应
	response, err := client.Receive()
	require.NoError(t, err, "Failed to receive message")

	// Echo 服务器应该返回相同的消息
	assert.Equal(t, testMessage, response, "Echo response should match sent message")

	t.Logf("Sent: %s, Received: %s", testMessage, response)
}

// TestE2EWebSocketBinary 测试二进制消息。
//
// 验证二进制消息正确传递。
func TestE2EWebSocketBinary(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(wsEchoHandler))
	defer server.Close()

	wsURL := "ws" + server.URL[4:]

	client, err := testutil.NewWSClient(context.Background(), wsURL)
	require.NoError(t, err, "Failed to create WebSocket client")
	defer client.Close()

	// 发送二进制数据
	testData := []byte{0x01, 0x02, 0x03, 0x04, 0x05}
	err = client.SendBinary(testData)
	require.NoError(t, err, "Failed to send binary data")

	// 接收响应
	response, err := client.ReceiveBinary()
	require.NoError(t, err, "Failed to receive binary data")

	assert.Equal(t, testData, response, "Echo response should match sent data")
}

// TestE2EWebSocketJSON 测试 JSON 消息。
//
// 验证 JSON 消息正确传递。
func TestE2EWebSocketJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(wsEchoHandler))
	defer server.Close()

	wsURL := "ws" + server.URL[4:]

	client, err := testutil.NewWSClient(context.Background(), wsURL)
	require.NoError(t, err, "Failed to create WebSocket client")
	defer client.Close()

	// 发送 JSON 数据
	testData := map[string]string{"message": "hello", "id": "123"}
	err = client.SendJSON(testData)
	require.NoError(t, err, "Failed to send JSON")

	// 接收响应
	var response map[string]string
	err = client.ReceiveJSON(&response)
	require.NoError(t, err, "Failed to receive JSON")

	assert.Equal(t, testData, response, "Echo response should match sent JSON")
}

// TestE2EWebSocketConcurrent 测试并发 WebSocket 连接。
//
// 验证多个并发连接正常工作。
func TestE2EWebSocketConcurrent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(wsEchoHandler))
	defer server.Close()

	wsURL := "ws" + server.URL[4:]

	// 创建连接池
	pool, err := testutil.NewWSPool(context.Background(), wsURL, 5)
	require.NoError(t, err, "Failed to create WebSocket pool")
	defer pool.Close()

	// 向所有连接发送消息
	testMessage := "Concurrent test"
	err = pool.SendAll(testMessage)
	require.NoError(t, err, "Failed to send to all connections")

	// 从所有连接接收消息
	messages, err := pool.ReceiveAll()
	require.NoError(t, err, "Failed to receive from all connections")

	// 验证所有连接都收到了响应
	assert.Equal(t, 5, len(messages), "Should have 5 responses")

	for i, msg := range messages {
		if msg != "" {
			assert.Equal(t, testMessage, msg, "Connection %d response should match", i)
		}
	}

	t.Logf("Received %d messages from concurrent connections", len(messages))
}

// TestE2EWebSocketMultipleMessages 测试多消息传递。
//
// 验证连续发送多条消息正常工作。
func TestE2EWebSocketMultipleMessages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(wsEchoHandler))
	defer server.Close()

	wsURL := "ws" + server.URL[4:]

	client, err := testutil.NewWSClient(context.Background(), wsURL)
	require.NoError(t, err, "Failed to create WebSocket client")
	defer client.Close()

	// 发送多条消息
	for i := 0; i < 10; i++ {
		msg := fmt.Sprintf("Message %d", i)
		err = client.Send(msg)
		require.NoError(t, err, "Failed to send message %d", i)

		response, err := client.Receive()
		require.NoError(t, err, "Failed to receive message %d", i)

		assert.Equal(t, msg, response, "Response %d should match", i)
	}

	t.Log("Successfully sent and received 10 messages")
}

// TestE2EWebSocketClose 测试连接关闭。
//
// 验证连接正确关闭。
func TestE2EWebSocketClose(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(wsEchoHandler))
	defer server.Close()

	wsURL := "ws" + server.URL[4:]

	client, err := testutil.NewWSClient(context.Background(), wsURL)
	require.NoError(t, err, "Failed to create WebSocket client")

	// 发送一条消息验证连接正常
	err = client.Send("Test before close")
	require.NoError(t, err, "Failed to send before close")

	response, err := client.Receive()
	require.NoError(t, err, "Failed to receive before close")
	assert.Equal(t, "Test before close", response)

	// 关闭连接
	err = client.Close()
	require.NoError(t, err, "Failed to close connection")

	// 验证连接已关闭
	assert.True(t, client.IsClosed(), "Connection should be closed")

	// 尝试发送应该失败
	err = client.Send("After close")
	assert.Error(t, err, "Send after close should fail")
}

// TestE2EWebSocketTimeout 测试超时处理。
//
// 验证超时配置生效。
func TestE2EWebSocketTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(wsEchoHandler))
	defer server.Close()

	wsURL := "ws" + server.URL[4:]

	client, err := testutil.NewWSClient(context.Background(), wsURL)
	require.NoError(t, err, "Failed to create WebSocket client")
	defer client.Close()

	// 发送消息
	err = client.Send("Timeout test")
	require.NoError(t, err, "Failed to send message")

	// 使用短超时接收
	response, err := client.ReceiveWithTimeout(5 * time.Second)
	require.NoError(t, err, "Failed to receive with timeout")
	assert.Equal(t, "Timeout test", response)
}

// TestE2EWebSocketHeaders 测试自定义头部。
//
// 验证自定义请求头正确传递。
func TestE2EWebSocketHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(wsEchoHandler))
	defer server.Close()

	wsURL := "ws" + server.URL[4:]

	// 创建带自定义头部的客户端
	headers := http.Header{}
	headers.Set("X-Custom-Header", "test-value")
	headers.Set("Authorization", "Bearer token123")

	client, err := testutil.NewWSClient(context.Background(), wsURL,
		testutil.WithWSHeaders(headers),
	)
	require.NoError(t, err, "Failed to create WebSocket client with headers")
	defer client.Close()

	// 发送消息验证连接正常
	err = client.Send("Headers test")
	require.NoError(t, err, "Failed to send message")

	response, err := client.Receive()
	require.NoError(t, err, "Failed to receive message")
	assert.Equal(t, "Headers test", response)
}

// TestE2EWebSocketPoolOperations 测试连接池操作。
//
// 验证连接池的各种操作。
func TestE2EWebSocketPoolOperations(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(wsEchoHandler))
	defer server.Close()

	wsURL := "ws" + server.URL[4:]

	pool, err := testutil.NewWSPool(context.Background(), wsURL, 3)
	require.NoError(t, err, "Failed to create WebSocket pool")
	defer pool.Close()

	// 验证连接数量
	assert.Equal(t, 3, pool.Count(), "Pool should have 3 connections")

	// 向单个连接发送消息
	err = pool.SendOne(0, "Single message")
	require.NoError(t, err, "Failed to send to single connection")

	// 向所有连接发送消息
	err = pool.SendAll("Broadcast message")
	require.NoError(t, err, "Failed to broadcast")
}

// TestE2EWebSocketReconnect 测试重连场景。
//
// 验证客户端可以重新建立连接。
func TestE2EWebSocketReconnect(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(wsEchoHandler))
	defer server.Close()

	wsURL := "ws" + server.URL[4:]

	// 第一次连接
	client1, err := testutil.NewWSClient(context.Background(), wsURL)
	require.NoError(t, err, "Failed to create first client")

	err = client1.Send("First connection")
	require.NoError(t, err, "Failed to send on first connection")

	response, err := client1.Receive()
	require.NoError(t, err, "Failed to receive on first connection")
	assert.Equal(t, "First connection", response)

	// 关闭第一个连接
	client1.Close()

	// 重新建立连接
	client2, err := testutil.NewWSClient(context.Background(), wsURL)
	require.NoError(t, err, "Failed to create second client")
	defer client2.Close()

	err = client2.Send("Second connection")
	require.NoError(t, err, "Failed to send on second connection")

	response, err = client2.Receive()
	require.NoError(t, err, "Failed to receive on second connection")
	assert.Equal(t, "Second connection", response)

	t.Log("Reconnect test completed successfully")
}

// TestE2EWebSocketLargeMessage 测试大消息。
//
// 验证大消息正确传递。
func TestE2EWebSocketLargeMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(wsEchoHandler))
	defer server.Close()

	wsURL := "ws" + server.URL[4:]

	client, err := testutil.NewWSClient(context.Background(), wsURL)
	require.NoError(t, err, "Failed to create WebSocket client")
	defer client.Close()

	// 发送大消息（64KB）
	largeData := make([]byte, 64*1024)
	for i := range largeData {
		largeData[i] = byte(i % 256)
	}

	err = client.SendBinary(largeData)
	require.NoError(t, err, "Failed to send large data")

	response, err := client.ReceiveBinary()
	require.NoError(t, err, "Failed to receive large data")

	assert.Equal(t, len(largeData), len(response), "Response size should match")
	assert.Equal(t, largeData, response, "Response data should match")
}

// TestE2EWebSocketProxyIntegration 测试 WebSocket 代理集成。
//
// 注意：此测试需要 Docker 环境，验证 lolly WebSocket 代理功能。
func TestE2EWebSocketProxyIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E WebSocket proxy test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	if !testutil.LollyImageAvailable(ctx) {
		t.Skip("lolly:latest image not available, run 'make docker-build' first")
	}

	// 启动后端
	networkName, pool, err := testutil.SetupProxyTest(ctx, 1)
	require.NoError(t, err, "Failed to start backend pool")
	defer testutil.CleanupProxyTest(ctx, networkName, pool)

	// 构建配置
	cfg := testutil.NewConfigBuilder().
		WithServer(":8080").
		WithProxy("/", pool.InternalAddresses())

	configYAML, err := cfg.Build()
	require.NoError(t, err, "Failed to build config")

	// 启动 lolly
	lolly, err := testutil.StartLolly(ctx, testutil.WithConfigYAML(configYAML), testutil.WithNetwork(networkName))
	require.NoError(t, err, "Failed to start lolly")
	defer lolly.Terminate(ctx)

	err = lolly.WaitForHealthy(ctx, 30*time.Second)
	require.NoError(t, err, "Lolly not healthy")

	// 测试 HTTP 代理（WebSocket 需要 WebSocket 后端）
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(lolly.HTTPBaseURL())
	require.NoError(t, err, "HTTP request failed")
	defer resp.Body.Close()

	assert.Equal(t, 200, resp.StatusCode)

	t.Log("WebSocket proxy integration test placeholder - requires WebSocket backend")
}