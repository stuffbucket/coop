package backend

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/stuffbucket/coop/internal/config"
	"github.com/stuffbucket/coop/internal/logging"
)

const (
	// bladerunnerSocket is the control socket filename within the state directory.
	bladerunnerSocket = "control.sock"

	// bladerunnerProtocolVersion is the wire protocol version we speak.
	bladerunnerProtocolVersion = 1

	// bladerunnerDialTimeout is the connection timeout for the control socket.
	bladerunnerDialTimeout = 2 * time.Second

	// bladerunnerCmdTimeout is the per-command read/write deadline.
	bladerunnerCmdTimeout = 5 * time.Second

	// bladerunnerDefaultAPIPort is the default Incus API port forwarded by bladerunner.
	bladerunnerDefaultAPIPort = "18443"
)

// BladerunnerBackend implements Backend using the bladerunner VM runner.
//
// Bladerunner runs a Linux VM via Apple Virtualization.framework and bootstraps
// Incus inside it. Communication with the bladerunner process happens over a
// Unix domain socket control protocol. The Incus API is forwarded to localhost
// via virtio-vsock.
type BladerunnerBackend struct {
	cfg *config.Config
}

// NewBladerunnerBackend creates a new bladerunner backend.
func NewBladerunnerBackend(cfg *config.Config) *BladerunnerBackend {
	return &BladerunnerBackend{cfg: cfg}
}

func (b *BladerunnerBackend) Name() string {
	return "bladerunner"
}

func (b *BladerunnerBackend) Available() bool {
	// Bladerunner is available if it's in PATH and the control socket exists
	if !commandExists("br") {
		return false
	}
	socketPath := b.socketPath()
	_, err := os.Stat(socketPath)
	return err == nil
}

func (b *BladerunnerBackend) Status() (*Status, error) {
	log := logging.Get()

	resp, err := b.controlCommand("status")
	if err != nil {
		log.Debug("bladerunner status check failed", "error", err)
		// If we can't connect, the VM isn't running
		if b.socketExists() {
			return &Status{Name: "bladerunner", State: StateStopped}, nil
		}
		return &Status{Name: "bladerunner", State: StateMissing}, nil
	}

	state := StateUnknown
	switch resp {
	case "running":
		state = StateRunning
	case "stopped":
		state = StateStopped
	}

	// Try to get resource info from config queries
	status := &Status{
		Name:    "bladerunner",
		State:   state,
		Runtime: "incus",
	}

	if cpus, err := b.controlCommand("config.get cpus"); err == nil {
		_, _ = fmt.Sscanf(cpus, "%d", &status.CPUs)
	}
	if mem, err := b.controlCommand("config.get memory-gib"); err == nil {
		_, _ = fmt.Sscanf(mem, "%d", &status.MemoryGB)
	}
	if disk, err := b.controlCommand("config.get disk-size-gib"); err == nil {
		_, _ = fmt.Sscanf(disk, "%d", &status.DiskGB)
	}
	if arch, err := b.controlCommand("config.get arch"); err == nil {
		status.Arch = arch
	}

	return status, nil
}

func (b *BladerunnerBackend) Start() error {
	// Check if already running
	status, err := b.Status()
	if err == nil && status.State == StateRunning {
		return nil
	}

	return runStreamingCmd("br", []string{"start"}, "failed to start bladerunner VM")
}

func (b *BladerunnerBackend) Stop() error {
	// Try graceful stop via control socket first
	if _, err := b.controlCommand("stop"); err == nil {
		return nil
	}

	// Fall back to CLI
	return runStreamingCmd("br", []string{"stop"}, "failed to stop bladerunner VM")
}

func (b *BladerunnerBackend) Delete() error {
	return fmt.Errorf("delete is not supported for bladerunner backend")
}

func (b *BladerunnerBackend) Shell() error {
	return runInteractiveCmd("br", []string{"shell"}, "failed to open shell in bladerunner VM")
}

func (b *BladerunnerBackend) Exec(command []string) ([]byte, error) {
	args := append([]string{"shell", "--"}, command...)
	return runOutputCmd("br", args, "failed to exec in bladerunner VM")
}

