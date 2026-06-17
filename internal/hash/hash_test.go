package hash

import (
	"hash/fnv"
	"testing"
)

func TestFNV64a(t *testing.T) {
	cases := []struct {
		input string
		want  uint64
	}{
		{"", 0xcbf29ce484222325},
		{"hello", 0xa430d84680aabd0b},
		{"a", 0xaf63dc4c8601ec8c},
		{"The quick brown fox jumps over the lazy dog", 0xf3f9b7f5e7e47110},
	}

	for _, tc := range cases {
		got := FNV64a(tc.input)
		if got != tc.want {
			t.Errorf("FNV64a(%q) = %#x, want %#x", tc.input, got, tc.want)
		}

		// Cross-check with stdlib FNV-1a implementation.
		h := fnv.New64a()
		_, _ = h.Write([]byte(tc.input))
		if got != h.Sum64() {
			t.Errorf("FNV64a(%q) = %#x, stdlib FNV-1a = %#x", tc.input, got, h.Sum64())
		}
	}
}

func TestFNV64aBytes(t *testing.T) {
	cases := []struct {
		input []byte
		want  uint64
	}{
		{[]byte{}, 0xcbf29ce484222325},
		{[]byte("hello"), 0xa430d84680aabd0b},
		{[]byte{0x00, 0x01, 0xff}, 0xd94a37186c0d38bf},
	}

	for _, tc := range cases {
		got := FNV64aBytes(tc.input)
		if got != tc.want {
			t.Errorf("FNV64aBytes(%q) = %#x, want %#x", tc.input, got, tc.want)
		}

		h := fnv.New64a()
		_, _ = h.Write(tc.input)
		if got != h.Sum64() {
			t.Errorf("FNV64aBytes(%q) = %#x, stdlib FNV-1a = %#x", tc.input, got, h.Sum64())
		}
	}
}

func TestFNV64aAndBytesConsistency(t *testing.T) {
	inputs := []string{
		"",
		"hello",
		"lolly",
		"The quick brown fox jumps over the lazy dog",
	}

	for _, s := range inputs {
		fromString := FNV64a(s)
		fromBytes := FNV64aBytes([]byte(s))
		if fromString != fromBytes {
			t.Errorf("FNV64a(%q) = %#x, FNV64aBytes(%q) = %#x", s, fromString, s, fromBytes)
		}
	}
}
