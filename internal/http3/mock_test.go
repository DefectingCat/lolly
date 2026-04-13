// Package http3 提供测试用的 Mock 实现
package http3

import (
	"context"
	"net"

	"github.com/quic-go/quic-go"
)

// MockQUICListener 是 QUIC 监听器的 Mock 实现
type MockQUICListener struct {
	AcceptFunc func(ctx context.Context) (*quic.Conn, error)
	CloseFunc  func() error
	AddrFunc   func() net.Addr
}

// Accept 接受连接
func (m *MockQUICListener) Accept(ctx context.Context) (*quic.Conn, error) {
	if m.AcceptFunc != nil {
		return m.AcceptFunc(ctx)
	}
	return nil, nil
}

// Close 关闭监听器
func (m *MockQUICListener) Close() error {
	if m.CloseFunc != nil {
		return m.CloseFunc()
	}
	return nil
}

// Addr 返回监听地址
func (m *MockQUICListener) Addr() net.Addr {
	if m.AddrFunc != nil {
		return m.AddrFunc()
	}
	return &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 443}
}
