// Package config handles coop configuration with XDG-compliant paths.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Directories holds all coop directory paths.
type Directories struct {
	// Config is the base config directory (~/.config/coop or $COOP_CONFIG_DIR)
	Config string
	// Data is the base data directory (~/.local/share/coop or $COOP_DATA_DIR)
	Data string
	// Cache is the cache directory (~/.cache/coop or $COOP_CACHE_DIR)
	Cache string

	// Derived paths
	SSH          string // Config/ssh - SSH keys and config
	SettingsFile string // Config/settings.json
	Images       string // Data/images - VM/container images
	Disks        string // Data/disks - Container disk files
	Profiles     string // Data/profiles - Incus profile configs
	Logs         string // Data/logs - Container logs
}

// GetDirectories returns all coop directories, respecting env overrides.
func GetDirectories() Directories {
	config := getConfigBase()
	data := getDataBase()
	cache := getCacheBase()

	return Directories{
		Config:       config,
		Data:         data,
		Cache:        cache,
		SSH:          filepath.Join(config, "ssh"),
		SettingsFile: filepath.Join(config, "settings.json"),
		Images:       filepath.Join(data, "images"),
		Disks:        filepath.Join(data, "disks"),
		Profiles:     filepath.Join(data, "profiles"),
		Logs:         filepath.Join(data, "logs"),
	}
}

func getConfigBase() string {
	if dir := os.Getenv("COOP_CONFIG_DIR"); dir != "" {
		return dir
	}
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "coop")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "coop")
}

func getDataBase() string {
	if dir := os.Getenv("COOP_DATA_DIR"); dir != "" {
		return dir
	}
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "coop")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "coop")
}

