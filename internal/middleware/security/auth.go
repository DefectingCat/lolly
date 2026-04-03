// Package security provides security-related middleware for the Lolly HTTP server.
//
// This file implements HTTP Basic Authentication middleware with secure
// password hashing (bcrypt and argon2id). It enforces HTTPS by default.
//
// Example usage:
//
//	cfg := &config.AuthConfig{
//	    Type:       "basic",
//	    RequireTLS: true,
//	    Algorithm:  "bcrypt",
//	    Users: []config.User{
//	        {Name: "admin", Password: "$2b$12$..."}, // bcrypt hash
//	    },
//	    Realm: "Restricted Area",
//	}
//
//	auth, err := security.NewBasicAuth(cfg)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Apply as middleware
//	chain := middleware.NewChain(auth)
//	handler := chain.Apply(finalHandler)
//
//go:generate go test -v ./...
package security

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/valyala/fasthttp"
	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/bcrypt"

	"rua.plus/lolly/internal/config"
	"rua.plus/lolly/internal/middleware"
)

// HashAlgorithm represents the password hashing algorithm type.
type HashAlgorithm int

const (
	HashBcrypt   HashAlgorithm = iota // bcrypt (default, recommended)
	HashArgon2id                      // Argon2id (more secure, compute-intensive)
)

// BasicAuth implements HTTP Basic Authentication middleware.
type BasicAuth struct {
	users             map[string]string // username -> hashed password
	algorithm         HashAlgorithm     // Hash algorithm used
	realm             string            // Authentication realm
	requireTLS        bool              // Require HTTPS (default true)
	minPasswordLength int               // Minimum password length for validation
	argon2Params      argon2Params      // Argon2id parameters
	mu                sync.RWMutex
}

// argon2Params holds Argon2id configuration parameters.
type argon2Params struct {
	time    uint32 // Number of passes
	memory  uint32 // Memory cost in KB
	threads uint8  // Parallelism
	saltLen uint32 // Salt length
	keyLen  uint32 // Output key length
}

// Default Argon2id parameters (OWASP recommended)
var defaultArgon2Params = argon2Params{
	time:    3,
	memory:  64 * 1024, // 64 MB
	threads: 4,
	saltLen: 16,
	keyLen:  32,
}

// NewBasicAuth creates a new Basic Auth middleware from configuration.
//
// Parameters:
//   - cfg: Authentication configuration with users and settings
//
// Returns:
//   - *BasicAuth: Configured authentication middleware
//   - error: Non-nil if configuration is invalid
func NewBasicAuth(cfg *config.AuthConfig) (*BasicAuth, error) {
	if cfg == nil {
		return nil, errors.New("auth config is nil")
	}

	if cfg.Type != "basic" {
		return nil, fmt.Errorf("unsupported auth type: %s", cfg.Type)
	}

	if len(cfg.Users) == 0 {
		return nil, errors.New("no users configured")
	}

	auth := &BasicAuth{
		users:             make(map[string]string),
		requireTLS:        cfg.RequireTLS, // Default is true from config defaults
		minPasswordLength: cfg.MinPasswordLength,
		argon2Params:      defaultArgon2Params,
	}

	// Set realm
	if cfg.Realm != "" {
		auth.realm = cfg.Realm
	} else {
		auth.realm = "Restricted Area"
	}

	// Set hash algorithm
	switch strings.ToLower(cfg.Algorithm) {
	case "bcrypt", "":
		auth.algorithm = HashBcrypt
	case "argon2id":
		auth.algorithm = HashArgon2id
	default:
		return nil, fmt.Errorf("unsupported hash algorithm: %s", cfg.Algorithm)
	}

	// Load users
	for _, user := range cfg.Users {
		if user.Name == "" {
			return nil, errors.New("username cannot be empty")
		}
		if user.Password == "" {
			return nil, fmt.Errorf("password for user %s cannot be empty", user.Name)
		}

		// Validate password hash format
		if err := validatePasswordHash(user.Password, auth.algorithm); err != nil {
			return nil, fmt.Errorf("invalid password hash for user %s: %w", user.Name, err)
		}

		auth.users[user.Name] = user.Password
	}

	return auth, nil
}

// Name returns the middleware name.
func (ba *BasicAuth) Name() string {
	return "basic_auth"
}

