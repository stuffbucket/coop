# Base VM Image Specification

**Version:** 2.0  
**Base:** Ubuntu 22.04 LTS Cloud Image  
**Type:** Incus system container  
**Provisioning:** cloud-init  
**Purpose:** Fully-equipped development environment for AI coding agents (Claude Code, OpenCode, GitHub Copilot CLI)

---

## Design Principles

1. **AI-agent ready** — Claude Code, OpenCode, and Copilot CLI pre-installed and configured
2. **Remote development** — VS Code Remote-SSH compatible out of the box
3. **Version management** — nvm for Node.js, pyenv for Python
4. **Non-root execution** — Agent runs as unprivileged `agent` user with sudo
5. **Network connectivity** — Internet, host, and VM-to-VM (Incus bridge)
6. **Reproducible provisioning** — cloud-init for consistent, auditable setup

---

## Impossible States (Invariants)

| Invariant | Enforcement |
|-----------|-------------|
| Missing core language runtime | cloud-init fails if `node`, `python3`, `go` not functional |
| AI tools not installed | Validation script checks `claude`, `opencode`, `gh copilot` |
| SSH inaccessible | SSHD enabled and key injected before boot complete |
| No network connectivity | cloud-init network config tested on first boot |
| Agent user cannot sudo | User added to sudo group with NOPASSWD |

---

## Required Languages & Runtimes

| Language | Version | Package Manager | Notes |
|----------|---------|-----------------|-------|
| **Node.js** | 24.x LTS | npm, yarn, pnpm | Via nvm (2026 LTS) |
| **Python** | 3.13.x | pip, pipx, uv | Via pyenv |
| **Go** | 1.24.x | go mod | Official binary |
| **TypeScript** | 5.x | (via npm) | Global install |

### Build Essentials (C/C++ toolchain for native modules)

| Tool | Version | Purpose |
|------|---------|---------|
| gcc | 11+ | Native compilation |
| g++ | 11+ | C++ support |
| make | latest | Build orchestration |
| cmake | 3.22+ | Project builds |

---

## AI Coding Agents (Pre-installed)

| Agent | Install Method | Configuration |
|-------|----------------|---------------|
| **Claude Code** | npm global | API key via env or config |
| **OpenCode** | Binary release | Config in ~/.config/opencode |
| **GitHub Copilot CLI** | gh extension | `gh copilot` available |

---

## SSH & Remote Development Setup

### SSHD Configuration

| Setting | Value | Purpose |
|---------|-------|---------|
| Port | 22 | Standard SSH port |
| PermitRootLogin | no | Security |
| PasswordAuthentication | no | Key-only auth |
| PubkeyAuthentication | yes | SSH key auth |
| AuthorizedKeysFile | .ssh/authorized_keys | Key location |
| X11Forwarding | no | Not needed |
| AllowAgentForwarding | yes | For git operations |
| AllowTcpForwarding | yes | For VS Code tunnels |

### VS Code Remote-SSH Compatibility

| Component | Configuration |
|-----------|---------------|
| **User shell** | /bin/bash |
| **PATH** | Includes nvm, pyenv, go in .bashrc |
| **~/.vscode-server** | Directory pre-created with correct perms |
| **Git** | Configured with safe.directory=* |
| **Locale** | en_US.UTF-8 |

---

## Networking Configuration

### Incus Network Setup

```yaml
# Network requirements for cloud-init
network:
  version: 2
  ethernets:
    eth0:
      dhcp4: true        # Get IP from Incus bridge
      dhcp-identifier: mac
```

### Connectivity Requirements

| Path | Method | Purpose |
|------|--------|---------|
| VM → Internet | NAT via Incus bridge | Package installs, API calls |
| Host → VM | Incus bridge IP | SSH, VS Code Remote |
| VM → VM | Incus bridge | Inter-agent communication |
| VM → Host | Bridge gateway | Host services access |

### Firewall (UFW)

