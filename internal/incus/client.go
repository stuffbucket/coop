// Package incus provides a client wrapper for the Incus API.
package incus

import (
	"bytes"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	incus "github.com/lxc/incus/v6/client"
	"github.com/lxc/incus/v6/shared/api"
	"github.com/stuffbucket/coop/internal/config"
	"github.com/stuffbucket/coop/internal/platform"
	"github.com/stuffbucket/coop/internal/vm"
)

// WaitCondition represents a condition to wait for.
type WaitCondition int

const (
	// WaitStatusRunning waits for instance status to be Running.
	WaitStatusRunning WaitCondition = iota
	// WaitStatusStopped waits for instance status to be Stopped.
	WaitStatusStopped
	// WaitHasIPv4 waits for a globally routable IPv4 address.
	WaitHasIPv4
	// WaitHasIPv6 waits for a globally routable IPv6 address.
	WaitHasIPv6
	// WaitHasIP waits for any globally routable IP address.
	WaitHasIP
)

// DefaultWaitTimeout is the default timeout for wait operations.
const DefaultWaitTimeout = 2 * time.Minute

// DefaultWaitInterval is the default polling interval for wait operations.
const DefaultWaitInterval = time.Second

// Platform is an alias for platform.Type for backward compatibility.
// Deprecated: Use platform.Type directly.
type Platform = platform.Type

// Platform constants for backward compatibility.
// Deprecated: Use platform.Linux, platform.MacOS, etc. directly.
const (
	PlatformLinux   = platform.Linux
	PlatformMacOS   = platform.MacOS
	PlatformWSL2    = platform.WSL2
	PlatformUnknown = platform.Unknown
)

// DetectPlatform determines the current platform.
// Deprecated: Use platform.Detect() directly.
func DetectPlatform() Platform {
	return platform.Detect()
}

// Client wraps the Incus API client with convenience methods.
type Client struct {
	conn     incus.InstanceServer
	platform Platform
	cfg      *config.Config
}

// Connect establishes a connection to the Incus daemon.
// Deprecated: Use ConnectWithConfig for explicit dependency injection.
func Connect() (*Client, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	return ConnectWithConfig(cfg)
}

// ConnectWithConfig establishes a connection using the provided config.
func ConnectWithConfig(cfg *config.Config) (*Client, error) {
	platform := DetectPlatform()
	if platform == PlatformUnknown {
		return nil, fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}

	var socketPath string

	// Determine socket path based on platform and config
	if cfg.Settings.IncusSocket != "" {
		// Explicit socket path from config
		socketPath = cfg.Settings.IncusSocket
	} else {
		switch platform {
		case PlatformMacOS:
			// Use VM manager to get socket and ensure VM is running
			vmMgr, err := vm.NewManager(cfg)
			if err != nil {
				return nil, fmt.Errorf("vm setup failed: %w", err)
			}
			if err := vmMgr.EnsureRunning(); err != nil {
				return nil, fmt.Errorf("vm start failed: %w", err)
			}
			socket, err := vmMgr.GetIncusSocket()
			if err != nil {
				return nil, fmt.Errorf("failed to get incus socket: %w", err)
			}
			socketPath = socket
		case PlatformLinux, PlatformWSL2:
			socketPath = "unix:///var/lib/incus/unix.socket"
		}
	}

	// ConnectIncusUnix expects raw path without unix:// prefix
	socketPath = strings.TrimPrefix(socketPath, "unix://")

	conn, err := incus.ConnectIncusUnix(socketPath, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to incus at %s: %w", socketPath, err)
	}

	return &Client{
		conn:     conn,
		platform: platform,
		cfg:      cfg,
	}, nil
}

// CreateContainer creates a new container with the given configuration.
// Image can be:
//   - "coop-agent-base" (local alias - no slash)
//   - "ubuntu/22.04/cloud" (remote from linuxcontainers.org - has slash)
func (c *Client) CreateContainer(name, image string, config map[string]string, profiles []string) error {
	var source api.InstanceSource

	// If image contains a slash, it's a remote image path
	// Otherwise treat as a local alias
	if strings.Contains(image, "/") {
		// Remote image from linuxcontainers.org
		source = api.InstanceSource{
			Type:     "image",
			Alias:    image,
			Server:   "https://images.linuxcontainers.org",
			Protocol: "simplestreams",
		}
	} else {
		// Local image alias
		source = api.InstanceSource{
			Type:  "image",
			Alias: image,
		}
	}

	req := api.InstancesPost{
		Name:   name,
		Source: source,
		Type:   api.InstanceTypeContainer,
		InstancePut: api.InstancePut{
			Config:   config,
			Profiles: profiles,
		},
	}

	op, err := c.conn.CreateInstance(req)
	if err != nil {
		return fmt.Errorf("failed to create container: %w", err)
	}

	if err := op.Wait(); err != nil {
		return fmt.Errorf("container creation failed: %w", err)
	}

	return nil
}

