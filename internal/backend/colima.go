package backend

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/stuffbucket/coop/internal/config"
	"github.com/stuffbucket/coop/internal/logging"
)

// ColimaBackend implements Backend using Colima.
type ColimaBackend struct {
	cfg *config.Config
}

// NewColimaBackend creates a new Colima backend.
func NewColimaBackend(cfg *config.Config) *ColimaBackend {
	return &ColimaBackend{cfg: cfg}
}

func (c *ColimaBackend) Name() string {
	return "colima"
}

func (c *ColimaBackend) Available() bool {
	return commandExists("colima")
}

func (c *ColimaBackend) profileName() string {
	if c.cfg.Settings.VM.Instance != "" {
		return c.cfg.Settings.VM.Instance
	}
	return "incus"
}

// validateProfileName checks that a profile name is safe for use in file paths.
// Prevents path traversal attacks when constructing socket paths.
func validateProfileName(name string) error {
	if name == "" {
		return fmt.Errorf("profile name cannot be empty")
	}
	if name == "." || name == ".." {
		return fmt.Errorf("invalid profile name: %q", name)
	}
	if strings.ContainsAny(name, "/\\\x00") {
		return fmt.Errorf("profile name contains invalid characters: %q", name)
	}
	return nil
}

// validateSocketPath checks that a socket path is safe and absolute.
func validateSocketPath(path string) error {
	if path == "" {
		return fmt.Errorf("socket path cannot be empty")
	}
	cleaned := filepath.Clean(path)
	if !filepath.IsAbs(cleaned) {
		return fmt.Errorf("socket path must be absolute: %q", path)
	}
	if strings.Contains(cleaned, "..") {
		return fmt.Errorf("socket path contains path traversal: %q", path)
	}
	return nil
}

// validateConfig checks for invalid VM configuration combinations.
func (c *ColimaBackend) validateConfig() error {
	vm := c.cfg.Settings.VM

	// Validate arch
	validArch := map[string]bool{"": true, "host": true, "aarch64": true, "x86_64": true}
	if !validArch[vm.Arch] {
		return fmt.Errorf("invalid arch %q: must be 'host', 'aarch64', or 'x86_64'", vm.Arch)
	}

	// Validate vm_type
	validVMType := map[string]bool{"": true, "vz": true, "qemu": true}
	if !validVMType[vm.VMType] {
		return fmt.Errorf("invalid vm_type %q: must be 'vz' or 'qemu'", vm.VMType)
	}

	// VZ is only available on macOS
	if vm.VMType == "vz" && runtime.GOOS != "darwin" {
		return fmt.Errorf("vm_type 'vz' is only available on macOS")
	}

	// Rosetta requires VZ and aarch64 host
	if vm.Rosetta {
		if vm.VMType == "qemu" {
			return fmt.Errorf("rosetta requires vm_type 'vz', not 'qemu'")
		}
		if runtime.GOARCH != "arm64" {
			return fmt.Errorf("rosetta requires Apple Silicon (aarch64 host)")
		}
		// Rosetta emulates x86_64, so arch must be host/aarch64 (Rosetta runs inside the aarch64 VM)
		if vm.Arch == "x86_64" {
			return fmt.Errorf("rosetta runs inside an aarch64 VM to emulate x86_64; set arch='host' or 'aarch64', not 'x86_64'")
		}
	}

	// Nested virtualization requires VZ
	if vm.NestedVirtualization && vm.VMType == "qemu" {
		return fmt.Errorf("nested_virtualization requires vm_type 'vz', not 'qemu'")
	}

	// VZ on non-native arch requires QEMU
	if vm.VMType == "vz" {
		var hostArch string
		switch runtime.GOARCH {
		case "arm64":
			hostArch = "aarch64"
		case "amd64":
			hostArch = "x86_64"
		default:
			hostArch = runtime.GOARCH
		}
		if vm.Arch != "" && vm.Arch != "host" && vm.Arch != hostArch {
			return fmt.Errorf("vm_type 'vz' only supports native architecture; for %s emulation use vm_type 'qemu'", vm.Arch)
		}
	}

	return nil
}

// colimaStatus represents colima list JSON output
type colimaStatus struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Arch    string `json:"arch"`
	CPUs    int    `json:"cpus"`
	Memory  int64  `json:"memory"` // bytes
	Disk    int64  `json:"disk"`   // bytes
	Runtime string `json:"runtime"`
}

func (c *ColimaBackend) Status() (*Status, error) {
	log := logging.Get()
	profile := c.profileName()

	cmd := exec.Command("colima", "list", "--json")
	log.Cmd("colima", []string{"list", "--json"})

	output, err := cmd.Output()
	if err != nil {
		log.CmdOutput("colima", output, err)
		return &Status{Name: profile, State: StateMissing}, nil
	}
	log.CmdOutput("colima", output, nil)

	// Colima outputs one JSON object per line
	decoder := json.NewDecoder(bytes.NewReader(output))
	for decoder.More() {
		var st colimaStatus
		if err := decoder.Decode(&st); err != nil {
			continue
		}
		if st.Name == profile {
			state := StateStopped
			if strings.ToLower(st.Status) == "running" {
				state = StateRunning
			}

			return &Status{
				Name:     st.Name,
				State:    state,
				CPUs:     st.CPUs,
				MemoryGB: int(st.Memory / (1024 * 1024 * 1024)),
				DiskGB:   int(st.Disk / (1024 * 1024 * 1024)),
				Arch:     st.Arch,
				Runtime:  st.Runtime,
			}, nil
		}
	}

	return &Status{Name: profile, State: StateMissing}, nil
}

