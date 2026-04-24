// Package nginx provides a recursive descent parser for nginx configuration files.
package nginx

import (
	"fmt"
	"os"
	"path/filepath"
)

// Directive represents a single nginx directive.
type Directive struct {
	Name  string      // directive name (e.g., "server", "listen", "proxy_pass")
	Args  []string    // directive arguments
	Block []Directive // child directives for block directives (e.g., server { ... })
	Line  int         // line number in source file
	File  string      // source file path (for include tracking)
}

// NginxConfig represents a parsed nginx configuration.
type NginxConfig struct {
	Directives []Directive
}

// ParseError represents a parse error with file and line information.
type ParseError struct {
	File    string
	Line    int
	Message string
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("%s:%d: %s", e.File, e.Line, e.Message)
}

type parser struct {
	input           []byte
	pos             int
	line            int
	file            string
	includeStack    map[string]bool
	depth           int
	extraDirectives []Directive // directives injected by include expansion
}

const maxDepth = 10

// Parse parses nginx configuration from a string.
func Parse(input string) (*NginxConfig, error) {
	p := &parser{
		input:        []byte(input),
		pos:          0,
		line:         1,
		file:         "",
		includeStack: make(map[string]bool),
		depth:        0,
	}
	directives, err := p.parseDirectives()
	if err != nil {
		return nil, err
	}
	return &NginxConfig{Directives: directives}, nil
}

// ParseFile parses an nginx configuration file, handling include directives.
func ParseFile(path string) (*NginxConfig, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, &ParseError{File: path, Line: 1, Message: fmt.Sprintf("resolve absolute path: %v", err)}
	}
	resolved, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		return nil, &ParseError{File: path, Line: 1, Message: fmt.Sprintf("resolve symlinks: %v", err)}
	}
	return parseFileWithStack(resolved, map[string]bool{resolved: true}, 0)
}

func parseFileWithStack(path string, includeStack map[string]bool, depth int) (*NginxConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, &ParseError{File: path, Line: 1, Message: fmt.Sprintf("read file: %v", err)}
	}

	newStack := make(map[string]bool, len(includeStack)+1)
	for k := range includeStack {
		newStack[k] = true
	}
	newStack[path] = true

	p := &parser{
		input:        data,
		pos:          0,
		line:         1,
		file:         path,
		includeStack: newStack,
		depth:        depth,
	}
	directives, err := p.parseDirectives()
	if err != nil {
		return nil, err
	}
	return &NginxConfig{Directives: directives}, nil
}

func (p *parser) errorf(msg string, args ...any) error {
	return &ParseError{File: p.file, Line: p.line, Message: fmt.Sprintf(msg, args...)}
}

func (p *parser) parseDirectives() ([]Directive, error) {
	var directives []Directive
	for {
		p.skipWhitespaceAndComments()
		if p.pos >= len(p.input) {
			break
		}
		if p.input[p.pos] == '}' {
			break
		}
		d, err := p.parseDirective()
		if err != nil {
			return nil, err
		}
		// handleInclude may produce zero directives (glob no match) or
		// multiple directives (include expands to several files).
		if d == nil {
			continue
		}
		directives = append(directives, *d)

		// Drain any extra directives injected by include expansion.
		for _, extra := range p.extraDirectives {
			directives = append(directives, extra)
		}
		p.extraDirectives = nil
	}
	return directives, nil
}

func (p *parser) parseDirective() (*Directive, error) {
	p.skipWhitespaceAndComments()

	line := p.line
	name, err := p.readToken()
	if err != nil {
		return nil, err
	}
	if name == "" {
		return nil, p.errorf("expected directive name")
	}

	d := &Directive{
		Name: name,
		Line: line,
		File: p.file,
	}

	// Read arguments until ; or {
	for {
		p.skipWhitespaceAndComments()
		if p.pos >= len(p.input) {
			return nil, p.errorf("unexpected end of input, expected ';' or '{'")
		}

		ch := p.input[p.pos]
		if ch == ';' {
			p.pos++
			break
		}
		if ch == '{' {
			p.pos++
			block, err := p.parseDirectives()
			if err != nil {
				return nil, err
			}
			p.skipWhitespaceAndComments()
			if p.pos >= len(p.input) || p.input[p.pos] != '}' {
				return nil, p.errorf("expected '}'")
			}
			p.pos++
			d.Block = block
			break
		}

		arg, err := p.readToken()
		if err != nil {
			return nil, err
		}
		if arg == "" {
			return nil, p.errorf("unexpected character %q", p.input[p.pos])
		}
		d.Args = append(d.Args, arg)
	}

	// Handle include directive: replace with expanded content.
	if d.Name == "include" && len(d.Args) > 0 {
		return p.handleInclude(d.Args[0])
	}

	return d, nil
}

