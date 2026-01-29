// Package vm provides an abstraction layer for VM backends (Colima, Lima).
package vm

import (
	"fmt"
	"os/exec"
	"runtime"

	"github.com/bsmi021/coop/internal/config"
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
	if runtime.GOOS == "darwin" {
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

	return nil, fmt.Errorf("no VM backend available (tried: colima, lima)")
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
		return fmt.Errorf("VM is not running and auto_start is disabled")
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