func getCacheBase() string {
	if dir := os.Getenv("COOP_CACHE_DIR"); dir != "" {
		return dir
	}
	if xdg := os.Getenv("XDG_CACHE_HOME"); xdg != "" {
		return filepath.Join(xdg, "coop")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cache", "coop")
}

// Settings represents the settings.json file contents.
type Settings struct {
	// Container defaults
	DefaultCPUs     int    `json:"default_cpus,omitempty"`
	DefaultMemoryMB int    `json:"default_memory_mb,omitempty"`
	DefaultDiskGB   int    `json:"default_disk_gb,omitempty"`
	DefaultImage    string `json:"default_image,omitempty"`
	FallbackImage   string `json:"fallback_image,omitempty"`
	// FallbackFingerprint must match the local alias used for fallback_image.
	FallbackFingerprint string `json:"fallback_fingerprint,omitempty"`

	// Incus connection (auto-detected if empty)
	IncusSocket string `json:"incus_socket,omitempty"`

	// Remote Incus server configuration
	Remote RemoteSettings `json:"remote,omitempty"`

	// VM settings (Colima/Lima backend)
	VM VMSettings `json:"vm,omitempty"`

	// Network/discovery settings
	Network NetworkSettings `json:"network,omitempty"`

	// Logging settings
	Log LogSettings `json:"log,omitempty"`

	// Deprecated: Use VM settings instead. Kept for backward compatibility.
	Lima LimaSettings `json:"lima,omitempty"`
}

// RemoteSettings configures a remote Incus server connection.
type RemoteSettings struct {
	// Name is the remote server name (used for cert lookup in ~/.config/incus/servercerts/)
	// If set, enables remote backend
	Name string `json:"name,omitempty"`

	// Address is the server address (e.g., "192.168.1.100:8443" or "incus.example.com:8443")
	// If empty, derived from incus config.yml
	Address string `json:"address,omitempty"`

	// IncusConfigDir overrides the default ~/.config/incus for cert lookup
	// Allows using LXD certs (~/.config/lxc) or custom paths
	IncusConfigDir string `json:"incus_config_dir,omitempty"`

	// Explicit cert paths (override auto-discovery from incus config)
	ClientCert string `json:"client_cert,omitempty"` // Path to client.crt
	ClientKey  string `json:"client_key,omitempty"`  // Path to client.key
	ServerCert string `json:"server_cert,omitempty"` // Path to server.crt
}

// VMSettings configures the VM backend for running Incus.
type VMSettings struct {
	// BackendPriority is the order of backends to try (default: ["colima", "lima"])
	// On macOS: colima is preferred. On Windows/WSL2: lima only.
	BackendPriority []string `json:"backend_priority,omitempty"`

	// Instance name (default: "incus")
	Instance string `json:"instance,omitempty"`

	// Template for Lima backend (ignored by Colima)
	Template string `json:"template,omitempty"`

	// Arch specifies the VM architecture: "aarch64", "x86_64", or "host" (match host arch)
	// Default: "host"
	Arch string `json:"arch,omitempty"`

	// VMType specifies the virtualization type: "vz" (macOS Virtualization.framework) or "qemu"
	// VZ is faster and more integrated on Apple Silicon, QEMU is more portable.
	// VZ requires macOS and only supports native arch (aarch64 on Apple Silicon).
	// Default: "vz" on macOS with Apple Silicon, "qemu" otherwise
	VMType string `json:"vm_type,omitempty"`

	// Rosetta enables x86_64 emulation on Apple Silicon via Rosetta 2
	// Requires: VMType="vz", Arch="aarch64" or "host" on Apple Silicon
	// Default: false
	Rosetta bool `json:"rosetta,omitempty"`

	// NestedVirtualization enables running VMs inside the VM (requires M3+ and VMType="vz")
	// Default: false
	NestedVirtualization bool `json:"nested_virtualization,omitempty"`

	// DNS servers for the VM (e.g., ["8.8.8.8", "1.1.1.1"])
	// Default: use host DNS
	DNS []string `json:"dns,omitempty"`

	// VM resources
	CPUs     int `json:"cpus,omitempty"`
	MemoryGB int `json:"memory_gb,omitempty"`
	DiskGB   int `json:"disk_gb,omitempty"`

	// StorageAutoRecover automatically recovers existing storage pools when VM is recreated
	// Default: true
	StorageAutoRecover *bool `json:"storage_auto_recover,omitempty"`

	// Auto-start VM when needed
	AutoStart bool `json:"auto_start,omitempty"`
}

// NetworkSettings configures container networking and discovery.
type NetworkSettings struct {
	// ManageHosts enables automatic /etc/hosts management for container discovery
	// When true, coop maintains a section in /etc/hosts with container name→IP mappings
	// Default: false (requires sudo for /etc/hosts modification)
	ManageHosts bool `json:"manage_hosts,omitempty"`

	// HostsDomain is the domain suffix for container hostnames in /etc/hosts
	// Containers will be accessible as <name>.<domain> (e.g., testagent.incus.local)
	// Default: "incus.local"
	HostsDomain string `json:"hosts_domain,omitempty"`

	// HostsFile path to manage (default: /etc/hosts)
	// Can be changed for testing or custom setups
	HostsFile string `json:"hosts_file,omitempty"`
}

// LimaSettings is deprecated. Use VMSettings instead.
type LimaSettings struct {
	Instance  string `json:"instance,omitempty"`
	Template  string `json:"template,omitempty"`
	CPUs      int    `json:"cpus,omitempty"`
	MemoryGB  int    `json:"memory_gb,omitempty"`
	DiskGB    int    `json:"disk_gb,omitempty"`
	AutoStart bool   `json:"auto_start,omitempty"`
}

// LogSettings configures logging behavior.
type LogSettings struct {
	MaxSizeMB  int  `json:"max_size_mb,omitempty"`  // Max log file size in MB (default: 10)
	MaxBackups int  `json:"max_backups,omitempty"`  // Max rotated files to keep (default: 3)
	MaxAgeDays int  `json:"max_age_days,omitempty"` // Max days to keep old logs (default: 28)
	Compress   bool `json:"compress,omitempty"`     // Compress rotated logs (default: true)
	Debug      bool `json:"debug,omitempty"`        // Enable debug logging
}

// Config holds runtime configuration (settings + directories).
type Config struct {
	Dirs     Directories
	Settings Settings
}

// Load loads configuration from settings.json and environment.
func Load() (*Config, error) {
	dirs := GetDirectories()

	cfg := &Config{
		Dirs: dirs,
		Settings: Settings{
			DefaultCPUs:         2,
			DefaultMemoryMB:     4096,
			DefaultDiskGB:       20,
			DefaultImage:        "coop-agent-base",
			FallbackImage:       "",
			FallbackFingerprint: "",
			VM: VMSettings{
				BackendPriority: []string{"colima", "lima"},
				Instance:        "incus",
				CPUs:            4,
				MemoryGB:        8,
				DiskGB:          100,
				AutoStart:       true,
			},
			Log: LogSettings{
				MaxSizeMB:  10,
				MaxBackups: 3,
				MaxAgeDays: 28,
				Compress:   true,
				Debug:      false,
			},
		},
	}

	// Load settings.json if it exists
	if data, err := os.ReadFile(dirs.SettingsFile); err == nil {
		if err := json.Unmarshal(data, &cfg.Settings); err != nil {
			return nil, err
		}
	}

	// Migrate deprecated Lima settings to VM settings
	if cfg.Settings.Lima.Instance != "" && cfg.Settings.VM.Instance == "" {
		cfg.Settings.VM.Instance = cfg.Settings.Lima.Instance
	}
	if cfg.Settings.Lima.Template != "" && cfg.Settings.VM.Template == "" {
		cfg.Settings.VM.Template = cfg.Settings.Lima.Template
	}
	if cfg.Settings.Lima.CPUs > 0 && cfg.Settings.VM.CPUs == 0 {
		cfg.Settings.VM.CPUs = cfg.Settings.Lima.CPUs
	}
	if cfg.Settings.Lima.MemoryGB > 0 && cfg.Settings.VM.MemoryGB == 0 {
		cfg.Settings.VM.MemoryGB = cfg.Settings.Lima.MemoryGB
	}
	if cfg.Settings.Lima.DiskGB > 0 && cfg.Settings.VM.DiskGB == 0 {
		cfg.Settings.VM.DiskGB = cfg.Settings.Lima.DiskGB
	}
	if cfg.Settings.Lima.AutoStart && !cfg.Settings.VM.AutoStart {
		cfg.Settings.VM.AutoStart = cfg.Settings.Lima.AutoStart
	}

	// Apply defaults for VM settings
	if cfg.Settings.VM.Instance == "" {
		cfg.Settings.VM.Instance = "incus"
	}
	if len(cfg.Settings.VM.BackendPriority) == 0 {
		cfg.Settings.VM.BackendPriority = []string{"colima", "lima"}
	}
	if cfg.Settings.VM.CPUs == 0 {
		cfg.Settings.VM.CPUs = 4
	}
	if cfg.Settings.VM.MemoryGB == 0 {
		cfg.Settings.VM.MemoryGB = 8
	}
	if cfg.Settings.VM.DiskGB == 0 {
		cfg.Settings.VM.DiskGB = 100
	}
	// StorageAutoRecover defaults to true (nil pointer means not explicitly set)
	if cfg.Settings.VM.StorageAutoRecover == nil {
		defaultTrue := true
		cfg.Settings.VM.StorageAutoRecover = &defaultTrue
	}

	// Apply defaults for Network settings
	if cfg.Settings.Network.HostsDomain == "" {
		cfg.Settings.Network.HostsDomain = "incus.local"
	}
	if cfg.Settings.Network.HostsFile == "" {
		cfg.Settings.Network.HostsFile = "/etc/hosts"
	}

	// Apply defaults for empty Log settings after load
	if cfg.Settings.Log.MaxSizeMB == 0 {
		cfg.Settings.Log.MaxSizeMB = 10
	}
	if cfg.Settings.Log.MaxBackups == 0 {
		cfg.Settings.Log.MaxBackups = 3
	}
	if cfg.Settings.Log.MaxAgeDays == 0 {
		cfg.Settings.Log.MaxAgeDays = 28
	}
	// Note: Compress and Debug default to false, which is the zero value

	// Environment overrides take precedence
	cfg.applyEnvOverrides()

	return cfg, nil
}

func (c *Config) applyEnvOverrides() {
	if v := os.Getenv("COOP_DEFAULT_IMAGE"); v != "" {
		c.Settings.DefaultImage = v
	}
	if v := os.Getenv("COOP_FALLBACK_IMAGE"); v != "" {
		c.Settings.FallbackImage = v
	}
	if v := os.Getenv("COOP_FALLBACK_FINGERPRINT"); v != "" {
		c.Settings.FallbackFingerprint = v
	}
	if v := os.Getenv("COOP_INCUS_SOCKET"); v != "" {
		c.Settings.IncusSocket = v
	}
	if v := os.Getenv("COOP_VM_INSTANCE"); v != "" {
		c.Settings.VM.Instance = v
	}
	if v := os.Getenv("COOP_VM_BACKEND"); v != "" {
		// Override backend priority with single backend
		c.Settings.VM.BackendPriority = []string{v}
	}
	// Backward compat: COOP_LIMA_INSTANCE
	if v := os.Getenv("COOP_LIMA_INSTANCE"); v != "" {
		c.Settings.VM.Instance = v
	}
}

// EnsureDirectories creates all coop directories with appropriate permissions.
func EnsureDirectories() error {
	dirs := GetDirectories()

	// Config dir and SSH subdir (0700 for security)
	if err := os.MkdirAll(dirs.SSH, 0700); err != nil {
		return err
	}

	// Ensure settings.json exists
	if _, err := os.Stat(dirs.SettingsFile); os.IsNotExist(err) {
		if err := os.WriteFile(dirs.SettingsFile, []byte("{}\n"), 0600); err != nil {
			return err
		}
	}

	// Data dirs (0755 is fine)
	dataDirs := []string{dirs.Images, dirs.Disks, dirs.Profiles, dirs.Logs}
	for _, d := range dataDirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return err
		}
	}

	// Cache dir
	if err := os.MkdirAll(dirs.Cache, 0755); err != nil {
		return err
	}

	return nil
}

// Save writes current settings to settings.json.
func (c *Config) Save() error {
	data, err := json.MarshalIndent(c.Settings, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(c.Dirs.SettingsFile, append(data, '\n'), 0600)
}

// DefaultConfig returns configuration with sensible defaults (legacy helper).
func DefaultConfig() Config {
	cfg, _ := Load()
	if cfg == nil {
		return Config{
			Dirs: GetDirectories(),
			Settings: Settings{
				DefaultCPUs:     2,
				DefaultMemoryMB: 4096,
				DefaultDiskGB:   20,
				DefaultImage:    "coop-agent-base",
			},
		}
	}
	return *cfg
}
