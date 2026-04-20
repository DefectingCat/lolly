package matcher

import (
	"testing"
)

func TestConflictDetector_New(t *testing.T) {
	cd := NewConflictDetector()
	if cd.registeredPaths == nil {
		t.Error("registeredPaths should be initialized")
	}
}

func TestConflictDetector_Register(t *testing.T) {
	cd := NewConflictDetector()

	err := cd.Register("/api", "exact")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cd.Exists("/api") {
		t.Error("path should exist after register")
	}
}

func TestConflictDetector_Register_Duplicate(t *testing.T) {
	cd := NewConflictDetector()

	cd.Register("/api", "exact")
	err := cd.Register("/api", "prefix")
	if err == nil {
		t.Fatal("expected conflict error")
	}
	expected := "path conflict: '/api' already registered as 'exact', trying to register as 'prefix'"
	if err.Error() != expected {
		t.Errorf("expected %q, got %q", expected, err.Error())
	}
}

func TestConflictDetector_Exists(t *testing.T) {
	cd := NewConflictDetector()

	if cd.Exists("/api") {
		t.Error("should not exist before register")
	}

	cd.Register("/api", "exact")
	if !cd.Exists("/api") {
		t.Error("should exist after register")
	}
}

func TestConflictDetector_Exists_EmptyString(t *testing.T) {
	cd := NewConflictDetector()

	cd.Register("", "exact")
	if !cd.Exists("") {
		t.Error("empty string path should be supported")
	}
}

func TestConflictDetector_GetRegisteredPaths(t *testing.T) {
	cd := NewConflictDetector()
	cd.Register("/api", "exact")
	cd.Register("/web", "prefix")

	paths := cd.GetRegisteredPaths()
	if len(paths) != 2 {
		t.Fatalf("expected 2 paths, got %d", len(paths))
	}
	if paths["/api"] != "exact" {
		t.Errorf("expected /api -> exact, got %s", paths["/api"])
	}
	if paths["/web"] != "prefix" {
		t.Errorf("expected /web -> prefix, got %s", paths["/web"])
	}
}

func TestConflictDetector_GetRegisteredPaths_Empty(t *testing.T) {
	cd := NewConflictDetector()

	paths := cd.GetRegisteredPaths()
	if len(paths) != 0 {
		t.Errorf("expected empty map, got %d entries", len(paths))
	}
}

func TestConflictDetector_Remove(t *testing.T) {
	cd := NewConflictDetector()
	cd.Register("/api", "exact")

	cd.Remove("/api")
	if cd.Exists("/api") {
		t.Error("path should not exist after remove")
	}
}

func TestConflictDetector_Remove_NonExistent(t *testing.T) {
	cd := NewConflictDetector()

	// Should not panic on removing non-existent path
	cd.Remove("/nonexistent")
	if cd.Exists("/nonexistent") {
		t.Error("should not exist")
	}
}

func TestConflictDetector_Clear(t *testing.T) {
	cd := NewConflictDetector()
	cd.Register("/api", "exact")
	cd.Register("/web", "prefix")

	cd.Clear()
	paths := cd.GetRegisteredPaths()
	if len(paths) != 0 {
		t.Errorf("expected empty after clear, got %d entries", len(paths))
	}
}

func TestConflictDetector_UnicodePaths(t *testing.T) {
	cd := NewConflictDetector()

	err := cd.Register("/cafe\u0301", "exact") // café with combining accent
	if err != nil {
		t.Fatalf("unicode path should be supported: %v", err)
	}
	if !cd.Exists("/cafe\u0301") {
		t.Error("unicode path should exist")
	}
}

func TestConflictDetector_SpecialCharPaths(t *testing.T) {
	cd := NewConflictDetector()

	paths := []string{
		"/api?query=1",
		"/path with spaces",
		"/path\twith\ttabs",
		"/#fragment",
	}

	for _, p := range paths {
		err := cd.Register(p, "exact")
		if err != nil {
			t.Errorf("special char path %q should be supported: %v", p, err)
		}
		if !cd.Exists(p) {
			t.Errorf("special char path %q should exist", p)
		}
	}
}
