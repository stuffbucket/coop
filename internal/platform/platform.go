// Package platform provides host platform detection.
package platform

import (
	"os"
	"runtime"
	"strings"
)

// Type represents the host platform type.
type Type int

const (
	Linux Type = iota
	MacOS
	WSL2
	Unknown
)

// String returns the string representation of the platform.
func (p Type) String() string {
	switch p {
	case Linux:
		return "linux"
	case MacOS:
		return "darwin"
	case WSL2:
		return "wsl2"
	default:
		return "unknown"
	}
}

// Detect determines the current platform.
func Detect() Type {
	switch runtime.GOOS {
	case "linux":
		if isWSL() {
			return WSL2
		}
		return Linux
	case "darwin":
		return MacOS
	default:
		return Unknown
	}
}

// isWSL checks if running in Windows Subsystem for Linux.
func isWSL() bool {
	data, err := os.ReadFile("/proc/version")
	if err != nil {
		return false
	}
	lower := strings.ToLower(string(data))
	return strings.Contains(lower, "microsoft") || strings.Contains(lower, "wsl")
}

// IsMacOS returns true if running on macOS.
func IsMacOS() bool {
	return runtime.GOOS == "darwin"
}

// IsLinux returns true if running on Linux (including WSL2).
func IsLinux() bool {
	return runtime.GOOS == "linux"
}

// RequiresVM returns true if the platform requires a VM for containers.
// macOS requires a VM (Colima/Lima) to run Incus containers.
func RequiresVM() bool {
	return Detect() == MacOS
}

// HostArch returns the host architecture in Incus/VM format.
func HostArch() string {
	switch runtime.GOARCH {
	case "arm64":
		return "aarch64"
	case "amd64":
		return "x86_64"
	default:
		return runtime.GOARCH
	}
}

// IsAppleSilicon returns true if running on Apple Silicon (M1/M2/M3).
func IsAppleSilicon() bool {
	return runtime.GOOS == "darwin" && runtime.GOARCH == "arm64"
}
