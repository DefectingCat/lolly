// Package sslutil provides SSL/TLS utility functions.
package sslutil

import (
	"crypto/tls"
	"fmt"
	"slices"
)

// TLS version string constants
const (
	tlsV12Lower = "TLSv1.2"
	tlsV13Lower = "TLSv1.3"
	tlsV12Upper = "TLSV1.2"
	tlsV13Upper = "TLSV1.3"
)

// ParseTLSVersion parses a TLS version string to a tls constant.
//
// Parameters:
//   - version: TLS version string (e.g., "TLSv1.0", "TLSv1.1", "TLSv1.2", "TLSv1.3")
//
// Returns:
//   - uint16: TLS version constant
//   - error: Invalid version string returns error
func ParseTLSVersion(version string) (uint16, error) {
	switch version {
	case "TLSv1.0", "TLSV1.0":
		return tls.VersionTLS10, nil
	case "TLSv1.1", "TLSV1.1":
		return tls.VersionTLS11, nil
	case tlsV12Lower, tlsV12Upper:
		return tls.VersionTLS12, nil
	case tlsV13Lower, tlsV13Upper:
		return tls.VersionTLS13, nil
	case "":
		return 0, nil // Empty string means use default
	default:
		return 0, fmt.Errorf("invalid TLS version: %s", version)
	}
}

// ParseTLSVersions parses TLS protocol version strings.
//
// Returns minimum and maximum TLS versions.
//
// Parameters:
//   - protocols: Protocol name list (e.g., "TLSv1.2", "TLSv1.3")
//
// Returns:
//   - uint16: Minimum version
//   - uint16: Maximum version
//   - error: Invalid protocol returns error
func ParseTLSVersions(protocols []string) (uint16, uint16, error) {
	var minVer, maxVer uint16
	minVer = tls.VersionTLS13 // Default to highest version
	maxVer = tls.VersionTLS13

	for _, p := range protocols {
		switch p {
		case tlsV12Lower, tlsV12Upper:
			if minVer > tls.VersionTLS12 {
				minVer = tls.VersionTLS12
			}
		case tlsV13Lower, tlsV13Upper:
			maxVer = tls.VersionTLS13
		case "TLSv1.0", "TLSV1.0", "TLSv1.1", "TLSV1.1":
			return 0, 0, fmt.Errorf("insecure TLS version %s is not supported", p)
		default:
			return 0, 0, fmt.Errorf("unknown TLS version: %s", p)
		}
	}

	return minVer, maxVer, nil
}

// ParseMinTLSVersion parses the minimum TLS version from protocol list.
//
// Parameters:
//   - protocols: Protocol version list
//
// Returns:
//   - uint16: TLS version constant
func ParseMinTLSVersion(protocols []string) uint16 {
	for _, p := range protocols {
		switch p {
		case tlsV13Lower, tlsV13Upper:
			return tls.VersionTLS13
		case tlsV12Lower, tlsV12Upper:
			return tls.VersionTLS12
		}
	}
	return tls.VersionTLS12
}

// cipherNameToID maps cipher suite names to TLS IDs.
// Supports both OpenSSL-style names and Go standard names.
var cipherNameToID = map[string]uint16{
	// OpenSSL-style names (ECDHE-RSA-AES128-GCM-SHA256)
	"ECDHE-RSA-AES128-GCM-SHA256":   tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
	"ECDHE-RSA-AES256-GCM-SHA384":   tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
	"ECDHE-ECDSA-AES128-GCM-SHA256": tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
	"ECDHE-ECDSA-AES256-GCM-SHA384": tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
	"ECDHE-RSA-CHACHA20-POLY1305":   tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
	"ECDHE-ECDSA-CHACHA20-POLY1305": tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
	"AES128-GCM-SHA256":             tls.TLS_AES_128_GCM_SHA256,
	"AES256-GCM-SHA384":             tls.TLS_AES_256_GCM_SHA384,
	"CHACHA20-POLY1305":             tls.TLS_CHACHA20_POLY1305_SHA256,
	"ECDHE-RSA-AES128-CBC-SHA":      tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
	"ECDHE-RSA-AES256-CBC-SHA":      tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
	"ECDHE-ECDSA-AES128-CBC-SHA":    tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA,
	"ECDHE-ECDSA-AES256-CBC-SHA":    tls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA,
	"RSA-AES128-GCM-SHA256":         tls.TLS_RSA_WITH_AES_128_GCM_SHA256,
	"RSA-AES256-GCM-SHA384":         tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
	"RSA-AES128-CBC-SHA":            tls.TLS_RSA_WITH_AES_128_CBC_SHA,
	"RSA-AES256-CBC-SHA":            tls.TLS_RSA_WITH_AES_256_CBC_SHA,
	"ECDHE-RSA-3DES-EDE-CBC-SHA":    tls.TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA,
	"RSA-3DES-EDE-CBC-SHA":          tls.TLS_RSA_WITH_3DES_EDE_CBC_SHA,
	// Go standard names (TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256)
	"TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256":   tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
	"TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384":   tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
	"TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305":    tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
	"TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256": tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
	"TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384": tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
	"TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305":  tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
	"TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA":      tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
	"TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA":      tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
	"TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA":    tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA,
	"TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA":    tls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA,
	"TLS_RSA_WITH_AES_128_GCM_SHA256":         tls.TLS_RSA_WITH_AES_128_GCM_SHA256,
	"TLS_RSA_WITH_AES_256_GCM_SHA384":         tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
	"TLS_RSA_WITH_AES_128_CBC_SHA":            tls.TLS_RSA_WITH_AES_128_CBC_SHA,
	"TLS_RSA_WITH_AES_256_CBC_SHA":            tls.TLS_RSA_WITH_AES_256_CBC_SHA,
}

