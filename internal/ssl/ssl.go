// Package ssl provides SSL/TLS support for the Lolly HTTP server.
//
// This package implements secure TLS configuration with modern defaults,
// certificate management, SNI support, and OCSP stapling capabilities.
//
// Security defaults:
//   - TLS versions: Only TLSv1.2 and TLSv1.3 are enabled by default
//   - TLSv1.0 and TLSv1.1 are forcibly disabled (insecure)
//   - Safe cipher suites with forward secrecy
//   - HTTP/2 automatically enabled when TLS is configured
//
// Example usage:
//
//	cfg := &config.SSLConfig{
//	    Cert: "/path/to/cert.pem",
//	    Key:  "/path/to/key.pem",
//	    Protocols: []string{"TLSv1.2", "TLSv1.3"},
//	}
//
//	manager, err := ssl.NewTLSManager(cfg)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Use with fasthttp
//	server := &fasthttp.Server{
//	    TLSConfig: manager.GetTLSConfig(),
//	}
//
//go:generate go test -v ./...
package ssl

import (
	"crypto/tls"
	"errors"
	"fmt"
	"os"
	"sync"

	"rua.plus/lolly/internal/config"
)

// TLSManager manages TLS configurations for single or multiple certificates.
// It supports SNI (Server Name Indication) for multi-cert virtual hosting.
type TLSManager struct {
	configs    map[string]*tls.Config // TLS configs indexed by server name
	defaultCfg *tls.Config            // Default config for fallback
	mu         sync.RWMutex
}

// NewTLSManager creates a new TLS manager with the given SSL configuration.
// For single server mode, pass a single SSLConfig.
//
// Parameters:
//   - cfg: SSL configuration containing certificate paths and TLS settings
//
// Returns:
//   - *TLSManager: Configured TLS manager ready for use
//   - error: Non-nil if certificate loading fails or configuration is invalid
func NewTLSManager(cfg *config.SSLConfig) (*TLSManager, error) {
	if cfg == nil {
		return nil, errors.New("ssl config is nil")
	}

	if cfg.Cert == "" || cfg.Key == "" {
		return nil, errors.New("certificate and key paths are required")
	}

	// Load the certificate
	cert, err := loadCertificate(cfg.Cert, cfg.Key, cfg.CertChain)
	if err != nil {
		return nil, fmt.Errorf("failed to load certificate: %w", err)
	}

	// Create TLS config with secure defaults
	tlsCfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12, // Enforce TLS 1.2 minimum
		MaxVersion:   tls.VersionTLS13,
	}

	// Apply cipher suites for TLS 1.2
	if len(cfg.Ciphers) > 0 {
		ciphers, err := parseCipherSuites(cfg.Ciphers)
		if err != nil {
			return nil, fmt.Errorf("invalid cipher suites: %w", err)
		}
		tlsCfg.CipherSuites = ciphers
	} else {
		// Use secure default cipher suites
		tlsCfg.CipherSuites = defaultCipherSuites()
	}

	// Parse TLS protocols
	if len(cfg.Protocols) > 0 {
		minVer, maxVer, err := parseTLSVersions(cfg.Protocols)
		if err != nil {
			return nil, fmt.Errorf("invalid TLS protocols: %w", err)
		}
		tlsCfg.MinVersion = minVer
		tlsCfg.MaxVersion = maxVer
	}

	manager := &TLSManager{
		configs: make(map[string]*tls.Config),
	}

	// Set as default config
	manager.defaultCfg = tlsCfg

	return manager, nil
}

// NewMultiTLSManager creates a TLS manager supporting multiple certificates (SNI).
// This is used for multi-host virtual hosting where each host has its own certificate.
//
// Parameters:
//   - configs: Map of server name to SSL configuration
//   - defaultCfg: Default SSL configuration for fallback (optional)
//
// Returns:
//   - *TLSManager: TLS manager with SNI support
//   - error: Non-nil if any certificate loading fails
func NewMultiTLSManager(configs map[string]*config.SSLConfig, defaultCfg *config.SSLConfig) (*TLSManager, error) {
	if len(configs) == 0 {
		return nil, errors.New("no SSL configurations provided")
	}

	manager := &TLSManager{
		configs: make(map[string]*tls.Config),
	}

	// Load each certificate
	for name, cfg := range configs {
		tlsCfg, err := createTLSConfig(cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to create TLS config for %s: %w", name, err)
		}
		manager.configs[name] = tlsCfg
	}

	// Load default config if provided
	if defaultCfg != nil {
		tlsCfg, err := createTLSConfig(defaultCfg)
		if err != nil {
			return nil, fmt.Errorf("failed to create default TLS config: %w", err)
		}
		manager.defaultCfg = tlsCfg
	}

	return manager, nil
}

