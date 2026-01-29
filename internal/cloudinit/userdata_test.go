package cloudinit

import (
	"strings"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Hostname != "agent-sandbox" {
		t.Errorf("Expected Hostname=agent-sandbox, got %q", cfg.Hostname)
	}
	if cfg.AgentPort != 8888 {
		t.Errorf("Expected AgentPort=8888, got %d", cfg.AgentPort)
	}
}

func TestGenerate(t *testing.T) {
	cfg := Config{
		Hostname:  "test-sandbox",
		SSHPubKey: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAATEST test@example.com",
		AgentPort: 9999,
	}

	output, err := Generate(cfg)
	if err != nil {
		t.Fatalf("Generate() failed: %v", err)
	}

	// Must be valid cloud-config
	if !strings.HasPrefix(output, "#cloud-config") {
		t.Error("Output should start with #cloud-config")
	}

	// Hostname should be present
	if !strings.Contains(output, "hostname: test-sandbox") {
		t.Error("Output should contain hostname")
	}

	// SSH key should be included
	if !strings.Contains(output, "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAATEST") {
		t.Error("Output should contain SSH public key")
	}

	// Agent port in firewall rule
	if !strings.Contains(output, "port 9999") {
		t.Error("Output should contain agent port in firewall rule")
	}

	// User configuration
	if !strings.Contains(output, "name: agent") {
		t.Error("Output should define agent user")
	}
	if !strings.Contains(output, "uid: 1000") {
		t.Error("Agent user should have UID 1000")
	}
}

func TestGenerateWithoutSSHKey(t *testing.T) {
	cfg := Config{
		Hostname:  "no-ssh-sandbox",
		SSHPubKey: "", // No SSH key
		AgentPort: 8888,
	}

	output, err := Generate(cfg)
	if err != nil {
		t.Fatalf("Generate() failed: %v", err)
	}

	// Should not have ssh_authorized_keys section
	if strings.Contains(output, "ssh_authorized_keys") {
		t.Error("Output should not contain ssh_authorized_keys when no key provided")
	}
}

func TestGenerateContainsSecurityHardening(t *testing.T) {
	output, err := Generate(DefaultConfig())
	if err != nil {
		t.Fatalf("Generate() failed: %v", err)
	}

	// SSH hardening
	securityChecks := []string{
		"PermitRootLogin no",
		"PasswordAuthentication no",
		"PubkeyAuthentication yes",
		"X11Forwarding no",
	}

	for _, check := range securityChecks {
		if !strings.Contains(output, check) {
			t.Errorf("Output should contain security setting: %q", check)
		}
	}

	// Firewall rules
	if !strings.Contains(output, "ufw default deny incoming") {
		t.Error("Output should deny incoming by default")
	}
	if !strings.Contains(output, "ufw allow 22/tcp") {
		t.Error("Output should allow SSH on port 22")
	}
}

func TestGenerateContainsUserSetup(t *testing.T) {
	output, err := Generate(DefaultConfig())
	if err != nil {
		t.Fatalf("Generate() failed: %v", err)
	}

	// Agent user setup
	userChecks := []string{
		"name: agent",
		"uid: 1000",
		"groups: [sudo, adm]",
		"sudo: ALL=(ALL) NOPASSWD:ALL",
	}

	for _, check := range userChecks {
		if !strings.Contains(output, check) {
			t.Errorf("Output should contain user setup: %q", check)
		}
	}
}

func TestGenerateContainsDevEnvironment(t *testing.T) {
	output, err := Generate(DefaultConfig())
	if err != nil {
		t.Fatalf("Generate() failed: %v", err)
	}

	// Development environment variables
	devEnvChecks := []string{
		"GOROOT=/usr/local/go",
		"GOPATH=$HOME/go",
		"EDITOR=vim",
	}

	for _, check := range devEnvChecks {
		if !strings.Contains(output, check) {
			t.Errorf("Output should contain dev environment: %q", check)
		}
	}

	// Git config
	if !strings.Contains(output, "defaultBranch = main") {
		t.Error("Output should set git default branch to main")
	}
}

func TestGenerateSkipsPackageUpdates(t *testing.T) {
	output, err := Generate(DefaultConfig())
	if err != nil {
		t.Fatalf("Generate() failed: %v", err)
	}

	// Base image has everything, skip updates for speed
	if !strings.Contains(output, "package_update: false") {
		t.Error("Output should skip package updates")
	}
	if !strings.Contains(output, "package_upgrade: false") {
		t.Error("Output should skip package upgrades")
	}
}

func BenchmarkGenerate(b *testing.B) {
	cfg := Config{
		Hostname:  "bench-sandbox",
		SSHPubKey: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAABENCH bench@example.com",
		AgentPort: 8888,
	}

	for i := 0; i < b.N; i++ {
		_, _ = Generate(cfg)
	}
}
