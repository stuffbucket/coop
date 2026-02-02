// Package sandbox manages AI agent sandboxed containers.
package sandbox

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/bsmi021/coop/internal/cloudinit"
	"github.com/bsmi021/coop/internal/config"
	"github.com/bsmi021/coop/internal/incus"
	"github.com/bsmi021/coop/internal/names"
	"github.com/bsmi021/coop/internal/sshkeys"
	"github.com/bsmi021/coop/internal/ui"
	securejoin "github.com/cyphar/filepath-securejoin"
)

const (
	// DefaultImage is the fallback image if config doesn't specify one.
	DefaultImage = "coop-agent-base"
	// FallbackImage is used if the local base image doesn't exist.
	FallbackImage = "ubuntu/22.04/cloud"
	// AgentProfile is the Incus profile for agent containers.
	AgentProfile = "agent-sandbox"

	// CloudInitTimeout is max time to wait for cloud-init to complete.
	CloudInitTimeout = 10 * time.Minute
	// CloudInitPollInterval is how often to check cloud-init status.
	CloudInitPollInterval = 3 * time.Second

	// DefaultProcessLimit protects against fork bombs in containers.
	DefaultProcessLimit = "500"
	// AgentUID is the UID for the agent user inside containers.
	AgentUID = 1000
)

// ErrContainerNotFound is returned when a container doesn't exist.
var ErrContainerNotFound = errors.New("container not found")

// containerNotFound returns a wrapped error for a missing container.
func containerNotFound(name string) error {
	return fmt.Errorf("%w: %s", ErrContainerNotFound, name)
}

// Manager handles container lifecycle operations.
type Manager struct {
	client *incus.Client
	config *config.Config
}

// NewManager creates a new sandbox manager.
// Deprecated: Use NewManagerWithConfig for explicit dependency injection.
func NewManager() (*Manager, error) {
	cfg, err := config.Load()
	if err != nil {
		// Use defaults if config doesn't exist
		defaultCfg := config.DefaultConfig()
		cfg = &defaultCfg
	}
	return NewManagerWithConfig(cfg)
}

// NewManagerWithConfig creates a new sandbox manager with the provided config.
// This is the preferred constructor for explicit dependency injection.
func NewManagerWithConfig(cfg *config.Config) (*Manager, error) {
	client, err := incus.ConnectWithConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to incus: %w", err)
	}

	return &Manager{
		client: client,
		config: cfg,
	}, nil
}

// GenerateName returns a whimsical container name
func GenerateName() string {
	return names.Generate()
}

// ContainerConfig holds configuration for creating a container.
type ContainerConfig struct {
	Name       string
	SSHPubKey  string
	CPUs       int
	MemoryMB   int
	DiskGB     int
	Profiles   []string
	WorkingDir string
	Verbose    bool // Stream cloud-init logs during setup
}

// DefaultContainerConfig returns sensible defaults from config.
func DefaultContainerConfig(name string) ContainerConfig {
	cfg := config.DefaultConfig()
	return ContainerConfig{
		Name:     name,
		CPUs:     cfg.Settings.DefaultCPUs,
		MemoryMB: cfg.Settings.DefaultMemoryMB,
		DiskGB:   cfg.Settings.DefaultDiskGB,
		Profiles: []string{"default", AgentProfile},
	}
}

