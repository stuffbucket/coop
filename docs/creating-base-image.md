# Creating the Coop Agent Base Image

This documents how to create a pre-built base image for coop agent containers.
Using a base image with packages pre-installed reduces container startup from ~10 minutes to ~30 seconds.

## Prerequisites

- Colima with Incus profile running: `colima start --profile incus`
- Incus CLI or Web UI access

## Step 1: Create Base Container

```bash
incus launch images:ubuntu/22.04 agent-base
```

Or via Incus Web UI: Create instance → ubuntu/22.04 → Name: `agent-base`

## Step 2: Install Packages

Shell into the container:
```bash
incus exec agent-base -- bash
```

Or use the Incus Web UI terminal.

### System Setup
```bash
apt-get update
apt-get install -y software-properties-common locales
locale-gen en_US.UTF-8
```

### Add Python 3.13 PPA
```bash
add-apt-repository -y ppa:deadsnakes/ppa
apt-get update
```

### Install All Packages
```bash
apt-get install -y \
  build-essential gcc g++ make cmake pkg-config \
  libssl-dev libffi-dev zlib1g-dev libbz2-dev libreadline-dev \
  libsqlite3-dev libncurses-dev liblzma-dev \
  git git-lfs bash tmux vim neovim less curl wget jq tree htop \
  ripgrep fd-find fzf zip unzip file \
  openssh-server openssh-client netcat-openbsd socat rsync \
  dnsutils iputils-ping ca-certificates \
  postgresql-client sqlite3 \
  python3.13 python3.13-venv python3.13-dev python3-pip \
  ufw
```

### Install Go 1.24
```bash
# For ARM64 (Apple Silicon via Colima)
curl -fsSL "https://go.dev/dl/go1.24.0.linux-arm64.tar.gz" | tar -xz -C /usr/local

# For AMD64
# curl -fsSL "https://go.dev/dl/go1.24.0.linux-amd64.tar.gz" | tar -xz -C /usr/local
```

### Install GitHub CLI
```bash
curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg \
  -o /usr/share/keyrings/githubcli-archive-keyring.gpg
chmod go+r /usr/share/keyrings/githubcli-archive-keyring.gpg
echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" \
  > /etc/apt/sources.list.d/github-cli.list
apt-get update && apt-get install -y gh
```

### Install yq
```bash
# ARM64
curl -fsSL https://github.com/mikefarah/yq/releases/latest/download/yq_linux_arm64 \
  -o /usr/local/bin/yq && chmod +x /usr/local/bin/yq

# AMD64
# curl -fsSL https://github.com/mikefarah/yq/releases/latest/download/yq_linux_amd64 \
#   -o /usr/local/bin/yq && chmod +x /usr/local/bin/yq
```

### Install golangci-lint
```bash
curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh \
  | sh -s -- -b /usr/local/bin
```

### Set Python 3.13 as Default
```bash
update-alternatives --install /usr/bin/python3 python3 /usr/bin/python3.13 1
update-alternatives --set python3 /usr/bin/python3.13
```

### Clean Up (Reduce Image Size)
```bash
apt-get clean
rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/*
```

## Step 3: Publish as Image

Exit the container shell, then:

```bash
# Stop the container
incus stop agent-base

# Publish as local image
incus publish agent-base --alias coop-agent-base \
  description="Coop agent base: Ubuntu 22.04 + Python 3.13 + Go 1.24 + dev tools"

# Verify image was created
incus image list local:

# Optional: Delete the source container
incus delete agent-base
```

## Step 4: Test the Image

```bash
incus launch local:coop-agent-base test-agent
incus exec test-agent -- python3 --version   # Should show 3.13.x
incus exec test-agent -- go version           # Should show 1.24.0
incus exec test-agent -- gh --version         # Should show gh version
incus delete test-agent --force
```

## What's in the Base Image

| Component | Version | Notes |
|-----------|---------|-------|
| Ubuntu | 22.04 LTS | Base OS |
| Python | 3.13.x | From deadsnakes PPA |
| Go | 1.24.0 | In /usr/local/go |
| Node.js | 24.x | System-wide via NodeSource |
| GitHub CLI | Latest | gh command |
| Claude Code CLI | Latest | Installed via claude.ai/install.sh |
| Build tools | gcc, make, cmake | For native extensions |
| Dev utilities | vim, neovim, tmux, jq, ripgrep, fzf | CLI productivity |

## What Cloud-Init Still Does

The base image has system packages. Cloud-init handles per-container setup:

1. Creates `agent` user (UID 1000) with SSH key
2. Installs npm global packages (yarn, pnpm, typescript, etc.)
3. Configures firewall (ufw)
4. Sets up SSH server

Note: The default ubuntu user is reassigned to UID 2000 in the base image to avoid collision with the agent user at UID 1000.

## Updating the Base Image

To update the base image with new packages:

```bash
# Launch from existing image
incus launch local:coop-agent-base agent-base-update

# Make changes
incus exec agent-base-update -- bash
# ... install new packages ...
# ... apt-get clean && rm -rf /var/lib/apt/lists/* ...

# Stop and republish
incus stop agent-base-update
incus image delete local:coop-agent-base
incus publish agent-base-update --alias coop-agent-base \
  description="Coop agent base: Ubuntu 22.04 + Python 3.13 + Go 1.24 + dev tools (updated)"
incus delete agent-base-update
```