func (c *ColimaBackend) Start() error {
	profile := c.profileName()
	vm := c.cfg.Settings.VM

	status, err := c.Status()
	if err != nil {
		return err
	}

	if status.State == StateRunning {
		return nil
	}

	// Validate configuration before creating new VM
	if status.State == StateMissing {
		if err := c.validateConfig(); err != nil {
			return fmt.Errorf("invalid VM configuration: %w", err)
		}
	}

	args := []string{"start", profile, "--runtime", "incus"}

	// Add resource args for new instance
	if status.State == StateMissing {
		// Architecture (aarch64, x86_64, or host)
		if vm.Arch != "" {
			args = append(args, "--arch", vm.Arch)
		}

		if vm.CPUs > 0 {
			args = append(args, "--cpu", fmt.Sprintf("%d", vm.CPUs))
		}
		if vm.MemoryGB > 0 {
			args = append(args, "--memory", fmt.Sprintf("%d", vm.MemoryGB))
		}
		if vm.DiskGB > 0 {
			args = append(args, "--disk", fmt.Sprintf("%d", vm.DiskGB))
		}

		// VM type (vz or qemu)
		if vm.VMType != "" {
			args = append(args, "--vm-type", vm.VMType)
		}

		// VZ-specific options (only valid with vz or when vz is default)
		if vm.VMType == "vz" || vm.VMType == "" {
			if vm.Rosetta {
				args = append(args, "--vz-rosetta")
			}
			if vm.NestedVirtualization {
				args = append(args, "--nested-virtualization")
			}
		}

		// DNS servers
		for _, dns := range vm.DNS {
			args = append(args, "--dns", dns)
		}
	}

	// Handle storage pool recovery prompt
	// Colima/Incus asks: "existing Incus data found, would you like to recover the storage pool(s)? [y/N]"
	autoRecover := vm.StorageAutoRecover != nil && *vm.StorageAutoRecover
	stdin := "n\n"
	if autoRecover {
		stdin = "y\n"
	}

	return runStreamingCmdWithStdin("colima", args, stdin, fmt.Sprintf("failed to start colima VM %q", profile))
}

func (c *ColimaBackend) Stop() error {
	profile := c.profileName()
	return runStreamingCmd("colima", []string{"stop", profile}, fmt.Sprintf("failed to stop colima VM %q", profile))
}

func (c *ColimaBackend) Delete() error {
	profile := c.profileName()
	return runStreamingCmd("colima", []string{"delete", profile, "--force"}, fmt.Sprintf("failed to delete colima VM %q", profile))
}

func (c *ColimaBackend) Shell() error {
	profile := c.profileName()
	return runInteractiveCmd("colima", []string{"ssh", profile}, fmt.Sprintf("failed to open shell in colima VM %q", profile))
}

func (c *ColimaBackend) Exec(command []string) ([]byte, error) {
	profile := c.profileName()
	args := append([]string{"ssh", profile, "--"}, command...)
	return runOutputCmd("colima", args, fmt.Sprintf("failed to exec in colima VM %q", profile))
}

func (c *ColimaBackend) GetIncusSocket() (string, error) {
	// Check explicit config first
	if c.cfg.Settings.IncusSocket != "" {
		return c.cfg.Settings.IncusSocket, nil
	}

	profile := c.profileName()

	// Validate profile name before using in paths
	if err := validateProfileName(profile); err != nil {
		return "", fmt.Errorf("invalid VM instance name: %w", err)
	}

	// Try to query Colima for the actual socket location
	if socketPath, err := c.queryColimaSocket(profile); err == nil {
		// Validate and clean the returned path
		if err := validateSocketPath(socketPath); err != nil {
			return "", fmt.Errorf("invalid socket path from colima: %w", err)
		}
		return "unix://" + filepath.Clean(socketPath), nil
	}

	// Fallback: try common locations
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	possiblePaths := []string{
		// New XDG-compliant path (Colima 0.6.0+)
		filepath.Clean(filepath.Join(home, ".config", "colima", profile, "incus.sock")),
		// Legacy path (Colima < 0.6.0)
		filepath.Clean(filepath.Join(home, ".colima", profile, "incus.sock")),
	}

	for _, socketPath := range possiblePaths {
		if _, err := os.Stat(socketPath); err == nil {
			return "unix://" + socketPath, nil
		}
	}

	return "", fmt.Errorf("incus socket not found (is Colima running?)\nTried locations:\n  %s\n  %s\nCheck: colima list",
		possiblePaths[0], possiblePaths[1])
}

// queryColimaSocket asks Colima where its socket is located.
func (c *ColimaBackend) queryColimaSocket(profile string) (string, error) {
	log := logging.Get()
	cmd := exec.Command("colima", "list", "--json")
	log.Cmd("colima", []string{"list", "--json"})

	output, err := cmd.Output()
	if err != nil {
		log.CmdOutput("colima", output, err)
		return "", err
	}
	log.CmdOutput("colima", output, nil)

	// Parse JSON output (Colima outputs one JSON object per line)
	decoder := json.NewDecoder(bytes.NewReader(output))
	for decoder.More() {
		var inst struct {
			Name   string `json:"name"`
			Status string `json:"status"`
			Socket string `json:"socket"`
		}
		if err := decoder.Decode(&inst); err != nil {
			continue
		}
		if inst.Name == profile {
			if inst.Socket != "" {
				return inst.Socket, nil
			}
			// Socket field might not be populated in older Colima versions
			break
		}
	}

	return "", fmt.Errorf("socket path not found in colima output")
}
