// Package cloudinit generates cloud-init user-data for agent containers.
package cloudinit

import (
	"bytes"
	"embed"
	"fmt"
	"regexp"
	"strings"
	"text/template"
)

//go:embed templates/*.tmpl
var templateFS embed.FS

// Config holds the configuration for generating cloud-init user-data.
type Config struct {
	Hostname  string
	SSHPubKey string
	AgentPort int
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Hostname:  "agent-sandbox",
		AgentPort: 8888,
	}
}

// hostnameRegex validates DNS-safe hostnames (RFC 1123).
var hostnameRegex = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,61}[a-z0-9]$|^[a-z0-9]$`)

// sshPubKeyRegex validates SSH public key format (type key comment).
var sshPubKeyRegex = regexp.MustCompile(`^(ssh-ed25519|ssh-rsa|ecdsa-sha2-nistp256|ecdsa-sha2-nistp384|ecdsa-sha2-nistp521) AAAA[0-9A-Za-z+/]+=* ?[\w@.-]*$`)

// ValidateHostname checks if hostname is DNS-safe.
func ValidateHostname(hostname string) error {
	if hostname == "" {
		return fmt.Errorf("hostname cannot be empty")
	}
	if len(hostname) > 63 {
		return fmt.Errorf("hostname exceeds 63 characters")
	}
	if !hostnameRegex.MatchString(strings.ToLower(hostname)) {
		return fmt.Errorf("hostname contains invalid characters (use only a-z, 0-9, and hyphens)")
	}
	return nil
}

// ValidateSSHPubKey checks if SSH public key has valid format.
func ValidateSSHPubKey(key string) error {
	if key == "" {
		return nil // Empty key is allowed (no SSH access)
	}
	key = strings.TrimSpace(key)
	if !sshPubKeyRegex.MatchString(key) {
		return fmt.Errorf("invalid SSH public key format")
	}
	// Additional check: no shell metacharacters
	if strings.ContainsAny(key, "`$(){}[]|;&<>\n\r") {
		return fmt.Errorf("SSH public key contains forbidden characters")
	}
	return nil
}

// Generate produces the cloud-init user-data YAML.
func Generate(cfg Config) (string, error) {
	// Validate inputs to prevent injection attacks
	if err := ValidateHostname(cfg.Hostname); err != nil {
		return "", fmt.Errorf("invalid hostname: %w", err)
	}
	if err := ValidateSSHPubKey(cfg.SSHPubKey); err != nil {
		return "", fmt.Errorf("invalid SSH key: %w", err)
	}

	tmplData, err := templateFS.ReadFile("templates/userdata.yaml.tmpl")
	if err != nil {
		return "", err
	}

	tmpl, err := template.New("userdata").Parse(string(tmplData))
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, cfg); err != nil {
		return "", err
	}

	return buf.String(), nil
}
