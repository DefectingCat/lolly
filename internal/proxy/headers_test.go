package proxy

import (
	"strings"
	"testing"

	"github.com/valyala/fasthttp"
)

// TestSetForwardedHeaders_SetHost 测试 SetForwardedHost 配置对 X-Forwarded-Host 头的控制
func TestSetForwardedHeaders_SetHost(t *testing.T) {
	tests := []struct {
		name       string
		setHost    bool
		expectHost bool
	}{
		{
			name:       "setHost=true sets X-Forwarded-Host",
			setHost:    true,
			expectHost: true,
		},
		{
			name:       "setHost=false does not set X-Forwarded-Host",
			setHost:    false,
			expectHost: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			headers := &fasthttp.RequestHeader{}
			fh := ForwardedHeaders{
				ClientIP: "192.168.1.1",
				Host:     "example.com:8080",
				Proto:    "https",
			}

			// setProto=true 因为我们要测试 setHost 的效果
			SetForwardedHeaders(headers, fh, true, tt.setHost, true)

			hasHost := len(headers.Peek("X-Forwarded-Host")) > 0
			if hasHost != tt.expectHost {
				t.Errorf("X-Forwarded-Host presence = %v, want %v", hasHost, tt.expectHost)
			}

			// X-Forwarded-For 和 X-Real-IP 应该始终设置
			if len(headers.Peek("X-Forwarded-For")) == 0 {
				t.Error("X-Forwarded-For should always be set")
			}
			if len(headers.Peek("X-Real-IP")) == 0 {
				t.Error("X-Real-IP should always be set")
			}
		})
	}
}

// TestSetForwardedHeaders_SetProto 测试 SetForwardedProto 配置对 X-Forwarded-Proto 头的控制
func TestSetForwardedHeaders_SetProto(t *testing.T) {
	tests := []struct {
		name        string
		setProto    bool
		expectProto bool
	}{
		{
			name:        "setProto=true sets X-Forwarded-Proto",
			setProto:    true,
			expectProto: true,
		},
		{
			name:        "setProto=false does not set X-Forwarded-Proto",
			setProto:    false,
			expectProto: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			headers := &fasthttp.RequestHeader{}
			fh := ForwardedHeaders{
				ClientIP: "192.168.1.1",
				Host:     "example.com:8080",
				Proto:    "https",
			}

			// setHost=true 因为我们要测试 setProto 的效果
			SetForwardedHeaders(headers, fh, true, true, tt.setProto)

			hasProto := len(headers.Peek("X-Forwarded-Proto")) > 0
			if hasProto != tt.expectProto {
				t.Errorf("X-Forwarded-Proto presence = %v, want %v", hasProto, tt.expectProto)
			}
		})
	}
}

// TestSetForwardedHeaders_DefaultBehavior 测试默认行为（所有参数为 true）
func TestSetForwardedHeaders_DefaultBehavior(t *testing.T) {
	headers := &fasthttp.RequestHeader{}
	fh := ForwardedHeaders{
		ClientIP: "10.0.0.1",
		Host:     "localhost:8082",
		Proto:    "http",
	}

	SetForwardedHeaders(headers, fh, true, true, true)

	// 验证所有头都被设置
	if string(headers.Peek("X-Forwarded-For")) != "10.0.0.1" {
		t.Errorf("X-Forwarded-For = %s, want 10.0.0.1", headers.Peek("X-Forwarded-For"))
	}
	if string(headers.Peek("X-Real-IP")) != "10.0.0.1" {
		t.Errorf("X-Real-IP = %s, want 10.0.0.1", headers.Peek("X-Real-IP"))
	}
	if string(headers.Peek("X-Forwarded-Host")) != "localhost:8082" {
		t.Errorf("X-Forwarded-Host = %s, want localhost:8082", headers.Peek("X-Forwarded-Host"))
	}
	if string(headers.Peek("X-Forwarded-Proto")) != "http" {
		t.Errorf("X-Forwarded-Proto = %s, want http", headers.Peek("X-Forwarded-Proto"))
	}
}