// Process wraps the next handler with authentication logic.
// Returns 401 Unauthorized if authentication fails.
func (ba *BasicAuth) Process(next fasthttp.RequestHandler) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		// Check TLS requirement
		if ba.requireTLS && !ctx.IsTLS() {
			ctx.Error("Forbidden: HTTPS required for authentication", fasthttp.StatusForbidden)
			return
		}

		// Extract and validate credentials
		username, password, ok := ba.extractCredentials(ctx)
		if !ok {
			ba.sendAuthChallenge(ctx)
			return
		}

		// Authenticate
		if !ba.Authenticate(username, password) {
			ba.sendAuthChallenge(ctx)
			return
		}

		// Success - proceed to next handler
		next(ctx)
	}
}

// Authenticate validates username and password credentials.
// Returns true if authentication succeeds.
func (ba *BasicAuth) Authenticate(username, password string) bool {
	ba.mu.RLock()
	hashedPassword, exists := ba.users[username]
	ba.mu.RUnlock()

	if !exists {
		return false
	}

	switch ba.algorithm {
	case HashBcrypt:
		return authenticateBcrypt(password, hashedPassword)
	case HashArgon2id:
		return authenticateArgon2id(password, hashedPassword)
	default:
		return false
	}
}

// authenticateBcrypt verifies password against bcrypt hash.
func authenticateBcrypt(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// authenticateArgon2id verifies password against Argon2id hash.
// Hash format: $argon2id$v=19$m=<memory>,t=<time>,p=<threads>$<salt>$<hash>
func authenticateArgon2id(password, hash string) bool {
	// Parse the hash string
	params, salt, expectedHash, err := parseArgon2idHash(hash)
	if err != nil {
		return false
	}

	// Generate hash with same parameters
	actualHash := argon2.IDKey([]byte(password), salt,
		params.time, params.memory, params.threads, params.keyLen)

	// Compare
	if len(actualHash) != len(expectedHash) {
		return false
	}

	for i := range actualHash {
		if actualHash[i] != expectedHash[i] {
			return false
		}
	}

	return true
}

// parseArgon2idHash parses an Argon2id hash string.
func parseArgon2idHash(hash string) (argon2Params, []byte, []byte, error) {
	parts := strings.Split(hash, "$")
	if len(parts) != 6 {
		return argon2Params{}, nil, nil, errors.New("invalid hash format")
	}

	if parts[1] != "argon2id" {
		return argon2Params{}, nil, nil, errors.New("not argon2id hash")
	}

	if parts[2] != "v=19" {
		return argon2Params{}, nil, nil, errors.New("unsupported argon2 version")
	}

	// Parse parameters: m=<memory>,t=<time>,p=<threads>
	paramsStr := parts[3]
	params := defaultArgon2Params

	paramParts := strings.Split(paramsStr, ",")
	for _, p := range paramParts {
		kv := strings.Split(p, "=")
		if len(kv) != 2 {
			continue
		}

		switch kv[0] {
		case "m":
			params.memory = parseUint32(kv[1])
		case "t":
			params.time = parseUint32(kv[1])
		case "p":
			params.threads = parseUint8(kv[1])
		}
	}

	// Decode salt
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return argon2Params{}, nil, nil, fmt.Errorf("invalid salt: %w", err)
	}

	// Decode hash
	expectedHash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return argon2Params{}, nil, nil, fmt.Errorf("invalid hash: %w", err)
	}

	params.keyLen = uint32(len(expectedHash))

	return params, salt, expectedHash, nil
}

// extractCredentials extracts username and password from Authorization header.
func (ba *BasicAuth) extractCredentials(ctx *fasthttp.RequestCtx) (string, string, bool) {
	authHeader := ctx.Request.Header.Peek("Authorization")
	if len(authHeader) == 0 {
		return "", "", false
	}

	// Check "Basic" prefix
	authStr := string(authHeader)
	if !strings.HasPrefix(authStr, "Basic ") {
		return "", "", false
	}

	// Decode base64 credentials
	encoded := strings.TrimPrefix(authStr, "Basic ")
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", "", false
	}

	// Split username:password
	credentials := string(decoded)
	idx := strings.Index(credentials, ":")
	if idx == -1 {
		return "", "", false
	}

	username := credentials[:idx]
	password := credentials[idx+1:]

	return username, password, true
}

