package testutil

import (
	"testing"
	"time"
)

func TestNewTestProxyConfig(t *testing.T) {
	cfg := NewTestProxyConfig("/api", "http://localhost:8080")

	if cfg.Path != "/api" {
		t.Errorf("expected path /api, got %s", cfg.Path)
	}
	if len(cfg.Targets) != 1 {
		t.Errorf("expected 1 target, got %d", len(cfg.Targets))
	}
	if cfg.Timeout.Connect != 5*time.Second {
		t.Errorf("expected 5s connect timeout, got %v", cfg.Timeout.Connect)
	}
}

func TestNewTestHealthyTarget(t *testing.T) {
	target := NewTestHealthyTarget("http://localhost:8080")

	if target.URL != "http://localhost:8080" {
		t.Errorf("expected URL http://localhost:8080, got %s", target.URL)
	}
	if !target.Healthy.Load() {
		t.Error("expected target to be healthy")
	}
}

func TestNewTestHealthyTargets(t *testing.T) {
	targets := NewTestHealthyTargets("http://localhost:8080", "http://localhost:8081")

	if len(targets) != 2 {
		t.Errorf("expected 2 targets, got %d", len(targets))
	}
	for i, target := range targets {
		if !target.Healthy.Load() {
			t.Errorf("expected target %d to be healthy", i)
		}
	}
}
