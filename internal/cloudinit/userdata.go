// Package cloudinit generates cloud-init user-data for agent containers.
package cloudinit

import (
	"bytes"
	"text/template"
)

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

// Generate produces the cloud-init user-data YAML.
func Generate(cfg Config) (string, error) {
	tmpl, err := template.New("userdata").Parse(userdataTemplate)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, cfg); err != nil {
		return "", err
	}

	return buf.String(), nil
}

const userdataTemplate = `#cloud-config
# Minimal cloud-init for coop agent containers
# Base image (coop-agent-base) has all tools pre-installed

hostname: {{.Hostname}}
manage_etc_hosts: true
locale: en_US.UTF-8
timezone: UTC

users:
  - name: agent
    uid: 1000
    groups: [sudo, adm]
    shell: /bin/bash
    sudo: ALL=(ALL) NOPASSWD:ALL
{{- if .SSHPubKey }}
    ssh_authorized_keys:
      - {{.SSHPubKey}}
{{- end }}

# Skip package updates - base image has everything
package_update: false
package_upgrade: false

write_files:
  - path: /etc/ssh/sshd_config.d/99-agent-hardening.conf
    content: |
      PermitRootLogin no
      PasswordAuthentication no
      PubkeyAuthentication yes
      X11Forwarding no
      AllowAgentForwarding yes
      AllowTcpForwarding yes
      PrintMotd no
    permissions: '0644'

  - path: /home/agent/.bashrc.d/agent-env.sh
    content: |
      export GOROOT=/usr/local/go
      export GOPATH=$HOME/go
      export PATH=$HOME/.local/bin:$PATH:$GOROOT/bin:$GOPATH/bin
      export EDITOR=vim
    permissions: '0644'

  - path: /home/agent/.gitconfig
    content: |
      [user]
          name = Agent
          email = agent@sandbox.local
      [init]
          defaultBranch = main
      [safe]
          directory = *
    permissions: '0644'

runcmd:
  # Ensure agent user exists with correct shell (handles UID conflicts)
  - id agent >/dev/null 2>&1 || useradd -m -s /bin/bash -u 1000 -U agent
  - usermod -s /bin/bash agent
  - usermod -aG sudo,adm agent || true
  - 'echo "agent ALL=(ALL) NOPASSWD:ALL" > /etc/sudoers.d/agent'
  
  # Setup agent home directories
  - mkdir -p /home/agent/.bashrc.d /home/agent/.local/bin /home/agent/.config
  - mkdir -p /home/agent/.ssh /home/agent/.vscode-server /home/agent/workspace /home/agent/go/bin
  - chmod 700 /home/agent/.ssh
{{- if .SSHPubKey }}
  - 'echo "{{.SSHPubKey}}" > /home/agent/.ssh/authorized_keys'
  - chmod 600 /home/agent/.ssh/authorized_keys
{{- end }}
  - 'grep -q "bashrc.d" /home/agent/.bashrc || echo "for f in ~/.bashrc.d/*.sh; do [ -r \"$f\" ] && . \"$f\"; done" >> /home/agent/.bashrc'
  
  # Optional upgrades (non-fatal) - user version in ~/.local/bin takes precedence
  - su - agent -c 'claude upgrade' || true
  
  # Firewall setup
  - ufw default deny incoming
  - ufw default allow outgoing
  - ufw allow 22/tcp
  - ufw allow from 10.0.0.0/8 to any port {{.AgentPort}}
  - ufw --force enable
  
  # Finalize
  - systemctl restart sshd
  - chown -R agent:agent /home/agent

final_message: "Cloud-init completed in $UPTIME seconds"
`
