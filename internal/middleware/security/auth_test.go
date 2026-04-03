package security

import (
	"testing"

	"github.com/valyala/fasthttp"
	"golang.org/x/crypto/bcrypt"
	"rua.plus/lolly/internal/config"
)

func TestNewBasicAuth(t *testing.T) {
	hashedPassword, _ := bcrypt.GenerateFromPassword([]byte("password"), bcrypt.DefaultCost)

	tests := []struct {
		name    string
		cfg     *config.AuthConfig
		wantErr bool
	}{
		{
			name:    "nil config",
			cfg:     nil,
			wantErr: true,
		},
		{
			name: "invalid type",
			cfg: &config.AuthConfig{
				Type: "digest",
			},
			wantErr: true,
		},
		{
			name: "no users",
			cfg: &config.AuthConfig{
				Type: "basic",
			},
			wantErr: true,
		},
		{
			name: "empty username",
			cfg: &config.AuthConfig{
				Type: "basic",
				Users: []config.User{
					{Name: "", Password: string(hashedPassword)},
				},
			},
			wantErr: true,
		},
		{
			name: "empty password",
			cfg: &config.AuthConfig{
				Type: "basic",
				Users: []config.User{
					{Name: "admin", Password: ""},
				},
			},
			wantErr: true,
		},
		{
			name: "valid config",
			cfg: &config.AuthConfig{
				Type: "basic",
				Users: []config.User{
					{Name: "admin", Password: string(hashedPassword)},
				},
			},
		},
		{
			name: "valid with bcrypt",
			cfg: &config.AuthConfig{
				Type:      "basic",
				Algorithm: "bcrypt",
				Users: []config.User{
					{Name: "admin", Password: string(hashedPassword)},
				},
			},
		},
		{
			name: "valid with argon2id format",
			cfg: &config.AuthConfig{
				Type:      "basic",
				Algorithm: "argon2id",
				Users: []config.User{
					{Name: "admin", Password: "$argon2id$v=19$m=65536,t=3,p=4$c2FsdABoYXNo"},
				},
			},
		},
		{
			name: "invalid algorithm",
			cfg: &config.AuthConfig{
				Type:      "basic",
				Algorithm: "md5",
				Users: []config.User{
					{Name: "admin", Password: string(hashedPassword)},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auth, err := NewBasicAuth(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewBasicAuth() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && auth == nil {
				t.Error("Expected non-nil BasicAuth")
			}
		})
	}
}

func TestBasicAuthAuthenticate(t *testing.T) {
	password := "testpassword"
	hashedPassword, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)

	auth, err := NewBasicAuth(&config.AuthConfig{
		Type: "basic",
		Users: []config.User{
			{Name: "admin", Password: string(hashedPassword)},
		},
	})
	if err != nil {
		t.Fatalf("NewBasicAuth() error: %v", err)
	}

	tests := []struct {
		name     string
		username string
		password string
		expected bool
	}{
		{
			name:     "valid credentials",
			username: "admin",
			password: password,
			expected: true,
		},
		{
			name:     "wrong password",
			username: "admin",
			password: "wrongpassword",
			expected: false,
		},
		{
			name:     "unknown user",
			username: "unknown",
			password: password,
			expected: false,
		},
		{
			name:     "empty username",
			username: "",
			password: password,
			expected: false,
		},
		{
			name:     "empty password",
			username: "admin",
			password: "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := auth.Authenticate(tt.username, tt.password)
			if result != tt.expected {
				t.Errorf("Authenticate(%s, ***) = %v, expected %v", tt.username, result, tt.expected)
			}
		})
	}
}

func TestBasicAuthProcess(t *testing.T) {
	password := "testpassword"
	hashedPassword, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)

	auth, err := NewBasicAuth(&config.AuthConfig{
		Type:      "basic",
		RequireTLS: false, // Disable TLS for testing
		Users: []config.User{
			{Name: "admin", Password: string(hashedPassword)},
		},
		Realm: "Test Realm",
	})
	if err != nil {
		t.Fatalf("NewBasicAuth() error: %v", err)
	}

	nextHandler := func(ctx *fasthttp.RequestCtx) {
		ctx.WriteString("OK")
	}

	handler := auth.Process(nextHandler)
	if handler == nil {
		t.Error("Process() returned nil handler")
	}
}

func TestBasicAuthAddUser(t *testing.T) {
	auth, err := NewBasicAuth(&config.AuthConfig{
		Type: "basic",
		Users: []config.User{
			{Name: "admin", Password: "$2b$12$existinghash"},
		},
	})
	if err != nil {
		t.Fatalf("NewBasicAuth() error: %v", err)
	}

	// Test adding user
	err = auth.AddUser("newuser", "$2b$12$newhash")
	if err != nil {
		t.Errorf("AddUser() error: %v", err)
	}

	if !auth.HasUser("newuser") {
		t.Error("Expected newuser to exist")
	}

	// Test empty username
	err = auth.AddUser("", "$2b$12$hash")
	if err == nil {
		t.Error("Expected error for empty username")
	}

	// Test invalid hash format
	err = auth.AddUser("user2", "invalidhash")
	if err == nil {
		t.Error("Expected error for invalid hash")
	}
}