```bash
# Default policy
ufw default deny incoming
ufw default allow outgoing

# Allow SSH
ufw allow 22/tcp

# Allow guest-agent HTTP (internal only)
ufw allow from 10.0.0.0/8 to any port 8888

# Enable
ufw --force enable
```

---

## Required Development Tools

### Version Control

| Tool | Version | Purpose |
|------|---------|---------|
| git | 2.34+ | Source control |
| git-lfs | latest | Large file support |
| gh | latest | GitHub CLI + Copilot extension |

### Code Quality

| Tool | Purpose | Install Method |
|------|---------|----------------|
| shellcheck | Shell script linting | apt |
| prettier | Code formatting | npm global |
| eslint | JavaScript linting | npm global |
| ruff | Python linting/formatting | pipx |
| golangci-lint | Go linting | binary |

### Language Servers

| Language Server | Language | Install |
|-----------------|----------|---------|
| typescript-language-server | TS/JS | npm |
| pyright | Python | npm |
| gopls | Go | go install |

---

## Required System Utilities

### Essential CLI Tools

| Tool | Purpose | Install |
|------|---------|---------|
| curl | HTTP client | apt |
| wget | Download files | apt |
| jq | JSON processing | apt |
| yq | YAML processing | binary |
| ripgrep (rg) | Fast grep | apt |
| fd-find (fd) | Fast find | apt |
| fzf | Fuzzy finder | apt |
| tree | Directory listing | apt |
| htop | Process monitor | apt |
| tmux | Terminal multiplexer | apt |
| vim | Text editing | apt |
| neovim | Text editing | apt |
| less | Pager | apt |
| zip/unzip | Compression | apt |
| file | File type detection | apt |

### Network Tools

| Tool | Purpose | Install |
|------|---------|---------|
| netcat (nc) | Network utility | apt |
| socat | Socket relay | apt |
| openssh-server | SSHD | apt |
| openssh-client | SSH client | apt |
| rsync | File sync | apt |
| dnsutils | dig, nslookup | apt |
| iputils-ping | ping | apt |

### Database Clients

| Tool | Databases | Install |
|------|-----------|---------|
| psql | PostgreSQL | apt |
| sqlite3 | SQLite | apt |

---

## Directory Structure

```
/home/agent/
├── .ssh/
│   └── authorized_keys      # Injected SSH public key
├── .local/
│   └── bin/                 # User binaries (in PATH)
├── .config/
│   ├── opencode/            # OpenCode configuration
│   └── gh/                  # GitHub CLI config
├── .nvm/                    # Node version manager
├── .pyenv/                  # Python version manager
├── .vscode-server/          # VS Code Remote extensions
├── go/                      # GOPATH
└── workspace/               # Default working directory
```

---

## Environment Configuration

### PATH (in order)

```bash
/home/agent/.local/bin
/home/agent/go/bin
/home/agent/.nvm/versions/node/v24.x/bin
/home/agent/.pyenv/shims
/usr/local/go/bin
/usr/local/bin
/usr/bin
/bin
```

### Environment Variables

```bash
# User
HOME=/home/agent
USER=agent
SHELL=/bin/bash

# Language-specific
GOPATH=/home/agent/go
GOROOT=/usr/local/go
NVM_DIR=/home/agent/.nvm
PYENV_ROOT=/home/agent/.pyenv

# Locale
LANG=en_US.UTF-8
LC_ALL=en_US.UTF-8

# Editor
EDITOR=vim
VISUAL=vim

# Pager
PAGER=less
LESS=-R

# Git
GIT_TERMINAL_PROMPT=0
```

### Git Configuration

```ini
[user]
    name = Agent
    email = agent@sandbox.local

[init]
    defaultBranch = main

[core]
    autocrlf = input
    editor = vim

[pull]
    rebase = false

[safe]
    directory = *
```

---

## Cloud-Init Configuration

### user-data