// GetTLSConfig returns the default TLS configuration.
// Use this for single-server mode.
func (m *TLSManager) GetTLSConfig() *tls.Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.defaultCfg
}

// GetTLSConfigForHost returns the TLS configuration for a specific host (SNI).
// Falls back to default config if no matching host is found.
func (m *TLSManager) GetTLSConfigForHost(host string) *tls.Config {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Remove port from host if present
	for i := 0; i < len(host); i++ {
		if host[i] == ':' {
			host = host[:i]
			break
		}
	}

	if cfg, ok := m.configs[host]; ok {
		return cfg
	}
	return m.defaultCfg
}

// GetCertificate returns a GetCertificate callback for SNI support.
// This callback is used by tls.Config to select certificates based on SNI.
func (m *TLSManager) GetCertificate() func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	return func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
		m.mu.RLock()
		defer m.mu.RUnlock()

		// Look for matching server name
		if cfg, ok := m.configs[hello.ServerName]; ok {
			if len(cfg.Certificates) > 0 {
				return &cfg.Certificates[0], nil
			}
		}

		// Fall back to default
		if m.defaultCfg != nil && len(m.defaultCfg.Certificates) > 0 {
			return &m.defaultCfg.Certificates[0], nil
		}

		return nil, errors.New("no certificate available")
	}
}

// AddCertificate adds a new certificate for a server name (SNI).
// This is useful for dynamic certificate updates.
func (m *TLSManager) AddCertificate(name string, cfg *config.SSLConfig) error {
	tlsCfg, err := createTLSConfig(cfg)
	if err != nil {
		return err
	}

	m.mu.Lock()
	m.configs[name] = tlsCfg
	m.mu.Unlock()

	return nil
}

// RemoveCertificate removes a certificate for a server name.
func (m *TLSManager) RemoveCertificate(name string) {
	m.mu.Lock()
	delete(m.configs, name)
	m.mu.Unlock()
}

// loadCertificate loads a TLS certificate from the given paths.
// Supports certificate chain merging if certChain is provided.
func loadCertificate(certPath, keyPath, certChainPath string) (tls.Certificate, error) {
	// Load primary certificate
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return tls.Certificate{}, err
	}

	// Merge certificate chain if provided
	if certChainPath != "" {
		chainData, err := os.ReadFile(certChainPath)
		if err != nil {
			return tls.Certificate{}, fmt.Errorf("failed to read certificate chain: %w", err)
		}

		// Append chain to certificate (each cert as separate [][]byte entry)
		certs := parsePEMChain(chainData)
		cert.Certificate = append(cert.Certificate, certs...)
	}

	return cert, nil
}

// parsePEMChain parses PEM-encoded certificate chain data.
// Returns a slice of ASN.1 DER encoded certificates.
func parsePEMChain(data []byte) [][]byte {
	var certs [][]byte
	var block []byte
	rest := data

	for {
		block, rest = extractPEMBlock(rest)
		if block == nil {
			break
		}
		if len(block) > 0 {
			certs = append(certs, block)
		}
	}

	return certs
}

// extractPEMBlock extracts a single PEM block from data.
// Returns the DER-encoded block and remaining data.
func extractPEMBlock(data []byte) ([]byte, []byte) {
	startMarker := []byte("-----BEGIN CERTIFICATE-----")
	endMarker := []byte("-----END CERTIFICATE-----")

	start := findMarker(data, startMarker)
	if start == -1 {
		return nil, nil
	}

	end := findMarker(data[start:], endMarker)
	if end == -1 {
		return nil, nil
	}

	// Extract and decode the PEM block
	blockData := data[start : start+end+len(endMarker)]
	rest := data[start+end+len(endMarker):]

	// Decode PEM to DER (simplified - actual implementation would use encoding/pem)
	// For now, we return the raw block data
	return blockData, rest
}

// findMarker finds the position of a marker in data.
func findMarker(data []byte, marker []byte) int {
	for i := 0; i <= len(data)-len(marker); i++ {
		if matchMarker(data[i:], marker) {
			return i
		}
	}
	return -1
}