// StartContainer starts the specified container.
func (c *Client) StartContainer(name string) error {
	req := api.InstanceStatePut{
		Action:  "start",
		Timeout: -1,
	}

	op, err := c.conn.UpdateInstanceState(name, req, "")
	if err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	return op.Wait()
}

// StopContainer stops the specified container.
func (c *Client) StopContainer(name string, force bool) error {
	req := api.InstanceStatePut{
		Action:  "stop",
		Timeout: -1,
		Force:   force,
	}

	op, err := c.conn.UpdateInstanceState(name, req, "")
	if err != nil {
		return fmt.Errorf("failed to stop container: %w", err)
	}

	return op.Wait()
}

// FreezeContainer freezes (pauses) a running container.
func (c *Client) FreezeContainer(name string) error {
	req := api.InstanceStatePut{
		Action:  "freeze",
		Timeout: -1,
	}

	op, err := c.conn.UpdateInstanceState(name, req, "")
	if err != nil {
		return fmt.Errorf("failed to freeze container: %w", err)
	}

	return op.Wait()
}

// UnfreezeContainer unfreezes (resumes) a frozen container.
func (c *Client) UnfreezeContainer(name string) error {
	req := api.InstanceStatePut{
		Action:  "unfreeze",
		Timeout: -1,
	}

	op, err := c.conn.UpdateInstanceState(name, req, "")
	if err != nil {
		return fmt.Errorf("failed to unfreeze container: %w", err)
	}

	return op.Wait()
}

// DeleteContainer deletes the specified container.
func (c *Client) DeleteContainer(name string) error {
	op, err := c.conn.DeleteInstance(name)
	if err != nil {
		return fmt.Errorf("failed to delete container: %w", err)
	}

	return op.Wait()
}

// GetContainer returns information about a container.
func (c *Client) GetContainer(name string) (*api.Instance, error) {
	instance, _, err := c.conn.GetInstance(name)
	if err != nil {
		return nil, err
	}
	return instance, nil
}

// ListContainers returns all containers matching the prefix.
func (c *Client) ListContainers(prefix string) ([]api.Instance, error) {
	instances, err := c.conn.GetInstances(api.InstanceTypeContainer)
	if err != nil {
		return nil, err
	}

	if prefix == "" {
		return instances, nil
	}

	var filtered []api.Instance
	for _, inst := range instances {
		if strings.HasPrefix(inst.Name, prefix) {
			filtered = append(filtered, inst)
		}
	}
	return filtered, nil
}

// ExecCommand executes a command inside the container.
func (c *Client) ExecCommand(name string, command []string) (int, error) {
	req := api.InstanceExecPost{
		Command:     command,
		WaitForWS:   true,
		Interactive: false,
	}

	args := incus.InstanceExecArgs{
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}

	op, err := c.conn.ExecInstance(name, req, &args)
	if err != nil {
		return -1, err
	}

	if err := op.Wait(); err != nil {
		return -1, err
	}

	opAPI := op.Get()
	return int(opAPI.Metadata["return"].(float64)), nil
}

// ExecCommandWithOutput executes a command and returns stdout as a string.
func (c *Client) ExecCommandWithOutput(name string, command []string) (string, error) {
	req := api.InstanceExecPost{
		Command:      command,
		WaitForWS:    true,
		Interactive:  false,
		RecordOutput: false, // Use websocket streaming instead
	}

	var stdout, stderr bytes.Buffer
	args := incus.InstanceExecArgs{
		Stdout: &stdout,
		Stderr: &stderr,
	}

	op, err := c.conn.ExecInstance(name, req, &args)
	if err != nil {
		return "", err
	}

	if err := op.Wait(); err != nil {
		return "", err
	}

	return stdout.String(), nil
}

// GetContainerIP returns the IPv4 address of the container.
func (c *Client) GetContainerIP(name string) (string, error) {
	state, _, err := c.conn.GetInstanceState(name)
	if err != nil {
		return "", err
	}

	for _, network := range state.Network {
		for _, addr := range network.Addresses {
			if addr.Family == "inet" && addr.Scope == "global" {
				return addr.Address, nil
			}
		}
	}

	return "", fmt.Errorf("no IPv4 address found for container %s", name)
}

