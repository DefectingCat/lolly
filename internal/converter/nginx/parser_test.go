package nginx

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestParseSimpleDirective(t *testing.T) {
	cfg, err := Parse("listen 80;")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Directives) != 1 {
		t.Fatalf("expected 1 directive, got %d", len(cfg.Directives))
	}
	d := cfg.Directives[0]
	if d.Name != "listen" {
		t.Errorf("expected name %q, got %q", "listen", d.Name)
	}
	if len(d.Args) != 1 || d.Args[0] != "80" {
		t.Errorf("expected args [80], got %v", d.Args)
	}
}

func TestParseBlockDirective(t *testing.T) {
	cfg, err := Parse("server { listen 80; }")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Directives) != 1 {
		t.Fatalf("expected 1 directive, got %d", len(cfg.Directives))
	}
	d := cfg.Directives[0]
	if d.Name != "server" {
		t.Errorf("expected name %q, got %q", "server", d.Name)
	}
	if len(d.Block) != 1 {
		t.Fatalf("expected 1 child, got %d", len(d.Block))
	}
	if d.Block[0].Name != "listen" {
		t.Errorf("expected child name %q, got %q", "listen", d.Block[0].Name)
	}
	if len(d.Block[0].Args) != 1 || d.Block[0].Args[0] != "80" {
		t.Errorf("expected child args [80], got %v", d.Block[0].Args)
	}
}

func TestParseComment(t *testing.T) {
	cfg, err := Parse("# this is a comment\nlisten 80;")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Directives) != 1 {
		t.Fatalf("expected 1 directive, got %d", len(cfg.Directives))
	}
	if cfg.Directives[0].Name != "listen" {
		t.Errorf("expected name %q, got %q", "listen", cfg.Directives[0].Name)
	}
}

func TestParseQuotedString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		dirName  string
		expected []string
	}{
		{
			name:     "double quoted",
			input:    `proxy_set_header Host "example.com";`,
			dirName:  "proxy_set_header",
			expected: []string{"Host", "example.com"},
		},
		{
			name:     "single quoted",
			input:    `proxy_set_header Host 'example.com';`,
			dirName:  "proxy_set_header",
			expected: []string{"Host", "example.com"},
		},
		{
			name:     "escaped quote inside double",
			input:    `set $x "hello\"world";`,
			dirName:  "set",
			expected: []string{"$x", `hello"world`},
		},
		{
			name:     "escaped quote inside single",
			input:    `set $x 'hello\'world';`,
			dirName:  "set",
			expected: []string{"$x", "hello'world"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := Parse(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(cfg.Directives) != 1 {
				t.Fatalf("expected 1 directive, got %d", len(cfg.Directives))
			}
			d := cfg.Directives[0]
			if d.Name != tt.dirName {
				t.Errorf("expected name %q, got %q", tt.dirName, d.Name)
			}
			if len(d.Args) != len(tt.expected) {
				t.Fatalf("expected %d args, got %d", len(tt.expected), len(d.Args))
			}
			for i, want := range tt.expected {
				if d.Args[i] != want {
					t.Errorf("arg[%d]: expected %q, got %q", i, want, d.Args[i])
				}
			}
		})
	}
}

func TestParseMultipleDirectives(t *testing.T) {
	input := `listen 80;
server_name example.com;`
	cfg, err := Parse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Directives) != 2 {
		t.Fatalf("expected 2 directives, got %d", len(cfg.Directives))
	}
	if cfg.Directives[0].Name != "listen" {
		t.Errorf("directive[0]: expected %q, got %q", "listen", cfg.Directives[0].Name)
	}
	if cfg.Directives[1].Name != "server_name" {
		t.Errorf("directive[1]: expected %q, got %q", "server_name", cfg.Directives[1].Name)
	}
}

func TestParseNestedBlocks(t *testing.T) {
	input := `http { server { location / { root /var/www; } } }`
	cfg, err := Parse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Directives) != 1 {
		t.Fatalf("expected 1 top-level directive, got %d", len(cfg.Directives))
	}
	http := cfg.Directives[0]
	if http.Name != "http" || len(http.Block) != 1 {
		t.Fatalf("expected http with 1 child")
	}
	srv := http.Block[0]
	if srv.Name != "server" || len(srv.Block) != 1 {
		t.Fatalf("expected server with 1 child")
	}
	loc := srv.Block[0]
	if loc.Name != "location" || len(loc.Block) != 1 {
		t.Fatalf("expected location with 1 child")
	}
	if loc.Block[0].Name != "root" {
		t.Errorf("expected root, got %q", loc.Block[0].Name)
	}
}

func TestParseUnclosedBlock(t *testing.T) {
	_, err := Parse("server { listen 80;")
	if err == nil {
		t.Fatal("expected error for unclosed block")
	}
	pe, ok := err.(*ParseError)
	if !ok {
		t.Fatalf("expected *ParseError, got %T", err)
	}
	if pe.Line == 0 {
		t.Error("expected non-zero line number")
	}
}

func TestParseMissingSemicolon(t *testing.T) {
	_, err := Parse("listen 80")
	if err == nil {
		t.Fatal("expected error for missing semicolon")
	}
	pe, ok := err.(*ParseError)
	if !ok {
		t.Fatalf("expected *ParseError, got %T", err)
	}
	if pe.Line == 0 {
		t.Error("expected non-zero line number")
	}
}

func TestParseFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nginx.conf")
	content := "listen 80;\nserver_name example.com;"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	cfg, err := ParseFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Directives) != 2 {
		t.Fatalf("expected 2 directives, got %d", len(cfg.Directives))
	}
	if cfg.Directives[0].Name != "listen" {
		t.Errorf("directive[0]: expected %q, got %q", "listen", cfg.Directives[0].Name)
	}
	if cfg.Directives[1].Name != "server_name" {
		t.Errorf("directive[1]: expected %q, got %q", "server_name", cfg.Directives[1].Name)
	}
}