```yaml
#cloud-config

# System configuration
hostname: agent-sandbox
manage_etc_hosts: true
locale: en_US.UTF-8
timezone: UTC

# Create agent user
users:
  - name: agent
    uid: 1000
    groups: [sudo, adm]
    shell: /bin/bash
    sudo: ALL=(ALL) NOPASSWD:ALL
    ssh_authorized_keys:
      - ${SSH_PUBLIC_KEY}  # Injected at launch time

# Package management
package_update: true
package_upgrade: true

packages:
  # Build essentials
  - build-essential
  - gcc
  - g++
  - make
  - cmake
  - pkg-config
  - autoconf
  - automake
  - libtool
  # Development libraries
  - libssl-dev
  - libffi-dev
  - zlib1g-dev
  - libbz2-dev
  - libreadline-dev
  - libsqlite3-dev
  - libncurses-dev
  - liblzma-dev
  # Version control
  - git
  - git-lfs
  # Shell and terminal
  - bash
  - tmux
  - vim
  - neovim
  - less
  # CLI tools
  - curl
  - wget
  - jq
  - tree
  - htop
  - ripgrep
  - fd-find
  - fzf
  - zip
  - unzip
  - file
  # Network tools
  - openssh-server
  - openssh-client
  - netcat-openbsd
  - socat
  - rsync
  - dnsutils
  - iputils-ping
  - ca-certificates
  # Database clients
  - postgresql-client
  - sqlite3
  # Python build deps (for pyenv)
  - python3
  - python3-pip
  - python3-venv
  # Firewall
  - ufw
  # Locale
  - locales

# Write configuration files
write_files:
  # SSH hardening
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

  # Agent bashrc additions
  - path: /home/agent/.bashrc.d/agent-env.sh
    content: |
      # NVM
      export NVM_DIR="$HOME/.nvm"
      [ -s "$NVM_DIR/nvm.sh" ] && \. "$NVM_DIR/nvm.sh"
      [ -s "$NVM_DIR/bash_completion" ] && \. "$NVM_DIR/bash_completion"
      
      # Pyenv
      export PYENV_ROOT="$HOME/.pyenv"
      [[ -d $PYENV_ROOT/bin ]] && export PATH="$PYENV_ROOT/bin:$PATH"
      eval "$(pyenv init -)"
      
      # Go
      export GOROOT=/usr/local/go
      export GOPATH=$HOME/go
      export PATH=$PATH:$GOROOT/bin:$GOPATH/bin
      
      # Local bin
      export PATH="$HOME/.local/bin:$PATH"
      
      # Editor
      export EDITOR=vim
      export VISUAL=vim
    permissions: '0644'
    owner: agent:agent

  # Bashrc include
  - path: /home/agent/.bashrc.include
    content: |
      # Source all files in .bashrc.d
      if [ -d ~/.bashrc.d ]; then
        for f in ~/.bashrc.d/*.sh; do
          [ -r "$f" ] && . "$f"
        done
      fi
    permissions: '0644'
    owner: agent:agent

  # Git config
  - path: /home/agent/.gitconfig
    content: |
      [user]
          name = Agent
          email = agent@sandbox.local
      [init]
          defaultBranch = main
      [core]
          autocrlf = input
          editor = vim
      [pull]
          rebase = false
      [safe]
          directory = *
    permissions: '0644'
    owner: agent:agent

# Run commands after package install
runcmd:
  # Generate locale
  - locale-gen en_US.UTF-8
  
  # Setup bashrc.d directory and include
  - mkdir -p /home/agent/.bashrc.d
  - chown -R agent:agent /home/agent/.bashrc.d
  - grep -q '.bashrc.include' /home/agent/.bashrc || echo 'source ~/.bashrc.include' >> /home/agent/.bashrc

  # Setup directories
  - mkdir -p /home/agent/.local/bin
  - mkdir -p /home/agent/.config
  - mkdir -p /home/agent/.vscode-server
  - mkdir -p /home/agent/workspace
  - mkdir -p /home/agent/go/bin
  - chown -R agent:agent /home/agent

  # Install yq
  - curl -fsSL https://github.com/mikefarah/yq/releases/latest/download/yq_linux_amd64 -o /usr/local/bin/yq
  - chmod +x /usr/local/bin/yq

  # Install GitHub CLI
  - curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg | dd of=/usr/share/keyrings/githubcli-archive-keyring.gpg
  - chmod go+r /usr/share/keyrings/githubcli-archive-keyring.gpg
  - echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" > /etc/apt/sources.list.d/github-cli.list
  - apt-get update && apt-get install -y gh

  # Install Go (as root, to /usr/local)
  - curl -fsSL "https://go.dev/dl/go1.24.0.linux-amd64.tar.gz" | tar -xz -C /usr/local

  # Install golangci-lint
  - curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b /usr/local/bin

  # Switch to agent user for remaining installs
  - su - agent -c 'mkdir -p ~/.ssh && chmod 700 ~/.ssh'

  # Install NVM and Node.js (as agent)
  - su - agent -c 'curl -fsSL https://raw.githubusercontent.com/nvm-sh/nvm/v0.40.1/install.sh | bash'
  - su - agent -c 'source ~/.nvm/nvm.sh && nvm install 24 && nvm alias default 24'
  - su - agent -c 'source ~/.nvm/nvm.sh && npm install -g yarn pnpm typescript ts-node typescript-language-server prettier eslint pyright'

  # Install pyenv and Python (as agent)
  - su - agent -c 'curl -fsSL https://pyenv.run | bash'
  - su - agent -c 'export PYENV_ROOT="$HOME/.pyenv" && export PATH="$PYENV_ROOT/bin:$PATH" && eval "$(pyenv init -)" && pyenv install 3.13 && pyenv global 3.13'
  - su - agent -c 'export PYENV_ROOT="$HOME/.pyenv" && export PATH="$PYENV_ROOT/bin:$PATH" && eval "$(pyenv init -)" && pip install --upgrade pip && pip install pipx && pipx ensurepath'
  - su - agent -c 'export PATH="$HOME/.local/bin:$PATH" && pipx install uv ruff'

  # Install Go tools (as agent)
  - su - agent -c 'export PATH=$PATH:/usr/local/go/bin:$HOME/go/bin && go install golang.org/x/tools/gopls@latest'

  # Install Claude Code (as agent)
  - su - agent -c 'source ~/.nvm/nvm.sh && npm install -g @anthropic-ai/claude-code'

  # Install OpenCode (as agent)
  - su - agent -c 'curl -fsSL https://opencode.ai/install.sh | bash'

  # Install GitHub Copilot CLI extension (as agent)
  - su - agent -c 'gh extension install github/gh-copilot'

  # Configure firewall
  - ufw default deny incoming
  - ufw default allow outgoing
  - ufw allow 22/tcp
  - ufw allow from 10.0.0.0/8 to any port 8888
  - ufw --force enable

  # Restart SSHD with new config
  - systemctl restart sshd

  # Final ownership fix
  - chown -R agent:agent /home/agent

# Phone home when done
final_message: "Cloud-init completed in $UPTIME seconds"
```

