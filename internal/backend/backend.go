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
	"github.com/stuffbucket/coop/internal/ui"
)

// Sentinel errors for VM operations.
var (
	// ErrVMNotRunning indicates the VM is not in a running state.
	ErrVMNotRunning = errors.New("platform for agent containers is not running")

	// ErrNoBackendAvailable indicates no VM backend could be found or used.
	ErrNoBackendAvailable = errors.New("no platform backend available (install bladerunner, colima, or lima)")

	// ErrAutoStartDisabled indicates the VM is stopped and auto_start is disabled.
	ErrAutoStartDisabled = errors.New("platform for agent containers is not running - start it with: coop vm start")
)

// UserCancelError represents a user-initiated cancellation.
// This is detected to show clean messages without error formatting.
type UserCancelError struct {
	Message string
}

func (e *UserCancelError) Error() string {
	return e.Message
}

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

	// GetIncusSocket returns the Incus socket path or HTTPS URL.
	GetIncusSocket() (string, error)

	// GetTLSCerts returns paths to client cert, client key, and server cert
	// for Incus TLS auth. Backends that use Unix sockets return empty strings.
	GetTLSCerts() (clientCert, clientKey, serverCert string, err error)

	// SSHProxyArgs returns extra SSH arguments needed to reach container IPs
	// from the host. Backends where container IPs are directly routable return nil.
	// For backends like bladerunner where container IPs are inside a VM,
	// this returns ProxyJump/config args to tunnel through the VM.
	SSHProxyArgs() []string
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
	// Remote backend is always registered first (if configured, it takes priority)
	m.backends = append(m.backends, NewRemoteBackend(cfg))

	if platform.IsMacOS() {
		// Bladerunner uses Apple Virtualization.framework (macOS only)
		m.backends = append(m.backends, NewBladerunnerBackend(cfg))
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

	return nil, fmt.Errorf("%w (tried: remote, bladerunner, colima, lima)", ErrNoBackendAvailable)
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

// GetIncusSocket returns the Incus socket path or HTTPS URL.
func (m *Manager) GetIncusSocket() (string, error) {
	return m.backend.GetIncusSocket()
}

// GetTLSCerts returns paths to TLS client cert, key, and server cert.
func (m *Manager) GetTLSCerts() (clientCert, clientKey, serverCert string, err error) {
	return m.backend.GetTLSCerts()
}

// SSHProxyArgs returns extra SSH arguments for reaching container IPs.
func (m *Manager) SSHProxyArgs() []string {
	return m.backend.SSHProxyArgs()
}

// EnsureRunning ensures the VM is running.
// If interactive is true, prompts user before starting.
func (m *Manager) EnsureRunning() error {
	return m.EnsureRunningWithPrompt(false)
}

// EnsureRunningWithPrompt ensures the VM is running.
// If interactive is true and VM needs to be started, prompts user first.
func (m *Manager) EnsureRunningWithPrompt(interactive bool) error {
	status, err := m.Status()
	if err != nil {
		return err
	}

	if status.State == StateRunning {
		m.ensureIncusRemote()
		return nil
	}

	if !m.cfg.Settings.VM.AutoStart {
		return &UserCancelError{
			Message: fmt.Sprintf("Platform for agent containers is not running.\n\nStart it with: %s\nOr enable auto_start in ~/.config/coop/settings.json", ui.Code("coop vm start")),
		}
	}

	// If interactive, prompt before starting
	if interactive {
		// Check if we can actually show the dialog
		if !ui.IsInteractive() {
			// Not in a terminal - return user-friendly cancel
			return &UserCancelError{
				Message: fmt.Sprintf("Platform for agent containers is not running. Start it with: %s", ui.Code("coop vm start")),
			}
		}

		confirmed := ui.InfoDialog{
			Title:       "Start Platform",
			Description: "Coop needs a Linux environment running to manage your agent containers.",
			Details: []string{
				fmt.Sprintf("Spins up %s in the background", m.backend.Name()),
				"First-time setup downloads ~100 MB (one-time only)",
				"Ready in 30-60 seconds on a fast connection",
			},
			Options: []string{
				"Start now and continue with your command",
				fmt.Sprintf("Skip for now (start later with: %s)", ui.Code("coop vm start")),
			},
			Recommended: 0, // First option is recommended
			Question:    "Start the platform?",
			Affirmative: "Start",
			Negative:    "Skip",
		}.Show()

		if !confirmed {
			return &UserCancelError{
				Message: fmt.Sprintf("Skipped starting platform. Run %s when ready.", ui.Code("coop vm start")),
			}
		}
	}

	if err := m.Start(); err != nil {
		return err
	}
	m.ensureIncusRemote()
	return nil
}

// ensureIncusRemote configures the incus CLI remote if the backend supports it.
func (m *Manager) ensureIncusRemote() {
	if br, ok := m.backend.(*BladerunnerBackend); ok {
		if err := br.EnsureIncusRemote(); err != nil {
			log := logging.Get()
			log.Debug("failed to configure incus remote", "error", err)
		}
	}
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
