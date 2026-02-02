package lima

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/stuffbucket/coop/internal/config"
	"github.com/stuffbucket/coop/internal/logging"
)

// Status represents the state of a Lima instance.
type Status string

const (
	StatusRunning Status = "Running"
	StatusStopped Status = "Stopped"
	StatusUnknown Status = "Unknown"
	StatusMissing Status = "Missing"
)

// Instance holds information about a Lima VM instance.
type Instance struct {
	Name   string `json:"name"`
	Status Status `json:"status"`
	Arch   string `json:"arch"`
	CPUs   int    `json:"cpus"`
	Memory int64  `json:"memory"` // bytes
	Disk   int64  `json:"disk"`   // bytes
	Dir    string `json:"dir"`
}

// Manager handles Lima VM operations.
type Manager struct {
	cfg *config.Config
}

// NewManager creates a new Lima manager.
func NewManager(cfg *config.Config) *Manager {
	return &Manager{cfg: cfg}
}

// runCmd executes a command, logging it and streaming output to both stdout/stderr and the log.
func runCmd(name string, args ...string) error {
	log := logging.Get()
	log.Cmd(name, args)

	cmd := exec.Command(name, args...)
	cmd.Stdout = log.MultiWriter(os.Stdout)
	cmd.Stderr = log.MultiWriter(os.Stderr)

	err := cmd.Run()
	log.CmdEnd(name, err)
	return err
}

// runCmdOutput executes a command and returns the output, logging everything.
func runCmdOutput(name string, args ...string) ([]byte, error) {
	log := logging.Get()
	log.Cmd(name, args)

	cmd := exec.Command(name, args...)
	output, err := cmd.Output()
	log.CmdOutput(name, output, err)
	return output, err
}

// IsAvailable checks if limactl is installed and accessible.
func IsAvailable() bool {
	if runtime.GOOS != "darwin" {
		return false
	}
	_, err := exec.LookPath("limactl")
	return err == nil
}

// IsMacOS returns true if running on macOS.
func IsMacOS() bool {
	return runtime.GOOS == "darwin"
}

// GetInstance returns information about the configured Lima instance.
func (m *Manager) GetInstance() (*Instance, error) {
	name := m.cfg.Settings.VM.Instance
	output, err := runCmdOutput("limactl", "list", "--json")
	if err != nil {
		return nil, fmt.Errorf("failed to list lima instances: %w", err)
	}

	// limactl outputs newline-delimited JSON (one object per line)
	decoder := json.NewDecoder(bytes.NewReader(output))
	for decoder.More() {
		var inst Instance
		if err := decoder.Decode(&inst); err != nil {
			return nil, fmt.Errorf("failed to parse lima output: %w", err)
		}
		if inst.Name == name {
			return &inst, nil
		}
	}

	return &Instance{Name: name, Status: StatusMissing}, nil
}

// GetStatus returns the status of the configured Lima instance.
func (m *Manager) GetStatus() (Status, error) {
	inst, err := m.GetInstance()
	if err != nil {
		return StatusUnknown, err
	}
	return inst.Status, nil
}

// Start starts the Lima VM, creating it if necessary.
func (m *Manager) Start() error {
	inst, err := m.GetInstance()
	if err != nil {
		return err
	}

	if inst.Status == StatusRunning {
		return nil // Already running
	}

	if inst.Status == StatusMissing {
		// Create the instance
		return m.Create()
	}

	// Start existing stopped instance
	fmt.Printf("Starting Lima VM '%s'...\n", inst.Name)
	return runCmd("limactl", "start", inst.Name)
}

// Create creates a new Lima VM with the configured settings.
func (m *Manager) Create() error {
	vm := m.cfg.Settings.VM
	fmt.Printf("Creating Lima VM '%s' from %s...\n", vm.Instance, vm.Template)

	args := []string{"start", "--name=" + vm.Instance, "--tty=false"}

	// Add resource overrides if specified
	if vm.CPUs > 0 {
		args = append(args, fmt.Sprintf("--cpus=%d", vm.CPUs))
	}
	if vm.MemoryGB > 0 {
		args = append(args, fmt.Sprintf("--memory=%d", vm.MemoryGB))
	}
	if vm.DiskGB > 0 {
		args = append(args, fmt.Sprintf("--disk=%d", vm.DiskGB))
	}

	args = append(args, vm.Template)

	return runCmd("limactl", args...)
}

// Stop stops the Lima VM.
func (m *Manager) Stop() error {
	name := m.cfg.Settings.VM.Instance
	fmt.Printf("Stopping Lima VM '%s'...\n", name)
	return runCmd("limactl", "stop", name)
}

// Delete removes the Lima VM entirely.
func (m *Manager) Delete() error {
	name := m.cfg.Settings.VM.Instance
	fmt.Printf("Deleting Lima VM '%s'...\n", name)
	return runCmd("limactl", "delete", "--force", name)
}

// Shell opens an interactive shell in the Lima VM.
func (m *Manager) Shell() error {
	log := logging.Get()
	name := m.cfg.Settings.VM.Instance
	log.Cmd("limactl", []string{"shell", name})

	cmd := exec.Command("limactl", "shell", name)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	log.CmdEnd("limactl", err)
	return err
}

// Exec runs a command inside the Lima VM.
func (m *Manager) Exec(command []string) ([]byte, error) {
	name := m.cfg.Settings.VM.Instance
	args := append([]string{"shell", name}, command...)
	return runCmdOutput("limactl", args...)
}

// GetIncusSocket returns the path to the Incus socket inside the Lima VM.
func (m *Manager) GetIncusSocket() (string, error) {
	// If explicitly configured, use that
	if m.cfg.Settings.IncusSocket != "" {
		return m.cfg.Settings.IncusSocket, nil
	}

	// Default Lima socket path
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	name := m.cfg.Settings.VM.Instance
	socketPath := filepath.Join(home, ".lima", name, "sock", "incus.sock")

	// Verify socket exists
	if _, err := os.Stat(socketPath); err != nil {
		return "", fmt.Errorf("incus socket not found at %s (is Lima running?): %w", socketPath, err)
	}

	return "unix://" + socketPath, nil
}

// EnsureRunning ensures the Lima VM is running, starting it if needed.
func (m *Manager) EnsureRunning() error {
	if !m.cfg.Settings.VM.AutoStart {
		status, err := m.GetStatus()
		if err != nil {
			return err
		}
		if status != StatusRunning {
			return fmt.Errorf("lima VM '%s' is not running and auto_start is disabled", m.cfg.Settings.VM.Instance)
		}
		return nil
	}
	return m.Start()
}

// Info returns information about the Lima VM.
func (m *Manager) Info() (*Instance, error) {
	return m.GetInstance()
}
