// Package sshkeys manages SSH keys isolated in XDG config directory.
package sshkeys

import (
	"bufio"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
// If an entry for the container already exists, it is replaced.
func WriteSSHConfig(containerName, ip string) error {
	paths := GetPaths()

	// Read existing config and filter out old entry for this container
	var filteredLines []string
	if data, err := os.ReadFile(paths.ConfigFile); err == nil {
		filteredLines = removeHostBlock(string(data), containerName)
	}

	// Build new host entry
	hostEntry := fmt.Sprintf(`# Coop agent container: %s
Host %s
    HostName %s
    User agent
    IdentityFile %s
    StrictHostKeyChecking no
    UserKnownHostsFile /dev/null
`, containerName, containerName, ip, paths.PrivateKey)

	// Combine filtered config with new entry
	var newConfig string
	if len(filteredLines) > 0 {
		newConfig = strings.Join(filteredLines, "\n") + "\n\n" + hostEntry
	} else {
		newConfig = hostEntry
	}

	if err := os.WriteFile(paths.ConfigFile, []byte(newConfig), 0600); err != nil {
		return fmt.Errorf("failed to write ssh config: %w", err)
	}

	return nil
}

// removeHostBlock removes a Host block and its preceding comment from the config.
// Returns the remaining lines.
func removeHostBlock(config, hostName string) []string {
	var result []string
	scanner := bufio.NewScanner(strings.NewReader(config))
	skipUntilNextHost := false
	var pendingComment string

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Check if this is the start of the host block we want to remove
		if strings.HasPrefix(trimmed, "Host ") {
			fields := strings.Fields(trimmed)
			if len(fields) >= 2 && fields[1] == hostName {
				// Skip this host block and any pending comment
				skipUntilNextHost = true
				pendingComment = ""
				continue
			}
			// Different host, stop skipping
			skipUntilNextHost = false
		}

		if skipUntilNextHost {
			continue
		}

		// Track comments that might belong to a host block
		if strings.HasPrefix(trimmed, "# Coop agent container:") {
			pendingComment = line
			continue
		}

		// If we have a pending comment and this isn't a Host line, keep the comment
		if pendingComment != "" {
			if !strings.HasPrefix(trimmed, "Host ") {
				result = append(result, pendingComment)
			}
			pendingComment = ""
		}

		result = append(result, line)
	}

	// Trim trailing empty lines
	for len(result) > 0 && strings.TrimSpace(result[len(result)-1]) == "" {
		result = result[:len(result)-1]
	}

	return result
}

// PrintIncludeHint prints instructions for including coop SSH config.
func PrintIncludeHint() {
	paths := GetPaths()
	fmt.Printf("\nTo use 'ssh agent-name' directly, add this to ~/.ssh/config:\n")
	fmt.Printf("  Include %s\n\n", paths.ConfigFile)
}
