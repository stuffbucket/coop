package sandbox

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stuffbucket/coop/internal/platform"
)

func TestGetSensitiveHomeDirs(t *testing.T) {
	dirs := getSensitiveHomeDirs()

	// Common dirs should always be present
	commonExpected := []string{".ssh", ".gnupg", ".aws", ".config/coop", "Desktop", "Documents", "Downloads"}
	for _, expected := range commonExpected {
		found := false
		for _, d := range dirs {
			if d == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("getSensitiveHomeDirs() missing common dir %q", expected)
		}
	}

	// Platform-specific checks
	switch platform.Detect() {
	case platform.MacOS:
		macExpected := []string{"Library", "Library/Keychains"}
		for _, expected := range macExpected {
			found := false
			for _, d := range dirs {
				if d == expected {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("getSensitiveHomeDirs() on macOS missing %q", expected)
			}
		}
	case platform.Linux, platform.WSL2:
		linuxExpected := []string{".local/share/keyrings", ".pki"}
		for _, expected := range linuxExpected {
			found := false
			for _, d := range dirs {
				if d == expected {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("getSensitiveHomeDirs() on Linux missing %q", expected)
			}
		}
	}
}

func TestIsSeatbelted_CommonPaths(t *testing.T) {
	home := os.Getenv("HOME")
	if home == "" {
		t.Skip("HOME not set")
	}

	tests := []struct {
		name       string
		path       string
		wantBlock  bool
		wantReason string
	}{
		{
			name:      "ssh directory",
			path:      filepath.Join(home, ".ssh"),
			wantBlock: true,
		},
		{
			name:      "ssh subdirectory",
			path:      filepath.Join(home, ".ssh", "id_rsa"),
			wantBlock: true,
		},
		{
			name:      "aws credentials",
			path:      filepath.Join(home, ".aws"),
			wantBlock: true,
		},
		{
			name:      "coop config (hard block)",
			path:      filepath.Join(home, ".config", "coop"),
			wantBlock: true,
		},
		{
			name:      "gnupg",
			path:      filepath.Join(home, ".gnupg"),
			wantBlock: true,
		},
		{
			name:      "Desktop",
			path:      filepath.Join(home, "Desktop"),
			wantBlock: true,
		},
		{
			name:      "Documents",
			path:      filepath.Join(home, "Documents"),
			wantBlock: true,
		},
		{
			name:      "Downloads",
			path:      filepath.Join(home, "Downloads"),
			wantBlock: true,
		},
		{
			name:      "safe directory",
			path:      filepath.Join(home, "projects"),
			wantBlock: false,
		},
		{
			name:      "safe nested directory",
			path:      filepath.Join(home, "projects", "myapp"),
			wantBlock: false,
		},
		{
			name:      "tilde expansion",
			path:      "~/.ssh",
			wantBlock: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blocked, reason := IsSeatbelted(tt.path)
			if blocked != tt.wantBlock {
				t.Errorf("IsSeatbelted(%q) = %v, want %v (reason: %s)",
					tt.path, blocked, tt.wantBlock, reason)
			}
		})
	}
}

func TestIsSeatbelted_MacOSSpecific(t *testing.T) {
	if platform.Detect() != platform.MacOS {
		t.Skip("macOS-specific test")
	}

	home := os.Getenv("HOME")
	if home == "" {
		t.Skip("HOME not set")
	}

	tests := []struct {
		path      string
		wantBlock bool
	}{
		{filepath.Join(home, "Library"), true},
		{filepath.Join(home, "Library", "Keychains"), true},
		{filepath.Join(home, "Library", "Cookies"), true},
		{"/System", true},
		{"/usr/bin", true},
		{"/var/log", true},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			blocked, _ := IsSeatbelted(tt.path)
			if blocked != tt.wantBlock {
				t.Errorf("IsSeatbelted(%q) = %v, want %v", tt.path, blocked, tt.wantBlock)
			}
		})
	}
}

func TestIsSeatbelted_LinuxSpecific(t *testing.T) {
	p := platform.Detect()
	if p != platform.Linux && p != platform.WSL2 {
		t.Skip("Linux-specific test")
	}

	home := os.Getenv("HOME")
	if home == "" {
		t.Skip("HOME not set")
	}

	tests := []struct {
		path      string
		wantBlock bool
	}{
		{filepath.Join(home, ".local", "share", "keyrings"), true},
		{filepath.Join(home, ".pki"), true},
		{filepath.Join(home, ".password-store"), true},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			blocked, _ := IsSeatbelted(tt.path)
			if blocked != tt.wantBlock {
				t.Errorf("IsSeatbelted(%q) = %v, want %v", tt.path, blocked, tt.wantBlock)
			}
		})
	}
}

func TestExpandPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("UserHomeDir not available")
	}

	tests := []struct {
		input string
		want  string
	}{
		{"~", home},
		{"~/foo", filepath.Join(home, "foo")},
		{"~/foo/bar", filepath.Join(home, "foo", "bar")},
		{"/absolute/path", "/absolute/path"},
		{"relative/path", "relative/path"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := expandPath(tt.input)
			if got != tt.want {
				t.Errorf("expandPath(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