func (b *BladerunnerBackend) GetIncusSocket() (string, error) {
	// Check explicit config first
	if b.cfg.Settings.IncusSocket != "" {
		return b.cfg.Settings.IncusSocket, nil
	}

	// Query bladerunner for the API port
	port := bladerunnerDefaultAPIPort
	if p, err := b.controlCommand("config.get local-api-port"); err == nil && p != "" {
		port = p
	}

	// Bladerunner forwards Incus API to localhost via virtio-vsock
	return "https://127.0.0.1:" + port, nil
}

// GetTLSCerts returns the paths to client cert and key for Incus TLS auth.
// Bladerunner stores generated certs in its state directory.
func (b *BladerunnerBackend) GetTLSCerts() (clientCert, clientKey, serverCert string, err error) {
	stateDir := b.stateDir()

	clientCert = filepath.Join(stateDir, "client.crt")
	clientKey = filepath.Join(stateDir, "client.key")

	// Validate the certs exist
	if _, err := os.Stat(clientCert); err != nil {
		return "", "", "", fmt.Errorf("bladerunner client cert not found: %s (has bladerunner been started?)", clientCert)
	}
	if _, err := os.Stat(clientKey); err != nil {
		return "", "", "", fmt.Errorf("bladerunner client key not found: %s", clientKey)
	}

	// Bladerunner uses self-signed certs; no separate server cert needed
	return clientCert, clientKey, "", nil
}

// SSHProxyArgs returns SSH arguments to tunnel through the bladerunner VM.
// Container IPs are only routable from inside the VM, so we ProxyJump via
// the VM's SSH endpoint (forwarded to localhost via vsock).
func (b *BladerunnerBackend) SSHProxyArgs() []string {
	// Query bladerunner for the SSH config file path (written at VM start)
	configPath, err := b.controlCommand("config.get ssh-config-path")
	if err == nil && configPath != "" {
		return []string{"-F", configPath, "-J", "bladerunner"}
	}

	// Fallback: build ProxyCommand from individual config values
	log := logging.Get()
	log.Debug("ssh-config-path not available, building proxy args from config values")

	port := "6022"
	if p, err := b.controlCommand("config.get local-ssh-port"); err == nil && p != "" {
		port = p
	}
	user := "incus"
	if u, err := b.controlCommand("config.get ssh-user"); err == nil && u != "" {
		user = u
	}
	keyPath, err := b.controlCommand("config.get ssh-private-key-path")
	if err != nil || keyPath == "" {
		log.Debug("cannot determine ssh-private-key-path, proxy SSH may fail")
		return nil
	}

	proxyCmd := fmt.Sprintf(
		"ssh -i %s -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR -p %s %s@127.0.0.1 -W %%h:%%p",
		keyPath, port, user,
	)
	return []string{"-o", "ProxyCommand=" + proxyCmd}
}

// controlCommand sends a command to the bladerunner control socket and returns the response.
func (b *BladerunnerBackend) controlCommand(command string) (string, error) {
	log := logging.Get()
	socketPath := b.socketPath()

	log.Debug("bladerunner control", "command", command, "socket", socketPath)

	conn, err := net.DialTimeout("unix", socketPath, bladerunnerDialTimeout)
	if err != nil {
		return "", fmt.Errorf("failed to connect to bladerunner control socket: %w", err)
	}
	defer func() { _ = conn.Close() }()

	// Set deadline for the entire exchange
	if err := conn.SetDeadline(time.Now().Add(bladerunnerCmdTimeout)); err != nil {
		return "", fmt.Errorf("failed to set deadline: %w", err)
	}

	// Send command in v1 line format
	msg := fmt.Sprintf("v%d %s\n", bladerunnerProtocolVersion, command)
	if _, err := conn.Write([]byte(msg)); err != nil {
		return "", fmt.Errorf("failed to send command: %w", err)
	}

	// Read response
	scanner := bufio.NewScanner(conn)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return "", fmt.Errorf("failed to read response: %w", err)
		}
		return "", fmt.Errorf("empty response from control socket")
	}

	resp := scanner.Text()
	log.Debug("bladerunner response", "response", resp)

	return b.parseResponse(resp)
}

