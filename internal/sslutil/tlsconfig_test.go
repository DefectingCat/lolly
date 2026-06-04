// Package sslutil 提供 TLS 配置解析函数的测试。
//
// 该文件测试 tlsconfig.go 中的所有导出函数和变量，包括：
//   - ParseTLSVersions: TLS 版本字符串解析
//   - ParseMinTLSVersion: 最低 TLS 版本解析
//   - ParseCipherSuites: 密码套件名称解析
//   - ParseCipherSuitesLenient: 宽松模式密码套件解析
//   - IsInsecureCipher: 不安全密码套件检测
//   - DefaultCipherSuites: 默认密码套件列表
//   - TLSVersionMap: TLS 版本映射表
package sslutil

import (
	"crypto/tls"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTLSVersions(t *testing.T) {
	tests := []struct {
		name      string
		protocols []string
		wantMin   uint16
		wantMax   uint16
		wantErr   bool
	}{
		{
			name:      "empty_returns_default_tls13",
			protocols: []string{},
			wantMin:   tls.VersionTLS13,
			wantMax:   tls.VersionTLS13,
		},
		{
			name:      "nil_returns_default_tls13",
			protocols: nil,
			wantMin:   tls.VersionTLS13,
			wantMax:   tls.VersionTLS13,
		},
		{
			name:      "only_tls12",
			protocols: []string{"TLSv1.2"},
			wantMin:   tls.VersionTLS12,
			wantMax:   tls.VersionTLS13,
		},
		{
			name:      "only_tls13",
			protocols: []string{"TLSv1.3"},
			wantMin:   tls.VersionTLS13,
			wantMax:   tls.VersionTLS13,
		},
		{
			name:      "tls12_and_tls13",
			protocols: []string{"TLSv1.2", "TLSv1.3"},
			wantMin:   tls.VersionTLS12,
			wantMax:   tls.VersionTLS13,
		},
		{
			name:      "tls13_and_tls12_order_independent",
			protocols: []string{"TLSv1.3", "TLSv1.2"},
			wantMin:   tls.VersionTLS12,
			wantMax:   tls.VersionTLS13,
		},
		{
			name:      "case_insensitive_upper_tls12",
			protocols: []string{"TLSV1.2"},
			wantMin:   tls.VersionTLS12,
			wantMax:   tls.VersionTLS13,
		},
		{
			name:      "case_insensitive_upper_tls13",
			protocols: []string{"TLSV1.3"},
			wantMin:   tls.VersionTLS13,
			wantMax:   tls.VersionTLS13,
		},
		{
			name:      "insecure_tls10_error",
			protocols: []string{"TLSv1.0"},
			wantErr:   true,
		},
		{
			name:      "insecure_tls10_upper_error",
			protocols: []string{"TLSV1.0"},
			wantErr:   true,
		},
		{
			name:      "insecure_tls11_error",
			protocols: []string{"TLSv1.1"},
			wantErr:   true,
		},
		{
			name:      "insecure_tls11_upper_error",
			protocols: []string{"TLSV1.1"},
			wantErr:   true,
		},
		{
			name:      "unknown_version_error",
			protocols: []string{"TLSv2.0"},
			wantErr:   true,
		},
		{
			name:      "random_string_error",
			protocols: []string{"foobar"},
			wantErr:   true,
		},
		{
			name:      "tls12_with_insecure_tls10_error",
			protocols: []string{"TLSv1.2", "TLSv1.0"},
			wantErr:   true,
		},
		{
			name:      "duplicate_tls12",
			protocols: []string{"TLSv1.2", "TLSv1.2"},
			wantMin:   tls.VersionTLS12,
			wantMax:   tls.VersionTLS13,
		},
		{
			name:      "duplicate_tls13",
			protocols: []string{"TLSv1.3", "TLSv1.3"},
			wantMin:   tls.VersionTLS13,
			wantMax:   tls.VersionTLS13,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			min, max, err := ParseTLSVersions(tt.protocols)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantMin, min)
			assert.Equal(t, tt.wantMax, max)
		})
	}
}

