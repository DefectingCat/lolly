package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExpandEnv(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		setup    func(*testing.T)
	}{
		{
			name:     "single variable",
			input:    "${VAR}",
			expected: "value",
			setup: func(t *testing.T) {
				t.Setenv("VAR", "value")
			},
		},
		{
			name:     "multiple variables",
			input:    "${HOST}:${PORT}",
			expected: "localhost:8080",
			setup: func(t *testing.T) {
				t.Setenv("HOST", "localhost")
				t.Setenv("PORT", "8080")
			},
		},
		{
			name:     "variable with prefix and suffix",
			input:    "prefix_${VAR}_suffix",
			expected: "prefix_value_suffix",
			setup: func(t *testing.T) {
				t.Setenv("VAR", "value")
			},
		},
		{
			name:     "missing variable unchanged",
			input:    "${MISSING}",
			expected: "${MISSING}",
		},
		{
			name:     "mixed existing and missing",
			input:    "${VAR}/${MISSING}",
			expected: "value/${MISSING}",
			setup: func(t *testing.T) {
				t.Setenv("VAR", "value")
			},
		},
		{
			name:     "empty input",
			input:    "",
			expected: "",
		},
		{
			name:     "no variables",
			input:    "just a plain string",
			expected: "just a plain string",
		},
		{
			name:     "empty variable name",
			input:    "${}",
			expected: "${}",
		},
		{
			name:     "adjacent variables",
			input:    "${VAR1}${VAR2}",
			expected: "value1value2",
			setup: func(t *testing.T) {
				t.Setenv("VAR1", "value1")
				t.Setenv("VAR2", "value2")
			},
		},
		{
			name:     "variable with empty value",
			input:    "${EMPTY_VAR}",
			expected: "",
			setup: func(t *testing.T) {
				t.Setenv("EMPTY_VAR", "")
			},
		},
		{
			name:     "same variable multiple times",
			input:    "${VAR}-${VAR}-${VAR}",
			expected: "val-val-val",
			setup: func(t *testing.T) {
				t.Setenv("VAR", "val")
			},
		},
		{
			name: "full yaml-like input",
			input: `server:
  host: ${HOST}
  port: ${PORT}
  name: ${APP_NAME}`,
			expected: `server:
  host: 127.0.0.1
  port: 9090
  name: lolly`,
			setup: func(t *testing.T) {
				t.Setenv("HOST", "127.0.0.1")
				t.Setenv("PORT", "9090")
				t.Setenv("APP_NAME", "lolly")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setup != nil {
				tt.setup(t)
			}
			result := ExpandEnv([]byte(tt.input))
			assert.Equal(t, tt.expected, string(result))
		})
	}
}
