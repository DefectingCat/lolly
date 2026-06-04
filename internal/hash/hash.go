package hash

// FNV64a computes FNV-1a hash without allocation.
func FNV64a(key string) uint64 {
	var h uint64 = 14695981039346656037
	for i := 0; i < len(key); i++ {
		h ^= uint64(key[i])
		h *= 1099511628211
	}
	return h
}

// FNV64aBytes computes FNV-1a hash from byte slice without allocation.
func FNV64aBytes(key []byte) uint64 {
	var h uint64 = 14695981039346656037
	for i := 0; i < len(key); i++ {
		h ^= uint64(key[i])
		h *= 1099511628211
	}
	return h
}
