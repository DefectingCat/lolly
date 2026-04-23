//go:build e2e

// Package testutil 提供 E2E 测试的工具函数。
//
// 包含 WebSocket 测试辅助工具。
//
// 作者：xfy
package testutil

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// WSClient WebSocket 测试客户端。
//
// 封装 gorilla/websocket，提供简单的测试接口。
type WSClient struct {
	conn      *websocket.Conn
	url       string
	mu        sync.Mutex
	closed    bool
	closeChan chan struct{}
}

// WSOption WebSocket 客户端选项。
type WSOption func(*wsConfig)

type wsConfig struct {
	headers    http.Header
	pingPeriod time.Duration
	pongWait   time.Duration
}

// WithHeaders 设置请求头。
func WithWSHeaders(headers http.Header) WSOption {
	return func(c *wsConfig) {
		c.headers = headers
	}
}

// WithWSTimeout 设置超时时间。
func WithWSTimeout(pongWait, pingPeriod time.Duration) WSOption {
	return func(c *wsConfig) {
		c.pongWait = pongWait
		c.pingPeriod = pingPeriod
	}
}

// NewWSClient 创建 WebSocket 客户端。
//
// 参数：
//   - ctx: 上下文
//   - url: WebSocket URL（ws:// 或 wss://）
//   - opts: 可选配置
//
// 返回 WebSocket 客户端实例。
func NewWSClient(ctx context.Context, url string, opts ...WSOption) (*WSClient, error) {
	cfg := &wsConfig{
		headers:    http.Header{},
		pongWait:   60 * time.Second,
		pingPeriod: 54 * time.Second,
	}

	for _, opt := range opts {
		opt(cfg)
	}

	dialer := websocket.DefaultDialer
	conn, _, err := dialer.DialContext(ctx, url, cfg.headers)
	if err != nil {
		return nil, fmt.Errorf("websocket dial failed: %w", err)
	}

	client := &WSClient{
		conn:      conn,
		url:       url,
		closeChan: make(chan struct{}),
	}

	// 设置 pong 处理
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(cfg.pongWait))
	})

	return client, nil
}

// Send 发送文本消息。
func (c *WSClient) Send(message string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return fmt.Errorf("connection closed")
	}

	c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return c.conn.WriteMessage(websocket.TextMessage, []byte(message))
}

// SendBinary 发送二进制消息。
func (c *WSClient) SendBinary(data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return fmt.Errorf("connection closed")
	}

	c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return c.conn.WriteMessage(websocket.BinaryMessage, data)
}

// SendJSON 发送 JSON 消息。
func (c *WSClient) SendJSON(v interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return fmt.Errorf("connection closed")
	}

	c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return c.conn.WriteJSON(v)
}

// Receive 接收文本消息。
//
// 返回消息内容和错误。
func (c *WSClient) Receive() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return "", fmt.Errorf("connection closed")
	}

	c.conn.SetReadDeadline(time.Now().Add(30 * time.Second))
	messageType, data, err := c.conn.ReadMessage()
	if err != nil {
		return "", err
	}

	if messageType != websocket.TextMessage {
		return "", fmt.Errorf("expected text message, got %d", messageType)
	}

	return string(data), nil
}

// ReceiveBinary 接收二进制消息。
func (c *WSClient) ReceiveBinary() ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil, fmt.Errorf("connection closed")
	}

	c.conn.SetReadDeadline(time.Now().Add(30 * time.Second))
	messageType, data, err := c.conn.ReadMessage()
	if err != nil {
		return nil, err
	}

	if messageType != websocket.BinaryMessage {
		return nil, fmt.Errorf("expected binary message, got %d", messageType)
	}

	return data, nil
}

// ReceiveJSON 接收 JSON 消息。
func (c *WSClient) ReceiveJSON(v interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return fmt.Errorf("connection closed")
	}

	c.conn.SetReadDeadline(time.Now().Add(30 * time.Second))
	return c.conn.ReadJSON(v)
}

