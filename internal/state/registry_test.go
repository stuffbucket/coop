package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRegistryBasic(t *testing.T) {
	tmpDir := t.TempDir()

	reg, err := LoadRegistry(tmpDir)
	if err != nil {
		t.Fatalf("LoadRegistry failed: %v", err)
	}

	// Record a published image
	err = reg.RecordPublish("my-image", "abc123fingerprint", "test-container", "checkpoint1")
	if err != nil {
		t.Fatalf("RecordPublish failed: %v", err)
	}

	// Query the source
	source := reg.GetSource("my-image")
	if source == nil {
		t.Fatal("GetSource returned nil")
	}
	if source.Instance != "test-container" {
		t.Errorf("Instance = %q, want %q", source.Instance, "test-container")
	}
	if source.Snapshot != "checkpoint1" {
		t.Errorf("Snapshot = %q, want %q", source.Snapshot, "checkpoint1")
	}

	// Unknown image returns nil
	if reg.GetSource("nonexistent") != nil {
		t.Error("GetSource should return nil for unknown image")
	}
}

func TestRegistryPersistence(t *testing.T) {
	tmpDir := t.TempDir()

	// Create and populate registry
	reg1, _ := LoadRegistry(tmpDir)
	_ = reg1.RecordPublish("dev-image", "def456", "mydev", "snap1")

	// Reload from disk
	reg2, err := LoadRegistry(tmpDir)
	if err != nil {
		t.Fatalf("LoadRegistry (reload) failed: %v", err)
	}

	source := reg2.GetSource("dev-image")
	if source == nil {
		t.Fatal("Image record not persisted")
	}
	if source.Instance != "mydev" {
		t.Errorf("Instance = %q, want %q", source.Instance, "mydev")
	}
}

func TestRegistryRemove(t *testing.T) {
	tmpDir := t.TempDir()

	reg, _ := LoadRegistry(tmpDir)
	_ = reg.RecordPublish("temp-image", "xyz", "container", "snap")

	if reg.GetSource("temp-image") == nil {
		t.Fatal("Image should exist before removal")
	}

	if err := reg.Remove("temp-image"); err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	if reg.GetSource("temp-image") != nil {
		t.Error("Image should not exist after removal")
	}

	// Verify persistence
	reg2, _ := LoadRegistry(tmpDir)
	if reg2.GetSource("temp-image") != nil {
		t.Error("Removal should be persisted")
	}
}

func TestRegistryFilePermissions(t *testing.T) {
	tmpDir := t.TempDir()

	reg, _ := LoadRegistry(tmpDir)
	_ = reg.RecordPublish("test", "fp", "c", "s")

	info, err := os.Stat(filepath.Join(tmpDir, "images.json"))
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}

	// Should be 0600 (owner read/write only)
	mode := info.Mode().Perm()
	if mode != 0600 {
		t.Errorf("File permissions = %o, want %o", mode, 0600)
	}
}