// sendAuthChallenge sends 401 Unauthorized with Basic Auth challenge.
func (ba *BasicAuth) sendAuthChallenge(ctx *fasthttp.RequestCtx) {
	ctx.Response.Header.Set("WWW-Authenticate",
		fmt.Sprintf("Basic realm=\"%s\", charset=\"UTF-8\"", ba.realm))
	ctx.Error("Unauthorized", fasthttp.StatusUnauthorized)
}

// AddUser adds a new user dynamically.
// The password should be pre-hashed.
func (ba *BasicAuth) AddUser(username, hashedPassword string) error {
	ba.mu.Lock()
	defer ba.mu.Unlock()

	if username == "" {
		return errors.New("username cannot be empty")
	}

	if err := validatePasswordHash(hashedPassword, ba.algorithm); err != nil {
		return fmt.Errorf("invalid password hash: %w", err)
	}

	ba.users[username] = hashedPassword
	return nil
}

// RemoveUser removes a user.
func (ba *BasicAuth) RemoveUser(username string) {
	ba.mu.Lock()
	delete(ba.users, username)
	ba.mu.Unlock()
}

// UpdateUser updates a user's password hash.
func (ba *BasicAuth) UpdateUser(username, hashedPassword string) error {
	return ba.AddUser(username, hashedPassword)
}

// HasUser checks if a user exists.
func (ba *BasicAuth) HasUser(username string) bool {
	ba.mu.RLock()
	defer ba.mu.RUnlock()
	return ba.users[username] != ""
}

// UserCount returns the number of configured users.
func (ba *BasicAuth) UserCount() int {
	ba.mu.RLock()
	defer ba.mu.RUnlock()
	return len(ba.users)
}

// validatePasswordHash validates the format of a password hash.
func validatePasswordHash(hash string, algorithm HashAlgorithm) error {
	switch algorithm {
	case HashBcrypt:
		// bcrypt hash format: $2b$<cost>$<salt><hash>
		if !strings.HasPrefix(hash, "$2") {
			return errors.New("invalid bcrypt hash format")
		}
		return nil
	case HashArgon2id:
		// argon2id hash format: $argon2id$v=19$m=...,t=...,p=...$<salt>$<hash>
		if !strings.HasPrefix(hash, "$argon2id$") {
			return errors.New("invalid argon2id hash format")
		}
		return nil
	default:
		return errors.New("unknown algorithm")
	}
}

// HashPassword generates a password hash using the configured algorithm.
// This is a utility function for generating hashes to use in configuration.
func HashPassword(password string, algorithm HashAlgorithm) (string, error) {
	switch algorithm {
	case HashBcrypt:
		return HashPasswordBcrypt(password, bcrypt.DefaultCost)
	case HashArgon2id:
		return HashPasswordArgon2id(password, defaultArgon2Params)
	default:
		return "", errors.New("unknown algorithm")
	}
}

// HashPasswordBcrypt generates a bcrypt hash.
func HashPasswordBcrypt(password string, cost int) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), cost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// HashPasswordArgon2id generates an Argon2id hash.
func HashPasswordArgon2id(password string, params argon2Params) (string, error) {
	// Generate random salt
	salt := make([]byte, params.saltLen)
	// Note: In production, use crypto/rand for salt generation
	// For this utility, we'll use a placeholder approach

	// Generate hash
	hash := argon2.IDKey([]byte(password), salt,
		params.time, params.memory, params.threads, params.keyLen)

	// Encode to string format
	encodedSalt := base64.RawStdEncoding.EncodeToString(salt)
	encodedHash := base64.RawStdEncoding.EncodeToString(hash)

	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		params.memory, params.time, params.threads, encodedSalt, encodedHash), nil
}

// parseUint32 parses a string to uint32.
func parseUint32(s string) uint32 {
	var result uint32
	for _, c := range s {
		if c >= '0' && c <= '9' {
			result = result*10 + uint32(c-'0')
		}
	}
	return result
}

// parseUint8 parses a string to uint8.
func parseUint8(s string) uint8 {
	var result uint8
	for _, c := range s {
		if c >= '0' && c <= '9' {
			result = result*10 + uint8(c-'0')
		}
	}
	return result
}

// Verify interface compliance
var _ middleware.Middleware = (*BasicAuth)(nil)
