// Package config 提供 YAML 配置文件的解析、验证和默认配置生成功能测试。
//
// 该文件包含 GeoIP 配置验证相关的测试。
//
// 作者：xfy
package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestValidateGeoIP 测试 GeoIP 配置验证。
func TestValidateGeoIP(t *testing.T) {
	tests := []struct {
		name    string
		errMsg  string
		config  GeoIPConfig
		wantErr bool
	}{
		{
			name: "未启用时跳过验证",
			config: GeoIPConfig{
				Enabled: false,
			},
			wantErr: false,
		},
		{
			name: "启用但缺少数据库路径",
			config: GeoIPConfig{
				Enabled:  true,
				Database: "",
			},
			wantErr: true,
			errMsg:  "database 是必填项",
		},
		{
			name: "有效的 GeoIP 配置",
			config: GeoIPConfig{
				Enabled:           true,
				Database:          "/var/lib/geoip/GeoIP2-Country.mmdb",
				AllowCountries:    []string{"US", "JP"},
				DenyCountries:     []string{"CN"},
				Default:           "deny",
				CacheSize:         10000,
				PrivateIPBehavior: "allow",
			},
			wantErr: false,
		},
		{
			name: "无效的国家代码（小写）",
			config: GeoIPConfig{
				Enabled:        true,
				Database:       "/var/lib/geoip/GeoIP2-Country.mmdb",
				AllowCountries: []string{"us"},
			},
			wantErr: true,
			errMsg:  "无效的 allow_countries",
		},
		{
			name: "无效的国家代码（3位）",
			config: GeoIPConfig{
				Enabled:       true,
				Database:      "/var/lib/geoip/GeoIP2-Country.mmdb",
				DenyCountries: []string{"USA"},
			},
			wantErr: true,
			errMsg:  "无效的 deny_countries",
		},
		{
			name: "无效的 private_ip_behavior",
			config: GeoIPConfig{
				Enabled:           true,
				Database:          "/var/lib/geoip/GeoIP2-Country.mmdb",
				PrivateIPBehavior: "invalid",
			},
			wantErr: true,
			errMsg:  "无效的 private_ip_behavior",
		},
		{
			name: "负的 cache_size",
			config: GeoIPConfig{
				Enabled:   true,
				Database:  "/var/lib/geoip/GeoIP2-Country.mmdb",
				CacheSize: -1,
			},
			wantErr: true,
			errMsg:  "cache_size 不能为负数",
		},
		{
			name: "负的 cache_ttl",
			config: GeoIPConfig{
				Enabled:  true,
				Database: "/var/lib/geoip/GeoIP2-Country.mmdb",
				CacheTTL: -1,
			},
			wantErr: true,
			errMsg:  "cache_ttl 不能为负数",
		},
		{
			name: "无效的 default 动作",
			config: GeoIPConfig{
				Enabled:  true,
				Database: "/var/lib/geoip/GeoIP2-Country.mmdb",
				Default:  "invalid",
			},
			wantErr: true,
			errMsg:  "无效的 default",
		},
		{
			name: "有效的 private_ip_behavior: deny",
			config: GeoIPConfig{
				Enabled:           true,
				Database:          "/var/lib/geoip/GeoIP2-Country.mmdb",
				PrivateIPBehavior: "deny",
			},
			wantErr: false,
		},
		{
			name: "有效的 private_ip_behavior: bypass",
			config: GeoIPConfig{
				Enabled:           true,
				Database:          "/var/lib/geoip/GeoIP2-Country.mmdb",
				PrivateIPBehavior: "bypass",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateGeoIP(&tt.config)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestIsValidCountryCode 测试国家代码验证。
func TestIsValidCountryCode(t *testing.T) {
	tests := []struct {
		code     string
		expected bool
	}{
		{"US", true},
		{"JP", true},
		{"GB", true},
		{"CN", true},
		{"us", false},  // 小写
		{"Us", false},  // 混合大小写
		{"USA", false}, // 3位
		{"U", false},   // 1位
		{"U1", false},  // 包含数字
		{"U-", false},  // 包含连字符
		{"", false},    // 空字符串
	}

	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			result := isValidCountryCode(tt.code)
			assert.Equal(t, tt.expected, result)
		})
	}
}
