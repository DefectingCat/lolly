// Package sslutil provides SSL/TLS utility functions tests.
package sslutil

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// generateTestCert generates a self-signed certificate for testing.
func generateTestCert(t *testing.T) ([]byte, []byte) {
	t.Helper()

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate private key: %v", err)
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Test"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost"},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("Failed to create certificate: %v", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})

	keyDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		t.Fatalf("Failed to marshal private key: %v", err)
	}

	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: keyDER,
	})

	return certPEM, keyPEM
}

// generateMultipleCerts generates multiple certificates in a single PEM file.
func generateMultipleCerts(t *testing.T, count int) []byte {
	t.Helper()

	var pemData []byte
	for i := 0; i < count; i++ {
		priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatalf("Failed to generate private key: %v", err)
		}

		template := x509.Certificate{
			SerialNumber: big.NewInt(int64(i + 1)),
			Subject: pkix.Name{
				Organization: []string{"Test"},
				CommonName:   "test-cert-" + string(rune('A'+i)),
			},
			NotBefore:             time.Now(),
			NotAfter:              time.Now().Add(time.Hour),
			KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
			ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
			BasicConstraintsValid: true,
			DNSNames:              []string{"localhost"},
		}

		certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
		if err != nil {
			t.Fatalf("Failed to create certificate: %v", err)
		}

		certPEM := pem.EncodeToMemory(&pem.Block{
			Type:  "CERTIFICATE",
			Bytes: certDER,
		})

		pemData = append(pemData, certPEM...)
	}

	return pemData
}

func TestLoadCertPool_Success(t *testing.T) {
	tmpDir := t.TempDir()
	certPath := filepath.Join(tmpDir, "cert.pem")

	cert, _ := generateTestCert(t)
	if err := os.WriteFile(certPath, cert, 0o644); err != nil {
		t.Fatalf("Failed to write cert: %v", err)
	}

	pool, err := LoadCertPool(certPath, "")
	if err != nil {
		t.Errorf("LoadCertPool() error = %v, want nil", err)
	}
	if pool == nil {
		t.Error("LoadCertPool() returned nil pool")
	}
}

func TestLoadCertPool_MultipleCertificates(t *testing.T) {
	tmpDir := t.TempDir()
	certPath := filepath.Join(tmpDir, "certs.pem")

	multiCerts := generateMultipleCerts(t, 3)
	if err := os.WriteFile(certPath, multiCerts, 0o644); err != nil {
		t.Fatalf("Failed to write multi-cert file: %v", err)
	}

	pool, err := LoadCertPool(certPath, "")
	if err != nil {
		t.Errorf("LoadCertPool() error = %v, want nil", err)
	}
	if pool == nil {
		t.Error("LoadCertPool() returned nil pool")
	}
}

func TestLoadCertPool_FileNotFound(t *testing.T) {
	_, err := LoadCertPool("/nonexistent/cert.pem", "")
	if err == nil {
		t.Error("LoadCertPool() expected error for non-existent file, got nil")
	}
}

func TestLoadCertPool_EmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	certPath := filepath.Join(tmpDir, "empty.pem")

	if err := os.WriteFile(certPath, []byte{}, 0o644); err != nil {
		t.Fatalf("Failed to write empty file: %v", err)
	}

	_, err := LoadCertPool(certPath, "")
	if err == nil {
		t.Error("LoadCertPool() expected error for empty file, got nil")
	}
}

func TestLoadCertPool_InvalidPEM(t *testing.T) {
	tmpDir := t.TempDir()
	certPath := filepath.Join(tmpDir, "invalid.pem")

	invalidData := []byte("not a valid PEM file")
	if err := os.WriteFile(certPath, invalidData, 0o644); err != nil {
		t.Fatalf("Failed to write invalid file: %v", err)
	}

	_, err := LoadCertPool(certPath, "")
	if err == nil {
		t.Error("LoadCertPool() expected error for invalid PEM, got nil")
	}
}

