// Package utils 提供 IP 白名单工具函数的测试。
//
// 该文件测试 ParseIPAllowList、ParseCIDR、IPInAllowList 函数，包括：
//   - 空列表处理
//   - CIDR 格式解析
//   - 单 IP 自动转换
//   - localhost 特殊值
//   - 无效输入处理
//   - IP 匹配检查
//
// 作者：xfy
package utils

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseIPAllowList(t *testing.T) {
	tests := []struct {
		name    string
		input   []string
		wantLen int
		wantErr bool
	}{
		{"nil_input", nil, 0, false},
		{"empty_input", []string{}, 0, false},
		{"single_cidr_v4", []string{"192.168.1.0/24"}, 1, false},
		{"single_cidr_v6", []string{"::1/128"}, 1, false},
		{"single_ipv4", []string{"192.168.1.1"}, 1, false},
		{"single_ipv6", []string{"::1"}, 1, false},
		{"localhost_expansion", []string{"localhost"}, 2, false},
		{"multiple_entries", []string{"192.168.1.0/24", "10.0.0.1", "::1/128"}, 3, false},
		{"localhost_with_others", []string{"localhost", "10.0.0.0/8"}, 3, false},
		{"invalid_entry", []string{"not-an-ip"}, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseIPAllowList(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Len(t, result, tt.wantLen)
		})
	}
}

func TestParseIPAllowList_Localhost(t *testing.T) {
	result, err := ParseIPAllowList([]string{"localhost"})
	require.NoError(t, err)
	require.Len(t, result, 2)

	assert.Equal(t, "127.0.0.1/32", result[0].String())
	assert.Equal(t, "::1/128", result[1].String())
}

func TestParseIPAllowList_SingleIPv4(t *testing.T) {
	result, err := ParseIPAllowList([]string{"192.168.1.100"})
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, "192.168.1.100/32", result[0].String())
}

func TestParseIPAllowList_SingleIPv6(t *testing.T) {
	result, err := ParseIPAllowList([]string{"2001:db8::1"})
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, "2001:db8::1/128", result[0].String())
}

func TestParseCIDR(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		want   string
		wantIP string
	}{
		{"cidr_v4", "192.168.1.0/24", "192.168.1.0/24", "192.168.1.0"},
		{"cidr_v6", "::1/128", "::1/128", "::1"},
		{"single_ipv4", "10.0.0.1", "10.0.0.1/32", "10.0.0.1"},
		{"single_ipv6", "::1", "::1/128", "::1"},
		{"slash16", "172.16.0.0/16", "172.16.0.0/16", "172.16.0.0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			network, err := ParseCIDR(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.want, network.String())
			assert.Equal(t, tt.wantIP, network.IP.String())
		})
	}
}

func TestParseCIDR_Invalid(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"empty_string", ""},
		{"invalid_ip", "not-an-ip"},
		{"invalid_cidr", "192.168.1.0/33"},
		{"invalid_cidr_v6", "::1/129"},
		{"garbage", "hello/world"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseCIDR(tt.input)
			assert.Error(t, err)
		})
	}
}

func TestIPInAllowList(t *testing.T) {
	_, v4Net, _ := net.ParseCIDR("192.168.1.0/24")
	_, v6Net, _ := net.ParseCIDR("::1/128")
	_, net10, _ := net.ParseCIDR("10.0.0.0/8")

	allowList := []net.IPNet{*v4Net, *v6Net, *net10}

	tests := []struct {
		name      string
		ip        string
		wantMatch bool
	}{
		{"matching_v4_in_range", "192.168.1.100", true},
		{"matching_v4_boundary_start", "192.168.1.0", true},
		{"matching_v4_boundary_end", "192.168.1.255", true},
		{"matching_v6_localhost", "::1", true},
		{"matching_10_range", "10.50.0.1", true},
		{"non_matching_v4", "172.16.0.1", false},
		{"non_matching_v6", "2001:db8::1", false},
		{"non_matching_nearby", "192.168.2.1", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			require.NotNil(t, ip)
			result := IPInAllowList(ip, allowList)
			assert.Equal(t, tt.wantMatch, result)
		})
	}
}

func TestIPInAllowList_EmptyList(t *testing.T) {
	ip := net.ParseIP("192.168.1.1")
	require.NotNil(t, ip)
	assert.False(t, IPInAllowList(ip, nil))
	assert.False(t, IPInAllowList(ip, []net.IPNet{}))
}

func TestIPInAllowList_SingleEntry(t *testing.T) {
	_, network, err := net.ParseCIDR("127.0.0.1/32")
	require.NoError(t, err)
	allowList := []net.IPNet{*network}

	assert.True(t, IPInAllowList(net.ParseIP("127.0.0.1"), allowList))
	assert.False(t, IPInAllowList(net.ParseIP("127.0.0.2"), allowList))
}

func TestParseIPAllowList_Integration(t *testing.T) {
	result, err := ParseIPAllowList([]string{"192.168.1.0/24", "10.0.0.1", "localhost"})
	require.NoError(t, err)
	require.Len(t, result, 4)

	assert.True(t, IPInAllowList(net.ParseIP("192.168.1.50"), result))
	assert.True(t, IPInAllowList(net.ParseIP("10.0.0.1"), result))
	assert.True(t, IPInAllowList(net.ParseIP("127.0.0.1"), result))
	assert.True(t, IPInAllowList(net.ParseIP("::1"), result))
	assert.False(t, IPInAllowList(net.ParseIP("8.8.8.8"), result))
}