// insecureCipherIDs contains cipher suite IDs considered insecure.
var insecureCipherIDs = []uint16{
	tls.TLS_RSA_WITH_RC4_128_SHA,
	tls.TLS_RSA_WITH_3DES_EDE_CBC_SHA,
	tls.TLS_RSA_WITH_AES_128_CBC_SHA256,
	tls.TLS_ECDHE_ECDSA_WITH_RC4_128_SHA,
	tls.TLS_ECDHE_RSA_WITH_RC4_128_SHA,
	tls.TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA,
	tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA256,
	tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256,
}

// IsInsecureCipher checks if a cipher suite is insecure.
//
// Parameters:
//   - id: Cipher suite ID
//
// Returns:
//   - bool: True if insecure
func IsInsecureCipher(id uint16) bool {
	return slices.Contains(insecureCipherIDs, id)
}

// ParseCipherSuites parses cipher suite name strings to TLS IDs.
//
// Parameters:
//   - ciphers: Cipher suite name list
//
// Returns:
//   - []uint16: Cipher suite ID list
//   - error: Unknown or insecure cipher suite returns error
func ParseCipherSuites(ciphers []string) ([]uint16, error) {
	result := make([]uint16, 0, len(ciphers))

	for _, c := range ciphers {
		id, ok := cipherNameToID[c]
		if !ok {
			return nil, fmt.Errorf("unknown cipher suite: %s", c)
		}
		// Check for insecure cipher suites
		if IsInsecureCipher(id) {
			return nil, fmt.Errorf("insecure cipher suite %s is not allowed", c)
		}
		result = append(result, id)
	}

	return result, nil
}

// ParseCipherSuitesLenient parses cipher suites without error on unknown names.
// Returns nil if no valid suites found, allowing fallback to defaults.
//
// Parameters:
//   - ciphers: Cipher suite name list
//
// Returns:
//   - []uint16: Cipher suite ID list (nil if none valid)
func ParseCipherSuitesLenient(ciphers []string) []uint16 {
	var suites []uint16
	for _, c := range ciphers {
		if id, ok := cipherNameToID[c]; ok && !IsInsecureCipher(id) {
			suites = append(suites, id)
		}
	}
	if len(suites) == 0 {
		return nil // Use defaults
	}
	return suites
}

// DefaultCipherSuites returns recommended cipher suites for TLS 1.2.
//
// Prioritizes forward secrecy and AEAD encryption algorithms.
//
// Returns:
//   - []uint16: Cipher suite ID list
func DefaultCipherSuites() []uint16 {
	return []uint16{
		tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
	}
}

// TLSVersionMap maps TLS version strings to tls constants.
// Supports TLSv1.0, TLSv1.1, TLSv1.2, TLSv1.3 formats (case insensitive).
// Empty string means use Go standard library default.
var TLSVersionMap = map[string]uint16{
	"TLSV1.0": tls.VersionTLS10,
	"TLSV1.1": tls.VersionTLS11,
	"TLSV1.2": tls.VersionTLS12,
	"TLSV1.3": tls.VersionTLS13,
	"":        0, // Empty string means use default
}