func TestBasicAuthRemoveUser(t *testing.T) {
	auth, err := NewBasicAuth(&config.AuthConfig{
		Type: "basic",
		Users: []config.User{
			{Name: "admin", Password: "$2b$12$hash"},
		},
	})
	if err != nil {
		t.Fatalf("NewBasicAuth() error: %v", err)
	}

	// Remove existing user
	auth.RemoveUser("admin")

	if auth.HasUser("admin") {
		t.Error("Expected admin to be removed")
	}

	// Remove non-existent user (should not error)
	auth.RemoveUser("nonexistent")
}

func TestBasicAuthUserCount(t *testing.T) {
	auth, err := NewBasicAuth(&config.AuthConfig{
		Type: "basic",
		Users: []config.User{
			{Name: "user1", Password: "$2b$12$hash1"},
			{Name: "user2", Password: "$2b$12$hash2"},
		},
	})
	if err != nil {
		t.Fatalf("NewBasicAuth() error: %v", err)
	}

	if count := auth.UserCount(); count != 2 {
		t.Errorf("Expected UserCount 2, got %d", count)
	}

	auth.AddUser("user3", "$2b$12$hash3")
	if count := auth.UserCount(); count != 3 {
		t.Errorf("Expected UserCount 3, got %d", count)
	}

	auth.RemoveUser("user1")
	if count := auth.UserCount(); count != 2 {
		t.Errorf("Expected UserCount 2, got %d", count)
	}
}

func TestHashPasswordBcrypt(t *testing.T) {
	password := "testpassword"

	hash, err := HashPasswordBcrypt(password, bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("HashPasswordBcrypt() error: %v", err)
	}

	if hash == "" {
		t.Error("Expected non-empty hash")
	}

	// Verify the hash works
	err = bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	if err != nil {
		t.Errorf("Hash verification failed: %v", err)
	}
}

func TestValidatePasswordHash(t *testing.T) {
	tests := []struct {
		name      string
		hash      string
		algorithm HashAlgorithm
		wantErr   bool
	}{
		{
			name:      "valid bcrypt",
			hash:      "$2b$12$hash",
			algorithm: HashBcrypt,
		},
		{
			name:      "invalid bcrypt format",
			hash:      "nothere",
			algorithm: HashBcrypt,
			wantErr:   true,
		},
		{
			name:      "valid argon2id",
			hash:      "$argon2id$v=19$m=65536,t=3,p=4$salt$hash",
			algorithm: HashArgon2id,
		},
		{
			name:      "invalid argon2id format",
			hash:      "$bcrypt$hash",
			algorithm: HashArgon2id,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePasswordHash(tt.hash, tt.algorithm)
			if (err != nil) != tt.wantErr {
				t.Errorf("validatePasswordHash() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestExtractCredentials(t *testing.T) {
	password := "testpassword"
	hashedPassword, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)

	auth, err := NewBasicAuth(&config.AuthConfig{
		Type:      "basic",
		RequireTLS: false,
		Users: []config.User{
			{Name: "admin", Password: string(hashedPassword)},
		},
	})
	if err != nil {
		t.Fatalf("NewBasicAuth() error: %v", err)
	}

	// Create a mock request context
	ctx := &fasthttp.RequestCtx{}

	// Test without Authorization header
	_, _, ok := auth.extractCredentials(ctx)
	if ok {
		t.Error("Expected no credentials without header")
	}

	// Test with valid Basic auth header
	ctx.Request.Header.Set("Authorization", "Basic YWRtaW46dGVzdHBhc3N3b3Jk")
	username, pwd, ok := auth.extractCredentials(ctx)
	if !ok {
		t.Error("Expected credentials to be extracted")
	}
	if username != "admin" {
		t.Errorf("Expected username 'admin', got %s", username)
	}
	if pwd != "testpassword" {
		t.Errorf("Expected password 'testpassword', got %s", pwd)
	}
}

func TestName(t *testing.T) {
	password := "test"
	hashedPassword, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)

	auth, err := NewBasicAuth(&config.AuthConfig{
		Type: "basic",
		Users: []config.User{
			{Name: "admin", Password: string(hashedPassword)},
		},
	})
	if err != nil {
		t.Fatalf("NewBasicAuth() error: %v", err)
	}

	if auth.Name() != "basic_auth" {
		t.Errorf("Expected name 'basic_auth', got %s", auth.Name())
	}
}