func TestParseIncludeGlob(t *testing.T) {
	dir := t.TempDir()

	// Create a subdirectory for included files to avoid matching nginx.conf itself.
	incDir := filepath.Join(dir, "includes")
	if err := os.Mkdir(incDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(incDir, "a.conf"), []byte("listen 80;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(incDir, "b.conf"), []byte("server_name a.com;\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create main config with include.
	main := filepath.Join(dir, "nginx.conf")
	content := "include " + incDir + "/*.conf;"
	if err := os.WriteFile(main, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := ParseFile(main)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Directives) != 2 {
		t.Fatalf("expected 2 directives from included files, got %d", len(cfg.Directives))
	}
	names := map[string]bool{}
	for _, d := range cfg.Directives {
		names[d.Name] = true
	}
	if !names["listen"] || !names["server_name"] {
		t.Errorf("expected listen and server_name, got %v", cfg.Directives)
	}
}

func TestParseIncludeCircular(t *testing.T) {
	dir := t.TempDir()

	a := filepath.Join(dir, "a.conf")
	b := filepath.Join(dir, "b.conf")

	if err := os.WriteFile(a, []byte("include "+b+";"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("include "+a+";"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := ParseFile(a)
	if err == nil {
		t.Fatal("expected error for circular include")
	}
	pe, ok := err.(*ParseError)
	if !ok {
		t.Fatalf("expected *ParseError, got %T", err)
	}
	if pe.Message == "" {
		t.Error("expected non-empty error message")
	}
}

func TestParseIncludeMaxDepth(t *testing.T) {
	dir := t.TempDir()

	// Create a chain of includes: 0.conf includes 1.conf, 1.conf includes 2.conf, etc.
	for i := 0; i <= maxDepth+1; i++ {
		path := filepath.Join(dir, fmt.Sprintf("%d.conf", i))
		var content string
		if i < maxDepth+1 {
			next := filepath.Join(dir, fmt.Sprintf("%d.conf", i+1))
			content = "include " + next + ";"
		} else {
			content = "listen 80;"
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	_, err := ParseFile(filepath.Join(dir, "0.conf"))
	if err == nil {
		t.Fatal("expected error for max include depth")
	}
	if _, ok := err.(*ParseError); !ok {
		t.Fatalf("expected *ParseError, got %T", err)
	}
}

func TestParseIncludeNotFound(t *testing.T) {
	dir := t.TempDir()
	main := filepath.Join(dir, "nginx.conf")
	content := "include /nonexistent/path.conf;"
	if err := os.WriteFile(main, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := ParseFile(main)
	if err == nil {
		t.Fatal("expected error for include of nonexistent file")
	}
}

func TestParseIncludeGlobNoMatch(t *testing.T) {
	dir := t.TempDir()
	main := filepath.Join(dir, "nginx.conf")
	content := "include " + dir + "/nonexistent/*.conf;"
	if err := os.WriteFile(main, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := ParseFile(main)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Directives) != 0 {
		t.Fatalf("expected 0 directives for glob with no matches, got %d", len(cfg.Directives))
	}
}

func TestParseLocationModifiers(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		dirName string
		args    []string
	}{
		{
			name:    "exact match",
			input:   "location = /path {}",
			dirName: "location",
			args:    []string{"=", "/path"},
		},
		{
			name:    "regex",
			input:   `location ~ \.php$ {}`,
			dirName: "location",
			args:    []string{"~", `\.php$`},
		},
		{
			name:    "case insensitive regex",
			input:   `location ~* \.jpg$ {}`,
			dirName: "location",
			args:    []string{"~*", `\.jpg$`},
		},
		{
			name:    "prefix with continuation",
			input:   "location ^~ /images/ {}",
			dirName: "location",
			args:    []string{"^~", "/images/"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := Parse(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(cfg.Directives) != 1 {
				t.Fatalf("expected 1 directive, got %d", len(cfg.Directives))
			}
			d := cfg.Directives[0]
			if d.Name != tt.dirName {
				t.Errorf("expected name %q, got %q", tt.dirName, d.Name)
			}
			if len(d.Args) != len(tt.args) {
				t.Fatalf("expected %d args, got %d: %v", len(tt.args), len(d.Args), d.Args)
			}
			for i, want := range tt.args {
				if d.Args[i] != want {
					t.Errorf("arg[%d]: expected %q, got %q", i, want, d.Args[i])
				}
			}
		})
	}
}

func TestParseEmptyBlock(t *testing.T) {
	cfg, err := Parse("server {}")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Directives) != 1 {
		t.Fatalf("expected 1 directive, got %d", len(cfg.Directives))
	}
	d := cfg.Directives[0]
	if d.Name != "server" {
		t.Errorf("expected name %q, got %q", "server", d.Name)
	}
	if len(d.Block) != 0 {
		t.Errorf("expected empty block, got %d children", len(d.Block))
	}
}

func TestParseMultipleArgs(t *testing.T) {
	cfg, err := Parse("return 301 https://example.com;")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Directives) != 1 {
		t.Fatalf("expected 1 directive, got %d", len(cfg.Directives))
	}
	d := cfg.Directives[0]
	if d.Name != "return" {
		t.Errorf("expected name %q, got %q", "return", d.Name)
	}
	expected := []string{"301", "https://example.com"}
	if len(d.Args) != len(expected) {
		t.Fatalf("expected %d args, got %d", len(expected), len(d.Args))
	}
	for i, want := range expected {
		if d.Args[i] != want {
			t.Errorf("arg[%d]: expected %q, got %q", i, want, d.Args[i])
		}
	}
}
