// Package sslutil provides SSL/TLS utility functions.
package sslutil

import (
	"crypto/x509"
	"errors"
	"fmt"
	"os"
)

// LoadCertPool loads a certificate pool from a file.
// Supports PEM format certificate files that may contain multiple certificates.
//
// Parameters:
//   - certFile: Certificate file path
//
// Returns:
//   - *x509.CertPool: Certificate pool
//   - error: Returns error if loading fails
func LoadCertPool(certFile string, _ string) (*x509.CertPool, error) {
	data, err := os.ReadFile(certFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read certificate file: %w", err)
	}

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(data) {
		return nil, fmt.Errorf("failed to parse certificates from %s", certFile)
	}

	return pool, nil
}

// LoadCACertPool loads a CA certificate pool from a file.
// This is a convenience function for loading CA certificates.
//
// Parameters:
//   - caFile: CA certificate file path
//
// Returns:
//   - *x509.CertPool: CA certificate pool
//   - error: Returns error if loading fails
func LoadCACertPool(caFile string) (*x509.CertPool, error) {
	data, err := os.ReadFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read CA file: %w", err)
	}

	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(data) {
		return nil, errors.New("failed to parse CA certificates")
	}

	return caPool, nil
}