// TestSetForwardedHeaders_AllDisabled 测试所有控制参数为 false
func TestSetForwardedHeaders_AllDisabled(t *testing.T) {
	headers := &fasthttp.RequestHeader{}
	fh := ForwardedHeaders{
		ClientIP: "10.0.0.1",
		Host:     "localhost:8082",
		Proto:    "http",
	}

	SetForwardedHeaders(headers, fh, true, false, false)

	// X-Forwarded-For 和 X-Real-IP 应该始终设置
	if len(headers.Peek("X-Forwarded-For")) == 0 {
		t.Error("X-Forwarded-For should be set even when setHost/setProto are false")
	}
	if len(headers.Peek("X-Real-IP")) == 0 {
		t.Error("X-Real-IP should be set even when setHost/setProto are false")
	}

	// X-Forwarded-Host 和 X-Forwarded-Proto 不应该设置
	if len(headers.Peek("X-Forwarded-Host")) > 0 {
		t.Error("X-Forwarded-Host should not be set when setHost=false")
	}
	if len(headers.Peek("X-Forwarded-Proto")) > 0 {
		t.Error("X-Forwarded-Proto should not be set when setProto=false")
	}
}

// TestWriteForwardedHeaders_SetHost 测试 WriteForwardedHeaders 的 setHost 参数
func TestWriteForwardedHeaders_SetHost(t *testing.T) {
	tests := []struct {
		name       string
		setHost    bool
		expectHost bool
	}{
		{
			name:       "setHost=true writes X-Forwarded-Host",
			setHost:    true,
			expectHost: true,
		},
		{
			name:       "setHost=false does not write X-Forwarded-Host",
			setHost:    false,
			expectHost: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var builder strings.Builder
			fh := ForwardedHeaders{
				ClientIP: "192.168.1.1",
				Host:     "example.com:8080",
				Proto:    "https",
			}

			WriteForwardedHeaders(&builder, fh, tt.setHost, true)

			result := builder.String()
			hasHost := strings.Contains(result, "X-Forwarded-Host:")
			if hasHost != tt.expectHost {
				t.Errorf("X-Forwarded-Host presence = %v, want %v", hasHost, tt.expectHost)
			}

			// X-Forwarded-For 和 X-Real-IP 应该始终存在
			if !strings.Contains(result, "X-Forwarded-For:") {
				t.Error("X-Forwarded-For should always be written")
			}
			if !strings.Contains(result, "X-Real-IP:") {
				t.Error("X-Real-IP should always be written")
			}
		})
	}
}

// TestWriteForwardedHeaders_SetProto 测试 WriteForwardedHeaders 的 setProto 参数
func TestWriteForwardedHeaders_SetProto(t *testing.T) {
	tests := []struct {
		name        string
		setProto    bool
		expectProto bool
	}{
		{
			name:        "setProto=true writes X-Forwarded-Proto",
			setProto:    true,
			expectProto: true,
		},
		{
			name:        "setProto=false does not write X-Forwarded-Proto",
			setProto:    false,
			expectProto: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var builder strings.Builder
			fh := ForwardedHeaders{
				ClientIP: "192.168.1.1",
				Host:     "example.com:8080",
				Proto:    "https",
			}

			WriteForwardedHeaders(&builder, fh, true, tt.setProto)

			result := builder.String()
			hasProto := strings.Contains(result, "X-Forwarded-Proto:")
			if hasProto != tt.expectProto {
				t.Errorf("X-Forwarded-Proto presence = %v, want %v", hasProto, tt.expectProto)
			}
		})
	}
}

// TestWriteForwardedHeaders_AllDisabled 测试 WriteForwardedHeaders 所有控制参数为 false
func TestWriteForwardedHeaders_AllDisabled(t *testing.T) {
	var builder strings.Builder
	fh := ForwardedHeaders{
		ClientIP: "10.0.0.1",
		Host:     "localhost:8082",
		Proto:    "http",
	}

	WriteForwardedHeaders(&builder, fh, false, false)

	result := builder.String()

	// X-Forwarded-For 和 X-Real-IP 应该始终存在
	if !strings.Contains(result, "X-Forwarded-For:") {
		t.Error("X-Forwarded-For should always be written")
	}
	if !strings.Contains(result, "X-Real-IP:") {
		t.Error("X-Real-IP should always be written")
	}

	// X-Forwarded-Host 和 X-Forwarded-Proto 不应该存在
	if strings.Contains(result, "X-Forwarded-Host:") {
		t.Error("X-Forwarded-Host should not be written when setHost=false")
	}
	if strings.Contains(result, "X-Forwarded-Proto:") {
		t.Error("X-Forwarded-Proto should not be written when setProto=false")
	}
}
