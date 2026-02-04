package backend

import (
	"fmt"
	"os"
	"path/filepath"

	securejoin "github.com/cyphar/filepath-securejoin"
	"gopkg.in/yaml.v2"

	"github.com/stuffbucket/coop/internal/config"
	"github.com/stuffbucket/coop/internal/names"
)

// RemoteBackend implements Backend for remote Incus servers over HTTPS.
type RemoteBackend struct {
	cfg *config.Config
}

// NewRemoteBackend creates a new remote backend.
func NewRemoteBackend(cfg *config.Config) *RemoteBackend {
	return &RemoteBackend{cfg: cfg}
}

func (r *RemoteBackend) Name() string {
	return "remote"
}

func (r *RemoteBackend) Available() bool {
	// Available if remote is configured and certs exist
	remote := r.cfg.Settings.Remote
	if remote.Name == "" && remote.Address == "" {
		return false
	}

	// Check if we can resolve certs
	_, _, _, err := r.resolveCerts()
	return err == nil
}

func (r *RemoteBackend) Status() (*Status, error) {
	if !r.Available() {
		return &Status{Name: "remote", State: StateMissing}, nil
	}

	// Remote is always "running" from our perspective - it's external
	return &Status{
		Name:    r.cfg.Settings.Remote.Name,
		State:   StateRunning,
		Runtime: "incus",
	}, nil
}

// Start is a no-op for remote - we don't manage the remote server lifecycle.
func (r *RemoteBackend) Start() error {
	if !r.Available() {
		return fmt.Errorf("remote backend not configured or certs missing")
	}
	return nil
}

// Stop is a no-op for remote.
func (r *RemoteBackend) Stop() error {
	return nil
}

// Delete is a no-op for remote.
func (r *RemoteBackend) Delete() error {
	return fmt.Errorf("cannot delete remote server from coop")
}

// Shell opens an SSH session to the remote server (not the Incus shell).
func (r *RemoteBackend) Shell() error {
	return fmt.Errorf("shell not supported for remote backend - use ssh directly")
}

// Exec is not supported for remote backend.
func (r *RemoteBackend) Exec(_ []string) ([]byte, error) {
	return nil, fmt.Errorf("exec not supported for remote backend")
}

// GetIncusSocket returns the HTTPS URL for the remote Incus server.
func (r *RemoteBackend) GetIncusSocket() (string, error) {
	address, err := r.resolveAddress()
	if err != nil {
		return "", err
	}
	return "https://" + address, nil
}

// GetTLSCerts returns the paths to client cert, client key, and server cert.
func (r *RemoteBackend) GetTLSCerts() (clientCert, clientKey, serverCert string, err error) {
	return r.resolveCerts()
}

// resolveAddress determines the server address from config.
func (r *RemoteBackend) resolveAddress() (string, error) {
	remote := r.cfg.Settings.Remote

	// Explicit address takes precedence
	if remote.Address != "" {
		return remote.Address, nil
	}

	// Look up address from incus config.yml
	if remote.Name != "" {
		if err := names.ValidateRemoteName(remote.Name); err != nil {
			return "", err
		}

		configDir := r.incusConfigDir()
		configPath := filepath.Join(configDir, "config.yml")

		addr, err := lookupRemoteAddress(configPath, remote.Name)
		if err != nil {
			return "", fmt.Errorf("failed to lookup remote %q in %s: %w", remote.Name, configPath, err)
		}
		return addr, nil
	}

	return "", fmt.Errorf("remote address not configured")
}

// resolveCerts finds the TLS certificates for the remote connection.
func (r *RemoteBackend) resolveCerts() (clientCert, clientKey, serverCert string, err error) {
	remote := r.cfg.Settings.Remote
	configDir := r.incusConfigDir()

	// Explicit paths take precedence
	if remote.ClientCert != "" && remote.ClientKey != "" {
		clientCert = remote.ClientCert
		clientKey = remote.ClientKey
		serverCert = remote.ServerCert // May be empty - use system CA
		return r.validateCerts(clientCert, clientKey, serverCert)
	}

	// Standard incus client cert location
	clientCert = filepath.Join(configDir, "client.crt")
	clientKey = filepath.Join(configDir, "client.key")

	// Server cert is per-remote - validate name and use securejoin for defense in depth
	if remote.Name != "" {
		if err := names.ValidateRemoteName(remote.Name); err != nil {
			return "", "", "", err
		}
		serverCertsDir := filepath.Join(configDir, "servercerts")
		serverCert, err = securejoin.SecureJoin(serverCertsDir, remote.Name+".crt")
		if err != nil {
			return "", "", "", fmt.Errorf("invalid remote name path: %w", err)
		}
	}

	return r.validateCerts(clientCert, clientKey, serverCert)
}

func (r *RemoteBackend) validateCerts(clientCert, clientKey, serverCert string) (string, string, string, error) {
	// Client certs are required
	if _, err := os.Stat(clientCert); err != nil {
		return "", "", "", fmt.Errorf("client cert not found: %s", clientCert)
	}
	if _, err := os.Stat(clientKey); err != nil {
		return "", "", "", fmt.Errorf("client key not found: %s", clientKey)
	}

	// Server cert is optional - if missing, use system CA
	if serverCert != "" {
		if _, err := os.Stat(serverCert); err != nil {
			// Not an error - just clear it to use system CA
			serverCert = ""
		}
	}

	return clientCert, clientKey, serverCert, nil
}

func (r *RemoteBackend) incusConfigDir() string {
	// Explicit override from coop config
	if r.cfg.Settings.Remote.IncusConfigDir != "" {
		return r.cfg.Settings.Remote.IncusConfigDir
	}

	// Respect XDG_CONFIG_HOME (same as incus CLI)
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "incus")
	}

	// Default to ~/.config/incus
	home, err := os.UserHomeDir()
	if err != nil {
		// Last resort fallback
		return "/tmp/incus-config"
	}
	return filepath.Join(home, ".config", "incus")
}

// incusConfig represents the structure of ~/.config/incus/config.yml
type incusConfig struct {
	DefaultRemote string                 `yaml:"default-remote"`
	Remotes       map[string]incusRemote `yaml:"remotes"`
}

type incusRemote struct {
	Addr     string `yaml:"addr"`
	AuthType string `yaml:"auth_type"`
	Project  string `yaml:"project"`
	Protocol string `yaml:"protocol"`
	Public   bool   `yaml:"public"`
	Static   bool   `yaml:"static"`
}

// lookupRemoteAddress reads the incus config.yml and returns the address for a remote.
func lookupRemoteAddress(configPath, remoteName string) (string, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return "", err
	}

	var cfg incusConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return "", err
	}

	remote, ok := cfg.Remotes[remoteName]
	if !ok {
		return "", fmt.Errorf("remote %q not found", remoteName)
	}

	if remote.Addr == "" {
		return "", fmt.Errorf("remote %q has no address", remoteName)
	}

	return remote.Addr, nil
}
