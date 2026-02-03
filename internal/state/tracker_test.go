package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTrackerBasic(t *testing.T) {
	tmpDir := t.TempDir()

	tracker, err := NewTracker(tmpDir, "test-instance", "ubuntu:24.04")
	if err != nil {
		t.Fatalf("NewTracker failed: %v", err)
	}

	// Check initial state
	inst := tracker.Instance()
	if inst.Name != "test-instance" {
		t.Errorf("Name = %q, want %q", inst.Name, "test-instance")
	}
	if inst.BaseImage != "ubuntu:24.04" {
		t.Errorf("BaseImage = %q, want %q", inst.BaseImage, "ubuntu:24.04")
	}

	// Verify git repo was created
	gitDir := filepath.Join(tmpDir, "test-instance", ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		t.Error(".git directory not created")
	}

	// Verify state.json was created
	stateFile := filepath.Join(tmpDir, "test-instance", "state.json")
	if _, err := os.Stat(stateFile); os.IsNotExist(err) {
		t.Error("state.json not created")
	}
}

func TestTrackerPackages(t *testing.T) {
	tmpDir := t.TempDir()

	tracker, err := NewTracker(tmpDir, "pkg-test", "ubuntu:24.04")
	if err != nil {
		t.Fatalf("NewTracker failed: %v", err)
	}

	// Install some packages
	if _, err := tracker.RecordPackageInstall("apt", []string{"vim", "git"}); err != nil {
		t.Fatalf("RecordPackageInstall failed: %v", err)
	}
	if _, err := tracker.RecordPackageInstall("pip", []string{"numpy", "pandas"}); err != nil {
		t.Fatalf("RecordPackageInstall failed: %v", err)
	}

	// Verify state
	inst := tracker.Instance()
	if len(inst.Packages.Apt) != 2 {
		t.Errorf("Apt packages = %v, want 2 items", inst.Packages.Apt)
	}
	if len(inst.Packages.Pip) != 2 {
		t.Errorf("Pip packages = %v, want 2 items", inst.Packages.Pip)
	}

	// Check history
	history, err := tracker.History(10)
	if err != nil {
		t.Fatalf("History failed: %v", err)
	}
	if len(history) != 3 { // initial + 2 installs
		t.Errorf("History has %d commits, want 3", len(history))
	}
}

func TestTrackerMounts(t *testing.T) {
	tmpDir := t.TempDir()

	tracker, err := NewTracker(tmpDir, "mount-test", "ubuntu:24.04")
	if err != nil {
		t.Fatalf("NewTracker failed: %v", err)
	}

	// Add mount
	if _, err := tracker.RecordMount("workspace", "/Users/me/code", "/home/agent/code", false); err != nil {
		t.Fatalf("RecordMount failed: %v", err)
	}

	inst := tracker.Instance()
	if len(inst.Mounts) != 1 {
		t.Fatalf("Mounts = %d, want 1", len(inst.Mounts))
	}
	if inst.Mounts[0].Name != "workspace" {
		t.Errorf("Mount name = %q, want %q", inst.Mounts[0].Name, "workspace")
	}

	// Remove mount
	if _, err := tracker.RecordUnmount("workspace"); err != nil {
		t.Fatalf("RecordUnmount failed: %v", err)
	}

	inst = tracker.Instance()
	if len(inst.Mounts) != 0 {
		t.Errorf("Mounts after unmount = %d, want 0", len(inst.Mounts))
	}
}

func TestTrackerSnapshots(t *testing.T) {
	tmpDir := t.TempDir()

	tracker, err := NewTracker(tmpDir, "snap-test", "ubuntu:24.04")
	if err != nil {
		t.Fatalf("NewTracker failed: %v", err)
	}

	// Create snapshot
	hash, err := tracker.RecordSnapshot("checkpoint1", "before risky change")
	if err != nil {
		t.Fatalf("RecordSnapshot failed: %v", err)
	}

	if hash == "" {
		t.Error("RecordSnapshot returned empty hash")
	}

	// Verify current snapshot is set
	inst := tracker.Instance()
	if inst.CurrentSnapshot != "checkpoint1" {
		t.Errorf("CurrentSnapshot = %q, want %q", inst.CurrentSnapshot, "checkpoint1")
	}
}

func TestTrackerSnapshotLinkSurvivesReload(t *testing.T) {
	tmpDir := t.TempDir()

	// Create tracker and snapshot
	tracker1, err := NewTracker(tmpDir, "survive-test", "ubuntu:24.04")
	if err != nil {
		t.Fatalf("NewTracker failed: %v", err)
	}

	_, err = tracker1.RecordSnapshot("checkpoint1", "test")
	if err != nil {
		t.Fatalf("RecordSnapshot failed: %v", err)
	}

	// Simulate restart - create new tracker for same instance
	tracker2, err := NewTracker(tmpDir, "survive-test", "")
	if err != nil {
		t.Fatalf("NewTracker (reload) failed: %v", err)
	}

	// UndoToSnapshot should work - link survives in links.json
	commitHash, err := tracker2.UndoToSnapshot("checkpoint1")
	if err != nil {
		t.Fatalf("UndoToSnapshot after reload failed: %v", err)
	}
	if commitHash == "" {
		t.Error("UndoToSnapshot returned empty commit hash")
	}
}