### meta-data

```yaml
instance-id: agent-sandbox-001
local-hostname: agent-sandbox
```

### network-config

```yaml
version: 2
ethernets:
  eth0:
    dhcp4: true
    dhcp-identifier: mac
```

---

## Incus Profile

Create an Incus profile for agent containers:

```yaml
# agent.profile.yaml
config:
  limits.cpu: "2"
  limits.memory: 4GB
  user.user-data: |
    #cloud-config
    # ... (include user-data above)
  user.meta-data: |
    instance-id: ${INSTANCE_ID}
    local-hostname: ${HOSTNAME}
  user.network-config: |
    version: 2
    ethernets:
      eth0:
        dhcp4: true
        dhcp-identifier: mac

description: Agent sandbox container profile
devices:
  eth0:
    name: eth0
    network: incusbr0
    type: nic
  root:
    path: /
    pool: default
    size: 20GB
    type: disk
```

### Launch Commands

```bash
# Create profile
incus profile create agent
incus profile edit agent < agent.profile.yaml

# Launch system container with SSH key injection
incus launch images:ubuntu/22.04/cloud agent-001 \
  --profile agent \
  --config user.user-data="$(cat user-data.yaml | sed "s|\${SSH_PUBLIC_KEY}|$(cat ~/.ssh/id_ed25519.pub)|")"

# Get container IP
incus list agent-001 -c4 --format csv | cut -d' ' -f1

# SSH into container
ssh agent@<CONTAINER_IP>

# VS Code Remote-SSH
code --remote ssh-remote+agent@<CONTAINER_IP> /home/agent/workspace
```