// Create creates a new agent container.
func (m *Manager) Create(cfg ContainerConfig) error {
	containerName := cfg.Name

	// Check if container already exists
	existing, err := m.client.GetContainer(containerName)
	if err == nil && existing != nil {
		return fmt.Errorf("container %s already exists", containerName)
	}

	// Generate cloud-init user-data
	cloudCfg := cloudinit.DefaultConfig()
	cloudCfg.Hostname = containerName
	cloudCfg.SSHPubKey = cfg.SSHPubKey

	userData, err := cloudinit.Generate(cloudCfg)
	if err != nil {
		return fmt.Errorf("failed to generate cloud-init config: %w", err)
	}

	// Ensure the agent profile exists
	if err := m.ensureAgentProfile(cfg); err != nil {
		return fmt.Errorf("failed to ensure agent profile: %w", err)
	}

	// Container config with cloud-init and resource limits
	// NOTE: Environment variables can be passed via "environment.VAR_NAME" keys
	// e.g., "environment.ANTHROPIC_API_KEY": cfg.AnthropicKey
	containerConfig := map[string]string{
		"user.user-data":   userData,
		"limits.cpu":       fmt.Sprintf("%d", cfg.CPUs),
		"limits.memory":    fmt.Sprintf("%dMiB", cfg.MemoryMB),
		"limits.processes": DefaultProcessLimit,
		"raw.idmap":        fmt.Sprintf("both %d %d", os.Getuid(), AgentUID),
	}

	// Resolve image: prefer base image, fall back to remote if missing
	image := m.config.Settings.DefaultImage
	if image == "" {
		image = DefaultImage
	}
	if !strings.Contains(image, "/") && !m.client.ImageExists(image) {
		image = m.handleMissingImage(image)
	}
	fmt.Printf("Creating container %s from %s...\n", containerName, image)
	if err := m.client.CreateContainer(containerName, image, containerConfig, cfg.Profiles); err != nil {
		return fmt.Errorf("failed to create container: %w", err)
	}

	// Start the container
	fmt.Printf("Starting container %s...\n", containerName)
	if err := m.client.StartContainer(containerName); err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	// Wait for cloud-init to complete
	fmt.Println("Waiting for cloud-init to complete...")
	if err := m.waitForCloudInit(containerName, cfg.Verbose); err != nil {
		return fmt.Errorf("cloud-init failed: %w", err)
	}

	// Get container IP
	ip, err := m.client.GetContainerIP(containerName)
	if err != nil {
		fmt.Printf("Warning: could not get container IP: %v\n", err)
	} else {
		fmt.Printf("Container %s is ready at %s\n", containerName, ip)
	}

	return nil
}

// handleMissingImage prompts user to build the base image or falls back gracefully.
// In non-interactive mode, it just warns and returns the fallback image.
func (m *Manager) handleMissingImage(requestedImage string) string {
	// Non-interactive: just warn and fallback
	if !ui.IsInteractive() {
		ui.Warn(fmt.Sprintf("Base image %q not found", requestedImage))
		ui.Muted("  Build it with: coop image build")
		ui.Muted(fmt.Sprintf("  Using fallback %s (cloud-init will take ~10 minutes)", FallbackImage))
		return FallbackImage
	}

	// Interactive: offer to build
	fmt.Println()
	fmt.Println(ui.WarningBox("Base Image Missing",
		fmt.Sprintf("The image %q is not available.\n\n", requestedImage)+
			"Without it, container setup takes ~10 minutes instead of ~30 seconds.\n"+
			"Building the base image is a one-time operation."))
	fmt.Println()

	choices := []string{
		"Build the base image now (~10 min)",
		"Continue with slow fallback",
		"Cancel",
	}

	choice := ui.Select("How would you like to proceed?", choices)

	switch choice {
	case choices[0]: // Build now
		fmt.Println()
		if err := BuildBaseImage(); err != nil {
			ui.Errorf("Build failed: %v", err)
			ui.Warn("Falling back to remote image")
			return FallbackImage
		}
		// Verify the image now exists
		if m.client.ImageExists(requestedImage) {
			ui.Success("Base image built successfully")
			return requestedImage
		}
		ui.Warn("Build completed but image not found, using fallback")
		return FallbackImage

	case choices[1]: // Continue with fallback
		ui.Muted("Using fallback image (this will be slower)")
		return FallbackImage

	default: // Cancel or empty (Ctrl+C)
		ui.Info("Cancelled")
		os.Exit(0)
		return "" // unreachable
	}
}

