package config

import (
	"os"
	"regexp"
)

var envPattern = regexp.MustCompile(`\$\{([^}]+)\}`)

// ExpandEnv replaces ${VAR} patterns with environment variable values.
func ExpandEnv(data []byte) []byte {
	return envPattern.ReplaceAllFunc(data, func(match []byte) []byte {
		name := string(match[2 : len(match)-1])
		if name == "" {
			return match
		}
		if value, ok := os.LookupEnv(name); ok {
			return []byte(value)
		}
		return match
	})
}
