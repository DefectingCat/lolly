package proxy

import "testing"

func TestContainsCRLF(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"empty", "", false},
		{"normal", "normal value", false},
		{"CRLF", "with\r\nCRLF", true},
		{"LF only", "with\nLF", true},
		{"CR only", "with\rCR", true},
		{"https url", "https://example.com", false},
		{"tab", "with\ttab", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := containsCRLF(tt.input)
			if result != tt.expected {
				t.Errorf("containsCRLF(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}