func (p *parser) handleInclude(pattern string) (*Directive, error) {
	if p.depth >= maxDepth {
		return nil, p.errorf("include depth exceeds maximum of %d", maxDepth)
	}

	var fullPattern string
	if filepath.IsAbs(pattern) {
		fullPattern = pattern
	} else {
		baseDir := filepath.Dir(p.file)
		fullPattern = filepath.Join(baseDir, pattern)
	}

	matches, err := filepath.Glob(fullPattern)
	if err != nil {
		return nil, p.errorf("invalid include pattern %q: %v", pattern, err)
	}

	if len(matches) == 0 {
		// If the pattern contains no glob metacharacters, it's a literal
		// file path that should exist. Return an error if it doesn't.
		if !isGlobPattern(pattern) {
			return nil, p.errorf("include file not found: %s", fullPattern)
		}
		// Glob pattern with no matches — silently skip (matches nginx behavior).
		return nil, nil
	}

	var allDirectives []Directive
	for _, match := range matches {
		resolved, err := filepath.EvalSymlinks(match)
		if err != nil {
			return nil, p.errorf("resolve symlinks for %q: %v", match, err)
		}
		if p.includeStack[resolved] {
			return nil, p.errorf("circular include detected: %s", resolved)
		}

		cfg, err := parseFileWithStack(resolved, p.includeStack, p.depth+1)
		if err != nil {
			return nil, err
		}
		allDirectives = append(allDirectives, cfg.Directives...)
	}

	if len(allDirectives) == 0 {
		return nil, nil
	}

	// Return the first directive; stash the rest for parseDirectives to drain.
	p.extraDirectives = allDirectives[1:]
	return &allDirectives[0], nil
}

func (p *parser) skipWhitespaceAndComments() {
	for p.pos < len(p.input) {
		ch := p.input[p.pos]
		if ch == ' ' || ch == '\t' || ch == '\r' {
			p.pos++
			continue
		}
		if ch == '\n' {
			p.pos++
			p.line++
			continue
		}
		if ch == '#' {
			for p.pos < len(p.input) && p.input[p.pos] != '\n' {
				p.pos++
			}
			continue
		}
		break
	}
}

func (p *parser) readToken() (string, error) {
	if p.pos >= len(p.input) {
		return "", nil
	}

	ch := p.input[p.pos]

	if ch == '"' || ch == '\'' {
		return p.readQuotedString(ch)
	}

	if ch == '{' || ch == '}' || ch == ';' {
		return "", nil
	}

	start := p.pos
	for p.pos < len(p.input) {
		ch = p.input[p.pos]
		if ch == ' ' || ch == '\t' || ch == '\r' || ch == '\n' ||
			ch == '{' || ch == '}' || ch == ';' || ch == '#' ||
			ch == '"' || ch == '\'' {
			break
		}
		p.pos++
	}
	if p.pos == start {
		return "", nil
	}
	return string(p.input[start:p.pos]), nil
}

func (p *parser) readQuotedString(quote byte) (string, error) {
	p.pos++ // skip opening quote
	var buf []byte
	for p.pos < len(p.input) {
		ch := p.input[p.pos]
		if ch == '\\' {
			p.pos++
			if p.pos >= len(p.input) {
				return "", p.errorf("unterminated escape in quoted string")
			}
			buf = append(buf, p.input[p.pos])
			p.pos++
			continue
		}
		if ch == quote {
			p.pos++ // skip closing quote
			return string(buf), nil
		}
		if ch == '\n' {
			p.line++
		}
		buf = append(buf, ch)
		p.pos++
	}
	return "", p.errorf("unterminated quoted string")
}

// isGlobPattern returns true if the path contains glob metacharacters.
func isGlobPattern(path string) bool {
	for i := 0; i < len(path); i++ {
		ch := path[i]
		if ch == '*' || ch == '?' || ch == '[' {
			return true
		}
	}
	return false
}
