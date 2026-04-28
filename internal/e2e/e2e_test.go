//go:build e2e

package e2e

import (
	"testing"

	_ "github.com/testcontainers/testcontainers-go"
)

func TestE2ESetup(t *testing.T) {
	t.Parallel()
	t.Log("E2E test infrastructure initialized")
}
