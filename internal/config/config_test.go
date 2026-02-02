package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetDirectories(t *testing.T) {
	dirs := GetDirectories()

	// All paths should be non-empty
	if dirs.Config == "" {
		t.Error("Config directory should not be empty")
	}
	if dirs.Data == "" {
		t.Error("Data directory should not be empty")
	}
	if dirs.Cache == "" {
		t.Error("Cache directory should not be empty")
	}

	// Derived paths should be based on base paths
	if filepath.Dir(dirs.SSH) != dirs.Config {
		t.Errorf("SSH dir %q should be under config dir %q", dirs.SSH, dirs.Config)
	}
	if filepath.Dir(dirs.SettingsFile) != dirs.Config {
		t.Errorf("Settings file %q should be under config dir %q", dirs.SettingsFile, dirs.Config)
	}
	if filepath.Dir(dirs.Images) != dirs.Data {
		t.Errorf("Images dir %q should be under data dir %q", dirs.Images, dirs.Data)
	}
	if filepath.Dir(dirs.Logs) != dirs.Data {
		t.Errorf("Logs dir %q should be under data dir %q", dirs.Logs, dirs.Data)
	}
}

func TestGetDirectoriesEnvOverride(t *testing.T) {
	// Set custom dirs (t.Setenv handles cleanup)
	t.Setenv("COOP_CONFIG_DIR", "/tmp/test-config")
	t.Setenv("COOP_DATA_DIR", "/tmp/test-data")
	t.Setenv("COOP_CACHE_DIR", "/tmp/test-cache")

	dirs := GetDirectories()

	if dirs.Config != "/tmp/test-config" {
		t.Errorf("Expected config dir /tmp/test-config, got %q", dirs.Config)
	}
	if dirs.Data != "/tmp/test-data" {
		t.Errorf("Expected data dir /tmp/test-data, got %q", dirs.Data)
	}
	if dirs.Cache != "/tmp/test-cache" {
		t.Errorf("Expected cache dir /tmp/test-cache, got %q", dirs.Cache)
	}
}

func TestGetDirectoriesXDGOverride(t *testing.T) {
	// Clear COOP_* vars by setting empty (t.Setenv handles cleanup)
	t.Setenv("COOP_CONFIG_DIR", "")
	t.Setenv("COOP_DATA_DIR", "")
	t.Setenv("COOP_CACHE_DIR", "")

	// Set XDG vars
	t.Setenv("XDG_CONFIG_HOME", "/tmp/xdg-config")
	t.Setenv("XDG_DATA_HOME", "/tmp/xdg-data")
	t.Setenv("XDG_CACHE_HOME", "/tmp/xdg-cache")

	dirs := GetDirectories()

	if dirs.Config != "/tmp/xdg-config/coop" {
		t.Errorf("Expected config dir /tmp/xdg-config/coop, got %q", dirs.Config)
	}
	if dirs.Data != "/tmp/xdg-data/coop" {
		t.Errorf("Expected data dir /tmp/xdg-data/coop, got %q", dirs.Data)
	}
	if dirs.Cache != "/tmp/xdg-cache/coop" {
		t.Errorf("Expected cache dir /tmp/xdg-cache/coop, got %q", dirs.Cache)
	}
}

func TestLoadDefaults(t *testing.T) {
	// Use temp directory to avoid loading real settings
	tmpDir := t.TempDir()
	t.Setenv("COOP_CONFIG_DIR", tmpDir)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Check defaults
	if cfg.Settings.DefaultCPUs != 2 {
		t.Errorf("Expected DefaultCPUs=2, got %d", cfg.Settings.DefaultCPUs)
	}
	if cfg.Settings.DefaultMemoryMB != 4096 {
		t.Errorf("Expected DefaultMemoryMB=4096, got %d", cfg.Settings.DefaultMemoryMB)
	}
	if cfg.Settings.DefaultDiskGB != 20 {
		t.Errorf("Expected DefaultDiskGB=20, got %d", cfg.Settings.DefaultDiskGB)
	}
	if cfg.Settings.DefaultImage != "coop-agent-base" {
		t.Errorf("Expected DefaultImage=coop-agent-base, got %q", cfg.Settings.DefaultImage)
	}

	// VM defaults
	if cfg.Settings.VM.Instance != "incus" {
		t.Errorf("Expected VM.Instance=incus, got %q", cfg.Settings.VM.Instance)
	}
	if cfg.Settings.VM.CPUs != 4 {
		t.Errorf("Expected VM.CPUs=4, got %d", cfg.Settings.VM.CPUs)
	}
	if cfg.Settings.VM.MemoryGB != 8 {
		t.Errorf("Expected VM.MemoryGB=8, got %d", cfg.Settings.VM.MemoryGB)
	}

	// Log defaults
	if cfg.Settings.Log.MaxSizeMB != 10 {
		t.Errorf("Expected Log.MaxSizeMB=10, got %d", cfg.Settings.Log.MaxSizeMB)
	}
	if cfg.Settings.Log.MaxBackups != 3 {
		t.Errorf("Expected Log.MaxBackups=3, got %d", cfg.Settings.Log.MaxBackups)
	}

	// Network defaults
	if cfg.Settings.Network.HostsDomain != "incus.local" {
		t.Errorf("Expected Network.HostsDomain=incus.local, got %q", cfg.Settings.Network.HostsDomain)
	}
}

