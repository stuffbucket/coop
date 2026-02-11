package backend

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/stuffbucket/coop/internal/config"
	"github.com/stuffbucket/coop/internal/logging"
	"github.com/stuffbucket/coop/internal/names"
)

// LimaBackend implements Backend using Lima directly.
type LimaBackend struct {
	cfg *config.Config
}

// NewLimaBackend creates a new Lima backend.
func NewLimaBackend(cfg *config.Config) *LimaBackend {
	return &LimaBackend{cfg: cfg}
}

func (l *LimaBackend) Name() string {
	return "lima"
}

func (l *LimaBackend) Available() bool {
	return commandExists("limactl")
}

func (l *LimaBackend) instanceName() string {
	name := l.cfg.Settings.VM.Instance
	if name == "" {
		name = "incus"
	}
	// Validate to prevent path injection (defense in depth)
	if err := names.ValidateInstanceName(name); err != nil {
		// Fall back to safe default if config is invalid
		return "incus"
	}
	return name
}

// limaInstance represents limactl list JSON output
type limaInstance struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Arch   string `json:"arch"`
	CPUs   int    `json:"cpus"`
	Memory int64  `json:"memory"` // bytes
	Disk   int64  `json:"disk"`   // bytes
	Dir    string `json:"dir"`
}

func (l *LimaBackend) Status() (*Status, error) {
	log := logging.Get()
	name := l.instanceName()

	cmd := exec.Command("limactl", "list", "--json")
	log.Cmd("limactl", []string{"list", "--json"})

	output, err := cmd.Output()
	if err != nil {
		log.CmdOutput("limactl", output, err)
		return &Status{Name: name, State: StateMissing}, nil
	}
	log.CmdOutput("limactl", output, nil)

	// Lima outputs newline-delimited JSON
	decoder := json.NewDecoder(bytes.NewReader(output))
	for decoder.More() {
		var inst limaInstance
		if err := decoder.Decode(&inst); err != nil {
			continue
		}
		if inst.Name == name {
			state := StateStopped
			if inst.Status == "Running" {
				state = StateRunning
			}

			return &Status{
				Name:     inst.Name,
				State:    state,
				CPUs:     inst.CPUs,
				MemoryGB: int(inst.Memory / (1024 * 1024 * 1024)),
				DiskGB:   int(inst.Disk / (1024 * 1024 * 1024)),
				Arch:     inst.Arch,
				Runtime:  "incus",
			}, nil
		}
	}

	return &Status{Name: name, State: StateMissing}, nil
}

func (l *LimaBackend) Start() error {
	name := l.instanceName()
	vm := l.cfg.Settings.VM

	status, err := l.Status()
	if err != nil {
		return err
	}

	if status.State == StateRunning {
		return nil
	}

	var args []string
	if status.State == StateMissing {
		// Create new instance
		args = []string{"start", "--name=" + name, "--tty=false"}

		if vm.CPUs > 0 {
			args = append(args, fmt.Sprintf("--cpus=%d", vm.CPUs))
		}
		if vm.MemoryGB > 0 {
			args = append(args, fmt.Sprintf("--memory=%d", vm.MemoryGB))
		}
		if vm.DiskGB > 0 {
			args = append(args, fmt.Sprintf("--disk=%d", vm.DiskGB))
		}

		// Use template
		template := vm.Template
		if template == "" {
			template = l.findTemplate()
		}
		args = append(args, template)
	} else {
		// Start existing instance
		args = []string{"start", name}
	}

	return runStreamingCmd("limactl", args, fmt.Sprintf("failed to start lima VM %q", name))
}

func (l *LimaBackend) findTemplate() string {
	dirs := l.cfg.Dirs

	// Check data dir templates
	dataTemplate := filepath.Join(dirs.Data, "templates", "incus.yaml")
	if _, err := os.Stat(dataTemplate); err == nil {
		return dataTemplate
	}

	// Check config dir templates
	configTemplate := filepath.Join(dirs.Config, "templates", "incus.yaml")
	if _, err := os.Stat(configTemplate); err == nil {
		return configTemplate
	}

	// Check relative to executable
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		for _, rel := range []string{"templates/incus.yaml", "../templates/incus.yaml"} {
			t := filepath.Join(exeDir, rel)
			if _, err := os.Stat(t); err == nil {
				return t
			}
		}
	}

	// Fallback to debian (we'll need to install incus manually)
	return "debian"
}

func (l *LimaBackend) Stop() error {
	name := l.instanceName()
	return runStreamingCmd("limactl", []string{"stop", name}, fmt.Sprintf("failed to stop lima VM %q", name))
}

func (l *LimaBackend) Delete() error {
	name := l.instanceName()
	return runStreamingCmd("limactl", []string{"delete", "--force", name}, fmt.Sprintf("failed to delete lima VM %q", name))
}

func (l *LimaBackend) Shell() error {
	name := l.instanceName()
	return runInteractiveCmd("limactl", []string{"shell", name}, fmt.Sprintf("failed to open shell in lima VM %q", name))
}

func (l *LimaBackend) Exec(command []string) ([]byte, error) {
	name := l.instanceName()
	args := append([]string{"shell", name}, command...)
	return runOutputCmd("limactl", args, fmt.Sprintf("failed to exec in lima VM %q", name))
}

// GetTLSCerts is not needed for Lima (uses Unix socket).
func (l *LimaBackend) GetTLSCerts() (clientCert, clientKey, serverCert string, err error) {
	return "", "", "", nil
}

func (l *LimaBackend) SSHProxyArgs() []string {
	return nil
}

func (l *LimaBackend) GetIncusSocket() (string, error) {
	// Check explicit config first
	if l.cfg.Settings.IncusSocket != "" {
		return l.cfg.Settings.IncusSocket, nil
	}

	name := l.instanceName()
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	// Lima stores socket at ~/.lima/<instance>/sock/incus.sock
	socketPath := filepath.Join(home, ".lima", name, "sock", "incus.sock")

	if _, err := os.Stat(socketPath); err != nil {
		return "", fmt.Errorf("incus socket not found at %s (is Lima running?): %w", socketPath, err)
	}

	return "unix://" + socketPath, nil
}
