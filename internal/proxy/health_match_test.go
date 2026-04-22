package proxy

import (
	"testing"
)

func TestDefaultHealthMatch(t *testing.T) {
	m := DefaultHealthMatch()

	tests := []struct {
		status int
		want   bool
	}{
		{200, true},
		{201, true},
		{299, true},
		{300, false},
		{400, false},
		{500, false},
		{199, false},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			got := m.Match(tt.status, nil, nil)
			if got != tt.want {
				t.Errorf("Match(%d) = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}

func TestCustomHealthMatch_StatusRange(t *testing.T) {
	cfg := &HealthMatchConfig{
		Status: []string{"200-299", "301", "302"},
	}
	m := NewHealthMatch(cfg)

	tests := []struct {
		status int
		want   bool
	}{
		{200, true},
		{250, true},
		{299, true},
		{301, true},
		{302, true},
		{300, false}, // 不在范围内
		{303, false},
		{400, false},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			got := m.Match(tt.status, nil, nil)
			if got != tt.want {
				t.Errorf("Match(%d) = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}

func TestCustomHealthMatch_BodyRegex(t *testing.T) {
	cfg := &HealthMatchConfig{
		Status: []string{"200"},
		Body:   `"status":"ok"`,
	}
	m := NewHealthMatch(cfg)

	tests := []struct {
		name   string
		status int
		body   string
		want   bool
	}{
		{
			name:   "matching body",
			status: 200,
			body:   `{"status":"ok","data":{}}`,
			want:   true,
		},
		{
			name:   "non-matching body",
			status: 200,
			body:   `{"status":"error"}`,
			want:   false,
		},
		{
			name:   "wrong status",
			status: 500,
			body:   `{"status":"ok"}`,
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := m.Match(tt.status, []byte(tt.body), nil)
			if got != tt.want {
				t.Errorf("Match() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCustomHealthMatch_Headers(t *testing.T) {
	cfg := &HealthMatchConfig{
		Status: []string{"200"},
		Headers: map[string]string{
			"X-Health": "ok",
		},
	}
	m := NewHealthMatch(cfg)

	tests := []struct {
		name    string
		status  int
		headers map[string]string
		want    bool
	}{
		{
			name:   "matching header",
			status: 200,
			headers: map[string]string{
				"x-health": "ok",
			},
			want: true,
		},
		{
			name:   "missing header",
			status: 200,
			headers: map[string]string{
				"content-type": "application/json",
			},
			want: false,
		},
		{
			name:   "wrong value",
			status: 200,
			headers: map[string]string{
				"x-health": "error",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := m.Match(tt.status, nil, tt.headers)
			if got != tt.want {
				t.Errorf("Match() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewHealthMatch_NilConfig(t *testing.T) {
	m := NewHealthMatch(nil)

	// 应该返回默认匹配器
	if !m.Match(200, nil, nil) {
		t.Error("nil config should return default matcher")
	}
	if m.Match(300, nil, nil) {
		t.Error("default matcher should not match 300")
	}
}

func TestNewHealthMatch_EmptyStatus(t *testing.T) {
	cfg := &HealthMatchConfig{
		Status: []string{}, // 空
	}
	m := NewHealthMatch(cfg)

	// 应该使用默认 2xx 范围
	if !m.Match(200, nil, nil) {
		t.Error("empty status should default to 2xx")
	}
	if m.Match(300, nil, nil) {
		t.Error("empty status should default to 2xx, not match 300")
	}
}

func TestParseStatusRange(t *testing.T) {
	tests := []struct {
		input   string
		min     int
		max     int
		wantErr bool
	}{
		{"200", 200, 200, false},
		{"200-299", 200, 299, false},
		{" 200-299 ", 200, 299, false},
		{"200 - 299", 200, 299, false},
		{"abc", 0, 0, true},
		{"200-abc", 0, 0, true},
		{"200-300-400", 0, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			r, err := parseStatusRange(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if r.min != tt.min || r.max != tt.max {
				t.Errorf("range = {%d, %d}, want {%d, %d}", r.min, r.max, tt.min, tt.max)
			}
		})
	}
}

func TestCustomHealthMatch_Combined(t *testing.T) {
	cfg := &HealthMatchConfig{
		Status: []string{"200-299"},
		Body:   `"healthy":true`,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
	}
	m := NewHealthMatch(cfg)

	tests := []struct {
		name    string
		status  int
		body    string
		headers map[string]string
		want    bool
	}{
		{
			name:   "all match",
			status: 200,
			body:   `{"healthy":true,"status":"ok"}`,
			headers: map[string]string{
				"content-type": "application/json",
			},
			want: true,
		},
		{
			name:   "status mismatch",
			status: 400,
			body:   `{"healthy":true}`,
			headers: map[string]string{
				"content-type": "application/json",
			},
			want: false,
		},
		{
			name:   "body mismatch",
			status: 200,
			body:   `{"healthy":false}`,
			headers: map[string]string{
				"content-type": "application/json",
			},
			want: false,
		},
		{
			name:   "header mismatch",
			status: 200,
			body:   `{"healthy":true}`,
			headers: map[string]string{
				"content-type": "text/plain",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := m.Match(tt.status, []byte(tt.body), tt.headers)
			if got != tt.want {
				t.Errorf("Match() = %v, want %v", got, tt.want)
			}
		})
	}
}