// GetInstanceState returns the current state of an instance.
func (c *Client) GetInstanceState(name string) (*api.InstanceState, error) {
	state, _, err := c.conn.GetInstanceState(name)
	return state, err
}

// WaitForCondition waits until the specified condition is met.
// Returns an error if the timeout is reached or the condition cannot be satisfied.
func (c *Client) WaitForCondition(name string, condition WaitCondition, timeout, interval time.Duration) error {
	if timeout == 0 {
		timeout = DefaultWaitTimeout
	}
	if interval == 0 {
		interval = DefaultWaitInterval
	}

	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	conditionName := conditionString(condition)

	for {
		met, err := c.checkCondition(name, condition)
		if err != nil {
			return fmt.Errorf("failed to check condition %s: %w", conditionName, err)
		}
		if met {
			return nil
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for %s on %s", conditionName, name)
		}

		<-ticker.C
	}
}

// checkCondition checks if a single condition is currently met.
func (c *Client) checkCondition(name string, condition WaitCondition) (bool, error) {
	state, _, err := c.conn.GetInstanceState(name)
	if err != nil {
		return false, err
	}

	switch condition {
	case WaitStatusRunning:
		return state.Status == "Running", nil
	case WaitStatusStopped:
		return state.Status == "Stopped", nil
	case WaitHasIPv4:
		return hasGlobalIP(state, "inet"), nil
	case WaitHasIPv6:
		return hasGlobalIP(state, "inet6"), nil
	case WaitHasIP:
		return hasGlobalIP(state, "inet") || hasGlobalIP(state, "inet6"), nil
	default:
		return false, fmt.Errorf("unknown condition: %d", condition)
	}
}

// hasGlobalIP checks if the instance has a globally routable IP of the given family.
func hasGlobalIP(state *api.InstanceState, family string) bool {
	for _, network := range state.Network {
		for _, addr := range network.Addresses {
			if addr.Family == family && addr.Scope == "global" {
				return true
			}
		}
	}
	return false
}

func conditionString(c WaitCondition) string {
	switch c {
	case WaitStatusRunning:
		return "status=Running"
	case WaitStatusStopped:
		return "status=Stopped"
	case WaitHasIPv4:
		return "ipv4"
	case WaitHasIPv6:
		return "ipv6"
	case WaitHasIP:
		return "ip"
	default:
		return fmt.Sprintf("unknown(%d)", c)
	}
}

// StorageInfo contains storage pool capacity information.
type StorageInfo struct {
	Available uint64 // Available bytes
	Total     uint64 // Total bytes
}

// GetStorageInfo returns storage capacity for the default pool.
func (c *Client) GetStorageInfo() (*StorageInfo, error) {
	resources, err := c.conn.GetStoragePoolResources("default")
	if err != nil {
		return nil, err
	}
	return &StorageInfo{
		Available: resources.Space.Total - resources.Space.Used,
		Total:     resources.Space.Total,
	}, nil
}

// ImageExists checks if a local image alias exists.
func (c *Client) ImageExists(alias string) bool {
	_, _, err := c.conn.GetImageAlias(alias)
	return err == nil
}

// EnsureProfile creates or updates an Incus profile.
func (c *Client) EnsureProfile(name string, config map[string]string, devices map[string]map[string]string) error {
	profile := api.ProfilesPost{
		Name: name,
		ProfilePut: api.ProfilePut{
			Config:  config,
			Devices: devices,
		},
	}

	err := c.conn.CreateProfile(profile)
	if err != nil {
		// Profile might already exist, try updating
		existingProfile, _, getErr := c.conn.GetProfile(name)
		if getErr != nil {
			return fmt.Errorf("failed to create or get profile: %w (original error: %v)", getErr, err)
		}

		existingProfile.Config = config
		existingProfile.Devices = devices

		if updateErr := c.conn.UpdateProfile(name, existingProfile.ProfilePut, ""); updateErr != nil {
			return fmt.Errorf("failed to update profile: %w", updateErr)
		}
	}

	return nil
}

// Platform returns the detected platform.
func (c *Client) Platform() Platform {
	return c.platform
}

// CreateSnapshot creates a snapshot of a container.
func (c *Client) CreateSnapshot(containerName, snapshotName string, stateful bool) error {
	req := api.InstanceSnapshotsPost{
		Name:     snapshotName,
		Stateful: stateful,
	}

	op, err := c.conn.CreateInstanceSnapshot(containerName, req)
	if err != nil {
		return fmt.Errorf("failed to create snapshot: %w", err)
	}

	return op.Wait()
}