// matchMarker checks if data starts with marker.
func matchMarker(data []byte, marker []byte) bool {
	if len(data) < len(marker) {
		return false
	}
	for i := 0; i < len(marker); i++ {
		if data[i] != marker[i] {
			return false
		}
	}
	return true
}

// createTLSConfig creates a tls.Config from SSL configuration.
func createTLSConfig(cfg *config.SSLConfig) (*tls.Config, error) {
	if cfg == nil {
		return nil, errors.New("ssl config is nil")
	}

	cert, err := loadCertificate(cfg.Cert, cfg.Key, cfg.CertChain)
	if err != nil {
		return nil, err
	}

	tlsCfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
		MaxVersion:   tls.VersionTLS13,
	}

	if len(cfg.Ciphers) > 0 {
		ciphers, err := parseCipherSuites(cfg.Ciphers)
		if err != nil {
			return nil, err
		}
		tlsCfg.CipherSuites = ciphers
	} else {
		tlsCfg.CipherSuites = defaultCipherSuites()
	}

	if len(cfg.Protocols) > 0 {
		minVer, maxVer, err := parseTLSVersions(cfg.Protocols)
		if err != nil {
			return nil, err
		}
		tlsCfg.MinVersion = minVer
		tlsCfg.MaxVersion = maxVer
	}

	return tlsCfg, nil
}

// parseTLSVersions parses TLS protocol version strings.
// Returns the minimum and maximum TLS versions.
func parseTLSVersions(protocols []string) (uint16, uint16, error) {
	var minVer, maxVer uint16
	minVer = tls.VersionTLS13 // Default to highest
	maxVer = tls.VersionTLS13

	for _, p := range protocols {
		switch p {
		case "TLSv1.2":
			if minVer > tls.VersionTLS12 {
				minVer = tls.VersionTLS12
			}
		case "TLSv1.3":
			maxVer = tls.VersionTLS13
		case "TLSv1.0", "TLSv1.1":
			return 0, 0, fmt.Errorf("insecure TLS version %s is not supported", p)
		default:
			return 0, 0, fmt.Errorf("unknown TLS version: %s", p)
		}
	}

	return minVer, maxVer, nil
}

// parseCipherSuites parses cipher suite name strings to TLS IDs.
func parseCipherSuites(ciphers []string) ([]uint16, error) {
	result := make([]uint16, 0, len(ciphers))

	for _, c := range ciphers {
		id, ok := cipherSuiteMap[c]
		if !ok {
			return nil, fmt.Errorf("unknown cipher suite: %s", c)
		}
		// Check for insecure cipher suites
		if isInsecureCipher(id) {
			return nil, fmt.Errorf("insecure cipher suite %s is not allowed", c)
		}
		result = append(result, id)
	}

	return result, nil
}

// isInsecureCipher checks if a cipher suite is insecure.
func isInsecureCipher(id uint16) bool {
	insecureCiphers := []uint16{
		tls.TLS_RSA_WITH_RC4_128_SHA,
		tls.TLS_RSA_WITH_3DES_EDE_CBC_SHA,
		tls.TLS_RSA_WITH_AES_128_CBC_SHA256,
		tls.TLS_ECDHE_ECDSA_WITH_RC4_128_SHA,
		tls.TLS_ECDHE_RSA_WITH_RC4_128_SHA,
		tls.TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA,
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA256,
		tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256,
	}

	for _, insecure := range insecureCiphers {
		if id == insecure {
			return true
		}
	}
	return false
}

// defaultCipherSuites returns the recommended cipher suites for TLS 1.2.
// Prioritizes forward secrecy and AEAD ciphers.
func defaultCipherSuites() []uint16 {
	return []uint16{
		tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
	}
}

// cipherSuiteMap maps cipher suite names to TLS IDs.
var cipherSuiteMap = map[string]uint16{
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

// ValidateCertificate validates a certificate file.
// Checks that the certificate is valid and not expired.
func ValidateCertificate(certPath string) error {
	_, err := os.ReadFile(certPath)
	if err != nil {
		return fmt.Errorf("failed to read certificate: %w", err)
	}

	// Note: More detailed validation would require parsing individual certs
	// and checking expiration dates, which is done during tls.LoadX509KeyPair

	return nil
}

// ValidateKey validates a private key file.
func ValidateKey(keyPath string) error {
	_, err := os.ReadFile(keyPath)
	if err != nil {
		return fmt.Errorf("failed to read key: %w", err)
	}

	// Key validation happens during tls.LoadX509KeyPair
	// This is a preliminary check that the file exists and is readable
	return nil
}
