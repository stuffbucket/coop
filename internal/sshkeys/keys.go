// Package sshkeys manages SSH keys isolated in XDG config directory.
package sshkeys

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"

	"github.com/bsmi021/coop/internal/config"
	"golang.org/x/crypto/ssh"
)

const (
	// KeyName is the default key name.
	KeyName = "id_ed25519"
)

// Paths returns the paths to the SSH key files.
type Paths struct {
	SSHDir     string // ~/.config/coop/ssh
	PrivateKey string // ~/.config/coop/ssh/id_ed25519
	PublicKey  string // ~/.config/coop/ssh/id_ed25519.pub
	ConfigFile string // ~/.config/coop/ssh/config
}

// GetPaths returns the paths for coop SSH files.
func GetPaths() Paths {
	dirs := config.GetDirectories()

	return Paths{
		SSHDir:     dirs.SSH,
		PrivateKey: filepath.Join(dirs.SSH, KeyName),
		PublicKey:  filepath.Join(dirs.SSH, KeyName+".pub"),
		ConfigFile: filepath.Join(dirs.SSH, "config"),
	}
}

// EnsureKeys creates SSH keys if they don't exist, returns the public key.
func EnsureKeys() (string, error) {
	paths := GetPaths()

	// Create directory with restricted permissions
	if err := os.MkdirAll(paths.SSHDir, 0700); err != nil {
		return "", fmt.Errorf("failed to create ssh directory: %w", err)
	}

	// Check if keys already exist
	if _, err := os.Stat(paths.PrivateKey); err == nil {
		// Keys exist, read public key
		pubKeyData, err := os.ReadFile(paths.PublicKey)
		if err != nil {
			return "", fmt.Errorf("failed to read public key: %w", err)
		}
		return string(pubKeyData), nil
	}

	// Generate new ed25519 keypair
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", fmt.Errorf("failed to generate key: %w", err)
	}

	// Convert to SSH format
	sshPubKey, err := ssh.NewPublicKey(pubKey)
	if err != nil {
		return "", fmt.Errorf("failed to create ssh public key: %w", err)
	}

	// Marshal private key to PEM
	privKeyPEM, err := ssh.MarshalPrivateKey(privKey, "coop agent sandbox key")
	if err != nil {
		return "", fmt.Errorf("failed to marshal private key: %w", err)
	}

	privKeyBytes := pem.EncodeToMemory(privKeyPEM)

	// Write private key with restricted permissions
	if err := os.WriteFile(paths.PrivateKey, privKeyBytes, 0600); err != nil {
		return "", fmt.Errorf("failed to write private key: %w", err)
	}

	// Write public key
	pubKeyStr := string(ssh.MarshalAuthorizedKey(sshPubKey))
	if err := os.WriteFile(paths.PublicKey, []byte(pubKeyStr), 0644); err != nil {
		return "", fmt.Errorf("failed to write public key: %w", err)
	}

	fmt.Printf("Generated new SSH keypair at %s\n", paths.SSHDir)

	return pubKeyStr, nil
}

// GetPublicKey returns the public key if it exists, empty string otherwise.
func GetPublicKey() string {
	paths := GetPaths()

	data, err := os.ReadFile(paths.PublicKey)
	if err != nil {
		return ""
	}

	return string(data)
}

// SSHCommand returns the SSH command with the correct identity file.
func SSHCommand(user, host string) string {
	paths := GetPaths()

	return fmt.Sprintf("ssh -i %s -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null %s@%s",
		paths.PrivateKey, user, host)
}

// SSHArgs returns the SSH arguments as a slice for use with exec.
func SSHArgs(user, host string) []string {
	paths := GetPaths()

	return []string{
		"-i", paths.PrivateKey,
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		fmt.Sprintf("%s@%s", user, host),
	}
}

// WriteSSHConfig writes/updates the SSH config for a container.
func WriteSSHConfig(containerName, ip string) error {
	paths := GetPaths()

	// Read existing config
	var existingConfig []byte
	if data, err := os.ReadFile(paths.ConfigFile); err == nil {
		existingConfig = data
	}

	// Build host entry
	hostEntry := fmt.Sprintf(`
# Coop agent container: %s
Host %s
    HostName %s
    User agent
    IdentityFile %s
    StrictHostKeyChecking no
    UserKnownHostsFile /dev/null
`, containerName, containerName, ip, paths.PrivateKey)

	// Simple append (could be smarter about updates)
	newConfig := string(existingConfig) + hostEntry

	if err := os.WriteFile(paths.ConfigFile, []byte(newConfig), 0600); err != nil {
		return fmt.Errorf("failed to write ssh config: %w", err)
	}

	return nil
}

// PrintIncludeHint prints instructions for including coop SSH config.
func PrintIncludeHint() {
	paths := GetPaths()
	fmt.Printf("\nTo use 'ssh agent-name' directly, add this to ~/.ssh/config:\n")
	fmt.Printf("  Include %s\n\n", paths.ConfigFile)
}