// BuildBaseImage runs the build script to create the coop-agent-base image.
// Returns an error if the script fails or cannot be found.
func BuildBaseImage() error {
	scriptLocations := []string{
		"./scripts/build-base-image.sh",
		"scripts/build-base-image.sh",
	}

	var scriptPath string
	for _, loc := range scriptLocations {
		if _, err := os.Stat(loc); err == nil {
			scriptPath = loc
			break
		}
	}

	if scriptPath == "" {
		return fmt.Errorf("build-base-image.sh not found (run from coop source directory)")
	}

	ui.Info("Building coop-agent-base image...")
	ui.Muted("This takes ~10 minutes on first run")
	fmt.Println()

	cmd := exec.Command("bash", scriptPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	return cmd.Run()
}

func (m *Manager) ensureAgentProfile(cfg ContainerConfig) error {
	profileConfig := map[string]string{
		"security.nesting": "true",
	}

	devices := map[string]map[string]string{
		"root": {
			"type": "disk",
			"pool": "default",
			"path": "/",
			"size": fmt.Sprintf("%dGiB", cfg.DiskGB),
		},
	}

	// Add workspace mount if specified
	if cfg.WorkingDir != "" {
		devices["workspace"] = map[string]string{
			"type":   "disk",
			"source": cfg.WorkingDir,
			"path":   "/home/agent/workspace",
		}
	}

	return m.client.EnsureProfile(AgentProfile, profileConfig, devices)
}

func (m *Manager) waitForCloudInit(name string, verbose bool) error {
	timeout := time.After(CloudInitTimeout)
	ticker := time.NewTicker(CloudInitPollInterval)
	defer ticker.Stop()

	lastStatus := ""
	lastLogLine := 0

	for {
		select {
		case <-timeout:
			return fmt.Errorf("timeout waiting for cloud-init (last status: %s)", lastStatus)
		case <-ticker.C:
			// Check cloud-init status without --wait to see progress
			status, _ := m.getCloudInitStatus(name)
			if status != "" && status != lastStatus {
				fmt.Printf("  cloud-init: %s\n", status)
				lastStatus = status
			}

			// Stream logs if verbose
			if verbose {
				lastLogLine = m.streamCloudInitLogs(name, lastLogLine)
			}

			ciStatus := CloudInitState(status)
			if ciStatus.IsDone() {
				if verbose {
					// Final log flush
					m.streamCloudInitLogs(name, lastLogLine)
				}
				return nil
			}
			if ciStatus.IsFailed() {
				if verbose {
					m.streamCloudInitLogs(name, lastLogLine)
				}
				return fmt.Errorf("cloud-init failed with status: %s", status)
			}
		}
	}
}

// streamCloudInitLogs tails the cloud-init output log and prints new lines
func (m *Manager) streamCloudInitLogs(name string, fromLine int) int {
	// Use tail with line numbers to get new content
	// cloud-init-output.log contains the actual script output
	cmd := fmt.Sprintf("tail -n +%d /var/log/cloud-init-output.log 2>/dev/null | head -100", fromLine+1)

	// We need to capture output - use exec with output capture
	output, err := m.client.ExecCommandWithOutput(name, []string{"sh", "-c", cmd})
	if err != nil || output == "" {
		return fromLine
	}

	lines := splitLines(output)
	for _, line := range lines {
		if line != "" {
			fmt.Printf("    %s\n", line)
		}
	}

	return fromLine + len(lines)
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

// getCloudInitStatus returns the current cloud-init status without blocking
func (m *Manager) getCloudInitStatus(name string) (string, error) {
	// Check if cloud-init result file exists (indicates completion)
	output, err := m.client.ExecCommandWithOutput(name, []string{
		"sh", "-c", "cat /run/cloud-init/result.json 2>/dev/null && echo DONE || echo NOTDONE",
	})
	if err == nil && strings.Contains(output, "DONE") && !strings.Contains(output, "NOTDONE") {
		return "done", nil
	}

	// Check cloud-init status command
	output, err = m.client.ExecCommandWithOutput(name, []string{
		"sh", "-c", "cloud-init status 2>/dev/null || echo pending",
	})
	if err != nil {
		return "pending", err
	}

	output = strings.TrimSpace(output)
	if strings.Contains(output, "done") {
		return "done", nil
	}
	if strings.Contains(output, "running") {
		return "running", nil
	}
	if strings.Contains(output, "error") {
		return "error", nil
	}
	if strings.Contains(output, "disabled") {
		return "disabled", nil
	}

	return "pending", nil
}

// Start starts a stopped container.
func (m *Manager) Start(name string) error {
	container, err := m.client.GetContainer(name)
	if err != nil {
		return containerNotFound(name)
	}

	if ContainerState(container.Status) == StateRunning {
		return fmt.Errorf("container %s is already running", name)
	}

	if err := m.client.StartContainer(name); err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	return nil
}

// Stop stops a running container.
func (m *Manager) Stop(name string, force bool) error {
	container, err := m.client.GetContainer(name)
	if err != nil {
		return containerNotFound(name)
	}

	if ContainerState(container.Status) != StateRunning {
		return fmt.Errorf("container %s is not running (status: %s)", name, container.Status)
	}

	if err := m.client.StopContainer(name, force); err != nil {
		return fmt.Errorf("failed to stop container: %w", err)
	}

	return nil
}

// Lock freezes a running container, pausing all processes.
func (m *Manager) Lock(name string) error {
	container, err := m.client.GetContainer(name)
	if err != nil {
		return containerNotFound(name)
	}

	if ContainerState(container.Status) != StateRunning {
		return fmt.Errorf("container %s is not running (status: %s)", name, container.Status)
	}

	if err := m.client.FreezeContainer(name); err != nil {
		return fmt.Errorf("failed to lock container: %w", err)
	}

	return nil
}

// Unlock unfreezes a frozen container, resuming all processes.
func (m *Manager) Unlock(name string) error {
	container, err := m.client.GetContainer(name)
	if err != nil {
		return containerNotFound(name)
	}

	if ContainerState(container.Status) != StateFrozen {
		return fmt.Errorf("container %s is not locked (status: %s)", name, container.Status)
	}

	if err := m.client.UnfreezeContainer(name); err != nil {
		return fmt.Errorf("failed to unlock container: %w", err)
	}

	return nil
}

// Logs returns cloud-init and system logs from a container.
func (m *Manager) Logs(name string, follow bool, lines int) error {
	container, err := m.client.GetContainer(name)
	if err != nil {
		return containerNotFound(name)
	}

	if ContainerState(container.Status) != StateRunning {
		return fmt.Errorf("container %s is not running", name)
	}

	// Build journalctl command
	args := []string{"journalctl", "--no-pager"}
	if follow {
		args = append(args, "-f")
	}
	if lines > 0 {
		args = append(args, "-n", fmt.Sprintf("%d", lines))
	}

	_, err = m.client.ExecCommand(name, args)
	return err
}

// Delete removes an agent container.
func (m *Manager) Delete(name string, force bool) error {
	containerName := name

	// Check if container exists
	container, err := m.client.GetContainer(containerName)
	if err != nil {
		return containerNotFound(containerName)
	}

	// Stop if running
	if ContainerState(container.Status) == StateRunning {
		fmt.Printf("Stopping container %s...\n", containerName)
		if err := m.client.StopContainer(containerName, force); err != nil {
			return fmt.Errorf("failed to stop container: %w", err)
		}
	}

	// Delete the container
	fmt.Printf("Deleting container %s...\n", containerName)
	if err := m.client.DeleteContainer(containerName); err != nil {
		return fmt.Errorf("failed to delete container: %w", err)
	}

	fmt.Printf("Container %s deleted\n", containerName)
	return nil
}

// List returns all agent containers.
func (m *Manager) List() ([]ContainerInfo, error) {
	containers, err := m.client.ListContainers("")
	if err != nil {
		return nil, err
	}

	var infos []ContainerInfo
	for _, c := range containers {
		info := ContainerInfo{
			Name:      c.Name,
			Status:    c.Status,
			CreatedAt: c.CreatedAt,
			CPUs:      c.Config["limits.cpu"],
			Memory:    c.Config["limits.memory"],
		}

		// Get disk size from root device in expanded config
		if root, ok := c.ExpandedDevices["root"]; ok {
			info.Disk = root["size"]
		}

		// Try to get IP if running
		if ContainerState(c.Status) == StateRunning {
			if ip, err := m.client.GetContainerIP(c.Name); err == nil {
				info.IP = ip
			}
		}

		infos = append(infos, info)
	}

	return infos, nil
}

// ContainerInfo holds display information about a container.
type ContainerInfo struct {
	Name      string
	Status    string
	IP        string
	CPUs      string
	Memory    string
	Disk      string
	CreatedAt time.Time
}

// Status returns detailed status of a container.
func (m *Manager) Status(name string) (*ContainerStatus, error) {
	container, err := m.client.GetContainer(name)
	if err != nil {
		return nil, fmt.Errorf("container %s not found", name)
	}

	status := &ContainerStatus{
		Name:      name,
		Status:    container.Status,
		CreatedAt: container.CreatedAt,
		Config:    container.Config,
	}

	if ContainerState(container.Status) == StateRunning {
		if ip, err := m.client.GetContainerIP(name); err == nil {
			status.IP = ip
		}
	}

	return status, nil
}

// ContainerStatus holds detailed status information.
type ContainerStatus struct {
	Name      string
	Status    string
	IP        string
	CreatedAt time.Time
	Config    map[string]string
}

// SSH returns the SSH command string to connect to a container.
func (m *Manager) SSH(name string) (string, error) {
	container, err := m.client.GetContainer(name)
	if err != nil {
		return "", fmt.Errorf("container %s not found", name)
	}

	if ContainerState(container.Status) != StateRunning {
		return "", fmt.Errorf("container %s is not running", name)
	}

	ip, err := m.client.GetContainerIP(name)
	if err != nil {
		return "", fmt.Errorf("could not get container IP: %w", err)
	}

	return sshkeys.SSHCommand("agent", ip), nil
}

// SSHArgs returns the SSH arguments as a slice for exec.
func (m *Manager) SSHArgs(name string) ([]string, error) {
	container, err := m.client.GetContainer(name)
	if err != nil {
		return nil, fmt.Errorf("container %s not found", name)
	}

	if ContainerState(container.Status) != StateRunning {
		return nil, fmt.Errorf("container %s is not running", name)
	}

	ip, err := m.client.GetContainerIP(name)
	if err != nil {
		return nil, fmt.Errorf("could not get container IP: %w", err)
	}

	return sshkeys.SSHArgs("agent", ip), nil
}

// Exec runs a command in the container.
func (m *Manager) Exec(name string, command []string) (int, error) {
	container, err := m.client.GetContainer(name)
	if err != nil {
		return -1, fmt.Errorf("container %s not found", name)
	}

	if ContainerState(container.Status) != StateRunning {
		return -1, fmt.Errorf("container %s is not running", name)
	}

	return m.client.ExecCommand(name, command)
}

// EnsureSSHKeys ensures coop SSH keys exist and returns the public key.
// Keys are stored in ~/.config/coop/ssh/, isolated from ~/.ssh/
func EnsureSSHKeys() (string, error) {
	return sshkeys.EnsureKeys()
}

// UpdateSSHConfig adds/updates the SSH config entry for a container.
func UpdateSSHConfig(name, ip string) error {
	return sshkeys.WriteSSHConfig(name, ip)
}

// GetContainerIP returns the IP address of a running container.
func (m *Manager) GetContainerIP(name string) (string, error) {
	return m.client.GetContainerIP(name)
}

// ImageExists checks if a local image alias exists.
func (m *Manager) ImageExists(alias string) bool {
	return m.client.ImageExists(alias)
}

// CreateSnapshot creates a snapshot of a container.
func (m *Manager) CreateSnapshot(name, snapshotName string) error {
	container, err := m.client.GetContainer(name)
	if err != nil {
		return containerNotFound(name)
	}

	wasRunning := ContainerState(container.Status) == StateRunning
	if wasRunning {
		// Stop for consistent snapshot
		if err := m.client.StopContainer(name, false); err != nil {
			return fmt.Errorf("failed to stop container for snapshot: %w", err)
		}
	}

	if err := m.client.CreateSnapshot(name, snapshotName, false); err != nil {
		// Try to restart if we stopped it
		if wasRunning {
			_ = m.client.StartContainer(name)
		}
		return err
	}

	if wasRunning {
		if err := m.client.StartContainer(name); err != nil {
			return fmt.Errorf("snapshot created but failed to restart container: %w", err)
		}
	}

	return nil
}

// RestoreSnapshot restores a container to a snapshot.
func (m *Manager) RestoreSnapshot(name, snapshotName string) error {
	container, err := m.client.GetContainer(name)
	if err != nil {
		return containerNotFound(name)
	}

	wasRunning := ContainerState(container.Status) == StateRunning
	if wasRunning {
		if err := m.client.StopContainer(name, false); err != nil {
			return fmt.Errorf("failed to stop container for restore: %w", err)
		}
	}

	if err := m.client.RestoreSnapshot(name, snapshotName); err != nil {
		return err
	}

	if wasRunning {
		if err := m.client.StartContainer(name); err != nil {
			return fmt.Errorf("restored but failed to restart container: %w", err)
		}
	}

	return nil
}

// SnapshotInfo holds information about a snapshot.
type SnapshotInfo struct {
	Name      string
	CreatedAt time.Time
}

// ListSnapshots returns all snapshots for a container.
func (m *Manager) ListSnapshots(name string) ([]SnapshotInfo, error) {
	if _, err := m.client.GetContainer(name); err != nil {
		return nil, fmt.Errorf("container %s not found", name)
	}

	snapshots, err := m.client.ListSnapshots(name)
	if err != nil {
		return nil, err
	}

	var infos []SnapshotInfo
	for _, s := range snapshots {
		infos = append(infos, SnapshotInfo{
			Name:      s.Name,
			CreatedAt: s.CreatedAt,
		})
	}
	return infos, nil
}

// DeleteSnapshot deletes a snapshot.
func (m *Manager) DeleteSnapshot(name, snapshotName string) error {
	if _, err := m.client.GetContainer(name); err != nil {
		return containerNotFound(name)
	}
	return m.client.DeleteSnapshot(name, snapshotName)
}

// MountInfo holds information about a mount.
type MountInfo struct {
	Name     string
	Source   string
	Path     string
	Readonly bool
}

// sipProtectedPaths are macOS directories protected by System Integrity Protection.
// From Apple: https://support.apple.com/en-us/102149
var sipProtectedPaths = []string{
	"/System",
	"/usr",
	"/bin",
	"/sbin",
	"/var",
	"/private/var",
}

// sensitiveHomeDirs are user directories containing credentials, keys, or sensitive config.
var sensitiveHomeDirs = []string{
	"Library",                                // Keychains, app data, cookies
	"Library/Keychains",                      // macOS keychain files
	"Library/Cookies",                        // Browser cookies
	"Library/Application Support/MobileSync", // iOS backups
	".ssh",                                   // SSH keys
	".gnupg",                                 // GPG keys
	".aws",                                   // AWS credentials
	".azure",                                 // Azure credentials
	".config/gcloud",                         // GCP credentials
	".kube",                                  // Kubernetes config
	".docker",                                // Docker config and creds
	".npmrc",                                 // npm tokens (file)
	".netrc",                                 // Generic credential file
	".gitconfig",                             // May contain credentials
	".git-credentials",                       // Git credential storage
	".config/gh",                             // GitHub CLI tokens
	".anthropic",                             // Anthropic API keys
	".openai",                                // OpenAI API keys
	".config",                                // Parent of coop config
	".config/coop",                           // Coop trust root (hard block)
}

// expandPath expands ~ and ~user to absolute paths.
func expandPath(path string) string {
	if path == "" {
		return path
	}

	if path == "~" {
		return os.Getenv("HOME")
	}

	if strings.HasPrefix(path, "~/") {
		return os.Getenv("HOME") + path[1:]
	}

	// Handle ~user format (rare but possible)
	if strings.HasPrefix(path, "~") {
		// Find the end of username
		slash := strings.Index(path, "/")
		if slash == -1 {
			slash = len(path)
		}
		// For simplicity, just expand to current user's home
		// A more complete impl would look up the user
		return os.Getenv("HOME") + path[slash:]
	}

	return path
}

// IsSeatbelted returns true if a path is in a protected directory.
func IsSeatbelted(path string) (bool, string) {
	// Expand ~ and resolve to absolute path
	expanded := expandPath(path)

	// Resolve any symlinks and clean the path
	if resolved, err := filepath.EvalSymlinks(expanded); err == nil {
		expanded = resolved
	}
	expanded = filepath.Clean(expanded)

	// Check SIP-protected system directories
	for _, prefix := range sipProtectedPaths {
		if expanded == prefix || strings.HasPrefix(expanded, prefix+"/") {
			return true, fmt.Sprintf("%s is protected by System Integrity Protection", prefix)
		}
	}

	// Check user home directory sensitive paths
	home := os.Getenv("HOME")
	if home != "" {
		for _, dir := range sensitiveHomeDirs {
			protected := filepath.Join(home, dir)
			if pathContains(protected, expanded) || pathContains(expanded, protected) {
				// Hard block for coop trust root
				if strings.HasSuffix(protected, ".config/coop") {
					return true, fmt.Sprintf("~/%s is a protected Coop path and cannot be mounted", dir)
				}
				return true, fmt.Sprintf("~/%s contains sensitive data (credentials, keys, tokens)", dir)
			}
		}
	}

	return false, ""
}

// pathContains returns true if child is inside parent (after secure path resolution).
func pathContains(parent, child string) bool {
	parent = filepath.Clean(parent)
	child = filepath.Clean(child)

	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	// SecureJoin prevents traversal via symlinks inside child
	secured, err := securejoin.SecureJoin(parent, rel)
	if err != nil {
		return false
	}
	return secured == child
}

// Mount adds a host directory mount to a running container.
// Set force=true to mount seatbelted directories (requires explicit acknowledgment).
func (m *Manager) Mount(containerName, mountName, source, path string, readonly, force bool) error {
	if _, err := m.client.GetContainer(containerName); err != nil {
		return containerNotFound(containerName)
	}

	// Check for seatbelted directories
	if seatbelted, reason := IsSeatbelted(source); seatbelted && !force {
		return fmt.Errorf("refusing to mount protected path: %s. Use --force to override", reason)
	}

	device := map[string]string{
		"type":   "disk",
		"source": source,
		"path":   path,
	}

	if readonly {
		device["readonly"] = "true"
	}

	return m.client.AddDevice(containerName, mountName, device)
}

// Unmount removes a mount from a container.
func (m *Manager) Unmount(containerName, mountName string) error {
	if _, err := m.client.GetContainer(containerName); err != nil {
		return containerNotFound(containerName)
	}

	return m.client.RemoveDevice(containerName, mountName)
}

// ListMounts returns all disk mounts for a container.
func (m *Manager) ListMounts(containerName string) ([]MountInfo, error) {
	if _, err := m.client.GetContainer(containerName); err != nil {
		return nil, fmt.Errorf("container %s not found", containerName)
	}

	devices, err := m.client.ListDevices(containerName)
	if err != nil {
		return nil, err
	}

	var mounts []MountInfo
	for name, dev := range devices {
		if dev["type"] == "disk" && name != "root" {
			mounts = append(mounts, MountInfo{
				Name:     name,
				Source:   dev["source"],
				Path:     dev["path"],
				Readonly: dev["readonly"] == "true",
			})
		}
	}
	return mounts, nil
}

// ContainerMounts holds mounts for a single container along with its status.
type ContainerMounts struct {
	Name   string
	Status string
	Mounts []MountInfo
}

// ListAllMounts returns mounts for all containers.
func (m *Manager) ListAllMounts() ([]ContainerMounts, error) {
	containers, err := m.client.ListContainers("")
	if err != nil {
		return nil, err
	}

	var result []ContainerMounts
	for _, c := range containers {
		devices, err := m.client.ListDevices(c.Name)
		if err != nil {
			continue
		}

		var mounts []MountInfo
		for name, dev := range devices {
			if dev["type"] == "disk" && name != "root" {
				mounts = append(mounts, MountInfo{
					Name:     name,
					Source:   dev["source"],
					Path:     dev["path"],
					Readonly: dev["readonly"] == "true",
				})
			}
		}

		result = append(result, ContainerMounts{
			Name:   c.Name,
			Status: c.Status,
			Mounts: mounts,
		})
	}

	return result, nil
}