// parseResponse parses a v1 line-protocol response.
// Format: "v1 <response>" or "v1 error: <message>"
func (b *BladerunnerBackend) parseResponse(line string) (string, error) {
	// Strip version prefix
	trimmed := line
	if strings.HasPrefix(line, "v") {
		parts := strings.SplitN(line, " ", 2)
		if len(parts) == 2 {
			trimmed = parts[1]
		}
	}

	// Check for error response
	if strings.HasPrefix(trimmed, "error: ") {
		return "", fmt.Errorf("bladerunner: %s", strings.TrimPrefix(trimmed, "error: "))
	}

	return trimmed, nil
}

// stateDir returns the bladerunner state directory.
func (b *BladerunnerBackend) stateDir() string {
	// Check XDG_STATE_HOME first
	if xdg := os.Getenv("XDG_STATE_HOME"); xdg != "" {
		return filepath.Join(xdg, "bladerunner")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "state", "bladerunner")
}

// socketPath returns the full path to the control socket.
func (b *BladerunnerBackend) socketPath() string {
	return filepath.Join(b.stateDir(), bladerunnerSocket)
}

// socketExists checks if the control socket file exists.
func (b *BladerunnerBackend) socketExists() bool {
	_, err := os.Stat(b.socketPath())
	return err == nil
}

// EnsureIncusRemote configures the incus CLI with a "bladerunner" remote
// pointing to the Incus HTTPS endpoint. This copies bladerunner's TLS client
// certs into the incus config dir and adds/updates the remote.
func (b *BladerunnerBackend) EnsureIncusRemote() error {
	log := logging.Get()

	// Get the API endpoint
	socket, err := b.GetIncusSocket()
	if err != nil {
		return fmt.Errorf("failed to get incus socket: %w", err)
	}

	// Determine incus config dir
	incusConfigDir := os.Getenv("INCUS_CONF")
	if incusConfigDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get home dir: %w", err)
		}
		incusConfigDir = filepath.Join(home, ".config", "incus")
	}

	// Copy bladerunner's client certs into the incus config dir.
	// The incus CLI uses a single global client cert for TLS remotes.
	// Bladerunner's cert is already trusted by its Incus server.
	stateDir := b.stateDir()
	srcCert := filepath.Join(stateDir, "client.crt")
	srcKey := filepath.Join(stateDir, "client.key")
	dstCert := filepath.Join(incusConfigDir, "client.crt")
	dstKey := filepath.Join(incusConfigDir, "client.key")

	if err := os.MkdirAll(incusConfigDir, 0700); err != nil {
		return fmt.Errorf("failed to create incus config dir: %w", err)
	}

	if err := copyFile(srcCert, dstCert); err != nil {
		return fmt.Errorf("failed to copy client cert: %w", err)
	}
	if err := copyFile(srcKey, dstKey); err != nil {
		return fmt.Errorf("failed to copy client key: %w", err)
	}
	log.Debug("copied bladerunner TLS certs to incus config", "dir", incusConfigDir)

	// Check if the remote already exists
	out, err := exec.Command("incus", "remote", "list", "--format=csv").Output()
	if err != nil {
		return fmt.Errorf("failed to list incus remotes: %w", err)
	}

	hasRemote := false
	for _, line := range strings.Split(string(out), "\n") {
		// CSV format: "bladerunner,https://..." or "bladerunner (current),https://..."
		if strings.HasPrefix(line, "bladerunner,") || strings.HasPrefix(line, "bladerunner ") {
			hasRemote = true
			break
		}
	}

	if !hasRemote {
		// Add the remote
		log.Debug("adding incus remote", "name", "bladerunner", "address", socket)
		cmd := exec.Command("incus", "remote", "add", "bladerunner", socket,
			"--accept-certificate", "--protocol=incus", "--auth-type=tls")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to add incus remote: %w", err)
		}
	}

	// Switch default remote to bladerunner
	if err := exec.Command("incus", "remote", "switch", "bladerunner").Run(); err != nil {
		return fmt.Errorf("failed to switch incus remote: %w", err)
	}

	log.Debug("incus remote configured", "default", "bladerunner", "address", socket)
	return nil
}

// copyFile copies src to dst, preserving permissions.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	_, err = io.Copy(out, in)
	return err
}