func TestLoadCertPool_InvalidCertificate(t *testing.T) {
	tmpDir := t.TempDir()
	certPath := filepath.Join(tmpDir, "invalid-cert.pem")

	// Valid PEM block but not a certificate
	invalidCert := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: []byte("not a valid certificate"),
	})
	if err := os.WriteFile(certPath, invalidCert, 0o644); err != nil {
		t.Fatalf("Failed to write invalid cert file: %v", err)
	}

	_, err := LoadCertPool(certPath, "")
	if err == nil {
		t.Error("LoadCertPool() expected error for invalid certificate, got nil")
	}
}

func TestLoadCertPool_WrongPEMType(t *testing.T) {
	tmpDir := t.TempDir()
	certPath := filepath.Join(tmpDir, "key.pem")

	_, key := generateTestCert(t)
	if err := os.WriteFile(certPath, key, 0o644); err != nil {
		t.Fatalf("Failed to write key file: %v", err)
	}

	_, err := LoadCertPool(certPath, "")
	if err == nil {
		t.Error("LoadCertPool() expected error for non-certificate PEM, got nil")
	}
}

func TestLoadCertPool_Directory(t *testing.T) {
	tmpDir := t.TempDir()

	_, err := LoadCertPool(tmpDir, "")
	if err == nil {
		t.Error("LoadCertPool() expected error for directory path, got nil")
	}
}

func TestLoadCACertPool_Success(t *testing.T) {
	tmpDir := t.TempDir()
	caPath := filepath.Join(tmpDir, "ca.pem")

	cert, _ := generateTestCert(t)
	if err := os.WriteFile(caPath, cert, 0o644); err != nil {
		t.Fatalf("Failed to write CA cert: %v", err)
	}

	pool, err := LoadCACertPool(caPath)
	if err != nil {
		t.Errorf("LoadCACertPool() error = %v, want nil", err)
	}
	if pool == nil {
		t.Error("LoadCACertPool() returned nil pool")
	}
}

func TestLoadCACertPool_MultipleCertificates(t *testing.T) {
	tmpDir := t.TempDir()
	caPath := filepath.Join(tmpDir, "ca-bundle.pem")

	multiCerts := generateMultipleCerts(t, 5)
	if err := os.WriteFile(caPath, multiCerts, 0o644); err != nil {
		t.Fatalf("Failed to write CA bundle: %v", err)
	}

	pool, err := LoadCACertPool(caPath)
	if err != nil {
		t.Errorf("LoadCACertPool() error = %v, want nil", err)
	}
	if pool == nil {
		t.Error("LoadCACertPool() returned nil pool")
	}
}

func TestLoadCACertPool_FileNotFound(t *testing.T) {
	_, err := LoadCACertPool("/nonexistent/ca.pem")
	if err == nil {
		t.Error("LoadCACertPool() expected error for non-existent file, got nil")
	}
}

func TestLoadCACertPool_EmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	caPath := filepath.Join(tmpDir, "empty.pem")

	if err := os.WriteFile(caPath, []byte{}, 0o644); err != nil {
		t.Fatalf("Failed to write empty file: %v", err)
	}

	_, err := LoadCACertPool(caPath)
	if err == nil {
		t.Error("LoadCACertPool() expected error for empty file, got nil")
	}
}

func TestLoadCACertPool_InvalidPEM(t *testing.T) {
	tmpDir := t.TempDir()
	caPath := filepath.Join(tmpDir, "invalid.pem")

	invalidData := []byte("not a valid PEM file")
	if err := os.WriteFile(caPath, invalidData, 0o644); err != nil {
		t.Fatalf("Failed to write invalid file: %v", err)
	}

	_, err := LoadCACertPool(caPath)
	if err == nil {
		t.Error("LoadCACertPool() expected error for invalid PEM, got nil")
	}
}

