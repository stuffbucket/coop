package sandbox

import (
	"testing"

	"github.com/stuffbucket/coop/internal/config"
)

// TestDefaultContainerConfig verifies that DefaultContainerConfig uses config defaults.
func TestDefaultContainerConfig(t *testing.T) {
	cfg := DefaultContainerConfig("test-container")

	// Verify name is set
	if cfg.Name != "test-container" {
		t.Errorf("Name = %q, want %q", cfg.Name, "test-container")
	}

	// Verify defaults match config.DefaultConfig()
	defaults := config.DefaultConfig()

	if cfg.CPUs != defaults.Settings.DefaultCPUs {
		t.Errorf("CPUs = %d, want %d", cfg.CPUs, defaults.Settings.DefaultCPUs)
	}

	if cfg.MemoryMB != defaults.Settings.DefaultMemoryMB {
		t.Errorf("MemoryMB = %d, want %d", cfg.MemoryMB, defaults.Settings.DefaultMemoryMB)
	}

	if cfg.DiskGB != defaults.Settings.DefaultDiskGB {
		t.Errorf("DiskGB = %d, want %d", cfg.DiskGB, defaults.Settings.DefaultDiskGB)
	}

	// Verify profiles include the agent profile
	hasAgentProfile := false
	for _, p := range cfg.Profiles {
		if p == AgentProfile {
			hasAgentProfile = true
			break
		}
	}
	if !hasAgentProfile {
		t.Errorf("Profiles %v does not include %q", cfg.Profiles, AgentProfile)
	}
}

// TestDefaultContainerConfigWithWorkDir verifies WorkingDir can be set.
func TestDefaultContainerConfigWithWorkDir(t *testing.T) {
	cfg := DefaultContainerConfig("test-agent")
	cfg.WorkingDir = "/path/to/workspace"

	if cfg.WorkingDir != "/path/to/workspace" {
		t.Errorf("WorkingDir = %q, want %q", cfg.WorkingDir, "/path/to/workspace")
	}
}

// TestNewManagerBackwardCompatibility documents that NewManager still works.
// This is an integration test - it will fail without Incus but documents the contract.
func TestNewManagerBackwardCompatibility(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// NewManager should still work (backward compat)
	// It internally calls NewManagerWithConfig with a default config
	mgr, err := NewManager()
	if err != nil {
		t.Skipf("skipping: Incus not available (%v)", err)
	}

	if mgr == nil {
		t.Fatal("NewManager returned nil manager")
		return
	}

	// Verify the manager has a non-nil config
	if mgr.config == nil {
		t.Error("Manager.config is nil, expected default config")
	}
}

// TestNewManagerWithConfigStoresConfig documents that config is stored.
// This is an integration test - it will fail without Incus but documents the contract.
func TestNewManagerWithConfigStoresConfig(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Create a custom config with distinct values
	cfg := config.DefaultConfig()
	cfg.Settings.DefaultImage = "test-custom-image"
	cfg.Settings.DefaultCPUs = 99 // Distinct value for verification

	mgr, err := NewManagerWithConfig(&cfg)
	if err != nil {
		t.Skipf("skipping: Incus not available (%v)", err)
	}
	if mgr == nil {
		t.Fatal("NewManagerWithConfig returned nil manager")
	}

	// Verify the config was stored
	if mgr.config == nil {
		t.Fatal("Manager.config is nil")
	}

	// Verify the specific config values were preserved
	if mgr.config.Settings.DefaultImage != "test-custom-image" {
		t.Errorf("config.Settings.DefaultImage = %q, want %q",
			mgr.config.Settings.DefaultImage, "test-custom-image")
	}

	if mgr.config.Settings.DefaultCPUs != 99 {
		t.Errorf("config.Settings.DefaultCPUs = %d, want %d",
			mgr.config.Settings.DefaultCPUs, 99)
	}
}

// TestGenerateName verifies name generation works.
func TestGenerateName(t *testing.T) {
	name1 := GenerateName()
	name2 := GenerateName()

	if name1 == "" {
		t.Error("GenerateName returned empty string")
	}

	// Names should be unique (statistically)
	if name1 == name2 {
		t.Errorf("GenerateName returned duplicate names: %q", name1)
	}
}