func TestLoadEnvOverrides(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("COOP_CONFIG_DIR", tmpDir)
	t.Setenv("COOP_DEFAULT_IMAGE", "custom-image")
	t.Setenv("COOP_VM_BACKEND", "lima")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if cfg.Settings.DefaultImage != "custom-image" {
		t.Errorf("Expected DefaultImage=custom-image, got %q", cfg.Settings.DefaultImage)
	}
	if len(cfg.Settings.VM.BackendPriority) != 1 || cfg.Settings.VM.BackendPriority[0] != "lima" {
		t.Errorf("Expected VM.BackendPriority=[lima], got %v", cfg.Settings.VM.BackendPriority)
	}
}

func TestEnsureDirectories(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("COOP_CONFIG_DIR", filepath.Join(tmpDir, "config"))
	t.Setenv("COOP_DATA_DIR", filepath.Join(tmpDir, "data"))
	t.Setenv("COOP_CACHE_DIR", filepath.Join(tmpDir, "cache"))

	err := EnsureDirectories()
	if err != nil {
		t.Fatalf("EnsureDirectories() failed: %v", err)
	}

	dirs := GetDirectories()

	// Check they exist
	dirsToCheck := []string{
		dirs.Config,
		dirs.Data,
		dirs.Cache,
		dirs.SSH,
		dirs.Images,
		dirs.Logs,
	}

	for _, d := range dirsToCheck {
		info, err := os.Stat(d)
		if err != nil {
			t.Errorf("Directory %q should exist: %v", d, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("%q should be a directory", d)
		}
	}

	// Check SSH dir has restricted permissions
	info, _ := os.Stat(dirs.SSH)
	if info.Mode().Perm() != 0700 {
		t.Errorf("SSH dir should have mode 0700, got %o", info.Mode().Perm())
	}

	// Check settings.json exists
	if _, err := os.Stat(dirs.SettingsFile); err != nil {
		t.Errorf("Settings file should exist: %v", err)
	}
}

func TestSaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("COOP_CONFIG_DIR", tmpDir)

	// Create initial config
	cfg := &Config{
		Dirs: GetDirectories(),
		Settings: Settings{
			DefaultCPUs:     8,
			DefaultMemoryMB: 16384,
			DefaultImage:    "test-image",
		},
	}

	// Save it
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	// Load it back
	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if loaded.Settings.DefaultCPUs != 8 {
		t.Errorf("Expected DefaultCPUs=8, got %d", loaded.Settings.DefaultCPUs)
	}
	if loaded.Settings.DefaultMemoryMB != 16384 {
		t.Errorf("Expected DefaultMemoryMB=16384, got %d", loaded.Settings.DefaultMemoryMB)
	}
	if loaded.Settings.DefaultImage != "test-image" {
		t.Errorf("Expected DefaultImage=test-image, got %q", loaded.Settings.DefaultImage)
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	// Should have sensible defaults even without a settings file
	if cfg.Settings.DefaultCPUs < 1 {
		t.Error("DefaultConfig should have positive DefaultCPUs")
	}
	if cfg.Settings.DefaultMemoryMB < 1024 {
		t.Error("DefaultConfig should have reasonable DefaultMemoryMB")
	}
	if cfg.Dirs.Config == "" {
		t.Error("DefaultConfig should have non-empty Dirs.Config")
	}
}