func TestLoadCACertPool_InvalidCertificate(t *testing.T) {
	tmpDir := t.TempDir()
	caPath := filepath.Join(tmpDir, "invalid-ca.pem")

	// Valid PEM block but not a certificate
	invalidCert := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: []byte("not a valid certificate"),
	})
	if err := os.WriteFile(caPath, invalidCert, 0o644); err != nil {
		t.Fatalf("Failed to write invalid cert file: %v", err)
	}

	_, err := LoadCACertPool(caPath)
	if err == nil {
		t.Error("LoadCACertPool() expected error for invalid certificate, got nil")
	}
}

func TestLoadCACertPool_WrongPEMType(t *testing.T) {
	tmpDir := t.TempDir()
	caPath := filepath.Join(tmpDir, "key.pem")

	_, key := generateTestCert(t)
	if err := os.WriteFile(caPath, key, 0o644); err != nil {
		t.Fatalf("Failed to write key file: %v", err)
	}

	_, err := LoadCACertPool(caPath)
	if err == nil {
		t.Error("LoadCACertPool() expected error for non-certificate PEM, got nil")
	}
}

func TestLoadCACertPool_Directory(t *testing.T) {
	tmpDir := t.TempDir()

	_, err := LoadCACertPool(tmpDir)
	if err == nil {
		t.Error("LoadCACertPool() expected error for directory path, got nil")
	}
}

func TestLoadCACertPool_PermissionDenied(t *testing.T) {
	// Skip on Windows as permission handling differs
	if os.Getenv("GOOS") == "windows" {
		t.Skip("Skipping on Windows")
	}

	tmpDir := t.TempDir()
	caPath := filepath.Join(tmpDir, "no-perm.pem")

	cert, _ := generateTestCert(t)
	if err := os.WriteFile(caPath, cert, 0o000); err != nil {
		t.Fatalf("Failed to write cert: %v", err)
	}

	_, err := LoadCACertPool(caPath)
	if err == nil {
		t.Error("LoadCACertPool() expected error for unreadable file, got nil")
	}
}

// Test that LoadCertPool and LoadCACertPool produce equivalent results
func TestLoadCertPool_Equivalence(t *testing.T) {
	tmpDir := t.TempDir()
	certPath := filepath.Join(tmpDir, "cert.pem")

	cert, _ := generateTestCert(t)
	if err := os.WriteFile(certPath, cert, 0o644); err != nil {
		t.Fatalf("Failed to write cert: %v", err)
	}

	pool1, err1 := LoadCertPool(certPath, "")
	pool2, err2 := LoadCACertPool(certPath)

	if err1 != err2 {
		t.Errorf("LoadCertPool() err = %v, LoadCACertPool() err = %v", err1, err2)
	}

	if (pool1 == nil) != (pool2 == nil) {
		t.Errorf("LoadCertPool() pool nil = %v, LoadCACertPool() pool nil = %v", pool1 == nil, pool2 == nil)
	}
}

// Benchmark LoadCertPool
func BenchmarkLoadCertPool(b *testing.B) {
	tmpDir := b.TempDir()
	certPath := filepath.Join(tmpDir, "cert.pem")

	cert, _ := generateTestCert(&testing.T{})
	if err := os.WriteFile(certPath, cert, 0o644); err != nil {
		b.Fatalf("Failed to write cert: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = LoadCertPool(certPath, "")
	}
}

// Benchmark LoadCACertPool
func BenchmarkLoadCACertPool(b *testing.B) {
	tmpDir := b.TempDir()
	caPath := filepath.Join(tmpDir, "ca.pem")

	cert, _ := generateTestCert(&testing.T{})
	if err := os.WriteFile(caPath, cert, 0o644); err != nil {
		b.Fatalf("Failed to write CA cert: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = LoadCACertPool(caPath)
	}
}

// Benchmark with multiple certificates
func BenchmarkLoadCACertPool_MultiCert(b *testing.B) {
	tmpDir := b.TempDir()
	caPath := filepath.Join(tmpDir, "ca-bundle.pem")

	multiCerts := generateMultipleCerts(&testing.T{}, 10)
	if err := os.WriteFile(caPath, multiCerts, 0o644); err != nil {
		b.Fatalf("Failed to write CA bundle: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = LoadCACertPool(caPath)
	}
}
