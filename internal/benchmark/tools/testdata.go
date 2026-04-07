// Package tools provides testing utilities and mock infrastructure for benchmark tests.
package tools

import (
	"fmt"
	"math/rand"
	"time"
)

// TestDataSize represents predefined test data sizes.
type TestDataSize int

const (
	// Size1KB represents 1KB test data.
	Size1KB TestDataSize = 1024
	// Size10KB represents 10KB test data.
	Size10KB TestDataSize = 10 * 1024
	// Size100KB represents 100KB test data.
	Size100KB TestDataSize = 100 * 1024
	// Size1MB represents 1MB test data.
	Size1MB TestDataSize = 1024 * 1024
	// Size10MB represents 10MB test data.
	Size10MB TestDataSize = 10 * 1024 * 1024
)

// GenerateTestData generates test data of the specified size.
func GenerateTestData(size TestDataSize) []byte {
	data := make([]byte, size)
	// Fill with random data for compression testing
	for i := range data {
		data[i] = byte(rand.Intn(256))
	}
	return data
}

// GenerateTestDataString generates test data as string.
func GenerateTestDataString(size TestDataSize) string {
	return string(GenerateTestData(size))
}

// TestTarget represents a backend target for testing.
type TestTarget struct {
	Address string
	Weight  int
}

// createTestTargets creates n test backend targets with mock servers.
// Returns the target configurations and a cleanup function.
func createTestTargets(n int) ([]TestTarget, func()) {
	targets := make([]TestTarget, n)
	cleanups := make([]func(), n)

	for i := 0; i < n; i++ {
		body := GenerateTestData(Size1KB)
		addr, cleanup := SimpleMockBackend(200, body)
		targets[i] = TestTarget{
			Address: addr,
			Weight:  1,
		}
		cleanups[i] = cleanup
	}

	cleanupAll := func() {
		for _, cleanup := range cleanups {
			if cleanup != nil {
				cleanup()
			}
		}
	}

	return targets, cleanupAll
}

// CreateTestTargets creates n test backend targets.
// Returns the target configurations and a cleanup function.
func CreateTestTargets(n int) ([]TestTarget, func()) {
	return createTestTargets(n)
}

// CreateWeightedTestTargets creates n test backend targets with varying weights.
// Returns the target configurations and a cleanup function.
func CreateWeightedTestTargets(n int) ([]TestTarget, func()) {
	targets := make([]TestTarget, n)
	cleanups := make([]func(), n)

	for i := 0; i < n; i++ {
		body := GenerateTestData(Size1KB)
		addr, cleanup := SimpleMockBackend(200, body)
		// Vary weights: 1, 2, 3, etc.
		targets[i] = TestTarget{
			Address: addr,
			Weight:  i + 1,
		}
		cleanups[i] = cleanup
	}

	cleanupAll := func() {
		for _, cleanup := range cleanups {
			if cleanup != nil {
				cleanup()
			}
		}
	}

	return targets, cleanupAll
}

// CreateDelayedTestTargets creates n test backend targets with varying delays.
// Useful for testing timeout and latency-related behavior.
func CreateDelayedTestTargets(n int, baseDelay time.Duration) ([]TestTarget, func()) {
	targets := make([]TestTarget, n)
	cleanups := make([]func(), n)

	for i := 0; i < n; i++ {
		body := GenerateTestData(Size1KB)
		// Each target has increasing delay
		delay := baseDelay * time.Duration(i+1)
		addr, cleanup := DelayedMockBackend(delay, body)
		targets[i] = TestTarget{
			Address: addr,
			Weight:  1,
		}
		cleanups[i] = cleanup
	}

	cleanupAll := func() {
		for _, cleanup := range cleanups {
			if cleanup != nil {
				cleanup()
			}
		}
	}

	return targets, cleanupAll
}

// CreateErrorTestTargets creates n test backend targets with varying error rates.
// Useful for testing error handling and circuit breaker behavior.
func CreateErrorTestTargets(n int, baseErrorRate float64) ([]TestTarget, func()) {
	targets := make([]TestTarget, n)
	cleanups := make([]func(), n)

	for i := 0; i < n; i++ {
		body := GenerateTestData(Size1KB)
		// Vary error rates slightly per target
		errorRate := baseErrorRate + float64(i)*0.05
		if errorRate > 1.0 {
			errorRate = 1.0
		}
		addr, cleanup := ErrorMockBackend(errorRate, body)
		targets[i] = TestTarget{
			Address: addr,
			Weight:  1,
		}
		cleanups[i] = cleanup
	}

	cleanupAll := func() {
		for _, cleanup := range cleanups {
			if cleanup != nil {
				cleanup()
			}
		}
	}

	return targets, cleanupAll
}

// GenerateCacheKey generates a unique cache key for testing.
func GenerateCacheKey(prefix string, index int) string {
	return fmt.Sprintf("%s-key-%d", prefix, index)
}

// GenerateCacheValue generates a cache value of specified size.
func GenerateCacheValue(size TestDataSize) []byte {
	return GenerateTestData(size)
}

// GenerateRandomCacheKey generates a random cache key.
func GenerateRandomCacheKey() string {
	return fmt.Sprintf("random-key-%d-%d", time.Now().UnixNano(), rand.Int())
}