// ReceiveWithTimeout 接收消息（带超时）。
func (c *WSClient) ReceiveWithTimeout(timeout time.Duration) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return "", fmt.Errorf("connection closed")
	}

	c.conn.SetReadDeadline(time.Now().Add(timeout))
	messageType, data, err := c.conn.ReadMessage()
	if err != nil {
		return "", err
	}

	if messageType != websocket.TextMessage {
		return "", fmt.Errorf("expected text message, got %d", messageType)
	}

	return string(data), nil
}

// ReceiveChan 返回消息通道。
//
// 在后台持续接收消息，通过通道返回。
func (c *WSClient) ReceiveChan() <-chan WSMessage {
	ch := make(chan WSMessage, 10)

	go func() {
		defer close(ch)
		for {
			msg, err := c.Receive()
			if err != nil {
				return
			}
			ch <- WSMessage{Data: msg}
		}
	}()

	return ch
}

// WSMessage WebSocket 消息。
type WSMessage struct {
	Data  string
	Error error
}

// Close 关闭连接。
func (c *WSClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}

	c.closed = true
	close(c.closeChan)

	// 发送关闭帧
	err := c.conn.WriteMessage(websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	if err != nil {
		c.conn.Close()
		return err
	}

	return c.conn.Close()
}

// IsClosed 检查连接是否已关闭。
func (c *WSClient) IsClosed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}

// URL 返回连接 URL。
func (c *WSClient) URL() string {
	return c.url
}

// CloseChan 返回关闭通道。
func (c *WSClient) CloseChan() <-chan struct{} {
	return c.closeChan
}

// WSPool WebSocket 连接池。
//
// 管理多个 WebSocket 连接，用于并发测试。
type WSPool struct {
	clients []*WSClient
	mu      sync.Mutex
}

// NewWSPool 创建 WebSocket 连接池。
//
// 参数：
//   - ctx: 上下文
//   - url: WebSocket URL
//   - count: 连接数量
//
// 返回连接池实例。
func NewWSPool(ctx context.Context, url string, count int) (*WSPool, error) {
	pool := &WSPool{
		clients: make([]*WSClient, count),
	}

	for i := 0; i < count; i++ {
		client, err := NewWSClient(ctx, url)
		if err != nil {
			pool.Close()
			return nil, fmt.Errorf("failed to create client %d: %w", i, err)
		}
		pool.clients[i] = client
	}

	return pool, nil
}

// SendAll 向所有连接发送消息。
func (p *WSPool) SendAll(message string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	var lastErr error
	for _, client := range p.clients {
		if client != nil {
			if err := client.Send(message); err != nil {
				lastErr = err
			}
		}
	}
	return lastErr
}

// SendOne 向指定连接发送消息。
func (p *WSPool) SendOne(index int, message string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if index < 0 || index >= len(p.clients) {
		return fmt.Errorf("invalid index %d", index)
	}

	if p.clients[index] == nil {
		return fmt.Errorf("client %d is nil", index)
	}

	return p.clients[index].Send(message)
}

// ReceiveAll 从所有连接接收消息。
//
// 返回每个连接收到的消息列表。
func (p *WSPool) ReceiveAll() ([]string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	messages := make([]string, len(p.clients))
	var lastErr error

	for i, client := range p.clients {
		if client != nil {
			msg, err := client.Receive()
			if err != nil {
				lastErr = err
				messages[i] = ""
			} else {
				messages[i] = msg
			}
		}
	}

	return messages, lastErr
}

// Count 返回连接数量。
func (p *WSPool) Count() int {
	return len(p.clients)
}

// Close 关闭所有连接。
func (p *WSPool) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	var lastErr error
	for _, client := range p.clients {
		if client != nil {
			if err := client.Close(); err != nil {
				lastErr = err
			}
		}
	}
	return lastErr
}

// WSEchoServer WebSocket Echo 服务器配置。
type WSEchoServer struct {
	Port    int
	Handler func(*websocket.Conn)
}

// NewWSEchoHandler 创建 Echo 处理器。
//
// 将收到的消息原样返回。
func NewWSEchoHandler() func(*websocket.Conn) {
	return func(conn *websocket.Conn) {
		defer conn.Close()
		for {
			messageType, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			conn.WriteMessage(messageType, data)
		}
	}
}
