// Package backend provides an abstraction layer for Incus backends (Colima, Lima, native, remote).
package backend

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/stuffbucket/coop/internal/config"
	"github.com/stuffbucket/coop/internal/logging"
	"github.com/stuffbucket/coop/internal/platform"
)

// Sentinel errors for VM operations.
var (
	// ErrVMNotRunning indicates the VM is not in a running state.
	ErrVMNotRunning = errors.New("VM is not running")

	// ErrNoBackendAvailable indicates no VM backend could be found or used.
	ErrNoBackendAvailable = errors.New("no VM backend available")

	// ErrAutoStartDisabled indicates the VM is stopped and auto_start is disabled.
	ErrAutoStartDisabled = errors.New("VM is not running and auto_start is disabled")
)

// Arch represents CPU architecture.
type Arch string

const (
	ArchHost    Arch = "host"
	ArchAArch64 Arch = "aarch64"
	ArchX86_64  Arch = "x86_64"
)

// VMType represents the virtualization type.
type VMType string

const (
	VMTypeVZ   VMType = "vz"   // Apple Virtualization.framework
	VMTypeQEMU VMType = "qemu" // QEMU emulation
)

// Backend represents a VM management backend.
type Backend interface {
	// Name returns the backend identifier.
	Name() string

	// Available checks if this backend is installed and usable.
	Available() bool

	// Status returns the current VM status.
	Status() (*Status, error)

	// Start starts or creates the VM.
	Start() error

	// Stop stops the VM.
	Stop() error

	// Delete removes the VM entirely.
	Delete() error

	// Shell opens an interactive shell in the VM.
	Shell() error

	// Exec runs a command in the VM and returns output.
	Exec(command []string) ([]byte, error)

	// GetIncusSocket returns the Incus socket path.
	GetIncusSocket() (string, error)
}

// Status holds VM status information.
type Status struct {
	Name     string
	State    State
	CPUs     int
	MemoryGB int
	DiskGB   int
	Arch     string
	Runtime  string // "incus", "docker", etc.
}

// State represents VM state.
type State string

const (
	StateRunning State = "Running"
	StateStopped State = "Stopped"
	StateMissing State = "Missing"
	StateUnknown State = "Unknown"
)

// Manager selects and manages VM backends.
type Manager struct {
	cfg      *config.Config
	backend  Backend
	backends []Backend
}

// NewManager creates a new VM manager with configured backend priority.
func NewManager(cfg *config.Config) (*Manager, error) {
	m := &Manager{cfg: cfg}

	// Register available backends based on platform
	if platform.IsMacOS() {
		m.backends = append(m.backends, NewColimaBackend(cfg))
	}
	// Lima works on macOS and Linux (including WSL2)
	m.backends = append(m.backends, NewLimaBackend(cfg))

	// Select backend based on priority
	backend, err := m.selectBackend()
	if err != nil {
		return nil, err
	}
	m.backend = backend

	return m, nil
}

func (m *Manager) selectBackend() (Backend, error) {
	priority := m.cfg.Settings.VM.BackendPriority

	// If priority is configured, try those first in order
	if len(priority) > 0 {
		for _, name := range priority {
			for _, b := range m.backends {
				if b.Name() == name && b.Available() {
					return b, nil
				}
			}
		}
	}

	// Fall back to first available
	for _, b := range m.backends {
		if b.Available() {
			return b, nil
		}
	}

	return nil, fmt.Errorf("%w (tried: colima, lima)", ErrNoBackendAvailable)
}

// Backend returns the selected backend.
func (m *Manager) Backend() Backend {
	return m.backend
}

// Status returns VM status.
func (m *Manager) Status() (*Status, error) {
	return m.backend.Status()
}

// Start starts the VM.
func (m *Manager) Start() error {
	return m.backend.Start()
}

// Stop stops the VM.
func (m *Manager) Stop() error {
	return m.backend.Stop()
}

// Delete removes the VM.
func (m *Manager) Delete() error {
	return m.backend.Delete()
}

// Shell opens a shell in the VM.
func (m *Manager) Shell() error {
	return m.backend.Shell()
}

// Exec runs a command in the VM.
func (m *Manager) Exec(command []string) ([]byte, error) {
	return m.backend.Exec(command)
}

// GetIncusSocket returns the Incus socket path.
func (m *Manager) GetIncusSocket() (string, error) {
	return m.backend.GetIncusSocket()
}

// EnsureRunning ensures the VM is running.
func (m *Manager) EnsureRunning() error {
	status, err := m.Status()
	if err != nil {
		return err
	}

	if status.State == StateRunning {
		return nil
	}

	if !m.cfg.Settings.VM.AutoStart {
		return ErrAutoStartDisabled
	}

	return m.Start()
}

// ListAvailableBackends returns names of available backends.
func (m *Manager) ListAvailableBackends() []string {
	var names []string
	for _, b := range m.backends {
		if b.Available() {
			names = append(names, b.Name())
		}
	}
	return names
}

// commandExists checks if a command is available in PATH.
func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// runStreamingCmd executes a command with stdout/stderr streamed to terminal and log.
// Returns a wrapped error with context on failure.
func runStreamingCmd(cmdName string, args []string, errContext string) error {
	log := logging.Get()
	log.Cmd(cmdName, args)

	cmd := exec.Command(cmdName, args...)
	cmd.Stdout = log.MultiWriter(os.Stdout)
	cmd.Stderr = log.MultiWriter(os.Stderr)

	if err := cmd.Run(); err != nil {
		log.CmdEnd(cmdName, err)
		return fmt.Errorf("%s: %w", errContext, err)
	}
	log.CmdEnd(cmdName, nil)
	return nil
}

// runStreamingCmdWithStdin executes a command with stdin provided and stdout/stderr streamed.
func runStreamingCmdWithStdin(cmdName string, args []string, stdin string, errContext string) error {
	log := logging.Get()
	log.Cmd(cmdName, args)

	cmd := exec.Command(cmdName, args...)
	cmd.Stdout = log.MultiWriter(os.Stdout)
	cmd.Stderr = log.MultiWriter(os.Stderr)
	cmd.Stdin = strings.NewReader(stdin)

	if err := cmd.Run(); err != nil {
		log.CmdEnd(cmdName, err)
		return fmt.Errorf("%s: %w", errContext, err)
	}
	log.CmdEnd(cmdName, nil)
	return nil
}

// runInteractiveCmd executes a command with full terminal I/O attached (for shells).
func runInteractiveCmd(cmdName string, args []string, errContext string) error {
	log := logging.Get()
	log.Cmd(cmdName, args)

	cmd := exec.Command(cmdName, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		log.CmdEnd(cmdName, err)
		return fmt.Errorf("%s: %w", errContext, err)
	}
	log.CmdEnd(cmdName, nil)
	return nil
}

// runOutputCmd executes a command and returns its output.
func runOutputCmd(cmdName string, args []string, errContext string) ([]byte, error) {
	log := logging.Get()
	log.Cmd(cmdName, args)

	cmd := exec.Command(cmdName, args...)
	output, err := cmd.Output()
	log.CmdOutput(cmdName, output, err)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", errContext, err)
	}
	return output, nil
}