func TestParseTLSVersions_ErrorMessages(t *testing.T) {
	t.Run("insecure_version_message", func(t *testing.T) {
		_, _, err := ParseTLSVersions([]string{"TLSv1.0"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "insecure")
		assert.Contains(t, err.Error(), "TLSv1.0")
	})

	t.Run("unknown_version_message", func(t *testing.T) {
		_, _, err := ParseTLSVersions([]string{"TLSv9.9"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown")
		assert.Contains(t, err.Error(), "TLSv9.9")
	})
}

func TestParseMinTLSVersion(t *testing.T) {
	tests := []struct {
		name      string
		protocols []string
		want      uint16
	}{
		{
			name:      "tls13",
			protocols: []string{"TLSv1.3"},
			want:      tls.VersionTLS13,
		},
		{
			name:      "tls12",
			protocols: []string{"TLSv1.2"},
			want:      tls.VersionTLS12,
		},
		{
			name:      "empty_returns_tls12_default",
			protocols: []string{},
			want:      tls.VersionTLS12,
		},
		{
			name:      "nil_returns_tls12_default",
			protocols: nil,
			want:      tls.VersionTLS12,
		},
		{
			name:      "tls13_upper",
			protocols: []string{"TLSV1.3"},
			want:      tls.VersionTLS13,
		},
		{
			name:      "tls12_upper",
			protocols: []string{"TLSV1.2"},
			want:      tls.VersionTLS12,
		},
		{
			name:      "prioritizes_first_matching",
			protocols: []string{"TLSv1.3", "TLSv1.2"},
			want:      tls.VersionTLS13,
		},
		{
			name:      "prioritizes_first_matching_tls12",
			protocols: []string{"TLSv1.2", "TLSv1.3"},
			want:      tls.VersionTLS12,
		},
		{
			name:      "unknown_versions_return_default",
			protocols: []string{"TLSv1.0", "TLSv1.1", "unknown"},
			want:      tls.VersionTLS12,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseMinTLSVersion(tt.protocols)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseCipherSuites(t *testing.T) {
	tests := []struct {
		name    string
		ciphers []string
		want    []uint16
		wantErr bool
	}{
		{
			name:    "empty_returns_empty_not_nil",
			ciphers: []string{},
			want:    []uint16{},
		},
		{
			name:    "openssl_style_single",
			ciphers: []string{"ECDHE-RSA-AES128-GCM-SHA256"},
			want:    []uint16{tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256},
		},
		{
			name:    "openssl_style_multiple",
			ciphers: []string{"ECDHE-RSA-AES128-GCM-SHA256", "ECDHE-RSA-AES256-GCM-SHA384"},
			want:    []uint16{tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256, tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384},
		},
		{
			name:    "go_standard_name",
			ciphers: []string{"TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256"},
			want:    []uint16{tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256},
		},
		{
			name: "mixed_openssl_and_go_names",
			ciphers: []string{
				"ECDHE-RSA-AES128-GCM-SHA256",
				"TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384",
			},
			want: []uint16{
				tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			},
		},
		{
			name:    "tls13_cipher_AES128",
			ciphers: []string{"AES128-GCM-SHA256"},
			want:    []uint16{tls.TLS_AES_128_GCM_SHA256},
		},
		{
			name:    "tls13_cipher_AES256",
			ciphers: []string{"AES256-GCM-SHA384"},
			want:    []uint16{tls.TLS_AES_256_GCM_SHA384},
		},
		{
			name:    "tls13_cipher_CHACHA20",
			ciphers: []string{"CHACHA20-POLY1305"},
			want:    []uint16{tls.TLS_CHACHA20_POLY1305_SHA256},
		},
		{
			name:    "unknown_cipher_error",
			ciphers: []string{"UNKNOWN-CIPHER"},
			wantErr: true,
		},
		{
			name:    "insecure_3des_openssl_error",
			ciphers: []string{"ECDHE-RSA-3DES-EDE-CBC-SHA"},
			wantErr: true,
		},
		{
			name:    "insecure_3des_rsa_openssl_error",
			ciphers: []string{"RSA-3DES-EDE-CBC-SHA"},
			wantErr: true,
		},
		{
			name:    "unknown_among_valid_error",
			ciphers: []string{"ECDHE-RSA-AES128-GCM-SHA256", "BOGUS"},
			wantErr: true,
		},
		{
			name:    "all_ecdhe_variants",
			ciphers: []string{"ECDHE-RSA-CHACHA20-POLY1305", "ECDHE-ECDSA-CHACHA20-POLY1305"},
			want:    []uint16{tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256, tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256},
		},
		{
			name: "all_default_ciphers_valid",
			ciphers: []string{
				"ECDHE-RSA-AES128-GCM-SHA256",
				"ECDHE-RSA-AES256-GCM-SHA384",
				"ECDHE-RSA-CHACHA20-POLY1305",
				"ECDHE-ECDSA-AES128-GCM-SHA256",
				"ECDHE-ECDSA-AES256-GCM-SHA384",
				"ECDHE-ECDSA-CHACHA20-POLY1305",
			},
			want: []uint16{
				tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
				tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseCipherSuites(tt.ciphers)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseCipherSuites_ErrorMessages(t *testing.T) {
	t.Run("unknown_cipher_message", func(t *testing.T) {
		_, err := ParseCipherSuites([]string{"BOGUS-CIPHER"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown cipher suite")
		assert.Contains(t, err.Error(), "BOGUS-CIPHER")
	})

	t.Run("insecure_cipher_message", func(t *testing.T) {
		_, err := ParseCipherSuites([]string{"RSA-3DES-EDE-CBC-SHA"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "insecure cipher suite")
		assert.Contains(t, err.Error(), "RSA-3DES-EDE-CBC-SHA")
	})
}

func TestParseCipherSuitesLenient(t *testing.T) {
	tests := []struct {
		name    string
		ciphers []string
		want    []uint16
	}{
		{
			name:    "empty_returns_nil",
			ciphers: []string{},
			want:    nil,
		},
		{
			name:    "nil_returns_nil",
			ciphers: nil,
			want:    nil,
		},
		{
			name:    "valid_cipher",
			ciphers: []string{"ECDHE-RSA-AES128-GCM-SHA256"},
			want:    []uint16{tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256},
		},
		{
			name: "multiple_valid",
			ciphers: []string{
				"ECDHE-RSA-AES128-GCM-SHA256",
				"ECDHE-RSA-AES256-GCM-SHA384",
			},
			want: []uint16{
				tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			},
		},
		{
			name:    "unknown_skipped",
			ciphers: []string{"ECDHE-RSA-AES128-GCM-SHA256", "UNKNOWN-CIPHER"},
			want:    []uint16{tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256},
		},
		{
			name:    "insecure_skipped",
			ciphers: []string{"ECDHE-RSA-AES128-GCM-SHA256", "RSA-3DES-EDE-CBC-SHA"},
			want:    []uint16{tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256},
		},
		{
			name:    "all_unknown_returns_nil",
			ciphers: []string{"FOO", "BAR", "BAZ"},
			want:    nil,
		},
		{
			name:    "all_insecure_returns_nil",
			ciphers: []string{"RSA-3DES-EDE-CBC-SHA", "ECDHE-RSA-3DES-EDE-CBC-SHA"},
			want:    nil,
		},
		{
			name:    "go_standard_names",
			ciphers: []string{"TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256"},
			want:    []uint16{tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256},
		},
		{
			name: "mix_valid_unknown_insecure",
			ciphers: []string{
				"UNKNOWN",
				"ECDHE-RSA-AES128-GCM-SHA256",
				"RSA-3DES-EDE-CBC-SHA",
				"ALSO-UNKNOWN",
				"ECDHE-RSA-AES256-GCM-SHA384",
			},
			want: []uint16{
				tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseCipherSuitesLenient(tt.ciphers)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIsInsecureCipher(t *testing.T) {
	t.Run("insecure_ciphers", func(t *testing.T) {
		insecureIDs := []uint16{
			tls.TLS_RSA_WITH_RC4_128_SHA,
			tls.TLS_RSA_WITH_3DES_EDE_CBC_SHA,
			tls.TLS_RSA_WITH_AES_128_CBC_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_RC4_128_SHA,
			tls.TLS_ECDHE_RSA_WITH_RC4_128_SHA,
			tls.TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256,
		}
		for _, id := range insecureIDs {
			assert.True(t, IsInsecureCipher(id), "should be insecure: 0x%04x", id)
		}
	})

	t.Run("secure_ciphers", func(t *testing.T) {
		secureIDs := []uint16{
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
		}
		for _, id := range secureIDs {
			assert.False(t, IsInsecureCipher(id), "should be secure: 0x%04x", id)
		}
	})

	t.Run("zero_id_not_insecure", func(t *testing.T) {
		assert.False(t, IsInsecureCipher(0))
	})

	t.Run("arbitrary_id_not_insecure", func(t *testing.T) {
		assert.False(t, IsInsecureCipher(0xFFFF))
	})
}

func TestDefaultCipherSuites(t *testing.T) {
	t.Run("returns_non_empty", func(t *testing.T) {
		suites := DefaultCipherSuites()
		assert.NotEmpty(t, suites)
	})

	t.Run("all_secure", func(t *testing.T) {
		suites := DefaultCipherSuites()
		for _, id := range suites {
			assert.False(t, IsInsecureCipher(id), "default suite should be secure: 0x%04x", id)
		}
	})

	t.Run("contains_expected_ciphers", func(t *testing.T) {
		expected := []uint16{
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
		}
		got := DefaultCipherSuites()
		assert.Equal(t, expected, got)
	})

	t.Run("returns_fresh_slice", func(t *testing.T) {
		s1 := DefaultCipherSuites()
		s2 := DefaultCipherSuites()
		assert.Equal(t, s1, s2)
		s1[0] = 0
		assert.NotEqual(t, s1[0], s2[0], "modifying returned slice should not affect future calls")
	})
}

func TestTLSVersionMap(t *testing.T) {
	t.Run("contains_expected_keys", func(t *testing.T) {
		expectedKeys := []string{"TLSV1.0", "TLSV1.1", "TLSV1.2", "TLSV1.3", ""}
		for _, key := range expectedKeys {
			_, ok := TLSVersionMap[key]
			assert.True(t, ok, "TLSVersionMap should contain key %q", key)
		}
	})

	t.Run("correct_values", func(t *testing.T) {
		assert.Equal(t, uint16(tls.VersionTLS10), TLSVersionMap["TLSV1.0"])
		assert.Equal(t, uint16(tls.VersionTLS11), TLSVersionMap["TLSV1.1"])
		assert.Equal(t, uint16(tls.VersionTLS12), TLSVersionMap["TLSV1.2"])
		assert.Equal(t, uint16(tls.VersionTLS13), TLSVersionMap["TLSV1.3"])
		assert.Equal(t, uint16(0), TLSVersionMap[""])
	})

	t.Run("total_entries", func(t *testing.T) {
		assert.Len(t, TLSVersionMap, 5)
	})
}

func TestCipherNameToID_Consistency(t *testing.T) {
	t.Run("openssl_and_go_names_map_to_same_id", func(t *testing.T) {
		pairs := []struct {
			openssl string
			goName  string
		}{
			{"ECDHE-RSA-AES128-GCM-SHA256", "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256"},
			{"ECDHE-RSA-AES256-GCM-SHA384", "TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384"},
			{"ECDHE-RSA-CHACHA20-POLY1305", "TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305"},
			{"ECDHE-ECDSA-AES128-GCM-SHA256", "TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256"},
			{"ECDHE-ECDSA-AES256-GCM-SHA384", "TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384"},
			{"ECDHE-ECDSA-CHACHA20-POLY1305", "TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305"},
		}
		for _, p := range pairs {
			assert.Equal(t, cipherNameToID[p.openssl], cipherNameToID[p.goName],
				"OpenSSL name %q and Go name %q should map to same ID", p.openssl, p.goName)
		}
	})
}