// RestoreSnapshot restores a container to a snapshot.
func (c *Client) RestoreSnapshot(containerName, snapshotName string) error {
	req := api.InstancePut{
		Restore: snapshotName,
	}

	op, err := c.conn.UpdateInstance(containerName, req, "")
	if err != nil {
		return fmt.Errorf("failed to restore snapshot: %w", err)
	}

	return op.Wait()
}

// ListSnapshots returns all snapshots for a container.
func (c *Client) ListSnapshots(containerName string) ([]api.InstanceSnapshot, error) {
	snapshots, err := c.conn.GetInstanceSnapshots(containerName)
	if err != nil {
		return nil, fmt.Errorf("failed to list snapshots: %w", err)
	}
	return snapshots, nil
}

// DeleteSnapshot deletes a snapshot.
func (c *Client) DeleteSnapshot(containerName, snapshotName string) error {
	op, err := c.conn.DeleteInstanceSnapshot(containerName, snapshotName)
	if err != nil {
		return fmt.Errorf("failed to delete snapshot: %w", err)
	}
	return op.Wait()
}

// AddDevice adds a device to a container.
func (c *Client) AddDevice(containerName, deviceName string, device map[string]string) error {
	instance, etag, err := c.conn.GetInstance(containerName)
	if err != nil {
		return fmt.Errorf("failed to get container: %w", err)
	}

	if instance.Devices == nil {
		instance.Devices = make(map[string]map[string]string)
	}
	instance.Devices[deviceName] = device

	op, err := c.conn.UpdateInstance(containerName, instance.Writable(), etag)
	if err != nil {
		return fmt.Errorf("failed to add device: %w", err)
	}
	return op.Wait()
}

// RemoveDevice removes a device from a container.
func (c *Client) RemoveDevice(containerName, deviceName string) error {
	instance, etag, err := c.conn.GetInstance(containerName)
	if err != nil {
		return fmt.Errorf("failed to get container: %w", err)
	}

	if _, exists := instance.Devices[deviceName]; !exists {
		return fmt.Errorf("device %s not found", deviceName)
	}
	delete(instance.Devices, deviceName)

	op, err := c.conn.UpdateInstance(containerName, instance.Writable(), etag)
	if err != nil {
		return fmt.Errorf("failed to remove device: %w", err)
	}
	return op.Wait()
}

// ListDevices returns all devices attached to a container.
func (c *Client) ListDevices(containerName string) (map[string]map[string]string, error) {
	instance, _, err := c.conn.GetInstance(containerName)
	if err != nil {
		return nil, fmt.Errorf("failed to get container: %w", err)
	}
	return instance.ExpandedDevices, nil
}

// PublishSnapshot publishes a container snapshot as a new image.
// Returns the image fingerprint.
func (c *Client) PublishSnapshot(containerName, snapshotName, alias string) (string, error) {
	// Create image from snapshot with coop metadata
	req := api.ImagesPost{
		Source: &api.ImagesPostSource{
			Type: "snapshot",
			Name: fmt.Sprintf("%s/%s", containerName, snapshotName),
		},
		ImagePut: api.ImagePut{
			Properties: map[string]string{
				"user.coop":        "true",
				"user.coop.source": fmt.Sprintf("%s/%s", containerName, snapshotName),
			},
		},
	}

	op, err := c.conn.CreateImage(req, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create image from snapshot: %w", err)
	}

	if err := op.Wait(); err != nil {
		return "", fmt.Errorf("image creation failed: %w", err)
	}

	// Get the fingerprint from the operation
	opAPI := op.Get()
	fingerprint, ok := opAPI.Metadata["fingerprint"].(string)
	if !ok {
		return "", fmt.Errorf("failed to get image fingerprint from operation")
	}

	// Add alias to the image
	aliasReq := api.ImageAliasesPost{
		ImageAliasesEntry: api.ImageAliasesEntry{
			ImageAliasesEntryPut: api.ImageAliasesEntryPut{
				Target:      fingerprint,
				Description: fmt.Sprintf("Published from %s/%s", containerName, snapshotName),
			},
			Name: alias,
		},
	}

	if err := c.conn.CreateImageAlias(aliasReq); err != nil {
		// Image was created but alias failed - try to clean up
		return fingerprint, fmt.Errorf("image created (fingerprint: %s) but failed to create alias: %w", fingerprint, err)
	}

	return fingerprint, nil
}