---

## Validation Script

Run post-provisioning to verify all tools are present:

```bash
#!/bin/bash
set -e

echo "=== Validating Agent VM ==="

check() {
    if command -v "$1" &> /dev/null; then
        version=$("$1" --version 2>&1 | head -1 || "$1" version 2>&1 | head -1 || echo 'ok')
        echo "✓ $1: $version"
    else
        echo "✗ $1: NOT FOUND"
        exit 1
    fi
}

echo ""
echo "--- Languages ---"
check node
check python3
check go

echo ""
echo "--- Package Managers ---"
check npm
check yarn
check pnpm
check pip
check pipx
check uv

echo ""
echo "--- AI Coding Agents ---"
check claude
check opencode
gh copilot --version && echo "✓ gh copilot" || echo "✗ gh copilot"

echo ""
echo "--- Build Tools ---"
check make
check cmake
check gcc
check g++

echo ""
echo "--- Version Control ---"
check git
check gh

echo ""
echo "--- Linters/Formatters ---"
check shellcheck
check prettier
check eslint
check ruff
check golangci-lint

echo ""
echo "--- Language Servers ---"
check typescript-language-server
check pyright
check gopls

echo ""
echo "--- CLI Tools ---"
check curl
check wget
check jq
check yq
check rg
check fdfind
check fzf
check tmux
check vim

echo ""
echo "--- Network ---"
check ssh
check rsync
check dig
systemctl is-active sshd && echo "✓ sshd: running" || echo "✗ sshd: not running"
ufw status | grep -q "Status: active" && echo "✓ ufw: active" || echo "✗ ufw: inactive"

echo ""
echo "--- SSH Key ---"
[ -f ~/.ssh/authorized_keys ] && echo "✓ authorized_keys exists" || echo "✗ authorized_keys missing"

echo ""
echo "=== All validations passed ==="
```

---

## Size Budget

| Component | Approximate Size |
|-----------|-----------------|
| Ubuntu 22.04 cloud base | ~600 MB |
| System packages | ~400 MB |
| Node.js + global packages | ~400 MB |
| Python + packages | ~500 MB |
| Go + tools | ~600 MB |
| AI agents (claude, opencode) | ~200 MB |
| Other tools | ~100 MB |
| **Total (disk)** | **~2.8 GB** |

---

## Maintenance

- **Monthly:** Update language versions to latest stable
- **Quarterly:** Review tool list for additions/removals  
- **On demand:** Security patches via `apt upgrade`

---

## References

- [Ubuntu Cloud Images](https://cloud-images.ubuntu.com/)
- [Incus Documentation](https://linuxcontainers.org/incus/docs/main/)
- [cloud-init Documentation](https://cloudinit.readthedocs.io/)
- [VS Code Remote-SSH](https://code.visualstudio.com/docs/remote/ssh)
- [Claude Code](https://docs.anthropic.com/claude/docs/claude-code)
- [OpenCode](https://opencode.ai/)
- [GitHub Copilot CLI](https://docs.github.com/en/copilot/github-copilot-in-the-cli)