func TestTrackerPersistence(t *testing.T) {
	tmpDir := t.TempDir()

	// Create tracker and make changes
	tracker1, err := NewTracker(tmpDir, "persist-test", "ubuntu:24.04")
	if err != nil {
		t.Fatalf("NewTracker failed: %v", err)
	}
	if _, err := tracker1.RecordPackageInstall("apt", []string{"vim"}); err != nil {
		t.Fatalf("RecordPackageInstall failed: %v", err)
	}

	// Open same instance with new tracker
	tracker2, err := NewTracker(tmpDir, "persist-test", "")
	if err != nil {
		t.Fatalf("NewTracker (reopen) failed: %v", err)
	}

	// Should have the previous state
	inst := tracker2.Instance()
	if len(inst.Packages.Apt) != 1 || inst.Packages.Apt[0] != "vim" {
		t.Errorf("Apt packages after reopen = %v, want [vim]", inst.Packages.Apt)
	}
}

func TestTrackerUndo(t *testing.T) {
	tmpDir := t.TempDir()

	tracker, err := NewTracker(tmpDir, "undo-test", "ubuntu:24.04")
	if err != nil {
		t.Fatalf("NewTracker failed: %v", err)
	}

	// Make some changes
	if _, err := tracker.RecordPackageInstall("apt", []string{"vim"}); err != nil {
		t.Fatalf("RecordPackageInstall failed: %v", err)
	}

	// Snapshot at known-good state
	_, err = tracker.RecordSnapshot("good-state", "")
	if err != nil {
		t.Fatalf("RecordSnapshot failed: %v", err)
	}

	// Make more changes
	if _, err := tracker.RecordPackageInstall("pip", []string{"bad-package"}); err != nil {
		t.Fatalf("RecordPackageInstall failed: %v", err)
	}

	// Verify we have the bad package
	inst := tracker.Instance()
	if len(inst.Packages.Pip) != 1 {
		t.Fatalf("expected 1 pip package before undo, got %d", len(inst.Packages.Pip))
	}

	// Undo back to snapshot by name
	commitHash, err := tracker.UndoToSnapshot("good-state")
	if err != nil {
		t.Fatalf("UndoToSnapshot failed: %v", err)
	}

	if commitHash == "" {
		t.Error("UndoToSnapshot returned empty commit hash")
	}

	// Verify state was reset - pip packages should be gone
	inst = tracker.Instance()
	if len(inst.Packages.Pip) != 0 {
		t.Errorf("expected 0 pip packages after undo, got %d: %v", len(inst.Packages.Pip), inst.Packages.Pip)
	}

	// Verify apt packages still there (from before snapshot)
	if len(inst.Packages.Apt) != 1 || inst.Packages.Apt[0] != "vim" {
		t.Errorf("expected [vim] apt packages after undo, got %v", inst.Packages.Apt)
	}

	// The caller would now run: incus restore undo-test good-state
}

func TestTrackerUndoRemovesLaterCommits(t *testing.T) {
	tmpDir := t.TempDir()

	tracker, err := NewTracker(tmpDir, "reset-test", "ubuntu:24.04")
	if err != nil {
		t.Fatalf("NewTracker failed: %v", err)
	}

	// Create snapshot (link stored in links.json, not git)
	_, err = tracker.RecordSnapshot("checkpoint", "")
	if err != nil {
		t.Fatalf("RecordSnapshot failed: %v", err)
	}

	// Make changes after snapshot
	if _, err := tracker.RecordPackageInstall("apt", []string{"vim"}); err != nil {
		t.Fatalf("RecordPackageInstall failed: %v", err)
	}
	if _, err := tracker.RecordPackageInstall("apt", []string{"git"}); err != nil {
		t.Fatalf("RecordPackageInstall failed: %v", err)
	}

	// Should have 4 commits: initial, snapshot, vim, git
	history, _ := tracker.History(10)
	if len(history) != 4 {
		t.Fatalf("expected 4 commits before undo, got %d", len(history))
	}

	// Undo to checkpoint by name
	if _, err := tracker.UndoToSnapshot("checkpoint"); err != nil {
		t.Fatalf("UndoToSnapshot failed: %v", err)
	}

	// Should now have 2 commits: initial, snapshot (later commits gone)
	history, _ = tracker.History(10)
	if len(history) != 2 {
		t.Errorf("expected 2 commits after undo, got %d", len(history))
		for _, c := range history {
			t.Logf("  commit: %s", c.Message)
		}
	}
}

func TestValidateInstanceName(t *testing.T) {
	cases := []struct {
		name    string
		wantErr bool
	}{
		{"valid-name", false},
		{"my_instance_123", false},
		{"", true},
		{".", true},
		{"..", true},
		{"../etc/passwd", true},
		{"foo/bar", true},
		{"foo\\bar", true},
		{"has\x00null", true},
	}

	for _, tc := range cases {
		err := ValidateInstanceName(tc.name)
		if tc.wantErr && err == nil {
			t.Errorf("ValidateInstanceName(%q) should have failed", tc.name)
		}
		if !tc.wantErr && err != nil {
			t.Errorf("ValidateInstanceName(%q) failed: %v", tc.name, err)
		}
	}
}

func TestPathTraversalBlocked(t *testing.T) {
	tmpDir := t.TempDir()

	// Attempt path traversal - should fail
	_, err := NewTracker(tmpDir, "../../../etc", "ubuntu:24.04")
	if err == nil {
		t.Error("NewTracker should reject path traversal")
	}
}
