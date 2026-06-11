package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoad_MergesDefaults(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "minimal.yaml")
	content := `
servers:
  - listen: ":8080"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.Performance.FileCache.MaxEntries == 0 {
		t.Error("Performance.FileCache.MaxEntries should have default value")
	}
	if cfg.Performance.FileCache.MaxSize == 0 {
		t.Error("Performance.FileCache.MaxSize should have default value")
	}
	if cfg.Performance.FileCache.Inactive == 0 {
		t.Error("Performance.FileCache.Inactive should have default value")
	}
	if cfg.Monitoring.Status.Path != "/_status" {
		t.Errorf("Monitoring.Status.Path = %q, want %q", cfg.Monitoring.Status.Path, "/_status")
	}
	if cfg.Monitoring.Pprof.Path != "/debug/pprof" {
		t.Errorf("Monitoring.Pprof.Path = %q, want %q", cfg.Monitoring.Pprof.Path, "/debug/pprof")
	}
	if cfg.Resolver.Valid == 0 {
		t.Error("Resolver.Valid should have default value")
	}
}

func TestLoad_ExplicitOverridesDefault(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "explicit.yaml")
	content := `
performance:
  file_cache:
    max_entries: 500
    max_size: 52428800
    inactive: 120s
servers:
  - listen: ":8080"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.Performance.FileCache.MaxEntries != 500 {
		t.Errorf("MaxEntries = %d, want 500", cfg.Performance.FileCache.MaxEntries)
	}
	if cfg.Performance.FileCache.MaxSize != 52428800 {
		t.Errorf("MaxSize = %d, want 52428800", cfg.Performance.FileCache.MaxSize)
	}
	if cfg.Performance.FileCache.Inactive != 120*time.Second {
		t.Errorf("Inactive = %v, want 120s", cfg.Performance.FileCache.Inactive)
	}
}

func TestLoad_MonitoringDisabledByDefault(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "minimal.yaml")
	content := `
servers:
  - listen: ":8080"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// 默认值 Path 存在，但 Enabled 应为 false
	if cfg.Monitoring.Status.Enabled {
		t.Error("Monitoring.Status.Enabled should be false by default")
	}
	if cfg.Monitoring.Pprof.Enabled {
		t.Error("Monitoring.Pprof.Enabled should be false by default")
	}
	if cfg.Monitoring.Status.Path != "/_status" {
		t.Errorf("Monitoring.Status.Path = %q, want %q", cfg.Monitoring.Status.Path, "/_status")
	}
}
